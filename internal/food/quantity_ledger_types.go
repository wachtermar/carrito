package food

import "github.com/wachtermar/carrito/internal/money"

type LedgerStatus string

const (
	LedgerCompleteExact     LedgerStatus = "complete_exact"
	LedgerCompleteEstimated LedgerStatus = "complete_estimated"
	LedgerIncomplete        LedgerStatus = "incomplete"
	LedgerNeedsReview       LedgerStatus = "needs_review"
)

type BasketSafetyStatus string

const (
	BasketSafetySafe       BasketSafetyStatus = "safe_to_build"
	BasketSafetyEstimated  BasketSafetyStatus = "safe_with_estimates"
	BasketSafetyReviewOnly BasketSafetyStatus = "review_only"
	BasketSafetyUnsafe     BasketSafetyStatus = "unsafe"
)

type QuantityLedger struct {
	Status                      LedgerStatus                  `json:"status"`
	GeneratedAt                 string                        `json:"generated_at,omitempty"`
	StoreID                     string                        `json:"store_id,omitempty"`
	MealPlanFingerprint         string                        `json:"mealplan_fingerprint,omitempty"`
	ServingPlanFingerprint      string                        `json:"serving_plan_fingerprint,omitempty"`
	ScaledMealPlanFingerprint   string                        `json:"scaled_mealplan_fingerprint,omitempty"`
	ProductSelectionFingerprint string                        `json:"product_selection_fingerprint,omitempty"`
	PantryResolutionFingerprint string                        `json:"pantry_resolution_fingerprint,omitempty"`
	ShopRequirementsFingerprint string                        `json:"shop_requirements_fingerprint,omitempty"`
	Requirements                []IngredientRequirement       `json:"requirements,omitempty"`
	ProductEvidence             []ProductEvidence             `json:"product_evidence,omitempty"`
	Allocations                 []IngredientProductAllocation `json:"allocations,omitempty"`
	Totals                      LedgerTotals                  `json:"totals,omitempty"`
	Nutrition                   *LedgerNutritionReport        `json:"nutrition,omitempty"`
	Summary                     LedgerSummary                 `json:"summary,omitempty"`
	Warnings                    []LedgerWarning               `json:"warnings,omitempty"`
	Provenance                  []DataProvenance              `json:"provenance,omitempty"`
}

type LedgerSummary struct {
	IngredientCount              int     `json:"ingredient_count"`
	CoveredIngredientCount       int     `json:"covered_ingredient_count"`
	ExactQuantityLines           int     `json:"exact_quantity_lines"`
	EstimatedVariableWeightLines int     `json:"estimated_variable_weight_lines"`
	NeedsReviewLines             int     `json:"needs_review_lines"`
	MissingLines                 int     `json:"missing_lines"`
	NutritionCoverageRatio       float64 `json:"nutrition_coverage_ratio"`
	SafeToBuildBasket            bool    `json:"safe_to_build_basket"`
}

type BasketSafety struct {
	Status                      BasketSafetyStatus `json:"status"`
	SafeToBuild                 bool               `json:"safe_to_build"`
	AllowsEstimates             bool               `json:"allows_estimates,omitempty"`
	Reason                      string             `json:"reason,omitempty"`
	Warnings                    []string           `json:"warnings,omitempty"`
	MealPlanFingerprint         string             `json:"mealplan_fingerprint,omitempty"`
	ServingPlanFingerprint      string             `json:"serving_plan_fingerprint,omitempty"`
	ScaledMealPlanFingerprint   string             `json:"scaled_mealplan_fingerprint,omitempty"`
	ProductSelectionFingerprint string             `json:"product_selection_fingerprint,omitempty"`
	PantryResolutionFingerprint string             `json:"pantry_resolution_fingerprint,omitempty"`
	ShopRequirementsFingerprint string             `json:"shop_requirements_fingerprint,omitempty"`
}

type NormalizedQuantity struct {
	Raw         string  `json:"raw,omitempty"`
	Value       float64 `json:"value"`
	Unit        string  `json:"unit"`
	BaseValue   float64 `json:"base_value"`
	BaseUnit    string  `json:"base_unit"`
	Confidence  float64 `json:"confidence"`
	ParseMethod string  `json:"parse_method,omitempty"`
}

type QuantityRange struct {
	Min      *NormalizedQuantity `json:"min,omitempty"`
	Expected NormalizedQuantity  `json:"expected"`
	Max      *NormalizedQuantity `json:"max,omitempty"`
	IsExact  bool                `json:"is_exact"`
	Reason   string              `json:"reason,omitempty"`
}

type IngredientRequirement struct {
	RequirementID           string               `json:"requirement_id"`
	RecipeID                string               `json:"recipe_id,omitempty"`
	RecipeTitle             string               `json:"recipe_title,omitempty"`
	Day                     int                  `json:"day,omitempty"`
	MealSlot                string               `json:"meal_slot,omitempty"`
	Usages                  []IngredientUsageRef `json:"usages,omitempty"`
	IngredientName          string               `json:"ingredient_name"`
	RawQuantity             string               `json:"raw_quantity,omitempty"`
	RequiredQuantity        *NormalizedQuantity  `json:"required_quantity,omitempty"`
	TotalRequiredQuantity   *NormalizedQuantity  `json:"total_required_quantity,omitempty"`
	PantryAllocatedQuantity *NormalizedQuantity  `json:"pantry_allocated_quantity,omitempty"`
	ShopRequiredQuantity    *NormalizedQuantity  `json:"shop_required_quantity,omitempty"`
	SourcingStatus          string               `json:"sourcing_status,omitempty"`
	PantryDecision          string               `json:"pantry_decision,omitempty"`
	ParseConfidence         float64              `json:"parse_confidence,omitempty"`
	Warnings                []string             `json:"warnings,omitempty"`
}

type IngredientUsageRef struct {
	Day                int                 `json:"day,omitempty"`
	MealSlot           string              `json:"meal_slot,omitempty"`
	RecipeID           string              `json:"recipe_id,omitempty"`
	RecipeTitle        string              `json:"recipe_title,omitempty"`
	BaseServings       float64             `json:"base_servings,omitempty"`
	TargetServingUnits float64             `json:"target_serving_units,omitempty"`
	CookedServingUnits float64             `json:"cooked_serving_units,omitempty"`
	ScaleFactor        float64             `json:"scale_factor,omitempty"`
	IngredientKey      string              `json:"ingredient_key,omitempty"`
	IngredientName     string              `json:"ingredient_name,omitempty"`
	RequiredQuantity   *NormalizedQuantity `json:"required_quantity,omitempty"`
}

type ProductEvidence struct {
	ProductID    string               `json:"product_id,omitempty"`
	SKU          string               `json:"sku,omitempty"`
	EAN          string               `json:"ean,omitempty"`
	Name         string               `json:"name,omitempty"`
	Brand        string               `json:"brand,omitempty"`
	URL          string               `json:"url,omitempty"`
	ImageURL     string               `json:"image_url,omitempty"`
	Price        money.Money          `json:"price,omitempty"`
	UnitPrice    money.Money          `json:"unit_price,omitempty"`
	Unit         string               `json:"unit,omitempty"`
	Package      PackageEvidence      `json:"package,omitempty"`
	Availability AvailabilityEvidence `json:"availability,omitempty"`
	Offers       []OfferEvidence      `json:"offers,omitempty"`
	Nutrition    *NutritionEvidence   `json:"nutrition,omitempty"`
	Sources      []DataProvenance     `json:"sources,omitempty"`
	Confidence   ProductConfidence    `json:"confidence,omitempty"`
	Warnings     []LedgerWarning      `json:"warnings,omitempty"`
}

type PackageEvidence struct {
	RawTexts          []string            `json:"raw_texts,omitempty"`
	NetQuantity       *QuantityRange      `json:"net_quantity,omitempty"`
	DrainedQuantity   *QuantityRange      `json:"drained_quantity,omitempty"`
	UnitQuantity      *NormalizedQuantity `json:"unit_quantity,omitempty"`
	PackCount         *int                `json:"pack_count,omitempty"`
	SalesUnit         string              `json:"sales_unit,omitempty"`
	VariableWeight    bool                `json:"variable_weight,omitempty"`
	ApproximateWeight bool                `json:"approximate_weight,omitempty"`
	PricePerBaseUnit  *float64            `json:"price_per_base_unit,omitempty"`
	BaseUnit          string              `json:"base_unit,omitempty"`
	Confidence        float64             `json:"confidence,omitempty"`
	ParseMethod       string              `json:"parse_method,omitempty"`
	Warnings          []string            `json:"warnings,omitempty"`
}

type AvailabilityEvidence struct {
	Available  *bool   `json:"available,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	Source     string  `json:"source,omitempty"`
}

type OfferEvidence struct {
	Text       string      `json:"text,omitempty"`
	Status     string      `json:"status,omitempty"`
	Confidence float64     `json:"confidence,omitempty"`
	Savings    money.Money `json:"savings,omitempty"`
	Warning    string      `json:"warning,omitempty"`
}

type NutritionEvidence struct {
	Label      *StructuredNutrition `json:"label,omitempty"`
	Source     string               `json:"source,omitempty"`
	Confidence float64              `json:"confidence,omitempty"`
	Warnings   []string             `json:"warnings,omitempty"`
}

type ProductConfidence struct {
	Overall      float64 `json:"overall,omitempty"`
	Detail       float64 `json:"detail,omitempty"`
	Package      float64 `json:"package,omitempty"`
	Nutrition    float64 `json:"nutrition,omitempty"`
	Availability float64 `json:"availability,omitempty"`
}

type IngredientProductAllocation struct {
	RequirementID           string              `json:"requirement_id"`
	ProductID               string              `json:"product_id,omitempty"`
	SKU                     string              `json:"sku,omitempty"`
	ProductName             string              `json:"product_name,omitempty"`
	MatchType               string              `json:"match_type"`
	RequiredQuantity        *NormalizedQuantity `json:"required_quantity,omitempty"`
	TotalRequiredQuantity   *NormalizedQuantity `json:"total_required_quantity,omitempty"`
	PantryAllocatedQuantity *NormalizedQuantity `json:"pantry_allocated_quantity,omitempty"`
	ShopRequiredQuantity    *NormalizedQuantity `json:"shop_required_quantity,omitempty"`
	SourcingStatus          string              `json:"sourcing_status,omitempty"`
	PantryDecision          string              `json:"pantry_decision,omitempty"`
	PurchasedQuantity       *QuantityRange      `json:"purchased_quantity,omitempty"`
	AllocatedQuantity       *QuantityRange      `json:"allocated_quantity,omitempty"`
	MissingQuantity         *NormalizedQuantity `json:"missing_quantity,omitempty"`
	ExcessQuantity          *NormalizedQuantity `json:"excess_quantity,omitempty"`
	PackageCount            int                 `json:"package_count,omitempty"`
	CoverageRatio           float64             `json:"coverage_ratio,omitempty"`
	MatchConfidence         float64             `json:"match_confidence,omitempty"`
	QuantityConfidence      float64             `json:"quantity_confidence,omitempty"`
	OverallConfidence       float64             `json:"overall_confidence,omitempty"`
	Badges                  []string            `json:"badges,omitempty"`
	Warnings                []LedgerWarning     `json:"warnings,omitempty"`
}

type LedgerTotals struct {
	EstimatedTotal         MoneyRange        `json:"estimated_total,omitempty"`
	FixedPriceTotal        money.Money       `json:"fixed_price_total,omitempty"`
	VariableWeightEstimate MoneyRange        `json:"variable_weight_estimate,omitempty"`
	OfferSavings           *money.Money      `json:"offer_savings,omitempty"`
	OfferSavingsConfidence float64           `json:"offer_savings_confidence,omitempty"`
	Lines                  []LedgerPriceLine `json:"lines,omitempty"`
}

type MoneyRange struct {
	Min      *money.Money `json:"min,omitempty"`
	Expected money.Money  `json:"expected,omitempty"`
	Max      *money.Money `json:"max,omitempty"`
	IsExact  bool         `json:"is_exact"`
	Reason   string       `json:"reason,omitempty"`
}

type LedgerPriceLine struct {
	ProductID    string      `json:"product_id,omitempty"`
	SKU          string      `json:"sku,omitempty"`
	ProductName  string      `json:"product_name,omitempty"`
	PackageCount int         `json:"package_count,omitempty"`
	LineTotal    money.Money `json:"line_total,omitempty"`
	Estimated    bool        `json:"estimated,omitempty"`
	Reason       string      `json:"reason,omitempty"`
}

type LedgerNutritionReport struct {
	RequiredEstimate  *NutritionEstimate      `json:"required_estimate,omitempty"`
	PurchasedEstimate *NutritionEstimate      `json:"purchased_estimate,omitempty"`
	Coverage          NutritionCoverageReport `json:"coverage"`
	Warnings          []string                `json:"warnings,omitempty"`
}

type NutritionCoverageReport struct {
	RequirementsTotal              int      `json:"requirements_total"`
	RequirementsWithLabelNutrition int      `json:"requirements_with_label_nutrition"`
	RequirementsWithQuantity       int      `json:"requirements_with_quantity"`
	CalorieCoverageRatio           float64  `json:"calorie_coverage_ratio"`
	MissingNutritionLabels         []string `json:"missing_nutrition_labels,omitempty"`
	SkippedNutritionReasons        []string `json:"skipped_nutrition_reasons,omitempty"`
}

type LedgerWarning struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	Scope   string `json:"scope,omitempty"`
}
