package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/wachtermar/carrito/internal/food"
	"github.com/wachtermar/carrito/internal/output"
)

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
