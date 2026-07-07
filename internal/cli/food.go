package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/food"
	"github.com/wachtermar/carrito/internal/output"
	"github.com/wachtermar/carrito/internal/strutil"

	"golang.org/x/term"
)

func runFood(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printFoodHelp(stdout)
		return nil
	}
	switch args[0] {
	case "profile":
		return runFoodProfile(args[1:], stdout, stderr)
	case "pantry":
		return runFoodPantry(args[1:], stdout, stderr)
	case "plan":
		return runFoodPlan(args[1:], stdout, stderr)
	case "use-up":
		return runFoodUseUp(args[1:], stdout, stderr)
	case "recipe":
		return runFoodRecipe(args[1:], stdout, stderr)
	case "recipes":
		return runFoodRecipes(args[1:], stdout, stderr)
	case "shop":
		return runFoodShop(args[1:], stdout, stderr)
	case "pdf":
		return runFoodPDF(args[1:], stdout, stderr)
	case "run":
		return runFoodRun(args[1:], stdout, stderr)
	case "cook":
		return runFoodCook(args[1:], stdout, stderr)
	case "receive":
		return runFoodReceive(args[1:], stdout, stderr)
	case "import-orders":
		return runFoodImportOrders(args[1:], stdout, stderr)
	case "import-receipt":
		return runFoodImportReceipt(args[1:], stdout, stderr)
	case "history":
		return runFoodHistory(args[1:], stdout, stderr)
	case "staples":
		return runFoodStaples(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown food command %q", args[0])
	}
}

func runFoodProfile(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "usage: carrito food profile <get|set> [options]")
		return nil
	}
	switch args[0] {
	case "get":
		fs := newFlagSet("food profile get", stderr)
		jsonOut := fs.Bool("json", false, "write JSON to stdout")
		if err := parseInterspersed(fs, args[1:], map[string]bool{"json": true}); err != nil {
			return err
		}
		profile, err := food.LoadProfile()
		if err != nil {
			return err
		}
		if *jsonOut {
			return output.JSON(stdout, profile)
		}
		printFoodProfile(stdout, profile)
		return nil
	case "set":
		return runFoodProfileSet(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown food profile command %q", args[0])
	}
}

func runFoodProfileSet(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food profile set", stderr)
	people := fs.Int("people", -1, "household people count")
	policy := fs.String("selection-policy", "", "balanced, cheapest, or quality")
	budget := fs.String("budget", "", "default budget in EUR")
	diets := fs.String("diets", "", "comma-separated diets")
	allergies := fs.String("allergies", "", "comma-separated allergies")
	dislikes := fs.String("dislikes", "", "comma-separated disliked ingredients")
	likedCuisines := fs.String("liked-cuisines", "", "comma-separated liked cuisines")
	likedRecipes := fs.String("liked-recipes", "", "comma-separated liked recipes")
	rejectedRecipes := fs.String("rejected-recipes", "", "comma-separated rejected recipe ids or titles")
	likedProducts := fs.String("liked-products", "", "comma-separated liked product SKUs, ids, EANs, brands, or names")
	rejectedProducts := fs.String("rejected-products", "", "comma-separated rejected product SKUs, ids, EANs, brands, or names")
	preferredBrands := fs.String("preferred-brands", "", "comma-separated preferred brands")
	rejectedBrands := fs.String("rejected-brands", "", "comma-separated rejected brands")
	nutritionGoals := fs.String("nutrition-goals", "", "comma-separated nutrition goals")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	profile, err := food.LoadProfile()
	if err != nil {
		return err
	}
	if *people >= 0 {
		profile.People = *people
	}
	if *policy != "" {
		normalized, ok := food.NormalizeSelectionPolicy(*policy)
		if !ok || normalized == "" {
			return fmt.Errorf("unknown selection policy %q", *policy)
		}
		profile.SelectionPolicy = normalized
	}
	if *budget != "" {
		profile.BudgetEUR = *budget
	}
	for _, field := range []struct {
		src *string
		dst *[]string
	}{
		{diets, &profile.Diets},
		{allergies, &profile.Allergies},
		{dislikes, &profile.Dislikes},
		{likedCuisines, &profile.LikedCuisines},
		{likedRecipes, &profile.LikedRecipes},
		{rejectedRecipes, &profile.RejectedRecipes},
		{likedProducts, &profile.LikedProducts},
		{rejectedProducts, &profile.RejectedProducts},
		{preferredBrands, &profile.PreferredBrands},
		{rejectedBrands, &profile.RejectedBrands},
		{nutritionGoals, &profile.NutritionGoals},
	} {
		if *field.src != "" {
			*field.dst = splitList(*field.src)
		}
	}
	if err := food.SaveProfile(profile); err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, profile)
	}
	printFoodProfile(stdout, profile)
	return nil
}

func runFoodPantry(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "usage: carrito food pantry <list|add|use|update> [options]")
		return nil
	}
	switch args[0] {
	case "list":
		fs := newFlagSet("food pantry list", stderr)
		expiringDays := fs.Int("expiring-days", -1, "only show items expiring within N days")
		jsonOut := fs.Bool("json", false, "write JSON to stdout")
		if err := parseInterspersed(fs, args[1:], map[string]bool{"json": true}); err != nil {
			return err
		}
		pantry, err := food.LoadPantry()
		if err != nil {
			return err
		}
		if *expiringDays >= 0 {
			pantry = food.FilterExpiringPantry(pantry, *expiringDays)
		}
		if *jsonOut {
			return output.JSON(stdout, pantry)
		}
		printPantry(stdout, pantry)
		return nil
	case "add", "update":
		return runFoodPantrySet(args[0], args[1:], stdout, stderr)
	case "use":
		return runFoodPantryUse(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown food pantry command %q", args[0])
	}
}

func runFoodPantrySet(action string, args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food pantry "+action, stderr)
	itemName := fs.String("item", "", "item name")
	qty := fs.Float64("qty", 1, "quantity")
	unit := fs.String("unit", "", "unit")
	location := fs.String("location", "", "pantry, fridge, or freezer")
	expiry := fs.String("expiry", "", "expiry date YYYY-MM-DD")
	confidence := fs.Float64("confidence", 1, "confidence from 0 to 1")
	notes := fs.String("notes", "", "free-text notes")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if *itemName == "" && fs.NArg() > 0 {
		*itemName = strings.Join(fs.Args(), " ")
	}
	pantry, err := food.LoadPantry()
	if err != nil {
		return err
	}
	item := food.PantryItem{
		Name:       *itemName,
		Quantity:   *qty,
		Unit:       *unit,
		Location:   *location,
		ExpiryDate: *expiry,
		Confidence: *confidence,
		Notes:      *notes,
	}
	var saved food.PantryItem
	if action == "update" {
		pantry, saved, err = food.SetPantryItem(pantry, item)
	} else {
		pantry, saved, err = food.AddOrUpdatePantryItem(pantry, item)
	}
	if err != nil {
		return err
	}
	if err := food.SavePantry(pantry); err != nil {
		return err
	}
	res := struct {
		Action string          `json:"action"`
		Item   food.PantryItem `json:"item"`
		Pantry food.Pantry     `json:"pantry"`
	}{Action: action, Item: saved, Pantry: pantry}
	if *jsonOut {
		return output.JSON(stdout, res)
	}
	fmt.Fprintf(stdout, "%s\t%s\t%.3g %s\t%s\n", action, saved.Name, saved.Quantity, saved.Unit, strutil.FirstNonEmpty(saved.Location, "-"))
	return nil
}

func runFoodPantryUse(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food pantry use", stderr)
	itemName := fs.String("item", "", "item name")
	qty := fs.Float64("qty", 1, "quantity to use")
	unit := fs.String("unit", "", "unit")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if *itemName == "" && fs.NArg() > 0 {
		*itemName = strings.Join(fs.Args(), " ")
	}
	pantry, err := food.LoadPantry()
	if err != nil {
		return err
	}
	pantry, used, err := food.UsePantryItem(pantry, *itemName, *qty, *unit)
	if err != nil {
		return err
	}
	if err := food.SavePantry(pantry); err != nil {
		return err
	}
	res := struct {
		Action string          `json:"action"`
		Item   food.PantryItem `json:"item"`
		Pantry food.Pantry     `json:"pantry"`
	}{Action: "use", Item: used, Pantry: pantry}
	if *jsonOut {
		return output.JSON(stdout, res)
	}
	fmt.Fprintf(stdout, "use\t%s\tremaining=%.3g %s\n", used.Name, used.Quantity, used.Unit)
	return nil
}

func runFoodPlan(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food plan", stderr)
	days := fs.Int("days", 3, "number of days")
	people := fs.Int("people", 0, "people count; required unless saved in profile")
	budget := fs.String("budget", "", "budget in EUR")
	meals := fs.String("meals", "dinner", "comma-separated meal types")
	outPath := fs.String("out", "", "optional output JSON path")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	profile, err := food.LoadProfile()
	if err != nil {
		return err
	}
	pantry, err := food.LoadPantry()
	if err != nil {
		return err
	}
	plan, err := food.GenerateMealPlan(profile, pantry, food.PlanOptions{
		Days:      *days,
		People:    *people,
		BudgetEUR: *budget,
		MealTypes: splitList(*meals),
	})
	if err != nil {
		return err
	}
	if *outPath != "" {
		plan.File = *outPath
		if err := writeJSONPath(*outPath, plan); err != nil {
			return err
		}
	} else {
		plan, err = food.SaveMealPlan(plan)
		if err != nil {
			return err
		}
	}
	if *jsonOut {
		return output.JSON(stdout, plan)
	}
	printMealPlan(stdout, plan)
	return nil
}

func runFoodUseUp(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food use-up", stderr)
	days := fs.Int("expiring-days", 3, "rank recipes using pantry items expiring within N days")
	limit := fs.Int("limit", 8, "maximum recipes to return")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	profile, err := food.LoadProfile()
	if err != nil {
		return err
	}
	pantry, err := food.LoadPantry()
	if err != nil {
		return err
	}
	suggestions, err := food.SuggestUseUpRecipes(profile, pantry, *days, *limit)
	if err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, suggestions)
	}
	if len(suggestions) == 0 {
		fmt.Fprintln(stdout, "no expiring pantry matches found")
		return nil
	}
	for _, suggestion := range suggestions {
		printUseUpSuggestion(stdout, suggestion)
	}
	return nil
}

func runFoodRecipe(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food recipe", stderr)
	people := fs.Int("people", 0, "people count; defaults to profile or 2")
	outPath := fs.String("out", "", "optional output JSON path")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	prompt := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if prompt == "" {
		return errors.New("food recipe requires a prompt or mealplan id/path")
	}
	profile, err := food.LoadProfile()
	if err != nil {
		return err
	}
	if plan, err := food.LoadMealPlan(prompt); err == nil {
		recipes := recipesFromPlan(plan)
		res := struct {
			MealPlanID string        `json:"mealplan_id"`
			Recipes    []food.Recipe `json:"recipes"`
		}{MealPlanID: plan.ID, Recipes: recipes}
		if *outPath != "" {
			if err := writeJSONPath(*outPath, res); err != nil {
				return err
			}
		}
		if *jsonOut {
			return output.JSON(stdout, res)
		}
		for _, recipe := range recipes {
			printRecipe(stdout, recipe)
		}
		return nil
	}
	recipe, err := food.GenerateRecipe(prompt, profile, *people)
	if err != nil {
		return err
	}
	if *outPath != "" {
		if err := writeJSONPath(*outPath, recipe); err != nil {
			return err
		}
	}
	if *jsonOut {
		return output.JSON(stdout, recipe)
	}
	printRecipe(stdout, recipe)
	return nil
}

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
	id := fs.String("id", "", "recipe id for --from-text")
	title := fs.String("title", "", "recipe title for --from-text")
	servings := fs.Int("servings", 2, "recipe servings for --from-text")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"from-text": true, "json": true}); err != nil {
		return err
	}
	ref := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if ref == "" {
		return errors.New("food recipes add requires <file|->")
	}
	data, err := readInputBytes(ref)
	if err != nil {
		return err
	}
	var recipes []food.Recipe
	if *fromText {
		recipe, err := food.RecipeFromText(string(data), food.TextRecipeOptions{ID: *id, Title: *title, Servings: *servings})
		if err != nil {
			return fmt.Errorf("%w; %s", err, food.TextRecipeUsage())
		}
		recipes = []food.Recipe{recipe}
	} else {
		var err error
		recipes, err = food.DecodeRecipes(data)
		if err != nil {
			return err
		}
	}
	paths, err := food.SaveUserRecipes(recipes)
	if err != nil {
		return err
	}
	res := struct {
		Action  string        `json:"action"`
		Recipes []food.Recipe `json:"recipes"`
		Paths   []string      `json:"paths"`
	}{Action: "add", Recipes: recipes, Paths: paths}
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

func runFoodShop(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food shop", stderr)
	policy := fs.String("selection-policy", "", "balanced, cheapest, or quality; saved if provided")
	limit := fs.Int("limit", 8, "candidate products per ingredient")
	outPath := fs.String("out", "", "optional output JSON path")
	basketOut := fs.String("basket-out", "", "optional basket file path for guarded cart set-many")
	store := marketFlag(fs, "Alcampo region/store UUID to use for pricing")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("food shop requires a mealplan JSON path or saved mealplan id")
	}
	profile, err := food.LoadProfile()
	if err != nil {
		return err
	}
	resolvedPolicy, updatedProfile, err := resolveFoodSelectionPolicy(&profile, *policy, stderr)
	if err != nil {
		return err
	}
	if updatedProfile {
		if err := food.SaveProfile(profile); err != nil {
			return err
		}
	}
	plan, err := food.LoadMealPlan(fs.Arg(0))
	if err != nil {
		return err
	}
	cfg, client, err := newClient(*store)
	if err != nil {
		return err
	}
	regionID, err := resolveMarket(cfg, *store)
	if err != nil {
		return err
	}
	client.RegionID = regionID
	result, err := food.ShopMealPlan(context.Background(), client, plan, profile, food.ShopOptions{Policy: resolvedPolicy, Limit: *limit})
	if err != nil {
		return err
	}
	if *outPath != "" {
		if err := writeJSONPath(*outPath, result); err != nil {
			return err
		}
	}
	if *basketOut != "" {
		if err := writeBasketLinesPath(*basketOut, result.BasketLines); err != nil {
			return err
		}
	}
	if *jsonOut {
		return output.JSON(stdout, result)
	}
	printShopResult(stdout, result)
	if *basketOut != "" {
		fmt.Fprintf(stdout, "basket\t%s\n", *basketOut)
	}
	return nil
}

func runFoodPDF(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food pdf", stderr)
	outPath := fs.String("out", "", "output PDF path")
	if err := parseInterspersed(fs, args, nil); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("food pdf requires a recipe, mealplan, or shop JSON file")
	}
	input := fs.Arg(0)
	if *outPath == "" {
		ext := filepath.Ext(input)
		if ext == "" {
			*outPath = input + ".pdf"
		} else {
			*outPath = strings.TrimSuffix(input, ext) + ".pdf"
		}
	}
	if err := food.WritePDFFromJSONFile(input, *outPath); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "pdf\t%s\n", *outPath)
	return nil
}

func runFoodRun(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food run", stderr)
	days := fs.Int("days", 3, "number of days")
	people := fs.Int("people", 0, "people count; required unless saved in profile")
	budget := fs.String("budget", "", "budget in EUR")
	meals := fs.String("meals", "dinner", "comma-separated meal types")
	policy := fs.String("selection-policy", "", "balanced, cheapest, or quality; saved if provided")
	limit := fs.Int("limit", 8, "candidate products per ingredient")
	planOut := fs.String("plan-out", "", "optional mealplan JSON path")
	shopOut := fs.String("shop-out", "", "optional shopping JSON path")
	basketOut := fs.String("basket-out", "", "optional basket file path for guarded cart set-many")
	pdfOut := fs.String("pdf-out", "", "optional shopping PDF path")
	store := marketFlag(fs, "Alcampo region/store UUID to use for pricing")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	profile, err := food.LoadProfile()
	if err != nil {
		return err
	}
	resolvedPolicy, updatedProfile, err := resolveFoodSelectionPolicy(&profile, *policy, stderr)
	if err != nil {
		return err
	}
	if updatedProfile {
		if err := food.SaveProfile(profile); err != nil {
			return err
		}
	}
	pantry, err := food.LoadPantry()
	if err != nil {
		return err
	}
	plan, err := food.GenerateMealPlan(profile, pantry, food.PlanOptions{
		Days:      *days,
		People:    *people,
		BudgetEUR: *budget,
		MealTypes: splitList(*meals),
	})
	if err != nil {
		return err
	}
	if *planOut != "" {
		plan.File = *planOut
		if err := writeJSONPath(*planOut, plan); err != nil {
			return err
		}
	} else {
		plan, err = food.SaveMealPlan(plan)
		if err != nil {
			return err
		}
	}
	cfg, client, err := newClient(*store)
	if err != nil {
		return err
	}
	regionID, err := resolveMarket(cfg, *store)
	if err != nil {
		return err
	}
	client.RegionID = regionID
	shop, err := food.ShopMealPlan(context.Background(), client, plan, profile, food.ShopOptions{Policy: resolvedPolicy, Limit: *limit})
	if err != nil {
		return err
	}
	if *shopOut != "" {
		if err := writeJSONPath(*shopOut, shop); err != nil {
			return err
		}
	}
	if *basketOut != "" {
		if err := writeBasketLinesPath(*basketOut, shop.BasketLines); err != nil {
			return err
		}
	}
	pdfPath := ""
	if *pdfOut != "" {
		source := *shopOut
		if source == "" {
			dir, err := food.MealPlansDir()
			if err != nil {
				return err
			}
			source = filepath.Join(dir, plan.ID+"-shop.json")
			if err := writeJSONPath(source, shop); err != nil {
				return err
			}
		}
		if err := food.WritePDFFromJSONFile(source, *pdfOut); err != nil {
			return err
		}
		pdfPath = *pdfOut
	}
	res := struct {
		MealPlan food.MealPlan   `json:"mealplan"`
		Shop     food.ShopResult `json:"shop"`
		PDF      string          `json:"pdf,omitempty"`
		Basket   string          `json:"basket,omitempty"`
	}{MealPlan: plan, Shop: shop, PDF: pdfPath, Basket: *basketOut}
	if *jsonOut {
		return output.JSON(stdout, res)
	}
	printMealPlan(stdout, plan)
	printShopResult(stdout, shop)
	if pdfPath != "" {
		fmt.Fprintf(stdout, "pdf\t%s\n", pdfPath)
	}
	if *basketOut != "" {
		fmt.Fprintf(stdout, "basket\t%s\n", *basketOut)
	}
	return nil
}

func runFoodCook(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food cook", stderr)
	note := fs.String("note", "", "optional cooking note")
	rating := fs.Int("rating", 0, "optional 1-5 rating")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("food cook requires a mealplan JSON path or saved mealplan id")
	}
	if err := food.ValidateRating(*rating); err != nil {
		return err
	}
	plan, err := food.LoadMealPlan(fs.Arg(0))
	if err != nil {
		return err
	}
	pantry, err := food.LoadPantry()
	if err != nil {
		return err
	}
	pantry, result := food.CookMealPlan(plan, pantry, *note, *rating)
	if err := food.SavePantry(pantry); err != nil {
		return err
	}
	if err := food.AppendHistory(food.HistoryEventForCook(result)); err != nil {
		return err
	}
	if *rating >= 4 || (*rating > 0 && *rating <= 2) {
		profile, err := food.LoadProfile()
		if err != nil {
			return err
		}
		likedBefore := len(profile.LikedRecipes)
		rejectedBefore := len(profile.RejectedRecipes)
		for _, recipe := range recipesFromPlan(plan) {
			key := strutil.FirstNonEmpty(recipe.ID, recipe.Title)
			if *rating >= 4 {
				profile.LikedRecipes = appendUnique(profile.LikedRecipes, key)
				profile.RejectedRecipes = removeStringValue(profile.RejectedRecipes, key)
			} else {
				profile.RejectedRecipes = appendUnique(profile.RejectedRecipes, key)
				profile.LikedRecipes = removeStringValue(profile.LikedRecipes, key)
			}
		}
		if len(profile.LikedRecipes) != likedBefore || len(profile.RejectedRecipes) != rejectedBefore {
			if err := food.SaveProfile(profile); err != nil {
				return err
			}
		}
	}
	if *jsonOut {
		return output.JSON(stdout, result)
	}
	fmt.Fprintf(stdout, "cooked\t%s\tapplied=%d\tmissing=%d\n", strutil.FirstNonEmpty(result.MealPlanID, "-"), len(result.Applied), len(result.Missing))
	for _, usage := range result.Applied {
		fmt.Fprintf(stdout, "  used\t%s\t%.3g %s\t%s\n", usage.PantryItem, usage.Quantity, usage.Unit, strutil.FirstNonEmpty(usage.Location, "-"))
	}
	for _, usage := range result.Missing {
		fmt.Fprintf(stdout, "  missing\t%s\t%.3g %s\n", usage.PantryItem, usage.Quantity, usage.Unit)
	}
	return nil
}

func runFoodReceive(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food receive", stderr)
	location := fs.String("location", "", "override storage location for all received items")
	expiry := fs.String("expiry", "", "optional expiry date YYYY-MM-DD to apply to received items")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("food receive requires a shop JSON path or food run JSON path")
	}
	shop, err := food.LoadShopResult(fs.Arg(0))
	if err != nil {
		return err
	}
	pantry, err := food.LoadPantry()
	if err != nil {
		return err
	}
	pantry, result := food.ReceiveShopResult(shop, pantry, food.ReceiveOptions{
		Location:   *location,
		ExpiryDate: *expiry,
	})
	if err := food.SavePantry(pantry); err != nil {
		return err
	}
	if err := food.AppendHistory(food.HistoryEventForReceive(result)); err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, result)
	}
	fmt.Fprintf(stdout, "received\t%s\tapplied=%d\tskipped=%d\n", strutil.FirstNonEmpty(result.MealPlanID, "-"), len(result.Applied), len(result.Skipped))
	for _, item := range result.Applied {
		fmt.Fprintf(stdout, "  pantry\t%s\t%.3g %s\t%s\n", item.PantryItem.Name, item.PantryItem.Quantity, item.PantryItem.Unit, strutil.FirstNonEmpty(item.PantryItem.Location, "-"))
	}
	for _, skipped := range result.Skipped {
		fmt.Fprintf(stdout, "  skipped\t%s\t%s\n", skipped.Ingredient.Name, skipped.Reason)
	}
	return nil
}

func runFoodImportOrders(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food import-orders", stderr)
	limit := fs.Int("limit", 10, "maximum orders to inspect")
	since := fs.String("since", "", "only request orders since YYYY-MM-DD when supported")
	location := fs.String("location", "", "override storage location for imported items")
	expiry := fs.String("expiry", "", "optional expiry date YYYY-MM-DD to apply to imported items")
	inferStaples := fs.Bool("infer-staples", false, "suggest staples from imported order frequency")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"infer-staples": true, "json": true}); err != nil {
		return err
	}
	cfg, client, err := newClient("")
	if err != nil {
		return err
	}
	if cfg.Auth.Cookie == "" && cfg.Auth.BearerToken == "" {
		return errors.New("food import-orders requires an Alcampo session; run login, import-har, or import-curl")
	}
	orders, err := client.PastOrders(context.Background(), alcampo.PastOrderOptions{Limit: *limit, Since: *since})
	if err != nil {
		return err
	}
	pantry, err := food.LoadPantry()
	if err != nil {
		return err
	}
	pantry, result := food.ImportOrdersToPantry(orders, pantry, food.PantryImportOptions{Location: *location, ExpiryDate: *expiry}, *inferStaples)
	if err := food.SavePantry(pantry); err != nil {
		return err
	}
	if err := food.AppendHistory(food.HistoryEventForImport(result)); err != nil {
		return err
	}
	res := struct {
		Orders []alcampo.PastOrder     `json:"orders"`
		Import food.PantryImportResult `json:"import"`
	}{Orders: orders, Import: result}
	if *jsonOut {
		return output.JSON(stdout, res)
	}
	fmt.Fprintf(stdout, "orders-imported\torders=%d\tapplied=%d\tskipped=%d\twarnings=%d\n", len(orders), len(result.Applied), len(result.Skipped), len(result.Warnings))
	for _, staple := range result.SuggestedStaples {
		fmt.Fprintf(stdout, "  suggested-staple\t%s\tmin=%.3g %s\tsearch=%s\n", staple.Name, staple.MinQty, staple.Unit, strutil.FirstNonEmpty(staple.SearchTerm, "-"))
	}
	return nil
}

func runFoodImportReceipt(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food import-receipt", stderr)
	file := fs.String("file", "", "receipt text file or - for stdin")
	location := fs.String("location", "", "override storage location for imported items")
	expiry := fs.String("expiry", "", "optional expiry date YYYY-MM-DD to apply to imported items")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("food import-receipt requires --file <text|->")
	}
	data, err := readInputBytes(*file)
	if err != nil {
		return err
	}
	pantry, err := food.LoadPantry()
	if err != nil {
		return err
	}
	pantry, result := food.ImportReceiptToPantry(string(data), pantry, food.PantryImportOptions{Location: *location, ExpiryDate: *expiry})
	if err := food.SavePantry(pantry); err != nil {
		return err
	}
	if err := food.AppendHistory(food.HistoryEventForImport(result)); err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, result)
	}
	fmt.Fprintf(stdout, "receipt-imported\tapplied=%d\tskipped=%d\twarnings=%d\n", len(result.Applied), len(result.Skipped), len(result.Warnings))
	for _, warning := range result.Warnings {
		fmt.Fprintf(stdout, "  warning\t%s\n", warning)
	}
	return nil
}

func runFoodHistory(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "usage: carrito food history list [--limit N] [--json]")
		return nil
	}
	if args[0] != "list" {
		return fmt.Errorf("unknown food history command %q", args[0])
	}
	fs := newFlagSet("food history list", stderr)
	limit := fs.Int("limit", 20, "maximum history events to return")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args[1:], map[string]bool{"json": true}); err != nil {
		return err
	}
	events, err := food.LoadHistory(*limit)
	if err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, events)
	}
	if len(events) == 0 {
		fmt.Fprintln(stdout, "history is empty")
		return nil
	}
	for _, event := range events {
		fmt.Fprintf(stdout, "%s\t%s\n", strutil.FirstNonEmpty(stringMapValue(event, "type"), "event"), strutil.FirstNonEmpty(stringMapValue(event, "cooked_at"), stringMapValue(event, "created_at"), "-"))
	}
	return nil
}

func runFoodStaples(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "usage: carrito food staples <list|add|remove> [options]")
		return nil
	}
	switch args[0] {
	case "list":
		fs := newFlagSet("food staples list", stderr)
		jsonOut := fs.Bool("json", false, "write JSON to stdout")
		if err := parseInterspersed(fs, args[1:], map[string]bool{"json": true}); err != nil {
			return err
		}
		profile, err := food.LoadProfile()
		if err != nil {
			return err
		}
		if *jsonOut {
			return output.JSON(stdout, profile.Staples)
		}
		printStaples(stdout, profile.Staples)
		return nil
	case "add":
		return runFoodStaplesAdd(args[1:], stdout, stderr)
	case "remove":
		return runFoodStaplesRemove(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown food staples command %q", args[0])
	}
}

func runFoodStaplesAdd(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food staples add", stderr)
	name := fs.String("item", "", "staple item name")
	minQty := fs.Float64("min", 0, "minimum quantity to keep stocked")
	unit := fs.String("unit", "", "unit")
	searchTerm := fs.String("search", "", "Alcampo search term")
	category := fs.String("category", "staple", "category label")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if *name == "" && fs.NArg() > 0 {
		*name = strings.Join(fs.Args(), " ")
	}
	profile, err := food.LoadProfile()
	if err != nil {
		return err
	}
	profile, staple, err := food.AddOrUpdateStaple(profile, food.Staple{
		Name:       *name,
		MinQty:     *minQty,
		Unit:       *unit,
		SearchTerm: *searchTerm,
		Category:   *category,
	})
	if err != nil {
		return err
	}
	if err := food.SaveProfile(profile); err != nil {
		return err
	}
	res := struct {
		Action  string        `json:"action"`
		Staple  food.Staple   `json:"staple"`
		Staples []food.Staple `json:"staples"`
	}{Action: "add", Staple: staple, Staples: profile.Staples}
	if *jsonOut {
		return output.JSON(stdout, res)
	}
	fmt.Fprintf(stdout, "staple\t%s\tmin=%.3g %s\tsearch=%s\n", staple.Name, staple.MinQty, staple.Unit, strutil.FirstNonEmpty(staple.SearchTerm, "-"))
	return nil
}

func runFoodStaplesRemove(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food staples remove", stderr)
	name := fs.String("item", "", "staple item name")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if *name == "" && fs.NArg() > 0 {
		*name = strings.Join(fs.Args(), " ")
	}
	profile, err := food.LoadProfile()
	if err != nil {
		return err
	}
	profile, removed, err := food.RemoveStaple(profile, *name)
	if err != nil {
		return err
	}
	if err := food.SaveProfile(profile); err != nil {
		return err
	}
	res := struct {
		Action  string        `json:"action"`
		Removed food.Staple   `json:"removed"`
		Staples []food.Staple `json:"staples"`
	}{Action: "remove", Removed: removed, Staples: profile.Staples}
	if *jsonOut {
		return output.JSON(stdout, res)
	}
	fmt.Fprintf(stdout, "removed-staple\t%s\n", removed.Name)
	return nil
}

func resolveFoodSelectionPolicy(profile *food.Profile, explicit string, stderr io.Writer) (string, bool, error) {
	if explicit != "" {
		normalized, ok := food.NormalizeSelectionPolicy(explicit)
		if !ok || normalized == "" {
			return "", false, fmt.Errorf("unknown selection policy %q", explicit)
		}
		profile.SelectionPolicy = normalized
		return normalized, true, nil
	}
	if profile.SelectionPolicy != "" {
		normalized, ok := food.NormalizeSelectionPolicy(profile.SelectionPolicy)
		if ok && normalized != "" {
			return normalized, false, nil
		}
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(stderr, "Selection policy (balanced, cheapest, quality): ")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", false, err
		}
		normalized, ok := food.NormalizeSelectionPolicy(line)
		if !ok || normalized == "" {
			return "", false, fmt.Errorf("unknown selection policy %q", strings.TrimSpace(line))
		}
		profile.SelectionPolicy = normalized
		return normalized, true, nil
	}
	return "", false, errors.New("selection policy is not set; run 'carrito food profile set --selection-policy balanced|cheapest|quality' or pass --selection-policy")
}

func recipesFromPlan(plan food.MealPlan) []food.Recipe {
	var recipes []food.Recipe
	seen := map[string]bool{}
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			key := strutil.FirstNonEmpty(meal.Recipe.ID, meal.Recipe.Title)
			if seen[key] {
				continue
			}
			seen[key] = true
			recipes = append(recipes, meal.Recipe)
		}
	}
	return recipes
}
