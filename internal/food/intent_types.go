package food

type ConstraintSatisfactionStatus string

const (
	ConstraintSatisfactionNotRun          ConstraintSatisfactionStatus = "not_run"
	ConstraintSatisfactionPass            ConstraintSatisfactionStatus = "pass"
	ConstraintSatisfactionPassWithWarning ConstraintSatisfactionStatus = "pass_with_warnings"
	ConstraintSatisfactionFail            ConstraintSatisfactionStatus = "fail"
	ConstraintSatisfactionUnknown         ConstraintSatisfactionStatus = "unknown"
)

const (
	ConstraintCheckSatisfied     = "satisfied"
	ConstraintCheckNotSatisfied  = "not_satisfied"
	ConstraintCheckUnknown       = "unknown"
	ConstraintCheckNotApplicable = "not_applicable"
)

type MealRunIntent struct {
	SchemaVersion      string              `json:"schema_version"`
	IntentID           string              `json:"intent_id,omitempty"`
	Source             string              `json:"source,omitempty"`
	UserRequestSummary string              `json:"user_request_summary,omitempty"`
	IntentFingerprint  string              `json:"intent_fingerprint,omitempty"`
	PlanCoverage       *PlanCoverageIntent `json:"plan_coverage,omitempty"`
	Serving            *ServingIntent      `json:"serving,omitempty"`
	Budget             *BudgetIntent       `json:"budget,omitempty"`
	Nutrition          *NutritionIntent    `json:"nutrition,omitempty"`
	Diet               *DietIntent         `json:"diet,omitempty"`
	Cooking            *CookingIntent      `json:"cooking,omitempty"`
	RecipePDF          *RecipePDFIntent    `json:"recipe_pdf,omitempty"`
	Pantry             *PantryIntent       `json:"pantry,omitempty"`
	HardConstraints    []IntentConstraint  `json:"hard_constraints,omitempty"`
	SoftPreferences    []IntentPreference  `json:"soft_preferences,omitempty"`
	ClaimPolicy        IntentClaimPolicy   `json:"claim_policy"`
}

type PlanCoverageIntent struct {
	ExpectedDays      *int     `json:"expected_days,omitempty"`
	ExpectedMealSlots []string `json:"expected_meal_slots,omitempty"`
	ExpectedMealCount *int     `json:"expected_meal_count,omitempty"`
}

type ServingIntent struct {
	AdultServings     *float64 `json:"adult_servings,omitempty"`
	ChildServings     *float64 `json:"child_servings,omitempty"`
	ToddlerServings   *float64 `json:"toddler_servings,omitempty"`
	TotalServingUnits *float64 `json:"total_serving_units,omitempty"`
	Tolerance         float64  `json:"tolerance,omitempty"`
}

type BudgetIntent struct {
	MaxTotalCents        *int     `json:"max_total_cents,omitempty"`
	MaxPerServingCents   *int     `json:"max_per_serving_cents,omitempty"`
	MaxPerMealCents      *int     `json:"max_per_meal_cents,omitempty"`
	RequireBudgetReady   bool     `json:"require_budget_ready"`
	MinPriceLineCoverage *float64 `json:"min_price_line_coverage,omitempty"`
}

type NutritionIntent struct {
	RequireNutritionReady bool              `json:"require_nutrition_ready"`
	MinLineCoverage       *float64          `json:"min_line_coverage,omitempty"`
	MinQuantityCoverage   *float64          `json:"min_quantity_coverage,omitempty"`
	Targets               []NutritionTarget `json:"targets,omitempty"`
}

type NutritionTarget struct {
	Scope    string  `json:"scope"`
	Nutrient string  `json:"nutrient"`
	Operator string  `json:"operator"`
	Value    float64 `json:"value"`
	Unit     string  `json:"unit"`
}

type DietIntent struct {
	ExcludedIngredients       []string `json:"excluded_ingredients,omitempty"`
	ExcludedAllergens         []string `json:"excluded_allergens,omitempty"`
	DietRules                 []string `json:"diet_rules,omitempty"`
	RequireDeterministicCheck bool     `json:"require_deterministic_check"`
}

type CookingIntent struct {
	MaxTotalMinutes     *int `json:"max_total_minutes,omitempty"`
	MaxActiveMinutes    *int `json:"max_active_minutes,omitempty"`
	RequireTimeEvidence bool `json:"require_time_evidence"`
}

type RecipePDFIntent struct {
	RequireCookingSteps bool `json:"require_cooking_steps"`
	RequireRecipeSource bool `json:"require_recipe_source"`
	RequireRecipeImages bool `json:"require_recipe_images"`
	RequireCompletePDF  bool `json:"require_complete_pdf"`
}

type PantryIntent struct {
	AllowAssumedPantry     *bool `json:"allow_assumed_pantry,omitempty"`
	RequireConfirmedPantry bool  `json:"require_confirmed_pantry"`
	ShopAllIngredients     bool  `json:"shop_all_ingredients"`
}

type IntentConstraint struct {
	ID               string `json:"id"`
	Type             string `json:"type"`
	Scope            string `json:"scope"`
	Description      string `json:"description"`
	Required         bool   `json:"required"`
	EvidenceRequired bool   `json:"evidence_required"`
	Source           string `json:"source,omitempty"`
}

type IntentPreference struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	Scope       string  `json:"scope"`
	Description string  `json:"description"`
	Weight      float64 `json:"weight,omitempty"`
	Source      string  `json:"source,omitempty"`
}

type IntentClaimPolicy struct {
	RequireAllHardConstraints  bool `json:"require_all_hard_constraints"`
	UnknownHardConstraintFails bool `json:"unknown_hard_constraint_fails"`
	AllowSoftPreferenceMisses  bool `json:"allow_soft_preference_misses"`
}

type ConstraintSatisfactionReport struct {
	SchemaVersion                     string                        `json:"schema_version"`
	Status                            ConstraintSatisfactionStatus  `json:"status"`
	IntentFingerprint                 string                        `json:"intent_fingerprint,omitempty"`
	ConstraintSatisfactionFingerprint string                        `json:"constraint_satisfaction_fingerprint,omitempty"`
	MealPlanFingerprint               string                        `json:"mealplan_fingerprint,omitempty"`
	RecipeSetFingerprint              string                        `json:"recipe_set_fingerprint,omitempty"`
	ServingPlanFingerprint            string                        `json:"serving_plan_fingerprint,omitempty"`
	ScaledMealPlanFingerprint         string                        `json:"scaled_mealplan_fingerprint,omitempty"`
	PantryResolutionFingerprint       string                        `json:"pantry_resolution_fingerprint,omitempty"`
	ProductSelectionFingerprint       string                        `json:"product_selection_fingerprint,omitempty"`
	NutritionLedgerFingerprint        string                        `json:"nutrition_ledger_fingerprint,omitempty"`
	BudgetDealFingerprint             string                        `json:"budget_deal_fingerprint,omitempty"`
	BudgetRepairFingerprint           string                        `json:"budget_repair_fingerprint,omitempty"`
	Summary                           ConstraintSatisfactionSummary `json:"summary"`
	Checks                            []ConstraintCheck             `json:"checks"`
	BlockingIssues                    []ConstraintIssue             `json:"blocking_issues,omitempty"`
	Warnings                          []ConstraintIssue             `json:"warnings,omitempty"`
	ClaimGuard                        IntentClaimGuard              `json:"claim_guard"`
}

type ConstraintSatisfactionSummary struct {
	HardConstraintCount      int  `json:"hard_constraint_count"`
	HardSatisfiedCount       int  `json:"hard_satisfied_count"`
	HardFailedCount          int  `json:"hard_failed_count"`
	HardUnknownCount         int  `json:"hard_unknown_count"`
	SoftPreferenceCount      int  `json:"soft_preference_count"`
	SoftSatisfiedCount       int  `json:"soft_satisfied_count"`
	SoftMissedCount          int  `json:"soft_missed_count"`
	SoftUnknownCount         int  `json:"soft_unknown_count"`
	MayClaimRequestSatisfied bool `json:"may_claim_request_satisfied"`
}

type ConstraintCheck struct {
	ID           string        `json:"id"`
	Type         string        `json:"type"`
	Scope        string        `json:"scope"`
	Required     bool          `json:"required"`
	Status       string        `json:"status"`
	Severity     string        `json:"severity"`
	EvidenceRefs []EvidenceRef `json:"evidence_refs,omitempty"`
	Expected     any           `json:"expected,omitempty"`
	Actual       any           `json:"actual,omitempty"`
	Message      string        `json:"message"`
	Remediation  string        `json:"remediation,omitempty"`
}

type EvidenceRef struct {
	Artifact string `json:"artifact"`
	JSONPath string `json:"json_path,omitempty"`
	Label    string `json:"label,omitempty"`
}

type ConstraintIssue struct {
	Code         string `json:"code"`
	Severity     string `json:"severity"`
	ConstraintID string `json:"constraint_id,omitempty"`
	Message      string `json:"message"`
	Remediation  string `json:"remediation,omitempty"`
}

type IntentClaimGuard struct {
	MayClaimRequestSatisfied     bool   `json:"may_claim_request_satisfied"`
	MayClaimBudgetSatisfied      bool   `json:"may_claim_budget_satisfied"`
	MayClaimNutritionSatisfied   bool   `json:"may_claim_nutrition_satisfied"`
	MayClaimServingSatisfied     bool   `json:"may_claim_serving_satisfied"`
	MayClaimDietSatisfied        bool   `json:"may_claim_diet_satisfied"`
	MayClaimCookingTimeSatisfied bool   `json:"may_claim_cooking_time_satisfied"`
	MayClaimPDFComplete          bool   `json:"may_claim_pdf_complete"`
	RequiredUserWarning          string `json:"required_user_warning,omitempty"`
	PrimaryFailureCode           string `json:"primary_failure_code,omitempty"`
	PrimaryFailureMessage        string `json:"primary_failure_message,omitempty"`
}
