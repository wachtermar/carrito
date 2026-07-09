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

func TestSeedRecipesUseMeasurableBasketQuantities(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	cases := []struct {
		recipeID   string
		ingredient string
		quantity   float64
		unit       string
	}{
		{"lentil-stew", "carrot", 160, "g"},
		{"lentil-stew", "vegetable stock", 500, "ml"},
		{"chickpea-spinach-curry", "curry spice", 6, "g"},
		{"chicken-fajita-bowls", "bell pepper", 300, "g"},
		{"cod-pisto", "cod fillets", 300, "g"},
		{"cod-pisto", "zucchini", 250, "g"},
		{"cod-pisto", "red pepper", 200, "g"},
		{"cod-pisto", "potato", 400, "g"},
		{"hake-rice-soup", "hake fillets", 300, "g"},
		{"hake-rice-soup", "leek", 180, "g"},
		{"hake-rice-soup", "fish stock", 500, "ml"},
		{"chicken-noodle-soup", "carrot", 160, "g"},
		{"chicken-noodle-soup", "celery", 160, "g"},
		{"chicken-noodle-soup", "chicken stock", 500, "ml"},
	}
	for _, tc := range cases {
		t.Run(tc.recipeID+"/"+tc.ingredient, func(t *testing.T) {
			recipe, ok, err := LoadRecipe(tc.recipeID)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				t.Fatalf("recipe %q not found", tc.recipeID)
			}
			ingredient, ok := recipeIngredientByName(recipe, tc.ingredient)
			if !ok {
				t.Fatalf("ingredient %q not found in %q", tc.ingredient, tc.recipeID)
			}
			if ingredient.Quantity != tc.quantity || ingredient.Unit != tc.unit {
				t.Fatalf("%s/%s quantity = %g %s, want %g %s", tc.recipeID, tc.ingredient, ingredient.Quantity, ingredient.Unit, tc.quantity, tc.unit)
			}
		})
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

func recipeIngredientByName(recipe Recipe, name string) (Ingredient, bool) {
	for _, ingredient := range recipe.Ingredients {
		if ingredient.Name == name {
			return ingredient, true
		}
	}
	return Ingredient{}, false
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

func TestRecipeDatabaseMigrationAddsSourceURLColumn(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	dbPath, err := RecipeDBPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE recipes (
			pk INTEGER PRIMARY KEY AUTOINCREMENT,
			id TEXT NOT NULL,
			id_key TEXT UNIQUE NOT NULL,
			title TEXT NOT NULL,
			title_key TEXT NOT NULL,
			servings INTEGER NOT NULL DEFAULT 0,
			prep_minutes INTEGER NOT NULL DEFAULT 0,
			cook_minutes INTEGER NOT NULL DEFAULT 0,
			image_url TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			nutrition_kcal REAL,
			nutrition_protein_g REAL,
			nutrition_carbs_g REAL,
			nutrition_fat_g REAL
		);
		INSERT INTO recipes (
			id, id_key, title, title_key, servings, prep_minutes, cook_minutes,
			image_url, source, created_at, updated_at
		) VALUES (
			'old-recipe', 'old recipe', 'Old Recipe', 'old recipe', 2, 5, 10,
			'', 'user', '2026-07-09T00:00:00Z', '2026-07-09T00:00:00Z'
		);
	`)
	closeErr := db.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}

	loaded, ok, err := LoadRecipe("old-recipe")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || loaded.ID != "old-recipe" || loaded.SourceURL != "" {
		t.Fatalf("old recipe did not load through migrated schema: ok=%t recipe=%+v", ok, loaded)
	}
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var sourceURL string
	if err := db.QueryRow(`SELECT source_url FROM recipes WHERE id = 'old-recipe'`).Scan(&sourceURL); err != nil {
		t.Fatal(err)
	}
	if sourceURL != "" {
		t.Fatalf("migrated source_url = %q, want empty default", sourceURL)
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
