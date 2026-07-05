package food

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type PlanOptions struct {
	Days      int
	People    int
	BudgetEUR string
	MealTypes []string
}

func GenerateMealPlan(profile Profile, pantry Pantry, opts PlanOptions) (MealPlan, error) {
	if opts.Days <= 0 {
		opts.Days = 3
	}
	if opts.People <= 0 {
		opts.People = profile.People
	}
	if opts.People <= 0 {
		opts.People = 2
	}
	if len(opts.MealTypes) == 0 {
		opts.MealTypes = []string{"dinner"}
	}
	budget := firstNonEmpty(opts.BudgetEUR, profile.BudgetEUR)

	templates := rankRecipeTemplates(filterTemplates(recipeTemplates(), profile), profile, pantry)
	if len(templates) == 0 {
		return MealPlan{}, fmt.Errorf("no recipe templates fit the current allergy/dislike profile")
	}
	plan := MealPlan{
		ID:              slugID("mealplan"),
		CreatedAt:       nowStamp(),
		People:          opts.People,
		BudgetEUR:       budget,
		SelectionPolicy: profile.SelectionPolicy,
	}

	var required []Ingredient
	recipeIndex := 0
	for day := 1; day <= opts.Days; day++ {
		dp := DayPlan{Day: day, Label: fmt.Sprintf("Day %d", day)}
		for _, mealType := range opts.MealTypes {
			template := templates[recipeIndex%len(templates)]
			recipeIndex++
			recipe := scaleRecipe(template, opts.People)
			dp.Meals = append(dp.Meals, Meal{
				Type:           strings.TrimSpace(mealType),
				Recipe:         recipe,
				PlanningReason: planningReason(template, profile, pantry),
			})
			required = append(required, recipe.Ingredients...)
		}
		plan.Days = append(plan.Days, dp)
	}
	plan.RequiredPurchases, plan.PantryUsage = ApplyPantry(required, pantry)
	if staples := StapleRestockIngredients(profile, pantry); len(staples) > 0 {
		plan.RequiredPurchases = mergeIngredients(append(plan.RequiredPurchases, staples...))
		plan.Notes = append(plan.Notes, "Low-stock household staples were added to the shopping list.")
	}
	if len(plan.PantryUsage) > 0 {
		plan.Notes = append(plan.Notes, "Pantry and fridge items were deducted before building the shopping list.")
	}
	if profile.SelectionPolicy == "" {
		plan.Notes = append(plan.Notes, "No product selection policy is saved yet; set one before shopping this plan.")
	}
	return plan, nil
}

func rankRecipeTemplates(templates []Recipe, profile Profile, pantry Pantry) []Recipe {
	ranked := make([]Recipe, len(templates))
	copy(ranked, templates)
	sort.SliceStable(ranked, func(i, j int) bool {
		left := recipePlanningScore(ranked[i], profile, pantry)
		right := recipePlanningScore(ranked[j], profile, pantry)
		if left == right {
			return ranked[i].Title < ranked[j].Title
		}
		return left > right
	})
	return ranked
}

func recipePlanningScore(recipe Recipe, profile Profile, pantry Pantry) float64 {
	score := 0.0
	text := recipeText(recipe)
	for _, liked := range profile.LikedRecipes {
		if key := normalizeKey(liked); key != "" && (recipeIDMatches(recipe, key) || strings.Contains(text, key)) {
			score += 20
		}
	}
	for _, cuisine := range profile.LikedCuisines {
		if key := normalizeKey(cuisine); key != "" && strings.Contains(text, key) {
			score += 12
		}
	}
	for _, item := range pantry.Items {
		match := recipeUsesPantryItem(recipe, item)
		if !match {
			continue
		}
		score += 8
		days, ok := daysUntilExpiry(item.ExpiryDate)
		switch {
		case ok && days < 0:
			score -= 4
		case ok && days <= 1:
			score += 24
		case ok && days <= 3:
			score += 18
		case ok && days <= 7:
			score += 10
		}
		if strings.EqualFold(item.Location, "fridge") {
			score += 4
		}
	}
	return score
}

func planningReason(recipe Recipe, profile Profile, pantry Pantry) string {
	var reasons []string
	text := recipeText(recipe)
	for _, liked := range profile.LikedRecipes {
		if key := normalizeKey(liked); key != "" && (recipeIDMatches(recipe, key) || strings.Contains(text, key)) {
			reasons = append(reasons, "matches liked recipe "+liked)
			break
		}
	}
	for _, cuisine := range profile.LikedCuisines {
		if key := normalizeKey(cuisine); key != "" && strings.Contains(text, key) {
			reasons = append(reasons, "matches liked cuisine "+cuisine)
			break
		}
	}
	for _, item := range pantry.Items {
		if !recipeUsesPantryItem(recipe, item) {
			continue
		}
		if days, ok := daysUntilExpiry(item.ExpiryDate); ok && days <= 7 {
			reasons = append(reasons, fmt.Sprintf("uses %s expiring in %d day(s)", item.Name, days))
			continue
		}
		reasons = append(reasons, "uses pantry/fridge item "+item.Name)
	}
	if len(reasons) == 0 {
		return "fits saved profile constraints"
	}
	if len(reasons) > 3 {
		reasons = reasons[:3]
	}
	return strings.Join(reasons, "; ")
}

func recipeUsesPantryItem(recipe Recipe, item PantryItem) bool {
	for _, ingredient := range recipe.Ingredients {
		if ingredientMatchesPantry(ingredient, item) {
			return true
		}
	}
	return false
}

func daysUntilExpiry(expiry string) (int, bool) {
	expiry = strings.TrimSpace(expiry)
	if expiry == "" {
		return 0, false
	}
	day, err := time.Parse("2006-01-02", expiry)
	if err != nil {
		return 0, false
	}
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return int(day.Sub(today).Hours() / 24), true
}

func GenerateRecipe(prompt string, profile Profile, people int) Recipe {
	if people <= 0 {
		people = profile.People
	}
	if people <= 0 {
		people = 2
	}
	promptKey := normalizeKey(prompt)
	templates := filterTemplates(recipeTemplates(), profile)
	if len(templates) == 0 {
		templates = recipeTemplates()
	}
	for _, template := range templates {
		text := normalizeKey(template.Title + " " + strings.Join(template.Tags, " "))
		if promptKey != "" && (strings.Contains(text, promptKey) || strings.Contains(promptKey, normalizeKey(template.Title))) {
			return scaleRecipe(template, people)
		}
	}
	if strings.Contains(promptKey, "pasta") {
		return scaleRecipe(templateByID("vegetable-pasta"), people)
	}
	if strings.Contains(promptKey, "lent") || strings.Contains(promptKey, "guiso") {
		return scaleRecipe(templateByID("lentil-stew"), people)
	}
	if strings.Contains(promptKey, "tortilla") || strings.Contains(promptKey, "egg") || strings.Contains(promptKey, "huevo") {
		return scaleRecipe(templateByID("spanish-tortilla"), people)
	}
	if strings.Contains(promptKey, "fish") || strings.Contains(promptKey, "salmon") || strings.Contains(promptKey, "pescado") {
		return scaleRecipe(templateByID("salmon-potatoes"), people)
	}
	return scaleRecipe(templates[0], people)
}

func filterTemplates(templates []Recipe, profile Profile) []Recipe {
	var out []Recipe
	for _, recipe := range templates {
		text := recipeText(recipe)
		if recipeMemoryMatches(recipe, profile.RejectedRecipes) {
			continue
		}
		if containsAny(text, profile.Allergies) || containsAny(text, profile.Dislikes) {
			continue
		}
		if len(profile.Diets) > 0 && !fitsDiets(recipe, profile.Diets) {
			continue
		}
		out = append(out, recipe)
	}
	return out
}

func recipeMemoryMatches(recipe Recipe, memories []string) bool {
	text := recipeText(recipe)
	for _, memory := range memories {
		key := normalizeKey(memory)
		if key == "" {
			continue
		}
		if recipeIDMatches(recipe, key) || strings.Contains(text, key) {
			return true
		}
	}
	return false
}

func recipeIDMatches(recipe Recipe, key string) bool {
	return key != "" && (normalizeKey(recipe.ID) == key || normalizeKey(recipe.Title) == key)
}

func fitsDiets(recipe Recipe, diets []string) bool {
	text := recipeText(recipe)
	for _, diet := range diets {
		switch normalizeKey(diet) {
		case "vegetarian", "vegetariano", "vegetariana":
			if strings.Contains(text, "chicken") || strings.Contains(text, "salmon") || strings.Contains(text, "turkey") || strings.Contains(text, "pollo") {
				return false
			}
		case "vegan", "vegano", "vegana":
			if strings.Contains(text, "chicken") || strings.Contains(text, "salmon") || strings.Contains(text, "turkey") || strings.Contains(text, "egg") || strings.Contains(text, "cheese") || strings.Contains(text, "yogurt") || strings.Contains(text, "pollo") {
				return false
			}
		}
	}
	return true
}

func containsAny(text string, values []string) bool {
	for _, value := range values {
		key := normalizeKey(value)
		if key != "" && strings.Contains(text, key) {
			return true
		}
	}
	return false
}

func recipeText(recipe Recipe) string {
	var parts []string
	parts = append(parts, recipe.ID, recipe.Title)
	parts = append(parts, recipe.Tags...)
	for _, ing := range recipe.Ingredients {
		parts = append(parts, ing.Name, ing.SearchTerm, ing.Category)
	}
	return normalizeKey(strings.Join(parts, " "))
}

func scaleRecipe(recipe Recipe, servings int) Recipe {
	if recipe.Servings <= 0 {
		recipe.Servings = 2
	}
	factor := float64(servings) / float64(recipe.Servings)
	recipe.Servings = servings
	for i := range recipe.Ingredients {
		recipe.Ingredients[i].Quantity = roundQty(recipe.Ingredients[i].Quantity * factor)
	}
	return recipe
}

func templateByID(id string) Recipe {
	for _, recipe := range recipeTemplates() {
		if recipe.ID == id {
			return recipe
		}
	}
	return recipeTemplates()[0]
}

func recipeTemplates() []Recipe {
	return []Recipe{
		{
			ID:          "mediterranean-chicken-rice",
			Title:       "Mediterranean Chicken Rice",
			Servings:    2,
			PrepMinutes: 15,
			CookMinutes: 30,
			Tags:        []string{"balanced", "mediterranean", "high protein"},
			Ingredients: []Ingredient{
				{Name: "chicken breast", Quantity: 350, Unit: "g", Category: "meat", SearchTerm: "pechuga de pollo"},
				{Name: "rice", Quantity: 180, Unit: "g", Category: "pantry", SearchTerm: "arroz"},
				{Name: "red pepper", Quantity: 1, Unit: "unit", Category: "vegetables", SearchTerm: "pimiento rojo"},
				{Name: "tomato", Quantity: 2, Unit: "unit", Category: "vegetables", SearchTerm: "tomate"},
				{Name: "onion", Quantity: 1, Unit: "unit", Category: "vegetables", SearchTerm: "cebolla"},
			},
			Equipment: []string{"large pan", "knife", "cutting board"},
			Steps: []RecipeStep{
				{Number: 1, Title: "Prep", Text: "Dice the onion, pepper, and tomatoes. Cut the chicken into bite-size pieces.", Minutes: 10},
				{Number: 2, Title: "Brown", Text: "Brown the chicken with a little oil, then add onion and pepper until softened.", Minutes: 10},
				{Number: 3, Title: "Simmer", Text: "Add rice, tomato, salt, and water. Simmer covered until the rice is tender.", Minutes: 20},
			},
			Substitutions: []string{"Use turkey breast instead of chicken.", "Use frozen vegetable mix if fresh peppers are unavailable."},
		},
		{
			ID:          "vegetable-pasta",
			Title:       "Vegetable Pasta With Tomato Sauce",
			Servings:    2,
			PrepMinutes: 10,
			CookMinutes: 20,
			Tags:        []string{"vegetarian", "quick", "family"},
			Ingredients: []Ingredient{
				{Name: "pasta", Quantity: 200, Unit: "g", Category: "pantry", SearchTerm: "pasta"},
				{Name: "tomato sauce", Quantity: 250, Unit: "g", Category: "pantry", SearchTerm: "tomate frito"},
				{Name: "zucchini", Quantity: 1, Unit: "unit", Category: "vegetables", SearchTerm: "calabacin"},
				{Name: "mushrooms", Quantity: 200, Unit: "g", Category: "vegetables", SearchTerm: "champiñones"},
				{Name: "grated cheese", Quantity: 80, Unit: "g", Category: "dairy", SearchTerm: "queso rallado", Optional: true},
			},
			Equipment: []string{"pot", "pan", "colander"},
			Steps: []RecipeStep{
				{Number: 1, Title: "Cook pasta", Text: "Cook pasta in salted boiling water until al dente.", Minutes: 10},
				{Number: 2, Title: "Cook vegetables", Text: "Saute sliced zucchini and mushrooms until browned.", Minutes: 8},
				{Number: 3, Title: "Finish", Text: "Combine pasta, vegetables, and tomato sauce. Top with cheese if using.", Minutes: 4},
			},
			Substitutions: []string{"Skip cheese for a lighter or vegan version.", "Use whole wheat pasta for more fiber."},
		},
		{
			ID:          "lentil-stew",
			Title:       "Lentil And Vegetable Stew",
			Servings:    2,
			PrepMinutes: 10,
			CookMinutes: 35,
			Tags:        []string{"vegetarian", "budget", "batch cooking"},
			Ingredients: []Ingredient{
				{Name: "lentils", Quantity: 200, Unit: "g", Category: "pantry", SearchTerm: "lentejas"},
				{Name: "carrot", Quantity: 2, Unit: "unit", Category: "vegetables", SearchTerm: "zanahoria"},
				{Name: "potato", Quantity: 2, Unit: "unit", Category: "vegetables", SearchTerm: "patata"},
				{Name: "onion", Quantity: 1, Unit: "unit", Category: "vegetables", SearchTerm: "cebolla"},
				{Name: "vegetable stock", Quantity: 1, Unit: "unit", Category: "pantry", SearchTerm: "caldo verduras"},
			},
			Equipment: []string{"soup pot", "knife", "ladle"},
			Steps: []RecipeStep{
				{Number: 1, Title: "Prep vegetables", Text: "Dice the onion, carrots, and potatoes.", Minutes: 10},
				{Number: 2, Title: "Build base", Text: "Saute onion, then add vegetables, lentils, stock, and water.", Minutes: 10},
				{Number: 3, Title: "Simmer", Text: "Simmer until lentils and potatoes are tender. Adjust seasoning.", Minutes: 30},
			},
			Substitutions: []string{"Use chickpeas when lentils are unavailable.", "Add spinach at the end for extra greens."},
		},
		{
			ID:          "spanish-tortilla",
			Title:       "Spanish Tortilla With Salad",
			Servings:    2,
			PrepMinutes: 15,
			CookMinutes: 25,
			Tags:        []string{"vegetarian", "spanish", "classic"},
			Ingredients: []Ingredient{
				{Name: "eggs", Quantity: 4, Unit: "unit", Category: "eggs", SearchTerm: "huevos"},
				{Name: "potato", Quantity: 3, Unit: "unit", Category: "vegetables", SearchTerm: "patata"},
				{Name: "onion", Quantity: 1, Unit: "unit", Category: "vegetables", SearchTerm: "cebolla"},
				{Name: "mixed salad", Quantity: 1, Unit: "unit", Category: "vegetables", SearchTerm: "ensalada"},
			},
			Equipment: []string{"nonstick pan", "bowl", "plate"},
			Steps: []RecipeStep{
				{Number: 1, Title: "Cook potatoes", Text: "Slice potatoes and onion thinly. Cook gently in oil until tender.", Minutes: 18},
				{Number: 2, Title: "Mix", Text: "Beat eggs, fold in potatoes and onion, and season.", Minutes: 5},
				{Number: 3, Title: "Set", Text: "Cook in a nonstick pan until just set, flipping carefully with a plate.", Minutes: 10},
			},
			Substitutions: []string{"Serve with tomatoes if bagged salad is unavailable."},
		},
		{
			ID:          "salmon-potatoes",
			Title:       "Salmon With Potatoes And Green Beans",
			Servings:    2,
			PrepMinutes: 10,
			CookMinutes: 25,
			Tags:        []string{"quality", "fish", "high protein"},
			Ingredients: []Ingredient{
				{Name: "salmon fillets", Quantity: 2, Unit: "unit", Category: "fish", SearchTerm: "salmon"},
				{Name: "potato", Quantity: 3, Unit: "unit", Category: "vegetables", SearchTerm: "patata"},
				{Name: "green beans", Quantity: 250, Unit: "g", Category: "vegetables", SearchTerm: "judias verdes"},
				{Name: "lemon", Quantity: 1, Unit: "unit", Category: "fruit", SearchTerm: "limon"},
			},
			Equipment: []string{"oven tray", "pot", "knife"},
			Steps: []RecipeStep{
				{Number: 1, Title: "Roast potatoes", Text: "Cut potatoes and roast with oil and salt until nearly tender.", Minutes: 20},
				{Number: 2, Title: "Add salmon", Text: "Place salmon on the tray with lemon and roast until cooked.", Minutes: 10},
				{Number: 3, Title: "Serve", Text: "Boil or steam green beans and serve with the salmon and potatoes.", Minutes: 8},
			},
			Substitutions: []string{"Use hake or cod if salmon is unavailable."},
		},
	}
}
