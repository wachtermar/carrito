package food

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type TextRecipeOptions struct {
	ID       string
	Title    string
	Servings int
}

var textIngredientRE = regexp.MustCompile(`(?i)^\s*(\d+(?:[,.]\d+)?)(?:\s*(kg|g|gr|l|ml|cl|ud|uds|unit|unidades|unidad))?\s+(.+)$`)

func RecipeFromText(text string, opts TextRecipeOptions) (Recipe, error) {
	servings := opts.Servings
	if servings <= 0 {
		servings = 2
	}
	title := strings.TrimSpace(opts.Title)
	var ingredients []Ingredient
	for _, part := range splitIngredientText(text) {
		if title == "" && looksLikeRecipeTitle(part) {
			title = cleanTextTitle(part)
			continue
		}
		ingredient, ok := ingredientFromText(part)
		if ok {
			ingredients = append(ingredients, ingredient)
		}
	}
	if title == "" {
		title = "Custom Recipe " + nowStamp()
	}
	id := strings.TrimSpace(opts.ID)
	if id == "" {
		id = recipeSlug(title)
	}
	recipe := Recipe{
		ID:          id,
		Title:       title,
		Servings:    servings,
		Tags:        []string{"custom"},
		Ingredients: ingredients,
		Steps: []RecipeStep{
			{Number: 1, Text: "Cook the pasted ingredients according to your recipe notes."},
		},
	}
	recipe = withNutrition(recipe)
	if err := ValidateRecipe(recipe); err != nil {
		return Recipe{}, err
	}
	return recipe, nil
}

func splitIngredientText(text string) []string {
	text = strings.ReplaceAll(text, ";", "\n")
	text = strings.ReplaceAll(text, "\u2022", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	var parts []string
	for _, line := range strings.Split(text, "\n") {
		for _, part := range strings.Split(line, ",") {
			part = strings.TrimSpace(strings.Trim(part, "-* "))
			if part != "" {
				parts = append(parts, part)
			}
		}
	}
	return parts
}

func ingredientFromText(raw string) (Ingredient, bool) {
	part := strings.TrimSpace(raw)
	if part == "" || looksLikeRecipeTitle(part) {
		return Ingredient{}, false
	}
	qty := 1.0
	unit := "unit"
	name := part
	if m := textIngredientRE.FindStringSubmatch(part); len(m) == 4 {
		if value, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64); err == nil && value > 0 {
			qty = value
		}
		if parsedUnit := normalizeUnit(m[2]); parsedUnit != "" {
			unit = parsedUnit
		}
		name = strings.TrimSpace(m[3])
	}
	name = strings.Trim(name, ".: ")
	if name == "" {
		return Ingredient{}, false
	}
	return Ingredient{
		Name:       name,
		Quantity:   roundQty(qty),
		Unit:       unit,
		Category:   inferIngredientCategory(name),
		SearchTerm: spanishSearchTerm(name),
	}, true
}

func looksLikeRecipeTitle(part string) bool {
	key := normalizeKey(part)
	return strings.HasPrefix(key, "title ") || strings.HasPrefix(key, "titulo ") || strings.HasPrefix(key, "recipe ") || strings.HasPrefix(key, "receta ")
}

func cleanTextTitle(part string) string {
	if idx := strings.IndexAny(part, ":=-"); idx >= 0 {
		return strings.TrimSpace(part[idx+1:])
	}
	fields := strings.Fields(part)
	if len(fields) > 1 {
		return strings.Join(fields[1:], " ")
	}
	return strings.TrimSpace(part)
}

func recipeSlug(title string) string {
	slug := recipeFileName(title)
	return strings.TrimSuffix(slug, ".json")
}

func spanishSearchTerm(name string) string {
	key := normalizeKey(name)
	var terms []string
	stop := map[string]bool{
		"de": true, "del": true, "la": true, "el": true, "los": true, "las": true,
		"un": true, "una": true, "y": true, "para": true, "fresco": true, "fresca": true,
	}
	for _, field := range strings.Fields(key) {
		if stop[field] {
			continue
		}
		terms = append(terms, singularSpanish(field))
	}
	if len(terms) == 0 {
		return key
	}
	return strings.Join(terms, " ")
}

func singularSpanish(value string) string {
	switch {
	case strings.HasSuffix(value, "ces") && len(value) > 3:
		return strings.TrimSuffix(value, "ces") + "z"
	case strings.HasSuffix(value, "es") && len(value) > 4:
		return strings.TrimSuffix(value, "es")
	case strings.HasSuffix(value, "s") && len(value) > 3:
		return strings.TrimSuffix(value, "s")
	default:
		return value
	}
}

func inferIngredientCategory(name string) string {
	key := normalizeKey(name)
	switch {
	case strings.Contains(key, "pollo") || strings.Contains(key, "pavo") || strings.Contains(key, "ternera") || strings.Contains(key, "carne"):
		return "meat"
	case strings.Contains(key, "salmon") || strings.Contains(key, "atun") || strings.Contains(key, "merluza") || strings.Contains(key, "bacalao") || strings.Contains(key, "pescado"):
		return "fish"
	case strings.Contains(key, "leche") || strings.Contains(key, "yogur") || strings.Contains(key, "queso"):
		return "dairy"
	case strings.Contains(key, "arroz") || strings.Contains(key, "pasta") || strings.Contains(key, "lenteja") || strings.Contains(key, "garbanzo") || strings.Contains(key, "alubia"):
		return "pantry"
	case strings.Contains(key, "manzana") || strings.Contains(key, "platano") || strings.Contains(key, "naranja") || strings.Contains(key, "limon") || strings.Contains(key, "fruta"):
		return "fruit"
	case strings.Contains(key, "tomate") || strings.Contains(key, "cebolla") || strings.Contains(key, "patata") || strings.Contains(key, "zanahoria") || strings.Contains(key, "pimiento") || strings.Contains(key, "verdura"):
		return "vegetables"
	default:
		return "pantry"
	}
}

func TextRecipeUsage() string {
	return fmt.Sprintf("recipe text should include comma- or newline-separated ingredients, for example: %q", "2 pechugas de pollo, 1 cebolla, 200 g arroz")
}
