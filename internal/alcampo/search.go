package alcampo

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

func (c *Client) Search(ctx context.Context, query string, opts SearchOptions) ([]Product, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, ErrEmptyQuery
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	regionID := opts.RegionID
	if regionID == "" {
		regionID = c.RegionID
	}
	q := url.Values{}
	q.Set("q", query)
	q.Set("tag", "web")
	q.Set("includeAdditionalPageInfo", "true")
	q.Set("maxProductsToDecorate", strconv.Itoa(limit))
	q.Set("maxPageSize", strconv.Itoa(limit))
	q.Set("regionId", regionID)
	if opts.Sort != "" {
		q.Set("sortOptionId", opts.Sort)
	}
	var root any
	err := c.getJSON(ctx, "/api/webproductpagews/v6/product-pages/search", q, c.BaseURL+"/search?q="+url.QueryEscape(query), "search", &root)
	if err != nil {
		return nil, err
	}
	products := collectProducts(root, c.BaseURL)
	if opts.Fresh {
		products = filterFresh(products)
	}
	if len(products) > limit {
		products = products[:limit]
	}
	return products, nil
}

func (c *Client) Lookup(ctx context.Context, ref string) (Product, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return Product{}, ErrEmptyQuery
	}
	products, err := c.Search(ctx, ref, SearchOptions{Limit: 8})
	if err != nil {
		return Product{}, err
	}
	for _, p := range products {
		if strings.EqualFold(p.SKU, ref) || strings.EqualFold(p.ID, ref) {
			return p, nil
		}
	}
	if len(products) == 0 {
		return Product{}, ErrNotFound
	}
	return products[0], nil
}

func filterFresh(products []Product) []Product {
	var out []Product
	for _, p := range products {
		text := strings.ToLower(strings.Join(append(p.CategoryPath, p.Category, p.Name), " "))
		if strings.Contains(text, "fruta") ||
			strings.Contains(text, "verdura") ||
			strings.Contains(text, "carne") ||
			strings.Contains(text, "pescado") ||
			strings.Contains(text, "charcut") ||
			strings.Contains(text, "panader") ||
			strings.Contains(text, "lácteo") ||
			strings.Contains(text, "lacteo") ||
			strings.Contains(text, "huevo") {
			out = append(out, p)
		}
	}
	return out
}
