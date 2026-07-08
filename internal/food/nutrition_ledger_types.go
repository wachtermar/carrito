package food

type NutritionLedgerStatus string

const (
	NutritionLedgerNotRun               NutritionLedgerStatus = "not_run"
	NutritionLedgerNoCoverage           NutritionLedgerStatus = "no_coverage"
	NutritionLedgerPartial              NutritionLedgerStatus = "partial"
	NutritionLedgerCompleteForPurchased NutritionLedgerStatus = "complete_for_purchased"
	NutritionLedgerCompleteHybrid       NutritionLedgerStatus = "complete_hybrid"
	NutritionLedgerBlocked              NutritionLedgerStatus = "blocked"
)

const (
	NutritionModeOff    = "off"
	NutritionModeLabels = "labels"
	NutritionModeRecipe = "recipe"
	NutritionModeHybrid = "hybrid"
)

type NutritionPolicy struct {
	Enabled                      bool    `json:"enabled"`
	Mode                         string  `json:"mode"`
	Basis                        string  `json:"basis"`
	IncludePurchasedExcess       bool    `json:"include_purchased_excess"`
	AllowRecipeDeclaredNutrition bool    `json:"allow_recipe_declared_nutrition"`
	AllowPantryProfileNutrition  bool    `json:"allow_pantry_profile_nutrition"`
	AllowBuiltinPantryNutrition  bool    `json:"allow_builtin_pantry_nutrition"`
	RequireNutritionReady        bool    `json:"require_nutrition_ready"`
	MinLineCoverageRatio         float64 `json:"min_line_coverage_ratio,omitempty"`
	MinQuantityCoverageRatio     float64 `json:"min_quantity_coverage_ratio,omitempty"`
	StrictNutritionBasis         bool    `json:"strict_nutrition_basis"`
}

type NutritionLedger struct {
	SchemaVersion               string                           `json:"schema_version"`
	Status                      NutritionLedgerStatus            `json:"status"`
	GeneratedAt                 string                           `json:"generated_at,omitempty"`
	Policy                      NutritionPolicy                  `json:"policy"`
	NutritionLedgerFingerprint  string                           `json:"nutrition_ledger_fingerprint,omitempty"`
	MealPlanFingerprint         string                           `json:"mealplan_fingerprint,omitempty"`
	ServingPlanFingerprint      string                           `json:"serving_plan_fingerprint,omitempty"`
	ScaledMealPlanFingerprint   string                           `json:"scaled_mealplan_fingerprint,omitempty"`
	PantryResolutionFingerprint string                           `json:"pantry_resolution_fingerprint,omitempty"`
	ShopRequirementsFingerprint string                           `json:"shop_requirements_fingerprint,omitempty"`
	ProductSelectionFingerprint string                           `json:"product_selection_fingerprint,omitempty"`
	Coverage                    NutritionCoverageSummary         `json:"coverage"`
	IngredientLines             []NutritionIngredientLine        `json:"ingredient_lines,omitempty"`
	MealSummaries               []NutritionMealSummary           `json:"meal_summaries,omitempty"`
	DaySummaries                []NutritionDaySummary            `json:"day_summaries,omitempty"`
	TotalConsumedKnown          *NutritionRollup                 `json:"total_consumed_known,omitempty"`
	PurchasedExcessKnown        *NutritionRollup                 `json:"purchased_excess_known,omitempty"`
	RecipeDeclaredSummaries     []RecipeDeclaredNutritionSummary `json:"recipe_declared_summaries,omitempty"`
	BlockingIssues              []NutritionIssue                 `json:"blocking_issues,omitempty"`
	Warnings                    []NutritionIssue                 `json:"warnings,omitempty"`
}

type NutrientAmount struct {
	Value      float64 `json:"value"`
	Unit       string  `json:"unit"`
	Comparator string  `json:"comparator,omitempty"`
	Confidence string  `json:"confidence,omitempty"`
	Source     string  `json:"source,omitempty"`
}

type NutritionFacts struct {
	EnergyKJ      *NutrientAmount `json:"energy_kj,omitempty"`
	EnergyKcal    *NutrientAmount `json:"energy_kcal,omitempty"`
	FatG          *NutrientAmount `json:"fat_g,omitempty"`
	SaturatedFatG *NutrientAmount `json:"saturated_fat_g,omitempty"`
	CarbohydrateG *NutrientAmount `json:"carbohydrate_g,omitempty"`
	SugarsG       *NutrientAmount `json:"sugars_g,omitempty"`
	FiberG        *NutrientAmount `json:"fiber_g,omitempty"`
	ProteinG      *NutrientAmount `json:"protein_g,omitempty"`
	SaltG         *NutrientAmount `json:"salt_g,omitempty"`
	SodiumG       *NutrientAmount `json:"sodium_g,omitempty"`
}

type NutritionEvidenceBasis struct {
	Type        NutritionBasis      `json:"type"`
	Quantity    *NormalizedQuantity `json:"quantity,omitempty"`
	ServingSize *NormalizedQuantity `json:"serving_size,omitempty"`
	RawText     string              `json:"raw_text,omitempty"`
	Confidence  string              `json:"confidence"`
}

type ProductNutritionEvidence struct {
	ProductID   string                 `json:"product_id,omitempty"`
	SKU         string                 `json:"sku,omitempty"`
	ProductName string                 `json:"product_name,omitempty"`
	Parsed      bool                   `json:"parsed"`
	Basis       NutritionEvidenceBasis `json:"basis"`
	Facts       NutritionFacts         `json:"facts"`
	RawFields   map[string]string      `json:"raw_fields,omitempty"`
	Confidence  string                 `json:"confidence,omitempty"`
	Warnings    []string               `json:"warnings,omitempty"`
}

type PantryNutritionEvidence struct {
	Basis      NutritionEvidenceBasis `json:"basis"`
	Facts      NutritionFacts         `json:"facts"`
	Source     string                 `json:"source,omitempty"`
	Confidence string                 `json:"confidence,omitempty"`
	Notes      string                 `json:"notes,omitempty"`
}

type NutritionIngredientLine struct {
	IngredientKey           string                    `json:"ingredient_key"`
	IngredientName          string                    `json:"ingredient_name"`
	Day                     int                       `json:"day,omitempty"`
	MealSlot                string                    `json:"meal_slot,omitempty"`
	RecipeID                string                    `json:"recipe_id,omitempty"`
	RecipeTitle             string                    `json:"recipe_title,omitempty"`
	UsageRef                IngredientUsageRef        `json:"usage_ref,omitempty"`
	TotalRequiredQuantity   *NormalizedQuantity       `json:"total_required_quantity,omitempty"`
	PantryAllocatedQuantity *NormalizedQuantity       `json:"pantry_allocated_quantity,omitempty"`
	ShopRequiredQuantity    *NormalizedQuantity       `json:"shop_required_quantity,omitempty"`
	ProductConsumedQuantity *NormalizedQuantity       `json:"product_consumed_quantity,omitempty"`
	PurchasedQuantity       *NormalizedQuantity       `json:"purchased_quantity,omitempty"`
	PurchasedExcessQuantity *NormalizedQuantity       `json:"purchased_excess_quantity,omitempty"`
	SourceStatus            string                    `json:"source_status"`
	ProductID               string                    `json:"product_id,omitempty"`
	SKU                     string                    `json:"sku,omitempty"`
	ProductName             string                    `json:"product_name,omitempty"`
	ProductNutrition        *ProductNutritionEvidence `json:"product_nutrition,omitempty"`
	ConsumedKnown           *NutritionRollup          `json:"consumed_known,omitempty"`
	PurchasedExcessKnown    *NutritionRollup          `json:"purchased_excess_known,omitempty"`
	Coverage                NutritionLineCoverage     `json:"coverage"`
	Warnings                []string                  `json:"warnings,omitempty"`
}

type NutritionLineCoverage struct {
	HasAnyNutrition         bool                `json:"has_any_nutrition"`
	ProductLabelCovered     bool                `json:"product_label_covered"`
	PantryNutritionCovered  bool                `json:"pantry_nutrition_covered"`
	RecipeDeclaredAvailable bool                `json:"recipe_declared_available"`
	CoveredQuantity         *NormalizedQuantity `json:"covered_quantity,omitempty"`
	MissingQuantity         *NormalizedQuantity `json:"missing_quantity,omitempty"`
	CoverageRatioByQuantity *float64            `json:"coverage_ratio_by_quantity,omitempty"`
	MissingReason           string              `json:"missing_reason,omitempty"`
}

type NutritionRollup struct {
	Facts                   NutritionFacts `json:"facts"`
	Source                  string         `json:"source,omitempty"`
	CoverageRatioByLines    float64        `json:"coverage_ratio_by_lines,omitempty"`
	CoverageRatioByQuantity *float64       `json:"coverage_ratio_by_quantity,omitempty"`
	Warnings                []string       `json:"warnings,omitempty"`
}

type NutritionMealSummary struct {
	Day                   int                      `json:"day"`
	MealSlot              string                   `json:"meal_slot"`
	RecipeID              string                   `json:"recipe_id,omitempty"`
	RecipeTitle           string                   `json:"recipe_title,omitempty"`
	TargetServingUnits    float64                  `json:"target_serving_units,omitempty"`
	CookedServingUnits    float64                  `json:"cooked_serving_units,omitempty"`
	TotalKnown            *NutritionRollup         `json:"total_known,omitempty"`
	PerCookedServingKnown *NutritionRollup         `json:"per_cooked_serving_known,omitempty"`
	Coverage              NutritionCoverageSummary `json:"coverage"`
	MissingIngredients    []string                 `json:"missing_ingredients,omitempty"`
	Warnings              []string                 `json:"warnings,omitempty"`
}

type NutritionDaySummary struct {
	Day        int                      `json:"day"`
	Date       string                   `json:"date,omitempty"`
	TotalKnown *NutritionRollup         `json:"total_known,omitempty"`
	Coverage   NutritionCoverageSummary `json:"coverage"`
	Warnings   []string                 `json:"warnings,omitempty"`
}

type NutritionCoverageSummary struct {
	IngredientLines                  int      `json:"ingredient_lines"`
	LinesWithAnyNutrition            int      `json:"lines_with_any_nutrition"`
	LinesWithAlcampoLabel            int      `json:"lines_with_alcampo_label"`
	LinesWithPantryNutrition         int      `json:"lines_with_pantry_nutrition"`
	LinesWithRecipeDeclaredNutrition int      `json:"lines_with_recipe_declared_nutrition"`
	LinesMissingNutrition            int      `json:"lines_missing_nutrition"`
	LinesUnconvertibleBasis          int      `json:"lines_unconvertible_basis"`
	LineCoverageRatio                float64  `json:"line_coverage_ratio"`
	QuantityCoverageRatio            *float64 `json:"quantity_coverage_ratio,omitempty"`
	ProductLabelCoverageRatio        float64  `json:"product_label_coverage_ratio"`
	PantryMissingLines               int      `json:"pantry_missing_lines"`
	MissingIngredients               []string `json:"missing_ingredients,omitempty"`
}

type RecipeDeclaredNutritionSummary struct {
	Day                int              `json:"day"`
	MealSlot           string           `json:"meal_slot"`
	RecipeID           string           `json:"recipe_id,omitempty"`
	RecipeTitle        string           `json:"recipe_title,omitempty"`
	Basis              string           `json:"basis"`
	BaseServings       float64          `json:"base_servings,omitempty"`
	CookedServingUnits float64          `json:"cooked_serving_units,omitempty"`
	TotalScaled        *NutritionRollup `json:"total_scaled,omitempty"`
	PerCookedServing   *NutritionRollup `json:"per_cooked_serving,omitempty"`
	Confidence         string           `json:"confidence,omitempty"`
	Warnings           []string         `json:"warnings,omitempty"`
}

type NutritionIssue struct {
	Code           string `json:"code"`
	Severity       string `json:"severity"`
	IngredientKey  string `json:"ingredient_key,omitempty"`
	IngredientName string `json:"ingredient_name,omitempty"`
	ProductID      string `json:"product_id,omitempty"`
	ProductName    string `json:"product_name,omitempty"`
	Day            int    `json:"day,omitempty"`
	MealSlot       string `json:"meal_slot,omitempty"`
	Message        string `json:"message"`
	Remediation    string `json:"remediation,omitempty"`
}
