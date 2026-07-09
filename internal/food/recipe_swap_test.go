package food

import "testing"

func TestReplaceRecipeInPlanDoesNotMutateOriginalPlan(t *testing.T) {
	original := Recipe{
		ID:          "chicken-fajita-bowls",
		Title:       "Chicken Fajita Bowls",
		Servings:    2,
		Ingredients: []Ingredient{{Name: "bell pepper", Quantity: 300, Unit: "g"}},
		Steps:       []RecipeStep{{Number: 1, Text: "Cook."}},
	}
	replacement := Recipe{
		ID:          "beef-vegetable-noodles",
		Title:       "Beef Vegetable Noodles",
		Servings:    2,
		Ingredients: []Ingredient{{Name: "beef strips", Quantity: 300, Unit: "g"}},
		Steps:       []RecipeStep{{Number: 1, Text: "Cook."}},
	}
	plan := MealPlan{
		Days: []DayPlan{{
			Day:   1,
			Meals: []Meal{{Type: "dinner", Recipe: original}},
		}},
	}

	replaced := replaceRecipeInPlan(plan, AffectedRecipeSlot{Day: 1, MealSlot: "dinner", RecipeID: original.ID}, replacement)

	if got := replaced.Days[0].Meals[0].Recipe.ID; got != replacement.ID {
		t.Fatalf("replaced recipe ID = %q, want %q", got, replacement.ID)
	}
	if got := replaced.Days[0].Meals[0].PlanningReason; got == "" {
		t.Fatalf("replaced meal planning reason is empty")
	}
	if got := plan.Days[0].Meals[0].Recipe.ID; got != original.ID {
		t.Fatalf("original plan recipe ID = %q, want %q", got, original.ID)
	}
	if got := plan.Days[0].Meals[0].PlanningReason; got != "" {
		t.Fatalf("original plan planning reason = %q, want empty", got)
	}
}
