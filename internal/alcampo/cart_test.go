package alcampo

import (
	"strings"
	"testing"
)

func TestCartTotalCentsRejectsNestedLineSubtotal(t *testing.T) {
	root := map[string]any{
		"items": []any{map[string]any{
			"productId": "product-1",
			"quantity":  1,
			"totals": map[string]any{
				"display": map[string]any{
					"total": map[string]any{"amount": "1.00", "currency": "EUR"},
				},
			},
		}},
	}
	if cents, ok := CartTotalCents(root); ok {
		t.Fatalf("nested line subtotal was accepted as cart total: %d", cents)
	}
}

func TestCartTotalCentsRejectsConflictingCartContainers(t *testing.T) {
	root := map[string]any{
		"totals": cartTotalsForTest("2.00"),
		"basket": map[string]any{
			"totals": cartTotalsForTest("9.00"),
		},
	}
	if cents, ok := CartTotalCents(root); ok {
		t.Fatalf("conflicting cart totals were accepted: %d", cents)
	}
}

func TestCartTotalCentsPrefersWholeTotalOverItemSubtotal(t *testing.T) {
	root := map[string]any{
		"totals": map[string]any{
			"display": map[string]any{
				"itemPriceAfterPromos": map[string]any{"amount": "3.00", "currency": "EUR"},
				"total":                map[string]any{"amount": "4.50", "currency": "EUR"},
			},
		},
	}
	cents, ok := CartTotalCents(root)
	if !ok || cents != 450 {
		t.Fatalf("whole-cart total = %d, %t; want 450, true", cents, ok)
	}
}

func TestCartTotalCentsRejectsNonEURTotal(t *testing.T) {
	root := map[string]any{"totals": map[string]any{
		"display": map[string]any{"total": map[string]any{"amount": "4.50", "currency": "USD"}},
	}}
	if cents, ok := CartTotalCents(root); ok {
		t.Fatalf("non-EUR total was accepted: %d", cents)
	}
}

func TestCartProductQuantityRejectsConflictingDuplicateQuantities(t *testing.T) {
	root := map[string]any{"items": []any{
		map[string]any{"productId": "same-id", "retailerProductId": "same-sku", "quantity": 1},
		map[string]any{"productId": "same-id", "retailerProductId": "same-sku", "quantity": 2},
	}}
	if _, _, err := CartProductQuantity(root, "same-id"); err == nil || !strings.Contains(err.Error(), "conflicting cart quantities") {
		t.Fatalf("expected conflicting quantity error, got %v", err)
	}
}

func TestCartProductQuantityRejectsDuplicateSKUWithDifferentIDs(t *testing.T) {
	root := map[string]any{"items": []any{
		map[string]any{"productId": "first-id", "retailerProductId": "same-sku", "quantity": 1},
		map[string]any{"productId": "second-id", "retailerProductId": "same-sku", "quantity": 1},
	}}
	if _, _, err := CartProductQuantity(root, "same-sku"); err == nil || !strings.Contains(err.Error(), "conflicting cart identities") {
		t.Fatalf("expected conflicting identity error, got %v", err)
	}
}

func TestCartProductQuantityAllowsRepeatedEquivalentProjection(t *testing.T) {
	root := map[string]any{"items": []any{
		map[string]any{"productId": "same-id", "retailerProductId": "same-sku", "quantity": "1.0"},
		map[string]any{"productId": "same-id", "retailerProductId": "same-sku", "quantity": 1},
	}}
	quantity, ok, err := CartProductQuantity(root, "same-id")
	if err != nil || !ok || quantity != "1.0" {
		t.Fatalf("equivalent projection = %q, %t, %v", quantity, ok, err)
	}
}

func TestCartProductQuantityIgnoresNestedRecommendationProjection(t *testing.T) {
	root := map[string]any{
		"totals": cartTotalsForTest("0.00"),
		"recommendations": map[string]any{
			"items": []any{map[string]any{
				"productId": "target-id", "retailerProductId": "target-sku", "quantity": 1,
			}},
		},
	}
	if quantity, ok, err := CartProductQuantity(root, "target-id"); err != nil || ok {
		t.Fatalf("recommendation projection = %q, %t, %v; want absent", quantity, ok, err)
	}
	if count := CartItemCount(root); count != 0 {
		t.Fatalf("recommendation item count = %d, want 0", count)
	}
	summary := SummarizeCart(root, "https://example.test")
	if summary.ItemCount != 0 || len(summary.Items) != 0 {
		t.Fatalf("recommendation leaked into cart summary: %+v", summary)
	}
}

func TestCartProductQuantityReadsExplicitNestedCartGroup(t *testing.T) {
	root := map[string]any{
		"productGroups": []any{map[string]any{
			"items": []any{map[string]any{
				"quantity": 2,
				"product":  map[string]any{"productId": "target-id", "retailerProductId": "target-sku", "name": "Target"},
			}},
		}},
	}
	quantity, ok, err := CartProductQuantity(root, "target-id")
	if err != nil || !ok || quantity != "2" {
		t.Fatalf("nested cart group quantity = %q, %t, %v", quantity, ok, err)
	}
}

func TestCartProductQuantityReadsObservedBasketUpdateResultShape(t *testing.T) {
	root := map[string]any{
		"basketUpdateResult": map[string]any{
			"totals": cartTotalsForTest("2.00"),
			"itemGroups": []any{map[string]any{
				"items": []any{map[string]any{
					"productId": "target-id", "quantity": 2,
				}},
			}},
		},
		"recommendations": map[string]any{
			"items": []any{map[string]any{"productId": "target-id", "quantity": 99}},
		},
	}
	quantity, ok, err := CartProductQuantity(root, "target-id")
	if err != nil || !ok || quantity != "2" {
		t.Fatalf("basket update quantity = %q, %t, %v", quantity, ok, err)
	}
	if cents, ok := CartTotalCents(root); !ok || cents != 200 {
		t.Fatalf("basket update total = %d, %t", cents, ok)
	}
}

func TestCartLineNestedProductIdentityOverridesGenericLineID(t *testing.T) {
	root := map[string]any{
		"totals": cartTotalsForTest("2.00"),
		"items": []any{map[string]any{
			"id":       "line-1",
			"quantity": 2,
			"product": map[string]any{
				"productId":         "target-id",
				"retailerProductId": "target-sku",
			},
		}},
	}
	quantity, ok, err := CartProductQuantity(root, "target-id")
	if err != nil || !ok || quantity != "2" {
		t.Fatalf("nested product quantity = %q, %t, %v", quantity, ok, err)
	}
	if quantity, ok, err := CartProductQuantity(root, "line-1"); err != nil || ok {
		t.Fatalf("generic line id was treated as product: %q, %t, %v", quantity, ok, err)
	}
	if count := CartItemCount(root); count != 1 {
		t.Fatalf("cart item count = %d, want 1", count)
	}
	summary := SummarizeCart(root, "https://example.test")
	if summary.ItemCount != 1 || len(summary.Items) != 1 || summary.Items[0].ProductID != "target-id" || summary.Items[0].SKU != "target-sku" || summary.Items[0].Quantity != "2" {
		t.Fatalf("nested product summary = %+v", summary)
	}
}

func TestCartProductQuantityRejectsConflictingLineProductProjections(t *testing.T) {
	root := map[string]any{
		"totals": cartTotalsForTest("1.00"),
		"items": []any{map[string]any{
			"quantity": 1,
			"product": map[string]any{
				"productId": "target-id", "retailerProductId": "target-sku", "name": "Target",
			},
			"decoratedProduct": map[string]any{
				"productId": "other-id", "retailerProductId": "other-sku", "name": "Other",
			},
		}},
	}
	if quantity, ok, err := CartProductQuantity(root, "target-id"); err == nil || ok || !strings.Contains(err.Error(), "conflicting cart product projections") {
		t.Fatalf("conflicting projections = %q, %t, %v", quantity, ok, err)
	}
	summary := SummarizeCart(root, "https://example.test")
	if summary.ItemCount != 0 || len(summary.Items) != 0 {
		t.Fatalf("conflicting projection leaked into cart summary: %+v", summary)
	}
}

func cartTotalsForTest(amount string) map[string]any {
	return map[string]any{
		"display": map[string]any{
			"total": map[string]any{"amount": amount, "currency": "EUR"},
		},
	}
}
