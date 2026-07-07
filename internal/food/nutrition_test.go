package food

import (
	"strings"
	"testing"
)

func TestSummarizeNutritionEstimatesCustomRecipe(t *testing.T) {
	recipe := Recipe{
		ID:       "rice-test",
		Title:    "Rice Test",
		Servings: 2,
		Ingredients: []Ingredient{
			{Name: "rice", Quantity: 200, Unit: "g"},
			{Name: "tomato", Quantity: 2, Unit: "unit"},
		},
		Steps: []RecipeStep{{Number: 1, Text: "Cook."}},
	}
	summary, err := SummarizeNutrition(recipe)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Kcal <= 0 || summary.CarbsG <= 0 {
		t.Fatalf("empty nutrition summary: %+v", summary)
	}
}

func TestGenerateMealPlanAddsNutritionAndGoalWarning(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	plan, err := GenerateMealPlan(Profile{NutritionGoals: []string{"kcal<=1"}}, Pantry{}, PlanOptions{Days: 1, People: 2, MealTypes: []string{"dinner"}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Nutrition == nil || plan.Nutrition.Kcal <= 0 {
		t.Fatalf("missing plan nutrition: %+v", plan.Nutrition)
	}
	if !strings.Contains(strings.Join(plan.Notes, " "), "Nutrition goal") {
		t.Fatalf("missing nutrition goal warning: %+v", plan.Notes)
	}
}
