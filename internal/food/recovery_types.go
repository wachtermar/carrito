package food

type RecoveryIssueKind string

const (
	RecoveryIssueMissingProduct        RecoveryIssueKind = "missing_product"
	RecoveryIssueQuantityShortfall     RecoveryIssueKind = "quantity_shortfall"
	RecoveryIssueLowConfidenceProduct  RecoveryIssueKind = "low_confidence_product"
	RecoveryIssueVariableWeightBlocked RecoveryIssueKind = "variable_weight_blocked"
)

type RecoveryStatus string

const (
	RecoveryStatusNotNeeded RecoveryStatus = "not_needed"
	RecoveryStatusRecovered RecoveryStatus = "recovered"
	RecoveryStatusPartial   RecoveryStatus = "partial"
	RecoveryStatusFailed    RecoveryStatus = "failed"
)

type RecoveryPlan struct {
	SchemaVersion               string             `json:"schema_version"`
	Status                      RecoveryStatus     `json:"status"`
	MealPlanFingerprint         string             `json:"mealplan_fingerprint,omitempty"`
	ServingPlanFingerprint      string             `json:"serving_plan_fingerprint,omitempty"`
	ScaledMealPlanFingerprint   string             `json:"scaled_mealplan_fingerprint,omitempty"`
	ProductSelectionFingerprint string             `json:"product_selection_fingerprint,omitempty"`
	PantryResolutionFingerprint string             `json:"pantry_resolution_fingerprint,omitempty"`
	ShopRequirementsFingerprint string             `json:"shop_requirements_fingerprint,omitempty"`
	StartedFromStatus           string             `json:"started_from_status,omitempty"`
	FinalLedgerStatus           string             `json:"final_ledger_status,omitempty"`
	FinalBasketSafety           string             `json:"final_basket_safety,omitempty"`
	SafeToBuild                 bool               `json:"safe_to_build"`
	Issues                      []RecoveryIssue    `json:"issues,omitempty"`
	Attempts                    []RecoveryAttempt  `json:"attempts,omitempty"`
	AppliedDecisions            []RecoveryDecision `json:"applied_decisions,omitempty"`
	RemainingIssues             []RecoveryIssue    `json:"remaining_issues,omitempty"`
}

type RecoveryIssue struct {
	ID               string              `json:"id"`
	Kind             RecoveryIssueKind   `json:"kind"`
	IngredientID     string              `json:"ingredient_id,omitempty"`
	IngredientName   string              `json:"ingredient_name"`
	RecipeID         string              `json:"recipe_id,omitempty"`
	Day              int                 `json:"day,omitempty"`
	MealSlotID       string              `json:"meal_slot_id,omitempty"`
	RequiredQuantity *NormalizedQuantity `json:"required_quantity,omitempty"`
	BlockingReason   string              `json:"blocking_reason"`
}

type RecoveryAttempt struct {
	IssueID       string              `json:"issue_id"`
	Strategy      string              `json:"strategy"`
	Queries       []string            `json:"queries,omitempty"`
	Candidates    []RecoveryCandidate `json:"candidates,omitempty"`
	SelectedID    string              `json:"selected_id,omitempty"`
	FailureReason string              `json:"failure_reason,omitempty"`
}

type RecoveryCandidate struct {
	ID              string           `json:"id"`
	Product         *ProductEvidence `json:"product,omitempty"`
	CandidateType   string           `json:"candidate_type"`
	Compatibility   string           `json:"compatibility"`
	Confidence      string           `json:"confidence"`
	Score           int              `json:"score"`
	RequiredChanges []RecipeChange   `json:"required_changes,omitempty"`
	AcceptReasons   []string         `json:"accept_reasons,omitempty"`
	RejectReasons   []string         `json:"reject_reasons,omitempty"`
}

type RecoveryDecision struct {
	ID                 string         `json:"id"`
	IssueID            string         `json:"issue_id"`
	DecisionType       string         `json:"decision_type"`
	OriginalIngredient string         `json:"original_ingredient"`
	FinalIngredient    string         `json:"final_ingredient,omitempty"`
	OriginalProductID  string         `json:"original_product_id,omitempty"`
	FinalProductID     string         `json:"final_product_id,omitempty"`
	OriginalRecipeID   string         `json:"original_recipe_id,omitempty"`
	FinalRecipeID      string         `json:"final_recipe_id,omitempty"`
	RecipeChanges      []RecipeChange `json:"recipe_changes,omitempty"`
	Reason             string         `json:"reason"`
	Confidence         string         `json:"confidence"`
}

type RecipeChange struct {
	ChangeType   string `json:"change_type"`
	OriginalText string `json:"original_text,omitempty"`
	NewText      string `json:"new_text"`
	Reason       string `json:"reason"`
	Confidence   string `json:"confidence"`
}
