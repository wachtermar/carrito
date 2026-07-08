package food

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConstraintSatisfactionPassesExplicitIntent(t *testing.T) {
	run := intentTestRun()
	total := 2.0
	maxBudget := 500
	maxCook := 20
	intent := MealRunIntent{
		SchemaVersion: "1",
		Source:        "test",
		Serving:       &ServingIntent{TotalServingUnits: &total, Tolerance: 0.01},
		Budget:        &BudgetIntent{MaxTotalCents: &maxBudget, RequireBudgetReady: true},
		Diet:          &DietIntent{ExcludedIngredients: []string{"pork"}, RequireDeterministicCheck: true},
		Cooking:       &CookingIntent{MaxTotalMinutes: &maxCook, RequireTimeEvidence: true},
		RecipePDF:     &RecipePDFIntent{RequireCookingSteps: true},
	}
	report := BuildConstraintSatisfactionReport(run, intent)
	if report.Status != ConstraintSatisfactionPass || !report.ClaimGuard.MayClaimRequestSatisfied {
		t.Fatalf("constraint report = %+v", report)
	}
	if !report.ClaimGuard.MayClaimBudgetSatisfied || !report.ClaimGuard.MayClaimServingSatisfied || !report.ClaimGuard.MayClaimDietSatisfied || !report.ClaimGuard.MayClaimCookingTimeSatisfied {
		t.Fatalf("category claim guard not set: %+v", report.ClaimGuard)
	}
}

func TestConstraintSatisfactionExcludedIngredientFails(t *testing.T) {
	run := intentTestRun()
	intent := MealRunIntent{
		SchemaVersion: "1",
		Source:        "test",
		Diet:          &DietIntent{ExcludedIngredients: []string{"rice"}, RequireDeterministicCheck: true},
	}
	report := BuildConstraintSatisfactionReport(run, intent)
	if report.Status != ConstraintSatisfactionFail || report.ClaimGuard.MayClaimRequestSatisfied {
		t.Fatalf("constraint report should fail: %+v", report)
	}
	if len(report.BlockingIssues) == 0 || !strings.Contains(report.BlockingIssues[0].Message, "rice") {
		t.Fatalf("expected rice blocking issue: %+v", report.BlockingIssues)
	}
}

func TestConstraintSatisfactionFingerprintSurvivesJSONRoundTrip(t *testing.T) {
	run := intentTestRun()
	maxBudget := 500
	intent := MealRunIntent{
		SchemaVersion: "1",
		Source:        "test",
		Budget:        &BudgetIntent{MaxTotalCents: &maxBudget, RequireBudgetReady: true},
		Diet:          &DietIntent{ExcludedIngredients: []string{"pork"}, ExcludedAllergens: []string{"pork"}, RequireDeterministicCheck: true},
	}
	report := BuildConstraintSatisfactionReport(run, intent)
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	var roundTripped ConstraintSatisfactionReport
	if err := json.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if got := ConstraintSatisfactionFingerprint(roundTripped); got != report.ConstraintSatisfactionFingerprint {
		t.Fatalf("fingerprint changed after JSON round trip: got %s want %s", got, report.ConstraintSatisfactionFingerprint)
	}
}

func TestReadinessRequireIntentReadyKeepsBasketBuildableButBlocksClaim(t *testing.T) {
	run := intentTestRun()
	intent := MealRunIntent{
		SchemaVersion: "1",
		Source:        "test",
		Diet:          &DietIntent{ExcludedIngredients: []string{"rice"}, RequireDeterministicCheck: true},
	}
	run = AttachMealRunIntent(run, intent)
	report := BuildConstraintSatisfactionReport(run, *run.MealRunIntent)
	run = AttachConstraintSatisfactionReport(run, report)
	gate := ApplyReadinessGate(&run, ReadinessPolicy{StrictQuantity: true, RequireSafeBasket: true, RequireIntentReady: true})
	if !gate.SafeToBuild || gate.SafeToSatisfyIntent || gate.ExitCode != ReadinessExitBlocked {
		t.Fatalf("gate = %+v", gate)
	}
	basket := strings.Join(ReadinessBasketLines([]string{"base-sku 1 # rice"}, &gate), "\n")
	if !strings.Contains(basket, "REQUEST IS NOT FULLY SATISFIED") || !strings.Contains(basket, "base-sku 1 # rice") {
		t.Fatalf("basket missing intent warning or actionable line:\n%s", basket)
	}
}

func intentTestRun() FoodRunArtifact {
	run := budgetRepairTestRun()
	run.MealPlan.BudgetEUR = "5.00"
	run.MealPlan.Days[0].Meals[0].Recipe.CookMinutes = 15
	run.MealPlan.Days[0].Meals[0].Recipe.Steps[0].Minutes = 15
	run.MealPlanFingerprint = MealPlanFingerprint(run.MealPlan)
	ledger := BuildQuantityLedger(run.MealPlan, run.Shop, QuantityLedgerOptions{StrictQuantity: true})
	run.QuantityLedger = &ledger
	safety := BasketSafetyFromLedger(ledger, QuantityLedgerOptions{StrictQuantity: true})
	run.BasketSafety = &safety
	budget := BuildBudgetDealReport(run)
	run = AttachBudgetDealReport(run, budget)
	gate := ApplyReadinessGate(&run, ReadinessPolicy{StrictQuantity: true, RequireBudgetReady: true})
	run.ReadinessGate = &gate
	return run
}
