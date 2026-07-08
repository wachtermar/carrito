package food

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"strings"

	"github.com/wachtermar/carrito/internal/money"
)

type BudgetDealStatus string

const (
	BudgetDealNotRun           BudgetDealStatus = "not_run"
	BudgetDealPass             BudgetDealStatus = "pass"
	BudgetDealPassWithWarnings BudgetDealStatus = "pass_with_warnings"
	BudgetDealOverBudget       BudgetDealStatus = "over_budget"
	BudgetDealBlocked          BudgetDealStatus = "blocked"
)

const (
	BudgetStatusNotSet       = "not_set"
	BudgetStatusWithinBudget = "within_budget"
	BudgetStatusOverBudget   = "over_budget"
	BudgetStatusUnparseable  = "unparseable"
	BudgetStatusUnknownTotal = "unknown_total"
)

type BudgetDealReport struct {
	SchemaVersion               string                         `json:"schema_version"`
	Status                      BudgetDealStatus               `json:"status"`
	GeneratedAt                 string                         `json:"generated_at,omitempty"`
	MealPlanFingerprint         string                         `json:"mealplan_fingerprint,omitempty"`
	ProductSelectionFingerprint string                         `json:"product_selection_fingerprint,omitempty"`
	ServingPlanFingerprint      string                         `json:"serving_plan_fingerprint,omitempty"`
	ScaledMealPlanFingerprint   string                         `json:"scaled_mealplan_fingerprint,omitempty"`
	PantryResolutionFingerprint string                         `json:"pantry_resolution_fingerprint,omitempty"`
	ShopRequirementsFingerprint string                         `json:"shop_requirements_fingerprint,omitempty"`
	BudgetDealFingerprint       string                         `json:"budget_deal_fingerprint,omitempty"`
	BudgetRaw                   string                         `json:"budget_raw,omitempty"`
	Budget                      *money.Money                   `json:"budget,omitempty"`
	BudgetStatus                string                         `json:"budget_status"`
	EstimatedTotal              money.Money                    `json:"estimated_total,omitempty"`
	PurchasedSubtotal           money.Money                    `json:"purchased_subtotal,omitempty"`
	ConsumedCostEstimate        money.Money                    `json:"consumed_cost_estimate,omitempty"`
	PackageExcessCostEstimate   money.Money                    `json:"package_excess_cost_estimate,omitempty"`
	CostBasis                   string                         `json:"cost_basis,omitempty"`
	Delta                       *money.Money                   `json:"delta,omitempty"`
	DeltaPercent                float64                        `json:"delta_percent,omitempty"`
	SafeToReportBudget          bool                           `json:"safe_to_report_budget"`
	SafeToReportDeals           bool                           `json:"safe_to_report_deals"`
	Summary                     BudgetDealSummary              `json:"summary"`
	OfferEvidence               []BudgetDealOfferEvidence      `json:"offer_evidence,omitempty"`
	Optimization                *BudgetDealOptimizationSummary `json:"optimization,omitempty"`
	Warnings                    []BudgetDealIssue              `json:"warnings,omitempty"`
	BlockingIssues              []BudgetDealIssue              `json:"blocking_issues,omitempty"`
	Provenance                  []DataProvenance               `json:"provenance,omitempty"`
}

type BudgetDealSummary struct {
	SelectedProductCount          int     `json:"selected_product_count"`
	SelectedProductsWithPrice     int     `json:"selected_products_with_price"`
	SelectedProductsMissingPrice  int     `json:"selected_products_missing_price"`
	EstimatedVariableWeightLines  int     `json:"estimated_variable_weight_lines,omitempty"`
	ProductsWithOffers            int     `json:"products_with_offers"`
	OfferCount                    int     `json:"offer_count"`
	AppliedOfferCount             int     `json:"applied_offer_count"`
	AvailableOfferCount           int     `json:"available_offer_count"`
	UnclearOfferCount             int     `json:"unclear_offer_count"`
	LoyaltyOfferCount             int     `json:"loyalty_offer_count"`
	OptimizationChangedLines      int     `json:"optimization_changed_lines,omitempty"`
	OptimizationOfferSavingsCents int     `json:"optimization_offer_savings_cents,omitempty"`
	RecognizedDealSavingsCents    int     `json:"recognized_deal_savings_cents,omitempty"`
	ConsumedCostCents             int     `json:"consumed_cost_cents,omitempty"`
	PackageExcessCostCents        int     `json:"package_excess_cost_cents,omitempty"`
	PriceCoverageRatio            float64 `json:"price_coverage_ratio,omitempty"`
}

type BudgetDealOfferEvidence struct {
	Source       string      `json:"source,omitempty"`
	ProductID    string      `json:"product_id,omitempty"`
	SKU          string      `json:"sku,omitempty"`
	ProductName  string      `json:"product_name,omitempty"`
	Ingredient   string      `json:"ingredient,omitempty"`
	Text         string      `json:"text,omitempty"`
	Status       string      `json:"status,omitempty"`
	Confidence   float64     `json:"confidence,omitempty"`
	PackageCount int         `json:"package_count,omitempty"`
	Savings      money.Money `json:"savings,omitempty"`
	Warning      string      `json:"warning,omitempty"`
}

type BudgetDealOptimizationSummary struct {
	Status                     BasketOptimizationStatus `json:"status"`
	Objective                  string                   `json:"objective,omitempty"`
	DealAware                  bool                     `json:"deal_aware"`
	BaselineEffectiveSubtotal  money.Money              `json:"baseline_effective_subtotal,omitempty"`
	OptimizedEffectiveSubtotal money.Money              `json:"optimized_effective_subtotal,omitempty"`
	SubtotalDelta              money.Money              `json:"subtotal_delta,omitempty"`
	ChangedLines               int                      `json:"changed_lines"`
	OfferSavings               money.Money              `json:"offer_savings,omitempty"`
	FinalReadinessStatus       string                   `json:"final_readiness_status,omitempty"`
	FinalSafeToBuild           bool                     `json:"final_safe_to_build"`
}

type BudgetDealIssue struct {
	Code        string `json:"code,omitempty"`
	Severity    string `json:"severity,omitempty"`
	ProductID   string `json:"product_id,omitempty"`
	ProductName string `json:"product_name,omitempty"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

func BuildBudgetDealReport(run FoodRunArtifact) BudgetDealReport {
	report := BudgetDealReport{
		SchemaVersion:               "1",
		Status:                      BudgetDealPass,
		GeneratedAt:                 nowStamp(),
		MealPlanFingerprint:         firstNonEmptyString(run.MealPlanFingerprint, MealPlanFingerprint(run.MealPlan)),
		ProductSelectionFingerprint: firstNonEmptyString(run.ProductSelectionFingerprint, ProductSelectionFingerprint(run.Shop)),
		ServingPlanFingerprint:      servingPlanFingerprintFromRun(&run),
		ScaledMealPlanFingerprint:   scaledMealPlanFingerprintFromRun(&run),
		PantryResolutionFingerprint: pantryResolutionFingerprintFromRun(&run),
		ShopRequirementsFingerprint: shopRequirementsFingerprintFromRun(&run),
		BudgetRaw:                   strings.TrimSpace(run.MealPlan.BudgetEUR),
		BudgetStatus:                BudgetStatusNotSet,
		EstimatedTotal:              normalizeReportMoney(run.Shop.EstimatedTotal),
		SafeToReportDeals:           true,
		Provenance: []DataProvenance{
			{
				Scope:      "budget",
				Source:     "food_run_shop_total",
				Confidence: "medium",
				Message:    "Budget comparison uses the final estimated Alcampo shopping total before cart confirmation.",
			},
			{
				Scope:      "deal",
				Source:     "quantity_ledger_and_basket_optimization",
				Confidence: "medium",
				Message:    "Deal evidence is derived from selected-product offer text and conservative parsed basket optimization decisions.",
			},
		},
	}
	report.Summary = budgetDealSummaryFromShop(run.Shop)
	applyBudgetCostBasis(&report, run)
	applyBudgetComparison(&report)
	report.OfferEvidence = budgetDealOfferEvidence(run)
	summarizeBudgetDealOffers(&report)
	if run.BasketOptimizationPlan != nil {
		report.Optimization = budgetDealOptimizationSummary(*run.BasketOptimizationPlan)
		if report.Optimization != nil {
			report.Summary.OptimizationChangedLines = report.Optimization.ChangedLines
			report.Summary.OptimizationOfferSavingsCents = int(report.Optimization.OfferSavings.Cents)
			report.Summary.RecognizedDealSavingsCents += int(report.Optimization.OfferSavings.Cents)
			if run.BasketOptimizationPlan.Policy.DealAware && report.Optimization.ChangedLines == 0 && report.Summary.OfferCount > 0 {
				report.addWarning("deal_aware_no_switch", "", "Deal-aware optimization was enabled, but no offer-backed product switch was applied.", "Review basket_optimization_plan decisions before claiming offer savings.")
			}
			if run.BasketOptimizationPlan.Status == BasketOptimizationFailed {
				report.addWarning("basket_optimization_failed", "", "Basket optimization failed validation or would have regressed readiness.", "Do not claim optimization savings; keep the baseline selected products.")
			}
		}
	}
	if report.Summary.SelectedProductsMissingPrice > 0 {
		report.addWarning("selected_price_missing", "", "One or more selected products are missing price evidence.", "Refresh Alcampo product data before presenting the budget as exact.")
	}
	if report.Summary.EstimatedVariableWeightLines > 0 {
		report.addWarning("variable_weight_budget_estimate", "", "Budget includes estimated variable-weight product lines.", "Confirm Alcampo final weights before presenting the total as exact.")
	}
	finalizeBudgetDealReport(&report)
	report.BudgetDealFingerprint = BudgetDealFingerprint(report)
	return report
}

func BudgetDealFingerprint(report BudgetDealReport) string {
	clone := report
	clone.GeneratedAt = ""
	clone.BudgetDealFingerprint = ""
	data, err := json.Marshal(clone)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func budgetDealSummaryFromShop(shop ShopResult) BudgetDealSummary {
	summary := BudgetDealSummary{SelectedProductCount: len(shop.SelectedProducts)}
	for _, selected := range shop.SelectedProducts {
		if selected.Error != "" {
			continue
		}
		if selected.LineTotal.Cents > 0 || selected.Product.Price.Cents > 0 {
			summary.SelectedProductsWithPrice++
		} else if selected.PackageCount > 0 {
			summary.SelectedProductsMissingPrice++
		}
	}
	if summary.SelectedProductCount > 0 {
		summary.PriceCoverageRatio = math.Round((float64(summary.SelectedProductsWithPrice)/float64(summary.SelectedProductCount))*10000) / 10000
	}
	return summary
}

func applyBudgetCostBasis(report *BudgetDealReport, run FoodRunArtifact) {
	if report == nil {
		return
	}
	report.PurchasedSubtotal = report.EstimatedTotal
	report.CostBasis = "purchased_subtotal"
	if run.QuantityLedger != nil {
		report.Summary.EstimatedVariableWeightLines = run.QuantityLedger.Summary.EstimatedVariableWeightLines
	}
	consumed, excess, ok := estimateConsumedAndExcessCost(run)
	if !ok {
		return
	}
	report.ConsumedCostEstimate = normalizeReportMoney(money.Money{Amount: money.FormatAmount(consumed), Currency: "EUR", Cents: consumed})
	report.PackageExcessCostEstimate = normalizeReportMoney(money.Money{Amount: money.FormatAmount(excess), Currency: "EUR", Cents: excess})
	report.Summary.ConsumedCostCents = int(consumed)
	report.Summary.PackageExcessCostCents = int(excess)
	report.CostBasis = "purchased_subtotal_with_consumed_estimate"
}

func estimateConsumedAndExcessCost(run FoodRunArtifact) (int64, int64, bool) {
	if run.QuantityLedger == nil {
		return 0, 0, false
	}
	type aggregate struct {
		requiredBase  float64
		purchasedBase float64
		baseUnit      string
		unitMismatch  bool
	}
	aggregates := map[string]*aggregate{}
	for _, allocation := range run.QuantityLedger.Allocations {
		key := firstNonEmptyString(allocation.ProductID, allocation.SKU, normalizeKey(allocation.ProductName))
		if key == "" || allocation.MatchType == "missing" {
			continue
		}
		req := allocation.ShopRequiredQuantity
		if req == nil {
			req = allocation.RequiredQuantity
		}
		purchased := allocation.PurchasedQuantity
		if req == nil || purchased == nil || req.BaseValue <= 0 || purchased.Expected.BaseValue <= 0 {
			continue
		}
		agg := aggregates[key]
		if agg == nil {
			agg = &aggregate{baseUnit: req.BaseUnit}
			aggregates[key] = agg
		}
		if agg.baseUnit != req.BaseUnit || agg.baseUnit != purchased.Expected.BaseUnit {
			agg.unitMismatch = true
			continue
		}
		agg.requiredBase += req.BaseValue
		if purchased.Expected.BaseValue > agg.purchasedBase {
			agg.purchasedBase = purchased.Expected.BaseValue
		}
	}
	if len(aggregates) == 0 {
		return 0, 0, false
	}
	var consumed int64
	var pricedLines int
	for _, selected := range run.Shop.SelectedProducts {
		key := productEvidenceKeyFromProduct(selected.Product)
		if key == "" {
			key = firstNonEmptyString(selected.Product.ID, selected.Product.SKU, normalizeKey(selected.Product.Name))
		}
		agg := aggregates[key]
		if agg == nil || agg.unitMismatch || agg.requiredBase <= 0 || agg.purchasedBase <= 0 || selected.LineTotal.Cents <= 0 {
			continue
		}
		ratio := math.Min(1, agg.requiredBase/agg.purchasedBase)
		consumed += int64(math.Round(float64(selected.LineTotal.Cents) * ratio))
		pricedLines++
	}
	if pricedLines == 0 {
		return 0, 0, false
	}
	excess := run.Shop.EstimatedTotal.Cents - consumed
	if excess < 0 {
		excess = 0
	}
	return consumed, excess, true
}

func applyBudgetComparison(report *BudgetDealReport) {
	if report == nil {
		return
	}
	if report.BudgetRaw == "" {
		report.BudgetStatus = BudgetStatusNotSet
		return
	}
	budgetCents, err := money.ParseCents(report.BudgetRaw)
	if err != nil || budgetCents <= 0 {
		report.BudgetStatus = BudgetStatusUnparseable
		report.SafeToReportBudget = false
		report.addWarning("budget_unparseable", "", "Budget target could not be parsed.", "Use a plain EUR amount such as 80 or 80.00.")
		return
	}
	budget := normalizeReportMoney(money.Money{Amount: money.FormatAmount(budgetCents), Currency: "EUR", Cents: budgetCents})
	report.Budget = &budget
	if report.EstimatedTotal.Cents <= 0 && report.Summary.SelectedProductCount > 0 {
		report.BudgetStatus = BudgetStatusUnknownTotal
		report.SafeToReportBudget = false
		report.addWarning("estimated_total_missing", "", "Budget target is set, but the final estimated shopping total is missing.", "Refresh shopping selection before presenting budget readiness.")
		return
	}
	deltaCents := report.EstimatedTotal.Cents - budgetCents
	delta := normalizeReportMoney(money.Money{Amount: money.FormatAmount(deltaCents), Currency: "EUR", Cents: deltaCents})
	report.Delta = &delta
	if budgetCents > 0 {
		report.DeltaPercent = math.Round((float64(deltaCents)/float64(budgetCents))*10000) / 100
	}
	if deltaCents > 0 {
		report.BudgetStatus = BudgetStatusOverBudget
		report.SafeToReportBudget = false
		report.addWarning("over_budget", "", "Estimated shopping total exceeds the budget target.", "Reduce recipe scope, switch policy/objective, use pantry coverage, or approve the over-budget plan explicitly.")
		return
	}
	report.BudgetStatus = BudgetStatusWithinBudget
	report.SafeToReportBudget = true
}

func budgetDealOfferEvidence(run FoodRunArtifact) []BudgetDealOfferEvidence {
	ingredientByProduct := map[string]string{}
	packageCountByProduct := map[string]int{}
	for _, selected := range run.Shop.SelectedProducts {
		key := productEvidenceKeyFromProduct(selected.Product)
		if key == "" {
			continue
		}
		ingredientByProduct[key] = selected.Ingredient.Name
		if selected.PackageCount > packageCountByProduct[key] {
			packageCountByProduct[key] = selected.PackageCount
		}
	}
	var out []BudgetDealOfferEvidence
	seen := map[string]bool{}
	if run.QuantityLedger != nil {
		for _, product := range run.QuantityLedger.ProductEvidence {
			key := productEvidenceKey(product)
			for _, offer := range product.Offers {
				row := BudgetDealOfferEvidence{
					Source:       "quantity_ledger",
					ProductID:    product.ProductID,
					SKU:          product.SKU,
					ProductName:  product.Name,
					Ingredient:   ingredientByProduct[key],
					Text:         offer.Text,
					Status:       offer.Status,
					Confidence:   offer.Confidence,
					PackageCount: packageCountByProduct[key],
					Savings:      normalizeReportMoney(offer.Savings),
					Warning:      offer.Warning,
				}
				addBudgetDealOffer(&out, seen, row)
			}
		}
	}
	for _, selected := range run.Shop.SelectedProducts {
		if len(selected.Product.Offers) == 0 {
			continue
		}
		key := productEvidenceKeyFromProduct(selected.Product)
		for _, offer := range offerEvidence(selected.Product.Offers, selected.PackageCount) {
			row := BudgetDealOfferEvidence{
				Source:       "selected_product",
				ProductID:    selected.Product.ID,
				SKU:          selected.Product.SKU,
				ProductName:  selected.Product.Name,
				Ingredient:   selected.Ingredient.Name,
				Text:         offer.Text,
				Status:       offer.Status,
				Confidence:   offer.Confidence,
				PackageCount: selected.PackageCount,
				Savings:      normalizeReportMoney(offer.Savings),
				Warning:      offer.Warning,
			}
			if key == "" {
				key = normalizeKey(selected.Product.Name)
			}
			addBudgetDealOffer(&out, seen, row)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a := strings.Join([]string{out[i].ProductName, out[i].SKU, out[i].Text, out[i].Source}, "|")
		b := strings.Join([]string{out[j].ProductName, out[j].SKU, out[j].Text, out[j].Source}, "|")
		return a < b
	})
	return out
}

func addBudgetDealOffer(out *[]BudgetDealOfferEvidence, seen map[string]bool, row BudgetDealOfferEvidence) {
	if strings.TrimSpace(row.Text) == "" {
		return
	}
	key := strings.Join([]string{row.ProductID, row.SKU, normalizeKey(row.ProductName), normalizeKey(row.Text)}, "|")
	if seen[key] {
		return
	}
	seen[key] = true
	*out = append(*out, row)
}

func summarizeBudgetDealOffers(report *BudgetDealReport) {
	if report == nil {
		return
	}
	productWithOffer := map[string]bool{}
	for _, offer := range report.OfferEvidence {
		report.Summary.OfferCount++
		key := firstNonEmptyString(offer.ProductID, offer.SKU, normalizeKey(offer.ProductName))
		if key != "" {
			productWithOffer[key] = true
		}
		if offer.Savings.Cents > 0 {
			report.Summary.RecognizedDealSavingsCents += int(offer.Savings.Cents)
		}
		switch offer.Status {
		case "applied_confirmed":
			report.Summary.AppliedOfferCount++
		case "available_not_applied", "quantity_threshold_not_met":
			report.Summary.AvailableOfferCount++
		case "requires_loyalty":
			report.Summary.LoyaltyOfferCount++
			report.addWarning("loyalty_offer_terms", firstNonEmptyString(offer.ProductID, offer.SKU), "Offer may require loyalty eligibility and was not treated as guaranteed savings.", "Confirm the offer in Alcampo before relying on savings.")
		case "unclear_terms":
			report.Summary.UnclearOfferCount++
			report.addWarning("unclear_offer_terms", firstNonEmptyString(offer.ProductID, offer.SKU), "Offer terms are unclear and were not treated as guaranteed savings.", "Confirm offer terms in Alcampo before relying on savings.")
		}
	}
	report.Summary.ProductsWithOffers = len(productWithOffer)
}

func budgetDealOptimizationSummary(plan BasketOptimizationPlan) *BudgetDealOptimizationSummary {
	summary := &BudgetDealOptimizationSummary{
		Status:                    plan.Status,
		Objective:                 strings.ReplaceAll(plan.Policy.Objective, "_", "-"),
		DealAware:                 plan.Policy.DealAware,
		FinalReadinessStatus:      plan.Validation.FinalReadinessStatus,
		FinalSafeToBuild:          plan.Validation.FinalSafeToBuild,
		BaselineEffectiveSubtotal: normalizeReportMoney(money.Money{Amount: money.FormatAmount(int64(plan.BaselineSummary.EffectiveSubtotalCents)), Currency: "EUR", Cents: int64(plan.BaselineSummary.EffectiveSubtotalCents)}),
	}
	if plan.OptimizedSummary != nil {
		summary.OptimizedEffectiveSubtotal = normalizeReportMoney(money.Money{Amount: money.FormatAmount(int64(plan.OptimizedSummary.EffectiveSubtotalCents)), Currency: "EUR", Cents: int64(plan.OptimizedSummary.EffectiveSubtotalCents)})
		delta := plan.OptimizedSummary.EffectiveSubtotalCents - plan.BaselineSummary.EffectiveSubtotalCents
		summary.SubtotalDelta = normalizeReportMoney(money.Money{Amount: money.FormatAmount(int64(delta)), Currency: "EUR", Cents: int64(delta)})
		summary.OfferSavings = normalizeReportMoney(money.Money{Amount: money.FormatAmount(int64(plan.OptimizedSummary.OfferSavingsCents)), Currency: "EUR", Cents: int64(plan.OptimizedSummary.OfferSavingsCents)})
	}
	for _, decision := range plan.Decisions {
		if decision.Changed {
			summary.ChangedLines++
		}
		if decision.OfferSavingsCents > int(summary.OfferSavings.Cents) {
			summary.OfferSavings = normalizeReportMoney(money.Money{Amount: money.FormatAmount(int64(decision.OfferSavingsCents)), Currency: "EUR", Cents: int64(decision.OfferSavingsCents)})
		}
	}
	return summary
}

func finalizeBudgetDealReport(report *BudgetDealReport) {
	if report == nil {
		return
	}
	if len(report.BlockingIssues) > 0 {
		report.Status = BudgetDealBlocked
		report.SafeToReportBudget = false
		report.SafeToReportDeals = false
		return
	}
	if report.BudgetStatus == BudgetStatusOverBudget {
		report.Status = BudgetDealOverBudget
		return
	}
	if len(report.Warnings) > 0 {
		report.Status = BudgetDealPassWithWarnings
		return
	}
	report.Status = BudgetDealPass
}

func (report *BudgetDealReport) addWarning(code, productID, message, remediation string) {
	if report == nil || strings.TrimSpace(message) == "" {
		return
	}
	issue := BudgetDealIssue{Code: code, Severity: "warning", ProductID: productID, Message: message, Remediation: remediation}
	for _, existing := range report.Warnings {
		if existing.Code == issue.Code && existing.ProductID == issue.ProductID && existing.Message == issue.Message {
			return
		}
	}
	report.Warnings = append(report.Warnings, issue)
}

func normalizeReportMoney(value money.Money) money.Money {
	if value.Currency == "" && value.Cents != 0 {
		value.Currency = "EUR"
	}
	if value.Amount == "" && value.Cents != 0 {
		value.Amount = money.FormatAmount(value.Cents)
	}
	return value
}
