package food

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

func NewFoodRunArtifact(plan MealPlan, shop ShopResult, pdfPath, basketPath string, meals []string) FoodRunArtifact {
	warnings := foodRunWarnings(plan, shop)
	fingerprint := MealPlanFingerprint(plan)
	selectionFingerprint := ProductSelectionFingerprint(shop)
	recipeSetFingerprint := RecipeSetFingerprint(plan)
	return FoodRunArtifact{
		SchemaVersion:               1,
		Kind:                        "food_run",
		CreatedAt:                   nowStamp(),
		MealPlanFingerprint:         fingerprint,
		ProductSelectionFingerprint: selectionFingerprint,
		RecipeSetFingerprint:        recipeSetFingerprint,
		MealPlan:                    plan,
		Shop:                        shop,
		IngredientLinks:             foodRunIngredientLinks(plan, shop),
		PDFPath:                     pdfPath,
		BasketPath:                  basketPath,
		People:                      plan.People,
		Days:                        len(plan.Days),
		Meals:                       append([]string(nil), meals...),
		Provenance: []DataProvenance{
			{
				Scope:      "recipe",
				Source:     "recipe_db",
				Confidence: "medium",
				Message:    "Meal plan recipes come from the local Carrito recipe library and user recipe overrides.",
			},
			{
				Scope:      "product",
				Source:     "alcampo_search",
				Confidence: "medium",
				Message:    "Shopping selections come from current Alcampo search results for the configured market.",
			},
		},
		Warnings: warnings,
	}
}

func AttachPantryResolution(artifact FoodRunArtifact, profile PantryProfile, resolution PantryResolution) FoodRunArtifact {
	if artifact.ServingPlanFingerprint != "" || artifact.ScaledMealPlanFingerprint != "" {
		StampPantryResolutionServing(&resolution, artifact.ServingPlanFingerprint, artifact.ScaledMealPlanFingerprint)
	}
	artifact.PantryProfileFingerprint = resolution.PantryProfileFingerprint
	if artifact.PantryProfileFingerprint == "" {
		artifact.PantryProfileFingerprint = PantryProfileFingerprint(profile)
	}
	artifact.PantryResolutionFingerprint = resolution.PantryResolutionFingerprint
	artifact.ShopRequirementsFingerprint = resolution.ShopRequirementsFingerprint
	artifact.PantryProfileSummary = &PantryProfileSummary{
		DefaultProfile: profile.DefaultProfile,
		ItemCount:      len(profile.Items),
	}
	summary := pantryProfileSummary(profile)
	artifact.PantryProfileSummary = &summary
	artifact.PantryResolution = &resolution
	artifact.PantryConsumptionPlan = pantryConsumptionPlan(&resolution)
	return artifact
}

func AttachRecipeQuality(artifact FoodRunArtifact, intake *RecipeIntakePlan, report *RecipeQualityReport) FoodRunArtifact {
	if intake != nil {
		intakeCopy := *intake
		artifact.RecipeIntakePlan = &intakeCopy
	}
	if report != nil {
		reportCopy := *report
		artifact.RecipeQualityReport = &reportCopy
		artifact.RecipeSetFingerprint = reportCopy.RecipeSetFingerprint
		artifact.RecipeQualityFingerprint = reportCopy.RecipeQualityFingerprint
		artifact.RecipeImageFingerprint = reportCopy.RecipeImageFingerprint
	}
	if artifact.RecipeSetFingerprint == "" {
		artifact.RecipeSetFingerprint = RecipeSetFingerprint(artifact.MealPlan)
	}
	return artifact
}

func AttachBudgetDealReport(artifact FoodRunArtifact, report BudgetDealReport) FoodRunArtifact {
	if report.MealPlanFingerprint == "" {
		report.MealPlanFingerprint = firstNonEmptyString(artifact.MealPlanFingerprint, MealPlanFingerprint(artifact.MealPlan))
	}
	if report.ProductSelectionFingerprint == "" {
		report.ProductSelectionFingerprint = firstNonEmptyString(artifact.ProductSelectionFingerprint, ProductSelectionFingerprint(artifact.Shop))
	}
	if report.ServingPlanFingerprint == "" {
		report.ServingPlanFingerprint = servingPlanFingerprintFromRun(&artifact)
	}
	if report.ScaledMealPlanFingerprint == "" {
		report.ScaledMealPlanFingerprint = scaledMealPlanFingerprintFromRun(&artifact)
	}
	if report.PantryResolutionFingerprint == "" {
		report.PantryResolutionFingerprint = pantryResolutionFingerprintFromRun(&artifact)
	}
	if report.ShopRequirementsFingerprint == "" {
		report.ShopRequirementsFingerprint = shopRequirementsFingerprintFromRun(&artifact)
	}
	report.BudgetDealFingerprint = BudgetDealFingerprint(report)
	artifact.BudgetDealReport = &report
	artifact.BudgetDealFingerprint = report.BudgetDealFingerprint
	return artifact
}

func MealPlanFingerprint(plan MealPlan) string {
	type canonicalIngredient struct {
		Name       string  `json:"name"`
		SearchTerm string  `json:"search_term,omitempty"`
		Quantity   float64 `json:"quantity,omitempty"`
		Unit       string  `json:"unit,omitempty"`
		Optional   bool    `json:"optional,omitempty"`
	}
	type canonicalMeal struct {
		Day         int                   `json:"day"`
		MealSlot    string                `json:"meal_slot"`
		RecipeID    string                `json:"recipe_id"`
		RecipeTitle string                `json:"recipe_title"`
		Servings    int                   `json:"servings"`
		Ingredients []canonicalIngredient `json:"ingredients"`
	}
	var meals []canonicalMeal
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			entry := canonicalMeal{
				Day:         day.Day,
				MealSlot:    normalizeKey(meal.Type),
				RecipeID:    strings.TrimSpace(meal.Recipe.ID),
				RecipeTitle: strings.TrimSpace(meal.Recipe.Title),
				Servings:    meal.Recipe.Servings,
			}
			for _, ingredient := range meal.Recipe.Ingredients {
				entry.Ingredients = append(entry.Ingredients, canonicalIngredient{
					Name:       normalizeKey(ingredient.Name),
					SearchTerm: normalizeKey(ingredient.SearchTerm),
					Quantity:   roundQty(ingredient.Quantity),
					Unit:       normalizeUnit(ingredient.Unit),
					Optional:   ingredient.Optional,
				})
			}
			meals = append(meals, entry)
		}
	}
	data, err := json.Marshal(meals)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func ProductSelectionFingerprint(shop ShopResult) string {
	type canonicalSelection struct {
		IngredientKey    string  `json:"ingredient_key,omitempty"`
		ProductID        string  `json:"product_id,omitempty"`
		SKU              string  `json:"sku,omitempty"`
		ProductName      string  `json:"product_name,omitempty"`
		ProductURL       string  `json:"product_url,omitempty"`
		PackageQuantity  float64 `json:"package_quantity,omitempty"`
		PackageUnit      string  `json:"package_unit,omitempty"`
		PackageCount     int     `json:"package_count,omitempty"`
		PurchaseQuantity string  `json:"purchase_quantity,omitempty"`
		BasketQuantity   string  `json:"basket_quantity,omitempty"`
		Error            string  `json:"error,omitempty"`
	}
	basketQtyByRef := map[string]string{}
	for _, line := range shop.BasketLines {
		ref, qty, ok := parseBasketLine(line)
		if ok {
			basketQtyByRef[ref] = qty
		}
	}
	var rows []canonicalSelection
	for _, selected := range shop.SelectedProducts {
		pkgQty := selected.Product.PackageQuantity
		pkgUnit := normalizeUnit(selected.Product.PackageUnit)
		if pkgQty <= 0 || pkgUnit == "" {
			ev := ParsePackageEvidence(selected.Product.Size, selected.Product.Name)
			if ev.NetQuantity != nil {
				pkgQty = ev.NetQuantity.Expected.BaseValue
				pkgUnit = ev.NetQuantity.Expected.BaseUnit
			}
		}
		ref := firstNonEmptyString(selected.Product.SKU, selected.Product.ID)
		rows = append(rows, canonicalSelection{
			IngredientKey:    normalizeKey(selected.Ingredient.Name),
			ProductID:        selected.Product.ID,
			SKU:              selected.Product.SKU,
			ProductName:      normalizeKey(selected.Product.Name),
			ProductURL:       selected.Product.URL,
			PackageQuantity:  roundQty(pkgQty),
			PackageUnit:      pkgUnit,
			PackageCount:     selected.PackageCount,
			PurchaseQuantity: selected.PurchaseQuantity,
			BasketQuantity:   basketQtyByRef[ref],
			Error:            selected.Error,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].IngredientKey != rows[j].IngredientKey {
			return rows[i].IngredientKey < rows[j].IngredientKey
		}
		return rows[i].SKU < rows[j].SKU
	})
	data, err := json.Marshal(rows)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func RefreshFoodRunArtifact(artifact FoodRunArtifact, meals []string) FoodRunArtifact {
	artifact.MealPlanFingerprint = MealPlanFingerprint(artifact.MealPlan)
	artifact.ProductSelectionFingerprint = ProductSelectionFingerprint(artifact.Shop)
	artifact.RecipeSetFingerprint = RecipeSetFingerprint(artifact.MealPlan)
	if artifact.RecipeQualityReport != nil {
		artifact.RecipeQualityReport.RecipeSetFingerprint = artifact.RecipeSetFingerprint
		artifact.RecipeQualityReport.RecipeImageFingerprint = RecipeImageFingerprint(*artifact.RecipeQualityReport)
		artifact.RecipeQualityReport.RecipeQualityFingerprint = RecipeQualityFingerprint(*artifact.RecipeQualityReport)
		artifact.RecipeQualityFingerprint = artifact.RecipeQualityReport.RecipeQualityFingerprint
		artifact.RecipeImageFingerprint = artifact.RecipeQualityReport.RecipeImageFingerprint
	}
	if artifact.RecipeIntakePlan != nil {
		artifact.RecipeIntakePlan.RecipeSetFingerprint = artifact.RecipeSetFingerprint
		artifact.RecipeIntakePlan.ActiveRecipeCount = activeRecipeCount(artifact.MealPlan)
	}
	if artifact.BudgetDealReport != nil {
		report := BuildBudgetDealReport(artifact)
		artifact = AttachBudgetDealReport(artifact, report)
	}
	if artifact.PantryResolution != nil {
		artifact.PantryResolution.MealPlanFingerprint = artifact.MealPlanFingerprint
		if artifact.ServingPlanFingerprint != "" {
			artifact.PantryResolution.ServingPlanFingerprint = artifact.ServingPlanFingerprint
		}
		if artifact.ScaledMealPlanFingerprint != "" {
			artifact.PantryResolution.ScaledMealPlanFingerprint = artifact.ScaledMealPlanFingerprint
		}
		artifact.PantryResolutionFingerprint = artifact.PantryResolution.PantryResolutionFingerprint
		artifact.ShopRequirementsFingerprint = artifact.PantryResolution.ShopRequirementsFingerprint
		artifact.PantryProfileFingerprint = artifact.PantryResolution.PantryProfileFingerprint
	}
	if artifact.ServingPlan != nil {
		artifact.ServingPlan.MealPlanFingerprint = artifact.MealPlanFingerprint
		artifact.ServingPlanFingerprint = artifact.ServingPlan.ServingPlanFingerprint
		if artifact.HouseholdProfileFingerprint == "" {
			artifact.HouseholdProfileFingerprint = artifact.ServingPlan.HouseholdProfileFingerprint
		}
	}
	if artifact.ScaledMealPlan != nil {
		artifact.ScaledMealPlan.MealPlanFingerprint = artifact.MealPlanFingerprint
		artifact.ScaledMealPlan.ServingPlanFingerprint = artifact.ServingPlanFingerprint
		artifact.ScaledMealPlanFingerprint = artifact.ScaledMealPlan.ScaledMealPlanFingerprint
	}
	artifact.IngredientLinks = foodRunIngredientLinks(artifact.MealPlan, artifact.Shop)
	artifact.People = artifact.MealPlan.People
	artifact.Days = len(artifact.MealPlan.Days)
	artifact.Meals = append([]string(nil), meals...)
	artifact.Warnings = foodRunWarnings(artifact.MealPlan, artifact.Shop)
	return artifact
}

func RebuildMealPlanPurchases(plan MealPlan, profile Profile, pantry Pantry) MealPlan {
	var required []Ingredient
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			required = append(required, meal.Recipe.Ingredients...)
		}
	}
	plan.RequiredPurchases, plan.PantryUsage = ApplyPantry(required, pantry)
	if staples := StapleRestockIngredients(profile, pantry); len(staples) > 0 {
		plan.RequiredPurchases = mergeIngredients(append(plan.RequiredPurchases, staples...))
	}
	summary := SummarizePlan(plan)
	if nutritionIsZero(summary) {
		plan.Nutrition = nil
	} else {
		plan.Nutrition = &summary
	}
	return plan
}

func foodRunWarnings(plan MealPlan, shop ShopResult) []string {
	var warnings []string
	if !shop.Complete {
		warnings = append(warnings, "Shopping selection is incomplete; review missing or unavailable products before any cart change.")
	}
	if len(plan.MissingItems) > 0 {
		warnings = append(warnings, "The meal plan has missing items that need manual review.")
	}
	warnings = append(warnings, shop.Notes...)
	warnings = append(warnings,
		"Prices, offers, stock, substitutions, and variable-weight item totals can change before Alcampo confirms an order.",
		"No order was submitted; cart and checkout writes still require explicit approval and a spending guard.",
	)
	return warnings
}

func foodRunIngredientLinks(plan MealPlan, shop ShopResult) []FoodRunIngredientLink {
	selectedByIngredient := make(map[string]SelectedProduct, len(shop.SelectedProducts))
	for _, selected := range shop.SelectedProducts {
		key := normalizeKey(selected.Ingredient.Name)
		if key != "" {
			selectedByIngredient[key] = selected
		}
	}
	var links []FoodRunIngredientLink
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			for _, ingredient := range meal.Recipe.Ingredients {
				link := FoodRunIngredientLink{
					Day:              day.Day,
					Meal:             meal.Type,
					RecipeID:         meal.Recipe.ID,
					RecipeTitle:      meal.Recipe.Title,
					Ingredient:       ingredient.Name,
					RequiredQuantity: ingredient.Quantity,
					RequiredUnit:     ingredient.Unit,
					Status:           "not_purchased",
					Message:          "No selected product was required or found for this recipe ingredient.",
				}
				if selected, ok := selectedByIngredient[normalizeKey(ingredient.Name)]; ok {
					link.SelectedProductID = selected.Product.ID
					link.SelectedProductSKU = selected.Product.SKU
					link.SelectedProductName = selected.Product.Name
					switch {
					case selected.Error != "":
						link.Status = "missing"
						link.Message = selected.Error
					case selected.Product.ID != "" || selected.Product.SKU != "":
						link.Status = "selected"
						link.Message = selected.SelectionReason
					}
				}
				links = append(links, link)
			}
		}
	}
	return links
}

func ingredientPurchaseKey(ingredient Ingredient) string {
	return normalizeKey(firstNonEmptyString(ingredient.SearchTerm, ingredient.Name)) + "|" + normalizeUnit(ingredient.Unit)
}

func sortedIngredientKeys(ingredients []Ingredient) []string {
	keys := make([]string, 0, len(ingredients))
	seen := map[string]bool{}
	for _, ingredient := range ingredients {
		key := normalizeKey(ingredient.Name)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
