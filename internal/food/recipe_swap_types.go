package food

import "context"

type RecipeSwapStatus string

const (
	RecipeSwapNotNeeded        RecipeSwapStatus = "not_needed"
	RecipeSwapDisabled         RecipeSwapStatus = "disabled"
	RecipeSwapAttemptedApplied RecipeSwapStatus = "attempted_applied"
	RecipeSwapAttemptedFailed  RecipeSwapStatus = "attempted_failed"
)

type RecipeSwapPlan struct {
	SchemaVersion             string               `json:"schema_version"`
	Status                    RecipeSwapStatus     `json:"status"`
	Policy                    RecipeSwapPolicy     `json:"policy"`
	TriggeredByIssues         []GateIssueRef       `json:"triggered_by_issues,omitempty"`
	BeforeMealPlanFingerprint string               `json:"before_mealplan_fingerprint,omitempty"`
	AfterMealPlanFingerprint  string               `json:"after_mealplan_fingerprint,omitempty"`
	AffectedSlots             []AffectedRecipeSlot `json:"affected_slots,omitempty"`
	Attempts                  []RecipeSwapAttempt  `json:"attempts,omitempty"`
	AppliedSwaps              []AppliedRecipeSwap  `json:"applied_swaps,omitempty"`
	RemainingIssues           []GateIssue          `json:"remaining_issues,omitempty"`
	FinalReadinessStatus      string               `json:"final_readiness_status,omitempty"`
	FinalSafeToBuild          bool                 `json:"final_safe_to_build"`
}

type RecipeSwapPolicy struct {
	AllowRecipeSwap              bool `json:"allow_recipe_swap"`
	MaxAppliedSwaps              int  `json:"max_applied_swaps"`
	MaxCandidatesPerSlot         int  `json:"max_candidates_per_slot"`
	MaxTotalCandidateChecks      int  `json:"max_total_candidate_checks"`
	PreserveMealSlotType         bool `json:"preserve_meal_slot_type"`
	PreserveDietaryRules         bool `json:"preserve_dietary_rules"`
	PreserveAllergenRules        bool `json:"preserve_allergen_rules"`
	PreserveServings             bool `json:"preserve_servings"`
	AllowSwappingPinnedRecipes   bool `json:"allow_swapping_pinned_recipes"`
	StrictQuantity               bool `json:"strict_quantity"`
	AllowEstimatedVariableWeight bool `json:"allow_estimated_variable_weight"`
	AllowLowConfidenceBasket     bool `json:"allow_low_confidence_basket"`
}

type GateIssueRef struct {
	Code           string `json:"code"`
	Phase          string `json:"phase,omitempty"`
	IngredientKey  string `json:"ingredient_key,omitempty"`
	IngredientName string `json:"ingredient_name,omitempty"`
	RecipeID       string `json:"recipe_id,omitempty"`
	Day            int    `json:"day,omitempty"`
	MealSlot       string `json:"meal_slot,omitempty"`
	Message        string `json:"message,omitempty"`
}

type AffectedRecipeSlot struct {
	Day                    int      `json:"day"`
	MealSlot               string   `json:"meal_slot"`
	RecipeID               string   `json:"recipe_id"`
	RecipeTitle            string   `json:"recipe_title"`
	IsPinned               bool     `json:"is_pinned"`
	BlockingIngredientKeys []string `json:"blocking_ingredient_keys,omitempty"`
	BlockingIssueCodes     []string `json:"blocking_issue_codes,omitempty"`
}

type RecipeSwapAttempt struct {
	ID                  string                `json:"id"`
	Day                 int                   `json:"day"`
	MealSlot            string                `json:"meal_slot"`
	OriginalRecipeID    string                `json:"original_recipe_id"`
	OriginalRecipeTitle string                `json:"original_recipe_title"`
	Status              string                `json:"status"`
	Reason              string                `json:"reason,omitempty"`
	CandidateCount      int                   `json:"candidate_count"`
	Candidates          []RecipeSwapCandidate `json:"candidates,omitempty"`
	AppliedCandidateID  string                `json:"applied_candidate_id,omitempty"`
}

type RecipeSwapCandidate struct {
	ID                  string                      `json:"id"`
	RecipeID            string                      `json:"recipe_id,omitempty"`
	Title               string                      `json:"title"`
	Source              string                      `json:"source"`
	Valid               bool                        `json:"valid"`
	Rejected            bool                        `json:"rejected"`
	RejectCodes         []string                    `json:"reject_codes,omitempty"`
	RejectReason        string                      `json:"reject_reason,omitempty"`
	Score               float64                     `json:"score,omitempty"`
	Validation          CandidateShoppingValidation `json:"validation"`
	CostDeltaCents      int                         `json:"cost_delta_cents,omitempty"`
	ProductLineDelta    int                         `json:"product_line_delta,omitempty"`
	HasRecipeImage      bool                        `json:"has_recipe_image"`
	NutritionComparable bool                        `json:"nutrition_comparable"`
	NutritionDelta      *NutritionDeltaSummary      `json:"nutrition_delta,omitempty"`
}

type CandidateShoppingValidation struct {
	ReadinessStatus       string      `json:"readiness_status,omitempty"`
	SafeToBuild           bool        `json:"safe_to_build"`
	LedgerStatus          string      `json:"ledger_status,omitempty"`
	BasketSafe            bool        `json:"basket_safe"`
	MissingIngredientKeys []string    `json:"missing_ingredient_keys,omitempty"`
	EstimatedLines        int         `json:"estimated_lines"`
	LowConfidenceLines    int         `json:"low_confidence_lines"`
	SelectedProductCount  int         `json:"selected_product_count"`
	RejectedProductCount  int         `json:"rejected_product_count"`
	BlockingIssues        []GateIssue `json:"blocking_issues,omitempty"`
	Warnings              []GateIssue `json:"warnings,omitempty"`
}

type NutritionDeltaSummary struct {
	KcalDelta     float64 `json:"kcal_delta,omitempty"`
	ProteinGDelta float64 `json:"protein_g_delta,omitempty"`
	CarbsGDelta   float64 `json:"carbs_g_delta,omitempty"`
	FatGDelta     float64 `json:"fat_g_delta,omitempty"`
}

type AppliedRecipeSwap struct {
	ID                     string   `json:"id"`
	Day                    int      `json:"day"`
	MealSlot               string   `json:"meal_slot"`
	OriginalRecipeID       string   `json:"original_recipe_id"`
	OriginalRecipeTitle    string   `json:"original_recipe_title"`
	ReplacementRecipeID    string   `json:"replacement_recipe_id"`
	ReplacementRecipeTitle string   `json:"replacement_recipe_title"`
	Reason                 string   `json:"reason"`
	ResolvedIssueCodes     []string `json:"resolved_issue_codes,omitempty"`
	RemovedIngredientKeys  []string `json:"removed_ingredient_keys,omitempty"`
	AddedIngredientKeys    []string `json:"added_ingredient_keys,omitempty"`
	CostDeltaCents         int      `json:"cost_delta_cents,omitempty"`
	CandidateID            string   `json:"candidate_id"`
}

type RecipeSwapRequest struct {
	Day                    int         `json:"day,omitempty"`
	MealSlot               string      `json:"meal_slot,omitempty"`
	OriginalRecipe         Recipe      `json:"original_recipe"`
	BlockingIssues         []GateIssue `json:"blocking_issues,omitempty"`
	BlockingIngredientKeys []string    `json:"blocking_ingredient_keys,omitempty"`
	Servings               int         `json:"servings,omitempty"`
	Profile                Profile     `json:"profile,omitempty"`
	MaxCandidates          int         `json:"max_candidates,omitempty"`
}

type RecipeCandidateRecipe struct {
	Recipe Recipe `json:"recipe"`
	Source string `json:"source"`
}

type RecipeCandidateProvider interface {
	Candidates(ctx context.Context, req RecipeSwapRequest) ([]RecipeCandidateRecipe, error)
}
