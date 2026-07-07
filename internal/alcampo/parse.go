package alcampo

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/wachtermar/carrito/internal/money"
)

func collectProducts(v any, baseURL string) []Product {
	var out []Product
	seen := map[string]bool{}
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case []any:
			for _, item := range t {
				walk(item)
			}
		case map[string]any:
			if isProductMap(t) {
				p := productFromMap(t, baseURL)
				key := p.SKU
				if key == "" {
					key = p.ID
				}
				if key != "" && !seen[key] {
					seen[key] = true
					out = append(out, p)
				}
				return
			}
			for _, key := range []string{"productGroups", "decoratedProducts", "products", "productEntities", "items", "results"} {
				if val, ok := t[key]; ok {
					walk(val)
				}
			}
		}
	}
	walk(v)
	return out
}

func isProductMap(m map[string]any) bool {
	if stringFromKeys(m, "retailerProductId", "productId", "sku") == "" {
		return false
	}
	return stringFromKeys(m, "name", "displayName", "productName") != "" || mapFromKeys(m, "price", "fopPrice") != nil
}

func productFromMap(m map[string]any, baseURL string) Product {
	p := Product{
		ID:          stringFromKeys(m, "productId", "id"),
		SKU:         stringFromKeys(m, "retailerProductId", "sku", "retailerSku"),
		Name:        stringFromKeys(m, "name", "displayName", "productName", "title"),
		Brand:       brandFrom(m["brand"]),
		Size:        stringFromKeys(m, "size", "format", "packSize", "packSizeDescription", "netContent"),
		EAN:         stringFromKeys(m, "ean", "gtin", "gtin13"),
		Description: stripHTML(stringFromKeys(m, "description", "shortDescription")),
	}
	if p.Brand == "" {
		p.Brand = stringFromKeys(m, "brandName")
	}
	p.Price = priceFrom(mapValue(m, "price", "fopPrice", "salesPrice"))
	p.UnitPrice, p.Unit = unitPriceFrom(mapValue(m, "unitPrice", "pricePerUnit", "referencePrice"))
	p.Available = boolFromKeys(m, "available", "isAvailable", "inStock")
	p.CategoryPath = stringSliceFrom(mapValue(m, "categoryPath", "categoriesPath", "breadcrumb"))
	if len(p.CategoryPath) > 0 {
		p.Category = p.CategoryPath[len(p.CategoryPath)-1]
	}
	if p.Category == "" {
		p.Category = stringFromKeys(m, "category", "categoryName")
	}
	p.URL = absoluteProductURL(baseURL, stringFromKeys(m, "url", "productUrl", "canonicalUrl", "href"), p.SKU)
	p.Images = imageURLs(baseURL, mapValue(m, "images", "image", "media"))
	p.Offers = offersFrom(mapValue(m, "promotions", "offers", "badges"))
	return p
}

func priceFrom(v any) money.Money {
	if v == nil {
		return money.Money{}
	}
	if m, ok := v.(map[string]any); ok {
		if nested := mapValue(m, "price", "amountWithCurrency"); nested != nil {
			if p := priceFrom(nested); p.Amount != "" {
				return p
			}
		}
		amount := stringFromKeys(m, "amount", "value", "price", "current")
		currency := stringFromKeys(m, "currency", "currencyCode")
		if amount != "" {
			if p, err := money.New(amount, currency); err == nil {
				return p
			}
		}
	}
	if s := scalarString(v); s != "" {
		if p, err := money.New(s, "EUR"); err == nil {
			return p
		}
	}
	return money.Money{}
}

func unitPriceFrom(v any) (money.Money, string) {
	if v == nil {
		return money.Money{}, ""
	}
	unit := ""
	if m, ok := v.(map[string]any); ok {
		unit = normalizeUnit(stringFromKeys(m, "unit", "unitOfMeasure", "measurementUnit", "referenceUnit"))
		for _, key := range []string{"price", "amount", "value"} {
			if val, ok := m[key]; ok {
				p := priceFrom(val)
				if p.Amount != "" {
					return p, unit
				}
			}
		}
	}
	return priceFrom(v), unit
}

func offersFrom(v any) []Offer {
	var offers []Offer
	switch t := v.(type) {
	case []any:
		for _, item := range t {
			if m, ok := item.(map[string]any); ok {
				o := Offer{
					ID:          stringFromKeys(m, "id", "promotionId", "code"),
					Name:        stringFromKeys(m, "name", "title", "label"),
					Description: stripHTML(stringFromKeys(m, "description", "shortDescription")),
					Type:        stringFromKeys(m, "type", "promotionType"),
				}
				if o.Name != "" || o.Description != "" {
					offers = append(offers, o)
				}
			} else if s := scalarString(item); s != "" {
				offers = append(offers, Offer{Name: s})
			}
		}
	case map[string]any:
		o := Offer{
			ID:          stringFromKeys(t, "id", "promotionId", "code"),
			Name:        stringFromKeys(t, "name", "title", "label"),
			Description: stripHTML(stringFromKeys(t, "description", "shortDescription")),
			Type:        stringFromKeys(t, "type", "promotionType"),
		}
		if o.Name != "" || o.Description != "" {
			offers = append(offers, o)
		}
	}
	return offers
}

func imageURLs(baseURL string, v any) []string {
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if strings.HasPrefix(s, "//") {
			s = "https:" + s
		}
		if strings.HasPrefix(s, "/") {
			s = baseURL + s
		}
		out = append(out, s)
	}
	switch t := v.(type) {
	case string:
		add(t)
	case []any:
		for _, item := range t {
			switch x := item.(type) {
			case string:
				add(x)
			case map[string]any:
				add(stringFromKeys(x, "url", "src", "imageUrl", "largeUrl", "mediumUrl"))
			}
		}
	case map[string]any:
		add(stringFromKeys(t, "url", "src", "imageUrl", "largeUrl", "mediumUrl"))
	}
	return out
}

func absoluteProductURL(baseURL, raw, sku string) string {
	if raw != "" {
		if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
			return raw
		}
		if strings.HasPrefix(raw, "/") {
			return baseURL + raw
		}
		return baseURL + "/" + strings.TrimPrefix(raw, "/")
	}
	return productPageURL(baseURL, sku)
}

func brandFrom(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case map[string]any:
		return stringFromKeys(t, "name", "brandName", "label")
	}
	return ""
}

func stringFromKeys(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if s := scalarString(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func mapFromKeys(m map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if child, ok := v.(map[string]any); ok {
				return child
			}
		}
	}
	return nil
}

func mapValue(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return nil
}

func scalarString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return t.String()
	case float64:
		return fmt.Sprintf("%g", t)
	case int:
		return fmt.Sprintf("%d", t)
	case int64:
		return fmt.Sprintf("%d", t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	}
	return ""
}

func floatFrom(v any) float64 {
	switch t := v.(type) {
	case json.Number:
		f, _ := t.Float64()
		return f
	case float64:
		return t
	case string:
		var f float64
		_, _ = fmt.Sscanf(t, "%f", &f)
		return f
	default:
		return 0
	}
}

func boolFromKeys(m map[string]any, keys ...string) *bool {
	for _, key := range keys {
		v, ok := m[key]
		if !ok {
			continue
		}
		switch t := v.(type) {
		case bool:
			return &t
		case string:
			b := strings.EqualFold(t, "true") || strings.EqualFold(t, "yes") || t == "1"
			return &b
		}
	}
	return nil
}

func stringSliceFrom(v any) []string {
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s := scalarString(item); s != "" {
				out = append(out, s)
				continue
			}
			if m, ok := item.(map[string]any); ok {
				if s := stringFromKeys(m, "name", "label", "title"); s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	case string:
		if t == "" {
			return nil
		}
		parts := strings.Split(t, ">")
		if len(parts) == 1 {
			parts = strings.Split(t, "/")
		}
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts
	}
	return nil
}

func normalizeUnit(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "fop.price.")
	s = strings.TrimPrefix(s, "per.")
	s = strings.ReplaceAll(s, ".", " ")
	s = strings.ReplaceAll(s, "_", " ")
	return strings.TrimSpace(s)
}

var tagRE = regexp.MustCompile(`(?s)<[^>]+>`)
var spaceRE = regexp.MustCompile(`\s+`)

func stripHTML(s string) string {
	s = strings.ReplaceAll(s, "<br>", "\n")
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br />", "\n")
	s = tagRE.ReplaceAllString(s, " ")
	s = htmlUnescape(s)
	s = spaceRE.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func htmlUnescape(s string) string {
	r := strings.NewReplacer(
		"&nbsp;", " ",
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
	)
	return r.Replace(s)
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	repl := strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n")
	s = repl.Replace(s)
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func pathSlug(raw string) string {
	if raw == "" {
		return ""
	}
	if u, err := url.Parse(raw); err == nil && u.Path != "" {
		raw = u.Path
	}
	return strings.Trim(path.Base(strings.Trim(raw, "/")), "/")
}
