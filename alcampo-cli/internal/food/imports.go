package food

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"alcampo-cli/internal/alcampo"
	"alcampo-cli/internal/strutil"
)

type PantryImportOptions struct {
	Location   string
	ExpiryDate string
}

type PantryImportResult struct {
	Type             string           `json:"type"`
	ImportedAt       string           `json:"imported_at"`
	Applied          []ReceiveItem    `json:"applied,omitempty"`
	Skipped          []ReceiveSkipped `json:"skipped,omitempty"`
	Warnings         []string         `json:"warnings,omitempty"`
	Pantry           Pantry           `json:"pantry"`
	SuggestedStaples []Staple         `json:"suggested_staples,omitempty"`
}

var receiptPriceSuffixRE = regexp.MustCompile(`(?i)\s+\d+[,.]\d{2}\s*(?:eur)?\s*$`)
var receiptLeadingQtyRE = regexp.MustCompile(`(?i)^\s*(\d+(?:[,.]\d+)?)\s*(kg|g|gr|l|ml|cl|ud|uds|unit|unidades|unidad)?\s+(.+)$`)

func ImportOrdersToPantry(orders []alcampo.PastOrder, pantry Pantry, opts PantryImportOptions, inferStaples bool) (Pantry, PantryImportResult) {
	result := PantryImportResult{Type: "orders_imported", ImportedAt: nowStamp()}
	frequency := map[string]Staple{}
	for _, order := range orders {
		for _, line := range order.Lines {
			item, ingredient, product, reason, ok := pantryItemFromOrderLine(order, line, opts)
			if !ok {
				result.Skipped = append(result.Skipped, ReceiveSkipped{Ingredient: ingredient, Product: product, Reason: reason})
				continue
			}
			var saved PantryItem
			var err error
			pantry, saved, err = AddOrUpdatePantryItem(pantry, item)
			if err != nil {
				result.Skipped = append(result.Skipped, ReceiveSkipped{Ingredient: ingredient, Product: product, Reason: err.Error()})
				continue
			}
			result.Applied = append(result.Applied, ReceiveItem{Ingredient: ingredient, Product: product, PantryItem: saved, QuantityReason: reason})
			if inferStaples {
				key := normalizeKey(strutil.FirstNonEmpty(ingredient.SearchTerm, ingredient.Name))
				if key != "" {
					staple := frequency[key]
					staple.Name = ingredient.Name
					staple.SearchTerm = ingredient.SearchTerm
					staple.Unit = ingredient.Unit
					staple.Category = ingredient.Category
					staple.MinQty += ingredient.Quantity
					frequency[key] = staple
				}
			}
		}
	}
	if inferStaples {
		for _, staple := range frequency {
			staple.MinQty = roundQty(staple.MinQty / 2)
			if staple.MinQty <= 0 {
				staple.MinQty = 1
			}
			result.SuggestedStaples = append(result.SuggestedStaples, staple)
		}
		sort.SliceStable(result.SuggestedStaples, func(i, j int) bool {
			return result.SuggestedStaples[i].Name < result.SuggestedStaples[j].Name
		})
	}
	result.Pantry = pantry
	return pantry, result
}

func ImportReceiptToPantry(text string, pantry Pantry, opts PantryImportOptions) (Pantry, PantryImportResult) {
	result := PantryImportResult{Type: "receipt_imported", ImportedAt: nowStamp()}
	for lineNo, raw := range strings.Split(text, "\n") {
		item, ingredient, reason, ok := pantryItemFromReceiptLine(raw, lineNo+1, opts)
		if !ok {
			if strings.TrimSpace(raw) != "" {
				result.Warnings = append(result.Warnings, reason)
			}
			continue
		}
		var saved PantryItem
		var err error
		pantry, saved, err = AddOrUpdatePantryItem(pantry, item)
		if err != nil {
			result.Skipped = append(result.Skipped, ReceiveSkipped{Ingredient: ingredient, Reason: err.Error()})
			continue
		}
		result.Applied = append(result.Applied, ReceiveItem{Ingredient: ingredient, PantryItem: saved, QuantityReason: reason})
	}
	if len(result.Applied) == 0 && len(result.Warnings) == 0 {
		result.Warnings = append(result.Warnings, "no grocery receipt lines were recognized")
	}
	result.Pantry = pantry
	return pantry, result
}

func HistoryEventForImport(result PantryImportResult) map[string]any {
	return map[string]any{
		"type":              result.Type,
		"imported_at":       result.ImportedAt,
		"applied":           result.Applied,
		"skipped":           result.Skipped,
		"warnings":          result.Warnings,
		"suggested_staples": result.SuggestedStaples,
	}
}

func pantryItemFromOrderLine(order alcampo.PastOrder, line alcampo.PastOrderLine, opts PantryImportOptions) (PantryItem, Ingredient, ProductSummary, string, bool) {
	name := strutil.FirstNonEmpty(line.Name, line.SKU, line.ProductID)
	if name == "" {
		return PantryItem{}, Ingredient{}, ProductSummary{}, "order line had no product name or id", false
	}
	qty := line.Quantity
	unit := normalizeUnit(line.Unit)
	if qty <= 0 || unit == "" {
		if parsedQty, parsedUnit, ok := ParsePackageQuantity(line.Size, line.Name); ok {
			qty = parsedQty
			unit = parsedUnit
		}
	}
	if qty <= 0 {
		qty = 1
	}
	if unit == "" {
		unit = "unit"
	}
	ingredient := Ingredient{Name: name, Quantity: roundQty(qty), Unit: unit, Category: line.Category, SearchTerm: name}
	product := ProductSummary{ID: line.ProductID, SKU: line.SKU, Name: line.Name, Size: line.Size, Category: line.Category, PackageQuantity: qty, PackageUnit: unit}
	location := strings.TrimSpace(opts.Location)
	if location == "" {
		location = defaultStorageLocationForText(name + " " + line.Category)
	}
	item := PantryItem{
		Name:        name,
		Quantity:    roundQty(qty),
		Unit:        unit,
		Location:    location,
		ExpiryDate:  strings.TrimSpace(opts.ExpiryDate),
		Confidence:  0.75,
		LastChecked: nowStamp(),
		Notes:       strings.TrimSpace("imported from Alcampo order " + strutil.FirstNonEmpty(order.Number, order.ID)),
	}
	return item, ingredient, product, "imported from past order line", true
}

func pantryItemFromReceiptLine(raw string, lineNo int, opts PantryImportOptions) (PantryItem, Ingredient, string, bool) {
	line := strings.TrimSpace(raw)
	if line == "" || receiptSkipLine(line) {
		return PantryItem{}, Ingredient{}, "", false
	}
	line = strings.ReplaceAll(line, "\u20ac", "eur")
	line = receiptPriceSuffixRE.ReplaceAllString(line, "")
	qty := 1.0
	unit := "unit"
	name := line
	if parsedQty, parsedUnit, ok := ParsePackageQuantity(line); ok {
		qty = parsedQty
		unit = parsedUnit
	}
	if m := receiptLeadingQtyRE.FindStringSubmatch(line); len(m) == 4 {
		if value, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64); err == nil && value > 0 {
			qty = value
			if parsedUnit := normalizeUnit(m[2]); parsedUnit != "" {
				unit = parsedUnit
			}
			name = strings.TrimSpace(m[3])
		}
	}
	name = cleanReceiptName(name)
	if name == "" || len([]rune(name)) < 3 {
		return PantryItem{}, Ingredient{}, fmt.Sprintf("line %d skipped: could not infer item name", lineNo), false
	}
	location := strings.TrimSpace(opts.Location)
	if location == "" {
		location = defaultStorageLocationForText(name)
	}
	ingredient := Ingredient{Name: name, Quantity: roundQty(qty), Unit: unit, SearchTerm: name}
	item := PantryItem{
		Name:        name,
		Quantity:    roundQty(qty),
		Unit:        unit,
		Location:    location,
		ExpiryDate:  strings.TrimSpace(opts.ExpiryDate),
		Confidence:  0.45,
		LastChecked: nowStamp(),
		Notes:       fmt.Sprintf("imported from receipt line %d", lineNo),
	}
	return item, ingredient, "parsed from receipt text", true
}

func receiptSkipLine(line string) bool {
	key := normalizeKey(line)
	for _, token := range []string{"total", "subtotal", "iva", "tarjeta", "visa", "mastercard", "cambio", "efectivo", "recibo", "ticket"} {
		if strings.Contains(key, token) {
			return true
		}
	}
	return false
}

func cleanReceiptName(name string) string {
	name = receiptPriceSuffixRE.ReplaceAllString(strings.TrimSpace(name), "")
	name = strings.Trim(name, "-:*#0123456789 ")
	name = strings.Join(strings.Fields(name), " ")
	return name
}

func defaultStorageLocationForText(text string) string {
	key := normalizeKey(text)
	switch {
	case strings.Contains(key, "congel"):
		return "freezer"
	case strings.Contains(key, "pollo") ||
		strings.Contains(key, "carne") ||
		strings.Contains(key, "pescado") ||
		strings.Contains(key, "salmon") ||
		strings.Contains(key, "atun") ||
		strings.Contains(key, "leche") ||
		strings.Contains(key, "yogur") ||
		strings.Contains(key, "queso") ||
		strings.Contains(key, "huevo") ||
		strings.Contains(key, "verdura") ||
		strings.Contains(key, "fruta") ||
		strings.Contains(key, "tomate") ||
		strings.Contains(key, "lechuga"):
		return "fridge"
	default:
		return "pantry"
	}
}
