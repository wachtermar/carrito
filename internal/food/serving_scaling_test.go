package food

import (
	"strings"
	"testing"

	"github.com/wachtermar/carrito/internal/money"
)

func TestServingScalingExplicitServingsScalesIngredients(t *testing.T) {
	plan := servingTestPlan(4, 400)
	policy := DefaultServingPolicy(true)
	policy.DefaultServings = 2.5

	scaledPlan, servingPlan, scaled := ApplyServingScaling(plan, HouseholdProfile{}, policy)

	if servingPlan.Status != ServingPlanExplicitServings || servingPlan.Summary.SlotsScaled != 1 {
		t.Fatalf("serving plan = %+v", servingPlan)
	}
	slot := scaled.Slots[0]
	if slot.ScaleFactor != 0.625 {
		t.Fatalf("scale factor = %.3g, want 0.625", slot.ScaleFactor)
	}
	if got := scaledPlan.Days[0].Meals[0].Recipe.Ingredients[0].Quantity; got != 250 {
		t.Fatalf("scaled rice = %.3g, want 250", got)
	}
	if scaled.ScaledMealPlanFingerprint == "" || servingPlan.ServingPlanFingerprint == "" {
		t.Fatalf("missing fingerprints: serving=%s scaled=%s", servingPlan.ServingPlanFingerprint, scaled.ScaledMealPlanFingerprint)
	}
}

func TestServingScalingAdultToddlerShorthand(t *testing.T) {
	plan := servingTestPlan(4, 400)
	policy := DefaultServingPolicy(true)
	policy.AdultServings = 2
	policy.ToddlerServings = 0.5

	_, servingPlan, _ := ApplyServingScaling(plan, HouseholdProfile{}, policy)

	if servingPlan.Summary.TotalTargetServingUnits != 2.5 {
		t.Fatalf("target servings = %.3g, want 2.5", servingPlan.Summary.TotalTargetServingUnits)
	}
	if len(servingPlan.Slots[0].Participants) != 2 {
		t.Fatalf("participants = %+v", servingPlan.Slots[0].Participants)
	}
}

func TestServingScalingPartialPantryDeltaFeedsLedger(t *testing.T) {
	plan := servingTestPlan(4, 400)
	servingPolicy := DefaultServingPolicy(true)
	servingPolicy.DefaultServings = 2.5
	scaledPlan, servingPlan, scaled := ApplyServingScaling(plan, HouseholdProfile{}, servingPolicy)
	pantryProfile := PantryProfile{Items: []PantryProfileItem{{
		IngredientKey: "arroz",
		Names:         []string{"arroz"},
		Status:        PantryItemStatusConfirmedAvailable,
		Quantity:      &NormalizedQuantity{Value: 150, Unit: "g", BaseValue: 150, BaseUnit: "g", Confidence: 1},
	}}}
	pantryPolicy := DefaultPantryPolicy(true, DefaultPantryNone)
	shopPlan, resolution := ApplyPantryResolution(scaledPlan, pantryProfile, pantryPolicy)
	StampPantryResolutionServing(&resolution, servingPlan.ServingPlanFingerprint, scaled.ScaledMealPlanFingerprint)

	line := pantryLine(resolution, "arroz")
	if line.ShopRequired == nil || line.ShopRequired.BaseValue != 100 {
		t.Fatalf("pantry delta = %+v, want 100g", line)
	}
	product := ProductSummary{ID: "rice-product", SKU: "rice-sku", Name: "Arroz redondo 500 g", Price: money.Money{Amount: "1.20", Currency: "EUR", Cents: 120}, PackageQuantity: 500, PackageUnit: "g"}
	count, total, reason, warnings := EstimatePurchaseQuantity(shopPlan.RequiredPurchases[0], product)
	shop := ShopResult{SelectedProducts: []SelectedProduct{{
		Ingredient:       shopPlan.RequiredPurchases[0],
		Product:          product,
		PurchaseQuantity: formatFloat(float64(count)),
		PackageCount:     count,
		LineTotal:        total,
		QuantityReason:   reason,
		Warnings:         warnings,
	}}}
	ledger := BuildQuantityLedger(shopPlan, shop, QuantityLedgerOptions{ServingPlanFingerprint: servingPlan.ServingPlanFingerprint, ScaledMealPlanFingerprint: scaled.ScaledMealPlanFingerprint, PantryResolution: &resolution, PantryResolutionFingerprint: resolution.PantryResolutionFingerprint, ShopRequirementsFingerprint: resolution.ShopRequirementsFingerprint})

	req := ledger.Requirements[0]
	assertNear(t, "ledger total required", req.TotalRequiredQuantity.BaseValue, 250)
	assertNear(t, "ledger pantry allocated", req.PantryAllocatedQuantity.BaseValue, 150)
	assertNear(t, "ledger shop required", req.ShopRequiredQuantity.BaseValue, 100)
	if ledger.ServingPlanFingerprint != servingPlan.ServingPlanFingerprint || ledger.ScaledMealPlanFingerprint != scaled.ScaledMealPlanFingerprint {
		t.Fatalf("ledger serving fingerprints missing: %+v", ledger)
	}
}

func TestServingScalingStrictMissingBaseBlocksCookReadiness(t *testing.T) {
	plan := servingTestPlan(0, 400)
	plan.People = 0
	policy := DefaultServingPolicy(true)
	policy.StrictServingScaling = true
	policy.RequireBaseRecipeServings = true
	policy.DefaultServings = 2
	scaledPlan, servingPlan, scaled := ApplyServingScaling(plan, HouseholdProfile{}, policy)
	run := readyExactRun()
	run.MealPlan = scaledPlan
	run = AttachServingScaling(run, HouseholdProfile{}, servingPlan, scaled)

	gate := ApplyReadinessGate(&run, ReadinessPolicy{StrictQuantity: true, RequireSafeBasket: true, RequireCookReady: true})

	if !gate.SafeToBuild || gate.SafeToCook || gate.ExitReason != "meal plan is not cook-ready" || !hasGateIssue(gate, "base_servings_missing") {
		t.Fatalf("gate = %+v, want buildable basket but blocked cooking", gate)
	}
}

func TestServingBasketAndPDFLines(t *testing.T) {
	plan := servingTestPlan(4, 400)
	policy := DefaultServingPolicy(true)
	policy.DefaultServings = 2.5
	scaledPlan, servingPlan, scaled := ApplyServingScaling(plan, HouseholdProfile{}, policy)
	run := readyExactRun()
	run.MealPlan = scaledPlan
	run = AttachServingScaling(run, HouseholdProfile{}, servingPlan, scaled)
	gate := ApplyReadinessGate(&run, ReadinessPolicy{StrictQuantity: true, RequireSafeBasket: true})
	lines := ReadinessBasketLinesWithRecipeSwapOptimizationPantryAndServing([]string{"rice-sku 1 # rice"}, &gate, nil, nil, nil, run.ServingPlan, run.ScaledMealPlan)
	if !strings.Contains(strings.Join(lines, "\n"), "Serving plan: explicit_servings") {
		t.Fatalf("basket missing serving summary: %+v", lines)
	}
	run.ReadinessGate = &gate
	_, pdfLines := pdfLinesForFoodRunArtifact(run)
	if !strings.Contains(strings.Join(pdfLines, "\n"), "Servings and scaling:") {
		t.Fatalf("PDF missing serving section:\n%s", strings.Join(pdfLines, "\n"))
	}
}

func servingTestPlan(servings int, riceGrams float64) MealPlan {
	return MealPlan{
		ID:     "serving-plan",
		People: servings,
		Days: []DayPlan{{Day: 1, Meals: []Meal{{Type: "dinner", Recipe: Recipe{
			ID:       "rice-bowl",
			Title:    "Rice Bowl",
			Servings: servings,
			Ingredients: []Ingredient{
				{Name: "rice", Quantity: riceGrams, Unit: "g", Category: "pantry", SearchTerm: "arroz"},
			},
			Steps: []RecipeStep{{Number: 1, Text: "Cook rice."}},
		}}}}},
		RequiredPurchases: []Ingredient{{Name: "rice", Quantity: riceGrams, Unit: "g", Category: "pantry", SearchTerm: "arroz"}},
	}
}
