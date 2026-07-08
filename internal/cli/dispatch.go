package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/wachtermar/carrito/internal/alcampo"
)

func Run(args []string, stdout, stderr io.Writer) error {
	err := run(args, stdout, stderr)
	if errors.Is(err, errHelpRequested) {
		return nil
	}
	var snapshotErr alcampo.SnapshotError
	if errors.As(err, &snapshotErr) {
		return ExitError{Code: alcampo.LiveSnapshotExitBlocked, Err: err}
	}
	return err
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printHelp(stdout)
		return nil
	}
	if args[0] == "--version" || args[0] == "version" {
		return runVersion(stdout)
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
	case "login-web":
		return runLoginWeb(args, stdout, stderr)
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

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "carrito - unofficial Alcampo Spain CLI")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  carrito search <query> [--limit N] [--json] [--postal CP] [--store REGION_ID]")
	fmt.Fprintln(w, "  carrito product <id-or-sku-or-url> [--json]")
	fmt.Fprintln(w, "  carrito categories [--id ID_OR_SLUG] [--json]")
	fmt.Fprintln(w, "  carrito batch -f <file|-> [--json]")
	fmt.Fprintln(w, "  carrito total -f <basket-file|-> [--json] [--max EUR]")
	fmt.Fprintln(w, "  carrito market [--json]")
	fmt.Fprintln(w, "  carrito set-market --region-id <uuid> [--name NAME]")
	fmt.Fprintln(w, "  carrito addresses [--json]")
	fmt.Fprintln(w, "  carrito set-address <delivery_destination_id>")
	fmt.Fprintln(w, "  carrito set-postal <postal_code>")
	fmt.Fprintln(w, "  carrito import-har --file <har|->")
	fmt.Fprintln(w, "  carrito import-curl --file <curl-file|->")
	fmt.Fprintln(w, "  carrito import-curl --clipboard")
	fmt.Fprintln(w, "  carrito login --username <email> [--password-stdin] [--json]")
	fmt.Fprintln(w, "  carrito login-web [--if-needed] [--json] [--no-open]")
	fmt.Fprintln(w, "  carrito whoami [--json]")
	fmt.Fprintln(w, "  carrito version")
	fmt.Fprintln(w, "  carrito cart get [--json] [--raw]")
	fmt.Fprintln(w, "  carrito cart add <product_id_or_sku> <qty> --max EUR")
	fmt.Fprintln(w, "  carrito cart set <product_id_or_sku> <qty> --max EUR")
	fmt.Fprintln(w, "  carrito cart set-many -f <basket-file|-> --max EUR")
	fmt.Fprintln(w, "  carrito cart clear --yes --max EUR")
	fmt.Fprintln(w, "  carrito checkout <addresses|slots|create|select-slot|confirm-slot>")
	fmt.Fprintln(w, "  carrito food <profile|pantry|plan|use-up|recipe|recipes|shop|pdf|run|cook|receive|import-orders|import-receipt|history|staples>")
}

func printFoodHelp(w io.Writer) {
	fmt.Fprintln(w, "carrito food - pantry-aware meal planning and shopping")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  carrito food profile get [--json]")
	fmt.Fprintln(w, "  carrito food profile set [--people N] [--selection-policy balanced|cheapest|quality] [--budget EUR]")
	fmt.Fprintln(w, "  carrito food pantry list [--expiring-days N] [--json]")
	fmt.Fprintln(w, "  carrito food pantry add <item> --qty N [--unit UNIT] [--location fridge|freezer|pantry] [--expiry YYYY-MM-DD]")
	fmt.Fprintln(w, "  carrito food pantry update <item> --qty N [--unit UNIT] [--location fridge|freezer|pantry] [--expiry YYYY-MM-DD]")
	fmt.Fprintln(w, "  carrito food pantry use <item> --qty N [--unit UNIT]")
	fmt.Fprintln(w, "  carrito food plan --days N --people N [--json] [--out file.json]")
	fmt.Fprintln(w, "  carrito food use-up [--expiring-days N] [--limit N] [--json]")
	fmt.Fprintln(w, "  carrito food recipe <prompt|mealplan-id> [--json] [--out file.json]")
	fmt.Fprintln(w, "  carrito food recipes list [--tag TAG] [--query TEXT] [--diet DIET] [--profile] [--limit N] [--json]")
	fmt.Fprintln(w, "  carrito food recipes search <query> [--tag TAG] [--diet DIET] [--profile] [--limit N] [--json]")
	fmt.Fprintln(w, "  carrito food recipes show <id-or-title> [--json]")
	fmt.Fprintln(w, "  carrito food recipes add <file|-> [--json]")
	fmt.Fprintln(w, "  carrito food recipes add <file|-> --from-text [--title TITLE] [--servings N] [--json]")
	fmt.Fprintln(w, "  carrito food recipes remove <id-or-title> [--json]")
	fmt.Fprintln(w, "  carrito food shop <mealplan-id-or-file> [--selection-policy POLICY] [--basket-out basket.txt] [--json]")
	fmt.Fprintln(w, "  carrito food pdf <recipe-or-plan-shop-run.json> --out file.pdf")
	fmt.Fprintln(w, "  carrito food run --days N --people N [--selection-policy POLICY] [--servings N|--adult-servings N --child-servings N --toddler-servings N|--household-profile household.json] [--serving-plan-out serving.json] [--scaled-mealplan-out scaled.json] [--strict-servings|--no-serving-scaling] [--basket-out basket.txt] [--run-out run.json] [--quantity-ledger-out ledger.json] [--nutrition-ledger-out nutrition.json] [--nutrition-mode off|labels|recipe|hybrid] [--require-nutrition-ready] [--min-nutrition-line-coverage R] [--min-nutrition-quantity-coverage R] [--include-purchased-excess-nutrition] [--allow-pantry-profile-nutrition=false] [--allow-builtin-pantry-nutrition] [--intent-file intent.json] [--intent-out intent.json] [--constraint-report-out constraints.json] [--require-intent-ready] [--max-cook-minutes N] [--exclude-ingredient NAME] [--exclude-allergen NAME] [--diet-rule RULE] [--recovery-out recovery.json] [--recipe-swap-out recipe_swap.json] [--basket-optimization-out basket_optimization.json] [--budget-repair-out budget_repair.json] [--budget-deal-out budget_deal.json] [--repair-budget|--no-budget-repair] [--allow-budget-product-switches|--no-budget-product-switches] [--allow-budget-recipe-swap|--no-budget-recipe-swap] [--pantry-profile pantry_profile.json] [--pantry-out pantry.json] [--pantry-consumption-out pantry_consumption.json] [--readiness-out readiness.json] [--manifest-out manifest.json] [--audit-out audit.json] [--audit-mode off|warn|fail] [--record-live-snapshot dir|--replay-live-snapshot dir] [--snapshot-strict] [--snapshot-id id] [--pdf-out file.pdf] [--enrich-products] [--strict-quantity] [--recover-missing|--no-recovery] [--allow-recipe-swap|--no-recipe-swap] [--optimize-basket|--no-basket-optimization] [--basket-objective safe-balanced|lowest-price|lowest-waste] [--default-pantry none|minimal-spanish|mediterranean-basic] [--allow-assumed-pantry|--require-confirmed-pantry] [--require-safe-basket] [--require-cook-ready] [--json]")
	fmt.Fprintln(w, "  carrito food validate-run --run run.json --manifest manifest.json [--audit-out audit.json] [--mode hermes|ci|local|live-smoke] [--audit-mode off|warn|fail] [--pdf file.pdf] [--basket basket.txt] [--json]")
	fmt.Fprintln(w, "  carrito food cook <mealplan-id-or-file> [--rating 1-5] [--json]")
	fmt.Fprintln(w, "  carrito food receive <shop-or-run.json> [--location pantry|fridge|freezer] [--json]")
	fmt.Fprintln(w, "  carrito food import-orders [--limit N] [--since YYYY-MM-DD] [--infer-staples] [--json]")
	fmt.Fprintln(w, "  carrito food import-receipt --file <text|-> [--location pantry|fridge|freezer] [--json]")
	fmt.Fprintln(w, "  carrito food history list [--limit N] [--json]")
	fmt.Fprintln(w, "  carrito food staples list [--json]")
	fmt.Fprintln(w, "  carrito food staples add <item> --min N [--unit UNIT] [--search TERM] [--json]")
	fmt.Fprintln(w, "  carrito food staples remove <item> [--json]")
}
