package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/wachtermar/carrito/internal/food"
	"github.com/wachtermar/carrito/internal/output"
	"github.com/wachtermar/carrito/internal/strutil"
)

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
	profile, err = food.LoadProfile()
	if err != nil {
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
	pantry, err = food.LoadPantry()
	if err != nil {
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
	pantry, err = food.LoadPantry()
	if err != nil {
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
