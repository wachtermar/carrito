package alcampo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wachtermar/carrito/internal/auth"
	"github.com/wachtermar/carrito/internal/httpx"
)

const defaultLoginDestination = "/"

type LoginOptions struct {
	Destination string
}

func (c *Client) Login(ctx context.Context, username, password string, opts LoginOptions) (auth.Session, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return auth.Session{}, errors.New("username is required")
	}
	if password == "" {
		return auth.Session{}, errors.New("password is required")
	}
	destination := strings.TrimSpace(opts.Destination)
	if destination == "" {
		destination = defaultLoginDestination
	}

	loginURL := c.BaseURL + "/login?destination=" + url.QueryEscape(destination)
	relayBody, relayURL, err := c.fetchLoginPage(ctx, loginURL, c.BaseURL+"/")
	if err != nil {
		return auth.Session{}, fmt.Errorf("start login: %w", err)
	}

	relay, err := parseRelayForm(relayBody)
	if err != nil {
		return auth.Session{}, err
	}
	relayAction, err := resolveRelativeURL(relayURL, relay.Action)
	if err != nil {
		return auth.Session{}, err
	}
	redirectBody, redirectURL, err := c.postLoginForm(ctx, relayAction, relay.Values, relayURL.String())
	if err != nil {
		return auth.Session{}, fmt.Errorf("submit login relay: %w", err)
	}
	authorizationPath := extractJSRedirect(redirectBody)
	if authorizationPath == "" {
		return auth.Session{}, errors.New("login relay did not return an authorization redirect")
	}
	authorizationURL, err := resolveRelativeURL(redirectURL, authorizationPath)
	if err != nil {
		return auth.Session{}, err
	}

	formBody, formURL, err := c.fetchLoginPage(ctx, authorizationURL, redirectURL.String())
	if err != nil {
		return auth.Session{}, fmt.Errorf("fetch credential form: %w", err)
	}
	form, err := parseCredentialForm(formBody)
	if err != nil {
		return auth.Session{}, err
	}
	credentialAction, err := resolveRelativeURL(formURL, form.Action)
	if err != nil {
		return auth.Session{}, err
	}
	loginValues := form.Values
	loginValues.Set("username", username)
	loginValues.Set("password", password)
	loginValues.Set("hpot", "")
	loginValues.Set("startUrl", form.StartURL)
	loginValues.Set(form.LoginParam, form.LoginParam)

	ajaxBody, ajaxURL, err := c.postLoginForm(ctx, credentialAction, loginValues, formURL.String())
	if err != nil {
		return auth.Session{}, fmt.Errorf("submit credentials: %w", err)
	}
	if msg := extractLoginError(ajaxBody); msg != "" {
		return auth.Session{}, fmt.Errorf("login failed: %s", msg)
	}
	next := extractJSRedirect(ajaxBody)
	finalBody := ajaxBody
	completionReferer := ajaxURL.String()
	if next != "" {
		nextURL, err := resolveRelativeURL(ajaxURL, next)
		if err != nil {
			return auth.Session{}, err
		}
		completionReferer = nextURL
		finalBody, _, err = c.followLoginCompletion(ctx, nextURL, ajaxURL.String())
		if err != nil {
			return auth.Session{}, err
		}
	} else if !isLoggedInAlcampoPage(c.BaseURL, ajaxURL, ajaxBody) {
		return auth.Session{}, errors.New("credential submit did not return an OAuth redirect or logged-in Alcampo session page")
	}
	if !isLoggedInBody(finalBody) {
		finalBody, _, err = c.fetchLoginPage(ctx, c.BaseURL+"/", completionReferer)
		if err != nil {
			return auth.Session{}, fmt.Errorf("fetch logged-in homepage: %w", err)
		}
	}
	if !isLoggedInBody(finalBody) {
		return auth.Session{}, errors.New("login completed but Alcampo did not return a logged-in session")
	}

	session := auth.ParseText(finalBody)
	session.Cookie = c.cookieHeaderForBase()
	session.CSRFToken = extractCSRFToken(finalBody)
	session.SourceVersion = firstString(extractSourceVersion(finalBody), c.SourceVersion)
	session.ImportedAt = time.Now().UTC().Format(time.RFC3339)
	if session.Cookie == "" {
		return auth.Session{}, errors.New("login completed but no Alcampo session cookies were captured")
	}
	if session.CSRFToken == "" {
		return auth.Session{}, errors.New("login completed but no Alcampo CSRF token was found")
	}
	return session, nil
}

type loginRelayForm struct {
	Action string
	Values url.Values
}

type credentialForm struct {
	Action     string
	LoginParam string
	StartURL   string
	Values     url.Values
}

func (c *Client) fetchLoginPage(ctx context.Context, rawURL, referer string) ([]byte, *url.URL, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, err
	}
	c.setPageHeaders(req, referer)
	body, finalURL, err := c.doLoginRequest(req)
	if err != nil {
		return nil, nil, err
	}
	return body, finalURL, nil
}

func (c *Client) postLoginForm(ctx context.Context, rawURL string, values url.Values, referer string) ([]byte, *url.URL, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, nil, err
	}
	c.setPageHeaders(req, referer)
	req.Header.Set("Accept", "text/xml,application/xml,application/xhtml+xml,text/html;q=0.9,*/*;q=0.8")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	body, finalURL, err := c.doLoginRequest(req)
	if err != nil {
		return nil, nil, err
	}
	return body, finalURL, nil
}

func (c *Client) doLoginRequest(req *http.Request) ([]byte, *url.URL, error) {
	c.throttle()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 6<<20))
	if version := resp.Header.Get("ecom-request-source-version"); version != "" {
		c.SourceVersion = version
	}
	if resp.StatusCode < 200 || resp.StatusCode > 399 {
		return nil, nil, &httpx.StatusError{Method: req.Method, URL: req.URL.String(), StatusCode: resp.StatusCode, Status: resp.Status, Body: snippet(body)}
	}
	if readErr != nil {
		return nil, nil, readErr
	}
	return body, resp.Request.URL, nil
}

func (c *Client) followLoginCompletion(ctx context.Context, rawURL, referer string) ([]byte, *url.URL, error) {
	current := rawURL
	ref := referer
	var body []byte
	var finalURL *url.URL
	var err error
	haveBody := false
	for i := 0; i < 16; i++ {
		if !haveBody {
			body, finalURL, err = c.fetchLoginPage(ctx, current, ref)
			if err != nil {
				return nil, nil, fmt.Errorf("complete OAuth login: %w", err)
			}
		}
		haveBody = false
		if isLoggedInAlcampoPage(c.BaseURL, finalURL, body) {
			return body, finalURL, nil
		}
		next := extractJSRedirect(body)
		if next == "" {
			if form, ok := parseJSFPostForm(body); ok {
				action, err := resolveRelativeURL(finalURL, form.Action)
				if err != nil {
					return nil, nil, err
				}
				body, finalURL, err = c.postLoginForm(ctx, action, form.Values, finalURL.String())
				if err != nil {
					return nil, nil, fmt.Errorf("submit OAuth intermediary form: %w", err)
				}
				haveBody = true
				continue
			}
			break
		}
		ref = finalURL.String()
		current, err = resolveRelativeURL(finalURL, next)
		if err != nil {
			return nil, nil, err
		}
	}
	return body, finalURL, nil
}

func (c *Client) cookieHeaderForBase() string {
	if c.httpClient == nil || c.httpClient.Jar == nil {
		return ""
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return ""
	}
	cookies := c.httpClient.Jar.Cookies(u)
	parts := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie.Name == "" {
			continue
		}
		parts = append(parts, cookie.Name+"="+cookie.Value)
	}
	return strings.Join(parts, "; ")
}

func parseRelayForm(body []byte) (loginRelayForm, error) {
	formTag, ok := findFormTag(body, "ARCD_CIAM_Redirect")
	if !ok {
		return loginRelayForm{}, errors.New("login relay form was not found")
	}
	action := attrValue(formTag, "action")
	formID := attrValue(formTag, "id")
	if action == "" || formID == "" {
		return loginRelayForm{}, errors.New("login relay form was missing action or id")
	}
	command := firstJSFCommand(body)
	if command == "" {
		command = formID + ":loginRedirect"
	}
	values := url.Values{}
	values.Set(formID, formID)
	values.Set(command, command)
	addHiddenValue(values, body, "com.salesforce.visualforce.ViewState")
	addHiddenValue(values, body, "com.salesforce.visualforce.ViewStateVersion")
	addHiddenValue(values, body, "com.salesforce.visualforce.ViewStateMAC")
	return loginRelayForm{Action: action, Values: values}, nil
}

func parseCredentialForm(body []byte) (credentialForm, error) {
	formTag, ok := findFormTag(body, "ARCD_CIAM_Login")
	if !ok {
		return credentialForm{}, errors.New("credential login form was not found")
	}
	action := attrValue(formTag, "action")
	formID := attrValue(formTag, "id")
	if action == "" || formID == "" {
		return credentialForm{}, errors.New("credential login form was missing action or id")
	}
	startURL := extractSingleQuotedJS(body, `var\s+startURL\s*=\s*'([^']*)'`)
	if startURL == "" {
		return credentialForm{}, errors.New("credential login form was missing startURL")
	}
	loginParam := extractSingleQuotedJS(body, `'parameters'\s*:\s*\{[^}]*'([^']+:login)'`)
	if loginParam == "" {
		loginParam = formID + ":login"
	}
	values := url.Values{}
	values.Set("AJAXREQUEST", "_viewRoot")
	values.Set(formID, formID)
	values.Set("uname1", "")
	values.Set("passwordLogin", "")
	addHiddenValue(values, body, "com.salesforce.visualforce.ViewState")
	addHiddenValue(values, body, "com.salesforce.visualforce.ViewStateVersion")
	addHiddenValue(values, body, "com.salesforce.visualforce.ViewStateMAC")
	return credentialForm{Action: action, LoginParam: loginParam, StartURL: startURL, Values: values}, nil
}

func parseJSFPostForm(body []byte) (loginRelayForm, bool) {
	formTag, ok := firstFormTag(body)
	if !ok {
		return loginRelayForm{}, false
	}
	action := attrValue(formTag, "action")
	formID := attrValue(formTag, "id")
	command := firstJSFCommand(body)
	if action == "" || formID == "" || command == "" {
		return loginRelayForm{}, false
	}
	values := url.Values{}
	values.Set(formID, formID)
	values.Set(command, command)
	addAllHiddenInputs(values, body)
	return loginRelayForm{Action: action, Values: values}, true
}

func findFormTag(body []byte, actionContains string) (string, bool) {
	for _, match := range regexp.MustCompile(`(?is)<form\b[^>]*>`).FindAll(body, -1) {
		tag := string(match)
		if strings.Contains(attrValue(tag, "action"), actionContains) {
			return tag, true
		}
	}
	return "", false
}

func firstFormTag(body []byte) (string, bool) {
	match := regexp.MustCompile(`(?is)<form\b[^>]*>`).Find(body)
	if len(match) == 0 {
		return "", false
	}
	return string(match), true
}

func addHiddenValue(values url.Values, body []byte, name string) {
	if v := inputValueByName(body, name); v != "" {
		values.Set(name, v)
	}
}

func addAllHiddenInputs(values url.Values, body []byte) {
	for _, match := range regexp.MustCompile(`(?is)<input\b[^>]*>`).FindAll(body, -1) {
		tag := string(match)
		if !strings.EqualFold(attrValue(tag, "type"), "hidden") {
			continue
		}
		name := attrValue(tag, "name")
		if name == "" {
			continue
		}
		values.Set(name, attrValue(tag, "value"))
	}
}

func inputValueByName(body []byte, name string) string {
	for _, match := range regexp.MustCompile(`(?is)<input\b[^>]*>`).FindAll(body, -1) {
		tag := string(match)
		if attrValue(tag, "name") == name {
			return attrValue(tag, "value")
		}
	}
	return ""
}

func attrValue(tag, name string) string {
	re := regexp.MustCompile(`(?is)\b` + regexp.QuoteMeta(name) + `\s*=\s*"([^"]*)"|\b` + regexp.QuoteMeta(name) + `\s*=\s*'([^']*)'`)
	m := re.FindStringSubmatch(tag)
	if len(m) != 3 {
		return ""
	}
	if m[1] != "" {
		return html.UnescapeString(m[1])
	}
	return html.UnescapeString(m[2])
}

func firstJSFCommand(body []byte) string {
	return extractSingleQuotedJS(body, `jsfcljs\([^,]+,\s*'([^,']+)`)
}

func extractSingleQuotedJS(body []byte, pattern string) string {
	re := regexp.MustCompile(`(?is)` + pattern)
	m := re.FindSubmatch(body)
	if len(m) != 2 {
		return ""
	}
	return html.UnescapeString(string(m[1]))
}

func extractJSRedirect(body []byte) string {
	patterns := []string{
		`(?is)window\.location\.replace\(\s*['"]([^'"]+)['"]\s*\)`,
		`(?is)window\.location\.href\s*=\s*['"]([^'"]+)['"]`,
		`(?is)handleRedirect\(\s*['"]([^'"]+)['"]\s*\)`,
		`(?is)<meta[^>]+\bname\s*=\s*['"]Location['"][^>]+\bcontent\s*=\s*['"]([^'"]+)['"]`,
		`(?is)<meta[^>]+http-equiv\s*=\s*['"]refresh['"][^>]+content\s*=\s*['"][^;]+;\s*url=([^'"]+)['"]`,
	}
	for _, pattern := range patterns {
		m := regexp.MustCompile(pattern).FindSubmatch(body)
		if len(m) == 2 {
			return html.UnescapeString(strings.TrimSpace(string(m[1])))
		}
	}
	return ""
}

func isLoggedInAlcampoPage(baseURL string, finalURL *url.URL, body []byte) bool {
	if finalURL == nil {
		return false
	}
	if !strings.EqualFold(finalURL.Host, baseHost(baseURL)) {
		return false
	}
	return isLoggedInBody(body)
}

func isLoggedInBody(body []byte) bool {
	return bytes.Contains(body, []byte(`"isLoggedIn":true`)) || bytes.Contains(body, []byte(`\"isLoggedIn\":true`))
}

func extractLoginError(body []byte) string {
	if !bytes.Contains(body, []byte("hasMessages = true")) {
		return ""
	}
	msg := extractSingleQuotedJS(body, `var\s+errorMessage\s*=\s*"([^"]*)"`)
	if msg == "" {
		msg = extractSingleQuotedJS(body, `var\s+errorMessage\s*=\s*'([^']*)'`)
	}
	msg = strings.TrimSpace(unquoteJSString(msg))
	if msg == "" {
		msg = "credentials were rejected"
	}
	return msg
}

func unquoteJSString(s string) string {
	if s == "" {
		return ""
	}
	if unquoted, err := strconv.Unquote(`"` + strings.ReplaceAll(s, `"`, `\"`) + `"`); err == nil {
		return unquoted
	}
	return s
}

func extractCSRFToken(body []byte) string {
	patterns := []string{
		`(?is)"csrf"\s*:\s*\{\s*"token"\s*:\s*"([^"]+)"`,
		`(?is)\\"csrf\\"\s*:\s*\{\s*\\"token\\"\s*:\s*\\"([^"\\]+)\\"`,
	}
	for _, pattern := range patterns {
		m := regexp.MustCompile(pattern).FindSubmatch(body)
		if len(m) == 2 {
			return string(m[1])
		}
	}
	return ""
}

func resolveRelativeURL(base *url.URL, raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	return base.ResolveReference(u).String(), nil
}

func baseHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}
