package food

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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
		ArtifactPaths: map[string]string{
			FoodArtifactRun:            paths[FoodArtifactRun],
			FoodArtifactQuantityLedger: paths[FoodArtifactQuantityLedger],
			FoodArtifactReadiness:      paths[FoodArtifactReadiness],
			FoodArtifactBasket:         paths[FoodArtifactBasket],
			FoodArtifactPDF:            paths[FoodArtifactPDF],
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeAuditJSON(t, paths["manifest"], manifest)
	return paths
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
