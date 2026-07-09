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

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/money"
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

func TestGenerateMealPlanMatchesRequestedMealTypes(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	plan, err := GenerateMealPlan(Profile{}, Pantry{}, PlanOptions{Days: 1, People: 1, MealTypes: []string{"breakfast", "lunch", "dinner"}})
	if err != nil {
		t.Fatal(err)
	}
	meals := plan.Days[0].Meals
	if len(meals) != 3 {
		t.Fatalf("meal count = %d, want 3", len(meals))
	}
	for _, meal := range meals {
		if !recipeHasTag(meal.Recipe, meal.Type) {
			t.Fatalf("%s selected recipe %q with tags %+v", meal.Type, meal.Recipe.ID, meal.Recipe.Tags)
		}
	}
}

func TestGenerateMealPlanOmnivoreDoesNotRestrictMealTypeCandidates(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	expiry := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	plan, err := GenerateMealPlan(Profile{
		Diets:         []string{"omnivore"},
		LikedCuisines: []string{"mediterranean", "spanish"},
	}, Pantry{Items: []PantryItem{
		{Name: "chicken breast", Quantity: 350, Unit: "g", Location: "fridge", ExpiryDate: expiry},
		{Name: "rice", Quantity: 300, Unit: "g", Location: "pantry"},
	}}, PlanOptions{Days: 1, People: 2, MealTypes: []string{"breakfast", "lunch", "dinner"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, meal := range plan.Days[0].Meals {
		if !recipeHasTag(meal.Recipe, meal.Type) {
			t.Fatalf("%s selected recipe %q with tags %+v", meal.Type, meal.Recipe.ID, meal.Recipe.Tags)
		}
	}
}

func TestTemplatesForKnownMealTypeDoNotFallbackToWrongSlot(t *testing.T) {
	candidates := templatesForMealType([]Recipe{{
		ID:    "dinner-only",
		Title: "Dinner Only",
		Tags:  []string{"dinner"},
	}}, "breakfast")
	if len(candidates) != 0 {
		t.Fatalf("breakfast candidates fell back to dinner recipes: %+v", candidates)
	}
}

func TestGenerateMealPlanRequiresPeopleWhenProfileMissing(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	_, err := GenerateMealPlan(Profile{}, Pantry{}, PlanOptions{Days: 1, MealTypes: []string{"dinner"}})
	if err == nil {
		t.Fatal("expected missing people count error")
	}
	if !strings.Contains(err.Error(), "people count is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGenerateMealPlanVegetarianExcludesMeatAndFish(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	plan, err := GenerateMealPlan(Profile{Diets: []string{"vegetarian"}}, Pantry{}, PlanOptions{Days: 7, People: 2, MealTypes: []string{"breakfast", "lunch", "dinner"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			text := recipeText(meal.Recipe)
			if containsAnyKey(text, vegetarianForbiddenKeys) {
				t.Fatalf("vegetarian plan selected %s recipe %q with tags %+v", meal.Type, meal.Recipe.ID, meal.Recipe.Tags)
			}
		}
	}
}

func TestGenerateMealPlanVeganMatchesMealTypesAndExcludesAnimalProducts(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	plan, err := GenerateMealPlan(Profile{Diets: []string{"vegan"}}, Pantry{}, PlanOptions{Days: 2, People: 2, MealTypes: []string{"breakfast", "lunch", "dinner"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			if !recipeHasTag(meal.Recipe, meal.Type) {
				t.Fatalf("%s selected recipe %q with tags %+v", meal.Type, meal.Recipe.ID, meal.Recipe.Tags)
			}
			text := recipeText(meal.Recipe)
			if containsAnyKey(veganDietText(text), append(vegetarianForbiddenKeys, veganForbiddenKeys...)) {
				t.Fatalf("vegan plan selected %s recipe %q with tags %+v", meal.Type, meal.Recipe.ID, meal.Recipe.Tags)
			}
		}
	}
}

func TestFitsDietsAllowsVeganPlantMilksButRejectsDairy(t *testing.T) {
	coconut := Recipe{
		ID:    "coconut-curry",
		Title: "Coconut Milk Curry",
		Tags:  []string{"vegan", "dinner"},
		Ingredients: []Ingredient{
			{Name: "coconut milk"},
			{Name: "chickpeas"},
		},
	}
	if !fitsDiets(coconut, []string{"vegan"}) {
		t.Fatal("coconut milk should be allowed for vegan recipes")
	}
	dairy := Recipe{
		ID:    "dairy-soup",
		Title: "Creamy Cheese Soup",
		Tags:  []string{"vegetarian", "dinner"},
		Ingredients: []Ingredient{
			{Name: "milk"},
			{Name: "cheese"},
		},
	}
	if fitsDiets(dairy, []string{"vegan"}) {
		t.Fatal("dairy milk and cheese should be rejected for vegan recipes")
	}
}

func recipeHasTag(recipe Recipe, tag string) bool {
	for _, got := range recipe.Tags {
		if normalizeMealType(got) == normalizeMealType(tag) {
			return true
		}
	}
	return false
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

func TestSelectProductRejectsDietConflictsAndHidesRejectedAlternates(t *testing.T) {
	available := true
	selection := SelectProduct(Ingredient{Name: "vegetable stock", SearchTerm: "caldo verduras"}, []alcampo.Product{
		{
			SKU:       "chicken-stock",
			Name:      "Caldo casero de pollo y verduras",
			Price:     money.Money{Amount: "0.50", Currency: "EUR", Cents: 50},
			UnitPrice: money.Money{Amount: "0.50", Currency: "EUR", Cents: 50},
			Available: &available,
		},
		{
			SKU:       "vegetable-stock",
			Name:      "Caldo de verduras",
			Price:     money.Money{Amount: "1.20", Currency: "EUR", Cents: 120},
			UnitPrice: money.Money{Amount: "1.20", Currency: "EUR", Cents: 120},
			Available: &available,
		},
		{
			SKU:       "beef-stock",
			Name:      "Caldo de carne",
			Price:     money.Money{Amount: "1.10", Currency: "EUR", Cents: 110},
			UnitPrice: money.Money{Amount: "1.10", Currency: "EUR", Cents: 110},
			Available: &available,
		},
	}, Profile{Diets: []string{"vegetarian"}}, PolicyCheapest)
	if selection.Error != "" {
		t.Fatalf("selection error: %s", selection.Error)
	}
	if selection.Product.SKU != "vegetable-stock" {
		t.Fatalf("selected sku = %q, want vegetable-stock; selection=%+v", selection.Product.SKU, selection)
	}
	for _, alternate := range selection.Alternates {
		if alternate.RejectedReason != "" {
			t.Fatalf("rejected alternate leaked into compatible choices: %+v", selection.Alternates)
		}
		text := normalizeKey(alternate.Product.Name)
		if containsAnyKey(text, vegetarianForbiddenKeys) {
			t.Fatalf("diet-conflicting alternate leaked: %+v", alternate)
		}
	}
}

func TestSelectProductRejectsPetFoodForHumanIngredient(t *testing.T) {
	available := true
	selection := SelectProduct(Ingredient{Name: "beef strips", Quantity: 300, Unit: "g", Category: "meat", SearchTerm: "ternera tiras"}, []alcampo.Product{{
		ID:        "dog-snack-id",
		SKU:       "495007",
		Name:      "CANES JON Nature Snaks en tiras para perro con de sabor a ternera 80 g",
		Category:  "Mascotas",
		Size:      "80 g",
		Price:     money.Money{Amount: "1.89", Currency: "EUR", Cents: 189},
		UnitPrice: money.Money{Amount: "23.63", Currency: "EUR", Cents: 2363},
		Available: &available,
	}}, Profile{}, PolicyBalanced)
	if selection.Error == "" {
		t.Fatalf("pet food should not satisfy a human ingredient: %+v", selection)
	}
	if len(selection.Alternates) == 0 || !strings.Contains(selection.Alternates[0].RejectedReason, "pet or non-human food") {
		t.Fatalf("expected pet-food rejection reason, got %+v", selection.Alternates)
	}
}

func TestSelectProductRejectsAccentedDietTermsWithoutFalseHamMatch(t *testing.T) {
	available := true
	selection := SelectProduct(Ingredient{Name: "vegetables", SearchTerm: "verduras"}, []alcampo.Product{
		{
			SKU:       "tuna",
			Name:      "Verduras con atún",
			Price:     money.Money{Amount: "0.80", Currency: "EUR", Cents: 80},
			UnitPrice: money.Money{Amount: "0.80", Currency: "EUR", Cents: 80},
			Available: &available,
		},
		{
			SKU:       "mushrooms",
			Name:      "Champiñones laminados",
			Price:     money.Money{Amount: "1.00", Currency: "EUR", Cents: 100},
			UnitPrice: money.Money{Amount: "1.00", Currency: "EUR", Cents: 100},
			Available: &available,
		},
	}, Profile{Diets: []string{"vegetarian"}}, PolicyCheapest)
	if selection.Error != "" {
		t.Fatalf("selection error: %s", selection.Error)
	}
	if selection.Product.SKU != "mushrooms" {
		t.Fatalf("selected sku = %q, want mushrooms; selection=%+v", selection.Product.SKU, selection)
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

func TestBudgetStatusNoteMarksOverBudget(t *testing.T) {
	note, over := budgetStatusNote("1.00", money.Money{Amount: "1.20", Currency: "EUR", Cents: 120})
	if !over || !strings.Contains(note, "above the budget") {
		t.Fatalf("over-budget note = %q over=%t", note, over)
	}
	note, over = budgetStatusNote("2.00", money.Money{Amount: "1.20", Currency: "EUR", Cents: 120})
	if over || !strings.Contains(note, "within the budget") {
		t.Fatalf("within-budget note = %q over=%t", note, over)
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
	for _, alternate := range selection.Alternates {
		if alternate.RejectedReason != "" {
			t.Fatalf("rejected product leaked into compatible alternates: %+v", selection.Alternates)
		}
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

func TestWritePDFFromJSONFileFoodRunArtifactIncludesMealPlanAndShop(t *testing.T) {
	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "cover.png"))
	writeTestPNG(t, filepath.Join(dir, "product.png"))
	labelNutrition := &StructuredNutrition{
		Basis:     NutritionPer100g,
		BasisQty:  100,
		BasisUnit: "g",
		Source:    "alcampo_label",
		Kcal:      floatPtr(350),
		ProteinG:  floatPtr(7),
	}
	requiredNutrition := &NutritionEstimate{
		RequiredQuantity: 180,
		RequiredUnit:     "g",
		Factor:           1.8,
		Nutrients:        StructuredNutrition{Basis: NutritionPer100g, Kcal: floatPtr(630), ProteinG: floatPtr(12.6)},
		Source:           "alcampo_label",
		Confidence:       0.85,
	}
	artifact := FoodRunArtifact{
		SchemaVersion: 1,
		Kind:          "food_run",
		MealPlan: MealPlan{
			ID:              "plan-full",
			People:          2,
			BudgetEUR:       "30",
			SelectionPolicy: PolicyBalanced,
			Days: []DayPlan{{
				Day: 1,
				Meals: []Meal{{
					Type: "dinner",
					Recipe: Recipe{
						ID:          "test-dinner",
						Title:       "Test Dinner",
						Servings:    2,
						ImageURL:    "cover.png",
						Ingredients: []Ingredient{{Name: "rice", Quantity: 180, Unit: "g"}},
						Steps: []RecipeStep{
							{Number: 1, Text: "Cook the rice."},
							{Number: 2, Text: "Serve hot."},
						},
						NutritionPerServing: &NutritionSummary{Kcal: 400, ProteinG: 12, CarbsG: 70, FatG: 8},
					},
				}},
			}},
			RequiredPurchases: []Ingredient{{Name: "rice", Quantity: 180, Unit: "g"}},
			Nutrition:         &NutritionSummary{Kcal: 800, ProteinG: 24, CarbsG: 140, FatG: 16},
		},
		Shop: ShopResult{
			MealPlanID: "plan-full",
			Policy:     PolicyBalanced,
			SelectedProducts: []SelectedProduct{{
				Ingredient:        Ingredient{Name: "rice", Quantity: 180, Unit: "g"},
				Product:           ProductSummary{SKU: "rice-sku", Name: "Arroz redondo", Price: money.Money{Amount: "1.50", Currency: "EUR", Cents: 150}, ImageURL: "product.png", Offers: []string{"Producto en Folleto"}},
				PurchaseQuantity:  "1",
				PackageCount:      1,
				LineTotal:         money.Money{Amount: "1.50", Currency: "EUR", Cents: 150},
				QuantityReason:    "calculated 1 package(s): 180 g required, 500 g per package",
				SelectionReason:   "balanced value policy",
				ProductNutrition:  labelNutrition,
				RequiredNutrition: requiredNutrition,
			}},
			ProductNutritionReports: []ProductNutritionReport{{
				SKU:               "rice-sku",
				ProductName:       "Arroz redondo",
				LabelNutrition:    labelNutrition,
				RequiredNutrition: requiredNutrition,
			}},
			BasketLines:    []string{"rice-sku 1 # rice"},
			EstimatedTotal: money.Money{Amount: "1.50", Currency: "EUR", Cents: 150},
			Complete:       true,
		},
		Meals:    []string{"dinner"},
		Warnings: []string{"No order was submitted; cart and checkout writes still require explicit approval and a spending guard."},
	}
	data, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(dir, "run.json")
	if err := os.WriteFile(input, data, 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "run.pdf")
	if err := WritePDFFromJSONFile(input, output); err != nil {
		t.Fatal(err)
	}
	pdfData, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Day 1",
		"Dinner: Test Dinner",
		"Ingredients:",
		"Cooking instructions:",
		"1. Cook the rice.",
		"Selected products:",
		"Selected-product nutrition coverage: 1 of 1 ingredients",
		"Nutrition: Alcampo label, per 100 g",
		"Required nutrition estimate: 630 kcal, 12.6g protein",
		"Arroz redondo",
		"Estimated total: 1.50 EUR",
		"Safety: no order was submitted",
		"Product image",
	} {
		if !bytes.Contains(pdfData, []byte(want)) {
			t.Fatalf("PDF missing %q\n%s", want, string(pdfData))
		}
	}
	if !bytes.Contains(pdfData, []byte("/Subtype /Image")) {
		t.Fatalf("PDF did not embed run artifact images")
	}
}

func TestFoodRunArtifactRendererMakesIncompleteShoppingLoud(t *testing.T) {
	artifact := FoodRunArtifact{
		Kind: "food_run",
		MealPlan: MealPlan{
			People:          2,
			SelectionPolicy: PolicyBalanced,
			Days: []DayPlan{{
				Day: 1,
				Meals: []Meal{{
					Type: "dinner",
					Recipe: Recipe{
						ID:          "test-recipe",
						Title:       "Test Recipe",
						Servings:    2,
						Ingredients: []Ingredient{{Name: "beef strips", Quantity: 300, Unit: "g"}},
						Steps:       []RecipeStep{{Number: 1, Text: "Cook."}},
					},
				}},
			}},
			RequiredPurchases: []Ingredient{{Name: "beef strips", Quantity: 300, Unit: "g"}},
		},
		Shop: ShopResult{
			Policy:   PolicyBalanced,
			Complete: false,
			SelectedProducts: []SelectedProduct{{
				Ingredient: Ingredient{Name: "beef strips", Quantity: 300, Unit: "g"},
				Error:      "no products found",
			}},
		},
		BasketPath: "basket.txt",
	}
	_, lines := pdfLinesForFoodRunArtifact(artifact)
	text := strings.Join(lines, "\n")
	for _, want := range []string{
		"Shopping status: incomplete",
		"Missing ingredients: beef strips",
		"Estimated total excludes missing or unavailable items.",
		"- beef strips: ERROR no products found",
		"Basket safety:",
		"- No order was submitted.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("renderer missing %q\n%s", want, text)
		}
	}
	assertSectionOrder(t, lines, "Summary:", "Day-by-day meal plan:", "Ingredients to buy:", "Selected products:", "Basket safety:")
}

func TestWritePDFFromJSONFileDispatchesPlanShopRunAndRejectsUnknown(t *testing.T) {
	dir := t.TempDir()
	plan := MealPlan{
		ID:     "plan-only",
		People: 2,
		Days: []DayPlan{{
			Day: 1,
			Meals: []Meal{{
				Type:   "dinner",
				Recipe: Recipe{ID: "r", Title: "Dinner", Servings: 2, Ingredients: []Ingredient{{Name: "rice"}}, Steps: []RecipeStep{{Number: 1, Text: "Cook."}}},
			}},
		}},
	}
	shop := ShopResult{
		Policy:           PolicyBalanced,
		SelectedProducts: []SelectedProduct{{Ingredient: Ingredient{Name: "rice"}, Product: ProductSummary{Name: "Rice"}}},
		Complete:         true,
	}
	run := FoodRunArtifact{Kind: "food_run", MealPlan: plan, Shop: shop}
	for name, value := range map[string]any{
		"plan": plan,
		"shop": shop,
		"run":  run,
	} {
		input := filepath.Join(dir, name+".json")
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(input, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := WritePDFFromJSONFile(input, filepath.Join(dir, name+".pdf")); err != nil {
			t.Fatalf("%s dispatch failed: %v", name, err)
		}
	}
	unknown := filepath.Join(dir, "unknown.json")
	if err := os.WriteFile(unknown, []byte(`{"foo":"bar"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	err := WritePDFFromJSONFile(unknown, filepath.Join(dir, "unknown.pdf"))
	if err == nil || !strings.Contains(err.Error(), "not a supported food recipe, mealplan, shop, or food-run JSON file") {
		t.Fatalf("unexpected unknown JSON error: %v", err)
	}
}

func TestWritePDFFromJSONFileKeepsRenderingWhenImageMissing(t *testing.T) {
	dir := t.TempDir()
	recipe := Recipe{
		ID:          "missing-image",
		Title:       "Missing Image Recipe",
		Servings:    2,
		ImageURL:    "missing.png",
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
	output := filepath.Join(dir, "recipe.pdf")
	if err := WritePDFFromJSONFile(input, output); err != nil {
		t.Fatal(err)
	}
	pdfData, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pdfData, []byte("Cover image: missing.png")) || !bytes.Contains(pdfData, []byte("1. Cook the rice.")) {
		t.Fatalf("PDF did not keep text fallback after missing image\n%s", string(pdfData))
	}
}

func assertSectionOrder(t *testing.T, lines []string, sections ...string) {
	t.Helper()
	last := -1
	for _, section := range sections {
		pos := -1
		for i, line := range lines {
			if line == section {
				pos = i
				break
			}
		}
		if pos < 0 {
			t.Fatalf("missing section %q in %+v", section, lines)
		}
		if pos <= last {
			t.Fatalf("section %q out of order in %+v", section, lines)
		}
		last = pos
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
