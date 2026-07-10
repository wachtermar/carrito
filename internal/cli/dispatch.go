package cli

import (
	"errors"
	"fmt"
	"io"
)

func Run(args []string, stdout, stderr io.Writer) error {
	err := run(args, stdout, stderr)
	if errors.Is(err, errHelpRequested) {
		return nil
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
	case "market":
		return runMarket(args, stdout, stderr)
	case "set-market":
		return runSetMarket(args, stdout, stderr)
	case "addresses":
		return runAddresses(args, stdout, stderr)
	case "set-address":
		return runSetAddress(args, stdout, stderr)
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
	case "mealplan":
		return runMealPlan(args, stdout, stderr)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "carrito - unofficial Alcampo Spain CLI")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  carrito search <query> [--limit N] [--json] [--store REGION_ID]")
	fmt.Fprintln(w, "  carrito product <id-or-sku-or-url> [--json]")
	fmt.Fprintln(w, "  carrito market [--json]")
	fmt.Fprintln(w, "  carrito set-market --region-id <uuid> [--name NAME]")
	fmt.Fprintln(w, "  carrito addresses [--json]")
	fmt.Fprintln(w, "  carrito set-address <delivery_destination_id>")
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
	fmt.Fprintln(w, "  carrito mealplan <validate|candidates|build> ...")
}
