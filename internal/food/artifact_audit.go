package food

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/wachtermar/carrito/internal/alcampo"
)

func NormalizeArtifactAuditMode(raw string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		mode = ArtifactAuditModeOff
	}
	switch mode {
	case ArtifactAuditModeOff, ArtifactAuditModeWarn, ArtifactAuditModeFail:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid audit mode %q; expected off, warn, or fail", raw)
	}
}

func BuildFoodRunManifest(artifact FoodRunArtifact, opts FoodRunManifestOptions) (FoodRunManifest, error) {
	mode, err := NormalizeArtifactAuditMode(opts.AuditMode)
	if err != nil {
		return FoodRunManifest{}, err
	}
	paths := normalizedArtifactPaths(opts.ArtifactPaths)
	hashes := make(map[string]string, len(paths))
	sizes := make(map[string]int64, len(paths))
	for _, key := range sortedKeys(paths) {
		path := paths[key]
		hash, size, err := fileSHA256(path)
		if err != nil {
			return FoodRunManifest{}, fmt.Errorf("hash %s artifact %q: %w", key, path, err)
		}
		hashes[key] = hash
		sizes[key] = size
	}
	runID := artifact.MealPlan.ID
	if runID == "" {
		runID = artifact.CreatedAt
	}
	snapshotMode := opts.SnapshotMode
	if snapshotMode == "" && opts.Snapshot != nil {
		snapshotMode = opts.Snapshot.Mode
	}
	readiness := manifestReadinessFromArtifact(artifact)
	readiness.GenerationExitCode = opts.GenerationExitCode
	readiness.GenerationExitReason = opts.GenerationExitReason
	return FoodRunManifest{
		SchemaVersion:        FoodRunManifestSchemaVersion,
		Kind:                 FoodRunManifestKind,
		RunID:                runID,
		CreatedAt:            nowStamp(),
		Command:              opts.Command,
		Args:                 append([]string(nil), opts.Args...),
		StoreID:              opts.StoreID,
		StoreName:            opts.StoreName,
		LiveMode:             opts.LiveMode,
		SnapshotMode:         snapshotMode,
		AuditMode:            mode,
		GenerationExitCode:   opts.GenerationExitCode,
		GenerationExitReason: opts.GenerationExitReason,
		ArtifactPaths:        paths,
		ArtifactHashes:       hashes,
		ArtifactSizes:        sizes,
		Fingerprints:         manifestFingerprintsFromArtifact(artifact),
		Readiness:            readiness,
		Snapshot:             opts.Snapshot,
	}, nil
}

func AuditFoodRunArtifacts(opts ArtifactAuditOptions) (ArtifactAuditReport, error) {
	mode, err := NormalizeArtifactAuditMode(opts.Mode)
	if err != nil {
		return ArtifactAuditReport{}, err
	}
	report := ArtifactAuditReport{
		SchemaVersion: ArtifactAuditSchemaVersion,
		Kind:          ArtifactAuditKind,
		Status:        ArtifactAuditStatusPass,
		Mode:          mode,
		ContextMode:   strings.TrimSpace(opts.ContextMode),
		CreatedAt:     nowStamp(),
		ManifestPath:  opts.ManifestPath,
		RunPath:       opts.RunPath,
	}
	var manifest *FoodRunManifest
	paths := map[string]string{}
	if opts.ManifestPath != "" {
		loaded, hash, ok := auditLoadManifest(&report, opts.ManifestPath)
		if ok {
			manifest = &loaded
			report.ManifestHash = hash
			report.RunID = loaded.RunID
			report.GenerationExitCode = loaded.GenerationExitCode
			report.RecommendedExitCode = loaded.GenerationExitCode
			for key, path := range loaded.ArtifactPaths {
				paths[key] = resolveArtifactPath(opts.ManifestPath, path)
			}
			report.Metrics.ManifestArtifacts = len(loaded.ArtifactPaths)
			auditManifestHashes(&report, loaded, opts.ManifestPath)
		}
	}
	if opts.RunPath != "" {
		paths[FoodArtifactRun] = opts.RunPath
	}
	if opts.PDFPath != "" {
		paths[FoodArtifactPDF] = opts.PDFPath
	}
	if opts.BasketPath != "" {
		paths[FoodArtifactBasket] = opts.BasketPath
	}
	runPath := paths[FoodArtifactRun]
	report.RunPath = runPath
	if strings.TrimSpace(runPath) == "" {
		report.addCheck("run_path_present", "artifact", "blocking", false, FoodArtifactRun, "", "Food run artifact path is missing.", "Run food run with --run-out or validate with --run.")
		report.finalize(nil, nil)
		return report, nil
	}
	var run FoodRunArtifact
	if ok := auditLoadJSON(&report, FoodArtifactRun, runPath, &run); !ok {
		report.finalize(nil, nil)
		return report, nil
	}
	if report.RunID == "" {
		report.RunID = run.MealPlan.ID
	}
	gate := run.ReadinessGate
	auditRunShape(&report, run)
	auditRunFingerprints(&report, run)
	if manifest != nil {
		auditManifestSummary(&report, *manifest, run)
		auditManifestSnapshot(&report, *manifest)
	}
	auditSidecarEquality(&report, run, paths)
	auditDerivedArtifactFingerprints(&report, run)
	basketSummary := auditBasketFile(&report, run, paths[FoodArtifactBasket])
	auditPDFFile(&report, run, paths[FoodArtifactPDF])
	auditLedgerConsistency(&report, run)
	report.finalize(gate, basketSummary)
	return report, nil
}

func manifestFingerprintsFromArtifact(artifact FoodRunArtifact) FoodRunFingerprintSummary {
	return FoodRunFingerprintSummary{
		MealPlan:          artifact.MealPlanFingerprint,
		ProductSelection:  artifact.ProductSelectionFingerprint,
		HouseholdProfile:  artifact.HouseholdProfileFingerprint,
		ServingPlan:       artifact.ServingPlanFingerprint,
		ScaledMealPlan:    artifact.ScaledMealPlanFingerprint,
		PantryProfile:     artifact.PantryProfileFingerprint,
		PantryResolution:  artifact.PantryResolutionFingerprint,
		ShopRequirements:  artifact.ShopRequirementsFingerprint,
		NutritionLedger:   artifact.NutritionLedgerFingerprint,
		RecipeSet:         artifact.RecipeSetFingerprint,
		RecipeQuality:     artifact.RecipeQualityFingerprint,
		RecipeImage:       artifact.RecipeImageFingerprint,
		PrePantryMealPlan: artifact.PrePantryMealPlanFingerprint,
	}
}

func manifestReadinessFromArtifact(artifact FoodRunArtifact) FoodRunManifestReadiness {
	out := FoodRunManifestReadiness{}
	if artifact.BasketSafety != nil {
		out.BasketStatus = string(artifact.BasketSafety.Status)
		out.SafeToBuild = artifact.BasketSafety.SafeToBuild
	}
	if artifact.QuantityLedger != nil {
		out.FinalLedgerStatus = string(artifact.QuantityLedger.Status)
	}
	if artifact.ReadinessGate != nil {
		gate := artifact.ReadinessGate
		out.Status = string(gate.Status)
		out.SafeToBuild = gate.SafeToBuild
		out.CookStatus = gate.CookReadinessStatus
		out.SafeToCook = gate.SafeToCook
		out.NutritionStatus = gate.NutritionStatus
		out.SafeToReportNutrition = gate.SafeToReportNutrition
		out.RecipeQualityStatus = gate.RecipeQualityStatus
		out.SafeToUseRecipes = gate.SafeToUseRecipes
		out.FinalLedgerStatus = gate.FinalLedgerStatus
		out.RequireSafeBasket = gate.Policy.RequireSafeBasket
		out.RequireCookReady = gate.Policy.RequireCookReady
		out.RequireNutritionReady = gate.Policy.RequireNutritionReady
		out.StrictRecipeQuality = gate.Policy.StrictRecipeQuality
		out.RequireRecipeImages = gate.Policy.RequireRecipeImages
	}
	return out
}

func normalizedArtifactPaths(paths map[string]string) map[string]string {
	out := map[string]string{}
	for key, path := range paths {
		key = strings.TrimSpace(key)
		path = strings.TrimSpace(path)
		if key == "" || path == "" {
			continue
		}
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		out[key] = path
	}
	return out
}

func auditLoadManifest(report *ArtifactAuditReport, path string) (FoodRunManifest, string, bool) {
	hash, _, hashErr := fileSHA256(path)
	if hashErr != nil {
		report.addCheck("manifest_file_readable", "manifest", "blocking", false, "manifest", "", "Manifest file is missing or unreadable.", hashErr.Error())
		return FoodRunManifest{}, "", false
	}
	var manifest FoodRunManifest
	if ok := auditLoadJSON(report, "manifest", path, &manifest); !ok {
		return FoodRunManifest{}, hash, false
	}
	report.addCheck("manifest_kind", "manifest", "blocking", manifest.Kind == FoodRunManifestKind, "manifest", "$.kind", "Manifest kind must be food_run_manifest.", "Regenerate the manifest with carrito food run --manifest-out.", FoodRunManifestKind, manifest.Kind)
	report.addCheck("manifest_schema", "manifest", "blocking", manifest.SchemaVersion == FoodRunManifestSchemaVersion, "manifest", "$.schema_version", "Manifest schema version must be supported.", "Regenerate the manifest with the current carrito binary.", FoodRunManifestSchemaVersion, manifest.SchemaVersion)
	return manifest, hash, true
}

func auditManifestHashes(report *ArtifactAuditReport, manifest FoodRunManifest, manifestPath string) {
	for _, key := range sortedKeys(manifest.ArtifactPaths) {
		path := resolveArtifactPath(manifestPath, manifest.ArtifactPaths[key])
		hash, size, err := fileSHA256(path)
		if err != nil {
			report.addCheck("artifact_file_readable", "manifest", "blocking", false, key, "", "Manifest-listed artifact is missing or unreadable.", err.Error(), manifest.ArtifactPaths[key], path)
			continue
		}
		report.Summary.ArtifactsChecked++
		if expected := manifest.ArtifactHashes[key]; expected != "" {
			report.addCheck("artifact_hash_matches", "manifest", "blocking", expected == hash, key, "$.artifact_hashes."+key, "Artifact hash must match the manifest.", "Regenerate the artifact bundle; do not use stale sidecars.", expected, hash)
		}
		if expected, ok := manifest.ArtifactSizes[key]; ok {
			report.addCheck("artifact_size_matches", "manifest", "blocking", expected == size, key, "$.artifact_sizes."+key, "Artifact size must match the manifest.", "Regenerate the artifact bundle; do not use stale sidecars.", expected, size)
		}
	}
}

func auditRunShape(report *ArtifactAuditReport, run FoodRunArtifact) {
	report.addCheck("run_kind", "artifact", "blocking", run.Kind == "food_run", FoodArtifactRun, "$.kind", "Run artifact kind must be food_run.", "Regenerate the food run artifact.", "food_run", run.Kind)
	report.addCheck("run_has_mealplan", "artifact", "blocking", len(run.MealPlan.Days) > 0, FoodArtifactRun, "$.mealplan.days", "Run artifact must contain a meal plan.", "Regenerate the food run.")
	report.addCheck("run_has_readiness_gate", "artifact", "blocking", run.ReadinessGate != nil, FoodArtifactRun, "$.readiness_gate", "Run artifact must contain the final readiness gate.", "Regenerate the food run with readiness output.")
	report.addCheck("run_has_quantity_ledger", "artifact", "blocking", run.QuantityLedger != nil, FoodArtifactRun, "$.quantity_ledger", "Run artifact must contain the final quantity ledger.", "Regenerate the food run.")
	report.Metrics.SelectedProductCount = actionableSelectedProductCount(run.Shop)
}

func auditRunFingerprints(report *ArtifactAuditReport, run FoodRunArtifact) {
	expectedMeal := MealPlanFingerprint(run.MealPlan)
	if run.MealPlanFingerprint == "" {
		report.addCheck("mealplan_fingerprint_present", "fingerprint", "warning", false, FoodArtifactRun, "$.mealplan_fingerprint", "Run artifact has no meal-plan fingerprint.", "Regenerate the food run.")
	} else {
		report.addCheck("mealplan_fingerprint_matches", "fingerprint", "blocking", run.MealPlanFingerprint == expectedMeal, FoodArtifactRun, "$.mealplan_fingerprint", "Run meal-plan fingerprint must match the embedded meal plan.", "Regenerate all artifacts from the final meal plan.", expectedMeal, run.MealPlanFingerprint)
	}
	expectedSelection := ProductSelectionFingerprint(run.Shop)
	if run.ProductSelectionFingerprint == "" {
		report.addCheck("product_selection_fingerprint_present", "fingerprint", "warning", false, FoodArtifactRun, "$.product_selection_fingerprint", "Run artifact has no product-selection fingerprint.", "Regenerate the food run.")
	} else {
		report.addCheck("product_selection_fingerprint_matches", "fingerprint", "blocking", run.ProductSelectionFingerprint == expectedSelection, FoodArtifactRun, "$.product_selection_fingerprint", "Run product-selection fingerprint must match the embedded selected products.", "Regenerate all artifacts from the final shopping result.", expectedSelection, run.ProductSelectionFingerprint)
	}
	if run.NutritionLedger != nil {
		expectedNutrition := NutritionLedgerFingerprint(*run.NutritionLedger)
		report.addCheck("nutrition_ledger_fingerprint_matches", "fingerprint", "blocking", run.NutritionLedger.NutritionLedgerFingerprint == expectedNutrition, FoodArtifactRun, "$.nutrition_ledger.nutrition_ledger_fingerprint", "Nutrition ledger fingerprint must match the embedded nutrition ledger.", "Regenerate the nutrition ledger.", expectedNutrition, run.NutritionLedger.NutritionLedgerFingerprint)
		if run.NutritionLedgerFingerprint != "" {
			report.addCheck("run_nutrition_fingerprint_matches_ledger", "fingerprint", "blocking", run.NutritionLedgerFingerprint == run.NutritionLedger.NutritionLedgerFingerprint, FoodArtifactRun, "$.nutrition_ledger_fingerprint", "Run nutrition ledger fingerprint must match the embedded nutrition ledger.", "Regenerate the food run.", run.NutritionLedger.NutritionLedgerFingerprint, run.NutritionLedgerFingerprint)
		}
	}
	if run.RecipeQualityReport != nil {
		expectedSet := RecipeSetFingerprint(run.MealPlan)
		expectedQuality := RecipeQualityFingerprint(*run.RecipeQualityReport)
		expectedImage := RecipeImageFingerprint(*run.RecipeQualityReport)
		report.addCheck("recipe_set_fingerprint_matches", "fingerprint", "blocking", run.RecipeSetFingerprint == expectedSet, FoodArtifactRun, "$.recipe_set_fingerprint", "Run recipe-set fingerprint must match the embedded meal plan recipes.", "Regenerate recipe quality and the food run after recipe changes.", expectedSet, run.RecipeSetFingerprint)
		report.addCheck("recipe_quality_fingerprint_matches", "fingerprint", "blocking", run.RecipeQualityReport.RecipeQualityFingerprint == expectedQuality, FoodArtifactRun, "$.recipe_quality_report.recipe_quality_fingerprint", "Recipe quality fingerprint must match the embedded report.", "Regenerate recipe quality sidecars and the run artifact.", expectedQuality, run.RecipeQualityReport.RecipeQualityFingerprint)
		report.addCheck("run_recipe_quality_fingerprint_matches_report", "fingerprint", "blocking", run.RecipeQualityFingerprint == run.RecipeQualityReport.RecipeQualityFingerprint, FoodArtifactRun, "$.recipe_quality_fingerprint", "Run recipe quality fingerprint must match the embedded report.", "Regenerate the food run.", run.RecipeQualityReport.RecipeQualityFingerprint, run.RecipeQualityFingerprint)
		report.addCheck("recipe_image_fingerprint_matches", "fingerprint", "blocking", run.RecipeQualityReport.RecipeImageFingerprint == expectedImage, FoodArtifactRun, "$.recipe_quality_report.recipe_image_fingerprint", "Recipe image fingerprint must match image evidence.", "Regenerate recipe quality after image cache changes.", expectedImage, run.RecipeQualityReport.RecipeImageFingerprint)
		if run.RecipeImageFingerprint != "" {
			report.addCheck("run_recipe_image_fingerprint_matches_report", "fingerprint", "blocking", run.RecipeImageFingerprint == run.RecipeQualityReport.RecipeImageFingerprint, FoodArtifactRun, "$.recipe_image_fingerprint", "Run recipe image fingerprint must match the embedded report.", "Regenerate the food run.", run.RecipeQualityReport.RecipeImageFingerprint, run.RecipeImageFingerprint)
		}
	}
}

func auditManifestSummary(report *ArtifactAuditReport, manifest FoodRunManifest, run FoodRunArtifact) {
	expected := manifestFingerprintsFromArtifact(run)
	checkManifestFingerprint(report, "mealplan", "$.fingerprints.mealplan", manifest.Fingerprints.MealPlan, expected.MealPlan)
	checkManifestFingerprint(report, "product_selection", "$.fingerprints.product_selection", manifest.Fingerprints.ProductSelection, expected.ProductSelection)
	checkManifestFingerprint(report, "serving_plan", "$.fingerprints.serving_plan", manifest.Fingerprints.ServingPlan, expected.ServingPlan)
	checkManifestFingerprint(report, "scaled_mealplan", "$.fingerprints.scaled_mealplan", manifest.Fingerprints.ScaledMealPlan, expected.ScaledMealPlan)
	checkManifestFingerprint(report, "pantry_resolution", "$.fingerprints.pantry_resolution", manifest.Fingerprints.PantryResolution, expected.PantryResolution)
	checkManifestFingerprint(report, "shop_requirements", "$.fingerprints.shop_requirements", manifest.Fingerprints.ShopRequirements, expected.ShopRequirements)
	checkManifestFingerprint(report, "nutrition_ledger", "$.fingerprints.nutrition_ledger", manifest.Fingerprints.NutritionLedger, expected.NutritionLedger)
	checkManifestFingerprint(report, "recipe_set", "$.fingerprints.recipe_set", manifest.Fingerprints.RecipeSet, expected.RecipeSet)
	checkManifestFingerprint(report, "recipe_quality", "$.fingerprints.recipe_quality", manifest.Fingerprints.RecipeQuality, expected.RecipeQuality)
	checkManifestFingerprint(report, "recipe_image", "$.fingerprints.recipe_image", manifest.Fingerprints.RecipeImage, expected.RecipeImage)
	expectedReadiness := manifestReadinessFromArtifact(run)
	report.addCheck("manifest_readiness_status_matches", "manifest", "blocking", manifest.Readiness.Status == expectedReadiness.Status, "manifest", "$.readiness.status", "Manifest readiness status must match the run readiness gate.", "Regenerate the manifest after the final run artifact.", expectedReadiness.Status, manifest.Readiness.Status)
	report.addCheck("manifest_safe_to_build_matches", "manifest", "blocking", manifest.Readiness.SafeToBuild == expectedReadiness.SafeToBuild, "manifest", "$.readiness.safe_to_build", "Manifest safe_to_build must match the run readiness gate.", "Regenerate the manifest after the final run artifact.", expectedReadiness.SafeToBuild, manifest.Readiness.SafeToBuild)
	report.addCheck("manifest_safe_to_cook_matches", "manifest", "blocking", manifest.Readiness.SafeToCook == expectedReadiness.SafeToCook, "manifest", "$.readiness.safe_to_cook", "Manifest safe_to_cook must match the run readiness gate.", "Regenerate the manifest after the final run artifact.", expectedReadiness.SafeToCook, manifest.Readiness.SafeToCook)
	report.addCheck("manifest_safe_to_report_nutrition_matches", "manifest", "blocking", manifest.Readiness.SafeToReportNutrition == expectedReadiness.SafeToReportNutrition, "manifest", "$.readiness.safe_to_report_nutrition", "Manifest safe_to_report_nutrition must match the run readiness gate.", "Regenerate the manifest after the final run artifact.", expectedReadiness.SafeToReportNutrition, manifest.Readiness.SafeToReportNutrition)
	report.addCheck("manifest_safe_to_use_recipes_matches", "manifest", "blocking", manifest.Readiness.SafeToUseRecipes == expectedReadiness.SafeToUseRecipes, "manifest", "$.readiness.safe_to_use_recipes", "Manifest safe_to_use_recipes must match the run readiness gate.", "Regenerate the manifest after the final run artifact.", expectedReadiness.SafeToUseRecipes, manifest.Readiness.SafeToUseRecipes)
	if run.ReadinessGate != nil {
		expectedExit := expectedGenerationExitCode(*run.ReadinessGate)
		report.addCheck("manifest_generation_exit_consistent", "manifest", "blocking", manifest.GenerationExitCode == expectedExit, "manifest", "$.generation_exit_code", "Manifest generation exit code must match readiness policy.", "Regenerate the manifest after readiness evaluation.", expectedExit, manifest.GenerationExitCode)
	}
}

func auditManifestSnapshot(report *ArtifactAuditReport, manifest FoodRunManifest) {
	if manifest.Snapshot == nil || strings.TrimSpace(manifest.Snapshot.Mode) == "" {
		return
	}
	summary := manifest.Snapshot
	report.Metrics.SnapshotEntryCount = summary.EntryCount
	report.Metrics.SnapshotReplayHits = summary.ReplayHits
	report.Metrics.SnapshotReplayMisses = summary.ReplayMisses
	path := strings.TrimSpace(summary.SnapshotManifestPath)
	if path == "" {
		report.addCheck("snapshot_manifest_path_present", "snapshot", "blocking", false, "snapshot", "$.snapshot.snapshot_manifest_path", "Snapshot manifest path must be present when snapshot mode is active.", "Regenerate the food run with snapshot flags.")
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		report.addCheck("snapshot_manifest_readable", "snapshot", "blocking", false, "snapshot", "$.snapshot.snapshot_manifest_path", "Snapshot manifest is missing or unreadable.", err.Error())
		return
	}
	hash := sha256.Sum256(data)
	gotManifestHash := hex.EncodeToString(hash[:])
	if summary.SnapshotSHA256 != "" {
		report.addCheck("snapshot_manifest_hash_matches", "snapshot", "blocking", summary.SnapshotSHA256 == gotManifestHash, "snapshot", "$.snapshot.snapshot_sha256", "Snapshot manifest hash must match.", "Regenerate the manifest after snapshot recording/replay.", summary.SnapshotSHA256, gotManifestHash)
	}
	var snap alcampo.LiveSnapshotManifest
	if err := json.Unmarshal(data, &snap); err != nil {
		report.addCheck("snapshot_manifest_valid", "snapshot", "blocking", false, "snapshot", "$.snapshot", "Snapshot manifest JSON must be valid.", err.Error())
		return
	}
	report.addCheck("snapshot_manifest_schema", "snapshot", "blocking", snap.SchemaVersion == alcampo.LiveSnapshotSchemaVersion, "snapshot", "$.snapshot.schema_version", "Snapshot manifest schema must be supported.", "Regenerate the snapshot with the current carrito binary.", alcampo.LiveSnapshotSchemaVersion, snap.SchemaVersion)
	report.addCheck("snapshot_entry_count_matches", "snapshot", "blocking", snap.EntryCount == summary.EntryCount && snap.EntryCount == len(snap.Entries), "snapshot", "$.snapshot.entry_count", "Snapshot entry count must match manifest entries.", "Regenerate the snapshot.", summary.EntryCount, snap.EntryCount)
	if summary.Mode == alcampo.LiveSnapshotModeReplay && summary.ReplayStrict {
		report.addCheck("snapshot_replay_no_misses", "snapshot", "blocking", summary.ReplayMisses == 0, "snapshot", "$.snapshot.replay_misses", "Strict snapshot replay must have zero misses.", "Record a new snapshot or run with the matching input.", 0, summary.ReplayMisses)
	}
	blockingIssues := 0
	for _, issue := range snap.Warnings {
		if strings.EqualFold(issue.Severity, "blocking") {
			blockingIssues++
		}
	}
	report.addCheck("snapshot_no_blocking_issues", "snapshot", "blocking", blockingIssues == 0, "snapshot", "$.warnings", "Snapshot manifest must not contain blocking issues.", "Regenerate the snapshot after resolving policy failures.", 0, blockingIssues)
	baseDir := filepath.Dir(path)
	for _, entry := range snap.Entries {
		auditSnapshotResponseEntry(report, baseDir, entry)
	}
}

func auditSnapshotResponseEntry(report *ArtifactAuditReport, baseDir string, entry alcampo.LiveSnapshotEntry) {
	path := filepath.Join(baseDir, entry.ResponsePath)
	data, err := os.ReadFile(path)
	if err != nil {
		report.addCheck("snapshot_response_readable", "snapshot", "blocking", false, "snapshot", "$.entries.response_path", "Snapshot response file is missing or unreadable.", err.Error())
		return
	}
	var response struct {
		BodySHA256 string `json:"body_sha256"`
		BodyBase64 string `json:"body_base64"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		report.addCheck("snapshot_response_valid", "snapshot", "blocking", false, "snapshot", "$.entries.response_path", "Snapshot response file JSON must be valid.", err.Error())
		return
	}
	body, err := base64.StdEncoding.DecodeString(response.BodyBase64)
	if err != nil {
		report.addCheck("snapshot_response_body_base64", "snapshot", "blocking", false, "snapshot", "$.entries.response_path", "Snapshot response body must be base64 encoded.", err.Error())
		return
	}
	sum := sha256.Sum256(body)
	bodyHash := hex.EncodeToString(sum[:])
	report.addCheck("snapshot_response_hash_matches_entry", "snapshot", "blocking", bodyHash == entry.ResponseSHA256 && bodyHash == response.BodySHA256, "snapshot", "$.entries.response_sha256", "Snapshot response body hash must match the manifest entry.", "Regenerate the snapshot; the response file was modified.", entry.ResponseSHA256, bodyHash)
}

func checkManifestFingerprint(report *ArtifactAuditReport, name, jsonPath, actual, expected string) {
	if expected == "" && actual == "" {
		report.addCheck("manifest_"+name+"_fingerprint_empty", "manifest", "info", true, "manifest", jsonPath, "Optional manifest fingerprint is absent.", "")
		return
	}
	report.addCheck("manifest_"+name+"_fingerprint_matches", "manifest", "blocking", actual == expected, "manifest", jsonPath, "Manifest fingerprint must match the run artifact.", "Regenerate the manifest after final artifact generation.", expected, actual)
}

func auditSidecarEquality(report *ArtifactAuditReport, run FoodRunArtifact, paths map[string]string) {
	auditValueSidecar(report, FoodArtifactPlan, paths[FoodArtifactPlan], run.MealPlan)
	auditValueSidecar(report, FoodArtifactShop, paths[FoodArtifactShop], run.Shop)
	auditPointerSidecar(report, FoodArtifactQuantityLedger, paths[FoodArtifactQuantityLedger], run.QuantityLedger)
	auditPointerSidecar(report, FoodArtifactNutritionLedger, paths[FoodArtifactNutritionLedger], run.NutritionLedger)
	auditPointerSidecar(report, FoodArtifactServingPlan, paths[FoodArtifactServingPlan], run.ServingPlan)
	auditPointerSidecar(report, FoodArtifactScaledMealPlan, paths[FoodArtifactScaledMealPlan], run.ScaledMealPlan)
	auditPointerSidecar(report, FoodArtifactPantry, paths[FoodArtifactPantry], run.PantryResolution)
	auditPointerSidecar(report, FoodArtifactPantryConsumption, paths[FoodArtifactPantryConsumption], run.PantryConsumptionPlan)
	auditPointerSidecar(report, FoodArtifactReadiness, paths[FoodArtifactReadiness], run.ReadinessGate)
	auditPointerSidecar(report, FoodArtifactRecovery, paths[FoodArtifactRecovery], run.RecoveryPlan)
	auditPointerSidecar(report, FoodArtifactRecipeSwap, paths[FoodArtifactRecipeSwap], run.RecipeSwapPlan)
	auditPointerSidecar(report, FoodArtifactBasketOptimization, paths[FoodArtifactBasketOptimization], run.BasketOptimizationPlan)
	auditPointerSidecar(report, FoodArtifactRecipeIntake, paths[FoodArtifactRecipeIntake], run.RecipeIntakePlan)
	auditPointerSidecar(report, FoodArtifactRecipeQuality, paths[FoodArtifactRecipeQuality], run.RecipeQualityReport)
}

func auditDerivedArtifactFingerprints(report *ArtifactAuditReport, run FoodRunArtifact) {
	expectedMeal := run.MealPlanFingerprint
	expectedSelection := run.ProductSelectionFingerprint
	expectedServing := run.ServingPlanFingerprint
	expectedScaled := run.ScaledMealPlanFingerprint
	expectedPantry := run.PantryResolutionFingerprint
	expectedShopReqs := run.ShopRequirementsFingerprint
	if run.QuantityLedger != nil {
		checkFingerprint(report, FoodArtifactQuantityLedger, "$.mealplan_fingerprint", "quantity_ledger_mealplan_fingerprint_matches", run.QuantityLedger.MealPlanFingerprint, expectedMeal)
		checkFingerprint(report, FoodArtifactQuantityLedger, "$.product_selection_fingerprint", "quantity_ledger_selection_fingerprint_matches", run.QuantityLedger.ProductSelectionFingerprint, expectedSelection)
		checkFingerprint(report, FoodArtifactQuantityLedger, "$.serving_plan_fingerprint", "quantity_ledger_serving_fingerprint_matches", run.QuantityLedger.ServingPlanFingerprint, expectedServing)
		checkFingerprint(report, FoodArtifactQuantityLedger, "$.scaled_mealplan_fingerprint", "quantity_ledger_scaled_fingerprint_matches", run.QuantityLedger.ScaledMealPlanFingerprint, expectedScaled)
		checkFingerprint(report, FoodArtifactQuantityLedger, "$.pantry_resolution_fingerprint", "quantity_ledger_pantry_fingerprint_matches", run.QuantityLedger.PantryResolutionFingerprint, expectedPantry)
		checkFingerprint(report, FoodArtifactQuantityLedger, "$.shop_requirements_fingerprint", "quantity_ledger_shop_requirements_fingerprint_matches", run.QuantityLedger.ShopRequirementsFingerprint, expectedShopReqs)
	}
	if run.BasketSafety != nil {
		checkFingerprint(report, "basket_safety", "$.mealplan_fingerprint", "basket_safety_mealplan_fingerprint_matches", run.BasketSafety.MealPlanFingerprint, expectedMeal)
		checkFingerprint(report, "basket_safety", "$.product_selection_fingerprint", "basket_safety_selection_fingerprint_matches", run.BasketSafety.ProductSelectionFingerprint, expectedSelection)
		checkFingerprint(report, "basket_safety", "$.serving_plan_fingerprint", "basket_safety_serving_fingerprint_matches", run.BasketSafety.ServingPlanFingerprint, expectedServing)
		checkFingerprint(report, "basket_safety", "$.scaled_mealplan_fingerprint", "basket_safety_scaled_fingerprint_matches", run.BasketSafety.ScaledMealPlanFingerprint, expectedScaled)
		checkFingerprint(report, "basket_safety", "$.pantry_resolution_fingerprint", "basket_safety_pantry_fingerprint_matches", run.BasketSafety.PantryResolutionFingerprint, expectedPantry)
		checkFingerprint(report, "basket_safety", "$.shop_requirements_fingerprint", "basket_safety_shop_requirements_fingerprint_matches", run.BasketSafety.ShopRequirementsFingerprint, expectedShopReqs)
	}
	if run.ReadinessGate != nil {
		checkFingerprint(report, FoodArtifactReadiness, "$.mealplan_fingerprint", "readiness_mealplan_fingerprint_matches", run.ReadinessGate.MealPlanFingerprint, expectedMeal)
		checkFingerprint(report, FoodArtifactReadiness, "$.product_selection_fingerprint", "readiness_selection_fingerprint_matches", run.ReadinessGate.ProductSelectionFingerprint, expectedSelection)
		checkFingerprint(report, FoodArtifactReadiness, "$.serving_plan_fingerprint", "readiness_serving_fingerprint_matches", run.ReadinessGate.ServingPlanFingerprint, expectedServing)
		checkFingerprint(report, FoodArtifactReadiness, "$.scaled_mealplan_fingerprint", "readiness_scaled_fingerprint_matches", run.ReadinessGate.ScaledMealPlanFingerprint, expectedScaled)
		checkFingerprint(report, FoodArtifactReadiness, "$.pantry_resolution_fingerprint", "readiness_pantry_fingerprint_matches", run.ReadinessGate.PantryResolutionFingerprint, expectedPantry)
		checkFingerprint(report, FoodArtifactReadiness, "$.shop_requirements_fingerprint", "readiness_shop_requirements_fingerprint_matches", run.ReadinessGate.ShopRequirementsFingerprint, expectedShopReqs)
		checkFingerprint(report, FoodArtifactReadiness, "$.nutrition_ledger_fingerprint", "readiness_nutrition_fingerprint_matches", run.ReadinessGate.NutritionLedgerFingerprint, run.NutritionLedgerFingerprint)
		checkFingerprint(report, FoodArtifactReadiness, "$.recipe_set_fingerprint", "readiness_recipe_set_fingerprint_matches", run.ReadinessGate.RecipeSetFingerprint, run.RecipeSetFingerprint)
		checkFingerprint(report, FoodArtifactReadiness, "$.recipe_quality_fingerprint", "readiness_recipe_quality_fingerprint_matches", run.ReadinessGate.RecipeQualityFingerprint, run.RecipeQualityFingerprint)
		checkFingerprint(report, FoodArtifactReadiness, "$.recipe_image_fingerprint", "readiness_recipe_image_fingerprint_matches", run.ReadinessGate.RecipeImageFingerprint, run.RecipeImageFingerprint)
		report.addCheck("readiness_safe_to_build_matches_basket_safety", "readiness", "blocking", run.BasketSafety == nil || run.ReadinessGate.SafeToBuild == run.BasketSafety.SafeToBuild, FoodArtifactReadiness, "$.safe_to_build", "Readiness safe_to_build must match basket safety.", "Regenerate readiness after basket safety evaluation.")
	}
	if run.RecipeQualityReport != nil {
		checkFingerprint(report, FoodArtifactRecipeQuality, "$.recipe_set_fingerprint", "recipe_quality_set_fingerprint_matches", run.RecipeQualityReport.RecipeSetFingerprint, run.RecipeSetFingerprint)
		checkFingerprint(report, FoodArtifactRecipeQuality, "$.recipe_quality_fingerprint", "recipe_quality_run_fingerprint_matches", run.RecipeQualityReport.RecipeQualityFingerprint, run.RecipeQualityFingerprint)
		checkFingerprint(report, FoodArtifactRecipeQuality, "$.recipe_image_fingerprint", "recipe_quality_image_fingerprint_matches", run.RecipeQualityReport.RecipeImageFingerprint, run.RecipeImageFingerprint)
		if run.RecipeQualityReport.Policy.RequireRecipeImages {
			missing := 0
			for _, ev := range run.RecipeQualityReport.ImageEvidence {
				if ev.Status != RecipeImageCached && ev.Status != RecipeImageLocal {
					missing++
				}
			}
			report.addCheck("required_recipe_images_cached", "recipe_quality", "blocking", missing == 0, FoodArtifactRecipeQuality, "$.image_evidence", "Required recipe images must have local or cached evidence.", "Fix image URLs or rerun recipe intake with image caching.", 0, missing)
		}
	}
	if run.NutritionLedger != nil {
		checkFingerprint(report, FoodArtifactNutritionLedger, "$.mealplan_fingerprint", "nutrition_mealplan_fingerprint_matches", run.NutritionLedger.MealPlanFingerprint, expectedMeal)
		checkFingerprint(report, FoodArtifactNutritionLedger, "$.product_selection_fingerprint", "nutrition_selection_fingerprint_matches", run.NutritionLedger.ProductSelectionFingerprint, expectedSelection)
		checkFingerprint(report, FoodArtifactNutritionLedger, "$.serving_plan_fingerprint", "nutrition_serving_fingerprint_matches", run.NutritionLedger.ServingPlanFingerprint, expectedServing)
		checkFingerprint(report, FoodArtifactNutritionLedger, "$.scaled_mealplan_fingerprint", "nutrition_scaled_fingerprint_matches", run.NutritionLedger.ScaledMealPlanFingerprint, expectedScaled)
		checkFingerprint(report, FoodArtifactNutritionLedger, "$.pantry_resolution_fingerprint", "nutrition_pantry_fingerprint_matches", run.NutritionLedger.PantryResolutionFingerprint, expectedPantry)
		checkFingerprint(report, FoodArtifactNutritionLedger, "$.shop_requirements_fingerprint", "nutrition_shop_requirements_fingerprint_matches", run.NutritionLedger.ShopRequirementsFingerprint, expectedShopReqs)
	}
}

func auditBasketFile(report *ArtifactAuditReport, run FoodRunArtifact, path string) *basketAuditSummary {
	if strings.TrimSpace(path) == "" {
		report.addCheck("basket_file_present", "basket", "warning", false, FoodArtifactBasket, "", "No basket file was provided for audit.", "Run with --basket-out when Hermes needs a cart-prep basket.")
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		report.addCheck("basket_file_readable", "basket", "blocking", false, FoodArtifactBasket, "", "Basket file is missing or unreadable.", err.Error())
		return nil
	}
	summary := classifyBasketText(string(data))
	report.Metrics.BasketActionableLines = summary.ActionableLines
	report.Metrics.BasketDiagnosticLines = summary.DiagnosticLines
	report.Metrics.BasketCommentLines = summary.CommentLines
	report.Metrics.BasketBlankLines = summary.BlankLines
	gate := run.ReadinessGate
	if gate == nil {
		return &summary
	}
	if !gate.SafeToBuild {
		report.addCheck("unsafe_basket_has_no_actionable_lines", "basket", "blocking", summary.ActionableLines == 0, FoodArtifactBasket, "", "Unsafe basket must not contain actionable SKU lines.", "Regenerate the basket; unsafe runs must be comment-only.", 0, summary.ActionableLines)
		report.addCheck("unsafe_basket_has_not_safe_marker", "basket", "blocking", summary.HasNotSafeMarker, FoodArtifactBasket, "", "Unsafe basket must include the NOT SAFE TO BUILD marker.", "Regenerate the basket with readiness guards.")
		report.Summary.BasketActionability = "comment_only"
		return &summary
	}
	report.addCheck("safe_basket_has_no_not_safe_marker", "basket", "blocking", !summary.HasNotSafeMarker, FoodArtifactBasket, "", "Safe basket must not include the NOT SAFE marker.", "Regenerate the basket from the final readiness state.")
	selected := actionableSelectedProductCount(run.Shop)
	if selected > 0 {
		report.addCheck("safe_basket_has_actionable_lines", "basket", "blocking", summary.ActionableLines > 0, FoodArtifactBasket, "", "Safe basket with selected products must contain actionable SKU lines.", "Regenerate the basket from final shopping output.")
	} else if run.PantryResolution != nil && run.PantryResolution.Summary.ShopIngredientLines == 0 && gate.SafeToCook {
		report.addCheck("all_pantry_basket_has_no_actionable_lines", "basket", "blocking", summary.ActionableLines == 0, FoodArtifactBasket, "", "All-pantry basket must not contain SKU lines.", "Regenerate the basket from final pantry resolution.", 0, summary.ActionableLines)
		report.addCheck("all_pantry_basket_has_marker", "basket", "blocking", summary.HasAllPantryMarker, FoodArtifactBasket, "", "All-pantry basket must state that no Alcampo items are needed.", "Regenerate the basket with the current carrito binary.")
	} else {
		report.addCheck("safe_empty_basket_explained", "basket", "warning", summary.HasAllPantryMarker, FoodArtifactBasket, "", "Safe basket has no actionable SKU lines and no all-pantry explanation.", "Review whether all ingredients are pantry-covered.")
	}
	if !gate.SafeToCook {
		report.addCheck("cook_blocked_basket_has_warning", "basket", "blocking", summary.HasCookWarning, FoodArtifactBasket, "", "Buildable but cook-blocked basket must include a cooking readiness warning.", "Regenerate the basket with readiness guards.")
	}
	if gate.Policy.RequireNutritionReady && !gate.SafeToReportNutrition {
		report.addCheck("nutrition_blocked_basket_has_warning", "basket", "blocking", summary.HasNutritionWarning, FoodArtifactBasket, "", "Buildable but nutrition-blocked basket must include a nutrition readiness warning.", "Regenerate the basket with readiness guards.")
	}
	report.Summary.BasketActionability = "actionable"
	if selected == 0 {
		report.Summary.BasketActionability = "all_pantry"
	}
	return &summary
}

func auditPDFFile(report *ArtifactAuditReport, run FoodRunArtifact, path string) {
	if strings.TrimSpace(path) == "" {
		report.addCheck("pdf_file_present", "pdf", "warning", false, FoodArtifactPDF, "", "No PDF file was provided for audit.", "Run with --pdf-out when Hermes needs a user-facing meal plan PDF.")
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		report.addCheck("pdf_file_readable", "pdf", "blocking", false, FoodArtifactPDF, "", "PDF file is missing or unreadable.", err.Error())
		return
	}
	report.Metrics.PDFSizeBytes = int64(len(data))
	report.addCheck("pdf_minimum_size", "pdf", "warning", len(data) >= 10*1024, FoodArtifactPDF, "", "PDF is smaller than the expected 10 KB smoke threshold.", "Open or regenerate the PDF if it looks truncated.", ">=10240", len(data))
	markers := []string{"Basket readiness:", "Cooking readiness:", "Basket safety:"}
	if run.NutritionLedger != nil && run.NutritionLedger.Status != NutritionLedgerNotRun {
		markers = append(markers, "Nutrition evidence:")
	}
	if run.RecipeQualityReport != nil && run.RecipeQualityReport.Status != RecipeQualityNotRun {
		markers = append(markers, "Recipe sources and quality:")
	}
	if run.ServingPlan != nil && run.ServingPlan.Status != ServingPlanNotUsed {
		markers = append(markers, "Servings and scaling:")
	}
	if run.PantryResolution != nil && run.PantryResolution.Status != PantryResolutionNotUsed {
		markers = append(markers, "Pantry and shopping delta:")
	}
	if run.QuantityLedger != nil {
		markers = append(markers, "Shopping confidence:")
	}
	text := string(data)
	allPresent := true
	for _, marker := range markers {
		present := strings.Contains(text, marker)
		allPresent = allPresent && present
		report.addCheck("pdf_marker_present", "pdf", "blocking", present, FoodArtifactPDF, "", "PDF must include trust marker "+marker, "Regenerate the PDF from the final run artifact.", marker, present)
	}
	report.Summary.PDFTrustSectionsPresent = allPresent
}

func auditLedgerConsistency(report *ArtifactAuditReport, run FoodRunArtifact) {
	if run.QuantityLedger == nil {
		return
	}
	requirements := map[string]IngredientRequirement{}
	for _, req := range run.QuantityLedger.Requirements {
		requirements[req.RequirementID] = req
		if req.TotalRequiredQuantity != nil {
			report.addCheck("ledger_required_quantity_non_negative", "ledger", "blocking", req.TotalRequiredQuantity.BaseValue >= 0, FoodArtifactQuantityLedger, "$.requirements", "Ledger required quantities must not be negative.", "Regenerate the quantity ledger.")
		}
		if req.ShopRequiredQuantity != nil {
			report.addCheck("ledger_shop_quantity_non_negative", "ledger", "blocking", req.ShopRequiredQuantity.BaseValue >= 0, FoodArtifactQuantityLedger, "$.requirements", "Ledger shop-required quantities must not be negative.", "Regenerate pantry resolution and quantity ledger.")
		}
	}
	for _, allocation := range run.QuantityLedger.Allocations {
		_, hasReq := requirements[allocation.RequirementID]
		report.addCheck("ledger_allocation_requirement_exists", "ledger", "blocking", hasReq, FoodArtifactQuantityLedger, "$.allocations", "Every allocation must reference a known requirement.", "Regenerate the quantity ledger.", "known requirement", allocation.RequirementID)
		report.addCheck("ledger_package_count_non_negative", "ledger", "blocking", allocation.PackageCount >= 0, FoodArtifactQuantityLedger, "$.allocations.package_count", "Package count must not be negative.", "Regenerate the quantity ledger.")
		if allocation.PurchasedQuantity != nil {
			report.addCheck("ledger_purchased_quantity_non_negative", "ledger", "blocking", allocation.PurchasedQuantity.Expected.BaseValue >= 0, FoodArtifactQuantityLedger, "$.allocations.purchased_quantity", "Purchased quantity must not be negative.", "Regenerate the quantity ledger.")
		}
		if allocation.ShopRequiredQuantity != nil && allocation.PurchasedQuantity != nil && allocation.MatchType != "missing" {
			enough := allocation.PurchasedQuantity.Expected.BaseUnit == allocation.ShopRequiredQuantity.BaseUnit && allocation.PurchasedQuantity.Expected.BaseValue+0.000001 >= allocation.ShopRequiredQuantity.BaseValue
			report.addCheck("ledger_purchased_covers_shop_required", "ledger", "blocking", enough, FoodArtifactQuantityLedger, "$.allocations", "Purchased quantity must cover the shop-required quantity in the same base unit.", "Review package math before using the basket.")
		}
	}
	if run.NutritionLedger != nil {
		report.addCheck("nutrition_coverage_lines_consistent", "nutrition", "blocking", run.NutritionLedger.Coverage.IngredientLines == len(run.NutritionLedger.IngredientLines), FoodArtifactNutritionLedger, "$.coverage.ingredient_lines", "Nutrition coverage line count must match nutrition ingredient lines.", "Regenerate the nutrition ledger.", len(run.NutritionLedger.IngredientLines), run.NutritionLedger.Coverage.IngredientLines)
	}
}

func auditLoadJSON(report *ArtifactAuditReport, artifact, path string, out any) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		report.addCheck(artifact+"_json_readable", "artifact", "blocking", false, artifact, "", "JSON artifact is missing or unreadable.", err.Error())
		return false
	}
	if err := json.Unmarshal(data, out); err != nil {
		report.addCheck(artifact+"_json_valid", "artifact", "blocking", false, artifact, "", "JSON artifact is invalid.", err.Error())
		return false
	}
	report.addCheck(artifact+"_json_valid", "artifact", "info", true, artifact, "", "JSON artifact is readable and valid.", "")
	return true
}

func auditValueSidecar[T any](report *ArtifactAuditReport, name, path string, expected T) {
	if strings.TrimSpace(path) == "" {
		return
	}
	var actual T
	if !auditLoadJSON(report, name, path, &actual) {
		return
	}
	equal := canonicalJSONEqual(expected, actual)
	report.addCheck(name+"_matches_embedded_run", "sidecar", "blocking", equal, name, "", "Sidecar JSON must match the corresponding embedded run object.", "Regenerate sidecars and run artifact together.")
}

func auditPointerSidecar[T any](report *ArtifactAuditReport, name, path string, expected *T) {
	if strings.TrimSpace(path) == "" {
		return
	}
	if expected == nil {
		report.addCheck(name+"_embedded_object_present", "sidecar", "blocking", false, name, "", "Sidecar path exists but the run artifact lacks the embedded object.", "Regenerate the run artifact and sidecar together.")
		return
	}
	var actual T
	if !auditLoadJSON(report, name, path, &actual) {
		return
	}
	equal := canonicalJSONEqual(*expected, actual)
	report.addCheck(name+"_matches_embedded_run", "sidecar", "blocking", equal, name, "", "Sidecar JSON must match the corresponding embedded run object.", "Regenerate sidecars and run artifact together.")
}

func checkFingerprint(report *ArtifactAuditReport, artifact, jsonPath, code, actual, expected string) {
	if expected == "" || actual == "" {
		report.addCheck(code+"_skipped", "fingerprint", "info", true, artifact, jsonPath, "Optional fingerprint is absent.", "")
		return
	}
	report.addCheck(code, "fingerprint", "blocking", actual == expected, artifact, jsonPath, "Derived artifact fingerprint must match the final run fingerprint.", "Regenerate derived artifacts after the final meal plan/shopping state.", expected, actual)
}

func (report *ArtifactAuditReport) addCheck(code, stage, severity string, passed bool, artifact, jsonPath, message, remediation string, expectedActual ...any) {
	check := ArtifactAuditCheck{
		Code:        code,
		Stage:       stage,
		Severity:    severity,
		Passed:      passed,
		Artifact:    artifact,
		JSONPath:    jsonPath,
		Message:     message,
		Remediation: remediation,
	}
	if len(expectedActual) > 0 {
		check.Expected = expectedActual[0]
	}
	if len(expectedActual) > 1 {
		check.Actual = expectedActual[1]
	}
	report.Checks = append(report.Checks, check)
	if passed {
		return
	}
	issue := ArtifactAuditIssue{Code: code, Stage: stage, Artifact: artifact, JSONPath: jsonPath, Message: message, Remediation: remediation}
	if severity == "blocking" {
		report.BlockingIssues = append(report.BlockingIssues, issue)
	} else if severity == "warning" {
		report.Warnings = append(report.Warnings, issue)
	}
}

func (report *ArtifactAuditReport) finalize(gate *ReadinessGate, basket *basketAuditSummary) {
	report.Summary.TotalChecks = len(report.Checks)
	for _, check := range report.Checks {
		if check.Passed {
			report.Summary.PassedChecks++
			continue
		}
		if check.Severity == "warning" {
			report.Summary.WarningChecks++
		} else {
			report.Summary.FailedChecks++
		}
	}
	if len(report.BlockingIssues) > 0 {
		report.Status = ArtifactAuditStatusFail
		report.RecommendedExitCode = ArtifactAuditExitBlocked
	} else if len(report.Warnings) > 0 {
		report.Status = ArtifactAuditStatusPassWarning
		if report.RecommendedExitCode == 0 {
			report.RecommendedExitCode = report.GenerationExitCode
		}
	} else {
		report.Status = ArtifactAuditStatusPass
		if report.RecommendedExitCode == 0 {
			report.RecommendedExitCode = report.GenerationExitCode
		}
	}
	if gate != nil {
		report.Summary.SafeToBuild = gate.SafeToBuild
		report.Summary.SafeToCook = gate.SafeToCook
		report.Summary.SafeToReportNutrition = gate.SafeToReportNutrition
		report.Summary.SafeToUseRecipes = gate.SafeToUseRecipes
	}
	if basket != nil && report.Summary.BasketActionability == "" {
		if basket.ActionableLines > 0 {
			report.Summary.BasketActionability = "actionable"
		} else {
			report.Summary.BasketActionability = "comment_only"
		}
	}
	report.HermesTrustSummary = HermesTrustSummary{
		Trustworthy:                 report.Status != ArtifactAuditStatusFail,
		MayPresentBasketAsReady:     report.Status != ArtifactAuditStatusFail && gate != nil && gate.SafeToBuild,
		MayPresentCookReady:         report.Status != ArtifactAuditStatusFail && gate != nil && gate.SafeToCook,
		MayPresentNutritionNumbers:  report.Status != ArtifactAuditStatusFail && gate != nil && gate.SafeToReportNutrition,
		MayPresentRecipesAsCookable: report.Status != ArtifactAuditStatusFail && gate != nil && gate.SafeToUseRecipes && gate.SafeToCook,
		MayPresentPDFAsComplete:     report.Status != ArtifactAuditStatusFail && gate != nil && gate.SafeToBuild && gate.SafeToCook && gate.SafeToUseRecipes && report.Summary.PDFTrustSectionsPresent,
	}
	if len(report.BlockingIssues) > 0 {
		issue := report.BlockingIssues[0]
		report.HermesTrustSummary.PrimaryFailureCode = issue.Code
		report.HermesTrustSummary.PrimaryFailureMessage = issue.Message
		report.HermesTrustSummary.RequiredUserWarning = "Artifact audit failed; do not present this meal plan, basket, or nutrition summary as ready until the bundle is regenerated."
	} else if gate != nil && !gate.SafeToBuild {
		report.HermesTrustSummary.RequiredUserWarning = "Basket is not safe to build; use the diagnostic artifact only."
	} else if gate != nil && !gate.SafeToCook {
		report.HermesTrustSummary.RequiredUserWarning = "Basket can be prepared, but the meal plan is not cook-ready without resolving the listed caveats."
	} else if gate != nil && !gate.SafeToReportNutrition {
		report.HermesTrustSummary.RequiredUserWarning = "Nutrition numbers are partial or caveated; do not present them as complete evidence-backed totals."
	}
}

type basketAuditSummary struct {
	ActionableLines     int
	DiagnosticLines     int
	CommentLines        int
	BlankLines          int
	HasNotSafeMarker    bool
	HasCookWarning      bool
	HasNutritionWarning bool
	HasAllPantryMarker  bool
}

func classifyBasketText(text string) basketAuditSummary {
	var summary basketAuditSummary
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			summary.BlankLines++
			continue
		}
		upper := strings.ToUpper(trimmed)
		if strings.Contains(upper, "NOT SAFE TO BUILD BASKET") {
			summary.HasNotSafeMarker = true
		}
		if strings.Contains(upper, "MEAL PLAN IS NOT COOK-READY") {
			summary.HasCookWarning = true
		}
		if strings.Contains(upper, "NUTRITION REPORTING IS NOT READY") {
			summary.HasNutritionWarning = true
		}
		if strings.Contains(upper, "NO ALCAMPO ITEMS NEEDED") || strings.Contains(upper, "ALL REQUIRED INGREDIENTS ARE COVERED BY PANTRY") {
			summary.HasAllPantryMarker = true
		}
		if strings.HasPrefix(trimmed, "#") {
			summary.CommentLines++
			body := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
			if looksLikeBasketSKU(body) {
				summary.DiagnosticLines++
			}
			continue
		}
		if looksLikeBasketSKU(trimmed) {
			summary.ActionableLines++
		}
	}
	return summary
}

func looksLikeBasketSKU(line string) bool {
	if idx := strings.IndexByte(line, '#'); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return false
	}
	if strings.HasPrefix(fields[0], "-") {
		return false
	}
	if _, err := strconv.ParseFloat(strings.ReplaceAll(fields[1], ",", "."), 64); err != nil {
		return false
	}
	return fields[0] != ""
}

func actionableSelectedProductCount(shop ShopResult) int {
	count := 0
	for _, selected := range shop.SelectedProducts {
		if selected.Error != "" {
			continue
		}
		if selected.PackageCount <= 0 {
			continue
		}
		if strings.TrimSpace(firstNonEmptyString(selected.Product.SKU, selected.Product.ID)) == "" {
			continue
		}
		count++
	}
	return count
}

func expectedGenerationExitCode(gate ReadinessGate) int {
	if gate.Policy.RequireCookReady && !gate.SafeToCook {
		return ReadinessExitBlocked
	}
	if gate.Policy.RequireNutritionReady && !gate.SafeToReportNutrition {
		return ReadinessExitBlocked
	}
	if gate.Policy.RequireSafeBasket && !gate.SafeToBuild {
		return ReadinessExitBlocked
	}
	return ReadinessExitOK
}

func fileSHA256(path string) (string, int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), int64(len(data)), nil
}

func canonicalJSONEqual(a, b any) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return bytes.Equal(left, right)
}

func resolveArtifactPath(manifestPath, artifactPath string) string {
	artifactPath = strings.TrimSpace(artifactPath)
	if artifactPath == "" || filepath.IsAbs(artifactPath) || manifestPath == "" {
		return artifactPath
	}
	return filepath.Join(filepath.Dir(manifestPath), artifactPath)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
