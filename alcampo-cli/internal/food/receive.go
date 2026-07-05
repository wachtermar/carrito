package food

import (
	"fmt"
	"strconv"
	"strings"
)

func ReceiveShopResult(shop ShopResult, pantry Pantry, opts ReceiveOptions) (Pantry, ReceiveResult) {
	result := ReceiveResult{
		MealPlanID: shop.MealPlanID,
		ReceivedAt: nowStamp(),
	}
	for _, selected := range shop.SelectedProducts {
		item, reason, ok := pantryItemFromSelection(selected, opts)
		if !ok {
			result.Skipped = append(result.Skipped, ReceiveSkipped{
				Ingredient: selected.Ingredient,
				Product:    selected.Product,
				Reason:     reason,
			})
			continue
		}
		var saved PantryItem
		var err error
		pantry, saved, err = AddOrUpdatePantryItem(pantry, item)
		if err != nil {
			result.Skipped = append(result.Skipped, ReceiveSkipped{
				Ingredient: selected.Ingredient,
				Product:    selected.Product,
				Reason:     err.Error(),
			})
			continue
		}
		result.Applied = append(result.Applied, ReceiveItem{
			Ingredient:       selected.Ingredient,
			Product:          selected.Product,
			PantryItem:       saved,
			PackageCount:     max(1, selected.PackageCount),
			QuantityReason:   reason,
			PurchaseQuantity: selected.PurchaseQuantity,
		})
	}
	result.Pantry = pantry
	return pantry, result
}

func pantryItemFromSelection(selected SelectedProduct, opts ReceiveOptions) (PantryItem, string, bool) {
	if selected.Error != "" {
		return PantryItem{}, "selected product has error: " + selected.Error, false
	}
	if selected.Product.SKU == "" && selected.Product.ID == "" && selected.Product.Name == "" {
		return PantryItem{}, "selected product is empty", false
	}
	count := selected.PackageCount
	if count <= 0 {
		if parsed, err := strconv.Atoi(strings.TrimSpace(selected.PurchaseQuantity)); err == nil && parsed > 0 {
			count = parsed
		}
	}
	if count <= 0 {
		count = 1
	}

	qty := 0.0
	unit := ""
	confidence := 0.9
	reason := ""
	productQty := selected.Product.PackageQuantity
	productUnit := normalizeUnit(selected.Product.PackageUnit)
	switch {
	case productQty > 0 && productUnit != "":
		qty = float64(count) * productQty
		unit = productUnit
		reason = fmt.Sprintf("received %d package(s): %.3g %s per package from selected product data", count, productQty, productUnit)
	case selected.Ingredient.Quantity > 0 && selected.Ingredient.Unit != "":
		qty = selected.Ingredient.Quantity
		unit = normalizeUnit(selected.Ingredient.Unit)
		confidence = 0.65
		reason = "package size was unknown; used planned ingredient quantity as pantry estimate"
	default:
		qty = float64(count)
		unit = "unit"
		confidence = 0.5
		reason = "package size was unknown; received quantity defaults to package count"
	}
	if qty <= 0 {
		return PantryItem{}, "could not infer received quantity", false
	}

	name := strings.TrimSpace(selected.Ingredient.Name)
	if name == "" {
		name = firstNonEmpty(selected.Product.Name, selected.Product.SKU, selected.Product.ID)
	}
	location := strings.TrimSpace(opts.Location)
	if location == "" {
		location = defaultStorageLocation(selected)
	}
	notes := strings.TrimSpace(strings.Join([]string{
		"received from Alcampo shop result",
		firstNonEmpty(selected.Product.SKU, selected.Product.ID),
		selected.Product.Name,
	}, "; "))
	return PantryItem{
		Name:        name,
		Quantity:    roundQty(qty),
		Unit:        unit,
		Location:    location,
		ExpiryDate:  strings.TrimSpace(opts.ExpiryDate),
		Confidence:  confidence,
		LastChecked: nowStamp(),
		Notes:       notes,
	}, reason, true
}

func defaultStorageLocation(selected SelectedProduct) string {
	text := normalizeKey(strings.Join([]string{
		selected.Ingredient.Category,
		selected.Ingredient.Name,
		selected.Product.Category,
		selected.Product.Name,
	}, " "))
	switch {
	case strings.Contains(text, "frozen") || strings.Contains(text, "congel"):
		return "freezer"
	case strings.Contains(text, "meat") ||
		strings.Contains(text, "fish") ||
		strings.Contains(text, "dairy") ||
		strings.Contains(text, "egg") ||
		strings.Contains(text, "vegetable") ||
		strings.Contains(text, "fruit") ||
		strings.Contains(text, "pollo") ||
		strings.Contains(text, "salmon") ||
		strings.Contains(text, "huevo") ||
		strings.Contains(text, "verdura") ||
		strings.Contains(text, "fruta"):
		return "fridge"
	default:
		return "pantry"
	}
}

func HistoryEventForReceive(result ReceiveResult) map[string]any {
	return map[string]any{
		"type":        "shop_received",
		"mealplan_id": result.MealPlanID,
		"received_at": result.ReceivedAt,
		"applied":     result.Applied,
		"skipped":     result.Skipped,
	}
}
