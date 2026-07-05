package food

import (
	"strings"
	"time"

	"alcampo-cli/internal/alcampo"
	"alcampo-cli/internal/money"
)

const (
	PolicyBalanced = "balanced"
	PolicyCheapest = "cheapest"
	PolicyQuality  = "quality"
)

type Profile struct {
	People           int      `json:"people,omitempty"`
	SelectionPolicy  string   `json:"selection_policy,omitempty"`
	BudgetEUR        string   `json:"budget_eur,omitempty"`
	Diets            []string `json:"diets,omitempty"`
	Allergies        []string `json:"allergies,omitempty"`
	Dislikes         []string `json:"dislikes,omitempty"`
	LikedCuisines    []string `json:"liked_cuisines,omitempty"`
	LikedRecipes     []string `json:"liked_recipes,omitempty"`
	RejectedRecipes  []string `json:"rejected_recipes,omitempty"`
	LikedProducts    []string `json:"liked_products,omitempty"`
	RejectedProducts []string `json:"rejected_products,omitempty"`
	PreferredBrands  []string `json:"preferred_brands,omitempty"`
	RejectedBrands   []string `json:"rejected_brands,omitempty"`
	NutritionGoals   []string `json:"nutrition_goals,omitempty"`
	Staples          []Staple `json:"staples,omitempty"`
	UpdatedAt        string   `json:"updated_at,omitempty"`
}

type Staple struct {
	Name       string  `json:"name"`
	MinQty     float64 `json:"min_qty"`
	Unit       string  `json:"unit,omitempty"`
	SearchTerm string  `json:"search_term,omitempty"`
	Category   string  `json:"category,omitempty"`
}

type Pantry struct {
	Items     []PantryItem `json:"items"`
	UpdatedAt string       `json:"updated_at,omitempty"`
}

type PantryItem struct {
	ID          string  `json:"id,omitempty"`
	Name        string  `json:"item"`
	Quantity    float64 `json:"quantity"`
	Unit        string  `json:"unit,omitempty"`
	Location    string  `json:"location,omitempty"`
	ExpiryDate  string  `json:"expiry,omitempty"`
	Confidence  float64 `json:"confidence,omitempty"`
	LastChecked string  `json:"last_checked,omitempty"`
	Notes       string  `json:"notes,omitempty"`
}

type Ingredient struct {
	Name       string  `json:"name"`
	Quantity   float64 `json:"quantity,omitempty"`
	Unit       string  `json:"unit,omitempty"`
	Category   string  `json:"category,omitempty"`
	SearchTerm string  `json:"search_term,omitempty"`
	Optional   bool    `json:"optional,omitempty"`
}

type RecipeStep struct {
	Number   int    `json:"number"`
	Title    string `json:"title,omitempty"`
	Text     string `json:"text"`
	Minutes  int    `json:"minutes,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

type Recipe struct {
	ID            string       `json:"id"`
	Title         string       `json:"title"`
	Servings      int          `json:"servings"`
	PrepMinutes   int          `json:"prep_minutes,omitempty"`
	CookMinutes   int          `json:"cook_minutes,omitempty"`
	Tags          []string     `json:"tags,omitempty"`
	ImageURL      string       `json:"image_url,omitempty"`
	Ingredients   []Ingredient `json:"ingredients"`
	Equipment     []string     `json:"equipment,omitempty"`
	Steps         []RecipeStep `json:"steps"`
	Substitutions []string     `json:"substitutions,omitempty"`
	AllergenNotes []string     `json:"allergen_notes,omitempty"`
}

type DayPlan struct {
	Day   int    `json:"day"`
	Label string `json:"label,omitempty"`
	Meals []Meal `json:"meals"`
}

type Meal struct {
	Type           string `json:"type"`
	Recipe         Recipe `json:"recipe"`
	PlanningReason string `json:"planning_reason,omitempty"`
}

type PantryUsage struct {
	Ingredient string  `json:"ingredient"`
	PantryItem string  `json:"pantry_item"`
	Quantity   float64 `json:"quantity"`
	Unit       string  `json:"unit,omitempty"`
	Location   string  `json:"location,omitempty"`
}

type MealPlan struct {
	ID                string        `json:"id"`
	CreatedAt         string        `json:"created_at"`
	People            int           `json:"people"`
	Days              []DayPlan     `json:"days"`
	BudgetEUR         string        `json:"budget_eur,omitempty"`
	SelectionPolicy   string        `json:"selection_policy,omitempty"`
	PantryUsage       []PantryUsage `json:"pantry_usage,omitempty"`
	RequiredPurchases []Ingredient  `json:"required_purchases"`
	MissingItems      []string      `json:"missing_items,omitempty"`
	Notes             []string      `json:"notes,omitempty"`
	File              string        `json:"file,omitempty"`
}

type ProductSummary struct {
	ID              string      `json:"id,omitempty"`
	SKU             string      `json:"sku,omitempty"`
	Name            string      `json:"name,omitempty"`
	Brand           string      `json:"brand,omitempty"`
	Price           money.Money `json:"price,omitempty"`
	UnitPrice       money.Money `json:"unit_price,omitempty"`
	Unit            string      `json:"unit,omitempty"`
	Size            string      `json:"size,omitempty"`
	URL             string      `json:"url,omitempty"`
	ImageURL        string      `json:"image_url,omitempty"`
	Category        string      `json:"category,omitempty"`
	Available       *bool       `json:"available,omitempty"`
	Offers          []string    `json:"offers,omitempty"`
	OfferCount      int         `json:"offer_count,omitempty"`
	PackageQuantity float64     `json:"package_quantity,omitempty"`
	PackageUnit     string      `json:"package_unit,omitempty"`
	Allergens       string      `json:"allergens,omitempty"`
	Nutrition       string      `json:"nutrition,omitempty"`
}

type ProductOption struct {
	Product        ProductSummary `json:"product"`
	Score          float64        `json:"score"`
	Reason         string         `json:"reason,omitempty"`
	RejectedReason string         `json:"rejected_reason,omitempty"`
}

type SelectedProduct struct {
	Ingredient       Ingredient      `json:"ingredient"`
	Product          ProductSummary  `json:"product,omitempty"`
	PurchaseQuantity string          `json:"purchase_quantity,omitempty"`
	PackageCount     int             `json:"package_count,omitempty"`
	QuantityReason   string          `json:"quantity_reason,omitempty"`
	LineTotal        money.Money     `json:"line_total,omitempty"`
	SelectionReason  string          `json:"selection_reason,omitempty"`
	Alternates       []ProductOption `json:"alternates_considered,omitempty"`
	Warnings         []string        `json:"warnings,omitempty"`
	Error            string          `json:"error,omitempty"`
}

type ShoppingGroup struct {
	Category    string            `json:"category"`
	Items       []SelectedProduct `json:"items"`
	BasketLines []string          `json:"basket_lines,omitempty"`
	Subtotal    money.Money       `json:"subtotal"`
}

type ShopResult struct {
	MealPlanID       string            `json:"mealplan_id,omitempty"`
	Policy           string            `json:"selection_policy"`
	SelectedProducts []SelectedProduct `json:"selected_products"`
	ShoppingGroups   []ShoppingGroup   `json:"shopping_groups,omitempty"`
	BasketLines      []string          `json:"basket_lines,omitempty"`
	EstimatedTotal   money.Money       `json:"estimated_total"`
	Complete         bool              `json:"complete"`
	Notes            []string          `json:"notes,omitempty"`
}

type CookResult struct {
	MealPlanID string           `json:"mealplan_id,omitempty"`
	CookedAt   string           `json:"cooked_at"`
	Recipes    []RecipeFeedback `json:"recipes,omitempty"`
	Applied    []PantryUsage    `json:"applied_pantry_usage,omitempty"`
	Missing    []PantryUsage    `json:"missing_pantry_usage,omitempty"`
	Pantry     Pantry           `json:"pantry"`
	Note       string           `json:"note,omitempty"`
	Rating     int              `json:"rating,omitempty"`
}

type RecipeFeedback struct {
	ID    string `json:"id,omitempty"`
	Title string `json:"title,omitempty"`
}

type ReceiveOptions struct {
	Location   string
	ExpiryDate string
}

type ReceiveItem struct {
	Ingredient       Ingredient     `json:"ingredient"`
	Product          ProductSummary `json:"product,omitempty"`
	PantryItem       PantryItem     `json:"pantry_item"`
	PackageCount     int            `json:"package_count,omitempty"`
	QuantityReason   string         `json:"quantity_reason,omitempty"`
	PurchaseQuantity string         `json:"purchase_quantity,omitempty"`
}

type ReceiveSkipped struct {
	Ingredient Ingredient     `json:"ingredient"`
	Product    ProductSummary `json:"product,omitempty"`
	Reason     string         `json:"reason"`
}

type ReceiveResult struct {
	MealPlanID string           `json:"mealplan_id,omitempty"`
	ReceivedAt string           `json:"received_at"`
	Applied    []ReceiveItem    `json:"applied,omitempty"`
	Skipped    []ReceiveSkipped `json:"skipped,omitempty"`
	Pantry     Pantry           `json:"pantry"`
}

func NormalizeSelectionPolicy(policy string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case PolicyBalanced, "":
		if strings.TrimSpace(policy) == "" {
			return "", false
		}
		return PolicyBalanced, true
	case PolicyCheapest:
		return PolicyCheapest, true
	case PolicyQuality:
		return PolicyQuality, true
	default:
		return "", false
	}
}

func ProductSummaryFromAlcampo(p alcampo.Product) ProductSummary {
	var image string
	if len(p.Images) > 0 {
		image = p.Images[0]
	}
	offers := make([]string, 0, len(p.Offers))
	for _, offer := range p.Offers {
		text := strings.TrimSpace(strings.Join([]string{offer.Name, offer.Description}, " "))
		if text != "" {
			offers = append(offers, text)
		}
	}
	summary := ProductSummary{
		ID:         p.ID,
		SKU:        p.SKU,
		Name:       p.Name,
		Brand:      p.Brand,
		Price:      p.Price,
		UnitPrice:  p.UnitPrice,
		Unit:       p.Unit,
		Size:       p.Size,
		URL:        p.URL,
		ImageURL:   image,
		Category:   p.Category,
		Available:  p.Available,
		Offers:     offers,
		OfferCount: len(offers),
		Allergens:  p.Allergens,
		Nutrition:  p.Nutrition,
	}
	if qty, unit, ok := ParsePackageQuantity(p.Size, p.Name); ok {
		summary.PackageQuantity = qty
		summary.PackageUnit = unit
	}
	return summary
}

func nowStamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}
