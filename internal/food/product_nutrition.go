package food

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var ErrNutritionUnavailable = errors.New("nutrition label has no numeric nutrient values")

func ParseAlcampoNutrition(raw string) (*StructuredNutrition, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrNutritionUnavailable
	}
	text := normalizeNutritionText(raw)
	nutrition := &StructuredNutrition{
		Basis:      NutritionUnknown,
		Source:     "alcampo_label",
		Raw:        raw,
		Confidence: 0.4,
	}
	if strings.Contains(text, "100 g") || strings.Contains(text, "100g") {
		nutrition.Basis = NutritionPer100g
		nutrition.BasisQty = 100
		nutrition.BasisUnit = "g"
		nutrition.Confidence = 0.85
	} else if strings.Contains(text, "100 ml") || strings.Contains(text, "100ml") {
		nutrition.Basis = NutritionPer100ml
		nutrition.BasisQty = 100
		nutrition.BasisUnit = "ml"
		nutrition.Confidence = 0.85
	} else if strings.Contains(text, "por racion") {
		nutrition.Basis = NutritionPerServing
		nutrition.BasisQty = 1
		nutrition.BasisUnit = "serving"
		nutrition.Confidence = 0.7
	} else if strings.Contains(text, "por unidad") {
		nutrition.Basis = NutritionPerUnit
		nutrition.BasisQty = 1
		nutrition.BasisUnit = "unit"
		nutrition.Confidence = 0.7
	} else {
		nutrition.ParseWarnings = append(nutrition.ParseWarnings, "nutrition basis was not found")
	}

	nutrition.EnergyKJ = parseNutrientValue(text, []string{"valor energetico"}, []string{"kj"})
	nutrition.Kcal = parseNutrientValue(text, []string{"valor energetico"}, []string{"kcal"})
	nutrition.SaturatesG = parseNutrientValue(text, []string{"de las cuales saturadas", "saturadas"}, []string{"g", "mg"})
	nutrition.SugarsG = parseNutrientValue(text, []string{"de los cuales azucares", "azucares"}, []string{"g", "mg"})
	nutrition.FatG = parseNutrientValue(text, []string{"grasas", "grasa"}, []string{"g", "mg"})
	nutrition.CarbsG = parseNutrientValue(text, []string{"hidratos de carbono"}, []string{"g", "mg"})
	nutrition.FiberG = parseNutrientValue(text, []string{"fibra alimentaria", "fibra"}, []string{"g", "mg"})
	nutrition.ProteinG = parseNutrientValue(text, []string{"proteinas", "proteina"}, []string{"g", "mg"})
	nutrition.SaltG = parseNutrientValue(text, []string{"sal"}, []string{"g", "mg"})
	nutrition.SodiumG = parseNutrientValue(text, []string{"sodio"}, []string{"g", "mg"})
	for _, lessThan := range lessThanNutritionWarnings(text) {
		nutrition.ParseWarnings = append(nutrition.ParseWarnings, lessThan)
	}
	if !structuredNutritionHasValues(nutrition) {
		return nil, ErrNutritionUnavailable
	}
	return nutrition, nil
}

func AttachSelectedProductNutrition(selection *SelectedProduct) {
	if selection == nil || selection.Error != "" {
		return
	}
	raw := strings.TrimSpace(selection.Product.Nutrition)
	if raw == "" {
		if selection.Product.Name != "" {
			selection.NutritionWarnings = append(selection.NutritionWarnings, "No Alcampo product nutrition label was available for this selected product.")
		}
		return
	}
	parsed, err := ParseAlcampoNutrition(raw)
	if err != nil {
		selection.NutritionWarnings = append(selection.NutritionWarnings, "Could not parse Alcampo product nutrition label: "+err.Error())
		return
	}
	selection.Product.NutritionParsed = parsed
	selection.ProductNutrition = parsed
	selection.NutritionProvenance = parsed.Source
	required, warnings := EstimateRequiredNutrition(selection.Ingredient, parsed)
	selection.NutritionWarnings = append(selection.NutritionWarnings, warnings...)
	selection.RequiredNutrition = required
	purchased, warnings := EstimatePurchasedNutrition(selection.Product, selection.PackageCount, parsed)
	selection.NutritionWarnings = append(selection.NutritionWarnings, warnings...)
	selection.PurchasedNutrition = purchased
}

func ProductNutritionReportFromSelection(selection SelectedProduct) *ProductNutritionReport {
	if selection.Product.Name == "" && selection.ProductNutrition == nil && len(selection.NutritionWarnings) == 0 {
		return nil
	}
	return &ProductNutritionReport{
		ProductID:          selection.Product.ID,
		SKU:                selection.Product.SKU,
		ProductName:        selection.Product.Name,
		LabelNutrition:     selection.ProductNutrition,
		RequiredNutrition:  selection.RequiredNutrition,
		PurchasedNutrition: selection.PurchasedNutrition,
		Warnings:           append([]string(nil), selection.NutritionWarnings...),
	}
}

func EstimateRequiredNutrition(ingredient Ingredient, label *StructuredNutrition) (*NutritionEstimate, []string) {
	if label == nil {
		return nil, []string{"No parsed product nutrition label was available."}
	}
	factor, warnings := nutritionFactor(ingredient.Quantity, ingredient.Unit, label)
	if len(warnings) > 0 {
		return nil, warnings
	}
	return &NutritionEstimate{
		RequiredQuantity: ingredient.Quantity,
		RequiredUnit:     ingredient.Unit,
		Factor:           factor,
		Nutrients:        scaleStructuredNutrition(*label, factor),
		Source:           label.Source,
		Confidence:       label.Confidence,
	}, nil
}

func EstimatePurchasedNutrition(product ProductSummary, packageCount int, label *StructuredNutrition) (*NutritionEstimate, []string) {
	if label == nil || packageCount <= 0 || product.PackageQuantity <= 0 || product.PackageUnit == "" {
		return nil, nil
	}
	quantity := product.PackageQuantity * float64(packageCount)
	factor, warnings := nutritionFactor(quantity, product.PackageUnit, label)
	if len(warnings) > 0 {
		return nil, warnings
	}
	return &NutritionEstimate{
		RequiredQuantity: quantity,
		RequiredUnit:     product.PackageUnit,
		Factor:           factor,
		Nutrients:        scaleStructuredNutrition(*label, factor),
		Source:           label.Source,
		Confidence:       label.Confidence,
	}, nil
}

func nutritionFactor(quantity float64, unit string, label *StructuredNutrition) (float64, []string) {
	if quantity <= 0 {
		return 0, []string{"Nutrition estimate unavailable because the quantity is missing."}
	}
	baseQty, baseUnit, ok := toBaseQuantity(quantity, unit)
	if !ok {
		return 0, []string{fmt.Sprintf("Nutrition estimate unavailable because unit %q cannot be converted safely.", unit)}
	}
	switch label.Basis {
	case NutritionPer100g:
		if baseUnit == "g" {
			return baseQty / 100, nil
		}
	case NutritionPer100ml:
		if baseUnit == "ml" {
			return baseQty / 100, nil
		}
	case NutritionPerUnit:
		if baseUnit == "unit" {
			return baseQty, nil
		}
	}
	return 0, []string{fmt.Sprintf("Nutrition estimate unavailable because %s does not match label basis %s.", baseUnit, label.Basis)}
}

func normalizeNutritionText(raw string) string {
	text := strings.ToLower(strings.TrimSpace(raw))
	replacer := strings.NewReplacer(
		"\u00a0", " ",
		"\n", " ",
		"\r", " ",
		"\t", " ",
		",", ".",
		"á", "a",
		"é", "e",
		"í", "i",
		"ó", "o",
		"ú", "u",
		"ü", "u",
		"ñ", "n",
	)
	text = replacer.Replace(text)
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`(?i)(\d)(g|mg|kj|kcal|ml)\b`).ReplaceAllString(text, "$1 $2")
	text = regexp.MustCompile(`(?i)100\s*(g|ml)\b`).ReplaceAllString(text, "100 $1")
	return text
}

func parseNutrientValue(text string, labels []string, units []string) *float64 {
	for _, label := range labels {
		idx := strings.Index(text, label)
		if idx < 0 {
			continue
		}
		end := nextNutritionLabelIndex(text, idx+len(label))
		if end > len(text) {
			end = len(text)
		}
		segment := text[idx:end]
		for _, unit := range units {
			pattern := regexp.MustCompile(`(<\s*)?([0-9]+(?:\.[0-9]+)?)\s*` + regexp.QuoteMeta(unit) + `\b`)
			m := pattern.FindStringSubmatch(segment)
			if len(m) < 3 {
				continue
			}
			value, err := strconv.ParseFloat(m[2], 64)
			if err != nil {
				continue
			}
			if unit == "mg" {
				value = value / 1000
			}
			return floatPtr(value)
		}
	}
	return nil
}

func nextNutritionLabelIndex(text string, start int) int {
	end := len(text)
	for _, label := range []string{
		"valor energetico",
		"de las cuales saturadas",
		"saturadas",
		"grasas",
		"grasa",
		"hidratos de carbono",
		"de los cuales azucares",
		"azucares",
		"fibra alimentaria",
		"fibra",
		"proteinas",
		"proteina",
		"sal",
		"sodio",
	} {
		if idx := strings.Index(text[start:], label); idx >= 0 && start+idx < end {
			end = start + idx
		}
	}
	return end
}

func lessThanNutritionWarnings(text string) []string {
	re := regexp.MustCompile(`<\s*([0-9]+(?:\.[0-9]+)?)\s*(g|mg)\b`)
	matches := re.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	var warnings []string
	for _, match := range matches {
		warnings = append(warnings, "less-than nutrition value was rounded up to "+match[1]+" "+match[2])
	}
	return warnings
}

func structuredNutritionHasValues(n *StructuredNutrition) bool {
	return n != nil && (n.EnergyKJ != nil || n.Kcal != nil || n.FatG != nil || n.SaturatesG != nil || n.CarbsG != nil || n.SugarsG != nil || n.FiberG != nil || n.ProteinG != nil || n.SaltG != nil || n.SodiumG != nil)
}

func scaleStructuredNutrition(n StructuredNutrition, factor float64) StructuredNutrition {
	n.Raw = ""
	n.ParseWarnings = nil
	n.EnergyKJ = scaleFloatPtr(n.EnergyKJ, factor)
	n.Kcal = scaleFloatPtr(n.Kcal, factor)
	n.FatG = scaleFloatPtr(n.FatG, factor)
	n.SaturatesG = scaleFloatPtr(n.SaturatesG, factor)
	n.CarbsG = scaleFloatPtr(n.CarbsG, factor)
	n.SugarsG = scaleFloatPtr(n.SugarsG, factor)
	n.FiberG = scaleFloatPtr(n.FiberG, factor)
	n.ProteinG = scaleFloatPtr(n.ProteinG, factor)
	n.SaltG = scaleFloatPtr(n.SaltG, factor)
	n.SodiumG = scaleFloatPtr(n.SodiumG, factor)
	return n
}

func scaleFloatPtr(v *float64, factor float64) *float64 {
	if v == nil {
		return nil
	}
	return floatPtr(roundNutritionValue(*v * factor))
}

func floatPtr(v float64) *float64 {
	return &v
}

func roundNutritionValue(v float64) float64 {
	rounded := strconv.FormatFloat(v, 'f', 3, 64)
	parsed, err := strconv.ParseFloat(strings.TrimRight(strings.TrimRight(rounded, "0"), "."), 64)
	if err != nil {
		return v
	}
	return parsed
}
