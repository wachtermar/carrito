package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/mealplan"
	"github.com/wachtermar/carrito/internal/money"
	"github.com/wachtermar/carrito/internal/output"
)

func runMealPlan(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printMealPlanHelp(stdout)
		return nil
	}
	switch args[0] {
	case "validate":
		return runMealPlanValidate(args[1:], stdout, stderr)
	case "candidates":
		return runMealPlanCandidates(args[1:], stdout, stderr)
	case "build":
		return runMealPlanBuild(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown mealplan command %q", args[0])
	}
}

func printMealPlanHelp(w io.Writer) {
	fmt.Fprintln(w, "carrito mealplan - turn a Hermes meal plan into Alcampo shopping and a cooking page")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  carrito mealplan validate <plan.json> [--selected] [--json]")
	fmt.Fprintln(w, "  carrito mealplan candidates <plan.json> [--limit N] [--json]")
	fmt.Fprintln(w, "  carrito mealplan build <plan.json> [--html-out plan.html] [--basket-out basket.txt] [--json]")
}

func runMealPlanValidate(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("mealplan validate", stderr)
	selected := fs.Bool("selected", false, "require selected Alcampo products and package quantities")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	if err := parseInterspersed(fs, args, map[string]bool{"selected": true, "json": true}); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("mealplan validate requires <plan.json>")
	}
	plan, err := mealplan.Load(fs.Arg(0))
	if err != nil {
		return err
	}
	issues := mealplan.ValidateDraft(plan)
	if *selected {
		issues = mealplan.ValidateSelected(plan)
	}
	result := struct {
		Valid  bool             `json:"valid"`
		Issues []mealplan.Issue `json:"issues"`
	}{Valid: len(issues) == 0, Issues: issues}
	if *jsonOut {
		if err := output.JSON(stdout, result); err != nil {
			return err
		}
		return mealplan.FormatIssues(issues)
	}
	if err := mealplan.FormatIssues(issues); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "valid meal plan")
	return nil
}

type candidateResult struct {
	Shopping mealplan.ShoppingItem `json:"shopping"`
	Products []candidateProduct    `json:"products"`
	Error    string                `json:"error,omitempty"`
}

type candidateResponse struct {
	Status          string            `json:"status"`
	UnresolvedCount int               `json:"unresolved_count"`
	Items           []candidateResult `json:"items"`
}

// candidateProduct keeps Hermes' selection context compact while retaining
// the fields needed for package fit, price, availability, and hard constraints.
type candidateProduct struct {
	ID                string      `json:"id"`
	SKU               string      `json:"sku,omitempty"`
	Name              string      `json:"name"`
	Brand             string      `json:"brand,omitempty"`
	Price             money.Money `json:"price"`
	Size              string      `json:"size,omitempty"`
	Unit              string      `json:"unit,omitempty"`
	Category          string      `json:"category,omitempty"`
	Available         *bool       `json:"available,omitempty"`
	Ingredients       string      `json:"ingredients,omitempty"`
	Allergens         string      `json:"allergens,omitempty"`
	Description       string      `json:"description,omitempty"`
	DetailUnavailable bool        `json:"detail_unavailable,omitempty"`
}

func runMealPlanCandidates(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("mealplan candidates", stderr)
	limit := fs.Int("limit", 4, "maximum Alcampo candidates per shopping item")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	store := marketFlag(fs, "Alcampo region/store UUID to use for pricing")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("mealplan candidates requires <plan.json>")
	}
	if *limit < 1 || *limit > 10 {
		return errors.New("--limit must be between 1 and 10")
	}
	plan, err := mealplan.Load(fs.Arg(0))
	if err != nil {
		return err
	}
	if err := mealplan.FormatIssues(mealplan.ValidateDraft(plan)); err != nil {
		return err
	}
	if len(plan.Shopping) == 0 {
		response := candidateResponse{Status: "ready", Items: []candidateResult{}}
		if *jsonOut {
			return output.JSON(stdout, response)
		}
		fmt.Fprintln(stdout, "no shopping needed; every ingredient is explicitly available in the pantry")
		return nil
	}
	cfg, client, err := newClient(*store)
	if err != nil {
		return err
	}
	regionID, err := resolveMarket(cfg, *store)
	if err != nil {
		return err
	}
	results := make([]candidateResult, 0, len(plan.Shopping))
	unresolved := 0
	for _, item := range plan.Shopping {
		result := candidateResult{Shopping: item}
		products, searchErr := client.Search(context.Background(), item.Query, alcampo.SearchOptions{Limit: *limit, RegionID: regionID})
		if searchErr != nil {
			result.Error = searchErr.Error()
			unresolved++
		} else {
			for _, product := range products {
				if buildableCandidate(product) {
					result.Products = append(result.Products, compactCandidate(product))
				}
			}
			if len(result.Products) == 0 {
				result.Error = "no currently available products found"
				unresolved++
			}
		}
		results = append(results, result)
	}
	status := "ready"
	if unresolved > 0 {
		status = "incomplete"
	}
	response := candidateResponse{Status: status, UnresolvedCount: unresolved, Items: results}
	if *jsonOut {
		if err := output.JSON(stdout, response); err != nil {
			return err
		}
		if unresolved > 0 {
			return fmt.Errorf("%d shopping item(s) have no usable candidates", unresolved)
		}
		return nil
	}
	for _, result := range results {
		fmt.Fprintf(stdout, "%s (%s)\n", result.Shopping.Name, result.Shopping.Needed)
		if result.Error != "" {
			fmt.Fprintf(stdout, "  error: %s\n", result.Error)
			continue
		}
		for _, product := range result.Products {
			fmt.Fprintf(stdout, "  %s\t%s\t%s\t€%s\t%s\n", product.ID, product.SKU, product.Name, product.Price.Amount, product.Size)
		}
	}
	if unresolved > 0 {
		return fmt.Errorf("%d shopping item(s) have no usable candidates", unresolved)
	}
	return nil
}

func buildableCandidate(product alcampo.Product) bool {
	if strings.TrimSpace(product.ID) == "" || strings.TrimSpace(product.SKU) == "" || strings.TrimSpace(product.Price.Amount) == "" || product.Available == nil || !*product.Available {
		return false
	}
	if product.Price.Cents < 0 || (product.Price.Currency != "" && !strings.EqualFold(product.Price.Currency, "EUR")) {
		return false
	}
	if strings.TrimSpace(product.Size) != "" {
		return true
	}
	text := strings.ToLower(strings.Join([]string{product.Name, product.Size, product.Description}, " "))
	return strings.Contains(text, "al peso") || strings.Contains(text, "peso variable") || strings.Contains(text, "variable weight")
}

func compactCandidate(product alcampo.Product) candidateProduct {
	return candidateProduct{
		ID:                product.ID,
		SKU:               product.SKU,
		Name:              product.Name,
		Brand:             product.Brand,
		Price:             product.Price,
		Size:              product.Size,
		Unit:              product.Unit,
		Category:          product.Category,
		Available:         product.Available,
		Ingredients:       product.Ingredients,
		Allergens:         product.Allergens,
		Description:       product.Description,
		DetailUnavailable: product.DetailUnavailable,
	}
}

type mealPlanBuildResult struct {
	HTMLPath       string              `json:"html_path"`
	BasketPath     string              `json:"basket_path"`
	EstimatedTotal money.Money         `json:"estimated_total"`
	Items          []mealPlanBuildItem `json:"items"`
	Warnings       []string            `json:"warnings,omitempty"`
}

type mealPlanBuildItem struct {
	Name        string      `json:"name"`
	Needed      string      `json:"needed"`
	ProductID   string      `json:"product_id"`
	SKU         string      `json:"sku,omitempty"`
	ProductName string      `json:"product_name"`
	Packages    float64     `json:"packages"`
	LineTotal   money.Money `json:"line_total"`
	SafetyNote  string      `json:"safety_note,omitempty"`
}

func runMealPlanBuild(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("mealplan build", stderr)
	htmlOut := fs.String("html-out", "", "shareable cooking HTML path")
	basketOut := fs.String("basket-out", "", "guarded cart basket path")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	store := marketFlag(fs, "Alcampo region/store UUID to use for pricing")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("mealplan build requires <plan.json>")
	}
	planPath := fs.Arg(0)
	plan, err := mealplan.Load(planPath)
	if err != nil {
		return err
	}
	if err := mealplan.FormatIssues(mealplan.ValidateSelected(plan)); err != nil {
		return err
	}
	if *htmlOut == "" {
		*htmlOut = replaceExtension(planPath, ".html")
	}
	if *basketOut == "" {
		*basketOut = replaceExtension(planPath, ".basket.txt")
	}
	htmlAbs, err := filepath.Abs(*htmlOut)
	if err != nil {
		return err
	}
	basketAbs, err := filepath.Abs(*basketOut)
	if err != nil {
		return err
	}
	planAbs, err := filepath.Abs(planPath)
	if err != nil {
		return err
	}
	if sameFilePath(htmlAbs, basketAbs) {
		return errors.New("--html-out and --basket-out must be different paths")
	}
	if sameFilePath(planAbs, htmlAbs) || sameFilePath(planAbs, basketAbs) {
		return errors.New("output paths must not overwrite the input meal-plan JSON")
	}
	products := make([]alcampo.Product, 0, len(plan.Shopping))
	if len(plan.Shopping) > 0 {
		cfg, client, clientErr := newClient(*store)
		if clientErr != nil {
			return clientErr
		}
		regionID, marketErr := resolveMarket(cfg, *store)
		if marketErr != nil {
			return marketErr
		}
		client.RegionID = regionID
		for index, item := range plan.Shopping {
			product, lookupErr := client.Product(context.Background(), item.ProductSKU)
			if lookupErr != nil {
				return fmt.Errorf("shopping[%d] %q: %w", index, item.Name, lookupErr)
			}
			products = append(products, product)
		}
	}
	build, err := mealplan.NewBuild(plan, products)
	if err != nil {
		return err
	}
	if err := writeBasket(*basketOut, mealplan.BasketLines(build)); err != nil {
		return err
	}
	if err := mealplan.WriteHTML(*htmlOut, build, time.Now()); err != nil {
		return err
	}
	result := mealPlanBuildResult{
		HTMLPath:       htmlAbs,
		BasketPath:     basketAbs,
		EstimatedTotal: build.EstimatedTotal,
		Warnings:       build.Warnings,
	}
	for _, item := range build.Items {
		result.Items = append(result.Items, mealPlanBuildItem{
			Name:        item.Shopping.Name,
			Needed:      item.Shopping.Needed,
			ProductID:   item.Product.ID,
			SKU:         item.Product.SKU,
			ProductName: item.Product.Name,
			Packages:    item.Shopping.Packages,
			LineTotal:   item.LineTotal,
			SafetyNote:  item.SafetyNote,
		})
	}
	if *jsonOut {
		return output.JSON(stdout, result)
	}
	fmt.Fprintf(stdout, "html\t%s\n", result.HTMLPath)
	fmt.Fprintf(stdout, "basket\t%s\n", result.BasketPath)
	fmt.Fprintf(stdout, "estimated products total\t€%s\n", build.EstimatedTotal.Amount)
	return nil
}

func replaceExtension(path, suffix string) string {
	ext := filepath.Ext(path)
	if ext == "" {
		return path + suffix
	}
	return strings.TrimSuffix(path, ext) + suffix
}

func writeBasket(path string, lines []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data := strings.Join(lines, "\n")
	if data != "" {
		data += "\n"
	}
	return atomicWrite(path, []byte(data), 0o600)
}

func sameFilePath(left, right string) bool {
	if canonicalOutputPath(left) == canonicalOutputPath(right) {
		return true
	}
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	return leftErr == nil && rightErr == nil && os.SameFile(leftInfo, rightInfo)
}

// canonicalOutputPath resolves the longest existing prefix before comparing
// paths. EvalSymlinks cannot resolve a file that has not been created yet, but
// its parent may still be a symlink to the same output directory.
func canonicalOutputPath(name string) string {
	cleaned := filepath.Clean(name)
	current := cleaned
	var missing []string
	for {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return cleaned
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func atomicWrite(path string, data []byte, mode os.FileMode) (returnErr error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		if returnErr != nil {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(mode); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}
