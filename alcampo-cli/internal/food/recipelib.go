package food

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
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

func LoadRecipes() ([]Recipe, error) {
	var recipes []Recipe
	index := map[string]int{}
	if err := loadSeedRecipes(&recipes, index); err != nil {
		return nil, err
	}
	if err := loadUserRecipes(&recipes, index); err != nil {
		return nil, err
	}
	if len(recipes) == 0 {
		return nil, errors.New("recipe library is empty")
	}
	return recipes, nil
}

func LoadRecipe(idOrTitle string) (Recipe, bool, error) {
	key := normalizeKey(idOrTitle)
	idKey := recipeIDKey(idOrTitle)
	if key == "" && idKey == "" {
		return Recipe{}, false, nil
	}
	recipes, err := LoadRecipes()
	if err != nil {
		return Recipe{}, false, err
	}
	for _, recipe := range recipes {
		if recipeIDKey(recipe.ID) == idKey || normalizeKey(recipe.ID) == key || normalizeKey(recipe.Title) == key {
			return recipe, true, nil
		}
	}
	return Recipe{}, false, nil
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
	dir, err := RecipesDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, recipeFileName(recipe.ID))
	return path, writeJSONFile(path, recipe)
}

func RemoveUserRecipe(id string) (bool, error) {
	dir, err := RecipesDir()
	if err != nil {
		return false, err
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return false, err
	}
	sort.Strings(paths)
	target := recipeIDKey(id)
	for _, path := range paths {
		recipes, isArray, err := readRecipeFile(path, func() ([]byte, error) {
			return os.ReadFile(path)
		})
		if err != nil {
			return false, err
		}
		var kept []Recipe
		removed := false
		for _, recipe := range recipes {
			if recipeIDKey(recipe.ID) == target || normalizeKey(recipe.Title) == normalizeKey(id) {
				removed = true
				continue
			}
			kept = append(kept, recipe)
		}
		if !removed {
			continue
		}
		if len(kept) == 0 {
			return true, os.Remove(path)
		}
		if isArray {
			return true, writeJSONFile(path, kept)
		}
		return false, fmt.Errorf("cannot remove %q from %s without removing the only recipe", id, path)
	}
	return false, nil
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

func loadSeedRecipes(recipes *[]Recipe, index map[string]int) error {
	paths, err := fs.Glob(seedRecipeFS, "recipes/seed/*.json")
	if err != nil {
		return err
	}
	sort.Strings(paths)
	for _, path := range paths {
		loaded, _, err := readRecipeFile(path, func() ([]byte, error) {
			return seedRecipeFS.ReadFile(path)
		})
		if err != nil {
			return err
		}
		mergeRecipes(recipes, index, loaded)
	}
	return nil
}

func loadUserRecipes(recipes *[]Recipe, index map[string]int) error {
	dir, err := RecipesDir()
	if err != nil {
		return err
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return err
	}
	sort.Strings(paths)
	for _, path := range paths {
		loaded, _, err := readRecipeFile(path, func() ([]byte, error) {
			return os.ReadFile(path)
		})
		if err != nil {
			return err
		}
		mergeRecipes(recipes, index, loaded)
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
