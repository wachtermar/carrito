package food

import (
	"errors"
	"strings"
	"testing"

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/money"
)

func TestParseAlcampoNutritionChocolateStyle(t *testing.T) {
	raw := `Valores medios por 100 g Valor energético: 2262 kJ / 544 kcal
Grasas: 36 g de las cuales saturadas: 15 g
Hidratos de carbono: 41 g de los cuales azúcares: 38 g
Proteínas: 11 g Sal: 0,01 g`
	n, err := ParseAlcampoNutrition(raw)
	if err != nil {
		t.Fatal(err)
	}
	if n.Basis != NutritionPer100g || n.BasisQty != 100 || n.BasisUnit != "g" {
		t.Fatalf("basis = %+v", n)
	}
	assertFloatPtr(t, "energy", n.EnergyKJ, 2262)
	assertFloatPtr(t, "kcal", n.Kcal, 544)
	assertFloatPtr(t, "fat", n.FatG, 36)
	assertFloatPtr(t, "saturates", n.SaturatesG, 15)
	assertFloatPtr(t, "carbs", n.CarbsG, 41)
	assertFloatPtr(t, "sugars", n.SugarsG, 38)
	assertFloatPtr(t, "protein", n.ProteinG, 11)
	assertFloatPtr(t, "salt", n.SaltG, 0.01)
}

func TestParseAlcampoNutritionCheeseStyleDecimalComma(t *testing.T) {
	raw := `Valores medios por 100g Valor energético 1828 kJ/441 kcal
Grasas 36,4 g de las cuales saturadas 26 g
Hidratos de carbono 1,6 g de los cuales azúcares 0,5 g
Proteínas 27 g Sal 1,6 g`
	n, err := ParseAlcampoNutrition(raw)
	if err != nil {
		t.Fatal(err)
	}
	assertFloatPtr(t, "fat", n.FatG, 36.4)
	assertFloatPtr(t, "carbs", n.CarbsG, 1.6)
	assertFloatPtr(t, "sugars", n.SugarsG, 0.5)
	assertFloatPtr(t, "salt", n.SaltG, 1.6)
}

func TestParseAlcampoNutritionLessThanAndMgToG(t *testing.T) {
	raw := `Por 100 ml Valor energético 20 kcal Grasas <0,5 g Proteínas 500 mg Sal <0,01 g`
	n, err := ParseAlcampoNutrition(raw)
	if err != nil {
		t.Fatal(err)
	}
	if n.Basis != NutritionPer100ml {
		t.Fatalf("basis = %s", n.Basis)
	}
	assertFloatPtr(t, "fat", n.FatG, 0.5)
	assertFloatPtr(t, "protein", n.ProteinG, 0.5)
	assertFloatPtr(t, "salt", n.SaltG, 0.01)
	if len(n.ParseWarnings) < 2 || !strings.Contains(strings.Join(n.ParseWarnings, " "), "less-than") {
		t.Fatalf("missing less-than warnings: %+v", n.ParseWarnings)
	}
}

func TestParseAlcampoNutritionNoNumericMarketingOnly(t *testing.T) {
	_, err := ParseAlcampoNutrition("Alto en proteínas y bajo en grasa.")
	if !errors.Is(err, ErrNutritionUnavailable) {
		t.Fatalf("err = %v, want ErrNutritionUnavailable", err)
	}
}

func TestParseAlcampoNutritionUnknownBasis(t *testing.T) {
	n, err := ParseAlcampoNutrition("Valor energético 100 kcal Proteínas 5 g")
	if err != nil {
		t.Fatal(err)
	}
	if n.Basis != NutritionUnknown || len(n.ParseWarnings) == 0 {
		t.Fatalf("expected unknown basis warning: %+v", n)
	}
}

func TestRequiredNutritionPer100g(t *testing.T) {
	label := &StructuredNutrition{Basis: NutritionPer100g, BasisQty: 100, BasisUnit: "g", Kcal: floatPtr(200), ProteinG: floatPtr(10), Source: "test", Confidence: 0.9}
	estimate, warnings := EstimateRequiredNutrition(Ingredient{Name: "rice", Quantity: 250, Unit: "g"}, label)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v", warnings)
	}
	if estimate == nil || estimate.Factor != 2.5 {
		t.Fatalf("estimate = %+v", estimate)
	}
	assertFloatPtr(t, "kcal", estimate.Nutrients.Kcal, 500)
	assertFloatPtr(t, "protein", estimate.Nutrients.ProteinG, 25)
}

func TestRequiredNutritionPer100ml(t *testing.T) {
	label := &StructuredNutrition{Basis: NutritionPer100ml, BasisQty: 100, BasisUnit: "ml", Kcal: floatPtr(50)}
	estimate, warnings := EstimateRequiredNutrition(Ingredient{Name: "milk", Quantity: 1.2, Unit: "l"}, label)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v", warnings)
	}
	assertFloatPtr(t, "kcal", estimate.Nutrients.Kcal, 600)
}

func TestRequiredNutritionPerUnit(t *testing.T) {
	label := &StructuredNutrition{Basis: NutritionPerUnit, BasisQty: 1, BasisUnit: "unit", Kcal: floatPtr(70)}
	estimate, warnings := EstimateRequiredNutrition(Ingredient{Name: "eggs", Quantity: 2, Unit: "unit"}, label)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v", warnings)
	}
	assertFloatPtr(t, "kcal", estimate.Nutrients.Kcal, 140)
}

func TestRequiredNutritionRejectsUnsafeUnitConversion(t *testing.T) {
	label := &StructuredNutrition{Basis: NutritionPer100ml, BasisQty: 100, BasisUnit: "ml", Kcal: floatPtr(800)}
	estimate, warnings := EstimateRequiredNutrition(Ingredient{Name: "oil", Quantity: 1, Unit: "tbsp"}, label)
	if estimate != nil || len(warnings) == 0 {
		t.Fatalf("estimate=%+v warnings=%+v, want unsafe conversion warning", estimate, warnings)
	}
}

func TestAttachSelectedProductNutrition(t *testing.T) {
	selection := SelectProduct(Ingredient{Name: "rice", Quantity: 200, Unit: "g"}, []alcampo.Product{nutritionRiceProduct()}, Profile{}, PolicyBalanced)
	if selection.ProductNutrition == nil || selection.RequiredNutrition == nil || len(selection.NutritionWarnings) != 0 {
		t.Fatalf("nutrition was not attached cleanly: %+v", selection)
	}
	assertFloatPtr(t, "required kcal", selection.RequiredNutrition.Nutrients.Kcal, 700)
}

func nutritionRiceProduct() alcampo.Product {
	available := true
	return alcampo.Product{
		ID:        "rice-product",
		SKU:       "rice-sku",
		Name:      "Rice 500 g",
		Price:     money.Money{Amount: "1.00", Currency: "EUR", Cents: 100},
		UnitPrice: money.Money{Amount: "2.00", Currency: "EUR", Cents: 200},
		Size:      "500 g",
		Available: &available,
		Nutrition: "Valores medios por 100 g Valor energético 350 kcal Proteínas 7 g",
	}
}

func assertFloatPtr(t *testing.T, name string, got *float64, want float64) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}
