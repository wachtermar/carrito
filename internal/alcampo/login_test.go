package alcampo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/wachtermar/carrito/internal/config"
)

func TestValidateCredentialPostURL(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		wantErr bool
	}{
		{name: "HTTPS identity provider", rawURL: "https://identity.example.test/login"},
		{name: "localhost test server", rawURL: "http://localhost:8080/login"},
		{name: "IPv4 loopback test server", rawURL: "http://127.0.0.1:8080/login"},
		{name: "IPv6 loopback test server", rawURL: "http://[::1]:8080/login"},
		{name: "remote plain HTTP", rawURL: "http://identity.example.test/login", wantErr: true},
		{name: "lookalike localhost", rawURL: "http://localhost.example.test/login", wantErr: true},
		{name: "relative action", rawURL: "/login", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCredentialPostURL(tt.rawURL)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateCredentialPostURL(%q) error = %v, wantErr %t", tt.rawURL, err, tt.wantErr)
			}
		})
	}
}

func TestCredentialPostRejectsInsecureRedirect(t *testing.T) {
	credentialServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://identity.example.test/capture", http.StatusTemporaryRedirect)
	}))
	defer credentialServer.Close()

	client, err := New(config.Default())
	if err != nil {
		t.Fatal(err)
	}
	client.httpClient.Transport = credentialServer.Client().Transport
	values := url.Values{"username": {"person@example.test"}, "password": {"secret"}}
	_, _, err = client.postCredentialForm(context.Background(), credentialServer.URL+"/login", values, credentialServer.URL+"/")
	if err == nil {
		t.Fatal("credential POST followed an insecure redirect")
	}
	if !strings.Contains(err.Error(), "refusing to submit credentials over an insecure connection") {
		t.Fatalf("unexpected error: %v", err)
	}
}
