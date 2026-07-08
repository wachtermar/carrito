package food

import (
	"strings"
	"testing"

	"github.com/wachtermar/carrito/internal/money"
)

func TestEvaluateReadinessGateReadyExact(t *testing.T) {
	run := readyExactRun()
	gate := ApplyReadinessGate(&run, ReadinessPolicy{StrictQuantity: true, RequireSafeBasket: true})
	if gate.Status != ReadinessReadyExact || !gate.SafeToBuild || gate.ExitCode != ReadinessExitOK {
		t.Fatalf("gate = %+v, want ready exact", gate)
	}
	if run.BasketSafety == nil || !run.BasketSafety.SafeToBuild {
		t.Fatalf("basket safety not synced: %+v", run.BasketSafety)
	}
}

func TestEvaluateReadinessGateCanBuildBasketButBlockCooking(t *testing.T) {
	run := readyExactRun()
	run.PantryResolution = &PantryResolution{
		Status:                      PantryResolutionBlocked,
		PantryResolutionFingerprint: "pantry-fp",
		ShopRequirementsFingerprint: "shop-fp",
		BlockingIssues: []PantryIssue{{
			Code:           PantryDecisionBlockedUnconfirmed,
			IngredientKey:  "salt",
			IngredientName: "salt",
			Message:        "Pantry item is assumed but confirmed pantry is required.",
			Remediation:    "Confirm salt or shop all ingredients.",
		}},
	}

	gate := ApplyReadinessGate(&run, ReadinessPolicy{StrictQuantity: true, RequireSafeBasket: true, RequireCookReady: true})

	if gate.Status != ReadinessBlocked || !gate.SafeToBuild || gate.SafeToCook || gate.ExitCode != ReadinessExitBlocked {
		t.Fatalf("gate = %+v, want buildable basket with blocked cooking", gate)
	}
	if gate.ExitReason != "meal plan is not cook-ready" {
		t.Fatalf("exit reason = %q", gate.ExitReason)
	}
	lines := ReadinessBasketLines([]string{"rice-sku 1 # rice"}, &gate)
	text := strings.Join(lines, "\n")
	if !strings.Contains(text, "# BASKET CAN BE BUILT, BUT MEAL PLAN IS NOT COOK-READY") || !strings.Contains(text, "\nrice-sku 1 # rice") {
		t.Fatalf("basket lines should keep usable basket with cook warning:\n%s", text)
	}
}

func TestEvaluateReadinessGateBlocksIncompleteLedger(t *testing.T) {
	run := readyExactRun()
	run.QuantityLedger.Status = LedgerIncomplete
	run.QuantityLedger.Allocations[0].MatchType = "missing"
	run.QuantityLedger.Allocations[0].ProductName = ""
	run.BasketSafety.SafeToBuild = false
	gate := ApplyReadinessGate(&run, ReadinessPolicy{StrictQuantity: true, RequireSafeBasket: true})
	if gate.Status != ReadinessBlocked || gate.SafeToBuild || gate.ExitCode != ReadinessExitBlocked {
		t.Fatalf("gate = %+v, want blocked", gate)
	}
	if len(gate.BlockingIssues) == 0 {
		t.Fatalf("expected blocking issues: %+v", gate)
	}
}

func TestEvaluateReadinessGateEstimatedVariableWeightAllowedAsCaveat(t *testing.T) {
	run := readyExactRun()
	run.QuantityLedger.Status = LedgerCompleteEstimated
	run.QuantityLedger.Allocations[0].Badges = []string{"ESTIMATED WEIGHT"}
	run.BasketSafety.SafeToBuild = true
	gate := ApplyReadinessGate(&run, ReadinessPolicy{StrictQuantity: true, RequireSafeBasket: true, AllowEstimatedVariableWeight: true})
	if gate.Status != ReadinessReadyWithCaveats || !gate.SafeToBuild || len(gate.Warnings) == 0 {
		t.Fatalf("gate = %+v, want ready_with_caveats", gate)
	}
}

func TestEvaluateReadinessGateEstimatedVariableWeightBlockedByDefault(t *testing.T) {
	run := readyExactRun()
	run.QuantityLedger.Status = LedgerCompleteEstimated
	run.QuantityLedger.Allocations[0].Badges = []string{"ESTIMATED WEIGHT"}
	run.BasketSafety.SafeToBuild = true
	gate := ApplyReadinessGate(&run, ReadinessPolicy{StrictQuantity: true, RequireSafeBasket: true})
	if gate.Status != ReadinessBlocked || gate.SafeToBuild {
		t.Fatalf("gate = %+v, want blocked", gate)
	}
}

func TestEvaluateReadinessGateRequiresRecipeAdjustmentForPrepTransform(t *testing.T) {
	run := readyExactRun()
	run.RecoveryPlan = &RecoveryPlan{
		Status: RecoveryStatusRecovered,
		AppliedDecisions: []RecoveryDecision{{
			ID:                 "decision-1",
			IssueID:            "issue-1",
			OriginalIngredient: "beef strips",
			RecipeChanges:      []RecipeChange{{ChangeType: "prep_note", NewText: "Cut into strips."}},
		}},
	}
	gate := ApplyReadinessGate(&run, ReadinessPolicy{StrictQuantity: true, RequireSafeBasket: true})
	if gate.Status != ReadinessBlocked || !hasGateIssue(gate, "recipe_adjustment_missing") {
		t.Fatalf("gate = %+v, want missing recipe adjustment block", gate)
	}
	run.RecipeAdjustments = []RecipeAdjustment{{ID: "adjustment-1", Type: "prep_transform", RecoveryDecisionID: "decision-1", OriginalIngredientName: "beef strips", Instruction: "Cut into strips."}}
	run.BasketSafety = &BasketSafety{Status: BasketSafetySafe, SafeToBuild: true}
	gate = ApplyReadinessGate(&run, ReadinessPolicy{StrictQuantity: true, RequireSafeBasket: true})
	if gate.Status != ReadinessReadyExact || !gate.SafeToBuild {
		t.Fatalf("gate = %+v, want ready after adjustment", gate)
	}
}

func TestReadinessBasketLinesSafeAndUnsafe(t *testing.T) {
	safe := ReadinessGate{Status: ReadinessReadyExact, SafeToBuild: true}
	safeLines := ReadinessBasketLines([]string{"sku 1 # rice"}, &safe)
	if !strings.HasPrefix(strings.Join(safeLines, "\n"), "# Basket readiness: ready_exact") || safeLines[len(safeLines)-1] != "sku 1 # rice" {
		t.Fatalf("safe basket lines = %+v", safeLines)
	}
	blocked := ReadinessGate{Status: ReadinessBlocked, SafeToBuild: false, BlockingIssues: []GateIssue{{Message: "missing rice"}}}
	blockedLines := ReadinessBasketLines([]string{"sku 1 # rice"}, &blocked)
	text := strings.Join(blockedLines, "\n")
	if !strings.Contains(text, "# NOT SAFE TO BUILD BASKET") || strings.Contains(text, "\nsku 1 # rice") {
		t.Fatalf("blocked basket should be diagnostic comments only: %+v", blockedLines)
	}
}

func TestReadinessPDFLines(t *testing.T) {
	run := readyExactRun()
	blocked := ReadinessGate{Status: ReadinessBlocked, SafeToBuild: false, ExitReason: "basket is not safe", BlockingIssues: []GateIssue{{Message: "missing rice", Remediation: "choose product"}}}
	run.ReadinessGate = &blocked
	_, lines := pdfLinesForFoodRunArtifact(run)
	text := strings.Join(lines, "\n")
	for _, want := range []string{"Basket readiness: BLOCKED", "Do not use this basket for shopping.", "Shopping not ready:", "missing rice"} {
		if !strings.Contains(text, want) {
			t.Fatalf("PDF text missing %q:\n%s", want, text)
		}
	}
}

func readyExactRun() FoodRunArtifact {
	available := true
	req := IngredientRequirement{
		RequirementID:    "req-1",
		IngredientName:   "rice",
		RequiredQuantity: &NormalizedQuantity{Value: 100, Unit: "g", BaseValue: 100, BaseUnit: "g", Confidence: 0.95},
	}
	allocation := IngredientProductAllocation{
		RequirementID:      "req-1",
		ProductID:          "rice-id",
		SKU:                "rice-sku",
		ProductName:        "Rice 1 kg",
		MatchType:          "exact",
		RequiredQuantity:   req.RequiredQuantity,
		PurchasedQuantity:  &QuantityRange{Expected: NormalizedQuantity{Value: 1, Unit: "kg", BaseValue: 1000, BaseUnit: "g", Confidence: 0.95}, IsExact: true},
		PackageCount:       1,
		CoverageRatio:      1,
		MatchConfidence:    0.9,
		QuantityConfidence: 0.9,
		OverallConfidence:  0.9,
		Badges:             []string{"EXACT"},
	}
	ledger := QuantityLedger{
		Status:       LedgerCompleteExact,
		Requirements: []IngredientRequirement{req},
		Allocations:  []IngredientProductAllocation{allocation},
		Summary:      LedgerSummary{IngredientCount: 1, CoveredIngredientCount: 1, ExactQuantityLines: 1, SafeToBuildBasket: true},
	}
	shop := ShopResult{
		Complete: true,
		SelectedProducts: []SelectedProduct{{
			Ingredient:       Ingredient{Name: "rice", Quantity: 100, Unit: "g"},
			Product:          ProductSummary{ID: "rice-id", SKU: "rice-sku", Name: "Rice 1 kg", Price: money.Money{Amount: "1.00", Currency: "EUR", Cents: 100}, Available: &available},
			PurchaseQuantity: "1",
			PackageCount:     1,
			LineTotal:        money.Money{Amount: "1.00", Currency: "EUR", Cents: 100},
		}},
		BasketLines: []string{"rice-sku 1 # rice"},
	}
	return FoodRunArtifact{
		Kind:           "food_run",
		MealPlan:       MealPlan{People: 2, RequiredPurchases: []Ingredient{{Name: "rice", Quantity: 100, Unit: "g"}}},
		Shop:           shop,
		QuantityLedger: &ledger,
		BasketSafety:   &BasketSafety{Status: BasketSafetySafe, SafeToBuild: true},
	}
}

func hasGateIssue(gate ReadinessGate, code string) bool {
	for _, issue := range gate.BlockingIssues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
