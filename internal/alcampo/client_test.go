package alcampo

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/wachtermar/carrito/internal/config"
)

func TestCrossOriginLoginRedirectStripsAlcampoCredentials(t *testing.T) {
	idpRequests := make(chan http.Header, 1)
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idpRequests <- r.Header.Clone()
		_, _ = w.Write([]byte("identity provider"))
	}))
	defer idp.Close()
	idpServerURL, err := url.Parse(idp.URL)
	if err != nil {
		t.Fatal(err)
	}
	idpHopURL := "http://identity.example.test:" + idpServerURL.Port()

	baseRequests := make(chan http.Header, 1)
	base := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		baseRequests <- r.Header.Clone()
		http.Redirect(w, r, idpHopURL+"/authorize", http.StatusFound)
	}))
	defer base.Close()

	cfg := config.Default()
	cfg.Auth.Cookie = "alcampo_saved=secret"
	cfg.Auth.BearerToken = "bearer-secret"
	cfg.Auth.CSRFToken = "csrf-secret"
	cfg.Auth.CustomerID = "customer-secret"
	cfg.Auth.VisitorID = "visitor-secret"
	client, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	client.BaseURL = base.URL
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	dialer := &net.Dialer{}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, _, splitErr := net.SplitHostPort(address)
		if splitErr == nil && strings.EqualFold(host, "identity.example.test") {
			address = idp.Listener.Addr().String()
		}
		return dialer.DialContext(ctx, network, address)
	}
	client.httpClient.Transport = transport

	idpURL, err := url.Parse(idpHopURL)
	if err != nil {
		t.Fatal(err)
	}
	client.httpClient.Jar.SetCookies(idpURL, []*http.Cookie{{Name: "idp_session", Value: "jar-secret"}})

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, base.URL+"/login-start", nil)
	if err != nil {
		t.Fatal(err)
	}
	client.setAPIHeaders(req, base.URL+"/", "login")
	_, finalURL, err := client.doLoginRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if finalURL.String() != idpHopURL+"/authorize" {
		t.Fatalf("final URL = %q", finalURL)
	}

	baseHeaders := <-baseRequests
	if got := baseHeaders.Get("Cookie"); !strings.Contains(got, "alcampo_saved=secret") {
		t.Errorf("base Cookie = %q, want saved Alcampo cookie", got)
	}
	assertHeader(t, baseHeaders, "Authorization", "Bearer bearer-secret")
	assertHeader(t, baseHeaders, "X-CSRF-Token", "csrf-secret")
	assertHeader(t, baseHeaders, "Customer-ID", "customer-secret")
	assertHeader(t, baseHeaders, "Visitor-ID", "visitor-secret")

	idpHeaders := <-idpRequests
	for _, name := range []string{"Authorization", "X-CSRF-Token", "Customer-ID", "Visitor-ID"} {
		if got := idpHeaders.Get(name); got != "" {
			t.Errorf("identity provider received Alcampo %s header %q", name, got)
		}
	}
	if got := idpHeaders.Get("Cookie"); got != "idp_session=jar-secret" {
		t.Fatalf("identity provider Cookie = %q, want only its cookie-jar session", got)
	}
	if strings.Contains(idpHeaders.Get("Cookie"), "alcampo_saved") {
		t.Fatal("identity provider received the saved Alcampo cookie")
	}
}

func TestAPIHeadersDoNotAttachCredentialsOutsideBaseOrigin(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.Cookie = "alcampo_saved=secret"
	cfg.Auth.BearerToken = "bearer-secret"
	cfg.Auth.CSRFToken = "csrf-secret"
	cfg.Auth.CustomerID = "customer-secret"
	cfg.Auth.VisitorID = "visitor-secret"
	client, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	client.BaseURL = "https://shop.example.test"

	req, err := http.NewRequest(http.MethodGet, "https://identity.example.test/authorize", nil)
	if err != nil {
		t.Fatal(err)
	}
	client.setAPIHeaders(req, client.BaseURL+"/", "login")
	for _, name := range []string{"Cookie", "Authorization", "X-CSRF-Token", "Customer-ID", "Visitor-ID"} {
		if got := req.Header.Get(name); got != "" {
			t.Errorf("external request received Alcampo %s header %q", name, got)
		}
	}
}

func TestNewRestrictsBaseURLOverridesToOfficialOrLoopbackOrigins(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		wantErr bool
	}{
		{name: "official", baseURL: config.DefaultBaseURL},
		{name: "loopback HTTP", baseURL: "http://127.0.0.1:18765"},
		{name: "localhost HTTP", baseURL: "http://localhost:18765"},
		{name: "remote HTTP", baseURL: "http://shop.example.test", wantErr: true},
		{name: "remote HTTPS", baseURL: "https://shop.example.test", wantErr: true},
		{name: "localhost lookalike", baseURL: "https://localhost.example.test", wantErr: true},
		{name: "embedded credentials", baseURL: "https://user:secret@www.compraonline.alcampo.es", wantErr: true},
		{name: "path override", baseURL: config.DefaultBaseURL + "/capture", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("CARRITO_BASE_URL", test.baseURL)
			_, err := New(config.Default())
			if (err != nil) != test.wantErr {
				t.Fatalf("New() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func assertHeader(t *testing.T, header http.Header, name, want string) {
	t.Helper()
	if got := header.Get(name); got != want {
		t.Errorf("%s = %q, want %q", name, got, want)
	}
}
