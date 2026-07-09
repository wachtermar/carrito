package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/food"
	"github.com/wachtermar/carrito/internal/output"
)

type foodPlanOptions struct {
	Days      int
	People    int
	BudgetEUR string
	Meals     string
	OutPath   string
}

func runFoodPlan(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food plan", stderr)
	days := fs.Int("days", 3, "number of days")
	people := fs.Int("people", 0, "people count; required unless saved in profile")
	budget := fs.String("budget", "", "budget in EUR")
	meals := fs.String("meals", "dinner", "comma-separated meal types")
	outPath := fs.String("out", "", "optional output JSON path")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	profile, pantry, err := loadFoodPlanningInputs()
	if err != nil {
		return err
	}
	plan, err := buildSavedFoodPlan(profile, pantry, foodPlanOptions{
		Days:      *days,
		People:    *people,
		BudgetEUR: *budget,
		Meals:     *meals,
		OutPath:   *outPath,
	})
	if err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, plan)
	}
	printMealPlan(stdout, plan)
	return nil
}

func runFoodUseUp(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food use-up", stderr)
	days := fs.Int("expiring-days", 3, "rank recipes using pantry items expiring within N days")
	limit := fs.Int("limit", 8, "maximum recipes to return")
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
	suggestions, err := food.SuggestUseUpRecipes(profile, pantry, *days, *limit)
	if err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, suggestions)
	}
	if len(suggestions) == 0 {
		fmt.Fprintln(stdout, "no expiring pantry matches found")
		return nil
	}
	for _, suggestion := range suggestions {
		printUseUpSuggestion(stdout, suggestion)
	}
	return nil
}

func runFoodRecipe(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food recipe", stderr)
	people := fs.Int("people", 0, "people count; defaults to profile or 2")
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
	recipe, err := food.GenerateRecipe(prompt, profile, *people)
	if err != nil {
		return err
	}
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
	resolvedPolicy, err := resolveAndSaveFoodPolicy(&profile, *policy, stderr)
	if err != nil {
		return err
	}
	plan, err := food.LoadMealPlan(fs.Arg(0))
	if err != nil {
		return err
	}
	result, err := shopFoodPlan(plan, profile, resolvedPolicy, *limit, *store)
	if err != nil {
		return err
	}
	if err := writeFoodShopOutputs(result, *outPath, *basketOut); err != nil {
		return err
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
		return errors.New("food pdf requires a recipe, mealplan, shop, or food-run JSON file")
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

func runFoodHTML(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food html", stderr)
	outPath := fs.String("out", "", "output HTML path")
	coverImage := fs.String("cover-image", "", "optional dish image path or URL for the first recipe")
	if err := parseInterspersed(fs, args, nil); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("food html requires a recipe, mealplan, shop, or food-run JSON file")
	}
	input := fs.Arg(0)
	if *outPath == "" {
		ext := filepath.Ext(input)
		if ext == "" {
			*outPath = input + ".html"
		} else {
			*outPath = strings.TrimSuffix(input, ext) + ".html"
		}
	}
	if err := food.WriteHTMLFromJSONFile(input, *outPath, food.HTMLPageOptions{CoverImageURL: *coverImage}); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "html\t%s\n", *outPath)
	return nil
}

func runFoodRun(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("food run", stderr)
	days := fs.Int("days", 3, "number of days")
	people := fs.Int("people", 0, "people count; required unless saved in profile")
	budget := fs.String("budget", "", "budget in EUR")
	meals := fs.String("meals", "dinner", "comma-separated meal types")
	policy := fs.String("selection-policy", "", "balanced, cheapest, or quality; saved if provided")
	limit := fs.Int("limit", 8, "candidate products per ingredient")
	planOut := fs.String("plan-out", "", "optional mealplan JSON path")
	shopOut := fs.String("shop-out", "", "optional shopping JSON path")
	runOut := fs.String("run-out", "", "optional combined food-run JSON path")
	ledgerOut := fs.String("quantity-ledger-out", "", "optional quantity ledger JSON path")
	basketOut := fs.String("basket-out", "", "optional basket file path for guarded cart set-many")
	pdfOut := fs.String("pdf-out", "", "optional combined meal-plan and shopping PDF path")
	enrichProducts := fs.Bool("enrich-products", false, "enrich selected products from detail pages before building the ledger")
	noProductDetailEnrichment := fs.Bool("no-product-detail-enrichment", false, "skip selected-product detail enrichment")
	detailCacheDir := fs.String("detail-cache-dir", "", "product detail cache directory")
	strictQuantity := fs.Bool("strict-quantity", false, "mark low-confidence quantity lines as review-only")
	allowEstimatedVariableWeight := fs.Bool("allow-estimated-variable-weight", false, "allow estimated variable-weight lines to be considered basket-safe")
	allowLowConfidenceBasket := fs.Bool("allow-low-confidence-basket", false, "allow low-confidence quantity lines to be considered basket-safe")
	recoverMissing := fs.Bool("recover-missing", false, "attempt audited recovery for missing or blocked ledger lines")
	noRecovery := fs.Bool("no-recovery", false, "skip ledger-driven recovery planning")
	recoveryOut := fs.String("recovery-out", "", "optional recovery plan JSON path")
	readinessOut := fs.String("readiness-out", "", "optional readiness gate JSON path")
	recipeSwapOut := fs.String("recipe-swap-out", "", "optional recipe swap plan JSON path")
	var recipeFiles stringListFlag
	var recipeDirs stringListFlag
	var recipeURLs stringListFlag
	fs.Var(&recipeFiles, "recipe-file", "structured recipe JSON file to inject into meal slots; repeat or comma-separate")
	fs.Var(&recipeDirs, "recipe-dir", "directory of structured recipe JSON files to inject; repeat or comma-separate")
	fs.Var(&recipeURLs, "recipe-url", "structured schema.org Recipe URL to import; repeat or comma-separate")
	recipeIntakeOut := fs.String("recipe-intake-out", "", "optional recipe intake plan JSON path")
	recipeQualityOut := fs.String("recipe-quality-out", "", "optional recipe quality report JSON path")
	productEvidenceOut := fs.String("product-evidence-out", "", "optional final Alcampo product evidence JSON path")
	strictRecipeQuality := fs.Bool("strict-recipe-quality", false, "treat recipe quality blockers as cook-readiness failures")
	requireRecipeSource := fs.Bool("require-recipe-source", false, "require every active recipe to have source provenance")
	requireRecipeImages := fs.Bool("require-recipe-images", false, "require every active recipe to have verified local/cached image evidence")
	recipeImageCacheDir := fs.String("recipe-image-cache-dir", "", "recipe image cache directory")
	noRecipeImageCache := fs.Bool("no-recipe-image-cache", false, "do not cache recipe images for PDF provenance")
	allowStructuredRecipeURL := fs.Bool("allow-structured-recipe-url", true, "allow schema.org JSON-LD Recipe URL import")
	noStructuredRecipeURL := fs.Bool("no-structured-recipe-url", false, "disable structured recipe URL import")
	basketOptimizationOut := fs.String("basket-optimization-out", "", "optional basket optimization plan JSON path")
	budgetDealOut := fs.String("budget-deal-out", "", "optional budget and deal evidence JSON path")
	budgetRepairOut := fs.String("budget-repair-out", "", "optional budget repair plan JSON path")
	intentFile := fs.String("intent-file", "", "optional meal-run intent contract JSON path")
	intentOut := fs.String("intent-out", "", "optional normalized meal-run intent JSON path")
	constraintReportOut := fs.String("constraint-report-out", "", "optional request constraint satisfaction report JSON path")
	requireIntentReady := fs.Bool("require-intent-ready", false, "return exit 20 when explicit request constraints are not satisfied")
	maxCookMinutes := fs.Int("max-cook-minutes", 0, "maximum total minutes per recipe when creating a CLI intent")
	var excludedIngredients stringListFlag
	var excludedAllergens stringListFlag
	var dietRules stringListFlag
	fs.Var(&excludedIngredients, "exclude-ingredient", "ingredient term to exclude from recipes/products; repeat or comma-separate")
	fs.Var(&excludedAllergens, "exclude-allergen", "allergen term to exclude from recipes/products; repeat or comma-separate")
	fs.Var(&dietRules, "diet-rule", "diet rule to record in the intent; repeat or comma-separate")
	repairBudget := fs.Bool("repair-budget", false, "attempt validated budget repair when the budget/deal report is over budget")
	noBudgetRepair := fs.Bool("no-budget-repair", false, "disable budget repair even when requested by output flags")
	allowBudgetProductSwitches := fs.Bool("allow-budget-product-switches", true, "allow budget repair to switch to validated cheaper equivalent products")
	noBudgetProductSwitches := fs.Bool("no-budget-product-switches", false, "disable budget repair product switches")
	allowBudgetRecipeSwap := fs.Bool("allow-budget-recipe-swap", false, "allow budget repair to change recipes when a later repair stage supports it")
	noBudgetRecipeSwap := fs.Bool("no-budget-recipe-swap", false, "disable budget recipe swaps")
	maxBudgetRepairProductSwitches := fs.Int("max-budget-repair-product-switches", 4, "maximum product switches budget repair may apply")
	maxBudgetRepairRecipeSwaps := fs.Int("max-budget-repair-recipe-swaps", 1, "maximum budget recipe swaps when enabled")
	maxBudgetRepairCandidates := fs.Int("max-budget-repair-candidates", 6, "maximum candidate products per budget cost driver")
	maxBudgetRepairChecks := fs.Int("max-budget-repair-checks", 30, "maximum total product candidate checks during budget repair")
	budgetRepairMinSavingsCents := fs.Int("budget-repair-min-savings-cents", 50, "minimum estimated savings in cents required before budget repair switches a product")
	nutritionLedgerOut := fs.String("nutrition-ledger-out", "", "optional nutrition evidence ledger JSON path")
	manifestOut := fs.String("manifest-out", "", "optional food run manifest JSON path")
	auditOut := fs.String("audit-out", "", "optional artifact audit report JSON path")
	auditModeFlag := fs.String("audit-mode", food.ArtifactAuditModeOff, "artifact audit exit behavior: off, warn, or fail")
	recordLiveSnapshot := fs.String("record-live-snapshot", "", "record read-only Alcampo HTTP responses into a live snapshot directory")
	replayLiveSnapshot := fs.String("replay-live-snapshot", "", "replay read-only Alcampo HTTP responses from a live snapshot directory")
	snapshotStrict := fs.Bool("snapshot-strict", false, "fail when a replay snapshot is missing a request signature")
	snapshotID := fs.String("snapshot-id", "", "optional snapshot id when recording live responses")
	nutritionMode := fs.String("nutrition-mode", "hybrid", "nutrition evidence mode: off, labels, recipe, or hybrid")
	requireNutritionReady := fs.Bool("require-nutrition-ready", false, "return exit 20 when nutrition evidence does not meet requested coverage")
	requireBudgetReady := fs.Bool("require-budget-ready", false, "return exit 20 when budget/deal evidence is missing, stale, over budget, or otherwise not ready")
	refreshProductEvidence := fs.Bool("refresh-product-evidence", false, "refresh final selected Alcampo product evidence before sealing artifacts")
	noProductEvidenceRefresh := fs.Bool("no-product-evidence-refresh", false, "skip final selected-product evidence refresh")
	requireFreshProductEvidence := fs.Bool("require-fresh-product-evidence", false, "return exit 20 when final selected products are not freshly verified")
	maxProductEvidenceAgeSeconds := fs.Int("max-product-evidence-age-seconds", 0, "maximum accepted product evidence age in seconds when supported")
	allowProductEvidenceCache := fs.Bool("allow-product-evidence-cache", true, "allow cached product detail evidence")
	noProductEvidenceCache := fs.Bool("no-product-evidence-cache", false, "disallow cached product detail evidence")
	minNutritionLineCoverage := fs.Float64("min-nutrition-line-coverage", 0, "minimum nutrition ingredient-line coverage ratio from 0.0 to 1.0")
	minNutritionQuantityCoverage := fs.Float64("min-nutrition-quantity-coverage", 0, "minimum nutrition quantity coverage ratio from 0.0 to 1.0")
	includePurchasedExcessNutrition := fs.Bool("include-purchased-excess-nutrition", false, "include purchased package excess nutrition separately")
	allowPantryProfileNutrition := fs.Bool("allow-pantry-profile-nutrition", true, "allow pantry-profile nutrition evidence")
	allowBuiltinPantryNutrition := fs.Bool("allow-builtin-pantry-nutrition", false, "allow built-in pantry nutrition evidence")
	pantryProfilePath := fs.String("pantry-profile", "", "optional structured pantry profile JSON path")
	pantryOut := fs.String("pantry-out", "", "optional pantry resolution JSON path")
	pantryConsumptionOut := fs.String("pantry-consumption-out", "", "optional pantry consumption plan JSON path")
	householdProfilePath := fs.String("household-profile", "", "optional household serving profile JSON path")
	servingPlanOut := fs.String("serving-plan-out", "", "optional serving plan JSON path")
	scaledMealPlanOut := fs.String("scaled-mealplan-out", "", "optional scaled meal plan JSON path")
	servings := fs.Float64("servings", 0, "adult-equivalent serving units per meal slot")
	adultServings := fs.Float64("adult-servings", 0, "adult-equivalent serving units from adults")
	childServings := fs.Float64("child-servings", 0, "adult-equivalent serving units from children")
	toddlerServings := fs.Float64("toddler-servings", 0, "adult-equivalent serving units from toddlers")
	strictServings := fs.Bool("strict-servings", false, "block cook readiness when recipes cannot be scaled safely")
	noServingScaling := fs.Bool("no-serving-scaling", false, "preserve current recipe quantities and skip serving scaling")
	allowLeftovers := fs.Bool("allow-leftovers", false, "allow explicit leftover serving units")
	noLeftovers := fs.Bool("no-leftovers", false, "disable leftover serving units")
	leftoverServings := fs.Float64("leftover-servings", 0, "extra cooked serving units per meal slot")
	roundPieceIngredients := fs.Bool("round-piece-ingredients", true, "round fractional piece ingredients up for shopping")
	noRoundPieceIngredients := fs.Bool("no-round-piece-ingredients", false, "keep fractional piece ingredient quantities")
	defaultPantry := fs.String("default-pantry", "none", "default pantry profile: none, minimal-spanish, or mediterranean-basic")
	noPantry := fs.Bool("no-pantry", false, "disable structured pantry resolution")
	allowAssumedPantry := fs.Bool("allow-assumed-pantry", false, "allow default pantry assumptions to satisfy cooking readiness")
	requireConfirmedPantry := fs.Bool("require-confirmed-pantry", false, "require pantry ingredients to be confirmed available")
	shopUnknownStaples := fs.Bool("shop-unknown-staples", false, "shop unknown pantry staples instead of assuming them")
	shopAllIngredients := fs.Bool("shop-all-ingredients", false, "ignore pantry coverage and shop every recipe ingredient")
	requireCookReady := fs.Bool("require-cook-ready", false, "return exit 20 when the basket is buildable but the meal plan is not cook-ready")
	requireSafeBasket := fs.Bool("require-safe-basket", false, "return exit 20 when artifacts are generated but the final basket is not safe to build")
	maxRecoverySearches := fs.Int("max-recovery-searches", 8, "maximum recovery alias searches per issue")
	allowRecipeSwap := fs.Bool("allow-recipe-swap", false, "allow recipe-swap recovery when product recovery fails")
	noRecipeSwap := fs.Bool("no-recipe-swap", false, "disable recipe-swap recovery")
	maxRecipeSwaps := fs.Int("max-recipe-swaps", 2, "maximum validated recipe swaps to apply")
	maxRecipeSwapCandidates := fs.Int("max-recipe-swap-candidates", 8, "maximum replacement recipe candidates per blocked slot")
	maxRecipeSwapChecks := fs.Int("max-recipe-swap-checks", 20, "maximum total replacement recipe shopping validations")
	allowIngredientSubstitution := fs.Bool("allow-ingredient-substitution", false, "allow ingredient substitution recovery")
	noIngredientSubstitution := fs.Bool("no-ingredient-substitution", false, "disable ingredient substitution recovery")
	optimizeBasket := fs.Bool("optimize-basket", false, "run audited basket product optimization after recovery and recipe swaps")
	noBasketOptimization := fs.Bool("no-basket-optimization", false, "disable audited basket product optimization")
	basketObjective := fs.String("basket-objective", "safe-balanced", "basket optimization objective: safe-balanced, lowest-price, or lowest-waste")
	maxOptimizationCandidates := fs.Int("max-optimization-candidates", 8, "maximum basket optimization candidates per ingredient")
	maxOptimizationSearches := fs.Int("max-optimization-searches-per-ingredient", 2, "maximum basket optimization searches per ingredient")
	maxTotalOptimizationChecks := fs.Int("max-total-optimization-checks", 80, "maximum total basket optimization candidate checks")
	minOptimizationSavingsCents := fs.Int("min-optimization-savings-cents", 50, "minimum savings in cents required before switching products")
	dealAware := fs.Bool("deal-aware", true, "score confidently parsed Alcampo offers during basket optimization")
	noDealAware := fs.Bool("no-deal-aware", false, "ignore Alcampo offers during basket optimization scoring")
	store := marketFlag(fs, "Alcampo region/store UUID to use for pricing")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	auditMode, err := food.NormalizeArtifactAuditMode(*auditModeFlag)
	if err != nil {
		return err
	}
	auditRequested := *auditOut != "" || auditMode != food.ArtifactAuditModeOff
	manifestRequested := *manifestOut != "" || auditRequested
	productEvidencePolicy := foodRunProductEvidencePolicy(*refreshProductEvidence, *noProductEvidenceRefresh, *productEvidenceOut, *runOut, *pdfOut, *basketOut, manifestRequested, auditRequested, *requireFreshProductEvidence, *maxProductEvidenceAgeSeconds, *allowProductEvidenceCache && !*noProductEvidenceCache, *strictQuantity)
	snapshotOpts, err := foodRunSnapshotOptions(*recordLiveSnapshot, *replayLiveSnapshot, *snapshotID, *snapshotStrict)
	if err != nil {
		return err
	}
	profile, pantry, err := loadFoodPlanningInputs()
	if err != nil {
		return err
	}
	resolvedPolicy, err := resolveAndSaveFoodPolicy(&profile, *policy, stderr)
	if err != nil {
		return err
	}
	mealRunIntent, err := foodRunIntent(*intentFile, *maxCookMinutes, []string(excludedIngredients), []string(excludedAllergens), []string(dietRules))
	if err != nil {
		return err
	}
	plan, err := buildSavedFoodPlan(profile, pantry, foodPlanOptions{
		Days:      *days,
		People:    *people,
		BudgetEUR: *budget,
		Meals:     *meals,
		OutPath:   *planOut,
	})
	if err != nil {
		return err
	}
	recipeIntakeOpts := foodRunRecipeIntakeOptions([]string(recipeFiles), []string(recipeDirs), []string(recipeURLs), *runOut, *pdfOut, *recipeIntakeOut, *recipeQualityOut, manifestRequested, auditRequested, *strictRecipeQuality, *strictServings, *requireRecipeSource, *requireRecipeImages, *recipeImageCacheDir, *noRecipeImageCache, *allowStructuredRecipeURL && !*noStructuredRecipeURL)
	var recipeIntakePlan *food.RecipeIntakePlan
	var recipeQualityReport *food.RecipeQualityReport
	if recipeIntakeOpts.Enabled {
		plan, recipeIntakePlan, recipeQualityReport, err = food.ApplyRecipeIntake(plan, recipeIntakeOpts)
		if err != nil {
			return err
		}
		plan = food.RebuildMealPlanPurchases(plan, profile, pantry)
		if *planOut != "" {
			if err := writeJSONPath(*planOut, plan); err != nil {
				return err
			}
		}
	}
	householdProfile, err := food.LoadHouseholdProfile(*householdProfilePath)
	if err != nil {
		return err
	}
	servingPolicy := foodRunServingPolicy(*householdProfilePath, *servings, *adultServings, *childServings, *toddlerServings, *strictServings, *noServingScaling, *allowLeftovers, *noLeftovers, *leftoverServings, *roundPieceIngredients && !*noRoundPieceIngredients)
	var servingPlan food.ServingPlan
	var scaledMealPlan food.ScaledMealPlan
	if servingPolicy.Enabled {
		plan, servingPlan, scaledMealPlan = food.ApplyServingScaling(plan, householdProfile, servingPolicy)
		plan = food.RebuildMealPlanPurchases(plan, profile, pantry)
		if *planOut != "" {
			if err := writeJSONPath(*planOut, plan); err != nil {
				return err
			}
		}
	}
	pantryProfile, err := food.LoadPantryProfile(*pantryProfilePath)
	if err != nil {
		return err
	}
	pantryProfile = food.MergePantryMemoryIntoProfile(pantryProfile, pantry)
	pantryPolicy := foodRunPantryPolicy(*defaultPantry, *noPantry, *allowAssumedPantry, *requireConfirmedPantry, *shopUnknownStaples, *shopAllIngredients)
	var pantryResolution food.PantryResolution
	if pantryPolicy.Enabled {
		plan, pantryResolution = food.ApplyPantryResolution(plan, pantryProfile, pantryPolicy)
		if servingPolicy.Enabled {
			food.StampPantryResolutionServing(&pantryResolution, servingPlan.ServingPlanFingerprint, scaledMealPlan.ScaledMealPlanFingerprint)
		}
		if *planOut != "" {
			if err := writeJSONPath(*planOut, plan); err != nil {
				return err
			}
		}
	}
	shop, client, err := shopFoodPlanWithClient(plan, profile, resolvedPolicy, *limit, *store, snapshotOpts)
	if err != nil {
		return err
	}
	ledgerOpts := foodRunLedgerOptions(client, *runOut, *pdfOut, *enrichProducts, *noProductDetailEnrichment, *detailCacheDir, *strictQuantity, *allowEstimatedVariableWeight, *allowLowConfidenceBasket)
	shop = food.EnrichShopResultProducts(context.Background(), shop, client, ledgerOpts)
	mealsList := splitList(*meals)
	artifact := food.NewFoodRunArtifact(plan, shop, "", *basketOut, mealsList)
	if recipeIntakeOpts.Enabled {
		artifact = food.AttachRecipeQuality(artifact, recipeIntakePlan, recipeQualityReport)
	}
	if servingPolicy.Enabled {
		artifact = food.AttachServingScaling(artifact, householdProfile, servingPlan, scaledMealPlan)
		ledgerOpts = foodRunLedgerOptionsWithServing(ledgerOpts, artifact)
	}
	if pantryPolicy.Enabled {
		artifact.PrePantryMealPlanFingerprint = food.MealPlanFingerprint(plan)
		artifact = food.AttachPantryResolution(artifact, pantryProfile, pantryResolution)
	}
	ledgerOpts = foodRunLedgerOptionsWithPantry(ledgerOpts, artifact)
	ledger := food.BuildQuantityLedger(plan, shop, ledgerOpts)
	artifact.QuantityLedger = &ledger
	basketSafety := food.BasketSafetyFromLedger(ledger, ledgerOpts)
	artifact.BasketSafety = &basketSafety
	recoveryOpts := foodRunRecoveryOptions(client, profile, resolvedPolicy, *limit, *runOut, *pdfOut, *recoverMissing, *noRecovery, *maxRecoverySearches, *allowRecipeSwap, *noRecipeSwap, *allowIngredientSubstitution, *noIngredientSubstitution, ledgerOpts)
	if recoveryOpts.Enabled {
		artifact = food.RecoverFoodRun(context.Background(), artifact, client, recoveryOpts)
		plan = artifact.MealPlan
		shop = artifact.Shop
		if artifact.QuantityLedger != nil {
			ledger = *artifact.QuantityLedger
		}
		if artifact.BasketSafety != nil {
			basketSafety = *artifact.BasketSafety
		}
	}
	recipeSwapAllowed := *allowRecipeSwap && !*noRecipeSwap
	nutritionPolicy := foodRunNutritionPolicy(*nutritionMode, *requireNutritionReady, *minNutritionLineCoverage, *minNutritionQuantityCoverage, *includePurchasedExcessNutrition, *allowPantryProfileNutrition, *allowBuiltinPantryNutrition)
	readinessPolicy := foodRunReadinessPolicy(*strictQuantity, *requireSafeBasket, *requireCookReady, *requireNutritionReady, *allowEstimatedVariableWeight, *allowLowConfidenceBasket, recipeSwapAllowed, recoveryOpts.AllowIngredientSubstitution, *strictRecipeQuality, *requireRecipeImages, *requireBudgetReady, *requireIntentReady, *requireFreshProductEvidence)
	recipeSwapOpts := foodRunRecipeSwapOptions(client, profile, pantry, householdProfile, servingPolicy, pantryProfile, pantryPolicy, resolvedPolicy, *limit, mealsList, recipeSwapAllowed, *maxRecipeSwaps, *maxRecipeSwapCandidates, *maxRecipeSwapChecks, ledgerOpts, recoveryOpts, readinessPolicy)
	if recipeSwapOpts.Enabled || *recipeSwapOut != "" {
		var swapPlan food.RecipeSwapPlan
		artifact, swapPlan, err = food.RepairFoodRunWithRecipeSwaps(context.Background(), artifact, client, recipeSwapOpts)
		if err != nil {
			return err
		}
		artifact.RecipeSwapPlan = &swapPlan
		plan = artifact.MealPlan
		shop = artifact.Shop
		if artifact.QuantityLedger != nil {
			ledger = *artifact.QuantityLedger
		}
		if artifact.BasketSafety != nil {
			basketSafety = *artifact.BasketSafety
		}
	}
	optimizationRequested := (*optimizeBasket || *basketOptimizationOut != "") && !*noBasketOptimization
	optimizationLedgerOpts := foodRunLedgerOptionsWithPantry(ledgerOpts, artifact)
	optimizationRecoveryOpts := recoveryOpts
	optimizationRecoveryOpts.LedgerOptions = optimizationLedgerOpts
	optimizationOpts := foodRunBasketOptimizationOptions(client, profile, resolvedPolicy, *limit, mealsList, optimizationRequested, *basketObjective, *maxOptimizationCandidates, *maxOptimizationSearches, *maxTotalOptimizationChecks, *minOptimizationSavingsCents, *dealAware && !*noDealAware, *strictQuantity, *allowEstimatedVariableWeight, *allowLowConfidenceBasket, optimizationLedgerOpts, optimizationRecoveryOpts, readinessPolicy)
	if optimizationRequested || *basketOptimizationOut != "" {
		var optimizationPlan food.BasketOptimizationPlan
		artifact, optimizationPlan, err = food.OptimizeBasket(context.Background(), artifact, client, optimizationOpts)
		if err != nil {
			return err
		}
		artifact.BasketOptimizationPlan = &optimizationPlan
		plan = artifact.MealPlan
		shop = artifact.Shop
		if artifact.QuantityLedger != nil {
			ledger = *artifact.QuantityLedger
		}
		if artifact.BasketSafety != nil {
			basketSafety = *artifact.BasketSafety
		}
	}
	if recipeIntakeOpts.Enabled {
		sourceByRecipe := food.RecipeSourcesFromQualityReport(recipeQualityReport)
		imageEvidence := food.CacheRecipeImagesInMealPlan(&artifact.MealPlan, recipeIntakeOpts)
		finalRecipeQuality := food.EvaluateRecipeQuality(artifact.MealPlan, sourceByRecipe, imageEvidence, recipeIntakeOpts.Policy)
		recipeQualityReport = &finalRecipeQuality
		if recipeIntakePlan != nil {
			recipeIntakePlan.ActiveRecipeCount = len(finalRecipeQuality.Items)
			recipeIntakePlan.RecipeSetFingerprint = finalRecipeQuality.RecipeSetFingerprint
			if finalRecipeQuality.Status == food.RecipeQualityFail && recipeIntakePlan.Status == food.RecipeIntakeComplete {
				recipeIntakePlan.Status = food.RecipeIntakeCompleteWithWarning
			}
		}
		artifact = food.AttachRecipeQuality(artifact, recipeIntakePlan, recipeQualityReport)
		artifact = food.RefreshFoodRunArtifact(artifact, mealsList)
		plan = artifact.MealPlan
	}
	budgetRepairRequested := (*repairBudget || *budgetRepairOut != "") && !*noBudgetRepair
	if budgetRepairRequested {
		baselineBudget := food.BuildBudgetDealReport(artifact)
		artifact = food.AttachBudgetDealReport(artifact, baselineBudget)
		budgetRepairOpts := foodRunBudgetRepairOptions(client, profile, resolvedPolicy, *limit, mealsList, budgetRepairRequested, *allowBudgetProductSwitches && !*noBudgetProductSwitches, *allowBudgetRecipeSwap && !*noBudgetRecipeSwap, *maxBudgetRepairProductSwitches, *maxBudgetRepairRecipeSwaps, *maxBudgetRepairCandidates, *maxBudgetRepairChecks, *budgetRepairMinSavingsCents, *dealAware && !*noDealAware, ledgerOpts, recoveryOpts, readinessPolicy)
		var budgetRepairPlan food.BudgetRepairPlan
		artifact, budgetRepairPlan, err = food.RepairFoodRunBudget(context.Background(), artifact, client, budgetRepairOpts)
		if err != nil {
			return err
		}
		artifact.BudgetRepairPlan = &budgetRepairPlan
		plan = artifact.MealPlan
		shop = artifact.Shop
		if artifact.QuantityLedger != nil {
			ledger = *artifact.QuantityLedger
		}
		if artifact.BasketSafety != nil {
			basketSafety = *artifact.BasketSafety
		}
	}
	if productEvidencePolicy.Enabled {
		finalLedgerOpts := foodRunLedgerOptionsWithPantry(ledgerOpts, artifact)
		finalLedgerOpts.EnrichProducts = productEvidencePolicy.RefreshProductEvidence && !*noProductDetailEnrichment
		if !productEvidencePolicy.AllowCacheEvidence {
			finalLedgerOpts.DetailCacheDir = ""
		}
		if finalLedgerOpts.EnrichProducts {
			artifact.Shop = food.EnrichShopResultProducts(context.Background(), artifact.Shop, client, finalLedgerOpts)
			artifact.Shop = foodRunMarkSnapshotProductEvidence(artifact.Shop, snapshotOpts)
			shop = artifact.Shop
			artifact = food.RefreshFoodRunArtifact(artifact, mealsList)
		}
		productEvidenceReport := food.BuildProductEvidenceReport(artifact, productEvidencePolicy)
		artifact = food.AttachProductEvidenceReport(artifact, productEvidenceReport)
		finalLedgerOpts.ProductEvidenceFingerprint = artifact.ProductEvidenceFingerprint
		ledger = food.BuildQuantityLedger(artifact.MealPlan, artifact.Shop, finalLedgerOpts)
		artifact.QuantityLedger = &ledger
		basketSafety = food.BasketSafetyFromLedger(ledger, finalLedgerOpts)
		artifact.BasketSafety = &basketSafety
		artifact = food.AttachProductEvidenceReport(artifact, *artifact.ProductEvidenceReport)
		plan = artifact.MealPlan
		shop = artifact.Shop
	}
	if nutritionPolicy.Enabled {
		nutritionLedger := food.BuildNutritionLedger(context.Background(), &artifact, pantryProfile, nutritionPolicy)
		artifact = food.AttachNutritionLedger(artifact, nutritionLedger)
	}
	budgetDealReport := food.BuildBudgetDealReport(artifact)
	artifact = food.AttachBudgetDealReport(artifact, budgetDealReport)
	readinessGate := food.ApplyReadinessGate(&artifact, readinessPolicy)
	if artifact.BudgetRepairPlan != nil {
		artifact = food.FinalizeBudgetRepairPlan(artifact, readinessGate)
		budgetDealReport = food.BuildBudgetDealReport(artifact)
		artifact = food.AttachBudgetDealReport(artifact, budgetDealReport)
		readinessGate = food.ApplyReadinessGate(&artifact, readinessPolicy)
	}
	if mealRunIntent != nil {
		artifact = food.AttachMealRunIntent(artifact, *mealRunIntent)
		constraintArtifact := artifact
		if strings.TrimSpace(constraintArtifact.PDFPath) == "" && strings.TrimSpace(*pdfOut) != "" {
			constraintArtifact.PDFPath = *pdfOut
		}
		constraintReport := food.BuildConstraintSatisfactionReport(constraintArtifact, *artifact.MealRunIntent)
		artifact = food.AttachConstraintSatisfactionReport(artifact, constraintReport)
		readinessGate = food.ApplyReadinessGate(&artifact, readinessPolicy)
		if *intentOut != "" {
			if err := writeJSONPath(*intentOut, artifact.MealRunIntent); err != nil {
				return err
			}
		}
	}
	if artifact.BasketSafety != nil {
		basketSafety = *artifact.BasketSafety
	}
	if *ledgerOut != "" {
		if err := writeJSONPath(*ledgerOut, ledger); err != nil {
			return err
		}
	}
	if *recoveryOut != "" && artifact.RecoveryPlan != nil {
		if err := writeJSONPath(*recoveryOut, artifact.RecoveryPlan); err != nil {
			return err
		}
	}
	if *recipeSwapOut != "" && artifact.RecipeSwapPlan != nil {
		if err := writeJSONPath(*recipeSwapOut, artifact.RecipeSwapPlan); err != nil {
			return err
		}
	}
	if *basketOptimizationOut != "" && artifact.BasketOptimizationPlan != nil {
		if err := writeJSONPath(*basketOptimizationOut, artifact.BasketOptimizationPlan); err != nil {
			return err
		}
	}
	if *budgetRepairOut != "" && artifact.BudgetRepairPlan != nil {
		if err := writeJSONPath(*budgetRepairOut, artifact.BudgetRepairPlan); err != nil {
			return err
		}
	}
	if *nutritionLedgerOut != "" && artifact.NutritionLedger != nil {
		if err := writeJSONPath(*nutritionLedgerOut, artifact.NutritionLedger); err != nil {
			return err
		}
	}
	if *budgetDealOut != "" && artifact.BudgetDealReport != nil {
		if err := writeJSONPath(*budgetDealOut, artifact.BudgetDealReport); err != nil {
			return err
		}
	}
	if *constraintReportOut != "" && artifact.ConstraintSatisfactionReport != nil {
		if err := writeJSONPath(*constraintReportOut, artifact.ConstraintSatisfactionReport); err != nil {
			return err
		}
	}
	if *productEvidenceOut != "" && artifact.ProductEvidenceReport != nil {
		if err := writeJSONPath(*productEvidenceOut, artifact.ProductEvidenceReport); err != nil {
			return err
		}
	}
	if *recipeIntakeOut != "" && artifact.RecipeIntakePlan != nil {
		if err := writeJSONPath(*recipeIntakeOut, artifact.RecipeIntakePlan); err != nil {
			return err
		}
	}
	if *recipeQualityOut != "" && artifact.RecipeQualityReport != nil {
		if err := writeJSONPath(*recipeQualityOut, artifact.RecipeQualityReport); err != nil {
			return err
		}
	}
	if *servingPlanOut != "" && artifact.ServingPlan != nil {
		if err := writeJSONPath(*servingPlanOut, artifact.ServingPlan); err != nil {
			return err
		}
	}
	if *scaledMealPlanOut != "" && artifact.ScaledMealPlan != nil {
		if err := writeJSONPath(*scaledMealPlanOut, artifact.ScaledMealPlan); err != nil {
			return err
		}
	}
	if *pantryOut != "" && artifact.PantryResolution != nil {
		if err := writeJSONPath(*pantryOut, artifact.PantryResolution); err != nil {
			return err
		}
	}
	if *pantryConsumptionOut != "" && artifact.PantryConsumptionPlan != nil {
		if err := writeJSONPath(*pantryConsumptionOut, artifact.PantryConsumptionPlan); err != nil {
			return err
		}
	}
	if *readinessOut != "" {
		if err := writeJSONPath(*readinessOut, readinessGate); err != nil {
			return err
		}
	}
	if err := writeFoodShopOutputsWithReadiness(shop, *shopOut, *basketOut, &readinessGate, artifact.RecipeSwapPlan, artifact.BasketOptimizationPlan, artifact.PantryResolution, artifact.ServingPlan, artifact.ScaledMealPlan, artifact.NutritionLedger, artifact.BudgetRepairPlan, artifact.BudgetDealReport); err != nil {
		return err
	}
	runPath, err := writeFoodRunArtifact(artifact, *runOut, *pdfOut, manifestRequested || auditRequested)
	if err != nil {
		return err
	}
	pdfPath, err := writeFoodRunPDF(runPath, *pdfOut)
	if err != nil {
		return err
	}
	if pdfPath != "" && runPath != "" {
		artifact.PDFPath = pdfPath
		if err := writeJSONPath(runPath, artifact); err != nil {
			return err
		}
	}
	if _, err := client.FinalizeLiveSnapshot(); err != nil {
		return err
	}
	snapshotSummary := foodManifestSnapshotFromClient(client.LiveSnapshotSummary())
	generationErr := readinessExitError(readinessGate, *requireSafeBasket, *requireCookReady, *requireNutritionReady, *requireBudgetReady, *requireIntentReady, *requireFreshProductEvidence)
	generationExitCode := ExitCode(generationErr)
	generationExitReason := readinessGate.ExitReason
	if generationErr != nil {
		generationExitReason = generationErr.Error()
	}
	manifestPath := *manifestOut
	if manifestPath == "" && auditRequested {
		manifestPath = defaultFoodRunManifestPath(runPath, *auditOut)
	}
	if manifestRequested {
		if runPath == "" {
			return errors.New("food run manifest or audit requires a run artifact; pass --run-out or --pdf-out")
		}
		manifest, err := food.BuildFoodRunManifest(artifact, food.FoodRunManifestOptions{
			Command:              "carrito food run",
			Args:                 append([]string{"food", "run"}, args...),
			StoreID:              *store,
			LiveMode:             true,
			AuditMode:            auditMode,
			GenerationExitCode:   generationExitCode,
			GenerationExitReason: generationExitReason,
			ArtifactPaths:        foodRunArtifactPaths(*planOut, *shopOut, runPath, pdfPath, *basketOut, *ledgerOut, *nutritionLedgerOut, *productEvidenceOut, *intentOut, *constraintReportOut, *budgetRepairOut, *budgetDealOut, *servingPlanOut, *scaledMealPlanOut, *pantryOut, *pantryConsumptionOut, *readinessOut, *recoveryOut, *recipeSwapOut, *basketOptimizationOut, *recipeIntakeOut, *recipeQualityOut),
			Snapshot:             snapshotSummary,
		})
		if err != nil {
			return err
		}
		if manifestPath != "" {
			if err := writeJSONPath(manifestPath, manifest); err != nil {
				return err
			}
		}
	}
	var auditReport *food.ArtifactAuditReport
	if auditRequested {
		report, err := food.AuditFoodRunArtifacts(food.ArtifactAuditOptions{
			Mode:         auditMode,
			ContextMode:  "hermes",
			ManifestPath: manifestPath,
			RunPath:      runPath,
			PDFPath:      pdfPath,
			BasketPath:   *basketOut,
		})
		if err != nil {
			return err
		}
		auditReport = &report
		if *auditOut != "" {
			if err := writeJSONPath(*auditOut, report); err != nil {
				return err
			}
		}
	}
	finalErr := generationErr
	if auditReport != nil {
		if err := artifactAuditExitError(*auditReport, auditMode); err != nil {
			finalErr = err
		}
	}
	res := struct {
		MealPlan                     food.MealPlan                      `json:"mealplan"`
		Shop                         food.ShopResult                    `json:"shop"`
		QuantityLedger               food.QuantityLedger                `json:"quantity_ledger"`
		BasketSafety                 food.BasketSafety                  `json:"basket_safety"`
		RecoveryPlan                 *food.RecoveryPlan                 `json:"recovery_plan,omitempty"`
		RecipeSwapPlan               *food.RecipeSwapPlan               `json:"recipe_swap_plan,omitempty"`
		BasketOptimizationPlan       *food.BasketOptimizationPlan       `json:"basket_optimization_plan,omitempty"`
		MealRunIntent                *food.MealRunIntent                `json:"meal_run_intent,omitempty"`
		ConstraintSatisfactionReport *food.ConstraintSatisfactionReport `json:"constraint_satisfaction_report,omitempty"`
		BudgetRepairPlan             *food.BudgetRepairPlan             `json:"budget_repair_plan,omitempty"`
		BudgetDealReport             *food.BudgetDealReport             `json:"budget_deal_report,omitempty"`
		ProductEvidenceReport        *food.ProductEvidenceReport        `json:"product_evidence_report,omitempty"`
		NutritionLedger              *food.NutritionLedger              `json:"nutrition_ledger,omitempty"`
		RecipeIntakePlan             *food.RecipeIntakePlan             `json:"recipe_intake_plan,omitempty"`
		RecipeQualityReport          *food.RecipeQualityReport          `json:"recipe_quality_report,omitempty"`
		ServingPlan                  *food.ServingPlan                  `json:"serving_plan,omitempty"`
		ScaledMealPlan               *food.ScaledMealPlan               `json:"scaled_mealplan,omitempty"`
		PantryResolution             *food.PantryResolution             `json:"pantry_resolution,omitempty"`
		ReadinessGate                food.ReadinessGate                 `json:"readiness_gate"`
		PDF                          string                             `json:"pdf,omitempty"`
		Basket                       string                             `json:"basket,omitempty"`
		Run                          string                             `json:"run,omitempty"`
		Ledger                       string                             `json:"ledger,omitempty"`
		Recovery                     string                             `json:"recovery,omitempty"`
		RecipeSwap                   string                             `json:"recipe_swap,omitempty"`
		BasketOptimization           string                             `json:"basket_optimization,omitempty"`
		Intent                       string                             `json:"intent,omitempty"`
		ConstraintReport             string                             `json:"constraint_report,omitempty"`
		BudgetRepair                 string                             `json:"budget_repair,omitempty"`
		BudgetDeal                   string                             `json:"budget_deal,omitempty"`
		ProductEvidence              string                             `json:"product_evidence,omitempty"`
		NutritionLedgerPath          string                             `json:"nutrition_ledger_path,omitempty"`
		RecipeIntake                 string                             `json:"recipe_intake,omitempty"`
		RecipeQuality                string                             `json:"recipe_quality,omitempty"`
		ServingPlanPath              string                             `json:"serving_plan_path,omitempty"`
		ScaledMealPlanPath           string                             `json:"scaled_mealplan_path,omitempty"`
		Pantry                       string                             `json:"pantry,omitempty"`
		PantryConsumption            string                             `json:"pantry_consumption,omitempty"`
		Readiness                    string                             `json:"readiness,omitempty"`
		Manifest                     string                             `json:"manifest,omitempty"`
		Audit                        string                             `json:"audit,omitempty"`
		Snapshot                     *food.ManifestSnapshotSummary      `json:"snapshot,omitempty"`
		ArtifactAudit                *food.ArtifactAuditReport          `json:"artifact_audit,omitempty"`
	}{MealPlan: plan, Shop: shop, QuantityLedger: ledger, BasketSafety: basketSafety, RecoveryPlan: artifact.RecoveryPlan, RecipeSwapPlan: artifact.RecipeSwapPlan, BasketOptimizationPlan: artifact.BasketOptimizationPlan, MealRunIntent: artifact.MealRunIntent, ConstraintSatisfactionReport: artifact.ConstraintSatisfactionReport, BudgetRepairPlan: artifact.BudgetRepairPlan, BudgetDealReport: artifact.BudgetDealReport, ProductEvidenceReport: artifact.ProductEvidenceReport, NutritionLedger: artifact.NutritionLedger, RecipeIntakePlan: artifact.RecipeIntakePlan, RecipeQualityReport: artifact.RecipeQualityReport, ServingPlan: artifact.ServingPlan, ScaledMealPlan: artifact.ScaledMealPlan, PantryResolution: artifact.PantryResolution, ReadinessGate: readinessGate, PDF: pdfPath, Basket: *basketOut, Run: runPath, Ledger: *ledgerOut, Recovery: *recoveryOut, RecipeSwap: *recipeSwapOut, BasketOptimization: *basketOptimizationOut, Intent: *intentOut, ConstraintReport: *constraintReportOut, BudgetRepair: *budgetRepairOut, BudgetDeal: *budgetDealOut, ProductEvidence: *productEvidenceOut, NutritionLedgerPath: *nutritionLedgerOut, RecipeIntake: *recipeIntakeOut, RecipeQuality: *recipeQualityOut, ServingPlanPath: *servingPlanOut, ScaledMealPlanPath: *scaledMealPlanOut, Pantry: *pantryOut, PantryConsumption: *pantryConsumptionOut, Readiness: *readinessOut, Manifest: manifestPath, Audit: *auditOut, Snapshot: snapshotSummary, ArtifactAudit: auditReport}
	if *jsonOut {
		if err := output.JSON(stdout, res); err != nil {
			return err
		}
		return finalErr
	}
	printMealPlan(stdout, plan)
	printShopResult(stdout, shop)
	printQuantityLedgerSummary(stdout, ledger, basketSafety)
	if artifact.ServingPlan != nil {
		printServingSummary(stdout, *artifact.ServingPlan, artifact.ScaledMealPlan)
	}
	if artifact.NutritionLedger != nil {
		printNutritionLedgerSummary(stdout, *artifact.NutritionLedger)
	}
	if artifact.RecipeQualityReport != nil {
		printRecipeQualitySummary(stdout, *artifact.RecipeQualityReport)
	}
	if artifact.BudgetRepairPlan != nil {
		printBudgetRepairPlan(stdout, *artifact.BudgetRepairPlan)
	}
	if artifact.ConstraintSatisfactionReport != nil {
		printConstraintSatisfaction(stdout, *artifact.ConstraintSatisfactionReport)
	}
	if artifact.BudgetDealReport != nil {
		printBudgetDealReport(stdout, *artifact.BudgetDealReport)
	}
	if artifact.ProductEvidenceReport != nil {
		printProductEvidenceReport(stdout, *artifact.ProductEvidenceReport)
	}
	printReadinessGate(stdout, readinessGate)
	if artifact.BasketOptimizationPlan != nil {
		printBasketOptimization(stdout, *artifact.BasketOptimizationPlan)
	}
	if runPath != "" {
		fmt.Fprintf(stdout, "run\t%s\n", runPath)
	}
	if *ledgerOut != "" {
		fmt.Fprintf(stdout, "ledger\t%s\n", *ledgerOut)
	}
	if *recoveryOut != "" {
		fmt.Fprintf(stdout, "recovery\t%s\n", *recoveryOut)
	}
	if *recipeSwapOut != "" {
		fmt.Fprintf(stdout, "recipe_swap\t%s\n", *recipeSwapOut)
	}
	if *basketOptimizationOut != "" {
		fmt.Fprintf(stdout, "basket_optimization\t%s\n", *basketOptimizationOut)
	}
	if *intentOut != "" {
		fmt.Fprintf(stdout, "intent\t%s\n", *intentOut)
	}
	if *constraintReportOut != "" {
		fmt.Fprintf(stdout, "constraint_report\t%s\n", *constraintReportOut)
	}
	if *budgetRepairOut != "" {
		fmt.Fprintf(stdout, "budget_repair\t%s\n", *budgetRepairOut)
	}
	if *budgetDealOut != "" {
		fmt.Fprintf(stdout, "budget_deal\t%s\n", *budgetDealOut)
	}
	if *productEvidenceOut != "" {
		fmt.Fprintf(stdout, "product_evidence\t%s\n", *productEvidenceOut)
	}
	if *nutritionLedgerOut != "" {
		fmt.Fprintf(stdout, "nutrition_ledger\t%s\n", *nutritionLedgerOut)
	}
	if *recipeIntakeOut != "" {
		fmt.Fprintf(stdout, "recipe_intake\t%s\n", *recipeIntakeOut)
	}
	if *recipeQualityOut != "" {
		fmt.Fprintf(stdout, "recipe_quality\t%s\n", *recipeQualityOut)
	}
	if *servingPlanOut != "" {
		fmt.Fprintf(stdout, "serving_plan\t%s\n", *servingPlanOut)
	}
	if *scaledMealPlanOut != "" {
		fmt.Fprintf(stdout, "scaled_mealplan\t%s\n", *scaledMealPlanOut)
	}
	if *pantryOut != "" {
		fmt.Fprintf(stdout, "pantry\t%s\n", *pantryOut)
	}
	if *pantryConsumptionOut != "" {
		fmt.Fprintf(stdout, "pantry_consumption\t%s\n", *pantryConsumptionOut)
	}
	if *readinessOut != "" {
		fmt.Fprintf(stdout, "readiness\t%s\n", *readinessOut)
	}
	if manifestPath != "" {
		fmt.Fprintf(stdout, "manifest\t%s\n", manifestPath)
	}
	printFoodSnapshotSummary(stdout, snapshotSummary)
	if auditReport != nil {
		printArtifactAudit(stdout, *auditReport)
	}
	if *auditOut != "" {
		fmt.Fprintf(stdout, "audit\t%s\n", *auditOut)
	}
	if pdfPath != "" {
		fmt.Fprintf(stdout, "pdf\t%s\n", pdfPath)
	}
	if *basketOut != "" {
		fmt.Fprintf(stdout, "basket\t%s\n", *basketOut)
	}
	return finalErr
}

func loadFoodPlanningInputs() (food.Profile, food.Pantry, error) {
	profile, err := food.LoadProfile()
	if err != nil {
		return food.Profile{}, food.Pantry{}, err
	}
	pantry, err := food.LoadPantry()
	if err != nil {
		return food.Profile{}, food.Pantry{}, err
	}
	return profile, pantry, nil
}

func buildSavedFoodPlan(profile food.Profile, pantry food.Pantry, opts foodPlanOptions) (food.MealPlan, error) {
	plan, err := food.GenerateMealPlan(profile, pantry, food.PlanOptions{
		Days:      opts.Days,
		People:    opts.People,
		BudgetEUR: opts.BudgetEUR,
		MealTypes: splitList(opts.Meals),
	})
	if err != nil {
		return food.MealPlan{}, err
	}
	if opts.OutPath != "" {
		plan.File = opts.OutPath
		if err := writeJSONPath(opts.OutPath, plan); err != nil {
			return food.MealPlan{}, err
		}
		return plan, nil
	}
	return food.SaveMealPlan(plan)
}

func resolveAndSaveFoodPolicy(profile *food.Profile, explicit string, stderr io.Writer) (string, error) {
	resolvedPolicy, updatedProfile, err := resolveFoodSelectionPolicy(profile, explicit, stderr)
	if err != nil {
		return "", err
	}
	if updatedProfile {
		if err := food.SaveProfile(*profile); err != nil {
			return "", err
		}
	}
	return resolvedPolicy, nil
}

func shopFoodPlan(plan food.MealPlan, profile food.Profile, policy string, limit int, store string) (food.ShopResult, error) {
	result, _, err := shopFoodPlanWithClient(plan, profile, policy, limit, store, nil)
	return result, err
}

func shopFoodPlanWithClient(plan food.MealPlan, profile food.Profile, policy string, limit int, store string, snapshotOpts *alcampo.LiveSnapshotOptions) (food.ShopResult, *alcampo.Client, error) {
	cfg, client, err := newClient(store)
	if err != nil {
		return food.ShopResult{}, nil, err
	}
	regionID, err := resolveMarket(cfg, store)
	if err != nil {
		return food.ShopResult{}, nil, err
	}
	client.RegionID = regionID
	if snapshotOpts != nil {
		snapshotOpts.StoreID = regionID
		snapshotOpts.StoreName = cfg.Defaults.RegionName
		if err := client.ConfigureLiveSnapshot(*snapshotOpts); err != nil {
			return food.ShopResult{}, nil, err
		}
	}
	result, err := food.ShopMealPlan(context.Background(), client, plan, profile, food.ShopOptions{Policy: policy, Limit: limit})
	return result, client, err
}

func foodRunSnapshotOptions(recordDir, replayDir, snapshotID string, strict bool) (*alcampo.LiveSnapshotOptions, error) {
	recordDir = strings.TrimSpace(recordDir)
	replayDir = strings.TrimSpace(replayDir)
	if recordDir == "" && replayDir == "" {
		return nil, nil
	}
	if recordDir != "" && replayDir != "" {
		return nil, fmt.Errorf("--record-live-snapshot and --replay-live-snapshot cannot be used together")
	}
	if replayDir != "" {
		return &alcampo.LiveSnapshotOptions{Mode: alcampo.LiveSnapshotModeReplay, Dir: replayDir, Strict: true}, nil
	}
	return &alcampo.LiveSnapshotOptions{Mode: alcampo.LiveSnapshotModeRecord, Dir: recordDir, SnapshotID: snapshotID, Strict: strict}, nil
}

func foodRunMarkSnapshotProductEvidence(shop food.ShopResult, snapshotOpts *alcampo.LiveSnapshotOptions) food.ShopResult {
	if snapshotOpts == nil || snapshotOpts.Mode != alcampo.LiveSnapshotModeReplay {
		return shop
	}
	for i := range shop.SelectedProducts {
		if shop.SelectedProducts[i].ProductEvidenceSource == "alcampo_product_detail" {
			shop.SelectedProducts[i].ProductEvidenceSource = food.ProductEvidenceSourceSnapshotReplay
		}
	}
	return shop
}

func foodRunLedgerOptions(client *alcampo.Client, runOut, pdfOut string, enrichProducts, noProductDetailEnrichment bool, detailCacheDir string, strictQuantity, allowEstimatedVariableWeight, allowLowConfidenceBasket bool) food.QuantityLedgerOptions {
	shouldEnrich := enrichProducts || (runOut != "" || pdfOut != "")
	if noProductDetailEnrichment {
		shouldEnrich = false
	}
	if detailCacheDir == "" && shouldEnrich {
		if dir, err := food.Dir(); err == nil {
			detailCacheDir = filepath.Join(dir, "product-cache")
		}
	}
	storeID := ""
	if client != nil {
		storeID = client.RegionID
	}
	return food.QuantityLedgerOptions{
		StoreID:                      storeID,
		EnrichProducts:               shouldEnrich,
		StrictQuantity:               strictQuantity,
		AllowEstimatedVariableWeight: allowEstimatedVariableWeight,
		AllowLowConfidenceBasket:     allowLowConfidenceBasket,
		DetailCacheDir:               detailCacheDir,
	}
}

func foodRunRecoveryOptions(client *alcampo.Client, profile food.Profile, policy string, searchLimit int, runOut, pdfOut string, recoverMissing, noRecovery bool, maxSearches int, allowRecipeSwap, noRecipeSwap, allowIngredientSubstitution, noIngredientSubstitution bool, ledgerOpts food.QuantityLedgerOptions) food.RecoveryOptions {
	enabled := recoverMissing || runOut != "" || pdfOut != ""
	if noRecovery {
		enabled = false
	}
	storeID := ""
	if client != nil {
		storeID = client.RegionID
	}
	recipeSwap := allowRecipeSwap || (runOut != "" || pdfOut != "")
	if noRecipeSwap {
		recipeSwap = false
	}
	ingredientSubstitution := allowIngredientSubstitution
	if noIngredientSubstitution {
		ingredientSubstitution = false
	}
	return food.RecoveryOptions{
		Enabled:                     enabled,
		StoreID:                     storeID,
		Policy:                      policy,
		Profile:                     profile,
		SearchLimit:                 searchLimit,
		MaxSearches:                 maxSearches,
		AllowIngredientSubstitution: ingredientSubstitution,
		AllowRecipeSwap:             recipeSwap,
		LedgerOptions:               ledgerOpts,
	}
}

func foodRunPantryPolicy(defaultPantry string, noPantry, allowAssumed, requireConfirmed, shopUnknownStaples, shopAllIngredients bool) food.PantryPolicy {
	enabled := !noPantry && (defaultPantry != "" && defaultPantry != "none" || allowAssumed || requireConfirmed || shopUnknownStaples || shopAllIngredients)
	policy := food.DefaultPantryPolicy(enabled, defaultPantry)
	if noPantry {
		policy.Enabled = false
	}
	if allowAssumed {
		policy.AllowAssumedPantry = true
	}
	if requireConfirmed {
		policy.RequireConfirmedPantry = true
		policy.AllowAssumedPantry = false
	}
	if shopUnknownStaples {
		policy.ShopUnknownStaples = true
	}
	if shopAllIngredients {
		policy.ShopAllIngredients = true
		policy.Enabled = false
	}
	return policy
}

func foodRunServingPolicy(householdProfilePath string, servings, adultServings, childServings, toddlerServings float64, strictServings, noServingScaling, allowLeftovers, noLeftovers bool, leftoverServings float64, roundPieceIngredients bool) food.ServingPolicy {
	enabled := !noServingScaling && (strings.TrimSpace(householdProfilePath) != "" || servings > 0 || adultServings > 0 || childServings > 0 || toddlerServings > 0 || strictServings || leftoverServings > 0)
	policy := food.DefaultServingPolicy(enabled)
	policy.DefaultServings = servings
	policy.AdultServings = adultServings
	policy.ChildServings = childServings
	policy.ToddlerServings = toddlerServings
	policy.StrictServingScaling = strictServings
	policy.RequireBaseRecipeServings = strictServings
	policy.AllowLeftovers = allowLeftovers && !noLeftovers
	if leftoverServings > 0 {
		policy.AllowLeftovers = !noLeftovers
		policy.LeftoverServings = leftoverServings
	}
	policy.RoundPieceIngredients = roundPieceIngredients
	if noServingScaling {
		policy.Enabled = false
	}
	return policy
}

func foodRunRecipeIntakeOptions(files, dirs, urls []string, runOut, pdfOut, intakeOut, qualityOut string, manifestRequested, auditRequested, strictRecipeQuality, strictServings, requireSource, requireImages bool, imageCacheDir string, noImageCache, allowStructuredURL bool) food.RecipeIntakeOptions {
	policy := food.DefaultRecipeIntakePolicy()
	policy.StrictRecipeQuality = strictRecipeQuality
	policy.RequireBaseServings = strictRecipeQuality && strictServings
	policy.RequireParseableRequiredAmounts = false
	policy.RequireRecipeSource = requireSource
	policy.RequireRecipeImages = requireImages
	policy.AllowStructuredURLImport = allowStructuredURL
	policy.AllowUnstructuredScrape = false
	policy.CacheImages = !noImageCache
	enabled := len(files) > 0 || len(dirs) > 0 || len(urls) > 0 || runOut != "" || pdfOut != "" || intakeOut != "" || qualityOut != "" || manifestRequested || auditRequested || strictRecipeQuality || requireSource || requireImages
	return food.RecipeIntakeOptions{
		Enabled:             enabled,
		RecipeFiles:         append([]string(nil), files...),
		RecipeDirs:          append([]string(nil), dirs...),
		RecipeURLs:          append([]string(nil), urls...),
		ImageCacheDir:       imageCacheDir,
		InjectImportedMeals: len(files) > 0 || len(dirs) > 0 || len(urls) > 0,
		Policy:              policy,
	}
}

func foodRunLedgerOptionsWithServing(opts food.QuantityLedgerOptions, artifact food.FoodRunArtifact) food.QuantityLedgerOptions {
	opts.ServingPlanFingerprint = artifact.ServingPlanFingerprint
	opts.ScaledMealPlanFingerprint = artifact.ScaledMealPlanFingerprint
	return opts
}

func foodRunLedgerOptionsWithPantry(opts food.QuantityLedgerOptions, artifact food.FoodRunArtifact) food.QuantityLedgerOptions {
	opts = foodRunLedgerOptionsWithServing(opts, artifact)
	opts.PantryResolution = artifact.PantryResolution
	opts.PantryResolutionFingerprint = artifact.PantryResolutionFingerprint
	opts.ShopRequirementsFingerprint = artifact.ShopRequirementsFingerprint
	return opts
}

func foodRunReadinessPolicy(strictQuantity, requireSafeBasket, requireCookReady, requireNutritionReady, allowEstimatedVariableWeight, allowLowConfidenceBasket, allowRecipeSwap, allowIngredientSubstitution, strictRecipeQuality, requireRecipeImages, requireBudgetReady, requireIntentReady, requireFreshProductEvidence bool) food.ReadinessPolicy {
	return food.ReadinessPolicy{
		StrictQuantity:               strictQuantity,
		RequireSafeBasket:            requireSafeBasket,
		RequireCookReady:             requireCookReady,
		RequireNutritionReady:        requireNutritionReady,
		AllowEstimatedVariableWeight: allowEstimatedVariableWeight,
		AllowLowConfidenceBasket:     allowLowConfidenceBasket,
		AllowRecipeSwap:              allowRecipeSwap,
		AllowIngredientSubstitution:  allowIngredientSubstitution,
		StrictRecipeQuality:          strictRecipeQuality,
		RequireRecipeImages:          requireRecipeImages,
		RequireBudgetReady:           requireBudgetReady,
		RequireIntentReady:           requireIntentReady,
		RequireFreshProductEvidence:  requireFreshProductEvidence,
	}
}

func foodRunProductEvidencePolicy(refresh, noRefresh bool, productEvidenceOut, runOut, pdfOut, basketOut string, manifestRequested, auditRequested, requireFresh bool, maxAgeSeconds int, allowCache bool, strictQuantity bool) food.ProductEvidencePolicy {
	policy := food.DefaultProductEvidencePolicy(strictQuantity)
	policy.RequireFreshEvidence = requireFresh
	policy.MaxEvidenceAgeSeconds = maxAgeSeconds
	policy.AllowCacheEvidence = allowCache
	policy.Enabled = refresh || productEvidenceOut != "" || runOut != "" || pdfOut != "" || basketOut != "" || manifestRequested || auditRequested || requireFresh
	policy.RefreshProductEvidence = policy.Enabled
	if noRefresh {
		policy.RefreshProductEvidence = false
	}
	return policy
}

func foodRunNutritionPolicy(mode string, requireReady bool, minLineCoverage, minQuantityCoverage float64, includePurchasedExcess, allowPantryProfile, allowBuiltinPantry bool) food.NutritionPolicy {
	policy := food.DefaultNutritionPolicy(mode)
	policy.RequireNutritionReady = requireReady
	policy.MinLineCoverageRatio = clampRatio(minLineCoverage)
	policy.MinQuantityCoverageRatio = clampRatio(minQuantityCoverage)
	policy.IncludePurchasedExcess = includePurchasedExcess
	policy.AllowPantryProfileNutrition = allowPantryProfile
	policy.AllowBuiltinPantryNutrition = allowBuiltinPantry
	return policy
}

func clampRatio(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func foodRunRecipeSwapOptions(client *alcampo.Client, profile food.Profile, pantry food.Pantry, householdProfile food.HouseholdProfile, servingPolicy food.ServingPolicy, pantryProfile food.PantryProfile, pantryPolicy food.PantryPolicy, policy string, searchLimit int, meals []string, enabled bool, maxSwaps, maxCandidates, maxChecks int, ledgerOpts food.QuantityLedgerOptions, recoveryOpts food.RecoveryOptions, readinessPolicy food.ReadinessPolicy) food.RecipeSwapOptions {
	storeID := ""
	if client != nil {
		storeID = client.RegionID
	}
	ledgerOpts.StoreID = storeID
	recoveryOpts.AllowRecipeSwap = false
	recoveryOpts.LedgerOptions = ledgerOpts
	return food.RecipeSwapOptions{
		Enabled:          enabled,
		Profile:          profile,
		Pantry:           pantry,
		HouseholdProfile: householdProfile,
		ServingPolicy:    servingPolicy,
		PantryProfile:    pantryProfile,
		PantryPolicy:     pantryPolicy,
		SelectionPolicy:  policy,
		SearchLimit:      searchLimit,
		Meals:            meals,
		LedgerOptions:    ledgerOpts,
		RecoveryOptions:  recoveryOpts,
		ReadinessPolicy:  readinessPolicy,
		Policy: food.RecipeSwapPolicy{
			AllowRecipeSwap:         enabled,
			MaxAppliedSwaps:         maxSwaps,
			MaxCandidatesPerSlot:    maxCandidates,
			MaxTotalCandidateChecks: maxChecks,
		},
	}
}

func foodRunBasketOptimizationOptions(client *alcampo.Client, profile food.Profile, policy string, searchLimit int, meals []string, enabled bool, objective string, maxCandidates, maxSearches, maxChecks, minSavingsCents int, dealAware, strictQuantity, allowEstimatedVariableWeight, allowLowConfidenceBasket bool, ledgerOpts food.QuantityLedgerOptions, recoveryOpts food.RecoveryOptions, readinessPolicy food.ReadinessPolicy) food.BasketOptimizationOptions {
	storeID := ""
	if client != nil {
		storeID = client.RegionID
	}
	ledgerOpts.StoreID = storeID
	recoveryOpts.AllowRecipeSwap = false
	recoveryOpts.LedgerOptions = ledgerOpts
	return food.BasketOptimizationOptions{
		Enabled:         enabled,
		Profile:         profile,
		SelectionPolicy: policy,
		SearchLimit:     searchLimit,
		Meals:           meals,
		LedgerOptions:   ledgerOpts,
		RecoveryOptions: recoveryOpts,
		ReadinessPolicy: readinessPolicy,
		Policy: food.BasketOptimizationPolicy{
			Enabled:                        enabled,
			Objective:                      normalizeBasketOptimizationObjective(objective),
			MaxCandidatesPerIngredient:     maxCandidates,
			MaxSearchesPerIngredient:       maxSearches,
			MaxTotalCandidateChecks:        maxChecks,
			DealAware:                      dealAware,
			StrictQuantity:                 strictQuantity,
			AllowEstimatedVariableWeight:   allowEstimatedVariableWeight,
			AllowLowConfidenceBasket:       allowLowConfidenceBasket,
			RequireNoReadinessRegression:   true,
			MinSavingsCentsToSwitch:        minSavingsCents,
			MinWasteReductionRatioToSwitch: 0.2,
			PreferProductImages:            true,
			PreferNutritionLabels:          true,
		},
	}
}

func foodRunBudgetRepairOptions(client *alcampo.Client, profile food.Profile, policy string, searchLimit int, meals []string, enabled, allowProductSwitches, allowBudgetRecipeSwap bool, maxProductSwitches, maxRecipeSwaps, maxCandidates, maxChecks, minSavingsCents int, dealAware bool, ledgerOpts food.QuantityLedgerOptions, recoveryOpts food.RecoveryOptions, readinessPolicy food.ReadinessPolicy) food.BudgetRepairOptions {
	storeID := ""
	if client != nil {
		storeID = client.RegionID
	}
	ledgerOpts.StoreID = storeID
	recoveryOpts.AllowRecipeSwap = false
	recoveryOpts.LedgerOptions = ledgerOpts
	repairPolicy := food.DefaultBudgetRepairPolicy(enabled)
	repairPolicy.AllowProductSwitches = allowProductSwitches
	repairPolicy.AllowBudgetRecipeSwap = allowBudgetRecipeSwap
	repairPolicy.MaxProductSwitches = maxProductSwitches
	repairPolicy.MaxRecipeSwaps = maxRecipeSwaps
	repairPolicy.MaxCandidatesPerDriver = maxCandidates
	repairPolicy.MaxTotalChecks = maxChecks
	repairPolicy.MinSavingsCentsToApply = minSavingsCents
	repairPolicy.DealAware = dealAware
	repairPolicy.RequireNoNutritionReadinessRegression = readinessPolicy.RequireNutritionReady
	return food.BudgetRepairOptions{
		Enabled:         enabled,
		Policy:          repairPolicy,
		Profile:         profile,
		SelectionPolicy: policy,
		SearchLimit:     searchLimit,
		Meals:           meals,
		LedgerOptions:   ledgerOpts,
		RecoveryOptions: recoveryOpts,
		ReadinessPolicy: readinessPolicy,
		Source:          "cli",
	}
}

func foodRunIntent(path string, maxCookMinutes int, excludedIngredients, excludedAllergens, dietRules []string) (*food.MealRunIntent, error) {
	var intent food.MealRunIntent
	hasIntent := false
	if strings.TrimSpace(path) != "" {
		loaded, err := food.LoadMealRunIntent(path)
		if err != nil {
			return nil, err
		}
		intent = loaded
		hasIntent = true
	}
	hasCLIIntent := maxCookMinutes > 0 || len(excludedIngredients) > 0 || len(excludedAllergens) > 0 || len(dietRules) > 0
	if hasCLIIntent && !hasIntent {
		intent = food.MealRunIntent{SchemaVersion: "1", Source: "cli"}
		hasIntent = true
	}
	if !hasIntent {
		return nil, nil
	}
	if maxCookMinutes > 0 {
		if intent.Cooking == nil {
			intent.Cooking = &food.CookingIntent{}
		}
		value := maxCookMinutes
		intent.Cooking.MaxTotalMinutes = &value
		intent.Cooking.RequireTimeEvidence = true
	}
	if len(excludedIngredients) > 0 || len(excludedAllergens) > 0 || len(dietRules) > 0 {
		if intent.Diet == nil {
			intent.Diet = &food.DietIntent{}
		}
		intent.Diet.ExcludedIngredients = append(intent.Diet.ExcludedIngredients, excludedIngredients...)
		intent.Diet.ExcludedAllergens = append(intent.Diet.ExcludedAllergens, excludedAllergens...)
		intent.Diet.DietRules = append(intent.Diet.DietRules, dietRules...)
		intent.Diet.RequireDeterministicCheck = true
	}
	intent = food.NormalizeMealRunIntent(intent)
	return &intent, nil
}

func normalizeBasketOptimizationObjective(value string) string {
	switch strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, "-", "_"))) {
	case food.BasketObjectiveLowestPrice:
		return food.BasketObjectiveLowestPrice
	case food.BasketObjectiveLowestWaste:
		return food.BasketObjectiveLowestWaste
	default:
		return food.BasketObjectiveSafeBalanced
	}
}

func writeFoodShopOutputs(shop food.ShopResult, outPath, basketOut string) error {
	return writeFoodShopOutputsWithReadiness(shop, outPath, basketOut, nil, nil, nil, nil, nil, nil, nil, nil, nil)
}

func writeFoodShopOutputsWithReadiness(shop food.ShopResult, outPath, basketOut string, readiness *food.ReadinessGate, swapPlan *food.RecipeSwapPlan, optimizationPlan *food.BasketOptimizationPlan, pantryResolution *food.PantryResolution, servingPlan *food.ServingPlan, scaledMealPlan *food.ScaledMealPlan, nutritionLedger *food.NutritionLedger, budgetRepair *food.BudgetRepairPlan, budgetDeal *food.BudgetDealReport) error {
	if outPath != "" {
		if err := writeJSONPath(outPath, shop); err != nil {
			return err
		}
	}
	if basketOut != "" {
		lines := shop.BasketLines
		if readiness != nil {
			lines = food.ReadinessBasketLinesWithRecipeSwapOptimizationPantryServingNutritionBudgetRepairAndBudgetDeal(lines, readiness, swapPlan, optimizationPlan, pantryResolution, servingPlan, scaledMealPlan, nutritionLedger, budgetRepair, budgetDeal)
		}
		if err := writeBasketLinesPath(basketOut, lines); err != nil {
			return err
		}
	}
	return nil
}

func readinessExitError(gate food.ReadinessGate, requireSafeBasket, requireCookReady, requireNutritionReady, requireBudgetReady, requireIntentReady, requireFreshProductEvidence bool) error {
	if requireCookReady && !gate.SafeToCook {
		return ExitError{Code: food.ReadinessExitBlocked, Err: fmt.Errorf("food run generated diagnostic artifacts but meal plan is not cook-ready: %s", gate.ExitReason)}
	}
	if requireNutritionReady && !gate.SafeToReportNutrition {
		return ExitError{Code: food.ReadinessExitBlocked, Err: fmt.Errorf("food run generated diagnostic artifacts but nutrition reporting is not ready: %s", gate.ExitReason)}
	}
	if requireBudgetReady && !gate.SafeToReportBudget {
		return ExitError{Code: food.ReadinessExitBlocked, Err: fmt.Errorf("food run generated diagnostic artifacts but budget reporting is not ready: %s", gate.ExitReason)}
	}
	if requireIntentReady && !gate.SafeToSatisfyIntent {
		return ExitError{Code: food.ReadinessExitBlocked, Err: fmt.Errorf("food run generated diagnostic artifacts but request constraints are not satisfied: %s", gate.ExitReason)}
	}
	if requireFreshProductEvidence && !gate.SafeToUseProductEvidence {
		return ExitError{Code: food.ReadinessExitBlocked, Err: fmt.Errorf("food run generated diagnostic artifacts but fresh product evidence is not ready: %s", gate.ExitReason)}
	}
	if !requireSafeBasket || gate.SafeToBuild {
		return nil
	}
	return ExitError{Code: food.ReadinessExitBlocked, Err: fmt.Errorf("food run generated diagnostic artifacts but basket is not safe to build: %s", gate.ExitReason)}
}

func writeFoodRunArtifact(artifact food.FoodRunArtifact, runOut, pdfOut string, force bool) (string, error) {
	path := runOut
	if path == "" && (pdfOut != "" || force) {
		dir, err := food.MealPlansDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(dir, artifact.MealPlan.ID+"-run.json")
	}
	if path == "" {
		return "", nil
	}
	if err := writeJSONPath(path, artifact); err != nil {
		return "", err
	}
	return path, nil
}

func writeFoodRunPDF(runPath, pdfOut string) (string, error) {
	if pdfOut == "" {
		return "", nil
	}
	if runPath == "" {
		return "", errors.New("food run PDF requires a combined run artifact source")
	}
	if err := food.WritePDFFromJSONFile(runPath, pdfOut); err != nil {
		return "", err
	}
	return pdfOut, nil
}
