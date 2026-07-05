package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"alcampo-cli/internal/alcampo"
	"alcampo-cli/internal/auth"
	"alcampo-cli/internal/basket"
	"alcampo-cli/internal/config"
	"alcampo-cli/internal/food"
	"alcampo-cli/internal/money"
	"alcampo-cli/internal/output"

	"golang.org/x/term"
)

func Run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printHelp(stdout)
		return nil
	}
	cmd := args[0]
	args = args[1:]
	switch cmd {
	case "search":
		return runSearch(args, stdout, stderr)
	case "product":
		return runProduct(args, stdout, stderr)
	case "categories":
		return runCategories(args, stdout, stderr)
	case "batch":
		return runBatch(args, stdout, stderr)
	case "total":
		return runTotal(args, stdout, stderr)
	case "market":
		return runMarket(args, stdout, stderr)
	case "set-market":
		return runSetMarket(args, stdout, stderr)
	case "addresses":
		return runAddresses(args, stdout, stderr)
	case "set-address":
		return runSetAddress(args, stdout, stderr)
	case "set-postal":
		return runSetPostal(args, stdout)
	case "import-har":
		return runImportHAR(args, stdout, stderr)
	case "import-curl":
		return runImportCurl(args, stdout, stderr)
	case "login":
		return runLogin(args, stdout, stderr)
	case "whoami":
		return runWhoami(args, stdout, stderr)
	case "cart":
		return runCart(args, stdout, stderr)
	case "checkout":
		return runCheckout(args, stdout, stderr)
	case "food":
		return runFood(args, stdout, stderr)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func runSearch(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("search", stderr)
	limit := fs.Int("limit", 10, "maximum products to return")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	postal := fs.String("postal", "", "postal code hint; anonymous resolver is not verified")
	store := marketFlag(fs, "Alcampo region/store UUID to use for pricing")
	sort := fs.String("sort", "", "site sort option id")
	fresh := fs.Bool("fresh", false, "locally filter likely fresh-food categories")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true, "fresh": true}); err != nil {
		return err
	}
	query := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if query == "" {
		return errors.New("search requires a query")
	}
	cfg, client, err := newClient(*store)
	if err != nil {
		return err
	}
	regionID, err := resolveMarket(cfg, *store)
	if err != nil {
		return err
	}
	warnLocation(stderr, cfg, *postal, *store)
	products, err := client.Search(context.Background(), query, alcampo.SearchOptions{
		Limit:    *limit,
		RegionID: regionID,
		Sort:     *sort,
		Fresh:    *fresh,
	})
	if err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, products)
	}
	for _, p := range products {
		printProductLine(stdout, p)
	}
	return nil
}

func runProduct(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("product", stderr)
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	store := marketFlag(fs, "Alcampo region/store UUID to use when merging listing data")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	ref := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if ref == "" {
		return errors.New("product requires an id, sku, URL, or slug")
	}
	cfg, client, err := newClient(*store)
	if err != nil {
		return err
	}
	regionID, err := resolveMarket(cfg, *store)
	if err != nil {
		return err
	}
	client.RegionID = regionID
	p, err := client.Product(context.Background(), ref)
	if err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, p)
	}
	printProductDetail(stdout, p)
	return nil
}

func runCategories(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("categories", stderr)
	id := fs.String("id", "", "category id, retailer id, slug, or path to list")
	limit := fs.Int("limit", 20, "maximum products to return with --id")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	store := marketFlag(fs, "Alcampo region/store UUID to use for products")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	cfg, client, err := newClient(*store)
	if err != nil {
		return err
	}
	cats, err := client.Categories(context.Background(), 3)
	if err != nil {
		return err
	}
	if *id == "" {
		if *jsonOut {
			return output.JSON(stdout, cats)
		}
		printCategories(stdout, cats, 0)
		return nil
	}
	cat, ok := alcampo.FindCategory(cats, *id)
	if !ok {
		return fmt.Errorf("category %q not found", *id)
	}
	regionID, err := resolveMarket(cfg, *store)
	if err != nil {
		return err
	}
	products, err := client.CategoryProducts(context.Background(), cat, alcampo.CategoryProductsOptions{Limit: *limit, RegionID: regionID})
	if err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, struct {
			Category alcampo.Category  `json:"category"`
			Products []alcampo.Product `json:"products"`
		}{Category: cat, Products: products})
	}
	fmt.Fprintf(stdout, "%s (%s)\n", cat.Name, firstNonEmpty(cat.RetailerID, cat.ID))
	for _, p := range products {
		printProductLine(stdout, p)
	}
	return nil
}

func runBatch(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("batch", stderr)
	file := fs.String("file", "", "file with one query per line, or - for stdin")
	fs.StringVar(file, "f", "", "file with one query per line, or - for stdin")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	store := marketFlag(fs, "Alcampo region/store UUID to use for pricing")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("batch requires -f <file|->")
	}
	cfg, client, err := newClient(*store)
	if err != nil {
		return err
	}
	regionID, err := resolveMarket(cfg, *store)
	if err != nil {
		return err
	}
	r, closeFn, err := openInput(*file)
	if err != nil {
		return err
	}
	defer closeFn()
	results := []BatchResult{}
	scanner := bufio.NewScanner(r)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		query := strings.TrimSpace(stripComment(scanner.Text()))
		if query == "" {
			continue
		}
		res := BatchResult{LineNo: lineNo, Query: query}
		products, err := client.Search(context.Background(), query, alcampo.SearchOptions{Limit: 1, RegionID: regionID})
		if err != nil {
			res.Error = err.Error()
		} else if len(products) == 0 {
			res.Error = "not found"
		} else {
			p := products[0]
			res.Product = &p
		}
		results = append(results, res)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, results)
	}
	for _, res := range results {
		if res.Error != "" {
			fmt.Fprintf(stdout, "%s\tERROR\t%s\n", res.Query, res.Error)
			continue
		}
		fmt.Fprintf(stdout, "%s\t", res.Query)
		printProductLine(stdout, *res.Product)
	}
	return nil
}

type BatchResult struct {
	LineNo  int              `json:"line_no"`
	Query   string           `json:"query"`
	Product *alcampo.Product `json:"product,omitempty"`
	Error   string           `json:"error,omitempty"`
}

func runTotal(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("total", stderr)
	file := fs.String("file", "", "basket file, or - for stdin")
	fs.StringVar(file, "f", "", "basket file, or - for stdin")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	maxEUR := fs.String("max", "", "optional spending guard in EUR")
	store := marketFlag(fs, "Alcampo region/store UUID to use for pricing")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("total requires -f <basket-file|->")
	}
	cfg, client, err := newClient(*store)
	if err != nil {
		return err
	}
	regionID, err := resolveMarket(cfg, *store)
	if err != nil {
		return err
	}
	client.RegionID = regionID
	r, closeFn, err := openInput(*file)
	if err != nil {
		return err
	}
	defer closeFn()
	lines, err := basket.Parse(r)
	if err != nil {
		return err
	}
	result := TotalResult{Complete: true, Currency: "EUR"}
	for _, line := range lines {
		item := TotalLine{LineNo: line.LineNo, Ref: line.Ref, Qty: line.Qty, Comment: line.Comment}
		p, err := client.Lookup(context.Background(), line.Ref)
		if err != nil {
			item.Error = err.Error()
			result.Complete = false
			result.Lines = append(result.Lines, item)
			continue
		}
		item.Product = &p
		if p.Price.Amount == "" {
			item.Error = "price unavailable"
			result.Complete = false
			result.Lines = append(result.Lines, item)
			continue
		}
		lineCents, err := money.MultiplyCentsByQuantity(p.Price.Cents, line.Qty)
		if err != nil {
			item.Error = err.Error()
			result.Complete = false
			result.Lines = append(result.Lines, item)
			continue
		}
		item.LineTotal = money.Money{Amount: money.FormatAmount(lineCents), Currency: "EUR", Cents: lineCents}
		result.TotalCents += lineCents
		result.Lines = append(result.Lines, item)
	}
	result.Total = money.Money{Amount: money.FormatAmount(result.TotalCents), Currency: "EUR", Cents: result.TotalCents}
	if err := enforceMax(result.TotalCents, *maxEUR, cfg); err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, result)
	}
	for _, line := range result.Lines {
		if line.Error != "" {
			fmt.Fprintf(stdout, "line %d\t%s x %s\tERROR\t%s\n", line.LineNo, line.Ref, line.Qty, line.Error)
			continue
		}
		fmt.Fprintf(stdout, "line %d\t%s x %s\t%s\t%s\n", line.LineNo, line.Product.SKU, line.Qty, money.Format(line.LineTotal.Cents, "EUR"), line.Product.Name)
	}
	fmt.Fprintf(stdout, "total\t%s\tcomplete=%t\n", money.Format(result.TotalCents, "EUR"), result.Complete)
	return nil
}

type TotalResult struct {
	Complete   bool        `json:"complete"`
	Currency   string      `json:"currency"`
	TotalCents int64       `json:"total_cents"`
	Total      money.Money `json:"total"`
	Lines      []TotalLine `json:"lines"`
}

type TotalLine struct {
	LineNo    int              `json:"line_no"`
	Ref       string           `json:"ref"`
	Qty       string           `json:"qty"`
	Comment   string           `json:"comment,omitempty"`
	Product   *alcampo.Product `json:"product,omitempty"`
	LineTotal money.Money      `json:"line_total,omitempty"`
	Error     string           `json:"error,omitempty"`
}

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
		res.RegionID, res.RegionName, res.RetailerRegionID, firstNonEmpty(res.DeliveryDestinationID, "-"))
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
			firstNonEmpty(a.ResolvedRegionID, "-"),
			firstNonEmpty(a.PostalCode, "-"),
			a.IsPrimary,
			firstNonEmpty(a.Name, a.FormattedAddress, "-"),
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
		res.HasCookie, res.HasBearer, res.HasCSRF, res.HasCustomerID, res.HasVisitorID, firstNonEmpty(res.RegionID, "-"), firstNonEmpty(res.DeliveryDestinationID, "-"))
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
		session.Cookie != "", session.BearerToken != "", session.CSRFToken != "", cfg.Defaults.RegionID, firstNonEmpty(cfg.Defaults.DeliveryDestinationID, "-"))
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
	fmt.Fprintf(stdout, "market_set=%t region=%s (%s) postal=%s delivery_destination=%s\n", res.MarketSet, res.RegionID, res.RegionName, res.PostalCode, firstNonEmpty(res.DeliveryID, "-"))
	return nil
}

func runCart(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "usage: alcampo cart <get|add|set|set-many|clear> [options]")
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
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	_, client, err := newClient("")
	if err != nil {
		return err
	}
	cart, err := client.Cart(context.Background(), true)
	if err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, cart)
	}
	total, ok := alcampo.CartTotalCents(cart)
	if ok {
		fmt.Fprintf(stdout, "items=%d\ttotal=%s\n", alcampo.CartItemCount(cart), money.Format(total, "EUR"))
		return nil
	}
	fmt.Fprintf(stdout, "items=%d\ttotal=unknown\n", alcampo.CartItemCount(cart))
	return nil
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
		return fmt.Errorf("product %s has no verified price; refusing cart write", firstNonEmpty(product.SKU, product.ID))
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
		fmt.Fprintf(stdout, "no change\t%s\tqty=%s\ttotal=%s\n", firstNonEmpty(product.SKU, product.ID), result.RequestedQuantity, formatMoney(result.CartTotalBefore))
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
		firstNonEmpty(product.SKU, product.ID),
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

func runFood(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printFoodHelp(stdout)
		return nil
	}
	switch args[0] {
	case "profile":
		return runFoodProfile(args[1:], stdout, stderr)
	case "pantry":
		return runFoodPantry(args[1:], stdout, stderr)
	case "plan":
		return runFoodPlan(args[1:], stdout, stderr)
	case "recipe":
		return runFoodRecipe(args[1:], stdout, stderr)
	case "shop":
		return runFoodShop(args[1:], stdout, stderr)
	case "pdf":
		return runFoodPDF(args[1:], stdout, stderr)
	case "run":
		return runFoodRun(args[1:], stdout, stderr)
	case "cook":
		return runFoodCook(args[1:], stdout, stderr)
	case "receive":
		return runFoodReceive(args[1:], stdout, stderr)
	case "history":
		return runFoodHistory(args[1:], stdout, stderr)
	case "staples":
		return runFoodStaples(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown food command %q", args[0])
	}
}

func runFoodProfile(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "usage: alcampo food profile <get|set> [options]")
		return nil
	}
	switch args[0] {
	case "get":
		fs := newFlagSet("food profile get", stderr)
		jsonOut := fs.Bool("json", false, "write JSON to stdout")
		if err := parseInterspersed(fs, args[1:], map[string]bool{"json": true}); err != nil {
			return err
		}
		profile, err := food.LoadProfile()
		if err != nil {
			return err
		}
		if *jsonOut {
			return output.JSON(stdout, profile)
		}
		printFoodProfile(stdout, profile)
		return nil
	case "set":
		return runFoodProfileSet(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown food profile command %q", args[0])
	}
}

func runFoodProfileSet(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food profile set", stderr)
	people := fs.Int("people", -1, "household people count")
	policy := fs.String("selection-policy", "", "balanced, cheapest, or quality")
	budget := fs.String("budget", "", "default budget in EUR")
	diets := fs.String("diets", "", "comma-separated diets")
	allergies := fs.String("allergies", "", "comma-separated allergies")
	dislikes := fs.String("dislikes", "", "comma-separated disliked ingredients")
	likedCuisines := fs.String("liked-cuisines", "", "comma-separated liked cuisines")
	likedRecipes := fs.String("liked-recipes", "", "comma-separated liked recipes")
	rejectedRecipes := fs.String("rejected-recipes", "", "comma-separated rejected recipe ids or titles")
	likedProducts := fs.String("liked-products", "", "comma-separated liked product SKUs, ids, EANs, brands, or names")
	rejectedProducts := fs.String("rejected-products", "", "comma-separated rejected product SKUs, ids, EANs, brands, or names")
	preferredBrands := fs.String("preferred-brands", "", "comma-separated preferred brands")
	rejectedBrands := fs.String("rejected-brands", "", "comma-separated rejected brands")
	nutritionGoals := fs.String("nutrition-goals", "", "comma-separated nutrition goals")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	profile, err := food.LoadProfile()
	if err != nil {
		return err
	}
	if *people >= 0 {
		profile.People = *people
	}
	if *policy != "" {
		normalized, ok := food.NormalizeSelectionPolicy(*policy)
		if !ok || normalized == "" {
			return fmt.Errorf("unknown selection policy %q", *policy)
		}
		profile.SelectionPolicy = normalized
	}
	if *budget != "" {
		profile.BudgetEUR = *budget
	}
	if *diets != "" {
		profile.Diets = splitList(*diets)
	}
	if *allergies != "" {
		profile.Allergies = splitList(*allergies)
	}
	if *dislikes != "" {
		profile.Dislikes = splitList(*dislikes)
	}
	if *likedCuisines != "" {
		profile.LikedCuisines = splitList(*likedCuisines)
	}
	if *likedRecipes != "" {
		profile.LikedRecipes = splitList(*likedRecipes)
	}
	if *rejectedRecipes != "" {
		profile.RejectedRecipes = splitList(*rejectedRecipes)
	}
	if *likedProducts != "" {
		profile.LikedProducts = splitList(*likedProducts)
	}
	if *rejectedProducts != "" {
		profile.RejectedProducts = splitList(*rejectedProducts)
	}
	if *preferredBrands != "" {
		profile.PreferredBrands = splitList(*preferredBrands)
	}
	if *rejectedBrands != "" {
		profile.RejectedBrands = splitList(*rejectedBrands)
	}
	if *nutritionGoals != "" {
		profile.NutritionGoals = splitList(*nutritionGoals)
	}
	if err := food.SaveProfile(profile); err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, profile)
	}
	printFoodProfile(stdout, profile)
	return nil
}

func runFoodPantry(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "usage: alcampo food pantry <list|add|use|update> [options]")
		return nil
	}
	switch args[0] {
	case "list":
		fs := newFlagSet("food pantry list", stderr)
		jsonOut := fs.Bool("json", false, "write JSON to stdout")
		if err := parseInterspersed(fs, args[1:], map[string]bool{"json": true}); err != nil {
			return err
		}
		pantry, err := food.LoadPantry()
		if err != nil {
			return err
		}
		if *jsonOut {
			return output.JSON(stdout, pantry)
		}
		printPantry(stdout, pantry)
		return nil
	case "add", "update":
		return runFoodPantrySet(args[0], args[1:], stdout, stderr)
	case "use":
		return runFoodPantryUse(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown food pantry command %q", args[0])
	}
}

func runFoodPantrySet(action string, args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food pantry "+action, stderr)
	itemName := fs.String("item", "", "item name")
	qty := fs.Float64("qty", 1, "quantity")
	unit := fs.String("unit", "", "unit")
	location := fs.String("location", "", "pantry, fridge, or freezer")
	expiry := fs.String("expiry", "", "expiry date YYYY-MM-DD")
	confidence := fs.Float64("confidence", 1, "confidence from 0 to 1")
	notes := fs.String("notes", "", "free-text notes")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if *itemName == "" && fs.NArg() > 0 {
		*itemName = strings.Join(fs.Args(), " ")
	}
	pantry, err := food.LoadPantry()
	if err != nil {
		return err
	}
	item := food.PantryItem{
		Name:       *itemName,
		Quantity:   *qty,
		Unit:       *unit,
		Location:   *location,
		ExpiryDate: *expiry,
		Confidence: *confidence,
		Notes:      *notes,
	}
	var saved food.PantryItem
	if action == "update" {
		pantry, saved, err = food.SetPantryItem(pantry, item)
	} else {
		pantry, saved, err = food.AddOrUpdatePantryItem(pantry, item)
	}
	if err != nil {
		return err
	}
	if err := food.SavePantry(pantry); err != nil {
		return err
	}
	res := struct {
		Action string          `json:"action"`
		Item   food.PantryItem `json:"item"`
		Pantry food.Pantry     `json:"pantry"`
	}{Action: action, Item: saved, Pantry: pantry}
	if *jsonOut {
		return output.JSON(stdout, res)
	}
	fmt.Fprintf(stdout, "%s\t%s\t%.3g %s\t%s\n", action, saved.Name, saved.Quantity, saved.Unit, firstNonEmpty(saved.Location, "-"))
	return nil
}

func runFoodPantryUse(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food pantry use", stderr)
	itemName := fs.String("item", "", "item name")
	qty := fs.Float64("qty", 1, "quantity to use")
	unit := fs.String("unit", "", "unit")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if *itemName == "" && fs.NArg() > 0 {
		*itemName = strings.Join(fs.Args(), " ")
	}
	pantry, err := food.LoadPantry()
	if err != nil {
		return err
	}
	pantry, used, err := food.UsePantryItem(pantry, *itemName, *qty, *unit)
	if err != nil {
		return err
	}
	if err := food.SavePantry(pantry); err != nil {
		return err
	}
	res := struct {
		Action string          `json:"action"`
		Item   food.PantryItem `json:"item"`
		Pantry food.Pantry     `json:"pantry"`
	}{Action: "use", Item: used, Pantry: pantry}
	if *jsonOut {
		return output.JSON(stdout, res)
	}
	fmt.Fprintf(stdout, "use\t%s\tremaining=%.3g %s\n", used.Name, used.Quantity, used.Unit)
	return nil
}

func runFoodPlan(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food plan", stderr)
	days := fs.Int("days", 3, "number of days")
	people := fs.Int("people", 0, "people count; defaults to profile")
	budget := fs.String("budget", "", "budget in EUR")
	meals := fs.String("meals", "dinner", "comma-separated meal types")
	outPath := fs.String("out", "", "optional output JSON path")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	profile, err := food.LoadProfile()
	if err != nil {
		return err
	}
	pantry, err := food.LoadPantry()
	if err != nil {
		return err
	}
	plan, err := food.GenerateMealPlan(profile, pantry, food.PlanOptions{
		Days:      *days,
		People:    *people,
		BudgetEUR: *budget,
		MealTypes: splitList(*meals),
	})
	if err != nil {
		return err
	}
	if *outPath != "" {
		plan.File = *outPath
		if err := writeJSONPath(*outPath, plan); err != nil {
			return err
		}
	} else {
		plan, err = food.SaveMealPlan(plan)
		if err != nil {
			return err
		}
	}
	if *jsonOut {
		return output.JSON(stdout, plan)
	}
	printMealPlan(stdout, plan)
	return nil
}

func runFoodRecipe(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food recipe", stderr)
	people := fs.Int("people", 0, "people count; defaults to profile")
	outPath := fs.String("out", "", "optional output JSON path")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	prompt := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if prompt == "" {
		return errors.New("food recipe requires a prompt or mealplan id/path")
	}
	profile, err := food.LoadProfile()
	if err != nil {
		return err
	}
	if plan, err := food.LoadMealPlan(prompt); err == nil {
		recipes := recipesFromPlan(plan)
		res := struct {
			MealPlanID string        `json:"mealplan_id"`
			Recipes    []food.Recipe `json:"recipes"`
		}{MealPlanID: plan.ID, Recipes: recipes}
		if *outPath != "" {
			if err := writeJSONPath(*outPath, res); err != nil {
				return err
			}
		}
		if *jsonOut {
			return output.JSON(stdout, res)
		}
		for _, recipe := range recipes {
			printRecipe(stdout, recipe)
		}
		return nil
	}
	recipe := food.GenerateRecipe(prompt, profile, *people)
	if *outPath != "" {
		if err := writeJSONPath(*outPath, recipe); err != nil {
			return err
		}
	}
	if *jsonOut {
		return output.JSON(stdout, recipe)
	}
	printRecipe(stdout, recipe)
	return nil
}

func runFoodShop(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food shop", stderr)
	policy := fs.String("selection-policy", "", "balanced, cheapest, or quality; saved if provided")
	limit := fs.Int("limit", 8, "candidate products per ingredient")
	outPath := fs.String("out", "", "optional output JSON path")
	basketOut := fs.String("basket-out", "", "optional basket file path for guarded cart set-many")
	store := marketFlag(fs, "Alcampo region/store UUID to use for pricing")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("food shop requires a mealplan JSON path or saved mealplan id")
	}
	profile, err := food.LoadProfile()
	if err != nil {
		return err
	}
	resolvedPolicy, updatedProfile, err := resolveFoodSelectionPolicy(&profile, *policy, stderr)
	if err != nil {
		return err
	}
	if updatedProfile {
		if err := food.SaveProfile(profile); err != nil {
			return err
		}
	}
	plan, err := food.LoadMealPlan(fs.Arg(0))
	if err != nil {
		return err
	}
	cfg, client, err := newClient(*store)
	if err != nil {
		return err
	}
	regionID, err := resolveMarket(cfg, *store)
	if err != nil {
		return err
	}
	client.RegionID = regionID
	result, err := food.ShopMealPlan(context.Background(), client, plan, profile, food.ShopOptions{Policy: resolvedPolicy, Limit: *limit})
	if err != nil {
		return err
	}
	if *outPath != "" {
		if err := writeJSONPath(*outPath, result); err != nil {
			return err
		}
	}
	if *basketOut != "" {
		if err := writeBasketLinesPath(*basketOut, result.BasketLines); err != nil {
			return err
		}
	}
	if *jsonOut {
		return output.JSON(stdout, result)
	}
	printShopResult(stdout, result)
	if *basketOut != "" {
		fmt.Fprintf(stdout, "basket\t%s\n", *basketOut)
	}
	return nil
}

func runFoodPDF(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food pdf", stderr)
	outPath := fs.String("out", "", "output PDF path")
	if err := parseInterspersed(fs, args, nil); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("food pdf requires a recipe, mealplan, or shop JSON file")
	}
	input := fs.Arg(0)
	if *outPath == "" {
		ext := filepath.Ext(input)
		if ext == "" {
			*outPath = input + ".pdf"
		} else {
			*outPath = strings.TrimSuffix(input, ext) + ".pdf"
		}
	}
	if err := food.WritePDFFromJSONFile(input, *outPath); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "pdf\t%s\n", *outPath)
	return nil
}

func runFoodRun(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food run", stderr)
	days := fs.Int("days", 3, "number of days")
	people := fs.Int("people", 0, "people count; defaults to profile")
	budget := fs.String("budget", "", "budget in EUR")
	meals := fs.String("meals", "dinner", "comma-separated meal types")
	policy := fs.String("selection-policy", "", "balanced, cheapest, or quality; saved if provided")
	limit := fs.Int("limit", 8, "candidate products per ingredient")
	planOut := fs.String("plan-out", "", "optional mealplan JSON path")
	shopOut := fs.String("shop-out", "", "optional shopping JSON path")
	basketOut := fs.String("basket-out", "", "optional basket file path for guarded cart set-many")
	pdfOut := fs.String("pdf-out", "", "optional shopping PDF path")
	store := marketFlag(fs, "Alcampo region/store UUID to use for pricing")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	profile, err := food.LoadProfile()
	if err != nil {
		return err
	}
	resolvedPolicy, updatedProfile, err := resolveFoodSelectionPolicy(&profile, *policy, stderr)
	if err != nil {
		return err
	}
	if updatedProfile {
		if err := food.SaveProfile(profile); err != nil {
			return err
		}
	}
	pantry, err := food.LoadPantry()
	if err != nil {
		return err
	}
	plan, err := food.GenerateMealPlan(profile, pantry, food.PlanOptions{
		Days:      *days,
		People:    *people,
		BudgetEUR: *budget,
		MealTypes: splitList(*meals),
	})
	if err != nil {
		return err
	}
	if *planOut != "" {
		plan.File = *planOut
		if err := writeJSONPath(*planOut, plan); err != nil {
			return err
		}
	} else {
		plan, err = food.SaveMealPlan(plan)
		if err != nil {
			return err
		}
	}
	cfg, client, err := newClient(*store)
	if err != nil {
		return err
	}
	regionID, err := resolveMarket(cfg, *store)
	if err != nil {
		return err
	}
	client.RegionID = regionID
	shop, err := food.ShopMealPlan(context.Background(), client, plan, profile, food.ShopOptions{Policy: resolvedPolicy, Limit: *limit})
	if err != nil {
		return err
	}
	if *shopOut != "" {
		if err := writeJSONPath(*shopOut, shop); err != nil {
			return err
		}
	}
	if *basketOut != "" {
		if err := writeBasketLinesPath(*basketOut, shop.BasketLines); err != nil {
			return err
		}
	}
	pdfPath := ""
	if *pdfOut != "" {
		source := *shopOut
		if source == "" {
			dir, err := food.MealPlansDir()
			if err != nil {
				return err
			}
			source = filepath.Join(dir, plan.ID+"-shop.json")
			if err := writeJSONPath(source, shop); err != nil {
				return err
			}
		}
		if err := food.WritePDFFromJSONFile(source, *pdfOut); err != nil {
			return err
		}
		pdfPath = *pdfOut
	}
	res := struct {
		MealPlan food.MealPlan   `json:"mealplan"`
		Shop     food.ShopResult `json:"shop"`
		PDF      string          `json:"pdf,omitempty"`
		Basket   string          `json:"basket,omitempty"`
	}{MealPlan: plan, Shop: shop, PDF: pdfPath, Basket: *basketOut}
	if *jsonOut {
		return output.JSON(stdout, res)
	}
	printMealPlan(stdout, plan)
	printShopResult(stdout, shop)
	if pdfPath != "" {
		fmt.Fprintf(stdout, "pdf\t%s\n", pdfPath)
	}
	if *basketOut != "" {
		fmt.Fprintf(stdout, "basket\t%s\n", *basketOut)
	}
	return nil
}

func runFoodCook(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food cook", stderr)
	note := fs.String("note", "", "optional cooking note")
	rating := fs.Int("rating", 0, "optional 1-5 rating")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("food cook requires a mealplan JSON path or saved mealplan id")
	}
	if err := food.ValidateRating(*rating); err != nil {
		return err
	}
	plan, err := food.LoadMealPlan(fs.Arg(0))
	if err != nil {
		return err
	}
	pantry, err := food.LoadPantry()
	if err != nil {
		return err
	}
	pantry, result := food.CookMealPlan(plan, pantry, *note, *rating)
	if err := food.SavePantry(pantry); err != nil {
		return err
	}
	if err := food.AppendHistory(food.HistoryEventForCook(result)); err != nil {
		return err
	}
	if *rating >= 4 || (*rating > 0 && *rating <= 2) {
		profile, err := food.LoadProfile()
		if err != nil {
			return err
		}
		likedBefore := len(profile.LikedRecipes)
		rejectedBefore := len(profile.RejectedRecipes)
		for _, recipe := range recipesFromPlan(plan) {
			key := firstNonEmpty(recipe.ID, recipe.Title)
			if *rating >= 4 {
				profile.LikedRecipes = appendUnique(profile.LikedRecipes, key)
				profile.RejectedRecipes = removeStringValue(profile.RejectedRecipes, key)
			} else {
				profile.RejectedRecipes = appendUnique(profile.RejectedRecipes, key)
				profile.LikedRecipes = removeStringValue(profile.LikedRecipes, key)
			}
		}
		if len(profile.LikedRecipes) != likedBefore || len(profile.RejectedRecipes) != rejectedBefore {
			if err := food.SaveProfile(profile); err != nil {
				return err
			}
		}
	}
	if *jsonOut {
		return output.JSON(stdout, result)
	}
	fmt.Fprintf(stdout, "cooked\t%s\tapplied=%d\tmissing=%d\n", firstNonEmpty(result.MealPlanID, "-"), len(result.Applied), len(result.Missing))
	for _, usage := range result.Applied {
		fmt.Fprintf(stdout, "  used\t%s\t%.3g %s\t%s\n", usage.PantryItem, usage.Quantity, usage.Unit, firstNonEmpty(usage.Location, "-"))
	}
	for _, usage := range result.Missing {
		fmt.Fprintf(stdout, "  missing\t%s\t%.3g %s\n", usage.PantryItem, usage.Quantity, usage.Unit)
	}
	return nil
}

func runFoodReceive(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food receive", stderr)
	location := fs.String("location", "", "override storage location for all received items")
	expiry := fs.String("expiry", "", "optional expiry date YYYY-MM-DD to apply to received items")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("food receive requires a shop JSON path or food run JSON path")
	}
	shop, err := food.LoadShopResult(fs.Arg(0))
	if err != nil {
		return err
	}
	pantry, err := food.LoadPantry()
	if err != nil {
		return err
	}
	pantry, result := food.ReceiveShopResult(shop, pantry, food.ReceiveOptions{
		Location:   *location,
		ExpiryDate: *expiry,
	})
	if err := food.SavePantry(pantry); err != nil {
		return err
	}
	if err := food.AppendHistory(food.HistoryEventForReceive(result)); err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, result)
	}
	fmt.Fprintf(stdout, "received\t%s\tapplied=%d\tskipped=%d\n", firstNonEmpty(result.MealPlanID, "-"), len(result.Applied), len(result.Skipped))
	for _, item := range result.Applied {
		fmt.Fprintf(stdout, "  pantry\t%s\t%.3g %s\t%s\n", item.PantryItem.Name, item.PantryItem.Quantity, item.PantryItem.Unit, firstNonEmpty(item.PantryItem.Location, "-"))
	}
	for _, skipped := range result.Skipped {
		fmt.Fprintf(stdout, "  skipped\t%s\t%s\n", skipped.Ingredient.Name, skipped.Reason)
	}
	return nil
}

func runFoodHistory(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "usage: alcampo food history list [--limit N] [--json]")
		return nil
	}
	if args[0] != "list" {
		return fmt.Errorf("unknown food history command %q", args[0])
	}
	fs := newFlagSet("food history list", stderr)
	limit := fs.Int("limit", 20, "maximum history events to return")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args[1:], map[string]bool{"json": true}); err != nil {
		return err
	}
	events, err := food.LoadHistory(*limit)
	if err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, events)
	}
	if len(events) == 0 {
		fmt.Fprintln(stdout, "history is empty")
		return nil
	}
	for _, event := range events {
		fmt.Fprintf(stdout, "%s\t%s\n", firstNonEmpty(stringMapValue(event, "type"), "event"), firstNonEmpty(stringMapValue(event, "cooked_at"), stringMapValue(event, "created_at"), "-"))
	}
	return nil
}

func runFoodStaples(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "usage: alcampo food staples <list|add|remove> [options]")
		return nil
	}
	switch args[0] {
	case "list":
		fs := newFlagSet("food staples list", stderr)
		jsonOut := fs.Bool("json", false, "write JSON to stdout")
		if err := parseInterspersed(fs, args[1:], map[string]bool{"json": true}); err != nil {
			return err
		}
		profile, err := food.LoadProfile()
		if err != nil {
			return err
		}
		if *jsonOut {
			return output.JSON(stdout, profile.Staples)
		}
		printStaples(stdout, profile.Staples)
		return nil
	case "add":
		return runFoodStaplesAdd(args[1:], stdout, stderr)
	case "remove":
		return runFoodStaplesRemove(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown food staples command %q", args[0])
	}
}

func runFoodStaplesAdd(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food staples add", stderr)
	name := fs.String("item", "", "staple item name")
	minQty := fs.Float64("min", 0, "minimum quantity to keep stocked")
	unit := fs.String("unit", "", "unit")
	searchTerm := fs.String("search", "", "Alcampo search term")
	category := fs.String("category", "staple", "category label")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if *name == "" && fs.NArg() > 0 {
		*name = strings.Join(fs.Args(), " ")
	}
	profile, err := food.LoadProfile()
	if err != nil {
		return err
	}
	profile, staple, err := food.AddOrUpdateStaple(profile, food.Staple{
		Name:       *name,
		MinQty:     *minQty,
		Unit:       *unit,
		SearchTerm: *searchTerm,
		Category:   *category,
	})
	if err != nil {
		return err
	}
	if err := food.SaveProfile(profile); err != nil {
		return err
	}
	res := struct {
		Action  string        `json:"action"`
		Staple  food.Staple   `json:"staple"`
		Staples []food.Staple `json:"staples"`
	}{Action: "add", Staple: staple, Staples: profile.Staples}
	if *jsonOut {
		return output.JSON(stdout, res)
	}
	fmt.Fprintf(stdout, "staple\t%s\tmin=%.3g %s\tsearch=%s\n", staple.Name, staple.MinQty, staple.Unit, firstNonEmpty(staple.SearchTerm, "-"))
	return nil
}

func runFoodStaplesRemove(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food staples remove", stderr)
	name := fs.String("item", "", "staple item name")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if *name == "" && fs.NArg() > 0 {
		*name = strings.Join(fs.Args(), " ")
	}
	profile, err := food.LoadProfile()
	if err != nil {
		return err
	}
	profile, removed, err := food.RemoveStaple(profile, *name)
	if err != nil {
		return err
	}
	if err := food.SaveProfile(profile); err != nil {
		return err
	}
	res := struct {
		Action  string        `json:"action"`
		Removed food.Staple   `json:"removed"`
		Staples []food.Staple `json:"staples"`
	}{Action: "remove", Removed: removed, Staples: profile.Staples}
	if *jsonOut {
		return output.JSON(stdout, res)
	}
	fmt.Fprintf(stdout, "removed-staple\t%s\n", removed.Name)
	return nil
}

func resolveFoodSelectionPolicy(profile *food.Profile, explicit string, stderr io.Writer) (string, bool, error) {
	if explicit != "" {
		normalized, ok := food.NormalizeSelectionPolicy(explicit)
		if !ok || normalized == "" {
			return "", false, fmt.Errorf("unknown selection policy %q", explicit)
		}
		profile.SelectionPolicy = normalized
		return normalized, true, nil
	}
	if profile.SelectionPolicy != "" {
		normalized, ok := food.NormalizeSelectionPolicy(profile.SelectionPolicy)
		if ok && normalized != "" {
			return normalized, false, nil
		}
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(stderr, "Selection policy (balanced, cheapest, quality): ")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", false, err
		}
		normalized, ok := food.NormalizeSelectionPolicy(line)
		if !ok || normalized == "" {
			return "", false, fmt.Errorf("unknown selection policy %q", strings.TrimSpace(line))
		}
		profile.SelectionPolicy = normalized
		return normalized, true, nil
	}
	return "", false, errors.New("selection policy is not set; run 'alcampo food profile set --selection-policy balanced|cheapest|quality' or pass --selection-policy")
}

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

func readJSONInput(path string) (any, error) {
	r, closeFn, err := openInput(path)
	if err != nil {
		return nil, err
	}
	defer closeFn()
	var v any
	dec := json.NewDecoder(r)
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

func parseNonNegativeQuantity(raw string) (*big.Rat, error) {
	q, err := parseSignedQuantity(raw)
	if err != nil {
		return nil, err
	}
	if q.Sign() < 0 {
		return nil, fmt.Errorf("quantity must be non-negative: %q", raw)
	}
	return q, nil
}

func parseSignedQuantity(raw string) (*big.Rat, error) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, ",", "."))
	if raw == "" {
		return nil, errors.New("empty quantity")
	}
	q, ok := new(big.Rat).SetString(raw)
	if !ok {
		return nil, fmt.Errorf("invalid quantity %q", raw)
	}
	return q, nil
}

func ratJSONNumber(q *big.Rat) json.Number {
	return json.Number(formatRat(q))
}

func formatRat(q *big.Rat) string {
	if q == nil {
		return "0"
	}
	s := q.FloatString(3)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-0" {
		return "0"
	}
	return s
}

func multiplyCentsRat(cents int64, q *big.Rat) int64 {
	total := new(big.Rat).Mul(new(big.Rat).SetInt64(cents), q)
	return roundRatToInt(total)
}

func roundRatToInt(r *big.Rat) int64 {
	num := new(big.Int).Set(r.Num())
	den := new(big.Int).Set(r.Denom())
	sign := num.Sign()
	if sign < 0 {
		num.Abs(num)
	}
	q, rem := new(big.Int), new(big.Int)
	q.QuoRem(num, den, rem)
	rem.Mul(rem, big.NewInt(2))
	if rem.Cmp(den) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if sign < 0 {
		q.Neg(q)
	}
	return q.Int64()
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

func writeJSONPath(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil && filepath.Dir(path) != "." {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return output.JSON(f, v)
}

func writeBasketLinesPath(path string, lines []string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil && filepath.Dir(path) != "." {
		return err
	}
	data := strings.Join(lines, "\n")
	if data != "" {
		data += "\n"
	}
	return os.WriteFile(path, []byte(data), 0o600)
}

func splitList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func appendUnique(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if strings.EqualFold(strings.TrimSpace(existing), value) {
			return values
		}
	}
	return append(values, value)
}

func removeStringValue(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	out := values[:0]
	for _, existing := range values {
		if strings.EqualFold(strings.TrimSpace(existing), value) {
			continue
		}
		out = append(out, existing)
	}
	return out
}

func stringMapValue(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	switch v := m[key].(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	default:
		return ""
	}
}

func recipesFromPlan(plan food.MealPlan) []food.Recipe {
	var recipes []food.Recipe
	seen := map[string]bool{}
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			key := firstNonEmpty(meal.Recipe.ID, meal.Recipe.Title)
			if seen[key] {
				continue
			}
			seen[key] = true
			recipes = append(recipes, meal.Recipe)
		}
	}
	return recipes
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "alcampo - unofficial Alcampo Spain CLI")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  alcampo search <query> [--limit N] [--json] [--postal CP] [--store REGION_ID]")
	fmt.Fprintln(w, "  alcampo product <id-or-sku-or-url> [--json]")
	fmt.Fprintln(w, "  alcampo categories [--id ID_OR_SLUG] [--json]")
	fmt.Fprintln(w, "  alcampo batch -f <file|-> [--json]")
	fmt.Fprintln(w, "  alcampo total -f <basket-file|-> [--json] [--max EUR]")
	fmt.Fprintln(w, "  alcampo market [--json]")
	fmt.Fprintln(w, "  alcampo set-market --region-id <uuid> [--name NAME]")
	fmt.Fprintln(w, "  alcampo addresses [--json]")
	fmt.Fprintln(w, "  alcampo set-address <delivery_destination_id>")
	fmt.Fprintln(w, "  alcampo set-postal <postal_code>")
	fmt.Fprintln(w, "  alcampo import-har --file <har|->")
	fmt.Fprintln(w, "  alcampo import-curl --file <curl-file|->")
	fmt.Fprintln(w, "  alcampo import-curl --clipboard")
	fmt.Fprintln(w, "  alcampo login --username <email> [--password-stdin] [--json]")
	fmt.Fprintln(w, "  alcampo whoami [--json]")
	fmt.Fprintln(w, "  alcampo cart get [--json]")
	fmt.Fprintln(w, "  alcampo cart add <product_id_or_sku> <qty> --max EUR")
	fmt.Fprintln(w, "  alcampo cart set <product_id_or_sku> <qty> --max EUR")
	fmt.Fprintln(w, "  alcampo cart set-many -f <basket-file|-> --max EUR")
	fmt.Fprintln(w, "  alcampo cart clear --yes --max EUR")
	fmt.Fprintln(w, "  alcampo checkout <addresses|slots|create|select-slot|confirm-slot|submit>")
	fmt.Fprintln(w, "  alcampo food <profile|pantry|plan|recipe|shop|pdf|receive>")
}

func printFoodHelp(w io.Writer) {
	fmt.Fprintln(w, "alcampo food - pantry-aware meal planning and shopping")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  alcampo food profile get [--json]")
	fmt.Fprintln(w, "  alcampo food profile set [--people N] [--selection-policy balanced|cheapest|quality]")
	fmt.Fprintln(w, "  alcampo food pantry list [--json]")
	fmt.Fprintln(w, "  alcampo food pantry add <item> --qty N [--unit UNIT] [--location fridge|freezer|pantry]")
	fmt.Fprintln(w, "  alcampo food pantry use <item> --qty N [--unit UNIT]")
	fmt.Fprintln(w, "  alcampo food plan --days N --people N [--json] [--out file.json]")
	fmt.Fprintln(w, "  alcampo food recipe <prompt|mealplan-id> [--json] [--out file.json]")
	fmt.Fprintln(w, "  alcampo food shop <mealplan-id-or-file> [--selection-policy POLICY] [--basket-out basket.txt] [--json]")
	fmt.Fprintln(w, "  alcampo food pdf <recipe-or-plan-or-shop.json> --out file.pdf")
	fmt.Fprintln(w, "  alcampo food run --days N --people N [--selection-policy POLICY] [--basket-out basket.txt] [--json]")
	fmt.Fprintln(w, "  alcampo food cook <mealplan-id-or-file> [--rating 1-5] [--json]")
	fmt.Fprintln(w, "  alcampo food receive <shop-or-run.json> [--location pantry|fridge|freezer] [--json]")
	fmt.Fprintln(w, "  alcampo food history list [--limit N] [--json]")
	fmt.Fprintln(w, "  alcampo food staples <list|add|remove> [--json]")
}

func printFoodProfile(w io.Writer, profile food.Profile) {
	fmt.Fprintf(w, "people=%d\n", profile.People)
	fmt.Fprintf(w, "selection_policy=%s\n", firstNonEmpty(profile.SelectionPolicy, "-"))
	fmt.Fprintf(w, "budget=%s\n", firstNonEmpty(profile.BudgetEUR, "-"))
	printStringList(w, "diets", profile.Diets)
	printStringList(w, "allergies", profile.Allergies)
	printStringList(w, "dislikes", profile.Dislikes)
	printStringList(w, "liked_cuisines", profile.LikedCuisines)
	printStringList(w, "liked_recipes", profile.LikedRecipes)
	printStringList(w, "rejected_recipes", profile.RejectedRecipes)
	printStringList(w, "liked_products", profile.LikedProducts)
	printStringList(w, "rejected_products", profile.RejectedProducts)
	printStringList(w, "preferred_brands", profile.PreferredBrands)
	printStringList(w, "rejected_brands", profile.RejectedBrands)
}

func printStringList(w io.Writer, label string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(w, "%s=%s\n", label, strings.Join(values, ", "))
}

func printPantry(w io.Writer, pantry food.Pantry) {
	if len(pantry.Items) == 0 {
		fmt.Fprintln(w, "pantry is empty")
		return
	}
	for _, item := range pantry.Items {
		fmt.Fprintf(w, "%s\t%.3g %s\t%s\texpiry=%s\tconfidence=%.2g\n",
			item.Name,
			item.Quantity,
			item.Unit,
			firstNonEmpty(item.Location, "-"),
			firstNonEmpty(item.ExpiryDate, "-"),
			item.Confidence,
		)
	}
}

func printStaples(w io.Writer, staples []food.Staple) {
	if len(staples) == 0 {
		fmt.Fprintln(w, "staples are empty")
		return
	}
	for _, staple := range staples {
		fmt.Fprintf(w, "%s\tmin=%.3g %s\tsearch=%s\n",
			staple.Name,
			staple.MinQty,
			staple.Unit,
			firstNonEmpty(staple.SearchTerm, "-"),
		)
	}
}

func printMealPlan(w io.Writer, plan food.MealPlan) {
	fmt.Fprintf(w, "mealplan\t%s\tpeople=%d\tfile=%s\n", plan.ID, plan.People, firstNonEmpty(plan.File, "-"))
	for _, day := range plan.Days {
		fmt.Fprintf(w, "day %d\n", day.Day)
		for _, meal := range day.Meals {
			fmt.Fprintf(w, "  %s\t%s\n", meal.Type, meal.Recipe.Title)
		}
	}
	if len(plan.PantryUsage) > 0 {
		fmt.Fprintln(w, "pantry_used")
		for _, use := range plan.PantryUsage {
			fmt.Fprintf(w, "  %s\t%.3g %s\t%s\n", use.Ingredient, use.Quantity, use.Unit, use.PantryItem)
		}
	}
	if len(plan.RequiredPurchases) > 0 {
		fmt.Fprintln(w, "shopping_list")
		for _, item := range plan.RequiredPurchases {
			fmt.Fprintf(w, "  %s\t%.3g %s\n", item.Name, item.Quantity, item.Unit)
		}
	}
}

func printRecipe(w io.Writer, recipe food.Recipe) {
	fmt.Fprintf(w, "%s\tservings=%d\tprep=%dm\tcook=%dm\n", recipe.Title, recipe.Servings, recipe.PrepMinutes, recipe.CookMinutes)
	fmt.Fprintln(w, "ingredients")
	for _, ing := range recipe.Ingredients {
		fmt.Fprintf(w, "  %s\t%.3g %s\n", ing.Name, ing.Quantity, ing.Unit)
	}
	fmt.Fprintln(w, "steps")
	for _, step := range recipe.Steps {
		fmt.Fprintf(w, "  %d\t%s\n", step.Number, step.Text)
	}
}

func printShopResult(w io.Writer, result food.ShopResult) {
	fmt.Fprintf(w, "shop\tpolicy=%s\tcomplete=%t\testimated_total=%s\n", result.Policy, result.Complete, formatMoney(result.EstimatedTotal))
	for _, selected := range result.SelectedProducts {
		if selected.Error != "" {
			fmt.Fprintf(w, "ERROR\t%s\t%s\n", selected.Ingredient.Name, selected.Error)
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			selected.Ingredient.Name,
			firstNonEmpty(selected.Product.SKU, selected.Product.ID, "-"),
			formatMoney(selected.Product.Price),
			selected.Product.Name,
		)
		if selected.Product.ImageURL != "" {
			fmt.Fprintf(w, "  image\t%s\n", selected.Product.ImageURL)
		}
		if selected.SelectionReason != "" {
			fmt.Fprintf(w, "  reason\t%s\n", selected.SelectionReason)
		}
		if selected.QuantityReason != "" {
			fmt.Fprintf(w, "  quantity\t%s\n", selected.QuantityReason)
		}
	}
	if len(result.ShoppingGroups) > 0 {
		fmt.Fprintln(w, "shopping_groups")
		for _, group := range result.ShoppingGroups {
			fmt.Fprintf(w, "  %s\titems=%d\tsubtotal=%s\n", group.Category, len(group.Items), formatMoney(group.Subtotal))
		}
	}
	if len(result.BasketLines) > 0 {
		fmt.Fprintln(w, "basket")
		for _, line := range result.BasketLines {
			fmt.Fprintf(w, "  %s\n", line)
		}
	}
}

func printSlotsSummary(w io.Writer, root any) {
	slots := alcampo.CollectDeliverySlots(root)
	for i, slot := range slots {
		if i >= 20 {
			fmt.Fprintf(w, "... %d more slots\n", len(slots)-20)
			break
		}
		fmt.Fprintf(w, "%s\t%s\t%s-%s\t%s\tmin=%s\n",
			firstNonEmpty(slot.SlotID, "-"),
			firstNonEmpty(slot.Day, "-"),
			firstNonEmpty(slot.StartTime, "-"),
			firstNonEmpty(slot.EndTime, "-"),
			formatMoney(slot.Price),
			formatMoney(slot.MinimumOrder),
		)
	}
	if len(slots) == 0 {
		fmt.Fprintln(w, "no slots returned")
	}
}

func walkSlots(root any, visit func(day, start, end, amount string)) {
	switch t := root.(type) {
	case map[string]any:
		if day, ok := t["day"].(string); ok {
			if items, ok := t["slots"].([]any); ok {
				for _, item := range items {
					slot, ok := item.(map[string]any)
					if !ok {
						continue
					}
					window, _ := slot["slotWindow"].(map[string]any)
					price, _ := slot["deliveryPrice"].(map[string]any)
					visit(day, stringAny(window["startTime"]), stringAny(window["endTime"]), stringAny(price["amount"]))
				}
			}
		}
		for _, v := range t {
			walkSlots(v, visit)
		}
	case []any:
		for _, v := range t {
			walkSlots(v, visit)
		}
	}
}

func stringAny(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

func marketFlag(fs *flag.FlagSet, usage string) *string {
	v := fs.String("store", "", usage)
	fs.StringVar(v, "market", "", usage)
	return v
}

func parseInterspersed(fs *flag.FlagSet, args []string, boolFlags map[string]bool) error {
	return fs.Parse(reorderArgs(args, boolFlags))
}

func reorderArgs(args []string, boolFlags map[string]bool) []string {
	var flagsPart []string
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if isFlagToken(arg) {
			flagsPart = append(flagsPart, arg)
			name, hasValue := flagName(arg)
			if !hasValue && !boolFlags[name] && i+1 < len(args) {
				flagsPart = append(flagsPart, args[i+1])
				i++
			}
			continue
		}
		positional = append(positional, arg)
	}
	return append(flagsPart, positional...)
}

func isFlagToken(s string) bool {
	return strings.HasPrefix(s, "-") && s != "-" && !strings.HasPrefix(s, "-.")
}

func flagName(s string) (name string, hasValue bool) {
	s = strings.TrimLeft(s, "-")
	if idx := strings.IndexByte(s, '='); idx >= 0 {
		return s[:idx], true
	}
	return s, false
}

func openInput(path string) (io.Reader, func(), error) {
	if path == "-" {
		return os.Stdin, func() {}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, func() {}, err
	}
	return f, func() { _ = f.Close() }, nil
}

func readInputBytes(path string) ([]byte, error) {
	r, closeFn, err := openInput(path)
	if err != nil {
		return nil, err
	}
	defer closeFn()
	return io.ReadAll(r)
}

func readClipboard() ([]byte, error) {
	var candidates [][]string
	switch runtime.GOOS {
	case "darwin":
		candidates = [][]string{{"pbpaste"}}
	case "windows":
		candidates = [][]string{{"powershell", "-NoProfile", "-Command", "Get-Clipboard"}}
	default:
		candidates = [][]string{
			{"wl-paste", "--no-newline"},
			{"xclip", "-selection", "clipboard", "-out"},
			{"xsel", "--clipboard", "--output"},
		}
	}
	var lastErr error
	for _, candidate := range candidates {
		out, err := exec.Command(candidate[0], candidate[1:]...).Output()
		if err == nil && len(strings.TrimSpace(string(out))) > 0 {
			return out, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("could not read clipboard: %w", lastErr)
	}
	return nil, errors.New("clipboard was empty")
}

func stripComment(s string) string {
	if idx := strings.IndexByte(s, '#'); idx >= 0 {
		return s[:idx]
	}
	return s
}

func printProductLine(w io.Writer, p alcampo.Product) {
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
		firstNonEmpty(p.SKU, p.ID, "-"),
		formatMoney(p.Price),
		formatUnitPrice(p),
		formatAvailable(p.Available),
		firstNonEmpty(p.Brand, "-"),
		p.Name,
	)
}

func printProductDetail(w io.Writer, p alcampo.Product) {
	fmt.Fprintf(w, "sku: %s\n", firstNonEmpty(p.SKU, "-"))
	fmt.Fprintf(w, "id: %s\n", firstNonEmpty(p.ID, "-"))
	fmt.Fprintf(w, "name: %s\n", p.Name)
	fmt.Fprintf(w, "brand: %s\n", firstNonEmpty(p.Brand, "-"))
	fmt.Fprintf(w, "price: %s\n", formatMoney(p.Price))
	if p.UnitPrice.Amount != "" {
		fmt.Fprintf(w, "unit_price: %s", formatMoney(p.UnitPrice))
		if p.Unit != "" {
			fmt.Fprintf(w, " / %s", p.Unit)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "available: %s\n", formatAvailable(p.Available))
	if p.Size != "" {
		fmt.Fprintf(w, "size: %s\n", p.Size)
	}
	if p.Category != "" {
		fmt.Fprintf(w, "category: %s\n", p.Category)
	}
	if p.URL != "" {
		fmt.Fprintf(w, "url: %s\n", p.URL)
	}
	if p.Description != "" {
		fmt.Fprintf(w, "description: %s\n", p.Description)
	}
	if p.Ingredients != "" {
		fmt.Fprintf(w, "ingredients: %s\n", p.Ingredients)
	}
	if p.Allergens != "" {
		fmt.Fprintf(w, "allergens: %s\n", p.Allergens)
	}
	if p.Nutrition != "" {
		fmt.Fprintf(w, "nutrition: %s\n", p.Nutrition)
	}
	if p.DetailUnavailable {
		fmt.Fprintf(w, "detail_unavailable: %s\n", p.DetailMessage)
	}
}

func printCategories(w io.Writer, cats []alcampo.Category, depth int) {
	prefix := strings.Repeat("  ", depth)
	for _, cat := range cats {
		fmt.Fprintf(w, "%s%s\t%s\t%s\n", prefix, firstNonEmpty(cat.RetailerID, cat.ID, "-"), cat.Slug, cat.Name)
		printCategories(w, cat.Children, depth+1)
	}
}

func formatMoney(m money.Money) string {
	if m.Amount == "" {
		return "-"
	}
	return m.Amount + " " + firstNonEmpty(m.Currency, "EUR")
}

func formatUnitPrice(p alcampo.Product) string {
	if p.UnitPrice.Amount == "" {
		return "-"
	}
	if p.Unit == "" {
		return formatMoney(p.UnitPrice)
	}
	return formatMoney(p.UnitPrice) + "/" + p.Unit
}

func formatAvailable(v *bool) string {
	if v == nil {
		return "unknown"
	}
	if *v {
		return "available"
	}
	return "unavailable"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
