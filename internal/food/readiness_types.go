package food

type ReadinessStatus string

const (
	ReadinessReadyExact                 ReadinessStatus = "ready_exact"
	ReadinessReadyWithCaveats           ReadinessStatus = "ready_with_caveats"
	ReadinessReadyWithPantryAssumptions ReadinessStatus = "ready_with_pantry_assumptions"
	ReadinessBlocked                    ReadinessStatus = "blocked"
)

type ReadinessPolicy struct {
	StrictQuantity               bool `json:"strict_quantity"`
	RequireSafeBasket            bool `json:"require_safe_basket"`
	RequireCookReady             bool `json:"require_cook_ready"`
	AllowEstimatedVariableWeight bool `json:"allow_estimated_variable_weight"`
	AllowLowConfidenceBasket     bool `json:"allow_low_confidence_basket"`
	AllowRecipeSwap              bool `json:"allow_recipe_swap"`
	AllowIngredientSubstitution  bool `json:"allow_ingredient_substitution"`
	RequireNutritionReady        bool `json:"require_nutrition_ready"`
	StrictRecipeQuality          bool `json:"strict_recipe_quality"`
	RequireRecipeImages          bool `json:"require_recipe_images"`
	RequireBudgetReady           bool `json:"require_budget_ready"`
}

type ReadinessGate struct {
	SchemaVersion               string                    `json:"schema_version"`
	Status                      ReadinessStatus           `json:"status"`
	SafeToBuild                 bool                      `json:"safe_to_build"`
	Policy                      ReadinessPolicy           `json:"policy"`
	Checks                      []GateCheck               `json:"checks"`
	BlockingIssues              []GateIssue               `json:"blocking_issues,omitempty"`
	Warnings                    []GateIssue               `json:"warnings,omitempty"`
	MealPlanFingerprint         string                    `json:"mealplan_fingerprint,omitempty"`
	ServingPlanFingerprint      string                    `json:"serving_plan_fingerprint,omitempty"`
	ScaledMealPlanFingerprint   string                    `json:"scaled_mealplan_fingerprint,omitempty"`
	ProductSelectionFingerprint string                    `json:"product_selection_fingerprint,omitempty"`
	PantryResolutionFingerprint string                    `json:"pantry_resolution_fingerprint,omitempty"`
	ShopRequirementsFingerprint string                    `json:"shop_requirements_fingerprint,omitempty"`
	NutritionLedgerFingerprint  string                    `json:"nutrition_ledger_fingerprint,omitempty"`
	RecipeSetFingerprint        string                    `json:"recipe_set_fingerprint,omitempty"`
	RecipeQualityFingerprint    string                    `json:"recipe_quality_fingerprint,omitempty"`
	RecipeImageFingerprint      string                    `json:"recipe_image_fingerprint,omitempty"`
	BudgetDealFingerprint       string                    `json:"budget_deal_fingerprint,omitempty"`
	FinalLedgerStatus           string                    `json:"final_ledger_status,omitempty"`
	FinalBasketSafety           bool                      `json:"final_basket_safety"`
	SafeToCook                  bool                      `json:"safe_to_cook"`
	CookReadinessStatus         string                    `json:"cook_readiness_status,omitempty"`
	SafeToUseRecipes            bool                      `json:"safe_to_use_recipes"`
	RecipeQualityStatus         string                    `json:"recipe_quality_status,omitempty"`
	BudgetDealStatus            string                    `json:"budget_deal_status,omitempty"`
	BudgetStatus                string                    `json:"budget_status,omitempty"`
	SafeToReportBudget          bool                      `json:"safe_to_report_budget"`
	SafeToReportDeals           bool                      `json:"safe_to_report_deals"`
	NutritionStatus             string                    `json:"nutrition_status,omitempty"`
	SafeToReportNutrition       bool                      `json:"safe_to_report_nutrition"`
	NutritionCoverageSummary    *NutritionCoverageSummary `json:"nutrition_coverage_summary,omitempty"`
	ServingStatus               string                    `json:"serving_status,omitempty"`
	ScalingStatus               string                    `json:"scaling_status,omitempty"`
	PantryStatus                string                    `json:"pantry_status,omitempty"`
	RecoveryStatus              string                    `json:"recovery_status,omitempty"`
	RecipeSwapStatus            string                    `json:"recipe_swap_status,omitempty"`
	ExitCode                    int                       `json:"exit_code"`
	ExitReason                  string                    `json:"exit_reason"`
}

type GateCheck struct {
	Code     string `json:"code"`
	Passed   bool   `json:"passed"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type GateIssue struct {
	Code            string   `json:"code"`
	Severity        string   `json:"severity"`
	Phase           string   `json:"phase"`
	RecipeID        string   `json:"recipe_id,omitempty"`
	Day             int      `json:"day,omitempty"`
	MealSlot        string   `json:"meal_slot,omitempty"`
	IngredientKey   string   `json:"ingredient_key,omitempty"`
	IngredientName  string   `json:"ingredient_name,omitempty"`
	ProductID       string   `json:"product_id,omitempty"`
	ProductName     string   `json:"product_name,omitempty"`
	RecoveryIssueID string   `json:"recovery_issue_id,omitempty"`
	DecisionID      string   `json:"decision_id,omitempty"`
	Message         string   `json:"message"`
	Remediation     string   `json:"remediation,omitempty"`
	AllowedBy       []string `json:"allowed_by,omitempty"`
}

type RecipeAdjustment struct {
	ID                     string `json:"id"`
	Type                   string `json:"type"`
	Day                    int    `json:"day,omitempty"`
	MealSlot               string `json:"meal_slot,omitempty"`
	RecipeID               string `json:"recipe_id,omitempty"`
	OriginalRecipeID       string `json:"original_recipe_id,omitempty"`
	OriginalRecipeTitle    string `json:"original_recipe_title,omitempty"`
	ReplacementRecipeID    string `json:"replacement_recipe_id,omitempty"`
	ReplacementRecipeTitle string `json:"replacement_recipe_title,omitempty"`
	IngredientKey          string `json:"ingredient_key"`
	OriginalIngredientName string `json:"original_ingredient_name"`
	SelectedProductID      string `json:"selected_product_id,omitempty"`
	SelectedProductName    string `json:"selected_product_name,omitempty"`
	RecoveryDecisionID     string `json:"recovery_decision_id,omitempty"`
	RecipeSwapID           string `json:"recipe_swap_id,omitempty"`
	Instruction            string `json:"instruction"`
	AppliedToSteps         []int  `json:"applied_to_steps,omitempty"`
}
