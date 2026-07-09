package food

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wachtermar/carrito/internal/money"
	"github.com/wachtermar/carrito/internal/strutil"
)

type HTMLPageOptions struct {
	CoverImageURL string
	GeneratedAt   time.Time
}

type htmlPageData struct {
	Title             string
	Subtitle          string
	GeneratedAt       string
	People            int
	PeopleLabel       string
	Days              []htmlDay
	DayCount          int
	DayCountLabel     string
	MealCount         int
	MealCountLabel    string
	MealNames         string
	SelectionPolicy   string
	Budget            string
	EstimatedTotal    string
	StatusLabel       string
	HeroImageURL      template.URL
	HeroImageText     string
	HeroImageAlt      string
	HeroMissing       bool
	HeroMissingReason string
	Shopping          htmlShopping
	Evidence          htmlEvidence
}

type htmlDay struct {
	ID     string
	Label  string
	Meals  []htmlMeal
	Active bool
}

type htmlMeal struct {
	Type          string
	Title         string
	Servings      int
	Time          string
	Tags          []string
	SourceURL     string
	ImageURL      template.URL
	ImageText     string
	ImageAlt      string
	ImageMissing  bool
	Ingredients   []htmlIngredient
	Equipment     []string
	Steps         []htmlStep
	Substitutions []string
	Nutrition     string
	PlanningNote  string
}

type htmlIngredient struct {
	Name     string
	Quantity string
	Unit     string
	Optional bool
}

type htmlStep struct {
	Number  int
	Title   string
	Text    string
	Minutes int
}

type htmlShopping struct {
	Visible        bool
	Complete       bool
	Status         string
	EstimatedTotal string
	Ingredients    []htmlIngredient
	Products       []htmlProduct
	Notes          []string
}

type htmlProduct struct {
	Ingredient      string
	Name            string
	Brand           string
	Price           string
	LineTotal       string
	Quantity        string
	PackageMath     string
	Reason          string
	ImageURL        template.URL
	ImageText       string
	ProductURL      string
	Warnings        []string
	EvidenceSource  string
	NutritionStatus string
}

type htmlEvidence struct {
	Visible         bool
	CookReady       string
	BasketReady     string
	RecipeQuality   string
	ProductEvidence string
	Nutrition       string
	Budget          string
	Caveats         []string
}

func WriteHTMLFromJSONFile(inputPath, outputPath string, opts HTMLPageOptions) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}
	opts.CoverImageURL = normalizeCoverImageURL(opts.CoverImageURL)
	page, err := htmlPageFromJSON(data, filepath.Dir(inputPath), opts)
	if err != nil {
		return fmt.Errorf("%s: %w", inputPath, err)
	}
	return writeFoodHTMLPage(outputPath, page)
}

func normalizeCoverImageURL(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	lower := strings.ToLower(ref)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "data:image/") || filepath.IsAbs(ref) {
		return ref
	}
	if _, err := os.Stat(ref); err == nil {
		if abs, absErr := filepath.Abs(ref); absErr == nil {
			return abs
		}
	}
	return ref
}

func htmlPageFromJSON(data []byte, imageBaseDir string, opts HTMLPageOptions) (htmlPageData, error) {
	var probe map[string]any
	if err := json.Unmarshal(data, &probe); err != nil {
		return htmlPageData{}, err
	}
	switch {
	case probe["mealplan"] != nil && probe["shop"] != nil:
		var artifact FoodRunArtifact
		if err := json.Unmarshal(data, &artifact); err != nil {
			return htmlPageData{}, err
		}
		return htmlPageForFoodRunArtifact(artifact, imageBaseDir, opts), nil
	case probe["days"] != nil:
		var plan MealPlan
		if err := json.Unmarshal(data, &plan); err != nil {
			return htmlPageData{}, err
		}
		return htmlPageForMealPlan(plan, ShopResult{}, nil, imageBaseDir, opts), nil
	case probe["ingredients"] != nil && probe["steps"] != nil:
		var recipe Recipe
		if err := json.Unmarshal(data, &recipe); err != nil {
			return htmlPageData{}, err
		}
		plan := MealPlan{People: recipe.Servings, Days: []DayPlan{{Day: 1, Meals: []Meal{{Type: "meal", Recipe: recipe}}}}}
		return htmlPageForMealPlan(plan, ShopResult{}, nil, imageBaseDir, opts), nil
	case probe["recipes"] != nil:
		var collection struct {
			MealPlanID string   `json:"mealplan_id"`
			Recipes    []Recipe `json:"recipes"`
		}
		if err := json.Unmarshal(data, &collection); err != nil {
			return htmlPageData{}, err
		}
		plan := MealPlan{ID: collection.MealPlanID}
		for i, recipe := range collection.Recipes {
			plan.Days = append(plan.Days, DayPlan{Day: i + 1, Meals: []Meal{{Type: "meal", Recipe: recipe}}})
		}
		return htmlPageForMealPlan(plan, ShopResult{}, nil, imageBaseDir, opts), nil
	case probe["selected_products"] != nil:
		var shop ShopResult
		if err := json.Unmarshal(data, &shop); err != nil {
			return htmlPageData{}, err
		}
		return htmlPageForMealPlan(MealPlan{}, shop, nil, imageBaseDir, opts), nil
	default:
		return htmlPageData{}, fmt.Errorf("not a supported food recipe, mealplan, shop, or food-run JSON file")
	}
}

func htmlPageForFoodRunArtifact(artifact FoodRunArtifact, imageBaseDir string, opts HTMLPageOptions) htmlPageData {
	return htmlPageForMealPlan(artifact.MealPlan, artifact.Shop, artifact.ReadinessGate, imageBaseDir, opts)
}

func htmlPageForMealPlan(plan MealPlan, shop ShopResult, gate *ReadinessGate, imageBaseDir string, opts HTMLPageOptions) htmlPageData {
	generatedAt := opts.GeneratedAt
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}
	page := htmlPageData{
		Title:           strutil.FirstNonEmpty(foodRunPDFTitle(FoodRunArtifact{MealPlan: plan}), "Cooking Plan"),
		GeneratedAt:     generatedAt.Format("2 Jan 2006 15:04"),
		People:          plan.People,
		DayCount:        len(plan.Days),
		SelectionPolicy: strutil.FirstNonEmpty(plan.SelectionPolicy, shop.Policy, "not set"),
		Budget:          strutil.FirstNonEmpty(plan.BudgetEUR, "not set"),
		EstimatedTotal:  formatMoneyForHTML(shop.EstimatedTotal),
		StatusLabel:     "Cook-ready plan",
	}
	if len(plan.Days) == 0 && len(shop.SelectedProducts) > 0 {
		page.Title = "Shopping Review"
		page.StatusLabel = "Shopping evidence"
	}
	mealTypes := map[string]bool{}
	for _, day := range plan.Days {
		htmlDay := htmlDay{ID: "day-" + strconv.Itoa(day.Day), Label: fmt.Sprintf("Day %d", day.Day), Active: len(page.Days) == 0}
		if strings.TrimSpace(day.Label) != "" {
			htmlDay.Label = day.Label
		}
		for _, meal := range day.Meals {
			mealTypes[formatMealType(meal.Type)] = true
			page.MealCount++
			htmlMeal := htmlMealForMeal(meal, imageBaseDir, opts.CoverImageURL, page.MealCount == 1)
			if page.HeroImageURL == "" && htmlMeal.ImageURL != "" {
				page.HeroImageURL = htmlMeal.ImageURL
				page.HeroImageText = htmlMeal.ImageText
				page.HeroImageAlt = htmlMeal.ImageAlt
			}
			htmlDay.Meals = append(htmlDay.Meals, htmlMeal)
		}
		page.Days = append(page.Days, htmlDay)
	}
	if page.HeroImageURL == "" {
		page.HeroMissing = true
		page.HeroMissingReason = "Dish photo missing. Product package photos are kept in the shopping section only."
	}
	page.MealNames = strings.Join(sortedMapKeys(mealTypes), ", ")
	if page.MealNames == "" {
		page.MealNames = "not set"
	}
	page.DayCountLabel = countLabel(page.DayCount, "day", "days")
	page.MealCountLabel = countLabel(page.MealCount, "meal", "meals")
	page.PeopleLabel = peopleCountLabel(page.People)
	if page.DayCount == 1 && len(page.Days) == 1 && len(page.Days[0].Meals) == 1 {
		page.Subtitle = page.Days[0].Meals[0].Type + " for " + strconv.Itoa(nonZero(plan.People, page.Days[0].Meals[0].Servings)) + " people"
	} else {
		page.Subtitle = fmt.Sprintf("%d days, %d meals", page.DayCount, page.MealCount)
	}
	page.Shopping = htmlShoppingForPlanAndShop(plan, shop, imageBaseDir)
	page.Evidence = htmlEvidenceForGate(gate)
	return page
}

func htmlMealForMeal(meal Meal, imageBaseDir, coverImage string, firstMeal bool) htmlMeal {
	recipe := meal.Recipe
	imageRef := strings.TrimSpace(recipe.ImageURL)
	if imageRef == "" && firstMeal {
		imageRef = strings.TrimSpace(coverImage)
	}
	htmlMeal := htmlMeal{
		Type:         formatMealType(meal.Type),
		Title:        recipe.Title,
		Servings:     recipe.Servings,
		Time:         recipeTimeLabel(recipe),
		Tags:         recipe.Tags,
		SourceURL:    recipe.SourceURL,
		ImageURL:     safeTemplateImageURL(imageRef, imageBaseDir),
		ImageText:    imageRef,
		ImageAlt:     recipe.Title,
		ImageMissing: strings.TrimSpace(imageRef) == "",
		Equipment:    recipe.Equipment,
		Nutrition:    nutritionSummaryLabel(recipe.NutritionPerServing),
		PlanningNote: meal.PlanningReason,
	}
	for _, ingredient := range recipe.Ingredients {
		htmlMeal.Ingredients = append(htmlMeal.Ingredients, htmlIngredientForIngredient(ingredient))
	}
	for _, step := range recipe.Steps {
		htmlMeal.Steps = append(htmlMeal.Steps, htmlStep{Number: step.Number, Title: step.Title, Text: step.Text, Minutes: step.Minutes})
	}
	htmlMeal.Substitutions = append(htmlMeal.Substitutions, recipe.Substitutions...)
	return htmlMeal
}

func htmlShoppingForPlanAndShop(plan MealPlan, shop ShopResult, imageBaseDir string) htmlShopping {
	shopping := htmlShopping{
		Visible:        len(plan.RequiredPurchases) > 0 || len(shop.SelectedProducts) > 0,
		Complete:       shop.Complete,
		Status:         shoppingStatusLabel(shop),
		EstimatedTotal: formatMoneyForHTML(shop.EstimatedTotal),
		Notes:          append([]string{}, shop.Notes...),
	}
	for _, ingredient := range plan.RequiredPurchases {
		shopping.Ingredients = append(shopping.Ingredients, htmlIngredientForIngredient(ingredient))
	}
	for _, selected := range shop.SelectedProducts {
		product := selected.Product
		item := htmlProduct{
			Ingredient:      selected.Ingredient.Name,
			Name:            product.Name,
			Brand:           product.Brand,
			Price:           formatMoneyForHTML(product.Price),
			LineTotal:       formatMoneyForHTML(selected.LineTotal),
			Quantity:        selected.PurchaseQuantity,
			PackageMath:     selected.QuantityReason,
			Reason:          selected.SelectionReason,
			ImageURL:        safeTemplateImageURL(product.ImageURL, imageBaseDir),
			ImageText:       product.ImageURL,
			ProductURL:      product.URL,
			Warnings:        append([]string{}, selected.Warnings...),
			EvidenceSource:  selected.ProductEvidenceSource,
			NutritionStatus: strings.Join(selected.NutritionWarnings, "; "),
		}
		if item.Quantity == "" && selected.PackageCount > 0 {
			item.Quantity = strconv.Itoa(selected.PackageCount)
		}
		if item.LineTotal == "" {
			item.LineTotal = item.Price
		}
		shopping.Products = append(shopping.Products, item)
	}
	return shopping
}

func htmlEvidenceForGate(gate *ReadinessGate) htmlEvidence {
	if gate == nil {
		return htmlEvidence{}
	}
	ev := htmlEvidence{
		Visible:         true,
		CookReady:       boolLabel(gate.SafeToCook),
		BasketReady:     boolLabel(gate.SafeToBuild),
		RecipeQuality:   strutil.FirstNonEmpty(gate.RecipeQualityStatus, "not evaluated"),
		ProductEvidence: strutil.FirstNonEmpty(gate.ProductEvidenceStatus, "not evaluated"),
		Nutrition:       strutil.FirstNonEmpty(gate.NutritionStatus, "not evaluated"),
		Budget:          strutil.FirstNonEmpty(gate.BudgetStatus, "not evaluated"),
	}
	for _, warning := range gate.Warnings {
		if strings.TrimSpace(warning.Message) != "" {
			ev.Caveats = append(ev.Caveats, warning.Message)
		}
	}
	for _, issue := range gate.BlockingIssues {
		if strings.TrimSpace(issue.Message) != "" {
			ev.Caveats = append(ev.Caveats, issue.Message)
		}
	}
	return ev
}

func htmlIngredientForIngredient(ingredient Ingredient) htmlIngredient {
	return htmlIngredient{
		Name:     ingredient.Name,
		Quantity: formatHTMLQuantity(ingredient.Quantity),
		Unit:     ingredient.Unit,
		Optional: ingredient.Optional,
	}
}

func writeFoodHTMLPage(outputPath string, page htmlPageData) error {
	if strings.TrimSpace(outputPath) == "" {
		return fmt.Errorf("output path is required")
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o700); err != nil && filepath.Dir(outputPath) != "." {
		return err
	}
	tmpl, err := template.New("food-html").Parse(foodHTMLTemplate)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, page); err != nil {
		return err
	}
	return os.WriteFile(outputPath, buf.Bytes(), 0o600)
}

func safeTemplateImageURL(ref, baseDir string) template.URL {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	lower := strings.ToLower(ref)
	switch {
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"), strings.HasPrefix(lower, "data:image/"):
		return template.URL(ref)
	case filepath.IsAbs(ref):
		if dataURL := localImageDataURL(ref); dataURL != "" {
			return template.URL(dataURL)
		}
		return template.URL(fileURL(ref))
	default:
		resolved := filepath.Join(baseDir, ref)
		if _, err := os.Stat(resolved); err == nil {
			if dataURL := localImageDataURL(resolved); dataURL != "" {
				return template.URL(dataURL)
			}
			return template.URL(fileURL(resolved))
		}
		return template.URL(ref)
	}
}

func localImageDataURL(path string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return ""
	}
	contentType := http.DetectContentType(data)
	if !strings.HasPrefix(contentType, "image/") {
		return ""
	}
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func fileURL(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}).String()
}

func formatHTMLQuantity(qty float64) string {
	if qty <= 0 {
		return ""
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", qty), "0"), ".")
}

func recipeTimeLabel(recipe Recipe) string {
	parts := []string{}
	if recipe.PrepMinutes > 0 {
		parts = append(parts, fmt.Sprintf("prep %d min", recipe.PrepMinutes))
	}
	if recipe.CookMinutes > 0 {
		parts = append(parts, fmt.Sprintf("cook %d min", recipe.CookMinutes))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
}

func nutritionSummaryLabel(summary *NutritionSummary) string {
	if summary == nil {
		return ""
	}
	return formatNutritionSummary(*summary)
}

func formatMoneyForHTML(value money.Money) string {
	if value.Amount == "" && value.Cents == 0 {
		return ""
	}
	return formatPDFMoney(value.Amount, value.Currency)
}

func boolLabel(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func nonZero(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func countLabel(count int, singular, plural string) string {
	if count == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", count, plural)
}

func peopleCountLabel(count int) string {
	switch count {
	case 0:
		return "people not set"
	case 1:
		return "1 person"
	default:
		return fmt.Sprintf("%d people", count)
	}
}

func sortedMapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

const foodHTMLTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
:root {
  color-scheme: light;
  --bg: #f7f8f4;
  --paper: #ffffff;
  --ink: #1d2520;
  --muted: #657168;
  --line: #dfe5dc;
  --leaf: #27664c;
  --leaf-dark: #173d30;
  --tomato: #c45135;
  --amber: #d59a2a;
  --sky: #dbeaf1;
}
* { box-sizing: border-box; }
html { scroll-behavior: smooth; }
body {
  margin: 0;
  font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  color: var(--ink);
  background: var(--bg);
  line-height: 1.45;
  letter-spacing: 0;
}
a { color: inherit; }
.shell { max-width: 1120px; margin: 0 auto; }
.hero {
  background: var(--leaf-dark);
  color: #fff;
}
.hero-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(320px, 520px);
  gap: 28px;
  align-items: stretch;
  padding: 28px 22px 18px;
}
.eyebrow { color: #d4e7dd; font-size: 0.85rem; font-weight: 700; text-transform: uppercase; }
h1 { font-size: clamp(2rem, 5vw, 4rem); line-height: 1.02; margin: 10px 0 14px; max-width: 760px; }
.subtitle { color: #e7f2ec; font-size: 1.1rem; margin: 0 0 18px; }
.meta-grid { display: flex; flex-wrap: wrap; gap: 8px; margin: 18px 0 0; }
.pill {
  display: inline-flex;
  min-height: 36px;
  align-items: center;
  border: 1px solid rgba(255,255,255,.24);
  border-radius: 999px;
  padding: 6px 12px;
  background: rgba(255,255,255,.08);
  color: #fff;
  font-size: .92rem;
  white-space: nowrap;
}
.hero-media {
  min-height: 280px;
  border-radius: 8px;
  overflow: hidden;
  background: #e9eee8;
  color: var(--ink);
  display: grid;
  place-items: center;
}
.hero-media img { width: 100%; height: 100%; object-fit: cover; display: block; }
.missing-photo { padding: 28px; text-align: left; width: 100%; }
.missing-photo strong { display: block; font-size: 1.15rem; margin-bottom: 8px; }
.missing-photo span { color: var(--muted); }
.day-nav-wrap {
  position: sticky;
  top: 0;
  z-index: 5;
  background: rgba(247,248,244,.94);
  border-bottom: 1px solid var(--line);
  backdrop-filter: blur(10px);
}
.day-nav {
  display: flex;
  gap: 8px;
  overflow-x: auto;
  padding: 10px 22px;
}
.day-nav a {
  flex: 0 0 auto;
  min-height: 44px;
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  padding: 0 16px;
  background: var(--paper);
  border: 1px solid var(--line);
  text-decoration: none;
  font-weight: 700;
}
main { padding: 20px 22px 52px; }
.section {
  margin: 22px 0;
  padding: 0;
}
.section-head {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: baseline;
  margin: 0 0 12px;
}
h2 { font-size: clamp(1.55rem, 4vw, 2.2rem); margin: 0; line-height: 1.08; }
h3 { font-size: 1.25rem; margin: 0; }
.meal {
  background: var(--paper);
  border: 1px solid var(--line);
  border-radius: 8px;
  overflow: clip;
}
.meal-layout {
  display: grid;
  grid-template-columns: 280px minmax(0, 1fr);
  gap: 0;
}
.meal-photo { background: #eef3ed; min-height: 280px; }
.meal-photo img { width: 100%; height: 100%; object-fit: cover; display: block; }
.meal-body { padding: 18px; }
.meal-kicker { color: var(--tomato); font-weight: 800; text-transform: uppercase; font-size: .78rem; }
.meal-title { font-size: clamp(1.6rem, 4vw, 2.4rem); line-height: 1.08; margin: 4px 0 10px; }
.meal-meta { display: flex; flex-wrap: wrap; gap: 8px; margin: 0 0 16px; }
.tag {
  min-height: 30px;
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  background: var(--sky);
  color: #1a3a45;
  padding: 4px 10px;
  font-size: .84rem;
  font-weight: 700;
}
.cook-grid {
  display: grid;
  grid-template-columns: minmax(0, .85fr) minmax(0, 1.15fr);
  gap: 18px;
}
.panel {
  border-top: 1px solid var(--line);
  padding-top: 14px;
}
.ingredient-list, .steps, .product-list, .caveats { margin: 0; padding: 0; list-style: none; }
.ingredient-list li {
  display: grid;
  grid-template-columns: minmax(76px, max-content) minmax(0, 1fr);
  gap: 10px;
  padding: 9px 0;
  border-bottom: 1px solid var(--line);
}
.amount { color: var(--leaf); font-weight: 800; white-space: nowrap; }
.steps { counter-reset: step; }
.steps li {
  display: grid;
  grid-template-columns: 36px minmax(0, 1fr);
  gap: 10px;
  padding: 12px 0;
  border-bottom: 1px solid var(--line);
}
.step-num {
  width: 36px;
  height: 36px;
  border-radius: 999px;
  background: var(--leaf);
  color: #fff;
  display: grid;
  place-items: center;
  font-weight: 800;
}
.step-title { display: block; font-weight: 800; margin-bottom: 3px; }
.shopping-summary {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
  margin-bottom: 14px;
}
.summary-tile {
  background: var(--paper);
  border: 1px solid var(--line);
  border-radius: 8px;
  padding: 14px;
}
.summary-tile span { display: block; color: var(--muted); font-size: .82rem; font-weight: 700; text-transform: uppercase; }
.summary-tile strong { display: block; font-size: 1.25rem; margin-top: 3px; }
.product-list {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}
.product {
  background: var(--paper);
  border: 1px solid var(--line);
  border-radius: 8px;
  overflow: clip;
  display: grid;
  grid-template-columns: 132px minmax(0, 1fr);
}
.product-img {
  min-height: 132px;
  background: #edf2ef;
  display: grid;
  place-items: center;
  color: var(--muted);
  font-size: .85rem;
  text-align: center;
  padding: 10px;
}
.product-img img { width: 100%; height: 100%; object-fit: contain; display: block; }
.product-body { padding: 12px; }
.product-body h3 { font-size: 1rem; line-height: 1.2; }
.product-label { color: var(--tomato); font-size: .78rem; font-weight: 800; text-transform: uppercase; margin-bottom: 4px; }
.muted { color: var(--muted); }
.evidence-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}
.evidence {
  background: var(--paper);
  border: 1px solid var(--line);
  border-radius: 8px;
  padding: 14px;
}
.evidence span { color: var(--muted); display: block; font-size: .82rem; font-weight: 700; text-transform: uppercase; }
.evidence strong { display: block; margin-top: 2px; }
.caveats li {
  border-left: 3px solid var(--amber);
  background: #fff9e8;
  padding: 8px 10px;
  margin: 8px 0;
  border-radius: 0 8px 8px 0;
}
.source-link { color: var(--leaf); font-weight: 800; word-break: break-word; }
@media (max-width: 760px) {
  .hero-grid { grid-template-columns: 1fr; padding: 22px 16px 14px; gap: 18px; }
  .hero-media { min-height: 230px; }
  main { padding: 16px 16px 42px; }
  .meal-layout, .cook-grid, .shopping-summary, .product-list, .evidence-grid { grid-template-columns: 1fr; }
  .meal-photo { min-height: 230px; }
  .meal-body { padding: 16px; }
  .product { grid-template-columns: 110px minmax(0, 1fr); }
  .day-nav { padding: 9px 16px; }
}
@media (max-width: 420px) {
  .product { grid-template-columns: 1fr; }
  .product-img { min-height: 160px; }
  .ingredient-list li { grid-template-columns: 72px minmax(0, 1fr); }
}
@media print {
  .day-nav-wrap { display: none; }
  body { background: #fff; }
  .hero { color: var(--ink); background: #fff; border-bottom: 1px solid var(--line); }
  .pill { color: var(--ink); border-color: var(--line); background: #fff; }
}
</style>
</head>
<body>
<header class="hero">
  <div class="shell hero-grid">
    <div>
      <div class="eyebrow">{{.StatusLabel}}</div>
      <h1>{{.Title}}</h1>
      <p class="subtitle">{{.Subtitle}}</p>
      <div class="meta-grid" aria-label="Plan facts">
        <span class="pill">{{.DayCountLabel}}</span>
        <span class="pill">{{.MealCountLabel}}</span>
        <span class="pill">{{.PeopleLabel}}</span>
        <span class="pill">{{.MealNames}}</span>
        {{if .EstimatedTotal}}<span class="pill">{{.EstimatedTotal}}</span>{{end}}
      </div>
    </div>
    <div class="hero-media">
      {{if .HeroImageURL}}<img src="{{.HeroImageURL}}" alt="{{.HeroImageAlt}}">{{else}}<div class="missing-photo"><strong>Dish photo missing</strong><span>{{.HeroMissingReason}}</span></div>{{end}}
    </div>
  </div>
</header>
{{if .Days}}
<div class="day-nav-wrap">
  <nav class="shell day-nav" aria-label="Days">
    {{range .Days}}<a href="#{{.ID}}">{{.Label}}</a>{{end}}
    {{if $.Shopping.Visible}}<a href="#shopping">Shopping</a>{{end}}
    {{if $.Evidence.Visible}}<a href="#evidence">Evidence</a>{{end}}
  </nav>
</div>
{{end}}
<main class="shell">
  {{range .Days}}
  <section class="section" id="{{.ID}}">
    <div class="section-head"><h2>{{.Label}}</h2></div>
    {{range .Meals}}
    <article class="meal">
      <div class="meal-layout">
        <div class="meal-photo">
          {{if .ImageURL}}<img src="{{.ImageURL}}" alt="{{.ImageAlt}}">{{else}}<div class="missing-photo"><strong>Recipe photo unavailable</strong><span>This page will not use product packaging as a stand-in for the dish.</span></div>{{end}}
        </div>
        <div class="meal-body">
          <div class="meal-kicker">{{.Type}}</div>
          <h3 class="meal-title">{{.Title}}</h3>
          <div class="meal-meta">
            {{if .Servings}}<span class="tag">{{.Servings}} servings</span>{{end}}
            {{if .Time}}<span class="tag">{{.Time}}</span>{{end}}
            {{if .Nutrition}}<span class="tag">{{.Nutrition}}</span>{{end}}
          </div>
          {{if .SourceURL}}<p class="muted">Recipe source: <a class="source-link" href="{{.SourceURL}}">{{.SourceURL}}</a></p>{{end}}
          {{if .PlanningNote}}<p class="muted">{{.PlanningNote}}</p>{{end}}
          <div class="cook-grid">
            <section class="panel">
              <h3>Ingredients</h3>
              <ul class="ingredient-list">
                {{range .Ingredients}}<li><span class="amount">{{.Quantity}} {{.Unit}}</span><span>{{.Name}}{{if .Optional}} <span class="muted">(optional)</span>{{end}}</span></li>{{end}}
              </ul>
            </section>
            <section class="panel">
              <h3>Steps</h3>
              <ol class="steps">
                {{range .Steps}}<li><span class="step-num">{{.Number}}</span><span>{{if .Title}}<span class="step-title">{{.Title}}</span>{{end}}{{.Text}}{{if .Minutes}} <span class="muted">({{.Minutes}} min)</span>{{end}}</span></li>{{end}}
              </ol>
            </section>
          </div>
          {{if .Substitutions}}<section class="panel"><h3>Swaps</h3><ul>{{range .Substitutions}}<li>{{.}}</li>{{end}}</ul></section>{{end}}
        </div>
      </div>
    </article>
    {{end}}
  </section>
  {{end}}
  {{if .Shopping.Visible}}
  <section class="section" id="shopping">
    <div class="section-head"><h2>Shopping</h2><span class="muted">Cart was not changed</span></div>
    <div class="shopping-summary">
      <div class="summary-tile"><span>Status</span><strong>{{.Shopping.Status}}</strong></div>
      <div class="summary-tile"><span>Total</span><strong>{{.Shopping.EstimatedTotal}}</strong></div>
      <div class="summary-tile"><span>Policy</span><strong>{{.SelectionPolicy}}</strong></div>
    </div>
    {{if .Shopping.Ingredients}}
    <section class="panel"><h3>Ingredients to buy</h3><ul class="ingredient-list">{{range .Shopping.Ingredients}}<li><span class="amount">{{.Quantity}} {{.Unit}}</span><span>{{.Name}}</span></li>{{end}}</ul></section>
    {{end}}
    {{if .Shopping.Products}}
    <section class="panel">
      <h3>Selected Alcampo products</h3>
      <div class="product-list">
        {{range .Shopping.Products}}
        <article class="product">
          <div class="product-img">{{if .ImageURL}}<img src="{{.ImageURL}}" alt="{{.Name}}">{{else}}No product photo{{end}}</div>
          <div class="product-body">
            <div class="product-label">Alcampo product photo</div>
            <h3>{{.Name}}</h3>
            <p class="muted">{{.Ingredient}}</p>
            <p><strong>{{.LineTotal}}</strong>{{if .Quantity}} - buy {{.Quantity}}{{end}}</p>
            {{if .PackageMath}}<p class="muted">{{.PackageMath}}</p>{{end}}
            {{if .Reason}}<p class="muted">{{.Reason}}</p>{{end}}
          </div>
        </article>
        {{end}}
      </div>
    </section>
    {{end}}
  </section>
  {{end}}
  {{if .Evidence.Visible}}
  <section class="section" id="evidence">
    <div class="section-head"><h2>Evidence</h2><span class="muted">For audit, not cooking</span></div>
    <div class="evidence-grid">
      <div class="evidence"><span>Cook-ready</span><strong>{{.Evidence.CookReady}}</strong></div>
      <div class="evidence"><span>Basket-ready</span><strong>{{.Evidence.BasketReady}}</strong></div>
      <div class="evidence"><span>Recipe quality</span><strong>{{.Evidence.RecipeQuality}}</strong></div>
      <div class="evidence"><span>Products</span><strong>{{.Evidence.ProductEvidence}}</strong></div>
      <div class="evidence"><span>Nutrition</span><strong>{{.Evidence.Nutrition}}</strong></div>
      <div class="evidence"><span>Budget</span><strong>{{.Evidence.Budget}}</strong></div>
    </div>
    {{if .Evidence.Caveats}}<ul class="caveats">{{range .Evidence.Caveats}}<li>{{.}}</li>{{end}}</ul>{{end}}
  </section>
  {{end}}
</main>
</body>
</html>
`
