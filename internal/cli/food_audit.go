package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/food"
	"github.com/wachtermar/carrito/internal/output"
)

func runFoodValidateRun(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food validate-run", stderr)
	runPath := fs.String("run", "", "food run JSON artifact path")
	manifestPath := fs.String("manifest", "", "food run manifest JSON path")
	auditOut := fs.String("audit-out", "", "optional audit report JSON path")
	contextMode := fs.String("mode", "local", "validation context: hermes, ci, local, or live-smoke")
	auditMode := fs.String("audit-mode", food.ArtifactAuditModeFail, "audit exit behavior: off, warn, or fail")
	pdfPath := fs.String("pdf", "", "optional PDF path override")
	basketPath := fs.String("basket", "", "optional basket path override")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if strings.TrimSpace(*runPath) == "" && strings.TrimSpace(*manifestPath) == "" {
		return fmt.Errorf("food validate-run requires --run or --manifest")
	}
	if err := validateFoodAuditContextMode(*contextMode); err != nil {
		return err
	}
	mode, err := food.NormalizeArtifactAuditMode(*auditMode)
	if err != nil {
		return err
	}
	report, err := food.AuditFoodRunArtifacts(food.ArtifactAuditOptions{
		Mode:         mode,
		ContextMode:  *contextMode,
		ManifestPath: *manifestPath,
		RunPath:      *runPath,
		PDFPath:      *pdfPath,
		BasketPath:   *basketPath,
	})
	if err != nil {
		return err
	}
	if *auditOut != "" {
		if err := writeJSONPath(*auditOut, report); err != nil {
			return err
		}
	}
	if *jsonOut {
		if err := output.JSON(stdout, report); err != nil {
			return err
		}
	} else {
		printArtifactAudit(stdout, report)
		if *auditOut != "" {
			fmt.Fprintf(stdout, "audit\t%s\n", *auditOut)
		}
	}
	return artifactAuditExitError(report, mode)
}

func validateFoodAuditContextMode(mode string) error {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "hermes", "ci", "local", "live-smoke":
		return nil
	default:
		return fmt.Errorf("invalid validate-run mode %q; expected hermes, ci, local, or live-smoke", mode)
	}
}

func printArtifactAudit(stdout io.Writer, report food.ArtifactAuditReport) {
	fmt.Fprintf(stdout, "audit\tstatus=%s\tmode=%s\trecommended_exit=%d\n", report.Status, report.Mode, report.RecommendedExitCode)
	fmt.Fprintf(stdout, "trust\ttrustworthy=%t\tbasket=%t\tcook=%t\tnutrition=%t\trecipes=%t\tpdf=%t\n", report.HermesTrustSummary.Trustworthy, report.HermesTrustSummary.MayPresentBasketAsReady, report.HermesTrustSummary.MayPresentCookReady, report.HermesTrustSummary.MayPresentNutritionNumbers, report.HermesTrustSummary.MayPresentRecipesAsCookable, report.HermesTrustSummary.MayPresentPDFAsComplete)
	if report.HermesTrustSummary.RequiredUserWarning != "" {
		fmt.Fprintf(stdout, "warning\t%s\n", report.HermesTrustSummary.RequiredUserWarning)
	}
	if len(report.BlockingIssues) > 0 {
		issue := report.BlockingIssues[0]
		fmt.Fprintf(stdout, "blocking\t%s\t%s\n", issue.Code, issue.Message)
	}
}

func printFoodSnapshotSummary(stdout io.Writer, summary *food.ManifestSnapshotSummary) {
	if summary == nil || summary.Mode == "" {
		return
	}
	fmt.Fprintf(stdout, "snapshot\tmode=%s\tentries=%d\tstrict=%t\thits=%d\tmisses=%d\n", summary.Mode, summary.EntryCount, summary.ReplayStrict, summary.ReplayHits, summary.ReplayMisses)
	if summary.SnapshotManifestPath != "" {
		fmt.Fprintf(stdout, "snapshot_manifest\t%s\n", summary.SnapshotManifestPath)
	}
}

func foodManifestSnapshotFromClient(summary *alcampo.LiveSnapshotSummary) *food.ManifestSnapshotSummary {
	if summary == nil || summary.Mode == "" {
		return nil
	}
	return &food.ManifestSnapshotSummary{
		Mode:                 summary.Mode,
		SnapshotID:           summary.SnapshotID,
		SnapshotDir:          summary.SnapshotDir,
		SnapshotManifestPath: summary.SnapshotManifestPath,
		EntryCount:           summary.EntryCount,
		ReplayStrict:         summary.ReplayStrict,
		SnapshotSHA256:       summary.SnapshotSHA256,
		ReplayHits:           summary.ReplayHits,
		ReplayMisses:         summary.ReplayMisses,
	}
}

func artifactAuditExitError(report food.ArtifactAuditReport, mode string) error {
	if mode == food.ArtifactAuditModeFail && report.Status == food.ArtifactAuditStatusFail {
		return ExitError{Code: food.ArtifactAuditExitBlocked, Err: fmt.Errorf("food artifact audit failed: %s", report.HermesTrustSummary.PrimaryFailureMessage)}
	}
	return nil
}

func defaultFoodRunManifestPath(runPath, auditPath string) string {
	if strings.TrimSpace(runPath) != "" {
		ext := filepath.Ext(runPath)
		if ext == "" {
			return runPath + "-manifest.json"
		}
		return strings.TrimSuffix(runPath, ext) + "-manifest.json"
	}
	if strings.TrimSpace(auditPath) != "" {
		ext := filepath.Ext(auditPath)
		if ext == "" {
			return auditPath + "-manifest.json"
		}
		return strings.TrimSuffix(auditPath, ext) + "-manifest.json"
	}
	return ""
}

func foodRunArtifactPaths(planOut, shopOut, runPath, pdfPath, basketPath, ledgerOut, nutritionLedgerOut, budgetRepairOut, budgetDealOut, servingPlanOut, scaledMealPlanOut, pantryOut, pantryConsumptionOut, readinessOut, recoveryOut, recipeSwapOut, basketOptimizationOut, recipeIntakeOut, recipeQualityOut string) map[string]string {
	paths := map[string]string{}
	add := func(key, path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if _, err := os.Stat(path); err == nil {
			paths[key] = path
		}
	}
	add(food.FoodArtifactPlan, planOut)
	add(food.FoodArtifactShop, shopOut)
	add(food.FoodArtifactRun, runPath)
	add(food.FoodArtifactPDF, pdfPath)
	add(food.FoodArtifactBasket, basketPath)
	add(food.FoodArtifactQuantityLedger, ledgerOut)
	add(food.FoodArtifactNutritionLedger, nutritionLedgerOut)
	add(food.FoodArtifactBudgetRepair, budgetRepairOut)
	add(food.FoodArtifactBudgetDeal, budgetDealOut)
	add(food.FoodArtifactServingPlan, servingPlanOut)
	add(food.FoodArtifactScaledMealPlan, scaledMealPlanOut)
	add(food.FoodArtifactPantry, pantryOut)
	add(food.FoodArtifactPantryConsumption, pantryConsumptionOut)
	add(food.FoodArtifactReadiness, readinessOut)
	add(food.FoodArtifactRecovery, recoveryOut)
	add(food.FoodArtifactRecipeSwap, recipeSwapOut)
	add(food.FoodArtifactBasketOptimization, basketOptimizationOut)
	add(food.FoodArtifactRecipeIntake, recipeIntakeOut)
	add(food.FoodArtifactRecipeQuality, recipeQualityOut)
	return paths
}
