package food

import (
	"strings"
	"time"

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/money"
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
	ID          string                   `json:"id,omitempty"`
	Name        string                   `json:"item"`
	Quantity    float64                  `json:"quantity"`
	Unit        string                   `json:"unit,omitempty"`
	Location    string                   `json:"location,omitempty"`
	ExpiryDate  string                   `json:"expiry,omitempty"`
	Confidence  float64                  `json:"confidence,omitempty"`
	LastChecked string                   `json:"last_checked,omitempty"`
	Notes       string                   `json:"notes,omitempty"`
	Nutrition   *PantryNutritionEvidence `json:"nutrition,omitempty"`
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

type NutritionSummary struct {
	Kcal     float64 `json:"kcal,omitempty"`
	ProteinG float64 `json:"protein_g,omitempty"`
	CarbsG   float64 `json:"carbs_g,omitempty"`
	FatG     float64 `json:"fat_g,omitempty"`
}

type NutritionBasis string

const (
	NutritionPer100g    NutritionBasis = "per_100g"
	NutritionPer100ml   NutritionBasis = "per_100ml"
	NutritionPerServing NutritionBasis = "per_serving"
	NutritionPerUnit    NutritionBasis = "per_unit"
	NutritionUnknown    NutritionBasis = "unknown"
)

type StructuredNutrition struct {
	Basis         NutritionBasis `json:"basis,omitempty"`
	BasisQty      float64        `json:"basis_qty,omitempty"`
	BasisUnit     string         `json:"basis_unit,omitempty"`
	EnergyKJ      *float64       `json:"energy_kj,omitempty"`
	Kcal          *float64       `json:"kcal,omitempty"`
	FatG          *float64       `json:"fat_g,omitempty"`
	SaturatesG    *float64       `json:"saturates_g,omitempty"`
	CarbsG        *float64       `json:"carbs_g,omitempty"`
	SugarsG       *float64       `json:"sugars_g,omitempty"`
	FiberG        *float64       `json:"fiber_g,omitempty"`
	ProteinG      *float64       `json:"protein_g,omitempty"`
	SaltG         *float64       `json:"salt_g,omitempty"`
	SodiumG       *float64       `json:"sodium_g,omitempty"`
	Source        string         `json:"source,omitempty"`
	Raw           string         `json:"raw,omitempty"`
	Confidence    float64        `json:"confidence,omitempty"`
	ParseWarnings []string       `json:"parse_warnings,omitempty"`
}

type NutritionEstimate struct {
	RequiredQuantity float64             `json:"required_quantity,omitempty"`
	RequiredUnit     string              `json:"required_unit,omitempty"`
	Factor           float64             `json:"factor,omitempty"`
	Nutrients        StructuredNutrition `json:"nutrients,omitempty"`
	Source           string              `json:"source,omitempty"`
	Confidence       float64             `json:"confidence,omitempty"`
	Warnings         []string            `json:"warnings,omitempty"`
}

type ProductNutritionReport struct {
	ProductID          string               `json:"product_id,omitempty"`
	SKU                string               `json:"sku,omitempty"`
	ProductName        string               `json:"product_name,omitempty"`
	LabelNutrition     *StructuredNutrition `json:"label_nutrition,omitempty"`
	RequiredNutrition  *NutritionEstimate   `json:"required_nutrition,omitempty"`
	PurchasedNutrition *NutritionEstimate   `json:"purchased_nutrition,omitempty"`
	Warnings           []string             `json:"warnings,omitempty"`
}

type Recipe struct {
	ID                  string            `json:"id"`
	Title               string            `json:"title"`
	Servings            int               `json:"servings"`
	PrepMinutes         int               `json:"prep_minutes,omitempty"`
	CookMinutes         int               `json:"cook_minutes,omitempty"`
	Tags                []string          `json:"tags,omitempty"`
	ImageURL            string            `json:"image_url,omitempty"`
	Ingredients         []Ingredient      `json:"ingredients"`
	Equipment           []string          `json:"equipment,omitempty"`
	Steps               []RecipeStep      `json:"steps"`
	Substitutions       []string          `json:"substitutions,omitempty"`
	AllergenNotes       []string          `json:"allergen_notes,omitempty"`
	NutritionPerServing *NutritionSummary `json:"nutrition_per_serving,omitempty"`
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
	ID                string            `json:"id"`
	CreatedAt         string            `json:"created_at"`
	People            int               `json:"people"`
	Days              []DayPlan         `json:"days"`
	BudgetEUR         string            `json:"budget_eur,omitempty"`
	SelectionPolicy   string            `json:"selection_policy,omitempty"`
	PantryUsage       []PantryUsage     `json:"pantry_usage,omitempty"`
	RequiredPurchases []Ingredient      `json:"required_purchases"`
	MissingItems      []string          `json:"missing_items,omitempty"`
	Notes             []string          `json:"notes,omitempty"`
	Nutrition         *NutritionSummary `json:"nutrition,omitempty"`
	File              string            `json:"file,omitempty"`
}

type UseUpSuggestion struct {
	Recipe        Recipe   `json:"recipe"`
	Score         float64  `json:"score"`
	ExpiringItems []string `json:"expiring_items"`
	Reason        string   `json:"reason,omitempty"`
}

type ProductSummary struct {
	ID              string               `json:"id,omitempty"`
	SKU             string               `json:"sku,omitempty"`
	Name            string               `json:"name,omitempty"`
	Brand           string               `json:"brand,omitempty"`
	Price           money.Money          `json:"price,omitempty"`
	UnitPrice       money.Money          `json:"unit_price,omitempty"`
	Unit            string               `json:"unit,omitempty"`
	Size            string               `json:"size,omitempty"`
	URL             string               `json:"url,omitempty"`
	ImageURL        string               `json:"image_url,omitempty"`
	Category        string               `json:"category,omitempty"`
	Available       *bool                `json:"available,omitempty"`
	Offers          []string             `json:"offers,omitempty"`
	OfferCount      int                  `json:"offer_count,omitempty"`
	PackageQuantity float64              `json:"package_quantity,omitempty"`
	PackageUnit     string               `json:"package_unit,omitempty"`
	Allergens       string               `json:"allergens,omitempty"`
	Nutrition       string               `json:"nutrition,omitempty"`
	NutritionParsed *StructuredNutrition `json:"nutrition_parsed,omitempty"`
}

type ProductOption struct {
	Product        ProductSummary `json:"product"`
	Score          float64        `json:"score"`
	Reason         string         `json:"reason,omitempty"`
	RejectedReason string         `json:"rejected_reason,omitempty"`
}

type SelectedProduct struct {
	Ingredient          Ingredient           `json:"ingredient"`
	Product             ProductSummary       `json:"product,omitempty"`
	PurchaseQuantity    string               `json:"purchase_quantity,omitempty"`
	PackageCount        int                  `json:"package_count,omitempty"`
	QuantityReason      string               `json:"quantity_reason,omitempty"`
	LineTotal           money.Money          `json:"line_total,omitempty"`
	SelectionReason     string               `json:"selection_reason,omitempty"`
	Alternates          []ProductOption      `json:"alternates_considered,omitempty"`
	Warnings            []string             `json:"warnings,omitempty"`
	ProductNutrition    *StructuredNutrition `json:"product_nutrition,omitempty"`
	RequiredNutrition   *NutritionEstimate   `json:"required_nutrition,omitempty"`
	PurchasedNutrition  *NutritionEstimate   `json:"purchased_nutrition,omitempty"`
	NutritionWarnings   []string             `json:"nutrition_warnings,omitempty"`
	NutritionProvenance string               `json:"nutrition_provenance,omitempty"`
	Error               string               `json:"error,omitempty"`
}

type ShoppingGroup struct {
	Category    string            `json:"category"`
	Items       []SelectedProduct `json:"items"`
	BasketLines []string          `json:"basket_lines,omitempty"`
	Subtotal    money.Money       `json:"subtotal"`
}

type ShopResult struct {
	MealPlanID              string                   `json:"mealplan_id,omitempty"`
	Policy                  string                   `json:"selection_policy"`
	SelectedProducts        []SelectedProduct        `json:"selected_products"`
	ShoppingGroups          []ShoppingGroup          `json:"shopping_groups,omitempty"`
	BasketLines             []string                 `json:"basket_lines,omitempty"`
	EstimatedTotal          money.Money              `json:"estimated_total"`
	Complete                bool                     `json:"complete"`
	Notes                   []string                 `json:"notes,omitempty"`
	Nutrition               *NutritionSummary        `json:"nutrition,omitempty"`
	ProductNutritionReports []ProductNutritionReport `json:"product_nutrition_reports,omitempty"`
	NutritionWarnings       []string                 `json:"nutrition_warnings,omitempty"`
}

type DataProvenance struct {
	Scope      string `json:"scope,omitempty"`
	Source     string `json:"source,omitempty"`
	URL        string `json:"url,omitempty"`
	Confidence string `json:"confidence,omitempty"`
	Message    string `json:"message,omitempty"`
}

type FoodRunArtifact struct {
	SchemaVersion                int                      `json:"schema_version,omitempty"`
	Kind                         string                   `json:"kind,omitempty"`
	CreatedAt                    string                   `json:"created_at,omitempty"`
	MealPlanFingerprint          string                   `json:"mealplan_fingerprint,omitempty"`
	ProductSelectionFingerprint  string                   `json:"product_selection_fingerprint,omitempty"`
	HouseholdProfileFingerprint  string                   `json:"household_profile_fingerprint,omitempty"`
	ServingPlanFingerprint       string                   `json:"serving_plan_fingerprint,omitempty"`
	ScaledMealPlanFingerprint    string                   `json:"scaled_mealplan_fingerprint,omitempty"`
	PantryProfileFingerprint     string                   `json:"pantry_profile_fingerprint,omitempty"`
	PantryResolutionFingerprint  string                   `json:"pantry_resolution_fingerprint,omitempty"`
	ShopRequirementsFingerprint  string                   `json:"shop_requirements_fingerprint,omitempty"`
	HouseholdProfileSummary      *HouseholdProfileSummary `json:"household_profile_summary,omitempty"`
	ServingPlan                  *ServingPlan             `json:"serving_plan,omitempty"`
	ScaledMealPlan               *ScaledMealPlan          `json:"scaled_mealplan,omitempty"`
	PantryProfileSummary         *PantryProfileSummary    `json:"pantry_profile_summary,omitempty"`
	PantryResolution             *PantryResolution        `json:"pantry_resolution,omitempty"`
	PantryConsumptionPlan        *PantryConsumptionPlan   `json:"pantry_consumption_plan,omitempty"`
	PrePantryMealPlanFingerprint string                   `json:"pre_pantry_mealplan_fingerprint,omitempty"`
	NutritionLedgerFingerprint   string                   `json:"nutrition_ledger_fingerprint,omitempty"`
	NutritionLedger              *NutritionLedger         `json:"nutrition_ledger,omitempty"`
	MealPlan                     MealPlan                 `json:"mealplan"`
	Shop                         ShopResult               `json:"shop"`
	IngredientLinks              []FoodRunIngredientLink  `json:"ingredient_links,omitempty"`
	PreRecoveryQuantityLedger    *QuantityLedger          `json:"pre_recovery_quantity_ledger,omitempty"`
	RecoveryPlan                 *RecoveryPlan            `json:"recovery_plan,omitempty"`
	PreRecipeSwapQuantityLedger  *QuantityLedger          `json:"pre_recipe_swap_quantity_ledger,omitempty"`
	PreRecipeSwapReadinessGate   *ReadinessGate           `json:"pre_recipe_swap_readiness_gate,omitempty"`
	RecipeSwapPlan               *RecipeSwapPlan          `json:"recipe_swap_plan,omitempty"`
	PreOptimizationReadinessGate *ReadinessGate           `json:"pre_optimization_readiness_gate,omitempty"`
	PreOptimizationBasketSafety  *BasketSafety            `json:"pre_optimization_basket_safety,omitempty"`
	BasketOptimizationPlan       *BasketOptimizationPlan  `json:"basket_optimization_plan,omitempty"`
	RecipeAdjustments            []RecipeAdjustment       `json:"recipe_adjustments,omitempty"`
	QuantityLedger               *QuantityLedger          `json:"quantity_ledger,omitempty"`
	BasketSafety                 *BasketSafety            `json:"basket_safety,omitempty"`
	ReadinessGate                *ReadinessGate           `json:"readiness_gate,omitempty"`
	PDFPath                      string                   `json:"pdf,omitempty"`
	BasketPath                   string                   `json:"basket,omitempty"`
	People                       int                      `json:"people,omitempty"`
	Days                         int                      `json:"days,omitempty"`
	Meals                        []string                 `json:"meals,omitempty"`
	Provenance                   []DataProvenance         `json:"provenance,omitempty"`
	Warnings                     []string                 `json:"warnings,omitempty"`
}

type FoodRunIngredientLink struct {
	Day                 int     `json:"day,omitempty"`
	Meal                string  `json:"meal,omitempty"`
	RecipeID            string  `json:"recipe_id,omitempty"`
	RecipeTitle         string  `json:"recipe_title,omitempty"`
	Ingredient          string  `json:"ingredient,omitempty"`
	RequiredQuantity    float64 `json:"required_qty,omitempty"`
	RequiredUnit        string  `json:"required_unit,omitempty"`
	SelectedProductID   string  `json:"selected_product_id,omitempty"`
	SelectedProductSKU  string  `json:"selected_product_sku,omitempty"`
	SelectedProductName string  `json:"selected_product_name,omitempty"`
	Status              string  `json:"status,omitempty"`
	Message             string  `json:"message,omitempty"`
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
