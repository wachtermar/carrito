package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"alcampo-cli/internal/alcampo"
	"alcampo-cli/internal/config"
	"alcampo-cli/internal/output"
	"alcampo-cli/internal/strutil"
)

func runMarket(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("market", stderr)
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	res := marketStatus(cfg)
	if *jsonOut {
		return output.JSON(stdout, res)
	}
	if !res.MarketSet {
		fmt.Fprintln(stdout, "no market set")
		fmt.Fprintln(stdout, "run: alcampo set-market --region-id <region_uuid> --name <market_name>")
		return nil
	}
	fmt.Fprintf(stdout, "region=%s (%s) retailer_region=%s delivery_destination=%s\n",
		res.RegionID, res.RegionName, res.RetailerRegionID, strutil.FirstNonEmpty(res.DeliveryDestinationID, "-"))
	return nil
}

func runSetMarket(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("set-market", stderr)
	regionID := fs.String("region-id", "", "required Alcampo region UUID")
	retailerRegionID := fs.String("retailer-region-id", "", "optional retailer region id")
	name := fs.String("name", "", "optional market name")
	deliveryDestinationID := fs.String("delivery-destination-id", "", "optional delivery destination/address id")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if *regionID == "" && fs.NArg() > 0 {
		*regionID = fs.Arg(0)
	}
	*regionID = strings.TrimSpace(*regionID)
	if *regionID == "" {
		return errors.New("set-market requires --region-id <region_uuid>")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.Defaults.RegionID = *regionID
	if *retailerRegionID != "" {
		cfg.Defaults.RetailerRegionID = *retailerRegionID
	}
	if *name != "" {
		cfg.Defaults.RegionName = *name
	}
	if *deliveryDestinationID != "" {
		cfg.Defaults.DeliveryDestinationID = *deliveryDestinationID
	}
	cfg.Defaults.MarketSet = true
	if err := config.Save(cfg); err != nil {
		return err
	}
	res := marketStatus(cfg)
	if *jsonOut {
		return output.JSON(stdout, res)
	}
	fmt.Fprintf(stdout, "saved market region=%s (%s)\n", res.RegionID, res.RegionName)
	return nil
}

func runAddresses(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("addresses", stderr)
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	_, client, err := newClient("")
	if err != nil {
		return err
	}
	addresses, err := client.DeliveryAddresses(context.Background())
	if err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, addresses)
	}
	for _, a := range addresses {
		fmt.Fprintf(stdout, "%s\t%s\t%s\tprimary=%t\t%s\n",
			a.DeliveryDestinationID,
			strutil.FirstNonEmpty(a.ResolvedRegionID, "-"),
			strutil.FirstNonEmpty(a.PostalCode, "-"),
			a.IsPrimary,
			strutil.FirstNonEmpty(a.Name, a.FormattedAddress, "-"),
		)
	}
	return nil
}

func runSetAddress(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("set-address", stderr)
	id := fs.String("id", "", "delivery destination id")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if *id == "" && fs.NArg() > 0 {
		*id = fs.Arg(0)
	}
	*id = strings.TrimSpace(*id)
	if *id == "" {
		return errors.New("set-address requires a delivery destination id")
	}
	cfg, client, err := newClient("")
	if err != nil {
		return err
	}
	destination, err := client.DeliveryAddress(context.Background(), *id)
	if err != nil {
		return err
	}
	if destination.ResolvedRegionID == "" {
		return errors.New("delivery address did not include a resolved region id")
	}
	cfg.Defaults.DeliveryDestinationID = destination.DeliveryDestinationID
	cfg.Defaults.RegionID = destination.ResolvedRegionID
	cfg.Defaults.PostalCode = destination.PostalCode
	cfg.Defaults.MarketSet = true
	if err := config.Save(cfg); err != nil {
		return err
	}
	res := struct {
		Market  MarketStatus                `json:"market"`
		Address alcampo.DeliveryDestination `json:"address"`
		Message string                      `json:"message"`
	}{Market: marketStatus(cfg), Address: destination, Message: "saved delivery address as CLI market context"}
	if *jsonOut {
		return output.JSON(stdout, res)
	}
	fmt.Fprintf(stdout, "saved delivery_destination=%s region=%s postal=%s\n", destination.DeliveryDestinationID, destination.ResolvedRegionID, destination.PostalCode)
	return nil
}

type MarketStatus struct {
	MarketSet             bool   `json:"market_set"`
	PostalCode            string `json:"postal_code,omitempty"`
	RegionID              string `json:"region_id,omitempty"`
	RetailerRegionID      string `json:"retailer_region_id,omitempty"`
	RegionName            string `json:"region_name,omitempty"`
	DeliveryDestinationID string `json:"delivery_destination_id,omitempty"`
}

func marketStatus(cfg *config.Config) MarketStatus {
	return MarketStatus{
		MarketSet:             cfg.Defaults.MarketSet,
		PostalCode:            cfg.Defaults.PostalCode,
		RegionID:              cfg.Defaults.RegionID,
		RetailerRegionID:      cfg.Defaults.RetailerRegionID,
		RegionName:            cfg.Defaults.RegionName,
		DeliveryDestinationID: cfg.Defaults.DeliveryDestinationID,
	}
}

func runSetPostal(args []string, stdout io.Writer) error {
	fs := newFlagSet("set-postal", io.Discard)
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("set-postal requires one postal code")
	}
	postal := strings.TrimSpace(fs.Arg(0))
	if postal == "" {
		return errors.New("postal code cannot be empty")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.Defaults.PostalCode = postal
	if err := config.Save(cfg); err != nil {
		return err
	}
	res := struct {
		PostalCode        string `json:"postal_code"`
		RegionID          string `json:"region_id"`
		RegionName        string `json:"region_name"`
		ResolverAvailable bool   `json:"resolver_available"`
		Message           string `json:"message"`
	}{
		PostalCode:        cfg.Defaults.PostalCode,
		RegionID:          cfg.Defaults.RegionID,
		RegionName:        cfg.Defaults.RegionName,
		ResolverAvailable: false,
		Message:           "saved postal code; anonymous postal-to-region resolver is blocked outside a full web session, so searches still require set-market or --store",
	}
	if *jsonOut {
		return output.JSON(stdout, res)
	}
	fmt.Fprintf(stdout, "saved postal_code=%s\n", res.PostalCode)
	fmt.Fprintln(stdout, "anonymous postal-to-region resolution is blocked outside a full web session; run set-market or pass --store/--market before searching.")
	return nil
}
