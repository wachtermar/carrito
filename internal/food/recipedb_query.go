package food

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

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
	if recipes == nil {
		recipes = []Recipe{}
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
	case "", "omnivore", "omnivoro", "omnivora", "no-restriction", "no-restrictions", "none":
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
