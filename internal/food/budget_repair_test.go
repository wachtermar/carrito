package food

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/config"
	"github.com/wachtermar/carrito/internal/money"
)

func TestBudgetRepairAppliesValidatedProductSwitch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html></html>`))
		case "/api/webproductpagews/v6/product-pages/search":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"productGroups": []any{
					map[string]any{"decoratedProducts": []any{
						map[string]any{
							"productId":         "rice-budget",
							"retailerProductId": "budget-sku",
							"name":              "Arroz redondo 500 g",
							"brand":             "ALCAMPO",
							"size":              "500 g",
							"price":             map[string]any{"amount": "1.20", "currency": "EUR"},
							"unitPrice":         map[string]any{"amount": "2.40", "currency": "EUR"},
							"available":         true,
							"images":            []any{map[string]any{"url": "/budget.jpg"}},
						},
					}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	run := budgetRepairTestRun()
	client, err := alcampo.New(&config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	client.BaseURL = server.URL
	client.RegionID = "region-test"
	next, plan, err := RepairFoodRunBudget(context.Background(), run, client, BudgetRepairOptions{
		Enabled:         true,
		Profile:         Profile{},
		SelectionPolicy: PolicyBalanced,
		SearchLimit:     4,
		Meals:           []string{"dinner"},
		LedgerOptions:   QuantityLedgerOptions{StrictQuantity: true, StoreID: "region-test"},
		ReadinessPolicy: ReadinessPolicy{StrictQuantity: true, RequireSafeBasket: true, RequireBudgetReady: true},
		Policy:          DefaultBudgetRepairPolicy(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != BudgetRepairAttemptedApplied || len(plan.AppliedDecisions) != 1 {
		t.Fatalf("budget repair not applied: %+v", plan)
	}
	if got := next.Shop.SelectedProducts[0].Product.SKU; got != "budget-sku" {
		t.Fatalf("selected SKU = %q, want budget-sku", got)
	}
	if plan.Baseline.BudgetStatus != BudgetStatusOverBudget || plan.Final.BudgetStatus != BudgetStatusWithinBudget {
		t.Fatalf("budget statuses = %s -> %s", plan.Baseline.BudgetStatus, plan.Final.BudgetStatus)
	}
	finalBudget := BuildBudgetDealReport(next)
	next = AttachBudgetDealReport(next, finalBudget)
	gate := ApplyReadinessGate(&next, ReadinessPolicy{StrictQuantity: true, RequireSafeBasket: true, RequireBudgetReady: true})
	next = FinalizeBudgetRepairPlan(next, gate)
	finalBudget = BuildBudgetDealReport(next)
	next = AttachBudgetDealReport(next, finalBudget)
	gate = ApplyReadinessGate(&next, ReadinessPolicy{StrictQuantity: true, RequireSafeBasket: true, RequireBudgetReady: true})
	if !gate.SafeToBuild || !gate.SafeToReportBudget || gate.BudgetRepairStatus != string(BudgetRepairAttemptedApplied) {
		t.Fatalf("final gate = %+v", gate)
	}
	if next.BudgetDealReport == nil || next.BudgetDealReport.BudgetRepairFingerprint != next.BudgetRepairFingerprint {
		t.Fatalf("budget/deal report did not carry repair fingerprint: report=%+v run=%s", next.BudgetDealReport, next.BudgetRepairFingerprint)
	}
	_, lines := pdfLinesForFoodRunArtifact(next)
	pdfText := strings.Join(lines, "\n")
	if !strings.Contains(pdfText, "Budget repair actions:") || !strings.Contains(pdfText, "Arroz redondo 1 kg -> Arroz redondo 500 g") {
		t.Fatalf("PDF text missing budget repair section:\n%s", pdfText)
	}
	basket := ReadinessBasketLinesWithRecipeSwapOptimizationPantryServingNutritionBudgetRepairAndBudgetDeal([]string{"budget-sku 1 # rice"}, &gate, nil, next.BasketOptimizationPlan, nil, nil, nil, nil, next.BudgetRepairPlan, next.BudgetDealReport)
	if text := strings.Join(basket, "\n"); !strings.Contains(text, "# Budget repair actions: attempted_applied") || !strings.Contains(text, "\nbudget-sku 1 # rice") {
		t.Fatalf("basket text missing repair marker or actionable line:\n%s", text)
	}
}

func budgetRepairTestRun() FoodRunArtifact {
	available := true
	plan := MealPlan{
		ID:        "budget-repair-test",
		People:    2,
		BudgetEUR: "2.00",
		Days: []DayPlan{{Day: 1, Meals: []Meal{{Type: "dinner", Recipe: Recipe{
			ID:       "rice-dinner",
			Title:    "Rice Dinner",
			Servings: 2,
			Ingredients: []Ingredient{
				{Name: "rice", Quantity: 300, Unit: "g", SearchTerm: "arroz"},
			},
			Steps: []RecipeStep{{Number: 1, Text: "Cook rice."}},
		}}}}},
		RequiredPurchases: []Ingredient{
			{Name: "rice", Quantity: 300, Unit: "g", SearchTerm: "arroz"},
		},
	}
	shop := ShopResult{
		MealPlanID: plan.ID,
		Policy:     PolicyBalanced,
		Complete:   true,
		SelectedProducts: []SelectedProduct{{
			Ingredient:       plan.RequiredPurchases[0],
			Product:          ProductSummary{ID: "rice-base", SKU: "base-sku", Name: "Arroz redondo 1 kg", Size: "1 kg", PackageQuantity: 1, PackageUnit: "kg", Price: money.Money{Amount: "4.00", Currency: "EUR", Cents: 400}, Available: &available},
			PurchaseQuantity: "1",
			PackageCount:     1,
			LineTotal:        money.Money{Amount: "4.00", Currency: "EUR", Cents: 400},
		}},
		BasketLines:    []string{"base-sku 1 # rice"},
		EstimatedTotal: money.Money{Amount: "4.00", Currency: "EUR", Cents: 400},
	}
	run := NewFoodRunArtifact(plan, shop, "", "", []string{"dinner"})
	ledger := BuildQuantityLedger(plan, shop, QuantityLedgerOptions{StrictQuantity: true})
	safety := BasketSafetyFromLedger(ledger, QuantityLedgerOptions{StrictQuantity: true})
	run.QuantityLedger = &ledger
	run.BasketSafety = &safety
	return RefreshFoodRunArtifact(run, []string{"dinner"})
}
