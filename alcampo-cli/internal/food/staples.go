package food

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

func AddOrUpdateStaple(profile Profile, staple Staple) (Profile, Staple, error) {
	staple.Name = strings.TrimSpace(staple.Name)
	if staple.Name == "" {
		return profile, Staple{}, errors.New("staple name is required")
	}
	if staple.MinQty <= 0 {
		return profile, Staple{}, errors.New("staple minimum quantity must be greater than zero")
	}
	staple.Unit = normalizeUnit(staple.Unit)
	if staple.SearchTerm == "" {
		staple.SearchTerm = staple.Name
	}
	for i := range profile.Staples {
		if sameStaple(profile.Staples[i], staple) || namesMatch(profile.Staples[i].Name, staple.Name) {
			profile.Staples[i] = staple
			sortStaples(profile.Staples)
			return profile, staple, nil
		}
	}
	profile.Staples = append(profile.Staples, staple)
	sortStaples(profile.Staples)
	return profile, staple, nil
}

func RemoveStaple(profile Profile, name string) (Profile, Staple, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return profile, Staple{}, errors.New("staple name is required")
	}
	for i, staple := range profile.Staples {
		if namesMatch(staple.Name, name) || namesMatch(staple.SearchTerm, name) {
			profile.Staples = append(profile.Staples[:i], profile.Staples[i+1:]...)
			return profile, staple, nil
		}
	}
	return profile, Staple{}, fmt.Errorf("staple %q not found", name)
}

func StapleRestockIngredients(profile Profile, pantry Pantry) []Ingredient {
	var needs []Ingredient
	for _, staple := range profile.Staples {
		unit := normalizeUnit(staple.Unit)
		minQty := staple.MinQty
		if minQty <= 0 || strings.TrimSpace(staple.Name) == "" {
			continue
		}
		have := pantryQuantityForStaple(staple, pantry)
		missing := roundQty(minQty - have)
		if missing <= 0 {
			continue
		}
		needs = append(needs, Ingredient{
			Name:       staple.Name,
			Quantity:   missing,
			Unit:       unit,
			Category:   firstNonEmpty(staple.Category, "staple"),
			SearchTerm: firstNonEmpty(staple.SearchTerm, staple.Name),
		})
	}
	return mergeIngredients(needs)
}

func pantryQuantityForStaple(staple Staple, pantry Pantry) float64 {
	unit := normalizeUnit(staple.Unit)
	total := 0.0
	for _, item := range pantry.Items {
		if !namesMatch(staple.Name, item.Name) && !namesMatch(firstNonEmpty(staple.SearchTerm, staple.Name), item.Name) {
			continue
		}
		if unit != "" && item.Unit != "" && normalizeUnit(item.Unit) != unit {
			continue
		}
		total += item.Quantity
	}
	return roundQty(total)
}

func sameStaple(a, b Staple) bool {
	return normalizeKey(a.Name) == normalizeKey(b.Name) && normalizeUnit(a.Unit) == normalizeUnit(b.Unit)
}

func sortStaples(staples []Staple) {
	sort.SliceStable(staples, func(i, j int) bool {
		return strings.ToLower(staples[i].Name) < strings.ToLower(staples[j].Name)
	})
}
