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

func TestLiveSnapshotSignatureStableAndRedactsVolatileQuery(t *testing.T) {
	reqA, err := http.NewRequest(http.MethodGet, "https://example.test/api/webproductpagews/v6/product-pages/search?token=secret&q=arroz&regionId=store-1&maxPageSize=3", nil)
	if err != nil {
		t.Fatal(err)
	}
	reqB, err := http.NewRequest(http.MethodGet, "https://example.test/api/webproductpagews/v6/product-pages/search?maxPageSize=3&regionId=store-1&q=arroz&session=abc", nil)
	if err != nil {
		t.Fatal(err)
	}
	sigA := buildSnapshotSignature(reqA, nil, "store-1")
	sigB := buildSnapshotSignature(reqB, nil, "store-1")
	if sigA.Signature != sigB.Signature {
		t.Fatalf("signature changed with reordered/sensitive query params: %s != %s", sigA.Signature, sigB.Signature)
	}
	if len(sigA.DroppedQueryParams) == 0 || len(sigB.DroppedQueryParams) == 0 {
		t.Fatalf("sensitive query params were not dropped: %+v %+v", sigA.DroppedQueryParams, sigB.DroppedQueryParams)
	}
	reqC, err := http.NewRequest(http.MethodGet, "https://example.test/api/webproductpagews/v6/product-pages/search?q=leche&regionId=store-1&maxPageSize=3", nil)
	if err != nil {
		t.Fatal(err)
	}
	sigC := buildSnapshotSignature(reqC, nil, "store-1")
	if sigA.Signature == sigC.Signature {
		t.Fatal("different search query should produce a different signature")
	}
}

func TestLiveSnapshotClassifierBlocksMutationRequests(t *testing.T) {
	for _, raw := range []string{
		"https://example.test/api/cart/v1/carts/active/apply-quantity",
		"https://example.test/api/checkout/v1/slots",
		"https://example.test/login",
		"https://example.test/api/account/address",
	} {
		req, err := http.NewRequest(http.MethodGet, raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := classifySnapshotRequest(req); got != SnapshotKindMutation {
			t.Fatalf("%s classified as %s, want mutation", raw, got)
		}
	}
}

func TestLiveSnapshotRecordReplaySearchWithoutNetwork(t *testing.T) {
	dir := t.TempDir()
	liveCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		liveCalls++
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html></html>`))
		case "/api/webproductpagews/v6/product-pages/search":
			_ = json.NewEncoder(w).Encode(snapshotSearchFixture())
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	recordClient, err := New(config.Default())
	if err != nil {
		t.Fatal(err)
	}
	recordClient.BaseURL = server.URL
	recordClient.RegionID = "store-1"
	if err := recordClient.ConfigureLiveSnapshot(LiveSnapshotOptions{Mode: LiveSnapshotModeRecord, Dir: dir, SnapshotID: "search-fixture"}); err != nil {
		t.Fatal(err)
	}
	recorded, err := recordClient.Search(context.Background(), "arroz", SearchOptions{Limit: 1, RegionID: "store-1"})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := recordClient.FinalizeLiveSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.EntryCount != 2 {
		t.Fatalf("entry count = %d, want 2", manifest.EntryCount)
	}
	if !manifest.Entries[1].ExtractedShape.HasPrice || !manifest.Entries[1].ExtractedShape.HasProductID || !manifest.Entries[1].ExtractedShape.HasOfferText {
		t.Fatalf("shape did not capture product evidence: %+v", manifest.Entries[1].ExtractedShape)
	}
	if liveCalls != 2 {
		t.Fatalf("record live calls = %d, want root+search", liveCalls)
	}

	replayClient, err := New(config.Default())
	if err != nil {
		t.Fatal(err)
	}
	replayClient.BaseURL = "http://127.0.0.1:1"
	replayClient.RegionID = "store-1"
	if err := replayClient.ConfigureLiveSnapshot(LiveSnapshotOptions{Mode: LiveSnapshotModeReplay, Dir: dir, Strict: true}); err != nil {
		t.Fatal(err)
	}
	replayed, err := replayClient.Search(context.Background(), "arroz", SearchOptions{Limit: 1, RegionID: "store-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(recorded) != 1 || len(replayed) != 1 || recorded[0].SKU != replayed[0].SKU || replayed[0].Price.Cents != 120 {
		t.Fatalf("replayed products changed: recorded=%+v replayed=%+v", recorded, replayed)
	}
	if _, err := replayClient.FinalizeLiveSnapshot(); err != nil {
		t.Fatal(err)
	}
	summary := replayClient.LiveSnapshotSummary()
	if summary.ReplayHits != 2 || summary.ReplayMisses != 0 {
		t.Fatalf("unexpected replay stats: %+v", summary)
	}
}

func TestLiveSnapshotStrictReplayMissFails(t *testing.T) {
	dir := recordSearchSnapshot(t, "arroz")
	client, err := New(config.Default())
	if err != nil {
		t.Fatal(err)
	}
	client.BaseURL = "http://127.0.0.1:1"
	client.RegionID = "store-1"
	if err := client.ConfigureLiveSnapshot(LiveSnapshotOptions{Mode: LiveSnapshotModeReplay, Dir: dir, Strict: true}); err != nil {
		t.Fatal(err)
	}
	_, err = client.Search(context.Background(), "leche", SearchOptions{Limit: 1, RegionID: "store-1"})
	var snapErr SnapshotError
	if !errors.As(err, &snapErr) || snapErr.Code != "snapshot_replay_miss" {
		t.Fatalf("err = %v, want snapshot_replay_miss", err)
	}
}

func TestLiveSnapshotRecordDuplicateDifferentBodyFails(t *testing.T) {
	dir := t.TempDir()
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html></html>`))
		case "/api/webproductpagews/v6/product-pages/search":
			count++
			fixture := snapshotSearchFixture()
			if count > 1 {
				fixture["productGroups"].([]any)[0].(map[string]any)["decoratedProducts"].([]any)[0].(map[string]any)["price"] = map[string]any{"amount": "2.00", "currency": "EUR"}
			}
			_ = json.NewEncoder(w).Encode(fixture)
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
	client.RegionID = "store-1"
	if err := client.ConfigureLiveSnapshot(LiveSnapshotOptions{Mode: LiveSnapshotModeRecord, Dir: dir}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Search(context.Background(), "arroz", SearchOptions{Limit: 1, RegionID: "store-1"}); err != nil {
		t.Fatal(err)
	}
	_, err = client.Search(context.Background(), "arroz", SearchOptions{Limit: 1, RegionID: "store-1"})
	var snapErr SnapshotError
	if !errors.As(err, &snapErr) || snapErr.Code != "snapshot_duplicate_signature" {
		t.Fatalf("err = %v, want snapshot_duplicate_signature", err)
	}
}

func recordSearchSnapshot(t *testing.T, query string) string {
	t.Helper()
	dir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html></html>`))
		case "/api/webproductpagews/v6/product-pages/search":
			_ = json.NewEncoder(w).Encode(snapshotSearchFixture())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client, err := New(config.Default())
	if err != nil {
		t.Fatal(err)
	}
	client.BaseURL = server.URL
	client.RegionID = "store-1"
	if err := client.ConfigureLiveSnapshot(LiveSnapshotOptions{Mode: LiveSnapshotModeRecord, Dir: dir}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Search(context.Background(), query, SearchOptions{Limit: 1, RegionID: "store-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.FinalizeLiveSnapshot(); err != nil {
		t.Fatal(err)
	}
	return dir
}

func snapshotSearchFixture() map[string]any {
	return map[string]any{
		"productGroups": []any{
			map[string]any{"decoratedProducts": []any{
				map[string]any{
					"productId":         "rice-product",
					"retailerProductId": "rice-sku",
					"name":              "Arroz redondo 1 kg",
					"brand":             "TEST",
					"size":              "1 kg",
					"price":             map[string]any{"amount": "1.20", "currency": "EUR"},
					"unitPrice":         map[string]any{"amount": "1.20", "currency": "EUR"},
					"available":         true,
					"images":            []any{map[string]any{"url": "https://example.test/rice.jpg"}},
					"offers":            []any{map[string]any{"description": "3x2"}},
					"nutrition":         map[string]any{"kcal": "350"},
				},
			}},
		},
	}
}
