package food

type BasketOptimizationStatus string

const (
	BasketOptimizationNotRun        BasketOptimizationStatus = "not_run"
	BasketOptimizationSkipped       BasketOptimizationStatus = "skipped"
	BasketOptimizationApplied       BasketOptimizationStatus = "applied"
	BasketOptimizationNoImprovement BasketOptimizationStatus = "no_improvement"
	BasketOptimizationFailed        BasketOptimizationStatus = "failed"
)

const (
	BasketObjectiveSafeBalanced = "safe_balanced"
	BasketObjectiveLowestPrice  = "lowest_price"
	BasketObjectiveLowestWaste  = "lowest_waste"
)

type BasketOptimizationPlan struct {
	SchemaVersion                     string                       `json:"schema_version"`
	Status                            BasketOptimizationStatus     `json:"status"`
	Policy                            BasketOptimizationPolicy     `json:"policy"`
	MealPlanFingerprint               string                       `json:"mealplan_fingerprint,omitempty"`
	ServingPlanFingerprint            string                       `json:"serving_plan_fingerprint,omitempty"`
	ScaledMealPlanFingerprint         string                       `json:"scaled_mealplan_fingerprint,omitempty"`
	BeforeProductSelectionFingerprint string                       `json:"before_product_selection_fingerprint,omitempty"`
	AfterProductSelectionFingerprint  string                       `json:"after_product_selection_fingerprint,omitempty"`
	PantryResolutionFingerprint       string                       `json:"pantry_resolution_fingerprint,omitempty"`
	ShopRequirementsFingerprint       string                       `json:"shop_requirements_fingerprint,omitempty"`
	BaselineSummary                   BasketOptimizationSummary    `json:"baseline_summary"`
	OptimizedSummary                  *BasketOptimizationSummary   `json:"optimized_summary,omitempty"`
	CandidatePools                    []IngredientCandidatePool    `json:"candidate_pools,omitempty"`
	Decisions                         []BasketOptimizationDecision `json:"decisions,omitempty"`
	RejectedCandidates                []RejectedBasketCandidate    `json:"rejected_candidates,omitempty"`
	Validation                        BasketOptimizationValidation `json:"validation"`
	Warnings                          []OptimizationWarning        `json:"warnings,omitempty"`
}

type BasketOptimizationPolicy struct {
	Enabled                        bool    `json:"enabled"`
	Objective                      string  `json:"objective"`
	MaxCandidatesPerIngredient     int     `json:"max_candidates_per_ingredient"`
	MaxSearchesPerIngredient       int     `json:"max_searches_per_ingredient"`
	MaxTotalCandidateChecks        int     `json:"max_total_candidate_checks"`
	MaxProductSwitches             int     `json:"max_product_switches,omitempty"`
	DealAware                      bool    `json:"deal_aware"`
	StrictQuantity                 bool    `json:"strict_quantity"`
	AllowEstimatedVariableWeight   bool    `json:"allow_estimated_variable_weight"`
	AllowLowConfidenceBasket       bool    `json:"allow_low_confidence_basket"`
	RequireNoReadinessRegression   bool    `json:"require_no_readiness_regression"`
	MinSavingsCentsToSwitch        int     `json:"min_savings_cents_to_switch"`
	MinWasteReductionRatioToSwitch float64 `json:"min_waste_reduction_ratio_to_switch"`
	PreferProductImages            bool    `json:"prefer_product_images"`
	PreferNutritionLabels          bool    `json:"prefer_nutrition_labels"`
}

type IngredientCandidatePool struct {
	IngredientKey       string                         `json:"ingredient_key"`
	IngredientName      string                         `json:"ingredient_name"`
	RequiredQuantity    *NormalizedQuantity            `json:"required_quantity,omitempty"`
	UnitDimension       string                         `json:"unit_dimension,omitempty"`
	SearchQueries       []string                       `json:"search_queries,omitempty"`
	BaselineProductID   string                         `json:"baseline_product_id,omitempty"`
	BaselineProductName string                         `json:"baseline_product_name,omitempty"`
	Candidates          []ProductOptimizationCandidate `json:"candidates,omitempty"`
	SelectedCandidateID string                         `json:"selected_candidate_id,omitempty"`
	Reason              string                         `json:"reason,omitempty"`
}

type ProductOptimizationCandidate struct {
	ID                 string                       `json:"id"`
	Product            ProductSummary               `json:"product,omitempty"`
	ProductID          string                       `json:"product_id,omitempty"`
	SKU                string                       `json:"sku,omitempty"`
	ProductName        string                       `json:"product_name,omitempty"`
	Brand              string                       `json:"brand,omitempty"`
	URL                string                       `json:"url,omitempty"`
	Source             string                       `json:"source"`
	SearchQuery        string                       `json:"search_query,omitempty"`
	Available          bool                         `json:"available"`
	AvailabilityStatus string                       `json:"availability_status,omitempty"`
	SemanticMatch      SemanticMatchEvidence        `json:"semantic_match"`
	QuantityParse      ProductQuantityParseEvidence `json:"quantity_parse"`
	Price              OptimizationPriceEvidence    `json:"price"`
	Offer              OptimizationOfferEvidence    `json:"offer,omitempty"`
	Allocation         ProductAllocationOption      `json:"allocation"`
	HasImage           bool                         `json:"has_image"`
	HasNutritionLabel  bool                         `json:"has_nutrition_label"`
	Valid              bool                         `json:"valid"`
	RejectCodes        []string                     `json:"reject_codes,omitempty"`
	RejectReason       string                       `json:"reject_reason,omitempty"`
	Score              BasketCandidateScore         `json:"score"`
}

type SemanticMatchEvidence struct {
	Status       string  `json:"status"`
	Confidence   float64 `json:"confidence"`
	MatchedAlias string  `json:"matched_alias,omitempty"`
	RejectReason string  `json:"reject_reason,omitempty"`
}

type ProductQuantityParseEvidence struct {
	Status            string              `json:"status"`
	RawText           string              `json:"raw_text,omitempty"`
	PackageQuantity   *NormalizedQuantity `json:"package_quantity,omitempty"`
	UsedNetWeight     bool                `json:"used_net_weight,omitempty"`
	UsedDrainedWeight bool                `json:"used_drained_weight,omitempty"`
	Confidence        float64             `json:"confidence"`
}

type OptimizationPriceEvidence struct {
	CurrentPriceCents  int    `json:"current_price_cents,omitempty"`
	PreviousPriceCents int    `json:"previous_price_cents,omitempty"`
	UnitPriceCents     int    `json:"unit_price_cents,omitempty"`
	UnitPriceUnit      string `json:"unit_price_unit,omitempty"`
	Currency           string `json:"currency,omitempty"`
	PriceConfidence    string `json:"price_confidence"`
	RawPriceText       string `json:"raw_price_text,omitempty"`
}

type OptimizationOfferEvidence struct {
	RawText             string  `json:"raw_text,omitempty"`
	Parsed              bool    `json:"parsed"`
	Type                string  `json:"type,omitempty"`
	Confidence          float64 `json:"confidence"`
	MinPackageCount     int     `json:"min_package_count,omitempty"`
	PaidPackageCount    int     `json:"paid_package_count,omitempty"`
	DiscountPercent     int     `json:"discount_percent,omitempty"`
	Applied             bool    `json:"applied"`
	AppliedPackageCount int     `json:"applied_package_count,omitempty"`
	SavingsCents        int     `json:"savings_cents,omitempty"`
	NotAppliedReason    string  `json:"not_applied_reason,omitempty"`
}

type ProductAllocationOption struct {
	RequiredQuantity   *NormalizedQuantity `json:"required_quantity,omitempty"`
	PackageQuantity    *NormalizedQuantity `json:"package_quantity,omitempty"`
	PackageCount       int                 `json:"package_count"`
	PurchasedQuantity  *NormalizedQuantity `json:"purchased_quantity,omitempty"`
	ExcessQuantity     *NormalizedQuantity `json:"excess_quantity,omitempty"`
	ExcessRatio        float64             `json:"excess_ratio,omitempty"`
	GrossCostCents     int                 `json:"gross_cost_cents,omitempty"`
	EffectiveCostCents int                 `json:"effective_cost_cents,omitempty"`
	OfferSavingsCents  int                 `json:"offer_savings_cents,omitempty"`
	QuantityStatus     string              `json:"quantity_status"`
	Reason             string              `json:"reason,omitempty"`
}

type BasketCandidateScore struct {
	SafetyTier    int     `json:"safety_tier"`
	SemanticScore float64 `json:"semantic_score"`
	QuantityScore float64 `json:"quantity_score"`
	WasteScore    float64 `json:"waste_score"`
	PriceScore    float64 `json:"price_score"`
	OfferScore    float64 `json:"offer_score"`
	EvidenceScore float64 `json:"evidence_score"`
	FinalScore    float64 `json:"final_score"`
	Explanation   string  `json:"explanation"`
}

type BasketOptimizationDecision struct {
	IngredientKey       string                  `json:"ingredient_key"`
	IngredientName      string                  `json:"ingredient_name"`
	BaselineProductID   string                  `json:"baseline_product_id,omitempty"`
	BaselineProductName string                  `json:"baseline_product_name,omitempty"`
	SelectedProductID   string                  `json:"selected_product_id,omitempty"`
	SelectedProductName string                  `json:"selected_product_name,omitempty"`
	Changed             bool                    `json:"changed"`
	Reason              string                  `json:"reason"`
	BaselineAllocation  ProductAllocationOption `json:"baseline_allocation"`
	SelectedAllocation  ProductAllocationOption `json:"selected_allocation"`
	CostDeltaCents      int                     `json:"cost_delta_cents,omitempty"`
	OfferSavingsCents   int                     `json:"offer_savings_cents,omitempty"`
	ReadinessImpact     string                  `json:"readiness_impact,omitempty"`
}

type BasketOptimizationSummary struct {
	ProductLineCount       int    `json:"product_line_count"`
	GrossSubtotalCents     int    `json:"gross_subtotal_cents,omitempty"`
	EffectiveSubtotalCents int    `json:"effective_subtotal_cents,omitempty"`
	OfferSavingsCents      int    `json:"offer_savings_cents,omitempty"`
	IngredientsWithExcess  int    `json:"ingredients_with_excess"`
	ExactQuantityLines     int    `json:"exact_quantity_lines"`
	EstimatedQuantityLines int    `json:"estimated_quantity_lines"`
	LowConfidenceLines     int    `json:"low_confidence_lines"`
	ProductImageCoverage   int    `json:"product_image_coverage"`
	NutritionLabelCoverage int    `json:"nutrition_label_coverage"`
	ReadinessStatus        string `json:"readiness_status,omitempty"`
	SafeToBuild            bool   `json:"safe_to_build"`
}

type BasketOptimizationValidation struct {
	BaselineReadinessStatus     string `json:"baseline_readiness_status,omitempty"`
	BaselineSafeToBuild         bool   `json:"baseline_safe_to_build"`
	FinalReadinessStatus        string `json:"final_readiness_status,omitempty"`
	FinalSafeToBuild            bool   `json:"final_safe_to_build"`
	ReadinessRegression         bool   `json:"readiness_regression"`
	FinalLedgerStatus           string `json:"final_ledger_status,omitempty"`
	FinalBasketSafety           bool   `json:"final_basket_safety"`
	AppliedSelectionFingerprint string `json:"applied_selection_fingerprint,omitempty"`
}

type RejectedBasketCandidate struct {
	IngredientKey string   `json:"ingredient_key,omitempty"`
	ProductID     string   `json:"product_id,omitempty"`
	ProductName   string   `json:"product_name,omitempty"`
	RejectCodes   []string `json:"reject_codes,omitempty"`
	RejectReason  string   `json:"reject_reason,omitempty"`
}

type OptimizationWarning struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	Scope   string `json:"scope,omitempty"`
}
