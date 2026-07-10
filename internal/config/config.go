package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	DefaultBaseURL          = "https://www.compraonline.alcampo.es"
	DefaultRegionID         = "ac90d761-9d58-4918-a37d-dd14e1ce384a"
	DefaultRetailerRegionID = "5"
	DefaultRegionName       = "Vaguada"
	DefaultSourceVersion    = "2.0.0-2026-07-02-08h35m10s-4a524088"
)

type Config struct {
	Defaults Defaults `json:"defaults"`
	Limits   Limits   `json:"limits"`
	Auth     Auth     `json:"auth"`
	Session  Session  `json:"session"`
}

type Defaults struct {
	PostalCode            string `json:"postal_code,omitempty"`
	RegionID              string `json:"region_id,omitempty"`
	RetailerRegionID      string `json:"retailer_region_id,omitempty"`
	RegionName            string `json:"region_name,omitempty"`
	DeliveryDestinationID string `json:"delivery_destination_id,omitempty"`
	MarketSet             bool   `json:"market_set"`
}

type Limits struct {
	MaxEUR string `json:"max_eur,omitempty"`
}

type Auth struct {
	Cookie      string `json:"-"`
	BearerToken string `json:"-"`
	CSRFToken   string `json:"-"`
	CustomerID  string `json:"-"`
	VisitorID   string `json:"-"`
	ImportedAt  string `json:"imported_at,omitempty"`
}

type Session struct {
	SourceVersion string `json:"source_version,omitempty"`
}

func Default() *Config {
	cfg := &Config{}
	cfg.Defaults.RegionID = DefaultRegionID
	cfg.Defaults.RetailerRegionID = DefaultRetailerRegionID
	cfg.Defaults.RegionName = DefaultRegionName
	cfg.Session.SourceVersion = DefaultSourceVersion
	return cfg
}

func Dir() (string, error) {
	if v := os.Getenv("CARRITO_CONFIG_DIR"); v != "" {
		return v, nil
	}
	if v := os.Getenv("ALCAMPO_CONFIG_DIR"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".carrito"), nil
}

func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

func Load() (*Config, error) {
	cfg := Default()
	path, err := Path()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	section := ""
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(stripInlineComment(scanner.Text()))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value := parseValue(strings.TrimSpace(val))
		set(cfg, section, key, value)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	fillDefaults(cfg)
	return cfg, nil
}

func Save(cfg *Config) error {
	fillDefaults(cfg)
	path, err := Path()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	data := []byte(render(cfg))
	temp, err := os.CreateTemp(dir, ".config.toml.tmp-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	ok := false
	defer func() {
		_ = temp.Close()
		if !ok {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	ok = true
	return nil
}

func fillDefaults(cfg *Config) {
	if cfg.Defaults.RegionID == "" {
		cfg.Defaults.RegionID = DefaultRegionID
	}
	if cfg.Defaults.RetailerRegionID == "" {
		cfg.Defaults.RetailerRegionID = DefaultRetailerRegionID
	}
	if cfg.Defaults.RegionName == "" {
		cfg.Defaults.RegionName = DefaultRegionName
	}
	if cfg.Session.SourceVersion == "" {
		cfg.Session.SourceVersion = DefaultSourceVersion
	}
}

func set(cfg *Config, section, key, value string) {
	switch section {
	case "defaults":
		switch key {
		case "postal_code":
			cfg.Defaults.PostalCode = value
		case "region_id":
			cfg.Defaults.RegionID = value
		case "retailer_region_id":
			cfg.Defaults.RetailerRegionID = value
		case "region_name":
			cfg.Defaults.RegionName = value
		case "delivery_destination_id":
			cfg.Defaults.DeliveryDestinationID = value
		case "market_set":
			cfg.Defaults.MarketSet = parseBool(value)
		}
	case "limits":
		if key == "max_eur" {
			cfg.Limits.MaxEUR = value
		}
	case "auth":
		switch key {
		case "cookie":
			cfg.Auth.Cookie = value
		case "bearer_token":
			cfg.Auth.BearerToken = value
		case "csrf_token":
			cfg.Auth.CSRFToken = value
		case "customer_id":
			cfg.Auth.CustomerID = value
		case "visitor_id":
			cfg.Auth.VisitorID = value
		case "imported_at":
			cfg.Auth.ImportedAt = value
		}
	case "session":
		if key == "source_version" {
			cfg.Session.SourceVersion = value
		}
	}
}

func render(cfg *Config) string {
	var b strings.Builder
	fmt.Fprintln(&b, "[defaults]")
	writeKV(&b, "postal_code", cfg.Defaults.PostalCode)
	writeKV(&b, "region_id", cfg.Defaults.RegionID)
	writeKV(&b, "retailer_region_id", cfg.Defaults.RetailerRegionID)
	writeKV(&b, "region_name", cfg.Defaults.RegionName)
	writeKV(&b, "delivery_destination_id", cfg.Defaults.DeliveryDestinationID)
	fmt.Fprintf(&b, "market_set = %t\n", cfg.Defaults.MarketSet)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "[limits]")
	writeKV(&b, "max_eur", cfg.Limits.MaxEUR)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "[session]")
	writeKV(&b, "source_version", cfg.Session.SourceVersion)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "[auth]")
	writeKV(&b, "cookie", cfg.Auth.Cookie)
	writeKV(&b, "bearer_token", cfg.Auth.BearerToken)
	writeKV(&b, "csrf_token", cfg.Auth.CSRFToken)
	writeKV(&b, "customer_id", cfg.Auth.CustomerID)
	writeKV(&b, "visitor_id", cfg.Auth.VisitorID)
	writeKV(&b, "imported_at", cfg.Auth.ImportedAt)
	return b.String()
}

func writeKV(b *strings.Builder, key, value string) {
	fmt.Fprintf(b, "%s = %s\n", key, strconv.Quote(value))
}

func parseValue(s string) string {
	if strings.HasPrefix(s, `"`) {
		if v, err := strconv.Unquote(s); err == nil {
			return v
		}
	}
	if strings.HasPrefix(s, `'`) && strings.HasSuffix(s, `'`) && len(s) >= 2 {
		return strings.Trim(s, `'`)
	}
	return strings.TrimSpace(s)
}

func parseBool(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	return s == "true" || s == "1" || s == "yes"
}

func stripInlineComment(s string) string {
	inQuote := rune(0)
	escaped := false
	for i, r := range s {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && inQuote == '"' {
			escaped = true
			continue
		}
		if inQuote != 0 {
			if r == inQuote {
				inQuote = 0
			}
			continue
		}
		if r == '"' || r == '\'' {
			inQuote = r
			continue
		}
		if r == '#' {
			return s[:i]
		}
	}
	return s
}
