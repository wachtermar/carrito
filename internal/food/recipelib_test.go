package food

import (
	"database/sql"
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

func TestLegacyRecipeJSONImportedOnceIntoDatabase(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	dir, err := RecipesDir()
	if err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(dir, "legacy.json")
	if err := writeJSONFile(legacy, testRecipe("legacy-beans", "Legacy Beans")); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := LoadRecipe("legacy-beans")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || loaded.Title != "Legacy Beans" {
		t.Fatalf("legacy recipe not imported: ok=%t recipe=%+v", ok, loaded)
	}
	dbPath, err := RecipeDBPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte(`{"id":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRecipes(); err != nil {
		t.Fatalf("LoadRecipes should ignore legacy JSON after import: %v", err)
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
	if filepath.Base(path) != "recipes.db" {
		t.Fatalf("SaveUserRecipe path = %s, want recipes.db", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	dir, err := RecipesDir()
	if err != nil {
		t.Fatal(err)
	}
	jsonFiles, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(jsonFiles) != 0 {
		t.Fatalf("SaveUserRecipe wrote JSON files: %v", jsonFiles)
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

func TestRemoveUserOverrideRestoresSeedRecipe(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	override := testRecipe("spanish-tortilla", "Custom Tortilla")
	if _, err := SaveUserRecipe(override); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := LoadRecipe("spanish-tortilla")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || loaded.Title != "Custom Tortilla" {
		t.Fatalf("override not loaded: ok=%t recipe=%+v", ok, loaded)
	}
	removed, err := RemoveUserRecipe("spanish-tortilla")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("RemoveUserRecipe returned false")
	}
	loaded, ok, err = LoadRecipe("spanish-tortilla")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || loaded.Title == "Custom Tortilla" {
		t.Fatalf("seed recipe not restored: ok=%t recipe=%+v", ok, loaded)
	}
}

func TestSearchRecipesUsesFTSAndDietTags(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	recipes, err := SearchRecipes(RecipeQuery{Query: "chickpea", Diets: []string{"vegan"}, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(recipes) == 0 {
		t.Fatal("expected chickpea vegan recipes")
	}
	for _, recipe := range recipes {
		if !hasTag(recipe, "vegan") {
			t.Fatalf("non-vegan recipe returned: %+v", recipe)
		}
		text := recipeText(recipe) + " " + normalizeKey(strings.Join(recipe.Substitutions, " "))
		if !strings.Contains(text, "chickpea") && !strings.Contains(text, "garbanzos") {
			t.Fatalf("recipe does not match query: %+v", recipe)
		}
	}
}

func TestRecipeDatabaseSchemaHasNormalizedTables(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	if _, err := LoadRecipes(); err != nil {
		t.Fatal(err)
	}
	dbPath, err := RecipeDBPath()
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type IN ('table', 'virtual table')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	tables := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"recipes",
		"recipe_tags",
		"recipe_ingredients",
		"recipe_equipment",
		"recipe_steps",
		"recipe_substitutions",
		"recipe_allergen_notes",
		"recipe_fts",
	} {
		if !tables[name] {
			t.Fatalf("missing recipe database table %q; got %v", name, tables)
		}
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

func hasTag(recipe Recipe, tag string) bool {
	for _, value := range recipe.Tags {
		if strings.EqualFold(value, tag) {
			return true
		}
	}
	return false
}
