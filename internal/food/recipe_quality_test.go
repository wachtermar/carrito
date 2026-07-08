package food

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRecipeIntakeLocalStructuredImportCachesImage(t *testing.T) {
	dir := t.TempDir()
	imagePath := filepath.Join(dir, "cover.png")
	writeRecipeQualityTestPNG(t, imagePath)
	recipePath := filepath.Join(dir, "recipe.json")
	writeRecipeJSON(t, recipePath, fmt.Sprintf(`{
		"id": "lentil-rice",
		"title": "Lentil Rice",
		"servings": 2,
		"image_url": %q,
		"ingredients": [
			{"name": "rice", "quantity": 200, "unit": "g"},
			{"name": "lentils", "quantity": 150, "unit": "g"}
		],
		"steps": [{"number": 1, "text": "Cook rice and lentils."}]
	}`, imagePath))
	plan := recipeQualityTestPlan()
	opts := RecipeIntakeOptions{
		Enabled:       true,
		RecipeFiles:   []string{recipePath},
		ImageCacheDir: filepath.Join(dir, "cache"),
		Policy:        DefaultRecipeIntakePolicy(),
	}
	got, intake, report, err := ApplyRecipeIntake(plan, opts)
	if err != nil {
		t.Fatalf("ApplyRecipeIntake() error = %v", err)
	}
	if got.Days[0].Meals[0].Recipe.Title != "Lentil Rice" {
		t.Fatalf("recipe was not injected: %#v", got.Days[0].Meals[0].Recipe)
	}
	if intake.ImportedRecipeCount != 1 || len(intake.AppliedMealSlots) != 1 {
		t.Fatalf("unexpected intake plan: %+v", intake)
	}
	if report.Status != RecipeQualityPass {
		t.Fatalf("quality status = %s, issues=%+v warnings=%+v", report.Status, report.BlockingIssues, report.Warnings)
	}
	if report.Summary.CachedImages != 1 || report.ImageEvidence[0].CachedPath == "" {
		t.Fatalf("image was not cached: %+v", report.ImageEvidence)
	}
	if _, err := os.Stat(report.ImageEvidence[0].CachedPath); err != nil {
		t.Fatalf("cached image missing: %v", err)
	}
	if got.Days[0].Meals[0].Recipe.ImageURL != report.ImageEvidence[0].CachedPath {
		t.Fatalf("recipe image was not rewritten to cache path")
	}
}

func TestRecipeQualityMissingStepsStrictBlocksNonStrictWarns(t *testing.T) {
	recipe := qualityRecipe()
	recipe.Steps = nil
	plan := mealPlanWithRecipe(recipe)
	nonStrict := EvaluateRecipeQuality(plan, nil, nil, DefaultRecipeIntakePolicy())
	if nonStrict.Status != RecipeQualityPassWithWarning {
		t.Fatalf("non-strict status = %s", nonStrict.Status)
	}
	policy := DefaultRecipeIntakePolicy()
	policy.StrictRecipeQuality = true
	strict := EvaluateRecipeQuality(plan, nil, nil, policy)
	if strict.Status != RecipeQualityFail || !hasRecipeQualityIssue(strict.BlockingIssues, "cooking_steps_missing") {
		t.Fatalf("strict report did not block missing steps: %+v", strict)
	}
}

func TestRecipeQualityStrictServingsAndVagueIngredient(t *testing.T) {
	recipe := qualityRecipe()
	recipe.Servings = 0
	recipe.Ingredients = append(recipe.Ingredients, Ingredient{Name: "meat", Quantity: 1, Unit: "kg"})
	policy := DefaultRecipeIntakePolicy()
	policy.StrictRecipeQuality = true
	policy.RequireBaseServings = true
	report := EvaluateRecipeQuality(mealPlanWithRecipe(recipe), map[string]RecipeSourceEvidence{
		recipe.ID: {SourceID: "file:test", SourceType: "file", BaseServings: 0},
	}, nil, policy)
	if report.Status != RecipeQualityFail {
		t.Fatalf("status = %s", report.Status)
	}
	if !hasRecipeQualityIssue(report.BlockingIssues, "base_servings_missing") {
		t.Fatalf("missing base servings not detected: %+v", report.BlockingIssues)
	}
	if !hasRecipeQualityIssue(report.BlockingIssues, "vague_required_ingredient") {
		t.Fatalf("vague ingredient not detected: %+v", report.BlockingIssues)
	}
}

func TestRecipeQualityPantryToTasteStaplesDoNotFailStrictAmounts(t *testing.T) {
	recipe := qualityRecipe()
	recipe.Ingredients = append(recipe.Ingredients,
		Ingredient{Name: "salt", Unit: "to taste"},
		Ingredient{Name: "water"},
	)
	policy := DefaultRecipeIntakePolicy()
	policy.StrictRecipeQuality = true
	report := EvaluateRecipeQuality(mealPlanWithRecipe(recipe), nil, nil, policy)
	if hasRecipeQualityIssue(report.BlockingIssues, "required_amount_unparseable") {
		t.Fatalf("pantry/to-taste staples should not fail strict amount parsing: %+v", report.BlockingIssues)
	}
}

func TestRecipeQualityBrokenImageWarnsAndRequireImagesBlocks(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	recipe := qualityRecipe()
	recipe.ImageURL = server.URL + "/missing.png"
	opts := RecipeIntakeOptions{Enabled: true, Policy: DefaultRecipeIntakePolicy()}
	plan := mealPlanWithRecipe(recipe)
	imageEvidence := CacheRecipeImagesInMealPlan(&plan, opts)
	report := EvaluateRecipeQuality(plan, nil, imageEvidence, opts.Policy)
	if report.Status != RecipeQualityPassWithWarning || !hasRecipeQualityIssue(report.Warnings, "recipe_image_failed") {
		t.Fatalf("broken image should warn by default: %+v", report)
	}
	policy := DefaultRecipeIntakePolicy()
	policy.RequireRecipeImages = true
	report = EvaluateRecipeQuality(plan, nil, imageEvidence, policy)
	if report.Status != RecipeQualityFail || !hasRecipeQualityIssue(report.BlockingIssues, "recipe_image_failed") {
		t.Fatalf("required broken image should block: %+v", report)
	}
}

func TestRecipeURLImportStructuredJSONLDAndUnstructuredFailClosed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/recipe":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<html><script type="application/ld+json">{
				"@context":"https://schema.org",
				"@type":"Recipe",
				"name":"Structured Chickpeas",
				"recipeYield":"2 servings",
				"recipeIngredient":["200 g chickpeas","1 tomato"],
				"recipeInstructions":[{"@type":"HowToStep","text":"Simmer everything."}]
			}</script></html>`)
		default:
			fmt.Fprint(w, "<html><h1>No structured recipe here</h1></html>")
		}
	}))
	defer server.Close()
	plan := recipeQualityTestPlan()
	opts := RecipeIntakeOptions{Enabled: true, RecipeURLs: []string{server.URL + "/recipe"}, Policy: DefaultRecipeIntakePolicy()}
	got, intake, report, err := ApplyRecipeIntake(plan, opts)
	if err != nil {
		t.Fatalf("structured URL import failed: %v", err)
	}
	if intake.ImportedRecipeCount != 1 || got.Days[0].Meals[0].Recipe.Title != "Structured Chickpeas" {
		t.Fatalf("structured recipe was not imported: intake=%+v plan=%+v", intake, got)
	}
	if report.Status != RecipeQualityPassWithWarning {
		t.Fatalf("expected only image caveat for structured recipe without image, got %+v", report)
	}
	opts.RecipeURLs = []string{server.URL + "/plain"}
	if _, _, _, err := ApplyRecipeIntake(plan, opts); err == nil {
		t.Fatalf("unstructured URL import should fail closed")
	}
}

func recipeQualityTestPlan() MealPlan {
	return MealPlan{ID: "quality-test", People: 2, Days: []DayPlan{{Day: 1, Meals: []Meal{{Type: "dinner", Recipe: qualityRecipe()}}}}}
}

func mealPlanWithRecipe(recipe Recipe) MealPlan {
	return MealPlan{ID: "quality-test", People: 2, Days: []DayPlan{{Day: 1, Meals: []Meal{{Type: "dinner", Recipe: recipe}}}}}
}

func qualityRecipe() Recipe {
	return Recipe{
		ID:       "quality-recipe",
		Title:    "Quality Recipe",
		Servings: 2,
		Ingredients: []Ingredient{
			{Name: "rice", Quantity: 200, Unit: "g"},
			{Name: "tomato", Quantity: 1, Unit: "unit"},
		},
		Steps: []RecipeStep{{Number: 1, Text: "Cook everything."}},
	}
}

func writeRecipeJSON(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write recipe: %v", err)
	}
}

func writeRecipeQualityTestPNG(t *testing.T, path string) {
	t.Helper()
	const png1x1 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGNgYPgPAAEDAQDqXU8hAAAAAElFTkSuQmCC"
	data, err := base64.StdEncoding.DecodeString(png1x1)
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write png: %v", err)
	}
}

func hasRecipeQualityIssue(issues []RecipeQualityIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
