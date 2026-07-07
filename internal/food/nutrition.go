package food

import (
	"embed"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
)

//go:embed nutrition/foods.json
var nutritionFS embed.FS

type nutritionFood struct {
	Terms    []string `json:"terms"`
	Per      string   `json:"per"`
	Kcal     float64  `json:"kcal,omitempty"`
	ProteinG float64  `json:"protein_g,omitempty"`
	CarbsG   float64  `json:"carbs_g,omitempty"`
	FatG     float64  `json:"fat_g,omitempty"`
}

var nutritionCache struct {
	once  sync.Once
	foods map[string]nutritionFood
	err   error
}

func SummarizeNutrition(recipe Recipe) (NutritionSummary, error) {
	if recipe.NutritionPerServing != nil && !nutritionIsZero(*recipe.NutritionPerServing) {
		return roundNutrition(*recipe.NutritionPerServing), nil
	}
	foods, err := nutritionFoods()
	if err != nil {
		return NutritionSummary{}, err
	}
	var total NutritionSummary
	var missing []string
	for _, ingredient := range recipe.Ingredients {
		food, ok := lookupNutritionFood(foods, ingredient)
		if !ok {
			missing = append(missing, ingredient.Name)
			continue
		}
		scale, ok := nutritionScale(ingredient, food.Per)
		if !ok {
			missing = append(missing, ingredient.Name)
			continue
		}
		total.add(food.summary(), scale)
	}
	if nutritionIsZero(total) {
		if len(missing) > 0 {
			return NutritionSummary{}, fmt.Errorf("nutrition estimate unavailable for %s", strings.Join(missing, ", "))
		}
		return NutritionSummary{}, fmt.Errorf("nutrition estimate unavailable for %q", recipe.Title)
	}
	servings := recipe.Servings
	if servings <= 0 {
		servings = 1
	}
	total.scale(1 / float64(servings))
	return roundNutrition(total), nil
}

func SummarizePlan(plan MealPlan) NutritionSummary {
	var total NutritionSummary
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			summary, err := SummarizeNutrition(meal.Recipe)
			if err != nil {
				continue
			}
			servings := meal.Recipe.Servings
			if servings <= 0 {
				servings = plan.People
			}
			if servings <= 0 {
				servings = 1
			}
			total.add(summary, float64(servings))
		}
	}
	return roundNutrition(total)
}

func NutritionGoalWarnings(summary NutritionSummary, goals []string) []string {
	if nutritionIsZero(summary) {
		return nil
	}
	var warnings []string
	for _, goal := range goals {
		metric, op, limit, ok := parseNutritionGoal(goal)
		if !ok {
			continue
		}
		actual := nutritionMetric(summary, metric)
		violated := false
		switch op {
		case "<", "<=":
			violated = actual > limit
		case ">", ">=":
			violated = actual < limit
		case "=":
			violated = actual > limit
		}
		if violated {
			warnings = append(warnings, fmt.Sprintf("Nutrition goal %s not met: %s %.0f vs goal %s%.0f.", goal, metric, actual, op, limit))
		}
	}
	return warnings
}

func nutritionFoods() (map[string]nutritionFood, error) {
	nutritionCache.once.Do(func() {
		data, err := nutritionFS.ReadFile("nutrition/foods.json")
		if err != nil {
			nutritionCache.err = err
			return
		}
		var foods []nutritionFood
		if err := json.Unmarshal(data, &foods); err != nil {
			nutritionCache.err = err
			return
		}
		nutritionCache.foods = make(map[string]nutritionFood)
		for _, food := range foods {
			for _, term := range food.Terms {
				key := normalizeKey(term)
				if key != "" {
					nutritionCache.foods[key] = food
				}
			}
		}
	})
	return nutritionCache.foods, nutritionCache.err
}

func lookupNutritionFood(foods map[string]nutritionFood, ingredient Ingredient) (nutritionFood, bool) {
	keys := []string{normalizeKey(ingredient.Name), normalizeKey(ingredient.SearchTerm)}
	for _, key := range keys {
		if key == "" {
			continue
		}
		if food, ok := foods[key]; ok {
			return food, true
		}
		for candidate, food := range foods {
			if strings.Contains(key, candidate) || strings.Contains(candidate, key) {
				return food, true
			}
		}
	}
	return nutritionFood{}, false
}

func nutritionScale(ingredient Ingredient, per string) (float64, bool) {
	qty := ingredient.Quantity
	if qty <= 0 {
		qty = 1
	}
	unit := normalizeUnit(ingredient.Unit)
	switch strings.ToLower(strings.TrimSpace(per)) {
	case "unit":
		if unit == "" || unit == "unit" {
			return qty, true
		}
	case "100g":
		switch unit {
		case "g":
			return qty / 100, true
		case "kg":
			return qty * 10, true
		case "":
			return qty, true
		}
	case "100ml":
		switch unit {
		case "ml":
			return qty / 100, true
		case "cl":
			return qty / 10, true
		case "l":
			return qty * 10, true
		}
	}
	return 0, false
}

func parseNutritionGoal(goal string) (metric, op string, value float64, ok bool) {
	compact := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(goal), " ", ""))
	aliases := []string{"protein_g", "protein", "carbs_g", "carb_g", "carbs", "carb", "fat_g", "fat", "kcal", "calories", "cal"}
	for _, alias := range aliases {
		if !strings.HasPrefix(compact, alias) {
			continue
		}
		rest := strings.TrimPrefix(compact, alias)
		for _, candidate := range []string{"<=", ">=", "<", ">", "=", ":"} {
			if strings.HasPrefix(rest, candidate) {
				raw := strings.TrimPrefix(rest, candidate)
				parsed, err := strconv.ParseFloat(raw, 64)
				if err != nil {
					return "", "", 0, false
				}
				if candidate == ":" {
					candidate = "<="
				}
				return canonicalNutritionMetric(alias), candidate, parsed, true
			}
		}
	}
	return "", "", 0, false
}

func canonicalNutritionMetric(metric string) string {
	switch metric {
	case "protein", "protein_g":
		return "protein"
	case "carb", "carb_g", "carbs", "carbs_g":
		return "carbs"
	case "fat", "fat_g":
		return "fat"
	default:
		return "kcal"
	}
}

func nutritionMetric(summary NutritionSummary, metric string) float64 {
	switch metric {
	case "protein":
		return summary.ProteinG
	case "carbs":
		return summary.CarbsG
	case "fat":
		return summary.FatG
	default:
		return summary.Kcal
	}
}

func (food nutritionFood) summary() NutritionSummary {
	return NutritionSummary{Kcal: food.Kcal, ProteinG: food.ProteinG, CarbsG: food.CarbsG, FatG: food.FatG}
}

func (summary *NutritionSummary) add(other NutritionSummary, factor float64) {
	summary.Kcal += other.Kcal * factor
	summary.ProteinG += other.ProteinG * factor
	summary.CarbsG += other.CarbsG * factor
	summary.FatG += other.FatG * factor
}

func (summary *NutritionSummary) scale(factor float64) {
	summary.Kcal *= factor
	summary.ProteinG *= factor
	summary.CarbsG *= factor
	summary.FatG *= factor
}

func nutritionIsZero(summary NutritionSummary) bool {
	return summary.Kcal == 0 && summary.ProteinG == 0 && summary.CarbsG == 0 && summary.FatG == 0
}

func roundNutrition(summary NutritionSummary) NutritionSummary {
	summary.Kcal = math.Round(summary.Kcal)
	summary.ProteinG = math.Round(summary.ProteinG*10) / 10
	summary.CarbsG = math.Round(summary.CarbsG*10) / 10
	summary.FatG = math.Round(summary.FatG*10) / 10
	return summary
}
