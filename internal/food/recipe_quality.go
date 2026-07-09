package food

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"image"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	RecipeIntakeNotRun              = "not_run"
	RecipeIntakeComplete            = "complete"
	RecipeIntakeCompleteWithWarning = "complete_with_warnings"
	RecipeIntakeFailed              = "failed"

	RecipeQualityNotRun          = "not_run"
	RecipeQualityPass            = "pass"
	RecipeQualityPassWithWarning = "pass_with_warnings"
	RecipeQualityFail            = "fail"

	RecipeImageMissing   = "missing"
	RecipeImageCached    = "cached"
	RecipeImageLocal     = "local_file"
	RecipeImageRemote    = "remote_unverified"
	RecipeImageFailed    = "failed"
	RecipeImageNotCached = "not_cached"
)

type RecipeIntakePolicy struct {
	StrictRecipeQuality             bool `json:"strict_recipe_quality"`
	RequireBaseServings             bool `json:"require_base_servings"`
	RequireParseableRequiredAmounts bool `json:"require_parseable_required_amounts"`
	RequireCookingSteps             bool `json:"require_cooking_steps"`
	RequireRecipeSource             bool `json:"require_recipe_source"`
	RequireRecipeImages             bool `json:"require_recipe_images"`
	AllowStructuredURLImport        bool `json:"allow_structured_url_import"`
	AllowUnstructuredScrape         bool `json:"allow_unstructured_scrape"`
	CacheImages                     bool `json:"cache_images"`
}

type RecipeIntakeOptions struct {
	Enabled             bool
	RecipeFiles         []string
	RecipeDirs          []string
	RecipeURLs          []string
	ImageCacheDir       string
	InjectImportedMeals bool
	Policy              RecipeIntakePolicy
	HTTPClient          *http.Client
}

type RecipeIntakePlan struct {
	SchemaVersion        string                    `json:"schema_version"`
	Status               string                    `json:"status"`
	Policy               RecipeIntakePolicy        `json:"policy"`
	RequestedSources     []RecipeSourceEvidence    `json:"requested_sources,omitempty"`
	Imports              []RecipeImportResult      `json:"imports,omitempty"`
	AppliedMealSlots     []RecipeIntakeAppliedSlot `json:"applied_meal_slots,omitempty"`
	ImportedRecipeCount  int                       `json:"imported_recipe_count"`
	ActiveRecipeCount    int                       `json:"active_recipe_count"`
	RecipeSetFingerprint string                    `json:"recipe_set_fingerprint,omitempty"`
	Warnings             []string                  `json:"warnings,omitempty"`
	Errors               []string                  `json:"errors,omitempty"`
}

type RecipeSourceEvidence struct {
	SourceID       string `json:"source_id,omitempty"`
	SourceType     string `json:"source_type,omitempty"`
	Path           string `json:"path,omitempty"`
	URL            string `json:"url,omitempty"`
	Title          string `json:"title,omitempty"`
	Author         string `json:"author,omitempty"`
	License        string `json:"license,omitempty"`
	StructuredType string `json:"structured_type,omitempty"`
	Confidence     string `json:"confidence,omitempty"`
	Message        string `json:"message,omitempty"`
	ImageURL       string `json:"image_url,omitempty"`
	BaseServings   int    `json:"base_servings,omitempty"`
	RetrievedAt    string `json:"retrieved_at,omitempty"`
}

type RecipeImportResult struct {
	Source    RecipeSourceEvidence `json:"source"`
	Status    string               `json:"status"`
	RecipeIDs []string             `json:"recipe_ids,omitempty"`
	Count     int                  `json:"count"`
	Warnings  []string             `json:"warnings,omitempty"`
	Error     string               `json:"error,omitempty"`
}

type RecipeIntakeAppliedSlot struct {
	Day           int    `json:"day"`
	MealSlot      string `json:"meal_slot"`
	RecipeID      string `json:"recipe_id,omitempty"`
	RecipeTitle   string `json:"recipe_title,omitempty"`
	SourceID      string `json:"source_id,omitempty"`
	SourceType    string `json:"source_type,omitempty"`
	OriginalID    string `json:"original_recipe_id,omitempty"`
	OriginalTitle string `json:"original_recipe_title,omitempty"`
}

type RecipeQualityReport struct {
	SchemaVersion            string                `json:"schema_version"`
	Status                   string                `json:"status"`
	Policy                   RecipeIntakePolicy    `json:"policy"`
	Summary                  RecipeQualitySummary  `json:"summary"`
	Items                    []RecipeQualityItem   `json:"items,omitempty"`
	BlockingIssues           []RecipeQualityIssue  `json:"blocking_issues,omitempty"`
	Warnings                 []RecipeQualityIssue  `json:"warnings,omitempty"`
	ImageEvidence            []RecipeImageEvidence `json:"image_evidence,omitempty"`
	RecipeSetFingerprint     string                `json:"recipe_set_fingerprint,omitempty"`
	RecipeQualityFingerprint string                `json:"recipe_quality_fingerprint,omitempty"`
	RecipeImageFingerprint   string                `json:"recipe_image_fingerprint,omitempty"`
}

type RecipeQualitySummary struct {
	RecipeCount                          int `json:"recipe_count"`
	PassingRecipes                       int `json:"passing_recipes"`
	RecipesWithWarnings                  int `json:"recipes_with_warnings"`
	FailingRecipes                       int `json:"failing_recipes"`
	RecipesWithSource                    int `json:"recipes_with_source"`
	RecipesWithImages                    int `json:"recipes_with_images"`
	CachedImages                         int `json:"cached_images"`
	MissingImages                        int `json:"missing_images"`
	BrokenImages                         int `json:"broken_images"`
	MissingBaseServingRecipes            int `json:"missing_base_serving_recipes"`
	MissingCookingStepRecipes            int `json:"missing_cooking_step_recipes"`
	VagueIngredientLines                 int `json:"vague_ingredient_lines"`
	UnparseableRequiredIngredientAmounts int `json:"unparseable_required_ingredient_amounts"`
}

type RecipeQualityItem struct {
	Day             int                  `json:"day,omitempty"`
	MealSlot        string               `json:"meal_slot,omitempty"`
	RecipeID        string               `json:"recipe_id,omitempty"`
	RecipeTitle     string               `json:"recipe_title,omitempty"`
	Status          string               `json:"status"`
	Source          RecipeSourceEvidence `json:"source,omitempty"`
	Servings        int                  `json:"servings,omitempty"`
	IngredientCount int                  `json:"ingredient_count"`
	StepCount       int                  `json:"step_count"`
	HasSource       bool                 `json:"has_source"`
	HasImage        bool                 `json:"has_image"`
	ImageStatus     string               `json:"image_status,omitempty"`
	Issues          []RecipeQualityIssue `json:"issues,omitempty"`
	Warnings        []RecipeQualityIssue `json:"warnings,omitempty"`
}

type RecipeQualityIssue struct {
	Code           string `json:"code"`
	Severity       string `json:"severity"`
	RecipeID       string `json:"recipe_id,omitempty"`
	RecipeTitle    string `json:"recipe_title,omitempty"`
	Day            int    `json:"day,omitempty"`
	MealSlot       string `json:"meal_slot,omitempty"`
	IngredientName string `json:"ingredient_name,omitempty"`
	IngredientKey  string `json:"ingredient_key,omitempty"`
	Message        string `json:"message"`
	Remediation    string `json:"remediation,omitempty"`
}

type RecipeImageEvidence struct {
	RecipeID    string `json:"recipe_id,omitempty"`
	RecipeTitle string `json:"recipe_title,omitempty"`
	Day         int    `json:"day,omitempty"`
	MealSlot    string `json:"meal_slot,omitempty"`
	SourceURL   string `json:"source_url,omitempty"`
	CachedPath  string `json:"cached_path,omitempty"`
	Status      string `json:"status"`
	SHA256      string `json:"sha256,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	Message     string `json:"message,omitempty"`
}

func DefaultRecipeIntakePolicy() RecipeIntakePolicy {
	return RecipeIntakePolicy{
		RequireCookingSteps:      true,
		AllowStructuredURLImport: true,
		AllowUnstructuredScrape:  false,
		CacheImages:              true,
	}
}

func ApplyRecipeIntake(plan MealPlan, opts RecipeIntakeOptions) (MealPlan, *RecipeIntakePlan, *RecipeQualityReport, error) {
	opts = normalizeRecipeIntakeOptions(opts)
	intake := &RecipeIntakePlan{
		SchemaVersion: "1",
		Status:        RecipeIntakeNotRun,
		Policy:        opts.Policy,
	}
	if !opts.Enabled {
		return plan, intake, nil, nil
	}
	intake.Status = RecipeIntakeComplete
	sourceByRecipe := recipeSourcesForMealPlan(plan)
	imported, importedSources, imports, err := loadRecipeImports(context.Background(), opts)
	intake.Imports = imports
	for _, result := range imports {
		intake.RequestedSources = append(intake.RequestedSources, result.Source)
		if result.Status != "imported" {
			intake.Status = RecipeIntakeFailed
			if result.Error != "" {
				intake.Errors = append(intake.Errors, result.Error)
			}
		}
		intake.Warnings = append(intake.Warnings, result.Warnings...)
	}
	if err != nil {
		intake.Status = RecipeIntakeFailed
		intake.Errors = append(intake.Errors, err.Error())
		report := EvaluateRecipeQuality(plan, sourceByRecipe, nil, opts.Policy)
		return plan, intake, &report, err
	}
	if len(imported) > 0 && opts.InjectImportedMeals {
		plan, intake.AppliedMealSlots = injectImportedRecipes(plan, imported, importedSources)
		for key, source := range importedSources {
			sourceByRecipe[key] = source
		}
	}
	intake.ImportedRecipeCount = len(imported)
	intake.ActiveRecipeCount = activeRecipeCount(plan)
	imageEvidence := CacheRecipeImagesInMealPlan(&plan, opts)
	report := EvaluateRecipeQuality(plan, sourceByRecipe, imageEvidence, opts.Policy)
	intake.RecipeSetFingerprint = report.RecipeSetFingerprint
	if report.Status == RecipeQualityFail && intake.Status == RecipeIntakeComplete {
		intake.Status = RecipeIntakeCompleteWithWarning
	}
	if len(intake.Warnings) > 0 && intake.Status == RecipeIntakeComplete {
		intake.Status = RecipeIntakeCompleteWithWarning
	}
	return plan, intake, &report, nil
}

func normalizeRecipeIntakeOptions(opts RecipeIntakeOptions) RecipeIntakeOptions {
	if opts.Policy == (RecipeIntakePolicy{}) {
		opts.Policy = DefaultRecipeIntakePolicy()
	}
	if len(opts.RecipeFiles) > 0 || len(opts.RecipeDirs) > 0 || len(opts.RecipeURLs) > 0 {
		opts.Enabled = true
		opts.InjectImportedMeals = true
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	return opts
}

func loadRecipeImports(ctx context.Context, opts RecipeIntakeOptions) ([]Recipe, map[string]RecipeSourceEvidence, []RecipeImportResult, error) {
	var recipes []Recipe
	sourceByRecipe := map[string]RecipeSourceEvidence{}
	var results []RecipeImportResult
	var errs []string
	for _, rawPath := range opts.RecipeFiles {
		loaded, source, warnings, err := loadRecipeFileForIntake(rawPath, "file")
		result := recipeImportResult(source, loaded, warnings, err)
		results = append(results, result)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		recipes = append(recipes, loaded...)
		stampSources(sourceByRecipe, loaded, source)
	}
	for _, rawDir := range opts.RecipeDirs {
		dir := strings.TrimSpace(rawDir)
		source := RecipeSourceEvidence{SourceID: "dir:" + dir, SourceType: "directory", Path: dir, Confidence: "high", Message: "Local recipe directory requested."}
		entries, err := os.ReadDir(dir)
		if err != nil {
			results = append(results, RecipeImportResult{Source: source, Status: "failed", Error: err.Error()})
			errs = append(errs, err.Error())
			continue
		}
		var dirRecipes []Recipe
		var dirWarnings []string
		for _, entry := range entries {
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			loaded, fileSource, warnings, err := loadRecipeFileForIntake(path, "directory")
			if err != nil {
				dirWarnings = append(dirWarnings, fmt.Sprintf("%s: %v", entry.Name(), err))
				continue
			}
			dirRecipes = append(dirRecipes, loaded...)
			stampSources(sourceByRecipe, loaded, fileSource)
			dirWarnings = append(dirWarnings, warnings...)
		}
		result := recipeImportResult(source, dirRecipes, dirWarnings, nil)
		if len(dirRecipes) == 0 {
			result.Status = "failed"
			result.Error = "directory did not contain any importable recipe JSON files"
			errs = append(errs, result.Error)
		}
		results = append(results, result)
		recipes = append(recipes, dirRecipes...)
	}
	for _, rawURL := range opts.RecipeURLs {
		loaded, source, warnings, err := loadStructuredRecipeURL(ctx, rawURL, opts)
		result := recipeImportResult(source, loaded, warnings, err)
		results = append(results, result)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		recipes = append(recipes, loaded...)
		stampSources(sourceByRecipe, loaded, source)
	}
	if len(errs) > 0 {
		return recipes, sourceByRecipe, results, errors.New(strings.Join(errs, "; "))
	}
	return recipes, sourceByRecipe, results, nil
}

func recipeImportResult(source RecipeSourceEvidence, recipes []Recipe, warnings []string, err error) RecipeImportResult {
	result := RecipeImportResult{Source: source, Status: "imported", Count: len(recipes), Warnings: warnings}
	for _, recipe := range recipes {
		result.RecipeIDs = append(result.RecipeIDs, recipe.ID)
	}
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
	}
	return result
}

func loadRecipeFileForIntake(rawPath, sourceType string) ([]Recipe, RecipeSourceEvidence, []string, error) {
	path := strings.TrimSpace(rawPath)
	absPath, _ := filepath.Abs(path)
	source := RecipeSourceEvidence{
		SourceID:    sourceType + ":" + absPath,
		SourceType:  sourceType,
		Path:        absPath,
		Confidence:  "high",
		Message:     "Local structured recipe JSON file.",
		RetrievedAt: nowStamp(),
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, source, nil, err
	}
	recipes, _, err := decodeRecipeFile(data)
	if err != nil {
		return nil, source, nil, err
	}
	var warnings []string
	baseDir := filepath.Dir(absPath)
	for i := range recipes {
		recipes[i] = normalizeRecipe(recipes[i])
		if strings.TrimSpace(recipes[i].ID) == "" {
			recipes[i].ID = recipeSlug(strutilRecipeTitle(recipes[i], filepath.Base(path), i+1))
			warnings = append(warnings, fmt.Sprintf("recipe %d had no id; generated %q", i+1, recipes[i].ID))
		}
		if strings.TrimSpace(recipes[i].Title) == "" {
			recipes[i].Title = strutilRecipeTitle(recipes[i], filepath.Base(path), i+1)
			warnings = append(warnings, fmt.Sprintf("recipe %q had no title; generated %q", recipes[i].ID, recipes[i].Title))
		}
		recipes[i].ImageURL = resolveLocalRecipeImage(recipes[i].ImageURL, baseDir)
		for si := range recipes[i].Steps {
			recipes[i].Steps[si].ImageURL = resolveLocalRecipeImage(recipes[i].Steps[si].ImageURL, baseDir)
		}
	}
	return recipes, source, warnings, nil
}

func strutilRecipeTitle(recipe Recipe, source string, index int) string {
	if strings.TrimSpace(recipe.Title) != "" {
		return strings.TrimSpace(recipe.Title)
	}
	if strings.TrimSpace(recipe.ID) != "" {
		return strings.TrimSpace(recipe.ID)
	}
	if source != "" {
		return fmt.Sprintf("Recipe from %s %d", source, index)
	}
	return fmt.Sprintf("Imported Recipe %d", index)
}

func resolveLocalRecipeImage(ref, baseDir string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "data:image/") || filepath.IsAbs(ref) {
		return ref
	}
	if _, err := os.Stat(ref); err == nil {
		if abs, absErr := filepath.Abs(ref); absErr == nil {
			return abs
		}
		return ref
	}
	return filepath.Join(baseDir, ref)
}

func stampSources(sourceByRecipe map[string]RecipeSourceEvidence, recipes []Recipe, source RecipeSourceEvidence) {
	for _, recipe := range recipes {
		evidence := source
		evidence.Title = recipe.Title
		evidence.ImageURL = recipe.ImageURL
		evidence.BaseServings = recipe.Servings
		sourceByRecipe[recipe.ID] = evidence
	}
}

func loadStructuredRecipeURL(ctx context.Context, rawURL string, opts RecipeIntakeOptions) ([]Recipe, RecipeSourceEvidence, []string, error) {
	source := RecipeSourceEvidence{
		SourceID:       "url:" + strings.TrimSpace(rawURL),
		SourceType:     "url",
		URL:            strings.TrimSpace(rawURL),
		StructuredType: "schema.org/Recipe",
		Confidence:     "medium",
		Message:        "Structured recipe URL import requested.",
		RetrievedAt:    nowStamp(),
	}
	if !opts.Policy.AllowStructuredURLImport {
		return nil, source, nil, fmt.Errorf("structured recipe URL import is disabled")
	}
	if opts.Policy.AllowUnstructuredScrape {
		source.Message = "Structured URL import requested; unstructured scraping remains unused by this importer."
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, source, nil, err
	}
	req.Header.Set("User-Agent", "carrito-food-agent/1.0")
	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return nil, source, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, source, nil, fmt.Errorf("recipe URL fetch failed with %s", resp.Status)
	}
	body, err := readLimitedReader(resp.Body, 3*1024*1024)
	if err != nil {
		return nil, source, nil, err
	}
	recipes, warnings, err := recipesFromJSONLD(body, rawURL)
	if err != nil {
		return nil, source, warnings, err
	}
	for i := range recipes {
		recipes[i] = normalizeRecipe(recipes[i])
		if strings.TrimSpace(recipes[i].SourceURL) == "" {
			recipes[i].SourceURL = strings.TrimSpace(rawURL)
		}
		if strings.TrimSpace(recipes[i].ID) == "" {
			recipes[i].ID = recipeSlug(strutilRecipeTitle(recipes[i], "url", i+1))
		}
		if strings.TrimSpace(recipes[i].Title) == "" {
			recipes[i].Title = strutilRecipeTitle(recipes[i], "url", i+1)
		}
	}
	if len(recipes) > 0 {
		source.Title = recipes[0].Title
		source.ImageURL = recipes[0].ImageURL
		source.BaseServings = recipes[0].Servings
	}
	return recipes, source, warnings, nil
}

func RecipesFromStructuredURL(ctx context.Context, rawURL string, opts RecipeIntakeOptions) ([]Recipe, RecipeSourceEvidence, []string, error) {
	opts = normalizeRecipeIntakeOptions(opts)
	return loadStructuredRecipeURL(ctx, rawURL, opts)
}

func recipesFromJSONLD(body []byte, baseURL string) ([]Recipe, []string, error) {
	blocks := extractJSONLDBlocks(string(body))
	if len(blocks) == 0 {
		return nil, nil, fmt.Errorf("no schema.org JSON-LD recipe data found; unstructured recipe scraping is intentionally unsupported")
	}
	var recipes []Recipe
	var warnings []string
	for _, block := range blocks {
		var decoded any
		if err := json.Unmarshal([]byte(block), &decoded); err != nil {
			warnings = append(warnings, "skipped invalid JSON-LD block: "+err.Error())
			continue
		}
		for _, node := range findRecipeNodes(decoded) {
			recipe, warn := recipeFromSchemaOrgNode(node, baseURL)
			warnings = append(warnings, warn...)
			if strings.TrimSpace(recipe.Title) == "" && len(recipe.Ingredients) == 0 && len(recipe.Steps) == 0 {
				continue
			}
			recipes = append(recipes, recipe)
		}
	}
	if len(recipes) == 0 {
		return nil, warnings, fmt.Errorf("no schema.org Recipe object found; unstructured recipe scraping is intentionally unsupported")
	}
	return recipes, warnings, nil
}

var jsonLDScriptRE = regexp.MustCompile(`(?is)<script[^>]*type=["']application/ld\+json["'][^>]*>(.*?)</script>`)

func extractJSONLDBlocks(page string) []string {
	var blocks []string
	for _, match := range jsonLDScriptRE.FindAllStringSubmatch(page, -1) {
		if len(match) == 2 {
			block := strings.TrimSpace(html.UnescapeString(match[1]))
			if block != "" {
				blocks = append(blocks, block)
			}
		}
	}
	return blocks
}

func findRecipeNodes(value any) []map[string]any {
	var out []map[string]any
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			out = append(out, findRecipeNodes(item)...)
		}
	case map[string]any:
		if schemaTypeMatches(v["@type"], "Recipe") {
			out = append(out, v)
		}
		if graph, ok := v["@graph"]; ok {
			out = append(out, findRecipeNodes(graph)...)
		}
		if main, ok := v["mainEntity"]; ok {
			out = append(out, findRecipeNodes(main)...)
		}
	}
	return out
}

func schemaTypeMatches(value any, want string) bool {
	switch v := value.(type) {
	case string:
		return strings.EqualFold(strings.TrimSpace(v), want) || strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(v), "schema:"), want)
	case []any:
		for _, item := range v {
			if schemaTypeMatches(item, want) {
				return true
			}
		}
	}
	return false
}

func recipeFromSchemaOrgNode(node map[string]any, baseURL string) (Recipe, []string) {
	var warnings []string
	title := stringFromAny(node["name"])
	servings := servingsFromSchemaYield(node["recipeYield"])
	if servings <= 0 {
		warnings = append(warnings, fmt.Sprintf("recipe %q has no parseable recipeYield/base servings", title))
	}
	imageURL := resolveURLRef(firstImageFromAny(node["image"]), baseURL)
	ingredients := ingredientsFromSchemaAny(node["recipeIngredient"])
	if len(ingredients) == 0 {
		warnings = append(warnings, fmt.Sprintf("recipe %q has no recipeIngredient lines", title))
	}
	steps := stepsFromSchemaAny(node["recipeInstructions"], baseURL)
	if len(steps) == 0 {
		warnings = append(warnings, fmt.Sprintf("recipe %q has no recipeInstructions steps", title))
	}
	recipe := Recipe{
		ID:          recipeSlug(title),
		Title:       title,
		Servings:    servings,
		Tags:        []string{"url-import", "structured"},
		SourceURL:   firstNonEmptyString(resolveURLRef(stringFromAny(node["url"]), baseURL), baseURL),
		ImageURL:    imageURL,
		Ingredients: ingredients,
		Steps:       steps,
	}
	if author := stringFromAny(node["author"]); author != "" {
		recipe.AllergenNotes = append(recipe.AllergenNotes, "Source author: "+author)
	}
	return recipe, warnings
}

func stringFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case map[string]any:
		return stringFromAny(v["name"])
	case []any:
		for _, item := range v {
			if s := stringFromAny(item); s != "" {
				return s
			}
		}
	}
	return ""
}

func servingsFromSchemaYield(value any) int {
	switch v := value.(type) {
	case float64:
		if v > 0 {
			return int(v)
		}
	case string:
		return firstPositiveInt(v)
	case []any:
		for _, item := range v {
			if servings := servingsFromSchemaYield(item); servings > 0 {
				return servings
			}
		}
	}
	return 0
}

var positiveIntRE = regexp.MustCompile(`\d+`)

func firstPositiveInt(value string) int {
	match := positiveIntRE.FindString(value)
	if match == "" {
		return 0
	}
	n, _ := strconv.Atoi(match)
	return n
}

func firstImageFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		for _, item := range v {
			if image := firstImageFromAny(item); image != "" {
				return image
			}
		}
	case map[string]any:
		return firstNonEmptyString(stringFromAny(v["url"]), stringFromAny(v["contentUrl"]))
	}
	return ""
}

func ingredientsFromSchemaAny(value any) []Ingredient {
	var ingredients []Ingredient
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			if ingredient := ingredientFromStructuredText(stringFromAny(item)); strings.TrimSpace(ingredient.Name) != "" {
				ingredients = append(ingredients, ingredient)
			}
		}
	case string:
		if ingredient := ingredientFromStructuredText(v); strings.TrimSpace(ingredient.Name) != "" {
			ingredients = append(ingredients, ingredient)
		}
	}
	return ingredients
}

func ingredientFromStructuredText(raw string) Ingredient {
	part := strings.TrimSpace(strings.Trim(raw, "-* "))
	if part == "" {
		return Ingredient{}
	}
	if m := textIngredientRE.FindStringSubmatch(part); len(m) == 4 {
		qty, _ := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64)
		qty, unit := normalizeRecipeIngredientQuantity(qty, m[2])
		if unit == "" {
			unit = "unit"
		}
		name := cleanIngredientName(m[3])
		return Ingredient{Name: name, Quantity: roundQty(qty), Unit: unit, Category: inferIngredientCategory(name), SearchTerm: spanishSearchTerm(name)}
	}
	name := cleanIngredientName(part)
	return Ingredient{Name: name, Category: inferIngredientCategory(name), SearchTerm: spanishSearchTerm(name)}
}

func stepsFromSchemaAny(value any, baseURL string) []RecipeStep {
	var steps []RecipeStep
	var addStep func(text, image string)
	addStep = func(text, image string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		steps = append(steps, RecipeStep{Number: len(steps) + 1, Text: text, ImageURL: resolveURLRef(image, baseURL)})
	}
	switch v := value.(type) {
	case string:
		addStep(v, "")
	case []any:
		for _, item := range v {
			switch step := item.(type) {
			case string:
				addStep(step, "")
			case map[string]any:
				if nested, ok := step["itemListElement"]; ok {
					for _, nestedStep := range stepsFromSchemaAny(nested, baseURL) {
						addStep(nestedStep.Text, nestedStep.ImageURL)
					}
					continue
				}
				addStep(firstNonEmptyString(stringFromAny(step["text"]), stringFromAny(step["name"])), firstImageFromAny(step["image"]))
			}
		}
	case map[string]any:
		if nested, ok := v["itemListElement"]; ok {
			return stepsFromSchemaAny(nested, baseURL)
		}
		addStep(firstNonEmptyString(stringFromAny(v["text"]), stringFromAny(v["name"])), firstImageFromAny(v["image"]))
	}
	return steps
}

func resolveURLRef(ref, baseURL string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	u, err := url.Parse(ref)
	if err != nil || u.IsAbs() {
		return ref
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return ref
	}
	return base.ResolveReference(u).String()
}

func injectImportedRecipes(plan MealPlan, imported []Recipe, sourceByRecipe map[string]RecipeSourceEvidence) (MealPlan, []RecipeIntakeAppliedSlot) {
	if len(imported) == 0 {
		return plan, nil
	}
	var applied []RecipeIntakeAppliedSlot
	index := 0
	targetServings := plan.People
	if targetServings <= 0 {
		targetServings = 2
	}
	for di := range plan.Days {
		for mi := range plan.Days[di].Meals {
			recipe := imported[index%len(imported)]
			original := plan.Days[di].Meals[mi].Recipe
			scaled := withNutrition(scaleRecipe(recipe, targetServings))
			plan.Days[di].Meals[mi].Recipe = scaled
			plan.Days[di].Meals[mi].PlanningReason = "recipe imported through recipe intake quality gate"
			source := sourceByRecipe[recipe.ID]
			applied = append(applied, RecipeIntakeAppliedSlot{
				Day:           plan.Days[di].Day,
				MealSlot:      plan.Days[di].Meals[mi].Type,
				RecipeID:      scaled.ID,
				RecipeTitle:   scaled.Title,
				SourceID:      source.SourceID,
				SourceType:    source.SourceType,
				OriginalID:    original.ID,
				OriginalTitle: original.Title,
			})
			index++
		}
	}
	return plan, applied
}

func CacheRecipeImagesInMealPlan(plan *MealPlan, opts RecipeIntakeOptions) map[string]RecipeImageEvidence {
	out := map[string]RecipeImageEvidence{}
	if plan == nil {
		return out
	}
	cacheDir := strings.TrimSpace(opts.ImageCacheDir)
	if cacheDir == "" && opts.Policy.CacheImages {
		if dir, err := Dir(); err == nil {
			cacheDir = filepath.Join(dir, "recipe-image-cache")
		}
	}
	for di := range plan.Days {
		for mi := range plan.Days[di].Meals {
			meal := &plan.Days[di].Meals[mi]
			key := recipeSlotKey(plan.Days[di].Day, meal.Type, meal.Recipe.ID)
			evidence := recipeImageEvidenceFor(meal.Recipe, plan.Days[di].Day, meal.Type, opts, cacheDir)
			if evidence.CachedPath != "" && evidence.Status == RecipeImageCached {
				meal.Recipe.ImageURL = evidence.CachedPath
			}
			out[key] = evidence
		}
	}
	return out
}

func recipeImageEvidenceFor(recipe Recipe, day int, mealSlot string, opts RecipeIntakeOptions, cacheDir string) RecipeImageEvidence {
	ref := strings.TrimSpace(recipe.ImageURL)
	ev := RecipeImageEvidence{RecipeID: recipe.ID, RecipeTitle: recipe.Title, Day: day, MealSlot: mealSlot, SourceURL: ref, Status: RecipeImageMissing}
	if ref == "" {
		ev.Message = "recipe has no image URL"
		return ev
	}
	if !opts.Policy.CacheImages {
		ev.Status = RecipeImageNotCached
		ev.Message = "recipe image cache disabled"
		return ev
	}
	data, sourceName, err := fetchRecipeImageData(ref, opts.HTTPClient)
	if err != nil {
		ev.Status = RecipeImageFailed
		ev.Message = err.Error()
		return ev
	}
	sum := sha256.Sum256(data)
	ev.SHA256 = hex.EncodeToString(sum[:])
	ev.ContentType = http.DetectContentType(data)
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		ev.Width = cfg.Width
		ev.Height = cfg.Height
	}
	if cacheDir == "" {
		ev.Status = RecipeImageLocal
		ev.Message = "image verified but no cache directory is available"
		return ev
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		ev.Status = RecipeImageFailed
		ev.Message = err.Error()
		return ev
	}
	ext := imageCacheExt(sourceName, ev.ContentType)
	name := fmt.Sprintf("%s-%s%s", recipeSlug(firstNonEmptyString(recipe.ID, recipe.Title, "recipe")), ev.SHA256[:12], ext)
	path := filepath.Join(cacheDir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		ev.Status = RecipeImageFailed
		ev.Message = err.Error()
		return ev
	}
	ev.CachedPath = path
	ev.Status = RecipeImageCached
	ev.Message = "recipe image cached with sha256 provenance"
	return ev
}

func fetchRecipeImageData(ref string, client *http.Client) ([]byte, string, error) {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		if client == nil {
			client = &http.Client{Timeout: 8 * time.Second}
		}
		req, err := http.NewRequest(http.MethodGet, ref, nil)
		if err != nil {
			return nil, ref, err
		}
		req.Header.Set("User-Agent", "carrito-food-agent/1.0")
		resp, err := client.Do(req)
		if err != nil {
			return nil, ref, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, ref, fmt.Errorf("image fetch failed with %s", resp.Status)
		}
		data, err := readLimitedReader(resp.Body, 5*1024*1024)
		if err != nil {
			return nil, ref, err
		}
		if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(strings.ToLower(ct), "image/") {
			return nil, ref, fmt.Errorf("image response content type %q is not an image", ct)
		}
		return data, ref, nil
	}
	if strings.HasPrefix(ref, "data:image/") {
		comma := strings.IndexByte(ref, ',')
		if comma < 0 {
			return nil, ref, fmt.Errorf("invalid data image")
		}
		meta := ref[:comma]
		payload := ref[comma+1:]
		if strings.Contains(meta, ";base64") {
			data, err := base64.StdEncoding.DecodeString(payload)
			return data, "data-image", err
		}
		decoded, err := url.QueryUnescape(payload)
		if err != nil {
			return nil, ref, err
		}
		return []byte(decoded), "data-image", nil
	}
	data, err := os.ReadFile(ref)
	return data, ref, err
}

func imageCacheExt(sourceName, contentType string) string {
	if ext := strings.ToLower(filepath.Ext(sourceName)); ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" || ext == ".webp" {
		return ext
	}
	if exts, err := mime.ExtensionsByType(contentType); err == nil && len(exts) > 0 {
		return exts[0]
	}
	return ".img"
}

func EvaluateRecipeQuality(plan MealPlan, sourceByRecipe map[string]RecipeSourceEvidence, imageEvidence map[string]RecipeImageEvidence, policy RecipeIntakePolicy) RecipeQualityReport {
	policy = normalizeRecipeIntakeOptions(RecipeIntakeOptions{Policy: policy}).Policy
	report := RecipeQualityReport{SchemaVersion: "1", Status: RecipeQualityPass, Policy: policy}
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			recipe := meal.Recipe
			slotKey := recipeSlotKey(day.Day, meal.Type, recipe.ID)
			source := sourceByRecipe[recipe.ID]
			if source.SourceID == "" {
				source = RecipeSourceEvidence{SourceID: "catalog:" + recipe.ID, SourceType: "catalog", Title: recipe.Title, Confidence: "medium", Message: "Active meal-plan recipe from local recipe catalog.", BaseServings: recipe.Servings}
			}
			image := imageEvidence[slotKey]
			if image.Status == "" {
				image = RecipeImageEvidence{RecipeID: recipe.ID, RecipeTitle: recipe.Title, Day: day.Day, MealSlot: meal.Type, SourceURL: recipe.ImageURL, Status: RecipeImageMissing, Message: "recipe image was not evaluated"}
				if strings.TrimSpace(recipe.ImageURL) != "" {
					image.Status = RecipeImageRemote
					image.Message = "recipe image URL present without cache evidence"
				}
			}
			item := RecipeQualityItem{
				Day:             day.Day,
				MealSlot:        meal.Type,
				RecipeID:        recipe.ID,
				RecipeTitle:     recipe.Title,
				Status:          RecipeQualityPass,
				Source:          source,
				Servings:        recipe.Servings,
				IngredientCount: len(recipe.Ingredients),
				StepCount:       len(recipe.Steps),
				HasSource:       recipeSourcePresent(source),
				HasImage:        image.Status != RecipeImageMissing && image.Status != RecipeImageFailed,
				ImageStatus:     image.Status,
			}
			issues := recipeQualityIssues(recipe, day.Day, meal.Type, source, image, policy)
			for _, issue := range issues {
				switch issue.Code {
				case "base_servings_missing":
					report.Summary.MissingBaseServingRecipes++
				case "cooking_steps_missing":
					report.Summary.MissingCookingStepRecipes++
				case "vague_required_ingredient":
					report.Summary.VagueIngredientLines++
				case "required_amount_unparseable":
					report.Summary.UnparseableRequiredIngredientAmounts++
				}
				if issue.Severity == "blocking" {
					item.Issues = append(item.Issues, issue)
					report.BlockingIssues = append(report.BlockingIssues, issue)
				} else {
					item.Warnings = append(item.Warnings, issue)
					report.Warnings = append(report.Warnings, issue)
				}
			}
			if len(item.Issues) > 0 {
				item.Status = RecipeQualityFail
				report.Summary.FailingRecipes++
			} else if len(item.Warnings) > 0 {
				item.Status = RecipeQualityPassWithWarning
				report.Summary.RecipesWithWarnings++
			} else {
				report.Summary.PassingRecipes++
			}
			report.Summary.RecipeCount++
			if item.HasSource {
				report.Summary.RecipesWithSource++
			}
			if item.HasImage {
				report.Summary.RecipesWithImages++
			}
			switch image.Status {
			case RecipeImageCached:
				report.Summary.CachedImages++
			case RecipeImageMissing:
				report.Summary.MissingImages++
			case RecipeImageFailed:
				report.Summary.BrokenImages++
			}
			report.Items = append(report.Items, item)
			report.ImageEvidence = append(report.ImageEvidence, image)
		}
	}
	if len(report.BlockingIssues) > 0 {
		report.Status = RecipeQualityFail
	} else if len(report.Warnings) > 0 {
		report.Status = RecipeQualityPassWithWarning
	}
	report.RecipeSetFingerprint = RecipeSetFingerprint(plan)
	report.RecipeImageFingerprint = RecipeImageFingerprint(report)
	report.RecipeQualityFingerprint = RecipeQualityFingerprint(report)
	return report
}

func recipeQualityIssues(recipe Recipe, day int, mealSlot string, source RecipeSourceEvidence, image RecipeImageEvidence, policy RecipeIntakePolicy) []RecipeQualityIssue {
	var issues []RecipeQualityIssue
	add := func(code, ingredientName, message, remediation string) {
		severity := "warning"
		if recipeQualityIssueBlocks(code, policy) {
			severity = "blocking"
		}
		issues = append(issues, RecipeQualityIssue{
			Code:           code,
			Severity:       severity,
			RecipeID:       recipe.ID,
			RecipeTitle:    recipe.Title,
			Day:            day,
			MealSlot:       mealSlot,
			IngredientName: ingredientName,
			IngredientKey:  normalizeKey(ingredientName),
			Message:        message,
			Remediation:    remediation,
		})
	}
	if strings.TrimSpace(recipe.ID) == "" || strings.TrimSpace(recipe.Title) == "" {
		add("recipe_identity_missing", "", "Recipe is missing an id or title.", "Add stable recipe id and title before presenting it as cookable.")
	}
	if !recipeSourcePresent(source) {
		add("recipe_source_missing", "", "Recipe source/provenance is missing.", "Provide a local file, catalog, or structured URL source.")
	}
	baseServings := recipe.Servings
	if source.BaseServings != 0 {
		baseServings = source.BaseServings
	}
	if baseServings <= 0 {
		add("base_servings_missing", "", "Recipe base servings are missing or zero.", "Set recipe.servings so ingredients can be scaled safely.")
	}
	if len(recipe.Ingredients) == 0 {
		add("ingredients_missing", "", "Recipe has no ingredients.", "Add ingredient lines with names, quantities, and units.")
	}
	if policy.RequireCookingSteps && len(recipe.Steps) == 0 {
		add("cooking_steps_missing", "", "Recipe has no cooking instructions.", "Add ordered cooking steps before presenting this meal as cookable.")
	}
	for _, ingredient := range recipe.Ingredients {
		key := normalizeKey(ingredient.Name)
		if strings.TrimSpace(ingredient.Name) == "" {
			add("ingredient_name_missing", "", "Recipe has an ingredient line without a name.", "Add a concrete ingredient name.")
			continue
		}
		if isVagueIngredientName(key) && !ingredient.Optional {
			add("vague_required_ingredient", ingredient.Name, "Recipe has a vague required ingredient: "+ingredient.Name+".", "Replace vague ingredient names with a concrete ingredient that can be shopped.")
		}
		if ingredient.Optional || isToTasteOrPantryStaple(ingredient) {
			continue
		}
		if _, ok := normalizedIngredientQuantity(ingredient.Quantity, ingredient.Unit, 0.95, "recipe_quality"); !ok {
			add("required_amount_unparseable", ingredient.Name, "Required ingredient amount is missing or unparseable: "+ingredient.Name+".", "Provide a positive quantity and supported unit for required ingredients.")
		}
	}
	switch image.Status {
	case RecipeImageMissing:
		add("recipe_image_missing", "", "Recipe has no image provenance.", "Add image_url or run without --require-recipe-images.")
	case RecipeImageFailed:
		add("recipe_image_failed", "", "Recipe image could not be verified or cached.", "Fix the image path/URL or disable required recipe images.")
	case RecipeImageNotCached, RecipeImageRemote:
		add("recipe_image_unverified", "", "Recipe image is present but does not have local/cache evidence.", "Enable recipe image caching or provide a verified local image.")
	}
	return issues
}

func recipeQualityIssueBlocks(code string, policy RecipeIntakePolicy) bool {
	switch code {
	case "recipe_source_missing":
		return policy.RequireRecipeSource || policy.StrictRecipeQuality
	case "recipe_image_missing", "recipe_image_failed", "recipe_image_unverified":
		return policy.RequireRecipeImages
	case "base_servings_missing":
		return policy.RequireBaseServings || policy.StrictRecipeQuality
	case "required_amount_unparseable":
		return policy.RequireParseableRequiredAmounts || policy.StrictRecipeQuality
	case "cooking_steps_missing", "recipe_identity_missing", "ingredients_missing", "ingredient_name_missing", "vague_required_ingredient":
		return policy.StrictRecipeQuality
	default:
		return policy.StrictRecipeQuality
	}
}

func recipeSourcePresent(source RecipeSourceEvidence) bool {
	return strings.TrimSpace(source.SourceID) != "" || strings.TrimSpace(source.Path) != "" || strings.TrimSpace(source.URL) != "" || strings.TrimSpace(source.SourceType) == "catalog"
}

func isVagueIngredientName(key string) bool {
	fields := strings.Fields(key)
	if len(fields) > 2 {
		return false
	}
	vague := map[string]bool{
		"meat": true, "fish": true, "vegetable": true, "vegetables": true, "sauce": true, "spice": true, "spices": true, "herbs": true,
		"carne": true, "pescado": true, "verdura": true, "verduras": true, "vegetal": true, "vegetales": true, "salsa": true, "especia": true, "especias": true,
	}
	return vague[key]
}

func isToTasteOrPantryStaple(ingredient Ingredient) bool {
	key := normalizeKey(strings.Join([]string{ingredient.Name, ingredient.Unit, ingredient.Category}, " "))
	if strings.Contains(key, "to taste") || strings.Contains(key, "al gusto") || strings.Contains(key, "q b") {
		return true
	}
	staples := []string{"salt", "sal", "pepper", "pimienta", "water", "agua", "oil", "aceite", "olive oil", "aceite oliva", "vinegar", "vinagre"}
	for _, staple := range staples {
		if normalizeKey(ingredient.Name) == normalizeKey(staple) {
			return true
		}
	}
	return false
}

func recipeSourcesForMealPlan(plan MealPlan) map[string]RecipeSourceEvidence {
	out := map[string]RecipeSourceEvidence{}
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			recipe := meal.Recipe
			if strings.TrimSpace(recipe.ID) == "" {
				continue
			}
			if _, exists := out[recipe.ID]; exists {
				continue
			}
			out[recipe.ID] = RecipeSourceEvidence{
				SourceID:     "catalog:" + recipe.ID,
				SourceType:   "catalog",
				Title:        recipe.Title,
				Confidence:   "medium",
				Message:      "Active meal-plan recipe from local recipe catalog.",
				ImageURL:     recipe.ImageURL,
				BaseServings: recipe.Servings,
			}
		}
	}
	return out
}

func RecipeSetFingerprint(plan MealPlan) string {
	type canonicalStep struct {
		Number int    `json:"number"`
		Text   string `json:"text"`
		Image  string `json:"image,omitempty"`
	}
	type canonicalIngredient struct {
		Name     string  `json:"name"`
		Quantity float64 `json:"quantity,omitempty"`
		Unit     string  `json:"unit,omitempty"`
		Optional bool    `json:"optional,omitempty"`
	}
	type canonicalRecipe struct {
		Day         int                   `json:"day"`
		MealSlot    string                `json:"meal_slot"`
		ID          string                `json:"id"`
		Title       string                `json:"title"`
		Servings    int                   `json:"servings"`
		Image       string                `json:"image,omitempty"`
		Ingredients []canonicalIngredient `json:"ingredients,omitempty"`
		Steps       []canonicalStep       `json:"steps,omitempty"`
	}
	var rows []canonicalRecipe
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			recipe := meal.Recipe
			row := canonicalRecipe{Day: day.Day, MealSlot: normalizeKey(meal.Type), ID: recipe.ID, Title: recipe.Title, Servings: recipe.Servings, Image: recipe.ImageURL}
			for _, ingredient := range recipe.Ingredients {
				row.Ingredients = append(row.Ingredients, canonicalIngredient{Name: normalizeKey(ingredient.Name), Quantity: roundQty(ingredient.Quantity), Unit: normalizeUnit(ingredient.Unit), Optional: ingredient.Optional})
			}
			for _, step := range recipe.Steps {
				row.Steps = append(row.Steps, canonicalStep{Number: step.Number, Text: normalizeKey(step.Text), Image: step.ImageURL})
			}
			rows = append(rows, row)
		}
	}
	data, err := json.Marshal(rows)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func RecipeQualityFingerprint(report RecipeQualityReport) string {
	clone := report
	clone.RecipeQualityFingerprint = ""
	data, err := json.Marshal(clone)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func RecipeImageFingerprint(report RecipeQualityReport) string {
	evidence := append([]RecipeImageEvidence(nil), report.ImageEvidence...)
	sort.SliceStable(evidence, func(i, j int) bool {
		a := fmt.Sprintf("%d|%s|%s", evidence[i].Day, normalizeKey(evidence[i].MealSlot), evidence[i].RecipeID)
		b := fmt.Sprintf("%d|%s|%s", evidence[j].Day, normalizeKey(evidence[j].MealSlot), evidence[j].RecipeID)
		return a < b
	})
	data, err := json.Marshal(evidence)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func RecipeSourcesFromQualityReport(report *RecipeQualityReport) map[string]RecipeSourceEvidence {
	out := map[string]RecipeSourceEvidence{}
	if report == nil {
		return out
	}
	for _, item := range report.Items {
		if strings.TrimSpace(item.RecipeID) == "" {
			continue
		}
		out[item.RecipeID] = item.Source
	}
	return out
}

func recipeSlotKey(day int, mealSlot, recipeID string) string {
	return fmt.Sprintf("%d|%s|%s", day, normalizeKey(mealSlot), strings.TrimSpace(recipeID))
}

func activeRecipeCount(plan MealPlan) int {
	count := 0
	for _, day := range plan.Days {
		count += len(day.Meals)
	}
	return count
}

func readLimitedReader(r io.Reader, limit int64) ([]byte, error) {
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, io.LimitReader(r, limit+1)); err != nil {
		return nil, err
	}
	if int64(buf.Len()) > limit {
		return nil, fmt.Errorf("response exceeded %d bytes", limit)
	}
	return buf.Bytes(), nil
}
