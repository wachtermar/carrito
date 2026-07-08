package food

import (
	"database/sql"
	"errors"
)

func configureRecipeDB(db *sql.DB) error {
	pragmas := []string{
		`PRAGMA foreign_keys = ON`,
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA journal_mode = WAL`,
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return err
		}
	}
	return nil
}

func migrateRecipeDB(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS recipe_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS recipes (
			pk INTEGER PRIMARY KEY AUTOINCREMENT,
			id TEXT NOT NULL,
			id_key TEXT NOT NULL UNIQUE,
			title TEXT NOT NULL,
			title_key TEXT NOT NULL,
			servings INTEGER NOT NULL,
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
		)`,
		`CREATE INDEX IF NOT EXISTS idx_recipes_title_key ON recipes(title_key)`,
		`CREATE INDEX IF NOT EXISTS idx_recipes_source ON recipes(source)`,
		`CREATE TABLE IF NOT EXISTS recipe_tags (
			recipe_pk INTEGER NOT NULL REFERENCES recipes(pk) ON DELETE CASCADE,
			pos INTEGER NOT NULL,
			tag TEXT NOT NULL,
			tag_key TEXT NOT NULL,
			PRIMARY KEY(recipe_pk, pos)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_recipe_tags_key ON recipe_tags(tag_key, recipe_pk)`,
		`CREATE TABLE IF NOT EXISTS recipe_ingredients (
			recipe_pk INTEGER NOT NULL REFERENCES recipes(pk) ON DELETE CASCADE,
			pos INTEGER NOT NULL,
			name TEXT NOT NULL,
			quantity REAL NOT NULL DEFAULT 0,
			unit TEXT NOT NULL DEFAULT '',
			category TEXT NOT NULL DEFAULT '',
			search_term TEXT NOT NULL DEFAULT '',
			optional INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY(recipe_pk, pos)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_recipe_ingredients_name ON recipe_ingredients(name)`,
		`CREATE INDEX IF NOT EXISTS idx_recipe_ingredients_search_term ON recipe_ingredients(search_term)`,
		`CREATE TABLE IF NOT EXISTS recipe_equipment (
			recipe_pk INTEGER NOT NULL REFERENCES recipes(pk) ON DELETE CASCADE,
			pos INTEGER NOT NULL,
			item TEXT NOT NULL,
			PRIMARY KEY(recipe_pk, pos)
		)`,
		`CREATE TABLE IF NOT EXISTS recipe_steps (
			recipe_pk INTEGER NOT NULL REFERENCES recipes(pk) ON DELETE CASCADE,
			pos INTEGER NOT NULL,
			number INTEGER NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			text TEXT NOT NULL,
			minutes INTEGER NOT NULL DEFAULT 0,
			image_url TEXT NOT NULL DEFAULT '',
			PRIMARY KEY(recipe_pk, pos)
		)`,
		`CREATE TABLE IF NOT EXISTS recipe_substitutions (
			recipe_pk INTEGER NOT NULL REFERENCES recipes(pk) ON DELETE CASCADE,
			pos INTEGER NOT NULL,
			text TEXT NOT NULL,
			PRIMARY KEY(recipe_pk, pos)
		)`,
		`CREATE TABLE IF NOT EXISTS recipe_allergen_notes (
			recipe_pk INTEGER NOT NULL REFERENCES recipes(pk) ON DELETE CASCADE,
			pos INTEGER NOT NULL,
			text TEXT NOT NULL,
			PRIMARY KEY(recipe_pk, pos)
		)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS recipe_fts USING fts5(title, tags, ingredients, steps)`,
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return setRecipeMeta(db, "schema_version", "1")
}

func recipeMetaValue(db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRow(`SELECT value FROM recipe_meta WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

type recipeMetaSetter interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func setRecipeMeta(exec recipeMetaSetter, key, value string) error {
	_, err := exec.Exec(`
		INSERT INTO recipe_meta(key, value)
		VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}
