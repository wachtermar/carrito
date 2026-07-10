package alcampo

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

var (
	ErrEmptyQuery = errors.New("query cannot be empty")
	ErrNotFound   = errors.New("product not found")
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
	var root any
	err := c.getJSON(ctx, "/api/webproductpagews/v6/product-pages/search", q, c.BaseURL+"/search?q="+url.QueryEscape(query), "search", &root)
	if err != nil {
		return nil, err
	}
	products := collectProducts(root, c.BaseURL)
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
	if err == nil {
		for _, p := range products {
			if strings.EqualFold(p.SKU, ref) || strings.EqualFold(p.ID, ref) {
				return p, nil
			}
		}
	}
	decorated, decorateErr := c.DecorateProducts(ctx, []string{ref})
	if decorateErr == nil {
		for _, p := range decorated {
			if strings.EqualFold(p.ID, ref) || strings.EqualFold(p.SKU, ref) {
				return p, nil
			}
		}
	}
	if err != nil {
		return Product{}, err
	}
	return Product{}, fmt.Errorf("%w: no exact id or SKU matched %q", ErrNotFound, ref)
}
