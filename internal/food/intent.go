package food

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"sort"
	"strings"
)

func LoadMealRunIntent(path string) (MealRunIntent, error) {
	var intent MealRunIntent
	data, err := os.ReadFile(path)
	if err != nil {
		return intent, err
	}
	if err := json.Unmarshal(data, &intent); err != nil {
		return intent, err
	}
	return NormalizeMealRunIntent(intent), nil
}

func NormalizeMealRunIntent(intent MealRunIntent) MealRunIntent {
	if strings.TrimSpace(intent.SchemaVersion) == "" {
		intent.SchemaVersion = "1"
	}
	if strings.TrimSpace(intent.Source) == "" {
		intent.Source = "cli"
	}
	if intent.ClaimPolicy == (IntentClaimPolicy{}) {
		intent.ClaimPolicy = IntentClaimPolicy{
			RequireAllHardConstraints:  true,
			UnknownHardConstraintFails: true,
			AllowSoftPreferenceMisses:  true,
		}
	}
	if intent.Serving != nil && intent.Serving.Tolerance <= 0 {
		intent.Serving.Tolerance = 0.01
	}
	if intent.Diet != nil && (len(intent.Diet.ExcludedIngredients) > 0 || len(intent.Diet.ExcludedAllergens) > 0) {
		intent.Diet.RequireDeterministicCheck = true
	}
	intent.IntentFingerprint = IntentFingerprint(intent)
	return intent
}

func IntentFingerprint(intent MealRunIntent) string {
	clone := intent
	clone.IntentFingerprint = ""
	data, err := json.Marshal(clone)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func ConstraintSatisfactionFingerprint(report ConstraintSatisfactionReport) string {
	clone := report
	clone.ConstraintSatisfactionFingerprint = ""
	for i := range clone.Checks {
		clone.Checks[i].Expected = nilIfTypedNil(clone.Checks[i].Expected)
		clone.Checks[i].Actual = nilIfTypedNil(clone.Checks[i].Actual)
	}
	data, err := json.Marshal(clone)
	if err != nil {
		return ""
	}
	var canonical any
	if err := json.Unmarshal(data, &canonical); err != nil {
		return ""
	}
	data, err = json.Marshal(canonical)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func AttachMealRunIntent(artifact FoodRunArtifact, intent MealRunIntent) FoodRunArtifact {
	intent = NormalizeMealRunIntent(intent)
	artifact.MealRunIntent = &intent
	artifact.IntentFingerprint = intent.IntentFingerprint
	return artifact
}

func AttachConstraintSatisfactionReport(artifact FoodRunArtifact, report ConstraintSatisfactionReport) FoodRunArtifact {
	if report.IntentFingerprint == "" && artifact.MealRunIntent != nil {
		report.IntentFingerprint = artifact.MealRunIntent.IntentFingerprint
	}
	if report.MealPlanFingerprint == "" {
		report.MealPlanFingerprint = firstNonEmptyString(artifact.MealPlanFingerprint, MealPlanFingerprint(artifact.MealPlan))
	}
	if report.RecipeSetFingerprint == "" {
		report.RecipeSetFingerprint = firstNonEmptyString(artifact.RecipeSetFingerprint, RecipeSetFingerprint(artifact.MealPlan))
	}
	if report.ServingPlanFingerprint == "" {
		report.ServingPlanFingerprint = servingPlanFingerprintFromRun(&artifact)
	}
	if report.ScaledMealPlanFingerprint == "" {
		report.ScaledMealPlanFingerprint = scaledMealPlanFingerprintFromRun(&artifact)
	}
	if report.PantryResolutionFingerprint == "" {
		report.PantryResolutionFingerprint = pantryResolutionFingerprintFromRun(&artifact)
	}
	if report.ProductSelectionFingerprint == "" {
		report.ProductSelectionFingerprint = firstNonEmptyString(artifact.ProductSelectionFingerprint, ProductSelectionFingerprint(artifact.Shop))
	}
	if report.NutritionLedgerFingerprint == "" {
		report.NutritionLedgerFingerprint = artifact.NutritionLedgerFingerprint
	}
	if report.BudgetDealFingerprint == "" {
		report.BudgetDealFingerprint = artifact.BudgetDealFingerprint
	}
	if report.BudgetRepairFingerprint == "" {
		report.BudgetRepairFingerprint = artifact.BudgetRepairFingerprint
	}
	report.ConstraintSatisfactionFingerprint = ConstraintSatisfactionFingerprint(report)
	artifact.ConstraintSatisfactionReport = &report
	artifact.ConstraintSatisfactionFingerprint = report.ConstraintSatisfactionFingerprint
	return artifact
}

func BuildConstraintSatisfactionReport(run FoodRunArtifact, intent MealRunIntent) ConstraintSatisfactionReport {
	intent = NormalizeMealRunIntent(intent)
	report := ConstraintSatisfactionReport{
		SchemaVersion:               "1",
		Status:                      ConstraintSatisfactionUnknown,
		IntentFingerprint:           intent.IntentFingerprint,
		MealPlanFingerprint:         firstNonEmptyString(run.MealPlanFingerprint, MealPlanFingerprint(run.MealPlan)),
		RecipeSetFingerprint:        firstNonEmptyString(run.RecipeSetFingerprint, RecipeSetFingerprint(run.MealPlan)),
		ServingPlanFingerprint:      servingPlanFingerprintFromRun(&run),
		ScaledMealPlanFingerprint:   scaledMealPlanFingerprintFromRun(&run),
		PantryResolutionFingerprint: pantryResolutionFingerprintFromRun(&run),
		ProductSelectionFingerprint: firstNonEmptyString(run.ProductSelectionFingerprint, ProductSelectionFingerprint(run.Shop)),
		NutritionLedgerFingerprint:  run.NutritionLedgerFingerprint,
		BudgetDealFingerprint:       run.BudgetDealFingerprint,
		BudgetRepairFingerprint:     run.BudgetRepairFingerprint,
	}
	addPlanCoverageChecks(&report, run, intent)
	addServingChecks(&report, run, intent)
	addBudgetChecks(&report, run, intent)
	addNutritionChecks(&report, run, intent)
	addDietChecks(&report, run, intent)
	addCookingChecks(&report, run, intent)
	addRecipePDFChecks(&report, run, intent)
	addPantryIntentChecks(&report, run, intent)
	addGenericIntentChecks(&report, intent)
	finalizeConstraintSatisfactionReport(&report, intent.ClaimPolicy)
	report.ConstraintSatisfactionFingerprint = ConstraintSatisfactionFingerprint(report)
	return report
}

func addPlanCoverageChecks(report *ConstraintSatisfactionReport, run FoodRunArtifact, intent MealRunIntent) {
	if intent.PlanCoverage == nil {
		return
	}
	coverage := intent.PlanCoverage
	if coverage.ExpectedDays != nil {
		actual := len(run.MealPlan.Days)
		addConstraintCheck(report, ConstraintCheck{
			ID:           "plan.expected_days",
			Type:         "plan_coverage",
			Scope:        "mealplan",
			Required:     true,
			Status:       statusFromBool(actual == *coverage.ExpectedDays),
			Severity:     severityFromBool(actual == *coverage.ExpectedDays, true),
			EvidenceRefs: []EvidenceRef{{Artifact: FoodArtifactRun, JSONPath: "$.mealplan.days", Label: "meal plan days"}},
			Expected:     *coverage.ExpectedDays,
			Actual:       actual,
			Message:      fmt.Sprintf("Meal plan has %d day(s); intent expected %d.", actual, *coverage.ExpectedDays),
			Remediation:  "Regenerate the plan with the requested day count.",
		})
	}
	if len(coverage.ExpectedMealSlots) > 0 {
		missing := missingMealSlots(run.MealPlan, coverage.ExpectedMealSlots)
		addConstraintCheck(report, ConstraintCheck{
			ID:           "plan.expected_meal_slots",
			Type:         "plan_coverage",
			Scope:        "mealplan",
			Required:     true,
			Status:       statusFromBool(len(missing) == 0),
			Severity:     severityFromBool(len(missing) == 0, true),
			EvidenceRefs: []EvidenceRef{{Artifact: FoodArtifactRun, JSONPath: "$.mealplan.days[].meals", Label: "meal slots"}},
			Expected:     normalizeListForReport(coverage.ExpectedMealSlots),
			Actual:       actualMealSlots(run.MealPlan),
			Message:      mealSlotCoverageMessage(missing),
			Remediation:  "Regenerate the plan with the requested meal slots for every planned day.",
		})
	}
	if coverage.ExpectedMealCount != nil {
		actual := activeRecipeCount(run.MealPlan)
		addConstraintCheck(report, ConstraintCheck{
			ID:           "plan.expected_meal_count",
			Type:         "plan_coverage",
			Scope:        "mealplan",
			Required:     true,
			Status:       statusFromBool(actual == *coverage.ExpectedMealCount),
			Severity:     severityFromBool(actual == *coverage.ExpectedMealCount, true),
			EvidenceRefs: []EvidenceRef{{Artifact: FoodArtifactRun, JSONPath: "$.mealplan.days[].meals", Label: "active meals"}},
			Expected:     *coverage.ExpectedMealCount,
			Actual:       actual,
			Message:      fmt.Sprintf("Meal plan has %d meal slot(s); intent expected %d.", actual, *coverage.ExpectedMealCount),
			Remediation:  "Regenerate the plan with the requested number of meals.",
		})
	}
}

func addServingChecks(report *ConstraintSatisfactionReport, run FoodRunArtifact, intent MealRunIntent) {
	if intent.Serving == nil {
		return
	}
	serving := intent.Serving
	tolerance := serving.Tolerance
	if tolerance <= 0 {
		tolerance = 0.01
	}
	actualTotal, evidence := totalServingUnits(run)
	if serving.TotalServingUnits != nil {
		status := ConstraintCheckUnknown
		if evidence {
			status = statusFromBool(math.Abs(actualTotal-*serving.TotalServingUnits) <= tolerance)
		}
		addConstraintCheck(report, ConstraintCheck{
			ID:           "serving.total_units",
			Type:         "serving",
			Scope:        "serving_plan",
			Required:     true,
			Status:       status,
			Severity:     severityFromStatus(status, true),
			EvidenceRefs: []EvidenceRef{{Artifact: FoodArtifactServingPlan, JSONPath: "$.summary.total_target_serving_units", Label: "serving plan target units"}},
			Expected:     map[string]any{"total_serving_units": *serving.TotalServingUnits, "tolerance": tolerance},
			Actual:       actualTotal,
			Message:      servingUnitsMessage(evidence, actualTotal, *serving.TotalServingUnits),
			Remediation:  "Regenerate with --servings, --adult-servings/--child-servings/--toddler-servings, or a household profile matching the intent.",
		})
	}
	adult, adultEvidence := runServingAdultUnits(run)
	child, childEvidence := runServingChildUnits(run)
	toddler, toddlerEvidence := runServingToddlerUnits(run)
	addServingPolicyCheck(report, run, "serving.adult_units", "adult_servings", serving.AdultServings, adult, adultEvidence, tolerance)
	addServingPolicyCheck(report, run, "serving.child_units", "child_servings", serving.ChildServings, child, childEvidence, tolerance)
	addServingPolicyCheck(report, run, "serving.toddler_units", "toddler_servings", serving.ToddlerServings, toddler, toddlerEvidence, tolerance)
}

func addBudgetChecks(report *ConstraintSatisfactionReport, run FoodRunArtifact, intent MealRunIntent) {
	if intent.Budget == nil {
		return
	}
	budget := intent.Budget
	budgetReady := run.ReadinessGate != nil && run.ReadinessGate.SafeToReportBudget
	if budget.RequireBudgetReady {
		addConstraintCheck(report, ConstraintCheck{
			ID:           "budget.ready",
			Type:         "budget",
			Scope:        "budget_deal_report",
			Required:     true,
			Status:       statusFromBool(budgetReady),
			Severity:     severityFromBool(budgetReady, true),
			EvidenceRefs: []EvidenceRef{{Artifact: FoodArtifactReadiness, JSONPath: "$.safe_to_report_budget", Label: "budget readiness"}},
			Expected:     true,
			Actual:       budgetReady,
			Message:      "Budget evidence must be safe to report before Hermes can claim the budget was satisfied.",
			Remediation:  "Regenerate with complete price evidence, a parseable budget, or without making a budget claim.",
		})
	}
	if budget.MaxTotalCents != nil {
		actual, ok := budgetEstimatedTotal(run)
		status := ConstraintCheckUnknown
		if ok && budgetReady {
			status = statusFromBool(actual <= *budget.MaxTotalCents)
		}
		addConstraintCheck(report, ConstraintCheck{
			ID:           "budget.max_total",
			Type:         "budget",
			Scope:        "basket",
			Required:     true,
			Status:       status,
			Severity:     severityFromStatus(status, true),
			EvidenceRefs: []EvidenceRef{{Artifact: FoodArtifactBudgetDeal, JSONPath: "$.estimated_total", Label: "budget/deal estimated total"}},
			Expected:     *budget.MaxTotalCents,
			Actual:       actual,
			Message:      moneyConstraintMessage("Estimated basket total", actual, ok, *budget.MaxTotalCents),
			Remediation:  "Repair budget, reduce meal scope, or ask the user to approve a higher budget.",
		})
	}
	if budget.MaxPerServingCents != nil {
		actualTotal, priceOK := budgetEstimatedTotal(run)
		servings, servingsOK := totalServingUnits(run)
		actual := 0
		status := ConstraintCheckUnknown
		if priceOK && servingsOK && servings > 0 && budgetReady {
			actual = int(math.Ceil(float64(actualTotal) / servings))
			status = statusFromBool(actual <= *budget.MaxPerServingCents)
		}
		addConstraintCheck(report, ConstraintCheck{
			ID:           "budget.max_per_serving",
			Type:         "budget",
			Scope:        "basket",
			Required:     true,
			Status:       status,
			Severity:     severityFromStatus(status, true),
			EvidenceRefs: []EvidenceRef{{Artifact: FoodArtifactBudgetDeal, JSONPath: "$.estimated_total", Label: "estimated total"}, {Artifact: FoodArtifactServingPlan, JSONPath: "$.summary.total_target_serving_units", Label: "target serving units"}},
			Expected:     *budget.MaxPerServingCents,
			Actual:       actual,
			Message:      moneyConstraintMessage("Estimated cost per serving unit", actual, priceOK && servingsOK && servings > 0, *budget.MaxPerServingCents),
			Remediation:  "Repair budget, reduce meal scope, or lower product costs.",
		})
	}
	if budget.MaxPerMealCents != nil {
		actualTotal, priceOK := budgetEstimatedTotal(run)
		meals := activeRecipeCount(run.MealPlan)
		actual := 0
		status := ConstraintCheckUnknown
		if priceOK && meals > 0 && budgetReady {
			actual = int(math.Ceil(float64(actualTotal) / float64(meals)))
			status = statusFromBool(actual <= *budget.MaxPerMealCents)
		}
		addConstraintCheck(report, ConstraintCheck{
			ID:           "budget.max_per_meal",
			Type:         "budget",
			Scope:        "basket",
			Required:     true,
			Status:       status,
			Severity:     severityFromStatus(status, true),
			EvidenceRefs: []EvidenceRef{{Artifact: FoodArtifactBudgetDeal, JSONPath: "$.estimated_total", Label: "estimated total"}, {Artifact: FoodArtifactRun, JSONPath: "$.mealplan.days[].meals", Label: "meal count"}},
			Expected:     *budget.MaxPerMealCents,
			Actual:       actual,
			Message:      moneyConstraintMessage("Estimated cost per meal", actual, priceOK && meals > 0, *budget.MaxPerMealCents),
			Remediation:  "Repair budget, reduce meal scope, or lower product costs.",
		})
	}
	if budget.MinPriceLineCoverage != nil {
		actual := 0.0
		if run.BudgetDealReport != nil {
			actual = run.BudgetDealReport.Summary.PriceCoverageRatio
		}
		ok := run.BudgetDealReport != nil && actual >= *budget.MinPriceLineCoverage
		addConstraintCheck(report, ConstraintCheck{
			ID:           "budget.price_line_coverage",
			Type:         "budget",
			Scope:        "budget_deal_report",
			Required:     true,
			Status:       statusFromBool(ok),
			Severity:     severityFromBool(ok, true),
			EvidenceRefs: []EvidenceRef{{Artifact: FoodArtifactBudgetDeal, JSONPath: "$.summary.price_coverage_ratio", Label: "price coverage"}},
			Expected:     *budget.MinPriceLineCoverage,
			Actual:       actual,
			Message:      fmt.Sprintf("Price line coverage is %.0f%%; intent requires at least %.0f%%.", actual*100, *budget.MinPriceLineCoverage*100),
			Remediation:  "Regenerate with more complete product price evidence before making budget claims.",
		})
	}
}

func addNutritionChecks(report *ConstraintSatisfactionReport, run FoodRunArtifact, intent MealRunIntent) {
	if intent.Nutrition == nil {
		return
	}
	nutrition := intent.Nutrition
	nutritionReady := run.ReadinessGate != nil && run.ReadinessGate.SafeToReportNutrition
	if nutrition.RequireNutritionReady {
		addConstraintCheck(report, ConstraintCheck{
			ID:           "nutrition.ready",
			Type:         "nutrition",
			Scope:        "nutrition_ledger",
			Required:     true,
			Status:       statusFromBool(nutritionReady),
			Severity:     severityFromBool(nutritionReady, true),
			EvidenceRefs: []EvidenceRef{{Artifact: FoodArtifactReadiness, JSONPath: "$.safe_to_report_nutrition", Label: "nutrition readiness"}},
			Expected:     true,
			Actual:       nutritionReady,
			Message:      "Nutrition evidence must meet the requested coverage before Hermes can claim nutrition targets were satisfied.",
			Remediation:  "Regenerate with more label, recipe, or pantry nutrition evidence, or lower the requested coverage thresholds.",
		})
	}
	if nutrition.MinLineCoverage != nil {
		actual := 0.0
		if run.NutritionLedger != nil {
			actual = run.NutritionLedger.Coverage.LineCoverageRatio
		}
		ok := run.NutritionLedger != nil && actual >= *nutrition.MinLineCoverage
		addConstraintCheck(report, ConstraintCheck{
			ID:           "nutrition.min_line_coverage",
			Type:         "nutrition",
			Scope:        "nutrition_ledger",
			Required:     true,
			Status:       statusFromBool(ok),
			Severity:     severityFromBool(ok, true),
			EvidenceRefs: []EvidenceRef{{Artifact: FoodArtifactNutritionLedger, JSONPath: "$.coverage.line_coverage_ratio", Label: "nutrition line coverage"}},
			Expected:     *nutrition.MinLineCoverage,
			Actual:       actual,
			Message:      fmt.Sprintf("Nutrition line coverage is %.0f%%; intent requires at least %.0f%%.", actual*100, *nutrition.MinLineCoverage*100),
			Remediation:  "Use products with labels, recipe-declared nutrition, or pantry-profile nutrition evidence.",
		})
	}
	if nutrition.MinQuantityCoverage != nil {
		actual := 0.0
		ok := false
		if run.NutritionLedger != nil && run.NutritionLedger.Coverage.QuantityCoverageRatio != nil {
			actual = *run.NutritionLedger.Coverage.QuantityCoverageRatio
			ok = actual >= *nutrition.MinQuantityCoverage
		}
		addConstraintCheck(report, ConstraintCheck{
			ID:           "nutrition.min_quantity_coverage",
			Type:         "nutrition",
			Scope:        "nutrition_ledger",
			Required:     true,
			Status:       statusFromBool(ok),
			Severity:     severityFromBool(ok, true),
			EvidenceRefs: []EvidenceRef{{Artifact: FoodArtifactNutritionLedger, JSONPath: "$.coverage.quantity_coverage_ratio", Label: "nutrition quantity coverage"}},
			Expected:     *nutrition.MinQuantityCoverage,
			Actual:       actual,
			Message:      fmt.Sprintf("Nutrition quantity coverage is %.0f%%; intent requires at least %.0f%%.", actual*100, *nutrition.MinQuantityCoverage*100),
			Remediation:  "Use products with labels and convertible quantities before claiming exact nutrition coverage.",
		})
	}
	for i, target := range nutrition.Targets {
		addNutritionTargetCheck(report, run, target, i, nutritionReady)
	}
}

func addDietChecks(report *ConstraintSatisfactionReport, run FoodRunArtifact, intent MealRunIntent) {
	if intent.Diet == nil {
		return
	}
	diet := intent.Diet
	for _, excluded := range diet.ExcludedIngredients {
		excluded = strings.TrimSpace(excluded)
		if excluded == "" {
			continue
		}
		matches := intentExclusionMatches(run, excluded, false)
		ok := len(matches) == 0
		addConstraintCheck(report, ConstraintCheck{
			ID:           "diet.exclude_ingredient." + normalizeKey(excluded),
			Type:         "diet",
			Scope:        "recipe_and_products",
			Required:     true,
			Status:       statusFromBool(ok),
			Severity:     severityFromBool(ok, true),
			EvidenceRefs: exclusionEvidenceRefs(matches),
			Expected:     "absent: " + excluded,
			Actual:       matches,
			Message:      exclusionMessage(excluded, matches),
			Remediation:  "Choose recipes and products that do not contain the excluded ingredient. This deterministic check is not a medical allergen guarantee.",
		})
	}
	for _, excluded := range diet.ExcludedAllergens {
		excluded = strings.TrimSpace(excluded)
		if excluded == "" {
			continue
		}
		matches := intentExclusionMatches(run, excluded, true)
		ok := len(matches) == 0
		addConstraintCheck(report, ConstraintCheck{
			ID:           "diet.exclude_allergen." + normalizeKey(excluded),
			Type:         "diet",
			Scope:        "recipe_and_products",
			Required:     true,
			Status:       statusFromBool(ok),
			Severity:     severityFromBool(ok, true),
			EvidenceRefs: exclusionEvidenceRefs(matches),
			Expected:     "absent: " + excluded,
			Actual:       matches,
			Message:      exclusionMessage(excluded, matches),
			Remediation:  "Choose recipes and products without the excluded allergen term. This does not verify traces or factory cross-contamination.",
		})
	}
	for _, rule := range diet.DietRules {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		status := ConstraintCheckUnknown
		severity := "warning"
		required := diet.RequireDeterministicCheck
		if required {
			severity = "blocking"
		}
		addConstraintCheck(report, ConstraintCheck{
			ID:          "diet.rule." + normalizeKey(rule),
			Type:        "diet",
			Scope:       "mealplan",
			Required:    required,
			Status:      status,
			Severity:    severity,
			Expected:    rule,
			Message:     "Diet rule " + rule + " is not fully machine-verifiable in v1 unless Hermes maps it to explicit exclusions.",
			Remediation: "Map diet rules to explicit excluded ingredients/allergens before claiming deterministic satisfaction.",
		})
	}
}

func addCookingChecks(report *ConstraintSatisfactionReport, run FoodRunArtifact, intent MealRunIntent) {
	if intent.Cooking == nil {
		return
	}
	cooking := intent.Cooking
	for _, ref := range activeRecipeRefs(run.MealPlan) {
		total, hasTotal := recipeTotalMinutes(ref.Recipe)
		if cooking.MaxTotalMinutes != nil {
			status := ConstraintCheckUnknown
			if hasTotal {
				status = statusFromBool(total <= *cooking.MaxTotalMinutes)
			}
			addConstraintCheck(report, ConstraintCheck{
				ID:           fmt.Sprintf("cooking.max_total.day_%d.%s", ref.Day, normalizeKey(ref.MealSlot)),
				Type:         "cooking_time",
				Scope:        "recipe",
				Required:     true,
				Status:       status,
				Severity:     severityFromStatus(status, true),
				EvidenceRefs: []EvidenceRef{{Artifact: FoodArtifactRun, JSONPath: "$.mealplan.days[].meals[].recipe", Label: ref.Recipe.Title}},
				Expected:     *cooking.MaxTotalMinutes,
				Actual:       total,
				Message:      cookingTimeMessage(ref.Recipe.Title, total, hasTotal, *cooking.MaxTotalMinutes),
				Remediation:  "Use a recipe with reliable prep/cook/step minutes or choose a faster recipe.",
			})
		}
		active := ref.Recipe.PrepMinutes
		hasActive := active > 0
		if cooking.MaxActiveMinutes != nil {
			status := ConstraintCheckUnknown
			if hasActive {
				status = statusFromBool(active <= *cooking.MaxActiveMinutes)
			}
			addConstraintCheck(report, ConstraintCheck{
				ID:           fmt.Sprintf("cooking.max_active.day_%d.%s", ref.Day, normalizeKey(ref.MealSlot)),
				Type:         "cooking_time",
				Scope:        "recipe",
				Required:     true,
				Status:       status,
				Severity:     severityFromStatus(status, true),
				EvidenceRefs: []EvidenceRef{{Artifact: FoodArtifactRun, JSONPath: "$.mealplan.days[].meals[].recipe.prep_minutes", Label: ref.Recipe.Title}},
				Expected:     *cooking.MaxActiveMinutes,
				Actual:       active,
				Message:      activeTimeMessage(ref.Recipe.Title, active, hasActive, *cooking.MaxActiveMinutes),
				Remediation:  "Use a recipe with reliable active/prep minutes or choose a faster recipe.",
			})
		}
		if cooking.RequireTimeEvidence && !hasTotal && cooking.MaxTotalMinutes == nil && cooking.MaxActiveMinutes == nil {
			addConstraintCheck(report, ConstraintCheck{
				ID:           fmt.Sprintf("cooking.time_evidence.day_%d.%s", ref.Day, normalizeKey(ref.MealSlot)),
				Type:         "cooking_time",
				Scope:        "recipe",
				Required:     true,
				Status:       ConstraintCheckUnknown,
				Severity:     "blocking",
				EvidenceRefs: []EvidenceRef{{Artifact: FoodArtifactRun, JSONPath: "$.mealplan.days[].meals[].recipe", Label: ref.Recipe.Title}},
				Message:      "Recipe time evidence is required but missing for " + ref.Recipe.Title + ".",
				Remediation:  "Provide recipe prep/cook/step minutes before claiming cooking-time satisfaction.",
			})
		}
	}
}

func addRecipePDFChecks(report *ConstraintSatisfactionReport, run FoodRunArtifact, intent MealRunIntent) {
	if intent.RecipePDF == nil {
		return
	}
	pdf := intent.RecipePDF
	if pdf.RequireCookingSteps {
		missing := recipesMissingSteps(run.MealPlan)
		ok := len(missing) == 0
		addConstraintCheck(report, ConstraintCheck{
			ID:           "recipe_pdf.cooking_steps",
			Type:         "recipe_pdf",
			Scope:        "recipes",
			Required:     true,
			Status:       statusFromBool(ok),
			Severity:     severityFromBool(ok, true),
			EvidenceRefs: []EvidenceRef{{Artifact: FoodArtifactRun, JSONPath: "$.mealplan.days[].meals[].recipe.steps", Label: "recipe steps"}},
			Expected:     "steps present for every active recipe",
			Actual:       missing,
			Message:      recipeMissingMessage("cooking steps", missing),
			Remediation:  "Use recipes with explicit cooking instructions.",
		})
	}
	if pdf.RequireRecipeSource {
		missing := recipesMissingSource(run)
		ok := len(missing) == 0
		status := statusFromBool(ok)
		if run.RecipeQualityReport == nil {
			status = ConstraintCheckUnknown
		}
		addConstraintCheck(report, ConstraintCheck{
			ID:           "recipe_pdf.recipe_source",
			Type:         "recipe_pdf",
			Scope:        "recipes",
			Required:     true,
			Status:       status,
			Severity:     severityFromStatus(status, true),
			EvidenceRefs: []EvidenceRef{{Artifact: FoodArtifactRecipeQuality, JSONPath: "$.items[].has_source", Label: "recipe source evidence"}},
			Expected:     "source present for every active recipe",
			Actual:       missing,
			Message:      recipeMissingMessage("source evidence", missing),
			Remediation:  "Use --recipe-file/--recipe-url with source metadata or rerun recipe intake with source checks.",
		})
	}
	if pdf.RequireRecipeImages {
		missing := recipesMissingImage(run)
		ok := len(missing) == 0
		status := statusFromBool(ok)
		if run.RecipeQualityReport == nil {
			status = ConstraintCheckUnknown
		}
		addConstraintCheck(report, ConstraintCheck{
			ID:           "recipe_pdf.recipe_images",
			Type:         "recipe_pdf",
			Scope:        "recipes",
			Required:     true,
			Status:       status,
			Severity:     severityFromStatus(status, true),
			EvidenceRefs: []EvidenceRef{{Artifact: FoodArtifactRecipeQuality, JSONPath: "$.image_evidence", Label: "recipe image evidence"}},
			Expected:     "local or cached image evidence for every active recipe",
			Actual:       missing,
			Message:      recipeMissingMessage("image evidence", missing),
			Remediation:  "Provide recipe image URLs/files and rerun recipe intake with image caching.",
		})
	}
	if pdf.RequireCompletePDF {
		ok := strings.TrimSpace(run.PDFPath) != ""
		addConstraintCheck(report, ConstraintCheck{
			ID:           "recipe_pdf.complete_pdf_requested",
			Type:         "recipe_pdf",
			Scope:        "pdf",
			Required:     true,
			Status:       statusFromBool(ok),
			Severity:     severityFromBool(ok, true),
			EvidenceRefs: []EvidenceRef{{Artifact: FoodArtifactRun, JSONPath: "$.pdf", Label: "PDF output path"}},
			Expected:     "pdf output path present",
			Actual:       run.PDFPath,
			Message:      "Intent requires a complete PDF artifact.",
			Remediation:  "Run with --pdf-out and audit the generated PDF before claiming PDF completeness.",
		})
	}
}

func addPantryIntentChecks(report *ConstraintSatisfactionReport, run FoodRunArtifact, intent MealRunIntent) {
	if intent.Pantry == nil {
		return
	}
	pantry := intent.Pantry
	if pantry.AllowAssumedPantry != nil {
		actual := false
		ok := run.PantryResolution != nil
		if ok {
			actual = run.PantryResolution.Policy.AllowAssumedPantry
		}
		status := ConstraintCheckUnknown
		if ok {
			status = statusFromBool(actual == *pantry.AllowAssumedPantry)
		}
		addConstraintCheck(report, ConstraintCheck{
			ID:           "pantry.allow_assumed",
			Type:         "pantry",
			Scope:        "pantry_resolution",
			Required:     true,
			Status:       status,
			Severity:     severityFromStatus(status, true),
			EvidenceRefs: []EvidenceRef{{Artifact: FoodArtifactPantry, JSONPath: "$.policy.allow_assumed_pantry", Label: "pantry assumptions policy"}},
			Expected:     *pantry.AllowAssumedPantry,
			Actual:       actual,
			Message:      "Pantry assumption policy must match the user request.",
			Remediation:  "Rerun with --allow-assumed-pantry or --require-confirmed-pantry to match the intent.",
		})
	}
	if pantry.RequireConfirmedPantry {
		assumed := 0
		if run.PantryResolution != nil {
			assumed = run.PantryResolution.Summary.PantryAssumedLines
		}
		ok := run.PantryResolution != nil && assumed == 0
		addConstraintCheck(report, ConstraintCheck{
			ID:           "pantry.require_confirmed",
			Type:         "pantry",
			Scope:        "pantry_resolution",
			Required:     true,
			Status:       statusFromBool(ok),
			Severity:     severityFromBool(ok, true),
			EvidenceRefs: []EvidenceRef{{Artifact: FoodArtifactPantry, JSONPath: "$.summary.pantry_assumed_lines", Label: "assumed pantry lines"}},
			Expected:     0,
			Actual:       assumed,
			Message:      fmt.Sprintf("Intent requires confirmed pantry; final plan has %d assumed pantry line(s).", assumed),
			Remediation:  "Confirm pantry items or rerun with shopping for unknown staples.",
		})
	}
	if pantry.ShopAllIngredients {
		actual := false
		ok := run.PantryResolution != nil
		if ok {
			actual = run.PantryResolution.Policy.ShopAllIngredients
		}
		status := ConstraintCheckUnknown
		if ok {
			status = statusFromBool(actual)
		}
		addConstraintCheck(report, ConstraintCheck{
			ID:           "pantry.shop_all_ingredients",
			Type:         "pantry",
			Scope:        "pantry_resolution",
			Required:     true,
			Status:       status,
			Severity:     severityFromStatus(status, true),
			EvidenceRefs: []EvidenceRef{{Artifact: FoodArtifactPantry, JSONPath: "$.policy.shop_all_ingredients", Label: "shop all ingredients policy"}},
			Expected:     true,
			Actual:       actual,
			Message:      "Intent requires shopping all ingredients rather than relying on pantry coverage.",
			Remediation:  "Rerun with --shop-all-ingredients.",
		})
	}
}

func addGenericIntentChecks(report *ConstraintSatisfactionReport, intent MealRunIntent) {
	for _, constraint := range intent.HardConstraints {
		id := firstNonEmptyString(constraint.ID, "hard."+normalizeKey(constraint.Type)+"."+normalizeKey(constraint.Description))
		required := constraint.Required
		if !required {
			required = true
		}
		status := ConstraintCheckUnknown
		severity := "blocking"
		if !constraint.EvidenceRequired {
			severity = "warning"
		}
		addConstraintCheck(report, ConstraintCheck{
			ID:          id,
			Type:        firstNonEmptyString(constraint.Type, "generic"),
			Scope:       constraint.Scope,
			Required:    required,
			Status:      status,
			Severity:    severity,
			Expected:    constraint.Description,
			Message:     "Generic hard constraint is not machine-verifiable in v1 without a typed intent field: " + constraint.Description,
			Remediation: "Map this request to a typed intent field before claiming satisfaction.",
		})
	}
	for _, pref := range intent.SoftPreferences {
		id := firstNonEmptyString(pref.ID, "soft."+normalizeKey(pref.Type)+"."+normalizeKey(pref.Description))
		addConstraintCheck(report, ConstraintCheck{
			ID:          id,
			Type:        firstNonEmptyString(pref.Type, "preference"),
			Scope:       pref.Scope,
			Required:    false,
			Status:      ConstraintCheckUnknown,
			Severity:    "warning",
			Expected:    pref.Description,
			Message:     "Soft preference is recorded but not machine-verifiable in v1: " + pref.Description,
			Remediation: "Map soft preferences to measurable fields when they should become claims.",
		})
	}
}

func finalizeConstraintSatisfactionReport(report *ConstraintSatisfactionReport, policy IntentClaimPolicy) {
	for _, check := range report.Checks {
		if check.Required {
			report.Summary.HardConstraintCount++
			switch check.Status {
			case ConstraintCheckSatisfied:
				report.Summary.HardSatisfiedCount++
			case ConstraintCheckUnknown:
				report.Summary.HardUnknownCount++
			case ConstraintCheckNotApplicable:
			default:
				report.Summary.HardFailedCount++
			}
			continue
		}
		report.Summary.SoftPreferenceCount++
		switch check.Status {
		case ConstraintCheckSatisfied:
			report.Summary.SoftSatisfiedCount++
		case ConstraintCheckNotSatisfied:
			report.Summary.SoftMissedCount++
		case ConstraintCheckUnknown:
			report.Summary.SoftUnknownCount++
		}
	}
	if len(report.Checks) == 0 {
		report.Status = ConstraintSatisfactionUnknown
		report.Warnings = append(report.Warnings, ConstraintIssue{
			Code:        "no_measurable_constraints",
			Severity:    "warning",
			Message:     "Intent was present but did not include measurable constraints.",
			Remediation: "Have Hermes map the user request to explicit intent fields before claiming request satisfaction.",
		})
	} else if report.Summary.HardFailedCount > 0 || (policy.UnknownHardConstraintFails && report.Summary.HardUnknownCount > 0) {
		report.Status = ConstraintSatisfactionFail
	} else if report.Summary.HardUnknownCount > 0 {
		report.Status = ConstraintSatisfactionUnknown
	} else if report.Summary.SoftMissedCount > 0 || report.Summary.SoftUnknownCount > 0 || len(report.Warnings) > 0 {
		report.Status = ConstraintSatisfactionPassWithWarning
	} else {
		report.Status = ConstraintSatisfactionPass
	}
	report.Summary.MayClaimRequestSatisfied = report.Status == ConstraintSatisfactionPass || report.Status == ConstraintSatisfactionPassWithWarning
	if policy.RequireAllHardConstraints && (report.Summary.HardFailedCount > 0 || (policy.UnknownHardConstraintFails && report.Summary.HardUnknownCount > 0)) {
		report.Summary.MayClaimRequestSatisfied = false
	}
	report.ClaimGuard = IntentClaimGuard{
		MayClaimRequestSatisfied:     report.Summary.MayClaimRequestSatisfied,
		MayClaimBudgetSatisfied:      categoryMayClaim(report.Checks, "budget"),
		MayClaimNutritionSatisfied:   categoryMayClaim(report.Checks, "nutrition"),
		MayClaimServingSatisfied:     categoryMayClaim(report.Checks, "serving"),
		MayClaimDietSatisfied:        categoryMayClaim(report.Checks, "diet"),
		MayClaimCookingTimeSatisfied: categoryMayClaim(report.Checks, "cooking_time"),
		MayClaimPDFComplete:          categoryMayClaim(report.Checks, "recipe_pdf"),
	}
	for _, check := range report.Checks {
		if check.Required && (check.Status == ConstraintCheckNotSatisfied || (policy.UnknownHardConstraintFails && check.Status == ConstraintCheckUnknown)) {
			issue := ConstraintIssue{Code: check.ID, Severity: "blocking", ConstraintID: check.ID, Message: check.Message, Remediation: check.Remediation}
			report.BlockingIssues = append(report.BlockingIssues, issue)
			if report.ClaimGuard.PrimaryFailureCode == "" {
				report.ClaimGuard.PrimaryFailureCode = issue.Code
				report.ClaimGuard.PrimaryFailureMessage = issue.Message
			}
		} else if !check.Required && (check.Status == ConstraintCheckNotSatisfied || check.Status == ConstraintCheckUnknown) {
			report.Warnings = append(report.Warnings, ConstraintIssue{Code: check.ID, Severity: "warning", ConstraintID: check.ID, Message: check.Message, Remediation: check.Remediation})
		}
	}
	if !report.ClaimGuard.MayClaimRequestSatisfied {
		report.ClaimGuard.RequiredUserWarning = "The basket may still be usable, but the final run does not prove the user's request constraints were satisfied."
	}
}

func addConstraintCheck(report *ConstraintSatisfactionReport, check ConstraintCheck) {
	if check.ID == "" {
		check.ID = normalizeKey(check.Type + "." + check.Scope)
	}
	check.Expected = nilIfTypedNil(check.Expected)
	check.Actual = nilIfTypedNil(check.Actual)
	if check.Status == "" {
		check.Status = ConstraintCheckUnknown
	}
	if check.Severity == "" {
		if check.Required {
			check.Severity = "blocking"
		} else {
			check.Severity = "warning"
		}
	}
	report.Checks = append(report.Checks, check)
}

func nilIfTypedNil(value any) any {
	if value == nil {
		return nil
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if v.IsNil() {
			return nil
		}
	}
	return value
}

func categoryMayClaim(checks []ConstraintCheck, typ string) bool {
	seen := false
	for _, check := range checks {
		if check.Type != typ {
			continue
		}
		seen = true
		if check.Required && check.Status != ConstraintCheckSatisfied {
			return false
		}
	}
	return seen
}

func statusFromBool(ok bool) string {
	if ok {
		return ConstraintCheckSatisfied
	}
	return ConstraintCheckNotSatisfied
}

func severityFromBool(ok, required bool) string {
	if ok {
		return "info"
	}
	if required {
		return "blocking"
	}
	return "warning"
}

func severityFromStatus(status string, required bool) string {
	if status == ConstraintCheckSatisfied {
		return "info"
	}
	if required {
		return "blocking"
	}
	return "warning"
}

func totalServingUnits(run FoodRunArtifact) (float64, bool) {
	if run.ServingPlan != nil && run.ServingPlan.Summary.TotalTargetServingUnits > 0 {
		return run.ServingPlan.Summary.TotalTargetServingUnits, true
	}
	if run.MealPlan.People > 0 && activeRecipeCount(run.MealPlan) > 0 {
		return float64(run.MealPlan.People), true
	}
	return 0, false
}

func runServingAdultUnits(run FoodRunArtifact) (float64, bool) {
	if run.ServingPlan == nil || run.ServingPlan.Policy.AdultServings <= 0 {
		return 0, false
	}
	return run.ServingPlan.Policy.AdultServings, true
}

func runServingChildUnits(run FoodRunArtifact) (float64, bool) {
	if run.ServingPlan == nil || run.ServingPlan.Policy.ChildServings <= 0 {
		return 0, false
	}
	return run.ServingPlan.Policy.ChildServings, true
}

func runServingToddlerUnits(run FoodRunArtifact) (float64, bool) {
	if run.ServingPlan == nil || run.ServingPlan.Policy.ToddlerServings <= 0 {
		return 0, false
	}
	return run.ServingPlan.Policy.ToddlerServings, true
}

func addServingPolicyCheck(report *ConstraintSatisfactionReport, run FoodRunArtifact, id, label string, expected *float64, actual float64, evidence bool, tolerance float64) {
	if expected == nil {
		return
	}
	status := ConstraintCheckUnknown
	if evidence {
		status = statusFromBool(math.Abs(actual-*expected) <= tolerance)
	}
	addConstraintCheck(report, ConstraintCheck{
		ID:           id,
		Type:         "serving",
		Scope:        "serving_plan",
		Required:     true,
		Status:       status,
		Severity:     severityFromStatus(status, true),
		EvidenceRefs: []EvidenceRef{{Artifact: FoodArtifactServingPlan, JSONPath: "$.policy." + label, Label: label}},
		Expected:     map[string]any{label: *expected, "tolerance": tolerance},
		Actual:       actual,
		Message:      fmt.Sprintf("Serving policy %s is %.3g; intent expected %.3g.", label, actual, *expected),
		Remediation:  "Rerun with serving flags or a household profile matching the intent.",
	})
}

func budgetEstimatedTotal(run FoodRunArtifact) (int, bool) {
	if run.BudgetDealReport != nil && run.BudgetDealReport.EstimatedTotal.Cents > 0 {
		return int(run.BudgetDealReport.EstimatedTotal.Cents), true
	}
	if run.Shop.EstimatedTotal.Cents > 0 {
		return int(run.Shop.EstimatedTotal.Cents), true
	}
	return 0, false
}

func addNutritionTargetCheck(report *ConstraintSatisfactionReport, run FoodRunArtifact, target NutritionTarget, idx int, nutritionReady bool) {
	id := fmt.Sprintf("nutrition.target_%02d.%s", idx+1, normalizeKey(target.Nutrient))
	value, ok := nutritionTargetValue(run, target)
	status := ConstraintCheckUnknown
	if ok && nutritionReady {
		status = statusFromBool(compareNutritionTarget(value, target.Operator, target.Value))
	}
	addConstraintCheck(report, ConstraintCheck{
		ID:           id,
		Type:         "nutrition",
		Scope:        firstNonEmptyString(target.Scope, "plan_total"),
		Required:     true,
		Status:       status,
		Severity:     severityFromStatus(status, true),
		EvidenceRefs: []EvidenceRef{{Artifact: FoodArtifactNutritionLedger, JSONPath: "$.total_consumed_known", Label: "known consumed nutrition"}},
		Expected:     target,
		Actual:       value,
		Message:      fmt.Sprintf("Nutrition target %s %s %.3g %s has value %.3g.", target.Nutrient, firstNonEmptyString(target.Operator, ">="), target.Value, target.Unit, value),
		Remediation:  "Regenerate with enough nutrition coverage before claiming nutrition target satisfaction.",
	})
}

func nutritionTargetValue(run FoodRunArtifact, target NutritionTarget) (float64, bool) {
	if run.NutritionLedger == nil {
		return 0, false
	}
	scope := normalizeKey(firstNonEmptyString(target.Scope, "plan_total"))
	switch scope {
	case "meal_per_serving", "meal":
		if len(run.NutritionLedger.MealSummaries) == 0 {
			return 0, false
		}
		for _, meal := range run.NutritionLedger.MealSummaries {
			if meal.PerCookedServingKnown == nil {
				return 0, false
			}
			value, ok := nutritionFactValue(meal.PerCookedServingKnown.Facts, target.Nutrient)
			if !ok || !compareNutritionTarget(value, target.Operator, target.Value) {
				return value, ok
			}
		}
		value, ok := nutritionFactValue(run.NutritionLedger.MealSummaries[0].PerCookedServingKnown.Facts, target.Nutrient)
		return value, ok
	case "day_total", "day":
		if len(run.NutritionLedger.DaySummaries) == 0 {
			return 0, false
		}
		for _, day := range run.NutritionLedger.DaySummaries {
			if day.TotalKnown == nil {
				return 0, false
			}
			value, ok := nutritionFactValue(day.TotalKnown.Facts, target.Nutrient)
			if !ok || !compareNutritionTarget(value, target.Operator, target.Value) {
				return value, ok
			}
		}
		value, ok := nutritionFactValue(run.NutritionLedger.DaySummaries[0].TotalKnown.Facts, target.Nutrient)
		return value, ok
	default:
		if run.NutritionLedger.TotalConsumedKnown == nil {
			return 0, false
		}
		return nutritionFactValue(run.NutritionLedger.TotalConsumedKnown.Facts, target.Nutrient)
	}
}

func nutritionFactValue(facts NutritionFacts, nutrient string) (float64, bool) {
	switch normalizeKey(nutrient) {
	case "energy_kcal", "kcal", "calories":
		return amountValue(facts.EnergyKcal)
	case "energy_kj", "kj":
		return amountValue(facts.EnergyKJ)
	case "fat_g", "fat":
		return amountValue(facts.FatG)
	case "saturated_fat_g", "saturates_g", "saturates":
		return amountValue(facts.SaturatedFatG)
	case "carbs_g", "carbohydrate_g", "carbs":
		return amountValue(facts.CarbohydrateG)
	case "sugar_g", "sugars_g", "sugars":
		return amountValue(facts.SugarsG)
	case "fiber_g", "fibre_g", "fiber":
		return amountValue(facts.FiberG)
	case "protein_g", "protein":
		return amountValue(facts.ProteinG)
	case "salt_g", "salt":
		return amountValue(facts.SaltG)
	case "sodium_g", "sodium":
		return amountValue(facts.SodiumG)
	default:
		return 0, false
	}
}

func amountValue(amount *NutrientAmount) (float64, bool) {
	if amount == nil {
		return 0, false
	}
	return amount.Value, true
}

func compareNutritionTarget(actual float64, operator string, expected float64) bool {
	switch strings.TrimSpace(operator) {
	case "<", "lt":
		return actual < expected
	case "<=", "le", "":
		return actual <= expected
	case ">", "gt":
		return actual > expected
	case ">=", "ge":
		return actual >= expected
	case "=", "==", "eq":
		return math.Abs(actual-expected) < 0.0001
	default:
		return false
	}
}

type intentExclusionMatch struct {
	Artifact string `json:"artifact"`
	JSONPath string `json:"json_path,omitempty"`
	Label    string `json:"label,omitempty"`
	Value    string `json:"value"`
}

func intentExclusionMatches(run FoodRunArtifact, term string, allergen bool) []intentExclusionMatch {
	var matches []intentExclusionMatch
	needle := normalizeKey(term)
	if needle == "" {
		return nil
	}
	for dayIdx, day := range run.MealPlan.Days {
		for mealIdx, meal := range day.Meals {
			recipePath := fmt.Sprintf("$.mealplan.days[%d].meals[%d].recipe", dayIdx, mealIdx)
			for ingredientIdx, ingredient := range meal.Recipe.Ingredients {
				value := strings.Join([]string{ingredient.Name, ingredient.SearchTerm, ingredient.Category}, " ")
				if containsIntentTerm(value, needle) {
					matches = append(matches, intentExclusionMatch{Artifact: FoodArtifactRun, JSONPath: fmt.Sprintf("%s.ingredients[%d]", recipePath, ingredientIdx), Label: meal.Recipe.Title, Value: ingredient.Name})
				}
			}
			for _, note := range meal.Recipe.AllergenNotes {
				if containsIntentTerm(note, needle) {
					matches = append(matches, intentExclusionMatch{Artifact: FoodArtifactRun, JSONPath: recipePath + ".allergen_notes", Label: meal.Recipe.Title, Value: note})
				}
			}
			if allergen && containsIntentTerm(strings.Join(meal.Recipe.AllergenNotes, " "), needle) {
				matches = append(matches, intentExclusionMatch{Artifact: FoodArtifactRun, JSONPath: recipePath + ".allergen_notes", Label: meal.Recipe.Title, Value: strings.Join(meal.Recipe.AllergenNotes, "; ")})
			}
		}
	}
	for i, selected := range run.Shop.SelectedProducts {
		value := strings.Join([]string{selected.Ingredient.Name, selected.Product.Name, selected.Product.Brand, selected.Product.Category, selected.Product.Allergens}, " ")
		if containsIntentTerm(value, needle) {
			matches = append(matches, intentExclusionMatch{Artifact: FoodArtifactShop, JSONPath: fmt.Sprintf("$.selected_products[%d]", i), Label: firstNonEmptyString(selected.Product.Name, selected.Ingredient.Name), Value: value})
		}
	}
	return matches
}

func containsIntentTerm(value, normalizedTerm string) bool {
	key := normalizeKey(value)
	if key == "" || normalizedTerm == "" {
		return false
	}
	if key == normalizedTerm {
		return true
	}
	for _, token := range strings.Fields(key) {
		if token == normalizedTerm {
			return true
		}
	}
	return strings.Contains(key, normalizedTerm)
}

func exclusionEvidenceRefs(matches []intentExclusionMatch) []EvidenceRef {
	refs := make([]EvidenceRef, 0, len(matches))
	for _, match := range matches {
		refs = append(refs, EvidenceRef{Artifact: match.Artifact, JSONPath: match.JSONPath, Label: match.Label})
	}
	return refs
}

func recipeTotalMinutes(recipe Recipe) (int, bool) {
	total := 0
	if recipe.PrepMinutes > 0 {
		total += recipe.PrepMinutes
	}
	if recipe.CookMinutes > 0 {
		total += recipe.CookMinutes
	}
	if total > 0 {
		return total, true
	}
	for _, step := range recipe.Steps {
		total += step.Minutes
	}
	return total, total > 0
}

type activeRecipeRef struct {
	Day      int
	MealSlot string
	Recipe   Recipe
}

func activeRecipeRefs(plan MealPlan) []activeRecipeRef {
	var refs []activeRecipeRef
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			refs = append(refs, activeRecipeRef{Day: day.Day, MealSlot: meal.Type, Recipe: meal.Recipe})
		}
	}
	return refs
}

func recipesMissingSteps(plan MealPlan) []string {
	var missing []string
	for _, ref := range activeRecipeRefs(plan) {
		if len(ref.Recipe.Steps) == 0 {
			missing = append(missing, ref.Recipe.Title)
		}
	}
	sort.Strings(missing)
	return missing
}

func recipesMissingSource(run FoodRunArtifact) []string {
	if run.RecipeQualityReport == nil {
		return recipeTitles(run.MealPlan)
	}
	var missing []string
	for _, item := range run.RecipeQualityReport.Items {
		if !item.HasSource {
			missing = append(missing, item.RecipeTitle)
		}
	}
	sort.Strings(missing)
	return missing
}

func recipesMissingImage(run FoodRunArtifact) []string {
	if run.RecipeQualityReport == nil {
		return recipeTitles(run.MealPlan)
	}
	var missing []string
	for _, item := range run.RecipeQualityReport.Items {
		if !item.HasImage || (item.ImageStatus != "" && item.ImageStatus != RecipeImageCached && item.ImageStatus != RecipeImageLocal) {
			missing = append(missing, item.RecipeTitle)
		}
	}
	sort.Strings(missing)
	return missing
}

func recipeTitles(plan MealPlan) []string {
	var titles []string
	for _, ref := range activeRecipeRefs(plan) {
		titles = append(titles, ref.Recipe.Title)
	}
	sort.Strings(titles)
	return titles
}

func missingMealSlots(plan MealPlan, expected []string) []string {
	var missing []string
	for _, day := range plan.Days {
		present := map[string]bool{}
		for _, meal := range day.Meals {
			present[normalizeKey(meal.Type)] = true
		}
		for _, slot := range expected {
			key := normalizeKey(slot)
			if key != "" && !present[key] {
				missing = append(missing, fmt.Sprintf("day %d %s", day.Day, slot))
			}
		}
	}
	sort.Strings(missing)
	return missing
}

func actualMealSlots(plan MealPlan) []string {
	seen := map[string]bool{}
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			key := normalizeKey(meal.Type)
			if key != "" {
				seen[key] = true
			}
		}
	}
	var out []string
	for key := range seen {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func normalizeListForReport(values []string) []string {
	var out []string
	for _, value := range values {
		if key := normalizeKey(value); key != "" {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

func mealSlotCoverageMessage(missing []string) string {
	if len(missing) == 0 {
		return "All expected meal slots are present for every planned day."
	}
	return "Missing expected meal slot(s): " + strings.Join(missing, ", ") + "."
}

func servingUnitsMessage(evidence bool, actual, expected float64) string {
	if !evidence {
		return "No serving-plan evidence was available for the requested serving target."
	}
	return fmt.Sprintf("Final target serving units are %.3g; intent expected %.3g.", actual, expected)
}

func moneyConstraintMessage(label string, actual int, evidence bool, expected int) string {
	if !evidence {
		return label + " is not safely known from current evidence."
	}
	return fmt.Sprintf("%s is %d cents; intent maximum is %d cents.", label, actual, expected)
}

func exclusionMessage(term string, matches []intentExclusionMatch) string {
	if len(matches) == 0 {
		return "No deterministic recipe or selected-product match found for excluded term: " + term + "."
	}
	return fmt.Sprintf("Excluded term %q matched %d recipe/product evidence item(s).", term, len(matches))
}

func cookingTimeMessage(title string, actual int, evidence bool, expected int) string {
	if !evidence {
		return "Recipe time evidence is missing for " + title + "."
	}
	return fmt.Sprintf("%s has total time %d minutes; intent maximum is %d minutes.", title, actual, expected)
}

func activeTimeMessage(title string, actual int, evidence bool, expected int) string {
	if !evidence {
		return "Recipe active/prep time evidence is missing for " + title + "."
	}
	return fmt.Sprintf("%s has active/prep time %d minutes; intent maximum is %d minutes.", title, actual, expected)
}

func recipeMissingMessage(label string, missing []string) string {
	if len(missing) == 0 {
		return "All active recipes have " + label + "."
	}
	return "Missing " + label + " for: " + strings.Join(missing, ", ") + "."
}
