package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/basket"
	"github.com/wachtermar/carrito/internal/money"
	"github.com/wachtermar/carrito/internal/output"
	"github.com/wachtermar/carrito/internal/strutil"
)

func runCart(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "usage: carrito cart <get|add|set|set-many|clear> [options]")
		return nil
	}
	switch args[0] {
	case "get":
		return runCartGet(args[1:], stdout, stderr)
	case "add":
		return runCartAdd(args[1:], stdout, stderr)
	case "set":
		return runCartSet(args[1:], stdout, stderr)
	case "set-many":
		return runCartSetMany(args[1:], stdout, stderr)
	case "clear":
		return runCartClear(args[1:], stdout, stderr)
	default:
		return alcampo.ErrCartUnsupported
	}
}

type CartMutationResult struct {
	Action                  string          `json:"action"`
	Product                 alcampo.Product `json:"product,omitempty"`
	RequestedQuantity       string          `json:"requested_quantity,omitempty"`
	CurrentQuantity         string          `json:"current_quantity,omitempty"`
	AppliedDelta            string          `json:"applied_delta,omitempty"`
	CartTotalBefore         money.Money     `json:"cart_total_before"`
	EstimatedCartTotalAfter money.Money     `json:"estimated_cart_total_after"`
	CartTotalAfter          *money.Money    `json:"cart_total_after,omitempty"`
	Max                     money.Money     `json:"max"`
	Noop                    bool            `json:"noop,omitempty"`
	Response                any             `json:"response,omitempty"`
}

type CartSetManyLine struct {
	LineNo            int              `json:"line_no"`
	Ref               string           `json:"ref"`
	Product           *alcampo.Product `json:"product,omitempty"`
	RequestedQuantity string           `json:"requested_quantity"`
	CurrentQuantity   string           `json:"current_quantity"`
	AppliedDelta      string           `json:"applied_delta"`
}

type CartSetManyResult struct {
	Action                  string            `json:"action"`
	Lines                   []CartSetManyLine `json:"lines"`
	CartTotalBefore         money.Money       `json:"cart_total_before"`
	EstimatedCartTotalAfter money.Money       `json:"estimated_cart_total_after"`
	CartTotalAfter          *money.Money      `json:"cart_total_after,omitempty"`
	Max                     money.Money       `json:"max"`
	Noop                    bool              `json:"noop,omitempty"`
	Response                any               `json:"response,omitempty"`
}

func runCartGet(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("cart get", stderr)
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	rawOut := fs.Bool("raw", false, "with --json, write the raw Alcampo cart response")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true, "raw": true}); err != nil {
		return err
	}
	cfg, client, err := newClient("")
	if err != nil {
		return err
	}
	if err := requireReadSession(cfg, "cart reads"); err != nil {
		return err
	}
	cart, err := client.Cart(context.Background(), true)
	if err != nil {
		return err
	}
	if *jsonOut {
		if *rawOut {
			return output.JSON(stdout, cart)
		}
		summary, err := enrichedCartSummary(context.Background(), client, cart)
		if err != nil {
			return err
		}
		return output.JSON(stdout, summary)
	}
	if *rawOut {
		return output.JSON(stdout, cart)
	}
	summary, err := enrichedCartSummary(context.Background(), client, cart)
	if err != nil {
		return err
	}
	printCartSummary(stdout, summary)
	return nil
}

func enrichedCartSummary(ctx context.Context, client *alcampo.Client, cart any) (alcampo.CartSummary, error) {
	summary := alcampo.SummarizeCart(cart, client.BaseURL)
	products, err := client.DecorateProducts(ctx, alcampo.CartSummaryProductIDs(summary))
	if err != nil {
		return summary, err
	}
	alcampo.EnrichCartSummary(&summary, products)
	return summary, nil
}

func runCartAdd(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("cart add", stderr)
	maxEUR := fs.String("max", "", "required spending guard in EUR")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	store := marketFlag(fs, "Alcampo region/store UUID to use for product pricing")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return errors.New("cart add requires <product_id_or_sku> <qty>")
	}
	return runCartSingleQuantity("add", fs.Arg(0), fs.Arg(1), "", *store, *maxEUR, *jsonOut, stdout)
}

func runCartSet(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("cart set", stderr)
	maxEUR := fs.String("max", "", "required spending guard in EUR")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	store := marketFlag(fs, "Alcampo region/store UUID to use for product pricing")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return errors.New("cart set requires <product_id_or_sku> <qty>")
	}
	return runCartSingleQuantity("set", fs.Arg(0), fs.Arg(1), "", *store, *maxEUR, *jsonOut, stdout)
}

func runCartSingleQuantity(action, ref, requested, currentOverride, store, maxEUR string, jsonOut bool, stdout io.Writer) error {
	cfg, client, err := newClient(store)
	if err != nil {
		return err
	}
	if err := requireWriteSession(cfg); err != nil {
		return err
	}
	regionID, err := resolveMarket(cfg, store)
	if err != nil {
		return err
	}
	client.RegionID = regionID
	maxCents, err := requiredMaxCents(maxEUR, cfg)
	if err != nil {
		return err
	}
	product, err := client.Lookup(context.Background(), ref)
	if err != nil {
		return err
	}
	if product.ID == "" {
		return fmt.Errorf("product %q did not include the internal product id required for cart writes", ref)
	}
	if product.Price.Amount == "" {
		return fmt.Errorf("product %s has no verified price; refusing cart write", strutil.FirstNonEmpty(product.SKU, product.ID))
	}
	cart, before, err := verifiedCartTotal(context.Background(), client)
	if err != nil {
		return err
	}

	requestedRat, err := parseNonNegativeQuantity(requested)
	if err != nil {
		return err
	}
	if action == "add" && requestedRat.Sign() <= 0 {
		return errors.New("cart add quantity must be greater than zero")
	}
	current := "0"
	if action == "set" {
		if currentOverride != "" {
			current = currentOverride
		} else if got, ok := alcampo.CartProductQuantity(cart, product.ID); ok {
			current = got
		}
	}
	currentRat, err := parseSignedQuantity(current)
	if err != nil {
		return fmt.Errorf("could not parse current cart quantity %q: %w", current, err)
	}
	deltaRat := new(big.Rat).Set(requestedRat)
	if action == "set" {
		deltaRat.Sub(requestedRat, currentRat)
	}
	deltaCents := multiplyCentsRat(product.Price.Cents, deltaRat)
	estimated := before + deltaCents
	if estimated < 0 {
		estimated = 0
	}
	if err := checkMax(estimated, maxCents); err != nil {
		return err
	}

	result := CartMutationResult{
		Action:                  action,
		Product:                 product,
		RequestedQuantity:       formatRat(requestedRat),
		CurrentQuantity:         current,
		AppliedDelta:            formatRat(deltaRat),
		CartTotalBefore:         moneyValue(before),
		EstimatedCartTotalAfter: moneyValue(estimated),
		Max:                     moneyValue(maxCents),
	}
	if deltaRat.Sign() == 0 {
		result.Noop = true
		if jsonOut {
			return output.JSON(stdout, result)
		}
		fmt.Fprintf(stdout, "no change\t%s\tqty=%s\ttotal=%s\n", strutil.FirstNonEmpty(product.SKU, product.ID), result.RequestedQuantity, formatMoney(result.CartTotalBefore))
		return nil
	}

	response, err := client.ApplyCartQuantity(context.Background(), []alcampo.CartQuantityChange{{
		ProductID: product.ID,
		Quantity:  ratJSONNumber(deltaRat),
	}})
	if err != nil {
		return err
	}
	result.Response = response
	if after, ok := alcampo.CartTotalCents(response); ok {
		v := moneyValue(after)
		result.CartTotalAfter = &v
	}
	if jsonOut {
		return output.JSON(stdout, result)
	}
	fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\testimated_total=%s\n",
		action,
		strutil.FirstNonEmpty(product.SKU, product.ID),
		result.AppliedDelta,
		product.Name,
		formatMoney(result.EstimatedCartTotalAfter),
	)
	return nil
}

func runCartSetMany(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("cart set-many", stderr)
	file := fs.String("file", "", "basket file, or - for stdin")
	fs.StringVar(file, "f", "", "basket file, or - for stdin")
	maxEUR := fs.String("max", "", "required spending guard in EUR")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	store := marketFlag(fs, "Alcampo region/store UUID to use for product pricing")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("cart set-many requires -f <basket-file|->")
	}
	cfg, client, err := newClient(*store)
	if err != nil {
		return err
	}
	if err := requireWriteSession(cfg); err != nil {
		return err
	}
	regionID, err := resolveMarket(cfg, *store)
	if err != nil {
		return err
	}
	client.RegionID = regionID
	maxCents, err := requiredMaxCents(*maxEUR, cfg)
	if err != nil {
		return err
	}
	r, closeFn, err := openInput(*file)
	if err != nil {
		return err
	}
	defer closeFn()
	lines, err := basket.Parse(r)
	if err != nil {
		return err
	}
	cart, before, err := verifiedCartTotal(context.Background(), client)
	if err != nil {
		return err
	}
	result := CartSetManyResult{
		Action:          "set-many",
		CartTotalBefore: moneyValue(before),
		Max:             moneyValue(maxCents),
	}
	var changes []alcampo.CartQuantityChange
	estimated := before
	for _, line := range lines {
		product, err := client.Lookup(context.Background(), line.Ref)
		if err != nil {
			return fmt.Errorf("line %d %q: %w", line.LineNo, line.Ref, err)
		}
		if product.ID == "" {
			return fmt.Errorf("line %d %q: product did not include internal product id required for cart writes", line.LineNo, line.Ref)
		}
		if product.Price.Amount == "" {
			return fmt.Errorf("line %d %q: product has no verified price; refusing cart write", line.LineNo, line.Ref)
		}
		requestedRat, err := parseNonNegativeQuantity(line.Qty)
		if err != nil {
			return fmt.Errorf("line %d %q: %w", line.LineNo, line.Ref, err)
		}
		current := "0"
		if got, ok := alcampo.CartProductQuantity(cart, product.ID); ok {
			current = got
		}
		currentRat, err := parseSignedQuantity(current)
		if err != nil {
			return fmt.Errorf("line %d %q: could not parse current cart quantity %q: %w", line.LineNo, line.Ref, current, err)
		}
		deltaRat := new(big.Rat).Sub(requestedRat, currentRat)
		estimated += multiplyCentsRat(product.Price.Cents, deltaRat)
		p := product
		result.Lines = append(result.Lines, CartSetManyLine{
			LineNo:            line.LineNo,
			Ref:               line.Ref,
			Product:           &p,
			RequestedQuantity: formatRat(requestedRat),
			CurrentQuantity:   current,
			AppliedDelta:      formatRat(deltaRat),
		})
		if deltaRat.Sign() != 0 {
			changes = append(changes, alcampo.CartQuantityChange{ProductID: product.ID, Quantity: ratJSONNumber(deltaRat)})
		}
	}
	if estimated < 0 {
		estimated = 0
	}
	result.EstimatedCartTotalAfter = moneyValue(estimated)
	if err := checkMax(estimated, maxCents); err != nil {
		return err
	}
	if len(changes) == 0 {
		result.Noop = true
		if *jsonOut {
			return output.JSON(stdout, result)
		}
		fmt.Fprintf(stdout, "no changes\ttotal=%s\n", formatMoney(result.CartTotalBefore))
		return nil
	}
	response, err := client.ApplyCartQuantity(context.Background(), changes)
	if err != nil {
		return err
	}
	result.Response = response
	if after, ok := alcampo.CartTotalCents(response); ok {
		v := moneyValue(after)
		result.CartTotalAfter = &v
	}
	if *jsonOut {
		return output.JSON(stdout, result)
	}
	fmt.Fprintf(stdout, "set-many\titems=%d\testimated_total=%s\n", len(changes), formatMoney(result.EstimatedCartTotalAfter))
	return nil
}

func runCartClear(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("cart clear", stderr)
	maxEUR := fs.String("max", "", "required spending guard in EUR")
	yes := fs.Bool("yes", false, "required confirmation for clearing the cart")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"yes": true, "json": true}); err != nil {
		return err
	}
	if !*yes {
		return errors.New("cart clear requires --yes")
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
	response, err := client.ClearCart(context.Background())
	if err != nil {
		return err
	}
	result := struct {
		Action          string      `json:"action"`
		CartTotalBefore money.Money `json:"cart_total_before"`
		Max             money.Money `json:"max"`
		Response        any         `json:"response,omitempty"`
	}{Action: "clear", CartTotalBefore: moneyValue(before), Max: moneyValue(maxCents), Response: response}
	if *jsonOut {
		return output.JSON(stdout, result)
	}
	fmt.Fprintf(stdout, "cleared\tprevious_total=%s\n", formatMoney(result.CartTotalBefore))
	return nil
}
