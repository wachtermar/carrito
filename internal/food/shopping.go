package food

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/money"
	"github.com/wachtermar/carrito/internal/strutil"
)

type ShopOptions struct {
	Policy string
	Limit  int
}

func ShopMealPlan(ctx context.Context, client *alcampo.Client, plan MealPlan, profile Profile, opts ShopOptions) (ShopResult, error) {
	policy := strings.TrimSpace(opts.Policy)
	if policy == "" {
		policy = strings.TrimSpace(profile.SelectionPolicy)
	}
	normalized, ok := NormalizeSelectionPolicy(policy)
	if !ok || normalized == "" {
		return ShopResult{}, fmt.Errorf("selection policy is not set; run 'carrito food profile set --selection-policy balanced|cheapest|quality'")
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 8
	}
	result := ShopResult{MealPlanID: plan.ID, Policy: normalized, Complete: true, Nutrition: plan.Nutrition}
	for _, ingredient := range plan.RequiredPurchases {
		query := strutil.FirstNonEmpty(ingredient.SearchTerm, ingredient.Name)
		products, err := client.Search(ctx, query, alcampo.SearchOptions{Limit: limit, RegionID: client.RegionID})
		if err != nil {
			result.Complete = false
			result.SelectedProducts = append(result.SelectedProducts, SelectedProduct{
				Ingredient: ingredient,
				Error:      err.Error(),
			})
			continue
		}
		selection := SelectProduct(ingredient, products, profile, normalized)
		if selection.Error != "" {
			result.Complete = false
		}
		if report := ProductNutritionReportFromSelection(selection); report != nil {
			result.ProductNutritionReports = append(result.ProductNutritionReports, *report)
			result.NutritionWarnings = append(result.NutritionWarnings, report.Warnings...)
		}
		if selection.Product.SKU != "" {
			result.BasketLines = append(result.BasketLines, BasketLineForSelection(selection))
			result.EstimatedTotal.Cents += selection.LineTotal.Cents
		}
		result.SelectedProducts = append(result.SelectedProducts, selection)
	}
	result.EstimatedTotal = money.Money{Amount: money.FormatAmount(result.EstimatedTotal.Cents), Currency: "EUR", Cents: result.EstimatedTotal.Cents}
	if note, over := budgetStatusNote(plan.BudgetEUR, result.EstimatedTotal); over {
		result.Complete = false
		result.Notes = append(result.Notes, note)
	} else if note != "" {
		result.Notes = append(result.Notes, note)
	}
	result.ShoppingGroups = GroupSelectedProducts(result.SelectedProducts)
	if len(result.SelectedProducts) == 0 {
		result.Notes = append(result.Notes, "No required purchases were found; pantry may already cover this plan.")
	}
	return result, nil
}

func BasketLineForSelection(selection SelectedProduct) string {
	if selection.Product.SKU == "" || selection.PurchaseQuantity == "" {
		return ""
	}
	return fmt.Sprintf("%s %s # %s", selection.Product.SKU, selection.PurchaseQuantity, selection.Ingredient.Name)
}

func GroupSelectedProducts(selected []SelectedProduct) []ShoppingGroup {
	var groups []ShoppingGroup
	index := make(map[string]int)
	for _, item := range selected {
		category := shoppingCategory(item)
		key := normalizeKey(category)
		if key == "" {
			key = "other"
		}
		pos, ok := index[key]
		if !ok {
			pos = len(groups)
			index[key] = pos
			groups = append(groups, ShoppingGroup{Category: category})
		}
		groups[pos].Items = append(groups[pos].Items, item)
		if line := BasketLineForSelection(item); line != "" {
			groups[pos].BasketLines = append(groups[pos].BasketLines, line)
		}
		groups[pos].Subtotal.Cents += item.LineTotal.Cents
	}
	for i := range groups {
		groups[i].Subtotal = money.Money{
			Amount:   money.FormatAmount(groups[i].Subtotal.Cents),
			Currency: "EUR",
			Cents:    groups[i].Subtotal.Cents,
		}
	}
	return groups
}

func shoppingCategory(item SelectedProduct) string {
	category := strutil.FirstNonEmpty(item.Product.Category, item.Ingredient.Category, "Other")
	category = strings.TrimSpace(category)
	if category == "" {
		return "Other"
	}
	return category
}

func SelectProduct(ingredient Ingredient, products []alcampo.Product, profile Profile, policy string) SelectedProduct {
	if len(products) == 0 {
		return SelectedProduct{Ingredient: ingredient, Error: "no products found"}
	}
	var options []ProductOption
	prices := priceContextFor(products)
	for _, product := range products {
		option := scoreProduct(ingredient, product, profile, policy, prices)
		options = append(options, option)
	}
	sort.SliceStable(options, func(i, j int) bool {
		if options[i].RejectedReason != "" && options[j].RejectedReason == "" {
			return false
		}
		if options[i].RejectedReason == "" && options[j].RejectedReason != "" {
			return true
		}
		if options[i].Score == options[j].Score {
			return optionCents(options[i]) < optionCents(options[j])
		}
		return options[i].Score > options[j].Score
	})
	selection := SelectedProduct{Ingredient: ingredient}
	if len(options) > 1 {
		selection.Alternates = compatibleAlternates(options[1:], 4)
	}
	if options[0].RejectedReason != "" {
		selection.Alternates = options
		selection.Error = "no compatible available product found"
		return selection
	}
	selection.Product = options[0].Product
	selection.SelectionReason = options[0].Reason
	count, lineTotal, quantityReason, warnings := EstimatePurchaseQuantity(ingredient, options[0].Product)
	selection.PurchaseQuantity = fmt.Sprintf("%d", count)
	selection.PackageCount = count
	selection.LineTotal = lineTotal
	selection.QuantityReason = quantityReason
	selection.Warnings = append(selection.Warnings, warnings...)
	AttachSelectedProductNutrition(&selection)
	return selection
}

type priceContext struct {
	CheapestUnitCents int64
}

func priceContextFor(products []alcampo.Product) priceContext {
	var cheapest int64
	for _, product := range products {
		cents := unitCents(product)
		if cents <= 0 {
			continue
		}
		if cheapest == 0 || cents < cheapest {
			cheapest = cents
		}
	}
	return priceContext{CheapestUnitCents: cheapest}
}

func scoreProduct(ingredient Ingredient, product alcampo.Product, profile Profile, policy string, prices priceContext) ProductOption {
	summary := ProductSummaryFromAlcampo(product)
	text := normalizeKey(strings.Join([]string{
		product.Name,
		product.Brand,
		product.Category,
		strings.Join(product.CategoryPath, " "),
		product.Ingredients,
		product.Allergens,
	}, " "))
	if product.Available != nil && !*product.Available {
		return ProductOption{Product: summary, Score: -1000000, RejectedReason: "unavailable in the selected market"}
	}
	if reason := productDietRejectedReason(text, profile.Diets); reason != "" {
		return ProductOption{Product: summary, Score: -1000000, RejectedReason: reason}
	}
	if containsAny(text, profile.Allergies) {
		return ProductOption{Product: summary, Score: -1000000, RejectedReason: "matches saved allergy terms"}
	}
	if containsAny(text, profile.Dislikes) {
		return ProductOption{Product: summary, Score: -1000000, RejectedReason: "matches saved dislikes"}
	}
	if containsAny(normalizeKey(product.Brand), profile.RejectedBrands) {
		return ProductOption{Product: summary, Score: -1000000, RejectedReason: "matches rejected brand"}
	}
	if productMemoryMatches(product, profile.RejectedProducts) {
		return ProductOption{Product: summary, Score: -1000000, RejectedReason: "matches rejected product memory"}
	}

	score := 100.0
	var reasons []string
	if product.Available != nil && *product.Available {
		score += 40
		reasons = append(reasons, "available")
	} else {
		score -= 10
		reasons = append(reasons, "availability unknown")
	}
	match := matchScore(ingredient, product)
	score += match
	if match >= 20 {
		reasons = append(reasons, "strong ingredient match")
	} else if match > 0 {
		reasons = append(reasons, "partial ingredient match")
	}
	if summary.ImageURL != "" {
		score += 5
		reasons = append(reasons, "has product image")
	}
	if summary.URL != "" {
		score += 2
	}
	score += offerDealScore(product, prices, &reasons)
	if containsAny(normalizeKey(product.Brand), profile.PreferredBrands) {
		score += 25
		reasons = append(reasons, "preferred brand")
	}
	if productMemoryMatches(product, profile.LikedProducts) {
		score += 30
		reasons = append(reasons, "liked product")
	}
	if packageScore, packageReason := packageFitScore(ingredient, summary); packageScore != 0 || packageReason != "" {
		score += packageScore
		if packageReason != "" {
			reasons = append(reasons, packageReason)
		}
	}
	unitCents := unitCents(product)
	switch policy {
	case PolicyCheapest:
		score += cheapScore(unitCents, product.Price.Cents)
		reasons = append(reasons, "ranked by lowest unit/package price")
	case PolicyQuality:
		score += qualityScore(product)
		if unitCents > 0 {
			score -= math.Min(float64(unitCents)/100.0, 15)
		}
		reasons = append(reasons, "ranked by quality signals before price")
	default:
		score += cheapScore(unitCents, product.Price.Cents) / 2
		score += qualityScore(product) / 2
		reasons = append(reasons, "balanced value policy")
	}
	return ProductOption{Product: summary, Score: roundScore(score), Reason: strings.Join(reasons, "; ")}
}

func compatibleAlternates(options []ProductOption, limit int) []ProductOption {
	if limit <= 0 {
		return nil
	}
	var out []ProductOption
	for _, option := range options {
		if option.RejectedReason != "" {
			continue
		}
		out = append(out, option)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func productDietRejectedReason(text string, diets []string) string {
	for _, diet := range diets {
		switch normalizeKey(diet) {
		case "vegetarian", "vegetariano", "vegetariana":
			if containsAnyKey(text, vegetarianForbiddenKeys) {
				return "conflicts with vegetarian diet"
			}
		case "vegan", "vegano", "vegana":
			if containsAnyKey(veganDietText(text), append(vegetarianForbiddenKeys, veganForbiddenKeys...)) {
				return "conflicts with vegan diet"
			}
		}
	}
	return ""
}

func budgetStatusNote(budgetEUR string, total money.Money) (string, bool) {
	budgetEUR = strings.TrimSpace(budgetEUR)
	if budgetEUR == "" || total.Cents <= 0 {
		return "", false
	}
	budgetCents, err := money.ParseCents(budgetEUR)
	if err != nil || budgetCents <= 0 {
		return fmt.Sprintf("Budget %q could not be parsed; compare the estimated total manually.", budgetEUR), false
	}
	if total.Cents > budgetCents {
		return fmt.Sprintf("Estimated total %s EUR is above the budget target %s EUR.", money.FormatAmount(total.Cents), money.FormatAmount(budgetCents)), true
	}
	return fmt.Sprintf("Estimated total %s EUR is within the budget target %s EUR.", money.FormatAmount(total.Cents), money.FormatAmount(budgetCents)), false
}

func productMemoryMatches(product alcampo.Product, memories []string) bool {
	if len(memories) == 0 {
		return false
	}
	id := normalizeKey(product.ID)
	sku := normalizeKey(product.SKU)
	ean := normalizeKey(product.EAN)
	text := normalizeKey(strings.Join([]string{
		product.ID,
		product.SKU,
		product.EAN,
		product.Name,
		product.Brand,
		product.Category,
		strings.Join(product.CategoryPath, " "),
	}, " "))
	for _, memory := range memories {
		key := normalizeKey(memory)
		if key == "" {
			continue
		}
		if key == id || key == sku || key == ean {
			return true
		}
		if len(key) >= 3 && strings.Contains(text, key) {
			return true
		}
	}
	return false
}

func offerDealScore(product alcampo.Product, prices priceContext, reasons *[]string) float64 {
	if len(product.Offers) == 0 {
		return 0
	}
	unit := unitCents(product)
	switch {
	case prices.CheapestUnitCents > 0 && unit > 0 && unit <= int64(float64(prices.CheapestUnitCents)*1.10):
		*reasons = append(*reasons, "offer available and unit price is within 10% of the cheapest candidate")
		return 22
	case prices.CheapestUnitCents > 0 && unit > 0 && unit <= int64(float64(prices.CheapestUnitCents)*1.25):
		*reasons = append(*reasons, "offer available with competitive unit price")
		return 14
	default:
		*reasons = append(*reasons, "offer available")
		return 6
	}
}

func packageFitScore(ingredient Ingredient, product ProductSummary) (float64, string) {
	requiredQty := ingredient.Quantity
	requiredUnit := normalizeUnit(ingredient.Unit)
	if requiredQty <= 0 || requiredUnit == "" || product.PackageQuantity <= 0 || product.PackageUnit == "" {
		return 0, ""
	}
	requiredBase, requiredBaseUnit, ok := toBaseQuantity(requiredQty, requiredUnit)
	if !ok {
		return 0, ""
	}
	packageBase, packageBaseUnit, ok := toBaseQuantity(product.PackageQuantity, product.PackageUnit)
	if !ok || requiredBaseUnit != packageBaseUnit || packageBase <= 0 {
		return -6, "package unit does not match recipe unit"
	}
	packages := math.Ceil(requiredBase / packageBase)
	overbuyRatio := (packages * packageBase) / requiredBase
	switch {
	case overbuyRatio <= 1.15:
		return 14, "package size closely fits recipe quantity"
	case overbuyRatio <= 1.6:
		return 6, "package size reasonably fits recipe quantity"
	case overbuyRatio >= 3:
		return -18, "package size is much larger than needed"
	default:
		return -5, "package size overbuys recipe quantity"
	}
}

func matchScore(ingredient Ingredient, product alcampo.Product) float64 {
	target := normalizeKey(strutil.FirstNonEmpty(ingredient.SearchTerm, ingredient.Name))
	text := normalizeKey(product.Name + " " + product.Brand + " " + product.Category + " " + strings.Join(product.CategoryPath, " "))
	if target == "" || text == "" {
		return 0
	}
	score := 0.0
	if strings.Contains(text, target) {
		score += 40
	}
	for _, token := range strings.Fields(target) {
		if len(token) <= 2 {
			continue
		}
		if strings.Contains(text, token) {
			score += 8
		}
	}
	return score
}

func cheapScore(unitCents, packageCents int64) float64 {
	cents := unitCents
	if cents <= 0 {
		cents = packageCents
	}
	if cents <= 0 {
		return -20
	}
	return math.Max(0, 70-math.Log(float64(cents)+1)*10)
}

func qualityScore(product alcampo.Product) float64 {
	score := 0.0
	if product.Brand != "" {
		score += 8
	}
	if product.Description != "" || product.Ingredients != "" || product.Nutrition != "" {
		score += 10
	}
	if len(product.Images) > 0 {
		score += 8
	}
	if product.UnitPrice.Amount != "" {
		score += 5
	}
	if len(product.CategoryPath) > 0 {
		score += 4
	}
	return score
}

func unitCents(product alcampo.Product) int64 {
	if product.UnitPrice.Cents > 0 {
		return product.UnitPrice.Cents
	}
	return product.Price.Cents
}

func optionCents(option ProductOption) int64 {
	if option.Product.UnitPrice.Cents > 0 {
		return option.Product.UnitPrice.Cents
	}
	return option.Product.Price.Cents
}

func roundScore(v float64) float64 {
	return math.Round(v*100) / 100
}
