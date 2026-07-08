package food

import (
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

func seedRecipeDB(db *sql.DB) error {
	recipes, err := loadEmbeddedSeedRecipes()
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for _, recipe := range recipes {
		if err := upsertRecipeTx(tx, recipe, recipeSourceSeed); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func importLegacyUserRecipesOnce(db *sql.DB) error {
	imported, err := recipeMetaValue(db, "legacy_json_imported")
	if err != nil {
		return err
	}
	if imported == "1" {
		return nil
	}
	recipes, err := loadLegacyUserRecipes()
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for _, recipe := range recipes {
		if err := upsertRecipeTx(tx, recipe, recipeSourceUser); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if err := setRecipeMeta(tx, "legacy_json_imported", "1"); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func loadEmbeddedSeedRecipes() ([]Recipe, error) {
	paths, err := fs.Glob(seedRecipeFS, "recipes/seed/*.json")
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var recipes []Recipe
	index := map[string]int{}
	for _, path := range paths {
		loaded, _, err := readRecipeFile(path, func() ([]byte, error) {
			return seedRecipeFS.ReadFile(path)
		})
		if err != nil {
			return nil, err
		}
		mergeRecipes(&recipes, index, loaded)
	}
	return recipes, nil
}

func loadLegacyUserRecipes() ([]Recipe, error) {
	dir, err := RecipesDir()
	if err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var recipes []Recipe
	for _, path := range paths {
		loaded, _, err := readRecipeFile(path, func() ([]byte, error) {
			return os.ReadFile(path)
		})
		if err != nil {
			return nil, err
		}
		recipes = append(recipes, loaded...)
	}
	return recipes, nil
}
