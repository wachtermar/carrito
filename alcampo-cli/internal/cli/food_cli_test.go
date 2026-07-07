package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alcampo-cli/internal/food"
)

func TestFoodProfileSetRemembersSelectionPolicy(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	var stdout, stderr bytes.Buffer
	if err := Run([]string{
		"food", "profile", "set",
		"--selection-policy", "quality",
		"--people", "3",
		"--liked-products", "sku-liked",
		"--rejected-products", "sku-rejected",
		"--rejected-recipes", "lentil-stew",
		"--json",
	}, &stdout, &stderr); err != nil {
		t.Fatalf("Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	profile, err := food.LoadProfile()
	if err != nil {
		t.Fatal(err)
	}
	if profile.SelectionPolicy != food.PolicyQuality || profile.People != 3 {
		t.Fatalf("profile not saved: %+v", profile)
	}
	if len(profile.LikedProducts) != 1 || profile.LikedProducts[0] != "sku-liked" {
		t.Fatalf("liked products not saved: %+v", profile)
	}
	if len(profile.RejectedProducts) != 1 || profile.RejectedProducts[0] != "sku-rejected" {
		t.Fatalf("rejected products not saved: %+v", profile)
	}
	if len(profile.RejectedRecipes) != 1 || profile.RejectedRecipes[0] != "lentil-stew" {
		t.Fatalf("rejected recipes not saved: %+v", profile)
	}
}

func TestFoodPlanRequiresPeopleWhenProfileMissing(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	var stdout, stderr bytes.Buffer
	err := Run([]string{"food", "plan", "--days", "1", "--json"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected missing people count error; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(err.Error(), "people count is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNestedHelpReturnsSuccess(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run([]string{"food", "plan", "--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("help returned error: %v stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "Usage of food plan") {
		t.Fatalf("missing flag usage on stderr: %s", stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if err := Run([]string{"food", "profile", "set", "--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("profile help returned error: %v stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "Usage of food profile set") {
		t.Fatalf("missing profile usage on stderr: %s", stderr.String())
	}
}

func TestFoodRecipesAddShowListRemove(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	recipe := food.Recipe{
		ID:       "cli-test-recipe",
		Title:    "CLI Test Recipe",
		Servings: 2,
		Tags:     []string{"cli-test"},
		Ingredients: []food.Ingredient{
			{Name: "rice", Quantity: 100, Unit: "g", SearchTerm: "arroz"},
		},
		Steps: []food.RecipeStep{{Number: 1, Text: "Cook rice."}},
	}
	data, err := json.Marshal(recipe)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "recipe.json")
	if err := os.WriteFile(input, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run([]string{"food", "recipes", "add", input, "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("add Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if err := Run([]string{"food", "recipes", "show", "cli-test-recipe", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("show Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	var shown food.Recipe
	if err := json.Unmarshal(stdout.Bytes(), &shown); err != nil {
		t.Fatal(err)
	}
	if shown.ID != recipe.ID {
		t.Fatalf("shown recipe = %+v", shown)
	}
	stdout.Reset()
	stderr.Reset()
	if err := Run([]string{"food", "recipes", "list", "--tag", "cli-test", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("list Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	var listed []food.Recipe
	if err := json.Unmarshal(stdout.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != recipe.ID {
		t.Fatalf("listed recipes = %+v", listed)
	}
	stdout.Reset()
	stderr.Reset()
	if err := Run([]string{"food", "recipes", "remove", "cli-test-recipe", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("remove Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
}

func TestFoodRecipesAddFromText(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	input := filepath.Join(t.TempDir(), "recipe.txt")
	if err := os.WriteFile(input, []byte("2 pechugas de pollo, 1 cebolla, 200 g arroz"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run([]string{"food", "recipes", "add", input, "--from-text", "--title", "Pollo rapido", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	var result struct {
		Recipes []food.Recipe `json:"recipes"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Recipes) != 1 || result.Recipes[0].ID != "pollo-rapido" {
		t.Fatalf("unexpected result: %+v", result)
	}
	loaded, ok, err := food.LoadRecipe("pollo-rapido")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || loaded.Title != "Pollo rapido" {
		t.Fatalf("text recipe not saved: ok=%t recipe=%+v", ok, loaded)
	}
}

func TestFoodRecipesSearchAppliesProfileFilters(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	if err := food.SaveProfile(food.Profile{Diets: []string{"vegan"}, Allergies: []string{"fish"}}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run([]string{"food", "recipes", "search", "chickpea", "--profile", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	var recipes []food.Recipe
	if err := json.Unmarshal(stdout.Bytes(), &recipes); err != nil {
		t.Fatal(err)
	}
	if len(recipes) == 0 {
		t.Fatal("expected profile-compatible recipes")
	}
	for _, recipe := range recipes {
		text := strings.ToLower(strings.Join(append([]string{recipe.ID, recipe.Title}, recipe.Tags...), " "))
		if !strings.Contains(text, "vegan") {
			t.Fatalf("non-vegan recipe returned: %+v", recipe)
		}
		if strings.Contains(text, "fish") {
			t.Fatalf("fish recipe returned despite profile allergy: %+v", recipe)
		}
	}
}

func TestFoodUseUpCommandReturnsSuggestions(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	expiry := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	if err := food.SavePantry(food.Pantry{Items: []food.PantryItem{
		{Name: "eggs", Quantity: 4, Unit: "unit", Location: "fridge", ExpiryDate: expiry},
	}}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run([]string{"food", "use-up", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	var suggestions []food.UseUpSuggestion
	if err := json.Unmarshal(stdout.Bytes(), &suggestions); err != nil {
		t.Fatal(err)
	}
	if len(suggestions) == 0 || suggestions[0].Recipe.ID != "spanish-tortilla" {
		t.Fatalf("unexpected suggestions: %+v", suggestions)
	}
}

func TestFoodImportReceiptCommand(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	input := filepath.Join(t.TempDir(), "receipt.txt")
	if err := os.WriteFile(input, []byte("2 leche entera 1,80\nArroz redondo 1 kg 2,10\nTOTAL 3,90\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run([]string{"food", "import-receipt", "--file", input, "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	var result food.PantryImportResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Applied) != 2 {
		t.Fatalf("unexpected import result: %+v", result)
	}
	pantry, err := food.LoadPantry()
	if err != nil {
		t.Fatal(err)
	}
	if len(pantry.Items) != 2 {
		t.Fatalf("pantry not saved: %+v", pantry)
	}
}

func TestFoodShopUsesRememberedPolicyAndReturnsImages(t *testing.T) {
	var sawRegion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html></html>`))
		case "/api/webproductpagews/v6/product-pages/search":
			sawRegion = r.URL.Query().Get("regionId")
			if got := r.URL.Query().Get("q"); got != "arroz" {
				t.Fatalf("q = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"productGroups": []any{
					map[string]any{"decoratedProducts": []any{
						map[string]any{
							"productId":         "rice-expensive",
							"retailerProductId": "111",
							"name":              "Premium arroz",
							"brand":             "PREMIUM",
							"price":             map[string]any{"amount": "3.00", "currency": "EUR"},
							"unitPrice":         map[string]any{"amount": "3.00", "currency": "EUR"},
							"available":         true,
							"images":            []any{map[string]any{"url": "/premium.jpg"}},
						},
						map[string]any{
							"productId":         "rice-cheap",
							"retailerProductId": "222",
							"name":              "Arroz redondo",
							"brand":             "VALUE",
							"price":             map[string]any{"amount": "1.00", "currency": "EUR"},
							"unitPrice":         map[string]any{"amount": "1.00", "currency": "EUR"},
							"available":         true,
							"images":            []any{map[string]any{"url": "/value.jpg"}},
						},
					}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	writeTestConfig(t, "region-home", "dest-home")
	t.Setenv("ALCAMPO_BASE_URL", server.URL)
	var stdout, stderr bytes.Buffer
	if err := Run([]string{"food", "profile", "set", "--selection-policy", "cheapest", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("profile Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	planPath := filepath.Join(t.TempDir(), "plan.json")
	plan := food.MealPlan{
		ID:     "plan-test",
		People: 2,
		RequiredPurchases: []food.Ingredient{
			{Name: "rice", Quantity: 180, Unit: "g", SearchTerm: "arroz"},
		},
	}
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(planPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if err := Run([]string{"food", "shop", planPath, "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("shop Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	if sawRegion != "region-home" {
		t.Fatalf("search region = %q", sawRegion)
	}
	var result food.ShopResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("output was not JSON: %v\n%s", err, stdout.String())
	}
	if result.Policy != food.PolicyCheapest || len(result.SelectedProducts) != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	selected := result.SelectedProducts[0]
	if selected.Product.SKU != "222" || selected.Product.ImageURL == "" || selected.SelectionReason == "" {
		t.Fatalf("unexpected selection: %+v", selected)
	}
}

func TestFoodShopRejectsRememberedProduct(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html></html>`))
		case "/api/webproductpagews/v6/product-pages/search":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"productGroups": []any{
					map[string]any{"decoratedProducts": []any{
						map[string]any{
							"productId":         "rice-rejected",
							"retailerProductId": "222",
							"name":              "Arroz rechazado",
							"price":             map[string]any{"amount": "0.50", "currency": "EUR"},
							"unitPrice":         map[string]any{"amount": "0.50", "currency": "EUR"},
							"available":         true,
						},
						map[string]any{
							"productId":         "rice-ok",
							"retailerProductId": "333",
							"name":              "Arroz aceptado",
							"price":             map[string]any{"amount": "1.00", "currency": "EUR"},
							"unitPrice":         map[string]any{"amount": "1.00", "currency": "EUR"},
							"available":         true,
						},
					}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	writeTestConfig(t, "region-home", "dest-home")
	t.Setenv("ALCAMPO_BASE_URL", server.URL)
	var stdout, stderr bytes.Buffer
	if err := Run([]string{"food", "profile", "set", "--selection-policy", "cheapest", "--rejected-products", "222", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("profile Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	planPath := filepath.Join(t.TempDir(), "plan.json")
	plan := food.MealPlan{
		ID:     "plan-reject-product",
		People: 2,
		RequiredPurchases: []food.Ingredient{
			{Name: "rice", Quantity: 180, Unit: "g", SearchTerm: "arroz"},
		},
	}
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(planPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if err := Run([]string{"food", "shop", planPath, "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("shop Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	var result food.ShopResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("output was not JSON: %v\n%s", err, stdout.String())
	}
	if len(result.SelectedProducts) != 1 || result.SelectedProducts[0].Product.SKU != "333" {
		t.Fatalf("remembered rejection was not applied: %+v", result)
	}
	for _, alternate := range result.SelectedProducts[0].Alternates {
		if alternate.RejectedReason != "" {
			t.Fatalf("rejected alternate leaked into compatible choices: %+v", result.SelectedProducts[0])
		}
	}
}

func TestFoodRunPlansAndShopsInOneCommand(t *testing.T) {
	searches := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html></html>`))
		case "/api/webproductpagews/v6/product-pages/search":
			searches++
			q := r.URL.Query().Get("q")
			size := "1 unidad"
			if q == "pechuga de pollo" || q == "arroz" {
				size = "500 g"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"productGroups": []any{
					map[string]any{"decoratedProducts": []any{
						map[string]any{
							"productId":         "product-" + q,
							"retailerProductId": "sku-" + q,
							"name":              q + " " + size,
							"brand":             "TEST",
							"size":              size,
							"price":             map[string]any{"amount": "1.00", "currency": "EUR"},
							"unitPrice":         map[string]any{"amount": "1.00", "currency": "EUR"},
							"available":         true,
							"images":            []any{map[string]any{"url": "/" + q + ".jpg"}},
							"offers": []any{
								map[string]any{"name": "Oferta"},
							},
						},
					}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	writeTestConfig(t, "region-home", "dest-home")
	t.Setenv("ALCAMPO_BASE_URL", server.URL)
	basketPath := filepath.Join(t.TempDir(), "basket.txt")
	var stdout, stderr bytes.Buffer
	err := Run([]string{"food", "run", "--days", "1", "--people", "2", "--selection-policy", "balanced", "--basket-out", basketPath, "--json"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	var result struct {
		MealPlan food.MealPlan   `json:"mealplan"`
		Shop     food.ShopResult `json:"shop"`
		Basket   string          `json:"basket"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("output was not JSON: %v\n%s", err, stdout.String())
	}
	if result.MealPlan.ID == "" || len(result.Shop.SelectedProducts) == 0 || searches == 0 {
		t.Fatalf("run did not plan and shop: searches=%d result=%+v", searches, result)
	}
	first := result.Shop.SelectedProducts[0]
	if first.Product.ImageURL == "" || first.Product.OfferCount == 0 || first.PurchaseQuantity == "" || first.QuantityReason == "" {
		t.Fatalf("run result missing product autonomy details: %+v", first)
	}
	if result.Basket != basketPath {
		t.Fatalf("basket path missing from JSON: %+v", result)
	}
	if len(result.Shop.ShoppingGroups) == 0 || result.Shop.ShoppingGroups[0].Subtotal.Cents == 0 {
		t.Fatalf("shopping groups missing: %+v", result.Shop.ShoppingGroups)
	}
	basketData, err := os.ReadFile(basketPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(basketData), "sku-") {
		t.Fatalf("basket file missing SKU lines: %s", string(basketData))
	}
}

func TestFoodCookUpdatesPantryAndHistory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ALCAMPO_CONFIG_DIR", dir)
	if err := food.SavePantry(food.Pantry{Items: []food.PantryItem{
		{Name: "rice", Quantity: 100, Unit: "g", Location: "pantry"},
	}}); err != nil {
		t.Fatal(err)
	}
	plan, err := food.SaveMealPlan(food.MealPlan{
		ID: "plan-cook",
		PantryUsage: []food.PantryUsage{
			{Ingredient: "rice", PantryItem: "rice", Quantity: 80, Unit: "g", Location: "pantry"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run([]string{"food", "cook", plan.ID, "--rating", "5", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	var result food.CookResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("output was not JSON: %v\n%s", err, stdout.String())
	}
	if len(result.Applied) != 1 || result.Applied[0].Quantity != 80 {
		t.Fatalf("unexpected cook result: %+v", result)
	}
	pantry, err := food.LoadPantry()
	if err != nil {
		t.Fatal(err)
	}
	if len(pantry.Items) != 1 || pantry.Items[0].Quantity != 20 {
		t.Fatalf("pantry not updated: %+v", pantry)
	}
	historyPath, err := food.HistoryPath()
	if err != nil {
		t.Fatal(err)
	}
	history, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(history), "mealplan_cooked") {
		t.Fatalf("history was not written: %s", string(history))
	}
	profile, err := food.LoadProfile()
	if err != nil {
		t.Fatal(err)
	}
	if len(profile.LikedRecipes) != 0 {
		t.Fatalf("empty plan should not add liked recipes: %+v", profile)
	}
}

func TestFoodCookHighRatingLearnsLikedRecipesAndListsHistory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ALCAMPO_CONFIG_DIR", dir)
	if err := food.SavePantry(food.Pantry{}); err != nil {
		t.Fatal(err)
	}
	plan, err := food.SaveMealPlan(food.MealPlan{
		ID: "plan-liked",
		Days: []food.DayPlan{{
			Day: 1,
			Meals: []food.Meal{{
				Type: "dinner",
				Recipe: food.Recipe{
					ID:    "lentil-stew",
					Title: "Lentil And Vegetable Stew",
				},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run([]string{"food", "cook", plan.ID, "--rating", "5", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("cook Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	profile, err := food.LoadProfile()
	if err != nil {
		t.Fatal(err)
	}
	if len(profile.LikedRecipes) != 1 || profile.LikedRecipes[0] != "lentil-stew" {
		t.Fatalf("liked recipe not learned: %+v", profile)
	}
	stdout.Reset()
	stderr.Reset()
	if err := Run([]string{"food", "history", "list", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("history Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	var events []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &events); err != nil {
		t.Fatalf("history output was not JSON: %v\n%s", err, stdout.String())
	}
	if len(events) != 1 || events[0]["type"] != "mealplan_cooked" {
		t.Fatalf("unexpected history events: %+v", events)
	}
}

func TestFoodCookLowRatingLearnsRejectedRecipes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ALCAMPO_CONFIG_DIR", dir)
	if err := food.SavePantry(food.Pantry{}); err != nil {
		t.Fatal(err)
	}
	if err := food.SaveProfile(food.Profile{LikedRecipes: []string{"lentil-stew"}}); err != nil {
		t.Fatal(err)
	}
	plan, err := food.SaveMealPlan(food.MealPlan{
		ID: "plan-rejected",
		Days: []food.DayPlan{{
			Day: 1,
			Meals: []food.Meal{{
				Type: "dinner",
				Recipe: food.Recipe{
					ID:    "lentil-stew",
					Title: "Lentil And Vegetable Stew",
				},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run([]string{"food", "cook", plan.ID, "--rating", "1", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("cook Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	var result food.CookResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("cook output was not JSON: %v\n%s", err, stdout.String())
	}
	if len(result.Recipes) != 1 || result.Recipes[0].ID != "lentil-stew" {
		t.Fatalf("cook result missing recipe feedback: %+v", result)
	}
	profile, err := food.LoadProfile()
	if err != nil {
		t.Fatal(err)
	}
	if len(profile.LikedRecipes) != 0 {
		t.Fatalf("low rating did not remove liked recipe: %+v", profile)
	}
	if len(profile.RejectedRecipes) != 1 || profile.RejectedRecipes[0] != "lentil-stew" {
		t.Fatalf("low rating did not learn rejected recipe: %+v", profile)
	}
}

func TestFoodReceiveImportsRunShopIntoPantryAndHistory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ALCAMPO_CONFIG_DIR", dir)
	shop := food.ShopResult{
		MealPlanID: "plan-receive",
		SelectedProducts: []food.SelectedProduct{
			{
				Ingredient:       food.Ingredient{Name: "milk", Category: "dairy"},
				Product:          food.ProductSummary{SKU: "milk-sku", Name: "Leche 1 l", PackageQuantity: 1, PackageUnit: "l"},
				PurchaseQuantity: "2",
				PackageCount:     2,
			},
		},
	}
	runOutput := struct {
		Shop food.ShopResult `json:"shop"`
	}{Shop: shop}
	data, err := json.Marshal(runOutput)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "run.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run([]string{"food", "receive", path, "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("receive Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	var result food.ReceiveResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("receive output was not JSON: %v\n%s", err, stdout.String())
	}
	if len(result.Applied) != 1 || result.Applied[0].PantryItem.Quantity != 2 || result.Applied[0].PantryItem.Unit != "l" {
		t.Fatalf("unexpected receive result: %+v", result)
	}
	pantry, err := food.LoadPantry()
	if err != nil {
		t.Fatal(err)
	}
	if len(pantry.Items) != 1 || pantry.Items[0].Name != "milk" || pantry.Items[0].Location != "fridge" {
		t.Fatalf("pantry not updated from receive: %+v", pantry)
	}
	history, err := food.LoadHistory(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0]["type"] != "shop_received" {
		t.Fatalf("receive history missing: %+v", history)
	}
}

func TestFoodStaplesAddListRemove(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	var stdout, stderr bytes.Buffer
	if err := Run([]string{"food", "staples", "add", "milk", "--min", "2", "--unit", "l", "--search", "leche", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("add Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	profile, err := food.LoadProfile()
	if err != nil {
		t.Fatal(err)
	}
	if len(profile.Staples) != 1 || profile.Staples[0].Name != "milk" || profile.Staples[0].MinQty != 2 {
		t.Fatalf("staple not saved: %+v", profile.Staples)
	}
	stdout.Reset()
	stderr.Reset()
	if err := Run([]string{"food", "staples", "list", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("list Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	var staples []food.Staple
	if err := json.Unmarshal(stdout.Bytes(), &staples); err != nil {
		t.Fatalf("list output was not JSON: %v\n%s", err, stdout.String())
	}
	if len(staples) != 1 || staples[0].SearchTerm != "leche" {
		t.Fatalf("unexpected staples list: %+v", staples)
	}
	stdout.Reset()
	stderr.Reset()
	if err := Run([]string{"food", "staples", "remove", "milk", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("remove Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	profile, err = food.LoadProfile()
	if err != nil {
		t.Fatal(err)
	}
	if len(profile.Staples) != 0 {
		t.Fatalf("staple was not removed: %+v", profile.Staples)
	}
}
