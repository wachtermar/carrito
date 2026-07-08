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

func TestParseOptimizationOfferSpanishFixtures(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantType    string
		wantParsed  bool
		wantSavings int
	}{
		{name: "3x2", raw: "3x2 en arroces", wantType: "n_for_m", wantParsed: true, wantSavings: 200},
		{name: "2x1", raw: "2x1", wantType: "n_for_m", wantParsed: true, wantSavings: 200},
		{name: "lleva paga", raw: "lleva 3 paga 2", wantType: "n_for_m", wantParsed: true, wantSavings: 200},
		{name: "llevate paga", raw: "llévate 3 y paga 2", wantType: "n_for_m", wantParsed: true, wantSavings: 200},
		{name: "second ordinal", raw: "2ª unidad -50%", wantType: "second_unit_percent", wantParsed: true, wantSavings: 100},
		{name: "second word", raw: "segunda unidad -50%", wantType: "second_unit_percent", wantParsed: true, wantSavings: 100},
		{name: "second al", raw: "segunda unidad al 50%", wantType: "second_unit_percent", wantParsed: true, wantSavings: 100},
		{name: "percent off recorded only", raw: "-20%", wantType: "percent_off", wantParsed: true, wantSavings: 0},
		{name: "unknown", raw: "Oferta Club", wantType: "unknown", wantParsed: false, wantSavings: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offer := parseOptimizationOffer(tt.raw)
			offer = applyOptimizationOffer(offer, 3, 200, true)
			if offer.Type != tt.wantType || offer.Parsed != tt.wantParsed {
				t.Fatalf("offer = %+v, want type=%s parsed=%t", offer, tt.wantType, tt.wantParsed)
			}
			if offer.SavingsCents != tt.wantSavings {
				t.Fatalf("savings = %d, want %d: %+v", offer.SavingsCents, tt.wantSavings, offer)
			}
			if tt.wantType == "percent_off" && offer.Applied {
				t.Fatalf("ambiguous percent-off should not be applied: %+v", offer)
			}
		})
	}
}

func TestProductSelectionFingerprintBlocksStaleDerivedArtifacts(t *testing.T) {
	run := readyExactRun()
	run = RefreshFoodRunArtifact(run, nil)
	stale := run.ProductSelectionFingerprint
	run.Shop.SelectedProducts[0].Product.SKU = "rice-new-sku"
	run.Shop.BasketLines = []string{"rice-new-sku 1 # rice"}
	run.ProductSelectionFingerprint = ProductSelectionFingerprint(run.Shop)
	run.QuantityLedger.ProductSelectionFingerprint = stale
	run.BasketSafety.ProductSelectionFingerprint = stale

	gate := ApplyReadinessGate(&run, ReadinessPolicy{StrictQuantity: true, RequireSafeBasket: true})
	if gate.Status != ReadinessBlocked || gate.SafeToBuild {
		t.Fatalf("gate should block stale product selection fingerprint: %+v", gate)
	}
	if !hasGateIssue(gate, "stale_quantity_ledger_selection_fingerprint") || !hasGateIssue(gate, "stale_basket_safety_selection_fingerprint") {
		t.Fatalf("missing stale fingerprint issues: %+v", gate.BlockingIssues)
	}
}

func TestOptimizeBasketAppliesValidatedLowerWasteProduct(t *testing.T) {
	searches := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html></html>`))
		case "/api/webproductpagews/v6/product-pages/search":
			searches++
			if got := r.URL.Query().Get("q"); got != "arroz" && got != "rice" {
				t.Fatalf("q = %q, want arroz or rice", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"productGroups": []any{
					map[string]any{"decoratedProducts": []any{
						map[string]any{
							"productId":         "rice-better",
							"retailerProductId": "better-sku",
							"name":              "Arroz redondo 500 g",
							"brand":             "VALUE",
							"size":              "500 g",
							"price":             map[string]any{"amount": "1.20", "currency": "EUR"},
							"unitPrice":         map[string]any{"amount": "2.40", "currency": "EUR"},
							"available":         true,
							"images":            []any{map[string]any{"url": "/better.jpg"}},
							"nutrition":         "Valores medios por 100 g Valor energético 350 kcal Proteínas 7 g",
						},
					}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	available := true
	plan := MealPlan{
		People: 2,
		RequiredPurchases: []Ingredient{
			{Name: "rice", Quantity: 300, Unit: "g", SearchTerm: "arroz"},
		},
	}
	shop := ShopResult{
		Complete: true,
		SelectedProducts: []SelectedProduct{{
			Ingredient:       plan.RequiredPurchases[0],
			Product:          ProductSummary{ID: "rice-base", SKU: "base-sku", Name: "Arroz redondo 1 kg", Size: "1 kg", PackageQuantity: 1, PackageUnit: "kg", Price: money.Money{Amount: "4.00", Currency: "EUR", Cents: 400}, Available: &available},
			PurchaseQuantity: "1",
			PackageCount:     1,
			LineTotal:        money.Money{Amount: "4.00", Currency: "EUR", Cents: 400},
		}},
		BasketLines:    []string{"base-sku 1 # rice"},
		EstimatedTotal: money.Money{Amount: "4.00", Currency: "EUR", Cents: 400},
		ShoppingGroups: nil,
		Nutrition:      plan.Nutrition,
		MealPlanID:     plan.ID,
		Policy:         PolicyBalanced,
	}
	artifact := NewFoodRunArtifact(plan, shop, "", "", []string{"dinner"})
	ledger := BuildQuantityLedger(plan, shop, QuantityLedgerOptions{StrictQuantity: true})
	safety := BasketSafetyFromLedger(ledger, QuantityLedgerOptions{StrictQuantity: true})
	artifact.QuantityLedger = &ledger
	artifact.BasketSafety = &safety
	client, err := alcampo.New(&config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	client.BaseURL = server.URL
	client.RegionID = "region-test"

	next, planOut, err := OptimizeBasket(context.Background(), artifact, client, BasketOptimizationOptions{
		Enabled:         true,
		Profile:         Profile{},
		SelectionPolicy: PolicyBalanced,
		SearchLimit:     4,
		Meals:           []string{"dinner"},
		LedgerOptions:   QuantityLedgerOptions{StrictQuantity: true, StoreID: "region-test"},
		ReadinessPolicy: ReadinessPolicy{StrictQuantity: true, RequireSafeBasket: true},
		Policy: BasketOptimizationPolicy{
			Enabled:                        true,
			Objective:                      BasketObjectiveSafeBalanced,
			MaxCandidatesPerIngredient:     4,
			MaxSearchesPerIngredient:       2,
			MaxTotalCandidateChecks:        8,
			DealAware:                      true,
			StrictQuantity:                 true,
			RequireNoReadinessRegression:   true,
			MinSavingsCentsToSwitch:        50,
			MinWasteReductionRatioToSwitch: 0.2,
			PreferProductImages:            true,
			PreferNutritionLabels:          true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if searches == 0 {
		t.Fatal("optimizer did not search")
	}
	if planOut.Status != BasketOptimizationApplied || len(planOut.Decisions) != 1 || !planOut.Decisions[0].Changed {
		t.Fatalf("optimization not applied: %+v", planOut)
	}
	if got := next.Shop.SelectedProducts[0].Product.SKU; got != "better-sku" {
		t.Fatalf("selected SKU = %q, want better-sku: %+v", got, next.Shop.SelectedProducts[0])
	}
	if next.ReadinessGate == nil || !next.ReadinessGate.SafeToBuild || next.ReadinessGate.Status != ReadinessReadyExact {
		t.Fatalf("optimization did not preserve readiness: %+v", next.ReadinessGate)
	}
	if artifact.Shop.SelectedProducts[0].Product.SKU != "base-sku" {
		t.Fatalf("baseline artifact mutated: %+v", artifact.Shop.SelectedProducts[0])
	}
	if next.ProductSelectionFingerprint == artifact.ProductSelectionFingerprint || strings.TrimSpace(next.ProductSelectionFingerprint) == "" {
		t.Fatalf("selection fingerprint did not change: before=%s after=%s", artifact.ProductSelectionFingerprint, next.ProductSelectionFingerprint)
	}
	if next.PreOptimizationReadinessGate == nil || !next.PreOptimizationReadinessGate.SafeToBuild || next.PreOptimizationBasketSafety == nil {
		t.Fatalf("missing pre-optimization safety evidence: gate=%+v safety=%+v", next.PreOptimizationReadinessGate, next.PreOptimizationBasketSafety)
	}
	if next.BasketOptimizationPlan == nil || next.BasketOptimizationPlan.AfterProductSelectionFingerprint != next.ProductSelectionFingerprint {
		t.Fatalf("missing final optimization fingerprint: plan=%+v fingerprint=%s", next.BasketOptimizationPlan, next.ProductSelectionFingerprint)
	}
}
