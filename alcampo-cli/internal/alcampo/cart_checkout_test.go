package alcampo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"alcampo-cli/internal/config"
)

func TestApplyCartQuantityPostsDelta(t *testing.T) {
	var sawRequest bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html></html>`))
		case "/api/cart/v1/carts/active/apply-quantity":
			sawRequest = true
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s", r.Method)
			}
			if got := r.URL.Query().Get("cartProductSorting"); got != "CATEGORIES" {
				t.Fatalf("cartProductSorting = %q", got)
			}
			if got := r.Header.Get("x-csrf-token"); got != "csrf-123" {
				t.Fatalf("csrf = %q", got)
			}
			var body []map[string]any
			dec := json.NewDecoder(r.Body)
			dec.UseNumber()
			if err := dec.Decode(&body); err != nil {
				t.Fatal(err)
			}
			if len(body) != 1 || body[0]["productId"] != "product-1" || body[0]["quantity"].(json.Number).String() != "-1.5" {
				t.Fatalf("unexpected body: %#v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"basketUpdateResult": map[string]any{
					"totals": map[string]any{
						"display": map[string]any{
							"itemPriceAfterPromos": map[string]any{"amount": "3.25", "currency": "EUR"},
						},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.Auth.CSRFToken = "csrf-123"
	client, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	client.BaseURL = server.URL
	root, err := client.ApplyCartQuantity(context.Background(), []CartQuantityChange{{ProductID: "product-1", Quantity: json.Number("-1.5")}})
	if err != nil {
		t.Fatal(err)
	}
	if !sawRequest {
		t.Fatal("cart mutation endpoint was not called")
	}
	total, ok := CartTotalCents(root)
	if !ok || total != 325 {
		t.Fatalf("total = %d, %t", total, ok)
	}
}

func TestReserveSlotPostsDiscoveredBody(t *testing.T) {
	var sawRequest bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html></html>`))
		case "/api/ecomslots/v1/slots/reservation":
			sawRequest = true
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s", r.Method)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["regionId"] != "region-1" || body["slotId"] != "slot-1" || body["deliveryDestinationId"] != "dest-1" {
				t.Fatalf("unexpected body: %#v", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"slot": map[string]any{"slotId": "slot-1"}})
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
	if _, err := client.ReserveSlot(context.Background(), SlotReservationRequest{RegionID: "region-1", SlotID: "slot-1", DeliveryDestinationID: "dest-1"}); err != nil {
		t.Fatal(err)
	}
	if !sawRequest {
		t.Fatal("slot reservation endpoint was not called")
	}
}

func TestCollectDeliverySlots(t *testing.T) {
	root := map[string]any{
		"days": []any{
			map[string]any{
				"day": "2026-07-05",
				"slots": []any{
					map[string]any{
						"slotId":     "slot-1",
						"slotWindow": map[string]any{"startTime": "10:00", "endTime": "12:00"},
						"deliveryPrice": map[string]any{
							"amount":   "4.99",
							"currency": "EUR",
						},
						"minimumOrder": map[string]any{
							"amount":   "30.00",
							"currency": "EUR",
						},
					},
				},
			},
		},
	}
	slots := CollectDeliverySlots(root)
	if len(slots) != 1 {
		t.Fatalf("slots = %d", len(slots))
	}
	slot := slots[0]
	if slot.SlotID != "slot-1" || slot.Day != "2026-07-05" || slot.StartTime != "10:00" || slot.EndTime != "12:00" || slot.Price.Cents != 499 || slot.MinimumOrder.Cents != 3000 {
		t.Fatalf("unexpected slot: %+v", slot)
	}
}
