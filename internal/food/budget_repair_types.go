package food

type BudgetRepairStatus string

const (
	BudgetRepairNotRun              BudgetRepairStatus = "not_run"
	BudgetRepairNotNeeded           BudgetRepairStatus = "not_needed"
	BudgetRepairDisabled            BudgetRepairStatus = "disabled"
	BudgetRepairSkippedUnrepairable BudgetRepairStatus = "skipped_unrepairable"
	BudgetRepairAttemptedApplied    BudgetRepairStatus = "attempted_applied"
	BudgetRepairAttemptedFailed     BudgetRepairStatus = "attempted_failed"
	BudgetRepairDiscardedRegression BudgetRepairStatus = "discarded_regression"
)

type BudgetRepairPolicy struct {
	Enabled                               bool `json:"enabled"`
	AllowProductSwitches                  bool `json:"allow_product_switches"`
	AllowBudgetRecipeSwap                 bool `json:"allow_budget_recipe_swap"`
	MaxProductSwitches                    int  `json:"max_product_switches"`
	MaxRecipeSwaps                        int  `json:"max_recipe_swaps"`
	MaxCandidatesPerDriver                int  `json:"max_candidates_per_driver"`
	MaxTotalChecks                        int  `json:"max_total_checks"`
	MinSavingsCentsToApply                int  `json:"min_savings_cents_to_apply"`
	ProtectPinnedRecipes                  bool `json:"protect_pinned_recipes"`
	RequireNoReadinessRegression          bool `json:"require_no_readiness_regression"`
	RequireNoNutritionReadinessRegression bool `json:"require_no_nutrition_readiness_regression"`
	RequireNoRecipeQualityRegression      bool `json:"require_no_recipe_quality_regression"`
	PreserveDietaryRules                  bool `json:"preserve_dietary_rules"`
	PreserveAllergenRules                 bool `json:"preserve_allergen_rules"`
	DealAware                             bool `json:"deal_aware"`
}

type BudgetRepairTrigger struct {
	Reason                      string `json:"reason"`
	BudgetTotalCents            *int   `json:"budget_total_cents,omitempty"`
	BaselineComparedAmountCents *int   `json:"baseline_compared_amount_cents,omitempty"`
	BaselineDifferenceCents     *int   `json:"baseline_difference_cents,omitempty"`
	Source                      string `json:"source"`
}

type BudgetRepairSummary struct {
	BudgetStatus                 string  `json:"budget_status"`
	SafeToReportBudget           bool    `json:"safe_to_report_budget"`
	SafeToMeetBudget             bool    `json:"safe_to_meet_budget"`
	EstimatedBasketSubtotalCents *int    `json:"estimated_basket_subtotal_cents,omitempty"`
	KnownConsumedCostCents       *int    `json:"known_consumed_cost_cents,omitempty"`
	PackageExcessCostCents       *int    `json:"package_excess_cost_cents,omitempty"`
	ReliableOfferSavingsCents    *int    `json:"reliable_offer_savings_cents,omitempty"`
	PriceCoverageRatio           float64 `json:"price_coverage_ratio,omitempty"`
	MissingPriceLines            int     `json:"missing_price_lines"`
	ProductLineCount             int     `json:"product_line_count"`
	RecipeCount                  int     `json:"recipe_count"`
}

type BudgetCostDriver struct {
	ID                     string   `json:"id"`
	Type                   string   `json:"type"`
	Rank                   int      `json:"rank"`
	ProductID              string   `json:"product_id,omitempty"`
	ProductName            string   `json:"product_name,omitempty"`
	IngredientKey          string   `json:"ingredient_key,omitempty"`
	IngredientName         string   `json:"ingredient_name,omitempty"`
	Day                    int      `json:"day,omitempty"`
	MealSlot               string   `json:"meal_slot,omitempty"`
	RecipeID               string   `json:"recipe_id,omitempty"`
	RecipeTitle            string   `json:"recipe_title,omitempty"`
	EstimatedCostCents     *int     `json:"estimated_cost_cents,omitempty"`
	PackageExcessCostCents *int     `json:"package_excess_cost_cents,omitempty"`
	PercentOfKnownSubtotal *float64 `json:"percent_of_known_subtotal,omitempty"`
	Reason                 string   `json:"reason"`
}

type BudgetRepairAttempt struct {
	ID                string                 `json:"id"`
	Type              string                 `json:"type"`
	Status            string                 `json:"status"`
	CostDriverID      string                 `json:"cost_driver_id,omitempty"`
	CandidateCount    int                    `json:"candidate_count"`
	AppliedDecisionID string                 `json:"applied_decision_id,omitempty"`
	Reason            string                 `json:"reason,omitempty"`
	Validation        BudgetRepairValidation `json:"validation"`
}

type BudgetRepairDecision struct {
	ID                    string `json:"id"`
	Type                  string `json:"type"`
	CostDriverID          string `json:"cost_driver_id,omitempty"`
	BeforeLabel           string `json:"before_label"`
	AfterLabel            string `json:"after_label"`
	EstimatedSavingsCents *int   `json:"estimated_savings_cents,omitempty"`
	BudgetStatusBefore    string `json:"budget_status_before"`
	BudgetStatusAfter     string `json:"budget_status_after"`
	ReadinessImpact       string `json:"readiness_impact"`
	Explanation           string `json:"explanation"`
}

type BudgetRepairRejectedCandidate struct {
	ID           string   `json:"id,omitempty"`
	Type         string   `json:"type,omitempty"`
	CostDriverID string   `json:"cost_driver_id,omitempty"`
	ProductID    string   `json:"product_id,omitempty"`
	ProductName  string   `json:"product_name,omitempty"`
	RejectCodes  []string `json:"reject_codes,omitempty"`
	RejectReason string   `json:"reject_reason,omitempty"`
}

type BudgetRepairValidation struct {
	SafeToBuild           bool   `json:"safe_to_build"`
	SafeToCook            bool   `json:"safe_to_cook"`
	SafeToUseRecipes      bool   `json:"safe_to_use_recipes"`
	SafeToReportNutrition bool   `json:"safe_to_report_nutrition"`
	SafeToReportBudget    bool   `json:"safe_to_report_budget"`
	SafeToMeetBudget      bool   `json:"safe_to_meet_budget"`
	BudgetStatus          string `json:"budget_status"`
	ReadinessStatus       string `json:"readiness_status"`
	AuditWouldPass        bool   `json:"audit_would_pass,omitempty"`
	RejectReason          string `json:"reject_reason,omitempty"`
}

type BudgetRepairIssue struct {
	Code        string `json:"code,omitempty"`
	Severity    string `json:"severity,omitempty"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

type BudgetRepairPlan struct {
	SchemaVersion            string                          `json:"schema_version"`
	Status                   BudgetRepairStatus              `json:"status"`
	GeneratedAt              string                          `json:"generated_at,omitempty"`
	Policy                   BudgetRepairPolicy              `json:"policy"`
	Trigger                  BudgetRepairTrigger             `json:"trigger"`
	MealPlanFingerprint      string                          `json:"mealplan_fingerprint,omitempty"`
	BaselineProductSelection string                          `json:"baseline_product_selection_fingerprint,omitempty"`
	FinalProductSelection    string                          `json:"final_product_selection_fingerprint,omitempty"`
	Baseline                 BudgetRepairSummary             `json:"baseline"`
	Final                    BudgetRepairSummary             `json:"final"`
	CostDrivers              []BudgetCostDriver              `json:"cost_drivers,omitempty"`
	Attempts                 []BudgetRepairAttempt           `json:"attempts,omitempty"`
	AppliedDecisions         []BudgetRepairDecision          `json:"applied_decisions,omitempty"`
	RejectedCandidates       []BudgetRepairRejectedCandidate `json:"rejected_candidates,omitempty"`
	BlockingIssues           []BudgetRepairIssue             `json:"blocking_issues,omitempty"`
	Warnings                 []BudgetRepairIssue             `json:"warnings,omitempty"`
	BudgetRepairFingerprint  string                          `json:"budget_repair_fingerprint,omitempty"`
}
