package food

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
)

func DefaultNutritionPolicy(mode string) NutritionPolicy {
	mode = normalizeNutritionMode(mode)
	return NutritionPolicy{
		Enabled:                      mode != NutritionModeOff,
		Mode:                         mode,
		Basis:                        "consumed_scaled_recipe",
		AllowRecipeDeclaredNutrition: true,
		AllowPantryProfileNutrition:  true,
	}
}

func BuildNutritionLedger(ctx context.Context, run *FoodRunArtifact, pantryProfile PantryProfile, policy NutritionPolicy) NutritionLedger {
	_ = ctx
	policy.Mode = normalizeNutritionMode(policy.Mode)
	if policy.Mode == NutritionModeOff {
		policy.Enabled = false
	}
	if policy.Basis == "" {
		policy.Basis = "consumed_scaled_recipe"
	}
	ledger := NutritionLedger{
		SchemaVersion: "1",
		Status:        NutritionLedgerNotRun,
		GeneratedAt:   nowStamp(),
		Policy:        policy,
	}
	if !policy.Enabled {
		ledger.NutritionLedgerFingerprint = NutritionLedgerFingerprint(ledger)
		return ledger
	}
	if run == nil {
		ledger.Status = NutritionLedgerBlocked
		ledger.BlockingIssues = append(ledger.BlockingIssues, NutritionIssue{Code: "run_missing", Severity: "blocking", Message: "Food run artifact is missing.", Remediation: "Regenerate the food run."})
		ledger.NutritionLedgerFingerprint = NutritionLedgerFingerprint(ledger)
		return ledger
	}
	ledger.MealPlanFingerprint = firstNonEmptyString(run.MealPlanFingerprint, MealPlanFingerprint(run.MealPlan))
	ledger.ServingPlanFingerprint = run.ServingPlanFingerprint
	ledger.ScaledMealPlanFingerprint = run.ScaledMealPlanFingerprint
	ledger.PantryResolutionFingerprint = run.PantryResolutionFingerprint
	ledger.ShopRequirementsFingerprint = run.ShopRequirementsFingerprint
	ledger.ProductSelectionFingerprint = firstNonEmptyString(run.ProductSelectionFingerprint, ProductSelectionFingerprint(run.Shop))

	if run.QuantityLedger == nil {
		ledger.Status = NutritionLedgerBlocked
		ledger.BlockingIssues = append(ledger.BlockingIssues, NutritionIssue{Code: "quantity_ledger_missing", Severity: "blocking", Message: "Nutrition evidence requires the final quantity ledger.", Remediation: "Rebuild the food run with the quantity ledger."})
		ledger.NutritionLedgerFingerprint = NutritionLedgerFingerprint(ledger)
		return ledger
	}

	selected := selectedProductsByNutritionKey(run.Shop.SelectedProducts)
	allocations := allocationsByRequirementID(run.QuantityLedger.Allocations)
	pantryLines := pantryResolutionLinesByKey(run.PantryResolution)
	pantryNutrition := pantryNutritionByKey(pantryProfile)
	recipeNutrition := recipeDeclaredAvailability(run.MealPlan)

	seenLines := map[string]bool{}
	for _, req := range run.QuantityLedger.Requirements {
		line := buildNutritionIngredientLine(req, selected, allocations[req.RequirementID], pantryLines, pantryNutrition, recipeNutrition, policy)
		ledger.IngredientLines = append(ledger.IngredientLines, line)
		seenLines[nutritionLineIdentity(line)] = true
		if line.ConsumedKnown != nil {
			ledger.TotalConsumedKnown = SumNutritionRollupsPtr(ledger.TotalConsumedKnown, line.ConsumedKnown)
		}
		if policy.IncludePurchasedExcess && line.PurchasedExcessKnown != nil {
			ledger.PurchasedExcessKnown = SumNutritionRollupsPtr(ledger.PurchasedExcessKnown, line.PurchasedExcessKnown)
		}
		for _, warning := range line.Warnings {
			ledger.Warnings = append(ledger.Warnings, NutritionIssue{
				Code:           "line_warning",
				Severity:       "warning",
				IngredientKey:  line.IngredientKey,
				IngredientName: line.IngredientName,
				ProductID:      line.ProductID,
				ProductName:    line.ProductName,
				Day:            line.Day,
				MealSlot:       line.MealSlot,
				Message:        warning,
			})
		}
	}
	if run.PantryResolution != nil {
		for _, pantryLine := range run.PantryResolution.Lines {
			if pantryLine.PantryAllocated == nil || pantryLine.PantryAllocated.BaseValue <= 0 {
				continue
			}
			if pantryLine.ShopRequired != nil && pantryLine.ShopRequired.BaseValue > 0 {
				continue
			}
			line := buildNutritionIngredientLineFromPantryLine(pantryLine, pantryNutrition, recipeNutrition, policy)
			if seenLines[nutritionLineIdentity(line)] {
				continue
			}
			ledger.IngredientLines = append(ledger.IngredientLines, line)
			if line.ConsumedKnown != nil {
				ledger.TotalConsumedKnown = SumNutritionRollupsPtr(ledger.TotalConsumedKnown, line.ConsumedKnown)
			}
			for _, warning := range line.Warnings {
				ledger.Warnings = append(ledger.Warnings, NutritionIssue{
					Code:           "line_warning",
					Severity:       "warning",
					IngredientKey:  line.IngredientKey,
					IngredientName: line.IngredientName,
					Day:            line.Day,
					MealSlot:       line.MealSlot,
					Message:        warning,
				})
			}
		}
	}
	ledger.RecipeDeclaredSummaries = buildRecipeDeclaredNutritionSummaries(run.MealPlan, run.ServingPlan, policy)
	ledger.MealSummaries = nutritionMealSummaries(ledger.IngredientLines)
	ledger.DaySummaries = nutritionDaySummaries(ledger.IngredientLines)
	ledger.Coverage = NutritionCoverageFromLines(ledger.IngredientLines)
	applyNutritionThresholds(&ledger, policy)
	ledger.Status = nutritionLedgerStatus(ledger)
	ledger.NutritionLedgerFingerprint = NutritionLedgerFingerprint(ledger)
	return ledger
}

func buildNutritionIngredientLineFromPantryLine(pantryLine PantryResolutionLine, pantryNutrition map[string]PantryNutritionEvidence, recipeNutrition map[string]bool, policy NutritionPolicy) NutritionIngredientLine {
	usage := IngredientUsageRef{}
	if len(pantryLine.Usages) > 0 {
		usage = pantryLine.Usages[0]
	}
	key := firstNonEmptyString(pantryLine.IngredientKey, usage.IngredientKey, normalizePantryIngredientKey(pantryLine.IngredientName))
	line := NutritionIngredientLine{
		IngredientKey:           key,
		IngredientName:          pantryLine.IngredientName,
		Day:                     usage.Day,
		MealSlot:                usage.MealSlot,
		RecipeID:                usage.RecipeID,
		RecipeTitle:             usage.RecipeTitle,
		UsageRef:                usage,
		TotalRequiredQuantity:   cloneNormalizedQuantity(pantryLine.TotalRequired),
		PantryAllocatedQuantity: cloneNormalizedQuantity(pantryLine.PantryAllocated),
		SourceStatus:            "missing_pantry_nutrition",
	}
	if line.Day == 0 {
		line.Day = usage.Day
	}
	line.Coverage.RecipeDeclaredAvailable = recipeNutrition[nutritionSlotKey(line.Day, line.MealSlot, line.RecipeID)]
	if policy.AllowPantryProfileNutrition {
		line = attachPantryNutritionToLine(line, pantryLine, pantryNutrition[key])
	}
	if line.ConsumedKnown == nil && line.Coverage.RecipeDeclaredAvailable {
		line.SourceStatus = "recipe_declared_only"
	}
	finalizeNutritionLineCoverage(&line)
	return line
}

func AttachNutritionLedger(artifact FoodRunArtifact, ledger NutritionLedger) FoodRunArtifact {
	if ledger.Status == NutritionLedgerNotRun && !ledger.Policy.Enabled {
		return artifact
	}
	artifact.NutritionLedgerFingerprint = ledger.NutritionLedgerFingerprint
	artifact.NutritionLedger = &ledger
	return artifact
}

func NutritionLedgerFingerprint(ledger NutritionLedger) string {
	type canonical struct {
		Status                      NutritionLedgerStatus            `json:"status"`
		Policy                      NutritionPolicy                  `json:"policy"`
		MealPlanFingerprint         string                           `json:"mealplan_fingerprint,omitempty"`
		ServingPlanFingerprint      string                           `json:"serving_plan_fingerprint,omitempty"`
		ScaledMealPlanFingerprint   string                           `json:"scaled_mealplan_fingerprint,omitempty"`
		PantryResolutionFingerprint string                           `json:"pantry_resolution_fingerprint,omitempty"`
		ShopRequirementsFingerprint string                           `json:"shop_requirements_fingerprint,omitempty"`
		ProductSelectionFingerprint string                           `json:"product_selection_fingerprint,omitempty"`
		Coverage                    NutritionCoverageSummary         `json:"coverage"`
		IngredientLines             []NutritionIngredientLine        `json:"ingredient_lines,omitempty"`
		RecipeDeclaredSummaries     []RecipeDeclaredNutritionSummary `json:"recipe_declared_summaries,omitempty"`
	}
	lines := append([]NutritionIngredientLine(nil), ledger.IngredientLines...)
	sort.SliceStable(lines, func(i, j int) bool {
		if lines[i].Day != lines[j].Day {
			return lines[i].Day < lines[j].Day
		}
		if lines[i].MealSlot != lines[j].MealSlot {
			return lines[i].MealSlot < lines[j].MealSlot
		}
		return lines[i].IngredientKey < lines[j].IngredientKey
	})
	data, err := json.Marshal(canonical{
		Status:                      ledger.Status,
		Policy:                      ledger.Policy,
		MealPlanFingerprint:         ledger.MealPlanFingerprint,
		ServingPlanFingerprint:      ledger.ServingPlanFingerprint,
		ScaledMealPlanFingerprint:   ledger.ScaledMealPlanFingerprint,
		PantryResolutionFingerprint: ledger.PantryResolutionFingerprint,
		ShopRequirementsFingerprint: ledger.ShopRequirementsFingerprint,
		ProductSelectionFingerprint: ledger.ProductSelectionFingerprint,
		Coverage:                    ledger.Coverage,
		IngredientLines:             lines,
		RecipeDeclaredSummaries:     ledger.RecipeDeclaredSummaries,
	})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func NormalizeProductNutrition(product ProductSummary) (*ProductNutritionEvidence, []string) {
	label := product.NutritionParsed
	var warnings []string
	if label == nil && strings.TrimSpace(product.Nutrition) != "" {
		parsed, err := ParseAlcampoNutrition(product.Nutrition)
		if err != nil {
			warnings = append(warnings, "Could not parse Alcampo product nutrition label: "+err.Error())
		} else {
			label = parsed
		}
	}
	if label == nil {
		return &ProductNutritionEvidence{
			ProductID:   product.ID,
			SKU:         product.SKU,
			ProductName: product.Name,
			Parsed:      false,
			Basis:       NutritionEvidenceBasis{Type: NutritionUnknown, Confidence: "unknown"},
			Confidence:  "unknown",
			Warnings:    append(warnings, "No Alcampo product nutrition label was available."),
		}, warnings
	}
	evidence := &ProductNutritionEvidence{
		ProductID:   product.ID,
		SKU:         product.SKU,
		ProductName: product.Name,
		Parsed:      true,
		Basis:       nutritionEvidenceBasisFromStructured(*label),
		Facts:       nutritionFactsFromStructured(*label, "alcampo_label", nutritionConfidenceLabel(label.Confidence)),
		RawFields:   map[string]string{"raw": label.Raw},
		Confidence:  nutritionConfidenceLabel(label.Confidence),
		Warnings:    append([]string(nil), label.ParseWarnings...),
	}
	evidence.Warnings = append(evidence.Warnings, warnings...)
	return evidence, warnings
}

func buildNutritionIngredientLine(req IngredientRequirement, selected map[string]SelectedProduct, allocation IngredientProductAllocation, pantryLines map[string]PantryResolutionLine, pantryNutrition map[string]PantryNutritionEvidence, recipeNutrition map[string]bool, policy NutritionPolicy) NutritionIngredientLine {
	usage := firstIngredientUsage(req)
	key := firstNonEmptyString(usage.IngredientKey, normalizePantryIngredientKey(req.IngredientName), normalizeKey(req.IngredientName))
	slotKey := nutritionSlotKey(req.Day, req.MealSlot, req.RecipeID)
	line := NutritionIngredientLine{
		IngredientKey:           key,
		IngredientName:          req.IngredientName,
		Day:                     req.Day,
		MealSlot:                req.MealSlot,
		RecipeID:                req.RecipeID,
		RecipeTitle:             req.RecipeTitle,
		UsageRef:                usage,
		TotalRequiredQuantity:   cloneNormalizedQuantity(firstQuantity(req.TotalRequiredQuantity, req.RequiredQuantity)),
		PantryAllocatedQuantity: cloneNormalizedQuantity(req.PantryAllocatedQuantity),
		ShopRequiredQuantity:    cloneNormalizedQuantity(firstQuantity(req.ShopRequiredQuantity, req.RequiredQuantity)),
		SourceStatus:            "missing_label",
	}
	line.Coverage.RecipeDeclaredAvailable = recipeNutrition[slotKey]
	if allocation.RequirementID != "" {
		line.PurchasedQuantity = cloneQuantityRangeExpected(allocation.PurchasedQuantity)
		line.PurchasedExcessQuantity = cloneNormalizedQuantity(allocation.ExcessQuantity)
	}

	selectedProduct, ok := selected[nutritionSelectionKey(req.IngredientName)]
	if !ok && usage.IngredientName != "" {
		selectedProduct, ok = selected[nutritionSelectionKey(usage.IngredientName)]
	}
	if ok && selectedProduct.Error == "" {
		line.ProductID = selectedProduct.Product.ID
		line.SKU = selectedProduct.Product.SKU
		line.ProductName = selectedProduct.Product.Name
	}
	if policy.Mode == NutritionModeLabels || policy.Mode == NutritionModeHybrid {
		line = attachProductNutritionToLine(line, selectedProduct, ok, policy)
	}
	if policy.AllowPantryProfileNutrition && (policy.Mode == NutritionModeHybrid || policy.Mode == NutritionModeRecipe || policy.Mode == NutritionModeLabels) {
		line = attachPantryNutritionToLine(line, pantryLines[key], pantryNutrition[key])
	}
	if line.ConsumedKnown == nil && line.Coverage.RecipeDeclaredAvailable {
		line.SourceStatus = "recipe_declared_only"
	}
	if line.ConsumedKnown == nil && line.PantryAllocatedQuantity != nil && line.PantryAllocatedQuantity.BaseValue > 0 && !line.Coverage.PantryNutritionCovered {
		line.SourceStatus = "missing_pantry_nutrition"
	}
	if line.ConsumedKnown == nil && line.ShopRequiredQuantity != nil && line.ShopRequiredQuantity.BaseValue > 0 && line.ProductID == "" {
		line.SourceStatus = "missing_product"
	}
	finalizeNutritionLineCoverage(&line)
	return line
}

func attachProductNutritionToLine(line NutritionIngredientLine, selected SelectedProduct, ok bool, policy NutritionPolicy) NutritionIngredientLine {
	if line.ShopRequiredQuantity == nil || line.ShopRequiredQuantity.BaseValue <= 0 {
		return line
	}
	if !ok || selected.Error != "" {
		line.Warnings = append(line.Warnings, "No selected Alcampo product is available for nutrition evidence.")
		line.Coverage.MissingReason = "missing selected product"
		return line
	}
	evidence, warnings := NormalizeProductNutrition(selected.Product)
	line.ProductNutrition = evidence
	line.Warnings = append(line.Warnings, warnings...)
	if !evidence.Parsed {
		line.Warnings = append(line.Warnings, "No parsed Alcampo nutrition label is available for "+selected.Product.Name+".")
		line.Coverage.MissingReason = "missing Alcampo nutrition label"
		return line
	}
	label := selected.ProductNutrition
	if label == nil {
		label = selected.Product.NutritionParsed
	}
	if label == nil && strings.TrimSpace(selected.Product.Nutrition) != "" {
		parsed, err := ParseAlcampoNutrition(selected.Product.Nutrition)
		if err == nil {
			label = parsed
		}
	}
	estimate, estimateWarnings := estimateNutritionForNormalizedQuantity(*line.ShopRequiredQuantity, label)
	if estimate == nil {
		line.SourceStatus = "unconvertible_basis"
		line.Warnings = append(line.Warnings, estimateWarnings...)
		line.Coverage.MissingReason = strings.Join(estimateWarnings, "; ")
		return line
	}
	line.ProductConsumedQuantity = cloneNormalizedQuantity(line.ShopRequiredQuantity)
	line.ConsumedKnown = rollupFromEstimate(*estimate, "alcampo_label")
	line.Coverage.ProductLabelCovered = true
	line.Coverage.CoveredQuantity = addNutritionCoveredQuantity(line.Coverage.CoveredQuantity, line.ShopRequiredQuantity)
	line.SourceStatus = "alcampo_label"
	if policy.IncludePurchasedExcess && line.PurchasedExcessQuantity != nil && line.PurchasedExcessQuantity.BaseValue > 0 {
		excess, excessWarnings := estimateNutritionForNormalizedQuantity(*line.PurchasedExcessQuantity, label)
		if excess != nil {
			line.PurchasedExcessKnown = rollupFromEstimate(*excess, "alcampo_label_package_excess")
		} else {
			line.Warnings = append(line.Warnings, excessWarnings...)
		}
	}
	return line
}

func attachPantryNutritionToLine(line NutritionIngredientLine, pantryLine PantryResolutionLine, evidence PantryNutritionEvidence) NutritionIngredientLine {
	if line.PantryAllocatedQuantity == nil || line.PantryAllocatedQuantity.BaseValue <= 0 {
		return line
	}
	if evidence.Source == "" && !nutritionFactsHasValues(evidence.Facts) {
		line.Warnings = append(line.Warnings, line.IngredientName+" has pantry coverage without pantry nutrition evidence.")
		if line.Coverage.MissingReason == "" {
			line.Coverage.MissingReason = "missing pantry nutrition"
		}
		return line
	}
	rollup, warnings := estimateFactsForQuantity(*line.PantryAllocatedQuantity, evidence.Facts, evidence.Basis, "pantry_profile")
	if rollup == nil {
		line.SourceStatus = "unconvertible_basis"
		line.Warnings = append(line.Warnings, warnings...)
		if line.Coverage.MissingReason == "" {
			line.Coverage.MissingReason = strings.Join(warnings, "; ")
		}
		return line
	}
	line.Coverage.PantryNutritionCovered = true
	line.Coverage.CoveredQuantity = addNutritionCoveredQuantity(line.Coverage.CoveredQuantity, line.PantryAllocatedQuantity)
	line.ConsumedKnown = SumNutritionRollupsPtr(line.ConsumedKnown, rollup)
	if line.SourceStatus == "alcampo_label" {
		line.SourceStatus = "mixed_partial"
	} else {
		line.SourceStatus = "pantry_profile"
	}
	if pantryLine.IngredientName != "" {
		line.Warnings = append(line.Warnings, warnings...)
	}
	return line
}

func finalizeNutritionLineCoverage(line *NutritionIngredientLine) {
	line.Coverage.HasAnyNutrition = line.ConsumedKnown != nil || line.Coverage.RecipeDeclaredAvailable
	total := line.TotalRequiredQuantity
	covered := line.Coverage.CoveredQuantity
	if total != nil && covered != nil && total.BaseUnit == covered.BaseUnit && total.BaseValue > 0 {
		ratio := math.Min(1, covered.BaseValue/total.BaseValue)
		line.Coverage.CoverageRatioByQuantity = &ratio
		if ratio < 1 {
			line.Coverage.MissingQuantity = cloneNormalizedQuantity(&NormalizedQuantity{
				Raw:         formatPantryQuantity(&NormalizedQuantity{BaseValue: total.BaseValue - covered.BaseValue, BaseUnit: total.BaseUnit, Value: total.BaseValue - covered.BaseValue, Unit: total.BaseUnit}),
				Value:       roundQty(total.BaseValue - covered.BaseValue),
				Unit:        total.BaseUnit,
				BaseValue:   roundQty(total.BaseValue - covered.BaseValue),
				BaseUnit:    total.BaseUnit,
				Confidence:  math.Min(total.Confidence, covered.Confidence),
				ParseMethod: "nutrition_missing_delta",
			})
		}
	} else if total != nil && !line.Coverage.HasAnyNutrition {
		line.Coverage.MissingQuantity = cloneNormalizedQuantity(total)
	}
	if line.Coverage.HasAnyNutrition && line.Coverage.MissingReason == "" && line.Coverage.CoverageRatioByQuantity != nil && *line.Coverage.CoverageRatioByQuantity < 1 {
		line.Coverage.MissingReason = "partial nutrition evidence"
	}
	if !line.Coverage.HasAnyNutrition && line.Coverage.MissingReason == "" {
		line.Coverage.MissingReason = "missing nutrition evidence"
	}
}

func NutritionCoverageFromLines(lines []NutritionIngredientLine) NutritionCoverageSummary {
	summary := NutritionCoverageSummary{IngredientLines: len(lines)}
	var coveredBase, totalBase float64
	var baseUnit string
	quantityCompatible := true
	missingSeen := map[string]bool{}
	for _, line := range lines {
		if line.Coverage.HasAnyNutrition {
			summary.LinesWithAnyNutrition++
		} else {
			summary.LinesMissingNutrition++
			if line.IngredientName != "" && !missingSeen[line.IngredientName] {
				summary.MissingIngredients = append(summary.MissingIngredients, line.IngredientName)
				missingSeen[line.IngredientName] = true
			}
		}
		if line.Coverage.ProductLabelCovered {
			summary.LinesWithAlcampoLabel++
		}
		if line.Coverage.PantryNutritionCovered {
			summary.LinesWithPantryNutrition++
		}
		if line.Coverage.RecipeDeclaredAvailable {
			summary.LinesWithRecipeDeclaredNutrition++
		}
		if line.SourceStatus == "unconvertible_basis" {
			summary.LinesUnconvertibleBasis++
		}
		if line.PantryAllocatedQuantity != nil && line.PantryAllocatedQuantity.BaseValue > 0 && !line.Coverage.PantryNutritionCovered {
			summary.PantryMissingLines++
		}
		if line.TotalRequiredQuantity != nil && line.TotalRequiredQuantity.BaseValue > 0 {
			if baseUnit == "" {
				baseUnit = line.TotalRequiredQuantity.BaseUnit
			}
			if line.TotalRequiredQuantity.BaseUnit != baseUnit {
				quantityCompatible = false
			}
			totalBase += line.TotalRequiredQuantity.BaseValue
			if line.Coverage.CoveredQuantity != nil && line.Coverage.CoveredQuantity.BaseUnit == line.TotalRequiredQuantity.BaseUnit {
				coveredBase += line.Coverage.CoveredQuantity.BaseValue
			}
		}
	}
	if summary.IngredientLines > 0 {
		summary.LineCoverageRatio = float64(summary.LinesWithAnyNutrition) / float64(summary.IngredientLines)
		summary.ProductLabelCoverageRatio = float64(summary.LinesWithAlcampoLabel) / float64(summary.IngredientLines)
	}
	if quantityCompatible && totalBase > 0 {
		ratio := math.Min(1, coveredBase/totalBase)
		summary.QuantityCoverageRatio = &ratio
	}
	sort.Strings(summary.MissingIngredients)
	return summary
}

func applyNutritionThresholds(ledger *NutritionLedger, policy NutritionPolicy) {
	if policy.MinLineCoverageRatio > 0 && nutritionEvidenceLineCoverageRatio(ledger.Coverage) < policy.MinLineCoverageRatio {
		ledger.BlockingIssues = append(ledger.BlockingIssues, NutritionIssue{Code: "nutrition_line_coverage_below_threshold", Severity: "blocking", Message: "Nutrition label coverage is below the required threshold.", Remediation: "Choose products with nutrition labels, provide pantry nutrition, or lower the required nutrition coverage."})
		return
	}
	if policy.MinQuantityCoverageRatio > 0 {
		if ledger.Coverage.QuantityCoverageRatio == nil || *ledger.Coverage.QuantityCoverageRatio < policy.MinQuantityCoverageRatio {
			ledger.BlockingIssues = append(ledger.BlockingIssues, NutritionIssue{Code: "nutrition_quantity_coverage_below_threshold", Severity: "blocking", Message: "Nutrition quantity coverage is below the required threshold.", Remediation: "Choose products with compatible nutrition labels or provide pantry nutrition."})
		}
	}
}

func nutritionEvidenceLineCoverageRatio(coverage NutritionCoverageSummary) float64 {
	if coverage.IngredientLines <= 0 {
		return 0
	}
	return float64(coverage.LinesWithAlcampoLabel+coverage.LinesWithPantryNutrition) / float64(coverage.IngredientLines)
}

func nutritionLedgerStatus(ledger NutritionLedger) NutritionLedgerStatus {
	if len(ledger.BlockingIssues) > 0 {
		return NutritionLedgerBlocked
	}
	if ledger.Coverage.LinesWithAnyNutrition == 0 && len(ledger.RecipeDeclaredSummaries) == 0 {
		return NutritionLedgerNoCoverage
	}
	if ledger.Coverage.PantryMissingLines > 0 {
		return NutritionLedgerPartial
	}
	if ledger.Coverage.QuantityCoverageRatio != nil && *ledger.Coverage.QuantityCoverageRatio < 1 {
		return NutritionLedgerPartial
	}
	if ledger.Coverage.LinesMissingNutrition == 0 && ledger.Coverage.LinesUnconvertibleBasis == 0 {
		if ledger.Coverage.LinesWithPantryNutrition > 0 || len(ledger.RecipeDeclaredSummaries) > 0 {
			return NutritionLedgerCompleteHybrid
		}
		return NutritionLedgerCompleteForPurchased
	}
	return NutritionLedgerPartial
}

func normalizeNutritionMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", NutritionModeHybrid:
		return NutritionModeHybrid
	case NutritionModeOff:
		return NutritionModeOff
	case NutritionModeLabels:
		return NutritionModeLabels
	case NutritionModeRecipe:
		return NutritionModeRecipe
	default:
		return NutritionModeHybrid
	}
}

func selectedProductsByNutritionKey(selected []SelectedProduct) map[string]SelectedProduct {
	out := map[string]SelectedProduct{}
	for _, item := range selected {
		keys := []string{
			nutritionSelectionKey(item.Ingredient.Name),
			nutritionSelectionKey(item.Ingredient.SearchTerm),
			normalizePantryIngredientKey(item.Ingredient.Name),
			normalizePantryIngredientKey(item.Ingredient.SearchTerm),
		}
		for _, key := range keys {
			if key != "" {
				out[key] = item
			}
		}
	}
	return out
}

func nutritionSelectionKey(value string) string {
	return normalizePantryIngredientKey(firstNonEmptyString(value, normalizeKey(value)))
}

func allocationsByRequirementID(allocations []IngredientProductAllocation) map[string]IngredientProductAllocation {
	out := map[string]IngredientProductAllocation{}
	for _, allocation := range allocations {
		out[allocation.RequirementID] = allocation
	}
	return out
}

func pantryResolutionLinesByKey(resolution *PantryResolution) map[string]PantryResolutionLine {
	out := map[string]PantryResolutionLine{}
	if resolution == nil {
		return out
	}
	for _, line := range resolution.Lines {
		for _, key := range []string{line.IngredientKey, line.IngredientName, line.SearchTerm} {
			key = normalizePantryIngredientKey(key)
			if key != "" {
				out[key] = line
			}
		}
	}
	return out
}

func pantryNutritionByKey(profile PantryProfile) map[string]PantryNutritionEvidence {
	out := map[string]PantryNutritionEvidence{}
	for _, item := range profile.Items {
		if item.Nutrition == nil {
			continue
		}
		keys := []string{item.IngredientKey}
		keys = append(keys, item.Names...)
		keys = append(keys, item.Aliases...)
		for _, key := range keys {
			key = normalizePantryIngredientKey(key)
			if key != "" {
				out[key] = *item.Nutrition
			}
		}
	}
	return out
}

func recipeDeclaredAvailability(plan MealPlan) map[string]bool {
	out := map[string]bool{}
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			if meal.Recipe.NutritionPerServing != nil {
				out[nutritionSlotKey(day.Day, meal.Type, meal.Recipe.ID)] = true
			}
		}
	}
	return out
}

func firstIngredientUsage(req IngredientRequirement) IngredientUsageRef {
	if len(req.Usages) > 0 {
		return req.Usages[0]
	}
	return IngredientUsageRef{
		Day:              req.Day,
		MealSlot:         req.MealSlot,
		RecipeID:         req.RecipeID,
		RecipeTitle:      req.RecipeTitle,
		IngredientKey:    normalizePantryIngredientKey(req.IngredientName),
		IngredientName:   req.IngredientName,
		RequiredQuantity: cloneNormalizedQuantity(req.TotalRequiredQuantity),
	}
}

func firstQuantity(values ...*NormalizedQuantity) *NormalizedQuantity {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func nutritionLineIdentity(line NutritionIngredientLine) string {
	return strings.Join([]string{strconvItoa(line.Day), line.MealSlot, line.RecipeID, line.IngredientKey}, "|")
}

func cloneQuantityRangeExpected(q *QuantityRange) *NormalizedQuantity {
	if q == nil {
		return nil
	}
	return cloneNormalizedQuantity(&q.Expected)
}

func addNutritionCoveredQuantity(current *NormalizedQuantity, add *NormalizedQuantity) *NormalizedQuantity {
	if add == nil {
		return current
	}
	if current == nil {
		return cloneNormalizedQuantity(add)
	}
	if current.BaseUnit != add.BaseUnit {
		return current
	}
	current.BaseValue = roundQty(current.BaseValue + add.BaseValue)
	current.Value = current.BaseValue
	current.Unit = current.BaseUnit
	current.Raw = formatPantryQuantity(current)
	current.Confidence = math.Min(current.Confidence, add.Confidence)
	return current
}

func rollupFromEstimate(estimate NutritionEstimate, source string) *NutritionRollup {
	facts := nutritionFactsFromStructured(estimate.Nutrients, source, nutritionConfidenceLabel(estimate.Confidence))
	if !nutritionFactsHasValues(facts) {
		return nil
	}
	return &NutritionRollup{Facts: facts, Source: source, Warnings: append([]string(nil), estimate.Warnings...)}
}

func estimateFactsForQuantity(q NormalizedQuantity, facts NutritionFacts, basis NutritionEvidenceBasis, source string) (*NutritionRollup, []string) {
	if !nutritionFactsHasValues(facts) {
		return nil, []string{"No nutrition facts were available."}
	}
	factor, warnings := nutritionFactorForBasis(q, basis)
	if len(warnings) > 0 {
		return nil, warnings
	}
	out := scaleNutritionFacts(facts, factor, source)
	return &NutritionRollup{Facts: out, Source: source}, nil
}

func nutritionFactorForBasis(q NormalizedQuantity, basis NutritionEvidenceBasis) (float64, []string) {
	if q.BaseValue <= 0 {
		return 0, []string{"Nutrition estimate unavailable because the quantity is missing."}
	}
	switch basis.Type {
	case NutritionPer100g:
		if q.BaseUnit == "g" {
			return q.BaseValue / 100, nil
		}
	case NutritionPer100ml:
		if q.BaseUnit == "ml" {
			return q.BaseValue / 100, nil
		}
	case NutritionPerUnit:
		if q.BaseUnit == "unit" {
			return q.BaseValue, nil
		}
	}
	return 0, []string{"Nutrition estimate unavailable because quantity unit " + q.BaseUnit + " does not match nutrition basis " + string(basis.Type) + "."}
}

func nutritionFactsFromStructured(n StructuredNutrition, source, confidence string) NutritionFacts {
	return NutritionFacts{
		EnergyKJ:      nutrientAmount(n.EnergyKJ, "kJ", source, confidence),
		EnergyKcal:    nutrientAmount(n.Kcal, "kcal", source, confidence),
		FatG:          nutrientAmount(n.FatG, "g", source, confidence),
		SaturatedFatG: nutrientAmount(n.SaturatesG, "g", source, confidence),
		CarbohydrateG: nutrientAmount(n.CarbsG, "g", source, confidence),
		SugarsG:       nutrientAmount(n.SugarsG, "g", source, confidence),
		FiberG:        nutrientAmount(n.FiberG, "g", source, confidence),
		ProteinG:      nutrientAmount(n.ProteinG, "g", source, confidence),
		SaltG:         nutrientAmount(n.SaltG, "g", source, confidence),
		SodiumG:       nutrientAmount(n.SodiumG, "g", source, confidence),
	}
}

func nutritionEvidenceBasisFromStructured(n StructuredNutrition) NutritionEvidenceBasis {
	var quantity *NormalizedQuantity
	if n.BasisQty > 0 && n.BasisUnit != "" {
		q := quantityFromBase(n.BasisQty, normalizeUnit(n.BasisUnit), n.Confidence, "nutrition_basis")
		quantity = &q
	}
	return NutritionEvidenceBasis{Type: n.Basis, Quantity: quantity, RawText: n.Raw, Confidence: nutritionConfidenceLabel(n.Confidence)}
}

func nutrientAmount(value *float64, unit, source, confidence string) *NutrientAmount {
	if value == nil {
		return nil
	}
	return &NutrientAmount{Value: roundNutritionValue(*value), Unit: unit, Source: source, Confidence: confidence}
}

func nutritionFactsHasValues(f NutritionFacts) bool {
	return f.EnergyKJ != nil || f.EnergyKcal != nil || f.FatG != nil || f.SaturatedFatG != nil || f.CarbohydrateG != nil || f.SugarsG != nil || f.FiberG != nil || f.ProteinG != nil || f.SaltG != nil || f.SodiumG != nil
}

func scaleNutritionFacts(f NutritionFacts, factor float64, source string) NutritionFacts {
	scale := func(v *NutrientAmount) *NutrientAmount {
		if v == nil {
			return nil
		}
		out := *v
		out.Value = roundNutritionValue(v.Value * factor)
		if source != "" {
			out.Source = source
		}
		return &out
	}
	return NutritionFacts{
		EnergyKJ:      scale(f.EnergyKJ),
		EnergyKcal:    scale(f.EnergyKcal),
		FatG:          scale(f.FatG),
		SaturatedFatG: scale(f.SaturatedFatG),
		CarbohydrateG: scale(f.CarbohydrateG),
		SugarsG:       scale(f.SugarsG),
		FiberG:        scale(f.FiberG),
		ProteinG:      scale(f.ProteinG),
		SaltG:         scale(f.SaltG),
		SodiumG:       scale(f.SodiumG),
	}
}

func SumNutritionRollupsPtr(a, b *NutritionRollup) *NutritionRollup {
	if a == nil {
		return cloneNutritionRollup(b)
	}
	if b == nil {
		return cloneNutritionRollup(a)
	}
	out := cloneNutritionRollup(a)
	out.Facts = addNutritionFacts(out.Facts, b.Facts)
	out.Source = mergeNutritionSource(a.Source, b.Source)
	out.Warnings = append(out.Warnings, b.Warnings...)
	return out
}

func addNutritionFacts(a, b NutritionFacts) NutritionFacts {
	add := func(x, y *NutrientAmount) *NutrientAmount {
		if x == nil {
			return cloneNutrientAmount(y)
		}
		if y == nil {
			return cloneNutrientAmount(x)
		}
		out := *x
		out.Value = roundNutritionValue(x.Value + y.Value)
		out.Source = mergeNutritionSource(x.Source, y.Source)
		return &out
	}
	return NutritionFacts{
		EnergyKJ:      add(a.EnergyKJ, b.EnergyKJ),
		EnergyKcal:    add(a.EnergyKcal, b.EnergyKcal),
		FatG:          add(a.FatG, b.FatG),
		SaturatedFatG: add(a.SaturatedFatG, b.SaturatedFatG),
		CarbohydrateG: add(a.CarbohydrateG, b.CarbohydrateG),
		SugarsG:       add(a.SugarsG, b.SugarsG),
		FiberG:        add(a.FiberG, b.FiberG),
		ProteinG:      add(a.ProteinG, b.ProteinG),
		SaltG:         add(a.SaltG, b.SaltG),
		SodiumG:       add(a.SodiumG, b.SodiumG),
	}
}

func DivideNutritionRollup(rollup NutritionRollup, divisor float64) NutritionRollup {
	if divisor <= 0 {
		return rollup
	}
	out := rollup
	out.Facts = scaleNutritionFacts(rollup.Facts, 1/divisor, rollup.Source)
	return out
}

func cloneNutritionRollup(r *NutritionRollup) *NutritionRollup {
	if r == nil {
		return nil
	}
	out := *r
	out.Warnings = append([]string(nil), r.Warnings...)
	return &out
}

func cloneNutrientAmount(v *NutrientAmount) *NutrientAmount {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func mergeNutritionSource(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" || a == b {
		return a
	}
	return "mixed"
}

func nutritionConfidenceLabel(confidence float64) string {
	switch {
	case confidence >= 0.9:
		return "exact"
	case confidence >= 0.7:
		return "parsed"
	case confidence > 0:
		return "low_confidence"
	default:
		return "unknown"
	}
}

func buildRecipeDeclaredNutritionSummaries(plan MealPlan, servingPlan *ServingPlan, policy NutritionPolicy) []RecipeDeclaredNutritionSummary {
	if !policy.AllowRecipeDeclaredNutrition || policy.Mode == NutritionModeLabels || policy.Mode == NutritionModeOff {
		return nil
	}
	slots := map[string]ServingSlot{}
	if servingPlan != nil {
		for _, slot := range servingPlan.Slots {
			slots[servingSlotKey(slot.Day, slot.MealSlot, slot.RecipeID)] = slot
		}
	}
	var out []RecipeDeclaredNutritionSummary
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			if meal.Recipe.NutritionPerServing == nil {
				continue
			}
			slot := slots[servingSlotKey(day.Day, meal.Type, meal.Recipe.ID)]
			cooked := positiveOrDefault(slot.CookedServingUnits, float64(meal.Recipe.Servings))
			perServing := rollupFromNutritionSummary(*meal.Recipe.NutritionPerServing, "recipe_declared")
			summary := RecipeDeclaredNutritionSummary{
				Day:                day.Day,
				MealSlot:           meal.Type,
				RecipeID:           meal.Recipe.ID,
				RecipeTitle:        meal.Recipe.Title,
				Basis:              "per_serving",
				BaseServings:       float64(meal.Recipe.Servings),
				CookedServingUnits: cooked,
				PerCookedServing:   perServing,
				Confidence:         "estimated",
			}
			if perServing != nil {
				total := *perServing
				total.Facts = scaleNutritionFacts(perServing.Facts, cooked, "recipe_declared")
				summary.TotalScaled = &total
			}
			out = append(out, summary)
		}
	}
	return out
}

func rollupFromNutritionSummary(summary NutritionSummary, source string) *NutritionRollup {
	facts := NutritionFacts{
		EnergyKcal:    nutrientAmount(floatPtr(summary.Kcal), "kcal", source, "estimated"),
		ProteinG:      nutrientAmount(floatPtr(summary.ProteinG), "g", source, "estimated"),
		CarbohydrateG: nutrientAmount(floatPtr(summary.CarbsG), "g", source, "estimated"),
		FatG:          nutrientAmount(floatPtr(summary.FatG), "g", source, "estimated"),
	}
	if !nutritionFactsHasValues(facts) {
		return nil
	}
	return &NutritionRollup{Facts: facts, Source: source}
}

func nutritionMealSummaries(lines []NutritionIngredientLine) []NutritionMealSummary {
	grouped := map[string][]NutritionIngredientLine{}
	for _, line := range lines {
		key := nutritionSlotKey(line.Day, line.MealSlot, line.RecipeID)
		grouped[key] = append(grouped[key], line)
	}
	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out []NutritionMealSummary
	for _, key := range keys {
		rows := grouped[key]
		if len(rows) == 0 {
			continue
		}
		var total *NutritionRollup
		var missing []string
		cooked := rows[0].UsageRef.CookedServingUnits
		target := rows[0].UsageRef.TargetServingUnits
		for _, row := range rows {
			total = SumNutritionRollupsPtr(total, row.ConsumedKnown)
			if !row.Coverage.HasAnyNutrition {
				missing = append(missing, row.IngredientName)
			}
			if cooked <= 0 {
				cooked = row.UsageRef.CookedServingUnits
			}
			if target <= 0 {
				target = row.UsageRef.TargetServingUnits
			}
		}
		summary := NutritionMealSummary{
			Day:                rows[0].Day,
			MealSlot:           rows[0].MealSlot,
			RecipeID:           rows[0].RecipeID,
			RecipeTitle:        rows[0].RecipeTitle,
			TargetServingUnits: target,
			CookedServingUnits: cooked,
			TotalKnown:         total,
			Coverage:           NutritionCoverageFromLines(rows),
			MissingIngredients: missing,
		}
		if total != nil && cooked > 0 {
			per := DivideNutritionRollup(*total, cooked)
			summary.PerCookedServingKnown = &per
		}
		out = append(out, summary)
	}
	return out
}

func nutritionDaySummaries(lines []NutritionIngredientLine) []NutritionDaySummary {
	grouped := map[int][]NutritionIngredientLine{}
	for _, line := range lines {
		grouped[line.Day] = append(grouped[line.Day], line)
	}
	var days []int
	for day := range grouped {
		days = append(days, day)
	}
	sort.Ints(days)
	var out []NutritionDaySummary
	for _, day := range days {
		rows := grouped[day]
		var total *NutritionRollup
		for _, row := range rows {
			total = SumNutritionRollupsPtr(total, row.ConsumedKnown)
		}
		out = append(out, NutritionDaySummary{Day: day, TotalKnown: total, Coverage: NutritionCoverageFromLines(rows)})
	}
	return out
}

func nutritionSlotKey(day int, mealSlot, recipeID string) string {
	return strings.Join([]string{strconvItoa(day), strings.TrimSpace(mealSlot), strings.TrimSpace(recipeID)}, "|")
}

func strconvItoa(value int) string {
	return strconv.FormatInt(int64(value), 10)
}
