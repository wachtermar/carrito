package food

const (
	FoodRunManifestSchemaVersion   = "1"
	FoodRunManifestKind            = "food_run_manifest"
	ArtifactAuditSchemaVersion     = "1"
	ArtifactAuditKind              = "food_run_artifact_audit"
	ArtifactAuditExitBlocked       = 30
	ArtifactAuditModeOff           = "off"
	ArtifactAuditModeWarn          = "warn"
	ArtifactAuditModeFail          = "fail"
	ArtifactAuditStatusPass        = "pass"
	ArtifactAuditStatusPassWarning = "pass_with_warnings"
	ArtifactAuditStatusFail        = "fail"
)

const (
	FoodArtifactRun                = "run"
	FoodArtifactPlan               = "plan"
	FoodArtifactShop               = "shop"
	FoodArtifactPDF                = "pdf"
	FoodArtifactBasket             = "basket"
	FoodArtifactQuantityLedger     = "quantity_ledger"
	FoodArtifactNutritionLedger    = "nutrition_ledger"
	FoodArtifactServingPlan        = "serving_plan"
	FoodArtifactScaledMealPlan     = "scaled_mealplan"
	FoodArtifactPantry             = "pantry"
	FoodArtifactPantryConsumption  = "pantry_consumption"
	FoodArtifactReadiness          = "readiness"
	FoodArtifactRecovery           = "recovery"
	FoodArtifactRecipeSwap         = "recipe_swap"
	FoodArtifactBasketOptimization = "basket_optimization"
	FoodArtifactRecipeIntake       = "recipe_intake"
	FoodArtifactRecipeQuality      = "recipe_quality"
	FoodArtifactBudgetRepair       = "budget_repair"
	FoodArtifactBudgetDeal         = "budget_deal"
)

type FoodRunManifest struct {
	SchemaVersion        string                    `json:"schema_version"`
	Kind                 string                    `json:"kind"`
	RunID                string                    `json:"run_id,omitempty"`
	CreatedAt            string                    `json:"created_at,omitempty"`
	Command              string                    `json:"command,omitempty"`
	Args                 []string                  `json:"args,omitempty"`
	StoreID              string                    `json:"store_id,omitempty"`
	StoreName            string                    `json:"store_name,omitempty"`
	LiveMode             bool                      `json:"live_mode"`
	SnapshotMode         string                    `json:"snapshot_mode,omitempty"`
	AuditMode            string                    `json:"audit_mode,omitempty"`
	GenerationExitCode   int                       `json:"generation_exit_code"`
	GenerationExitReason string                    `json:"generation_exit_reason,omitempty"`
	ArtifactPaths        map[string]string         `json:"artifact_paths,omitempty"`
	ArtifactHashes       map[string]string         `json:"artifact_hashes,omitempty"`
	ArtifactSizes        map[string]int64          `json:"artifact_sizes,omitempty"`
	Fingerprints         FoodRunFingerprintSummary `json:"fingerprints"`
	Readiness            FoodRunManifestReadiness  `json:"readiness"`
	Snapshot             *ManifestSnapshotSummary  `json:"snapshot,omitempty"`
}

type FoodRunFingerprintSummary struct {
	MealPlan          string `json:"mealplan,omitempty"`
	ProductSelection  string `json:"product_selection,omitempty"`
	HouseholdProfile  string `json:"household_profile,omitempty"`
	ServingPlan       string `json:"serving_plan,omitempty"`
	ScaledMealPlan    string `json:"scaled_mealplan,omitempty"`
	PantryProfile     string `json:"pantry_profile,omitempty"`
	PantryResolution  string `json:"pantry_resolution,omitempty"`
	ShopRequirements  string `json:"shop_requirements,omitempty"`
	NutritionLedger   string `json:"nutrition_ledger,omitempty"`
	RecipeSet         string `json:"recipe_set,omitempty"`
	RecipeQuality     string `json:"recipe_quality,omitempty"`
	RecipeImage       string `json:"recipe_image,omitempty"`
	BudgetRepair      string `json:"budget_repair,omitempty"`
	BudgetDeal        string `json:"budget_deal,omitempty"`
	PrePantryMealPlan string `json:"pre_pantry_mealplan,omitempty"`
}

type FoodRunManifestReadiness struct {
	Status                string `json:"status,omitempty"`
	BasketStatus          string `json:"basket_status,omitempty"`
	SafeToBuild           bool   `json:"safe_to_build"`
	CookStatus            string `json:"cook_status,omitempty"`
	SafeToCook            bool   `json:"safe_to_cook"`
	NutritionStatus       string `json:"nutrition_status,omitempty"`
	SafeToReportNutrition bool   `json:"safe_to_report_nutrition"`
	RecipeQualityStatus   string `json:"recipe_quality_status,omitempty"`
	SafeToUseRecipes      bool   `json:"safe_to_use_recipes"`
	BudgetRepairStatus    string `json:"budget_repair_status,omitempty"`
	BudgetDealStatus      string `json:"budget_deal_status,omitempty"`
	BudgetStatus          string `json:"budget_status,omitempty"`
	SafeToReportBudget    bool   `json:"safe_to_report_budget"`
	SafeToReportDeals     bool   `json:"safe_to_report_deals"`
	GenerationExitCode    int    `json:"generation_exit_code"`
	GenerationExitReason  string `json:"generation_exit_reason,omitempty"`
	FinalLedgerStatus     string `json:"final_ledger_status,omitempty"`
	RequireSafeBasket     bool   `json:"require_safe_basket,omitempty"`
	RequireCookReady      bool   `json:"require_cook_ready,omitempty"`
	RequireNutritionReady bool   `json:"require_nutrition_ready,omitempty"`
	StrictRecipeQuality   bool   `json:"strict_recipe_quality,omitempty"`
	RequireRecipeImages   bool   `json:"require_recipe_images,omitempty"`
	RequireBudgetReady    bool   `json:"require_budget_ready,omitempty"`
}

type FoodRunManifestOptions struct {
	Command              string
	Args                 []string
	StoreID              string
	StoreName            string
	LiveMode             bool
	SnapshotMode         string
	AuditMode            string
	GenerationExitCode   int
	GenerationExitReason string
	ArtifactPaths        map[string]string
	Snapshot             *ManifestSnapshotSummary
}

type ManifestSnapshotSummary struct {
	Mode                 string `json:"mode,omitempty"`
	SnapshotID           string `json:"snapshot_id,omitempty"`
	SnapshotDir          string `json:"snapshot_dir,omitempty"`
	SnapshotManifestPath string `json:"snapshot_manifest_path,omitempty"`
	EntryCount           int    `json:"entry_count,omitempty"`
	ReplayStrict         bool   `json:"replay_strict,omitempty"`
	SnapshotSHA256       string `json:"snapshot_sha256,omitempty"`
	ReplayHits           int    `json:"replay_hits"`
	ReplayMisses         int    `json:"replay_misses"`
}

type ArtifactAuditOptions struct {
	Mode         string
	ContextMode  string
	ManifestPath string
	RunPath      string
	PDFPath      string
	BasketPath   string
}

type ArtifactAuditReport struct {
	SchemaVersion       string               `json:"schema_version"`
	Kind                string               `json:"kind"`
	Status              string               `json:"status"`
	Mode                string               `json:"mode"`
	ContextMode         string               `json:"context_mode,omitempty"`
	RunID               string               `json:"run_id,omitempty"`
	CreatedAt           string               `json:"created_at,omitempty"`
	ManifestPath        string               `json:"manifest_path,omitempty"`
	ManifestHash        string               `json:"manifest_hash,omitempty"`
	RunPath             string               `json:"run_path,omitempty"`
	GenerationExitCode  int                  `json:"generation_exit_code"`
	RecommendedExitCode int                  `json:"recommended_exit_code"`
	Summary             ArtifactAuditSummary `json:"summary"`
	HermesTrustSummary  HermesTrustSummary   `json:"hermes_trust_summary"`
	Metrics             ArtifactAuditMetrics `json:"metrics"`
	Checks              []ArtifactAuditCheck `json:"checks"`
	BlockingIssues      []ArtifactAuditIssue `json:"blocking_issues,omitempty"`
	Warnings            []ArtifactAuditIssue `json:"warnings,omitempty"`
}

type ArtifactAuditSummary struct {
	TotalChecks             int    `json:"total_checks"`
	PassedChecks            int    `json:"passed_checks"`
	WarningChecks           int    `json:"warning_checks"`
	FailedChecks            int    `json:"failed_checks"`
	ArtifactsChecked        int    `json:"artifacts_checked"`
	SafeToBuild             bool   `json:"safe_to_build"`
	SafeToCook              bool   `json:"safe_to_cook"`
	SafeToReportNutrition   bool   `json:"safe_to_report_nutrition"`
	SafeToUseRecipes        bool   `json:"safe_to_use_recipes"`
	SafeToReportBudget      bool   `json:"safe_to_report_budget"`
	SafeToReportDeals       bool   `json:"safe_to_report_deals"`
	BasketActionability     string `json:"basket_actionability,omitempty"`
	PDFTrustSectionsPresent bool   `json:"pdf_trust_sections_present"`
}

type HermesTrustSummary struct {
	Trustworthy                 bool   `json:"trustworthy"`
	MayPresentBasketAsReady     bool   `json:"may_present_basket_as_ready"`
	MayPresentCookReady         bool   `json:"may_present_cook_ready"`
	MayPresentNutritionNumbers  bool   `json:"may_present_nutrition_numbers"`
	MayPresentRecipesAsCookable bool   `json:"may_present_recipes_as_cookable"`
	MayPresentBudgetAsReady     bool   `json:"may_present_budget_as_ready"`
	MayPresentDealsAsReady      bool   `json:"may_present_deals_as_ready"`
	MayPresentPDFAsComplete     bool   `json:"may_present_pdf_as_complete"`
	RequiredUserWarning         string `json:"required_user_warning,omitempty"`
	PrimaryFailureCode          string `json:"primary_failure_code,omitempty"`
	PrimaryFailureMessage       string `json:"primary_failure_message,omitempty"`
}

type ArtifactAuditMetrics struct {
	ManifestArtifacts     int   `json:"manifest_artifacts"`
	BasketActionableLines int   `json:"basket_actionable_lines"`
	BasketDiagnosticLines int   `json:"basket_diagnostic_lines"`
	BasketCommentLines    int   `json:"basket_comment_lines"`
	BasketBlankLines      int   `json:"basket_blank_lines"`
	SelectedProductCount  int   `json:"selected_product_count"`
	PDFSizeBytes          int64 `json:"pdf_size_bytes,omitempty"`
	SnapshotEntryCount    int   `json:"snapshot_entry_count,omitempty"`
	SnapshotReplayHits    int   `json:"snapshot_replay_hits,omitempty"`
	SnapshotReplayMisses  int   `json:"snapshot_replay_misses"`
}

type ArtifactAuditCheck struct {
	Code        string `json:"code"`
	Stage       string `json:"stage,omitempty"`
	Severity    string `json:"severity"`
	Passed      bool   `json:"passed"`
	Artifact    string `json:"artifact,omitempty"`
	JSONPath    string `json:"json_path,omitempty"`
	Expected    any    `json:"expected,omitempty"`
	Actual      any    `json:"actual,omitempty"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

type ArtifactAuditIssue struct {
	Code        string `json:"code"`
	Stage       string `json:"stage,omitempty"`
	Artifact    string `json:"artifact,omitempty"`
	JSONPath    string `json:"json_path,omitempty"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}
