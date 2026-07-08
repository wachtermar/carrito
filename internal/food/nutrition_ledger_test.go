package food

import (
	"context"
	"strings"
	"testing"

	"github.com/wachtermar/carrito/internal/money"
)

func TestNutritionLedgerCountsConsumedDeltaNotPurchasedPackage(t *testing.T) {
	run := nutritionLedgerRiceRun(250, 0, 500, true)
	policy := DefaultNutritionPolicy(NutritionModeHybrid)
	ledger := BuildNutritionLedger(context.Background(), &run, PantryProfile{}, policy)

	if ledger.Status != NutritionLedgerCompleteForPurchased {
		t.Fatalf("status = %s, want complete_for_purchased: %+v", ledger.Status, ledger)
	}
	assertNutrientValue(t, "consumed kcal", ledger.TotalConsumedKnown.Facts.EnergyKcal, 875)
	if ledger.PurchasedExcessKnown != nil {
		t.Fatalf("package excess should be excluded by default: %+v", ledger.PurchasedExcessKnown)
	}

	policy.IncludePurchasedExcess = true
	withExcess := BuildNutritionLedger(context.Background(), &run, PantryProfile{}, policy)
	if withExcess.PurchasedExcessKnown == nil {
		t.Fatalf("expected package excess nutrition")
	}
	assertNutrientValue(t, "excess kcal", withExcess.PurchasedExcessKnown.Facts.EnergyKcal, 875)
}

func TestNutritionLedgerPartialPantryCountsOnlyShopDelta(t *testing.T) {
	run := nutritionLedgerRiceRun(250, 150, 500, true)
	ledger := BuildNutritionLedger(context.Background(), &run, PantryProfile{}, DefaultNutritionPolicy(NutritionModeHybrid))

	if ledger.Status != NutritionLedgerPartial {
		t.Fatalf("status = %s, want partial: %+v", ledger.Status, ledger)
	}
	assertNutrientValue(t, "shop delta kcal", ledger.TotalConsumedKnown.Facts.EnergyKcal, 350)
	if ledger.Coverage.PantryMissingLines != 1 {
		t.Fatalf("pantry missing lines = %d, want 1", ledger.Coverage.PantryMissingLines)
	}
	if got := *ledger.Coverage.QuantityCoverageRatio; got != 0.4 {
		t.Fatalf("quantity coverage = %.3g, want 0.4", got)
	}
}

func TestNutritionLedgerPantryProfileNutritionCompletesCoverage(t *testing.T) {
	run := nutritionLedgerRiceRun(250, 150, 500, true)
	profile := PantryProfile{Items: []PantryProfileItem{{
		IngredientKey: "rice",
		Names:         []string{"rice"},
		Status:        PantryItemStatusConfirmedAvailable,
		Nutrition: &PantryNutritionEvidence{
			Basis:      NutritionEvidenceBasis{Type: NutritionPer100g, Confidence: "user"},
			Facts:      NutritionFacts{EnergyKcal: &NutrientAmount{Value: 360, Unit: "kcal", Source: "pantry_profile", Confidence: "user"}},
			Source:     "user",
			Confidence: "user",
		},
	}}}
	ledger := BuildNutritionLedger(context.Background(), &run, profile, DefaultNutritionPolicy(NutritionModeHybrid))

	if ledger.Status != NutritionLedgerCompleteHybrid {
		t.Fatalf("status = %s, want complete_hybrid: %+v", ledger.Status, ledger)
	}
	assertNutrientValue(t, "hybrid kcal", ledger.TotalConsumedKnown.Facts.EnergyKcal, 890)
	if got := *ledger.Coverage.QuantityCoverageRatio; got != 1 {
		t.Fatalf("quantity coverage = %.3g, want 1", got)
	}
}

func TestNutritionReadinessRequiredDoesNotBlockBasketOrCooking(t *testing.T) {
	run := nutritionLedgerRiceRun(250, 0, 500, false)
	policy := DefaultNutritionPolicy(NutritionModeHybrid)
	policy.RequireNutritionReady = true
	policy.MinLineCoverageRatio = 1
	ledger := BuildNutritionLedger(context.Background(), &run, PantryProfile{}, policy)
	run = AttachNutritionLedger(run, ledger)

	gate := ApplyReadinessGate(&run, ReadinessPolicy{StrictQuantity: true, RequireSafeBasket: true, RequireCookReady: true, RequireNutritionReady: true})
	if !gate.SafeToBuild || !gate.SafeToCook || gate.SafeToReportNutrition || gate.ExitCode != ReadinessExitBlocked || gate.ExitReason != "nutrition reporting is not ready" {
		t.Fatalf("unexpected gate: %+v", gate)
	}
	lines := ReadinessBasketLinesWithRecipeSwapOptimizationPantryServingAndNutrition(run.Shop.BasketLines, &gate, nil, nil, nil, nil, nil, run.NutritionLedger)
	text := strings.Join(lines, "\n")
	if !strings.Contains(text, "BASKET CAN BE BUILT, BUT NUTRITION REPORTING IS NOT READY") || !strings.Contains(text, "rice-sku 1 # rice") {
		t.Fatalf("basket text did not preserve safe SKU lines with nutrition warning:\n%s", text)
	}
}

func TestNutritionLedgerPDFLines(t *testing.T) {
	run := nutritionLedgerRiceRun(250, 0, 500, true)
	ledger := BuildNutritionLedger(context.Background(), &run, PantryProfile{}, DefaultNutritionPolicy(NutritionModeHybrid))
	run = AttachNutritionLedger(run, ledger)
	gate := ApplyReadinessGate(&run, ReadinessPolicy{StrictQuantity: true, RequireSafeBasket: true})
	run.ReadinessGate = &gate
	_, lines := pdfLinesForFoodRunArtifact(run)
	text := strings.Join(lines, "\n")
	for _, want := range []string{"Nutrition evidence:", "Known nutrition coverage: 1 of 1 ingredient lines", "875 kcal"} {
		if !strings.Contains(text, want) {
			t.Fatalf("PDF missing %q:\n%s", want, text)
		}
	}
}

func nutritionLedgerRiceRun(totalGrams, pantryGrams, purchasedGrams float64, withLabel bool) FoodRunArtifact {
	available := true
	shopGrams := totalGrams - pantryGrams
	req := IngredientRequirement{
		RequirementID:           "req-1",
		RecipeID:                "rice-bowl",
		RecipeTitle:             "Rice Bowl",
		Day:                     1,
		MealSlot:                "dinner",
		IngredientName:          "rice",
		RequiredQuantity:        nutritionTestQuantity(shopGrams, "g"),
		TotalRequiredQuantity:   nutritionTestQuantity(totalGrams, "g"),
		ShopRequiredQuantity:    nutritionTestQuantity(shopGrams, "g"),
		PantryAllocatedQuantity: nutritionTestQuantity(pantryGrams, "g"),
		Usages: []IngredientUsageRef{{
			Day:                1,
			MealSlot:           "dinner",
			RecipeID:           "rice-bowl",
			RecipeTitle:        "Rice Bowl",
			IngredientKey:      "rice",
			IngredientName:     "rice",
			CookedServingUnits: 2.5,
			RequiredQuantity:   nutritionTestQuantity(totalGrams, "g"),
		}},
	}
	allocation := IngredientProductAllocation{
		RequirementID:           "req-1",
		ProductID:               "rice-id",
		SKU:                     "rice-sku",
		ProductName:             "Rice 500 g",
		MatchType:               "exact",
		RequiredQuantity:        req.RequiredQuantity,
		TotalRequiredQuantity:   req.TotalRequiredQuantity,
		PantryAllocatedQuantity: req.PantryAllocatedQuantity,
		ShopRequiredQuantity:    req.ShopRequiredQuantity,
		PurchasedQuantity:       &QuantityRange{Expected: *nutritionTestQuantity(purchasedGrams, "g"), IsExact: true},
		ExcessQuantity:          nutritionTestQuantity(purchasedGrams-shopGrams, "g"),
		PackageCount:            1,
		CoverageRatio:           1,
		MatchConfidence:         0.95,
		QuantityConfidence:      0.95,
		OverallConfidence:       0.95,
		Badges:                  []string{"EXACT"},
	}
	product := ProductSummary{
		ID:              "rice-id",
		SKU:             "rice-sku",
		Name:            "Rice 500 g",
		Price:           money.Money{Amount: "1.00", Currency: "EUR", Cents: 100},
		Available:       &available,
		PackageQuantity: 500,
		PackageUnit:     "g",
	}
	if withLabel {
		product.Nutrition = "Valores medios por 100 g Valor energético 350 kcal Proteínas 7 g"
	}
	shop := ShopResult{
		Complete: true,
		SelectedProducts: []SelectedProduct{{
			Ingredient:       Ingredient{Name: "rice", Quantity: shopGrams, Unit: "g", SearchTerm: "arroz"},
			Product:          product,
			PurchaseQuantity: "1",
			PackageCount:     1,
			LineTotal:        product.Price,
		}},
		BasketLines: []string{"rice-sku 1 # rice"},
	}
	ledger := QuantityLedger{
		Status:                      LedgerCompleteExact,
		MealPlanFingerprint:         "meal-fp",
		ProductSelectionFingerprint: "product-fp",
		PantryResolutionFingerprint: "pantry-fp",
		ShopRequirementsFingerprint: "shop-req-fp",
		Requirements:                []IngredientRequirement{req},
		Allocations:                 []IngredientProductAllocation{allocation},
		Summary:                     LedgerSummary{IngredientCount: 1, CoveredIngredientCount: 1, ExactQuantityLines: 1, SafeToBuildBasket: true},
	}
	pantryResolution := PantryResolution{
		Status:                      PantryResolutionApplied,
		PantryResolutionFingerprint: "pantry-fp",
		ShopRequirementsFingerprint: "shop-req-fp",
		Lines: []PantryResolutionLine{{
			IngredientKey:   "rice",
			IngredientName:  "rice",
			TotalRequired:   req.TotalRequiredQuantity,
			PantryAllocated: req.PantryAllocatedQuantity,
			ShopRequired:    req.ShopRequiredQuantity,
			PantryDecision:  PantryDecisionPantryPartialConfirmed,
			PantryStatus:    PantryItemStatusConfirmedAvailable,
			PantryItemKey:   "rice",
		}},
	}
	return FoodRunArtifact{
		Kind:                        "food_run",
		MealPlanFingerprint:         "meal-fp",
		ProductSelectionFingerprint: "product-fp",
		PantryResolutionFingerprint: "pantry-fp",
		ShopRequirementsFingerprint: "shop-req-fp",
		MealPlan:                    MealPlan{People: 4, Days: []DayPlan{{Day: 1, Meals: []Meal{{Type: "dinner", Recipe: Recipe{ID: "rice-bowl", Title: "Rice Bowl", Servings: 4, Ingredients: []Ingredient{{Name: "rice", Quantity: totalGrams, Unit: "g"}}}}}}}},
		Shop:                        shop,
		PantryResolution:            &pantryResolution,
		QuantityLedger:              &ledger,
		BasketSafety:                &BasketSafety{Status: BasketSafetySafe, SafeToBuild: true},
	}
}

func nutritionTestQuantity(value float64, unit string) *NormalizedQuantity {
	if value <= 0 {
		return nil
	}
	q := quantityFromBase(value, unit, 0.95, "test")
	return &q
}

func assertNutrientValue(t *testing.T, name string, got *NutrientAmount, want float64) {
	t.Helper()
	if got == nil || got.Value != want {
		t.Fatalf("%s = %+v, want %.3g", name, got, want)
	}
}
