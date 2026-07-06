package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"alcampo-cli/internal/alcampo"
	"alcampo-cli/internal/auth"
	"alcampo-cli/internal/config"
	"alcampo-cli/internal/output"
	"alcampo-cli/internal/strutil"

	"golang.org/x/term"
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
	username := fs.String("username", "", "Alcampo account email; defaults to ALCAMPO_USERNAME")
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
		return "", errors.New("login requires --username <email> or ALCAMPO_USERNAME")
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
		password = os.Getenv("ALCAMPO_PASSWORD")
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
		return "", errors.New("login requires ALCAMPO_PASSWORD, --password-stdin, or an interactive terminal prompt")
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
