package food

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

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
		SELECT pk, id, title, servings, prep_minutes, cook_minutes, image_url,
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

func saveRecipe(db *sql.DB, recipe Recipe, source string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if err := upsertRecipeTx(tx, recipe, source); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func upsertRecipeTx(tx *sql.Tx, recipe Recipe, source string) error {
	recipe = normalizeRecipe(recipe)
	if err := ValidateRecipe(recipe); err != nil {
		return err
	}
	idKey := recipeIDKey(recipe.ID)
	if source == recipeSourceSeed {
		var existingSource string
		err := tx.QueryRow(`SELECT source FROM recipes WHERE id_key = ?`, idKey).Scan(&existingSource)
		if err == nil && existingSource == recipeSourceUser {
			return nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	now := nowStamp()
	nutrition := recipe.NutritionPerServing
	var kcal, protein, carbs, fat any
	if nutrition != nil {
		kcal = nutrition.Kcal
		protein = nutrition.ProteinG
		carbs = nutrition.CarbsG
		fat = nutrition.FatG
	}
	_, err := tx.Exec(`
		INSERT INTO recipes (
			id, id_key, title, title_key, servings, prep_minutes, cook_minutes, image_url,
			source, created_at, updated_at, nutrition_kcal, nutrition_protein_g,
			nutrition_carbs_g, nutrition_fat_g
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id_key) DO UPDATE SET
			id = excluded.id,
			title = excluded.title,
			title_key = excluded.title_key,
			servings = excluded.servings,
			prep_minutes = excluded.prep_minutes,
			cook_minutes = excluded.cook_minutes,
			image_url = excluded.image_url,
			source = excluded.source,
			updated_at = excluded.updated_at,
			nutrition_kcal = excluded.nutrition_kcal,
			nutrition_protein_g = excluded.nutrition_protein_g,
			nutrition_carbs_g = excluded.nutrition_carbs_g,
			nutrition_fat_g = excluded.nutrition_fat_g`,
		recipe.ID, idKey, recipe.Title, normalizeKey(recipe.Title), recipe.Servings,
		recipe.PrepMinutes, recipe.CookMinutes, recipe.ImageURL, source, now, now,
		kcal, protein, carbs, fat)
	if err != nil {
		return err
	}
	var pk int64
	if err := tx.QueryRow(`SELECT pk FROM recipes WHERE id_key = ?`, idKey).Scan(&pk); err != nil {
		return err
	}
	if err := deleteRecipeDetailsTx(tx, pk); err != nil {
		return err
	}
	if err := insertRecipeDetailsTx(tx, pk, recipe); err != nil {
		return err
	}
	return nil
}

func deleteRecipeDetailsTx(tx *sql.Tx, pk int64) error {
	tables := []string{
		"recipe_tags",
		"recipe_ingredients",
		"recipe_equipment",
		"recipe_steps",
		"recipe_substitutions",
		"recipe_allergen_notes",
	}
	for _, table := range tables {
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE recipe_pk = ?`, pk); err != nil {
			return err
		}
	}
	_, err := tx.Exec(`DELETE FROM recipe_fts WHERE rowid = ?`, pk)
	return err
}

func insertRecipeDetailsTx(tx *sql.Tx, pk int64, recipe Recipe) error {
	for i, tag := range recipe.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO recipe_tags(recipe_pk, pos, tag, tag_key) VALUES (?, ?, ?, ?)`, pk, i, tag, normalizeKey(tag)); err != nil {
			return err
		}
	}
	for i, ingredient := range recipe.Ingredients {
		optional := 0
		if ingredient.Optional {
			optional = 1
		}
		if _, err := tx.Exec(`
			INSERT INTO recipe_ingredients(recipe_pk, pos, name, quantity, unit, category, search_term, optional)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			pk, i, ingredient.Name, ingredient.Quantity, ingredient.Unit, ingredient.Category, ingredient.SearchTerm, optional); err != nil {
			return err
		}
	}
	for i, item := range recipe.Equipment {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO recipe_equipment(recipe_pk, pos, item) VALUES (?, ?, ?)`, pk, i, item); err != nil {
			return err
		}
	}
	for i, step := range recipe.Steps {
		if _, err := tx.Exec(`
			INSERT INTO recipe_steps(recipe_pk, pos, number, title, text, minutes, image_url)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			pk, i, step.Number, step.Title, step.Text, step.Minutes, step.ImageURL); err != nil {
			return err
		}
	}
	for i, text := range recipe.Substitutions {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO recipe_substitutions(recipe_pk, pos, text) VALUES (?, ?, ?)`, pk, i, text); err != nil {
			return err
		}
	}
	for i, text := range recipe.AllergenNotes {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO recipe_allergen_notes(recipe_pk, pos, text) VALUES (?, ?, ?)`, pk, i, text); err != nil {
			return err
		}
	}
	_, err := tx.Exec(`
		INSERT INTO recipe_fts(rowid, title, tags, ingredients, steps)
		VALUES (?, ?, ?, ?, ?)`,
		pk, recipe.Title, strings.Join(recipe.Tags, " "), recipeIngredientsText(recipe), recipeStepsText(recipe))
	return err
}

func queryRecipes(db *sql.DB, query RecipeQuery) ([]Recipe, error) {
	var args []any
	var joins []string
	var wheres []string
	fts := recipeFTSQuery(query.Query)
	if fts != "" {
		joins = append(joins, `JOIN recipe_fts ON recipe_fts.rowid = r.pk`)
		wheres = append(wheres, `recipe_fts MATCH ?`)
		args = append(args, fts)
	}
	for _, tag := range query.Tags {
		key := normalizeKey(tag)
		if key == "" {
			continue
		}
		wheres = append(wheres, `EXISTS (SELECT 1 FROM recipe_tags rt WHERE rt.recipe_pk = r.pk AND rt.tag_key = ?)`)
		args = append(args, key)
	}
	for _, diet := range query.Diets {
		keys := dietTagKeys(diet)
		if len(keys) == 0 {
			continue
		}
		placeholders := placeholders(len(keys))
		wheres = append(wheres, fmt.Sprintf(`EXISTS (SELECT 1 FROM recipe_tags rd WHERE rd.recipe_pk = r.pk AND rd.tag_key IN (%s))`, placeholders))
		for _, key := range keys {
			args = append(args, key)
		}
	}

	sqlText := `
		SELECT r.pk, r.id, r.title, r.servings, r.prep_minutes, r.cook_minutes, r.image_url,
		       r.nutrition_kcal, r.nutrition_protein_g, r.nutrition_carbs_g, r.nutrition_fat_g
		FROM recipes r
		` + strings.Join(joins, "\n")
	if len(wheres) > 0 {
		sqlText += "\nWHERE " + strings.Join(wheres, " AND ")
	}
	if fts != "" {
		sqlText += "\nORDER BY bm25(recipe_fts), r.title COLLATE NOCASE"
	} else {
		sqlText += "\nORDER BY r.title COLLATE NOCASE"
	}
	if query.Limit > 0 {
		sqlText += "\nLIMIT ?"
		args = append(args, query.Limit)
	}
	records, err := selectRecipeRecords(db, sqlText, args...)
	if err != nil {
		return nil, err
	}
	recipes, err := hydrateRecipeRecords(db, records)
	if err != nil {
		return nil, err
	}
	if len(query.Allergies) > 0 || len(query.Dislikes) > 0 || len(query.Diets) > 0 {
		recipes = filterTemplates(recipes, Profile{Diets: query.Diets, Allergies: query.Allergies, Dislikes: query.Dislikes})
	}
	if len(recipes) == 0 && strings.TrimSpace(query.Query) == "" && len(query.Tags) == 0 && len(query.Diets) == 0 && len(query.Allergies) == 0 && len(query.Dislikes) == 0 {
		return nil, errors.New("recipe library is empty")
	}
	return recipes, nil
}

func selectRecipeRecords(db *sql.DB, query string, args ...any) ([]recipeRecord, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []recipeRecord
	for rows.Next() {
		var record recipeRecord
		var kcal, protein, carbs, fat sql.NullFloat64
		if err := rows.Scan(
			&record.pk,
			&record.recipe.ID,
			&record.recipe.Title,
			&record.recipe.Servings,
			&record.recipe.PrepMinutes,
			&record.recipe.CookMinutes,
			&record.recipe.ImageURL,
			&kcal,
			&protein,
			&carbs,
			&fat,
		); err != nil {
			return nil, err
		}
		if kcal.Valid || protein.Valid || carbs.Valid || fat.Valid {
			record.recipe.NutritionPerServing = &NutritionSummary{
				Kcal:     nullFloat(kcal),
				ProteinG: nullFloat(protein),
				CarbsG:   nullFloat(carbs),
				FatG:     nullFloat(fat),
			}
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func hydrateRecipeRecords(db *sql.DB, records []recipeRecord) ([]Recipe, error) {
	if len(records) == 0 {
		return nil, nil
	}
	index := map[int64]int{}
	pks := make([]int64, 0, len(records))
	for i := range records {
		index[records[i].pk] = i
		pks = append(pks, records[i].pk)
	}
	if err := loadRecipeTags(db, records, index, pks); err != nil {
		return nil, err
	}
	if err := loadRecipeIngredients(db, records, index, pks); err != nil {
		return nil, err
	}
	if err := loadRecipeEquipment(db, records, index, pks); err != nil {
		return nil, err
	}
	if err := loadRecipeSteps(db, records, index, pks); err != nil {
		return nil, err
	}
	if err := loadRecipeTextList(db, records, index, pks, "recipe_substitutions", func(recipe *Recipe, value string) {
		recipe.Substitutions = append(recipe.Substitutions, value)
	}); err != nil {
		return nil, err
	}
	if err := loadRecipeTextList(db, records, index, pks, "recipe_allergen_notes", func(recipe *Recipe, value string) {
		recipe.AllergenNotes = append(recipe.AllergenNotes, value)
	}); err != nil {
		return nil, err
	}
	recipes := make([]Recipe, 0, len(records))
	for _, record := range records {
		recipes = append(recipes, record.recipe)
	}
	return recipes, nil
}

func loadRecipeTags(db *sql.DB, records []recipeRecord, index map[int64]int, pks []int64) error {
	return queryRecipePKChunks(db, pks, `SELECT recipe_pk, tag FROM recipe_tags WHERE recipe_pk IN (%s) ORDER BY recipe_pk, pos`, func(rows *sql.Rows) error {
		var pk int64
		var tag string
		if err := rows.Scan(&pk, &tag); err != nil {
			return err
		}
		i, ok := index[pk]
		if ok {
			records[i].recipe.Tags = append(records[i].recipe.Tags, tag)
		}
		return nil
	})
}

func loadRecipeIngredients(db *sql.DB, records []recipeRecord, index map[int64]int, pks []int64) error {
	return queryRecipePKChunks(db, pks, `
		SELECT recipe_pk, name, quantity, unit, category, search_term, optional
		FROM recipe_ingredients
		WHERE recipe_pk IN (%s)
		ORDER BY recipe_pk, pos`, func(rows *sql.Rows) error {
		var pk int64
		var ingredient Ingredient
		var optional int
		if err := rows.Scan(&pk, &ingredient.Name, &ingredient.Quantity, &ingredient.Unit, &ingredient.Category, &ingredient.SearchTerm, &optional); err != nil {
			return err
		}
		ingredient.Optional = optional != 0
		i, ok := index[pk]
		if ok {
			records[i].recipe.Ingredients = append(records[i].recipe.Ingredients, ingredient)
		}
		return nil
	})
}

func loadRecipeEquipment(db *sql.DB, records []recipeRecord, index map[int64]int, pks []int64) error {
	return queryRecipePKChunks(db, pks, `SELECT recipe_pk, item FROM recipe_equipment WHERE recipe_pk IN (%s) ORDER BY recipe_pk, pos`, func(rows *sql.Rows) error {
		var pk int64
		var item string
		if err := rows.Scan(&pk, &item); err != nil {
			return err
		}
		i, ok := index[pk]
		if ok {
			records[i].recipe.Equipment = append(records[i].recipe.Equipment, item)
		}
		return nil
	})
}

func loadRecipeSteps(db *sql.DB, records []recipeRecord, index map[int64]int, pks []int64) error {
	return queryRecipePKChunks(db, pks, `
		SELECT recipe_pk, number, title, text, minutes, image_url
		FROM recipe_steps
		WHERE recipe_pk IN (%s)
		ORDER BY recipe_pk, pos`, func(rows *sql.Rows) error {
		var pk int64
		var step RecipeStep
		if err := rows.Scan(&pk, &step.Number, &step.Title, &step.Text, &step.Minutes, &step.ImageURL); err != nil {
			return err
		}
		i, ok := index[pk]
		if ok {
			records[i].recipe.Steps = append(records[i].recipe.Steps, step)
		}
		return nil
	})
}

func loadRecipeTextList(db *sql.DB, records []recipeRecord, index map[int64]int, pks []int64, table string, appendValue func(*Recipe, string)) error {
	query := fmt.Sprintf(`SELECT recipe_pk, text FROM %s WHERE recipe_pk IN (%%s) ORDER BY recipe_pk, pos`, table)
	return queryRecipePKChunks(db, pks, query, func(rows *sql.Rows) error {
		var pk int64
		var text string
		if err := rows.Scan(&pk, &text); err != nil {
			return err
		}
		i, ok := index[pk]
		if ok {
			appendValue(&records[i].recipe, text)
		}
		return nil
	})
}

func queryRecipePKChunks(db *sql.DB, pks []int64, queryTemplate string, scan func(*sql.Rows) error) error {
	const chunkSize = 500
	for start := 0; start < len(pks); start += chunkSize {
		end := start + chunkSize
		if end > len(pks) {
			end = len(pks)
		}
		chunk := pks[start:end]
		args := make([]any, len(chunk))
		for i, pk := range chunk {
			args[i] = pk
		}
		rows, err := db.Query(fmt.Sprintf(queryTemplate, placeholders(len(chunk))), args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			if err := scan(rows); err != nil {
				rows.Close()
				return err
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}
	return nil
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

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", n), ",")
}

func nullFloat(value sql.NullFloat64) float64 {
	if !value.Valid {
		return 0
	}
	return value.Float64
}

func dietTagKeys(diet string) []string {
	switch normalizeKey(diet) {
	case "":
		return nil
	case "vegetarian", "vegetariano", "vegetariana":
		return []string{"vegetarian", "vegan"}
	case "vegan", "vegano", "vegana":
		return []string{"vegan"}
	default:
		return []string{normalizeKey(diet)}
	}
}

func recipeFTSQuery(query string) string {
	var tokens []string
	for _, token := range strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	}) {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		tokens = append(tokens, token+"*")
	}
	return strings.Join(tokens, " AND ")
}

func recipeIngredientsText(recipe Recipe) string {
	var parts []string
	for _, ingredient := range recipe.Ingredients {
		parts = append(parts, ingredient.Name, ingredient.SearchTerm, ingredient.Category)
	}
	return strings.Join(parts, " ")
}

func recipeStepsText(recipe Recipe) string {
	var parts []string
	for _, step := range recipe.Steps {
		parts = append(parts, step.Title, step.Text)
	}
	parts = append(parts, recipe.Equipment...)
	parts = append(parts, recipe.Substitutions...)
	parts = append(parts, recipe.AllergenNotes...)
	return strings.Join(parts, " ")
}
