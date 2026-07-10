package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/basket"
	"github.com/wachtermar/carrito/internal/money"
	"github.com/wachtermar/carrito/internal/output"
	"github.com/wachtermar/carrito/internal/strutil"
)

func runCart(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "usage: carrito cart <get|add|set|set-many> [options]")
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
	default:
		return alcampo.ErrCartUnsupported
	}
}

type CartMutationResult struct {
	Action                  string       `json:"action"`
	ProductID               string       `json:"product_id,omitempty"`
	SKU                     string       `json:"sku,omitempty"`
	ProductName             string       `json:"product_name,omitempty"`
	RequestedQuantity       string       `json:"requested_quantity,omitempty"`
	CurrentQuantity         string       `json:"current_quantity,omitempty"`
	ActualQuantity          string       `json:"actual_quantity,omitempty"`
	AppliedDelta            string       `json:"applied_delta,omitempty"`
	CartTotalBefore         money.Money  `json:"cart_total_before"`
	EstimatedCartTotalAfter money.Money  `json:"estimated_cart_total_after"`
	CartTotalAfter          *money.Money `json:"cart_total_after,omitempty"`
	Max                     money.Money  `json:"max"`
	Noop                    bool         `json:"noop,omitempty"`
	Verified                bool         `json:"verified"`
}

type CartSetManyLine struct {
	LineNo            int    `json:"line_no"`
	Ref               string `json:"ref"`
	ProductID         string `json:"product_id"`
	SKU               string `json:"sku,omitempty"`
	ProductName       string `json:"product_name"`
	RequestedQuantity string `json:"requested_quantity"`
	CurrentQuantity   string `json:"current_quantity"`
	AppliedDelta      string `json:"applied_delta"`
	ActualQuantity    string `json:"actual_quantity"`
	PreservedExisting bool   `json:"preserved_existing,omitempty"`
}

type CartSetManyResult struct {
	Action                  string            `json:"action"`
	Lines                   []CartSetManyLine `json:"lines"`
	CartTotalBefore         money.Money       `json:"cart_total_before"`
	EstimatedCartTotalAfter money.Money       `json:"estimated_cart_total_after"`
	CartTotalAfter          *money.Money      `json:"cart_total_after,omitempty"`
	Max                     money.Money       `json:"max"`
	Noop                    bool              `json:"noop,omitempty"`
	Verified                bool              `json:"verified"`
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
	return runCartSingleQuantity("add", fs.Arg(0), fs.Arg(1), *store, *maxEUR, *jsonOut, stdout)
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
	return runCartSingleQuantity("set", fs.Arg(0), fs.Arg(1), *store, *maxEUR, *jsonOut, stdout)
}

func runCartSingleQuantity(action, ref, requested, store, maxEUR string, jsonOut bool, stdout io.Writer) error {
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
	if product.Price.Cents < 0 || (product.Price.Currency != "" && !strings.EqualFold(product.Price.Currency, "EUR")) {
		return fmt.Errorf("product %s has an invalid non-EUR or negative price; refusing cart write", strutil.FirstNonEmpty(product.SKU, product.ID))
	}
	if product.Available == nil {
		return fmt.Errorf("product %s has unknown availability; refusing cart write", strutil.FirstNonEmpty(product.SKU, product.ID))
	}
	if !*product.Available {
		return fmt.Errorf("product %s is unavailable; refusing cart write", strutil.FirstNonEmpty(product.SKU, product.ID))
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
	if got, ok, quantityErr := alcampo.CartProductQuantity(cart, product.ID); quantityErr != nil {
		return fmt.Errorf("could not read current cart quantity for product %s: %w", product.ID, quantityErr)
	} else if ok {
		current = got
	}
	currentRat, err := parseSignedQuantity(current)
	if err != nil {
		return fmt.Errorf("could not parse current cart quantity %q: %w", current, err)
	}
	deltaRat := new(big.Rat).Set(requestedRat)
	expectedRat := new(big.Rat).Add(currentRat, requestedRat)
	if action == "set" {
		deltaRat.Sub(requestedRat, currentRat)
		expectedRat.Set(requestedRat)
	}
	deltaCents, err := multiplyCentsRat(product.Price.Cents, deltaRat)
	if err != nil {
		return err
	}
	estimated, err := addCents(before, deltaCents)
	if err != nil {
		return err
	}
	if estimated < 0 {
		estimated = 0
	}
	if err := checkMax(estimated, maxCents); err != nil {
		return err
	}

	result := CartMutationResult{
		Action:                  action,
		ProductID:               product.ID,
		SKU:                     product.SKU,
		ProductName:             product.Name,
		RequestedQuantity:       formatRat(requestedRat),
		CurrentQuantity:         current,
		AppliedDelta:            formatRat(deltaRat),
		CartTotalBefore:         moneyValue(before),
		EstimatedCartTotalAfter: moneyValue(estimated),
		Max:                     moneyValue(maxCents),
	}
	if deltaRat.Sign() == 0 {
		afterCents, actual, _, verifyErr := verifyExactCartState(context.Background(), client, map[string]*big.Rat{product.ID: expectedRat}, maxCents)
		if verifyErr != nil {
			return fmt.Errorf("could not verify unchanged cart: %w", verifyErr)
		}
		result.Noop = true
		result.Verified = true
		result.ActualQuantity = actual[product.ID]
		after := moneyValue(afterCents)
		result.CartTotalAfter = &after
		if jsonOut {
			return output.JSON(stdout, result)
		}
		fmt.Fprintf(stdout, "no change\t%s\tqty=%s\ttotal=%s\n", strutil.FirstNonEmpty(product.SKU, product.ID), result.RequestedQuantity, formatMoney(after))
		return nil
	}

	ctx := context.Background()
	original := map[string]*big.Rat{product.ID: new(big.Rat).Set(currentRat)}
	deltas := map[string]*big.Rat{product.ID: new(big.Rat).Set(deltaRat)}
	response, err := client.ApplyCartQuantity(ctx, []alcampo.CartQuantityChange{{
		ProductID: product.ID,
		Quantity:  ratJSONNumber(deltaRat),
	}})
	if err != nil {
		return reportAmbiguousCartWrite(ctx, client, original, before, fmt.Errorf("cart write failed: %w", err))
	}
	responseConfirmed := cartHasExactQuantities(response, map[string]*big.Rat{product.ID: expectedRat})
	after, actual, observed, err := verifyExactCartState(ctx, client, map[string]*big.Rat{product.ID: expectedRat}, maxCents)
	if err != nil {
		if !responseConfirmed || !readbackConfirmsDeltas(original, deltas, observed) {
			return fmt.Errorf("%w; the write response and persisted cart read-back did not conservatively confirm the full requested change, so automatic reversal is unsafe; review the cart manually", err)
		}
		return recoverCartMutation(ctx, client, original, deltas, err)
	}
	v := moneyValue(after)
	result.CartTotalAfter = &v
	result.ActualQuantity = actual[product.ID]
	result.Verified = true
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
	original := map[string]*big.Rat{}
	expected := map[string]*big.Rat{}
	deltas := map[string]*big.Rat{}
	seenProducts := map[string]int{}
	refs := make([]string, 0, len(lines))
	for _, line := range lines {
		refs = append(refs, line.Ref)
	}
	resolvedProducts := map[string]alcampo.Product{}
	if decorated, decorateErr := client.DecorateProducts(context.Background(), refs); decorateErr == nil {
		for _, product := range decorated {
			if product.ID != "" {
				resolvedProducts[strings.ToLower(product.ID)] = product
			}
			if product.SKU != "" {
				resolvedProducts[strings.ToLower(product.SKU)] = product
			}
		}
	}
	for _, line := range lines {
		product, resolved := resolvedProducts[strings.ToLower(strings.TrimSpace(line.Ref))]
		if !resolved {
			product, err = client.Lookup(context.Background(), line.Ref)
			if err != nil {
				return fmt.Errorf("line %d %q: %w", line.LineNo, line.Ref, err)
			}
		}
		if product.ID == "" {
			return fmt.Errorf("line %d %q: product did not include internal product id required for cart writes", line.LineNo, line.Ref)
		}
		if product.Price.Amount == "" {
			return fmt.Errorf("line %d %q: product has no verified price; refusing cart write", line.LineNo, line.Ref)
		}
		if product.Price.Cents < 0 || (product.Price.Currency != "" && !strings.EqualFold(product.Price.Currency, "EUR")) {
			return fmt.Errorf("line %d %q: product has an invalid non-EUR or negative price; refusing cart write", line.LineNo, line.Ref)
		}
		if product.Available == nil {
			return fmt.Errorf("line %d %q: product availability is unknown; refusing cart write", line.LineNo, line.Ref)
		}
		if !*product.Available {
			return fmt.Errorf("line %d %q: product is unavailable; refusing cart write", line.LineNo, line.Ref)
		}
		productKey := strings.ToLower(strings.TrimSpace(product.ID))
		if previous, exists := seenProducts[productKey]; exists {
			return fmt.Errorf("line %d %q duplicates the product resolved from line %d; consolidate basket lines", line.LineNo, line.Ref, previous)
		}
		seenProducts[productKey] = line.LineNo
		requestedRat, err := parseNonNegativeQuantity(line.Qty)
		if err != nil {
			return fmt.Errorf("line %d %q: %w", line.LineNo, line.Ref, err)
		}
		current := "0"
		if got, ok, quantityErr := alcampo.CartProductQuantity(cart, product.ID); quantityErr != nil {
			return fmt.Errorf("line %d %q: could not read current cart quantity: %w", line.LineNo, line.Ref, quantityErr)
		} else if ok {
			current = got
		}
		currentRat, err := parseSignedQuantity(current)
		if err != nil {
			return fmt.Errorf("line %d %q: could not parse current cart quantity %q: %w", line.LineNo, line.Ref, current, err)
		}
		deltaRat := new(big.Rat).Sub(requestedRat, currentRat)
		preserved := false
		if deltaRat.Sign() < 0 {
			deltaRat.SetInt64(0)
			preserved = true
		}
		original[product.ID] = new(big.Rat).Set(currentRat)
		expectedRat := new(big.Rat).Set(requestedRat)
		if preserved {
			expectedRat.Set(currentRat)
		}
		expected[product.ID] = expectedRat
		deltaCents, valueErr := multiplyCentsRat(product.Price.Cents, deltaRat)
		if valueErr != nil {
			return fmt.Errorf("line %d %q: %w", line.LineNo, line.Ref, valueErr)
		}
		estimated, valueErr = addCents(estimated, deltaCents)
		if valueErr != nil {
			return fmt.Errorf("line %d %q: %w", line.LineNo, line.Ref, valueErr)
		}
		result.Lines = append(result.Lines, CartSetManyLine{
			LineNo:            line.LineNo,
			Ref:               line.Ref,
			ProductID:         product.ID,
			SKU:               product.SKU,
			ProductName:       product.Name,
			RequestedQuantity: formatRat(requestedRat),
			CurrentQuantity:   current,
			AppliedDelta:      formatRat(deltaRat),
			PreservedExisting: preserved,
		})
		if deltaRat.Sign() != 0 {
			changes = append(changes, alcampo.CartQuantityChange{ProductID: product.ID, Quantity: ratJSONNumber(deltaRat)})
			deltas[product.ID] = new(big.Rat).Set(deltaRat)
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
		afterCents, actual, _, verifyErr := verifyExactCartState(context.Background(), client, expected, maxCents)
		if verifyErr != nil {
			return fmt.Errorf("could not verify unchanged cart: %w", verifyErr)
		}
		result.Noop = true
		result.Verified = true
		after := moneyValue(afterCents)
		result.CartTotalAfter = &after
		for index := range result.Lines {
			result.Lines[index].ActualQuantity = actual[result.Lines[index].ProductID]
		}
		if *jsonOut {
			return output.JSON(stdout, result)
		}
		fmt.Fprintf(stdout, "no changes\ttotal=%s\n", formatMoney(after))
		return nil
	}
	ctx := context.Background()
	response, err := client.ApplyCartQuantity(ctx, changes)
	if err != nil {
		return reportAmbiguousCartWrite(ctx, client, original, before, fmt.Errorf("cart write failed: %w", err))
	}
	responseConfirmed := cartHasExactQuantities(response, expected)
	after, actual, observed, err := verifyExactCartState(ctx, client, expected, maxCents)
	if err != nil {
		if !responseConfirmed || !readbackConfirmsDeltas(original, deltas, observed) {
			return fmt.Errorf("%w; the write response and persisted cart read-back did not conservatively confirm the full requested change, so automatic reversal is unsafe; review the cart manually", err)
		}
		return recoverCartMutation(ctx, client, original, deltas, err)
	}
	v := moneyValue(after)
	result.CartTotalAfter = &v
	result.Verified = true
	for index := range result.Lines {
		result.Lines[index].ActualQuantity = actual[result.Lines[index].ProductID]
	}
	if *jsonOut {
		return output.JSON(stdout, result)
	}
	fmt.Fprintf(stdout, "set-many\titems=%d\testimated_total=%s\n", len(changes), formatMoney(result.EstimatedCartTotalAfter))
	return nil
}

func verifyExactCartState(ctx context.Context, client *alcampo.Client, expected map[string]*big.Rat, maxCents int64) (int64, map[string]string, map[string]*big.Rat, error) {
	cart, total, err := verifiedCartTotal(ctx, client)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("could not verify cart after write: %w", err)
	}
	actual := make(map[string]string, len(expected))
	observed := make(map[string]*big.Rat, len(expected))
	for productID := range expected {
		gotText := "0"
		if got, ok, quantityErr := alcampo.CartProductQuantity(cart, productID); quantityErr != nil {
			return total, actual, observed, fmt.Errorf("could not verify quantity for product %s: %w", productID, quantityErr)
		} else if ok {
			gotText = got
		}
		got, parseErr := parseSignedQuantity(gotText)
		if parseErr != nil {
			return total, actual, observed, fmt.Errorf("could not parse verified quantity %q for product %s: %w", gotText, productID, parseErr)
		}
		if got.Sign() < 0 {
			return total, actual, observed, fmt.Errorf("verified quantity for product %s is negative: %s", productID, formatRat(got))
		}
		observed[productID] = got
		actual[productID] = formatRat(got)
	}
	if total > maxCents {
		return total, actual, observed, fmt.Errorf("actual cart total %s exceeds spending guard %s", money.Format(total, "EUR"), money.Format(maxCents, "EUR"))
	}
	for productID, want := range expected {
		got := observed[productID]
		if got.Cmp(want) != 0 {
			return total, actual, observed, fmt.Errorf("cart verification failed for product %s: got quantity %s, want %s", productID, formatRat(got), formatRat(want))
		}
	}
	return total, actual, observed, nil
}

// readbackConfirmsDeltas requires fresh persisted quantities to contain every
// requested directional change. The apply response alone is never sufficient:
// an optimistic or stale response followed by the original quantity must not
// trigger an inverse that removes pre-existing groceries.
func readbackConfirmsDeltas(original, deltas, observed map[string]*big.Rat) bool {
	for productID, delta := range deltas {
		before, beforeOK := original[productID]
		got, gotOK := observed[productID]
		if !beforeOK || !gotOK || delta == nil || got.Sign() < 0 {
			return false
		}
		expected := new(big.Rat).Add(before, delta)
		switch delta.Sign() {
		case 1:
			if got.Cmp(expected) < 0 {
				return false
			}
		case -1:
			if got.Cmp(expected) > 0 {
				return false
			}
		default:
			return false
		}
	}
	return len(deltas) > 0
}

func cartHasExactQuantities(cart any, expected map[string]*big.Rat) bool {
	for productID, want := range expected {
		gotText := "0"
		if got, ok, err := alcampo.CartProductQuantity(cart, productID); err != nil {
			return false
		} else if ok {
			gotText = got
		}
		got, err := parseSignedQuantity(gotText)
		if err != nil || got.Cmp(want) != 0 {
			return false
		}
	}
	return true
}

func reportAmbiguousCartWrite(ctx context.Context, client *alcampo.Client, original map[string]*big.Rat, totalBefore int64, cause error) error {
	cart, total, err := verifiedCartTotal(ctx, client)
	if err == nil && total == totalBefore && cartHasExactQuantities(cart, original) {
		return fmt.Errorf("%w; the cart was reread and remained unchanged", cause)
	}
	if err != nil {
		return fmt.Errorf("%w; the write outcome could not be read back (%v); review the cart manually", cause, err)
	}
	return fmt.Errorf("%w; the write outcome is ambiguous and automatic reversal is unsafe; review the cart manually", cause)
}

func recoverCartMutation(ctx context.Context, client *alcampo.Client, original, deltas map[string]*big.Rat, cause error) error {
	if err := reverseCartDeltas(ctx, client, original, deltas); err != nil {
		return fmt.Errorf("%w; automatic rollback failed: %v; review the cart manually", cause, err)
	}
	return fmt.Errorf("%w; this command's cart change was reversed and verified", cause)
}

func reverseCartDeltas(ctx context.Context, client *alcampo.Client, original, deltas map[string]*big.Rat) error {
	cart, _, err := verifiedCartTotal(ctx, client)
	if err != nil {
		return err
	}
	changes := make([]alcampo.CartQuantityChange, 0, len(deltas))
	expected := make(map[string]*big.Rat, len(deltas))
	for productID, appliedDelta := range deltas {
		gotText := "0"
		if got, ok, quantityErr := alcampo.CartProductQuantity(cart, productID); quantityErr != nil {
			return fmt.Errorf("read current quantity for %s: %w", productID, quantityErr)
		} else if ok {
			gotText = got
		}
		got, parseErr := parseSignedQuantity(gotText)
		if parseErr != nil {
			return fmt.Errorf("parse current quantity for %s: %w", productID, parseErr)
		}
		before, ok := original[productID]
		if !ok {
			return fmt.Errorf("missing original quantity for %s", productID)
		}
		forwardExpected := new(big.Rat).Add(before, appliedDelta)
		if (appliedDelta.Sign() > 0 && got.Cmp(forwardExpected) < 0) || (appliedDelta.Sign() < 0 && got.Cmp(forwardExpected) > 0) {
			return fmt.Errorf("persisted quantity for %s no longer conservatively contains the confirmed delta", productID)
		}
		want := new(big.Rat).Sub(got, appliedDelta)
		if want.Sign() < 0 {
			want.SetInt64(0)
		}
		expected[productID] = want
		inverse := new(big.Rat).Neg(appliedDelta)
		changes = append(changes, alcampo.CartQuantityChange{ProductID: productID, Quantity: ratJSONNumber(inverse)})
	}
	response, err := client.ApplyCartQuantity(ctx, changes)
	if err != nil {
		return err
	}
	if !cartHasExactQuantities(response, expected) {
		return errors.New("rollback response did not confirm the inverse quantities")
	}
	restoredCart, _, err := verifiedCartTotal(ctx, client)
	if err != nil {
		return err
	}
	if !cartHasExactQuantities(restoredCart, expected) {
		return errors.New("inverse quantities were not stable on read-back")
	}
	return nil
}
