package food

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/money"
)

func TestParsePackageEvidenceSpanishGroceryFixtures(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		baseValue   float64
		baseUnit    string
		exact       bool
		variable    bool
		drainedBase float64
		packCount   int
		noNet       bool
	}{
		{name: "chicken", text: "Pechuga de pollo 600 g", baseValue: 600, baseUnit: "g", exact: true},
		{name: "yogurt multipack", text: "Yogur natural 4 x 125 g", baseValue: 500, baseUnit: "g", exact: true, packCount: 4},
		{name: "tuna drained", text: "Atún claro 3x80g peso escurrido 52g", baseValue: 240, baseUnit: "g", exact: true, drainedBase: 156, packCount: 3},
		{name: "oil", text: "Aceite de oliva virgen extra 1 l", baseValue: 1000, baseUnit: "ml", exact: true},
		{name: "milk decimal comma", text: "Leche semidesnatada 1,5 l", baseValue: 1500, baseUnit: "ml", exact: true},
		{name: "eggs", text: "Huevos camperos M 12 ud", baseValue: 12, baseUnit: "unit", exact: true, packCount: 12},
		{name: "approx beef", text: "Filetes de ternera peso aprox. 500 g", baseValue: 500, baseUnit: "g", exact: false},
		{name: "loose bananas", text: "Plátano granel kg", variable: true, noNet: true},
		{name: "fish by weight", text: "Salmón fresco al peso", variable: true, noNet: true},
		{name: "pack spaced", text: "Pack 2 x 250 g", baseValue: 500, baseUnit: "g", exact: true, packCount: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParsePackageEvidence(tt.text)
			if got.VariableWeight != tt.variable {
				t.Fatalf("VariableWeight = %t, want %t: %+v", got.VariableWeight, tt.variable, got)
			}
			if tt.noNet {
				if got.NetQuantity != nil {
					t.Fatalf("NetQuantity = %+v, want nil", got.NetQuantity)
				}
				return
			}
			if got.NetQuantity == nil {
				t.Fatalf("missing net quantity: %+v", got)
			}
			assertNear(t, "base value", got.NetQuantity.Expected.BaseValue, tt.baseValue)
			if got.NetQuantity.Expected.BaseUnit != tt.baseUnit {
				t.Fatalf("base unit = %q, want %q", got.NetQuantity.Expected.BaseUnit, tt.baseUnit)
			}
			if got.NetQuantity.IsExact != tt.exact {
				t.Fatalf("IsExact = %t, want %t: %+v", got.NetQuantity.IsExact, tt.exact, got)
			}
			if tt.packCount > 0 {
				if got.PackCount == nil || *got.PackCount != tt.packCount {
					t.Fatalf("PackCount = %+v, want %d", got.PackCount, tt.packCount)
				}
			}
			if tt.drainedBase > 0 {
				if got.DrainedQuantity == nil {
					t.Fatalf("missing drained quantity: %+v", got)
				}
				assertNear(t, "drained base", got.DrainedQuantity.Expected.BaseValue, tt.drainedBase)
			}
		})
	}
}

func TestParsePackageEvidencePrefersVisibleLiquidVolumeOverConflictingSize(t *testing.T) {
	got := ParsePackageEvidence("250g", "KIKKOMAN Salsa de soja bajo en sal frasco de 250 ml.")
	if got.NetQuantity == nil {
		t.Fatalf("missing net quantity: %+v", got)
	}
	assertNear(t, "base value", got.NetQuantity.Expected.BaseValue, 250)
	if got.NetQuantity.Expected.BaseUnit != "ml" {
		t.Fatalf("base unit = %q, want ml: %+v", got.NetQuantity.Expected.BaseUnit, got)
	}
}

func TestQuantityLedgerAggregatesSameProductAcrossIngredients(t *testing.T) {
	plan := MealPlan{RequiredPurchases: []Ingredient{
		{Name: "rice", Quantity: 300, Unit: "g"},
		{Name: "rice", Quantity: 300, Unit: "g"},
	}}
	shop := ShopResult{SelectedProducts: []SelectedProduct{selectedProduct("rice", ProductSummary{
		ID:              "rice-product",
		SKU:             "rice-sku",
		Name:            "Arroz redondo 1 kg",
		Price:           money.Money{Amount: "2.00", Currency: "EUR", Cents: 200},
		PackageQuantity: 1,
		PackageUnit:     "kg",
	})}}

	ledger := BuildQuantityLedger(plan, shop, QuantityLedgerOptions{})

	if ledger.Status != LedgerCompleteExact {
		t.Fatalf("status = %s, want complete_exact: %+v", ledger.Status, ledger)
	}
	if len(ledger.Totals.Lines) != 1 || ledger.Totals.Lines[0].PackageCount != 1 {
		t.Fatalf("expected one aggregated basket line buying one package: %+v", ledger.Totals.Lines)
	}
	if len(ledger.Allocations) != 2 {
		t.Fatalf("allocations = %d, want 2", len(ledger.Allocations))
	}
	for _, allocation := range ledger.Allocations {
		if allocation.ExcessQuantity == nil {
			t.Fatalf("allocation missing excess quantity: %+v", allocation)
		}
		assertNear(t, "rice excess", allocation.ExcessQuantity.BaseValue, 400)
	}
}

func TestQuantityLedgerAnnotatesPantryDeltaByIngredientNameAlias(t *testing.T) {
	plan := MealPlan{
		Days: []DayPlan{{Day: 1, Meals: []Meal{{Type: "dinner", Recipe: Recipe{
			ID:       "rice-bowl",
			Title:    "Rice Bowl",
			Servings: 2,
			Ingredients: []Ingredient{{
				Name:       "rice",
				Quantity:   300,
				Unit:       "g",
				SearchTerm: "arroz",
			}},
			Steps: []RecipeStep{{Number: 1, Text: "Cook rice."}},
		}}}}},
		RequiredPurchases: []Ingredient{{Name: "rice", Quantity: 300, Unit: "g", SearchTerm: "arroz"}},
	}
	profile := PantryProfile{Items: []PantryProfileItem{{
		IngredientKey: "arroz",
		Names:         []string{"arroz"},
		Status:        PantryItemStatusConfirmedAvailable,
		Quantity:      &NormalizedQuantity{Value: 100, Unit: "g", BaseValue: 100, BaseUnit: "g", Confidence: 0.95},
	}}}
	policy := DefaultPantryPolicy(true, DefaultPantryNone)
	updated, resolution := ApplyPantryResolution(plan, profile, policy)
	if len(updated.RequiredPurchases) != 1 || updated.RequiredPurchases[0].Quantity != 200 {
		t.Fatalf("shopping delta = %+v, want 200g rice", updated.RequiredPurchases)
	}

	product := ProductSummary{
		ID:              "rice-product",
		SKU:             "rice-sku",
		Name:            "Arroz redondo 500 g",
		Price:           money.Money{Amount: "1.20", Currency: "EUR", Cents: 120},
		PackageQuantity: 500,
		PackageUnit:     "g",
	}
	count, total, reason, warnings := EstimatePurchaseQuantity(updated.RequiredPurchases[0], product)
	shop := ShopResult{SelectedProducts: []SelectedProduct{{
		Ingredient:       updated.RequiredPurchases[0],
		Product:          product,
		PurchaseQuantity: formatFloat(float64(count)),
		PackageCount:     count,
		LineTotal:        total,
		QuantityReason:   reason,
		Warnings:         warnings,
	}}}

	ledger := BuildQuantityLedger(updated, shop, QuantityLedgerOptions{PantryResolution: &resolution})
	if len(ledger.Requirements) != 1 {
		t.Fatalf("requirements = %+v", ledger.Requirements)
	}
	req := ledger.Requirements[0]
	if req.PantryDecision != PantryDecisionPantryPartialConfirmed || req.SourcingStatus != "pantry_partial_shop_delta" {
		t.Fatalf("requirement pantry annotation missing: %+v", req)
	}
	assertNear(t, "total required", req.TotalRequiredQuantity.BaseValue, 300)
	assertNear(t, "pantry allocated", req.PantryAllocatedQuantity.BaseValue, 100)
	assertNear(t, "shop required", req.ShopRequiredQuantity.BaseValue, 200)
}

func TestQuantityLedgerAllocationScenarios(t *testing.T) {
	tests := []struct {
		name       string
		ingredient Ingredient
		product    ProductSummary
		wantStatus LedgerStatus
		wantCount  int
		wantExcess float64
		wantBadge  string
	}{
		{
			name:       "chicken exact packs",
			ingredient: Ingredient{Name: "chicken", Quantity: 750, Unit: "g"},
			product:    ProductSummary{ID: "chicken", Name: "Pechuga de pollo 600 g", Price: money.Money{Amount: "3.00", Currency: "EUR", Cents: 300}},
			wantStatus: LedgerCompleteExact,
			wantCount:  2,
			wantExcess: 450,
			wantBadge:  "EXACT",
		},
		{
			name:       "potatoes kg bags",
			ingredient: Ingredient{Name: "potatoes", Quantity: 1.2, Unit: "kg"},
			product:    ProductSummary{ID: "potatoes", Name: "Patatas bolsa 1 kg", Price: money.Money{Amount: "1.00", Currency: "EUR", Cents: 100}},
			wantStatus: LedgerCompleteExact,
			wantCount:  2,
			wantExcess: 800,
			wantBadge:  "EXACT",
		},
		{
			name:       "garlic cloves need review",
			ingredient: Ingredient{Name: "garlic", Quantity: 2, Unit: "clove"},
			product:    ProductSummary{ID: "garlic", Name: "Ajo cabeza 100 g", Price: money.Money{Amount: "0.80", Currency: "EUR", Cents: 80}},
			wantStatus: LedgerNeedsReview,
			wantCount:  1,
			wantBadge:  "LOW QUANTITY CONFIDENCE",
		},
		{
			name:       "fish approximate estimated",
			ingredient: Ingredient{Name: "fish", Quantity: 400, Unit: "g"},
			product:    ProductSummary{ID: "fish", Name: "Salmón fresco peso aprox. 400 g", Price: money.Money{Amount: "6.00", Currency: "EUR", Cents: 600}},
			wantStatus: LedgerCompleteEstimated,
			wantCount:  1,
			wantBadge:  "ESTIMATED WEIGHT",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := MealPlan{RequiredPurchases: []Ingredient{tt.ingredient}}
			shop := ShopResult{SelectedProducts: []SelectedProduct{selectedProduct(tt.ingredient.Name, tt.product)}}
			ledger := BuildQuantityLedger(plan, shop, QuantityLedgerOptions{})
			if ledger.Status != tt.wantStatus {
				t.Fatalf("status = %s, want %s: %+v", ledger.Status, tt.wantStatus, ledger)
			}
			if len(ledger.Allocations) != 1 {
				t.Fatalf("allocations = %d, want 1", len(ledger.Allocations))
			}
			allocation := ledger.Allocations[0]
			if tt.wantCount > 0 && allocation.PackageCount != tt.wantCount {
				t.Fatalf("package count = %d, want %d: %+v", allocation.PackageCount, tt.wantCount, allocation)
			}
			if tt.wantCount > 0 && allocation.PurchasedQuantity == nil {
				t.Fatalf("purchased quantity missing for package count fallback: %+v", allocation)
			}
			if tt.wantExcess > 0 {
				if allocation.ExcessQuantity == nil {
					t.Fatalf("missing excess quantity: %+v", allocation)
				}
				assertNear(t, "excess", allocation.ExcessQuantity.BaseValue, tt.wantExcess)
			}
			if !containsString(allocation.Badges, tt.wantBadge) {
				t.Fatalf("badges = %+v, want %q", allocation.Badges, tt.wantBadge)
			}
		})
	}
}

func TestQuantityLedgerMissingProductIsIncomplete(t *testing.T) {
	plan := MealPlan{RequiredPurchases: []Ingredient{{Name: "beef strips", Quantity: 500, Unit: "g"}}}
	shop := ShopResult{SelectedProducts: []SelectedProduct{{Ingredient: plan.RequiredPurchases[0], Error: "no products found"}}}
	ledger := BuildQuantityLedger(plan, shop, QuantityLedgerOptions{})
	if ledger.Status != LedgerIncomplete || ledger.Summary.MissingLines != 1 {
		t.Fatalf("ledger should be incomplete: %+v", ledger)
	}
	safety := BasketSafetyFromLedger(ledger, QuantityLedgerOptions{})
	if safety.SafeToBuild || safety.Status != BasketSafetyUnsafe {
		t.Fatalf("incomplete ledger should not be basket-safe: %+v", safety)
	}
}

func TestQuantityLedgerNutritionSeparatesRequiredAndPurchasedCoverage(t *testing.T) {
	label := &StructuredNutrition{Basis: NutritionPer100g, BasisQty: 100, BasisUnit: "g", Kcal: floatPtr(120), ProteinG: floatPtr(10), Source: "test", Confidence: 0.9}
	plan := MealPlan{RequiredPurchases: []Ingredient{
		{Name: "fish", Quantity: 250, Unit: "g"},
		{Name: "garlic", Quantity: 2, Unit: "clove"},
		{Name: "herbs", Quantity: 5, Unit: "g"},
	}}
	shop := ShopResult{SelectedProducts: []SelectedProduct{
		selectedProductWithNutrition("fish", ProductSummary{ID: "fish", Name: "Fish 500 g", Price: money.Money{Amount: "4.00", Currency: "EUR", Cents: 400}}, label),
		selectedProductWithNutrition("garlic", ProductSummary{ID: "garlic", Name: "Ajo 100 g", Price: money.Money{Amount: "0.80", Currency: "EUR", Cents: 80}}, label),
		selectedProduct("herbs", ProductSummary{ID: "herbs", Name: "Hierbas 10 g", Price: money.Money{Amount: "1.00", Currency: "EUR", Cents: 100}}),
	}}

	ledger := BuildQuantityLedger(plan, shop, QuantityLedgerOptions{})

	if ledger.Nutrition == nil {
		t.Fatal("missing nutrition report")
	}
	assertFloatPtrValue(t, "required kcal", ledger.Nutrition.RequiredEstimate.Nutrients.Kcal, 300)
	assertFloatPtrValue(t, "purchased kcal", ledger.Nutrition.PurchasedEstimate.Nutrients.Kcal, 600)
	if ledger.Nutrition.Coverage.RequirementsWithLabelNutrition != 1 || ledger.Nutrition.Coverage.RequirementsTotal != 3 {
		t.Fatalf("unexpected nutrition coverage: %+v", ledger.Nutrition.Coverage)
	}
	if ledger.Nutrition.Coverage.CalorieCoverageRatio >= 1 {
		t.Fatalf("coverage should be partial: %+v", ledger.Nutrition.Coverage)
	}
	if len(ledger.Nutrition.Coverage.MissingNutritionLabels) == 0 || len(ledger.Nutrition.Coverage.SkippedNutritionReasons) == 0 {
		t.Fatalf("missing skipped nutrition metadata: %+v", ledger.Nutrition.Coverage)
	}
}

func TestOfferEvidenceClassifiesConservatively(t *testing.T) {
	tests := []struct {
		name         string
		offer        string
		packageCount int
		want         string
	}{
		{name: "3x2 applied", offer: "3x2", packageCount: 3, want: "applied_confirmed"},
		{name: "3x2 threshold", offer: "3x2", packageCount: 2, want: "quantity_threshold_not_met"},
		{name: "second unit applied", offer: "Segunda unidad -50%", packageCount: 2, want: "applied_confirmed"},
		{name: "second unit not applied", offer: "Segunda unidad -50%", packageCount: 1, want: "available_not_applied"},
		{name: "loyalty", offer: "Precio exclusivo Tarjeta Mi Alcampo", packageCount: 1, want: "requires_loyalty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := productEvidenceFromSelection(SelectedProduct{
				Product:      ProductSummary{Name: "Product 1 kg", Offers: []string{tt.offer}},
				PackageCount: tt.packageCount,
			})
			if len(got.Offers) != 1 {
				t.Fatalf("offers = %+v", got.Offers)
			}
			if got.Offers[0].Status != tt.want {
				t.Fatalf("status = %q, want %q: %+v", got.Offers[0].Status, tt.want, got.Offers[0])
			}
		})
	}
}

func TestEnrichShopResultProductsUsesDetailNutritionAndPackage(t *testing.T) {
	available := true
	fetcher := &fakeProductDetailFetcher{product: alcampo.Product{
		SKU:       "111",
		Name:      "Arroz redondo detalle 500 g",
		Size:      "500 g",
		Price:     money.Money{Amount: "1.20", Currency: "EUR", Cents: 120},
		Available: &available,
		Nutrition: "Valores medios por 100 g Valor energético 350 kcal Proteínas 7 g",
	}}
	shop := ShopResult{SelectedProducts: []SelectedProduct{{
		Ingredient: Ingredient{Name: "rice", Quantity: 250, Unit: "g"},
		Product:    ProductSummary{SKU: "111", Name: "Arroz redondo", Price: money.Money{Amount: "1.20", Currency: "EUR", Cents: 120}},
	}}}

	enriched := EnrichShopResultProducts(context.Background(), shop, fetcher, QuantityLedgerOptions{EnrichProducts: true})

	if fetcher.calls != 1 {
		t.Fatalf("detail calls = %d, want 1", fetcher.calls)
	}
	selected := enriched.SelectedProducts[0]
	if selected.Product.PackageQuantity != 500 || selected.Product.PackageUnit != "g" {
		t.Fatalf("package quantity was not enriched: %+v", selected.Product)
	}
	if selected.ProductNutrition == nil || selected.RequiredNutrition == nil || len(enriched.ProductNutritionReports) != 1 {
		t.Fatalf("nutrition was not enriched: selected=%+v reports=%+v", selected, enriched.ProductNutritionReports)
	}
	if selected.PackageCount != 1 || selected.LineTotal.Cents != 120 {
		t.Fatalf("quantity math not recomputed after detail: %+v", selected)
	}
}

func TestQuantityLedgerArtifactInvariantsAndPDFTrustText(t *testing.T) {
	plan := MealPlan{
		People: 2,
		Days: []DayPlan{{Day: 1, Meals: []Meal{{Type: "dinner", Recipe: Recipe{
			ID: "fish-dinner", Title: "Fish Dinner", Servings: 2,
			Ingredients: []Ingredient{{Name: "fish", Quantity: 400, Unit: "g"}},
			Steps:       []RecipeStep{{Number: 1, Text: "Cook fish."}},
		}}}}},
		RequiredPurchases: []Ingredient{{Name: "fish", Quantity: 400, Unit: "g"}},
	}
	shop := ShopResult{SelectedProducts: []SelectedProduct{
		selectedProduct("fish", ProductSummary{ID: "fish-product", SKU: "fish-sku", Name: "Salmón fresco peso aprox. 400 g", Price: money.Money{Amount: "6.00", Currency: "EUR", Cents: 600}}),
	}}
	ledger := BuildQuantityLedger(plan, shop, QuantityLedgerOptions{})
	safety := BasketSafetyFromLedger(ledger, QuantityLedgerOptions{})
	artifact := NewFoodRunArtifact(plan, shop, "", "basket.txt", []string{"dinner"})
	artifact.QuantityLedger = &ledger
	artifact.BasketSafety = &safety

	if len(ledger.ProductEvidence) != len(shop.SelectedProducts) {
		t.Fatalf("every basket line should have product evidence: evidence=%d selected=%d", len(ledger.ProductEvidence), len(shop.SelectedProducts))
	}
	if ledger.Nutrition == nil || ledger.Nutrition.Coverage.RequirementsTotal == 0 {
		t.Fatalf("nutrition coverage metadata missing: %+v", ledger.Nutrition)
	}
	if safety.SafeToBuild {
		t.Fatalf("estimated variable-weight ledger should be review-only by default: %+v", safety)
	}
	_, lines := pdfLinesForFoodRunArtifact(artifact)
	text := strings.Join(lines, "\n")
	for _, want := range []string{"Shopping confidence:", "Ledger status: COMPLETE_ESTIMATED", "Basket safety: REVIEW_ONLY", "Quantity ledger:", "ESTIMATED WEIGHT"} {
		if !strings.Contains(text, want) {
			t.Fatalf("PDF lines missing %q:\n%s", want, text)
		}
	}
}

func selectedProduct(ingredient string, product ProductSummary) SelectedProduct {
	count, lineTotal, reason, warnings := EstimatePurchaseQuantity(Ingredient{Name: ingredient, Quantity: 1, Unit: "unit"}, product)
	return SelectedProduct{
		Ingredient:       Ingredient{Name: ingredient},
		Product:          product,
		PurchaseQuantity: formatFloat(float64(count)),
		PackageCount:     count,
		LineTotal:        lineTotal,
		QuantityReason:   reason,
		Warnings:         warnings,
		SelectionReason:  "strong ingredient match",
	}
}

func selectedProductWithNutrition(ingredient string, product ProductSummary, label *StructuredNutrition) SelectedProduct {
	selected := selectedProduct(ingredient, product)
	selected.ProductNutrition = label
	selected.Product.NutritionParsed = label
	return selected
}

func assertNear(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.001 {
		t.Fatalf("%s = %.6g, want %.6g", label, got, want)
	}
}

func assertFloatPtrValue(t *testing.T, label string, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil, want %.6g", label, want)
	}
	assertNear(t, label, *got, want)
}

type fakeProductDetailFetcher struct {
	product alcampo.Product
	err     error
	calls   int
}

func (f *fakeProductDetailFetcher) Product(context.Context, string) (alcampo.Product, error) {
	f.calls++
	return f.product, f.err
}
