package cli

import (
	"bufio"
	"context"
	cryptorand "crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/auth"
	"github.com/wachtermar/carrito/internal/config"
	"github.com/wachtermar/carrito/internal/output"
	"github.com/wachtermar/carrito/internal/strutil"

	"golang.org/x/term"
)

var (
	loginWebOpenBrowser  = openBrowser
	loginWebLoginAndSave = loginAndSave
)

func runImportHAR(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("import-har", stderr)
	file := fs.String("file", "", "HAR file exported from your own Alcampo web session, or - for stdin")
	if err := parseInterspersed(fs, args, nil); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("import-har requires --file <har|->")
	}
	data, err := readInputBytes(*file)
	if err != nil {
		return err
	}
	session, err := auth.ParseHAR(data)
	if err != nil {
		return err
	}
	return saveImportedSession(session, stdout)
}

func runImportCurl(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("import-curl", stderr)
	file := fs.String("file", "", "file containing a copied cURL command from your own Alcampo web session, or - for stdin")
	clipboard := fs.Bool("clipboard", false, "read a copied cURL command from the system clipboard")
	if err := parseInterspersed(fs, args, map[string]bool{"clipboard": true}); err != nil {
		return err
	}
	var data []byte
	var err error
	switch {
	case *clipboard:
		data, err = readClipboard()
	case *file != "":
		data, err = readInputBytes(*file)
	default:
		return errors.New("import-curl requires --file <curl-file|-> or --clipboard")
	}
	if err != nil {
		return err
	}
	session, err := auth.ParseCurl(data)
	if err != nil {
		return err
	}
	return saveImportedSession(session, stdout)
}

func runLogin(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("login", stderr)
	username := fs.String("username", "", "Alcampo account email; defaults to CARRITO_USERNAME")
	passwordStdin := fs.Bool("password-stdin", false, "read password from stdin")
	destination := fs.String("destination", "/", "post-login Alcampo path")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"password-stdin": true, "json": true}); err != nil {
		return err
	}
	resolvedUsername, err := resolveLoginUsername(*username, stderr)
	if err != nil {
		return err
	}
	password, err := resolveLoginPassword(*passwordStdin, stderr)
	if err != nil {
		return err
	}
	cfg, client, err := newClient("")
	if err != nil {
		return err
	}
	session, err := client.Login(context.Background(), resolvedUsername, password, alcampo.LoginOptions{Destination: *destination})
	if err != nil {
		return err
	}
	if err := mergeImportedSession(cfg, session); err != nil {
		return err
	}
	if err := config.Save(cfg); err != nil {
		return err
	}
	res := struct {
		Authenticated         bool   `json:"authenticated"`
		HasCookie             bool   `json:"has_cookie"`
		HasBearer             bool   `json:"has_bearer_token"`
		HasCSRF               bool   `json:"has_csrf_token"`
		HasCustomerID         bool   `json:"has_customer_id"`
		HasVisitorID          bool   `json:"has_visitor_id"`
		ImportedAt            string `json:"imported_at,omitempty"`
		RegionID              string `json:"region_id,omitempty"`
		RegionName            string `json:"region_name,omitempty"`
		DeliveryDestinationID string `json:"delivery_destination_id,omitempty"`
		SourceVersion         string `json:"source_version,omitempty"`
		Message               string `json:"message"`
	}{
		Authenticated:         cfg.Auth.Cookie != "" || cfg.Auth.BearerToken != "",
		HasCookie:             cfg.Auth.Cookie != "",
		HasBearer:             cfg.Auth.BearerToken != "",
		HasCSRF:               cfg.Auth.CSRFToken != "",
		HasCustomerID:         cfg.Auth.CustomerID != "",
		HasVisitorID:          cfg.Auth.VisitorID != "",
		ImportedAt:            cfg.Auth.ImportedAt,
		RegionID:              cfg.Defaults.RegionID,
		RegionName:            cfg.Defaults.RegionName,
		DeliveryDestinationID: cfg.Defaults.DeliveryDestinationID,
		SourceVersion:         cfg.Session.SourceVersion,
		Message:               "logged in with direct HTTP; password was not stored",
	}
	if *jsonOut {
		return output.JSON(stdout, res)
	}
	fmt.Fprintf(stdout, "logged in: cookie=%t bearer=%t csrf=%t customer_id=%t visitor_id=%t region=%s delivery_destination=%s\n",
		res.HasCookie, res.HasBearer, res.HasCSRF, res.HasCustomerID, res.HasVisitorID, strutil.FirstNonEmpty(res.RegionID, "-"), strutil.FirstNonEmpty(res.DeliveryDestinationID, "-"))
	return nil
}

func runLoginWeb(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("login-web", stderr)
	addr := fs.String("addr", "127.0.0.1:0", "loopback listen address for the temporary login page")
	destination := fs.String("destination", "/", "post-login Alcampo path")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	noOpen := fs.Bool("no-open", false, "print the local login URL instead of opening a browser")
	ifNeeded := fs.Bool("if-needed", false, "skip browser login only when authentication material and a CSRF token are already saved")
	timeoutFlag := fs.String("timeout", "5m", "maximum time to wait for browser login")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true, "no-open": true, "if-needed": true}); err != nil {
		return err
	}
	if err := validateLoginWebAddr(*addr); err != nil {
		return err
	}
	timeout, err := time.ParseDuration(*timeoutFlag)
	if err != nil {
		return fmt.Errorf("invalid --timeout: %w", err)
	}
	if timeout <= 0 {
		return errors.New("--timeout must be greater than zero")
	}
	if *ifNeeded {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if loginWebSessionIsWriteReady(cfg) {
			res := authStatusResponse(cfg, "already logged in; browser login was skipped")
			if *jsonOut {
				return output.JSON(stdout, res)
			}
			fmt.Fprintln(stdout, "already logged in; browser login was skipped")
			return nil
		}
	}
	formToken, err := newLoginWebFormToken()
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok || tcpAddr.IP == nil || !tcpAddr.IP.IsLoopback() {
		_ = ln.Close()
		return errors.New("local login listener did not resolve to a loopback IP")
	}
	defer ln.Close()
	loginOrigin := "http://" + ln.Addr().String()

	type loginWebResult struct {
		response any
		err      error
	}
	resultCh := make(chan loginWebResult, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, loginWebPage(formToken))
	})
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" && origin != loginOrigin {
			http.Error(w, "invalid origin", http.StatusForbidden)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "could not read form", http.StatusBadRequest)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Form.Get("form_token")), []byte(formToken)) != 1 {
			http.Error(w, "invalid form token", http.StatusForbidden)
			return
		}
		username := strings.TrimSpace(r.Form.Get("username"))
		password := r.Form.Get("password")
		if username == "" || password == "" {
			http.Error(w, "email and password are required", http.StatusBadRequest)
			return
		}
		res, err := loginWebLoginAndSave(username, password, *destination, "logged in from local browser; password was not stored")
		password = ""
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err != nil {
			_, _ = fmt.Fprintf(w, "<!doctype html><title>Alcampo login failed</title><h1>Login failed</h1><p>%s</p><p>You can close this tab.</p>", htmlText(err.Error()))
		} else {
			_, _ = io.WriteString(w, "<!doctype html><title>Alcampo login complete</title><h1>Alcampo login complete</h1><p>You can close this tab and return to Hermes.</p>")
		}
		select {
		case resultCh <- loginWebResult{response: res, err: err}:
		default:
		}
	})

	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	serverErr := make(chan error, 1)
	go func() {
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()
	defer server.Shutdown(context.Background())

	loginURL := loginOrigin + "/"
	if *noOpen {
		fmt.Fprintf(stderr, "Open this local login page: %s\n", loginURL)
	} else if err := loginWebOpenBrowser(loginURL); err != nil {
		fmt.Fprintf(stderr, "could not open browser automatically: %v\nOpen this local login page: %s\n", err, loginURL)
	}

	select {
	case result := <-resultCh:
		if result.err != nil {
			return result.err
		}
		if *jsonOut {
			return output.JSON(stdout, result.response)
		}
		fmt.Fprintln(stdout, "logged in from browser; password was not stored")
		return nil
	case err := <-serverErr:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("timed out waiting for browser login at %s", loginURL)
	}
}

func loginWebSessionIsWriteReady(cfg *config.Config) bool {
	return cfg != nil && (cfg.Auth.Cookie != "" || cfg.Auth.BearerToken != "") && cfg.Auth.CSRFToken != ""
}

func validateLoginWebAddr(addr string) error {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return fmt.Errorf("invalid --addr %q: expected host:port: %w", addr, err)
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("--addr must use literal localhost or a loopback IP, such as 127.0.0.1:0 or [::1]:0")
	}
	return nil
}

func newLoginWebFormToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := cryptorand.Read(raw); err != nil {
		return "", fmt.Errorf("generate local login form token: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

func loginWebPage(formToken string) string {
	return strings.Replace(loginWebPageHTML, "__CARRITO_FORM_TOKEN__", formToken, 1)
}

const loginWebPageHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Alcampo login</title>
<style>body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;max-width:440px;margin:10vh auto;padding:24px;color:#17202a}label{display:block;margin:14px 0 6px}input,button{font:inherit;width:100%;box-sizing:border-box;padding:10px 12px}button{margin-top:18px;background:#14532d;color:white;border:0;border-radius:8px;cursor:pointer}.note{color:#5f6b7a;font-size:14px;line-height:1.4}</style>
</head><body><h1>Alcampo login</h1><p class="note">This local page sends your credentials only to the carrito CLI running on this computer. The password is used once to create a session and is not stored.</p>
<form method="post" action="/login"><input type="hidden" name="form_token" value="__CARRITO_FORM_TOKEN__"><label for="username">Email</label><input id="username" name="username" type="email" autocomplete="username" required autofocus><label for="password">Password</label><input id="password" name="password" type="password" autocomplete="current-password" required><button type="submit">Log in</button></form></body></html>`

func htmlText(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

func openBrowser(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	return cmd.Start()
}

func authStatusResponse(cfg *config.Config, message string) any {
	return struct {
		Authenticated         bool   `json:"authenticated"`
		HasCookie             bool   `json:"has_cookie"`
		HasBearer             bool   `json:"has_bearer_token"`
		HasCSRF               bool   `json:"has_csrf_token"`
		HasCustomerID         bool   `json:"has_customer_id"`
		HasVisitorID          bool   `json:"has_visitor_id"`
		ImportedAt            string `json:"imported_at,omitempty"`
		RegionID              string `json:"region_id,omitempty"`
		RegionName            string `json:"region_name,omitempty"`
		DeliveryDestinationID string `json:"delivery_destination_id,omitempty"`
		SourceVersion         string `json:"source_version,omitempty"`
		Message               string `json:"message"`
	}{
		Authenticated:         cfg.Auth.Cookie != "" || cfg.Auth.BearerToken != "",
		HasCookie:             cfg.Auth.Cookie != "",
		HasBearer:             cfg.Auth.BearerToken != "",
		HasCSRF:               cfg.Auth.CSRFToken != "",
		HasCustomerID:         cfg.Auth.CustomerID != "",
		HasVisitorID:          cfg.Auth.VisitorID != "",
		ImportedAt:            cfg.Auth.ImportedAt,
		RegionID:              cfg.Defaults.RegionID,
		RegionName:            cfg.Defaults.RegionName,
		DeliveryDestinationID: cfg.Defaults.DeliveryDestinationID,
		SourceVersion:         cfg.Session.SourceVersion,
		Message:               message,
	}
}

func loginAndSave(username, password, destination, message string) (any, error) {
	cfg, client, err := newClient("")
	if err != nil {
		return nil, err
	}
	session, err := client.Login(context.Background(), username, password, alcampo.LoginOptions{Destination: destination})
	if err != nil {
		return nil, err
	}
	if err := mergeImportedSession(cfg, session); err != nil {
		return nil, err
	}
	if err := config.Save(cfg); err != nil {
		return nil, err
	}
	return authStatusResponse(cfg, message), nil
}

func saveImportedSession(session auth.Session, stdout io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := mergeImportedSession(cfg, session); err != nil {
		return err
	}
	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "imported session: cookie=%t bearer=%t csrf=%t region=%s delivery_destination=%s\n",
		session.Cookie != "", session.BearerToken != "", session.CSRFToken != "", cfg.Defaults.RegionID, strutil.FirstNonEmpty(cfg.Defaults.DeliveryDestinationID, "-"))
	return nil
}

func mergeImportedSession(cfg *config.Config, session auth.Session) error {
	if session.Cookie != "" {
		cfg.Auth.Cookie = session.Cookie
	}
	if session.BearerToken != "" {
		cfg.Auth.BearerToken = session.BearerToken
	}
	if session.CSRFToken != "" {
		cfg.Auth.CSRFToken = session.CSRFToken
	}
	if session.CustomerID != "" {
		cfg.Auth.CustomerID = session.CustomerID
	}
	if session.VisitorID != "" {
		cfg.Auth.VisitorID = session.VisitorID
	}
	if session.ImportedAt != "" {
		cfg.Auth.ImportedAt = session.ImportedAt
	}
	if session.SourceVersion != "" {
		cfg.Session.SourceVersion = session.SourceVersion
	}
	if session.RegionID != "" {
		cfg.Defaults.RegionID = session.RegionID
		cfg.Defaults.MarketSet = true
	}
	if session.RetailerRegionID != "" {
		cfg.Defaults.RetailerRegionID = session.RetailerRegionID
	}
	if session.RegionName != "" {
		cfg.Defaults.RegionName = session.RegionName
	}
	if session.DeliveryDestinationID != "" {
		cfg.Defaults.DeliveryDestinationID = session.DeliveryDestinationID
	}
	return nil
}

func resolveLoginUsername(explicit string, stderr io.Writer) (string, error) {
	username := strings.TrimSpace(explicit)
	if username == "" {
		username = strings.TrimSpace(os.Getenv("CARRITO_USERNAME"))
	}
	if username == "" {
		username = strings.TrimSpace(os.Getenv("ALCAMPO_USERNAME"))
	}
	if username == "" && term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(stderr, "Alcampo email: ")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		username = strings.TrimSpace(line)
	}
	if username == "" {
		return "", errors.New("login requires --username <email> or CARRITO_USERNAME")
	}
	return username, nil
}

func resolveLoginPassword(passwordStdin bool, stderr io.Writer) (string, error) {
	var password string
	if passwordStdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		password = strings.TrimRight(string(data), "\r\n")
	} else {
		password = os.Getenv("CARRITO_PASSWORD")
		if password == "" {
			password = os.Getenv("ALCAMPO_PASSWORD")
		}
	}
	if password == "" && term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(stderr, "Alcampo password: ")
		data, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(stderr)
		if err != nil {
			return "", err
		}
		password = string(data)
	}
	if password == "" {
		return "", errors.New("login requires CARRITO_PASSWORD, --password-stdin, or an interactive terminal prompt")
	}
	return password, nil
}

func runWhoami(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("whoami", stderr)
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	res := struct {
		Authenticated bool   `json:"authenticated"`
		HasCookie     bool   `json:"has_cookie"`
		HasBearer     bool   `json:"has_bearer_token"`
		HasCSRF       bool   `json:"has_csrf_token"`
		HasCustomerID bool   `json:"has_customer_id"`
		HasVisitorID  bool   `json:"has_visitor_id"`
		ImportedAt    string `json:"imported_at,omitempty"`
		PostalCode    string `json:"postal_code,omitempty"`
		MarketSet     bool   `json:"market_set"`
		RegionID      string `json:"region_id,omitempty"`
		RegionName    string `json:"region_name,omitempty"`
		DeliveryID    string `json:"delivery_destination_id,omitempty"`
		SourceVersion string `json:"source_version,omitempty"`
		Message       string `json:"message"`
	}{
		HasCookie:     cfg.Auth.Cookie != "",
		HasBearer:     cfg.Auth.BearerToken != "",
		HasCSRF:       cfg.Auth.CSRFToken != "",
		HasCustomerID: cfg.Auth.CustomerID != "",
		HasVisitorID:  cfg.Auth.VisitorID != "",
		ImportedAt:    cfg.Auth.ImportedAt,
		PostalCode:    cfg.Defaults.PostalCode,
		MarketSet:     cfg.Defaults.MarketSet,
		RegionID:      cfg.Defaults.RegionID,
		RegionName:    cfg.Defaults.RegionName,
		DeliveryID:    cfg.Defaults.DeliveryDestinationID,
		SourceVersion: cfg.Session.SourceVersion,
		Message:       "identity endpoint not verified; this reports locally imported session material only",
	}
	res.Authenticated = res.HasCookie || res.HasBearer
	if *jsonOut {
		return output.JSON(stdout, res)
	}
	if !res.Authenticated {
		fmt.Fprintln(stdout, "not authenticated; run login or import an Alcampo web HAR/copied cURL command to store session cookies/tokens")
	} else {
		fmt.Fprintf(stdout, "session present: cookie=%t bearer=%t csrf=%t customer_id=%t visitor_id=%t imported_at=%s\n", res.HasCookie, res.HasBearer, res.HasCSRF, res.HasCustomerID, res.HasVisitorID, res.ImportedAt)
	}
	fmt.Fprintf(stdout, "market_set=%t region=%s (%s) postal=%s delivery_destination=%s\n", res.MarketSet, res.RegionID, res.RegionName, res.PostalCode, strutil.FirstNonEmpty(res.DeliveryID, "-"))
	return nil
}
