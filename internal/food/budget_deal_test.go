package food

import (
	"strings"
	"testing"

	"github.com/wachtermar/carrito/internal/money"
)

func TestBuildBudgetDealReportOverBudget(t *testing.T) {
	run := readyExactRun()
	run.MealPlan.BudgetEUR = "0.50"
	run.Shop.EstimatedTotal = money.Money{Amount: "1.00", Currency: "EUR", Cents: 100}

	report := BuildBudgetDealReport(run)

	if report.Status != BudgetDealOverBudget || report.BudgetStatus != BudgetStatusOverBudget || report.SafeToReportBudget {
		t.Fatalf("report = %+v, want over-budget unsafe budget report", report)
	}
	if report.Delta == nil || report.Delta.Cents != 50 {
		t.Fatalf("delta = %+v, want 0.50", report.Delta)
	}
	if report.Summary.ConsumedCostCents != 10 || report.Summary.PackageExcessCostCents != 90 {
		t.Fatalf("cost basis summary = %+v, want consumed=10 excess=90", report.Summary)
	}
	if report.BudgetDealFingerprint == "" || BudgetDealFingerprint(report) != report.BudgetDealFingerprint {
		t.Fatalf("budget/deal fingerprint was not stable: %+v", report)
	}
}

func TestBuildBudgetDealReportClassifiesOfferCaveats(t *testing.T) {
	run := readyExactRun()
	run.MealPlan.BudgetEUR = "5.00"
	run.Shop.EstimatedTotal = money.Money{Amount: "3.00", Currency: "EUR", Cents: 300}
	run.Shop.SelectedProducts[0].PackageCount = 3
	run.Shop.SelectedProducts[0].LineTotal = money.Money{Amount: "3.00", Currency: "EUR", Cents: 300}
	run.Shop.SelectedProducts[0].Product.Offers = []string{"3x2", "Oferta exclusiva Club Alcampo"}

	report := BuildBudgetDealReport(run)

	if report.BudgetStatus != BudgetStatusWithinBudget || !report.SafeToReportBudget {
		t.Fatalf("budget status = %s safe=%t", report.BudgetStatus, report.SafeToReportBudget)
	}
	if report.Summary.OfferCount != 2 || report.Summary.AppliedOfferCount != 1 || report.Summary.LoyaltyOfferCount != 1 {
		t.Fatalf("unexpected offer summary: %+v", report.Summary)
	}
	if report.Status != BudgetDealPassWithWarnings {
		t.Fatalf("status = %s, want caveated because loyalty/optimization caveats exist", report.Status)
	}
}

func TestReadinessRequireBudgetReadyBlocksReportingNotBasket(t *testing.T) {
	run := readyExactRun()
	run.MealPlan.BudgetEUR = "0.50"
	run.Shop.EstimatedTotal = money.Money{Amount: "1.00", Currency: "EUR", Cents: 100}
	run = AttachBudgetDealReport(run, BuildBudgetDealReport(run))

	gate := ApplyReadinessGate(&run, ReadinessPolicy{RequireSafeBasket: true, RequireBudgetReady: true})

	if !gate.SafeToBuild || gate.SafeToReportBudget || gate.ExitCode != ReadinessExitBlocked {
		t.Fatalf("gate = %+v, want buildable basket with blocked budget reporting", gate)
	}
	if gate.ExitReason != "budget reporting is not ready" {
		t.Fatalf("exit reason = %q", gate.ExitReason)
	}
	if !hasGateWarning(gate, "budget_over_target") {
		t.Fatalf("expected budget warning: %+v", gate.Warnings)
	}
	lines := ReadinessBasketLinesWithRecipeSwapOptimizationPantryServingNutritionAndBudgetDeal([]string{"rice-sku 1 # rice"}, &gate, nil, nil, nil, nil, nil, nil, run.BudgetDealReport)
	text := strings.Join(lines, "\n")
	if !strings.Contains(text, "# BASKET CAN BE BUILT, BUT BUDGET REPORTING IS NOT READY") || !strings.Contains(text, "\nrice-sku 1 # rice") {
		t.Fatalf("basket lines should keep usable basket with budget warning:\n%s", text)
	}
}

func TestBudgetDealPDFSection(t *testing.T) {
	run := readyExactRun()
	run.MealPlan.BudgetEUR = "5.00"
	run.Shop.EstimatedTotal = money.Money{Amount: "1.00", Currency: "EUR", Cents: 100}
	run = AttachBudgetDealReport(run, BuildBudgetDealReport(run))
	gate := ApplyReadinessGate(&run, ReadinessPolicy{RequireSafeBasket: true})
	run.ReadinessGate = &gate

	_, lines := pdfLinesForFoodRunArtifact(run)
	text := strings.Join(lines, "\n")
	for _, want := range []string{"Budget and deal evidence:", "Budget status: WITHIN_BUDGET", "Estimated shopping total: 1.00 EUR"} {
		if !strings.Contains(text, want) {
			t.Fatalf("PDF text missing %q:\n%s", want, text)
		}
	}
}

func hasGateWarning(gate ReadinessGate, code string) bool {
	for _, issue := range gate.Warnings {
		if issue.Code == code {
			return true
		}
	}
	return false
}
