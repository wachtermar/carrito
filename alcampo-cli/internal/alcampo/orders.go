package alcampo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type PastOrderOptions struct {
	Limit int
	Since string
}

type PastOrder struct {
	ID        string          `json:"id,omitempty"`
	Number    string          `json:"number,omitempty"`
	CreatedAt string          `json:"created_at,omitempty"`
	Lines     []PastOrderLine `json:"lines,omitempty"`
	Raw       any             `json:"raw,omitempty"`
}

type PastOrderLine struct {
	Name      string  `json:"name,omitempty"`
	SKU       string  `json:"sku,omitempty"`
	ProductID string  `json:"product_id,omitempty"`
	Quantity  float64 `json:"quantity,omitempty"`
	Unit      string  `json:"unit,omitempty"`
	Size      string  `json:"size,omitempty"`
	Category  string  `json:"category,omitempty"`
}

func (c *Client) PastOrders(ctx context.Context, opts PastOrderOptions) ([]PastOrder, error) {
	q := url.Values{}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
		q.Set("pageSize", strconv.Itoa(opts.Limit))
	}
	if strings.TrimSpace(opts.Since) != "" {
		q.Set("since", strings.TrimSpace(opts.Since))
		q.Set("from", strings.TrimSpace(opts.Since))
	}
	paths := []string{
		"/api/order/v1/orders",
		"/api/order/v2/orders",
		"/api/account/v1/orders",
		"/api/commerce/v1/orders",
	}
	var lastErr error
	for _, path := range paths {
		var root any
		if err := c.getJSON(ctx, path, q, c.BaseURL+"/my-account/orders", "orders", &root); err != nil {
			lastErr = err
			continue
		}
		orders := CollectPastOrders(root)
		if opts.Limit > 0 && len(orders) > opts.Limit {
			orders = orders[:opts.Limit]
		}
		return orders, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no order history endpoint returned data")
}

func CollectPastOrders(root any) []PastOrder {
	var orders []PastOrder
	seen := map[string]bool{}
	walk(root, func(m map[string]any) {
		order, ok := orderFromMap(m)
		if !ok || len(order.Lines) == 0 {
			return
		}
		key := strings.TrimSpace(order.ID + "|" + order.Number + "|" + order.CreatedAt)
		if key == "||" {
			raw, _ := json.Marshal(m)
			key = string(raw)
		}
		if seen[key] {
			return
		}
		seen[key] = true
		orders = append(orders, order)
	})
	sort.SliceStable(orders, func(i, j int) bool {
		return orders[i].CreatedAt > orders[j].CreatedAt
	})
	return orders
}

func orderFromMap(m map[string]any) (PastOrder, bool) {
	order := PastOrder{
		ID:        stringFromKeys(m, "id", "orderId", "orderID"),
		Number:    stringFromKeys(m, "number", "orderNumber", "orderNo"),
		CreatedAt: stringFromKeys(m, "createdAt", "creationDate", "orderDate", "date"),
		Raw:       m,
	}
	order.Lines = collectOrderLines(m)
	if order.ID == "" && order.Number == "" && order.CreatedAt == "" {
		return PastOrder{}, false
	}
	return order, len(order.Lines) > 0
}

func collectOrderLines(root any) []PastOrderLine {
	var lines []PastOrderLine
	seen := map[string]bool{}
	walk(root, func(m map[string]any) {
		line, ok := orderLineFromMap(m)
		if !ok {
			return
		}
		key := strings.ToLower(strings.TrimSpace(line.ProductID + "|" + line.SKU + "|" + line.Name))
		if key == "||" || seen[key] {
			return
		}
		seen[key] = true
		lines = append(lines, line)
	})
	return lines
}

func orderLineFromMap(m map[string]any) (PastOrderLine, bool) {
	product := mapFromKeys(m, "product", "item", "article")
	line := PastOrderLine{
		Name:      stringFromKeys(m, "name", "productName", "description", "title"),
		SKU:       stringFromKeys(m, "sku", "retailerProductId", "retailerProductID"),
		ProductID: stringFromKeys(m, "productId", "productID", "id"),
		Unit:      stringFromKeys(m, "unit", "unitOfMeasure"),
		Size:      stringFromKeys(m, "size", "format", "packaging"),
		Category:  stringFromKeys(m, "category", "categoryName"),
	}
	if product != nil {
		line.Name = firstString(line.Name, stringFromKeys(product, "name", "productName", "description", "title"))
		line.SKU = firstString(line.SKU, stringFromKeys(product, "sku", "retailerProductId", "retailerProductID"))
		line.ProductID = firstString(line.ProductID, stringFromKeys(product, "productId", "productID", "id"))
		line.Unit = firstString(line.Unit, stringFromKeys(product, "unit", "unitOfMeasure"))
		line.Size = firstString(line.Size, stringFromKeys(product, "size", "format", "packaging"))
		line.Category = firstString(line.Category, stringFromKeys(product, "category", "categoryName"))
	}
	line.Quantity = floatFromKeys(m, "quantity", "qty", "orderedQuantity", "amount")
	if line.Quantity == 0 && product != nil {
		line.Quantity = floatFromKeys(product, "quantity", "qty", "orderedQuantity", "amount")
	}
	if line.Name == "" && line.SKU == "" && line.ProductID == "" {
		return PastOrderLine{}, false
	}
	return line, true
}

func floatFromKeys(m map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if value := floatValue(m[key]); value > 0 {
			return value
		}
	}
	return 0
}

func floatValue(v any) float64 {
	switch t := v.(type) {
	case json.Number:
		value, _ := strconv.ParseFloat(t.String(), 64)
		return value
	case float64:
		return t
	case int:
		return float64(t)
	case string:
		value, _ := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(t), ",", "."), 64)
		return value
	case map[string]any:
		return floatFromKeys(t, "amount", "value", "quantity")
	default:
		return 0
	}
}
