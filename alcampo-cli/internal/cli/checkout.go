package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"alcampo-cli/internal/alcampo"
	"alcampo-cli/internal/money"
	"alcampo-cli/internal/output"
	"alcampo-cli/internal/strutil"
)

func runCheckout(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "usage: alcampo checkout <addresses|slots|create|select-slot|confirm-slot|submit> [options]")
		return nil
	}
	switch args[0] {
	case "addresses":
		return runAddresses(args[1:], stdout, stderr)
	case "slots":
		return runCheckoutSlots(args[1:], stdout, stderr)
	case "create":
		return runCheckoutCreate(args[1:], stdout, stderr)
	case "select-slot", "reserve-slot":
		return runCheckoutSelectSlot(args[1:], stdout, stderr)
	case "confirm-slot":
		return runCheckoutConfirmSlot(args[1:], stdout, stderr)
	case "submit":
		return alcampo.ErrCheckoutUnsupported
	default:
		return alcampo.ErrCheckoutUnsupported
	}
}

func runCheckoutSlots(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("checkout slots", stderr)
	addressID := fs.String("address", "", "delivery destination id; defaults to configured address")
	days := fs.Int("days", 7, "number of days to request")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	cfg, client, err := newClient("")
	if err != nil {
		return err
	}
	if *addressID == "" {
		*addressID = cfg.Defaults.DeliveryDestinationID
	}
	if *addressID == "" {
		return errors.New("checkout slots requires --address or a configured delivery destination from set-address")
	}
	regionID, err := resolveMarket(cfg, "")
	if err != nil {
		return err
	}
	slots, err := client.DeliverySlots(context.Background(), *addressID, regionID, *days)
	if err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, slots)
	}
	printSlotsSummary(stdout, slots)
	return nil
}

func runCheckoutCreate(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("checkout create", stderr)
	maxEUR := fs.String("max", "", "required spending guard in EUR")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	cfg, client, err := newClient("")
	if err != nil {
		return err
	}
	if err := requireWriteSession(cfg); err != nil {
		return err
	}
	maxCents, err := requiredMaxCents(*maxEUR, cfg)
	if err != nil {
		return err
	}
	_, before, err := verifiedCartTotal(context.Background(), client)
	if err != nil {
		return err
	}
	if err := checkMax(before, maxCents); err != nil {
		return err
	}
	response, err := client.CheckoutStart(context.Background(), "Alcampo Vans")
	if err != nil {
		return err
	}
	result := struct {
		Action    string      `json:"action"`
		CartTotal money.Money `json:"cart_total"`
		Max       money.Money `json:"max"`
		Response  any         `json:"response,omitempty"`
	}{Action: "create", CartTotal: moneyValue(before), Max: moneyValue(maxCents), Response: response}
	if *jsonOut {
		return output.JSON(stdout, result)
	}
	fmt.Fprintf(stdout, "checkout-started\tcart_total=%s\n", formatMoney(result.CartTotal))
	return nil
}

func runCheckoutSelectSlot(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("checkout select-slot", stderr)
	slotID := fs.String("slot", "", "slot id returned by checkout slots")
	addressID := fs.String("address", "", "delivery destination id; defaults to configured address")
	days := fs.Int("days", 7, "number of days to request while verifying the slot")
	maxEUR := fs.String("max", "", "required spending guard in EUR")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if *slotID == "" && fs.NArg() > 0 {
		*slotID = fs.Arg(0)
	}
	*slotID = strings.TrimSpace(*slotID)
	if *slotID == "" {
		return errors.New("checkout select-slot requires --slot <slot_id>")
	}
	cfg, client, err := newClient("")
	if err != nil {
		return err
	}
	if err := requireWriteSession(cfg); err != nil {
		return err
	}
	if *addressID == "" {
		*addressID = cfg.Defaults.DeliveryDestinationID
	}
	if *addressID == "" {
		return errors.New("checkout select-slot requires --address or a configured delivery destination from set-address")
	}
	regionID, err := resolveMarket(cfg, "")
	if err != nil {
		return err
	}
	maxCents, err := requiredMaxCents(*maxEUR, cfg)
	if err != nil {
		return err
	}
	_, cartTotal, err := verifiedCartTotal(context.Background(), client)
	if err != nil {
		return err
	}
	slots, err := client.DeliverySlots(context.Background(), *addressID, regionID, *days)
	if err != nil {
		return err
	}
	slot, ok := alcampo.FindDeliverySlot(slots, *slotID)
	if !ok {
		return fmt.Errorf("slot %q was not returned for address %s and region %s", *slotID, *addressID, regionID)
	}
	estimated := cartTotal
	if slot.Price.Amount != "" {
		estimated += slot.Price.Cents
	}
	if err := checkMax(estimated, maxCents); err != nil {
		return err
	}
	response, err := client.ReserveSlot(context.Background(), alcampo.SlotReservationRequest{
		RegionID:              regionID,
		SlotID:                *slotID,
		DeliveryDestinationID: *addressID,
	})
	if err != nil {
		return err
	}
	result := struct {
		Action                 string               `json:"action"`
		AddressID              string               `json:"address_id"`
		RegionID               string               `json:"region_id"`
		Slot                   alcampo.DeliverySlot `json:"slot"`
		CartTotal              money.Money          `json:"cart_total"`
		EstimatedTotalWithSlot money.Money          `json:"estimated_total_with_slot"`
		Max                    money.Money          `json:"max"`
		Response               any                  `json:"response,omitempty"`
	}{Action: "select-slot", AddressID: *addressID, RegionID: regionID, Slot: slot, CartTotal: moneyValue(cartTotal), EstimatedTotalWithSlot: moneyValue(estimated), Max: moneyValue(maxCents), Response: response}
	if *jsonOut {
		return output.JSON(stdout, result)
	}
	fmt.Fprintf(stdout, "selected-slot\t%s\t%s %s-%s\testimated_total=%s\n", slot.SlotID, slot.Day, slot.StartTime, slot.EndTime, formatMoney(result.EstimatedTotalWithSlot))
	return nil
}

func runCheckoutConfirmSlot(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("checkout confirm-slot", stderr)
	file := fs.String("file", "", "JSON body from a prior slot reservation confirmationData object, or - for stdin")
	fs.StringVar(file, "f", "", "JSON body from a prior slot reservation confirmationData object, or - for stdin")
	maxEUR := fs.String("max", "", "required spending guard in EUR")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("checkout confirm-slot requires -f <json-file|->")
	}
	cfg, client, err := newClient("")
	if err != nil {
		return err
	}
	if err := requireWriteSession(cfg); err != nil {
		return err
	}
	maxCents, err := requiredMaxCents(*maxEUR, cfg)
	if err != nil {
		return err
	}
	_, cartTotal, err := verifiedCartTotal(context.Background(), client)
	if err != nil {
		return err
	}
	if err := checkMax(cartTotal, maxCents); err != nil {
		return err
	}
	body, err := readJSONInput(*file)
	if err != nil {
		return err
	}
	response, err := client.ConfirmSlot(context.Background(), body)
	if err != nil {
		return err
	}
	result := struct {
		Action    string      `json:"action"`
		CartTotal money.Money `json:"cart_total"`
		Max       money.Money `json:"max"`
		Response  any         `json:"response,omitempty"`
	}{Action: "confirm-slot", CartTotal: moneyValue(cartTotal), Max: moneyValue(maxCents), Response: response}
	if *jsonOut {
		return output.JSON(stdout, result)
	}
	fmt.Fprintf(stdout, "confirmed-slot\tcart_total=%s\n", formatMoney(result.CartTotal))
	return nil
}

func printSlotsSummary(w io.Writer, root any) {
	slots := alcampo.CollectDeliverySlots(root)
	for i, slot := range slots {
		if i >= 20 {
			fmt.Fprintf(w, "... %d more slots\n", len(slots)-20)
			break
		}
		fmt.Fprintf(w, "%s\t%s\t%s-%s\t%s\tmin=%s\n",
			strutil.FirstNonEmpty(slot.SlotID, "-"),
			strutil.FirstNonEmpty(slot.Day, "-"),
			strutil.FirstNonEmpty(slot.StartTime, "-"),
			strutil.FirstNonEmpty(slot.EndTime, "-"),
			formatMoney(slot.Price),
			formatMoney(slot.MinimumOrder),
		)
	}
	if len(slots) == 0 {
		fmt.Fprintln(w, "no slots returned")
	}
}
