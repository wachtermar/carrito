package cli

import (
	"fmt"
	"io"
)

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
	case "use-up":
		return runFoodUseUp(args[1:], stdout, stderr)
	case "recipe":
		return runFoodRecipe(args[1:], stdout, stderr)
	case "recipes":
		return runFoodRecipes(args[1:], stdout, stderr)
	case "shop":
		return runFoodShop(args[1:], stdout, stderr)
	case "pdf":
		return runFoodPDF(args[1:], stdout, stderr)
	case "run":
		return runFoodRun(args[1:], stdout, stderr)
	case "validate-run":
		return runFoodValidateRun(args[1:], stdout, stderr)
	case "cook":
		return runFoodCook(args[1:], stdout, stderr)
	case "receive":
		return runFoodReceive(args[1:], stdout, stderr)
	case "import-orders":
		return runFoodImportOrders(args[1:], stdout, stderr)
	case "import-receipt":
		return runFoodImportReceipt(args[1:], stdout, stderr)
	case "history":
		return runFoodHistory(args[1:], stdout, stderr)
	case "staples":
		return runFoodStaples(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown food command %q", args[0])
	}
}
