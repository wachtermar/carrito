package food

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/money"
	"github.com/wachtermar/carrito/internal/strutil"
)

type BasketOptimizationOptions struct {
	Enabled         bool
	Policy          BasketOptimizationPolicy
	Profile         Profile
	SelectionPolicy string
	SearchLimit     int
	Meals           []string
	LedgerOptions   QuantityLedgerOptions
	RecoveryOptions RecoveryOptions
	ReadinessPolicy ReadinessPolicy
}

var (
	offerNxMRE        = regexp.MustCompile(`(?i)\b(\d+)\s*x\s*(\d+)\b`)
	offerLlevaPagaRE  = regexp.MustCompile(`(?i)\blleva(?:te)?\s+(\d+)\s+(?:y\s+)?paga\s+(\d+)\b`)
	offerSecondUnitRE = regexp.MustCompile(`(?i)\b(?:segunda|2a|2)\s+unidad\s+(?:al\s+)?-?(\d+)\s*%?\b`)
	offerPercentRE    = regexp.MustCompile(`(?i)-\s*(\d+)\s*%`)
)

func OptimizeBasket(ctx context.Context, artifact FoodRunArtifact, client *alcampo.Client, opts BasketOptimizationOptions) (FoodRunArtifact, BasketOptimizationPlan, error) {
	opts = normalizeBasketOptimizationOptions(opts)
	artifact = RefreshFoodRunArtifact(artifact, opts.Meals)
	if artifact.QuantityLedger == nil {
		ledger := BuildQuantityLedger(artifact.MealPlan, artifact.Shop, opts.LedgerOptions)
		artifact.QuantityLedger = &ledger
	}
	if artifact.BasketSafety == nil && artifact.QuantityLedger != nil {
		safety := BasketSafetyFromLedger(*artifact.QuantityLedger, opts.LedgerOptions)
		artifact.BasketSafety = &safety
	}
	baselineGate := EvaluateReadinessGate(&artifact, opts.ReadinessPolicy)
	if artifact.BasketSafety != nil {
		safetyCopy := *artifact.BasketSafety
		artifact.PreOptimizationBasketSafety = &safetyCopy
	}
	artifact.PreOptimizationReadinessGate = &baselineGate
	plan := BasketOptimizationPlan{
		SchemaVersion:                     "1",
		Status:                            BasketOptimizationNotRun,
		Policy:                            opts.Policy,
		MealPlanFingerprint:               artifact.MealPlanFingerprint,
		ServingPlanFingerprint:            artifact.ServingPlanFingerprint,
		ScaledMealPlanFingerprint:         artifact.ScaledMealPlanFingerprint,
		BeforeProductSelectionFingerprint: artifact.ProductSelectionFingerprint,
		PantryResolutionFingerprint:       artifact.PantryResolutionFingerprint,
		ShopRequirementsFingerprint:       artifact.ShopRequirementsFingerprint,
		BaselineSummary:                   basketOptimizationSummary(artifact, baselineGate),
		Validation: BasketOptimizationValidation{
			BaselineReadinessStatus: string(baselineGate.Status),
			BaselineSafeToBuild:     baselineGate.SafeToBuild,
			FinalReadinessStatus:    string(baselineGate.Status),
			FinalSafeToBuild:        baselineGate.SafeToBuild,
			FinalLedgerStatus:       finalLedgerStatus(artifact.QuantityLedger),
			FinalBasketSafety:       artifact.BasketSafety != nil && artifact.BasketSafety.SafeToBuild,
		},
	}
	if !opts.Enabled || !opts.Policy.Enabled || client == nil {
		plan.Status = BasketOptimizationSkipped
		plan.Warnings = append(plan.Warnings, OptimizationWarning{Code: "disabled", Message: "Basket optimization was disabled."})
		artifact.BasketOptimizationPlan = &plan
		return artifact, plan, nil
	}
	if blockedByNonOptimizableIssues(baselineGate) {
		plan.Status = BasketOptimizationSkipped
		plan.Warnings = append(plan.Warnings, OptimizationWarning{Code: "non_optimizable_readiness", Message: "Readiness is blocked by issues that product optimization should not repair."})
		plan.Validation.FinalReadinessStatus = string(baselineGate.Status)
		plan.Validation.FinalSafeToBuild = baselineGate.SafeToBuild
		artifact.BasketOptimizationPlan = &plan
		return artifact, plan, nil
	}

	selectedByIngredient := mapSelectedByIngredient(artifact.Shop.SelectedProducts)
	var decisions []BasketOptimizationDecision
	var pools []IngredientCandidatePool
	totalChecks := 0
	appliedSwitches := 0
	optimizedShop := cloneShopResult(artifact.Shop)
	for _, req := range artifact.QuantityLedger.Requirements {
		if totalChecks >= opts.Policy.MaxTotalCandidateChecks {
			break
		}
		ingredient := ingredientForRequirement(artifact.MealPlan, req)
		baseline, hasBaseline := selectedByIngredient[normalizeKey(req.IngredientName)]
		pool, decision, changed := optimizeRequirement(ctx, client, opts, req, ingredient, baseline, hasBaseline, &totalChecks)
		for _, candidate := range pool.Candidates {
			if !candidate.Valid {
				plan.RejectedCandidates = append(plan.RejectedCandidates, RejectedBasketCandidate{
					IngredientKey: normalizeKey(req.IngredientName),
					ProductID:     candidate.ProductID,
					ProductName:   candidate.ProductName,
					RejectCodes:   candidate.RejectCodes,
					RejectReason:  candidate.RejectReason,
				})
			}
		}
		if changed {
			if opts.Policy.MaxProductSwitches > 0 && appliedSwitches >= opts.Policy.MaxProductSwitches {
				decision.Changed = false
				decision.SelectedProductID = decision.BaselineProductID
				decision.SelectedProductName = decision.BaselineProductName
				decision.CostDeltaCents = 0
				decision.OfferSavingsCents = 0
				decision.ReadinessImpact = "preserved"
				decision.Reason = "kept original product because the optimization switch limit was reached"
				pool.SelectedCandidateID = firstCandidateBySource(pool.Candidates, "baseline").ID
				changed = false
			}
		}
		pools = append(pools, pool)
		decisions = append(decisions, decision)
		if changed {
			optimizedShop = replaceOptimizedSelection(optimizedShop, req.IngredientName, selectedFromOptimizationCandidate(ingredient, pool.Candidates, decision.SelectedProductID))
			appliedSwitches++
		}
	}
	plan.CandidatePools = pools
	plan.Decisions = decisions
	changedCount := 0
	for _, decision := range decisions {
		if decision.Changed {
			changedCount++
		}
	}
	if changedCount == 0 {
		plan.Status = BasketOptimizationNoImprovement
		plan.AfterProductSelectionFingerprint = artifact.ProductSelectionFingerprint
		plan.OptimizedSummary = &plan.BaselineSummary
		artifact.BasketOptimizationPlan = &plan
		return artifact, plan, nil
	}

	optimizedShop = rebuildShopDerivedFields(optimizedShop)
	next := artifact
	next.Shop = optimizedShop
	next = RefreshFoodRunArtifact(next, opts.Meals)
	ledger := BuildQuantityLedger(next.MealPlan, next.Shop, opts.LedgerOptions)
	safety := BasketSafetyFromLedger(ledger, opts.LedgerOptions)
	next.QuantityLedger = &ledger
	next.BasketSafety = &safety
	if opts.RecoveryOptions.Enabled && !safety.SafeToBuild {
		recoveryOpts := opts.RecoveryOptions
		recoveryOpts.AllowRecipeSwap = false
		recoveryOpts.LedgerOptions = opts.LedgerOptions
		next = RecoverFoodRun(ctx, next, client, recoveryOpts)
		next = RefreshFoodRunArtifact(next, opts.Meals)
	}
	finalGate := ApplyReadinessGate(&next, opts.ReadinessPolicy)
	optimizedSummary := basketOptimizationSummary(next, finalGate)
	plan.OptimizedSummary = &optimizedSummary
	plan.AfterProductSelectionFingerprint = next.ProductSelectionFingerprint
	plan.Validation.FinalReadinessStatus = string(finalGate.Status)
	plan.Validation.FinalSafeToBuild = finalGate.SafeToBuild
	plan.Validation.FinalLedgerStatus = finalLedgerStatus(next.QuantityLedger)
	plan.Validation.FinalBasketSafety = next.BasketSafety != nil && next.BasketSafety.SafeToBuild
	plan.Validation.AppliedSelectionFingerprint = next.ProductSelectionFingerprint
	if opts.Policy.RequireNoReadinessRegression && readinessRegressed(baselineGate, finalGate) {
		plan.Status = BasketOptimizationFailed
		plan.Validation.ReadinessRegression = true
		plan.Warnings = append(plan.Warnings, OptimizationWarning{Code: "readiness_regression", Message: "Optimization was discarded because it would weaken final readiness."})
		artifact.BasketOptimizationPlan = &plan
		return artifact, plan, nil
	}
	plan.Status = BasketOptimizationApplied
	next.BasketOptimizationPlan = &plan
	return next, plan, nil
}

func normalizeBasketOptimizationOptions(opts BasketOptimizationOptions) BasketOptimizationOptions {
	if opts.Policy.Objective == "" {
		opts.Policy.Objective = BasketObjectiveSafeBalanced
	}
	if opts.Policy.MaxCandidatesPerIngredient <= 0 {
		opts.Policy.MaxCandidatesPerIngredient = 8
	}
	if opts.Policy.MaxSearchesPerIngredient <= 0 {
		opts.Policy.MaxSearchesPerIngredient = 2
	}
	if opts.Policy.MaxTotalCandidateChecks <= 0 {
		opts.Policy.MaxTotalCandidateChecks = 80
	}
	if opts.Policy.MinSavingsCentsToSwitch <= 0 {
		opts.Policy.MinSavingsCentsToSwitch = 50
	}
	if opts.Policy.MinWasteReductionRatioToSwitch <= 0 {
		opts.Policy.MinWasteReductionRatioToSwitch = 0.2
	}
	opts.Policy.RequireNoReadinessRegression = true
	opts.Policy.PreferProductImages = true
	if opts.SearchLimit <= 0 {
		opts.SearchLimit = opts.Policy.MaxCandidatesPerIngredient
	}
	if opts.SelectionPolicy == "" {
		opts.SelectionPolicy = strutil.FirstNonEmpty(opts.Profile.SelectionPolicy, PolicyBalanced)
	}
	return opts
}

func optimizeRequirement(ctx context.Context, client *alcampo.Client, opts BasketOptimizationOptions, req IngredientRequirement, ingredient Ingredient, baseline SelectedProduct, hasBaseline bool, totalChecks *int) (IngredientCandidatePool, BasketOptimizationDecision, bool) {
	pool := IngredientCandidatePool{
		IngredientKey:    normalizeKey(req.IngredientName),
		IngredientName:   req.IngredientName,
		RequiredQuantity: req.RequiredQuantity,
		UnitDimension:    unitDimension(quantityUnit(req.RequiredQuantity)),
	}
	if hasBaseline {
		pool.BaselineProductID = firstNonEmptyString(baseline.Product.ID, baseline.Product.SKU)
		pool.BaselineProductName = baseline.Product.Name
	}
	queries := optimizationQueries(ingredient, req, opts.Policy.MaxSearchesPerIngredient)
	pool.SearchQueries = queries
	candidatesByKey := map[string]ProductOptimizationCandidate{}
	if hasBaseline {
		c := optimizationCandidateFromSelection("baseline-"+pool.IngredientKey, "baseline", "", req, ingredient, baseline, opts)
		candidatesByKey[optimizationCandidateKey(c)] = c
	}
	for _, query := range queries {
		if *totalChecks >= opts.Policy.MaxTotalCandidateChecks {
			break
		}
		products, err := client.Search(ctx, query, alcampo.SearchOptions{Limit: opts.SearchLimit, RegionID: opts.LedgerOptions.StoreID})
		if err != nil {
			continue
		}
		for i, product := range products {
			if *totalChecks >= opts.Policy.MaxTotalCandidateChecks || len(candidatesByKey) >= opts.Policy.MaxCandidatesPerIngredient {
				break
			}
			*totalChecks++
			selected := selectionFromProduct(ingredient, product)
			if opts.LedgerOptions.EnrichProducts {
				enriched := EnrichShopResultProducts(ctx, ShopResult{SelectedProducts: []SelectedProduct{selected}}, client, opts.LedgerOptions)
				if len(enriched.SelectedProducts) > 0 {
					selected = enriched.SelectedProducts[0]
				}
			}
			id := fmt.Sprintf("%s-candidate-%03d", pool.IngredientKey, i+1)
			c := optimizationCandidateFromSelection(id, "search", query, req, ingredient, selected, opts)
			key := optimizationCandidateKey(c)
			if key != "" {
				candidatesByKey[key] = c
			}
		}
	}
	for _, candidate := range candidatesByKey {
		pool.Candidates = append(pool.Candidates, candidate)
	}
	sort.SliceStable(pool.Candidates, func(i, j int) bool {
		return pool.Candidates[i].Score.FinalScore > pool.Candidates[j].Score.FinalScore
	})
	baselineCandidate := firstCandidateBySource(pool.Candidates, "baseline")
	best := bestValidCandidate(pool.Candidates)
	decision := BasketOptimizationDecision{
		IngredientKey:       pool.IngredientKey,
		IngredientName:      pool.IngredientName,
		BaselineProductID:   pool.BaselineProductID,
		BaselineProductName: pool.BaselineProductName,
		SelectedProductID:   pool.BaselineProductID,
		SelectedProductName: pool.BaselineProductName,
		Reason:              "kept original product; no safer or meaningfully better candidate was found",
		BaselineAllocation:  baselineCandidate.Allocation,
		SelectedAllocation:  baselineCandidate.Allocation,
		ReadinessImpact:     "preserved",
	}
	if best.ID == "" || best.Source == "baseline" || !worthSwitching(baselineCandidate, best, opts.Policy) {
		pool.SelectedCandidateID = baselineCandidate.ID
		return pool, decision, false
	}
	pool.SelectedCandidateID = best.ID
	pool.Reason = best.Score.Explanation
	decision.SelectedProductID = firstNonEmptyString(best.ProductID, best.SKU)
	decision.SelectedProductName = best.ProductName
	decision.Changed = true
	decision.Reason = best.Score.Explanation
	decision.SelectedAllocation = best.Allocation
	decision.CostDeltaCents = best.Allocation.EffectiveCostCents - baselineCandidate.Allocation.EffectiveCostCents
	decision.OfferSavingsCents = best.Allocation.OfferSavingsCents
	decision.ReadinessImpact = "preserved_or_improved"
	return pool, decision, true
}

func selectionFromProduct(ingredient Ingredient, product alcampo.Product) SelectedProduct {
	summary := ProductSummaryFromAlcampo(product)
	selected := SelectedProduct{Ingredient: ingredient, Product: summary}
	count, lineTotal, reason, warnings := EstimatePurchaseQuantity(ingredient, summary)
	selected.PurchaseQuantity = fmt.Sprintf("%d", count)
	selected.PackageCount = count
	selected.LineTotal = lineTotal
	selected.QuantityReason = reason
	selected.Warnings = warnings
	selected.SelectionReason = "optimization candidate"
	AttachSelectedProductNutrition(&selected)
	return selected
}

func optimizationCandidateFromSelection(id, source, query string, req IngredientRequirement, ingredient Ingredient, selected SelectedProduct, opts BasketOptimizationOptions) ProductOptimizationCandidate {
	product := selected.Product
	c := ProductOptimizationCandidate{
		ID:                id,
		Product:           product,
		ProductID:         product.ID,
		SKU:               product.SKU,
		ProductName:       product.Name,
		Brand:             product.Brand,
		URL:               product.URL,
		Source:            source,
		SearchQuery:       query,
		Available:         product.Available == nil || *product.Available,
		HasImage:          product.ImageURL != "",
		HasNutritionLabel: product.NutritionParsed != nil || selected.ProductNutrition != nil || strings.TrimSpace(product.Nutrition) != "",
	}
	c.AvailabilityStatus = "unknown"
	if product.Available != nil {
		if *product.Available {
			c.AvailabilityStatus = "available"
		} else {
			c.AvailabilityStatus = "unavailable"
		}
	}
	c.SemanticMatch = semanticMatchEvidence(ingredient, product, opts.Profile)
	c.QuantityParse, c.Allocation = quantityAllocationEvidence(req.RequiredQuantity, product, opts.Policy)
	c.Price = optimizationPriceEvidence(product)
	rawOffer := firstNonEmptyString(product.Offers...)
	c.Offer = parseOptimizationOffer(rawOffer)
	c.Offer = applyOptimizationOffer(c.Offer, c.Allocation.PackageCount, c.Price.CurrentPriceCents, opts.Policy.DealAware)
	c.Allocation.OfferSavingsCents = c.Offer.SavingsCents
	c.Allocation.EffectiveCostCents = c.Allocation.GrossCostCents - c.Offer.SavingsCents
	c.Valid, c.RejectCodes, c.RejectReason = validateOptimizationCandidate(c, opts.Policy)
	c.Score = scoreOptimizationCandidate(c, opts.Policy)
	return c
}

func semanticMatchEvidence(ingredient Ingredient, product ProductSummary, profile Profile) SemanticMatchEvidence {
	text := normalizeKey(strings.Join([]string{product.Name, product.Brand, product.Category, product.Allergens}, " "))
	if reason := nonHumanFoodRejectedReason(text); reason != "" {
		return SemanticMatchEvidence{Status: "mismatch", Confidence: 0, RejectReason: reason}
	}
	if productDietRejectedReason(text, profile.Diets) != "" || containsAny(text, profile.Allergies) || containsAny(text, profile.Dislikes) {
		return SemanticMatchEvidence{Status: "mismatch", Confidence: 0, RejectReason: "conflicts with saved diet, allergy, or dislike rules"}
	}
	p := alcampo.Product{Name: product.Name, Brand: product.Brand, Category: product.Category, Allergens: product.Allergens}
	score := matchScore(ingredient, p)
	switch {
	case score >= 20:
		return SemanticMatchEvidence{Status: "exact", Confidence: 0.95}
	case score > 0:
		return SemanticMatchEvidence{Status: "low_confidence", Confidence: 0.55}
	default:
		return SemanticMatchEvidence{Status: "mismatch", Confidence: 0, RejectReason: "product does not match required ingredient"}
	}
}

func quantityAllocationEvidence(required *NormalizedQuantity, product ProductSummary, policy BasketOptimizationPolicy) (ProductQuantityParseEvidence, ProductAllocationOption) {
	pkg := ParsePackageEvidence(product.Size, product.Name)
	if product.PackageQuantity > 0 && product.PackageUnit != "" {
		q := normalizedQuantity(fmt.Sprintf("%.3g %s", product.PackageQuantity, product.PackageUnit), product.PackageQuantity, product.PackageUnit, 0.9, "product_summary_package")
		pkg.NetQuantity = quantityRange(q, true, "")
		pkg.SalesUnit = packageSalesUnit(q.BaseUnit)
		pkg.BaseUnit = q.BaseUnit
		pkg.Confidence = math.Max(pkg.Confidence, q.Confidence)
		pkg.ParseMethod = q.ParseMethod
	}
	out := ProductQuantityParseEvidence{
		Status:     "unparseable",
		RawText:    strings.Join(pkg.RawTexts, " "),
		Confidence: pkg.Confidence,
	}
	allocation := ProductAllocationOption{
		RequiredQuantity: required,
		QuantityStatus:   "blocked",
		Reason:           "package quantity is not parseable",
	}
	if pkg.NetQuantity == nil || required == nil {
		return out, allocation
	}
	pkgQty := pkg.NetQuantity.Expected
	out.PackageQuantity = &pkgQty
	out.UsedNetWeight = true
	if pkg.VariableWeight || pkg.ApproximateWeight || !pkg.NetQuantity.IsExact {
		out.Status = "estimated_variable_weight"
	} else {
		out.Status = "exact"
	}
	if required.BaseUnit != pkgQty.BaseUnit || required.BaseValue <= 0 || pkgQty.BaseValue <= 0 {
		out.Status = "low_confidence"
		allocation.Reason = "required unit and package unit are incompatible"
		allocation.PackageQuantity = &pkgQty
		return out, allocation
	}
	count := int(math.Ceil(required.BaseValue / pkgQty.BaseValue))
	if count <= 0 {
		count = 1
	}
	purchased := normalizedQuantity(fmt.Sprintf("%.3g %s", pkgQty.BaseValue*float64(count), pkgQty.BaseUnit), pkgQty.BaseValue*float64(count), pkgQty.BaseUnit, pkgQty.Confidence, "optimization_allocation")
	excess := normalizedQuantity(fmt.Sprintf("%.3g %s", purchased.BaseValue-required.BaseValue, required.BaseUnit), purchased.BaseValue-required.BaseValue, required.BaseUnit, required.Confidence, "optimization_excess")
	allocation.PackageQuantity = &pkgQty
	allocation.PackageCount = count
	allocation.PurchasedQuantity = &purchased
	if excess.BaseValue > 0 {
		allocation.ExcessQuantity = &excess
		allocation.ExcessRatio = excess.BaseValue / required.BaseValue
	}
	allocation.GrossCostCents = int(product.Price.Cents) * count
	allocation.EffectiveCostCents = allocation.GrossCostCents
	allocation.QuantityStatus = "exact"
	allocation.Reason = "candidate package quantity exactly covers the required unit dimension"
	if out.Status == "estimated_variable_weight" {
		if policy.AllowEstimatedVariableWeight {
			allocation.QuantityStatus = "estimated_allowed"
			allocation.Reason = "estimated variable-weight quantity is allowed by policy"
		} else {
			allocation.QuantityStatus = "blocked"
			allocation.Reason = "estimated variable-weight quantity is blocked by policy"
		}
	}
	return out, allocation
}

func optimizationPriceEvidence(product ProductSummary) OptimizationPriceEvidence {
	confidence := "missing"
	if product.Price.Cents > 0 {
		confidence = "display_price"
	}
	return OptimizationPriceEvidence{
		CurrentPriceCents: int(product.Price.Cents),
		UnitPriceCents:    int(product.UnitPrice.Cents),
		UnitPriceUnit:     product.Unit,
		Currency:          strutil.FirstNonEmpty(product.Price.Currency, "EUR"),
		PriceConfidence:   confidence,
		RawPriceText:      product.Price.Amount,
	}
}

func validateOptimizationCandidate(c ProductOptimizationCandidate, policy BasketOptimizationPolicy) (bool, []string, string) {
	var codes []string
	if !c.Available {
		codes = append(codes, "unavailable")
	}
	if c.SemanticMatch.Status == "mismatch" {
		codes = append(codes, "semantic_mismatch")
	}
	if c.Allocation.QuantityStatus == "blocked" {
		codes = append(codes, "quantity_blocked")
	}
	if c.QuantityParse.Status == "unparseable" && policy.StrictQuantity {
		codes = append(codes, "quantity_unparseable")
	}
	if c.QuantityParse.Status == "estimated_variable_weight" && !policy.AllowEstimatedVariableWeight {
		codes = append(codes, "estimated_variable_weight_blocked")
	}
	if c.Price.CurrentPriceCents <= 0 && policy.Objective == BasketObjectiveLowestPrice {
		codes = append(codes, "price_missing")
	}
	if len(codes) > 0 {
		return false, dedupeStrings(codes), strings.Join(dedupeStrings(codes), ", ")
	}
	return true, nil, ""
}

func scoreOptimizationCandidate(c ProductOptimizationCandidate, policy BasketOptimizationPolicy) BasketCandidateScore {
	score := BasketCandidateScore{}
	if c.Valid {
		score.SafetyTier = 100
	}
	switch c.SemanticMatch.Status {
	case "exact":
		score.SemanticScore = 100
	case "alias":
		score.SemanticScore = 85
	case "low_confidence":
		score.SemanticScore = 40
	}
	switch c.Allocation.QuantityStatus {
	case "exact":
		score.QuantityScore = 100
	case "estimated_allowed":
		score.QuantityScore = 65
	case "low_confidence_allowed":
		score.QuantityScore = 45
	}
	score.WasteScore = 100 - math.Min(c.Allocation.ExcessRatio*100, 100)
	score.PriceScore = -float64(c.Allocation.EffectiveCostCents) / 100
	score.OfferScore = float64(c.Offer.SavingsCents) / 100
	if c.HasImage {
		score.EvidenceScore += 5
	}
	if c.HasNutritionLabel && policy.PreferNutritionLabels {
		score.EvidenceScore += 3
	}
	switch policy.Objective {
	case BasketObjectiveLowestPrice:
		score.FinalScore = float64(score.SafetyTier)*10000 + score.SemanticScore*100 + score.QuantityScore*50 + score.PriceScore*20 + score.WasteScore + score.OfferScore
	case BasketObjectiveLowestWaste:
		score.FinalScore = float64(score.SafetyTier)*10000 + score.SemanticScore*100 + score.QuantityScore*50 + score.WasteScore*20 + score.PriceScore + score.OfferScore
	default:
		score.FinalScore = float64(score.SafetyTier)*10000 + score.SemanticScore*100 + score.QuantityScore*80 + score.WasteScore*20 + score.PriceScore*5 + score.OfferScore*3 + score.EvidenceScore
	}
	score.FinalScore = roundScore(score.FinalScore)
	score.Explanation = optimizationScoreExplanation(c)
	return score
}

func optimizationScoreExplanation(c ProductOptimizationCandidate) string {
	switch {
	case c.Source != "baseline" && c.Allocation.OfferSavingsCents > 0:
		return fmt.Sprintf("selected %s because it preserves quantity safety and applies a reliable offer", c.ProductName)
	case c.Source != "baseline" && c.Allocation.ExcessRatio < 0.2:
		return fmt.Sprintf("selected %s because it preserves readiness with lower package excess", c.ProductName)
	case c.Source != "baseline":
		return fmt.Sprintf("selected %s because it is the best validated optimization candidate", c.ProductName)
	default:
		return "kept original product"
	}
}

func worthSwitching(baseline, candidate ProductOptimizationCandidate, policy BasketOptimizationPolicy) bool {
	if !candidate.Valid {
		return false
	}
	if !baseline.Valid {
		return true
	}
	if baseline.Allocation.QuantityStatus != "exact" && candidate.Allocation.QuantityStatus == "exact" {
		return true
	}
	savings := baseline.Allocation.EffectiveCostCents - candidate.Allocation.EffectiveCostCents
	if savings >= policy.MinSavingsCentsToSwitch {
		return true
	}
	if baseline.Allocation.ExcessRatio > 0 && baseline.Allocation.ExcessRatio-candidate.Allocation.ExcessRatio >= policy.MinWasteReductionRatioToSwitch {
		return true
	}
	return false
}

func parseOptimizationOffer(raw string) OptimizationOfferEvidence {
	raw = strings.TrimSpace(raw)
	ev := OptimizationOfferEvidence{RawText: raw}
	if raw == "" {
		return ev
	}
	normalized := normalizeKey(strings.NewReplacer("ª", "a", "º", "o").Replace(raw))
	if m := offerNxMRE.FindStringSubmatch(raw); len(m) == 3 {
		n, _ := strconv.Atoi(m[1])
		paid, _ := strconv.Atoi(m[2])
		if n > paid && paid > 0 {
			ev.Parsed = true
			ev.Type = "n_for_m"
			ev.Confidence = 0.95
			ev.MinPackageCount = n
			ev.PaidPackageCount = paid
			return ev
		}
	}
	if m := offerLlevaPagaRE.FindStringSubmatch(normalized); len(m) == 3 {
		n, _ := strconv.Atoi(m[1])
		paid, _ := strconv.Atoi(m[2])
		if n > paid && paid > 0 {
			ev.Parsed = true
			ev.Type = "n_for_m"
			ev.Confidence = 0.9
			ev.MinPackageCount = n
			ev.PaidPackageCount = paid
			return ev
		}
	}
	if m := offerSecondUnitRE.FindStringSubmatch(normalized); len(m) == 2 {
		percent, _ := strconv.Atoi(m[1])
		if percent > 0 && percent < 100 {
			ev.Parsed = true
			ev.Type = "second_unit_percent"
			ev.Confidence = 0.9
			ev.MinPackageCount = 2
			ev.DiscountPercent = percent
			return ev
		}
	}
	if m := offerPercentRE.FindStringSubmatch(raw); len(m) == 2 {
		percent, _ := strconv.Atoi(m[1])
		if percent > 0 && percent < 100 {
			ev.Parsed = true
			ev.Type = "percent_off"
			ev.Confidence = 0.55
			ev.DiscountPercent = percent
			ev.NotAppliedReason = "Percent-off offer is recorded but not applied because current display price may already include the discount."
			return ev
		}
	}
	ev.Type = "unknown"
	ev.NotAppliedReason = "Offer terms are not parsed with high confidence."
	return ev
}

func applyOptimizationOffer(ev OptimizationOfferEvidence, packageCount, unitCents int, dealAware bool) OptimizationOfferEvidence {
	if !dealAware || !ev.Parsed || packageCount <= 0 || unitCents <= 0 {
		if ev.Parsed && !dealAware {
			ev.NotAppliedReason = "Deal-aware scoring is disabled."
		}
		return ev
	}
	switch ev.Type {
	case "n_for_m":
		if packageCount < ev.MinPackageCount {
			ev.NotAppliedReason = "Required package count does not qualify for this multi-buy offer."
			return ev
		}
		groups := packageCount / ev.MinPackageCount
		freeUnits := groups * (ev.MinPackageCount - ev.PaidPackageCount)
		ev.Applied = freeUnits > 0
		ev.AppliedPackageCount = packageCount
		ev.SavingsCents = freeUnits * unitCents
	case "second_unit_percent":
		pairs := packageCount / 2
		if pairs == 0 {
			ev.NotAppliedReason = "Required package count does not qualify for second-unit discount."
			return ev
		}
		ev.Applied = true
		ev.AppliedPackageCount = packageCount
		ev.SavingsCents = int(math.Round(float64(pairs*unitCents*ev.DiscountPercent) / 100.0))
	}
	return ev
}

func basketOptimizationSummary(artifact FoodRunArtifact, gate ReadinessGate) BasketOptimizationSummary {
	summary := BasketOptimizationSummary{
		ProductLineCount:       len(artifact.Shop.SelectedProducts),
		GrossSubtotalCents:     int(artifact.Shop.EstimatedTotal.Cents),
		EffectiveSubtotalCents: int(artifact.Shop.EstimatedTotal.Cents),
		ReadinessStatus:        string(gate.Status),
		SafeToBuild:            gate.SafeToBuild,
	}
	var grossCents int64
	var effectiveCents int64
	for _, selected := range artifact.Shop.SelectedProducts {
		if selected.Error != "" {
			continue
		}
		count := selected.PackageCount
		if count <= 0 {
			if parsed, err := strconv.Atoi(strings.TrimSpace(selected.PurchaseQuantity)); err == nil && parsed > 0 {
				count = parsed
			}
		}
		if count > 0 && selected.Product.Price.Cents > 0 {
			grossCents += selected.Product.Price.Cents * int64(count)
		} else {
			grossCents += selected.LineTotal.Cents
		}
		effectiveCents += selected.LineTotal.Cents
	}
	if effectiveCents > 0 {
		summary.GrossSubtotalCents = int(grossCents)
		summary.EffectiveSubtotalCents = int(effectiveCents)
		if grossCents > effectiveCents {
			summary.OfferSavingsCents = int(grossCents - effectiveCents)
		}
	}
	if artifact.QuantityLedger != nil {
		summary.ExactQuantityLines = artifact.QuantityLedger.Summary.ExactQuantityLines
		summary.EstimatedQuantityLines = artifact.QuantityLedger.Summary.EstimatedVariableWeightLines
		summary.LowConfidenceLines = artifact.QuantityLedger.Summary.NeedsReviewLines
		for _, allocation := range artifact.QuantityLedger.Allocations {
			if allocation.ExcessQuantity != nil && allocation.ExcessQuantity.BaseValue > 0 {
				summary.IngredientsWithExcess++
			}
		}
	}
	for _, selected := range artifact.Shop.SelectedProducts {
		if selected.Product.ImageURL != "" {
			summary.ProductImageCoverage++
		}
		if selected.ProductNutrition != nil || strings.TrimSpace(selected.Product.Nutrition) != "" {
			summary.NutritionLabelCoverage++
		}
	}
	return summary
}

func optimizationQueries(ingredient Ingredient, req IngredientRequirement, max int) []string {
	var out []string
	for _, value := range []string{ingredient.SearchTerm, req.IngredientName, ingredient.Name} {
		value = strings.TrimSpace(value)
		if value != "" {
			out = appendUniqueString(out, value)
		}
	}
	if len(out) > max && max > 0 {
		out = out[:max]
	}
	return out
}

func ingredientForRequirement(plan MealPlan, req IngredientRequirement) Ingredient {
	for _, ingredient := range plan.RequiredPurchases {
		if normalizeKey(ingredient.Name) == normalizeKey(req.IngredientName) {
			return ingredient
		}
	}
	return Ingredient{Name: req.IngredientName, Quantity: quantityValue(req.RequiredQuantity), Unit: quantityUnit(req.RequiredQuantity)}
}

func replaceOptimizedSelection(shop ShopResult, ingredientName string, selected SelectedProduct) ShopResult {
	for i := range shop.SelectedProducts {
		if normalizeKey(shop.SelectedProducts[i].Ingredient.Name) == normalizeKey(ingredientName) {
			shop.SelectedProducts[i] = selected
			return shop
		}
	}
	shop.SelectedProducts = append(shop.SelectedProducts, selected)
	return shop
}

func selectedFromOptimizationCandidate(ingredient Ingredient, candidates []ProductOptimizationCandidate, productID string) SelectedProduct {
	for _, candidate := range candidates {
		if firstNonEmptyString(candidate.ProductID, candidate.SKU) != productID {
			continue
		}
		product := candidate.Product
		if product.Name == "" {
			product = ProductSummary{ID: candidate.ProductID, SKU: candidate.SKU, Name: candidate.ProductName, Brand: candidate.Brand, URL: candidate.URL}
		}
		if product.Price.Cents <= 0 && candidate.Price.CurrentPriceCents > 0 {
			product.Price = money.Money{Cents: int64(candidate.Price.CurrentPriceCents), Amount: money.FormatAmount(int64(candidate.Price.CurrentPriceCents)), Currency: strutil.FirstNonEmpty(candidate.Price.Currency, "EUR")}
		}
		if candidate.QuantityParse.PackageQuantity != nil {
			product.PackageQuantity = candidate.QuantityParse.PackageQuantity.Value
			product.PackageUnit = candidate.QuantityParse.PackageQuantity.Unit
			product.Size = candidate.QuantityParse.PackageQuantity.Raw
		}
		selected := SelectedProduct{Ingredient: ingredient, Product: product, PackageCount: candidate.Allocation.PackageCount, PurchaseQuantity: fmt.Sprintf("%d", candidate.Allocation.PackageCount), QuantityReason: candidate.Score.Explanation, SelectionReason: "basket optimization: " + candidate.Score.Explanation}
		selected.LineTotal = money.Money{Cents: int64(candidate.Allocation.EffectiveCostCents), Amount: money.FormatAmount(int64(candidate.Allocation.EffectiveCostCents)), Currency: strutil.FirstNonEmpty(candidate.Price.Currency, "EUR")}
		return selected
	}
	return SelectedProduct{Ingredient: ingredient, Error: "optimization selected product not found"}
}

func cloneShopResult(shop ShopResult) ShopResult {
	out := shop
	out.SelectedProducts = append([]SelectedProduct(nil), shop.SelectedProducts...)
	out.BasketLines = append([]string(nil), shop.BasketLines...)
	out.NutritionWarnings = append([]string(nil), shop.NutritionWarnings...)
	out.ProductNutritionReports = append([]ProductNutritionReport(nil), shop.ProductNutritionReports...)
	out.ShoppingGroups = append([]ShoppingGroup(nil), shop.ShoppingGroups...)
	for i := range out.ShoppingGroups {
		out.ShoppingGroups[i].Items = append([]SelectedProduct(nil), shop.ShoppingGroups[i].Items...)
		out.ShoppingGroups[i].BasketLines = append([]string(nil), shop.ShoppingGroups[i].BasketLines...)
	}
	out.Notes = append([]string(nil), shop.Notes...)
	return out
}

func firstCandidateBySource(candidates []ProductOptimizationCandidate, source string) ProductOptimizationCandidate {
	for _, candidate := range candidates {
		if candidate.Source == source {
			return candidate
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return ProductOptimizationCandidate{}
}

func bestValidCandidate(candidates []ProductOptimizationCandidate) ProductOptimizationCandidate {
	var best ProductOptimizationCandidate
	for _, candidate := range candidates {
		if !candidate.Valid {
			continue
		}
		if best.ID == "" || candidate.Score.FinalScore > best.Score.FinalScore {
			best = candidate
		}
	}
	return best
}

func optimizationCandidateKey(c ProductOptimizationCandidate) string {
	return firstNonEmptyString(c.ProductID, c.SKU, normalizeKey(c.ProductName))
}

func finalLedgerStatus(ledger *QuantityLedger) string {
	if ledger == nil {
		return ""
	}
	return string(ledger.Status)
}

func readinessRegressed(before, after ReadinessGate) bool {
	if before.SafeToBuild && !after.SafeToBuild {
		return true
	}
	if before.Status == ReadinessReadyExact && after.Status != ReadinessReadyExact {
		return true
	}
	return false
}

func blockedByNonOptimizableIssues(gate ReadinessGate) bool {
	if gate.SafeToBuild {
		return false
	}
	for _, issue := range gate.BlockingIssues {
		switch issue.Code {
		case "estimated_variable_weight_blocked", "estimated_allocation_blocked", "low_confidence_blocked", "low_confidence_allocation_blocked", "package_count_invalid", "missing_required_quantity":
			return false
		}
	}
	return len(gate.BlockingIssues) > 0
}

func unitDimension(unit string) string {
	switch normalizeUnit(unit) {
	case "g":
		return "mass"
	case "ml":
		return "volume"
	case "unit":
		return "units"
	default:
		return "unknown"
	}
}
