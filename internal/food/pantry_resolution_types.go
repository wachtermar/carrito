package food

type PantryResolutionStatus string

const (
	PantryResolutionNotUsed                PantryResolutionStatus = "not_used"
	PantryResolutionApplied                PantryResolutionStatus = "applied"
	PantryResolutionAppliedWithAssumptions PantryResolutionStatus = "applied_with_assumptions"
	PantryResolutionNeedsReview            PantryResolutionStatus = "needs_review"
	PantryResolutionBlocked                PantryResolutionStatus = "blocked"
)

const (
	DefaultPantryNone               = "none"
	DefaultPantryMinimalSpanish     = "minimal_spanish"
	DefaultPantryMediterraneanBasic = "mediterranean_basic"
)

const (
	PantryDecisionBuyFull                = "buy_full"
	PantryDecisionBuyDelta               = "buy_delta"
	PantryDecisionPantryFullConfirmed    = "pantry_full_confirmed"
	PantryDecisionPantryFullAssumed      = "pantry_full_assumed"
	PantryDecisionPantryPartialConfirmed = "pantry_partial_confirmed"
	PantryDecisionOptionalSkipped        = "optional_skipped"
	PantryDecisionBlockedUnconfirmed     = "blocked_unconfirmed"
	PantryDecisionBlockedAmbiguousUnit   = "blocked_ambiguous_unit"
	PantryDecisionBlockedExpired         = "blocked_expired"
	PantryDecisionIgnored                = "ignored"
)

const (
	PantryItemStatusConfirmedAvailable   = "confirmed_available"
	PantryItemStatusAssumedAvailable     = "assumed_available"
	PantryItemStatusConfirmedUnavailable = "confirmed_unavailable"
	PantryItemStatusIgnore               = "ignore"
)

type PantryProfile struct {
	SchemaVersion  string              `json:"schema_version"`
	ProfileID      string              `json:"profile_id,omitempty"`
	Name           string              `json:"name,omitempty"`
	Locale         string              `json:"locale,omitempty"`
	DefaultProfile string              `json:"default_profile,omitempty"`
	Items          []PantryProfileItem `json:"items,omitempty"`
	UpdatedAt      string              `json:"updated_at,omitempty"`
}

type PantryProfileSummary struct {
	DefaultProfile string `json:"default_profile,omitempty"`
	ItemCount      int    `json:"item_count"`
	ConfirmedItems int    `json:"confirmed_items"`
	AssumedItems   int    `json:"assumed_items"`
}

type PantryProfileItem struct {
	IngredientKey  string                   `json:"ingredient_key"`
	Names          []string                 `json:"names,omitempty"`
	Aliases        []string                 `json:"aliases,omitempty"`
	Category       string                   `json:"category,omitempty"`
	Form           string                   `json:"form,omitempty"`
	Status         string                   `json:"status"`
	Quantity       *NormalizedQuantity      `json:"quantity,omitempty"`
	MinKeep        *NormalizedQuantity      `json:"min_keep,omitempty"`
	ExpiresAt      string                   `json:"expires_at,omitempty"`
	LastVerifiedAt string                   `json:"last_verified_at,omitempty"`
	Confidence     float64                  `json:"confidence,omitempty"`
	Source         string                   `json:"source,omitempty"`
	Notes          string                   `json:"notes,omitempty"`
	Nutrition      *PantryNutritionEvidence `json:"nutrition,omitempty"`
}

type PantryPolicy struct {
	Enabled                    bool   `json:"enabled"`
	DefaultPantry              string `json:"default_pantry"`
	AllowAssumedPantry         bool   `json:"allow_assumed_pantry"`
	RequireConfirmedPantry     bool   `json:"require_confirmed_pantry"`
	UseExpiredItems            bool   `json:"use_expired_items"`
	ShopUnknownStaples         bool   `json:"shop_unknown_staples"`
	ShopAllIngredients         bool   `json:"shop_all_ingredients"`
	AllowPartialPantryCoverage bool   `json:"allow_partial_pantry_coverage"`
	BlockAmbiguousPantryUnits  bool   `json:"block_ambiguous_pantry_units"`
	AlwaysBuyFresh             bool   `json:"always_buy_fresh"`
}

type PantryResolution struct {
	SchemaVersion               string                  `json:"schema_version"`
	Status                      PantryResolutionStatus  `json:"status"`
	Policy                      PantryPolicy            `json:"policy"`
	MealPlanFingerprint         string                  `json:"mealplan_fingerprint,omitempty"`
	ServingPlanFingerprint      string                  `json:"serving_plan_fingerprint,omitempty"`
	ScaledMealPlanFingerprint   string                  `json:"scaled_mealplan_fingerprint,omitempty"`
	PantryProfileFingerprint    string                  `json:"pantry_profile_fingerprint,omitempty"`
	PantryResolutionFingerprint string                  `json:"pantry_resolution_fingerprint,omitempty"`
	ShopRequirementsFingerprint string                  `json:"shop_requirements_fingerprint,omitempty"`
	Lines                       []PantryResolutionLine  `json:"lines,omitempty"`
	Summary                     PantryResolutionSummary `json:"summary,omitempty"`
	BlockingIssues              []PantryIssue           `json:"blocking_issues,omitempty"`
	Warnings                    []PantryIssue           `json:"warnings,omitempty"`
}

type PantryResolutionLine struct {
	IngredientKey     string               `json:"ingredient_key"`
	IngredientName    string               `json:"ingredient_name"`
	SearchTerm        string               `json:"search_term,omitempty"`
	Category          string               `json:"category,omitempty"`
	Usages            []IngredientUsageRef `json:"usages,omitempty"`
	TotalRequired     *NormalizedQuantity  `json:"total_required,omitempty"`
	PantryDecision    string               `json:"pantry_decision"`
	PantryItemKey     string               `json:"pantry_item_key,omitempty"`
	PantryStatus      string               `json:"pantry_status,omitempty"`
	PantryAvailable   *NormalizedQuantity  `json:"pantry_available,omitempty"`
	PantryAllocated   *NormalizedQuantity  `json:"pantry_allocated,omitempty"`
	ShopRequired      *NormalizedQuantity  `json:"shop_required,omitempty"`
	RemainingAfterUse *NormalizedQuantity  `json:"remaining_after_use,omitempty"`
	Confidence        float64              `json:"confidence,omitempty"`
	Reason            string               `json:"reason,omitempty"`
	Warnings          []string             `json:"warnings,omitempty"`
}

type PantryResolutionSummary struct {
	TotalIngredientLines         int `json:"total_ingredient_lines"`
	BuyFullLines                 int `json:"buy_full_lines"`
	BuyDeltaLines                int `json:"buy_delta_lines"`
	PantryConfirmedLines         int `json:"pantry_confirmed_lines"`
	PantryAssumedLines           int `json:"pantry_assumed_lines"`
	OptionalSkippedLines         int `json:"optional_skipped_lines"`
	BlockedLines                 int `json:"blocked_lines"`
	ShopIngredientLines          int `json:"shop_ingredient_lines"`
	PantryCoveredIngredientLines int `json:"pantry_covered_ingredient_lines"`
}

type PantryIssue struct {
	Code           string `json:"code"`
	Severity       string `json:"severity"`
	IngredientKey  string `json:"ingredient_key,omitempty"`
	IngredientName string `json:"ingredient_name,omitempty"`
	Message        string `json:"message"`
	Remediation    string `json:"remediation,omitempty"`
}

type ShoppingRequirement struct {
	IngredientKey   string               `json:"ingredient_key"`
	IngredientName  string               `json:"ingredient_name"`
	SearchTerm      string               `json:"search_term,omitempty"`
	Category        string               `json:"category,omitempty"`
	TotalRequired   *NormalizedQuantity  `json:"total_required,omitempty"`
	PantryAllocated *NormalizedQuantity  `json:"pantry_allocated,omitempty"`
	ShopRequired    *NormalizedQuantity  `json:"shop_required,omitempty"`
	SourcingStatus  string               `json:"sourcing_status,omitempty"`
	PantryDecision  string               `json:"pantry_decision,omitempty"`
	Usages          []IngredientUsageRef `json:"usages,omitempty"`
}

type PantryConsumptionPlan struct {
	SchemaVersion               string                 `json:"schema_version"`
	PantryResolutionFingerprint string                 `json:"pantry_resolution_fingerprint,omitempty"`
	Lines                       []PantryResolutionLine `json:"lines,omitempty"`
}
