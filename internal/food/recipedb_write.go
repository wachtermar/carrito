package food

import (
	"database/sql"
	"errors"
	"strings"
)

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
			source_url, source, created_at, updated_at, nutrition_kcal, nutrition_protein_g,
			nutrition_carbs_g, nutrition_fat_g
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id_key) DO UPDATE SET
			id = excluded.id,
			title = excluded.title,
			title_key = excluded.title_key,
			servings = excluded.servings,
			prep_minutes = excluded.prep_minutes,
			cook_minutes = excluded.cook_minutes,
			image_url = excluded.image_url,
			source_url = excluded.source_url,
			source = excluded.source,
			updated_at = excluded.updated_at,
			nutrition_kcal = excluded.nutrition_kcal,
			nutrition_protein_g = excluded.nutrition_protein_g,
			nutrition_carbs_g = excluded.nutrition_carbs_g,
			nutrition_fat_g = excluded.nutrition_fat_g`,
		recipe.ID, idKey, recipe.Title, normalizeKey(recipe.Title), recipe.Servings,
		recipe.PrepMinutes, recipe.CookMinutes, recipe.ImageURL, recipe.SourceURL, source, now, now,
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
