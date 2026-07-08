package food

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wachtermar/carrito/internal/money"
)

const (
	ReadinessExitOK      = 0
	ReadinessExitBlocked = 20
)

func EvaluateReadinessGate(run *FoodRunArtifact, policy ReadinessPolicy) ReadinessGate {
	gate := ReadinessGate{
		SchemaVersion: "1",
		Status:        ReadinessBlocked,
		Policy:        policy,
		ExitCode:      ReadinessExitBlocked,
		ExitReason:    "basket is not safe to build",
	}
	if run == nil {
		gate.addBlocking("run_missing", "artifact", GateIssue{Code: "run_missing", Phase: "artifact", Message: "Food run artifact is missing.", Remediation: "Regenerate the food run."})
		return gate
	}
	gate.MealPlanFingerprint = firstNonEmptyString(run.MealPlanFingerprint, MealPlanFingerprint(run.MealPlan))
	gate.ServingPlanFingerprint = firstNonEmptyString(run.ServingPlanFingerprint, servingPlanFingerprintFromRun(run))
	gate.ScaledMealPlanFingerprint = firstNonEmptyString(run.ScaledMealPlanFingerprint, scaledMealPlanFingerprintFromRun(run))
	gate.ProductSelectionFingerprint = firstNonEmptyString(run.ProductSelectionFingerprint, ProductSelectionFingerprint(run.Shop))
	gate.PantryResolutionFingerprint = firstNonEmptyString(run.PantryResolutionFingerprint, pantryResolutionFingerprintFromRun(run))
	gate.ShopRequirementsFingerprint = firstNonEmptyString(run.ShopRequirementsFingerprint, shopRequirementsFingerprintFromRun(run))
	gate.NutritionLedgerFingerprint = firstNonEmptyString(run.NutritionLedgerFingerprint, nutritionLedgerFingerprintFromRun(run))
	gate.SafeToCook = true
	gate.CookReadinessStatus = string(ReadinessReadyExact)
	gate.SafeToReportNutrition = false
	if run.PantryResolution != nil {
		gate.PantryStatus = string(run.PantryResolution.Status)
	}
	if run.ServingPlan != nil {
		gate.ServingStatus = string(run.ServingPlan.Status)
	}
	if run.ScaledMealPlan != nil {
		gate.ScalingStatus = scalingStatusFromScaledMealPlan(*run.ScaledMealPlan)
	}
	if run.QuantityLedger != nil {
		gate.FinalLedgerStatus = string(run.QuantityLedger.Status)
	}
	if run.BasketSafety != nil {
		gate.FinalBasketSafety = run.BasketSafety.SafeToBuild
	}
	if run.RecoveryPlan != nil {
		gate.RecoveryStatus = string(run.RecoveryPlan.Status)
	}
	if run.RecipeSwapPlan != nil {
		gate.RecipeSwapStatus = string(run.RecipeSwapPlan.Status)
	}
	if run.NutritionLedger != nil {
		gate.NutritionStatus = string(run.NutritionLedger.Status)
		coverage := run.NutritionLedger.Coverage
		gate.NutritionCoverageSummary = &coverage
	}

	checkMealPlanFingerprints(&gate, run)
	checkLedgerComplete(&gate, run)
	checkBasketSafety(&gate, run)
	checkRecoveryRemaining(&gate, run)
	checkAllocations(&gate, run)
	checkBasketLines(&gate, run)
	checkRecipeAdjustments(&gate, run)
	checkRecipeSwapConsistency(&gate, run)
	buildBlockingCount := len(gate.BlockingIssues)
	checkServingReadiness(&gate, run)
	checkPantryReadiness(&gate, run)
	checkNutritionReadiness(&gate, run)

	buildBlocked := buildBlockingCount > 0
	if !buildBlocked {
		gate.SafeToBuild = true
		if !gate.SafeToCook && gate.Policy.RequireCookReady {
			gate.Status = ReadinessBlocked
			gate.ExitCode = ReadinessExitBlocked
			gate.ExitReason = "meal plan is not cook-ready"
		} else if gate.Policy.RequireNutritionReady && !gate.SafeToReportNutrition {
			gate.Status = ReadinessReadyWithCaveats
			gate.ExitCode = ReadinessExitBlocked
			gate.ExitReason = "nutrition reporting is not ready"
		} else {
			gate.ExitCode = ReadinessExitOK
			gate.ExitReason = "basket is safe to build"
			if gate.CookReadinessStatus == string(ReadinessReadyWithPantryAssumptions) {
				gate.Status = ReadinessReadyWithPantryAssumptions
			} else if len(gate.Warnings) > 0 {
				gate.Status = ReadinessReadyWithCaveats
			} else {
				gate.Status = ReadinessReadyExact
			}
		}
	} else {
		gate.SafeToCook = false
		if gate.CookReadinessStatus == "" || gate.CookReadinessStatus == string(ReadinessReadyExact) {
			gate.CookReadinessStatus = string(ReadinessBlocked)
		}
		gate.ExitReason = "basket is not safe to build"
	}
	return gate
}

func ApplyReadinessGate(run *FoodRunArtifact, policy ReadinessPolicy) ReadinessGate {
	gate := EvaluateReadinessGate(run, policy)
	if run != nil {
		run.ReadinessGate = &gate
		if run.BasketSafety == nil {
			run.BasketSafety = &BasketSafety{}
		}
		if run.MealPlanFingerprint == "" {
			run.MealPlanFingerprint = gate.MealPlanFingerprint
		}
		if run.ProductSelectionFingerprint == "" {
			run.ProductSelectionFingerprint = gate.ProductSelectionFingerprint
		}
		if run.NutritionLedgerFingerprint == "" {
			run.NutritionLedgerFingerprint = gate.NutritionLedgerFingerprint
		}
		run.BasketSafety.MealPlanFingerprint = gate.MealPlanFingerprint
		run.BasketSafety.ServingPlanFingerprint = gate.ServingPlanFingerprint
		run.BasketSafety.ScaledMealPlanFingerprint = gate.ScaledMealPlanFingerprint
		run.BasketSafety.ProductSelectionFingerprint = gate.ProductSelectionFingerprint
		run.BasketSafety.PantryResolutionFingerprint = gate.PantryResolutionFingerprint
		run.BasketSafety.ShopRequirementsFingerprint = gate.ShopRequirementsFingerprint
		run.BasketSafety.SafeToBuild = gate.SafeToBuild
		switch gate.Status {
		case ReadinessReadyExact:
			run.BasketSafety.Status = BasketSafetySafe
			run.BasketSafety.Reason = "Readiness gate passed with exact quantity coverage."
		case ReadinessReadyWithCaveats, ReadinessReadyWithPantryAssumptions:
			run.BasketSafety.Status = BasketSafetyEstimated
			run.BasketSafety.Reason = "Readiness gate passed with allowed caveats."
		default:
			run.BasketSafety.Status = BasketSafetyUnsafe
			run.BasketSafety.Reason = "Readiness gate blocked automatic basket creation."
		}
	}
	return gate
}

func (gate *ReadinessGate) addCheck(code string, passed bool, severity, message string) {
	gate.Checks = append(gate.Checks, GateCheck{Code: code, Passed: passed, Severity: severity, Message: message})
}

func (gate *ReadinessGate) addBlocking(code, phase string, issue GateIssue) {
	issue.Code = firstNonEmptyString(issue.Code, code)
	issue.Phase = firstNonEmptyString(issue.Phase, phase)
	issue.Severity = "blocking"
	gate.BlockingIssues = append(gate.BlockingIssues, issue)
	gate.addCheck(code, false, "blocking", issue.Message)
}

func (gate *ReadinessGate) addWarning(code, phase string, issue GateIssue) {
	issue.Code = firstNonEmptyString(issue.Code, code)
	issue.Phase = firstNonEmptyString(issue.Phase, phase)
	issue.Severity = "warning"
	gate.Warnings = append(gate.Warnings, issue)
	gate.addCheck(code, true, "warning", issue.Message)
}

func checkLedgerComplete(gate *ReadinessGate, run *FoodRunArtifact) {
	if run.QuantityLedger == nil {
		gate.addBlocking("quantity_ledger_missing", "ledger", GateIssue{Message: "Final quantity ledger is missing.", Remediation: "Regenerate the food run with quantity ledger enabled."})
		return
	}
	switch run.QuantityLedger.Status {
	case LedgerCompleteExact:
		gate.addCheck("final_ledger_complete_exact", true, "info", "Final ledger is complete and exact.")
	case LedgerCompleteEstimated:
		if gate.Policy.AllowEstimatedVariableWeight {
			gate.addWarning("estimated_variable_weight_allowed", "ledger", GateIssue{Message: "Final ledger contains allowed estimated variable-weight lines.", AllowedBy: []string{"allow_estimated_variable_weight"}})
		} else {
			gate.addBlocking("estimated_variable_weight_blocked", "ledger", GateIssue{Message: "Final ledger contains estimated variable-weight lines.", Remediation: "Review the line or rerun with --allow-estimated-variable-weight."})
		}
	case LedgerNeedsReview:
		if gate.Policy.AllowLowConfidenceBasket {
			gate.addWarning("low_confidence_allowed", "ledger", GateIssue{Message: "Final ledger contains allowed low-confidence lines.", AllowedBy: []string{"allow_low_confidence_basket"}})
		} else {
			gate.addBlocking("low_confidence_blocked", "ledger", GateIssue{Message: "Final ledger has quantity lines that need review.", Remediation: "Review low-confidence lines or rerun with --allow-low-confidence-basket."})
		}
	default:
		gate.addBlocking("final_ledger_incomplete", "ledger", GateIssue{Message: "Final ledger is incomplete.", Remediation: "Recover missing products or choose a different recipe."})
	}
}

func checkNutritionReadiness(gate *ReadinessGate, run *FoodRunArtifact) {
	if run.NutritionLedger == nil {
		gate.NutritionStatus = string(NutritionLedgerNotRun)
		if gate.Policy.RequireNutritionReady {
			gate.SafeToReportNutrition = false
			gate.addWarning("nutrition_ledger_missing", "nutrition", GateIssue{Message: "Nutrition reporting is required but the nutrition ledger is missing.", Remediation: "Rerun with nutrition evidence enabled."})
		} else {
			gate.addCheck("nutrition_ledger_not_run", true, "info", "Nutrition evidence was not requested.")
		}
		return
	}
	ledger := run.NutritionLedger
	gate.NutritionStatus = string(ledger.Status)
	coverage := ledger.Coverage
	gate.NutritionCoverageSummary = &coverage
	expectedNutrition := nutritionLedgerFingerprintFromRun(run)
	if run.NutritionLedgerFingerprint != "" && expectedNutrition != "" && run.NutritionLedgerFingerprint != expectedNutrition {
		gate.NutritionStatus = string(NutritionLedgerBlocked)
		gate.SafeToReportNutrition = false
		gate.addWarning("stale_nutrition_ledger_fingerprint", "nutrition", GateIssue{Message: "Nutrition ledger fingerprint does not match the final nutrition artifact.", Remediation: "Rebuild nutrition evidence after the final food run changes."})
		return
	}
	if staleNutritionLedger(run, gate) {
		gate.NutritionStatus = string(NutritionLedgerBlocked)
		gate.SafeToReportNutrition = false
		gate.addWarning("stale_nutrition_ledger_inputs", "nutrition", GateIssue{Message: "Nutrition ledger inputs do not match the final food run fingerprints.", Remediation: "Rebuild nutrition evidence after serving, pantry, shopping, or product selections change."})
		return
	}
	switch ledger.Status {
	case NutritionLedgerCompleteForPurchased, NutritionLedgerCompleteHybrid, NutritionLedgerPartial:
		gate.SafeToReportNutrition = true
		gate.addCheck("nutrition_reportable", true, "info", "Nutrition evidence is available with explicit coverage.")
	case NutritionLedgerNoCoverage:
		gate.SafeToReportNutrition = false
		gate.addWarning("nutrition_no_coverage", "nutrition", GateIssue{Message: "No label-derived or recipe-declared nutrition evidence is available.", Remediation: "Use products with nutrition labels, provide pantry nutrition, or add recipe nutrition."})
	default:
		gate.SafeToReportNutrition = false
		for _, issue := range ledger.BlockingIssues {
			gate.addWarning(issue.Code, "nutrition", GateIssue{Message: issue.Message, Remediation: issue.Remediation, IngredientKey: issue.IngredientKey, IngredientName: issue.IngredientName, ProductID: issue.ProductID, ProductName: issue.ProductName, Day: issue.Day, MealSlot: issue.MealSlot})
		}
		if len(ledger.BlockingIssues) == 0 {
			gate.addWarning("nutrition_not_ready", "nutrition", GateIssue{Message: "Nutrition evidence is not ready.", Remediation: "Review nutrition ledger warnings and coverage."})
		}
	}
}

func staleNutritionLedger(run *FoodRunArtifact, gate *ReadinessGate) bool {
	if run == nil || run.NutritionLedger == nil {
		return false
	}
	ledger := run.NutritionLedger
	if ledger.MealPlanFingerprint != "" && gate.MealPlanFingerprint != "" && ledger.MealPlanFingerprint != gate.MealPlanFingerprint {
		return true
	}
	if ledger.ServingPlanFingerprint != "" && gate.ServingPlanFingerprint != "" && ledger.ServingPlanFingerprint != gate.ServingPlanFingerprint {
		return true
	}
	if ledger.ScaledMealPlanFingerprint != "" && gate.ScaledMealPlanFingerprint != "" && ledger.ScaledMealPlanFingerprint != gate.ScaledMealPlanFingerprint {
		return true
	}
	if ledger.PantryResolutionFingerprint != "" && gate.PantryResolutionFingerprint != "" && ledger.PantryResolutionFingerprint != gate.PantryResolutionFingerprint {
		return true
	}
	if ledger.ShopRequirementsFingerprint != "" && gate.ShopRequirementsFingerprint != "" && ledger.ShopRequirementsFingerprint != gate.ShopRequirementsFingerprint {
		return true
	}
	return ledger.ProductSelectionFingerprint != "" && gate.ProductSelectionFingerprint != "" && ledger.ProductSelectionFingerprint != gate.ProductSelectionFingerprint
}

func checkMealPlanFingerprints(gate *ReadinessGate, run *FoodRunArtifact) {
	expected := gate.MealPlanFingerprint
	if expected == "" {
		gate.addWarning("mealplan_fingerprint_missing", "artifact", GateIssue{Message: "Meal plan fingerprint is missing; stale derived artifacts cannot be fully checked."})
		return
	}
	if run.MealPlanFingerprint != "" && run.MealPlanFingerprint != expected {
		gate.addBlocking("stale_mealplan_fingerprint", "artifact", GateIssue{Message: "Run artifact meal plan fingerprint does not match the final meal plan.", Remediation: "Rebuild the food run artifact from the final meal plan."})
	}
	if run.QuantityLedger != nil && run.QuantityLedger.MealPlanFingerprint != "" && run.QuantityLedger.MealPlanFingerprint != expected {
		gate.addBlocking("stale_quantity_ledger_fingerprint", "ledger", GateIssue{Message: "Quantity ledger fingerprint does not match the final meal plan.", Remediation: "Rebuild the quantity ledger after the final meal plan changes."})
	}
	expectedSelection := gate.ProductSelectionFingerprint
	if run.QuantityLedger != nil && run.QuantityLedger.ProductSelectionFingerprint != "" && expectedSelection != "" && run.QuantityLedger.ProductSelectionFingerprint != expectedSelection {
		gate.addBlocking("stale_quantity_ledger_selection_fingerprint", "ledger", GateIssue{Message: "Quantity ledger product-selection fingerprint does not match the final selected products.", Remediation: "Rebuild the quantity ledger after product selections change."})
	}
	if run.BasketSafety != nil && run.BasketSafety.MealPlanFingerprint != "" && run.BasketSafety.MealPlanFingerprint != expected {
		gate.addBlocking("stale_basket_safety_fingerprint", "basket", GateIssue{Message: "Basket safety fingerprint does not match the final meal plan.", Remediation: "Rebuild basket safety after the final meal plan changes."})
	}
	if run.BasketSafety != nil && run.BasketSafety.ProductSelectionFingerprint != "" && expectedSelection != "" && run.BasketSafety.ProductSelectionFingerprint != expectedSelection {
		gate.addBlocking("stale_basket_safety_selection_fingerprint", "basket", GateIssue{Message: "Basket safety product-selection fingerprint does not match the final selected products.", Remediation: "Rebuild basket safety after product selections change."})
	}
	if run.RecoveryPlan != nil && run.RecoveryPlan.MealPlanFingerprint != "" && run.RecoveryPlan.MealPlanFingerprint != expected {
		gate.addBlocking("stale_recovery_fingerprint", "recovery", GateIssue{Message: "Recovery plan fingerprint does not match the final meal plan.", Remediation: "Rerun recovery after the final meal plan changes."})
	}
	if run.RecoveryPlan != nil && run.RecoveryPlan.ProductSelectionFingerprint != "" && expectedSelection != "" && run.RecoveryPlan.ProductSelectionFingerprint != expectedSelection {
		gate.addBlocking("stale_recovery_selection_fingerprint", "recovery", GateIssue{Message: "Recovery plan product-selection fingerprint does not match the final selected products.", Remediation: "Rerun recovery after product selections change."})
	}
	expectedServing := gate.ServingPlanFingerprint
	if run.ServingPlan != nil && run.ServingPlan.ServingPlanFingerprint != "" && expectedServing != "" && run.ServingPlan.ServingPlanFingerprint != expectedServing {
		gate.addBlocking("stale_serving_plan_fingerprint", "serving", GateIssue{Message: "Serving plan fingerprint does not match the final artifact.", Remediation: "Rebuild serving plan and scaled meal plan after serving targets change."})
	}
	expectedScaled := gate.ScaledMealPlanFingerprint
	if run.ScaledMealPlan != nil && run.ScaledMealPlan.ScaledMealPlanFingerprint != "" && expectedScaled != "" && run.ScaledMealPlan.ScaledMealPlanFingerprint != expectedScaled {
		gate.addBlocking("stale_scaled_mealplan_fingerprint", "serving", GateIssue{Message: "Scaled meal plan fingerprint does not match the final artifact.", Remediation: "Rebuild scaled meal plan after serving targets or recipes change."})
	}
	if run.ServingPlan != nil && run.ServingPlan.Policy.Enabled && run.ScaledMealPlan == nil {
		gate.addBlocking("scaled_mealplan_missing", "serving", GateIssue{Message: "Serving scaling is enabled but the scaled meal plan artifact is missing.", Remediation: "Regenerate the food run with serving scaling enabled."})
	}
	if run.QuantityLedger != nil && run.QuantityLedger.ServingPlanFingerprint != "" && expectedServing != "" && run.QuantityLedger.ServingPlanFingerprint != expectedServing {
		gate.addBlocking("stale_quantity_ledger_serving_fingerprint", "ledger", GateIssue{Message: "Quantity ledger serving fingerprint does not match the final serving plan.", Remediation: "Rebuild the quantity ledger after serving plan changes."})
	}
	if run.QuantityLedger != nil && run.QuantityLedger.ScaledMealPlanFingerprint != "" && expectedScaled != "" && run.QuantityLedger.ScaledMealPlanFingerprint != expectedScaled {
		gate.addBlocking("stale_quantity_ledger_scaled_fingerprint", "ledger", GateIssue{Message: "Quantity ledger scaled meal plan fingerprint does not match the final scaled meal plan.", Remediation: "Rebuild the quantity ledger after scaled quantities change."})
	}
	if run.BasketSafety != nil && run.BasketSafety.ServingPlanFingerprint != "" && expectedServing != "" && run.BasketSafety.ServingPlanFingerprint != expectedServing {
		gate.addBlocking("stale_basket_safety_serving_fingerprint", "basket", GateIssue{Message: "Basket safety serving fingerprint does not match the final serving plan.", Remediation: "Rebuild basket safety after serving plan changes."})
	}
	if run.BasketSafety != nil && run.BasketSafety.ScaledMealPlanFingerprint != "" && expectedScaled != "" && run.BasketSafety.ScaledMealPlanFingerprint != expectedScaled {
		gate.addBlocking("stale_basket_safety_scaled_fingerprint", "basket", GateIssue{Message: "Basket safety scaled meal plan fingerprint does not match the final scaled meal plan.", Remediation: "Rebuild basket safety after scaled quantities change."})
	}
	if run.RecoveryPlan != nil && run.RecoveryPlan.ServingPlanFingerprint != "" && expectedServing != "" && run.RecoveryPlan.ServingPlanFingerprint != expectedServing {
		gate.addBlocking("stale_recovery_serving_fingerprint", "recovery", GateIssue{Message: "Recovery plan serving fingerprint does not match the final serving plan.", Remediation: "Rerun recovery after serving plan changes."})
	}
	if run.RecoveryPlan != nil && run.RecoveryPlan.ScaledMealPlanFingerprint != "" && expectedScaled != "" && run.RecoveryPlan.ScaledMealPlanFingerprint != expectedScaled {
		gate.addBlocking("stale_recovery_scaled_fingerprint", "recovery", GateIssue{Message: "Recovery plan scaled meal plan fingerprint does not match final scaled quantities.", Remediation: "Rerun recovery after scaled quantities change."})
	}
	if run.BasketOptimizationPlan != nil && run.BasketOptimizationPlan.ServingPlanFingerprint != "" && expectedServing != "" && run.BasketOptimizationPlan.ServingPlanFingerprint != expectedServing {
		gate.addBlocking("stale_basket_optimization_serving_fingerprint", "optimization", GateIssue{Message: "Basket optimization serving fingerprint does not match the final serving plan.", Remediation: "Rerun basket optimization after serving plan changes."})
	}
	if run.BasketOptimizationPlan != nil && run.BasketOptimizationPlan.ScaledMealPlanFingerprint != "" && expectedScaled != "" && run.BasketOptimizationPlan.ScaledMealPlanFingerprint != expectedScaled {
		gate.addBlocking("stale_basket_optimization_scaled_fingerprint", "optimization", GateIssue{Message: "Basket optimization scaled meal plan fingerprint does not match final scaled quantities.", Remediation: "Rerun basket optimization after scaled quantities change."})
	}
	expectedPantry := gate.PantryResolutionFingerprint
	if run.PantryResolution != nil && run.PantryResolution.PantryResolutionFingerprint != "" && expectedPantry != "" && run.PantryResolution.PantryResolutionFingerprint != expectedPantry {
		gate.addBlocking("stale_pantry_resolution_fingerprint", "pantry", GateIssue{Message: "Pantry resolution fingerprint does not match the final artifact.", Remediation: "Rerun pantry resolution after the final meal plan changes."})
	}
	if run.PantryResolution != nil && run.PantryResolution.ServingPlanFingerprint != "" && expectedServing != "" && run.PantryResolution.ServingPlanFingerprint != expectedServing {
		gate.addBlocking("stale_pantry_resolution_serving_fingerprint", "pantry", GateIssue{Message: "Pantry resolution serving fingerprint does not match the final serving plan.", Remediation: "Rerun pantry resolution after serving plan changes."})
	}
	if run.PantryResolution != nil && run.PantryResolution.ScaledMealPlanFingerprint != "" && expectedScaled != "" && run.PantryResolution.ScaledMealPlanFingerprint != expectedScaled {
		gate.addBlocking("stale_pantry_resolution_scaled_fingerprint", "pantry", GateIssue{Message: "Pantry resolution scaled meal plan fingerprint does not match final scaled quantities.", Remediation: "Rerun pantry resolution after scaled quantities change."})
	}
	if run.QuantityLedger != nil && run.QuantityLedger.PantryResolutionFingerprint != "" && expectedPantry != "" && run.QuantityLedger.PantryResolutionFingerprint != expectedPantry {
		gate.addBlocking("stale_quantity_ledger_pantry_fingerprint", "ledger", GateIssue{Message: "Quantity ledger pantry fingerprint does not match the final pantry resolution.", Remediation: "Rebuild the quantity ledger after pantry resolution changes."})
	}
	if run.BasketSafety != nil && run.BasketSafety.PantryResolutionFingerprint != "" && expectedPantry != "" && run.BasketSafety.PantryResolutionFingerprint != expectedPantry {
		gate.addBlocking("stale_basket_safety_pantry_fingerprint", "basket", GateIssue{Message: "Basket safety pantry fingerprint does not match the final pantry resolution.", Remediation: "Rebuild basket safety after pantry resolution changes."})
	}
	if run.RecoveryPlan != nil && run.RecoveryPlan.PantryResolutionFingerprint != "" && expectedPantry != "" && run.RecoveryPlan.PantryResolutionFingerprint != expectedPantry {
		gate.addBlocking("stale_recovery_pantry_fingerprint", "recovery", GateIssue{Message: "Recovery plan pantry fingerprint does not match the final pantry resolution.", Remediation: "Rerun recovery after pantry resolution changes."})
	}
	expectedShopRequirements := gate.ShopRequirementsFingerprint
	if run.QuantityLedger != nil && run.QuantityLedger.ShopRequirementsFingerprint != "" && expectedShopRequirements != "" && run.QuantityLedger.ShopRequirementsFingerprint != expectedShopRequirements {
		gate.addBlocking("stale_quantity_ledger_shop_requirements_fingerprint", "ledger", GateIssue{Message: "Quantity ledger shop-requirements fingerprint does not match the final shopping requirements.", Remediation: "Rebuild the quantity ledger after pantry or shopping requirements change."})
	}
	if run.BasketSafety != nil && run.BasketSafety.ShopRequirementsFingerprint != "" && expectedShopRequirements != "" && run.BasketSafety.ShopRequirementsFingerprint != expectedShopRequirements {
		gate.addBlocking("stale_basket_safety_shop_requirements_fingerprint", "basket", GateIssue{Message: "Basket safety shop-requirements fingerprint does not match the final shopping requirements.", Remediation: "Rebuild basket safety after pantry or shopping requirements change."})
	}
}

func checkBasketSafety(gate *ReadinessGate, run *FoodRunArtifact) {
	if run.BasketSafety == nil {
		gate.addBlocking("basket_safety_missing", "basket", GateIssue{Message: "Basket safety result is missing.", Remediation: "Regenerate the food run."})
		return
	}
	if run.BasketSafety.SafeToBuild {
		gate.addCheck("final_basket_safety_true", true, "info", "Basket safety allows build.")
		return
	}
	gate.addBlocking("final_basket_safety_false", "basket", GateIssue{Message: "Basket safety does not allow automatic basket creation.", Remediation: run.BasketSafety.Reason})
}

func checkRecoveryRemaining(gate *ReadinessGate, run *FoodRunArtifact) {
	if run.RecoveryPlan == nil || len(run.RecoveryPlan.RemainingIssues) == 0 {
		gate.addCheck("no_blocked_recovery_issues_remaining", true, "info", "No blocking recovery issues remain.")
		return
	}
	for _, issue := range run.RecoveryPlan.RemainingIssues {
		gate.addBlocking("recovery_issue_remaining", "recovery", GateIssue{
			IngredientKey:   normalizeKey(issue.IngredientName),
			IngredientName:  issue.IngredientName,
			RecipeID:        issue.RecipeID,
			Day:             issue.Day,
			MealSlot:        issue.MealSlotID,
			RecoveryIssueID: issue.ID,
			Message:         "Recovery issue remains: " + issue.BlockingReason,
			Remediation:     "Choose a manual product, allow recipe swap, or change the meal.",
		})
	}
}

func checkAllocations(gate *ReadinessGate, run *FoodRunArtifact) {
	if run.QuantityLedger == nil {
		return
	}
	requirements := make(map[string]IngredientRequirement, len(run.QuantityLedger.Requirements))
	for _, req := range run.QuantityLedger.Requirements {
		requirements[req.RequirementID] = req
	}
	if len(run.QuantityLedger.Requirements) > 0 && len(run.QuantityLedger.Allocations) == 0 {
		gate.addBlocking("allocations_missing", "ledger", GateIssue{Message: "Final ledger has requirements but no allocations.", Remediation: "Regenerate shopping selection."})
		return
	}
	for _, allocation := range run.QuantityLedger.Allocations {
		req := requirements[allocation.RequirementID]
		if allocation.MatchType == "missing" || allocation.ProductName == "" {
			gate.addBlocking("required_ingredient_missing_product", "ledger", GateIssue{
				IngredientKey:  normalizeKey(req.IngredientName),
				IngredientName: req.IngredientName,
				RecipeID:       req.RecipeID,
				Day:            req.Day,
				MealSlot:       req.MealSlot,
				Message:        "Required ingredient has no accepted selected product.",
				Remediation:    "Recover the product or choose a different recipe.",
			})
			continue
		}
		if allocation.PackageCount <= 0 {
			gate.addBlocking("package_count_invalid", "ledger", GateIssue{
				IngredientKey:  normalizeKey(req.IngredientName),
				IngredientName: req.IngredientName,
				RecipeID:       req.RecipeID,
				Day:            req.Day,
				MealSlot:       req.MealSlot,
				ProductID:      allocation.SKU,
				ProductName:    allocation.ProductName,
				Message:        "Selected product package count is zero or missing.",
				Remediation:    "Review quantity math before using the basket.",
			})
		}
		if allocation.MissingQuantity != nil && allocation.MissingQuantity.BaseValue > 0 {
			gate.addBlocking("missing_required_quantity", "ledger", GateIssue{
				IngredientKey:  normalizeKey(req.IngredientName),
				IngredientName: req.IngredientName,
				RecipeID:       req.RecipeID,
				Day:            req.Day,
				MealSlot:       req.MealSlot,
				ProductID:      allocation.SKU,
				ProductName:    allocation.ProductName,
				Message:        "Required ingredient has missing quantity.",
				Remediation:    "Select enough product quantity or change the recipe.",
			})
		}
		if containsString(allocation.Badges, "ESTIMATED WEIGHT") && !gate.Policy.AllowEstimatedVariableWeight {
			gate.addBlocking("estimated_allocation_blocked", "ledger", GateIssue{
				IngredientKey:  normalizeKey(req.IngredientName),
				IngredientName: req.IngredientName,
				RecipeID:       req.RecipeID,
				Day:            req.Day,
				MealSlot:       req.MealSlot,
				ProductID:      allocation.SKU,
				ProductName:    allocation.ProductName,
				Message:        "Allocation uses estimated variable weight.",
				Remediation:    "Review the estimate or rerun with --allow-estimated-variable-weight.",
			})
		}
		if containsString(allocation.Badges, "LOW QUANTITY CONFIDENCE") && !gate.Policy.AllowLowConfidenceBasket {
			gate.addBlocking("low_confidence_allocation_blocked", "ledger", GateIssue{
				IngredientKey:  normalizeKey(req.IngredientName),
				IngredientName: req.IngredientName,
				RecipeID:       req.RecipeID,
				Day:            req.Day,
				MealSlot:       req.MealSlot,
				ProductID:      allocation.SKU,
				ProductName:    allocation.ProductName,
				Message:        "Allocation has low quantity confidence.",
				Remediation:    "Review the line or rerun with --allow-low-confidence-basket.",
			})
		}
	}
}

func checkRecipeSwapConsistency(gate *ReadinessGate, run *FoodRunArtifact) {
	if run.RecipeSwapPlan == nil || len(run.RecipeSwapPlan.AppliedSwaps) == 0 {
		return
	}
	adjustmentBySwap := map[string]bool{}
	for _, adjustment := range run.RecipeAdjustments {
		if adjustment.Type == "recipe_swap" && adjustment.RecipeSwapID != "" {
			adjustmentBySwap[adjustment.RecipeSwapID] = true
		}
	}
	finalRecipeIDs := map[string]bool{}
	for _, day := range run.MealPlan.Days {
		for _, meal := range day.Meals {
			finalRecipeIDs[meal.Recipe.ID] = true
		}
	}
	for _, swap := range run.RecipeSwapPlan.AppliedSwaps {
		if !adjustmentBySwap[swap.ID] {
			gate.addBlocking("recipe_swap_applied_but_no_recipe_adjustment", "recipe_swap", GateIssue{
				RecipeID:    swap.ReplacementRecipeID,
				Day:         swap.Day,
				MealSlot:    swap.MealSlot,
				Message:     "Recipe swap was applied without a structured recipe adjustment.",
				Remediation: "Regenerate meal-plan repair so the final recipe and PDF remain auditable.",
			})
		}
		if swap.ReplacementRecipeID != "" && !finalRecipeIDs[swap.ReplacementRecipeID] {
			gate.addBlocking("final_shop_not_rebuilt_after_recipe_swap", "recipe_swap", GateIssue{
				RecipeID:    swap.ReplacementRecipeID,
				Day:         swap.Day,
				MealSlot:    swap.MealSlot,
				Message:     "Applied replacement recipe is not present in the final meal plan.",
				Remediation: "Rebuild the meal plan, shop, ledger, and basket from the applied replacement recipe.",
			})
		}
	}
}

func checkBasketLines(gate *ReadinessGate, run *FoodRunArtifact) {
	if len(run.Shop.SelectedProducts) == 0 {
		if run.QuantityLedger != nil && len(run.QuantityLedger.Requirements) > 0 {
			gate.addBlocking("basket_lines_missing", "basket", GateIssue{Message: "No selected products were found for required ingredients.", Remediation: "Recover missing products or change recipes."})
		}
		return
	}
	selectedByRef := map[string]SelectedProduct{}
	for _, selected := range run.Shop.SelectedProducts {
		if selected.Error != "" {
			continue
		}
		ref := firstNonEmptyString(selected.Product.SKU, selected.Product.ID)
		if ref != "" {
			selectedByRef[ref] = selected
		}
	}
	lineCount := 0
	for _, line := range run.Shop.BasketLines {
		ref, qty, ok := parseBasketLine(line)
		if !ok {
			gate.addBlocking("basket_line_invalid", "basket", GateIssue{Message: "Basket line is not parseable: " + line, Remediation: "Regenerate the basket file."})
			continue
		}
		lineCount++
		selected, ok := selectedByRef[ref]
		if !ok {
			gate.addBlocking("basket_line_product_unmatched", "basket", GateIssue{ProductID: ref, Message: "Basket line product does not match final selected products.", Remediation: "Regenerate the basket from the final artifact."})
			continue
		}
		if parsed, err := strconv.ParseFloat(qty, 64); err != nil || parsed <= 0 {
			gate.addBlocking("basket_line_quantity_invalid", "basket", GateIssue{ProductID: ref, ProductName: selected.Product.Name, Message: "Basket line quantity is not positive.", Remediation: "Review package count before using the basket."})
		}
	}
	if len(selectedByRef) > 0 && lineCount == 0 {
		gate.addBlocking("basket_lines_missing", "basket", GateIssue{Message: "Selected products exist but basket lines are missing.", Remediation: "Regenerate the basket file."})
	}
}

func checkRecipeAdjustments(gate *ReadinessGate, run *FoodRunArtifact) {
	adjustmentByDecision := map[string]bool{}
	for _, adjustment := range run.RecipeAdjustments {
		if adjustment.RecoveryDecisionID != "" {
			adjustmentByDecision[adjustment.RecoveryDecisionID] = true
		}
	}
	if run.RecoveryPlan == nil {
		return
	}
	for _, decision := range run.RecoveryPlan.AppliedDecisions {
		needsAdjustment := false
		for _, change := range decision.RecipeChanges {
			if change.ChangeType == "prep_note" || change.ChangeType == "ingredient_replace" || change.ChangeType == "recipe_swap" {
				needsAdjustment = true
			}
		}
		if needsAdjustment && !adjustmentByDecision[decision.ID] {
			gate.addBlocking("recipe_adjustment_missing", "recovery", GateIssue{
				IngredientKey:   normalizeKey(decision.OriginalIngredient),
				IngredientName:  decision.OriginalIngredient,
				RecoveryIssueID: decision.IssueID,
				DecisionID:      decision.ID,
				Message:         "Recovery changed recipe behavior but no structured recipe adjustment was recorded.",
				Remediation:     "Regenerate recovery so the final recipe and PDF remain auditable.",
			})
		}
	}
}

func checkPantryReadiness(gate *ReadinessGate, run *FoodRunArtifact) {
	if run.PantryResolution == nil || run.PantryResolution.Status == PantryResolutionNotUsed {
		if gate.SafeToCook {
			gate.CookReadinessStatus = string(ReadinessReadyExact)
		}
		if gate.PantryStatus == "" {
			gate.PantryStatus = string(PantryResolutionNotUsed)
		}
		return
	}
	resolution := run.PantryResolution
	gate.PantryStatus = string(resolution.Status)
	if resolution.Status == PantryResolutionBlocked || len(resolution.BlockingIssues) > 0 {
		gate.SafeToCook = false
		gate.CookReadinessStatus = string(ReadinessBlocked)
		for _, issue := range resolution.BlockingIssues {
			gateIssue := GateIssue{
				IngredientKey:  issue.IngredientKey,
				IngredientName: issue.IngredientName,
				Message:        issue.Message,
				Remediation:    issue.Remediation,
			}
			if gate.Policy.RequireCookReady {
				gate.addBlocking(firstNonEmptyString(issue.Code, "pantry_blocked"), "pantry", gateIssue)
			} else {
				gate.addWarning(firstNonEmptyString(issue.Code, "pantry_blocked"), "pantry", gateIssue)
			}
		}
		return
	}
	if resolution.Summary.PantryAssumedLines > 0 {
		if gate.Policy.RequireCookReady && resolution.Policy.RequireConfirmedPantry {
			gate.SafeToCook = false
			gate.CookReadinessStatus = string(ReadinessBlocked)
			gate.addBlocking("pantry_assumption_requires_confirmation", "pantry", GateIssue{
				Message:     "Cooking readiness requires confirmed pantry, but the plan uses pantry assumptions.",
				Remediation: "Confirm pantry items, disable assumed pantry, or shop all ingredients.",
			})
			return
		}
		if gate.SafeToCook {
			gate.CookReadinessStatus = string(ReadinessReadyWithPantryAssumptions)
		}
		for _, issue := range resolution.Warnings {
			if issue.Code == "pantry_assumption" {
				gate.addWarning(issue.Code, "pantry", GateIssue{
					IngredientKey:  issue.IngredientKey,
					IngredientName: issue.IngredientName,
					Message:        issue.Message,
					Remediation:    issue.Remediation,
				})
			}
		}
		return
	}
	if gate.SafeToCook {
		gate.CookReadinessStatus = string(ReadinessReadyExact)
	}
}

func checkServingReadiness(gate *ReadinessGate, run *FoodRunArtifact) {
	if run.ServingPlan == nil || run.ServingPlan.Status == ServingPlanNotUsed {
		if gate.ServingStatus == "" {
			gate.ServingStatus = string(ServingPlanNotUsed)
		}
		return
	}
	servingPlan := run.ServingPlan
	gate.ServingStatus = string(servingPlan.Status)
	if run.ScaledMealPlan != nil {
		gate.ScalingStatus = scalingStatusFromScaledMealPlan(*run.ScaledMealPlan)
	}
	for _, issue := range servingPlan.Warnings {
		gate.addWarning(firstNonEmptyString(issue.Code, "serving_warning"), "serving", GateIssue{
			Day:         issue.Day,
			MealSlot:    issue.MealSlot,
			RecipeID:    issue.RecipeID,
			Message:     issue.Message,
			Remediation: issue.Remediation,
		})
	}
	if servingPlan.Status != ServingPlanBlocked && len(servingPlan.BlockingIssues) == 0 {
		return
	}
	gate.SafeToCook = false
	gate.CookReadinessStatus = string(ReadinessBlocked)
	for _, issue := range servingPlan.BlockingIssues {
		gateIssue := GateIssue{
			Day:         issue.Day,
			MealSlot:    issue.MealSlot,
			RecipeID:    issue.RecipeID,
			Message:     issue.Message,
			Remediation: issue.Remediation,
		}
		if gate.Policy.RequireCookReady {
			gate.addBlocking(firstNonEmptyString(issue.Code, "serving_blocked"), "serving", gateIssue)
		} else {
			gate.addWarning(firstNonEmptyString(issue.Code, "serving_blocked"), "serving", gateIssue)
		}
	}
}

func pantryResolutionFingerprintFromRun(run *FoodRunArtifact) string {
	if run == nil {
		return ""
	}
	if run.PantryResolution != nil {
		return run.PantryResolution.PantryResolutionFingerprint
	}
	if run.QuantityLedger != nil {
		return run.QuantityLedger.PantryResolutionFingerprint
	}
	if run.BasketSafety != nil {
		return run.BasketSafety.PantryResolutionFingerprint
	}
	return ""
}

func servingPlanFingerprintFromRun(run *FoodRunArtifact) string {
	if run == nil {
		return ""
	}
	if run.ServingPlan != nil {
		return run.ServingPlan.ServingPlanFingerprint
	}
	if run.QuantityLedger != nil {
		return run.QuantityLedger.ServingPlanFingerprint
	}
	if run.BasketSafety != nil {
		return run.BasketSafety.ServingPlanFingerprint
	}
	return ""
}

func scaledMealPlanFingerprintFromRun(run *FoodRunArtifact) string {
	if run == nil {
		return ""
	}
	if run.ScaledMealPlan != nil {
		return run.ScaledMealPlan.ScaledMealPlanFingerprint
	}
	if run.QuantityLedger != nil {
		return run.QuantityLedger.ScaledMealPlanFingerprint
	}
	if run.BasketSafety != nil {
		return run.BasketSafety.ScaledMealPlanFingerprint
	}
	return ""
}

func nutritionLedgerFingerprintFromRun(run *FoodRunArtifact) string {
	if run == nil || run.NutritionLedger == nil {
		return ""
	}
	return run.NutritionLedger.NutritionLedgerFingerprint
}

func scalingStatusFromScaledMealPlan(plan ScaledMealPlan) string {
	switch {
	case plan.Summary.BlockedLines > 0:
		return ScalingStatusBlocked
	case plan.Summary.UnparseableLines > 0:
		return ScalingStatusNeedsReview
	case plan.Summary.RoundedPieceLines > 0:
		return ScalingStatusRounded
	default:
		return ScalingStatusExact
	}
}

func shopRequirementsFingerprintFromRun(run *FoodRunArtifact) string {
	if run == nil {
		return ""
	}
	if run.PantryResolution != nil {
		return run.PantryResolution.ShopRequirementsFingerprint
	}
	if run.QuantityLedger != nil {
		return run.QuantityLedger.ShopRequirementsFingerprint
	}
	if run.BasketSafety != nil {
		return run.BasketSafety.ShopRequirementsFingerprint
	}
	return ""
}

func parseBasketLine(line string) (ref, qty string, ok bool) {
	body := strings.TrimSpace(line)
	if idx := strings.IndexByte(body, '#'); idx >= 0 {
		body = strings.TrimSpace(body[:idx])
	}
	fields := strings.Fields(body)
	if len(fields) < 2 {
		return "", "", false
	}
	return fields[0], fields[1], true
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func ReadinessBasketLines(lines []string, gate *ReadinessGate) []string {
	if gate == nil {
		return lines
	}
	if gate.SafeToBuild {
		out := []string{
			"# Basket readiness: " + string(gate.Status),
			"# Safe to build basket: true",
			"# Cooking readiness: " + firstNonEmptyString(gate.CookReadinessStatus, string(ReadinessReadyExact)),
			fmt.Sprintf("# Safe to cook: %t", gate.SafeToCook),
		}
		if gate.NutritionStatus != "" {
			out = append(out, "# Nutrition readiness: "+gate.NutritionStatus)
			out = append(out, fmt.Sprintf("# Safe to report nutrition: %t", gate.SafeToReportNutrition))
		}
		if !gate.SafeToCook {
			out = append(out, "# BASKET CAN BE BUILT, BUT MEAL PLAN IS NOT COOK-READY")
			out = append(out, "# Do not rely on this basket alone for cooking.")
		}
		if gate.Policy.RequireNutritionReady && !gate.SafeToReportNutrition {
			out = append(out, "# BASKET CAN BE BUILT, BUT NUTRITION REPORTING IS NOT READY")
			out = append(out, "# Do not treat calorie or macro numbers as reliable yet.")
		}
		if len(gate.Warnings) > 0 {
			out = append(out, "# Caveats:")
			for _, warning := range gate.Warnings {
				out = append(out, "# - "+warning.Message)
			}
		}
		out = append(out, lines...)
		return out
	}
	out := []string{
		"# NOT SAFE TO BUILD BASKET",
		"# This basket has unresolved blocking issues.",
		"# Do not use this file to buy products.",
		"# Blocking issues:",
	}
	for _, issue := range gate.BlockingIssues {
		out = append(out, "# - "+issue.Message)
	}
	out = append(out, "# Diagnostic selected products, not a complete basket:")
	for _, line := range lines {
		out = append(out, "# "+line)
	}
	return out
}

func ReadinessBasketLinesWithRecipeSwap(lines []string, gate *ReadinessGate, swapPlan *RecipeSwapPlan) []string {
	out := ReadinessBasketLines(lines, gate)
	if gate == nil || swapPlan == nil {
		return out
	}
	summary := recipeSwapBasketSummary(*swapPlan, gate.SafeToBuild)
	if summary == "" {
		return out
	}
	insertAt := 0
	if gate.SafeToBuild {
		insertAt = minInt(len(out), 4)
	} else {
		insertAt = minInt(len(out), 3)
	}
	out = append(out, "")
	copy(out[insertAt+1:], out[insertAt:])
	out[insertAt] = "# " + summary
	return out
}

func ReadinessBasketLinesWithRecipeSwapAndOptimization(lines []string, gate *ReadinessGate, swapPlan *RecipeSwapPlan, optimizationPlan *BasketOptimizationPlan) []string {
	out := ReadinessBasketLinesWithRecipeSwap(lines, gate, swapPlan)
	if gate == nil || optimizationPlan == nil {
		return out
	}
	summary := basketOptimizationBasketSummary(*optimizationPlan, gate.SafeToBuild)
	if summary == "" {
		return out
	}
	insertAt := 0
	if gate.SafeToBuild {
		insertAt = minInt(len(out), 4)
	} else {
		insertAt = minInt(len(out), 3)
	}
	rows := []string{"# " + summary}
	if optimizationPlan.Policy.Objective != "" {
		rows = append(rows, "# Basket optimization objective: "+strings.ReplaceAll(optimizationPlan.Policy.Objective, "_", "-"))
	}
	if optimizationPlan.OptimizedSummary != nil && optimizationPlan.BaselineSummary.EffectiveSubtotalCents > 0 {
		delta := optimizationPlan.OptimizedSummary.EffectiveSubtotalCents - optimizationPlan.BaselineSummary.EffectiveSubtotalCents
		if delta != 0 {
			rows = append(rows, "# Basket optimization subtotal delta: "+formatSignedCents(delta))
		}
	}
	next := make([]string, 0, len(out)+len(rows))
	next = append(next, out[:insertAt]...)
	next = append(next, rows...)
	next = append(next, out[insertAt:]...)
	return next
}

func ReadinessBasketLinesWithRecipeSwapOptimizationAndPantry(lines []string, gate *ReadinessGate, swapPlan *RecipeSwapPlan, optimizationPlan *BasketOptimizationPlan, pantry *PantryResolution) []string {
	return ReadinessBasketLinesWithRecipeSwapOptimizationPantryAndServing(lines, gate, swapPlan, optimizationPlan, pantry, nil, nil)
}

func ReadinessBasketLinesWithRecipeSwapOptimizationPantryAndServing(lines []string, gate *ReadinessGate, swapPlan *RecipeSwapPlan, optimizationPlan *BasketOptimizationPlan, pantry *PantryResolution, serving *ServingPlan, scaled *ScaledMealPlan) []string {
	return ReadinessBasketLinesWithRecipeSwapOptimizationPantryServingAndNutrition(lines, gate, swapPlan, optimizationPlan, pantry, serving, scaled, nil)
}

func ReadinessBasketLinesWithRecipeSwapOptimizationPantryServingAndNutrition(lines []string, gate *ReadinessGate, swapPlan *RecipeSwapPlan, optimizationPlan *BasketOptimizationPlan, pantry *PantryResolution, serving *ServingPlan, scaled *ScaledMealPlan, nutrition *NutritionLedger) []string {
	out := ReadinessBasketLinesWithRecipeSwapAndOptimization(lines, gate, swapPlan, optimizationPlan)
	if gate != nil && nutrition != nil && nutrition.Status != NutritionLedgerNotRun {
		nutritionLines := nutritionBasketLines(*nutrition)
		if len(nutritionLines) > 0 {
			insertAt := 0
			if gate.SafeToBuild {
				insertAt = minInt(len(out), 6)
			} else {
				insertAt = minInt(len(out), 3)
			}
			next := make([]string, 0, len(out)+len(nutritionLines))
			next = append(next, out[:insertAt]...)
			next = append(next, nutritionLines...)
			next = append(next, out[insertAt:]...)
			out = next
		}
	}
	if gate != nil && serving != nil && serving.Status != ServingPlanNotUsed {
		servingLines := servingBasketLines(*serving, scaled)
		if len(servingLines) > 0 {
			insertAt := 0
			if gate.SafeToBuild {
				insertAt = minInt(len(out), 4)
			} else {
				insertAt = minInt(len(out), 3)
			}
			next := make([]string, 0, len(out)+len(servingLines))
			next = append(next, out[:insertAt]...)
			next = append(next, servingLines...)
			next = append(next, out[insertAt:]...)
			out = next
		}
	}
	if gate == nil || pantry == nil || pantry.Status == PantryResolutionNotUsed {
		return out
	}
	pantryLines := pantryBasketLines(*pantry)
	if len(pantryLines) == 0 {
		return out
	}
	insertAt := 0
	if gate.SafeToBuild {
		insertAt = minInt(len(out), 4)
	} else {
		insertAt = minInt(len(out), 3)
	}
	next := make([]string, 0, len(out)+len(pantryLines))
	next = append(next, out[:insertAt]...)
	next = append(next, pantryLines...)
	next = append(next, out[insertAt:]...)
	return next
}

func nutritionBasketLines(ledger NutritionLedger) []string {
	if ledger.Status == NutritionLedgerNotRun {
		return nil
	}
	lines := []string{
		"# Nutrition evidence: " + string(ledger.Status),
		fmt.Sprintf("# Known nutrition coverage: %d/%d ingredient lines", ledger.Coverage.LinesWithAnyNutrition, ledger.Coverage.IngredientLines),
	}
	if ledger.Coverage.LinesWithAlcampoLabel > 0 {
		lines = append(lines, fmt.Sprintf("# Alcampo label coverage: %d/%d ingredient lines", ledger.Coverage.LinesWithAlcampoLabel, ledger.Coverage.IngredientLines))
	}
	if ledger.Coverage.PantryMissingLines > 0 {
		lines = append(lines, fmt.Sprintf("# Pantry nutrition missing for %d pantry-covered line(s)", ledger.Coverage.PantryMissingLines))
	}
	lines = append(lines, "# Package excess is not counted as eaten by default.")
	for _, issue := range ledger.BlockingIssues {
		if issue.Message != "" {
			lines = append(lines, "# Nutrition issue: "+issue.Message)
		}
	}
	return lines
}

func servingBasketLines(plan ServingPlan, scaled *ScaledMealPlan) []string {
	var lines []string
	lines = append(lines, fmt.Sprintf("# Serving plan: %s, %.3g target / %.3g cooked serving units", plan.Status, plan.Summary.TotalTargetServingUnits, plan.Summary.TotalCookedServingUnits))
	for _, slot := range plan.Slots {
		lines = append(lines, fmt.Sprintf("# - Day %d %s: %.3g cooked servings, scale %.3g", slot.Day, slot.MealSlot, slot.CookedServingUnits, slot.ScaleFactor))
	}
	if scaled != nil {
		for _, slot := range scaled.Slots {
			for _, ingredient := range slot.Ingredients {
				if ingredient.RoundingApplied {
					lines = append(lines, fmt.Sprintf("# - %s rounded for shopping: cook %s, buy %s", ingredient.IngredientName, formatPantryQuantity(ingredient.CookingQuantity), formatPantryQuantity(ingredient.ShoppingQuantity)))
				}
			}
		}
	}
	return lines
}

func pantryBasketLines(resolution PantryResolution) []string {
	var lines []string
	var covered []string
	var partial []string
	for _, line := range resolution.Lines {
		switch line.PantryDecision {
		case PantryDecisionPantryFullAssumed, PantryDecisionPantryFullConfirmed:
			label := line.IngredientName
			if line.PantryDecision == PantryDecisionPantryFullAssumed {
				label += " (assumed pantry)"
			} else {
				label += " (confirmed pantry)"
			}
			covered = append(covered, "# - "+label)
		case PantryDecisionPantryPartialConfirmed:
			partial = append(partial, fmt.Sprintf("# - %s: %s pantry, buy %s delta", line.IngredientName, formatPantryQuantity(line.PantryAllocated), formatPantryQuantity(line.ShopRequired)))
		}
	}
	if len(covered) > 0 {
		lines = append(lines, "# Pantry items not included in basket:")
		lines = append(lines, covered...)
	}
	if len(partial) > 0 {
		lines = append(lines, "# Partial pantry coverage:")
		lines = append(lines, partial...)
	}
	return lines
}

func recipeSwapBasketSummary(plan RecipeSwapPlan, safe bool) string {
	switch plan.Status {
	case RecipeSwapAttemptedApplied:
		count := len(plan.AppliedSwaps)
		if count == 1 {
			return "Meal plan repair: 1 recipe swapped"
		}
		if count > 1 {
			return fmt.Sprintf("Meal plan repair: %d recipes swapped", count)
		}
	case RecipeSwapAttemptedFailed:
		if safe {
			return ""
		}
		return "Meal plan repair was attempted but no fully valid replacement was found."
	case RecipeSwapDisabled:
		if safe {
			return ""
		}
		return "Meal plan repair: disabled"
	}
	return ""
}

func basketOptimizationBasketSummary(plan BasketOptimizationPlan, safe bool) string {
	changed := 0
	for _, decision := range plan.Decisions {
		if decision.Changed {
			changed++
		}
	}
	switch plan.Status {
	case BasketOptimizationApplied:
		return fmt.Sprintf("Basket optimization: applied %d product switch(es); final readiness %s; safe_to_build=%t", changed, strings.ToUpper(plan.Validation.FinalReadinessStatus), safe)
	case BasketOptimizationNoImprovement:
		return fmt.Sprintf("Basket optimization: no improvement found; final readiness %s; safe_to_build=%t", strings.ToUpper(plan.Validation.FinalReadinessStatus), safe)
	case BasketOptimizationFailed:
		return "Basket optimization: discarded because validation failed or readiness would regress"
	case BasketOptimizationSkipped:
		return "Basket optimization: skipped"
	default:
		if plan.Status != "" {
			return "Basket optimization: " + string(plan.Status)
		}
		return ""
	}
}

func formatSignedCents(cents int) string {
	sign := "+"
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return sign + money.FormatAmount(int64(cents))
}

func ReadinessSummaryLine(gate ReadinessGate) string {
	return fmt.Sprintf("readiness\tstatus=%s\tsafe=%t\tcook=%t\tcook_status=%s\texit=%d", gate.Status, gate.SafeToBuild, gate.SafeToCook, firstNonEmptyString(gate.CookReadinessStatus, "-"), gate.ExitCode)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
