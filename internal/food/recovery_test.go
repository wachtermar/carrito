package food

import (
	"context"
	"strings"
	"testing"

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/money"
)

func TestRecoverFoodRunMissingBeefStripsWithPrepTransform(t *testing.T) {
	available := true
	plan := recoveryTestMealPlan()
	shop := ShopResult{
		Complete: false,
		SelectedProducts: []SelectedProduct{{
			Ingredient: plan.RequiredPurchases[0],
			Error:      "no products found",
		}},
	}
	artifact := NewFoodRunArtifact(plan, shop, "", "basket.txt", []string{"dinner"})
	ledger := BuildQuantityLedger(plan, shop, QuantityLedgerOptions{})
	safety := BasketSafetyFromLedger(ledger, QuantityLedgerOptions{})
	artifact.QuantityLedger = &ledger
	artifact.BasketSafety = &safety
	client := &recoveryTestClient{search: map[string][]alcampo.Product{
		"filetes de ternera": {{
			ID:        "filetes-id",
			SKU:       "filetes-sku",
			Name:      "Filetes de ternera finos 500 g",
			Size:      "500 g",
			Price:     money.Money{Amount: "4.50", Currency: "EUR", Cents: 450},
			Available: &available,
		}},
	}}

	recovered := RecoverFoodRun(context.Background(), artifact, client, RecoveryOptions{
		Enabled:       true,
		Policy:        PolicyBalanced,
		SearchLimit:   4,
		MaxSearches:   8,
		LedgerOptions: QuantityLedgerOptions{},
	})

	if recovered.PreRecoveryQuantityLedger == nil || recovered.PreRecoveryQuantityLedger.Status != LedgerIncomplete {
		t.Fatalf("pre-recovery ledger not preserved: %+v", recovered.PreRecoveryQuantityLedger)
	}
	if recovered.RecoveryPlan == nil || recovered.RecoveryPlan.Status != RecoveryStatusRecovered || len(recovered.RecoveryPlan.AppliedDecisions) != 1 {
		t.Fatalf("recovery was not applied: %+v", recovered.RecoveryPlan)
	}
	if recovered.RecoveryPlan.ProductSelectionFingerprint != recovered.ProductSelectionFingerprint {
		t.Fatalf("recovery plan fingerprint is stale: plan=%s artifact=%s", recovered.RecoveryPlan.ProductSelectionFingerprint, recovered.ProductSelectionFingerprint)
	}
	gate := ApplyReadinessGate(&recovered, ReadinessPolicy{StrictQuantity: true, RequireSafeBasket: true})
	if hasGateIssue(gate, "stale_recovery_selection_fingerprint") {
		t.Fatalf("recovered run should not have stale recovery fingerprint: %+v", gate.BlockingIssues)
	}
	if recovered.QuantityLedger == nil || recovered.QuantityLedger.Status != LedgerCompleteExact {
		t.Fatalf("final ledger not recovered: %+v", recovered.QuantityLedger)
	}
	if recovered.BasketSafety == nil || !recovered.BasketSafety.SafeToBuild {
		t.Fatalf("recovered basket should be safe: %+v", recovered.BasketSafety)
	}
	if recovered.Shop.SelectedProducts[0].Error != "" || recovered.Shop.SelectedProducts[0].Product.SKU != "filetes-sku" {
		t.Fatalf("selected product not replaced: %+v", recovered.Shop.SelectedProducts[0])
	}
	if !recipeHasStepText(recovered.MealPlan.Days[0].Meals[0].Recipe, "Use thin beef fillets and cut them into strips before cooking.") {
		t.Fatalf("prep note was not added to recipe steps: %+v", recovered.MealPlan.Days[0].Meals[0].Recipe.Steps)
	}
	_, lines := pdfLinesForFoodRunArtifact(recovered)
	text := strings.Join(lines, "\n")
	for _, want := range []string{"Recovery actions:", "prep_note", "cut them into strips"} {
		if !strings.Contains(text, want) {
			t.Fatalf("PDF missing recovery text %q:\n%s", want, text)
		}
	}
}

func TestRecoverFoodRunRejectsMincedBeefForBeefStrips(t *testing.T) {
	available := true
	plan := recoveryTestMealPlan()
	shop := ShopResult{
		Complete: false,
		SelectedProducts: []SelectedProduct{{
			Ingredient: plan.RequiredPurchases[0],
			Error:      "no products found",
		}},
	}
	artifact := NewFoodRunArtifact(plan, shop, "", "basket.txt", []string{"dinner"})
	ledger := BuildQuantityLedger(plan, shop, QuantityLedgerOptions{})
	safety := BasketSafetyFromLedger(ledger, QuantityLedgerOptions{})
	artifact.QuantityLedger = &ledger
	artifact.BasketSafety = &safety
	client := &recoveryTestClient{defaultProducts: []alcampo.Product{{
		ID:        "minced-id",
		SKU:       "minced-sku",
		Name:      "Carne picada de ternera 500 g",
		Size:      "500 g",
		Price:     money.Money{Amount: "3.50", Currency: "EUR", Cents: 350},
		Available: &available,
	}}}

	recovered := RecoverFoodRun(context.Background(), artifact, client, RecoveryOptions{
		Enabled:       true,
		Policy:        PolicyBalanced,
		SearchLimit:   4,
		MaxSearches:   8,
		LedgerOptions: QuantityLedgerOptions{},
	})

	if recovered.RecoveryPlan == nil || recovered.RecoveryPlan.Status != RecoveryStatusFailed {
		t.Fatalf("recovery should fail: %+v", recovered.RecoveryPlan)
	}
	if recovered.QuantityLedger == nil || recovered.QuantityLedger.Status != LedgerIncomplete {
		t.Fatalf("final ledger should remain incomplete: %+v", recovered.QuantityLedger)
	}
	if recovered.BasketSafety == nil || recovered.BasketSafety.SafeToBuild {
		t.Fatalf("failed recovery should not be basket-safe: %+v", recovered.BasketSafety)
	}
	if len(recovered.RecoveryPlan.Attempts) == 0 || len(recovered.RecoveryPlan.Attempts[0].Candidates) == 0 {
		t.Fatalf("expected rejected candidates to be recorded: %+v", recovered.RecoveryPlan)
	}
}

func TestRecoverFoodRunRejectsPetFoodForBeefStrips(t *testing.T) {
	available := true
	plan := recoveryTestMealPlan()
	shop := ShopResult{
		Complete: false,
		SelectedProducts: []SelectedProduct{{
			Ingredient: plan.RequiredPurchases[0],
			Error:      "no products found",
		}},
	}
	artifact := NewFoodRunArtifact(plan, shop, "", "basket.txt", []string{"dinner"})
	ledger := BuildQuantityLedger(plan, shop, QuantityLedgerOptions{})
	safety := BasketSafetyFromLedger(ledger, QuantityLedgerOptions{})
	artifact.QuantityLedger = &ledger
	artifact.BasketSafety = &safety
	client := &recoveryTestClient{defaultProducts: []alcampo.Product{{
		ID:        "dog-snack-id",
		SKU:       "495007",
		Name:      "CANES JON Nature Snaks en tiras para perro con de sabor a ternera 80 g",
		Category:  "Mascotas",
		Size:      "80 g",
		Price:     money.Money{Amount: "1.89", Currency: "EUR", Cents: 189},
		Available: &available,
	}}}

	recovered := RecoverFoodRun(context.Background(), artifact, client, RecoveryOptions{
		Enabled:       true,
		Policy:        PolicyBalanced,
		SearchLimit:   4,
		MaxSearches:   8,
		LedgerOptions: QuantityLedgerOptions{},
	})

	if recovered.RecoveryPlan == nil || recovered.RecoveryPlan.Status != RecoveryStatusFailed {
		t.Fatalf("recovery should fail: %+v", recovered.RecoveryPlan)
	}
	if recovered.QuantityLedger == nil || recovered.QuantityLedger.Status != LedgerIncomplete {
		t.Fatalf("final ledger should remain incomplete: %+v", recovered.QuantityLedger)
	}
	if recovered.BasketSafety == nil || recovered.BasketSafety.SafeToBuild {
		t.Fatalf("failed recovery should not be basket-safe: %+v", recovered.BasketSafety)
	}
	foundPetRejection := false
	for _, attempt := range recovered.RecoveryPlan.Attempts {
		for _, candidate := range attempt.Candidates {
			if strings.Contains(strings.Join(candidate.RejectReasons, " "), "pet or non-human food") {
				foundPetRejection = true
			}
		}
	}
	if !foundPetRejection {
		t.Fatalf("expected pet-food rejection to be recorded: %+v", recovered.RecoveryPlan.Attempts)
	}
}

func recoveryTestMealPlan() MealPlan {
	return MealPlan{
		People: 2,
		Days: []DayPlan{{Day: 1, Meals: []Meal{{Type: "dinner", Recipe: Recipe{
			ID:       "beef-stir-fry",
			Title:    "Beef Stir Fry",
			Servings: 2,
			Ingredients: []Ingredient{{
				Name:     "beef strips",
				Quantity: 400,
				Unit:     "g",
			}},
			Steps: []RecipeStep{{Number: 1, Text: "Stir fry the beef strips."}},
		}}}}},
		RequiredPurchases: []Ingredient{{
			Name:     "beef strips",
			Quantity: 400,
			Unit:     "g",
		}},
	}
}

type recoveryTestClient struct {
	search          map[string][]alcampo.Product
	defaultProducts []alcampo.Product
}

func (c *recoveryTestClient) Search(_ context.Context, query string, _ alcampo.SearchOptions) ([]alcampo.Product, error) {
	if c.search != nil {
		if products, ok := c.search[query]; ok {
			return products, nil
		}
	}
	return c.defaultProducts, nil
}

func (c *recoveryTestClient) Product(_ context.Context, _ string) (alcampo.Product, error) {
	return alcampo.Product{DetailUnavailable: true, DetailMessage: "test detail unavailable"}, nil
}
