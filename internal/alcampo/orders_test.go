package alcampo

import "testing"

func TestCollectPastOrdersExtractsLines(t *testing.T) {
	root := map[string]any{
		"orders": []any{
			map[string]any{
				"orderId":   "order-1",
				"orderDate": "2026-07-01",
				"items": []any{
					map[string]any{
						"quantity": float64(2),
						"product": map[string]any{
							"name":              "Arroz redondo 1 kg",
							"retailerProductId": "sku-rice",
							"productId":         "product-rice",
							"size":              "1 kg",
						},
					},
				},
			},
		},
	}
	orders := CollectPastOrders(root)
	if len(orders) != 1 {
		t.Fatalf("orders len = %d, want 1: %+v", len(orders), orders)
	}
	if orders[0].ID != "order-1" || len(orders[0].Lines) != 1 {
		t.Fatalf("unexpected order: %+v", orders[0])
	}
	line := orders[0].Lines[0]
	if line.Name != "Arroz redondo 1 kg" || line.SKU != "sku-rice" || line.Quantity != 2 {
		t.Fatalf("unexpected line: %+v", line)
	}
}
