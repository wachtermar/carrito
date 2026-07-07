package food

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"

	"alcampo-cli/internal/strutil"
)

func AddOrUpdatePantryItem(p Pantry, item PantryItem) (Pantry, PantryItem, error) {
	item.Name = strings.TrimSpace(item.Name)
	if item.Name == "" {
		return p, PantryItem{}, errors.New("pantry item name is required")
	}
	item.Unit = normalizeUnit(item.Unit)
	if item.Quantity < 0 {
		return p, PantryItem{}, errors.New("quantity cannot be negative")
	}
	if item.Confidence == 0 {
		item.Confidence = 1
	}
	if item.LastChecked == "" {
		item.LastChecked = nowStamp()
	}
	item.ID = itemID(item.Name, item.Location)
	for i := range p.Items {
		if samePantryItem(p.Items[i], item) {
			p.Items[i].Quantity += item.Quantity
			if item.Unit != "" {
				p.Items[i].Unit = item.Unit
			}
			if item.Location != "" {
				p.Items[i].Location = item.Location
			}
			if item.ExpiryDate != "" {
				p.Items[i].ExpiryDate = item.ExpiryDate
			}
			if item.Confidence != 0 {
				p.Items[i].Confidence = item.Confidence
			}
			p.Items[i].LastChecked = item.LastChecked
			if item.Notes != "" {
				p.Items[i].Notes = item.Notes
			}
			return p, p.Items[i], nil
		}
	}
	p.Items = append(p.Items, item)
	sortPantry(p.Items)
	return p, item, nil
}

func SetPantryItem(p Pantry, item PantryItem) (Pantry, PantryItem, error) {
	item.Name = strings.TrimSpace(item.Name)
	if item.Name == "" {
		return p, PantryItem{}, errors.New("pantry item name is required")
	}
	item.Unit = normalizeUnit(item.Unit)
	if item.Quantity < 0 {
		return p, PantryItem{}, errors.New("quantity cannot be negative")
	}
	if item.Confidence == 0 {
		item.Confidence = 1
	}
	if item.LastChecked == "" {
		item.LastChecked = nowStamp()
	}
	item.ID = itemID(item.Name, item.Location)
	for i := range p.Items {
		if samePantryItem(p.Items[i], item) || namesMatch(p.Items[i].Name, item.Name) {
			if item.Location == "" {
				item.Location = p.Items[i].Location
			}
			if item.Unit == "" {
				item.Unit = p.Items[i].Unit
			}
			p.Items[i] = item
			return p, p.Items[i], nil
		}
	}
	p.Items = append(p.Items, item)
	sortPantry(p.Items)
	return p, item, nil
}

func UsePantryItem(p Pantry, name string, qty float64, unit string) (Pantry, PantryItem, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return p, PantryItem{}, errors.New("pantry item name is required")
	}
	if qty <= 0 {
		return p, PantryItem{}, errors.New("quantity must be greater than zero")
	}
	unit = normalizeUnit(unit)
	for i := range p.Items {
		item := p.Items[i]
		if !namesMatch(item.Name, name) {
			continue
		}
		if unit != "" && item.Unit != "" && normalizeUnit(item.Unit) != unit {
			continue
		}
		used := qty
		if used > item.Quantity {
			used = item.Quantity
		}
		p.Items[i].Quantity = roundQty(item.Quantity - used)
		p.Items[i].LastChecked = nowStamp()
		updated := p.Items[i]
		if p.Items[i].Quantity == 0 {
			p.Items = append(p.Items[:i], p.Items[i+1:]...)
		}
		return p, updated, nil
	}
	return p, PantryItem{}, fmt.Errorf("pantry item %q not found", name)
}

func ApplyPantry(planRequired []Ingredient, pantry Pantry) ([]Ingredient, []PantryUsage) {
	items := make([]PantryItem, len(pantry.Items))
	copy(items, pantry.Items)
	sortPantry(items)

	var purchases []Ingredient
	var usage []PantryUsage
	for _, ing := range planRequired {
		remaining := ing.Quantity
		if remaining <= 0 {
			remaining = 1
		}
		for i := range items {
			if remaining <= 0 {
				break
			}
			if items[i].Quantity <= 0 || !ingredientMatchesPantry(ing, items[i]) {
				continue
			}
			used := math.Min(remaining, items[i].Quantity)
			if used <= 0 {
				continue
			}
			items[i].Quantity = roundQty(items[i].Quantity - used)
			remaining = roundQty(remaining - used)
			usage = append(usage, PantryUsage{
				Ingredient: ing.Name,
				PantryItem: items[i].Name,
				Quantity:   used,
				Unit:       strutil.FirstNonEmpty(ing.Unit, items[i].Unit),
				Location:   items[i].Location,
			})
		}
		if remaining > 0 {
			need := ing
			need.Quantity = remaining
			purchases = append(purchases, need)
		}
	}
	return mergeIngredients(purchases), usage
}

func mergeIngredients(items []Ingredient) []Ingredient {
	byKey := map[string]int{}
	var out []Ingredient
	for _, item := range items {
		key := normalizeKey(strutil.FirstNonEmpty(item.SearchTerm, item.Name)) + "|" + normalizeUnit(item.Unit)
		if idx, ok := byKey[key]; ok {
			out[idx].Quantity = roundQty(out[idx].Quantity + item.Quantity)
			continue
		}
		byKey[key] = len(out)
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func ingredientMatchesPantry(ing Ingredient, item PantryItem) bool {
	if ing.Unit != "" && item.Unit != "" && normalizeUnit(ing.Unit) != normalizeUnit(item.Unit) {
		return false
	}
	return namesMatch(ing.Name, item.Name) || namesMatch(strutil.FirstNonEmpty(ing.SearchTerm, ing.Name), item.Name)
}

func samePantryItem(a, b PantryItem) bool {
	return normalizeKey(a.Name) == normalizeKey(b.Name) &&
		normalizeUnit(a.Unit) == normalizeUnit(b.Unit) &&
		strings.EqualFold(strings.TrimSpace(a.Location), strings.TrimSpace(b.Location))
}

func namesMatch(a, b string) bool {
	ak := normalizeKey(a)
	bk := normalizeKey(b)
	if ak == "" || bk == "" {
		return false
	}
	return ak == bk || strings.Contains(ak, bk) || strings.Contains(bk, ak)
}

func sortPantry(items []PantryItem) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].ExpiryDate != "" && items[j].ExpiryDate != "" && items[i].ExpiryDate != items[j].ExpiryDate {
			return items[i].ExpiryDate < items[j].ExpiryDate
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
}

func normalizeKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastSpace := false
	for _, r := range s {
		r = foldKeyRune(r)
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastSpace = false
		case unicode.IsSpace(r) || r == '-' || r == '_' || r == '/':
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func foldKeyRune(r rune) rune {
	switch r {
	case '\u00e1', '\u00e0', '\u00e2', '\u00e4':
		return 'a'
	case '\u00e9', '\u00e8', '\u00ea', '\u00eb':
		return 'e'
	case '\u00ed', '\u00ec', '\u00ee', '\u00ef':
		return 'i'
	case '\u00f3', '\u00f2', '\u00f4', '\u00f6':
		return 'o'
	case '\u00fa', '\u00f9', '\u00fb', '\u00fc':
		return 'u'
	case '\u00f1':
		return 'n'
	default:
		return r
	}
}

func normalizeUnit(unit string) string {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "grams", "gram", "gr", "g":
		return "g"
	case "kilogram", "kilograms", "kg":
		return "kg"
	case "millilitre", "milliliter", "millilitres", "milliliters", "ml":
		return "ml"
	case "centilitre", "centiliter", "centilitres", "centiliters", "cl":
		return "cl"
	case "litre", "liter", "litres", "liters", "l":
		return "l"
	case "unit", "units", "unidad", "unidades", "ud", "u":
		return "unit"
	default:
		return strings.ToLower(strings.TrimSpace(unit))
	}
}

func roundQty(v float64) float64 {
	return math.Round(v*1000) / 1000
}
