package food

import "testing"

func TestRecipeFromTextParsesIngredientList(t *testing.T) {
	recipe, err := RecipeFromText("2 pechugas de pollo, 1 cebolla, 200 g arroz", TextRecipeOptions{Title: "Pollo rapido", Servings: 2})
	if err != nil {
		t.Fatal(err)
	}
	if recipe.ID != "pollo-rapido" || recipe.Title != "Pollo rapido" {
		t.Fatalf("bad recipe identity: %+v", recipe)
	}
	if len(recipe.Ingredients) != 3 {
		t.Fatalf("ingredients len = %d: %+v", len(recipe.Ingredients), recipe.Ingredients)
	}
	if recipe.Ingredients[0].SearchTerm != "pechuga pollo" {
		t.Fatalf("search term = %q", recipe.Ingredients[0].SearchTerm)
	}
}
