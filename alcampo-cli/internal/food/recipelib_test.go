package food

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRecipesMergesSeedAndUserOverride(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	dir, err := RecipesDir()
	if err != nil {
		t.Fatal(err)
	}
	override := testRecipe("spanish-tortilla", "Custom Tortilla")
	custom := testRecipe("custom-rice-bowl", "Custom Rice Bowl")
	if err := writeJSONFile(filepath.Join(dir, "override.json"), []Recipe{override, custom}); err != nil {
		t.Fatal(err)
	}

	recipes, err := LoadRecipes()
	if err != nil {
		t.Fatal(err)
	}
	if len(recipes) < 24 {
		t.Fatalf("recipes len = %d, want at least 24", len(recipes))
	}
	loaded, ok, err := LoadRecipe("spanish-tortilla")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || loaded.Title != "Custom Tortilla" {
		t.Fatalf("override not loaded: ok=%t recipe=%+v", ok, loaded)
	}
	loaded, ok, err = LoadRecipe("Custom Rice Bowl")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || loaded.ID != "custom-rice-bowl" {
		t.Fatalf("custom recipe not loaded by title: ok=%t recipe=%+v", ok, loaded)
	}
}

func TestLoadRecipesRejectsMalformedUserFile(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	dir, err := RecipesDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	badPath := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badPath, []byte(`{"id":`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = LoadRecipes()
	if err == nil || !strings.Contains(err.Error(), "bad.json") {
		t.Fatalf("LoadRecipes error = %v, want bad.json context", err)
	}
}

func TestSaveAndRemoveUserRecipe(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	recipe := testRecipe("weeknight-beans", "Weeknight Beans")
	path, err := SaveUserRecipe(recipe)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := LoadRecipe("weeknight-beans")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || loaded.Title != recipe.Title {
		t.Fatalf("saved recipe not loaded: ok=%t recipe=%+v", ok, loaded)
	}
	removed, err := RemoveUserRecipe("weeknight-beans")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("RemoveUserRecipe returned false")
	}
	_, ok, err = LoadRecipe("weeknight-beans")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("removed recipe still loaded")
	}
}

func testRecipe(id, title string) Recipe {
	return Recipe{
		ID:          id,
		Title:       title,
		Servings:    2,
		PrepMinutes: 5,
		CookMinutes: 10,
		Tags:        []string{"test"},
		Ingredients: []Ingredient{
			{Name: "rice", Quantity: 100, Unit: "g", Category: "pantry", SearchTerm: "arroz"},
		},
		Steps: []RecipeStep{{Number: 1, Text: "Cook and serve."}},
	}
}
