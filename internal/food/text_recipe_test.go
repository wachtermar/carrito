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

func TestSpanishSearchTermCleansImportedEnglishRecipeNoise(t *testing.T) {
	cases := map[string]string{
		"Santa Fe Blend frozen vegetables ($1.25)": "verduras congeladas",
		"1 15oz. can pinto beans ($1.25)":          "alubias pintas",
		"cheddar cheese ($1.25)":                   "queso cheddar",
		"Bayou Blend seasoning* ($1.25)":           "especias",
		"8 flour tortillas ($1.25)":                "tortillas trigo",
		"5.6oz. pkg Spanish rice ($1.25)":          "arroz",
		"10oz. can Rotel (diced tomatoes)":         "tomate troceado",
	}
	for input, want := range cases {
		if got := spanishSearchTerm(cleanIngredientName(input)); got != want {
			t.Fatalf("spanishSearchTerm(%q) = %q, want %q", input, got, want)
		}
	}
}
