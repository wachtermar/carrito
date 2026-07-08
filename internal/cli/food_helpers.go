package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/wachtermar/carrito/internal/food"
	"github.com/wachtermar/carrito/internal/strutil"

	"golang.org/x/term"
)

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
