package alcampo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/wachtermar/carrito/internal/config"
	"github.com/wachtermar/carrito/internal/httpx"
)

const defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36 carrito/0.1"

type Client struct {
	BaseURL       string
	RegionID      string
	RegionName    string
	SourceVersion string
	UserAgent     string

	authCookie string
	bearer     string
	csrf       string
	customerID string
	visitorID  string

	httpClient *http.Client

	mu          sync.Mutex
	initialized bool
	lastRequest time.Time
}

func New(cfg *config.Config) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	baseURL := strings.TrimRight(firstEnv("CARRITO_BASE_URL", "ALCAMPO_BASE_URL"), "/")
	if baseURL == "" {
		baseURL = config.DefaultBaseURL
	}
	if err := validateBaseURL(baseURL); err != nil {
		return nil, err
	}
	c := &Client{
		BaseURL:       baseURL,
		RegionID:      cfg.Defaults.RegionID,
		RegionName:    cfg.Defaults.RegionName,
		SourceVersion: cfg.Session.SourceVersion,
		UserAgent:     defaultUserAgent,
		authCookie:    cfg.Auth.Cookie,
		bearer:        cfg.Auth.BearerToken,
		csrf:          cfg.Auth.CSRFToken,
		customerID:    cfg.Auth.CustomerID,
		visitorID:     cfg.Auth.VisitorID,
		httpClient: &http.Client{
			Timeout: 25 * time.Second,
			Jar:     jar,
		},
	}
	c.httpClient.CheckRedirect = c.checkRedirect
	if c.RegionID == "" {
		c.RegionID = config.DefaultRegionID
	}
	if c.SourceVersion == "" {
		c.SourceVersion = config.DefaultSourceVersion
	}
	return c, nil
}

func validateBaseURL(rawURL string) error {
	target, err := url.Parse(rawURL)
	if err != nil || target.Hostname() == "" {
		return fmt.Errorf("invalid Alcampo base URL %q", rawURL)
	}
	if target.User != nil || (target.Path != "" && target.Path != "/") || target.RawQuery != "" || target.Fragment != "" {
		return fmt.Errorf("invalid Alcampo base URL %q: use an origin without credentials, path, query, or fragment", rawURL)
	}
	official, _ := url.Parse(config.DefaultBaseURL)
	if sameOrigin(target, official) {
		return nil
	}
	if (strings.EqualFold(target.Scheme, "http") || strings.EqualFold(target.Scheme, "https")) && isLoopbackHost(target.Hostname()) {
		return nil
	}
	return fmt.Errorf("refusing non-official Alcampo base URL %q; test overrides must use localhost or a loopback IP", rawURL)
}

func firstEnv(names ...string) string {
	for _, name := range names {
		if value := os.Getenv(name); value != "" {
			return value
		}
	}
	return ""
}

func (c *Client) InitSession(ctx context.Context) error {
	c.mu.Lock()
	if c.initialized {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/", nil)
	if err != nil {
		return err
	}
	c.setPageHeaders(req, c.BaseURL+"/")
	body, err := c.do(req)
	if err != nil {
		return err
	}
	if version := extractSourceVersion(body); version != "" {
		c.SourceVersion = version
	}

	c.mu.Lock()
	c.initialized = true
	c.mu.Unlock()
	return nil
}

func (c *Client) getJSON(ctx context.Context, path string, q url.Values, referer, route string, out any) error {
	if err := c.InitSession(ctx); err != nil {
		return err
	}
	u, err := url.Parse(c.BaseURL + path)
	if err != nil {
		return err
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	c.setAPIHeaders(req, referer, route)
	body, err := c.do(req)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	return dec.Decode(out)
}

func (c *Client) postJSON(ctx context.Context, path string, body any, referer, route string, out any) error {
	if err := c.InitSession(ctx); err != nil {
		return err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	c.setAPIHeaders(req, referer, route)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	respBody, err := c.do(req)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(respBody))
	dec.UseNumber()
	return dec.Decode(out)
}

func (c *Client) putJSON(ctx context.Context, path string, body any, referer, route string, out any) error {
	if err := c.InitSession(ctx); err != nil {
		return err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.BaseURL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	c.setAPIHeaders(req, referer, route)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	respBody, err := c.do(req)
	if err != nil {
		return err
	}
	return decodeJSON(respBody, out)
}

func (c *Client) getPage(ctx context.Context, pathOrURL, referer string) ([]byte, error) {
	if err := c.InitSession(ctx); err != nil {
		return nil, err
	}
	u := pathOrURL
	if strings.HasPrefix(pathOrURL, "/") {
		u = c.BaseURL + pathOrURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	c.setPageHeaders(req, referer)
	return c.do(req)
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	c.throttle()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 6<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 399 {
		return nil, &httpx.StatusError{Method: req.Method, URL: req.URL.String(), StatusCode: resp.StatusCode, Status: resp.Status, Body: snippet(body)}
	}
	if readErr != nil {
		return nil, readErr
	}
	return body, nil
}

func (c *Client) throttle() {
	c.mu.Lock()
	defer c.mu.Unlock()
	min := 180 * time.Millisecond
	if !c.lastRequest.IsZero() {
		if wait := min - time.Since(c.lastRequest); wait > 0 {
			time.Sleep(wait)
		}
	}
	c.lastRequest = time.Now()
}

func (c *Client) setAPIHeaders(req *http.Request, referer, route string) {
	c.setPageHeaders(req, referer)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("ecom-request-source", "web")
	req.Header.Set("ecom-request-source-version", c.SourceVersion)
	if route != "" {
		req.Header.Set("client-route-id", route)
	}
	if !c.isBaseOrigin(req.URL) {
		return
	}
	if c.csrf != "" {
		req.Header.Set("x-csrf-token", c.csrf)
	}
	if c.customerID != "" {
		req.Header.Set("customer-id", c.customerID)
	}
	if c.visitorID != "" {
		req.Header.Set("visitor-id", c.visitorID)
	}
}

func (c *Client) setPageHeaders(req *http.Request, referer string) {
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept-Language", "es-ES,es;q=0.9,en;q=0.7")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	if !c.isBaseOrigin(req.URL) {
		return
	}
	if c.authCookie != "" {
		req.Header.Set("Cookie", c.authCookie)
	}
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
}

func (c *Client) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("stopped after 10 redirects")
	}
	if req.Method == http.MethodPost && isCredentialPost(req.Context()) {
		if err := validateCredentialPostURL(req.URL.String()); err != nil {
			return err
		}
	}
	if !c.isBaseOrigin(req.URL) {
		stripAlcampoCredentials(req.Header)
	}
	return nil
}

func (c *Client) isBaseOrigin(target *url.URL) bool {
	base, err := url.Parse(c.BaseURL)
	return err == nil && sameOrigin(base, target)
}

func sameOrigin(a, b *url.URL) bool {
	if a == nil || b == nil || a.Hostname() == "" || b.Hostname() == "" {
		return false
	}
	if !strings.EqualFold(a.Scheme, b.Scheme) || !strings.EqualFold(a.Hostname(), b.Hostname()) {
		return false
	}
	return originPort(a) == originPort(b)
}

func originPort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func stripAlcampoCredentials(header http.Header) {
	for _, name := range []string{"Cookie", "Authorization", "X-CSRF-Token", "Customer-ID", "Visitor-ID"} {
		header.Del(name)
	}
}

var sourceVersionRE = regexp.MustCompile(`ecom-request-source-version["']?\s*[:=]\s*["']([^"']+)`)

func extractSourceVersion(body []byte) string {
	m := sourceVersionRE.FindSubmatch(body)
	if len(m) == 2 {
		return string(m[1])
	}
	return ""
}

func snippet(body []byte) string {
	s := strings.TrimSpace(string(body))
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 500 {
		return s[:500] + "..."
	}
	return s
}

func decodeJSON(body []byte, out any) error {
	if out == nil || len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	return dec.Decode(out)
}

func productPageURL(baseURL, sku string) string {
	if sku == "" {
		return ""
	}
	return fmt.Sprintf("%s/products/%s", baseURL, url.PathEscape(sku))
}
