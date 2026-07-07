package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/wachtermar/carrito/internal/config"
)

func TestLoginWebIfNeededSkipsBrowserWhenAuthenticated(t *testing.T) {
	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	cfg := config.Default()
	cfg.Auth.Cookie = "sid=already-authenticated"
	cfg.Auth.CSRFToken = "csrf-existing"
	cfg.Auth.CustomerID = "customer-existing"
	cfg.Auth.VisitorID = "visitor-existing"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	oldOpen := loginWebOpenBrowser
	loginWebOpenBrowser = func(rawURL string) error {
		t.Fatalf("browser should not open when already authenticated; url=%s", rawURL)
		return nil
	}
	defer func() { loginWebOpenBrowser = oldOpen }()

	var stdout, stderr bytes.Buffer
	err := Run([]string{"login-web", "--if-needed", "--json"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	var result struct {
		Authenticated bool   `json:"authenticated"`
		Message       string `json:"message"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout was not JSON: %v\n%s", err, stdout.String())
	}
	if !result.Authenticated || !strings.Contains(result.Message, "already logged in") {
		t.Fatalf("unexpected output: %+v", result)
	}
}

func TestLoginWebCollectsCredentialsInBrowserAndStoresSession(t *testing.T) {
	var sawUsername string
	var sawPassword string

	alcampoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			http.Redirect(w, r, "/relay", http.StatusFound)
		case "/relay":
			_, _ = w.Write([]byte(`<form id="regPage:j_id5" name="regPage:j_id5" method="post" action="/ARCD_CIAM_Redirect">
<input type="hidden" name="regPage:j_id5" value="regPage:j_id5" />
<script>function loginRedirect(){jsfcljs(document.forms['regPage:j_id5'],'regPage:j_id5:loginRedirect,regPage:j_id5:loginRedirect','');}</script>
<span id="ajax-view-state"><input type="hidden" name="com.salesforce.visualforce.ViewState" value="relay-vs" />
<input type="hidden" name="com.salesforce.visualforce.ViewStateVersion" value="relay-vsv" />
<input type="hidden" name="com.salesforce.visualforce.ViewStateMAC" value="relay-mac" /></span></form>`))
		case "/ARCD_CIAM_Redirect":
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
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><html><head><meta name="Ajax-Response" content="redirect" /><meta name="Location" content="/setup/secur/RemoteAccessAuthorizationPage.apexp?source=abc" /></head></html>`))
		case "/setup/secur/RemoteAccessAuthorizationPage.apexp":
			http.Redirect(w, r, "/sso-login?code=oauth-code&state=state", http.StatusFound)
		case "/sso-login":
			http.SetCookie(w, &http.Cookie{Name: "logged_in", Value: "yes", Path: "/"})
			http.Redirect(w, r, "/", http.StatusFound)
		case "/":
			_, _ = w.Write([]byte(`<!doctype html><script>window.__INITIAL_STATE__={"session":{"csrf":{"token":"csrf-web"},"metadata":{"customerId":"customer-web","visitorId":"visitor-web"},"isLoggedIn":true},"defaults":{"regionId":"region-web","deliveryDestinationId":"dest-web"}};</script>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer alcampoServer.Close()

	t.Setenv("ALCAMPO_CONFIG_DIR", t.TempDir())
	t.Setenv("ALCAMPO_BASE_URL", alcampoServer.URL)

	opened := make(chan string, 1)
	oldOpen := loginWebOpenBrowser
	loginWebOpenBrowser = func(rawURL string) error {
		opened <- rawURL
		return nil
	}
	defer func() { loginWebOpenBrowser = oldOpen }()

	var stdout, stderr bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- Run([]string{"login-web", "--json", "--timeout", "5s"}, &stdout, &stderr)
	}()

	var rawURL string
	select {
	case rawURL = <-opened:
	case <-time.After(2 * time.Second):
		t.Fatalf("browser URL was not opened; stderr=%s", stderr.String())
	}

	resp, err := http.PostForm(rawURL+"login", url.Values{
		"username": {"desktop@example.com"},
		"password": {"browser-secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("local login form returned status %s", resp.Status)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run error: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("login-web did not finish; stderr=%s", stderr.String())
	}

	if sawUsername != "desktop@example.com" || sawPassword != "browser-secret" {
		t.Fatalf("credentials not submitted correctly username=%q password=%q", sawUsername, sawPassword)
	}
	if strings.Contains(stdout.String(), "browser-secret") || strings.Contains(stderr.String(), "browser-secret") {
		t.Fatalf("password leaked to command output")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.Cookie == "" || cfg.Auth.CSRFToken != "csrf-web" || cfg.Auth.CustomerID != "customer-web" || cfg.Auth.VisitorID != "visitor-web" {
		t.Fatalf("session not saved: %+v", cfg.Auth)
	}
	var result struct {
		Authenticated bool   `json:"authenticated"`
		Message       string `json:"message"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout was not JSON: %v\n%s", err, stdout.String())
	}
	if !result.Authenticated || !strings.Contains(result.Message, "browser") {
		t.Fatalf("unexpected output: %+v", result)
	}
}
