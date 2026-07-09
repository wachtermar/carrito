package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/wachtermar/carrito/internal/food"
	"github.com/wachtermar/carrito/internal/output"
)

func runFoodRecipes(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "usage: carrito food recipes <list|search|show|add|remove> [options]")
		return nil
	}
	switch args[0] {
	case "list":
		return runFoodRecipesList(args[1:], stdout, stderr)
	case "search":
		return runFoodRecipesSearch(args[1:], stdout, stderr)
	case "show":
		return runFoodRecipesShow(args[1:], stdout, stderr)
	case "add":
		return runFoodRecipesAdd(args[1:], stdout, stderr)
	case "remove":
		return runFoodRecipesRemove(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown food recipes command %q", args[0])
	}
}

func runFoodRecipesList(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food recipes list", stderr)
	tag := fs.String("tag", "", "comma-separated recipe tags")
	query := fs.String("query", "", "full-text recipe search query")
	diet := fs.String("diet", "", "comma-separated diet tags to require")
	allergy := fs.String("allergy", "", "comma-separated allergies to exclude")
	dislike := fs.String("dislike", "", "comma-separated disliked ingredients to exclude")
	useProfile := fs.Bool("profile", false, "apply saved profile diets, allergies, and dislikes")
	limit := fs.Int("limit", 0, "maximum recipes to return")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"profile": true, "json": true}); err != nil {
		return err
	}
	recipeQuery, err := buildRecipeQuery(*query, *tag, *diet, *allergy, *dislike, *useProfile, *limit)
	if err != nil {
		return err
	}
	recipes, err := food.SearchRecipes(recipeQuery)
	if err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, recipes)
	}
	for _, recipe := range recipes {
		printRecipeSummary(stdout, recipe)
	}
	return nil
}

func runFoodRecipesSearch(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food recipes search", stderr)
	tag := fs.String("tag", "", "comma-separated recipe tags")
	diet := fs.String("diet", "", "comma-separated diet tags to require")
	allergy := fs.String("allergy", "", "comma-separated allergies to exclude")
	dislike := fs.String("dislike", "", "comma-separated disliked ingredients to exclude")
	useProfile := fs.Bool("profile", false, "apply saved profile diets, allergies, and dislikes")
	limit := fs.Int("limit", 20, "maximum recipes to return")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"profile": true, "json": true}); err != nil {
		return err
	}
	query := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if query == "" {
		return errors.New("food recipes search requires a query")
	}
	recipeQuery, err := buildRecipeQuery(query, *tag, *diet, *allergy, *dislike, *useProfile, *limit)
	if err != nil {
		return err
	}
	recipes, err := food.SearchRecipes(recipeQuery)
	if err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, recipes)
	}
	for _, recipe := range recipes {
		printRecipeSummary(stdout, recipe)
	}
	return nil
}

func buildRecipeQuery(query, tag, diet, allergy, dislike string, useProfile bool, limit int) (food.RecipeQuery, error) {
	recipeQuery := food.RecipeQuery{
		Query:     query,
		Tags:      splitList(tag),
		Diets:     splitList(diet),
		Allergies: splitList(allergy),
		Dislikes:  splitList(dislike),
		Limit:     limit,
	}
	if useProfile {
		profile, err := food.LoadProfile()
		if err != nil {
			return recipeQuery, err
		}
		recipeQuery.Diets = appendUniqueValues(recipeQuery.Diets, profile.Diets...)
		recipeQuery.Allergies = appendUniqueValues(recipeQuery.Allergies, profile.Allergies...)
		recipeQuery.Dislikes = appendUniqueValues(recipeQuery.Dislikes, profile.Dislikes...)
	}
	return recipeQuery, nil
}

func appendUniqueValues(values []string, additions ...string) []string {
	for _, value := range additions {
		values = appendUnique(values, value)
	}
	return values
}

func runFoodRecipesShow(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food recipes show", stderr)
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	ref := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if ref == "" {
		return errors.New("food recipes show requires a recipe id or title")
	}
	recipe, ok, err := food.LoadRecipe(ref)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("recipe %q not found", ref)
	}
	if *jsonOut {
		return output.JSON(stdout, recipe)
	}
	printRecipe(stdout, recipe)
	return nil
}

func runFoodRecipesAdd(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food recipes add", stderr)
	fromText := fs.Bool("from-text", false, "parse a free-text ingredient list instead of JSON")
	urlImport := fs.String("url", "", "import a schema.org Recipe JSON-LD webpage")
	id := fs.String("id", "", "recipe id for --from-text")
	title := fs.String("title", "", "recipe title for --from-text")
	servings := fs.Int("servings", 2, "recipe servings for --from-text")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"from-text": true, "json": true}); err != nil {
		return err
	}
	ref := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if strings.TrimSpace(*urlImport) != "" && *fromText {
		return errors.New("food recipes add --url cannot be combined with --from-text")
	}
	if strings.TrimSpace(*urlImport) != "" && ref != "" {
		return errors.New("food recipes add --url does not accept a file argument")
	}
	if ref == "" && strings.TrimSpace(*urlImport) == "" {
		return errors.New("food recipes add requires <file|->")
	}
	var recipes []food.Recipe
	var source food.RecipeSourceEvidence
	var warnings []string
	var err error
	if *fromText {
		data, err := readInputBytes(ref)
		if err != nil {
			return err
		}
		recipe, err := food.RecipeFromText(string(data), food.TextRecipeOptions{ID: *id, Title: *title, Servings: *servings})
		if err != nil {
			return fmt.Errorf("%w; %s", err, food.TextRecipeUsage())
		}
		recipes = []food.Recipe{recipe}
	} else if strings.TrimSpace(*urlImport) != "" {
		recipes, source, warnings, err = food.RecipesFromStructuredURL(context.Background(), *urlImport, food.RecipeIntakeOptions{})
		if err != nil {
			return err
		}
	} else {
		data, err := readInputBytes(ref)
		if err != nil {
			return err
		}
		recipes, err = food.DecodeRecipes(data)
		if err != nil {
			return err
		}
	}
	paths, err := food.SaveUserRecipes(recipes)
	if err != nil {
		return err
	}
	var sourceOut *food.RecipeSourceEvidence
	if source.SourceID != "" {
		sourceOut = &source
	}
	res := struct {
		Action   string                     `json:"action"`
		Recipes  []food.Recipe              `json:"recipes"`
		Paths    []string                   `json:"paths"`
		Source   *food.RecipeSourceEvidence `json:"source,omitempty"`
		Warnings []string                   `json:"warnings,omitempty"`
	}{Action: "add", Recipes: recipes, Paths: paths, Source: sourceOut, Warnings: warnings}
	if *jsonOut {
		return output.JSON(stdout, res)
	}
	for i, recipe := range recipes {
		path := ""
		if i < len(paths) {
			path = paths[i]
		}
		fmt.Fprintf(stdout, "recipe\t%s\t%s\t%s\n", recipe.ID, recipe.Title, path)
	}
	return nil
}

func runFoodRecipesRemove(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food recipes remove", stderr)
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	ref := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if ref == "" {
		return errors.New("food recipes remove requires a recipe id or title")
	}
	removed, err := food.RemoveUserRecipe(ref)
	if err != nil {
		return err
	}
	if !removed {
		return fmt.Errorf("recipe %q not found in user recipe library", ref)
	}
	res := struct {
		Action  string `json:"action"`
		Recipe  string `json:"recipe"`
		Removed bool   `json:"removed"`
	}{Action: "remove", Recipe: ref, Removed: true}
	if *jsonOut {
		return output.JSON(stdout, res)
	}
	fmt.Fprintf(stdout, "removed-recipe\t%s\n", ref)
	return nil
}

func filterRecipesByTag(recipes []food.Recipe, tag string) []food.Recipe {
	key := normalizeCLITag(tag)
	if key == "" {
		return recipes
	}
	var out []food.Recipe
	for _, recipe := range recipes {
		for _, value := range recipe.Tags {
			if normalizeCLITag(value) == key {
				out = append(out, recipe)
				break
			}
		}
	}
	return out
}

func normalizeCLITag(tag string) string {
	return strings.ToLower(strings.TrimSpace(tag))
}
