package food

import (
	"fmt"
	"sort"
	"strings"
)

func DaysUntilExpiry(expiry string) (int, bool) {
	return daysUntilExpiry(expiry)
}

func FilterExpiringPantry(pantry Pantry, days int) Pantry {
	if days < 0 {
		return pantry
	}
	filtered := Pantry{UpdatedAt: pantry.UpdatedAt}
	for _, item := range pantry.Items {
		if daysLeft, ok := daysUntilExpiry(item.ExpiryDate); ok && daysLeft <= days {
			filtered.Items = append(filtered.Items, item)
		}
	}
	return filtered
}

func SuggestUseUpRecipes(profile Profile, pantry Pantry, days, limit int) ([]UseUpSuggestion, error) {
	if days <= 0 {
		days = 3
	}
	if limit <= 0 {
		limit = 8
	}
	expiring := expiringPantryItems(pantry, days)
	if len(expiring) == 0 {
		return nil, nil
	}
	recipes, err := LoadRecipes()
	if err != nil {
		return nil, err
	}
	recipes = rankRecipeTemplates(filterTemplates(recipes, profile), profile, Pantry{Items: expiring})
	people := profile.People
	if people <= 0 {
		people = 2
	}
	var suggestions []UseUpSuggestion
	for _, recipe := range recipes {
		score, items := useUpScore(recipe, expiring)
		if score <= 0 {
			continue
		}
		suggestions = append(suggestions, UseUpSuggestion{
			Recipe:        withNutrition(scaleRecipe(recipe, people)),
			Score:         score,
			ExpiringItems: items,
			Reason:        fmt.Sprintf("uses %d expiring pantry item(s)", len(items)),
		})
	}
	sort.SliceStable(suggestions, func(i, j int) bool {
		if suggestions[i].Score == suggestions[j].Score {
			return suggestions[i].Recipe.Title < suggestions[j].Recipe.Title
		}
		return suggestions[i].Score > suggestions[j].Score
	})
	if len(suggestions) > limit {
		suggestions = suggestions[:limit]
	}
	return suggestions, nil
}

func expiringPlanNotes(pantry Pantry, usage []PantryUsage, days int) []string {
	expiring := expiringPantryItems(pantry, days)
	if len(expiring) == 0 {
		return nil
	}
	expiringNames := make(map[string]PantryItem, len(expiring))
	for _, item := range expiring {
		expiringNames[normalizeKey(item.Name)] = item
	}
	used := map[string]bool{}
	for _, use := range usage {
		key := normalizeKey(use.PantryItem)
		if _, ok := expiringNames[key]; ok {
			used[use.PantryItem] = true
		}
	}
	var usedNames []string
	for name := range used {
		usedNames = append(usedNames, name)
	}
	sort.Strings(usedNames)
	if len(usedNames) == 0 {
		return []string{fmt.Sprintf("Expiring soon: %d pantry item(s), none used by selected recipes.", len(expiring))}
	}
	return []string{fmt.Sprintf("Expiring soon: %d pantry item(s), used in %d planned item(s): %s.", len(expiring), len(usedNames), strings.Join(usedNames, ", "))}
}

func expiringPantryItems(pantry Pantry, days int) []PantryItem {
	if days < 0 {
		return nil
	}
	var items []PantryItem
	for _, item := range pantry.Items {
		daysLeft, ok := daysUntilExpiry(item.ExpiryDate)
		if !ok || daysLeft < 0 || daysLeft > days {
			continue
		}
		items = append(items, item)
	}
	return items
}

func useUpScore(recipe Recipe, items []PantryItem) (float64, []string) {
	var score float64
	var matched []string
	for _, item := range items {
		if !recipeUsesPantryItem(recipe, item) {
			continue
		}
		daysLeft, _ := daysUntilExpiry(item.ExpiryDate)
		score += 100 - float64(daysLeft*10)
		matched = append(matched, item.Name)
	}
	return score, matched
}
