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

	"alcampo-cli/internal/config"
	"alcampo-cli/internal/httpx"
)

const defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36 alcampo-cli/0.1"

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
	baseURL := strings.TrimRight(os.Getenv("ALCAMPO_BASE_URL"), "/")
	if baseURL == "" {
		baseURL = config.DefaultBaseURL
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
	if c.RegionID == "" {
		c.RegionID = config.DefaultRegionID
	}
	if c.SourceVersion == "" {
		c.SourceVersion = config.DefaultSourceVersion
	}
	return c, nil
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
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 399 {
		return &httpx.StatusError{Method: req.Method, URL: req.URL.String(), StatusCode: resp.StatusCode, Status: resp.Status, Body: snippet(body)}
	}
	if readErr != nil {
		return readErr
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

func (c *Client) deleteJSON(ctx context.Context, path string, referer, route string, out any) error {
	if err := c.InitSession(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	c.setAPIHeaders(req, referer, route)
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
	if c.authCookie != "" {
		req.Header.Set("Cookie", c.authCookie)
	}
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
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

func (c *Client) absoluteURL(v string) string {
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		return v
	}
	if strings.HasPrefix(v, "/") {
		return c.BaseURL + v
	}
	return c.BaseURL + "/" + strings.TrimPrefix(v, "/")
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
