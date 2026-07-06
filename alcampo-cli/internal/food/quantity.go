package food

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"alcampo-cli/internal/money"
	"alcampo-cli/internal/strutil"
)

var (
	multipliedPackageRE = regexp.MustCompile(`(?i)(\d+(?:[,.]\d+)?)\s*(?:x|×)\s*(\d+(?:[,.]\d+)?)\s*(kg|kilo|kilos|g|gr|gramos|ml|cl|l|litro|litros|unit|ud|uds|unidad|unidades|u)\b`)
	packageRE           = regexp.MustCompile(`(?i)(\d+(?:[,.]\d+)?)\s*(kg|kilo|kilos|g|gr|gramos|ml|cl|l|litro|litros|unit|ud|uds|unidad|unidades|u)\b`)
	countPackageRE      = regexp.MustCompile(`(?i)(?:pack|paquete|caja|docena|bandeja)?\s*(?:de)?\s*(\d+)\s*(?:unit|unidades|unidad|uds|ud|u)\b`)
)

func EstimatePurchaseQuantity(ingredient Ingredient, product ProductSummary) (count int, lineTotal money.Money, reason string, warnings []string) {
	count = 1
	requiredQty := ingredient.Quantity
	requiredUnit := normalizeUnit(ingredient.Unit)
	if requiredQty <= 0 || requiredUnit == "" {
		reason = "quantity defaults to one retail package because the recipe ingredient has no measurable quantity"
		lineTotal = multiplyMoney(product.Price, count)
		return count, lineTotal, reason, warnings
	}

	packageQty := product.PackageQuantity
	packageUnit := normalizeUnit(product.PackageUnit)
	if packageQty <= 0 || packageUnit == "" {
		warnings = append(warnings, "Could not infer retail package size from product data; quantity defaults to one package.")
		reason = "quantity defaults to one retail package because package size is unknown"
		lineTotal = multiplyMoney(product.Price, count)
		return count, lineTotal, reason, warnings
	}

	requiredBase, requiredBaseUnit, ok := toBaseQuantity(requiredQty, requiredUnit)
	if !ok {
		warnings = append(warnings, fmt.Sprintf("Could not normalize required unit %q; quantity defaults to one package.", ingredient.Unit))
		reason = "quantity defaults to one retail package because required unit is unsupported"
		lineTotal = multiplyMoney(product.Price, count)
		return count, lineTotal, reason, warnings
	}
	packageBase, packageBaseUnit, ok := toBaseQuantity(packageQty, packageUnit)
	if !ok || packageBaseUnit != requiredBaseUnit {
		warnings = append(warnings, fmt.Sprintf("Product package unit %q does not match required unit %q; verify quantity manually.", product.PackageUnit, ingredient.Unit))
		reason = "quantity defaults to one retail package because product package unit does not match recipe unit"
		lineTotal = multiplyMoney(product.Price, count)
		return count, lineTotal, reason, warnings
	}
	if packageBase <= 0 {
		warnings = append(warnings, "Product package size is zero; quantity defaults to one package.")
		reason = "quantity defaults to one retail package because package size is invalid"
		lineTotal = multiplyMoney(product.Price, count)
		return count, lineTotal, reason, warnings
	}

	count = int(math.Ceil(requiredBase / packageBase))
	if count < 1 {
		count = 1
	}
	lineTotal = multiplyMoney(product.Price, count)
	reason = fmt.Sprintf("calculated %d package(s): %.3g %s required, %.3g %s per package", count, requiredQty, requiredUnit, packageQty, packageUnit)
	if count*int(math.Round(packageBase)) > int(math.Round(requiredBase*1.6)) && count > 1 {
		warnings = append(warnings, "Package fit is inefficient; consider an alternate product or recipe substitution.")
	}
	return count, lineTotal, reason, warnings
}

func ParsePackageQuantity(values ...string) (float64, string, bool) {
	text := normalizePackageText(strings.Join(values, " "))
	if text == "" {
		return 0, "", false
	}
	if m := multipliedPackageRE.FindStringSubmatch(text); len(m) == 4 {
		left, ok1 := parsePackageNumber(m[1])
		right, ok2 := parsePackageNumber(m[2])
		unit := normalizeUnit(m[3])
		if ok1 && ok2 && unit != "" {
			return left * right, unit, true
		}
	}
	if m := countPackageRE.FindStringSubmatch(text); len(m) == 2 {
		qty, ok := parsePackageNumber(m[1])
		if ok {
			return qty, "unit", true
		}
	}
	if m := packageRE.FindStringSubmatch(text); len(m) == 3 {
		qty, ok := parsePackageNumber(m[1])
		unit := normalizeUnit(m[2])
		if ok && unit != "" {
			return qty, unit, true
		}
	}
	return 0, "", false
}

func toBaseQuantity(qty float64, unit string) (float64, string, bool) {
	switch normalizeUnit(unit) {
	case "g":
		return qty, "g", true
	case "kg":
		return qty * 1000, "g", true
	case "ml":
		return qty, "ml", true
	case "cl":
		return qty * 10, "ml", true
	case "l":
		return qty * 1000, "ml", true
	case "unit":
		return qty, "unit", true
	default:
		return 0, "", false
	}
}

func multiplyMoney(price money.Money, count int) money.Money {
	if count <= 0 || price.Amount == "" {
		return money.Money{}
	}
	cents := price.Cents * int64(count)
	return money.Money{Amount: money.FormatAmount(cents), Currency: strutil.FirstNonEmpty(price.Currency, "EUR"), Cents: cents}
}

func normalizePackageText(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	replacements := []struct {
		old string
		new string
	}{
		{"unidades", "unit"},
		{"unidad", "unit"},
		{"litros", "l"},
		{"litro", "l"},
		{"gramos", "g"},
		{"kilos", "kg"},
		{"kilo", "kg"},
		{"×", "x"},
	}
	for _, replacement := range replacements {
		s = strings.ReplaceAll(s, replacement.old, replacement.new)
	}
	return s
}

func parsePackageNumber(raw string) (float64, bool) {
	raw = strings.ReplaceAll(strings.TrimSpace(raw), ",", ".")
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(raw, 64)
	return value, err == nil && value > 0
}
