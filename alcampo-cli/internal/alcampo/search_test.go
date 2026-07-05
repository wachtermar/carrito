package alcampo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"alcampo-cli/internal/config"
)

func TestSearchParsesDecoratedProducts(t *testing.T) {
	var sawSourceHeader bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html></html>`))
		case "/api/webproductpagews/v6/product-pages/search":
			sawSourceHeader = r.Header.Get("ecom-request-source") == "web"
			if got := r.URL.Query().Get("q"); got != "leche" {
				t.Fatalf("q = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"productGroups": []any{
					map[string]any{"decoratedProducts": []any{
						map[string]any{
							"productId":         "internal-1",
							"retailerProductId": "54178",
							"name":              "AUCHAN Leche entera",
							"brand":             "AUCHAN",
							"price":             map[string]any{"amount": "5.76", "currency": "EUR"},
							"unitPrice":         map[string]any{"price": map[string]any{"amount": "0.96", "currency": "EUR"}, "unit": "fop.price.per.litre"},
							"available":         true,
							"categoryPath":      []string{"Leche", "Leche entera"},
						},
					}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := New(config.Default())
	if err != nil {
		t.Fatal(err)
	}
	client.BaseURL = server.URL
	products, err := client.Search(context.Background(), "leche", SearchOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !sawSourceHeader {
		t.Fatal("missing web source header")
	}
	if len(products) != 1 {
		t.Fatalf("got %d products", len(products))
	}
	p := products[0]
	if p.SKU != "54178" || p.Price.Cents != 576 || p.Unit != "litre" || p.Category != "Leche entera" {
		t.Fatalf("unexpected product: %+v", p)
	}
}

func TestProductFromHTML(t *testing.T) {
	body := []byte(`<html><script data-test="product-details-structured-data">{"sku":"54178","name":"Milk","brand":{"name":"AUCHAN"},"offers":{"price":"5.76","priceCurrency":"EUR"},"image":["/img.jpg"]}</script><script>window.__QUERY_INITIAL_STATE__={"queries":[{"state":{"data":{"bopData":{"detailedDescription":"Good milk","additionalInfo":[{"title":"Ingredientes","content":"Leche"}]}}}}]};</script></html>`)
	p, ok := productFromHTML(body, "https://example.test")
	if !ok {
		t.Fatal("product not parsed")
	}
	if p.SKU != "54178" || p.Name != "Milk" || p.Price.Cents != 576 || p.Ingredients != "Leche" {
		t.Fatalf("unexpected product: %+v", p)
	}
}
