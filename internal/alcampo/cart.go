package alcampo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"

	"github.com/wachtermar/carrito/internal/httpx"
	"github.com/wachtermar/carrito/internal/money"
)

var ErrCartUnsupported = errors.New("unknown or unsupported cart command; supported commands: get, add, set, set-many")

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
	rootMap, ok := root.(map[string]any)
	if !ok {
		return 0, false
	}

	// Cart totals are trusted only at documented cart-container locations. A
	// recursive "first totals object" fallback can mistake a line subtotal for
	// the whole cart and defeat the spending guard when an API shape changes.
	containers := authoritativeCartContainers(rootMap)

	var found *int64
	for _, container := range containers {
		if _, present := container["totals"]; !present {
			continue
		}
		cents, valid := cartContainerTotalCents(container)
		if !valid || cents < 0 {
			return 0, false
		}
		if found != nil && *found != cents {
			return 0, false
		}
		value := cents
		found = &value
	}
	if found == nil {
		return 0, false
	}
	return *found, true
}

// cartContainerTotalCents selects the most authoritative total available in a
// cart container. A final "total" takes precedence over the item subtotal;
// duplicate representations of the selected semantic value must agree.
func cartContainerTotalCents(container map[string]any) (int64, bool) {
	tiers := [][][]string{
		{
			{"totals", "display", "total"},
			{"totals", "total"},
		},
		{
			{"totals", "display", "itemPriceAfterPromos"},
			{"totals", "itemPriceAfterPromos"},
		},
	}
	for _, paths := range tiers {
		var found *int64
		present := false
		for _, path := range paths {
			value, exists := valueAtPath(container, path...)
			if !exists {
				continue
			}
			present = true
			cents, valid := centsFromValue(value)
			if !valid {
				return 0, false
			}
			if found != nil && *found != cents {
				return 0, false
			}
			amount := cents
			found = &amount
		}
		if present {
			if found == nil {
				return 0, false
			}
			return *found, true
		}
	}
	return 0, false
}

// CartProductQuantity returns one unambiguous quantity for an exact product
// identity. Repeated cart projections are accepted only when their identities
// and quantities agree; conflicting duplicates are an error, never a first
// match that can falsely satisfy write verification.
func CartProductQuantity(root any, productID string) (string, bool, error) {
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return "", false, errors.New("cart product identity cannot be empty")
	}
	target := strings.ToLower(productID)
	var quantity string
	var normalizedQuantity string
	ids := map[string]bool{}
	skus := map[string]bool{}
	var conflict error
	for _, m := range cartLineMaps(root) {
		if conflict != nil {
			break
		}
		product, identityErr := cartLineProductFromMap(m, "")
		if identityErr != nil {
			conflict = identityErr
			break
		}
		id := strings.ToLower(product.ID)
		sku := strings.ToLower(product.SKU)
		if id != target && sku != target {
			continue
		}
		candidate := quantityFromMap(m)
		if candidate == "" {
			continue
		}
		normalized, ok := normalizeCartQuantity(candidate)
		if !ok {
			conflict = fmt.Errorf("invalid cart quantity %q for product %s", candidate, productID)
			break
		}
		if id != "" {
			ids[id] = true
		}
		if sku != "" {
			skus[sku] = true
		}
		if len(ids) > 1 || len(skus) > 1 {
			conflict = fmt.Errorf("conflicting cart identities for product %s", productID)
			break
		}
		if normalizedQuantity != "" && normalizedQuantity != normalized {
			conflict = fmt.Errorf("conflicting cart quantities for product %s", productID)
			break
		}
		if normalizedQuantity == "" {
			normalizedQuantity = normalized
			quantity = candidate
		}
	}
	if conflict != nil {
		return "", false, conflict
	}
	if quantity == "" {
		return "", false, nil
	}
	return quantity, true, nil
}

var cartLineCollectionKeys = []string{
	"items", "cartItems", "cartLines", "basketLines", "lines",
	"itemGroups", "productGroups", "groups", "categories",
}

// cartLineMaps enumerates only documented cart containers and their explicit
// line/group collections. It deliberately does not recursively scan arbitrary
// response subtrees, where recommendations can carry product-like quantities.
func cartLineMaps(root any) []map[string]any {
	rootMap, ok := root.(map[string]any)
	if !ok {
		return nil
	}
	containers := authoritativeCartContainers(rootMap)

	var lines []map[string]any
	var collect func(any)
	collect = func(value any) {
		switch typed := value.(type) {
		case []any:
			for _, item := range typed {
				collect(item)
			}
		case map[string]any:
			if quantityFromMap(typed) != "" && cartLineHasProductIdentity(typed) {
				lines = append(lines, typed)
				return
			}
			for _, key := range cartLineCollectionKeys {
				if child, exists := typed[key]; exists {
					collect(child)
				}
			}
		}
	}
	for _, container := range containers {
		for _, key := range cartLineCollectionKeys {
			if value, exists := container[key]; exists {
				collect(value)
			}
		}
	}
	return lines
}

func authoritativeCartContainers(root map[string]any) []map[string]any {
	containers := []map[string]any{root}
	for _, key := range []string{"basket", "basketUpdateResult"} {
		if container, ok := root[key].(map[string]any); ok {
			containers = append(containers, container)
		}
	}
	if data, ok := root["data"].(map[string]any); ok {
		for _, key := range []string{"basket", "basketUpdateResult"} {
			if container, ok := data[key].(map[string]any); ok {
				containers = append(containers, container)
			}
		}
	}
	return containers
}

func cartLineHasProductIdentity(m map[string]any) bool {
	if stringFromKeys(m, "productId", "productID", "retailerProductId", "retailerProductID", "sku", "retailerSku") != "" {
		return true
	}
	for _, key := range []string{"product", "decoratedProduct", "bopData", "productData", "article"} {
		if child, ok := m[key].(map[string]any); ok && stringFromKeys(child, "productId", "productID", "id", "retailerProductId", "retailerProductID", "sku", "retailerSku") != "" {
			return true
		}
	}
	return false
}

// cartLineProductFromMap gives explicit nested products precedence over
// line-level fields. A generic line "id" is not a product identity: cart APIs
// commonly use it for the row itself. Every identified projection must be
// compatible; conflicting product/decorated-product data is rejected instead
// of silently verifying whichever projection happened to be visited first.
func cartLineProductFromMap(m map[string]any, baseURL string) (Product, error) {
	var product Product
	for _, key := range []string{"product", "decoratedProduct", "bopData", "productData", "article"} {
		child, ok := m[key].(map[string]any)
		if !ok {
			continue
		}
		candidate := productFromMap(child, baseURL)
		if candidate.ID == "" {
			candidate.ID = stringFromKeys(child, "productId", "productID", "id")
		}
		if candidate.SKU == "" {
			candidate.SKU = stringFromKeys(child, "retailerProductId", "retailerProductID", "sku", "retailerSku")
		}
		if candidate.ID == "" && candidate.SKU == "" {
			continue
		}
		if err := mergeCartLineProduct(&product, candidate); err != nil {
			return Product{}, fmt.Errorf("conflicting cart product projections: %w", err)
		}
	}

	direct := productFromMap(m, baseURL)
	direct.ID = stringFromKeys(m, "productId", "productID")
	direct.SKU = stringFromKeys(m, "retailerProductId", "retailerProductID", "sku", "retailerSku")
	if direct.ID != "" || direct.SKU != "" {
		if err := mergeCartLineProduct(&product, direct); err != nil {
			return Product{}, fmt.Errorf("conflicting cart line and product projection: %w", err)
		}
	} else {
		mergeProduct(&product, direct)
	}
	return product, nil
}

func mergeCartLineProduct(dst *Product, src Product) error {
	if dst.ID != "" && src.ID != "" && !strings.EqualFold(dst.ID, src.ID) {
		return fmt.Errorf("product ids %q and %q disagree", dst.ID, src.ID)
	}
	if dst.SKU != "" && src.SKU != "" && !strings.EqualFold(dst.SKU, src.SKU) {
		return fmt.Errorf("product SKUs %q and %q disagree", dst.SKU, src.SKU)
	}
	mergeProduct(dst, src)
	return nil
}

func normalizeCartQuantity(raw string) (string, bool) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, ",", "."))
	quantity, ok := new(big.Rat).SetString(raw)
	if !ok {
		return "", false
	}
	return quantity.RatString(), true
}

func CartItemCount(root any) int {
	seen := map[string]bool{}
	for _, m := range cartLineMaps(root) {
		if quantityFromMap(m) == "" {
			continue
		}
		product, err := cartLineProductFromMap(m, "")
		if err != nil {
			continue
		}
		identity := strings.ToLower(strings.TrimSpace(product.ID))
		if identity == "" {
			identity = strings.ToLower(strings.TrimSpace(product.SKU))
		}
		if identity != "" {
			seen[identity] = true
		}
	}
	return len(seen)
}

func centsAtPath(root any, path ...string) (int64, bool) {
	v, ok := valueAtPath(root, path...)
	if !ok {
		return 0, false
	}
	return centsFromValue(v)
}

func valueAtPath(root any, path ...string) (any, bool) {
	v := root
	for _, key := range path {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok = m[key]
		if !ok {
			return nil, false
		}
	}
	return v, true
}

func centsFromValue(v any) (int64, bool) {
	p := priceFrom(v)
	if p.Amount != "" {
		if p.Cents < 0 || (p.Currency != "" && !strings.EqualFold(p.Currency, "EUR")) {
			return 0, false
		}
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
