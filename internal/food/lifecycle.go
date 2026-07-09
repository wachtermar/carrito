package food

import (
	"fmt"
	"strings"

	"github.com/wachtermar/carrito/internal/strutil"
)

func CookMealPlan(plan MealPlan, pantry Pantry, note string, rating int) (Pantry, CookResult) {
	result := CookResult{
		MealPlanID: plan.ID,
		CookedAt:   nowStamp(),
		Recipes:    recipeFeedbackFromPlan(plan),
		Note:       strings.TrimSpace(note),
		Rating:     rating,
	}
	for _, usage := range cookUsageCandidates(plan) {
		var applied PantryUsage
		var ok bool
		pantry, applied, ok = applyPantryUsage(pantry, usage)
		if ok {
			result.Applied = append(result.Applied, applied)
		} else {
			result.Missing = append(result.Missing, usage)
		}
	}
	result.Pantry = pantry
	return pantry, result
}

func cookUsageCandidates(plan MealPlan) []PantryUsage {
	var ingredients []Ingredient
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			for _, ing := range meal.Recipe.Ingredients {
				if ing.Optional || ing.Quantity <= 0 {
					continue
				}
				ingredients = append(ingredients, ing)
			}
		}
	}
	if len(ingredients) == 0 {
		return append([]PantryUsage(nil), plan.PantryUsage...)
	}
	ingredients = mergeIngredients(ingredients)
	usages := make([]PantryUsage, 0, len(ingredients))
	for _, ing := range ingredients {
		usages = append(usages, PantryUsage{
			Ingredient: ing.Name,
			PantryItem: strutil.FirstNonEmpty(ing.SearchTerm, ing.Name),
			Quantity:   ing.Quantity,
			Unit:       ing.Unit,
		})
	}
	return usages
}

func applyPantryUsage(p Pantry, usage PantryUsage) (Pantry, PantryUsage, bool) {
	unit := normalizeUnit(usage.Unit)
	names := []string{usage.PantryItem, usage.Ingredient}
	if strings.TrimSpace(strings.Join(names, "")) == "" || usage.Quantity <= 0 {
		return p, PantryUsage{}, false
	}
	for i := range p.Items {
		item := p.Items[i]
		if !matchesAnyPantryName(item.Name, names) {
			continue
		}
		if unit != "" && item.Unit != "" && normalizeUnit(item.Unit) != unit {
			continue
		}
		used := usage.Quantity
		if used > item.Quantity {
			used = item.Quantity
		}
		if used <= 0 {
			continue
		}
		p.Items[i].Quantity = roundQty(item.Quantity - used)
		p.Items[i].LastChecked = nowStamp()
		applied := PantryUsage{
			Ingredient: usage.Ingredient,
			PantryItem: item.Name,
			Quantity:   used,
			Unit:       strutil.FirstNonEmpty(usage.Unit, item.Unit),
			Location:   item.Location,
		}
		if p.Items[i].Quantity == 0 {
			p.Items = append(p.Items[:i], p.Items[i+1:]...)
		}
		return p, applied, true
	}
	return p, PantryUsage{}, false
}

func matchesAnyPantryName(item string, names []string) bool {
	for _, name := range names {
		if strings.TrimSpace(name) != "" && namesMatch(item, name) {
			return true
		}
	}
	return false
}

func recipeFeedbackFromPlan(plan MealPlan) []RecipeFeedback {
	var out []RecipeFeedback
	seen := make(map[string]bool)
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			recipe := meal.Recipe
			key := normalizeKey(strutil.FirstNonEmpty(recipe.ID, recipe.Title))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, RecipeFeedback{ID: recipe.ID, Title: recipe.Title})
		}
	}
	return out
}

func HistoryEventForCook(result CookResult) map[string]any {
	return map[string]any{
		"type":                 "mealplan_cooked",
		"mealplan_id":          result.MealPlanID,
		"cooked_at":            result.CookedAt,
		"recipes":              result.Recipes,
		"applied_pantry_usage": result.Applied,
		"missing_pantry_usage": result.Missing,
		"note":                 result.Note,
		"rating":               result.Rating,
	}
}

func ValidateRating(rating int) error {
	if rating < 0 || rating > 5 {
		return fmt.Errorf("rating must be between 0 and 5")
	}
	return nil
}
