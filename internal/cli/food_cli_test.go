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

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/food"
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

func TestFoodWriteResponsesIncludePersistedUpdatedAt(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	var stdout, stderr bytes.Buffer
	if err := Run([]string{"food", "profile", "set", "--selection-policy", "balanced", "--people", "2", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("profile set Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	var profile food.Profile
	if err := json.Unmarshal(stdout.Bytes(), &profile); err != nil {
		t.Fatalf("profile response was not JSON: %v\n%s", err, stdout.String())
	}
	persistedProfile, err := food.LoadProfile()
	if err != nil {
		t.Fatal(err)
	}
	if profile.UpdatedAt == "" || profile.UpdatedAt != persistedProfile.UpdatedAt {
		t.Fatalf("profile response updated_at=%q persisted=%q", profile.UpdatedAt, persistedProfile.UpdatedAt)
	}

	stdout.Reset()
	stderr.Reset()
	if err := Run([]string{"food", "pantry", "add", "rice", "--qty", "500", "--unit", "g", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("pantry add Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	var addResp struct {
		Action string          `json:"action"`
		Item   food.PantryItem `json:"item"`
		Pantry food.Pantry     `json:"pantry"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &addResp); err != nil {
		t.Fatalf("pantry add response was not JSON: %v\n%s", err, stdout.String())
	}
	persistedPantry, err := food.LoadPantry()
	if err != nil {
		t.Fatal(err)
	}
	if addResp.Pantry.UpdatedAt == "" || addResp.Pantry.UpdatedAt != persistedPantry.UpdatedAt {
		t.Fatalf("pantry add response updated_at=%q persisted=%q", addResp.Pantry.UpdatedAt, persistedPantry.UpdatedAt)
	}

	time.Sleep(1100 * time.Millisecond)
	stdout.Reset()
	stderr.Reset()
	if err := Run([]string{"food", "pantry", "use", "rice", "--qty", "100", "--unit", "g", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("pantry use Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	var useResp struct {
		Action string          `json:"action"`
		Item   food.PantryItem `json:"item"`
		Pantry food.Pantry     `json:"pantry"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &useResp); err != nil {
		t.Fatalf("pantry use response was not JSON: %v\n%s", err, stdout.String())
	}
	persistedPantry, err = food.LoadPantry()
	if err != nil {
		t.Fatal(err)
	}
	if useResp.Pantry.UpdatedAt == "" || useResp.Pantry.UpdatedAt != persistedPantry.UpdatedAt {
		t.Fatalf("pantry use response updated_at=%q persisted=%q", useResp.Pantry.UpdatedAt, persistedPantry.UpdatedAt)
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

func TestFoodEmptyListCommandsReturnJSONArrays(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())

	var stdout, stderr bytes.Buffer
	if err := Run([]string{"food", "pantry", "list", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("pantry list error: %v stderr=%s", err, stderr.String())
	}
	var pantry struct {
		Items []food.PantryItem `json:"items"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &pantry); err != nil {
		t.Fatalf("pantry output was not JSON: %v\n%s", err, stdout.String())
	}
	if pantry.Items == nil || len(pantry.Items) != 0 {
		t.Fatalf("pantry items should be an empty array: %+v", pantry)
	}

	stdout.Reset()
	stderr.Reset()
	if err := Run([]string{"food", "staples", "list", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("staples list error: %v stderr=%s", err, stderr.String())
	}
	var staples []food.Staple
	if err := json.Unmarshal(stdout.Bytes(), &staples); err != nil {
		t.Fatalf("staples output was not JSON: %v\n%s", err, stdout.String())
	}
	if staples == nil || len(staples) != 0 {
		t.Fatalf("staples should be an empty array: %+v", staples)
	}

	stdout.Reset()
	stderr.Reset()
	if err := Run([]string{"food", "recipes", "search", "definitely-no-such-recipe-qa", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("recipes search error: %v stderr=%s", err, stderr.String())
	}
	var recipes []food.Recipe
	if err := json.Unmarshal(stdout.Bytes(), &recipes); err != nil {
		t.Fatalf("recipes output was not JSON: %v\n%s", err, stdout.String())
	}
	if recipes == nil || len(recipes) != 0 {
		t.Fatalf("recipes should be an empty array: %+v", recipes)
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
							"nutrition":         "Valores medios por 100 g Valor energético 350 kcal Proteínas 7 g",
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
	if selected.ProductNutrition == nil || selected.RequiredNutrition == nil || len(result.ProductNutritionReports) != 1 {
		t.Fatalf("nutrition was not attached to selected product: selected=%+v reports=%+v", selected, result.ProductNutritionReports)
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
			sku := "sku-" + strings.ReplaceAll(q, " ", "-")
			size := "500 g"
			if strings.Contains(q, "salsa") {
				size = "150 ml"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"productGroups": []any{
					map[string]any{"decoratedProducts": []any{
						map[string]any{
							"productId":         "product-" + strings.ReplaceAll(q, " ", "-"),
							"retailerProductId": sku,
							"name":              q + " " + size,
							"brand":             "TEST",
							"size":              size,
							"price":             map[string]any{"amount": "1.00", "currency": "EUR"},
							"unitPrice":         map[string]any{"amount": "1.00", "currency": "EUR"},
							"available":         true,
							"images":            []any{map[string]any{"url": "/" + strings.ReplaceAll(q, " ", "-") + ".jpg"}},
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
	outputDir := t.TempDir()
	basketPath := filepath.Join(outputDir, "basket.txt")
	runPath := filepath.Join(outputDir, "run.json")
	pdfPath := filepath.Join(outputDir, "run.pdf")
	ledgerPath := filepath.Join(outputDir, "ledger.json")
	readinessPath := filepath.Join(outputDir, "readiness.json")
	var stdout, stderr bytes.Buffer
	err := Run([]string{"food", "run", "--days", "1", "--people", "2", "--selection-policy", "balanced", "--basket-out", basketPath, "--run-out", runPath, "--quantity-ledger-out", ledgerPath, "--readiness-out", readinessPath, "--pdf-out", pdfPath, "--require-safe-basket", "--json"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	var result struct {
		MealPlan       food.MealPlan       `json:"mealplan"`
		Shop           food.ShopResult     `json:"shop"`
		QuantityLedger food.QuantityLedger `json:"quantity_ledger"`
		BasketSafety   food.BasketSafety   `json:"basket_safety"`
		ReadinessGate  food.ReadinessGate  `json:"readiness_gate"`
		PDF            string              `json:"pdf"`
		Basket         string              `json:"basket"`
		Run            string              `json:"run"`
		Ledger         string              `json:"ledger"`
		Readiness      string              `json:"readiness"`
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
	if result.Run != runPath || result.PDF != pdfPath {
		t.Fatalf("run/pdf paths missing from JSON: %+v", result)
	}
	if result.Ledger != ledgerPath {
		t.Fatalf("ledger path missing from JSON: %+v", result)
	}
	if result.Readiness != readinessPath || !result.ReadinessGate.SafeToBuild || result.ReadinessGate.Status != food.ReadinessReadyExact {
		t.Fatalf("readiness missing or unsafe in JSON: %+v", result.ReadinessGate)
	}
	if len(result.Shop.ShoppingGroups) == 0 || result.Shop.ShoppingGroups[0].Subtotal.Cents == 0 {
		t.Fatalf("shopping groups missing: %+v", result.Shop.ShoppingGroups)
	}
	if result.QuantityLedger.Status == "" || result.QuantityLedger.Summary.IngredientCount == 0 || result.BasketSafety.Status == "" {
		t.Fatalf("run JSON missing quantity ledger or basket safety: %+v safety=%+v", result.QuantityLedger, result.BasketSafety)
	}
	basketData, err := os.ReadFile(basketPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(basketData), "sku-") {
		t.Fatalf("basket file missing SKU lines: %s", string(basketData))
	}
	if !strings.HasPrefix(string(basketData), "# Basket readiness: ready_exact") {
		t.Fatalf("basket file missing readiness header: %s", string(basketData))
	}
	runData, err := os.ReadFile(runPath)
	if err != nil {
		t.Fatal(err)
	}
	var artifact food.FoodRunArtifact
	if err := json.Unmarshal(runData, &artifact); err != nil {
		t.Fatalf("run artifact was not JSON: %v\n%s", err, string(runData))
	}
	if artifact.Kind != "food_run" || artifact.SchemaVersion <= 0 || len(artifact.MealPlan.Days) != 1 || len(artifact.Shop.SelectedProducts) == 0 {
		t.Fatalf("run artifact missing meal plan or shop: %+v", artifact)
	}
	if artifact.ReadinessGate == nil || !artifact.ReadinessGate.SafeToBuild {
		t.Fatalf("run artifact missing safe readiness gate: %+v", artifact.ReadinessGate)
	}
	if artifact.QuantityLedger == nil || artifact.BasketSafety == nil || len(artifact.QuantityLedger.ProductEvidence) == 0 {
		t.Fatalf("run artifact missing ledger trust data: %+v", artifact)
	}
	if artifact.PDFPath != pdfPath || artifact.BasketPath != basketPath {
		t.Fatalf("run artifact missing output paths: %+v", artifact)
	}
	if len(artifact.MealPlan.Days[0].Meals) != 1 || artifact.MealPlan.Days[0].Meals[0].Type != "dinner" {
		t.Fatalf("run artifact missing requested dinner slot: %+v", artifact.MealPlan.Days)
	}
	for _, day := range artifact.MealPlan.Days {
		for _, meal := range day.Meals {
			if meal.Recipe.Title == "" || len(meal.Recipe.Ingredients) == 0 || len(meal.Recipe.Steps) == 0 {
				t.Fatalf("run artifact meal missing recipe content: %+v", meal)
			}
		}
	}
	if len(artifact.Shop.SelectedProducts) != len(artifact.MealPlan.RequiredPurchases) {
		t.Fatalf("selected product coverage mismatch: required=%d selected=%d", len(artifact.MealPlan.RequiredPurchases), len(artifact.Shop.SelectedProducts))
	}
	if len(artifact.IngredientLinks) == 0 {
		t.Fatalf("run artifact missing ingredient traceability links")
	}
	ledgerData, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	var ledger food.QuantityLedger
	if err := json.Unmarshal(ledgerData, &ledger); err != nil {
		t.Fatalf("ledger artifact was not JSON: %v\n%s", err, string(ledgerData))
	}
	if ledger.Status == "" || len(ledger.Allocations) == 0 {
		t.Fatalf("standalone ledger missing allocations: %+v", ledger)
	}
	readinessData, err := os.ReadFile(readinessPath)
	if err != nil {
		t.Fatal(err)
	}
	var readiness food.ReadinessGate
	if err := json.Unmarshal(readinessData, &readiness); err != nil {
		t.Fatalf("readiness artifact was not JSON: %v\n%s", err, string(readinessData))
	}
	if !readiness.SafeToBuild || readiness.Status != food.ReadinessReadyExact {
		t.Fatalf("unexpected readiness artifact: %+v", readiness)
	}
	pdfData, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Day 1", "Cooking instructions:", "Basket readiness: READY_EXACT", "Shopping confidence:", "Quantity ledger:", "Selected products:", "Safety: no order was submitted"} {
		if !strings.Contains(string(pdfData), want) {
			t.Fatalf("run PDF missing %q", want)
		}
	}
}

func TestFoodRunBasketOptimizationAppliesValidatedSwitch(t *testing.T) {
	searches := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html></html>`))
		case "/api/webproductpagews/v6/product-pages/search":
			q := r.URL.Query().Get("q")
			searches[q]++
			product := map[string]any{
				"productId":         "rice-better",
				"retailerProductId": "better-sku",
				"name":              "Arroz redondo 500 g",
				"brand":             "VALUE",
				"size":              "500 g",
				"price":             map[string]any{"amount": "1.20", "currency": "EUR"},
				"unitPrice":         map[string]any{"amount": "2.40", "currency": "EUR"},
				"available":         true,
				"images":            []any{map[string]any{"url": "/better.jpg"}},
				"nutrition":         "Valores medios por 100 g Valor energético 350 kcal Proteínas 7 g",
				"offers":            []any{map[string]any{"name": "Oferta"}},
			}
			if q == "arroz" && searches[q] == 1 {
				product = map[string]any{
					"productId":         "rice-baseline",
					"retailerProductId": "base-sku",
					"name":              "Arroz redondo 1 kg",
					"brand":             "PREMIUM",
					"size":              "1 kg",
					"price":             map[string]any{"amount": "4.00", "currency": "EUR"},
					"unitPrice":         map[string]any{"amount": "4.00", "currency": "EUR"},
					"available":         true,
					"images":            []any{map[string]any{"url": "/base.jpg"}},
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"productGroups": []any{
					map[string]any{"decoratedProducts": []any{product}},
				},
			})
		case "/products/rice-sku":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"productId":         "rice-product",
				"retailerProductId": "rice-sku",
				"name":              "Arroz redondo 500 g",
				"brand":             "TEST",
				"size":              "500 g",
				"price":             map[string]any{"amount": "1.20", "currency": "EUR"},
				"unitPrice":         map[string]any{"amount": "2.40", "currency": "EUR"},
				"available":         true,
				"nutrition":         "Valores medios por 100 g Valor energético 350 kcal Proteínas 7 g",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	writeTestConfig(t, "region-home", "dest-home")
	if _, err := food.SaveUserRecipes([]food.Recipe{{
		ID:       "optimizer-rice-bowl",
		Title:    "Optimizer Rice Bowl",
		Servings: 2,
		Tags:     []string{"dinner", "quick"},
		ImageURL: "/rice-bowl.jpg",
		Ingredients: []food.Ingredient{
			{Name: "rice", Quantity: 300, Unit: "g", Category: "pantry", SearchTerm: "arroz"},
		},
		Steps: []food.RecipeStep{{Number: 1, Text: "Cook the rice."}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := food.SaveProfile(food.Profile{LikedRecipes: []string{"optimizer-rice-bowl"}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALCAMPO_BASE_URL", server.URL)
	outputDir := t.TempDir()
	basketPath := filepath.Join(outputDir, "basket.txt")
	runPath := filepath.Join(outputDir, "run.json")
	pdfPath := filepath.Join(outputDir, "run.pdf")
	readinessPath := filepath.Join(outputDir, "readiness.json")
	optimizationPath := filepath.Join(outputDir, "basket_optimization.json")
	var stdout, stderr bytes.Buffer
	err := Run([]string{"food", "run", "--days", "1", "--people", "2", "--meals", "dinner", "--selection-policy", "balanced", "--basket-out", basketPath, "--run-out", runPath, "--readiness-out", readinessPath, "--basket-optimization-out", optimizationPath, "--pdf-out", pdfPath, "--strict-quantity", "--no-recovery", "--no-recipe-swap", "--optimize-basket", "--basket-objective", "safe-balanced", "--require-safe-basket", "--json"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	if searches["arroz"] < 2 {
		t.Fatalf("optimizer did not re-search arroz: searches=%+v", searches)
	}
	var result struct {
		Shop                   food.ShopResult              `json:"shop"`
		ReadinessGate          food.ReadinessGate           `json:"readiness_gate"`
		BasketOptimizationPlan *food.BasketOptimizationPlan `json:"basket_optimization_plan,omitempty"`
		BasketOptimization     string                       `json:"basket_optimization"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("output was not JSON: %v\n%s", err, stdout.String())
	}
	if result.BasketOptimization != optimizationPath || result.BasketOptimizationPlan == nil {
		t.Fatalf("missing optimization output path or plan: %+v", result)
	}
	if result.BasketOptimizationPlan.Status != food.BasketOptimizationApplied {
		t.Fatalf("optimization plan not applied: %+v", result.BasketOptimizationPlan)
	}
	if got := result.Shop.SelectedProducts[0].Product.SKU; got != "better-sku" {
		t.Fatalf("final selected SKU = %q, want better-sku: %+v", got, result.Shop.SelectedProducts[0])
	}
	if !result.ReadinessGate.SafeToBuild || result.ReadinessGate.Status != food.ReadinessReadyExact {
		t.Fatalf("final readiness not safe: %+v", result.ReadinessGate)
	}
	optimizationData, err := os.ReadFile(optimizationPath)
	if err != nil {
		t.Fatal(err)
	}
	var optimization food.BasketOptimizationPlan
	if err := json.Unmarshal(optimizationData, &optimization); err != nil {
		t.Fatalf("optimization artifact was not JSON: %v\n%s", err, string(optimizationData))
	}
	if optimization.Status != food.BasketOptimizationApplied || optimization.AfterProductSelectionFingerprint == "" {
		t.Fatalf("optimization artifact missing applied status/fingerprint: %+v", optimization)
	}
	runData, err := os.ReadFile(runPath)
	if err != nil {
		t.Fatal(err)
	}
	var artifact food.FoodRunArtifact
	if err := json.Unmarshal(runData, &artifact); err != nil {
		t.Fatalf("run artifact was not JSON: %v\n%s", err, string(runData))
	}
	if artifact.BasketOptimizationPlan == nil || artifact.PreOptimizationReadinessGate == nil || artifact.PreOptimizationBasketSafety == nil {
		t.Fatalf("run artifact missing optimization evidence: %+v", artifact.BasketOptimizationPlan)
	}
	if artifact.ProductSelectionFingerprint != artifact.BasketOptimizationPlan.AfterProductSelectionFingerprint {
		t.Fatalf("final fingerprint mismatch: artifact=%s plan=%s", artifact.ProductSelectionFingerprint, artifact.BasketOptimizationPlan.AfterProductSelectionFingerprint)
	}
	basketData, err := os.ReadFile(basketPath)
	if err != nil {
		t.Fatal(err)
	}
	basketText := string(basketData)
	if !strings.Contains(basketText, "# Basket optimization: applied") || !strings.Contains(basketText, "better-sku 1 # rice") {
		t.Fatalf("basket file missing optimization summary or final SKU: %s", basketText)
	}
	pdfData, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Basket readiness: READY_EXACT", "Basket optimization:", "Optimizer Rice Bowl", "better-sku"} {
		if !strings.Contains(string(pdfData), want) {
			t.Fatalf("optimized run PDF missing %q", want)
		}
	}
}

func TestFoodRunDefaultPantryExcludesAssumedSalt(t *testing.T) {
	searches := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html></html>`))
		case "/api/webproductpagews/v6/product-pages/search":
			q := r.URL.Query().Get("q")
			searches[q]++
			if strings.Contains(q, "sal") || strings.Contains(q, "salt") {
				t.Fatalf("pantry-assumed salt should not be searched: q=%q", q)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"productGroups": []any{
					map[string]any{"decoratedProducts": []any{
						map[string]any{
							"productId":         "rice-product",
							"retailerProductId": "rice-sku",
							"name":              "Arroz redondo 500 g",
							"brand":             "TEST",
							"size":              "500 g",
							"price":             map[string]any{"amount": "1.20", "currency": "EUR"},
							"unitPrice":         map[string]any{"amount": "2.40", "currency": "EUR"},
							"available":         true,
							"images":            []any{map[string]any{"url": "/rice.jpg"}},
							"nutrition":         "Valores medios por 100 g Valor energético 350 kcal Proteínas 7 g",
						},
					}},
				},
			})
		case "/products/rice-sku":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body><script>window.__QUERY_INITIAL_STATE__ = {"product":{"bopData":{"productId":"rice-product","retailerProductId":"rice-sku","name":"Arroz redondo 500 g","brand":"TEST","size":"500 g","price":{"amount":"1.20","currency":"EUR"},"unitPrice":{"amount":"2.40","currency":"EUR"},"available":true,"nutrition":"Valores medios por 100 g Valor energético 350 kcal Proteínas 7 g"}}};</script></body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	writeTestConfig(t, "region-home", "dest-home")
	if _, err := food.SaveUserRecipes([]food.Recipe{{
		ID:       "pantry-rice",
		Title:    "Pantry Rice",
		Servings: 2,
		Tags:     []string{"dinner", "pantryfixture"},
		Ingredients: []food.Ingredient{
			{Name: "rice", Quantity: 300, Unit: "g", SearchTerm: "arroz"},
			{Name: "salt", Quantity: 2, Unit: "g", SearchTerm: "sal"},
		},
		Steps: []food.RecipeStep{{Number: 1, Text: "Cook rice with salt."}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := food.SaveProfile(food.Profile{LikedRecipes: []string{"pantry-rice", "pantryfixture"}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALCAMPO_BASE_URL", server.URL)
	outputDir := t.TempDir()
	basketPath := filepath.Join(outputDir, "basket.txt")
	runPath := filepath.Join(outputDir, "run.json")
	pantryPath := filepath.Join(outputDir, "pantry.json")
	pdfPath := filepath.Join(outputDir, "run.pdf")
	var stdout, stderr bytes.Buffer
	err := Run([]string{"food", "run", "--days", "1", "--people", "2", "--meals", "dinner", "--selection-policy", "balanced", "--basket-out", basketPath, "--run-out", runPath, "--pantry-out", pantryPath, "--pdf-out", pdfPath, "--default-pantry", "minimal-spanish", "--allow-assumed-pantry", "--strict-quantity", "--require-safe-basket", "--require-cook-ready", "--json"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	if searches["arroz"] == 0 {
		t.Fatalf("rice should have been searched: searches=%+v", searches)
	}
	var result struct {
		ReadinessGate    food.ReadinessGate     `json:"readiness_gate"`
		PantryResolution *food.PantryResolution `json:"pantry_resolution,omitempty"`
		Shop             food.ShopResult        `json:"shop"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("output was not JSON: %v\n%s", err, stdout.String())
	}
	if !result.ReadinessGate.SafeToBuild || !result.ReadinessGate.SafeToCook || result.ReadinessGate.CookReadinessStatus != string(food.ReadinessReadyWithPantryAssumptions) {
		t.Fatalf("unexpected readiness: %+v", result.ReadinessGate)
	}
	if result.PantryResolution == nil || result.PantryResolution.Summary.PantryAssumedLines != 1 {
		t.Fatalf("missing pantry assumption: %+v", result.PantryResolution)
	}
	if len(result.Shop.SelectedProducts) != 1 || result.Shop.SelectedProducts[0].Ingredient.SearchTerm != "arroz" {
		t.Fatalf("shop should contain only rice delta: %+v", result.Shop.SelectedProducts)
	}
	pantryData, err := os.ReadFile(pantryPath)
	if err != nil {
		t.Fatal(err)
	}
	var pantryResolution food.PantryResolution
	if err := json.Unmarshal(pantryData, &pantryResolution); err != nil {
		t.Fatalf("pantry artifact was not JSON: %v\n%s", err, string(pantryData))
	}
	if pantryResolution.Status != food.PantryResolutionAppliedWithAssumptions {
		t.Fatalf("unexpected pantry artifact: %+v", pantryResolution)
	}
	basketData, err := os.ReadFile(basketPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(basketData), "Cooking readiness: ready_with_pantry_assumptions") || !strings.Contains(string(basketData), "salt (assumed pantry)") {
		t.Fatalf("basket missing pantry/cook summary: %s", string(basketData))
	}
	pdfData, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Cooking readiness: READY_WITH_PANTRY_ASSUMPTIONS", "Pantry and shopping delta:", "salt: assumed pantry staple"} {
		if !strings.Contains(string(pdfData), want) {
			t.Fatalf("pantry PDF missing %q", want)
		}
	}
}

func TestFoodRunServingScalingWritesArtifacts(t *testing.T) {
	searches := map[string]int{}
	detailHits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html></html>`))
		case "/api/webproductpagews/v6/product-pages/search":
			q := r.URL.Query().Get("q")
			searches[q]++
			if strings.Contains(q, "sal") || strings.Contains(q, "salt") {
				t.Fatalf("pantry-assumed salt should not be searched: q=%q", q)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"productGroups": []any{
					map[string]any{"decoratedProducts": []any{
						map[string]any{
							"productId":         "rice-product",
							"retailerProductId": "rice-sku",
							"name":              "Arroz redondo 500 g",
							"brand":             "TEST",
							"size":              "500 g",
							"price":             map[string]any{"amount": "1.20", "currency": "EUR"},
							"unitPrice":         map[string]any{"amount": "2.40", "currency": "EUR"},
							"available":         true,
							"images":            []any{map[string]any{"url": "/rice.jpg"}},
						},
					}},
				},
			})
		case "/products/rice-sku":
			detailHits++
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body><script>window.__QUERY_INITIAL_STATE__ = {"product":{"bopData":{"productId":"rice-product","retailerProductId":"rice-sku","name":"Arroz redondo 500 g","brand":"TEST","size":"500 g","price":{"amount":"1.20","currency":"EUR"},"unitPrice":{"amount":"2.40","currency":"EUR"},"available":true,"nutrition":"Valores medios por 100 g Valor energético 350 kcal Proteínas 7 g"}}};</script></body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	writeTestConfig(t, "region-home", "dest-home")
	if _, err := food.SaveUserRecipes([]food.Recipe{{
		ID:       "scaled-pantry-rice",
		Title:    "Scaled Pantry Rice",
		Servings: 4,
		Tags:     []string{"dinner", "scaledfixture"},
		Ingredients: []food.Ingredient{
			{Name: "rice", Quantity: 400, Unit: "g", SearchTerm: "arroz"},
			{Name: "salt", Quantity: 2, Unit: "g", SearchTerm: "sal"},
		},
		Steps: []food.RecipeStep{{Number: 1, Text: "Cook rice with salt."}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := food.SaveProfile(food.Profile{LikedRecipes: []string{"scaled-pantry-rice", "scaledfixture"}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALCAMPO_BASE_URL", server.URL)
	outputDir := t.TempDir()
	basketPath := filepath.Join(outputDir, "basket.txt")
	runPath := filepath.Join(outputDir, "run.json")
	servingPath := filepath.Join(outputDir, "serving.json")
	scaledPath := filepath.Join(outputDir, "scaled.json")
	nutritionPath := filepath.Join(outputDir, "nutrition.json")
	pantryPath := filepath.Join(outputDir, "pantry.json")
	pdfPath := filepath.Join(outputDir, "run.pdf")
	var stdout, stderr bytes.Buffer
	err := Run([]string{"food", "run", "--days", "1", "--people", "4", "--meals", "dinner", "--selection-policy", "balanced", "--servings", "2.5", "--basket-out", basketPath, "--run-out", runPath, "--serving-plan-out", servingPath, "--scaled-mealplan-out", scaledPath, "--nutrition-ledger-out", nutritionPath, "--pantry-out", pantryPath, "--pdf-out", pdfPath, "--enrich-products", "--default-pantry", "minimal-spanish", "--allow-assumed-pantry", "--strict-quantity", "--require-safe-basket", "--require-cook-ready", "--json"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	if searches["arroz"] == 0 {
		t.Fatalf("rice should have been searched: searches=%+v", searches)
	}
	if detailHits == 0 {
		t.Fatalf("rice detail page should have been fetched")
	}
	var result struct {
		ReadinessGate    food.ReadinessGate     `json:"readiness_gate"`
		ServingPlan      *food.ServingPlan      `json:"serving_plan,omitempty"`
		ScaledMealPlan   *food.ScaledMealPlan   `json:"scaled_mealplan,omitempty"`
		NutritionLedger  *food.NutritionLedger  `json:"nutrition_ledger,omitempty"`
		PantryResolution *food.PantryResolution `json:"pantry_resolution,omitempty"`
		Shop             food.ShopResult        `json:"shop"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("output was not JSON: %v\n%s", err, stdout.String())
	}
	if result.ServingPlan == nil || result.ServingPlan.Summary.TotalTargetServingUnits != 2.5 {
		t.Fatalf("missing serving plan: %+v", result.ServingPlan)
	}
	if result.ScaledMealPlan == nil || result.ScaledMealPlan.Slots[0].ScaleFactor != 0.625 {
		t.Fatalf("missing scaled meal plan: %+v", result.ScaledMealPlan)
	}
	if len(result.Shop.SelectedProducts) != 1 || result.Shop.SelectedProducts[0].Ingredient.Quantity != 250 {
		t.Fatalf("shop should contain 250g scaled rice only: %+v", result.Shop.SelectedProducts)
	}
	if !result.ReadinessGate.SafeToBuild || !result.ReadinessGate.SafeToCook || result.ReadinessGate.ServingStatus != string(food.ServingPlanExplicitServings) {
		t.Fatalf("unexpected readiness: %+v", result.ReadinessGate)
	}
	if result.NutritionLedger == nil || result.NutritionLedger.Status != food.NutritionLedgerPartial || result.NutritionLedger.Coverage.PantryMissingLines != 1 {
		t.Fatalf("unexpected nutrition ledger: %+v", result.NutritionLedger)
	}
	if result.NutritionLedger.TotalConsumedKnown == nil || result.NutritionLedger.TotalConsumedKnown.Facts.EnergyKcal == nil || result.NutritionLedger.TotalConsumedKnown.Facts.EnergyKcal.Value != 875 {
		t.Fatalf("nutrition should count 250g rice only: total=%+v ledger=%+v", result.NutritionLedger.TotalConsumedKnown, result.NutritionLedger)
	}
	scaledData, err := os.ReadFile(scaledPath)
	if err != nil {
		t.Fatal(err)
	}
	var scaledArtifact food.ScaledMealPlan
	if err := json.Unmarshal(scaledData, &scaledArtifact); err != nil {
		t.Fatalf("scaled artifact was not JSON: %v\n%s", err, string(scaledData))
	}
	if scaledArtifact.Slots[0].Ingredients[0].ShoppingQuantity.BaseValue != 250 {
		t.Fatalf("scaled artifact did not persist 250g rice: %+v", scaledArtifact.Slots[0].Ingredients[0])
	}
	nutritionData, err := os.ReadFile(nutritionPath)
	if err != nil {
		t.Fatal(err)
	}
	var nutritionArtifact food.NutritionLedger
	if err := json.Unmarshal(nutritionData, &nutritionArtifact); err != nil {
		t.Fatalf("nutrition artifact was not JSON: %v\n%s", err, string(nutritionData))
	}
	if nutritionArtifact.NutritionLedgerFingerprint == "" || nutritionArtifact.ProductSelectionFingerprint == "" {
		t.Fatalf("nutrition artifact missing fingerprints: %+v", nutritionArtifact)
	}
	basketData, err := os.ReadFile(basketPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(basketData), "Serving plan: explicit_servings") || !strings.Contains(string(basketData), "salt (assumed pantry)") || !strings.Contains(string(basketData), "Nutrition evidence: partial") {
		t.Fatalf("basket missing serving or pantry summary: %s", string(basketData))
	}
	pdfData, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Servings and scaling:", "Target serving units: 2.5", "Nutrition evidence:", "875 kcal", "Scaled Pantry Rice"} {
		if !strings.Contains(string(pdfData), want) {
			t.Fatalf("serving PDF missing %q", want)
		}
	}
}

func TestFoodRunIntentCompletePDFSeesRequestedPDFPath(t *testing.T) {
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
							"productId":         "rice-product",
							"retailerProductId": "rice-sku",
							"name":              "Arroz redondo 500 g",
							"brand":             "TEST",
							"size":              "500 g",
							"price":             map[string]any{"amount": "1.20", "currency": "EUR"},
							"unitPrice":         map[string]any{"amount": "2.40", "currency": "EUR"},
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
	if _, err := food.SaveUserRecipes([]food.Recipe{{
		ID:       "intent-pdf-rice",
		Title:    "Intent PDF Rice",
		Servings: 2,
		Tags:     []string{"dinner", "intentpdffixture"},
		Ingredients: []food.Ingredient{
			{Name: "rice", Quantity: 250, Unit: "g", SearchTerm: "arroz"},
		},
		Steps: []food.RecipeStep{{Number: 1, Text: "Cook rice."}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := food.SaveProfile(food.Profile{LikedRecipes: []string{"intent-pdf-rice", "intentpdffixture"}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALCAMPO_BASE_URL", server.URL)

	outputDir := t.TempDir()
	intentPath := filepath.Join(outputDir, "intent.json")
	runPath := filepath.Join(outputDir, "run.json")
	pdfPath := filepath.Join(outputDir, "run.pdf")
	constraintPath := filepath.Join(outputDir, "constraint.json")
	basketPath := filepath.Join(outputDir, "basket.txt")
	intent := food.MealRunIntent{
		SchemaVersion: "1",
		Source:        "test",
		RecipePDF:     &food.RecipePDFIntent{RequireCookingSteps: true, RequireCompletePDF: true},
		ClaimPolicy: food.IntentClaimPolicy{
			RequireAllHardConstraints:  true,
			UnknownHardConstraintFails: true,
			AllowSoftPreferenceMisses:  true,
		},
	}
	intentData, err := json.Marshal(intent)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(intentPath, intentData, 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err = Run([]string{"food", "run", "--days", "1", "--people", "2", "--meals", "dinner", "--selection-policy", "balanced", "--basket-out", basketPath, "--run-out", runPath, "--pdf-out", pdfPath, "--intent-file", intentPath, "--constraint-report-out", constraintPath, "--strict-quantity", "--require-safe-basket", "--require-cook-ready", "--require-intent-ready", "--json"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	var result struct {
		PDF                          string                             `json:"pdf"`
		ConstraintSatisfactionReport *food.ConstraintSatisfactionReport `json:"constraint_satisfaction_report,omitempty"`
		ReadinessGate                food.ReadinessGate                 `json:"readiness_gate"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("output was not JSON: %v\n%s", err, stdout.String())
	}
	if result.PDF != pdfPath || result.ConstraintSatisfactionReport == nil || !result.ConstraintSatisfactionReport.ClaimGuard.MayClaimRequestSatisfied || !result.ReadinessGate.SafeToSatisfyIntent {
		t.Fatalf("PDF intent should be satisfied: %+v", result)
	}
	var artifact food.FoodRunArtifact
	readJSONFile(t, runPath, &artifact)
	if artifact.PDFPath != pdfPath {
		t.Fatalf("run artifact PDF path = %q, want %q", artifact.PDFPath, pdfPath)
	}
	var report food.ConstraintSatisfactionReport
	readJSONFile(t, constraintPath, &report)
	if report.Status != food.ConstraintSatisfactionPass || !report.ClaimGuard.MayClaimPDFComplete {
		t.Fatalf("constraint report should pass PDF completeness: %+v", report)
	}
}

func TestFoodRunRequireNutritionReadyKeepsSafeBasket(t *testing.T) {
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
							"productId":         "rice-product",
							"retailerProductId": "rice-sku",
							"name":              "Arroz redondo 500 g",
							"brand":             "TEST",
							"size":              "500 g",
							"price":             map[string]any{"amount": "1.20", "currency": "EUR"},
							"unitPrice":         map[string]any{"amount": "2.40", "currency": "EUR"},
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
	if _, err := food.SaveUserRecipes([]food.Recipe{{
		ID:       "nutrition-required-rice",
		Title:    "Nutrition Required Rice",
		Servings: 2,
		Tags:     []string{"dinner", "nutritionfixture"},
		Ingredients: []food.Ingredient{
			{Name: "rice", Quantity: 250, Unit: "g", SearchTerm: "arroz"},
		},
		Steps: []food.RecipeStep{{Number: 1, Text: "Cook rice."}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := food.SaveProfile(food.Profile{LikedRecipes: []string{"nutrition-required-rice", "nutritionfixture"}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALCAMPO_BASE_URL", server.URL)
	outputDir := t.TempDir()
	basketPath := filepath.Join(outputDir, "basket.txt")
	runPath := filepath.Join(outputDir, "run.json")
	nutritionPath := filepath.Join(outputDir, "nutrition.json")
	readinessPath := filepath.Join(outputDir, "readiness.json")
	var stdout, stderr bytes.Buffer
	err := Run([]string{"food", "run", "--days", "1", "--people", "2", "--meals", "dinner", "--selection-policy", "balanced", "--basket-out", basketPath, "--run-out", runPath, "--nutrition-ledger-out", nutritionPath, "--readiness-out", readinessPath, "--require-safe-basket", "--require-cook-ready", "--require-nutrition-ready", "--min-nutrition-line-coverage", "1", "--json"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected nutrition readiness exit; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if code := ExitCode(err); code != food.ReadinessExitBlocked {
		t.Fatalf("exit code = %d, want %d; err=%v stdout=%s stderr=%s", code, food.ReadinessExitBlocked, err, stdout.String(), stderr.String())
	}
	var result struct {
		ReadinessGate   food.ReadinessGate    `json:"readiness_gate"`
		NutritionLedger *food.NutritionLedger `json:"nutrition_ledger,omitempty"`
	}
	if jsonErr := json.Unmarshal(stdout.Bytes(), &result); jsonErr != nil {
		t.Fatalf("output was not JSON: %v\n%s", jsonErr, stdout.String())
	}
	if !result.ReadinessGate.SafeToBuild || !result.ReadinessGate.SafeToCook || result.ReadinessGate.SafeToReportNutrition || result.ReadinessGate.ExitReason != "nutrition reporting is not ready" {
		t.Fatalf("unexpected readiness: %+v", result.ReadinessGate)
	}
	if result.NutritionLedger == nil || result.NutritionLedger.Status != food.NutritionLedgerBlocked {
		t.Fatalf("unexpected nutrition ledger: %+v", result.NutritionLedger)
	}
	basketData, readErr := os.ReadFile(basketPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(basketData), "BASKET CAN BE BUILT, BUT NUTRITION REPORTING IS NOT READY") || !strings.Contains(string(basketData), "rice-sku 1 # rice") {
		t.Fatalf("basket should keep safe SKU line with nutrition warning: %s", string(basketData))
	}
}

func TestFoodRunRequireSafeBasketWritesDiagnosticArtifacts(t *testing.T) {
	sawMissingRice := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html></html>`))
		case "/api/webproductpagews/v6/product-pages/search":
			q := r.URL.Query().Get("q")
			if q == "arroz" {
				sawMissingRice = true
				_ = json.NewEncoder(w).Encode(map[string]any{"productGroups": []any{}})
				return
			}
			sku := "sku-" + strings.ReplaceAll(q, " ", "-")
			size := "1 unidad"
			if q == "pechuga de pollo" {
				size = "500 g"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"productGroups": []any{
					map[string]any{"decoratedProducts": []any{
						map[string]any{
							"productId":         "product-" + strings.ReplaceAll(q, " ", "-"),
							"retailerProductId": sku,
							"name":              q + " " + size,
							"brand":             "TEST",
							"size":              size,
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
	if err := food.SaveProfile(food.Profile{LikedRecipes: []string{"mediterranean-chicken-rice"}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALCAMPO_BASE_URL", server.URL)
	outputDir := t.TempDir()
	basketPath := filepath.Join(outputDir, "basket.txt")
	runPath := filepath.Join(outputDir, "run.json")
	pdfPath := filepath.Join(outputDir, "run.pdf")
	ledgerPath := filepath.Join(outputDir, "ledger.json")
	readinessPath := filepath.Join(outputDir, "readiness.json")
	recipeSwapPath := filepath.Join(outputDir, "recipe_swap.json")
	manifestPath := filepath.Join(outputDir, "manifest.json")
	auditPath := filepath.Join(outputDir, "audit.json")
	var stdout, stderr bytes.Buffer
	err := Run([]string{"food", "run", "--days", "1", "--people", "2", "--meals", "dinner", "--selection-policy", "balanced", "--basket-out", basketPath, "--run-out", runPath, "--quantity-ledger-out", ledgerPath, "--recipe-swap-out", recipeSwapPath, "--readiness-out", readinessPath, "--manifest-out", manifestPath, "--audit-out", auditPath, "--audit-mode", "fail", "--pdf-out", pdfPath, "--strict-quantity", "--no-recovery", "--require-safe-basket", "--json"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected unsafe basket exit; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if code := ExitCode(err); code != food.ReadinessExitBlocked {
		t.Fatalf("exit code = %d, want %d; err=%v stdout=%s stderr=%s", code, food.ReadinessExitBlocked, err, stdout.String(), stderr.String())
	}
	if !sawMissingRice {
		t.Fatal("fixture did not exercise missing rice product")
	}
	var result struct {
		ReadinessGate food.ReadinessGate        `json:"readiness_gate"`
		Readiness     string                    `json:"readiness"`
		Run           string                    `json:"run"`
		PDF           string                    `json:"pdf"`
		Basket        string                    `json:"basket"`
		RecipeSwap    string                    `json:"recipe_swap"`
		Manifest      string                    `json:"manifest"`
		Audit         string                    `json:"audit"`
		ArtifactAudit *food.ArtifactAuditReport `json:"artifact_audit,omitempty"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("output was not JSON despite diagnostic exit: %v\n%s", err, stdout.String())
	}
	if result.Readiness != readinessPath || result.Run != runPath || result.PDF != pdfPath || result.Basket != basketPath {
		t.Fatalf("diagnostic output missing artifact paths: %+v", result)
	}
	if result.ReadinessGate.SafeToBuild || result.ReadinessGate.Status != food.ReadinessBlocked || len(result.ReadinessGate.BlockingIssues) == 0 {
		t.Fatalf("JSON readiness should be blocked with issues: %+v", result.ReadinessGate)
	}
	if result.RecipeSwap != recipeSwapPath {
		t.Fatalf("diagnostic output missing recipe swap path: %+v", result)
	}
	if result.Manifest != manifestPath || result.Audit != auditPath || result.ArtifactAudit == nil || result.ArtifactAudit.Status == food.ArtifactAuditStatusFail {
		t.Fatalf("diagnostic audit should pass while readiness returns 20: %+v", result)
	}
	recipeSwapData, err := os.ReadFile(recipeSwapPath)
	if err != nil {
		t.Fatal(err)
	}
	var recipeSwap food.RecipeSwapPlan
	if err := json.Unmarshal(recipeSwapData, &recipeSwap); err != nil {
		t.Fatalf("recipe swap artifact was not JSON: %v\n%s", err, string(recipeSwapData))
	}
	if recipeSwap.Status != food.RecipeSwapDisabled {
		t.Fatalf("recipe swap should be disabled: %+v", recipeSwap)
	}
	readinessData, err := os.ReadFile(readinessPath)
	if err != nil {
		t.Fatal(err)
	}
	var readiness food.ReadinessGate
	if err := json.Unmarshal(readinessData, &readiness); err != nil {
		t.Fatalf("readiness artifact was not JSON: %v\n%s", err, string(readinessData))
	}
	if readiness.SafeToBuild || readiness.Status != food.ReadinessBlocked {
		t.Fatalf("readiness artifact should be blocked: %+v", readiness)
	}
	runData, err := os.ReadFile(runPath)
	if err != nil {
		t.Fatal(err)
	}
	var artifact food.FoodRunArtifact
	if err := json.Unmarshal(runData, &artifact); err != nil {
		t.Fatalf("run artifact was not JSON: %v\n%s", err, string(runData))
	}
	if artifact.ReadinessGate == nil || artifact.ReadinessGate.SafeToBuild || artifact.BasketSafety == nil || artifact.BasketSafety.SafeToBuild {
		t.Fatalf("run artifact should preserve blocked readiness and basket safety: %+v safety=%+v", artifact.ReadinessGate, artifact.BasketSafety)
	}
	basketData, err := os.ReadFile(basketPath)
	if err != nil {
		t.Fatal(err)
	}
	basketText := string(basketData)
	if !strings.HasPrefix(basketText, "# NOT SAFE TO BUILD BASKET") || !strings.Contains(basketText, "# Diagnostic selected products") {
		t.Fatalf("unsafe basket should be diagnostic comments: %s", basketText)
	}
	for _, line := range strings.Split(strings.TrimSpace(basketText), "\n") {
		if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "#") {
			t.Fatalf("unsafe basket has actionable line %q in:\n%s", line, basketText)
		}
	}
	pdfData, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Basket readiness: BLOCKED", "Do not use this basket for shopping.", "Shopping not ready:"} {
		if !strings.Contains(string(pdfData), want) {
			t.Fatalf("diagnostic PDF missing %q", want)
		}
	}
	auditData, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	var auditReport food.ArtifactAuditReport
	if err := json.Unmarshal(auditData, &auditReport); err != nil {
		t.Fatalf("audit report was not JSON: %v\n%s", err, string(auditData))
	}
	if auditReport.Status == food.ArtifactAuditStatusFail || !auditReport.HermesTrustSummary.Trustworthy {
		t.Fatalf("readiness-blocked diagnostic bundle should audit as trustworthy: %+v", auditReport)
	}
	if err := os.WriteFile(basketPath, append(basketData, []byte("rogue-sku 1 # rogue\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	validateAuditPath := filepath.Join(outputDir, "validate-audit.json")
	stdout.Reset()
	stderr.Reset()
	err = Run([]string{"food", "validate-run", "--run", runPath, "--manifest", manifestPath, "--basket", basketPath, "--pdf", pdfPath, "--audit-out", validateAuditPath, "--mode", "hermes", "--audit-mode", "fail", "--json"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected validate-run audit failure after corrupt basket; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if code := ExitCode(err); code != food.ArtifactAuditExitBlocked {
		t.Fatalf("validate-run exit code = %d, want %d; err=%v stdout=%s stderr=%s", code, food.ArtifactAuditExitBlocked, err, stdout.String(), stderr.String())
	}
	var validateReport food.ArtifactAuditReport
	if jsonErr := json.Unmarshal(stdout.Bytes(), &validateReport); jsonErr != nil {
		t.Fatalf("validate-run output was not JSON: %v\n%s", jsonErr, stdout.String())
	}
	if validateReport.Status != food.ArtifactAuditStatusFail || validateReport.RecommendedExitCode != food.ArtifactAuditExitBlocked {
		t.Fatalf("validate-run audit should fail with exit recommendation: %+v", validateReport)
	}
}

func TestFoodRunRecordAndReplayLiveSnapshot(t *testing.T) {
	searches := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html></html>`))
		case "/api/webproductpagews/v6/product-pages/search":
			q := r.URL.Query().Get("q")
			searches[q]++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"productGroups": []any{
					map[string]any{"decoratedProducts": []any{
						map[string]any{
							"productId":         "product-" + q,
							"retailerProductId": "sku-" + q,
							"name":              q + " 1 kg",
							"brand":             "TEST",
							"size":              "1 kg",
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
	if _, err := food.SaveUserRecipes([]food.Recipe{
		{
			ID:          "snapshot-rice",
			Title:       "Snapshot Rice",
			Servings:    2,
			Tags:        []string{"dinner", "snapshotfixture"},
			Ingredients: []food.Ingredient{{Name: "rice", Quantity: 250, Unit: "g", SearchTerm: "arroz"}},
			Steps:       []food.RecipeStep{{Number: 1, Text: "Cook rice."}},
		},
		{
			ID:          "snapshot-milk",
			Title:       "Snapshot Milk",
			Servings:    2,
			Tags:        []string{"dinner", "snapshotfixture"},
			Ingredients: []food.Ingredient{{Name: "milk", Quantity: 1, Unit: "l", SearchTerm: "leche"}},
			Steps:       []food.RecipeStep{{Number: 1, Text: "Pour milk."}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := food.SaveProfile(food.Profile{LikedRecipes: []string{"snapshot-rice"}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALCAMPO_BASE_URL", server.URL)
	outputDir := t.TempDir()
	snapshotDir := filepath.Join(outputDir, "snapshot")
	recordRun := filepath.Join(outputDir, "record-run.json")
	recordManifest := filepath.Join(outputDir, "record-manifest.json")
	recordAudit := filepath.Join(outputDir, "record-audit.json")
	recordBasket := filepath.Join(outputDir, "record-basket.txt")
	var stdout, stderr bytes.Buffer
	err := Run([]string{"food", "run", "--days", "1", "--people", "2", "--meals", "dinner", "--selection-policy", "balanced", "--basket-out", recordBasket, "--run-out", recordRun, "--manifest-out", recordManifest, "--audit-out", recordAudit, "--audit-mode", "fail", "--record-live-snapshot", snapshotDir, "--snapshot-id", "cli-snapshot", "--no-product-detail-enrichment", "--require-safe-basket", "--json"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("record run error: %v stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	if searches["arroz"] == 0 {
		t.Fatalf("record run did not hit live fixture: searches=%+v", searches)
	}
	var recordOut struct {
		Snapshot *food.ManifestSnapshotSummary `json:"snapshot,omitempty"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &recordOut); err != nil {
		t.Fatalf("record output was not JSON: %v\n%s", err, stdout.String())
	}
	if recordOut.Snapshot == nil || recordOut.Snapshot.Mode != alcampo.LiveSnapshotModeRecord || recordOut.Snapshot.EntryCount == 0 {
		t.Fatalf("record output missing snapshot summary: %+v", recordOut.Snapshot)
	}

	t.Setenv("ALCAMPO_BASE_URL", "http://127.0.0.1:1")
	replayRun := filepath.Join(outputDir, "replay-run.json")
	replayManifest := filepath.Join(outputDir, "replay-manifest.json")
	replayAudit := filepath.Join(outputDir, "replay-audit.json")
	replayBasket := filepath.Join(outputDir, "replay-basket.txt")
	stdout.Reset()
	stderr.Reset()
	err = Run([]string{"food", "run", "--days", "1", "--people", "2", "--meals", "dinner", "--selection-policy", "balanced", "--basket-out", replayBasket, "--run-out", replayRun, "--manifest-out", replayManifest, "--audit-out", replayAudit, "--audit-mode", "fail", "--replay-live-snapshot", snapshotDir, "--snapshot-strict", "--no-product-detail-enrichment", "--require-safe-basket", "--json"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("replay run error: %v stdout=%s stderr=%s", err, stdout.String(), stderr.String())
	}
	var replayOut struct {
		Snapshot *food.ManifestSnapshotSummary `json:"snapshot,omitempty"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &replayOut); err != nil {
		t.Fatalf("replay output was not JSON: %v\n%s", err, stdout.String())
	}
	if replayOut.Snapshot == nil || replayOut.Snapshot.Mode != alcampo.LiveSnapshotModeReplay || replayOut.Snapshot.ReplayHits == 0 || replayOut.Snapshot.ReplayMisses != 0 {
		t.Fatalf("unexpected replay snapshot summary: %+v", replayOut.Snapshot)
	}
	var recordedRun, replayedRun food.FoodRunArtifact
	readJSONFile(t, recordRun, &recordedRun)
	readJSONFile(t, replayRun, &replayedRun)
	if recordedRun.ProductSelectionFingerprint != replayedRun.ProductSelectionFingerprint {
		t.Fatalf("replay changed product selection: record=%s replay=%s", recordedRun.ProductSelectionFingerprint, replayedRun.ProductSelectionFingerprint)
	}
	var replayAuditReport food.ArtifactAuditReport
	readJSONFile(t, replayAudit, &replayAuditReport)
	if replayAuditReport.Status == food.ArtifactAuditStatusFail || replayAuditReport.Metrics.SnapshotReplayMisses != 0 {
		t.Fatalf("replay audit should pass with zero misses: %+v", replayAuditReport)
	}

	if err := food.SaveProfile(food.Profile{LikedRecipes: []string{"snapshot-milk"}}); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	err = Run([]string{"food", "run", "--days", "1", "--people", "2", "--meals", "dinner", "--selection-policy", "balanced", "--basket-out", filepath.Join(outputDir, "miss-basket.txt"), "--run-out", filepath.Join(outputDir, "miss-run.json"), "--replay-live-snapshot", snapshotDir, "--snapshot-strict", "--no-product-detail-enrichment", "--require-safe-basket", "--json"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected strict replay miss; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if code := ExitCode(err); code != alcampo.LiveSnapshotExitBlocked {
		t.Fatalf("strict replay miss exit = %d, want %d; err=%v stdout=%s stderr=%s", code, alcampo.LiveSnapshotExitBlocked, err, stdout.String(), stderr.String())
	}
}

func TestFoodRunRecipeSwapRepairsBlockedRun(t *testing.T) {
	searches := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html></html>`))
		case "/api/webproductpagews/v6/product-pages/search":
			q := r.URL.Query().Get("q")
			searches[q]++
			if strings.Contains(q, "ternera") {
				_ = json.NewEncoder(w).Encode(map[string]any{"productGroups": []any{}})
				return
			}
			sku := "sku-" + strings.ReplaceAll(q, " ", "-")
			size := "500 g"
			if strings.Contains(q, "arroz") {
				size = "1 kg"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"productGroups": []any{
					map[string]any{"decoratedProducts": []any{
						map[string]any{
							"productId":         "product-" + strings.ReplaceAll(q, " ", "-"),
							"retailerProductId": sku,
							"name":              q + " " + size,
							"brand":             "TEST",
							"size":              size,
							"price":             map[string]any{"amount": "1.00", "currency": "EUR"},
							"unitPrice":         map[string]any{"amount": "1.00", "currency": "EUR"},
							"available":         true,
							"images":            []any{map[string]any{"url": "/" + strings.ReplaceAll(q, " ", "-") + ".jpg"}},
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
	if _, err := food.SaveUserRecipes([]food.Recipe{{
		ID:       "a-valid-replacement",
		Title:    "A Valid Replacement",
		Servings: 2,
		Tags:     []string{"dinner", "quick", "omnivore"},
		ImageURL: "/replacement.jpg",
		Ingredients: []food.Ingredient{
			{Name: "chicken breast", Quantity: 300, Unit: "g", Category: "meat", SearchTerm: "pechuga de pollo"},
			{Name: "rice", Quantity: 180, Unit: "g", Category: "pantry", SearchTerm: "arroz"},
		},
		Steps: []food.RecipeStep{{Number: 1, Text: "Cook rice."}, {Number: 2, Text: "Cook chicken and serve."}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := food.SaveProfile(food.Profile{LikedRecipes: []string{"beef-vegetable-noodles"}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALCAMPO_BASE_URL", server.URL)
	outputDir := t.TempDir()
	basketPath := filepath.Join(outputDir, "basket.txt")
	runPath := filepath.Join(outputDir, "run.json")
	pdfPath := filepath.Join(outputDir, "run.pdf")
	ledgerPath := filepath.Join(outputDir, "ledger.json")
	readinessPath := filepath.Join(outputDir, "readiness.json")
	recipeSwapPath := filepath.Join(outputDir, "recipe_swap.json")
	var stdout, stderr bytes.Buffer
	err := Run([]string{"food", "run", "--days", "1", "--people", "2", "--meals", "dinner", "--selection-policy", "balanced", "--basket-out", basketPath, "--run-out", runPath, "--quantity-ledger-out", ledgerPath, "--recipe-swap-out", recipeSwapPath, "--readiness-out", readinessPath, "--pdf-out", pdfPath, "--strict-quantity", "--no-recovery", "--allow-recipe-swap", "--require-safe-basket", "--json"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	if searches["ternera tiras"] == 0 {
		t.Fatalf("fixture did not block original beef recipe: searches=%+v", searches)
	}
	var result struct {
		MealPlan       food.MealPlan       `json:"mealplan"`
		ReadinessGate  food.ReadinessGate  `json:"readiness_gate"`
		RecipeSwapPlan food.RecipeSwapPlan `json:"recipe_swap_plan"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("output was not JSON: %v\n%s", err, stdout.String())
	}
	if !result.ReadinessGate.SafeToBuild || result.ReadinessGate.Status != food.ReadinessReadyExact {
		t.Fatalf("readiness should be repaired and safe: %+v", result.ReadinessGate)
	}
	if result.RecipeSwapPlan.Status != food.RecipeSwapAttemptedApplied || len(result.RecipeSwapPlan.AppliedSwaps) != 1 {
		t.Fatalf("recipe swap not applied: %+v", result.RecipeSwapPlan)
	}
	if result.MealPlan.Days[0].Meals[0].Recipe.ID != "a-valid-replacement" {
		t.Fatalf("final meal plan did not use replacement: %+v", result.MealPlan.Days[0].Meals[0].Recipe)
	}
	runData, err := os.ReadFile(runPath)
	if err != nil {
		t.Fatal(err)
	}
	var artifact food.FoodRunArtifact
	if err := json.Unmarshal(runData, &artifact); err != nil {
		t.Fatalf("run artifact was not JSON: %v\n%s", err, string(runData))
	}
	if artifact.PreRecipeSwapReadinessGate == nil || artifact.PreRecipeSwapReadinessGate.SafeToBuild {
		t.Fatalf("pre-swap readiness should preserve blocked state: %+v", artifact.PreRecipeSwapReadinessGate)
	}
	if artifact.RecipeSwapPlan == nil || artifact.RecipeSwapPlan.Status != food.RecipeSwapAttemptedApplied || artifact.ReadinessGate == nil || !artifact.ReadinessGate.SafeToBuild {
		t.Fatalf("run artifact missing final swap/readiness: swap=%+v readiness=%+v", artifact.RecipeSwapPlan, artifact.ReadinessGate)
	}
	recipeSwapData, err := os.ReadFile(recipeSwapPath)
	if err != nil {
		t.Fatal(err)
	}
	var swapArtifact food.RecipeSwapPlan
	if err := json.Unmarshal(recipeSwapData, &swapArtifact); err != nil {
		t.Fatalf("recipe swap artifact was not JSON: %v\n%s", err, string(recipeSwapData))
	}
	if swapArtifact.Status != food.RecipeSwapAttemptedApplied {
		t.Fatalf("recipe swap artifact not applied: %+v", swapArtifact)
	}
	basketData, err := os.ReadFile(basketPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(basketData), "# Meal plan repair: 1 recipe swapped") || strings.Contains(string(basketData), "ternera") {
		t.Fatalf("basket should mention repair and exclude original beef lines: %s", string(basketData))
	}
	pdfData, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Meal plan repair:", "A Valid Replacement", "Replaced original recipe: Beef Vegetable Noodles", "Basket readiness: READY_EXACT"} {
		if !strings.Contains(string(pdfData), want) {
			t.Fatalf("recipe swap PDF missing %q", want)
		}
	}
}

func TestFoodRunPDFErrorLeavesArtifactWithoutPDFClaim(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html></html>`))
		case "/api/webproductpagews/v6/product-pages/search":
			q := r.URL.Query().Get("q")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"productGroups": []any{
					map[string]any{"decoratedProducts": []any{
						map[string]any{
							"productId":         "product-" + q,
							"retailerProductId": "sku-" + q,
							"name":              q + " 500 g",
							"brand":             "TEST",
							"size":              "500 g",
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
	outputDir := t.TempDir()
	runPath := filepath.Join(outputDir, "run.json")
	var stdout, stderr bytes.Buffer
	err := Run([]string{"food", "run", "--days", "1", "--people", "2", "--selection-policy", "balanced", "--run-out", runPath, "--pdf-out", outputDir, "--json"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected PDF write failure; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	runData, readErr := os.ReadFile(runPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	var artifact food.FoodRunArtifact
	if err := json.Unmarshal(runData, &artifact); err != nil {
		t.Fatalf("run artifact was not JSON: %v\n%s", err, string(runData))
	}
	if artifact.PDFPath != "" {
		t.Fatalf("failed PDF run artifact should not claim pdf path: %+v", artifact)
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
	if result.Pantry.UpdatedAt == "" || result.Pantry.UpdatedAt != pantry.UpdatedAt {
		t.Fatalf("cook response pantry updated_at=%q persisted=%q", result.Pantry.UpdatedAt, pantry.UpdatedAt)
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

func TestFoodCookRunArtifactConsumesReceivedIngredients(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ALCAMPO_CONFIG_DIR", dir)
	plan := food.MealPlan{
		ID: "plan-run-cook",
		Days: []food.DayPlan{{
			Day: 1,
			Meals: []food.Meal{{
				Type: "dinner",
				Recipe: food.Recipe{
					ID:    "chicken-rice",
					Title: "Chicken Rice",
					Ingredients: []food.Ingredient{
						{Name: "chicken breast fillets", SearchTerm: "pechuga pollo filetes", Quantity: 320, Unit: "g", Category: "meat"},
						{Name: "brown rice", SearchTerm: "arroz integral", Quantity: 180, Unit: "g", Category: "pantry"},
					},
				},
			}},
		}},
	}
	runOutput := struct {
		SchemaVersion int             `json:"schema_version"`
		Kind          string          `json:"kind"`
		MealPlan      food.MealPlan   `json:"mealplan"`
		Shop          food.ShopResult `json:"shop"`
	}{
		SchemaVersion: 1,
		Kind:          "food_run",
		MealPlan:      plan,
		Shop: food.ShopResult{
			MealPlanID: plan.ID,
			SelectedProducts: []food.SelectedProduct{
				{
					Ingredient:       food.Ingredient{Name: "chicken breast fillets", Category: "meat"},
					Product:          food.ProductSummary{SKU: "chicken-sku", Name: "Pechuga pollo", PackageQuantity: 320, PackageUnit: "g"},
					PurchaseQuantity: "1",
					PackageCount:     1,
				},
				{
					Ingredient:       food.Ingredient{Name: "brown rice", Category: "pantry"},
					Product:          food.ProductSummary{SKU: "rice-sku", Name: "Arroz integral", PackageQuantity: 250, PackageUnit: "g"},
					PurchaseQuantity: "1",
					PackageCount:     1,
				},
			},
		},
	}
	data, err := json.Marshal(runOutput)
	if err != nil {
		t.Fatal(err)
	}
	runPath := filepath.Join(t.TempDir(), "run.json")
	if err := os.WriteFile(runPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Run([]string{"food", "receive", runPath, "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("receive Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if err := Run([]string{"food", "cook", runPath, "--rating", "5", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("cook Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	var result food.CookResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("cook output was not JSON: %v\n%s", err, stdout.String())
	}
	if len(result.Applied) != 2 || len(result.Missing) != 0 {
		t.Fatalf("run artifact cook did not consume delivered ingredients: %+v", result)
	}
	pantry, err := food.LoadPantry()
	if err != nil {
		t.Fatal(err)
	}
	remaining := map[string]float64{}
	for _, item := range pantry.Items {
		remaining[item.Name] = item.Quantity
	}
	if _, ok := remaining["chicken breast fillets"]; ok {
		t.Fatalf("fully used chicken should be removed from pantry: %+v", pantry)
	}
	if remaining["brown rice"] != 70 {
		t.Fatalf("brown rice remaining = %.3g, want 70; pantry=%+v", remaining["brown rice"], pantry)
	}
	profile, err := food.LoadProfile()
	if err != nil {
		t.Fatal(err)
	}
	if len(profile.LikedRecipes) != 1 || profile.LikedRecipes[0] != "chicken-rice" {
		t.Fatalf("liked recipe not learned from run artifact cook: %+v", profile)
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
	if result.Pantry.UpdatedAt == "" || result.Pantry.UpdatedAt != pantry.UpdatedAt {
		t.Fatalf("receive response pantry updated_at=%q persisted=%q", result.Pantry.UpdatedAt, pantry.UpdatedAt)
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

func readJSONFile(t *testing.T, path string, out any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("%s was not JSON: %v\n%s", path, err, string(data))
	}
}
