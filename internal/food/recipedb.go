package food

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const (
	recipeSourceSeed = "seed"
	recipeSourceUser = "user"
)

type RecipeQuery struct {
	Query     string
	Tags      []string
	Diets     []string
	Allergies []string
	Dislikes  []string
	Limit     int
}

type recipeRecord struct {
	pk     int64
	recipe Recipe
}

func SearchRecipes(query RecipeQuery) ([]Recipe, error) {
	db, err := openRecipeDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return queryRecipes(db, query)
}

func loadRecipeFromDB(idOrTitle string) (Recipe, bool, error) {
	key := normalizeKey(idOrTitle)
	idKey := recipeIDKey(idOrTitle)
	if key == "" && idKey == "" {
		return Recipe{}, false, nil
	}
	db, err := openRecipeDB()
	if err != nil {
		return Recipe{}, false, err
	}
	defer db.Close()

	records, err := selectRecipeRecords(db, `
		SELECT pk, id, title, servings, prep_minutes, cook_minutes, image_url, source_url,
		       nutrition_kcal, nutrition_protein_g, nutrition_carbs_g, nutrition_fat_g
		FROM recipes
		WHERE id_key = ? OR title_key = ?
		ORDER BY CASE WHEN id_key = ? THEN 0 ELSE 1 END, title COLLATE NOCASE
		LIMIT 1`, idKey, key, idKey)
	if err != nil {
		return Recipe{}, false, err
	}
	if len(records) == 0 {
		return Recipe{}, false, nil
	}
	recipes, err := hydrateRecipeRecords(db, records)
	if err != nil {
		return Recipe{}, false, err
	}
	if len(recipes) == 0 {
		return Recipe{}, false, nil
	}
	return recipes[0], true, nil
}

func saveUserRecipeToDB(recipe Recipe) (string, error) {
	db, err := openRecipeDB()
	if err != nil {
		return "", err
	}
	defer db.Close()
	if err := saveRecipe(db, recipe, recipeSourceUser); err != nil {
		return "", err
	}
	return RecipeDBPath()
}

func removeUserRecipeFromDB(idOrTitle string) (bool, error) {
	key := normalizeKey(idOrTitle)
	idKey := recipeIDKey(idOrTitle)
	if key == "" && idKey == "" {
		return false, nil
	}
	db, err := openRecipeDB()
	if err != nil {
		return false, err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var pk int64
	var source string
	err = tx.QueryRow(`
		SELECT pk, source
		FROM recipes
		WHERE id_key = ? OR title_key = ?
		ORDER BY CASE WHEN id_key = ? THEN 0 ELSE 1 END
		LIMIT 1`, idKey, key, idKey).Scan(&pk, &source)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if source != recipeSourceUser {
		return false, nil
	}
	if _, err := tx.Exec(`DELETE FROM recipe_fts WHERE rowid = ?`, pk); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`DELETE FROM recipes WHERE pk = ?`, pk); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	committed = true
	if err := seedRecipeDB(db); err != nil {
		return false, err
	}
	return true, nil
}

func openRecipeDB() (*sql.DB, error) {
	path, err := RecipeDBPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := configureRecipeDB(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := migrateRecipeDB(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := seedRecipeDB(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := importLegacyUserRecipesOnce(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
