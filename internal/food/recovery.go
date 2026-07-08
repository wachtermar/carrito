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

type RecoveryClient interface {
	ProductDetailFetcher
	Search(context.Context, string, alcampo.SearchOptions) ([]alcampo.Product, error)
}

type RecoveryOptions struct {
	Enabled                     bool
	StoreID                     string
	Policy                      string
	Profile                     Profile
	SearchLimit                 int
	MaxSearches                 int
	AllowIngredientSubstitution bool
	AllowRecipeSwap             bool
	LedgerOptions               QuantityLedgerOptions
}

type recoveryAliasRule struct {
	CanonicalName string
	FoodClass     string
	RequiredForm  string
	Aliases       []string
	PrepAliases   []string
	RejectTerms   []string
}

var recoveryAliasRules = []recoveryAliasRule{
	{
		CanonicalName: "beef strips",
		FoodClass:     "beef",
		RequiredForm:  "strips",
		Aliases: []string{
			"tiras de ternera",
			"ternera en tiras",
			"carne de ternera en tiras",
			"tiras de vacuno",
			"ternera para fajitas",
			"carne de vacuno para saltear",
		},
		PrepAliases: []string{
			"filetes de ternera",
			"filete fino de ternera",
			"filetes finos de vacuno",
		},
		RejectTerms: []string{"picada", "hamburguesa", "hamburguesas", "burger", "albóndiga", "albondiga", "carne picada"},
	},
	{
		CanonicalName: "chicken breast",
		FoodClass:     "chicken",
		RequiredForm:  "breast",
		Aliases:       []string{"pechuga de pollo", "filetes de pollo", "pechuga pollo"},
		PrepAliases:   []string{"solomillos de pollo"},
		RejectTerms:   []string{"nuggets", "empanado", "hamburguesa"},
	},
}

func RecoverFoodRun(ctx context.Context, artifact FoodRunArtifact, client RecoveryClient, opts RecoveryOptions) FoodRunArtifact {
	if artifact.MealPlanFingerprint == "" {
		artifact.MealPlanFingerprint = MealPlanFingerprint(artifact.MealPlan)
	}
	if artifact.ProductSelectionFingerprint == "" {
		artifact.ProductSelectionFingerprint = ProductSelectionFingerprint(artifact.Shop)
	}
	if artifact.QuantityLedger == nil {
		ledger := BuildQuantityLedger(artifact.MealPlan, artifact.Shop, opts.LedgerOptions)
		artifact.QuantityLedger = &ledger
	}
	startLedger := *artifact.QuantityLedger
	startSafety := BasketSafetyFromLedger(startLedger, opts.LedgerOptions)
	artifact.PreRecoveryQuantityLedger = &startLedger
	issues := ExtractRecoveryIssues(&startLedger, &startSafety)
	plan := RecoveryPlan{
		SchemaVersion:               "1",
		Status:                      RecoveryStatusNotNeeded,
		MealPlanFingerprint:         artifact.MealPlanFingerprint,
		ServingPlanFingerprint:      artifact.ServingPlanFingerprint,
		ScaledMealPlanFingerprint:   artifact.ScaledMealPlanFingerprint,
		ProductSelectionFingerprint: artifact.ProductSelectionFingerprint,
		PantryResolutionFingerprint: artifact.PantryResolutionFingerprint,
		ShopRequirementsFingerprint: artifact.ShopRequirementsFingerprint,
		StartedFromStatus:           string(startLedger.Status),
		Issues:                      issues,
	}
	if !opts.Enabled || len(issues) == 0 || client == nil {
		plan.MealPlanFingerprint = artifact.MealPlanFingerprint
		plan.ServingPlanFingerprint = artifact.ServingPlanFingerprint
		plan.ScaledMealPlanFingerprint = artifact.ScaledMealPlanFingerprint
		plan.ProductSelectionFingerprint = artifact.ProductSelectionFingerprint
		plan.PantryResolutionFingerprint = artifact.PantryResolutionFingerprint
		plan.ShopRequirementsFingerprint = artifact.ShopRequirementsFingerprint
		finalizeRecoveryPlan(&plan, artifact.QuantityLedger, artifact.BasketSafety)
		artifact.RecoveryPlan = &plan
		return artifact
	}

	for _, issue := range issues {
		attempt, decision, selection, ok := recoverIssue(ctx, artifact, client, issue, opts)
		plan.Attempts = append(plan.Attempts, attempt)
		if !ok {
			continue
		}
		artifact = applyRecoveryDecision(artifact, decision)
		artifact.Shop = replaceRecoveredSelection(artifact.Shop, issue, selection, decision)
		plan.AppliedDecisions = append(plan.AppliedDecisions, decision)
	}

	artifact.Shop = EnrichShopResultProducts(ctx, artifact.Shop, client, opts.LedgerOptions)
	artifact.ProductSelectionFingerprint = ProductSelectionFingerprint(artifact.Shop)
	artifact.IngredientLinks = foodRunIngredientLinks(artifact.MealPlan, artifact.Shop)
	artifact.Warnings = foodRunWarnings(artifact.MealPlan, artifact.Shop)
	ledger := BuildQuantityLedger(artifact.MealPlan, artifact.Shop, opts.LedgerOptions)
	safety := BasketSafetyFromLedger(ledger, opts.LedgerOptions)
	artifact.QuantityLedger = &ledger
	artifact.BasketSafety = &safety
	plan.RemainingIssues = ExtractRecoveryIssues(&ledger, &safety)
	plan.MealPlanFingerprint = artifact.MealPlanFingerprint
	plan.ServingPlanFingerprint = artifact.ServingPlanFingerprint
	plan.ScaledMealPlanFingerprint = artifact.ScaledMealPlanFingerprint
	plan.ProductSelectionFingerprint = artifact.ProductSelectionFingerprint
	plan.PantryResolutionFingerprint = artifact.PantryResolutionFingerprint
	plan.ShopRequirementsFingerprint = artifact.ShopRequirementsFingerprint
	finalizeRecoveryPlan(&plan, artifact.QuantityLedger, artifact.BasketSafety)
	artifact.RecoveryPlan = &plan
	return artifact
}

func ExtractRecoveryIssues(ledger *QuantityLedger, safety *BasketSafety) []RecoveryIssue {
	if ledger == nil {
		return nil
	}
	requirements := make(map[string]IngredientRequirement, len(ledger.Requirements))
	for _, req := range ledger.Requirements {
		requirements[req.RequirementID] = req
	}
	var issues []RecoveryIssue
	for i, allocation := range ledger.Allocations {
		req := requirements[allocation.RequirementID]
		switch {
		case allocation.MatchType == "missing":
			issues = append(issues, recoveryIssueFromAllocation(i, RecoveryIssueMissingProduct, req, allocation, firstAllocationWarning(allocation, "No selected product covers this ingredient.")))
		case containsString(allocation.Badges, "LOW QUANTITY CONFIDENCE"):
			issues = append(issues, recoveryIssueFromAllocation(i, RecoveryIssueLowConfidenceProduct, req, allocation, firstAllocationWarning(allocation, "Selected product needs quantity review.")))
		case safety != nil && !safety.SafeToBuild && containsString(allocation.Badges, "ESTIMATED WEIGHT"):
			issues = append(issues, recoveryIssueFromAllocation(i, RecoveryIssueVariableWeightBlocked, req, allocation, "Variable-weight estimate is blocked by basket safety policy."))
		}
	}
	return issues
}

func recoverIssue(ctx context.Context, artifact FoodRunArtifact, client RecoveryClient, issue RecoveryIssue, opts RecoveryOptions) (RecoveryAttempt, RecoveryDecision, SelectedProduct, bool) {
	rule, ok := recoveryRuleForIngredient(issue.IngredientName)
	if !ok {
		return RecoveryAttempt{IssueID: issue.ID, Strategy: "alias_search", FailureReason: "no deterministic recovery aliases for ingredient"}, RecoveryDecision{}, SelectedProduct{}, false
	}
	maxSearches := opts.MaxSearches
	if maxSearches <= 0 {
		maxSearches = 8
	}
	limit := opts.SearchLimit
	if limit <= 0 {
		limit = 8
	}
	queries := recoveryQueries(rule, maxSearches)
	attempt := RecoveryAttempt{IssueID: issue.ID, Strategy: "alias_search", Queries: queries}
	var best *recoverySelection
	for _, query := range queries {
		products, err := client.Search(ctx, query, alcampo.SearchOptions{Limit: limit, RegionID: opts.StoreID})
		if err != nil {
			attempt.Candidates = append(attempt.Candidates, RecoveryCandidate{
				ID:            candidateID(issue.ID, query, 0),
				CandidateType: "product",
				Compatibility: "rejected",
				Confidence:    "rejected",
				RejectReasons: []string{err.Error()},
			})
			continue
		}
		selection := SelectProduct(recoveryIngredient(issue), products, opts.Profile, strutil.FirstNonEmpty(opts.Policy, PolicyBalanced))
		candidate := recoveryCandidateFromSelection(issue, query, selection, rule)
		attempt.Candidates = append(attempt.Candidates, candidate)
		if len(candidate.RejectReasons) > 0 || selection.Error != "" {
			continue
		}
		current := recoverySelection{Selection: selection, Candidate: candidate}
		if best == nil || current.Candidate.Score > best.Candidate.Score {
			best = &current
		}
	}
	if best == nil {
		attempt.FailureReason = "no safe recovery candidate found"
		return attempt, RecoveryDecision{}, SelectedProduct{}, false
	}
	attempt.SelectedID = best.Candidate.ID
	decision := RecoveryDecision{
		ID:                 "decision-" + issue.ID,
		IssueID:            issue.ID,
		DecisionType:       "replace_product",
		OriginalIngredient: issue.IngredientName,
		FinalIngredient:    issue.IngredientName,
		FinalProductID:     strutil.FirstNonEmpty(best.Selection.Product.ID, best.Selection.Product.SKU, best.Selection.Product.Name),
		RecipeChanges:      append([]RecipeChange(nil), best.Candidate.RequiredChanges...),
		Reason:             strings.Join(best.Candidate.AcceptReasons, "; "),
		Confidence:         best.Candidate.Confidence,
	}
	decision.OriginalProductID = originalProductID(artifact.Shop, issue)
	return attempt, decision, best.Selection, true
}

type recoverySelection struct {
	Selection SelectedProduct
	Candidate RecoveryCandidate
}

func applyRecoveryDecision(artifact FoodRunArtifact, decision RecoveryDecision) FoodRunArtifact {
	for _, change := range decision.RecipeChanges {
		if change.ChangeType == "prep_note" {
			artifact.MealPlan = addPrepNoteForIngredient(artifact.MealPlan, decision.OriginalIngredient, change.NewText)
			artifact.RecipeAdjustments = append(artifact.RecipeAdjustments, recipeAdjustmentsForPrepNote(artifact.MealPlan, decision, change)...)
		}
	}
	return artifact
}

func replaceRecoveredSelection(shop ShopResult, issue RecoveryIssue, selection SelectedProduct, decision RecoveryDecision) ShopResult {
	selection.Ingredient = Ingredient{
		Name:     issue.IngredientName,
		Quantity: quantityValue(issue.RequiredQuantity),
		Unit:     quantityUnit(issue.RequiredQuantity),
	}
	selection.SelectionReason = strings.TrimSpace("recovery: " + decision.Reason)
	replaced := false
	for i := range shop.SelectedProducts {
		if normalizeKey(shop.SelectedProducts[i].Ingredient.Name) == normalizeKey(issue.IngredientName) {
			shop.SelectedProducts[i] = selection
			replaced = true
			break
		}
	}
	if !replaced {
		shop.SelectedProducts = append(shop.SelectedProducts, selection)
	}
	return shop
}

func recoveryCandidateFromSelection(issue RecoveryIssue, query string, selection SelectedProduct, rule recoveryAliasRule) RecoveryCandidate {
	candidate := RecoveryCandidate{
		ID:            candidateID(issue.ID, query, 1),
		CandidateType: "product",
		Compatibility: "rejected",
		Confidence:    "rejected",
	}
	if selection.Error != "" {
		candidate.RejectReasons = append(candidate.RejectReasons, selection.Error)
		return candidate
	}
	ev := productEvidenceFromSelection(selection)
	candidate.Product = &ev
	compatibility, reasons, rejects, changes := recoveryCompatibility(issue, query, selection.Product, rule)
	candidate.Compatibility = compatibility
	candidate.RequiredChanges = changes
	if len(rejects) > 0 {
		candidate.RejectReasons = rejects
		return candidate
	}
	score := recoveryScore(selection, ev, compatibility)
	candidate.Score = score
	candidate.AcceptReasons = reasons
	candidate.Confidence = recoveryConfidence(score, compatibility, ev)
	return candidate
}

func recoveryCompatibility(issue RecoveryIssue, query string, product ProductSummary, rule recoveryAliasRule) (string, []string, []string, []RecipeChange) {
	text := normalizeKey(strings.Join([]string{query, product.Name, product.Brand, product.Category, product.Allergens}, " "))
	for _, reject := range rule.RejectTerms {
		if strings.Contains(text, normalizeKey(reject)) {
			return "rejected", nil, []string{"semantic mismatch for " + issue.IngredientName + ": " + reject}, nil
		}
	}
	if product.Available != nil && !*product.Available {
		return "rejected", nil, []string{"candidate is unavailable in the selected market"}, nil
	}
	if rule.FoodClass == "beef" && !containsAnyKey(text, []string{"ternera", "vacuno", "buey", "beef"}) {
		return "rejected", nil, []string{"candidate does not look like beef/ternera"}, nil
	}
	for _, alias := range rule.PrepAliases {
		if strings.Contains(normalizeKey(query+" "+product.Name), normalizeKey(alias)) || containsAnyKey(text, []string{"filete", "filetes"}) {
			change := RecipeChange{
				ChangeType: "prep_note",
				NewText:    "Use thin beef fillets and cut them into strips before cooking.",
				Reason:     "Recovered beef strips with a same-ingredient prep transform.",
				Confidence: "medium",
			}
			return "prep_transform", []string{"same ingredient recovered with a prep transform"}, nil, []RecipeChange{change}
		}
	}
	if containsAnyKey(text, []string{"tiras", "fajitas", "saltear"}) {
		return "exact", []string{"same ingredient and compatible strip/stir-fry form"}, nil, nil
	}
	return "rejected", nil, []string{"candidate is not a safe same-ingredient form"}, nil
}

func recoveryScore(selection SelectedProduct, ev ProductEvidence, compatibility string) int {
	score := 0
	switch compatibility {
	case "exact":
		score += 80
	case "prep_transform":
		score += 65
	}
	if selection.Product.PackageQuantity > 0 || ev.Package.NetQuantity != nil {
		score += 15
	}
	if selection.Product.ImageURL != "" {
		score += 5
	}
	if selection.ProductNutrition != nil {
		score += 3
	}
	if ev.Package.VariableWeight || ev.Package.ApproximateWeight {
		score -= 10
	}
	if selection.LineTotal.Cents > 0 {
		score += int(math.Max(0, 10-math.Min(float64(selection.LineTotal.Cents)/100, 10)))
	}
	return score
}

func recoveryConfidence(score int, compatibility string, ev ProductEvidence) string {
	if compatibility == "exact" && score >= 85 && ev.Package.Confidence >= 0.75 {
		return "exact"
	}
	if score >= 65 {
		return "estimated"
	}
	return "low_confidence"
}

func recoveryRuleForIngredient(name string) (recoveryAliasRule, bool) {
	key := normalizeKey(name)
	for _, rule := range recoveryAliasRules {
		if key == normalizeKey(rule.CanonicalName) || containsAnyKey(key, append(rule.Aliases, rule.PrepAliases...)) {
			return rule, true
		}
	}
	return recoveryAliasRule{}, false
}

func recoveryQueries(rule recoveryAliasRule, max int) []string {
	queries := append([]string(nil), rule.Aliases...)
	queries = append(queries, rule.PrepAliases...)
	if max > 0 && len(queries) > max {
		return queries[:max]
	}
	return queries
}

func recoveryIngredient(issue RecoveryIssue) Ingredient {
	ing := Ingredient{Name: issue.IngredientName}
	if issue.RequiredQuantity != nil {
		ing.Quantity = issue.RequiredQuantity.Value
		ing.Unit = issue.RequiredQuantity.Unit
	}
	return ing
}

func addPrepNoteForIngredient(plan MealPlan, ingredientName, note string) MealPlan {
	if strings.TrimSpace(note) == "" {
		return plan
	}
	for d := range plan.Days {
		for m := range plan.Days[d].Meals {
			meal := &plan.Days[d].Meals[m]
			if !recipeUsesIngredient(meal.Recipe, ingredientName) || recipeHasStepText(meal.Recipe, note) {
				continue
			}
			for i := range meal.Recipe.Steps {
				meal.Recipe.Steps[i].Number++
			}
			meal.Recipe.Steps = append([]RecipeStep{{Number: 1, Title: "Prep recovered ingredient", Text: note}}, meal.Recipe.Steps...)
		}
	}
	return plan
}

func recipeAdjustmentsForPrepNote(plan MealPlan, decision RecoveryDecision, change RecipeChange) []RecipeAdjustment {
	var adjustments []RecipeAdjustment
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			if !recipeUsesIngredient(meal.Recipe, decision.OriginalIngredient) {
				continue
			}
			stepNumbers := []int{}
			for _, step := range meal.Recipe.Steps {
				if normalizeKey(step.Text) == normalizeKey(change.NewText) {
					stepNumbers = append(stepNumbers, step.Number)
				}
			}
			adjustments = append(adjustments, RecipeAdjustment{
				ID:                     "adjustment-" + decision.ID + "-" + meal.Recipe.ID,
				Type:                   "prep_transform",
				RecipeID:               meal.Recipe.ID,
				IngredientKey:          normalizeKey(decision.OriginalIngredient),
				OriginalIngredientName: decision.OriginalIngredient,
				SelectedProductID:      decision.FinalProductID,
				RecoveryDecisionID:     decision.ID,
				Instruction:            change.NewText,
				AppliedToSteps:         stepNumbers,
			})
		}
	}
	return adjustments
}

func recipeUsesIngredient(recipe Recipe, ingredientName string) bool {
	key := normalizeKey(ingredientName)
	for _, ingredient := range recipe.Ingredients {
		if normalizeKey(ingredient.Name) == key {
			return true
		}
	}
	return false
}

func recipeHasStepText(recipe Recipe, text string) bool {
	key := normalizeKey(text)
	for _, step := range recipe.Steps {
		if normalizeKey(step.Text) == key {
			return true
		}
	}
	return false
}

func recoveryIssueFromAllocation(index int, kind RecoveryIssueKind, req IngredientRequirement, allocation IngredientProductAllocation, reason string) RecoveryIssue {
	return RecoveryIssue{
		ID:               fmt.Sprintf("issue-%03d", index+1),
		Kind:             kind,
		IngredientID:     req.RequirementID,
		IngredientName:   req.IngredientName,
		RecipeID:         req.RecipeID,
		Day:              req.Day,
		MealSlotID:       req.MealSlot,
		RequiredQuantity: req.RequiredQuantity,
		BlockingReason:   reason,
	}
}

func firstAllocationWarning(allocation IngredientProductAllocation, fallback string) string {
	for _, warning := range allocation.Warnings {
		if warning.Message != "" {
			return warning.Message
		}
	}
	return fallback
}

func originalProductID(shop ShopResult, issue RecoveryIssue) string {
	for _, selected := range shop.SelectedProducts {
		if normalizeKey(selected.Ingredient.Name) == normalizeKey(issue.IngredientName) {
			return strutil.FirstNonEmpty(selected.Product.ID, selected.Product.SKU, selected.Product.Name)
		}
	}
	return ""
}

func finalizeRecoveryPlan(plan *RecoveryPlan, ledger *QuantityLedger, safety *BasketSafety) {
	if ledger != nil {
		plan.FinalLedgerStatus = string(ledger.Status)
	}
	if safety != nil {
		plan.FinalBasketSafety = string(safety.Status)
		plan.SafeToBuild = safety.SafeToBuild
	}
	switch {
	case len(plan.Issues) == 0:
		plan.Status = RecoveryStatusNotNeeded
	case len(plan.AppliedDecisions) == 0:
		plan.Status = RecoveryStatusFailed
	case len(plan.RemainingIssues) == 0:
		plan.Status = RecoveryStatusRecovered
	default:
		plan.Status = RecoveryStatusPartial
	}
}

func candidateID(issueID, query string, index int) string {
	return normalizeKey(fmt.Sprintf("%s-%s-%d", issueID, query, index))
}

func quantityValue(q *NormalizedQuantity) float64 {
	if q == nil {
		return 0
	}
	return q.Value
}

func quantityUnit(q *NormalizedQuantity) string {
	if q == nil {
		return ""
	}
	return q.Unit
}

func sortRecoveryCandidates(candidates []RecoveryCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
}
