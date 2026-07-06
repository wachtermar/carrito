package food

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alcampo-cli/internal/alcampo"
	"alcampo-cli/internal/money"
)

func TestApplyPantrySubtractsPartialQuantities(t *testing.T) {
	required := []Ingredient{
		{Name: "rice", Quantity: 180, Unit: "g", SearchTerm: "arroz"},
		{Name: "tomato", Quantity: 2, Unit: "unit"},
	}
	pantry := Pantry{Items: []PantryItem{
		{Name: "rice", Quantity: 100, Unit: "g", Location: "pantry", ExpiryDate: "2026-07-10"},
		{Name: "tomato", Quantity: 2, Unit: "unit", Location: "fridge"},
	}}
	purchases, usage := ApplyPantry(required, pantry)
	if len(usage) != 2 {
		t.Fatalf("usage len = %d, want 2: %+v", len(usage), usage)
	}
	if len(purchases) != 1 {
		t.Fatalf("purchases len = %d, want 1: %+v", len(purchases), purchases)
	}
	if purchases[0].Name != "rice" || purchases[0].Quantity != 80 || purchases[0].Unit != "g" {
		t.Fatalf("unexpected remaining purchase: %+v", purchases[0])
	}
}

func TestGenerateMealPlanPrioritizesExpiringPantryItems(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	expiry := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	plan, err := GenerateMealPlan(Profile{}, Pantry{Items: []PantryItem{
		{Name: "eggs", Quantity: 4, Unit: "unit", Location: "fridge", ExpiryDate: expiry},
	}}, PlanOptions{Days: 1, People: 2, MealTypes: []string{"dinner"}})
	if err != nil {
		t.Fatal(err)
	}
	got := plan.Days[0].Meals[0]
	if got.Recipe.ID != "spanish-tortilla" {
		t.Fatalf("first recipe = %q, want spanish-tortilla; reason=%q", got.Recipe.ID, got.PlanningReason)
	}
	if !strings.Contains(got.PlanningReason, "expiring") {
		t.Fatalf("planning reason did not mention expiry: %q", got.PlanningReason)
	}
	if !strings.Contains(strings.Join(plan.Notes, " "), "Expiring soon") {
		t.Fatalf("plan notes did not mention expiring items: %+v", plan.Notes)
	}
}

func TestGenerateMealPlanSkipsRejectedRecipes(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	expiry := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	plan, err := GenerateMealPlan(Profile{RejectedRecipes: []string{"spanish-tortilla"}}, Pantry{Items: []PantryItem{
		{Name: "eggs", Quantity: 4, Unit: "unit", Location: "fridge", ExpiryDate: expiry},
	}}, PlanOptions{Days: 1, People: 2, MealTypes: []string{"dinner"}})
	if err != nil {
		t.Fatal(err)
	}
	got := plan.Days[0].Meals[0]
	if got.Recipe.ID == "spanish-tortilla" {
		t.Fatalf("rejected recipe was selected: %+v", got)
	}
}

func TestGenerateMealPlanAddsLowStockStaples(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	plan, err := GenerateMealPlan(Profile{
		Staples: []Staple{
			{Name: "milk", MinQty: 2, Unit: "l", SearchTerm: "leche"},
		},
	}, Pantry{Items: []PantryItem{
		{Name: "milk", Quantity: 0.5, Unit: "l", Location: "fridge"},
	}}, PlanOptions{Days: 1, People: 2, MealTypes: []string{"dinner"}})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range plan.RequiredPurchases {
		if item.Name == "milk" {
			found = true
			if item.Quantity != 1.5 || item.Unit != "l" || item.SearchTerm != "leche" {
				t.Fatalf("unexpected staple restock item: %+v", item)
			}
		}
	}
	if !found {
		t.Fatalf("milk staple missing from required purchases: %+v", plan.RequiredPurchases)
	}
	if !strings.Contains(strings.Join(plan.Notes, " "), "Low-stock household staples") {
		t.Fatalf("missing staple note: %+v", plan.Notes)
	}
}

func TestSuggestUseUpRecipesRanksExpiringPantry(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	expiry := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	suggestions, err := SuggestUseUpRecipes(Profile{}, Pantry{Items: []PantryItem{
		{Name: "eggs", Quantity: 4, Unit: "unit", Location: "fridge", ExpiryDate: expiry},
	}}, 3, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(suggestions) == 0 {
		t.Fatal("no suggestions returned")
	}
	if suggestions[0].Recipe.ID != "spanish-tortilla" {
		t.Fatalf("first suggestion = %q, want spanish-tortilla", suggestions[0].Recipe.ID)
	}
	if len(suggestions[0].ExpiringItems) != 1 || suggestions[0].ExpiringItems[0] != "eggs" {
		t.Fatalf("unexpected expiring items: %+v", suggestions[0])
	}
}

func TestCookMealPlanUpdatesPantryUsage(t *testing.T) {
	pantry := Pantry{Items: []PantryItem{{Name: "rice", Quantity: 100, Unit: "g", Location: "pantry"}}}
	plan := MealPlan{
		ID: "plan-1",
		Days: []DayPlan{{
			Day: 1,
			Meals: []Meal{{
				Type:   "dinner",
				Recipe: Recipe{ID: "rice-bowl", Title: "Rice Bowl"},
			}},
		}},
		PantryUsage: []PantryUsage{
			{Ingredient: "rice", PantryItem: "rice", Quantity: 80, Unit: "g", Location: "pantry"},
		},
	}
	updated, result := CookMealPlan(plan, pantry, "good", 5)
	if len(result.Applied) != 1 || len(result.Missing) != 0 {
		t.Fatalf("unexpected cook result: %+v", result)
	}
	if len(updated.Items) != 1 || updated.Items[0].Quantity != 20 {
		t.Fatalf("pantry not reduced: %+v", updated)
	}
	event := HistoryEventForCook(result)
	if event["type"] != "mealplan_cooked" || event["mealplan_id"] != "plan-1" {
		t.Fatalf("unexpected history event: %+v", event)
	}
	if len(result.Recipes) != 1 || result.Recipes[0].ID != "rice-bowl" {
		t.Fatalf("recipe feedback missing: %+v", result.Recipes)
	}
}

func TestSelectProductCheapestRejectsUnavailableAndAllergy(t *testing.T) {
	unavailable := false
	available := true
	selection := SelectProduct(Ingredient{Name: "milk", SearchTerm: "leche"}, []alcampo.Product{
		{
			SKU:       "bad",
			Name:      "Cheap almond milk",
			Price:     money.Money{Amount: "0.50", Currency: "EUR", Cents: 50},
			UnitPrice: money.Money{Amount: "0.50", Currency: "EUR", Cents: 50},
			Available: &unavailable,
		},
		{
			SKU:       "allergy",
			Name:      "Almond milk",
			Price:     money.Money{Amount: "0.70", Currency: "EUR", Cents: 70},
			UnitPrice: money.Money{Amount: "0.70", Currency: "EUR", Cents: 70},
			Available: &available,
		},
		{
			SKU:       "ok",
			Name:      "Whole milk",
			Price:     money.Money{Amount: "1.20", Currency: "EUR", Cents: 120},
			UnitPrice: money.Money{Amount: "1.20", Currency: "EUR", Cents: 120},
			Available: &available,
			Images:    []string{"https://example.test/milk.jpg"},
		},
	}, Profile{Allergies: []string{"almond"}}, PolicyCheapest)
	if selection.Error != "" {
		t.Fatalf("selection error: %s", selection.Error)
	}
	if selection.Product.SKU != "ok" {
		t.Fatalf("selected sku = %q, want ok; selection=%+v", selection.Product.SKU, selection)
	}
	if selection.Product.ImageURL == "" || !strings.Contains(selection.SelectionReason, "available") {
		t.Fatalf("missing image or reason: %+v", selection)
	}
}

func TestSelectProductPrefersGoodOfferAndCalculatesPackages(t *testing.T) {
	available := true
	selection := SelectProduct(Ingredient{Name: "rice", Quantity: 900, Unit: "g", SearchTerm: "arroz"}, []alcampo.Product{
		{
			SKU:       "cheap",
			Name:      "Arroz redondo 500 g",
			Size:      "500 g",
			Price:     money.Money{Amount: "1.00", Currency: "EUR", Cents: 100},
			UnitPrice: money.Money{Amount: "2.00", Currency: "EUR", Cents: 200},
			Available: &available,
		},
		{
			SKU:       "offer",
			Name:      "Arroz extra 500 g",
			Size:      "500 g",
			Price:     money.Money{Amount: "1.05", Currency: "EUR", Cents: 105},
			UnitPrice: money.Money{Amount: "2.05", Currency: "EUR", Cents: 205},
			Available: &available,
			Offers:    []alcampo.Offer{{Name: "Oferta"}},
			Images:    []string{"https://example.test/rice.jpg"},
		},
	}, Profile{}, PolicyBalanced)
	if selection.Error != "" {
		t.Fatalf("selection error: %s", selection.Error)
	}
	if selection.Product.SKU != "offer" {
		t.Fatalf("selected sku = %q, want offer; selection=%+v", selection.Product.SKU, selection)
	}
	if selection.PackageCount != 2 || selection.PurchaseQuantity != "2" || selection.LineTotal.Cents != 210 {
		t.Fatalf("quantity not calculated from package size: %+v", selection)
	}
	if !strings.Contains(selection.SelectionReason, "offer available") {
		t.Fatalf("missing offer reason: %q", selection.SelectionReason)
	}
}

func TestGroupSelectedProductsBuildsCategorySubtotalsAndBasketLines(t *testing.T) {
	groups := GroupSelectedProducts([]SelectedProduct{
		{
			Ingredient:       Ingredient{Name: "rice", Category: "pantry"},
			Product:          ProductSummary{SKU: "rice-sku", Category: "pantry"},
			PurchaseQuantity: "2",
			LineTotal:        money.Money{Amount: "2.10", Currency: "EUR", Cents: 210},
		},
		{
			Ingredient:       Ingredient{Name: "tomato", Category: "vegetables"},
			Product:          ProductSummary{SKU: "tomato-sku", Category: "produce"},
			PurchaseQuantity: "1",
			LineTotal:        money.Money{Amount: "1.30", Currency: "EUR", Cents: 130},
		},
		{
			Ingredient: Ingredient{Name: "missing", Category: "pantry"},
			Error:      "no products found",
		},
	})
	if len(groups) != 2 {
		t.Fatalf("groups len = %d, want 2: %+v", len(groups), groups)
	}
	if groups[0].Category != "pantry" || groups[0].Subtotal.Cents != 210 || len(groups[0].BasketLines) != 1 {
		t.Fatalf("unexpected first group: %+v", groups[0])
	}
	if groups[1].Category != "produce" || groups[1].Subtotal.Cents != 130 || groups[1].BasketLines[0] != "tomato-sku 1 # tomato" {
		t.Fatalf("unexpected second group: %+v", groups[1])
	}
}

func TestReceiveShopResultAddsPackagesToPantryAndHistory(t *testing.T) {
	available := true
	shop := ShopResult{
		MealPlanID: "plan-receive",
		SelectedProducts: []SelectedProduct{
			{
				Ingredient:       Ingredient{Name: "rice", Category: "pantry"},
				Product:          ProductSummary{SKU: "rice-sku", Name: "Arroz redondo 500 g", Category: "pantry", Available: &available, PackageQuantity: 500, PackageUnit: "g"},
				PurchaseQuantity: "2",
				PackageCount:     2,
			},
			{
				Ingredient: Ingredient{Name: "missing"},
				Error:      "no products found",
			},
		},
	}
	pantry, result := ReceiveShopResult(shop, Pantry{}, ReceiveOptions{})
	if len(result.Applied) != 1 || len(result.Skipped) != 1 {
		t.Fatalf("unexpected receive result: %+v", result)
	}
	if len(pantry.Items) != 1 {
		t.Fatalf("pantry item not added: %+v", pantry)
	}
	item := pantry.Items[0]
	if item.Name != "rice" || item.Quantity != 1000 || item.Unit != "g" || item.Location != "pantry" {
		t.Fatalf("unexpected pantry item: %+v", item)
	}
	event := HistoryEventForReceive(result)
	if event["type"] != "shop_received" || event["mealplan_id"] != "plan-receive" {
		t.Fatalf("unexpected receive history event: %+v", event)
	}
}

func TestSelectProductUsesLikedAndRejectedProductMemory(t *testing.T) {
	available := true
	selection := SelectProduct(Ingredient{Name: "yogurt", SearchTerm: "yogur"}, []alcampo.Product{
		{
			ID:        "bad-product",
			SKU:       "bad-sku",
			Name:      "Yogur natural barato",
			Price:     money.Money{Amount: "0.50", Currency: "EUR", Cents: 50},
			UnitPrice: money.Money{Amount: "0.50", Currency: "EUR", Cents: 50},
			Available: &available,
		},
		{
			ID:        "liked-product",
			SKU:       "liked-sku",
			Name:      "Yogur natural favorito",
			Price:     money.Money{Amount: "0.70", Currency: "EUR", Cents: 70},
			UnitPrice: money.Money{Amount: "0.70", Currency: "EUR", Cents: 70},
			Available: &available,
		},
	}, Profile{
		LikedProducts:    []string{"liked-sku"},
		RejectedProducts: []string{"bad-sku"},
	}, PolicyBalanced)
	if selection.Error != "" {
		t.Fatalf("selection error: %s", selection.Error)
	}
	if selection.Product.SKU != "liked-sku" {
		t.Fatalf("selected sku = %q, want liked-sku; selection=%+v", selection.Product.SKU, selection)
	}
	if !strings.Contains(selection.SelectionReason, "liked product") {
		t.Fatalf("missing liked product reason: %q", selection.SelectionReason)
	}
	if len(selection.Alternates) == 0 || selection.Alternates[0].RejectedReason != "matches rejected product memory" {
		t.Fatalf("rejected product not explained: %+v", selection.Alternates)
	}
}

func TestParsePackageQuantityMultipack(t *testing.T) {
	qty, unit, ok := ParsePackageQuantity("6 x 1 l", "milk")
	if !ok || qty != 6 || unit != "l" {
		t.Fatalf("ParsePackageQuantity = %.3g %s %t, want 6 l true", qty, unit, ok)
	}
	qty, unit, ok = ParsePackageQuantity("12 unidades", "huevos")
	if !ok || qty != 12 || unit != "unit" {
		t.Fatalf("ParsePackageQuantity = %.3g %s %t, want 12 unit true", qty, unit, ok)
	}
}

func TestWriteSimplePDF(t *testing.T) {
	dir := t.TempDir()
	imagePath := filepath.Join(dir, "cover.png")
	writeTestPNG(t, imagePath)

	path := filepath.Join(dir, "recipe.pdf")
	if err := WriteSimplePDF(path, "Recipe", []string{"Cover image: " + imagePath, "Step 1"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "%PDF-1.4") {
		t.Fatalf("not a PDF: %q", string(data[:8]))
	}
	if !bytes.Contains(data, []byte("/Subtype /Image")) || !bytes.Contains(data, []byte("/XObject")) {
		t.Fatalf("PDF did not embed an image object")
	}
}

func TestWritePDFFromJSONFileEmbedsRelativeImage(t *testing.T) {
	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "cover.png"))
	recipe := Recipe{
		ID:          "relative-image",
		Title:       "Relative Image Recipe",
		Servings:    2,
		ImageURL:    "cover.png",
		Ingredients: []Ingredient{{Name: "rice", Quantity: 100, Unit: "g"}},
		Steps:       []RecipeStep{{Number: 1, Text: "Cook the rice."}},
	}
	data, err := json.Marshal(recipe)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(dir, "recipe.json")
	if err := os.WriteFile(input, data, 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "recipe.pdf")
	if err := WritePDFFromJSONFile(input, output); err != nil {
		t.Fatal(err)
	}
	pdfData, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pdfData, []byte("/Subtype /Image")) {
		t.Fatalf("PDF did not embed relative image")
	}
}

func writeTestPNG(t *testing.T, path string) {
	t.Helper()
	imageFile, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 220, G: 60, B: 30, A: 255})
	img.Set(1, 0, color.RGBA{R: 30, G: 160, B: 90, A: 255})
	img.Set(0, 1, color.RGBA{R: 20, G: 90, B: 180, A: 255})
	img.Set(1, 1, color.RGBA{R: 250, G: 240, B: 210, A: 255})
	if err := png.Encode(imageFile, img); err != nil {
		_ = imageFile.Close()
		t.Fatal(err)
	}
	if err := imageFile.Close(); err != nil {
		t.Fatal(err)
	}
}
