package food

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/money"
	"github.com/wachtermar/carrito/internal/strutil"
)

type ProductDetailFetcher interface {
	Product(context.Context, string) (alcampo.Product, error)
}

type QuantityLedgerOptions struct {
	StoreID                      string
	EnrichProducts               bool
	StrictQuantity               bool
	AllowEstimatedVariableWeight bool
	AllowLowConfidenceBasket     bool
	DetailCacheDir               string
	ServingPlanFingerprint       string
	ScaledMealPlanFingerprint    string
	PantryResolutionFingerprint  string
	ShopRequirementsFingerprint  string
	ProductEvidenceFingerprint   string
	PantryResolution             *PantryResolution
}

var (
	ledgerMultipliedQuantityRE = regexp.MustCompile(`(?i)(\d+(?:[,.]\d+)?)\s*(?:x|×)\s*(\d+(?:[,.]\d+)?)\s*(kg|kilo|kilos|g|gr|gramos|ml|cl|l|litro|litros|ud|uds|u|unidad|unidades)\b`)
	ledgerSimpleQuantityRE     = regexp.MustCompile(`(?i)(\d+(?:[,.]\d+)?)\s*(kg|kilo|kilos|g|gr|gramos|ml|cl|l|litro|litros|ud|uds|u|unidad|unidades)\b`)
	ledgerCountQuantityRE      = regexp.MustCompile(`(?i)(?:pack|paquete|caja|bandeja)?\s*(?:de)?\s*(\d+)\s*(?:ud|uds|u|unidad|unidades)\b`)
	ledgerDrainedQuantityRE    = regexp.MustCompile(`(?i)(?:peso\s*)?escurrid[ao]?\s*(\d+(?:[,.]\d+)?)\s*(kg|g|gr|gramos|ml|cl|l|litro|litros)\b`)
	ledgerNetQuantityRE        = regexp.MustCompile(`(?i)(?:peso\s*)?net[ao]?\s*(\d+(?:[,.]\d+)?)\s*(kg|g|gr|gramos|ml|cl|l|litro|litros)\b`)
)

func EnrichShopResultProducts(ctx context.Context, shop ShopResult, fetcher ProductDetailFetcher, opts QuantityLedgerOptions) ShopResult {
	if len(shop.SelectedProducts) == 0 {
		return shop
	}
	for i := range shop.SelectedProducts {
		selected := &shop.SelectedProducts[i]
		if selected.Error != "" || selected.Product.Name == "" {
			continue
		}
		selected.ProductEvidenceSource = ProductEvidenceSourceEmbeddedSelection
		selected.ProductEvidenceCheckedAt = nowStamp()
		selected.ProductEvidenceError = ""
		if opts.EnrichProducts && fetcher != nil {
			detail, source, err := productDetailForSelection(ctx, fetcher, selected.Product, opts)
			if err != nil {
				selected.Warnings = append(selected.Warnings, "Product detail enrichment unavailable; using search result data: "+err.Error())
				selected.ProductEvidenceError = err.Error()
			} else {
				selected.Product = mergeProductSummary(selected.Product, detail)
				if source != "" {
					selected.NutritionProvenance = source
					selected.ProductEvidenceSource = source
				}
			}
		}
		count, lineTotal, reason, warnings := EstimatePurchaseQuantity(selected.Ingredient, selected.Product)
		selected.PurchaseQuantity = fmt.Sprintf("%d", count)
		selected.PackageCount = count
		selected.LineTotal = lineTotal
		selected.QuantityReason = reason
		selected.Warnings = append(nonEnrichmentWarnings(selected.Warnings), warnings...)
		selected.ProductNutrition = nil
		selected.RequiredNutrition = nil
		selected.PurchasedNutrition = nil
		selected.NutritionWarnings = nil
		AttachSelectedProductNutrition(selected)
	}
	return rebuildShopDerivedFields(shop)
}

func BuildQuantityLedger(plan MealPlan, shop ShopResult, opts QuantityLedgerOptions) QuantityLedger {
	requirements := ingredientRequirementsFromPlan(plan)
	annotateRequirementsWithPantry(requirements, opts.PantryResolution)
	evidence := productEvidenceFromShop(shop)
	evidenceByKey := map[string]ProductEvidence{}
	for _, ev := range evidence {
		evidenceByKey[productEvidenceKey(ev)] = ev
	}
	selectedByIngredient := mapSelectedByIngredient(shop.SelectedProducts)
	productAggregates := buildProductAggregates(requirements, selectedByIngredient, evidenceByKey)
	allocations := buildAllocations(requirements, selectedByIngredient, evidenceByKey, productAggregates)
	annotateAllocationsWithPantry(allocations, requirements)
	totals := buildLedgerTotals(productAggregates, shop.EstimatedTotal)
	nutrition := buildLedgerNutrition(requirements, selectedByIngredient, productAggregates)
	summary := summarizeLedger(requirements, allocations, nutrition)
	status := ledgerStatus(summary)
	warnings := collectLedgerWarnings(allocations, evidence)
	ledger := QuantityLedger{
		Status:                      status,
		GeneratedAt:                 nowStamp(),
		StoreID:                     strings.TrimSpace(opts.StoreID),
		MealPlanFingerprint:         MealPlanFingerprint(plan),
		ServingPlanFingerprint:      strings.TrimSpace(opts.ServingPlanFingerprint),
		ScaledMealPlanFingerprint:   strings.TrimSpace(opts.ScaledMealPlanFingerprint),
		ProductSelectionFingerprint: ProductSelectionFingerprint(shop),
		ProductEvidenceFingerprint:  strings.TrimSpace(opts.ProductEvidenceFingerprint),
		PantryResolutionFingerprint: strings.TrimSpace(opts.PantryResolutionFingerprint),
		ShopRequirementsFingerprint: strings.TrimSpace(opts.ShopRequirementsFingerprint),
		Requirements:                requirements,
		ProductEvidence:             evidence,
		Allocations:                 allocations,
		Totals:                      totals,
		Nutrition:                   nutrition,
		Summary:                     summary,
		Warnings:                    warnings,
		Provenance: []DataProvenance{
			{
				Scope:      "quantity_ledger",
				Source:     "carrito_quantity_ledger",
				Confidence: "medium",
				Message:    "Ingredient coverage, package counts, confidence badges, nutrition coverage, and basket safety are derived from deterministic product evidence and quantity parsing.",
			},
		},
	}
	ledger.Summary.SafeToBuildBasket = BasketSafetyFromLedger(ledger, opts).SafeToBuild
	return ledger
}

func BasketSafetyFromLedger(ledger QuantityLedger, opts QuantityLedgerOptions) BasketSafety {
	var warnings []string
	for _, warning := range ledger.Warnings {
		if warning.Message != "" {
			warnings = append(warnings, warning.Message)
		}
	}
	switch ledger.Status {
	case LedgerCompleteExact:
		return BasketSafety{
			Status:                      BasketSafetySafe,
			SafeToBuild:                 true,
			Reason:                      "All selected products have exact package quantities and cover the required ingredients.",
			Warnings:                    warnings,
			MealPlanFingerprint:         ledger.MealPlanFingerprint,
			ServingPlanFingerprint:      ledger.ServingPlanFingerprint,
			ScaledMealPlanFingerprint:   ledger.ScaledMealPlanFingerprint,
			ProductSelectionFingerprint: ledger.ProductSelectionFingerprint,
			ProductEvidenceFingerprint:  ledger.ProductEvidenceFingerprint,
			PantryResolutionFingerprint: ledger.PantryResolutionFingerprint,
			ShopRequirementsFingerprint: ledger.ShopRequirementsFingerprint,
		}
	case LedgerCompleteEstimated:
		if opts.AllowEstimatedVariableWeight {
			return BasketSafety{
				Status:                      BasketSafetyEstimated,
				SafeToBuild:                 true,
				AllowsEstimates:             true,
				Reason:                      "All ingredients are covered, but at least one variable-weight or approximate package line is estimated.",
				Warnings:                    warnings,
				MealPlanFingerprint:         ledger.MealPlanFingerprint,
				ServingPlanFingerprint:      ledger.ServingPlanFingerprint,
				ScaledMealPlanFingerprint:   ledger.ScaledMealPlanFingerprint,
				ProductSelectionFingerprint: ledger.ProductSelectionFingerprint,
				ProductEvidenceFingerprint:  ledger.ProductEvidenceFingerprint,
				PantryResolutionFingerprint: ledger.PantryResolutionFingerprint,
				ShopRequirementsFingerprint: ledger.ShopRequirementsFingerprint,
			}
		}
		return BasketSafety{
			Status:                      BasketSafetyReviewOnly,
			SafeToBuild:                 false,
			AllowsEstimates:             false,
			Reason:                      "All ingredients are covered, but estimated variable-weight products require review before automatic basket creation.",
			Warnings:                    warnings,
			MealPlanFingerprint:         ledger.MealPlanFingerprint,
			ServingPlanFingerprint:      ledger.ServingPlanFingerprint,
			ScaledMealPlanFingerprint:   ledger.ScaledMealPlanFingerprint,
			ProductSelectionFingerprint: ledger.ProductSelectionFingerprint,
			ProductEvidenceFingerprint:  ledger.ProductEvidenceFingerprint,
			PantryResolutionFingerprint: ledger.PantryResolutionFingerprint,
			ShopRequirementsFingerprint: ledger.ShopRequirementsFingerprint,
		}
	case LedgerNeedsReview:
		return BasketSafety{
			Status:                      BasketSafetyReviewOnly,
			SafeToBuild:                 opts.AllowLowConfidenceBasket,
			Reason:                      "At least one selected product has low quantity confidence or incompatible units.",
			Warnings:                    warnings,
			MealPlanFingerprint:         ledger.MealPlanFingerprint,
			ServingPlanFingerprint:      ledger.ServingPlanFingerprint,
			ScaledMealPlanFingerprint:   ledger.ScaledMealPlanFingerprint,
			ProductSelectionFingerprint: ledger.ProductSelectionFingerprint,
			ProductEvidenceFingerprint:  ledger.ProductEvidenceFingerprint,
			PantryResolutionFingerprint: ledger.PantryResolutionFingerprint,
			ShopRequirementsFingerprint: ledger.ShopRequirementsFingerprint,
		}
	default:
		return BasketSafety{
			Status:                      BasketSafetyUnsafe,
			SafeToBuild:                 false,
			Reason:                      "At least one required ingredient is missing or uncovered.",
			Warnings:                    warnings,
			MealPlanFingerprint:         ledger.MealPlanFingerprint,
			ServingPlanFingerprint:      ledger.ServingPlanFingerprint,
			ScaledMealPlanFingerprint:   ledger.ScaledMealPlanFingerprint,
			ProductSelectionFingerprint: ledger.ProductSelectionFingerprint,
			ProductEvidenceFingerprint:  ledger.ProductEvidenceFingerprint,
			PantryResolutionFingerprint: ledger.PantryResolutionFingerprint,
			ShopRequirementsFingerprint: ledger.ShopRequirementsFingerprint,
		}
	}
}

func ParsePackageEvidence(values ...string) PackageEvidence {
	rawTexts := compactStrings(values...)
	text := normalizeLedgerQuantityText(strings.Join(rawTexts, " "))
	ev := PackageEvidence{
		RawTexts:   rawTexts,
		SalesUnit:  "unknown",
		Confidence: 0.2,
	}
	if text == "" {
		ev.Warnings = append(ev.Warnings, "No package text was available.")
		return ev
	}
	ev.VariableWeight = containsVariableWeightText(text)
	ev.ApproximateWeight = containsApproximateWeightText(text)
	if ev.VariableWeight {
		ev.SalesUnit = variableSalesUnit(text)
		ev.ParseMethod = "variable_weight_text"
		ev.Warnings = append(ev.Warnings, "Product appears to be sold by variable or approximate weight.")
	}
	if strings.Contains(text, "docena") {
		count := 12
		q := normalizedQuantity("docena", 12, "unit", 0.95, "docena")
		ev.UnitQuantity = &q
		ev.NetQuantity = quantityRange(q, !ev.VariableWeight && !ev.ApproximateWeight, quantityRangeReason(ev))
		ev.PackCount = &count
		ev.SalesUnit = "unit"
		ev.Confidence = q.Confidence
		ev.ParseMethod = "docena"
		return ev
	}
	if m := ledgerMultipliedQuantityRE.FindStringSubmatch(text); len(m) == 4 {
		left, ok1 := parsePackageNumber(m[1])
		right, ok2 := parsePackageNumber(m[2])
		unit := normalizeUnit(m[3])
		if ok1 && ok2 && unit != "" {
			total := left * right
			q := normalizedQuantity(m[0], total, unit, fixedPackageConfidence(ev, 0.95), "pack_multiplication")
			count := int(math.Round(left))
			if math.Abs(left-float64(count)) < 0.001 {
				ev.PackCount = &count
			}
			ev.NetQuantity = quantityRange(q, !ev.VariableWeight && !ev.ApproximateWeight, quantityRangeReason(ev))
			ev.SalesUnit = packageSalesUnit(q.BaseUnit)
			ev.BaseUnit = q.BaseUnit
			ev.Confidence = q.Confidence
			ev.ParseMethod = q.ParseMethod
		}
	}
	if m := ledgerDrainedQuantityRE.FindStringSubmatch(text); len(m) == 3 {
		if q, ok := drainedOrNetQuantity(m, "drained_weight", ev.PackCount); ok {
			ev.DrainedQuantity = quantityRange(q, !ev.VariableWeight && !ev.ApproximateWeight, quantityRangeReason(ev))
		}
	}
	if ev.NetQuantity == nil {
		if m := ledgerNetQuantityRE.FindStringSubmatch(text); len(m) == 3 {
			if q, ok := drainedOrNetQuantity(m, "net_weight", nil); ok {
				ev.NetQuantity = quantityRange(q, !ev.VariableWeight && !ev.ApproximateWeight, quantityRangeReason(ev))
				ev.SalesUnit = packageSalesUnit(q.BaseUnit)
				ev.BaseUnit = q.BaseUnit
				ev.Confidence = q.Confidence
				ev.ParseMethod = q.ParseMethod
			}
		}
	}
	if ev.NetQuantity == nil {
		if m := ledgerCountQuantityRE.FindStringSubmatch(text); len(m) == 2 {
			if qty, ok := parsePackageNumber(m[1]); ok {
				count := int(math.Round(qty))
				q := normalizedQuantity(m[0], qty, "unit", fixedPackageConfidence(ev, 0.9), "unit_count")
				ev.PackCount = &count
				ev.UnitQuantity = &q
				ev.NetQuantity = quantityRange(q, !ev.VariableWeight && !ev.ApproximateWeight, quantityRangeReason(ev))
				ev.SalesUnit = "unit"
				ev.BaseUnit = "unit"
				ev.Confidence = q.Confidence
				ev.ParseMethod = q.ParseMethod
			}
		}
	}
	if ev.NetQuantity == nil {
		if q, ok := bestSimplePackageQuantity(rawTexts, ev); ok {
			ev.NetQuantity = quantityRange(q, !ev.VariableWeight && !ev.ApproximateWeight, quantityRangeReason(ev))
			ev.SalesUnit = packageSalesUnit(q.BaseUnit)
			ev.BaseUnit = q.BaseUnit
			ev.Confidence = q.Confidence
			ev.ParseMethod = q.ParseMethod
		}
	}
	if ev.NetQuantity == nil {
		if ev.VariableWeight {
			ev.Confidence = 0.35
			ev.ParseMethod = strutil.FirstNonEmpty(ev.ParseMethod, "variable_weight_without_quantity")
			ev.Warnings = append(ev.Warnings, "No expected package weight was found for this variable-weight product.")
		} else {
			ev.Warnings = append(ev.Warnings, "Could not infer package quantity from product data.")
		}
	}
	return ev
}

type packageQuantityCandidate struct {
	quantity NormalizedQuantity
	source   string
	index    int
}

func bestSimplePackageQuantity(rawTexts []string, ev PackageEvidence) (NormalizedQuantity, bool) {
	var candidates []packageQuantityCandidate
	for i, raw := range rawTexts {
		text := normalizeLedgerQuantityText(raw)
		for _, m := range ledgerSimpleQuantityRE.FindAllStringSubmatch(text, -1) {
			if len(m) != 3 {
				continue
			}
			qty, ok := parsePackageNumber(m[1])
			unit := normalizeUnit(m[2])
			if !ok || unit == "" {
				continue
			}
			q := normalizedQuantity(m[0], qty, unit, fixedPackageConfidence(ev, 0.85), "single_package_quantity")
			if q.BaseUnit == "" {
				continue
			}
			candidates = append(candidates, packageQuantityCandidate{quantity: q, source: text, index: i})
		}
	}
	if len(candidates) == 0 {
		return NormalizedQuantity{}, false
	}
	best := candidates[0]
	for _, c := range candidates[1:] {
		if packageQuantityCandidatePreferred(best, c) {
			best = c
		}
	}
	return best.quantity, true
}

func packageQuantityCandidatePreferred(current, candidate packageQuantityCandidate) bool {
	if current.quantity.BaseUnit == candidate.quantity.BaseUnit {
		return false
	}
	if current.quantity.BaseUnit == "g" && candidate.quantity.BaseUnit == "ml" && liquidPackageText(candidate.source) {
		return true
	}
	return false
}

func liquidPackageText(text string) bool {
	return strings.Contains(text, " ml") ||
		strings.Contains(text, " litro") ||
		strings.Contains(text, " litros") ||
		strings.Contains(text, " l ") ||
		strings.HasSuffix(text, " l")
}

func productDetailForSelection(ctx context.Context, fetcher ProductDetailFetcher, product ProductSummary, opts QuantityLedgerOptions) (alcampo.Product, string, error) {
	ref := productDetailRef(product)
	if ref == "" {
		return alcampo.Product{}, "", fmt.Errorf("selected product has no SKU, URL, ID, or name for detail lookup")
	}
	cachePath := productDetailCachePath(product, opts)
	if cachePath != "" {
		if cached, err := readCachedProductDetail(cachePath); err == nil {
			return cached, "alcampo_product_detail_cache", nil
		}
	}
	detail, err := fetcher.Product(ctx, ref)
	if err != nil {
		return alcampo.Product{}, "", err
	}
	if detail.DetailUnavailable {
		return alcampo.Product{}, "", fmt.Errorf("detail page unavailable; kept selected search result")
	}
	if cachePath != "" {
		_ = writeCachedProductDetail(cachePath, detail)
	}
	return detail, "alcampo_product_detail", nil
}

func productDetailRef(product ProductSummary) string {
	return strutil.FirstNonEmpty(product.URL, product.SKU, product.ID, product.Name)
}

func productDetailCachePath(product ProductSummary, opts QuantityLedgerOptions) string {
	if strings.TrimSpace(opts.DetailCacheDir) == "" {
		return ""
	}
	keySource := strings.Join([]string{opts.StoreID, product.ID, product.SKU, product.URL, product.Name, time.Now().UTC().Format("2006-01-02")}, "|")
	sum := sha256.Sum256([]byte(keySource))
	return filepath.Join(opts.DetailCacheDir, hex.EncodeToString(sum[:])+".json")
}

func readCachedProductDetail(path string) (alcampo.Product, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return alcampo.Product{}, err
	}
	var product alcampo.Product
	if err := json.Unmarshal(data, &product); err != nil {
		return alcampo.Product{}, err
	}
	return product, nil
}

func writeCachedProductDetail(path string, product alcampo.Product) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(product, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func mergeProductSummary(base ProductSummary, detail alcampo.Product) ProductSummary {
	d := ProductSummaryFromAlcampo(detail)
	out := base
	out.ID = strutil.FirstNonEmpty(d.ID, out.ID)
	out.SKU = strutil.FirstNonEmpty(d.SKU, out.SKU)
	out.Name = strutil.FirstNonEmpty(d.Name, out.Name)
	out.Brand = strutil.FirstNonEmpty(d.Brand, out.Brand)
	if d.Price.Amount != "" {
		out.Price = d.Price
	}
	if d.UnitPrice.Amount != "" {
		out.UnitPrice = d.UnitPrice
	}
	out.Unit = strutil.FirstNonEmpty(d.Unit, out.Unit)
	out.Size = strutil.FirstNonEmpty(d.Size, out.Size)
	out.URL = strutil.FirstNonEmpty(d.URL, out.URL)
	out.ImageURL = strutil.FirstNonEmpty(d.ImageURL, out.ImageURL)
	out.Category = strutil.FirstNonEmpty(d.Category, out.Category)
	if d.Available != nil {
		out.Available = d.Available
	}
	if len(d.Offers) > 0 {
		out.Offers = d.Offers
		out.OfferCount = len(d.Offers)
	}
	out.Allergens = strutil.FirstNonEmpty(d.Allergens, out.Allergens)
	out.Nutrition = strutil.FirstNonEmpty(d.Nutrition, out.Nutrition)
	if d.PackageQuantity > 0 && d.PackageUnit != "" {
		out.PackageQuantity = d.PackageQuantity
		out.PackageUnit = d.PackageUnit
	} else if out.PackageQuantity == 0 || out.PackageUnit == "" {
		if qty, unit, ok := ParsePackageQuantity(out.Size, out.Name); ok {
			out.PackageQuantity = qty
			out.PackageUnit = unit
		}
	}
	return out
}

func nonEnrichmentWarnings(warnings []string) []string {
	var out []string
	for _, warning := range warnings {
		if strings.Contains(warning, "Product detail enrichment unavailable") {
			out = append(out, warning)
		}
	}
	return out
}

func rebuildShopDerivedFields(shop ShopResult) ShopResult {
	shop.BasketLines = nil
	shop.ProductNutritionReports = nil
	shop.NutritionWarnings = nil
	shop.EstimatedTotal = money.Money{}
	shop.Complete = true
	for _, selected := range shop.SelectedProducts {
		if selected.Error != "" {
			shop.Complete = false
		}
		if report := ProductNutritionReportFromSelection(selected); report != nil {
			shop.ProductNutritionReports = append(shop.ProductNutritionReports, *report)
			shop.NutritionWarnings = append(shop.NutritionWarnings, report.Warnings...)
		}
		if line := BasketLineForSelection(selected); line != "" {
			shop.BasketLines = append(shop.BasketLines, line)
		}
		shop.EstimatedTotal.Cents += selected.LineTotal.Cents
	}
	shop.EstimatedTotal = money.Money{Amount: money.FormatAmount(shop.EstimatedTotal.Cents), Currency: "EUR", Cents: shop.EstimatedTotal.Cents}
	shop.ShoppingGroups = GroupSelectedProducts(shop.SelectedProducts)
	return shop
}

func ingredientRequirementsFromPlan(plan MealPlan) []IngredientRequirement {
	refs := ingredientUsageRefsByPurchaseKey(plan)
	var requirements []IngredientRequirement
	for i, ingredient := range plan.RequiredPurchases {
		req := IngredientRequirement{
			RequirementID:  fmt.Sprintf("req-%03d", i+1),
			IngredientName: ingredient.Name,
			RawQuantity:    formatIngredientQuantity(ingredient.Quantity, ingredient.Unit),
		}
		if usages := refs[ingredientPurchaseKey(ingredient)]; len(usages) > 0 {
			req.Usages = append([]IngredientUsageRef(nil), usages...)
			req.RecipeID = usages[0].RecipeID
			req.RecipeTitle = usages[0].RecipeTitle
			req.Day = usages[0].Day
			req.MealSlot = usages[0].MealSlot
		}
		if q, ok := normalizedIngredientQuantity(ingredient.Quantity, ingredient.Unit, 0.95, "meal_plan_required_purchase"); ok {
			req.RequiredQuantity = &q
			req.ParseConfidence = q.Confidence
		} else {
			req.ParseConfidence = 0.2
			req.Warnings = append(req.Warnings, fmt.Sprintf("Ingredient unit %q cannot be converted safely for quantity coverage.", ingredient.Unit))
		}
		requirements = append(requirements, req)
	}
	return requirements
}

func annotateRequirementsWithPantry(requirements []IngredientRequirement, resolution *PantryResolution) {
	if resolution == nil {
		for i := range requirements {
			requirements[i].ShopRequiredQuantity = cloneNormalizedQuantity(requirements[i].RequiredQuantity)
			requirements[i].TotalRequiredQuantity = cloneNormalizedQuantity(requirements[i].RequiredQuantity)
			requirements[i].SourcingStatus = PantryDecisionBuyFull
			requirements[i].PantryDecision = PantryDecisionBuyFull
		}
		return
	}
	byKey := pantryResolutionByKey(resolution)
	for i := range requirements {
		req := &requirements[i]
		key := normalizePantryIngredientKey(req.IngredientName)
		line, ok := byKey[key]
		if !ok {
			req.ShopRequiredQuantity = cloneNormalizedQuantity(req.RequiredQuantity)
			req.TotalRequiredQuantity = cloneNormalizedQuantity(req.RequiredQuantity)
			req.SourcingStatus = PantryDecisionBuyFull
			req.PantryDecision = PantryDecisionBuyFull
			continue
		}
		req.TotalRequiredQuantity = cloneNormalizedQuantity(line.TotalRequired)
		req.PantryAllocatedQuantity = cloneNormalizedQuantity(line.PantryAllocated)
		req.ShopRequiredQuantity = cloneNormalizedQuantity(line.ShopRequired)
		req.SourcingStatus = pantrySourcingStatus(line)
		req.PantryDecision = line.PantryDecision
	}
}

func annotateAllocationsWithPantry(allocations []IngredientProductAllocation, requirements []IngredientRequirement) {
	byID := map[string]IngredientRequirement{}
	for _, req := range requirements {
		byID[req.RequirementID] = req
	}
	for i := range allocations {
		req := byID[allocations[i].RequirementID]
		allocations[i].TotalRequiredQuantity = cloneNormalizedQuantity(req.TotalRequiredQuantity)
		allocations[i].PantryAllocatedQuantity = cloneNormalizedQuantity(req.PantryAllocatedQuantity)
		allocations[i].ShopRequiredQuantity = cloneNormalizedQuantity(req.ShopRequiredQuantity)
		allocations[i].SourcingStatus = req.SourcingStatus
		allocations[i].PantryDecision = req.PantryDecision
	}
}

func ingredientUsageRefsByPurchaseKey(plan MealPlan) map[string][]IngredientUsageRef {
	refs := make(map[string][]IngredientUsageRef)
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			for _, ingredient := range meal.Recipe.Ingredients {
				q, _ := normalizedIngredientQuantity(ingredient.Quantity, ingredient.Unit, 0.95, "recipe_ingredient")
				ref := IngredientUsageRef{
					Day:            day.Day,
					MealSlot:       meal.Type,
					RecipeID:       meal.Recipe.ID,
					RecipeTitle:    meal.Recipe.Title,
					IngredientKey:  normalizeKey(ingredient.Name),
					IngredientName: ingredient.Name,
				}
				if q.Unit != "" {
					ref.RequiredQuantity = &q
				}
				key := ingredientPurchaseKey(ingredient)
				if key != "|" {
					refs[key] = append(refs[key], ref)
				}
			}
		}
	}
	return refs
}

func productEvidenceFromShop(shop ShopResult) []ProductEvidence {
	var evidence []ProductEvidence
	seen := map[string]bool{}
	for _, selected := range shop.SelectedProducts {
		if selected.Product.Name == "" {
			continue
		}
		ev := productEvidenceFromSelection(selected)
		key := productEvidenceKey(ev)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		evidence = append(evidence, ev)
	}
	return evidence
}

func productEvidenceFromSelection(selected SelectedProduct) ProductEvidence {
	product := selected.Product
	pkg := ParsePackageEvidence(product.Size, product.Name)
	if product.PackageQuantity > 0 && product.PackageUnit != "" {
		q := normalizedQuantity(fmt.Sprintf("%.3g %s", product.PackageQuantity, product.PackageUnit), product.PackageQuantity, product.PackageUnit, 0.9, "product_summary_package")
		pkg.NetQuantity = quantityRange(q, true, "")
		pkg.SalesUnit = packageSalesUnit(q.BaseUnit)
		pkg.BaseUnit = q.BaseUnit
		pkg.Confidence = math.Max(pkg.Confidence, q.Confidence)
		pkg.ParseMethod = q.ParseMethod
	}
	var nutrition *NutritionEvidence
	if selected.ProductNutrition != nil {
		nutrition = &NutritionEvidence{
			Label:      selected.ProductNutrition,
			Source:     strutil.FirstNonEmpty(selected.NutritionProvenance, selected.ProductNutrition.Source),
			Confidence: selected.ProductNutrition.Confidence,
			Warnings:   append([]string(nil), selected.NutritionWarnings...),
		}
	}
	var availableConfidence float64
	if product.Available != nil {
		availableConfidence = 0.85
	}
	offers := offerEvidence(product.Offers, selected.PackageCount)
	confidence := ProductConfidence{
		Detail:       detailConfidence(product),
		Package:      pkg.Confidence,
		Nutrition:    nutritionConfidence(nutrition),
		Availability: availableConfidence,
	}
	confidence.Overall = averagePositive(confidence.Detail, confidence.Package, confidence.Nutrition, confidence.Availability)
	ev := ProductEvidence{
		ProductID: product.ID,
		SKU:       product.SKU,
		Name:      product.Name,
		Brand:     product.Brand,
		URL:       product.URL,
		ImageURL:  product.ImageURL,
		Price:     product.Price,
		UnitPrice: product.UnitPrice,
		Unit:      product.Unit,
		Package:   pkg,
		Availability: AvailabilityEvidence{
			Available:  product.Available,
			Confidence: availableConfidence,
			Source:     "alcampo_product",
		},
		Offers:     offers,
		Nutrition:  nutrition,
		Confidence: confidence,
		Sources: []DataProvenance{
			{
				Scope:      "product",
				Source:     "alcampo_search_or_detail",
				URL:        product.URL,
				Confidence: confidenceLabel(confidence.Overall),
				Message:    "Product evidence combines Alcampo search result data with selected-product detail enrichment when available.",
			},
		},
	}
	for _, warning := range pkg.Warnings {
		ev.Warnings = append(ev.Warnings, LedgerWarning{Code: "package_quantity", Message: warning, Scope: product.Name})
	}
	if nutrition == nil {
		ev.Warnings = append(ev.Warnings, LedgerWarning{Code: "nutrition_missing", Message: "No parsed product nutrition label is available.", Scope: product.Name})
	}
	for _, offer := range offers {
		if offer.Status == "unclear_terms" || offer.Status == "requires_loyalty" {
			ev.Warnings = append(ev.Warnings, LedgerWarning{Code: "offer_terms", Message: "Offer terms are not guaranteed: " + offer.Text, Scope: product.Name})
		}
	}
	return ev
}

func mapSelectedByIngredient(selected []SelectedProduct) map[string]SelectedProduct {
	out := make(map[string]SelectedProduct, len(selected))
	for _, item := range selected {
		key := normalizeKey(item.Ingredient.Name)
		if key != "" {
			out[key] = item
		}
	}
	return out
}

type productAggregate struct {
	Key                 string
	Selected            SelectedProduct
	Evidence            ProductEvidence
	RequirementIDs      []string
	RequiredBaseValue   float64
	RequiredBaseUnit    string
	PackageCount        int
	Purchased           *QuantityRange
	Excess              *NormalizedQuantity
	QuantityConfidence  float64
	NeedsReview         bool
	VariableOrEstimated bool
	Warnings            []LedgerWarning
}

func buildProductAggregates(requirements []IngredientRequirement, selectedByIngredient map[string]SelectedProduct, evidenceByKey map[string]ProductEvidence) map[string]*productAggregate {
	aggregates := map[string]*productAggregate{}
	for _, req := range requirements {
		selected, ok := selectedByIngredient[normalizeKey(req.IngredientName)]
		if !ok || selected.Error != "" || selected.Product.Name == "" {
			continue
		}
		ev := evidenceByKey[productEvidenceKeyFromProduct(selected.Product)]
		key := productEvidenceKey(ev)
		if key == "" {
			key = productEvidenceKeyFromProduct(selected.Product)
		}
		agg := aggregates[key]
		if agg == nil {
			agg = &productAggregate{
				Key:                key,
				Selected:           selected,
				Evidence:           ev,
				QuantityConfidence: 1,
			}
			aggregates[key] = agg
		}
		agg.RequirementIDs = append(agg.RequirementIDs, req.RequirementID)
		if req.RequiredQuantity == nil {
			agg.NeedsReview = true
			agg.QuantityConfidence = math.Min(agg.QuantityConfidence, 0.2)
			agg.Warnings = append(agg.Warnings, LedgerWarning{Code: "required_quantity_unknown", Message: "Required quantity cannot be normalized.", Scope: req.IngredientName})
			continue
		}
		pkg := ev.Package.NetQuantity
		if pkg == nil || pkg.Expected.BaseValue <= 0 {
			agg.NeedsReview = true
			agg.QuantityConfidence = math.Min(agg.QuantityConfidence, 0.25)
			agg.Warnings = append(agg.Warnings, LedgerWarning{Code: "package_quantity_unknown", Message: "Selected product package quantity is unknown.", Scope: ev.Name})
			continue
		}
		if agg.RequiredBaseUnit == "" {
			agg.RequiredBaseUnit = req.RequiredQuantity.BaseUnit
		}
		if req.RequiredQuantity.BaseUnit != agg.RequiredBaseUnit || pkg.Expected.BaseUnit != req.RequiredQuantity.BaseUnit {
			agg.NeedsReview = true
			agg.QuantityConfidence = math.Min(agg.QuantityConfidence, 0.3)
			agg.Warnings = append(agg.Warnings, LedgerWarning{Code: "unit_mismatch", Message: fmt.Sprintf("Recipe unit %s and product package unit %s are incompatible.", req.RequiredQuantity.BaseUnit, pkg.Expected.BaseUnit), Scope: req.IngredientName})
			continue
		}
		agg.RequiredBaseValue += req.RequiredQuantity.BaseValue
		agg.QuantityConfidence = math.Min(agg.QuantityConfidence, math.Min(req.RequiredQuantity.Confidence, pkg.Expected.Confidence))
		if !pkg.IsExact || ev.Package.VariableWeight || ev.Package.ApproximateWeight {
			agg.VariableOrEstimated = true
		}
	}
	for _, agg := range aggregates {
		finalizeProductAggregate(agg)
	}
	return aggregates
}

func finalizeProductAggregate(agg *productAggregate) {
	if agg == nil || agg.Evidence.Package.NetQuantity == nil || agg.RequiredBaseValue <= 0 || agg.RequiredBaseUnit == "" {
		if agg != nil {
			agg.NeedsReview = true
		}
		return
	}
	pkg := agg.Evidence.Package.NetQuantity.Expected
	if pkg.BaseUnit != agg.RequiredBaseUnit || pkg.BaseValue <= 0 {
		agg.NeedsReview = true
		return
	}
	count := int(math.Ceil(agg.RequiredBaseValue / pkg.BaseValue))
	if count < 1 {
		count = 1
	}
	agg.PackageCount = count
	purchased := pkg
	purchased.Value *= float64(count)
	purchased.BaseValue *= float64(count)
	purchased.Raw = fmt.Sprintf("%d package(s) x %s", count, pkg.Raw)
	agg.Purchased = quantityRange(purchased, agg.Evidence.Package.NetQuantity.IsExact, agg.Evidence.Package.NetQuantity.Reason)
	excessBase := purchased.BaseValue - agg.RequiredBaseValue
	if excessBase > 0 {
		excess := quantityFromBase(excessBase, purchased.BaseUnit, 0.9, "aggregate_excess")
		agg.Excess = &excess
	}
	if agg.VariableOrEstimated {
		agg.QuantityConfidence = math.Min(agg.QuantityConfidence, 0.65)
	}
	if agg.QuantityConfidence == 0 {
		agg.QuantityConfidence = 0.5
	}
}

func buildAllocations(requirements []IngredientRequirement, selectedByIngredient map[string]SelectedProduct, evidenceByKey map[string]ProductEvidence, aggregates map[string]*productAggregate) []IngredientProductAllocation {
	var allocations []IngredientProductAllocation
	for _, req := range requirements {
		selected, ok := selectedByIngredient[normalizeKey(req.IngredientName)]
		if !ok {
			allocations = append(allocations, missingAllocation(req, "No selected product covers this ingredient."))
			continue
		}
		if selected.Error != "" {
			allocations = append(allocations, missingAllocation(req, selected.Error))
			continue
		}
		ev := evidenceByKey[productEvidenceKeyFromProduct(selected.Product)]
		key := productEvidenceKey(ev)
		agg := aggregates[key]
		allocation := IngredientProductAllocation{
			RequirementID:      req.RequirementID,
			ProductID:          selected.Product.ID,
			SKU:                selected.Product.SKU,
			ProductName:        selected.Product.Name,
			MatchType:          "exact",
			RequiredQuantity:   req.RequiredQuantity,
			MatchConfidence:    matchConfidenceForSelection(selected),
			QuantityConfidence: 0.2,
			OverallConfidence:  0.2,
		}
		if agg != nil {
			allocation.PackageCount = agg.PackageCount
			allocation.PurchasedQuantity = agg.Purchased
			allocation.ExcessQuantity = agg.Excess
			allocation.QuantityConfidence = agg.QuantityConfidence
			allocation.Warnings = append(allocation.Warnings, agg.Warnings...)
			if req.RequiredQuantity != nil && agg.RequiredBaseValue > 0 && agg.Purchased != nil {
				allocation.AllocatedQuantity = quantityRange(*req.RequiredQuantity, true, "")
				allocation.CoverageRatio = math.Min(1, agg.Purchased.Expected.BaseValue/agg.RequiredBaseValue)
			}
			if agg.NeedsReview {
				allocation.Badges = append(allocation.Badges, "LOW QUANTITY CONFIDENCE")
			} else if agg.VariableOrEstimated {
				allocation.Badges = append(allocation.Badges, "ESTIMATED WEIGHT")
			} else {
				allocation.Badges = append(allocation.Badges, "EXACT")
			}
		}
		if selected.ProductNutrition == nil {
			allocation.Badges = append(allocation.Badges, "NUTRITION MISSING")
		}
		if hasUnclearOffer(ev.Offers) {
			allocation.Badges = append(allocation.Badges, "OFFER TERMS UNCLEAR")
		}
		if len(allocation.Badges) == 0 {
			allocation.Badges = append(allocation.Badges, "LOW QUANTITY CONFIDENCE")
		}
		allocation.OverallConfidence = math.Min(allocation.MatchConfidence, allocation.QuantityConfidence)
		allocations = append(allocations, allocation)
	}
	return allocations
}

func missingAllocation(req IngredientRequirement, message string) IngredientProductAllocation {
	allocation := IngredientProductAllocation{
		RequirementID:      req.RequirementID,
		MatchType:          "missing",
		RequiredQuantity:   req.RequiredQuantity,
		MissingQuantity:    req.RequiredQuantity,
		CoverageRatio:      0,
		MatchConfidence:    0,
		QuantityConfidence: 0,
		OverallConfidence:  0,
		Badges:             []string{"MISSING"},
		Warnings:           []LedgerWarning{{Code: "missing_product", Message: message, Scope: req.IngredientName}},
	}
	return allocation
}

func buildLedgerTotals(aggregates map[string]*productAggregate, fallback money.Money) LedgerTotals {
	var totalCents int64
	var fixedCents int64
	var variableCents int64
	var lines []LedgerPriceLine
	keys := sortedAggregateKeys(aggregates)
	for _, key := range keys {
		agg := aggregates[key]
		if agg == nil || agg.Selected.Product.Name == "" {
			continue
		}
		count := agg.PackageCount
		if count <= 0 {
			count = agg.Selected.PackageCount
		}
		if count <= 0 {
			count = 1
		}
		line := multiplyMoney(agg.Selected.Product.Price, count)
		if line.Amount == "" {
			line = agg.Selected.LineTotal
		}
		totalCents += line.Cents
		if agg.VariableOrEstimated {
			variableCents += line.Cents
		} else {
			fixedCents += line.Cents
		}
		lines = append(lines, LedgerPriceLine{
			ProductID:    agg.Selected.Product.ID,
			SKU:          agg.Selected.Product.SKU,
			ProductName:  agg.Selected.Product.Name,
			PackageCount: count,
			LineTotal:    line,
			Estimated:    agg.VariableOrEstimated,
			Reason:       priceLineReason(agg),
		})
	}
	if totalCents == 0 {
		totalCents = fallback.Cents
		fixedCents = fallback.Cents
	}
	expected := money.Money{Amount: money.FormatAmount(totalCents), Currency: "EUR", Cents: totalCents}
	return LedgerTotals{
		EstimatedTotal: MoneyRange{
			Expected: expected,
			IsExact:  variableCents == 0,
			Reason:   totalRangeReason(variableCents),
		},
		FixedPriceTotal: money.Money{Amount: money.FormatAmount(fixedCents), Currency: "EUR", Cents: fixedCents},
		VariableWeightEstimate: MoneyRange{
			Expected: money.Money{Amount: money.FormatAmount(variableCents), Currency: "EUR", Cents: variableCents},
			IsExact:  variableCents == 0,
			Reason:   totalRangeReason(variableCents),
		},
		Lines: lines,
	}
}

func buildLedgerNutrition(requirements []IngredientRequirement, selectedByIngredient map[string]SelectedProduct, aggregates map[string]*productAggregate) *LedgerNutritionReport {
	if len(requirements) == 0 {
		return nil
	}
	coverage := NutritionCoverageReport{RequirementsTotal: len(requirements)}
	var requiredTotal StructuredNutrition
	var purchasedTotal StructuredNutrition
	var warnings []string
	for _, req := range requirements {
		if req.RequiredQuantity != nil {
			coverage.RequirementsWithQuantity++
		}
		selected, ok := selectedByIngredient[normalizeKey(req.IngredientName)]
		if !ok || selected.Error != "" {
			coverage.MissingNutritionLabels = append(coverage.MissingNutritionLabels, req.IngredientName)
			coverage.SkippedNutritionReasons = append(coverage.SkippedNutritionReasons, req.IngredientName+": missing selected product")
			continue
		}
		if selected.ProductNutrition == nil {
			coverage.MissingNutritionLabels = append(coverage.MissingNutritionLabels, req.IngredientName)
			coverage.SkippedNutritionReasons = append(coverage.SkippedNutritionReasons, req.IngredientName+": missing Alcampo nutrition label")
			continue
		}
		if req.RequiredQuantity == nil {
			coverage.SkippedNutritionReasons = append(coverage.SkippedNutritionReasons, req.IngredientName+": quantity cannot be normalized")
			continue
		}
		requiredEstimate, reqWarnings := estimateNutritionForNormalizedQuantity(*req.RequiredQuantity, selected.ProductNutrition)
		if requiredEstimate == nil {
			warnings = append(warnings, prefixWarnings(req.IngredientName, reqWarnings)...)
			coverage.SkippedNutritionReasons = append(coverage.SkippedNutritionReasons, prefixWarnings(req.IngredientName, reqWarnings)...)
			continue
		}
		coverage.RequirementsWithLabelNutrition++
		addStructuredNutrition(&requiredTotal, requiredEstimate.Nutrients)
		key := productEvidenceKeyFromProduct(selected.Product)
		if agg := aggregates[key]; agg != nil && agg.Purchased != nil {
			purchasedEstimate, purchaseWarnings := estimateNutritionForNormalizedQuantity(agg.Purchased.Expected, selected.ProductNutrition)
			if purchasedEstimate != nil {
				addStructuredNutrition(&purchasedTotal, purchasedEstimate.Nutrients)
			} else {
				warnings = append(warnings, prefixWarnings(req.IngredientName, purchaseWarnings)...)
			}
		}
	}
	if coverage.RequirementsTotal > 0 {
		coverage.CalorieCoverageRatio = float64(coverage.RequirementsWithLabelNutrition) / float64(coverage.RequirementsTotal)
	}
	return &LedgerNutritionReport{
		RequiredEstimate: &NutritionEstimate{
			Nutrients:  requiredTotal,
			Source:     "alcampo_label_quantity_ledger",
			Confidence: coverage.CalorieCoverageRatio,
		},
		PurchasedEstimate: &NutritionEstimate{
			Nutrients:  purchasedTotal,
			Source:     "alcampo_label_quantity_ledger",
			Confidence: coverage.CalorieCoverageRatio,
		},
		Coverage: coverage,
		Warnings: warnings,
	}
}

func summarizeLedger(requirements []IngredientRequirement, allocations []IngredientProductAllocation, nutrition *LedgerNutritionReport) LedgerSummary {
	summary := LedgerSummary{IngredientCount: len(requirements)}
	for _, allocation := range allocations {
		if allocation.MatchType == "missing" {
			summary.MissingLines++
			continue
		}
		summary.CoveredIngredientCount++
		if containsString(allocation.Badges, "LOW QUANTITY CONFIDENCE") {
			summary.NeedsReviewLines++
		} else if containsString(allocation.Badges, "ESTIMATED WEIGHT") {
			summary.EstimatedVariableWeightLines++
		} else if containsString(allocation.Badges, "EXACT") {
			summary.ExactQuantityLines++
		}
	}
	if nutrition != nil {
		summary.NutritionCoverageRatio = nutrition.Coverage.CalorieCoverageRatio
	}
	return summary
}

func ledgerStatus(summary LedgerSummary) LedgerStatus {
	switch {
	case summary.MissingLines > 0 || summary.CoveredIngredientCount < summary.IngredientCount:
		return LedgerIncomplete
	case summary.NeedsReviewLines > 0:
		return LedgerNeedsReview
	case summary.EstimatedVariableWeightLines > 0:
		return LedgerCompleteEstimated
	default:
		return LedgerCompleteExact
	}
}

func collectLedgerWarnings(allocations []IngredientProductAllocation, evidence []ProductEvidence) []LedgerWarning {
	seen := map[string]bool{}
	var warnings []LedgerWarning
	add := func(w LedgerWarning) {
		if w.Message == "" {
			return
		}
		key := w.Code + "|" + w.Scope + "|" + w.Message
		if seen[key] {
			return
		}
		seen[key] = true
		warnings = append(warnings, w)
	}
	for _, ev := range evidence {
		for _, warning := range ev.Warnings {
			add(warning)
		}
	}
	for _, allocation := range allocations {
		for _, warning := range allocation.Warnings {
			add(warning)
		}
	}
	return warnings
}

func estimateNutritionForNormalizedQuantity(q NormalizedQuantity, label *StructuredNutrition) (*NutritionEstimate, []string) {
	if label == nil {
		return nil, []string{"No parsed product nutrition label was available."}
	}
	factor, warnings := nutritionFactor(q.Value, q.Unit, label)
	if len(warnings) > 0 {
		return nil, warnings
	}
	return &NutritionEstimate{
		RequiredQuantity: q.Value,
		RequiredUnit:     q.Unit,
		Factor:           factor,
		Nutrients:        scaleStructuredNutrition(*label, factor),
		Source:           label.Source,
		Confidence:       math.Min(label.Confidence, q.Confidence),
	}, nil
}

func addStructuredNutrition(dst *StructuredNutrition, src StructuredNutrition) {
	addFloatPtr(&dst.EnergyKJ, src.EnergyKJ)
	addFloatPtr(&dst.Kcal, src.Kcal)
	addFloatPtr(&dst.FatG, src.FatG)
	addFloatPtr(&dst.SaturatesG, src.SaturatesG)
	addFloatPtr(&dst.CarbsG, src.CarbsG)
	addFloatPtr(&dst.SugarsG, src.SugarsG)
	addFloatPtr(&dst.FiberG, src.FiberG)
	addFloatPtr(&dst.ProteinG, src.ProteinG)
	addFloatPtr(&dst.SaltG, src.SaltG)
	addFloatPtr(&dst.SodiumG, src.SodiumG)
	dst.Source = "alcampo_label_quantity_ledger"
	dst.Confidence = 0.8
}

func addFloatPtr(dst **float64, src *float64) {
	if src == nil {
		return
	}
	if *dst == nil {
		*dst = floatPtr(0)
	}
	**dst += *src
}

func normalizedIngredientQuantity(qty float64, unit string, confidence float64, method string) (NormalizedQuantity, bool) {
	if qty <= 0 || strings.TrimSpace(unit) == "" {
		return NormalizedQuantity{}, false
	}
	q := normalizedQuantity(formatFloat(qty)+" "+unit, qty, unit, confidence, method)
	return q, q.BaseUnit != ""
}

func normalizedQuantity(raw string, qty float64, unit string, confidence float64, method string) NormalizedQuantity {
	unit = normalizeUnit(unit)
	base, baseUnit, ok := toBaseQuantity(qty, unit)
	if !ok {
		return NormalizedQuantity{Raw: strings.TrimSpace(raw), Value: qty, Unit: unit, Confidence: math.Min(confidence, 0.2), ParseMethod: method}
	}
	return NormalizedQuantity{
		Raw:         strings.TrimSpace(raw),
		Value:       roundQty(qty),
		Unit:        unit,
		BaseValue:   roundQty(base),
		BaseUnit:    baseUnit,
		Confidence:  confidence,
		ParseMethod: method,
	}
}

func quantityFromBase(baseValue float64, baseUnit string, confidence float64, method string) NormalizedQuantity {
	return NormalizedQuantity{
		Raw:         fmt.Sprintf("%.3g %s", baseValue, baseUnit),
		Value:       roundQty(baseValue),
		Unit:        baseUnit,
		BaseValue:   roundQty(baseValue),
		BaseUnit:    baseUnit,
		Confidence:  confidence,
		ParseMethod: method,
	}
}

func quantityRange(q NormalizedQuantity, exact bool, reason string) *QuantityRange {
	return &QuantityRange{Expected: q, IsExact: exact, Reason: reason}
}

func drainedOrNetQuantity(match []string, method string, packCount *int) (NormalizedQuantity, bool) {
	if len(match) < 3 {
		return NormalizedQuantity{}, false
	}
	qty, ok := parsePackageNumber(match[1])
	if !ok {
		return NormalizedQuantity{}, false
	}
	if packCount != nil && *packCount > 1 {
		qty *= float64(*packCount)
	}
	q := normalizedQuantity(match[0], qty, normalizeUnit(match[2]), 0.9, method)
	return q, q.BaseUnit != ""
}

func fixedPackageConfidence(ev PackageEvidence, base float64) float64 {
	if ev.VariableWeight || ev.ApproximateWeight {
		return math.Min(base, 0.65)
	}
	return base
}

func quantityRangeReason(ev PackageEvidence) string {
	switch {
	case ev.VariableWeight && ev.ApproximateWeight:
		return "Product is sold by variable approximate weight."
	case ev.VariableWeight:
		return "Product appears to be sold by variable weight."
	case ev.ApproximateWeight:
		return "Product package weight is approximate."
	default:
		return ""
	}
}

func normalizeLedgerQuantityText(raw string) string {
	text := strings.ToLower(strings.TrimSpace(raw))
	replacer := strings.NewReplacer(
		"\u00a0", " ",
		"×", "x",
		",", ".",
		"á", "a",
		"é", "e",
		"í", "i",
		"ó", "o",
		"ú", "u",
		"ü", "u",
		"ñ", "n",
	)
	text = replacer.Replace(text)
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

func containsVariableWeightText(text string) bool {
	return strings.Contains(text, "al peso") ||
		strings.Contains(text, "granel") ||
		strings.Contains(text, "€/kg") ||
		strings.Contains(text, "eur/kg") ||
		strings.Contains(text, "/kg") ||
		strings.Contains(text, "precio kg") ||
		strings.Contains(text, "precio por kg")
}

func containsApproximateWeightText(text string) bool {
	return strings.Contains(text, "aprox") ||
		strings.Contains(text, "peso aproximado") ||
		strings.Contains(text, "bandeja aprox")
}

func variableSalesUnit(text string) string {
	switch {
	case strings.Contains(text, "kg") || strings.Contains(text, "/kg"):
		return "kg"
	case strings.Contains(text, "g"):
		return "g"
	default:
		return "variable"
	}
}

func packageSalesUnit(baseUnit string) string {
	switch baseUnit {
	case "g":
		return "pack"
	case "ml":
		return "pack"
	case "unit":
		return "unit"
	default:
		return "unknown"
	}
}

func offerEvidence(offers []string, packageCount int) []OfferEvidence {
	var out []OfferEvidence
	for _, offer := range offers {
		text := strings.TrimSpace(offer)
		if text == "" {
			continue
		}
		norm := normalizeKey(text)
		ev := OfferEvidence{Text: text, Status: "unclear_terms", Confidence: 0.35}
		switch {
		case containsAnyKey(norm, []string{"club", "tarjeta", "fidelidad", "mi alcampo"}):
			ev.Status = "requires_loyalty"
			ev.Warning = "Offer may require loyalty eligibility; savings were not guaranteed."
		case strings.Contains(norm, "3x2") || strings.Contains(norm, "3 x 2"):
			if packageCount >= 3 {
				ev.Status = "applied_confirmed"
				ev.Confidence = 0.75
			} else {
				ev.Status = "quantity_threshold_not_met"
				ev.Confidence = 0.75
			}
		case strings.Contains(norm, "segunda unidad") || strings.Contains(norm, "2a unidad") || strings.Contains(norm, "2 unidad"):
			if packageCount >= 2 {
				ev.Status = "applied_confirmed"
				ev.Confidence = 0.65
			} else {
				ev.Status = "available_not_applied"
				ev.Confidence = 0.65
			}
		case strings.Contains(norm, "oferta") || strings.Contains(norm, "descuento"):
			ev.Status = "available_not_applied"
			ev.Confidence = 0.5
		}
		out = append(out, ev)
	}
	return out
}

func detailConfidence(product ProductSummary) float64 {
	score := 0.45
	if product.URL != "" {
		score += 0.1
	}
	if product.Allergens != "" || product.Nutrition != "" {
		score += 0.15
	}
	if product.PackageQuantity > 0 && product.PackageUnit != "" {
		score += 0.15
	}
	if product.ImageURL != "" {
		score += 0.05
	}
	return math.Min(score, 0.95)
}

func nutritionConfidence(nutrition *NutritionEvidence) float64 {
	if nutrition == nil {
		return 0
	}
	return nutrition.Confidence
}

func averagePositive(values ...float64) float64 {
	var total float64
	var count float64
	for _, value := range values {
		if value > 0 {
			total += value
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return total / count
}

func confidenceLabel(confidence float64) string {
	switch {
	case confidence >= 0.8:
		return "high"
	case confidence >= 0.55:
		return "medium"
	case confidence > 0:
		return "low"
	default:
		return "unknown"
	}
}

func productEvidenceKey(ev ProductEvidence) string {
	return strutil.FirstNonEmpty(ev.ProductID, ev.SKU, normalizeKey(ev.Name))
}

func productEvidenceKeyFromProduct(product ProductSummary) string {
	return strutil.FirstNonEmpty(product.ID, product.SKU, normalizeKey(product.Name))
}

func matchConfidenceForSelection(selected SelectedProduct) float64 {
	if strings.Contains(selected.SelectionReason, "strong ingredient match") {
		return 0.9
	}
	if strings.Contains(selected.SelectionReason, "partial ingredient match") {
		return 0.65
	}
	if selected.SelectionReason != "" {
		return 0.55
	}
	return 0.45
}

func hasUnclearOffer(offers []OfferEvidence) bool {
	for _, offer := range offers {
		if offer.Status == "unclear_terms" || offer.Status == "requires_loyalty" {
			return true
		}
	}
	return false
}

func sortedAggregateKeys(aggregates map[string]*productAggregate) []string {
	keys := make([]string, 0, len(aggregates))
	for key := range aggregates {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func priceLineReason(agg *productAggregate) string {
	if agg == nil {
		return ""
	}
	if agg.VariableOrEstimated {
		return "Estimated because package weight is approximate or variable."
	}
	return "Fixed package count from quantity ledger."
}

func totalRangeReason(variableCents int64) string {
	if variableCents > 0 {
		return "Total includes variable-weight or approximate package estimates."
	}
	return "All priced lines use fixed package quantities."
}

func prefixWarnings(prefix string, warnings []string) []string {
	var out []string
	for _, warning := range warnings {
		if warning != "" {
			out = append(out, prefix+": "+warning)
		}
	}
	return out
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func compactStrings(values ...string) []string {
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func formatIngredientQuantity(qty float64, unit string) string {
	if qty <= 0 && strings.TrimSpace(unit) == "" {
		return ""
	}
	return strings.TrimSpace(formatFloat(qty) + " " + unit)
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(roundQty(value), 'f', -1, 64)
}
