package alcampo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wachtermar/carrito/internal/config"
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
	body := []byte(`<html><script data-test="product-details-structured-data">{"sku":"54178","name":"Milk","brand":{"name":"AUCHAN"},"offers":{"price":"5.76","priceCurrency":"EUR"},"image":["/img.jpg"]}</script><script>window.__QUERY_INITIAL_STATE__={"queries":[{"state":{"data":{"product":{"productId":"milk-id","retailerProductId":"54178","name":"Milk"},"bopData":{"detailedDescription":"Good milk","additionalInfo":[{"title":"Ingredientes","content":"Leche"}]}}}}]};</script></html>`)
	p, ok := productFromHTML(body, "https://example.test")
	if !ok {
		t.Fatal("product not parsed")
	}
	if p.SKU != "54178" || p.Name != "Milk" || p.Price.Cents != 576 || p.Ingredients != "Leche" {
		t.Fatalf("unexpected product: %+v", p)
	}
}

func TestProductFromHTMLSkipsConflictingEmbeddedProducts(t *testing.T) {
	body := []byte(`<html>
<script data-test="product-details-structured-data">{"sku":"REQUESTED","name":"Requested product"}</script>
<script>window.__QUERY_INITIAL_STATE__={"data":{"retailerProductId":"OTHER","productId":"wrong-query-id","name":"Wrong query product","detailedDescription":"Wrong description","ingredients":"wrong ingredient"}};</script>
<script>window.__INITIAL_STATE__={"products":[
  {"productId":"wrong-id","retailerProductId":"OTHER","name":"Wrong product","price":{"amount":"99.00","currency":"EUR"},"available":true},
  {"productId":"correct-id","retailerProductId":"REQUESTED","name":"Requested product","price":{"amount":"2.25","currency":"EUR"},"available":true}
]};</script></html>`)
	p, ok := productFromHTML(body, "https://example.test")
	if !ok {
		t.Fatal("product not parsed")
	}
	if p.SKU != "REQUESTED" || p.ID != "correct-id" || p.Price.Cents != 225 {
		t.Fatalf("conflicting embedded product contaminated result: %+v", p)
	}
	if p.Description == "Wrong description" || p.Ingredients == "wrong ingredient" {
		t.Fatalf("conflicting detail state contaminated result: %+v", p)
	}
}

func TestProductFromHTMLDoesNotTrustAnonymousNestedLabels(t *testing.T) {
	body := []byte(`<html>
<script data-test="product-details-structured-data">{"sku":"REQUESTED","name":"Requested product"}</script>
<script>window.__QUERY_INITIAL_STATE__={"queries":[{"state":{"data":{"bopData":{"detailedDescription":"Unattributed description","additionalInfo":[{"title":"Ingredientes","content":"unattributed ingredient"},{"title":"Alérgenos","content":"unattributed allergen"}]}}}}]};</script>
</html>`)
	p, ok := productFromHTML(body, "https://example.test")
	if !ok {
		t.Fatal("product not parsed")
	}
	if p.Description != "" || p.Ingredients != "" || p.Allergens != "" {
		t.Fatalf("anonymous nested labels were attributed to requested product: %+v", p)
	}
}

func TestProductFromHTMLSkipsLabelsInsideNestedDifferentProduct(t *testing.T) {
	body := []byte(`<html>
<script data-test="product-details-structured-data">{"sku":"REQUESTED","name":"Requested product"}</script>
<script>window.__QUERY_INITIAL_STATE__={"queries":[{"state":{"data":{
  "product":{"productId":"requested-id","retailerProductId":"REQUESTED","name":"Requested product"},
  "bopData":{"fields":[{"title":"brand","content":"Requested brand"}],"recommendation":{"product":{"productId":"other-id","retailerProductId":"OTHER","name":"Other product"},"fields":[{"title":"Ingredientes","content":"wrong nested ingredient"},{"title":"Alérgenos","content":"wrong nested allergen"}]}}
}}}]};</script></html>`)
	p, ok := productFromHTML(body, "https://example.test")
	if !ok {
		t.Fatal("product not parsed")
	}
	if p.Ingredients != "" || p.Allergens != "" {
		t.Fatalf("nested different product contaminated requested labels: %+v", p)
	}
}

func TestProductUsesCurrentMarketListingAsAuthority(t *testing.T) {
	available := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			_, _ = w.Write([]byte(`<html></html>`))
		case r.URL.Path == "/products/REQUESTED":
			_, _ = w.Write([]byte(`<script data-test="product-details-structured-data">{"sku":"REQUESTED","name":"Stale page name","description":"Current label details","offers":{"price":"99.00","priceCurrency":"EUR"}}</script><script>window.__INITIAL_STATE__={"products":[{"productId":"stale-id","retailerProductId":"REQUESTED","name":"Stale page name","available":true}]};</script>`))
		case r.URL.Path == "/api/webproductpagews/v6/product-pages/search":
			if got := r.URL.Query().Get("regionId"); got != "current-market" {
				t.Fatalf("regionId = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"products": []any{map[string]any{
				"productId": "current-id", "retailerProductId": "REQUESTED", "name": "Current listing name",
				"price": map[string]any{"amount": "2.25", "currency": "EUR"}, "available": available,
			}}})
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
	client.RegionID = "current-market"
	product, err := client.Product(context.Background(), "REQUESTED")
	if err != nil {
		t.Fatal(err)
	}
	if product.ID != "current-id" || product.SKU != "REQUESTED" || product.Name != "Current listing name" || product.Price.Cents != 225 || product.Available == nil || *product.Available != available {
		t.Fatalf("current listing was not authoritative: %+v", product)
	}
	if product.Description != "Current label details" {
		t.Fatalf("detail fields were not preserved: %+v", product)
	}
}

func TestLookupRejectsFuzzyFirstResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = w.Write([]byte(`<html></html>`))
			return
		}
		if r.URL.Path != "/api/webproductpagews/v6/product-pages/search" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"products": []any{map[string]any{
			"productId": "actual-id", "retailerProductId": "PASTA500", "name": "Pasta", "available": true,
		}}})
	}))
	defer server.Close()
	client, err := New(config.Default())
	if err != nil {
		t.Fatal(err)
	}
	client.BaseURL = server.URL
	if _, err := client.Lookup(context.Background(), "PASTA-DOES-NOT-EXIST"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected exact lookup failure, got %v", err)
	}
	product, err := client.Lookup(context.Background(), "PASTA500")
	if err != nil || product.ID != "actual-id" {
		t.Fatalf("exact lookup = %+v, %v", product, err)
	}
}

func TestLookupFallsBackToExactDecorationForInternalID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			_, _ = w.Write([]byte(`<html></html>`))
		case r.URL.Path == "/api/webproductpagews/v6/product-pages/search":
			_ = json.NewEncoder(w).Encode(map[string]any{"products": []any{}})
		case r.URL.Path == "/api/webproductpagews/v6/products" && r.Method == http.MethodPut:
			_ = json.NewEncoder(w).Encode(map[string]any{"products": []any{map[string]any{
				"productId": "internal-uuid", "retailerProductId": "99193", "name": "Exact product", "available": true,
			}}})
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
	product, err := client.Lookup(context.Background(), "internal-uuid")
	if err != nil || product.SKU != "99193" {
		t.Fatalf("decorated lookup = %+v, %v", product, err)
	}
}

func TestProductRejectsMismatchedSuccessfulDetailPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			_, _ = w.Write([]byte(`<html></html>`))
		case r.URL.Path == "/products/REQUESTED":
			_, _ = w.Write([]byte(`<script data-test="product-details-structured-data">{"sku":"OTHER","name":"Wrong product"}</script>`))
		case r.URL.Path == "/api/webproductpagews/v6/product-pages/search":
			_ = json.NewEncoder(w).Encode(map[string]any{"products": []any{map[string]any{
				"productId": "other-id", "retailerProductId": "OTHER", "name": "Wrong product",
			}}})
		case r.URL.Path == "/api/webproductpagews/v6/products" && r.Method == http.MethodPut:
			_ = json.NewEncoder(w).Encode(map[string]any{"products": []any{}})
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
	if _, err := client.Product(context.Background(), "REQUESTED"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("mismatched detail page should fail exact identity: %v", err)
	}
}
