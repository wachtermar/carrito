package food

type ServingPlanStatus string

const (
	ServingPlanNotUsed          ServingPlanStatus = "not_used"
	ServingPlanRecipeDefault    ServingPlanStatus = "recipe_default"
	ServingPlanExplicitServings ServingPlanStatus = "explicit_servings"
	ServingPlanHouseholdProfile ServingPlanStatus = "household_profile"
	ServingPlanBlocked          ServingPlanStatus = "blocked"
)

const (
	ScalingStatusExact         = "exact"
	ScalingStatusRounded       = "rounded"
	ScalingStatusRecipeDefault = "recipe_default"
	ScalingStatusNeedsReview   = "needs_review"
	ScalingStatusBlocked       = "blocked"

	ScaledQuantityExact        = "exact"
	ScaledQuantityRoundedPiece = "rounded_piece"
	ScaledQuantityToTaste      = "to_taste"
	ScaledQuantityOptional     = "optional"
	ScaledQuantityUnparseable  = "unparseable"
	ScaledQuantityBlocked      = "blocked"
)

type HouseholdProfile struct {
	SchemaVersion            string                  `json:"schema_version"`
	ProfileID                string                  `json:"profile_id,omitempty"`
	Name                     string                  `json:"name,omitempty"`
	Locale                   string                  `json:"locale,omitempty"`
	Members                  []HouseholdMember       `json:"members,omitempty"`
	DefaultMealParticipation []MealParticipationRule `json:"default_meal_participation,omitempty"`
	UpdatedAt                string                  `json:"updated_at,omitempty"`
}

type HouseholdMember struct {
	ID                   string   `json:"id"`
	DisplayName          string   `json:"display_name,omitempty"`
	Type                 string   `json:"type,omitempty"`
	DefaultServingFactor float64  `json:"default_serving_factor,omitempty"`
	DietaryRules         []string `json:"dietary_rules,omitempty"`
	AllergensExcluded    []string `json:"allergens_excluded,omitempty"`
	DislikedIngredients  []string `json:"disliked_ingredients,omitempty"`
	Notes                string   `json:"notes,omitempty"`
}

type MealParticipationRule struct {
	MealSlot  string   `json:"meal_slot,omitempty"`
	MemberIDs []string `json:"member_ids,omitempty"`
}

type HouseholdProfileSummary struct {
	ProfileID          string  `json:"profile_id,omitempty"`
	MemberCount        int     `json:"member_count"`
	AdultCount         int     `json:"adult_count,omitempty"`
	ChildCount         int     `json:"child_count,omitempty"`
	ToddlerCount       int     `json:"toddler_count,omitempty"`
	ServingUnitTotal   float64 `json:"serving_unit_total,omitempty"`
	AnonymousOnly      bool    `json:"anonymous_only,omitempty"`
	ParticipationRules int     `json:"participation_rules,omitempty"`
}

type ServingPolicy struct {
	Enabled                               bool    `json:"enabled"`
	StrictServingScaling                  bool    `json:"strict_serving_scaling"`
	DefaultServings                       float64 `json:"default_servings,omitempty"`
	AdultServings                         float64 `json:"adult_servings,omitempty"`
	ChildServings                         float64 `json:"child_servings,omitempty"`
	ToddlerServings                       float64 `json:"toddler_servings,omitempty"`
	PreserveRecipeServingsWhenUnspecified bool    `json:"preserve_recipe_servings_when_unspecified"`
	AllowFractionalServings               bool    `json:"allow_fractional_servings"`
	AllowLeftovers                        bool    `json:"allow_leftovers"`
	LeftoverServings                      float64 `json:"leftover_servings,omitempty"`
	RoundPieceIngredients                 bool    `json:"round_piece_ingredients"`
	RequireBaseRecipeServings             bool    `json:"require_base_recipe_servings"`
}

type ScalingPolicy struct {
	StrictServingScaling        bool `json:"strict_serving_scaling"`
	AllowFractionalServings     bool `json:"allow_fractional_servings"`
	RoundPieceIngredients       bool `json:"round_piece_ingredients"`
	PreserveToTasteQuantities   bool `json:"preserve_to_taste_quantities"`
	PreserveOptionalIngredients bool `json:"preserve_optional_ingredients"`
	RequireBaseRecipeServings   bool `json:"require_base_recipe_servings"`
}

type ServingPlan struct {
	SchemaVersion               string             `json:"schema_version"`
	Status                      ServingPlanStatus  `json:"status"`
	Policy                      ServingPolicy      `json:"policy"`
	MealPlanFingerprint         string             `json:"mealplan_fingerprint,omitempty"`
	HouseholdProfileFingerprint string             `json:"household_profile_fingerprint,omitempty"`
	ServingPlanFingerprint      string             `json:"serving_plan_fingerprint,omitempty"`
	Slots                       []ServingSlot      `json:"slots,omitempty"`
	Summary                     ServingPlanSummary `json:"summary,omitempty"`
	BlockingIssues              []ServingIssue     `json:"blocking_issues,omitempty"`
	Warnings                    []ServingIssue     `json:"warnings,omitempty"`
}

type ServingSlot struct {
	Day                int                  `json:"day,omitempty"`
	Date               string               `json:"date,omitempty"`
	MealSlot           string               `json:"meal_slot,omitempty"`
	RecipeID           string               `json:"recipe_id,omitempty"`
	RecipeTitle        string               `json:"recipe_title,omitempty"`
	BaseRecipeServings float64              `json:"base_recipe_servings,omitempty"`
	TargetServingUnits float64              `json:"target_serving_units,omitempty"`
	CookedServingUnits float64              `json:"cooked_serving_units,omitempty"`
	ScaleFactor        float64              `json:"scale_factor,omitempty"`
	Participants       []ServingParticipant `json:"participants,omitempty"`
	LeftoverPlan       *LeftoverPlan        `json:"leftover_plan,omitempty"`
	ScalingStatus      string               `json:"scaling_status,omitempty"`
	Warnings           []string             `json:"warnings,omitempty"`
}

type ServingParticipant struct {
	MemberID      string  `json:"member_id,omitempty"`
	DisplayName   string  `json:"display_name,omitempty"`
	ServingFactor float64 `json:"serving_factor,omitempty"`
	PortionType   string  `json:"portion_type,omitempty"`
}

type LeftoverPlan struct {
	Enabled           bool     `json:"enabled"`
	ExtraServingUnits float64  `json:"extra_serving_units,omitempty"`
	IntendedUse       string   `json:"intended_use,omitempty"`
	ConsumedBy        []string `json:"consumed_by,omitempty"`
}

type ServingPlanSummary struct {
	TotalMealSlots          int     `json:"total_meal_slots"`
	TotalTargetServingUnits float64 `json:"total_target_serving_units,omitempty"`
	TotalCookedServingUnits float64 `json:"total_cooked_serving_units,omitempty"`
	SlotsUsingRecipeDefault int     `json:"slots_using_recipe_default,omitempty"`
	SlotsScaled             int     `json:"slots_scaled,omitempty"`
	SlotsWithLeftovers      int     `json:"slots_with_leftovers,omitempty"`
	SlotsNeedingReview      int     `json:"slots_needing_review,omitempty"`
	BlockedSlots            int     `json:"blocked_slots,omitempty"`
}

type ServingIssue struct {
	Code        string `json:"code"`
	Severity    string `json:"severity"`
	Day         int    `json:"day,omitempty"`
	MealSlot    string `json:"meal_slot,omitempty"`
	RecipeID    string `json:"recipe_id,omitempty"`
	RecipeTitle string `json:"recipe_title,omitempty"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

type ScaledMealPlan struct {
	SchemaVersion             string                `json:"schema_version"`
	MealPlanFingerprint       string                `json:"mealplan_fingerprint,omitempty"`
	ServingPlanFingerprint    string                `json:"serving_plan_fingerprint,omitempty"`
	ScaledMealPlanFingerprint string                `json:"scaled_mealplan_fingerprint,omitempty"`
	Slots                     []ScaledRecipeSlot    `json:"slots,omitempty"`
	Summary                   ScaledMealPlanSummary `json:"summary,omitempty"`
	Warnings                  []ScalingWarning      `json:"warnings,omitempty"`
}

type ScaledRecipeSlot struct {
	Day                int                `json:"day,omitempty"`
	Date               string             `json:"date,omitempty"`
	MealSlot           string             `json:"meal_slot,omitempty"`
	RecipeID           string             `json:"recipe_id,omitempty"`
	RecipeTitle        string             `json:"recipe_title,omitempty"`
	BaseServings       float64            `json:"base_servings,omitempty"`
	TargetServingUnits float64            `json:"target_serving_units,omitempty"`
	CookedServingUnits float64            `json:"cooked_serving_units,omitempty"`
	ScaleFactor        float64            `json:"scale_factor,omitempty"`
	Ingredients        []ScaledIngredient `json:"ingredients,omitempty"`
	ScalingNotes       []string           `json:"scaling_notes,omitempty"`
}

type ScaledIngredient struct {
	IngredientKey    string               `json:"ingredient_key,omitempty"`
	IngredientName   string               `json:"ingredient_name,omitempty"`
	SearchTerm       string               `json:"search_term,omitempty"`
	Category         string               `json:"category,omitempty"`
	OriginalQuantity *NormalizedQuantity  `json:"original_quantity,omitempty"`
	ScaledQuantity   *NormalizedQuantity  `json:"scaled_quantity,omitempty"`
	ShoppingQuantity *NormalizedQuantity  `json:"shopping_quantity,omitempty"`
	CookingQuantity  *NormalizedQuantity  `json:"cooking_quantity,omitempty"`
	QuantityStatus   string               `json:"quantity_status,omitempty"`
	ScalingFactor    float64              `json:"scaling_factor,omitempty"`
	RoundingApplied  bool                 `json:"rounding_applied,omitempty"`
	RoundingReason   string               `json:"rounding_reason,omitempty"`
	Optional         bool                 `json:"optional,omitempty"`
	PantryCandidate  bool                 `json:"pantry_candidate,omitempty"`
	Usages           []IngredientUsageRef `json:"usages,omitempty"`
}

type ScaledMealPlanSummary struct {
	IngredientLines   int `json:"ingredient_lines"`
	ExactLines        int `json:"exact_lines,omitempty"`
	RoundedPieceLines int `json:"rounded_piece_lines,omitempty"`
	ToTasteLines      int `json:"to_taste_lines,omitempty"`
	OptionalLines     int `json:"optional_lines,omitempty"`
	UnparseableLines  int `json:"unparseable_lines,omitempty"`
	BlockedLines      int `json:"blocked_lines,omitempty"`
}

type ScalingWarning struct {
	Code           string `json:"code"`
	Severity       string `json:"severity"`
	IngredientKey  string `json:"ingredient_key,omitempty"`
	IngredientName string `json:"ingredient_name,omitempty"`
	Day            int    `json:"day,omitempty"`
	MealSlot       string `json:"meal_slot,omitempty"`
	Message        string `json:"message"`
}
