package food

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/wachtermar/carrito/internal/strutil"
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
		return MealPlan{}, fmt.Errorf("people count is required; pass --people or save one with 'carrito food profile set --people <n>'")
	}
	if len(opts.MealTypes) == 0 {
		opts.MealTypes = []string{"dinner"}
	}
	budget := strutil.FirstNonEmpty(opts.BudgetEUR, profile.BudgetEUR)

	recipes, err := SearchRecipes(RecipeQuery{
		Diets:     profile.Diets,
		Allergies: profile.Allergies,
		Dislikes:  profile.Dislikes,
	})
	if err != nil {
		return MealPlan{}, err
	}
	templates := rankRecipeTemplates(filterTemplates(recipes, profile), profile, pantry)
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
	recipeIndexes := make(map[string]int)
	for day := 1; day <= opts.Days; day++ {
		dp := DayPlan{Day: day, Label: fmt.Sprintf("Day %d", day)}
		for _, mealType := range opts.MealTypes {
			trimmedMealType := strings.TrimSpace(mealType)
			candidates := templatesForMealType(templates, trimmedMealType)
			mealKey := normalizeMealType(trimmedMealType)
			template := candidates[recipeIndexes[mealKey]%len(candidates)]
			recipeIndexes[mealKey]++
			recipe := withNutrition(scaleRecipe(template, opts.People))
			dp.Meals = append(dp.Meals, Meal{
				Type:           trimmedMealType,
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
	plan.Notes = append(plan.Notes, expiringPlanNotes(pantry, plan.PantryUsage, 3)...)
	if profile.SelectionPolicy == "" {
		plan.Notes = append(plan.Notes, "No product selection policy is saved yet; set one before shopping this plan.")
	}
	if summary := SummarizePlan(plan); !nutritionIsZero(summary) {
		plan.Nutrition = &summary
		plan.Notes = append(plan.Notes, NutritionGoalWarnings(summary, profile.NutritionGoals)...)
	}
	return plan, nil
}

func templatesForMealType(templates []Recipe, mealType string) []Recipe {
	mealKey := normalizeMealType(mealType)
	if mealKey == "" {
		return templates
	}
	var out []Recipe
	for _, recipe := range templates {
		for _, tag := range recipe.Tags {
			if normalizeMealType(tag) == mealKey {
				out = append(out, recipe)
				break
			}
		}
	}
	if len(out) == 0 {
		return templates
	}
	return out
}

func normalizeMealType(value string) string {
	switch normalizeKey(value) {
	case "breakfast", "desayuno":
		return "breakfast"
	case "lunch", "comida", "almuerzo":
		return "lunch"
	case "dinner", "cena":
		return "dinner"
	default:
		return normalizeKey(value)
	}
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

func GenerateRecipe(prompt string, profile Profile, people int) (Recipe, error) {
	if people <= 0 {
		people = profile.People
	}
	if people <= 0 {
		people = 2
	}
	recipes, err := SearchRecipes(RecipeQuery{
		Diets:     profile.Diets,
		Allergies: profile.Allergies,
		Dislikes:  profile.Dislikes,
	})
	if err != nil {
		return Recipe{}, err
	}
	promptKey := normalizeKey(prompt)
	templates := filterTemplates(recipes, profile)
	if len(templates) == 0 {
		templates = recipes
	}
	if len(templates) == 0 {
		return Recipe{}, fmt.Errorf("no recipe templates fit the current allergy/dislike profile")
	}
	if promptKey != "" {
		matches, err := SearchRecipes(RecipeQuery{
			Query:     prompt,
			Diets:     profile.Diets,
			Allergies: profile.Allergies,
			Dislikes:  profile.Dislikes,
			Limit:     20,
		})
		if err != nil {
			return Recipe{}, err
		}
		matches = filterTemplates(matches, profile)
		if len(matches) > 0 {
			return withNutrition(scaleRecipe(matches[0], people)), nil
		}
	}
	for _, template := range templates {
		text := normalizeKey(template.Title + " " + strings.Join(template.Tags, " "))
		if promptKey != "" && (strings.Contains(text, promptKey) || strings.Contains(promptKey, normalizeKey(template.Title))) {
			return withNutrition(scaleRecipe(template, people)), nil
		}
	}
	if strings.Contains(promptKey, "pasta") {
		return withNutrition(scaleRecipe(templateByID(recipes, "vegetable-pasta"), people)), nil
	}
	if strings.Contains(promptKey, "lent") || strings.Contains(promptKey, "guiso") {
		return withNutrition(scaleRecipe(templateByID(recipes, "lentil-stew"), people)), nil
	}
	if strings.Contains(promptKey, "tortilla") || strings.Contains(promptKey, "egg") || strings.Contains(promptKey, "huevo") {
		return withNutrition(scaleRecipe(templateByID(recipes, "spanish-tortilla"), people)), nil
	}
	if strings.Contains(promptKey, "fish") || strings.Contains(promptKey, "salmon") || strings.Contains(promptKey, "pescado") {
		return withNutrition(scaleRecipe(templateByID(recipes, "salmon-potatoes"), people)), nil
	}
	return withNutrition(scaleRecipe(templates[0], people)), nil
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
			if containsAnyKey(text, vegetarianForbiddenKeys) {
				return false
			}
		case "vegan", "vegano", "vegana":
			if containsAnyKey(veganDietText(text), append(vegetarianForbiddenKeys, veganForbiddenKeys...)) {
				return false
			}
		}
	}
	return true
}

var vegetarianForbiddenKeys = []string{
	"beef", "chicken", "cod", "fish", "hake", "ham", "meat", "pork", "salmon", "seafood", "shellfish", "shrimp", "tuna", "turkey",
	"atun", "bacalao", "carne", "cerdo", "gamba", "gambas", "jamon", "marisco", "merluza", "pavo", "pescado", "pollo", "ternera",
}

var veganForbiddenKeys = []string{
	"butter", "cheese", "cream", "dairy", "egg", "eggs", "honey", "milk", "yogurt", "yoghurt",
	"huevo", "huevos", "leche", "mantequilla", "miel", "nata", "queso",
}

func containsAnyKey(text string, keys []string) bool {
	for _, key := range keys {
		if containsNormalizedTerm(text, key) {
			return true
		}
	}
	return false
}

func veganDietText(text string) string {
	for _, allowed := range []string{
		"almond milk", "coconut milk", "oat milk", "plant milk", "rice milk", "soy milk",
		"leche de almendra", "leche de arroz", "leche de avena", "leche de coco", "leche de soja", "leche vegetal",
	} {
		text = strings.ReplaceAll(text, allowed, "")
	}
	return text
}

func containsAny(text string, values []string) bool {
	for _, value := range values {
		if containsNormalizedTerm(text, value) {
			return true
		}
	}
	return false
}

func containsNormalizedTerm(text, term string) bool {
	text = normalizeKey(text)
	term = normalizeKey(term)
	if text == "" || term == "" {
		return false
	}
	if strings.Contains(term, " ") {
		return strings.Contains(" "+text+" ", " "+term+" ")
	}
	for _, token := range strings.Fields(text) {
		if token == term || singularToken(token) == singularToken(term) {
			return true
		}
	}
	return false
}

func singularToken(token string) string {
	token = strings.TrimSpace(token)
	if len(token) > 4 && strings.HasSuffix(token, "es") {
		return strings.TrimSuffix(token, "es")
	}
	if len(token) > 3 && strings.HasSuffix(token, "s") {
		return strings.TrimSuffix(token, "s")
	}
	return token
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

func withNutrition(recipe Recipe) Recipe {
	summary, err := SummarizeNutrition(recipe)
	if err != nil {
		return recipe
	}
	recipe.NutritionPerServing = &summary
	return recipe
}

func templateByID(recipes []Recipe, id string) Recipe {
	for _, recipe := range recipes {
		if recipe.ID == id {
			return recipe
		}
	}
	if len(recipes) == 0 {
		return Recipe{}
	}
	return recipes[0]
}
