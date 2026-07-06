package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"alcampo-cli/internal/alcampo"
	"alcampo-cli/internal/config"
	"alcampo-cli/internal/money"
)

func requireWriteSession(cfg *config.Config) error {
	if cfg.Auth.Cookie == "" && cfg.Auth.BearerToken == "" {
		return errors.New("cart/checkout writes require an Alcampo session; run login, import-har, or import-curl from your own logged-in session")
	}
	if cfg.Auth.CSRFToken == "" {
		return errors.New("cart/checkout writes require an x-csrf-token; run login or re-import a copied cURL/HAR request that includes it")
	}
	return nil
}

func verifiedCartTotal(ctx context.Context, client *alcampo.Client) (any, int64, error) {
	cart, err := client.Cart(ctx, true)
	if err != nil {
		return nil, 0, err
	}
	total, ok := alcampo.CartTotalCents(cart)
	if !ok {
		return nil, 0, errors.New("could not verify active cart total; refusing write")
	}
	return cart, total, nil
}

func requiredMaxCents(explicit string, cfg *config.Config) (int64, error) {
	value := strings.TrimSpace(explicit)
	if value == "" {
		value = strings.TrimSpace(os.Getenv("ALCAMPO_MAX_EUR"))
	}
	if value == "" {
		value = strings.TrimSpace(cfg.Limits.MaxEUR)
	}
	if value == "" {
		return 0, errors.New("cart/checkout writes require a nonzero spending guard via --max, ALCAMPO_MAX_EUR, or [limits] max_eur")
	}
	maxCents, err := money.ParseCents(value)
	if err != nil {
		return 0, fmt.Errorf("invalid max EUR value: %w", err)
	}
	if maxCents <= 0 {
		return 0, errors.New("spending guard max must be greater than zero")
	}
	return maxCents, nil
}

func checkMax(totalCents, maxCents int64) error {
	if totalCents > maxCents {
		return fmt.Errorf("estimated total %s exceeds spending guard %s", money.Format(totalCents, "EUR"), money.Format(maxCents, "EUR"))
	}
	return nil
}

func moneyValue(cents int64) money.Money {
	return money.Money{Amount: money.FormatAmount(cents), Currency: "EUR", Cents: cents}
}

func newClient(store string) (*config.Config, *alcampo.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	client, err := alcampo.New(cfg)
	if err != nil {
		return nil, nil, err
	}
	if store != "" {
		client.RegionID = store
	}
	return cfg, client, nil
}

func resolveMarket(cfg *config.Config, store string) (string, error) {
	if strings.TrimSpace(store) != "" {
		return strings.TrimSpace(store), nil
	}
	if !cfg.Defaults.MarketSet || strings.TrimSpace(cfg.Defaults.RegionID) == "" {
		return "", errors.New("no market set; run 'alcampo set-market --region-id <region_uuid>' or pass --store/--market for this command")
	}
	return cfg.Defaults.RegionID, nil
}

func warnLocation(stderr io.Writer, cfg *config.Config, postal, store string) {
	if postal != "" && store == "" && postal != cfg.Defaults.PostalCode {
		fmt.Fprintln(stderr, "warning: --postal is accepted as a hint, but anonymous postal-to-region resolution is not verified; using configured region")
	}
}

func enforceMax(totalCents int64, explicit string, cfg *config.Config) error {
	value := strings.TrimSpace(explicit)
	if value == "" {
		value = strings.TrimSpace(os.Getenv("ALCAMPO_MAX_EUR"))
	}
	if value == "" {
		value = strings.TrimSpace(cfg.Limits.MaxEUR)
	}
	if value == "" {
		return nil
	}
	maxCents, err := money.ParseCents(value)
	if err != nil {
		return fmt.Errorf("invalid max EUR value: %w", err)
	}
	if maxCents <= 0 {
		return errors.New("spending guard max must be greater than zero")
	}
	if totalCents > maxCents {
		return fmt.Errorf("total %s exceeds spending guard %s", money.Format(totalCents, "EUR"), money.Format(maxCents, "EUR"))
	}
	return nil
}
