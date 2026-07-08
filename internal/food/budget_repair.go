package food

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/wachtermar/carrito/internal/alcampo"
)

type BudgetRepairOptions struct {
	Enabled         bool
	Policy          BudgetRepairPolicy
	Profile         Profile
	SelectionPolicy string
	SearchLimit     int
	Meals           []string
	LedgerOptions   QuantityLedgerOptions
	RecoveryOptions RecoveryOptions
	ReadinessPolicy ReadinessPolicy
	Source          string
}

func DefaultBudgetRepairPolicy(enabled bool) BudgetRepairPolicy {
	return BudgetRepairPolicy{
		Enabled:                          enabled,
		AllowProductSwitches:             true,
		AllowBudgetRecipeSwap:            false,
		MaxProductSwitches:               4,
		MaxRecipeSwaps:                   1,
		MaxCandidatesPerDriver:           6,
		MaxTotalChecks:                   30,
		MinSavingsCentsToApply:           50,
		ProtectPinnedRecipes:             true,
		RequireNoReadinessRegression:     true,
		RequireNoRecipeQualityRegression: true,
		PreserveDietaryRules:             true,
		PreserveAllergenRules:            true,
		DealAware:                        true,
	}
}

func RepairFoodRunBudget(ctx context.Context, artifact FoodRunArtifact, client *alcampo.Client, opts BudgetRepairOptions) (FoodRunArtifact, BudgetRepairPlan, error) {
	opts = normalizeBudgetRepairOptions(opts)
	artifact = RefreshFoodRunArtifact(artifact, opts.Meals)
	baselineReport := BuildBudgetDealReport(artifact)
	baselineGate := EvaluateReadinessGate(&artifact, budgetRepairReadinessPolicy(opts.ReadinessPolicy))
	plan := BudgetRepairPlan{
		SchemaVersion:            "1",
		Status:                   BudgetRepairNotRun,
		GeneratedAt:              nowStamp(),
		Policy:                   opts.Policy,
		Trigger:                  budgetRepairTrigger(baselineReport, opts.Source),
		MealPlanFingerprint:      artifact.MealPlanFingerprint,
		BaselineProductSelection: artifact.ProductSelectionFingerprint,
		Baseline:                 budgetRepairSummaryFromReport(baselineReport, artifact),
		Final:                    budgetRepairSummaryFromReport(baselineReport, artifact),
		CostDrivers:              budgetCostDrivers(artifact, baselineReport),
	}
	artifact.PreBudgetRepairBudgetDealReport = &baselineReport
	artifact.PreBudgetRepairReadinessGate = &baselineGate
	if !opts.Enabled || !opts.Policy.Enabled {
		plan.Status = BudgetRepairDisabled
		plan.Warnings = append(plan.Warnings, BudgetRepairIssue{Code: "disabled", Severity: "info", Message: "Budget repair was disabled."})
		return AttachBudgetRepairPlan(artifact, plan), plan, nil
	}
	if !budgetRepairShouldRun(baselineReport) {
		plan.Status = BudgetRepairNotNeeded
		plan.Warnings = append(plan.Warnings, BudgetRepairIssue{Code: "not_needed", Severity: "info", Message: "Budget repair was not needed because the baseline budget report is already reportable and within budget."})
		return AttachBudgetRepairPlan(artifact, plan), plan, nil
	}
	if !opts.Policy.AllowProductSwitches {
		plan.Status = BudgetRepairAttemptedFailed
		plan.BlockingIssues = append(plan.BlockingIssues, BudgetRepairIssue{Code: "product_switches_disabled", Severity: "blocking", Message: "Budget repair could not apply product switches because the policy disables them.", Remediation: "Rerun with --allow-budget-product-switches if equivalent cheaper products are acceptable."})
		return AttachBudgetRepairPlan(artifact, plan), plan, nil
	}
	if client == nil {
		plan.Status = BudgetRepairSkippedUnrepairable
		plan.BlockingIssues = append(plan.BlockingIssues, BudgetRepairIssue{Code: "client_missing", Severity: "blocking", Message: "Budget repair requires an Alcampo client to search validated alternatives.", Remediation: "Rerun budget repair in a live or replay-backed food run."})
		return AttachBudgetRepairPlan(artifact, plan), plan, nil
	}

	next, optimizationPlan, err := OptimizeBasket(ctx, artifact, client, BudgetRepairBasketOptimizationOptions(opts))
	if err != nil {
		plan.Status = BudgetRepairAttemptedFailed
		plan.BlockingIssues = append(plan.BlockingIssues, BudgetRepairIssue{Code: "product_repair_error", Severity: "blocking", Message: err.Error(), Remediation: "Review product search and retry budget repair."})
		return AttachBudgetRepairPlan(artifact, plan), plan, err
	}
	plan.Attempts = append(plan.Attempts, budgetRepairAttemptFromOptimization(optimizationPlan))
	for _, rejected := range optimizationPlan.RejectedCandidates {
		plan.RejectedCandidates = append(plan.RejectedCandidates, BudgetRepairRejectedCandidate{
			ID:           firstNonEmptyString(rejected.ProductID, normalizeKey(rejected.ProductName)),
			Type:         "product_switch",
			CostDriverID: budgetDriverIDForIngredient(plan.CostDrivers, rejected.IngredientKey),
			ProductID:    rejected.ProductID,
			ProductName:  rejected.ProductName,
			RejectCodes:  append([]string(nil), rejected.RejectCodes...),
			RejectReason: rejected.RejectReason,
		})
	}
	if optimizationPlan.Status == BasketOptimizationFailed {
		plan.Status = BudgetRepairDiscardedRegression
		plan.Warnings = append(plan.Warnings, BudgetRepairIssue{Code: "readiness_regression", Severity: "warning", Message: "Budget repair discarded product switches because validation would regress readiness."})
		return AttachBudgetRepairPlan(artifact, plan), plan, nil
	}
	if optimizationPlan.Status != BasketOptimizationApplied {
		plan.Status = BudgetRepairAttemptedFailed
		plan.Warnings = append(plan.Warnings, BudgetRepairIssue{Code: "no_valid_product_repair", Severity: "warning", Message: "No validated cheaper product switch was found."})
		return AttachBudgetRepairPlan(artifact, plan), plan, nil
	}

	finalReport := BuildBudgetDealReport(next)
	plan.FinalProductSelection = next.ProductSelectionFingerprint
	plan.Final = budgetRepairSummaryFromReport(finalReport, next)
	plan.AppliedDecisions = budgetRepairDecisionsFromOptimization(optimizationPlan, baselineReport, finalReport, plan.CostDrivers)
	if finalReport.BudgetStatus == BudgetStatusWithinBudget && finalReport.SafeToReportBudget {
		plan.Status = BudgetRepairAttemptedApplied
	} else {
		plan.Status = BudgetRepairAttemptedFailed
		plan.Warnings = append(plan.Warnings, BudgetRepairIssue{Code: "still_over_budget", Severity: "warning", Message: "Product-level budget repair applied validated changes, but the final budget report is still not within budget.", Remediation: "Reduce meal scope, approve a higher budget, or allow explicit budget recipe swaps."})
	}
	next.PreBudgetRepairBudgetDealReport = &baselineReport
	next.PreBudgetRepairReadinessGate = &baselineGate
	next = AttachBudgetRepairPlan(next, plan)
	return next, *next.BudgetRepairPlan, nil
}

func BudgetRepairBasketOptimizationOptions(opts BudgetRepairOptions) BasketOptimizationOptions {
	policy := BasketOptimizationPolicy{
		Enabled:                        true,
		Objective:                      BasketObjectiveLowestPrice,
		MaxCandidatesPerIngredient:     opts.Policy.MaxCandidatesPerDriver,
		MaxSearchesPerIngredient:       2,
		MaxTotalCandidateChecks:        opts.Policy.MaxTotalChecks,
		MaxProductSwitches:             opts.Policy.MaxProductSwitches,
		DealAware:                      opts.Policy.DealAware,
		StrictQuantity:                 opts.LedgerOptions.StrictQuantity,
		AllowEstimatedVariableWeight:   opts.LedgerOptions.AllowEstimatedVariableWeight,
		AllowLowConfidenceBasket:       opts.LedgerOptions.AllowLowConfidenceBasket,
		RequireNoReadinessRegression:   opts.Policy.RequireNoReadinessRegression,
		MinSavingsCentsToSwitch:        opts.Policy.MinSavingsCentsToApply,
		MinWasteReductionRatioToSwitch: 0.2,
		PreferProductImages:            true,
		PreferNutritionLabels:          true,
	}
	return BasketOptimizationOptions{
		Enabled:         true,
		Policy:          policy,
		Profile:         opts.Profile,
		SelectionPolicy: opts.SelectionPolicy,
		SearchLimit:     opts.SearchLimit,
		Meals:           opts.Meals,
		LedgerOptions:   opts.LedgerOptions,
		RecoveryOptions: opts.RecoveryOptions,
		ReadinessPolicy: budgetRepairReadinessPolicy(opts.ReadinessPolicy),
	}
}

func AttachBudgetRepairPlan(artifact FoodRunArtifact, plan BudgetRepairPlan) FoodRunArtifact {
	plan.MealPlanFingerprint = firstNonEmptyString(plan.MealPlanFingerprint, artifact.MealPlanFingerprint, MealPlanFingerprint(artifact.MealPlan))
	plan.FinalProductSelection = firstNonEmptyString(plan.FinalProductSelection, artifact.ProductSelectionFingerprint, ProductSelectionFingerprint(artifact.Shop))
	plan.FinalProductEvidence = firstNonEmptyString(plan.FinalProductEvidence, artifact.ProductEvidenceFingerprint)
	plan.BudgetRepairFingerprint = BudgetRepairFingerprint(plan)
	artifact.BudgetRepairPlan = &plan
	artifact.BudgetRepairFingerprint = plan.BudgetRepairFingerprint
	return artifact
}

func FinalizeBudgetRepairPlan(artifact FoodRunArtifact, gate ReadinessGate) FoodRunArtifact {
	if artifact.BudgetRepairPlan == nil {
		return artifact
	}
	plan := *artifact.BudgetRepairPlan
	report := artifact.BudgetDealReport
	if report == nil {
		built := BuildBudgetDealReport(artifact)
		report = &built
	}
	plan.Final = budgetRepairSummaryFromReport(*report, artifact)
	plan.FinalProductSelection = firstNonEmptyString(artifact.ProductSelectionFingerprint, ProductSelectionFingerprint(artifact.Shop))
	plan.FinalProductEvidence = artifact.ProductEvidenceFingerprint
	validation := BudgetRepairValidation{
		SafeToBuild:           gate.SafeToBuild,
		SafeToCook:            gate.SafeToCook,
		SafeToUseRecipes:      gate.SafeToUseRecipes,
		SafeToReportNutrition: gate.SafeToReportNutrition,
		SafeToReportBudget:    gate.SafeToReportBudget,
		SafeToMeetBudget:      report.BudgetStatus == BudgetStatusWithinBudget && report.SafeToReportBudget,
		BudgetStatus:          report.BudgetStatus,
		ReadinessStatus:       string(gate.Status),
		AuditWouldPass:        true,
	}
	for i := range plan.Attempts {
		plan.Attempts[i].Validation = validation
	}
	if len(plan.Attempts) == 0 && plan.Status != BudgetRepairNotNeeded && plan.Status != BudgetRepairDisabled {
		plan.Attempts = append(plan.Attempts, BudgetRepairAttempt{
			ID:         "budget-repair-final",
			Type:       "final_validation",
			Status:     "validated",
			Reason:     "final run state was validated after budget repair planning",
			Validation: validation,
		})
	}
	if plan.Status == BudgetRepairAttemptedApplied && !validation.SafeToMeetBudget {
		plan.Status = BudgetRepairAttemptedFailed
		plan.Warnings = append(plan.Warnings, BudgetRepairIssue{Code: "final_budget_not_met", Severity: "warning", Message: "Final budget/deal evidence is still not within budget after rebuild.", Remediation: "Treat the budget repair as diagnostic and do not claim the plan meets budget."})
	}
	return AttachBudgetRepairPlan(artifact, plan)
}

func BudgetRepairFingerprint(plan BudgetRepairPlan) string {
	clone := plan
	clone.GeneratedAt = ""
	clone.BudgetRepairFingerprint = ""
	data, err := json.Marshal(clone)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func normalizeBudgetRepairOptions(opts BudgetRepairOptions) BudgetRepairOptions {
	if opts.Policy == (BudgetRepairPolicy{}) {
		opts.Policy = DefaultBudgetRepairPolicy(opts.Enabled)
	}
	if opts.Enabled {
		opts.Policy.Enabled = true
	}
	if opts.Policy.MaxProductSwitches <= 0 {
		opts.Policy.MaxProductSwitches = 4
	}
	if opts.Policy.MaxRecipeSwaps <= 0 {
		opts.Policy.MaxRecipeSwaps = 1
	}
	if opts.Policy.MaxCandidatesPerDriver <= 0 {
		opts.Policy.MaxCandidatesPerDriver = 6
	}
	if opts.Policy.MaxTotalChecks <= 0 {
		opts.Policy.MaxTotalChecks = 30
	}
	if opts.Policy.MinSavingsCentsToApply <= 0 {
		opts.Policy.MinSavingsCentsToApply = 50
	}
	opts.Policy.RequireNoReadinessRegression = true
	opts.Policy.RequireNoRecipeQualityRegression = true
	opts.Policy.PreserveDietaryRules = true
	opts.Policy.PreserveAllergenRules = true
	if opts.Source == "" {
		opts.Source = "cli"
	}
	return opts
}

func budgetRepairReadinessPolicy(policy ReadinessPolicy) ReadinessPolicy {
	policy.RequireNutritionReady = false
	return policy
}

func budgetRepairShouldRun(report BudgetDealReport) bool {
	if report.BudgetStatus == BudgetStatusOverBudget || report.Status == BudgetDealOverBudget {
		return true
	}
	if report.BudgetStatus == BudgetStatusUnknownTotal || report.Summary.SelectedProductsMissingPrice > 0 {
		return true
	}
	return false
}

func budgetRepairTrigger(report BudgetDealReport, source string) BudgetRepairTrigger {
	trigger := BudgetRepairTrigger{Reason: "not_needed", Source: firstNonEmptyString(source, "cli")}
	switch report.BudgetStatus {
	case BudgetStatusOverBudget:
		trigger.Reason = "over_budget"
	case BudgetStatusUnknownTotal:
		trigger.Reason = "budget_unknown_due_to_price_gaps"
	case BudgetStatusUnparseable:
		trigger.Reason = "budget_required_not_met"
	default:
		if !report.SafeToReportBudget && report.BudgetRaw != "" {
			trigger.Reason = "budget_required_not_met"
		}
	}
	if report.Budget != nil {
		cents := int(report.Budget.Cents)
		trigger.BudgetTotalCents = &cents
	}
	if report.EstimatedTotal.Cents > 0 {
		cents := int(report.EstimatedTotal.Cents)
		trigger.BaselineComparedAmountCents = &cents
	}
	if report.Delta != nil {
		cents := int(report.Delta.Cents)
		trigger.BaselineDifferenceCents = &cents
	}
	return trigger
}

func budgetRepairSummaryFromReport(report BudgetDealReport, artifact FoodRunArtifact) BudgetRepairSummary {
	summary := BudgetRepairSummary{
		BudgetStatus:       firstNonEmptyString(report.BudgetStatus, BudgetStatusNotSet),
		SafeToReportBudget: report.SafeToReportBudget,
		SafeToMeetBudget:   report.BudgetStatus == BudgetStatusWithinBudget && report.SafeToReportBudget,
		PriceCoverageRatio: report.Summary.PriceCoverageRatio,
		MissingPriceLines:  report.Summary.SelectedProductsMissingPrice,
		ProductLineCount:   report.Summary.SelectedProductCount,
		RecipeCount:        activeRecipeCount(artifact.MealPlan),
	}
	if report.EstimatedTotal.Cents > 0 {
		cents := int(report.EstimatedTotal.Cents)
		summary.EstimatedBasketSubtotalCents = &cents
	}
	if report.Summary.ConsumedCostCents > 0 {
		cents := report.Summary.ConsumedCostCents
		summary.KnownConsumedCostCents = &cents
	}
	if report.Summary.PackageExcessCostCents > 0 {
		cents := report.Summary.PackageExcessCostCents
		summary.PackageExcessCostCents = &cents
	}
	if report.Summary.RecognizedDealSavingsCents > 0 {
		cents := report.Summary.RecognizedDealSavingsCents
		summary.ReliableOfferSavingsCents = &cents
	}
	return summary
}

func budgetCostDrivers(artifact FoodRunArtifact, report BudgetDealReport) []BudgetCostDriver {
	var drivers []BudgetCostDriver
	total := int(report.EstimatedTotal.Cents)
	for i, selected := range artifact.Shop.SelectedProducts {
		if selected.Error != "" {
			continue
		}
		cents := int(selected.LineTotal.Cents)
		if cents <= 0 && selected.Product.Price.Cents > 0 {
			count := selected.PackageCount
			if count <= 0 {
				count = 1
			}
			cents = int(selected.Product.Price.Cents) * count
		}
		if cents <= 0 {
			drivers = append(drivers, BudgetCostDriver{
				ID:             fmt.Sprintf("missing-price-%03d", i+1),
				Type:           "missing_price",
				ProductID:      selected.Product.ID,
				ProductName:    selected.Product.Name,
				IngredientKey:  normalizeKey(selected.Ingredient.Name),
				IngredientName: selected.Ingredient.Name,
				Reason:         "selected product has no usable price evidence",
			})
			continue
		}
		percent := 0.0
		if total > 0 {
			percent = float64(cents) / float64(total)
		}
		drivers = append(drivers, BudgetCostDriver{
			ID:                     fmt.Sprintf("product-line-%03d", i+1),
			Type:                   "product_line",
			ProductID:              selected.Product.ID,
			ProductName:            selected.Product.Name,
			IngredientKey:          normalizeKey(selected.Ingredient.Name),
			IngredientName:         selected.Ingredient.Name,
			EstimatedCostCents:     intPtr(cents),
			PercentOfKnownSubtotal: budgetFloatPtr(percent),
			Reason:                 "selected product line is a current budget cost driver",
		})
	}
	if report.Summary.PackageExcessCostCents > 0 {
		drivers = append(drivers, BudgetCostDriver{
			ID:                     "package-excess",
			Type:                   "package_excess",
			PackageExcessCostCents: intPtr(report.Summary.PackageExcessCostCents),
			Reason:                 "some purchased package cost is excess beyond the planned consumed quantities",
		})
	}
	sort.SliceStable(drivers, func(i, j int) bool {
		left := 0
		right := 0
		if drivers[i].EstimatedCostCents != nil {
			left = *drivers[i].EstimatedCostCents
		}
		if drivers[i].PackageExcessCostCents != nil {
			left += *drivers[i].PackageExcessCostCents
		}
		if drivers[j].EstimatedCostCents != nil {
			right = *drivers[j].EstimatedCostCents
		}
		if drivers[j].PackageExcessCostCents != nil {
			right += *drivers[j].PackageExcessCostCents
		}
		if left != right {
			return left > right
		}
		return drivers[i].ID < drivers[j].ID
	})
	for i := range drivers {
		drivers[i].Rank = i + 1
	}
	return drivers
}

func budgetRepairAttemptFromOptimization(plan BasketOptimizationPlan) BudgetRepairAttempt {
	attempt := BudgetRepairAttempt{
		ID:     "product-repair-001",
		Type:   "product_switch",
		Status: "no_valid_candidate",
		Reason: "no validated cheaper product switch was applied",
		Validation: BudgetRepairValidation{
			SafeToBuild:      plan.Validation.FinalSafeToBuild,
			ReadinessStatus:  plan.Validation.FinalReadinessStatus,
			BudgetStatus:     BudgetStatusNotSet,
			SafeToMeetBudget: false,
		},
	}
	for _, pool := range plan.CandidatePools {
		attempt.CandidateCount += len(pool.Candidates)
	}
	switch plan.Status {
	case BasketOptimizationApplied:
		attempt.Status = "applied"
		attempt.Reason = "validated product switches were applied"
	case BasketOptimizationFailed:
		attempt.Status = "rejected_regression"
		attempt.Reason = "candidate switches were discarded because readiness would regress"
	case BasketOptimizationSkipped:
		attempt.Status = "skipped_policy"
		attempt.Reason = "product optimization was skipped"
	}
	for _, decision := range plan.Decisions {
		if decision.Changed {
			attempt.AppliedDecisionID = "budget-product-" + normalizeKey(firstNonEmptyString(decision.IngredientKey, decision.SelectedProductID))
			break
		}
	}
	return attempt
}

func budgetRepairDecisionsFromOptimization(plan BasketOptimizationPlan, before, after BudgetDealReport, drivers []BudgetCostDriver) []BudgetRepairDecision {
	var decisions []BudgetRepairDecision
	for _, decision := range plan.Decisions {
		if !decision.Changed {
			continue
		}
		savings := -decision.CostDeltaCents
		decisions = append(decisions, BudgetRepairDecision{
			ID:                    "budget-product-" + normalizeKey(firstNonEmptyString(decision.IngredientKey, decision.SelectedProductID)),
			Type:                  "product_switch",
			CostDriverID:          budgetDriverIDForIngredient(drivers, decision.IngredientKey),
			BeforeLabel:           firstNonEmptyString(decision.BaselineProductName, decision.BaselineProductID, decision.IngredientName),
			AfterLabel:            firstNonEmptyString(decision.SelectedProductName, decision.SelectedProductID, decision.IngredientName),
			EstimatedSavingsCents: intPtr(maxInt(0, savings)),
			BudgetStatusBefore:    before.BudgetStatus,
			BudgetStatusAfter:     after.BudgetStatus,
			ReadinessImpact:       firstNonEmptyString(decision.ReadinessImpact, "preserved"),
			Explanation:           decision.Reason,
		})
	}
	return decisions
}

func budgetDriverIDForIngredient(drivers []BudgetCostDriver, ingredientKey string) string {
	ingredientKey = normalizeKey(ingredientKey)
	for _, driver := range drivers {
		if normalizeKey(driver.IngredientKey) == ingredientKey {
			return driver.ID
		}
	}
	return ""
}

func intPtr(value int) *int {
	return &value
}

func budgetFloatPtr(value float64) *float64 {
	return &value
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
