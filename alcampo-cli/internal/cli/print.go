package cli

import (
	"fmt"
	"io"
	"strings"

	"alcampo-cli/internal/alcampo"
	"alcampo-cli/internal/food"
	"alcampo-cli/internal/money"
	"alcampo-cli/internal/strutil"
)

func printFoodProfile(w io.Writer, profile food.Profile) {
	fmt.Fprintf(w, "people=%d\n", profile.People)
	fmt.Fprintf(w, "selection_policy=%s\n", strutil.FirstNonEmpty(profile.SelectionPolicy, "-"))
	fmt.Fprintf(w, "budget=%s\n", strutil.FirstNonEmpty(profile.BudgetEUR, "-"))
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
		expiry := strutil.FirstNonEmpty(item.ExpiryDate, "-")
		if days, ok := food.DaysUntilExpiry(item.ExpiryDate); ok && days <= 3 {
			expiry += "!"
		}
		fmt.Fprintf(w, "%s\t%.3g %s\t%s\texpiry=%s\tconfidence=%.2g\n",
			item.Name,
			item.Quantity,
			item.Unit,
			strutil.FirstNonEmpty(item.Location, "-"),
			expiry,
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
			strutil.FirstNonEmpty(staple.SearchTerm, "-"),
		)
	}
}

func printMealPlan(w io.Writer, plan food.MealPlan) {
	fmt.Fprintf(w, "mealplan\t%s\tpeople=%d\tfile=%s\n", plan.ID, plan.People, strutil.FirstNonEmpty(plan.File, "-"))
	if plan.Nutrition != nil {
		fmt.Fprintf(w, "nutrition\t%s\n", formatNutrition(*plan.Nutrition))
	}
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
	if recipe.NutritionPerServing != nil {
		fmt.Fprintf(w, "nutrition_per_serving\t%s\n", formatNutrition(*recipe.NutritionPerServing))
	}
	fmt.Fprintln(w, "ingredients")
	for _, ing := range recipe.Ingredients {
		fmt.Fprintf(w, "  %s\t%.3g %s\n", ing.Name, ing.Quantity, ing.Unit)
	}
	fmt.Fprintln(w, "steps")
	for _, step := range recipe.Steps {
		fmt.Fprintf(w, "  %d\t%s\n", step.Number, step.Text)
	}
}

func printRecipeSummary(w io.Writer, recipe food.Recipe) {
	fmt.Fprintf(w, "%s\t%s\tservings=%d\ttags=%s\n",
		recipe.ID,
		recipe.Title,
		recipe.Servings,
		strings.Join(recipe.Tags, ","),
	)
}

func printUseUpSuggestion(w io.Writer, suggestion food.UseUpSuggestion) {
	fmt.Fprintf(w, "%s\t%s\tscore=%.0f\tuses=%s\n",
		suggestion.Recipe.ID,
		suggestion.Recipe.Title,
		suggestion.Score,
		strings.Join(suggestion.ExpiringItems, ","),
	)
}

func printShopResult(w io.Writer, result food.ShopResult) {
	fmt.Fprintf(w, "shop\tpolicy=%s\tcomplete=%t\testimated_total=%s\n", result.Policy, result.Complete, formatMoney(result.EstimatedTotal))
	if result.Nutrition != nil {
		fmt.Fprintf(w, "nutrition\t%s\n", formatNutrition(*result.Nutrition))
	}
	for _, selected := range result.SelectedProducts {
		if selected.Error != "" {
			fmt.Fprintf(w, "ERROR\t%s\t%s\n", selected.Ingredient.Name, selected.Error)
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			selected.Ingredient.Name,
			strutil.FirstNonEmpty(selected.Product.SKU, selected.Product.ID, "-"),
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

func printProductLine(w io.Writer, p alcampo.Product) {
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
		strutil.FirstNonEmpty(p.SKU, p.ID, "-"),
		formatMoney(p.Price),
		formatUnitPrice(p),
		formatAvailable(p.Available),
		strutil.FirstNonEmpty(p.Brand, "-"),
		p.Name,
	)
}

func printProductDetail(w io.Writer, p alcampo.Product) {
	fmt.Fprintf(w, "sku: %s\n", strutil.FirstNonEmpty(p.SKU, "-"))
	fmt.Fprintf(w, "id: %s\n", strutil.FirstNonEmpty(p.ID, "-"))
	fmt.Fprintf(w, "name: %s\n", p.Name)
	fmt.Fprintf(w, "brand: %s\n", strutil.FirstNonEmpty(p.Brand, "-"))
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
		fmt.Fprintf(w, "%s%s\t%s\t%s\n", prefix, strutil.FirstNonEmpty(cat.RetailerID, cat.ID, "-"), cat.Slug, cat.Name)
		printCategories(w, cat.Children, depth+1)
	}
}

func formatMoney(m money.Money) string {
	if m.Amount == "" {
		return "-"
	}
	return m.Amount + " " + strutil.FirstNonEmpty(m.Currency, "EUR")
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

func formatNutrition(n food.NutritionSummary) string {
	return fmt.Sprintf("kcal=%.0f protein=%.1fg carbs=%.1fg fat=%.1fg", n.Kcal, n.ProteinG, n.CarbsG, n.FatG)
}
