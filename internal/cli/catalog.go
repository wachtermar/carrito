package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/output"
)

func runSearch(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("search", stderr)
	limit := fs.Int("limit", 10, "maximum products to return")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	store := marketFlag(fs, "Alcampo region/store UUID to use for pricing")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	query := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if query == "" {
		return errors.New("search requires a query")
	}
	if *limit < 1 || *limit > 50 {
		return errors.New("--limit must be between 1 and 50")
	}
	cfg, client, err := newClient(*store)
	if err != nil {
		return err
	}
	regionID, err := resolveMarket(cfg, *store)
	if err != nil {
		return err
	}
	products, err := client.Search(context.Background(), query, alcampo.SearchOptions{Limit: *limit, RegionID: regionID})
	if err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, products)
	}
	for _, product := range products {
		printProductLine(stdout, product)
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
	product, err := client.Product(context.Background(), ref)
	if err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, product)
	}
	printProductDetail(stdout, product)
	return nil
}

func printProductLine(w io.Writer, product alcampo.Product) {
	availability := "availability unknown"
	if product.Available != nil {
		if *product.Available {
			availability = "available"
		} else {
			availability = "unavailable"
		}
	}
	fmt.Fprintf(w, "%s\t€%s\t%s\t%s\t%s\n", firstNonEmpty(product.SKU, product.ID, "-"), product.Price.Amount, product.Size, availability, product.Name)
}

func printProductDetail(w io.Writer, product alcampo.Product) {
	printProductLine(w, product)
	if product.Brand != "" {
		fmt.Fprintf(w, "brand\t%s\n", product.Brand)
	}
	if product.URL != "" {
		fmt.Fprintf(w, "url\t%s\n", product.URL)
	}
	if product.Ingredients != "" {
		fmt.Fprintf(w, "ingredients\t%s\n", product.Ingredients)
	}
	if product.Allergens != "" {
		fmt.Fprintf(w, "allergens\t%s\n", product.Allergens)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
