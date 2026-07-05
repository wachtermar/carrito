package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"alcampo-cli/internal/config"
)

func TestCartAddUsesImportedSessionAndMarket(t *testing.T) {
	var sawSearchRegion string
	var sawCartCookie string
	var sawCartCSRF string
	var addBody []map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html></html>`))
		case "/api/webproductpagews/v6/product-pages/search":
			sawSearchRegion = r.URL.Query().Get("regionId")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"productGroups": []any{
					map[string]any{"decoratedProducts": []any{
						map[string]any{
							"productId":         "product-eggs",
							"retailerProductId": "947535",
							"name":              "Free range eggs",
							"brand":             "TEST",
							"price":             map[string]any{"amount": "3.25", "currency": "EUR"},
							"unitPrice":         map[string]any{"amount": "3.25", "currency": "EUR"},
							"available":         true,
						},
					}},
				},
			})
		case "/api/cart/v2/carts/active/cart-view":
			if got := r.URL.Query().Get("productGroupingType"); got != "CATEGORIES" {
				t.Fatalf("productGroupingType = %q", got)
			}
			_ = json.NewEncoder(w).Encode(cartWithTotal("0.00"))
		case "/api/cart/v1/carts/active/apply-quantity":
			sawCartCookie = r.Header.Get("Cookie")
			sawCartCSRF = r.Header.Get("x-csrf-token")
			dec := json.NewDecoder(r.Body)
			dec.UseNumber()
			if err := dec.Decode(&addBody); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(cartWithTotal("3.25"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	writeTestConfig(t, "region-home", "dest-home")
	t.Setenv("ALCAMPO_BASE_URL", server.URL)

	var stdout, stderr bytes.Buffer
	err := Run([]string{"cart", "add", "947535", "1", "--max", "20", "--json"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	if sawSearchRegion != "region-home" {
		t.Fatalf("search region = %q", sawSearchRegion)
	}
	if sawCartCookie != "sid=test" || sawCartCSRF != "csrf-test" {
		t.Fatalf("write auth headers cookie=%q csrf=%q", sawCartCookie, sawCartCSRF)
	}
	if len(addBody) != 1 || addBody[0]["productId"] != "product-eggs" || addBody[0]["quantity"].(json.Number).String() != "1" {
		t.Fatalf("unexpected add body: %#v", addBody)
	}
	var result struct {
		Action         string `json:"action"`
		AppliedDelta   string `json:"applied_delta"`
		CartTotalAfter struct {
			Cents int64 `json:"cents"`
		} `json:"cart_total_after"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("output was not JSON: %v\n%s", err, stdout.String())
	}
	if result.Action != "add" || result.AppliedDelta != "1" || result.CartTotalAfter.Cents != 325 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestCheckoutSelectSlotVerifiesSlotAndReserves(t *testing.T) {
	var slotsBody map[string]any
	var reserveBody map[string]any
	var reserveCSRF string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html></html>`))
		case "/api/cart/v2/carts/active/cart-view":
			_ = json.NewEncoder(w).Encode(cartWithTotal("3.25"))
		case "/api/ecomslots/v2/slots":
			if err := json.NewDecoder(r.Body).Decode(&slotsBody); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"days": []any{
					map[string]any{
						"day": "2026-07-05",
						"slots": []any{
							map[string]any{
								"slotId":        "slot-1",
								"slotWindow":    map[string]any{"startTime": "10:00", "endTime": "12:00"},
								"deliveryPrice": map[string]any{"amount": "4.99", "currency": "EUR"},
							},
						},
					},
				},
			})
		case "/api/ecomslots/v1/slots/reservation":
			reserveCSRF = r.Header.Get("x-csrf-token")
			if err := json.NewDecoder(r.Body).Decode(&reserveBody); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"slot": map[string]any{"slotId": "slot-1"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	writeTestConfig(t, "region-home", "dest-home")
	t.Setenv("ALCAMPO_BASE_URL", server.URL)

	var stdout, stderr bytes.Buffer
	err := Run([]string{"checkout", "select-slot", "--slot", "slot-1", "--max", "20", "--json"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	if slotsBody["deliveryDestinationId"] != "dest-home" || slotsBody["regionId"] != "region-home" {
		t.Fatalf("unexpected slots body: %#v", slotsBody)
	}
	if reserveCSRF != "csrf-test" {
		t.Fatalf("reserve csrf = %q", reserveCSRF)
	}
	if reserveBody["deliveryDestinationId"] != "dest-home" || reserveBody["regionId"] != "region-home" || reserveBody["slotId"] != "slot-1" {
		t.Fatalf("unexpected reserve body: %#v", reserveBody)
	}
	var result struct {
		Action string `json:"action"`
		Slot   struct {
			SlotID string `json:"slot_id"`
		} `json:"slot"`
		EstimatedTotalWithSlot struct {
			Cents int64 `json:"cents"`
		} `json:"estimated_total_with_slot"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("output was not JSON: %v\n%s", err, stdout.String())
	}
	if result.Action != "select-slot" || result.Slot.SlotID != "slot-1" || result.EstimatedTotalWithSlot.Cents != 824 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestLoginStoresCookieAndCSRF(t *testing.T) {
	var sawUsername string
	var sawPassword string
	var sawStartURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			http.SetCookie(w, &http.Cookie{Name: "global_sid", Value: "visitor", Path: "/"})
			w.Header().Set("ecom-request-source-version", "test-source-version")
			http.Redirect(w, r, "/relay", http.StatusFound)
		case "/relay":
			_, _ = w.Write([]byte(`<form id="regPage:j_id5" name="regPage:j_id5" method="post" action="/ARCD_CIAM_Redirect">
<input type="hidden" name="regPage:j_id5" value="regPage:j_id5" />
<script>function loginRedirect(){jsfcljs(document.forms['regPage:j_id5'],'regPage:j_id5:loginRedirect,regPage:j_id5:loginRedirect','');}</script>
<span id="ajax-view-state"><input type="hidden" name="com.salesforce.visualforce.ViewState" value="relay-vs" />
<input type="hidden" name="com.salesforce.visualforce.ViewStateVersion" value="relay-vsv" />
<input type="hidden" name="com.salesforce.visualforce.ViewStateMAC" value="relay-mac" /></span></form>`))
		case "/ARCD_CIAM_Redirect":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("regPage:j_id5") != "regPage:j_id5" || r.Form.Get("regPage:j_id5:loginRedirect") != "regPage:j_id5:loginRedirect" {
				t.Fatalf("unexpected relay form: %#v", r.Form)
			}
			_, _ = w.Write([]byte(`<script>window.location.replace('/authorization?language=es&startURL=%2Fsetup%2Fsecur%2FRemoteAccessAuthorizationPage.apexp%3Fsource%3Dabc');</script>`))
		case "/authorization":
			_, _ = w.Write([]byte(`<script>var startURL = '%2Fsetup%2Fsecur%2FRemoteAccessAuthorizationPage.apexp%3Fsource%3Dabc';</script>
<form id="j_id0:j_id17" name="j_id0:j_id17" method="post" action="/ARCD_CIAM_Login">
<input type="hidden" name="j_id0:j_id17" value="j_id0:j_id17" />
<script>login=function(username,password,startUrl,hpot){A4J.AJAX.Submit('j_id0:j_id17',null,{'similarityGroupingId':'j_id0:j_id17:login','parameters':{'startUrl':startUrl,'password':password,'j_id0:j_id17:login':'j_id0:j_id17:login','hpot':hpot,'username':username}})}</script>
<input name="uname1" type="email" />
<input name="passwordLogin" type="password" />
<span id="ajax-view-state"><input type="hidden" name="com.salesforce.visualforce.ViewState" value="login-vs" />
<input type="hidden" name="com.salesforce.visualforce.ViewStateVersion" value="login-vsv" />
<input type="hidden" name="com.salesforce.visualforce.ViewStateMAC" value="login-mac" /></span></form>`))
		case "/ARCD_CIAM_Login":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			sawUsername = r.Form.Get("username")
			sawPassword = r.Form.Get("password")
			sawStartURL = r.Form.Get("startUrl")
			if r.Form.Get("AJAXREQUEST") != "_viewRoot" || r.Form.Get("j_id0:j_id17:login") != "j_id0:j_id17:login" {
				t.Fatalf("unexpected credential form: %#v", r.Form)
			}
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><html><head><meta name="Ajax-Response" content="redirect" /><meta name="Location" content="/setup/secur/RemoteAccessAuthorizationPage.apexp?source=abc" /></head></html>`))
		case "/setup/secur/RemoteAccessAuthorizationPage.apexp":
			http.Redirect(w, r, "/sso-login?code=oauth-code&state=state", http.StatusFound)
		case "/sso-login":
			http.SetCookie(w, &http.Cookie{Name: "logged_in", Value: "yes", Path: "/"})
			http.Redirect(w, r, "/", http.StatusFound)
		case "/":
			_, _ = w.Write([]byte(`<!doctype html><script>window.__INITIAL_STATE__={"session":{"csrf":{"token":"csrf-from-login"},"metadata":{"customerId":"customer-1","visitorId":"visitor-1"},"isLoggedIn":true},"defaults":{"regionId":"region-login","deliveryDestinationId":"dest-login"}}; window.config={"ecom-request-source-version":"test-source-version"};</script>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	t.Setenv("ALCAMPO_BASE_URL", server.URL)
	t.Setenv("ALCAMPO_PASSWORD", "secret-password")

	var stdout, stderr bytes.Buffer
	err := Run([]string{"login", "--username", "user@example.com", "--json"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	if sawUsername != "user@example.com" || sawPassword != "secret-password" {
		t.Fatalf("credentials were not submitted correctly username=%q password=%q", sawUsername, sawPassword)
	}
	if sawStartURL == "" {
		t.Fatal("startUrl was not submitted")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.Cookie == "" || cfg.Auth.CSRFToken != "csrf-from-login" || cfg.Auth.CustomerID != "customer-1" || cfg.Auth.VisitorID != "visitor-1" || cfg.Auth.ImportedAt == "" {
		t.Fatalf("session not saved: %+v", cfg.Auth)
	}
	if cfg.Session.SourceVersion != "test-source-version" {
		t.Fatalf("source version = %q", cfg.Session.SourceVersion)
	}
	var result struct {
		Authenticated bool `json:"authenticated"`
		HasCookie     bool `json:"has_cookie"`
		HasCSRF       bool `json:"has_csrf_token"`
		HasCustomerID bool `json:"has_customer_id"`
		HasVisitorID  bool `json:"has_visitor_id"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("output was not JSON: %v\n%s", err, stdout.String())
	}
	if !result.Authenticated || !result.HasCookie || !result.HasCSRF || !result.HasCustomerID || !result.HasVisitorID {
		t.Fatalf("unexpected output: %+v", result)
	}
}

func writeTestConfig(t *testing.T, regionID, destinationID string) {
	t.Helper()
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	cfg := config.Default()
	cfg.Defaults.RegionID = regionID
	cfg.Defaults.DeliveryDestinationID = destinationID
	cfg.Defaults.MarketSet = true
	cfg.Auth.Cookie = "sid=test"
	cfg.Auth.CSRFToken = "csrf-test"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
}

func cartWithTotal(amount string) map[string]any {
	return map[string]any{
		"totals": map[string]any{
			"display": map[string]any{
				"itemPriceAfterPromos": map[string]any{"amount": amount, "currency": "EUR"},
			},
		},
	}
}
