package food

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

//go:embed recipes/seed/*.json
var seedRecipeFS embed.FS

func RecipesDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "recipes"), nil
}

func RecipeDBPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "recipes.db"), nil
}

func LoadRecipes() ([]Recipe, error) {
	return SearchRecipes(RecipeQuery{})
}

func LoadRecipe(idOrTitle string) (Recipe, bool, error) {
	return loadRecipeFromDB(idOrTitle)
}

func DecodeRecipes(data []byte) ([]Recipe, error) {
	recipes, _, err := decodeRecipeFile(data)
	if err != nil {
		return nil, err
	}
	for i := range recipes {
		recipes[i] = normalizeRecipe(recipes[i])
		if err := ValidateRecipe(recipes[i]); err != nil {
			return nil, err
		}
	}
	return recipes, nil
}

func SaveUserRecipes(recipes []Recipe) ([]string, error) {
	var paths []string
	for _, recipe := range recipes {
		path, err := SaveUserRecipe(recipe)
		if err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func SaveUserRecipe(recipe Recipe) (string, error) {
	recipe = normalizeRecipe(recipe)
	if err := ValidateRecipe(recipe); err != nil {
		return "", err
	}
	return saveUserRecipeToDB(recipe)
}

func RemoveUserRecipe(id string) (bool, error) {
	return removeUserRecipeFromDB(id)
}

func ValidateRecipe(recipe Recipe) error {
	if strings.TrimSpace(recipe.ID) == "" {
		return errors.New("recipe id is required")
	}
	if strings.TrimSpace(recipe.Title) == "" {
		return fmt.Errorf("recipe %q title is required", recipe.ID)
	}
	if recipe.Servings <= 0 {
		return fmt.Errorf("recipe %q servings must be greater than zero", recipe.ID)
	}
	if len(recipe.Ingredients) == 0 {
		return fmt.Errorf("recipe %q must include at least one ingredient", recipe.ID)
	}
	for i, ingredient := range recipe.Ingredients {
		if strings.TrimSpace(ingredient.Name) == "" {
			return fmt.Errorf("recipe %q ingredient %d name is required", recipe.ID, i+1)
		}
		if ingredient.Quantity < 0 {
			return fmt.Errorf("recipe %q ingredient %q quantity cannot be negative", recipe.ID, ingredient.Name)
		}
	}
	if len(recipe.Steps) == 0 {
		return fmt.Errorf("recipe %q must include at least one step", recipe.ID)
	}
	for i, step := range recipe.Steps {
		if strings.TrimSpace(step.Text) == "" {
			return fmt.Errorf("recipe %q step %d text is required", recipe.ID, i+1)
		}
	}
	return nil
}

func readRecipeFile(name string, read func() ([]byte, error)) ([]Recipe, bool, error) {
	data, err := read()
	if err != nil {
		return nil, false, err
	}
	recipes, isArray, err := decodeRecipeFile(data)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", name, err)
	}
	for i := range recipes {
		recipes[i] = normalizeRecipe(recipes[i])
		if err := ValidateRecipe(recipes[i]); err != nil {
			return nil, false, fmt.Errorf("%s: %w", name, err)
		}
	}
	return recipes, isArray, nil
}

func decodeRecipeFile(data []byte) ([]Recipe, bool, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, false, errors.New("empty recipe file")
	}
	if data[0] == '[' {
		var recipes []Recipe
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&recipes); err != nil {
			return nil, true, err
		}
		return recipes, true, nil
	}
	var recipe Recipe
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&recipe); err != nil {
		return nil, false, err
	}
	return []Recipe{recipe}, false, nil
}

func mergeRecipes(recipes *[]Recipe, index map[string]int, loaded []Recipe) {
	for _, recipe := range loaded {
		key := recipeIDKey(recipe.ID)
		if existing, ok := index[key]; ok {
			(*recipes)[existing] = recipe
			continue
		}
		index[key] = len(*recipes)
		*recipes = append(*recipes, recipe)
	}
}

func normalizeRecipe(recipe Recipe) Recipe {
	recipe.ID = strings.TrimSpace(recipe.ID)
	recipe.Title = strings.TrimSpace(recipe.Title)
	for i := range recipe.Tags {
		recipe.Tags[i] = strings.TrimSpace(recipe.Tags[i])
	}
	for i := range recipe.Ingredients {
		recipe.Ingredients[i].Name = strings.TrimSpace(recipe.Ingredients[i].Name)
		recipe.Ingredients[i].Unit = strings.TrimSpace(recipe.Ingredients[i].Unit)
		recipe.Ingredients[i].Category = strings.TrimSpace(recipe.Ingredients[i].Category)
		recipe.Ingredients[i].SearchTerm = strings.TrimSpace(recipe.Ingredients[i].SearchTerm)
	}
	for i := range recipe.Steps {
		if recipe.Steps[i].Number == 0 {
			recipe.Steps[i].Number = i + 1
		}
		recipe.Steps[i].Title = strings.TrimSpace(recipe.Steps[i].Title)
		recipe.Steps[i].Text = strings.TrimSpace(recipe.Steps[i].Text)
	}
	return recipe
}

func recipeIDKey(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

func recipeFileName(id string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(id)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || unicode.IsSpace(r):
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		name = slugID("recipe")
	}
	return name + ".json"
}
