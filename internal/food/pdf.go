package food

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wachtermar/carrito/internal/money"
	"github.com/wachtermar/carrito/internal/strutil"
)

func WritePDFFromJSONFile(inputPath, outputPath string) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}
	var probe map[string]any
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	var title string
	var lines []string
	switch {
	case probe["mealplan"] != nil && probe["shop"] != nil:
		var artifact FoodRunArtifact
		if err := json.Unmarshal(data, &artifact); err != nil {
			return err
		}
		title, lines = pdfLinesForFoodRunArtifact(artifact)
	case probe["selected_products"] != nil:
		var shop ShopResult
		if err := json.Unmarshal(data, &shop); err != nil {
			return err
		}
		title, lines = pdfLinesForShop(shop)
	case probe["recipes"] != nil:
		var collection struct {
			MealPlanID string   `json:"mealplan_id"`
			Recipes    []Recipe `json:"recipes"`
		}
		if err := json.Unmarshal(data, &collection); err != nil {
			return err
		}
		title, lines = pdfLinesForRecipes(collection.MealPlanID, collection.Recipes)
	case probe["days"] != nil:
		var plan MealPlan
		if err := json.Unmarshal(data, &plan); err != nil {
			return err
		}
		title, lines = pdfLinesForMealPlan(plan)
	case probe["ingredients"] != nil && probe["steps"] != nil:
		var recipe Recipe
		if err := json.Unmarshal(data, &recipe); err != nil {
			return err
		}
		title, lines = pdfLinesForRecipe(recipe)
	default:
		return fmt.Errorf("%s is not a supported food recipe, mealplan, shop, or food-run JSON file", inputPath)
	}
	return writeSimplePDF(outputPath, title, lines, filepath.Dir(inputPath))
}

func WriteSimplePDF(outputPath, title string, lines []string) error {
	return writeSimplePDF(outputPath, title, lines, "")
}

func writeSimplePDF(outputPath, title string, lines []string, imageBaseDir string) error {
	if strings.TrimSpace(outputPath) == "" {
		return fmt.Errorf("output path is required")
	}
	if title == "" {
		title = "Alcampo Food Plan"
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o700); err != nil && filepath.Dir(outputPath) != "." {
		return err
	}
	elements, images := pdfElements(title, lines, imageBaseDir)
	pages := layoutPDFPages(elements)
	if len(pages) == 0 {
		pages = []pdfPage{{Items: []pdfPageItem{{Text: title, FontSize: 18, X: 50, Y: 800}}}}
	}

	var objects [][]byte
	objects = append(objects, nil) // 1 catalog
	objects = append(objects, nil) // 2 pages
	objects = append(objects, []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"))
	imageObjectNums := make(map[string]int, len(images))
	for _, img := range images {
		objNum := len(objects) + 1
		imageObjectNums[img.Name] = objNum
		objects = append(objects, img.Object())
	}
	pageObjectNums := make([]int, 0, len(pages))
	for _, page := range pages {
		contentObj := len(objects) + 1
		content := pdfContentStream(page)
		objects = append(objects, pdfStreamObject([]byte(content)))
		pageObj := len(objects) + 1
		pageObjectNums = append(pageObjectNums, pageObj)
		objects = append(objects, []byte(pdfPageObject(contentObj, imageObjectNums)))
	}
	var kids strings.Builder
	for _, num := range pageObjectNums {
		fmt.Fprintf(&kids, "%d 0 R ", num)
	}
	objects[0] = []byte("<< /Type /Catalog /Pages 2 0 R >>")
	objects[1] = []byte(fmt.Sprintf("<< /Type /Pages /Kids [ %s] /Count %d >>", kids.String(), len(pageObjectNums)))

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for i, obj := range objects {
		num := i + 1
		offsets[num] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n", num)
		buf.Write(obj)
		buf.WriteString("\nendobj\n")
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", len(objects)+1)
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= len(objects); i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return os.WriteFile(outputPath, buf.Bytes(), 0o600)
}

func pdfLinesForMealPlan(plan MealPlan) (string, []string) {
	title := "Meal Plan"
	if plan.ID != "" {
		title = "Meal Plan " + plan.ID
	}
	lines := []string{
		fmt.Sprintf("People: %d", plan.People),
		"Selection policy: " + strutil.FirstNonEmpty(plan.SelectionPolicy, "not set"),
		"Budget: " + strutil.FirstNonEmpty(plan.BudgetEUR, "not set"),
	}
	if plan.Nutrition != nil {
		lines = append(lines, "Nutrition: "+formatNutritionSummary(*plan.Nutrition))
	}
	lines = append(lines, "")
	for _, day := range plan.Days {
		lines = append(lines, fmt.Sprintf("Day %d", day.Day))
		for _, meal := range day.Meals {
			lines = append(lines, strings.ToUpper(meal.Type)+": "+meal.Recipe.Title)
			lines = append(lines, recipeSummaryLines(meal.Recipe)...)
		}
		lines = append(lines, "")
	}
	if len(plan.PantryUsage) > 0 {
		lines = append(lines, "Pantry and fridge used:")
		for _, use := range plan.PantryUsage {
			lines = append(lines, fmt.Sprintf("- %s: %.3g %s from %s", use.Ingredient, use.Quantity, use.Unit, use.PantryItem))
		}
		lines = append(lines, "")
	}
	if len(plan.RequiredPurchases) > 0 {
		lines = append(lines, "Shopping list:")
		for _, item := range plan.RequiredPurchases {
			lines = append(lines, fmt.Sprintf("- %s: %.3g %s", item.Name, item.Quantity, item.Unit))
		}
	}
	return title, lines
}

func pdfLinesForFoodRunArtifact(artifact FoodRunArtifact) (string, []string) {
	title := "Alcampo Meal Plan & Shopping List"
	plan := artifact.MealPlan
	shop := artifact.Shop
	lines := []string{
		"Summary:",
		fmt.Sprintf("People: %d", plan.People),
		fmt.Sprintf("Days: %d", len(plan.Days)),
		"Selection policy: " + strutil.FirstNonEmpty(plan.SelectionPolicy, shop.Policy, "not set"),
		"Budget: " + strutil.FirstNonEmpty(plan.BudgetEUR, "not set"),
		"Shopping status: " + shoppingStatusLabel(shop),
		"Estimated shopping total: " + formatPDFMoney(shop.EstimatedTotal.Amount, shop.EstimatedTotal.Currency),
		"Data caveat: prices, offers, stock, substitutions, and variable-weight totals can change before Alcampo confirms an order.",
		"Safety: no order was submitted; cart and checkout writes require explicit approval and a spending guard.",
	}
	if missing := missingShoppingIngredients(shop); len(missing) > 0 {
		lines = append(lines, "Missing ingredients: "+strings.Join(missing, ", "))
		lines = append(lines, "Estimated total excludes missing or unavailable items.")
	}
	if len(artifact.Meals) > 0 {
		lines = append(lines, "Meals: "+strings.Join(artifact.Meals, ", "))
	}
	if plan.Nutrition != nil {
		lines = append(lines, "Plan nutrition estimate: "+formatNutritionSummary(*plan.Nutrition))
	}
	if artifact.ReadinessGate != nil {
		lines = append(lines, readinessGatePDFLines(*artifact.ReadinessGate)...)
	}
	if artifact.BudgetRepairPlan != nil {
		lines = append(lines, budgetRepairPDFLines(*artifact.BudgetRepairPlan)...)
	}
	if artifact.BudgetDealReport != nil {
		lines = append(lines, budgetDealPDFLines(*artifact.BudgetDealReport)...)
	}
	if artifact.RecipeQualityReport != nil {
		lines = append(lines, recipeQualityPDFLines(*artifact.RecipeQualityReport)...)
	}
	if artifact.NutritionLedger != nil {
		lines = append(lines, nutritionLedgerPDFLines(*artifact.NutritionLedger)...)
	}
	if artifact.ServingPlan != nil {
		lines = append(lines, servingPlanPDFLines(*artifact.ServingPlan, artifact.ScaledMealPlan)...)
	}
	if artifact.PantryResolution != nil {
		lines = append(lines, pantryResolutionPDFLines(*artifact.PantryResolution)...)
	}
	if artifact.RecipeSwapPlan != nil {
		lines = append(lines, recipeSwapPlanPDFLines(*artifact.RecipeSwapPlan)...)
	}
	if artifact.BasketOptimizationPlan != nil {
		lines = append(lines, basketOptimizationPlanPDFLines(*artifact.BasketOptimizationPlan)...)
	}
	if artifact.QuantityLedger != nil {
		lines = append(lines, ledgerTrustPDFLines(*artifact.QuantityLedger, artifact.BasketSafety)...)
	}
	if artifact.RecoveryPlan != nil {
		lines = append(lines, recoveryPlanPDFLines(*artifact.RecoveryPlan)...)
	}
	lines = append(lines, nutritionCoveragePDFLines(shop)...)
	lines = append(lines, "", "Day-by-day meal plan:")
	swapsBySlot := recipeSwapsBySlot(artifact.RecipeSwapPlan)
	for _, day := range plan.Days {
		lines = append(lines, fmt.Sprintf("Day %d", day.Day))
		for _, meal := range day.Meals {
			lines = append(lines, formatMealType(meal.Type)+": "+meal.Recipe.Title)
			if swap, ok := swapsBySlot[recipeSwapSlotKey(day.Day, meal.Type)]; ok && swap.ReplacementRecipeID == meal.Recipe.ID {
				lines = append(lines, "Replaced original recipe: "+swap.OriginalRecipeTitle)
			}
			lines = append(lines, recipeSummaryLines(meal.Recipe)...)
			if meal.PlanningReason != "" {
				lines = append(lines, "Planning reason: "+meal.PlanningReason)
			}
		}
		lines = append(lines, "")
	}
	lines = append(lines, pantryUsagePDFLines(plan)...)
	lines = append(lines, requiredPurchasePDFLines(plan)...)
	if artifact.QuantityLedger != nil {
		lines = append(lines, ledgerAllocationPDFLines(*artifact.QuantityLedger)...)
	}
	lines = append(lines, shopResultPDFLines(shop)...)
	lines = append(lines, "", "Basket safety:")
	if artifact.BasketSafety != nil {
		lines = append(lines, "- Status: "+strings.ToUpper(string(artifact.BasketSafety.Status)))
		lines = append(lines, "- Safe to build basket: "+fmt.Sprintf("%t", artifact.BasketSafety.SafeToBuild))
		if artifact.BasketSafety.Reason != "" {
			lines = append(lines, "- "+artifact.BasketSafety.Reason)
		}
	}
	lines = append(lines, "- No order was submitted.")
	lines = append(lines, "- Cart and checkout writes require explicit approval and a spending guard.")
	if len(artifact.Warnings) > 0 {
		lines = append(lines, "", "Warnings and caveats:")
		for _, warning := range artifact.Warnings {
			lines = append(lines, "- "+warning)
		}
	}
	return title, lines
}

func readinessGatePDFLines(gate ReadinessGate) []string {
	lines := []string{
		"",
		"Basket readiness: " + strings.ToUpper(string(gate.Status)),
		fmt.Sprintf("Safe to build: %t", gate.SafeToBuild),
		"Cooking readiness: " + strings.ToUpper(strutil.FirstNonEmpty(gate.CookReadinessStatus, string(ReadinessReadyExact))),
		fmt.Sprintf("Safe to cook: %t", gate.SafeToCook),
	}
	if gate.NutritionStatus != "" {
		lines = append(lines, "Nutrition readiness: "+strings.ToUpper(gate.NutritionStatus), fmt.Sprintf("Safe to report nutrition: %t", gate.SafeToReportNutrition))
	}
	if gate.RecipeQualityStatus != "" {
		lines = append(lines, "Recipe quality: "+strings.ToUpper(gate.RecipeQualityStatus), fmt.Sprintf("Safe to use recipes: %t", gate.SafeToUseRecipes))
	}
	if gate.BudgetRepairStatus != "" && gate.BudgetRepairStatus != string(BudgetRepairNotRun) {
		lines = append(lines, "Budget repair: "+strings.ToUpper(gate.BudgetRepairStatus))
	}
	if gate.BudgetDealStatus != "" {
		lines = append(lines, "Budget/deal readiness: "+strings.ToUpper(gate.BudgetDealStatus), "Budget status: "+strings.ToUpper(strutil.FirstNonEmpty(gate.BudgetStatus, BudgetStatusNotSet)), fmt.Sprintf("Safe to report budget: %t", gate.SafeToReportBudget), fmt.Sprintf("Safe to report deals: %t", gate.SafeToReportDeals))
	}
	if !gate.SafeToBuild {
		lines = append(lines, "Do not use this basket for shopping.")
	}
	if gate.ExitReason != "" {
		lines = append(lines, "Readiness reason: "+gate.ExitReason)
	}
	if len(gate.BlockingIssues) > 0 {
		lines = append(lines, "Shopping not ready:")
		for _, issue := range gate.BlockingIssues {
			lines = append(lines, "- "+issue.Message)
			if issue.Remediation != "" {
				lines = append(lines, "  Fix: "+issue.Remediation)
			}
		}
	}
	if len(gate.Warnings) > 0 {
		lines = append(lines, "Readiness caveats:")
		for _, issue := range gate.Warnings {
			lines = append(lines, "- "+issue.Message)
		}
	}
	return lines
}

func budgetDealPDFLines(report BudgetDealReport) []string {
	if report.Status == BudgetDealNotRun {
		return nil
	}
	lines := []string{
		"",
		"Budget and deal evidence:",
		"- Status: " + strings.ToUpper(string(report.Status)),
		"- Budget status: " + strings.ToUpper(strutil.FirstNonEmpty(report.BudgetStatus, BudgetStatusNotSet)),
		"- Estimated shopping total: " + formatPDFMoney(report.EstimatedTotal.Amount, report.EstimatedTotal.Currency),
	}
	if report.Summary.SelectedProductCount > 0 {
		lines = append(lines, fmt.Sprintf("- Price coverage: %d of %d selected lines", report.Summary.SelectedProductsWithPrice, report.Summary.SelectedProductCount))
	}
	if report.ConsumedCostEstimate.Cents > 0 || report.PackageExcessCostEstimate.Cents > 0 {
		lines = append(lines, "- Consumed-cost estimate: "+formatPDFMoney(report.ConsumedCostEstimate.Amount, report.ConsumedCostEstimate.Currency))
		lines = append(lines, "- Package-excess estimate: "+formatPDFMoney(report.PackageExcessCostEstimate.Amount, report.PackageExcessCostEstimate.Currency))
	}
	if report.CostBasis != "" {
		lines = append(lines, "- Cost basis: "+strings.ReplaceAll(report.CostBasis, "_", " "))
	}
	if report.Budget != nil {
		lines = append(lines, "- Budget target: "+formatPDFMoney(report.Budget.Amount, report.Budget.Currency))
	}
	if report.Delta != nil {
		lines = append(lines, "- Budget delta: "+formatOptimizationCents(int(report.Delta.Cents)))
	}
	if report.DeltaPercent != 0 {
		lines = append(lines, fmt.Sprintf("- Budget delta percent: %.2f%%", report.DeltaPercent))
	}
	lines = append(lines, fmt.Sprintf("- Offer evidence: %d offer(s), %d applied, %d unclear, %d loyalty-gated", report.Summary.OfferCount, report.Summary.AppliedOfferCount, report.Summary.UnclearOfferCount, report.Summary.LoyaltyOfferCount))
	if report.Summary.RecognizedDealSavingsCents > 0 {
		lines = append(lines, "- Recognized offer savings: "+money.FormatAmount(int64(report.Summary.RecognizedDealSavingsCents)))
	}
	if report.Optimization != nil {
		lines = append(lines, "- Deal-aware optimization: "+strings.ToUpper(string(report.Optimization.Status)))
		lines = append(lines, "- Optimization objective: "+strutil.FirstNonEmpty(report.Optimization.Objective, BasketObjectiveSafeBalanced))
		if report.Optimization.SubtotalDelta.Cents != 0 {
			lines = append(lines, "- Optimization subtotal delta: "+formatOptimizationCents(int(report.Optimization.SubtotalDelta.Cents)))
		}
		if report.Optimization.OfferSavings.Cents > 0 {
			lines = append(lines, "- Optimization offer savings: "+money.FormatAmount(report.Optimization.OfferSavings.Cents))
		}
		lines = append(lines, fmt.Sprintf("- Optimization changed products: %d", report.Optimization.ChangedLines))
	}
	for _, offer := range report.OfferEvidence {
		if offer.Text == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- Offer on %s: %s [%s]", strutil.FirstNonEmpty(offer.ProductName, offer.SKU, "selected product"), offer.Text, strutil.FirstNonEmpty(offer.Status, "unknown")))
		if offer.Warning != "" {
			lines = append(lines, "  Caveat: "+offer.Warning)
		}
	}
	for _, warning := range report.Warnings {
		if warning.Message != "" {
			lines = append(lines, "- Caveat: "+warning.Message)
		}
	}
	return lines
}

func budgetRepairPDFLines(plan BudgetRepairPlan) []string {
	if plan.Status == BudgetRepairNotRun {
		return nil
	}
	lines := []string{
		"",
		"Budget repair actions:",
		"- Status: " + strings.ToUpper(string(plan.Status)),
		"- Initial budget status: " + strings.ToUpper(strutil.FirstNonEmpty(plan.Baseline.BudgetStatus, BudgetStatusNotSet)),
		"- Final budget status: " + strings.ToUpper(strutil.FirstNonEmpty(plan.Final.BudgetStatus, BudgetStatusNotSet)),
	}
	if plan.Baseline.EstimatedBasketSubtotalCents != nil {
		lines = append(lines, "- Initial estimated subtotal: "+money.Format(int64(*plan.Baseline.EstimatedBasketSubtotalCents), "EUR"))
	}
	if plan.Final.EstimatedBasketSubtotalCents != nil {
		lines = append(lines, "- Final estimated subtotal: "+money.Format(int64(*plan.Final.EstimatedBasketSubtotalCents), "EUR"))
	}
	if len(plan.AppliedDecisions) > 0 {
		lines = append(lines, "- Changes:")
		totalSavings := 0
		for _, decision := range plan.AppliedDecisions {
			savings := ""
			if decision.EstimatedSavingsCents != nil && *decision.EstimatedSavingsCents > 0 {
				totalSavings += *decision.EstimatedSavingsCents
				savings = "; estimated saving " + money.Format(int64(*decision.EstimatedSavingsCents), "EUR")
			}
			lines = append(lines, fmt.Sprintf("  - %s -> %s%s", decision.BeforeLabel, decision.AfterLabel, savings))
			if decision.Explanation != "" {
				lines = append(lines, "    Reason: "+decision.Explanation)
			}
		}
		if totalSavings > 0 {
			lines = append(lines, "- Total estimated repair savings: "+money.Format(int64(totalSavings), "EUR"))
		}
	} else if plan.Status == BudgetRepairAttemptedFailed || plan.Status == BudgetRepairSkippedUnrepairable {
		lines = append(lines, "- No validated cheaper product switch met the budget and readiness constraints.")
	}
	lines = append(lines, "- Safety: product-level budget repair may only use validated same-ingredient replacements that preserve basket readiness.")
	if plan.Policy.AllowBudgetRecipeSwap {
		lines = append(lines, "- Recipe budget swaps were allowed by policy, but this PDF only reports swaps recorded in the repair plan.")
	} else {
		lines = append(lines, "- Recipe budget swaps were not allowed; recipes were not changed solely for cost.")
	}
	for _, issue := range plan.BlockingIssues {
		if issue.Message != "" {
			lines = append(lines, "- Repair blocker: "+issue.Message)
		}
	}
	for _, issue := range plan.Warnings {
		if issue.Message != "" {
			lines = append(lines, "- Repair caveat: "+issue.Message)
		}
	}
	return lines
}

func recipeQualityPDFLines(report RecipeQualityReport) []string {
	if report.Status == RecipeQualityNotRun {
		return nil
	}
	lines := []string{
		"",
		"Recipe sources and quality:",
		"- Status: " + strings.ToUpper(report.Status),
		fmt.Sprintf("- Recipes checked: %d", report.Summary.RecipeCount),
		fmt.Sprintf("- Recipes with source: %d", report.Summary.RecipesWithSource),
		fmt.Sprintf("- Recipe images: %d present, %d cached, %d missing, %d broken", report.Summary.RecipesWithImages, report.Summary.CachedImages, report.Summary.MissingImages, report.Summary.BrokenImages),
	}
	if report.RecipeQualityFingerprint != "" {
		lines = append(lines, "- Quality fingerprint: "+report.RecipeQualityFingerprint)
	}
	for _, item := range report.Items {
		source := item.Source.SourceType
		if item.Source.Path != "" {
			source += " " + item.Source.Path
		} else if item.Source.URL != "" {
			source += " " + item.Source.URL
		}
		lines = append(lines, fmt.Sprintf("- Day %d %s %s: %s; source=%s; image=%s", item.Day, item.MealSlot, item.RecipeTitle, strings.ToUpper(item.Status), strutil.FirstNonEmpty(source, "unknown"), strutil.FirstNonEmpty(item.ImageStatus, "not_evaluated")))
		for _, issue := range item.Issues {
			lines = append(lines, "  Blocking: "+issue.Message)
		}
		for _, issue := range item.Warnings {
			lines = append(lines, "  Caveat: "+issue.Message)
		}
	}
	return lines
}

func nutritionLedgerPDFLines(ledger NutritionLedger) []string {
	if ledger.Status == NutritionLedgerNotRun {
		return nil
	}
	lines := []string{
		"",
		"Nutrition evidence:",
		"- Status: " + strings.ToUpper(string(ledger.Status)),
		fmt.Sprintf("- Known nutrition coverage: %d of %d ingredient lines", ledger.Coverage.LinesWithAnyNutrition, ledger.Coverage.IngredientLines),
		fmt.Sprintf("- Alcampo label coverage: %d of %d ingredient lines", ledger.Coverage.LinesWithAlcampoLabel, ledger.Coverage.IngredientLines),
		fmt.Sprintf("- Pantry nutrition coverage: %d pantry line(s); missing for %d pantry-covered line(s)", ledger.Coverage.LinesWithPantryNutrition, ledger.Coverage.PantryMissingLines),
		"- Basis: consumed scaled recipe quantities",
		"- Package excess is not counted as eaten by default.",
	}
	if ledger.Coverage.QuantityCoverageRatio != nil {
		lines = append(lines, fmt.Sprintf("- Quantity coverage: %.0f%%", *ledger.Coverage.QuantityCoverageRatio*100))
	}
	if ledger.TotalConsumedKnown != nil {
		lines = append(lines, "- Known consumed total: "+formatNutritionRollupFacts(*ledger.TotalConsumedKnown))
	}
	for _, meal := range ledger.MealSummaries {
		if meal.PerCookedServingKnown == nil {
			if len(meal.MissingIngredients) > 0 {
				lines = append(lines, fmt.Sprintf("- Day %d %s: nutrition incomplete; missing %s", meal.Day, meal.MealSlot, strings.Join(meal.MissingIngredients, ", ")))
			}
			continue
		}
		lines = append(lines, fmt.Sprintf("- Day %d %s per cooked serving: %s", meal.Day, meal.MealSlot, formatNutritionRollupFacts(*meal.PerCookedServingKnown)))
		if len(meal.MissingIngredients) > 0 {
			lines = append(lines, "  Missing nutrition evidence: "+strings.Join(meal.MissingIngredients, ", "))
		}
	}
	for _, declared := range ledger.RecipeDeclaredSummaries {
		if declared.PerCookedServing == nil {
			continue
		}
		lines = append(lines, fmt.Sprintf("- Recipe-declared nutrition for Day %d %s: %s per serving", declared.Day, declared.MealSlot, formatNutritionRollupFacts(*declared.PerCookedServing)))
	}
	for _, issue := range ledger.BlockingIssues {
		if issue.Message != "" {
			lines = append(lines, "- Nutrition issue: "+issue.Message)
		}
	}
	return lines
}

func formatNutritionRollupFacts(rollup NutritionRollup) string {
	var parts []string
	if rollup.Facts.EnergyKcal != nil {
		parts = append(parts, fmt.Sprintf("%.3g kcal", rollup.Facts.EnergyKcal.Value))
	}
	if rollup.Facts.ProteinG != nil {
		parts = append(parts, fmt.Sprintf("%.3g g protein", rollup.Facts.ProteinG.Value))
	}
	if rollup.Facts.CarbohydrateG != nil {
		parts = append(parts, fmt.Sprintf("%.3g g carbs", rollup.Facts.CarbohydrateG.Value))
	}
	if rollup.Facts.FatG != nil {
		parts = append(parts, fmt.Sprintf("%.3g g fat", rollup.Facts.FatG.Value))
	}
	if rollup.Facts.SaltG != nil {
		parts = append(parts, fmt.Sprintf("%.3g g salt", rollup.Facts.SaltG.Value))
	}
	if len(parts) == 0 {
		return "known nutrients unavailable"
	}
	return strings.Join(parts, ", ")
}

func servingPlanPDFLines(plan ServingPlan, scaled *ScaledMealPlan) []string {
	if plan.Status == ServingPlanNotUsed {
		return nil
	}
	lines := []string{
		"",
		"Servings and scaling:",
		"- Serving status: " + strings.ToUpper(string(plan.Status)),
		fmt.Sprintf("- Meal slots: %d", plan.Summary.TotalMealSlots),
		fmt.Sprintf("- Target serving units: %.3g", plan.Summary.TotalTargetServingUnits),
		fmt.Sprintf("- Cooked serving units: %.3g", plan.Summary.TotalCookedServingUnits),
	}
	if plan.Summary.SlotsWithLeftovers > 0 {
		lines = append(lines, fmt.Sprintf("- Slots with planned leftovers: %d", plan.Summary.SlotsWithLeftovers))
	}
	for _, slot := range plan.Slots {
		lines = append(lines, fmt.Sprintf("- Day %d %s: %s, %.3g servings cooked, scale %.3g", slot.Day, slot.MealSlot, slot.RecipeTitle, slot.CookedServingUnits, slot.ScaleFactor))
		if len(slot.Participants) > 0 {
			var parts []string
			for _, participant := range slot.Participants {
				label := strutil.FirstNonEmpty(participant.MemberID, participant.PortionType, "serving")
				parts = append(parts, fmt.Sprintf("%s %.3g", label, participant.ServingFactor))
			}
			lines = append(lines, "  Participants: "+strings.Join(parts, ", "))
		}
	}
	if scaled != nil {
		for _, slot := range scaled.Slots {
			for _, ingredient := range slot.Ingredients {
				if ingredient.ShoppingQuantity == nil {
					continue
				}
				if ingredient.RoundingApplied {
					lines = append(lines, fmt.Sprintf("  %s: cook about %s; buy %s", ingredient.IngredientName, formatPantryQuantity(ingredient.CookingQuantity), formatPantryQuantity(ingredient.ShoppingQuantity)))
				}
			}
		}
	}
	return lines
}

func pantryResolutionPDFLines(resolution PantryResolution) []string {
	if resolution.Status == PantryResolutionNotUsed {
		return nil
	}
	lines := []string{
		"",
		"Pantry and shopping delta:",
		"- Pantry status: " + strings.ToUpper(string(resolution.Status)),
		fmt.Sprintf("- Shopping ingredients: %d", resolution.Summary.ShopIngredientLines),
		fmt.Sprintf("- Pantry-covered ingredients: %d", resolution.Summary.PantryCoveredIngredientLines),
	}
	var covered []string
	var partial []string
	var blocked []string
	for _, line := range resolution.Lines {
		switch line.PantryDecision {
		case PantryDecisionPantryFullAssumed:
			covered = append(covered, fmt.Sprintf("- %s: assumed pantry staple", line.IngredientName))
		case PantryDecisionPantryFullConfirmed:
			covered = append(covered, fmt.Sprintf("- %s: confirmed pantry item, %s allocated", line.IngredientName, formatPantryQuantity(line.PantryAllocated)))
		case PantryDecisionPantryPartialConfirmed:
			partial = append(partial, fmt.Sprintf("- %s: %s from pantry, %s still needed from Alcampo", line.IngredientName, formatPantryQuantity(line.PantryAllocated), formatPantryQuantity(line.ShopRequired)))
		case PantryDecisionBlockedUnconfirmed, PantryDecisionBlockedAmbiguousUnit, PantryDecisionBlockedExpired:
			blocked = append(blocked, fmt.Sprintf("- %s: %s", line.IngredientName, line.Reason))
		}
	}
	if len(covered) > 0 {
		lines = append(lines, "Not included in basket because covered by pantry:")
		lines = append(lines, covered...)
	}
	if len(partial) > 0 {
		lines = append(lines, "Partially covered by pantry:")
		lines = append(lines, partial...)
	}
	if len(blocked) > 0 {
		lines = append(lines, "Cooking readiness blockers:")
		lines = append(lines, blocked...)
		lines = append(lines, "Do not rely on this plan for cooking until pantry issues are resolved.")
	}
	if len(resolution.Warnings) > 0 {
		lines = append(lines, "Pantry caveats:")
		for _, warning := range resolution.Warnings {
			lines = append(lines, "- "+warning.Message)
		}
	}
	lines = append(lines, "Nutrition caveat: Alcampo label nutrition covers purchased products only; pantry-covered ingredients may not have label nutrition.")
	return lines
}

func recipeSwapPlanPDFLines(plan RecipeSwapPlan) []string {
	lines := []string{"", "Meal plan repair:"}
	switch plan.Status {
	case RecipeSwapNotNeeded:
		lines = append(lines, "- Not needed; the basket was already ready or recoverable without recipe swaps.")
	case RecipeSwapDisabled:
		lines = append(lines, "- Disabled.")
	case RecipeSwapAttemptedApplied:
		lines = append(lines, fmt.Sprintf("- Applied swaps: %d", len(plan.AppliedSwaps)))
		for _, swap := range plan.AppliedSwaps {
			lines = append(lines, fmt.Sprintf("- Day %d %s: %s -> %s", swap.Day, formatMealType(swap.MealSlot), swap.OriginalRecipeTitle, swap.ReplacementRecipeTitle))
			if swap.Reason != "" {
				lines = append(lines, "  Reason: "+swap.Reason)
			}
			lines = append(lines, "  Validation: replacement recipe passed the final readiness gate.")
		}
	case RecipeSwapAttemptedFailed:
		lines = append(lines, "- Attempted, no fully valid replacement was found.")
		if len(plan.RemainingIssues) > 0 {
			lines = append(lines, "- Remaining repair blockers:")
			for _, issue := range plan.RemainingIssues {
				lines = append(lines, "  - "+issue.Message)
			}
		}
	default:
		lines = append(lines, "- Status: "+string(plan.Status))
	}
	if plan.FinalReadinessStatus != "" {
		lines = append(lines, "- Final readiness after repair: "+strings.ToUpper(plan.FinalReadinessStatus))
	}
	return lines
}

func basketOptimizationPlanPDFLines(plan BasketOptimizationPlan) []string {
	lines := []string{
		"",
		"Basket optimization:",
		"- Status: " + strings.ToUpper(string(plan.Status)),
		"- Objective: " + strings.ReplaceAll(strutil.FirstNonEmpty(plan.Policy.Objective, BasketObjectiveSafeBalanced), "_", "-"),
	}
	if plan.Validation.FinalReadinessStatus != "" {
		lines = append(lines, "- Final readiness after optimization: "+strings.ToUpper(plan.Validation.FinalReadinessStatus))
	}
	if plan.OptimizedSummary != nil {
		if plan.BaselineSummary.EffectiveSubtotalCents > 0 {
			delta := plan.OptimizedSummary.EffectiveSubtotalCents - plan.BaselineSummary.EffectiveSubtotalCents
			lines = append(lines, "- Estimated subtotal delta: "+formatOptimizationCents(delta))
		}
		if plan.OptimizedSummary.OfferSavingsCents > 0 {
			lines = append(lines, "- Applied offer savings: "+money.FormatAmount(int64(plan.OptimizedSummary.OfferSavingsCents)))
		}
	}
	changed := 0
	for _, decision := range plan.Decisions {
		if !decision.Changed {
			continue
		}
		changed++
		lines = append(lines, fmt.Sprintf("- %s: %s -> %s", decision.IngredientName, strutil.FirstNonEmpty(decision.BaselineProductName, "baseline product"), strutil.FirstNonEmpty(decision.SelectedProductName, "optimized product")))
		if decision.CostDeltaCents != 0 {
			lines = append(lines, "  Cost delta: "+formatOptimizationCents(decision.CostDeltaCents))
		}
		if decision.OfferSavingsCents > 0 {
			lines = append(lines, "  Offer savings: "+money.FormatAmount(int64(decision.OfferSavingsCents)))
		}
		if decision.Reason != "" {
			lines = append(lines, "  Reason: "+decision.Reason)
		}
	}
	if changed == 0 {
		lines = append(lines, "- No product switch was applied.")
	}
	for _, warning := range plan.Warnings {
		if warning.Message != "" {
			lines = append(lines, "- Caveat: "+warning.Message)
		}
	}
	return lines
}

func formatOptimizationCents(cents int) string {
	sign := "+"
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return sign + money.FormatAmount(int64(cents))
}

func recipeSwapsBySlot(plan *RecipeSwapPlan) map[string]AppliedRecipeSwap {
	out := map[string]AppliedRecipeSwap{}
	if plan == nil {
		return out
	}
	for _, swap := range plan.AppliedSwaps {
		out[recipeSwapSlotKey(swap.Day, swap.MealSlot)] = swap
	}
	return out
}

func recipeSwapSlotKey(day int, mealSlot string) string {
	return fmt.Sprintf("%d|%s", day, normalizeKey(mealSlot))
}

func ledgerTrustPDFLines(ledger QuantityLedger, safety *BasketSafety) []string {
	lines := []string{
		"",
		"Shopping confidence:",
		"Ledger status: " + strings.ToUpper(string(ledger.Status)),
		fmt.Sprintf("Ingredient coverage: %d / %d", ledger.Summary.CoveredIngredientCount, ledger.Summary.IngredientCount),
		fmt.Sprintf("Exact quantity lines: %d", ledger.Summary.ExactQuantityLines),
		fmt.Sprintf("Estimated variable-weight lines: %d", ledger.Summary.EstimatedVariableWeightLines),
		fmt.Sprintf("Needs-review lines: %d", ledger.Summary.NeedsReviewLines),
		fmt.Sprintf("Missing lines: %d", ledger.Summary.MissingLines),
	}
	if safety != nil {
		lines = append(lines, "Basket safety: "+strings.ToUpper(string(safety.Status)))
		if safety.Reason != "" {
			lines = append(lines, "Basket safety reason: "+safety.Reason)
		}
	}
	if ledger.Nutrition != nil {
		lines = append(lines, fmt.Sprintf("Nutrition label coverage: %.0f%%", ledger.Nutrition.Coverage.CalorieCoverageRatio*100))
	}
	if ledger.Totals.EstimatedTotal.Expected.Amount != "" {
		label := "Estimated total: "
		if !ledger.Totals.EstimatedTotal.IsExact {
			label = "Estimated total/range: "
		}
		lines = append(lines, label+formatPDFMoney(ledger.Totals.EstimatedTotal.Expected.Amount, ledger.Totals.EstimatedTotal.Expected.Currency))
	}
	for _, warning := range ledger.Warnings {
		if warning.Message != "" {
			lines = append(lines, "Ledger warning: "+warning.Message)
		}
	}
	return lines
}

func ledgerAllocationPDFLines(ledger QuantityLedger) []string {
	if len(ledger.Allocations) == 0 {
		return nil
	}
	requirements := make(map[string]IngredientRequirement, len(ledger.Requirements))
	for _, req := range ledger.Requirements {
		requirements[req.RequirementID] = req
	}
	lines := []string{"Quantity ledger:"}
	for _, allocation := range ledger.Allocations {
		req := requirements[allocation.RequirementID]
		name := strutil.FirstNonEmpty(req.IngredientName, allocation.RequirementID)
		badges := strings.Join(allocation.Badges, ", ")
		if badges != "" {
			badges = " [" + badges + "]"
		}
		if allocation.MatchType == "missing" {
			lines = append(lines, fmt.Sprintf("- %s: MISSING%s", name, badges))
		} else {
			lines = append(lines, fmt.Sprintf("- %s -> %s%s", name, strutil.FirstNonEmpty(allocation.ProductName, allocation.SKU, "selected product"), badges))
		}
		if allocation.RequiredQuantity != nil {
			lines = append(lines, "  Required: "+formatNormalizedQuantity(*allocation.RequiredQuantity))
		}
		if allocation.PackageCount > 0 {
			lines = append(lines, fmt.Sprintf("  Buy: %d package(s)", allocation.PackageCount))
		}
		if allocation.PurchasedQuantity != nil {
			lines = append(lines, "  Purchased coverage: "+formatQuantityRange(*allocation.PurchasedQuantity))
		}
		if allocation.ExcessQuantity != nil && allocation.ExcessQuantity.BaseValue > 0 {
			lines = append(lines, "  Excess: "+formatNormalizedQuantity(*allocation.ExcessQuantity))
		}
		if allocation.MissingQuantity != nil {
			lines = append(lines, "  Missing: "+formatNormalizedQuantity(*allocation.MissingQuantity))
		}
		lines = append(lines, fmt.Sprintf("  Confidence: match %.0f%%, quantity %.0f%%", allocation.MatchConfidence*100, allocation.QuantityConfidence*100))
		for _, warning := range allocation.Warnings {
			if warning.Message != "" {
				lines = append(lines, "  Warning: "+warning.Message)
			}
		}
	}
	lines = append(lines, "")
	return lines
}

func recoveryPlanPDFLines(plan RecoveryPlan) []string {
	lines := []string{
		"",
		"Recovery actions:",
		"Recovery status: " + strings.ToUpper(string(plan.Status)),
	}
	if plan.StartedFromStatus != "" || plan.FinalLedgerStatus != "" {
		lines = append(lines, "Ledger recovery: "+strutil.FirstNonEmpty(plan.StartedFromStatus, "unknown")+" -> "+strutil.FirstNonEmpty(plan.FinalLedgerStatus, "unknown"))
	}
	if len(plan.AppliedDecisions) == 0 {
		if len(plan.RemainingIssues) > 0 {
			lines = append(lines, "No safe recovery was applied; remaining issues require review.")
		} else if len(plan.Issues) == 0 {
			lines = append(lines, "No recovery was needed.")
		}
		return lines
	}
	for _, decision := range plan.AppliedDecisions {
		lines = append(lines, fmt.Sprintf("- %s: %s", decision.OriginalIngredient, decision.DecisionType))
		if decision.FinalProductID != "" {
			lines = append(lines, "  Final product: "+decision.FinalProductID)
		}
		if decision.Reason != "" {
			lines = append(lines, "  Reason: "+decision.Reason)
		}
		if decision.Confidence != "" {
			lines = append(lines, "  Confidence: "+decision.Confidence)
		}
		for _, change := range decision.RecipeChanges {
			label := "Recipe change"
			if change.ChangeType != "" {
				label += " (" + change.ChangeType + ")"
			}
			lines = append(lines, "  "+label+": "+change.NewText)
		}
	}
	if len(plan.RemainingIssues) > 0 {
		lines = append(lines, "Remaining recovery issues:")
		for _, issue := range plan.RemainingIssues {
			lines = append(lines, "- "+issue.IngredientName+": "+issue.BlockingReason)
		}
	}
	return lines
}

func pdfLinesForRecipe(recipe Recipe) (string, []string) {
	return recipe.Title, recipeSummaryLines(recipe)
}

func pdfLinesForRecipes(mealPlanID string, recipes []Recipe) (string, []string) {
	title := "Recipes"
	if mealPlanID != "" {
		title = "Recipes for " + mealPlanID
	}
	var lines []string
	for i, recipe := range recipes {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, recipe.Title)
		lines = append(lines, recipeSummaryLines(recipe)...)
	}
	if len(lines) == 0 {
		lines = append(lines, "No recipes found.")
	}
	return title, lines
}

func pdfLinesForShop(shop ShopResult) (string, []string) {
	lines := []string{
		"Selection policy: " + shop.Policy,
		"Estimated total: " + formatPDFMoney(shop.EstimatedTotal.Amount, shop.EstimatedTotal.Currency),
	}
	if shop.Nutrition != nil {
		lines = append(lines, "Nutrition: "+formatNutritionSummary(*shop.Nutrition))
	}
	lines = append(lines, shopResultPDFLines(shop)...)
	return "Carrito Plan", lines
}

func shopResultPDFLines(shop ShopResult) []string {
	lines := []string{"", "Selected products:"}
	for _, selected := range shop.SelectedProducts {
		if selected.Error != "" {
			lines = append(lines, fmt.Sprintf("- %s: ERROR %s", selected.Ingredient.Name, selected.Error))
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s -> %s (%s)", selected.Ingredient.Name, selected.Product.Name, formatPDFMoney(selected.Product.Price.Amount, selected.Product.Price.Currency)))
		if selected.PurchaseQuantity != "" {
			lines = append(lines, "  Buy: "+selected.PurchaseQuantity+" package(s)")
		}
		if selected.LineTotal.Amount != "" {
			lines = append(lines, "  Line total: "+formatPDFMoney(selected.LineTotal.Amount, selected.LineTotal.Currency))
		}
		if selected.QuantityReason != "" {
			lines = append(lines, "  Package math: "+selected.QuantityReason)
		}
		if selected.Product.ImageURL != "" {
			lines = append(lines, "  Product image: "+selected.Product.ImageURL)
		}
		for _, offer := range selected.Product.Offers {
			lines = append(lines, "  Offer: "+offer)
		}
		if selected.ProductNutrition != nil {
			lines = append(lines, "  Nutrition: "+formatNutritionBasis(*selected.ProductNutrition))
			lines = append(lines, formatStructuredNutritionPDFLines("  Label", *selected.ProductNutrition)...)
		}
		if selected.RequiredNutrition != nil {
			lines = append(lines, "  Required nutrition estimate: "+formatNutritionEstimate(*selected.RequiredNutrition))
		}
		if selected.SelectionReason != "" {
			lines = append(lines, "  Reason: "+selected.SelectionReason)
		}
		for _, warning := range selected.Warnings {
			lines = append(lines, "  Warning: "+warning)
		}
		for _, warning := range selected.NutritionWarnings {
			lines = append(lines, "  Nutrition warning: "+warning)
		}
	}
	lines = append(lines, "Estimated total: "+formatPDFMoney(shop.EstimatedTotal.Amount, shop.EstimatedTotal.Currency))
	if len(shop.BasketLines) > 0 {
		lines = append(lines, "", "Basket lines:")
		lines = append(lines, shop.BasketLines...)
	}
	return lines
}

func shoppingStatusLabel(shop ShopResult) string {
	if shop.Complete {
		return "complete"
	}
	return "incomplete"
}

func missingShoppingIngredients(shop ShopResult) []string {
	var missing []string
	for _, selected := range shop.SelectedProducts {
		if selected.Error != "" {
			missing = append(missing, selected.Ingredient.Name)
		}
	}
	return missing
}

func nutritionCoveragePDFLines(shop ShopResult) []string {
	if len(shop.SelectedProducts) == 0 {
		return nil
	}
	total := 0
	covered := 0
	var missing []string
	for _, selected := range shop.SelectedProducts {
		if selected.Error != "" {
			continue
		}
		total++
		if selected.RequiredNutrition != nil {
			covered++
			continue
		}
		missing = append(missing, selected.Ingredient.Name)
	}
	if total == 0 {
		return nil
	}
	lines := []string{fmt.Sprintf("Selected-product nutrition coverage: %d of %d ingredients with safe label-based estimates.", covered, total)}
	if len(missing) > 0 {
		lines = append(lines, "Nutrition missing or unsafe for: "+strings.Join(missing, ", "))
	}
	if len(shop.NutritionWarnings) > 0 {
		lines = append(lines, "Nutrition warnings:")
		for _, warning := range shop.NutritionWarnings {
			lines = append(lines, "- "+warning)
		}
	}
	return lines
}

func pantryUsagePDFLines(plan MealPlan) []string {
	if len(plan.PantryUsage) == 0 {
		return nil
	}
	lines := []string{"Pantry and fridge used:"}
	for _, use := range plan.PantryUsage {
		lines = append(lines, fmt.Sprintf("- %s: %.3g %s from %s", use.Ingredient, use.Quantity, use.Unit, use.PantryItem))
	}
	lines = append(lines, "")
	return lines
}

func requiredPurchasePDFLines(plan MealPlan) []string {
	if len(plan.RequiredPurchases) == 0 {
		return nil
	}
	lines := []string{"Ingredients to buy:"}
	for _, item := range plan.RequiredPurchases {
		lines = append(lines, fmt.Sprintf("- %s: %.3g %s", item.Name, item.Quantity, item.Unit))
	}
	lines = append(lines, "")
	return lines
}

func recipeSummaryLines(recipe Recipe) []string {
	lines := []string{
		fmt.Sprintf("Servings: %d", recipe.Servings),
		fmt.Sprintf("Time: prep %d min, cook %d min", recipe.PrepMinutes, recipe.CookMinutes),
	}
	if recipe.ImageURL != "" {
		lines = append(lines, "Cover image: "+recipe.ImageURL)
	}
	if len(recipe.Tags) > 0 {
		lines = append(lines, "Tags: "+strings.Join(recipe.Tags, ", "))
	}
	if recipe.NutritionPerServing != nil {
		lines = append(lines, "Nutrition per serving: "+formatNutritionSummary(*recipe.NutritionPerServing))
	}
	lines = append(lines, "Ingredients:")
	for _, ing := range recipe.Ingredients {
		lines = append(lines, fmt.Sprintf("- %s: %.3g %s", ing.Name, ing.Quantity, ing.Unit))
	}
	if len(recipe.Equipment) > 0 {
		lines = append(lines, "Equipment: "+strings.Join(recipe.Equipment, ", "))
	}
	lines = append(lines, "Cooking instructions:")
	for _, step := range recipe.Steps {
		lines = append(lines, fmt.Sprintf("%d. %s", step.Number, step.Text))
		if step.ImageURL != "" {
			lines = append(lines, "   Step image: "+step.ImageURL)
		}
	}
	if len(recipe.Substitutions) > 0 {
		lines = append(lines, "Substitutions:")
		for _, sub := range recipe.Substitutions {
			lines = append(lines, "- "+sub)
		}
	}
	return lines
}

func formatNutritionSummary(summary NutritionSummary) string {
	return fmt.Sprintf("%.0f kcal, %.1fg protein, %.1fg carbs, %.1fg fat", summary.Kcal, summary.ProteinG, summary.CarbsG, summary.FatG)
}

func formatPDFMoney(amount, currency string) string {
	if amount == "" {
		return "unknown"
	}
	return amount + " " + strutil.FirstNonEmpty(currency, "EUR")
}

func formatNutritionBasis(n StructuredNutrition) string {
	switch n.Basis {
	case NutritionPer100g:
		return "Alcampo label, per 100 g"
	case NutritionPer100ml:
		return "Alcampo label, per 100 ml"
	case NutritionPerServing:
		return "Alcampo label, per serving"
	case NutritionPerUnit:
		return "Alcampo label, per unit"
	default:
		return "Alcampo label, basis unknown"
	}
}

func formatStructuredNutritionPDFLines(prefix string, n StructuredNutrition) []string {
	var lines []string
	if n.Kcal != nil {
		lines = append(lines, fmt.Sprintf("%s energy: %.0f kcal", prefix, *n.Kcal))
	}
	if n.ProteinG != nil {
		lines = append(lines, fmt.Sprintf("%s protein: %.1f g", prefix, *n.ProteinG))
	}
	if n.CarbsG != nil {
		lines = append(lines, fmt.Sprintf("%s carbs: %.1f g", prefix, *n.CarbsG))
	}
	if n.FatG != nil {
		lines = append(lines, fmt.Sprintf("%s fat: %.1f g", prefix, *n.FatG))
	}
	if n.SaltG != nil {
		lines = append(lines, fmt.Sprintf("%s salt: %.3g g", prefix, *n.SaltG))
	}
	return lines
}

func formatNutritionEstimate(estimate NutritionEstimate) string {
	n := estimate.Nutrients
	var parts []string
	if n.Kcal != nil {
		parts = append(parts, fmt.Sprintf("%.0f kcal", *n.Kcal))
	}
	if n.ProteinG != nil {
		parts = append(parts, fmt.Sprintf("%.1fg protein", *n.ProteinG))
	}
	if n.CarbsG != nil {
		parts = append(parts, fmt.Sprintf("%.1fg carbs", *n.CarbsG))
	}
	if n.FatG != nil {
		parts = append(parts, fmt.Sprintf("%.1fg fat", *n.FatG))
	}
	if len(parts) == 0 {
		return "available"
	}
	return strings.Join(parts, ", ")
}

func formatNormalizedQuantity(q NormalizedQuantity) string {
	if q.Unit == "" {
		return fmt.Sprintf("%.3g", q.Value)
	}
	if q.BaseUnit != "" && q.BaseUnit != q.Unit {
		return fmt.Sprintf("%.3g %s (%.3g %s)", q.Value, q.Unit, q.BaseValue, q.BaseUnit)
	}
	return fmt.Sprintf("%.3g %s", q.Value, q.Unit)
}

func formatQuantityRange(q QuantityRange) string {
	label := formatNormalizedQuantity(q.Expected)
	if !q.IsExact {
		label += " estimated"
	}
	if q.Reason != "" {
		label += " (" + q.Reason + ")"
	}
	return label
}

func formatMealType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "Meal"
	}
	value = strings.ToLower(value)
	return strings.ToUpper(value[:1]) + value[1:]
}

type pdfElement struct {
	Text     string
	FontSize float64
	Image    *pdfImageObject
	Caption  string
}

type pdfPage struct {
	Items []pdfPageItem
}

type pdfPageItem struct {
	Text     string
	FontSize float64
	X        float64
	Y        float64
	Image    *pdfImageObject
	Width    float64
	Height   float64
}

type pdfImageObject struct {
	Name   string
	Width  int
	Height int
	Data   []byte
}

func pdfElements(title string, lines []string, imageBaseDir string) ([]pdfElement, []pdfImageObject) {
	elements := []pdfElement{
		{Text: title, FontSize: 18},
		{Text: "", FontSize: 12},
	}
	var images []pdfImageObject
	imageByRef := make(map[string]*pdfImageObject)
	for _, line := range lines {
		if label, ref, ok := pdfImageReference(line); ok {
			if img, exists := imageByRef[ref]; exists {
				elements = append(elements, pdfElement{Image: img, Caption: label})
				continue
			}
			img, err := loadPDFImage(ref, len(images)+1, imageBaseDir)
			if err == nil {
				images = append(images, *img)
				imageByRef[ref] = &images[len(images)-1]
				elements = append(elements, pdfElement{Image: &images[len(images)-1], Caption: label})
				continue
			}
		}
		for _, wrapped := range wrapPDFLine(line, 92) {
			elements = append(elements, pdfElement{Text: wrapped, FontSize: 12})
		}
	}
	return elements, images
}

func pdfImageReference(line string) (string, string, bool) {
	trimmed := strings.TrimSpace(line)
	for _, prefix := range []string{"Cover image:", "Product image:", "Step image:"} {
		if strings.HasPrefix(trimmed, prefix) {
			ref := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
			if ref == "" {
				return "", "", false
			}
			return strings.TrimSuffix(prefix, ":"), ref, true
		}
	}
	return "", "", false
}

func loadPDFImage(ref string, number int, imageBaseDir string) (*pdfImageObject, error) {
	data, err := readPDFImageData(ref, imageBaseDir)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	img = resizePDFImage(img, 640)
	bounds := img.Bounds()
	var raw bytes.Buffer
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			raw.Write(pdfRGB(img.At(x, y)))
		}
	}
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(raw.Bytes()); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return &pdfImageObject{
		Name:   fmt.Sprintf("Im%d", number),
		Width:  bounds.Dx(),
		Height: bounds.Dy(),
		Data:   compressed.Bytes(),
	}, nil
}

func readPDFImageData(ref string, imageBaseDir string) ([]byte, error) {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "data:image/") {
		comma := strings.IndexByte(ref, ',')
		if comma < 0 {
			return nil, fmt.Errorf("invalid data image")
		}
		meta := ref[:comma]
		payload := ref[comma+1:]
		if strings.Contains(meta, ";base64") {
			return base64.StdEncoding.DecodeString(payload)
		}
		decoded, err := url.QueryUnescape(payload)
		if err != nil {
			return nil, err
		}
		return []byte(decoded), nil
	}
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		req, err := http.NewRequest(http.MethodGet, ref, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "carrito-food-agent/1.0")
		client := http.Client{Timeout: 8 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("image fetch failed with %s", resp.Status)
		}
		return readLimited(resp.Body, 5*1024*1024)
	}
	if imageBaseDir != "" && !filepath.IsAbs(ref) {
		ref = filepath.Join(imageBaseDir, ref)
	}
	return os.ReadFile(ref)
}

func readLimited(r io.Reader, limit int64) ([]byte, error) {
	var buf bytes.Buffer
	n, err := io.Copy(&buf, io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if n > limit {
		return nil, fmt.Errorf("image exceeds %d bytes", limit)
	}
	return buf.Bytes(), nil
}

func resizePDFImage(img image.Image, maxSide int) image.Image {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= maxSide && height <= maxSide {
		return img
	}
	scale := math.Min(float64(maxSide)/float64(width), float64(maxSide)/float64(height))
	newWidth := max(1, int(math.Round(float64(width)*scale)))
	newHeight := max(1, int(math.Round(float64(height)*scale)))
	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	for y := 0; y < newHeight; y++ {
		srcY := bounds.Min.Y + int(float64(y)/scale)
		if srcY >= bounds.Max.Y {
			srcY = bounds.Max.Y - 1
		}
		for x := 0; x < newWidth; x++ {
			srcX := bounds.Min.X + int(float64(x)/scale)
			if srcX >= bounds.Max.X {
				srcX = bounds.Max.X - 1
			}
			dst.Set(x, y, img.At(srcX, srcY))
		}
	}
	return dst
}

func pdfRGB(c color.Color) []byte {
	r, g, b, a := c.RGBA()
	if a == 0 {
		return []byte{255, 255, 255}
	}
	if a < 0xffff {
		r += 0xffff - a
		g += 0xffff - a
		b += 0xffff - a
	}
	return []byte{byte(r >> 8), byte(g >> 8), byte(b >> 8)}
}

func layoutPDFPages(elements []pdfElement) []pdfPage {
	const (
		pageHeight = 842.0
		margin     = 50.0
		lineHeight = 16.0
		imageMaxW  = 235.0
		imageMaxH  = 150.0
	)
	var pages []pdfPage
	page := pdfPage{}
	y := pageHeight - margin
	flush := func() {
		if len(page.Items) > 0 {
			pages = append(pages, page)
		}
		page = pdfPage{}
		y = pageHeight - margin
	}
	ensure := func(height float64) {
		if y-height < margin {
			flush()
		}
	}
	for _, el := range elements {
		if el.Image != nil {
			width, height := fitPDFImage(el.Image, imageMaxW, imageMaxH)
			needed := height + 22
			ensure(needed)
			imageY := y - height
			page.Items = append(page.Items, pdfPageItem{Image: el.Image, X: margin, Y: imageY, Width: width, Height: height})
			y = imageY - 12
			if el.Caption != "" {
				page.Items = append(page.Items, pdfPageItem{Text: el.Caption, FontSize: 10, X: margin, Y: y})
				y -= 14
			}
			continue
		}
		if el.Text == "" {
			ensure(8)
			y -= 8
			continue
		}
		fontSize := el.FontSize
		if fontSize <= 0 {
			fontSize = 12
		}
		height := lineHeight
		if fontSize > 12 {
			height = 24
		}
		ensure(height)
		page.Items = append(page.Items, pdfPageItem{Text: el.Text, FontSize: fontSize, X: margin, Y: y})
		y -= height
	}
	flush()
	return pages
}

func fitPDFImage(img *pdfImageObject, maxWidth, maxHeight float64) (float64, float64) {
	if img == nil || img.Width <= 0 || img.Height <= 0 {
		return maxWidth, maxHeight
	}
	scale := math.Min(maxWidth/float64(img.Width), maxHeight/float64(img.Height))
	if scale <= 0 {
		scale = 1
	}
	return float64(img.Width) * scale, float64(img.Height) * scale
}

func (img pdfImageObject) Object() []byte {
	header := fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode /Length %d >>\nstream\n", img.Width, img.Height, len(img.Data))
	var buf bytes.Buffer
	buf.WriteString(header)
	buf.Write(img.Data)
	buf.WriteString("\nendstream")
	return buf.Bytes()
}

func pdfStreamObject(data []byte) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "<< /Length %d >>\nstream\n", len(data))
	buf.Write(data)
	buf.WriteString("\nendstream")
	return buf.Bytes()
}

func pdfPageObject(contentObj int, imageObjectNums map[string]int) string {
	var xobjects strings.Builder
	for name, objNum := range imageObjectNums {
		fmt.Fprintf(&xobjects, "/%s %d 0 R ", name, objNum)
	}
	resources := "<< /Font << /F1 3 0 R >>"
	if xobjects.Len() > 0 {
		resources += " /XObject << " + xobjects.String() + ">>"
	}
	resources += " >>"
	return fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources %s /Contents %d 0 R >>", resources, contentObj)
}

func wrapPDFLine(line string, width int) []string {
	line = strings.TrimRight(line, "\r\n")
	if len(line) <= width {
		return []string{line}
	}
	var out []string
	words := strings.Fields(line)
	if len(words) == 0 {
		return []string{""}
	}
	current := words[0]
	for _, word := range words[1:] {
		if len(current)+1+len(word) > width {
			out = append(out, current)
			current = word
			continue
		}
		current += " " + word
	}
	if current != "" {
		out = append(out, current)
	}
	return out
}

func pdfContentStream(page pdfPage) string {
	var b strings.Builder
	for _, item := range page.Items {
		if item.Image != nil {
			fmt.Fprintf(&b, "q\n%.2f 0 0 %.2f %.2f %.2f cm\n/%s Do\nQ\n", item.Width, item.Height, item.X, item.Y, item.Image.Name)
			continue
		}
		fontSize := item.FontSize
		if fontSize <= 0 {
			fontSize = 12
		}
		fmt.Fprintf(&b, "BT\n/F1 %.0f Tf\n%.2f %.2f Td\n(%s) Tj\nET\n", fontSize, item.X, item.Y, escapePDFText(item.Text))
	}
	return b.String()
}

func escapePDFText(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "(", `\(`)
	s = strings.ReplaceAll(s, ")", `\)`)
	return s
}
