package alcampo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/wachtermar/carrito/internal/strutil"
)

func (c *Client) Product(ctx context.Context, ref string) (Product, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return Product{}, ErrEmptyQuery
	}
	productRef := extractProductRef(ref)
	if productRef == "" || strings.ContainsAny(productRef, " \t") {
		return c.productFromSearch(ctx, ref, "")
	}

	body, err := c.getPage(ctx, "/products/"+url.PathEscape(productRef), c.BaseURL+"/")
	if err != nil {
		p, lookupErr := c.productFromSearch(ctx, productRef, err.Error())
		if lookupErr != nil {
			return Product{}, err
		}
		return p, nil
	}
	p, ok := productFromHTML(body, c.BaseURL)
	if !ok {
		p, lookupErr := c.productFromSearch(ctx, productRef, "product detail JSON was not found in page")
		if lookupErr != nil {
			return Product{}, lookupErr
		}
		return p, nil
	}
	if !strings.EqualFold(p.SKU, productRef) && !strings.EqualFold(p.ID, productRef) {
		p, lookupErr := c.productFromSearch(ctx, productRef, "product detail did not identify the exact requested SKU or ID")
		if lookupErr != nil {
			return Product{}, fmt.Errorf("%w: detail page returned SKU %q and ID %q for requested ref %q", ErrNotFound, p.SKU, p.ID, productRef)
		}
		return p, nil
	}
	listingRef := strutil.FirstNonEmpty(p.SKU, productRef)
	listing, err := c.Lookup(ctx, listingRef)
	if err != nil {
		return Product{}, fmt.Errorf("refresh current market listing for %q: %w", listingRef, err)
	}
	// Search is region-aware, so its identity, price, size, and availability are
	// authoritative. The detail page is used only to enrich static label data.
	mergeProductDetails(&listing, p)
	if listing.URL == "" {
		listing.URL = productPageURL(c.BaseURL, listing.SKU)
	}
	return listing, nil
}

func (c *Client) productFromSearch(ctx context.Context, ref, detailMessage string) (Product, error) {
	p, err := c.Lookup(ctx, ref)
	if err != nil {
		return Product{}, err
	}
	p.DetailUnavailable = true
	if detailMessage == "" {
		detailMessage = "detail page was unavailable; returned listing data"
	}
	p.DetailMessage = detailMessage
	return p, nil
}

func extractProductRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if u, err := url.Parse(ref); err == nil && u.Path != "" && (u.Scheme != "" || strings.Contains(ref, "/")) {
		base := path.Base(strings.Trim(u.Path, "/"))
		if base != "." && base != "/" {
			return base
		}
	}
	ref = strings.Trim(ref, "/")
	if strings.Contains(ref, "/") {
		return path.Base(ref)
	}
	return ref
}

var jsonLDRE = regexp.MustCompile(`(?is)<script[^>]*data-test=["']product-details-structured-data["'][^>]*>(.*?)</script>`)

func productFromHTML(body []byte, baseURL string) (Product, bool) {
	var p Product
	found := false
	if m := jsonLDRE.FindSubmatch(body); len(m) == 2 {
		if prod, ok := productFromJSONLD(m[1], baseURL); ok {
			p = prod
			found = true
		}
	}
	if raw := extractAssignment(body, "__QUERY_INITIAL_STATE__"); len(raw) > 0 {
		var root any
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&root); err == nil {
			found = applyDetailState(&p, root, baseURL) || found
		}
	}
	if raw := extractAssignment(body, "__INITIAL_STATE__"); len(raw) > 0 {
		var root any
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&root); err == nil {
			products := collectProducts(root, baseURL)
			for _, product := range products {
				if mergeCompatibleProduct(&p, product) {
					found = true
					break
				}
			}
		}
	}
	return p, found && (p.Name != "" || p.SKU != "")
}

func productFromJSONLD(raw []byte, baseURL string) (Product, bool) {
	raw = []byte(html.UnescapeString(strings.TrimSpace(string(raw))))
	var root any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil {
		return Product{}, false
	}
	m := findMap(root, func(m map[string]any) bool {
		return stringFromKeys(m, "sku", "name") != ""
	})
	if m == nil {
		return Product{}, false
	}
	p := Product{
		SKU:         stringFromKeys(m, "sku", "productID"),
		Name:        stringFromKeys(m, "name"),
		Brand:       brandFrom(m["brand"]),
		Description: stripHTML(stringFromKeys(m, "description")),
		EAN:         stringFromKeys(m, "gtin13", "gtin", "mpn"),
		URL:         absoluteProductURL(baseURL, stringFromKeys(m, "url"), stringFromKeys(m, "sku")),
		Images:      imageURLs(baseURL, mapValue(m, "image")),
	}
	if offers, ok := m["offers"].(map[string]any); ok {
		p.Price = priceFrom(map[string]any{
			"amount":   mapValue(offers, "price", "lowPrice"),
			"currency": mapValue(offers, "priceCurrency", "currency"),
		})
	} else if offers, ok := m["offers"].([]any); ok && len(offers) > 0 {
		if first, ok := offers[0].(map[string]any); ok {
			p.Price = priceFrom(map[string]any{
				"amount":   mapValue(first, "price", "lowPrice"),
				"currency": mapValue(first, "priceCurrency", "currency"),
			})
		}
	}
	return p, true
}

func applyDetailState(p *Product, root any, baseURL string) bool {
	applied := false
	walk(root, func(m map[string]any) {
		if bop, ok := m["bopData"].(map[string]any); ok {
			contextProduct := productFromMap(m, baseURL)
			if nested, ok := m["product"].(map[string]any); ok {
				nestedProduct := productFromMap(nested, baseURL)
				if contextProduct.SKU == "" {
					contextProduct.SKU = nestedProduct.SKU
				}
				if contextProduct.ID == "" {
					contextProduct.ID = nestedProduct.ID
				}
			}
			applied = applyBOP(p, bop, contextProduct, baseURL) || applied
		}
		if data, ok := m["data"].(map[string]any); ok && stringFromKeys(data, "detailedDescription", "longDescription") != "" {
			applied = applyBOP(p, data, productFromMap(data, baseURL), baseURL) || applied
		}
	})
	return applied
}

func applyBOP(p *Product, m map[string]any, contextProduct Product, baseURL string) bool {
	detail := productFromMap(m, baseURL)
	if detail.SKU == "" {
		detail.SKU = contextProduct.SKU
	}
	if detail.ID == "" {
		detail.ID = contextProduct.ID
	}
	// Label data is safety-sensitive. Do not attribute an anonymous nested
	// object to the page product merely because it appears in the same state
	// blob; require an explicit matching SKU or internal ID in the object or
	// its immediate parent.
	if detail.SKU == "" && detail.ID == "" {
		return false
	}
	if !mergeCompatibleProduct(p, detail) {
		return false
	}
	if p.Description == "" {
		p.Description = stripHTML(stringFromKeys(m, "detailedDescription", "longDescription", "description"))
	}
	if path := stringSliceFrom(mapValue(m, "breadcrumbs", "breadCrumbs", "categoryPath")); len(path) > 0 {
		p.CategoryPath = path
		p.Category = path[len(path)-1]
	}
	scanLabelledDetails(p, m)
	return true
}

func scanLabelledDetails(p *Product, root any) {
	var scan func(any)
	scan = func(value any) {
		switch typed := value.(type) {
		case []any:
			for _, item := range typed {
				scan(item)
			}
		case map[string]any:
			embedded := Product{
				ID:  stringFromKeys(typed, "productId"),
				SKU: stringFromKeys(typed, "retailerProductId", "sku", "retailerSku"),
			}
			if nested, ok := typed["product"].(map[string]any); ok {
				if embedded.ID == "" {
					embedded.ID = stringFromKeys(nested, "productId")
				}
				if embedded.SKU == "" {
					embedded.SKU = stringFromKeys(nested, "retailerProductId", "sku", "retailerSku")
				}
			}
			if (embedded.ID != "" || embedded.SKU != "") && !compatibleProductIdentity(*p, embedded) {
				return
			}
			m := typed
			label := strings.ToLower(stripHTML(stringFromKeys(m, "title", "name", "label", "key", "fieldName", "heading")))
			value := stripHTML(stringFromKeys(m, "value", "text", "content", "html", "fieldValue", "description"))
			if label != "" && value != "" {
				switch {
				case strings.Contains(label, "ingred"):
					if p.Ingredients == "" {
						p.Ingredients = value
					}
				case strings.Contains(label, "alerg"):
					if p.Allergens == "" {
						p.Allergens = value
					}
				case strings.Contains(label, "nutric"):
					if p.Nutrition == "" {
						p.Nutrition = value
					}
				case strings.Contains(label, "descrip"):
					if p.Description == "" {
						p.Description = value
					}
				}
			}
			for _, child := range m {
				scan(child)
			}
		}
	}
	scan(root)
}

func mergeProduct(dst *Product, src Product) {
	if dst.ID == "" {
		dst.ID = src.ID
	}
	if dst.SKU == "" {
		dst.SKU = src.SKU
	}
	if dst.Name == "" {
		dst.Name = src.Name
	}
	if dst.Brand == "" {
		dst.Brand = src.Brand
	}
	if dst.Price.Amount == "" {
		dst.Price = src.Price
	}
	if dst.UnitPrice.Amount == "" {
		dst.UnitPrice = src.UnitPrice
	}
	if dst.Unit == "" {
		dst.Unit = src.Unit
	}
	if dst.Size == "" {
		dst.Size = src.Size
	}
	if dst.URL == "" {
		dst.URL = src.URL
	}
	if dst.Category == "" {
		dst.Category = src.Category
	}
	if len(dst.CategoryPath) == 0 {
		dst.CategoryPath = src.CategoryPath
	}
	if dst.Available == nil {
		dst.Available = src.Available
	}
	if dst.EAN == "" {
		dst.EAN = src.EAN
	}
	if dst.Description == "" {
		dst.Description = src.Description
	}
	if dst.Ingredients == "" {
		dst.Ingredients = src.Ingredients
	}
	if dst.Allergens == "" {
		dst.Allergens = src.Allergens
	}
	if dst.Nutrition == "" {
		dst.Nutrition = src.Nutrition
	}
	dst.Images = appendUnique(dst.Images, src.Images...)
	if len(dst.Offers) == 0 {
		dst.Offers = src.Offers
	}
}

// mergeCompatibleProduct refuses to combine two identified products unless
// every identity field they share agrees and at least one field is comparable.
// This prevents unrelated products embedded elsewhere in a page from donating
// an internal ID, price, or availability to the requested SKU.
func mergeCompatibleProduct(dst *Product, src Product) bool {
	if !compatibleProductIdentity(*dst, src) {
		return false
	}
	mergeProduct(dst, src)
	return true
}

func compatibleProductIdentity(dst, src Product) bool {
	dstIdentified := dst.SKU != "" || dst.ID != ""
	srcIdentified := src.SKU != "" || src.ID != ""
	if dstIdentified && srcIdentified {
		compared := false
		if dst.SKU != "" && src.SKU != "" {
			compared = true
			if !strings.EqualFold(dst.SKU, src.SKU) {
				return false
			}
		}
		if dst.ID != "" && src.ID != "" {
			compared = true
			if !strings.EqualFold(dst.ID, src.ID) {
				return false
			}
		}
		if !compared {
			return false
		}
	}
	return true
}

func mergeProductDetails(dst *Product, detail Product) {
	if dst.Description == "" {
		dst.Description = detail.Description
	}
	if dst.Ingredients == "" {
		dst.Ingredients = detail.Ingredients
	}
	if dst.Allergens == "" {
		dst.Allergens = detail.Allergens
	}
	if dst.Nutrition == "" {
		dst.Nutrition = detail.Nutrition
	}
	if dst.EAN == "" {
		dst.EAN = detail.EAN
	}
	if dst.URL == "" {
		dst.URL = detail.URL
	}
	dst.Images = appendUnique(dst.Images, detail.Images...)
}

func appendUnique(dst []string, values ...string) []string {
	seen := map[string]bool{}
	for _, v := range dst {
		seen[v] = true
	}
	for _, v := range values {
		if v != "" && !seen[v] {
			dst = append(dst, v)
			seen[v] = true
		}
	}
	return dst
}

func findMap(root any, pred func(map[string]any) bool) map[string]any {
	var found map[string]any
	walk(root, func(m map[string]any) {
		if found == nil && pred(m) {
			found = m
		}
	})
	return found
}

func walk(root any, visit func(map[string]any)) {
	switch t := root.(type) {
	case map[string]any:
		visit(t)
		for _, v := range t {
			walk(v, visit)
		}
	case []any:
		for _, v := range t {
			walk(v, visit)
		}
	}
}

func extractAssignment(body []byte, name string) []byte {
	idx := bytes.Index(body, []byte(name))
	if idx < 0 {
		return nil
	}
	rest := body[idx+len(name):]
	start := -1
	for i, b := range rest {
		if b == '{' || b == '[' {
			start = i
			break
		}
		if b == '<' {
			return nil
		}
	}
	if start < 0 {
		return nil
	}
	data := rest[start:]
	open := data[0]
	close := byte('}')
	if open == '[' {
		close = ']'
	}
	depth := 0
	inString := false
	escaped := false
	for i, b := range data {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if b == '\\' {
				escaped = true
				continue
			}
			if b == '"' {
				inString = false
			}
			continue
		}
		if b == '"' {
			inString = true
			continue
		}
		if b == open {
			depth++
			continue
		}
		if b == close {
			depth--
			if depth == 0 {
				return data[:i+1]
			}
		}
	}
	return nil
}
