package mealplan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/money"
)

func TestWriteHTMLContainsCookingAndShoppingWithoutLegacyEvidence(t *testing.T) {
	plan := validPlan()
	product := alcampo.Product{
		ID:     "product-1",
		SKU:    "sku-1",
		Name:   "Pasta <family pack>",
		Size:   "500 g",
		URL:    "https://example.test/pasta",
		Images: []string{"https://example.test/pasta.jpg"},
		Price:  money.Money{Amount: "1.75", Currency: "EUR", Cents: 175},
	}
	build := Build{
		Plan: plan,
		Items: []SelectedItem{{
			Shopping:  plan.Shopping[0],
			Product:   product,
			LineTotal: money.Money{Amount: "3.50", Currency: "EUR", Cents: 350},
		}},
		EstimatedTotal: money.Money{Amount: "3.50", Currency: "EUR", Cents: 350},
	}
	path := filepath.Join(t.TempDir(), "share", "plan.html")
	if err := WriteHTML(path, build, time.Date(2026, 7, 9, 18, 30, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)
	for _, want := range []string{"Three easy dinners", "Tomato pasta", "Boil the pasta.", "Pasta &lt;family pack&gt;", "€3.50", "@media (max-width: 760px)", "Print / save PDF"} {
		if !strings.Contains(html, want) {
			t.Fatalf("HTML missing %q", want)
		}
	}
	for _, legacy := range []string{"artifact_audit", "readiness_gate", "Cart was not changed", "nutrition ledger"} {
		if strings.Contains(html, legacy) {
			t.Fatalf("HTML contains legacy concept %q", legacy)
		}
	}
	if len(raw) > 80_000 {
		t.Fatalf("share page is unexpectedly large: %d bytes", len(raw))
	}
}

func TestWriteHTMLUsesSingularLabelsAndNormalizesConstraintPunctuation(t *testing.T) {
	plan := validPlan()
	plan.Household.People = 1
	plan.Household.Description = "1 adult"
	plan.Household.Allergies = []string{"Peanut allergy."}
	plan.Household.Dislikes = []string{"mushrooms!"}
	plan.Days[0].Meals[0].Servings = 1
	product := alcampo.Product{
		ID:          "product-1",
		Name:        "Plain pasta",
		Size:        "500 g",
		Price:       money.Money{Amount: "1.75", Currency: "EUR", Cents: 175},
		Ingredients: "durum wheat",
	}
	build := Build{
		Plan: plan,
		Items: []SelectedItem{{
			Shopping:  plan.Shopping[0],
			Product:   product,
			LineTotal: money.Money{Amount: "3.50", Currency: "EUR", Cents: 350},
		}},
		EstimatedTotal: money.Money{Amount: "3.50", Currency: "EUR", Cents: 350},
	}
	html := renderHTML(t, build)

	for _, want := range []string{
		"1 person",
		"1 day",
		"1 meal",
		"1 serving",
		"Peanut allergy. Check every physical package before serving.",
		"<strong>Avoid:</strong> mushrooms.",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML missing %q", want)
		}
	}
	for _, unwanted := range []string{"1 people", "1 days", "1 meals", "1 servings", "allergy..", "mushrooms!."} {
		if strings.Contains(html, unwanted) {
			t.Errorf("HTML contains %q", unwanted)
		}
	}
}

func TestWriteHTMLShowsHouseholdDescriptionWithoutOtherNotes(t *testing.T) {
	plan := validPlan()
	plan.Household.Description = "2 adults and 1 toddler"
	plan.Household.Allergies = nil
	plan.Household.Dislikes = nil
	plan.Assumptions = nil
	html := renderHTML(t, Build{Plan: plan})
	if !strings.Contains(html, "<strong>Cooking for:</strong> 2 adults and 1 toddler") {
		t.Fatal("household description should remain visible when other family-note lists are empty")
	}
}

func TestWriteHTMLFormatsVariableWeightAndMissingSizePurchases(t *testing.T) {
	plan := validPlan()
	build := Build{
		Plan: plan,
		Items: []SelectedItem{
			{
				Shopping:  ShoppingItem{Name: "Tomatoes", Needed: "750 g", Packages: 0.75},
				Product:   alcampo.Product{ID: "tomatoes", Name: "Tomatoes al peso", Unit: "kg", Price: money.Money{Amount: "2.99", Currency: "EUR", Cents: 299}},
				LineTotal: money.Money{Amount: "2.24", Currency: "EUR", Cents: 224},
			},
			{
				Shopping:  ShoppingItem{Name: "Herbs", Needed: "1 bunch", Packages: 1},
				Product:   alcampo.Product{ID: "herbs", Name: "Fresh herbs", Price: money.Money{Amount: "1.00", Currency: "EUR", Cents: 100}},
				LineTotal: money.Money{Amount: "1.00", Currency: "EUR", Cents: 100},
			},
		},
		EstimatedTotal: money.Money{Amount: "3.24", Currency: "EUR", Cents: 324},
	}
	html := renderHTML(t, build)

	for _, want := range []string{"Buy 0.75 kg (variable weight)", "€2.99/kg", "Buy 1 package"} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML missing %q", want)
		}
	}
	for _, unwanted := range []string{"Buy 0.75 ×", "€2.99 each", "Buy 1 × </div>"} {
		if strings.Contains(html, unwanted) {
			t.Errorf("HTML contains malformed purchase label %q", unwanted)
		}
	}
}

func TestCookingPageCSSPreventsNarrowViewportTextOverflow(t *testing.T) {
	for _, want := range []string{
		"body { min-width: 0; max-width: 100%;",
		"grid-template-columns: minmax(0, 1fr) auto",
		".hero-grid > *, .section-title > *, .shop-item > * { min-width: 0; }",
		"overflow-wrap: anywhere",
		"word-break: break-word",
		"@media (max-width: 420px)",
		"h1 { max-width: 100%;",
	} {
		if !strings.Contains(cookingPageHTML, want) {
			t.Errorf("cooking page CSS missing narrow-viewport guard %q", want)
		}
	}
}

func renderHTML(t *testing.T, build Build) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plan.html")
	if err := WriteHTML(path, build, time.Date(2026, 7, 9, 18, 30, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
