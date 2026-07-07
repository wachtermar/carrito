package alcampo

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

var (
	ErrEmptyQuery = errors.New("empty query")
	ErrNotFound   = errors.New("not found")
)

func (c *Client) Categories(ctx context.Context, depth int) ([]Category, error) {
	if depth <= 0 {
		depth = 3
	}
	q := url.Values{}
	q.Set("decoration", "false")
	q.Set("categoryDepth", strconv.Itoa(depth))
	var root any
	err := c.getJSON(ctx, "/api/webproductpagews/v1/categories", q, c.BaseURL+"/", "categories", &root)
	if err != nil {
		return nil, err
	}
	return collectCategories(root, "", ""), nil
}

func (c *Client) CategoryProducts(ctx context.Context, cat Category, opts CategoryProductsOptions) ([]Product, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	regionID := opts.RegionID
	if regionID == "" {
		regionID = c.RegionID
	}
	q := url.Values{}
	if cat.RetailerID != "" {
		q.Set("retailerCategoryId", cat.RetailerID)
	} else if cat.ID != "" {
		q.Set("categoryId", cat.ID)
	} else {
		return nil, ErrNotFound
	}
	q.Set("tag", "web")
	q.Set("includeAdditionalPageInfo", "true")
	q.Set("maxProductsToDecorate", strconv.Itoa(limit))
	q.Set("maxPageSize", strconv.Itoa(limit))
	q.Set("regionId", regionID)
	var root any
	err := c.getJSON(ctx, "/api/webproductpagews/v6/product-pages", q, c.BaseURL+"/"+strings.TrimPrefix(cat.Path, "/"), "category", &root)
	if err != nil {
		return nil, err
	}
	products := collectProducts(root, c.BaseURL)
	if len(products) > limit {
		products = products[:limit]
	}
	return products, nil
}

func FindCategory(categories []Category, ref string) (Category, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return Category{}, false
	}
	refFold := strings.ToLower(ref)
	refSlug := slugify(pathSlug(ref))
	for _, cat := range FlattenCategories(categories) {
		if strings.EqualFold(cat.ID, ref) ||
			strings.EqualFold(cat.RetailerID, ref) ||
			strings.EqualFold(cat.Slug, ref) ||
			strings.EqualFold(slugify(cat.Name), refSlug) ||
			strings.EqualFold(slugify(cat.Path), refSlug) ||
			strings.Contains(strings.ToLower(cat.Path), refFold) {
			return cat, true
		}
	}
	return Category{}, false
}

func FlattenCategories(categories []Category) []Category {
	var out []Category
	var walk func([]Category)
	walk = func(cats []Category) {
		for _, cat := range cats {
			out = append(out, cat)
			walk(cat.Children)
		}
	}
	walk(categories)
	return out
}

func collectCategories(v any, parentPath, parentSlug string) []Category {
	var out []Category
	switch t := v.(type) {
	case []any:
		for _, item := range t {
			out = append(out, collectCategories(item, parentPath, parentSlug)...)
		}
	case map[string]any:
		if isCategoryMap(t) {
			name := stringFromKeys(t, "name", "label", "title")
			rawPath := stringFromKeys(t, "fullURLPath", "url", "path")
			slug := stringFromKeys(t, "slug", "categorySlug")
			if slug == "" {
				slug = pathSlug(rawPath)
			}
			if slug == "" {
				slug = slugify(name)
			}
			catPath := rawPath
			if catPath == "" && parentPath != "" {
				catPath = strings.TrimRight(parentPath, "/") + "/" + slug
			}
			cat := Category{
				ID:         stringFromKeys(t, "categoryId", "id"),
				RetailerID: stringFromKeys(t, "retailerCategoryId", "retailerId", "code"),
				Slug:       slug,
				Name:       name,
				Path:       catPath,
			}
			for _, key := range []string{"children", "categories", "subCategories", "childCategories"} {
				if child, ok := t[key]; ok {
					cat.Children = collectCategories(child, cat.Path, cat.Slug)
					break
				}
			}
			out = append(out, cat)
			return out
		}
		for _, key := range []string{"categories", "children", "items"} {
			if child, ok := t[key]; ok {
				out = append(out, collectCategories(child, parentPath, parentSlug)...)
			}
		}
	}
	return out
}

func isCategoryMap(m map[string]any) bool {
	return stringFromKeys(m, "name", "label", "title") != "" &&
		(stringFromKeys(m, "categoryId", "id", "retailerCategoryId", "retailerId", "code") != "" ||
			mapValue(m, "children", "categories", "subCategories", "childCategories") != nil)
}
