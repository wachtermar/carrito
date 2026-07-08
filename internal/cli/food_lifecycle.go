package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/food"
	"github.com/wachtermar/carrito/internal/output"
	"github.com/wachtermar/carrito/internal/strutil"
)

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
