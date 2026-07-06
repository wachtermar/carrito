package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"alcampo-cli/internal/alcampo"
	"alcampo-cli/internal/output"
	"alcampo-cli/internal/strutil"
)

func runSearch(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("search", stderr)
	limit := fs.Int("limit", 10, "maximum products to return")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	postal := fs.String("postal", "", "postal code hint; anonymous resolver is not verified")
	store := marketFlag(fs, "Alcampo region/store UUID to use for pricing")
	sort := fs.String("sort", "", "site sort option id")
	fresh := fs.Bool("fresh", false, "locally filter likely fresh-food categories")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true, "fresh": true}); err != nil {
		return err
	}
	query := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if query == "" {
		return errors.New("search requires a query")
	}
	cfg, client, err := newClient(*store)
	if err != nil {
		return err
	}
	regionID, err := resolveMarket(cfg, *store)
	if err != nil {
		return err
	}
	warnLocation(stderr, cfg, *postal, *store)
	products, err := client.Search(context.Background(), query, alcampo.SearchOptions{
		Limit:    *limit,
		RegionID: regionID,
		Sort:     *sort,
		Fresh:    *fresh,
	})
	if err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, products)
	}
	for _, p := range products {
		printProductLine(stdout, p)
	}
	return nil
}

func runProduct(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("product", stderr)
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	store := marketFlag(fs, "Alcampo region/store UUID to use when merging listing data")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	ref := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if ref == "" {
		return errors.New("product requires an id, sku, URL, or slug")
	}
	cfg, client, err := newClient(*store)
	if err != nil {
		return err
	}
	regionID, err := resolveMarket(cfg, *store)
	if err != nil {
		return err
	}
	client.RegionID = regionID
	p, err := client.Product(context.Background(), ref)
	if err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, p)
	}
	printProductDetail(stdout, p)
	return nil
}

func runCategories(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("categories", stderr)
	id := fs.String("id", "", "category id, retailer id, slug, or path to list")
	limit := fs.Int("limit", 20, "maximum products to return with --id")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	store := marketFlag(fs, "Alcampo region/store UUID to use for products")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	cfg, client, err := newClient(*store)
	if err != nil {
		return err
	}
	cats, err := client.Categories(context.Background(), 3)
	if err != nil {
		return err
	}
	if *id == "" {
		if *jsonOut {
			return output.JSON(stdout, cats)
		}
		printCategories(stdout, cats, 0)
		return nil
	}
	cat, ok := alcampo.FindCategory(cats, *id)
	if !ok {
		return fmt.Errorf("category %q not found", *id)
	}
	regionID, err := resolveMarket(cfg, *store)
	if err != nil {
		return err
	}
	products, err := client.CategoryProducts(context.Background(), cat, alcampo.CategoryProductsOptions{Limit: *limit, RegionID: regionID})
	if err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, struct {
			Category alcampo.Category  `json:"category"`
			Products []alcampo.Product `json:"products"`
		}{Category: cat, Products: products})
	}
	fmt.Fprintf(stdout, "%s (%s)\n", cat.Name, strutil.FirstNonEmpty(cat.RetailerID, cat.ID))
	for _, p := range products {
		printProductLine(stdout, p)
	}
	return nil
}

func runBatch(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("batch", stderr)
	file := fs.String("file", "", "file with one query per line, or - for stdin")
	fs.StringVar(file, "f", "", "file with one query per line, or - for stdin")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	store := marketFlag(fs, "Alcampo region/store UUID to use for pricing")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("batch requires -f <file|->")
	}
	cfg, client, err := newClient(*store)
	if err != nil {
		return err
	}
	regionID, err := resolveMarket(cfg, *store)
	if err != nil {
		return err
	}
	r, closeFn, err := openInput(*file)
	if err != nil {
		return err
	}
	defer closeFn()
	results := []BatchResult{}
	scanner := bufio.NewScanner(r)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		query := strings.TrimSpace(stripComment(scanner.Text()))
		if query == "" {
			continue
		}
		res := BatchResult{LineNo: lineNo, Query: query}
		products, err := client.Search(context.Background(), query, alcampo.SearchOptions{Limit: 1, RegionID: regionID})
		if err != nil {
			res.Error = err.Error()
		} else if len(products) == 0 {
			res.Error = "not found"
		} else {
			p := products[0]
			res.Product = &p
		}
		results = append(results, res)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, results)
	}
	for _, res := range results {
		if res.Error != "" {
			fmt.Fprintf(stdout, "%s\tERROR\t%s\n", res.Query, res.Error)
			continue
		}
		fmt.Fprintf(stdout, "%s\t", res.Query)
		printProductLine(stdout, *res.Product)
	}
	return nil
}

type BatchResult struct {
	LineNo  int              `json:"line_no"`
	Query   string           `json:"query"`
	Product *alcampo.Product `json:"product,omitempty"`
	Error   string           `json:"error,omitempty"`
}
