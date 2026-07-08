package food

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/strutil"
)

type RecipeSwapOptions struct {
	Enabled           bool
	Policy            RecipeSwapPolicy
	Profile           Profile
	Pantry            Pantry
	HouseholdProfile  HouseholdProfile
	ServingPolicy     ServingPolicy
	PantryProfile     PantryProfile
	PantryPolicy      PantryPolicy
	SelectionPolicy   string
	SearchLimit       int
	Meals             []string
	LedgerOptions     QuantityLedgerOptions
	RecoveryOptions   RecoveryOptions
	ReadinessPolicy   ReadinessPolicy
	CandidateProvider RecipeCandidateProvider
}

type CatalogRecipeCandidateProvider struct{}

func (CatalogRecipeCandidateProvider) Candidates(ctx context.Context, req RecipeSwapRequest) ([]RecipeCandidateRecipe, error) {
	_ = ctx
	limit := req.MaxCandidates * 4
	if limit <= 0 {
		limit = 24
	}
	recipes, err := SearchRecipes(RecipeQuery{
		Tags:      []string{req.MealSlot},
		Diets:     req.Profile.Diets,
		Allergies: req.Profile.Allergies,
		Dislikes:  req.Profile.Dislikes,
		Limit:     limit,
	})
	if err != nil {
		return nil, err
	}
	var out []RecipeCandidateRecipe
	for _, recipe := range recipes {
		if recipe.ID == req.OriginalRecipe.ID {
			continue
		}
		scaled := withNutrition(scaleRecipe(recipe, req.Servings))
		out = append(out, RecipeCandidateRecipe{Recipe: scaled, Source: "catalog"})
		if req.MaxCandidates > 0 && len(out) >= req.MaxCandidates {
			break
		}
	}
	return out, nil
}

func RepairFoodRunWithRecipeSwaps(ctx context.Context, artifact FoodRunArtifact, client *alcampo.Client, opts RecipeSwapOptions) (FoodRunArtifact, RecipeSwapPlan, error) {
	opts = normalizeRecipeSwapOptions(opts)
	if artifact.MealPlanFingerprint == "" {
		artifact.MealPlanFingerprint = MealPlanFingerprint(artifact.MealPlan)
	}
	plan := RecipeSwapPlan{
		SchemaVersion:             "1",
		Status:                    RecipeSwapNotNeeded,
		Policy:                    opts.Policy,
		BeforeMealPlanFingerprint: artifact.MealPlanFingerprint,
	}
	if artifact.QuantityLedger != nil {
		ledgerCopy := *artifact.QuantityLedger
		artifact.PreRecipeSwapQuantityLedger = &ledgerCopy
	}
	preGate := EvaluateReadinessGate(&artifact, opts.ReadinessPolicy)
	artifact.PreRecipeSwapReadinessGate = &preGate
	plan.FinalReadinessStatus = string(preGate.Status)
	plan.FinalSafeToBuild = preGate.SafeToBuild
	if preGate.SafeToBuild {
		plan.Status = RecipeSwapNotNeeded
		artifact.RecipeSwapPlan = &plan
		return artifact, plan, nil
	}
	plan.TriggeredByIssues = gateIssueRefs(preGate.BlockingIssues)
	plan.AffectedSlots = affectedRecipeSlots(artifact, preGate)
	if !opts.Enabled || !opts.Policy.AllowRecipeSwap || client == nil {
		plan.Status = RecipeSwapDisabled
		plan.RemainingIssues = append([]GateIssue(nil), preGate.BlockingIssues...)
		artifact.RecipeSwapPlan = &plan
		return artifact, plan, nil
	}
	if len(plan.AffectedSlots) == 0 {
		plan.Status = RecipeSwapAttemptedFailed
		plan.RemainingIssues = append([]GateIssue(nil), preGate.BlockingIssues...)
		artifact.RecipeSwapPlan = &plan
		return artifact, plan, nil
	}

	current := artifact
	totalChecks := 0
	for len(plan.AppliedSwaps) < opts.Policy.MaxAppliedSwaps && totalChecks < opts.Policy.MaxTotalCandidateChecks {
		currentGate := EvaluateReadinessGate(&current, opts.ReadinessPolicy)
		if currentGate.SafeToBuild {
			break
		}
		slots := affectedRecipeSlots(current, currentGate)
		if len(slots) == 0 {
			break
		}
		appliedThisRound := false
		for _, slot := range slots {
			if totalChecks >= opts.Policy.MaxTotalCandidateChecks || len(plan.AppliedSwaps) >= opts.Policy.MaxAppliedSwaps {
				break
			}
			attempt, bestArtifact, bestCandidate, ok, err := attemptRecipeSwapForSlot(ctx, current, client, opts, slot, &totalChecks)
			plan.Attempts = append(plan.Attempts, attempt)
			if err != nil {
				return current, plan, err
			}
			if !ok {
				continue
			}
			bestArtifact.PreRecipeSwapQuantityLedger = artifact.PreRecipeSwapQuantityLedger
			bestArtifact.PreRecipeSwapReadinessGate = artifact.PreRecipeSwapReadinessGate
			swap := appliedRecipeSwapFromCandidate(len(plan.AppliedSwaps)+1, current, slot, bestCandidate, bestArtifact)
			bestArtifact.RecipeAdjustments = append(bestArtifact.RecipeAdjustments, recipeSwapAdjustment(swap))
			plan.AppliedSwaps = append(plan.AppliedSwaps, swap)
			plan.AfterMealPlanFingerprint = bestArtifact.MealPlanFingerprint
			current = bestArtifact
			appliedThisRound = true
			break
		}
		if !appliedThisRound {
			break
		}
	}

	finalGate := ApplyReadinessGate(&current, opts.ReadinessPolicy)
	plan.FinalReadinessStatus = string(finalGate.Status)
	plan.FinalSafeToBuild = finalGate.SafeToBuild
	plan.AfterMealPlanFingerprint = current.MealPlanFingerprint
	if finalGate.SafeToBuild && len(plan.AppliedSwaps) > 0 {
		plan.Status = RecipeSwapAttemptedApplied
	} else if len(plan.AppliedSwaps) == 0 {
		plan.Status = RecipeSwapAttemptedFailed
		plan.RemainingIssues = append([]GateIssue(nil), finalGate.BlockingIssues...)
	} else {
		plan.Status = RecipeSwapAttemptedFailed
		plan.RemainingIssues = append([]GateIssue(nil), finalGate.BlockingIssues...)
	}
	current.RecipeSwapPlan = &plan
	finalGate = ApplyReadinessGate(&current, opts.ReadinessPolicy)
	plan.FinalReadinessStatus = string(finalGate.Status)
	plan.FinalSafeToBuild = finalGate.SafeToBuild
	current.RecipeSwapPlan = &plan
	return current, plan, nil
}

func normalizeRecipeSwapOptions(opts RecipeSwapOptions) RecipeSwapOptions {
	if opts.Policy.MaxAppliedSwaps <= 0 {
		opts.Policy.MaxAppliedSwaps = 2
	}
	if opts.Policy.MaxCandidatesPerSlot <= 0 {
		opts.Policy.MaxCandidatesPerSlot = 8
	}
	if opts.Policy.MaxTotalCandidateChecks <= 0 {
		opts.Policy.MaxTotalCandidateChecks = 20
	}
	opts.Policy.PreserveMealSlotType = true
	opts.Policy.PreserveDietaryRules = true
	opts.Policy.PreserveAllergenRules = true
	opts.Policy.PreserveServings = true
	opts.Policy.StrictQuantity = opts.ReadinessPolicy.StrictQuantity
	opts.Policy.AllowEstimatedVariableWeight = opts.ReadinessPolicy.AllowEstimatedVariableWeight
	opts.Policy.AllowLowConfidenceBasket = opts.ReadinessPolicy.AllowLowConfidenceBasket
	if opts.SearchLimit <= 0 {
		opts.SearchLimit = 8
	}
	if strings.TrimSpace(opts.SelectionPolicy) == "" {
		opts.SelectionPolicy = strutil.FirstNonEmpty(opts.Profile.SelectionPolicy, PolicyBalanced)
	}
	if opts.CandidateProvider == nil {
		opts.CandidateProvider = CatalogRecipeCandidateProvider{}
	}
	return opts
}

func attemptRecipeSwapForSlot(ctx context.Context, artifact FoodRunArtifact, client *alcampo.Client, opts RecipeSwapOptions, slot AffectedRecipeSlot, totalChecks *int) (RecipeSwapAttempt, FoodRunArtifact, RecipeSwapCandidate, bool, error) {
	original, ok := mealForSlot(artifact.MealPlan, slot)
	attempt := RecipeSwapAttempt{
		ID:                  fmt.Sprintf("swap-attempt-%03d-%s", len(artifact.RecipeAdjustments)+1, normalizeKey(slot.MealSlot)),
		Day:                 slot.Day,
		MealSlot:            slot.MealSlot,
		OriginalRecipeID:    slot.RecipeID,
		OriginalRecipeTitle: slot.RecipeTitle,
		Status:              "no_valid_candidate",
		Reason:              "no validated replacement recipe made the basket safe",
	}
	if !ok {
		attempt.Status = "error"
		attempt.Reason = "affected recipe slot was not found in the meal plan"
		return attempt, FoodRunArtifact{}, RecipeSwapCandidate{}, false, nil
	}
	if slot.IsPinned && !opts.Policy.AllowSwappingPinnedRecipes {
		attempt.Status = "skipped_pinned"
		attempt.Reason = "recipe slot is pinned and recipe swap policy does not allow changing it"
		return attempt, FoodRunArtifact{}, RecipeSwapCandidate{}, false, nil
	}
	req := RecipeSwapRequest{
		Day:                    slot.Day,
		MealSlot:               slot.MealSlot,
		OriginalRecipe:         original.Recipe,
		BlockingIngredientKeys: append([]string(nil), slot.BlockingIngredientKeys...),
		Servings:               artifact.MealPlan.People,
		Profile:                opts.Profile,
		MaxCandidates:          opts.Policy.MaxCandidatesPerSlot,
	}
	candidates, err := opts.CandidateProvider.Candidates(ctx, req)
	if err != nil {
		attempt.Status = "error"
		attempt.Reason = err.Error()
		return attempt, FoodRunArtifact{}, RecipeSwapCandidate{}, false, nil
	}
	attempt.CandidateCount = len(candidates)
	var bestArtifact FoodRunArtifact
	var best RecipeSwapCandidate
	for i, candidate := range candidates {
		if *totalChecks >= opts.Policy.MaxTotalCandidateChecks {
			break
		}
		*totalChecks++
		candidateID := fmt.Sprintf("%s-candidate-%03d", attempt.ID, i+1)
		report, candidateArtifact := validateRecipeSwapCandidate(ctx, artifact, client, opts, slot, candidate, candidateID)
		attempt.Candidates = append(attempt.Candidates, report)
		if !report.Valid {
			continue
		}
		if best.ID == "" || report.Score > best.Score {
			best = report
			bestArtifact = candidateArtifact
		}
	}
	if best.ID == "" {
		return attempt, FoodRunArtifact{}, RecipeSwapCandidate{}, false, nil
	}
	attempt.Status = "applied"
	attempt.Reason = "validated replacement recipe passed the final readiness gate"
	attempt.AppliedCandidateID = best.ID
	return attempt, bestArtifact, best, true, nil
}

func validateRecipeSwapCandidate(ctx context.Context, artifact FoodRunArtifact, client *alcampo.Client, opts RecipeSwapOptions, slot AffectedRecipeSlot, candidate RecipeCandidateRecipe, candidateID string) (RecipeSwapCandidate, FoodRunArtifact) {
	report := RecipeSwapCandidate{
		ID:             candidateID,
		RecipeID:       candidate.Recipe.ID,
		Title:          candidate.Recipe.Title,
		Source:         strutil.FirstNonEmpty(candidate.Source, "catalog"),
		HasRecipeImage: strings.TrimSpace(candidate.Recipe.ImageURL) != "",
	}
	if codes, reason := rejectRecipeSwapCandidate(candidate.Recipe, slot, opts.Profile); len(codes) > 0 {
		report.Rejected = true
		report.RejectCodes = codes
		report.RejectReason = reason
		return report, FoodRunArtifact{}
	}
	plan := replaceRecipeInPlan(artifact.MealPlan, slot, candidate.Recipe)
	var servingPlan ServingPlan
	var scaledMealPlan ScaledMealPlan
	if opts.ServingPolicy.Enabled && artifact.ServingPlan != nil && artifact.ScaledMealPlan != nil {
		plan, servingPlan, scaledMealPlan = ReplaceScaledRecipeSlot(plan, *artifact.ServingPlan, *artifact.ScaledMealPlan, slot.Day, slot.MealSlot, candidate.Recipe, ScalingPolicyFromServingPolicy(opts.ServingPolicy))
	}
	plan = RebuildMealPlanPurchases(plan, opts.Profile, opts.Pantry)
	var pantryResolution PantryResolution
	ledgerOpts := opts.LedgerOptions
	if opts.PantryPolicy.Enabled {
		plan, pantryResolution = ApplyPantryResolution(plan, opts.PantryProfile, opts.PantryPolicy)
	}
	shop, err := ShopMealPlan(ctx, client, plan, opts.Profile, ShopOptions{Policy: opts.SelectionPolicy, Limit: opts.SearchLimit})
	if err != nil {
		report.Rejected = true
		report.RejectCodes = []string{"shop_failed"}
		report.RejectReason = err.Error()
		return report, FoodRunArtifact{}
	}
	shop = EnrichShopResultProducts(ctx, shop, client, ledgerOpts)
	next := NewFoodRunArtifact(plan, shop, artifact.PDFPath, artifact.BasketPath, opts.Meals)
	if opts.ServingPolicy.Enabled && servingPlan.ServingPlanFingerprint != "" {
		next = AttachServingScaling(next, opts.HouseholdProfile, servingPlan, scaledMealPlan)
		ledgerOpts.ServingPlanFingerprint = next.ServingPlanFingerprint
		ledgerOpts.ScaledMealPlanFingerprint = next.ScaledMealPlanFingerprint
	}
	if opts.PantryPolicy.Enabled {
		next = AttachPantryResolution(next, opts.PantryProfile, pantryResolution)
		ledgerOpts.PantryResolution = next.PantryResolution
		ledgerOpts.PantryResolutionFingerprint = next.PantryResolutionFingerprint
		ledgerOpts.ShopRequirementsFingerprint = next.ShopRequirementsFingerprint
	}
	ledger := BuildQuantityLedger(plan, shop, ledgerOpts)
	safety := BasketSafetyFromLedger(ledger, ledgerOpts)
	next.QuantityLedger = &ledger
	next.BasketSafety = &safety
	recoveryOpts := opts.RecoveryOptions
	recoveryOpts.AllowRecipeSwap = false
	recoveryOpts.LedgerOptions = ledgerOpts
	if recoveryOpts.Enabled {
		next = RecoverFoodRun(ctx, next, client, recoveryOpts)
	}
	next = RefreshFoodRunArtifact(next, opts.Meals)
	gate := ApplyReadinessGate(&next, opts.ReadinessPolicy)
	report.Validation = validationFromArtifact(next, gate)
	report.CostDeltaCents = int(next.Shop.EstimatedTotal.Cents - artifact.Shop.EstimatedTotal.Cents)
	report.ProductLineDelta = len(next.Shop.SelectedProducts) - len(artifact.Shop.SelectedProducts)
	report.NutritionComparable = artifact.MealPlan.Nutrition != nil && next.MealPlan.Nutrition != nil
	if report.NutritionComparable {
		report.NutritionDelta = nutritionDelta(artifact.MealPlan.Nutrition, next.MealPlan.Nutrition)
	}
	if gate.SafeToBuild && len(gate.BlockingIssues) == 0 {
		report.Valid = true
		report.Score = recipeSwapScore(report, gate)
		return report, next
	}
	report.Rejected = true
	report.RejectCodes = append(report.RejectCodes, "readiness_blocked")
	report.RejectReason = "candidate did not pass final readiness gate"
	return report, FoodRunArtifact{}
}

func rejectRecipeSwapCandidate(recipe Recipe, slot AffectedRecipeSlot, profile Profile) ([]string, string) {
	var codes []string
	if strings.TrimSpace(recipe.ID) == "" || strings.TrimSpace(recipe.Title) == "" {
		codes = append(codes, "recipe_identity_missing")
	}
	if len(recipe.Steps) == 0 {
		codes = append(codes, "recipe_steps_missing")
	}
	text := recipeText(recipe)
	if containsAny(text, profile.Allergies) {
		codes = append(codes, "allergy_conflict")
	}
	if containsAny(text, profile.Dislikes) {
		codes = append(codes, "dislike_conflict")
	}
	if len(profile.Diets) > 0 && !fitsDiets(recipe, profile.Diets) {
		codes = append(codes, "diet_conflict")
	}
	blocking := stringSet(slot.BlockingIngredientKeys)
	for _, ingredient := range recipe.Ingredients {
		key := normalizeKey(ingredient.Name)
		if blocking[key] {
			codes = append(codes, "same_blocked_ingredient")
		}
		if !ingredient.Optional {
			if _, ok := normalizedIngredientQuantity(ingredient.Quantity, ingredient.Unit, 0.95, "recipe_swap_candidate"); !ok {
				codes = append(codes, "required_quantity_unparseable")
			}
		}
		if key == "meat" || key == "vegetables" || key == "sauce" {
			codes = append(codes, "vague_required_ingredient")
		}
	}
	if len(codes) == 0 {
		return nil, ""
	}
	return dedupeStrings(codes), strings.Join(dedupeStrings(codes), ", ")
}

func validationFromArtifact(artifact FoodRunArtifact, gate ReadinessGate) CandidateShoppingValidation {
	out := CandidateShoppingValidation{
		ReadinessStatus:      string(gate.Status),
		SafeToBuild:          gate.SafeToBuild,
		BasketSafe:           artifact.BasketSafety != nil && artifact.BasketSafety.SafeToBuild,
		SelectedProductCount: len(artifact.Shop.SelectedProducts),
		BlockingIssues:       append([]GateIssue(nil), gate.BlockingIssues...),
		Warnings:             append([]GateIssue(nil), gate.Warnings...),
	}
	if artifact.QuantityLedger != nil {
		out.LedgerStatus = string(artifact.QuantityLedger.Status)
		out.EstimatedLines = artifact.QuantityLedger.Summary.EstimatedVariableWeightLines
		out.LowConfidenceLines = artifact.QuantityLedger.Summary.NeedsReviewLines
		for _, allocation := range artifact.QuantityLedger.Allocations {
			if allocation.MatchType == "missing" {
				out.MissingIngredientKeys = append(out.MissingIngredientKeys, normalizeKey(requirementNameByID(*artifact.QuantityLedger, allocation.RequirementID)))
			}
		}
	}
	for _, selected := range artifact.Shop.SelectedProducts {
		if selected.Error != "" {
			out.RejectedProductCount++
		}
	}
	return out
}

func recipeSwapScore(candidate RecipeSwapCandidate, gate ReadinessGate) float64 {
	score := 1000.0
	if gate.Status == ReadinessReadyExact {
		score += 250
	}
	score -= float64(candidate.Validation.EstimatedLines * 30)
	score -= float64(candidate.Validation.LowConfidenceLines * 50)
	score -= math.Max(float64(candidate.CostDeltaCents), 0) / 100.0
	score -= math.Max(float64(candidate.ProductLineDelta), 0) * 5
	if candidate.HasRecipeImage {
		score += 10
	}
	return score
}

func affectedRecipeSlots(artifact FoodRunArtifact, gate ReadinessGate) []AffectedRecipeSlot {
	reqsByIngredient := map[string][]IngredientRequirement{}
	if artifact.QuantityLedger != nil {
		for _, req := range artifact.QuantityLedger.Requirements {
			reqsByIngredient[normalizeKey(req.IngredientName)] = append(reqsByIngredient[normalizeKey(req.IngredientName)], req)
		}
	}
	slotsByKey := map[string]*AffectedRecipeSlot{}
	for _, issue := range gate.BlockingIssues {
		if !swappableGateIssue(issue) {
			continue
		}
		refs := usageRefsForIssue(issue, reqsByIngredient)
		for _, ref := range refs {
			key := fmt.Sprintf("%d|%s|%s", ref.Day, normalizeKey(ref.MealSlot), ref.RecipeID)
			slot := slotsByKey[key]
			if slot == nil {
				slot = &AffectedRecipeSlot{
					Day:         ref.Day,
					MealSlot:    ref.MealSlot,
					RecipeID:    ref.RecipeID,
					RecipeTitle: ref.RecipeTitle,
				}
				slotsByKey[key] = slot
			}
			slot.BlockingIngredientKeys = appendUniqueString(slot.BlockingIngredientKeys, strutil.FirstNonEmpty(issue.IngredientKey, ref.IngredientKey))
			slot.BlockingIssueCodes = appendUniqueString(slot.BlockingIssueCodes, issue.Code)
		}
	}
	var slots []AffectedRecipeSlot
	for _, slot := range slotsByKey {
		slots = append(slots, *slot)
	}
	sort.SliceStable(slots, func(i, j int) bool {
		if len(slots[i].BlockingIssueCodes) != len(slots[j].BlockingIssueCodes) {
			return len(slots[i].BlockingIssueCodes) > len(slots[j].BlockingIssueCodes)
		}
		if slots[i].Day != slots[j].Day {
			return slots[i].Day < slots[j].Day
		}
		return slots[i].MealSlot < slots[j].MealSlot
	})
	return slots
}

func swappableGateIssue(issue GateIssue) bool {
	switch issue.Code {
	case "required_ingredient_missing_product", "missing_required_quantity", "recovery_issue_remaining", "low_confidence_allocation_blocked", "estimated_allocation_blocked", "final_ledger_incomplete":
		return issue.IngredientKey != "" || issue.IngredientName != "" || issue.RecipeID != ""
	default:
		return false
	}
}

func usageRefsForIssue(issue GateIssue, reqsByIngredient map[string][]IngredientRequirement) []IngredientUsageRef {
	var refs []IngredientUsageRef
	ingredientKey := normalizeKey(strutil.FirstNonEmpty(issue.IngredientKey, issue.IngredientName))
	for _, req := range reqsByIngredient[ingredientKey] {
		if len(req.Usages) > 0 {
			refs = append(refs, req.Usages...)
			continue
		}
		refs = append(refs, IngredientUsageRef{Day: req.Day, MealSlot: req.MealSlot, RecipeID: req.RecipeID, RecipeTitle: req.RecipeTitle, IngredientKey: normalizeKey(req.IngredientName), IngredientName: req.IngredientName})
	}
	if len(refs) == 0 && issue.RecipeID != "" {
		refs = append(refs, IngredientUsageRef{Day: issue.Day, MealSlot: issue.MealSlot, RecipeID: issue.RecipeID, IngredientKey: ingredientKey, IngredientName: issue.IngredientName})
	}
	return refs
}

func mealForSlot(plan MealPlan, slot AffectedRecipeSlot) (Meal, bool) {
	for _, day := range plan.Days {
		if day.Day != slot.Day {
			continue
		}
		for _, meal := range day.Meals {
			if normalizeKey(meal.Type) == normalizeKey(slot.MealSlot) && meal.Recipe.ID == slot.RecipeID {
				return meal, true
			}
		}
	}
	return Meal{}, false
}

func replaceRecipeInPlan(plan MealPlan, slot AffectedRecipeSlot, recipe Recipe) MealPlan {
	for di := range plan.Days {
		if plan.Days[di].Day != slot.Day {
			continue
		}
		for mi := range plan.Days[di].Meals {
			meal := &plan.Days[di].Meals[mi]
			if normalizeKey(meal.Type) == normalizeKey(slot.MealSlot) && meal.Recipe.ID == slot.RecipeID {
				meal.Recipe = recipe
				meal.PlanningReason = "replaced blocked recipe after Alcampo availability validation"
			}
		}
	}
	return plan
}

func appliedRecipeSwapFromCandidate(index int, before FoodRunArtifact, slot AffectedRecipeSlot, candidate RecipeSwapCandidate, after FoodRunArtifact) AppliedRecipeSwap {
	return AppliedRecipeSwap{
		ID:                     fmt.Sprintf("recipe-swap-%03d", index),
		Day:                    slot.Day,
		MealSlot:               slot.MealSlot,
		OriginalRecipeID:       slot.RecipeID,
		OriginalRecipeTitle:    slot.RecipeTitle,
		ReplacementRecipeID:    candidate.RecipeID,
		ReplacementRecipeTitle: candidate.Title,
		Reason:                 "replacement recipe passed full shopping, quantity ledger, recovery, and readiness validation",
		ResolvedIssueCodes:     append([]string(nil), slot.BlockingIssueCodes...),
		RemovedIngredientKeys:  sortedIngredientKeys(recipeIngredientsByID(before.MealPlan, slot.RecipeID)),
		AddedIngredientKeys:    sortedIngredientKeys(recipeIngredientsByID(after.MealPlan, candidate.RecipeID)),
		CostDeltaCents:         candidate.CostDeltaCents,
		CandidateID:            candidate.ID,
	}
}

func recipeSwapAdjustment(swap AppliedRecipeSwap) RecipeAdjustment {
	return RecipeAdjustment{
		ID:                     "adjustment-" + swap.ID,
		Type:                   "recipe_swap",
		Day:                    swap.Day,
		MealSlot:               swap.MealSlot,
		RecipeID:               swap.ReplacementRecipeID,
		OriginalRecipeID:       swap.OriginalRecipeID,
		OriginalRecipeTitle:    swap.OriginalRecipeTitle,
		ReplacementRecipeID:    swap.ReplacementRecipeID,
		ReplacementRecipeTitle: swap.ReplacementRecipeTitle,
		RecipeSwapID:           swap.ID,
		Instruction:            fmt.Sprintf("Replaced %s with %s because the original recipe could not be safely shopped at Alcampo.", swap.OriginalRecipeTitle, swap.ReplacementRecipeTitle),
	}
}

func recipeIngredientsByID(plan MealPlan, recipeID string) []Ingredient {
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			if meal.Recipe.ID == recipeID {
				return meal.Recipe.Ingredients
			}
		}
	}
	return nil
}

func gateIssueRefs(issues []GateIssue) []GateIssueRef {
	var out []GateIssueRef
	for _, issue := range issues {
		out = append(out, GateIssueRef{
			Code:           issue.Code,
			Phase:          issue.Phase,
			IngredientKey:  issue.IngredientKey,
			IngredientName: issue.IngredientName,
			RecipeID:       issue.RecipeID,
			Day:            issue.Day,
			MealSlot:       issue.MealSlot,
			Message:        issue.Message,
		})
	}
	return out
}

func requirementNameByID(ledger QuantityLedger, id string) string {
	for _, req := range ledger.Requirements {
		if req.RequirementID == id {
			return req.IngredientName
		}
	}
	return ""
}

func nutritionDelta(before, after *NutritionSummary) *NutritionDeltaSummary {
	if before == nil || after == nil {
		return nil
	}
	return &NutritionDeltaSummary{
		KcalDelta:     math.Round(after.Kcal - before.Kcal),
		ProteinGDelta: math.Round((after.ProteinG-before.ProteinG)*10) / 10,
		CarbsGDelta:   math.Round((after.CarbsG-before.CarbsG)*10) / 10,
		FatGDelta:     math.Round((after.FatG-before.FatG)*10) / 10,
	}
}

func stringSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		key := normalizeKey(value)
		if key != "" {
			out[key] = true
		}
	}
	return out
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func dedupeStrings(values []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
