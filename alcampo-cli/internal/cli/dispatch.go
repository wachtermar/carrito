package cli

import (
	"fmt"
	"io"
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
	fmt.Fprintln(w, "  alcampo login-web [--if-needed] [--json] [--no-open]")
	fmt.Fprintln(w, "  alcampo whoami [--json]")
	fmt.Fprintln(w, "  alcampo cart get [--json] [--raw]")
	fmt.Fprintln(w, "  alcampo cart add <product_id_or_sku> <qty> --max EUR")
	fmt.Fprintln(w, "  alcampo cart set <product_id_or_sku> <qty> --max EUR")
	fmt.Fprintln(w, "  alcampo cart set-many -f <basket-file|-> --max EUR")
	fmt.Fprintln(w, "  alcampo cart clear --yes --max EUR")
	fmt.Fprintln(w, "  alcampo checkout <addresses|slots|create|select-slot|confirm-slot|submit>")
	fmt.Fprintln(w, "  alcampo food <profile|pantry|plan|use-up|recipe|recipes|shop|pdf|run|cook|receive|import-orders|import-receipt|history|staples>")
}

func printFoodHelp(w io.Writer) {
	fmt.Fprintln(w, "alcampo food - pantry-aware meal planning and shopping")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  alcampo food profile get [--json]")
	fmt.Fprintln(w, "  alcampo food profile set [--people N] [--selection-policy balanced|cheapest|quality] [--budget EUR]")
	fmt.Fprintln(w, "  alcampo food pantry list [--expiring-days N] [--json]")
	fmt.Fprintln(w, "  alcampo food pantry add <item> --qty N [--unit UNIT] [--location fridge|freezer|pantry] [--expiry YYYY-MM-DD]")
	fmt.Fprintln(w, "  alcampo food pantry update <item> --qty N [--unit UNIT] [--location fridge|freezer|pantry] [--expiry YYYY-MM-DD]")
	fmt.Fprintln(w, "  alcampo food pantry use <item> --qty N [--unit UNIT]")
	fmt.Fprintln(w, "  alcampo food plan --days N --people N [--json] [--out file.json]")
	fmt.Fprintln(w, "  alcampo food use-up [--expiring-days N] [--limit N] [--json]")
	fmt.Fprintln(w, "  alcampo food recipe <prompt|mealplan-id> [--json] [--out file.json]")
	fmt.Fprintln(w, "  alcampo food recipes list [--tag TAG] [--query TEXT] [--diet DIET] [--profile] [--limit N] [--json]")
	fmt.Fprintln(w, "  alcampo food recipes search <query> [--tag TAG] [--diet DIET] [--profile] [--limit N] [--json]")
	fmt.Fprintln(w, "  alcampo food recipes show <id-or-title> [--json]")
	fmt.Fprintln(w, "  alcampo food recipes add <file|-> [--json]")
	fmt.Fprintln(w, "  alcampo food recipes add <file|-> --from-text [--title TITLE] [--servings N] [--json]")
	fmt.Fprintln(w, "  alcampo food recipes remove <id-or-title> [--json]")
	fmt.Fprintln(w, "  alcampo food shop <mealplan-id-or-file> [--selection-policy POLICY] [--basket-out basket.txt] [--json]")
	fmt.Fprintln(w, "  alcampo food pdf <recipe-or-plan-or-shop.json> --out file.pdf")
	fmt.Fprintln(w, "  alcampo food run --days N --people N [--selection-policy POLICY] [--basket-out basket.txt] [--json]")
	fmt.Fprintln(w, "  alcampo food cook <mealplan-id-or-file> [--rating 1-5] [--json]")
	fmt.Fprintln(w, "  alcampo food receive <shop-or-run.json> [--location pantry|fridge|freezer] [--json]")
	fmt.Fprintln(w, "  alcampo food import-orders [--limit N] [--since YYYY-MM-DD] [--infer-staples] [--json]")
	fmt.Fprintln(w, "  alcampo food import-receipt --file <text|-> [--location pantry|fridge|freezer] [--json]")
	fmt.Fprintln(w, "  alcampo food history list [--limit N] [--json]")
	fmt.Fprintln(w, "  alcampo food staples list [--json]")
	fmt.Fprintln(w, "  alcampo food staples add <item> --min N [--unit UNIT] [--search TERM] [--json]")
	fmt.Fprintln(w, "  alcampo food staples remove <item> [--json]")
}
