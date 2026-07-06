package alcampo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"alcampo-cli/internal/httpx"
	"alcampo-cli/internal/money"
)

var ErrCartUnsupported = errors.New("unknown or unsupported cart command; supported commands: get, add, set, set-many, clear")

type CartQuantityChange struct {
	ProductID string      `json:"productId"`
	Quantity  json.Number `json:"quantity"`
}

func (c *Client) Cart(ctx context.Context, view bool) (any, error) {
	var root any
	path := "/api/cart/v1/carts/active"
	q := url.Values{}
	if view {
		path = "/api/cart/v2/carts/active/cart-view"
		q.Set("productGroupingType", "CATEGORIES")
	}
	if err := retryTransient(func() error {
		return c.getJSON(ctx, path, q, c.BaseURL+"/basket", "basket", &root)
	}); err != nil {
		return nil, err
	}
	return root, nil
}

func (c *Client) DecorateProducts(ctx context.Context, productIDs []string) ([]Product, error) {
	ids := uniqueNonEmpty(productIDs)
	if len(ids) == 0 {
		return nil, nil
	}
	q := url.Values{}
	q.Set("regionId", c.RegionID)
	var root any
	if err := retryTransient(func() error {
		return c.putJSON(ctx, "/api/webproductpagews/v6/products?"+q.Encode(), ids, c.BaseURL+"/basket", "basket", &root)
	}); err != nil {
		return nil, err
	}
	return collectProducts(root, c.BaseURL), nil
}

func (c *Client) ApplyCartQuantity(ctx context.Context, items []CartQuantityChange) (any, error) {
	var root any
	if err := c.postJSON(ctx, "/api/cart/v1/carts/active/apply-quantity?cartProductSorting=CATEGORIES", items, c.BaseURL+"/basket", "basket", &root); err != nil {
		return nil, err
	}
	return root, nil
}

func (c *Client) ClearCart(ctx context.Context) (any, error) {
	var root any
	if err := c.deleteJSON(ctx, "/api/cart/v1/carts/active/items", c.BaseURL+"/basket", "basket", &root); err != nil {
		return nil, err
	}
	return root, nil
}

func uniqueNonEmpty(values []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func retryTransient(fn func() error) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}
		if !isTransientStatus(err) || attempt == 2 {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 250 * time.Millisecond)
	}
	return err
}

func isTransientStatus(err error) bool {
	var statusErr *httpx.StatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode == 429 || statusErr.StatusCode >= 500
	}
	return false
}

func CartTotalCents(root any) (int64, bool) {
	paths := [][]string{
		{"totals", "display", "itemPriceAfterPromos", "amount"},
		{"totals", "display", "total", "amount"},
		{"totals", "itemPriceAfterPromos", "amount"},
		{"totals", "total", "amount"},
		{"basket", "totals", "display", "itemPriceAfterPromos", "amount"},
		{"basket", "totals", "display", "total", "amount"},
		{"basket", "totals", "itemPriceAfterPromos", "amount"},
		{"basket", "totals", "total", "amount"},
		{"data", "basket", "totals", "display", "itemPriceAfterPromos", "amount"},
		{"data", "basket", "totals", "display", "total", "amount"},
	}
	for _, path := range paths {
		if cents, ok := centsAtPath(root, path...); ok {
			return cents, true
		}
	}

	var found *int64
	walk(root, func(m map[string]any) {
		if found != nil {
			return
		}
		totals := mapFromKeys(m, "totals")
		if totals == nil {
			return
		}
		for _, path := range [][]string{
			{"display", "itemPriceAfterPromos", "amount"},
			{"display", "total", "amount"},
			{"itemPriceAfterPromos", "amount"},
			{"total", "amount"},
		} {
			if cents, ok := centsAtPath(totals, path...); ok {
				found = &cents
				return
			}
		}
	})
	if found != nil {
		return *found, true
	}
	return 0, false
}

func CartProductQuantity(root any, productID string) (string, bool) {
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return "", false
	}
	target := strings.ToLower(productID)
	var quantity string
	walk(root, func(m map[string]any) {
		if quantity != "" {
			return
		}
		id := strings.ToLower(stringFromKeys(m, "productId", "id"))
		if id != target {
			return
		}
		quantity = quantityFromMap(m)
	})
	if quantity == "" {
		return "", false
	}
	return quantity, true
}

func CartItemCount(root any) int {
	seen := map[string]bool{}
	walk(root, func(m map[string]any) {
		id := stringFromKeys(m, "productId")
		if id == "" {
			return
		}
		if quantityFromMap(m) == "" {
			return
		}
		seen[id] = true
	})
	return len(seen)
}

func centsAtPath(root any, path ...string) (int64, bool) {
	v := root
	for _, key := range path {
		m, ok := v.(map[string]any)
		if !ok {
			return 0, false
		}
		v, ok = m[key]
		if !ok {
			return 0, false
		}
	}
	return centsFromValue(v)
}

func centsFromValue(v any) (int64, bool) {
	p := priceFrom(v)
	if p.Amount != "" {
		return p.Cents, true
	}
	if s := scalarString(v); s != "" {
		p, err := money.New(s, "EUR")
		if err == nil {
			return p.Cents, true
		}
	}
	return 0, false
}

func quantityFromMap(m map[string]any) string {
	for _, key := range []string{"quantity", "quantityInBasket", "qty"} {
		if q := quantityString(m[key]); q != "" {
			return q
		}
	}
	return ""
}

func quantityString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case json.Number:
		return t.String()
	case string:
		return strings.TrimSpace(t)
	case float64:
		return fmt.Sprintf("%g", t)
	case int:
		return fmt.Sprintf("%d", t)
	case int64:
		return fmt.Sprintf("%d", t)
	case map[string]any:
		for _, key := range []string{"quantityInBasket", "quantity", "amount", "value"} {
			if q := quantityString(t[key]); q != "" {
				return q
			}
		}
	}
	return ""
}
