package food

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/money"
)

func TestArtifactAuditSafeBundlePasses(t *testing.T) {
	run := auditTestRun(t)
	paths := writeAuditBundle(t, run, nil)

	report, err := AuditFoodRunArtifacts(ArtifactAuditOptions{
		Mode:         ArtifactAuditModeFail,
		ContextMode:  "ci",
		ManifestPath: paths["manifest"],
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status == ArtifactAuditStatusFail {
		t.Fatalf("audit failed: %+v", report.BlockingIssues)
	}
	if !report.HermesTrustSummary.Trustworthy || !report.HermesTrustSummary.MayPresentBasketAsReady || !report.HermesTrustSummary.MayPresentCookReady {
		t.Fatalf("unexpected trust summary: %+v", report.HermesTrustSummary)
	}
}

func TestArtifactAuditUnsafeBasketRejectsActionableLines(t *testing.T) {
	run := auditTestRun(t)
	run.ReadinessGate.SafeToBuild = false
	run.ReadinessGate.Status = ReadinessBlocked
	run.ReadinessGate.ExitCode = ReadinessExitBlocked
	run.ReadinessGate.ExitReason = "basket is not safe to build"
	run.ReadinessGate.BlockingIssues = []GateIssue{{Code: "test_block", Severity: "blocking", Phase: "test", Message: "blocked"}}
	run.BasketSafety.SafeToBuild = false
	run.BasketSafety.Status = BasketSafetyUnsafe
	paths := writeAuditBundle(t, run, func(paths map[string]string, run FoodRunArtifact) {
		if err := os.WriteFile(paths[FoodArtifactBasket], []byte("rice-sku 1 # rice\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	})

	report, err := AuditFoodRunArtifacts(ArtifactAuditOptions{
		Mode:         ArtifactAuditModeFail,
		ContextMode:  "ci",
		ManifestPath: paths["manifest"],
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != ArtifactAuditStatusFail {
		t.Fatalf("status = %s, want fail", report.Status)
	}
	if !hasAuditIssue(report, "unsafe_basket_has_no_actionable_lines") {
		t.Fatalf("expected actionable basket issue, got %+v", report.BlockingIssues)
	}
}

func TestArtifactAuditSidecarMismatchFailsEvenWhenManifestHashMatches(t *testing.T) {
	run := auditTestRun(t)
	paths := writeAuditBundle(t, run, func(paths map[string]string, run FoodRunArtifact) {
		ledger := *run.QuantityLedger
		ledger.Status = LedgerIncomplete
		writeAuditJSON(t, paths[FoodArtifactQuantityLedger], ledger)
	})

	report, err := AuditFoodRunArtifacts(ArtifactAuditOptions{
		Mode:         ArtifactAuditModeFail,
		ContextMode:  "ci",
		ManifestPath: paths["manifest"],
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != ArtifactAuditStatusFail {
		t.Fatalf("status = %s, want fail", report.Status)
	}
	if !hasAuditIssue(report, "quantity_ledger_matches_embedded_run") {
		t.Fatalf("expected sidecar mismatch issue, got %+v", report.BlockingIssues)
	}
}

func TestArtifactAuditRecipeQualitySidecarMismatchFails(t *testing.T) {
	run := auditTestRun(t)
	quality := EvaluateRecipeQuality(run.MealPlan, nil, nil, DefaultRecipeIntakePolicy())
	intake := RecipeIntakePlan{SchemaVersion: "1", Status: RecipeIntakeComplete, Policy: DefaultRecipeIntakePolicy(), ActiveRecipeCount: len(quality.Items), RecipeSetFingerprint: quality.RecipeSetFingerprint}
	run = AttachRecipeQuality(run, &intake, &quality)
	gate := ApplyReadinessGate(&run, ReadinessPolicy{})
	run.ReadinessGate = &gate
	paths := writeAuditBundle(t, run, func(paths map[string]string, run FoodRunArtifact) {
		changed := *run.RecipeQualityReport
		changed.Items[0].RecipeTitle = "stale title"
		writeAuditJSON(t, paths[FoodArtifactRecipeQuality], changed)
	})

	report, err := AuditFoodRunArtifacts(ArtifactAuditOptions{
		Mode:         ArtifactAuditModeFail,
		ContextMode:  "ci",
		ManifestPath: paths["manifest"],
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != ArtifactAuditStatusFail {
		t.Fatalf("status = %s, want fail", report.Status)
	}
	if !hasAuditIssue(report, "recipe_quality_matches_embedded_run") {
		t.Fatalf("expected recipe quality sidecar mismatch, got %+v", report.BlockingIssues)
	}
}

func TestArtifactAuditSnapshotCorruptResponseFails(t *testing.T) {
	run := auditTestRun(t)
	paths := writeAuditBundle(t, run, nil)
	var manifest FoodRunManifest
	readAuditJSON(t, paths["manifest"], &manifest)
	snapshotDir := filepath.Join(filepath.Dir(paths["manifest"]), "snapshot")
	responsesDir := filepath.Join(snapshotDir, "responses")
	if err := os.MkdirAll(responsesDir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"productGroups":[]}`)
	bodySum := sha256.Sum256(body)
	bodyHash := hex.EncodeToString(bodySum[:])
	responsePath := filepath.Join("responses", "000001_search_deadbeef1234.json")
	writeAuditJSON(t, filepath.Join(snapshotDir, responsePath), map[string]any{
		"schema_version": alcampo.LiveSnapshotSchemaVersion,
		"status_code":    200,
		"body_sha256":    bodyHash,
		"body_base64":    base64.StdEncoding.EncodeToString([]byte("corrupted response")),
	})
	snapshot := alcampo.LiveSnapshotManifest{
		SchemaVersion: alcampo.LiveSnapshotSchemaVersion,
		SnapshotID:    "audit-corrupt-snapshot",
		Mode:          alcampo.LiveSnapshotModeRecord,
		EntryCount:    1,
		Entries: []alcampo.LiveSnapshotEntry{{
			Sequence:         1,
			ID:               "000001-deadbeef1234",
			RequestSignature: "sig",
			ShortSignature:   "deadbeef1234",
			RequestKind:      alcampo.SnapshotKindSearch,
			Method:           "GET",
			ResponsePath:     responsePath,
			ResponseSHA256:   bodyHash,
			ResponseBytes:    int64(len(body)),
			StatusCode:       200,
		}},
	}
	snapshotPath := filepath.Join(snapshotDir, alcampo.LiveSnapshotManifestFile)
	writeAuditJSON(t, snapshotPath, snapshot)
	snapshotData, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshotSum := sha256.Sum256(snapshotData)
	manifest.Snapshot = &ManifestSnapshotSummary{
		Mode:                 alcampo.LiveSnapshotModeRecord,
		SnapshotID:           snapshot.SnapshotID,
		SnapshotDir:          snapshotDir,
		SnapshotManifestPath: snapshotPath,
		EntryCount:           1,
		SnapshotSHA256:       hex.EncodeToString(snapshotSum[:]),
	}
	writeAuditJSON(t, paths["manifest"], manifest)

	report, err := AuditFoodRunArtifacts(ArtifactAuditOptions{
		Mode:         ArtifactAuditModeFail,
		ContextMode:  "ci",
		ManifestPath: paths["manifest"],
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != ArtifactAuditStatusFail {
		t.Fatalf("status = %s, want fail", report.Status)
	}
	if !hasAuditIssue(report, "snapshot_response_hash_matches_entry") {
		t.Fatalf("expected snapshot hash issue, got %+v", report.BlockingIssues)
	}
}

func auditTestRun(t *testing.T) FoodRunArtifact {
	t.Helper()
	available := true
	plan := MealPlan{
		ID:                "audit-plan",
		CreatedAt:         "2026-07-08T12:00:00Z",
		People:            2,
		SelectionPolicy:   PolicyBalanced,
		RequiredPurchases: []Ingredient{{Name: "rice", Quantity: 100, Unit: "g", SearchTerm: "arroz"}},
		Days: []DayPlan{{
			Day: 1,
			Meals: []Meal{{
				Type: "dinner",
				Recipe: Recipe{
					ID:          "audit-rice",
					Title:       "Audit Rice",
					Servings:    2,
					Ingredients: []Ingredient{{Name: "rice", Quantity: 100, Unit: "g", SearchTerm: "arroz"}},
					Steps:       []RecipeStep{{Number: 1, Text: "Cook rice."}},
				},
			}},
		}},
	}
	shop := ShopResult{
		MealPlanID: "audit-plan",
		Policy:     PolicyBalanced,
		Complete:   true,
		SelectedProducts: []SelectedProduct{{
			Ingredient:       Ingredient{Name: "rice", Quantity: 100, Unit: "g", SearchTerm: "arroz"},
			Product:          ProductSummary{ID: "rice-product", SKU: "rice-sku", Name: "Arroz redondo 1 kg", Size: "1 kg", PackageQuantity: 1000, PackageUnit: "g", Price: money.Money{Amount: "1.20", Currency: "EUR", Cents: 120}, Available: &available},
			PurchaseQuantity: "1",
			PackageCount:     1,
			LineTotal:        money.Money{Amount: "1.20", Currency: "EUR", Cents: 120},
		}},
		BasketLines:    []string{"rice-sku 1 # rice"},
		EstimatedTotal: money.Money{Amount: "1.20", Currency: "EUR", Cents: 120},
	}
	run := NewFoodRunArtifact(plan, shop, "", "", []string{"dinner"})
	ledger := BuildQuantityLedger(plan, shop, QuantityLedgerOptions{})
	run.QuantityLedger = &ledger
	safety := BasketSafetyFromLedger(ledger, QuantityLedgerOptions{})
	run.BasketSafety = &safety
	gate := ApplyReadinessGate(&run, ReadinessPolicy{RequireSafeBasket: true})
	if !gate.SafeToBuild {
		t.Fatalf("audit fixture should be safe: %+v", gate)
	}
	return run
}

func writeAuditBundle(t *testing.T, run FoodRunArtifact, mutateBeforeManifest func(map[string]string, FoodRunArtifact)) map[string]string {
	t.Helper()
	dir := t.TempDir()
	paths := map[string]string{
		FoodArtifactRun:            filepath.Join(dir, "run.json"),
		FoodArtifactQuantityLedger: filepath.Join(dir, "ledger.json"),
		FoodArtifactReadiness:      filepath.Join(dir, "readiness.json"),
		FoodArtifactBasket:         filepath.Join(dir, "basket.txt"),
		FoodArtifactPDF:            filepath.Join(dir, "run.pdf"),
		"manifest":                 filepath.Join(dir, "manifest.json"),
	}
	run.BasketPath = paths[FoodArtifactBasket]
	basketLines := ReadinessBasketLinesWithRecipeSwapOptimizationPantryServingAndNutrition(run.Shop.BasketLines, run.ReadinessGate, run.RecipeSwapPlan, run.BasketOptimizationPlan, run.PantryResolution, run.ServingPlan, run.ScaledMealPlan, run.NutritionLedger)
	if err := os.WriteFile(paths[FoodArtifactBasket], []byte(joinLinesForTest(basketLines)), 0o600); err != nil {
		t.Fatal(err)
	}
	writeAuditJSON(t, paths[FoodArtifactQuantityLedger], run.QuantityLedger)
	writeAuditJSON(t, paths[FoodArtifactReadiness], run.ReadinessGate)
	if run.RecipeIntakePlan != nil {
		paths[FoodArtifactRecipeIntake] = filepath.Join(dir, "recipe_intake.json")
		writeAuditJSON(t, paths[FoodArtifactRecipeIntake], run.RecipeIntakePlan)
	}
	if run.RecipeQualityReport != nil {
		paths[FoodArtifactRecipeQuality] = filepath.Join(dir, "recipe_quality.json")
		writeAuditJSON(t, paths[FoodArtifactRecipeQuality], run.RecipeQualityReport)
	}
	writeAuditJSON(t, paths[FoodArtifactRun], run)
	if err := WritePDFFromJSONFile(paths[FoodArtifactRun], paths[FoodArtifactPDF]); err != nil {
		t.Fatal(err)
	}
	run.PDFPath = paths[FoodArtifactPDF]
	writeAuditJSON(t, paths[FoodArtifactRun], run)
	if mutateBeforeManifest != nil {
		mutateBeforeManifest(paths, run)
	}
	manifest, err := BuildFoodRunManifest(run, FoodRunManifestOptions{
		Command:              "carrito food run",
		Args:                 []string{"food", "run", "--test"},
		AuditMode:            ArtifactAuditModeFail,
		GenerationExitCode:   expectedGenerationExitCode(*run.ReadinessGate),
		GenerationExitReason: run.ReadinessGate.ExitReason,
		ArtifactPaths:        auditBundleArtifactPaths(paths),
	})
	if err != nil {
		t.Fatal(err)
	}
	writeAuditJSON(t, paths["manifest"], manifest)
	return paths
}

func auditBundleArtifactPaths(paths map[string]string) map[string]string {
	out := map[string]string{
		FoodArtifactRun:            paths[FoodArtifactRun],
		FoodArtifactQuantityLedger: paths[FoodArtifactQuantityLedger],
		FoodArtifactReadiness:      paths[FoodArtifactReadiness],
		FoodArtifactBasket:         paths[FoodArtifactBasket],
		FoodArtifactPDF:            paths[FoodArtifactPDF],
	}
	if paths[FoodArtifactRecipeIntake] != "" {
		out[FoodArtifactRecipeIntake] = paths[FoodArtifactRecipeIntake]
	}
	if paths[FoodArtifactRecipeQuality] != "" {
		out[FoodArtifactRecipeQuality] = paths[FoodArtifactRecipeQuality]
	}
	return out
}

func writeAuditJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readAuditJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		t.Fatalf("%s was not JSON: %v\n%s", path, err, string(data))
	}
}

func joinLinesForTest(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	out := ""
	for _, line := range lines {
		out += line + "\n"
	}
	return out
}

func hasAuditIssue(report ArtifactAuditReport, code string) bool {
	for _, issue := range report.BlockingIssues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
