package mealplan

import (
	"fmt"
	"math"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/money"
)

type SelectedItem struct {
	Shopping   ShoppingItem    `json:"shopping"`
	Product    alcampo.Product `json:"product"`
	LineTotal  money.Money     `json:"line_total"`
	SafetyNote string          `json:"safety_note,omitempty"`
}

type Build struct {
	Plan           Plan           `json:"plan"`
	Items          []SelectedItem `json:"items"`
	EstimatedTotal money.Money    `json:"estimated_total"`
	Warnings       []string       `json:"warnings,omitempty"`
}

type allergyMatchSpec struct {
	terms                   []string
	supported               bool
	needsIngredientEvidence bool
}

type allergyEvidenceGap struct {
	allergy string
	reason  string
}

var basketProductIDRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,255}$`)

func NewBuild(plan Plan, products []alcampo.Product) (Build, error) {
	if err := FormatIssues(ValidateSelected(plan)); err != nil {
		return Build{}, err
	}
	if len(products) != len(plan.Shopping) {
		return Build{}, fmt.Errorf("resolved %d products for %d shopping items", len(products), len(plan.Shopping))
	}

	result := Build{Plan: plan}
	for _, allergy := range plan.Household.Allergies {
		if analyzeAllergy(allergy).supported {
			continue
		}
		if hasSevereAllergy([]string{allergy}) {
			return Build{}, fmt.Errorf("household allergy %q could not be mechanically interpreted as one or more named foods; clarify it before building", allergy)
		}
		result.Warnings = append(result.Warnings, fmt.Sprintf("Household allergy %q could not be mechanically interpreted as one or more named foods; clarify it and review every recipe ingredient", allergy))
	}
	for _, rule := range plan.Household.DietaryRules {
		if !hasRecognizedDietRule([]string{rule}) {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Dietary rule %q is not mechanically verified; review the recipes and selected products", rule))
		}
	}
	var total int64
	selectedProductIndexes := map[string]int{}
	for index, item := range plan.Shopping {
		product := products[index]
		if strings.TrimSpace(product.ID) == "" {
			return Build{}, fmt.Errorf("shopping[%d] %q: Alcampo did not return the internal product id needed for the cart", index, item.Name)
		}
		if !basketProductIDRE.MatchString(product.ID) {
			return Build{}, fmt.Errorf("shopping[%d] %q: Alcampo returned an invalid internal product id; refusing to write it to the basket", index, item.Name)
		}
		if product.Price.Amount == "" {
			return Build{}, fmt.Errorf("shopping[%d] %q: selected Alcampo product has no current price", index, item.Name)
		}
		if product.Price.Cents < 0 || (product.Price.Currency != "" && !strings.EqualFold(product.Price.Currency, "EUR")) {
			return Build{}, fmt.Errorf("shopping[%d] %q: selected Alcampo product has an invalid non-EUR or negative price", index, item.Name)
		}
		if product.Available == nil {
			return Build{}, fmt.Errorf("shopping[%d] %q: Alcampo did not confirm whether the selected product is available", index, item.Name)
		}
		if !*product.Available {
			return Build{}, fmt.Errorf("shopping[%d] %q: selected Alcampo product is unavailable", index, item.Name)
		}
		productKey := strings.ToLower(strings.TrimSpace(product.ID))
		if previous, exists := selectedProductIndexes[productKey]; exists {
			return Build{}, fmt.Errorf("shopping[%d] %q: selected product duplicates shopping[%d]; consolidate the shopping lines", index, item.Name, previous)
		}
		selectedProductIndexes[productKey] = index
		variableWeight := isVariableWeightProduct(product)
		if _, fractional := math.Modf(item.Packages); fractional != 0 && !variableWeight {
			return Build{}, fmt.Errorf("shopping[%d] %q: fractional packages are allowed only for a product explicitly sold by variable weight", index, item.Name)
		}
		if strings.TrimSpace(product.Size) == "" && !variableWeight {
			return Build{}, fmt.Errorf("shopping[%d] %q: selected Alcampo product has no package size; choose a product with clear package data", index, item.Name)
		}
		quantityText := decimalQuantity(item.Packages)
		lineCents, err := money.MultiplyCentsByQuantity(product.Price.Cents, quantityText)
		if err != nil {
			return Build{}, fmt.Errorf("shopping[%d] %q: invalid package quantity: %w", index, item.Name, err)
		}
		if lineCents > 0 && total > math.MaxInt64-lineCents {
			return Build{}, fmt.Errorf("shopping[%d] %q: estimated total is outside the supported range", index, item.Name)
		}
		total += lineCents
		selected := SelectedItem{
			Shopping:  item,
			Product:   product,
			LineTotal: money.Money{Amount: money.FormatAmount(lineCents), Currency: "EUR", Cents: lineCents},
		}
		var safetyErr error
		selected.SafetyNote, result.Warnings, safetyErr = productSafety(plan, product, result.Warnings)
		if safetyErr != nil {
			return Build{}, fmt.Errorf("shopping[%d] %q: %w", index, item.Name, safetyErr)
		}
		result.Items = append(result.Items, selected)
	}
	result.EstimatedTotal = money.Money{Amount: money.FormatAmount(total), Currency: "EUR", Cents: total}
	result.Warnings = uniqueSorted(result.Warnings)
	return result, nil
}

func BasketLines(build Build) []string {
	quantities := map[string]*big.Rat{}
	names := map[string]string{}
	for _, item := range build.Items {
		quantity, ok := new(big.Rat).SetString(decimalQuantity(item.Shopping.Packages))
		if !ok {
			continue
		}
		if quantities[item.Product.ID] == nil {
			quantities[item.Product.ID] = new(big.Rat)
		}
		quantities[item.Product.ID].Add(quantities[item.Product.ID], quantity)
		if names[item.Product.ID] == "" {
			names[item.Product.ID] = item.Product.Name
		}
	}
	ids := make([]string, 0, len(quantities))
	for id := range quantities {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	lines := make([]string, 0, len(ids))
	for _, id := range ids {
		quantity := decimalRat(quantities[id])
		comment := strings.TrimSpace(names[id])
		if comment != "" {
			comment = " # " + strings.NewReplacer("\n", " ", "\r", " ").Replace(comment)
		}
		lines = append(lines, id+" "+quantity+comment)
	}
	return lines
}

func decimalQuantity(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func decimalRat(value *big.Rat) string {
	if value == nil {
		return "0"
	}
	text := value.FloatString(9)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	if text == "" || text == "-0" {
		return "0"
	}
	return text
}

func isVariableWeightProduct(product alcampo.Product) bool {
	text := strings.ToLower(strings.Join([]string{product.Name, product.Size, product.Description}, " "))
	return strings.Contains(text, "al peso") || strings.Contains(text, "peso variable") || strings.Contains(text, "variable weight")
}

func productSafety(plan Plan, product alcampo.Product, warnings []string) (string, []string, error) {
	evidence := strings.ToLower(strings.Join([]string{product.Name, product.Category, strings.Join(product.CategoryPath, " "), product.Description, product.Allergens, product.Ingredients}, " "))
	var evidenceGaps []allergyEvidenceGap
	for _, allergy := range plan.Household.Allergies {
		if term := firstUnsafeAllergyTerm(evidence, allergy); term != "" {
			return "", warnings, fmt.Errorf("selected product conflicts with household allergy %q (%s); replace it", allergy, term)
		}
		spec := analyzeAllergy(allergy)
		switch {
		case !spec.supported:
			evidenceGaps = append(evidenceGaps, allergyEvidenceGap{
				allergy: allergy,
				reason:  "the allergy wording could not be mechanically interpreted as one or more named foods",
			})
		case spec.needsIngredientEvidence && !hasReadableOnlineLabel(product.Ingredients):
			evidenceGaps = append(evidenceGaps, allergyEvidenceGap{
				allergy: allergy,
				reason:  "the product has no readable online ingredient list for this named-food allergy",
			})
		}
	}
	for _, rule := range plan.Household.DietaryRules {
		if reason := dietaryConflict(rule, evidence); reason != "" {
			return "", warnings, fmt.Errorf("selected product conflicts with dietary rule %q (%s); replace it", rule, reason)
		}
	}
	clearlySimpleFood := isClearlySimpleFood(product)
	for _, gap := range evidenceGaps {
		if hasSevereAllergy([]string{gap.allergy}) && !clearlySimpleFood {
			return "", warnings, fmt.Errorf("selected product cannot be verified for severe allergy %q: %s; clarify the allergy wording and choose a processed product with a readable, relevant ingredient list", gap.allergy, gap.reason)
		}
	}

	missingLabel := !hasReadableOnlineLabel(product.Allergens) && !hasReadableOnlineLabel(product.Ingredients)
	var safetyNotes []string
	if missingLabel && hasSevereAllergy(plan.Household.Allergies) {
		if clearlySimpleFood {
			warning := fmt.Sprintf("%s appears to be unprocessed food but has no readable online ingredient/allergen label; check the physical package or counter for the severe allergy", product.Name)
			safetyNotes = append(safetyNotes, warning)
		} else {
			return "", warnings, fmt.Errorf("selected product has no readable online ingredient/allergen label for a severe allergy; choose a fully labelled product")
		}
	}
	if missingLabel && !hasSevereAllergy(plan.Household.Allergies) && (len(plan.Household.Allergies) > 0 || hasRecognizedDietRule(plan.Household.DietaryRules)) {
		constraints := append([]string{}, plan.Household.Allergies...)
		constraints = append(constraints, plan.Household.DietaryRules...)
		warning := fmt.Sprintf("%s has no readable online ingredient/allergen label; check the physical package for %s", product.Name, strings.Join(constraints, ", "))
		safetyNotes = append(safetyNotes, warning)
	}
	for _, gap := range evidenceGaps {
		warning := fmt.Sprintf("%s could not be fully checked for allergy %q because %s; check the physical package and confirm the constraint before serving", product.Name, gap.allergy, gap.reason)
		safetyNotes = append(safetyNotes, warning)
	}
	if len(safetyNotes) > 0 {
		safetyNotes = uniqueSorted(safetyNotes)
		warnings = append(warnings, safetyNotes...)
		return strings.Join(safetyNotes, " "), warnings, nil
	}
	if len(plan.Household.Allergies) > 0 {
		return "Online label showed no direct allergy match; still check the package because formulations can change.", warnings, nil
	}
	return "", warnings, nil
}

func hasReadableOnlineLabel(value string) bool {
	key := foldForMatch(value)
	if key == "" || key == "-" {
		return false
	}
	for _, placeholder := range []string{
		"not available", "unavailable", "unknown", "no disponible", "sin informacion",
		"consult package", "see package", "consultar envase", "ver envase",
	} {
		if key == placeholder {
			return false
		}
	}
	return true
}

func firstUnsafeAllergyTerm(text, allergy string) string {
	allergyKey := foldForMatch(allergy)
	plantScrubbed := text
	if containsAny(allergyKey, "milk", "dairy", "leche", "lacteo") {
		plantScrubbed = scrubPlantAlternatives(text)
	}
	for _, term := range allergyTerms(allergy) {
		matchText := text
		if isMilkAllergenTerm(term) {
			matchText = plantScrubbed
		}
		if term != "" && containsUnsafeTerm(matchText, term) {
			return term
		}
	}
	return ""
}

func isMilkAllergenTerm(term string) bool {
	term = foldForMatch(term)
	for _, candidate := range []string{
		"leche", "leches", "milk", "lactosa", "lactose", "suero", "whey",
		"caseina", "caseinas", "casein", "caseins", "queso", "quesos", "cheese", "cheeses",
		"nata", "natas", "cream", "creams", "mantequilla", "mantequillas", "butter", "butters",
		"yogur", "yogures", "yogurt", "yogurts",
	} {
		if term == candidate {
			return true
		}
	}
	return false
}

func dietaryConflict(rule, text string) string {
	key := foldForMatch(rule)
	if isLactoseFreeRule(key) {
		lower := scrubPlantAlternatives(text)
		dairyLike := containsAnyUnsafe(lower, "leche", "milk", "lactosa", "lactose", "nata", "cream", "queso", "cheese", "yogur", "yogurt", "mantequilla", "butter")
		explicitFree := containsAny(foldForMatch(text), "sin lactosa", "lactose-free", "lactose free")
		if dairyLike && !explicitFree {
			return "dairy-like item lacks positive lactose-free or plant-based labeling"
		}
		return ""
	}
	dietText := text
	if strings.Contains(key, "vegan") || strings.Contains(key, "vegano") || strings.Contains(key, "dairy-free") || strings.Contains(key, "dairy free") || strings.Contains(key, "sin lacteos") {
		dietText = scrubPlantAlternatives(text)
	}
	for _, term := range dietaryExclusionTerms(rule) {
		if containsUnsafeTerm(dietText, term) {
			return term
		}
	}
	return ""
}

func containsUnsafeTerm(text, term string) bool {
	text = foldForMatch(text)
	term = foldForMatch(term)
	for _, safePhrase := range []string{
		"gluten-free", "gluten free", "sin gluten", "sin trigo", "wheat-free", "wheat free", "barley-free", "barley free", "rye-free", "rye free",
		"peanut-free", "peanut free", "sin cacahuete", "sin cacahuetes",
		"nut-free", "nut free", "sin frutos secos", "sin frutos de cáscara", "sin nueces",
		"dairy-free", "dairy free", "sin leche", "sin lácteos", "sin lacteos",
		"egg-free", "egg free", "sin huevo", "soy-free", "soy free", "sin soja",
	} {
		text = strings.ReplaceAll(text, foldForMatch(safePhrase), " ")
	}
	for _, safePhrase := range []string{
		term + "-free", term + " free", "free from " + term, "without " + term,
		"sin " + term, "libre de " + term, "no contiene " + term, "does not contain " + term,
	} {
		text = strings.ReplaceAll(text, safePhrase, " ")
	}
	return term != "" && containsBoundedPhrase(text, term)
}

func containsBoundedPhrase(text, phrase string) bool {
	for offset := 0; offset <= len(text); {
		index := strings.Index(text[offset:], phrase)
		if index < 0 {
			return false
		}
		start := offset + index
		end := start + len(phrase)
		beforeOK := start == 0 || !isMatchWordByte(text[start-1])
		afterOK := end == len(text) || !isMatchWordByte(text[end])
		if beforeOK && afterOK {
			return true
		}
		offset = start + 1
	}
	return false
}

func isMatchWordByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= '0' && value <= '9'
}

func containsAnyUnsafe(text string, terms ...string) bool {
	for _, term := range terms {
		if containsUnsafeTerm(text, term) {
			return true
		}
	}
	return false
}

func scrubPlantAlternatives(value string) string {
	value = foldForMatch(value)
	phrases := []string{
		"coconut milk", "coconut cream", "coconut yogurt", "coconut yoghurt",
		"oat milk", "oat cream", "oat yogurt", "oat yoghurt",
		"soy milk", "soya milk", "soy cream", "soy yogurt", "soya yogurt",
		"rice milk", "almond milk", "almond butter", "peanut butter", "nut butter",
		"plant-based milk", "plant based milk", "plant-based cream", "plant based cream",
		"plant-based cheese", "plant based cheese", "plant-based butter", "plant based butter",
		"plant-based yogurt", "plant based yogurt", "vegan milk", "vegan cream", "vegan cheese", "vegan butter", "vegan yogurt",
		"leche de coco", "crema de coco", "nata de coco", "yogur de coco",
		"leche de avena", "crema de avena", "yogur de avena",
		"leche de soja", "crema de soja", "yogur de soja", "leche de arroz", "leche de almendra",
		"mantequilla de cacahuete", "mantequilla de almendra", "bebida vegetal", "yogur vegetal", "queso vegetal", "nata vegetal",
	}
	for _, phrase := range phrases {
		value = strings.ReplaceAll(value, foldForMatch(phrase), " ")
	}
	return value
}

func containsAny(text string, values ...string) bool {
	text = foldForMatch(text)
	for _, value := range values {
		if strings.Contains(text, foldForMatch(value)) {
			return true
		}
	}
	return false
}

func isLactoseFreeRule(value string) bool {
	value = foldForMatch(value)
	return strings.Contains(value, "lactose-free") || strings.Contains(value, "lactose free") || strings.Contains(value, "sin lactosa")
}

func hasSevereAllergy(values []string) bool {
	for _, value := range values {
		key := foldForMatch(value)
		if strings.Contains(key, "sever") || strings.Contains(key, "anaphyl") || strings.Contains(key, "grave") || strings.Contains(key, "anafil") {
			return true
		}
	}
	return false
}

func hasRecognizedDietRule(values []string) bool {
	for _, value := range values {
		if isLactoseFreeRule(strings.ToLower(value)) || len(dietaryExclusionTerms(value)) > 0 {
			return true
		}
	}
	return false
}

var namedAllergyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^(?:(?:severe|serious|life-threatening|life threatening|anaphylactic)\s+)?(.+?)\s+allerg(?:y|ies)(?:\s+(?:is|are)\s+(?:a\s+)?(?:hard|strict)\s+constraint)?$`),
	regexp.MustCompile(`^(?:(?:severely|seriously|anaphylactically)\s+)?allergic\s+to\s+(.+)$`),
	regexp.MustCompile(`^i(?:\s+am|'m)\s+(?:(?:severely|seriously)\s+)?allergic\s+to\s+(.+)$`),
	regexp.MustCompile(`^(?:(?:severe|serious|life-threatening|life threatening)\s+)?allerg(?:y|ies)\s+to\s+(.+)$`),
	regexp.MustCompile(`^allerg(?:y|ies)\s+(?:include|includes|including)\s+(.+)$`),
	regexp.MustCompile(`^alergias?\s+(?:(?:grave|graves|severa|severas|seria|serias)\s+)?(?:a las|a los|a la|al|a)\s+(.+)$`),
	regexp.MustCompile(`^alergic[oa]s?\s+(?:a las|a los|a la|al|a)\s+(.+)$`),
	regexp.MustCompile(`^soy\s+(?:(?:muy|severamente)\s+)?alergic[oa]\s+(?:a las|a los|a la|al|a)\s+(.+)$`),
	regexp.MustCompile(`^tengo\s+(?:una\s+)?alergia\s+(?:(?:grave|severa|seria)\s+)?(?:a las|a los|a la|al|a)\s+(.+)$`),
	regexp.MustCompile(`^alergias?\s+(?:incluye|incluyen|incluyendo)\s+(.+)$`),
	regexp.MustCompile(`^(?:anaphylaxis|anaphylactic reaction)\s+to\s+(.+)$`),
	regexp.MustCompile(`^(?:anafilaxia|reaccion anafilactica)\s+(?:a las|a los|a la|al|a)\s+(.+)$`),
}

var namedAllergenListSplitter = regexp.MustCompile(`(?:\s+(?:and|or|y|o)\s+|[,;/])`)
var namedAllergenWord = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

var allergyGroups = []struct {
	triggers []string
	terms    []string
}{
	{[]string{"peanut", "cacahuete"}, []string{"peanut", "peanuts", "cacahuete", "cacahuetes"}},
	{[]string{"tree nut", "tree-nut", "nut allergy", "nut-allergy", "nuts", "frutos secos", "frutos de cáscara", "almond", "almendra", "walnut", "nuez", "nueces", "hazelnut", "avellana", "cashew", "anacardo", "pistachio", "pistacho"}, []string{"tree nut", "tree nuts", "tree-nut", "tree-nuts", "nuts", "frutos secos", "frutos de cáscara", "almond", "almonds", "almendra", "almendras", "walnut", "walnuts", "nuez", "nueces", "hazelnut", "hazelnuts", "avellana", "avellanas", "pistachio", "pistachios", "pistacho", "pistachos", "cashew", "cashews", "anacardo", "anacardos", "pecan", "pecans", "pecana", "pecanas", "macadamia", "macadamias", "brazil nut", "brazil nuts"}},
	{[]string{"gluten"}, []string{"gluten", "trigo", "wheat", "cebada", "barley", "centeno", "rye"}},
	{[]string{"wheat", "trigo"}, []string{"trigo", "wheat"}},
	{[]string{"milk", "dairy", "leche", "lácteo", "lacteo"}, []string{"leche", "leches", "milk", "lactosa", "lactose", "suero", "whey", "caseina", "caseinas", "casein", "caseins", "queso", "quesos", "cheese", "cheeses", "nata", "natas", "cream", "creams", "mantequilla", "mantequillas", "butter", "butters", "yogur", "yogures", "yogurt", "yogurts"}},
	{[]string{"egg", "huevo"}, []string{"huevo", "huevos", "egg", "eggs"}},
	{[]string{"soy", "soya", "soja"}, []string{"soja", "soy"}},
	{[]string{"fish", "pescado"}, []string{"pescado", "pescados", "fish", "salmon", "salmones", "tuna", "tunas", "atun", "atunes", "hake", "hakes", "merluza", "merluzas"}},
	{[]string{"shellfish", "marisco"}, []string{"crustáceo", "crustáceos", "molusco", "moluscos", "marisco", "mariscos", "shellfish"}},
	{[]string{"sesame", "sésamo", "sesamo"}, []string{"sésamo", "sesame"}},
	{[]string{"mustard", "mostaza"}, []string{"mostaza", "mustard"}},
	{[]string{"celery", "apio"}, []string{"apio", "celery"}},
	{[]string{"sulphite", "sulfite", "sulfito"}, []string{"sulfito", "sulfitos", "sulfite", "sulfites", "sulphite", "sulphites"}},
	{[]string{"lupin", "altramuz", "altramuces"}, []string{"altramuz", "altramuces", "lupin", "lupins"}},
	{[]string{"mollusc", "mollusk", "molusco"}, []string{"molusco", "moluscos", "mollusc", "molluscs", "mollusk", "mollusks"}},
	{[]string{"crustacean", "crustáceo", "crustaceo"}, []string{"crustáceo", "crustáceos", "crustacean", "crustaceans"}},
}

func dietaryExclusionTerms(value string) []string {
	key := foldForMatch(value)
	switch {
	case strings.Contains(key, "gluten-free"), strings.Contains(key, "gluten free"), strings.Contains(key, "sin gluten"):
		return []string{"gluten", "trigo", "wheat", "cebada", "barley", "centeno", "rye"}
	case strings.Contains(key, "vegan"), strings.Contains(key, "vegano"):
		return []string{"leche", "leches", "milk", "lactosa", "lactose", "suero", "whey", "caseina", "caseinas", "casein", "caseins", "queso", "quesos", "cheese", "cheeses", "nata", "natas", "cream", "creams", "mantequilla", "mantequillas", "butter", "butters", "yogur", "yogures", "yogurt", "yogurts", "huevo", "huevos", "egg", "eggs", "miel", "mieles", "honey", "gelatina", "gelatinas", "gelatin", "gelatins", "manteca", "mantecas", "lard", "pollo", "pollos", "chicken", "chickens", "pavo", "pavos", "turkey", "turkeys", "ternera", "terneras", "beef", "cerdo", "cerdos", "pork", "pescado", "pescados", "fish", "salmon", "salmones", "merluza", "merluzas", "hake", "hakes", "atun", "atunes", "tuna", "tunas", "marisco", "mariscos", "shellfish"}
	case strings.Contains(key, "vegetarian"), strings.Contains(key, "vegetariano"):
		return []string{"gelatina", "gelatinas", "gelatin", "gelatins", "manteca", "mantecas", "lard", "pollo", "pollos", "chicken", "chickens", "pavo", "pavos", "turkey", "turkeys", "ternera", "terneras", "beef", "cerdo", "cerdos", "pork", "pescado", "pescados", "fish", "salmon", "salmones", "merluza", "merluzas", "hake", "hakes", "atun", "atunes", "tuna", "tunas", "marisco", "mariscos", "shellfish"}
	case strings.Contains(key, "pescatari"), strings.Contains(key, "pescetari"):
		return []string{"gelatina", "gelatinas", "gelatin", "gelatins", "manteca", "mantecas", "lard", "pollo", "pollos", "chicken", "chickens", "pavo", "pavos", "turkey", "turkeys", "ternera", "terneras", "beef", "cerdo", "cerdos", "pork"}
	case strings.Contains(key, "dairy-free"), strings.Contains(key, "dairy free"), strings.Contains(key, "sin lácteos"), strings.Contains(key, "sin lacteos"):
		return []string{"leche", "leches", "milk", "lactosa", "lactose", "suero", "whey", "caseina", "caseinas", "casein", "caseins", "nata", "natas", "cream", "creams", "queso", "quesos", "cheese", "cheeses", "yogur", "yogures", "yogurt", "yogurts", "mantequilla", "mantequillas", "butter", "butters"}
	default:
		return nil
	}
}

func allergyTerms(value string) []string {
	return analyzeAllergy(value).terms
}

func analyzeAllergy(value string) allergyMatchSpec {
	key := foldForMatch(value)
	groupKey := key
	if strings.HasPrefix(groupKey, "soy alergic") {
		groupKey = strings.TrimPrefix(groupKey, "soy ")
	}
	var terms []string
	var knownTerms []string
	groupMatched := false
	for _, group := range allergyGroups {
		if containsAnyBoundedPhrase(groupKey, group.triggers) || containsAnyBoundedPhrase(groupKey, group.terms) {
			terms = append(terms, group.terms...)
			knownTerms = append(knownTerms, group.terms...)
			groupMatched = true
		}
	}

	namedTerms, attempted, parsed := extractNamedAllergyTerms(key)
	if !attempted {
		if terms, ok := parseNamedAllergenList(key); ok {
			namedTerms = terms
			parsed = true
		}
	}
	needsIngredientEvidence := false
	if parsed {
		for _, namedTerm := range namedTerms {
			variants := namedAllergenVariants(namedTerm)
			terms = append(terms, variants...)
			if !namedTermCoveredByKnownGroup(variants, knownTerms) {
				needsIngredientEvidence = true
			}
		}
	}
	supported := groupMatched || parsed
	if attempted && !parsed {
		supported = false
	}
	return allergyMatchSpec{
		terms:                   uniqueSorted(terms),
		supported:               supported,
		needsIngredientEvidence: needsIngredientEvidence,
	}
}

func containsAnyBoundedPhrase(text string, values []string) bool {
	for _, value := range values {
		if containsBoundedPhrase(text, foldForMatch(value)) {
			return true
		}
	}
	return false
}

func extractNamedAllergyTerms(value string) ([]string, bool, bool) {
	value = strings.Join(strings.Fields(strings.Trim(value, " .!?;:")), " ")
	for _, pattern := range namedAllergyPatterns {
		matches := pattern.FindStringSubmatch(value)
		if len(matches) != 2 {
			continue
		}
		terms, ok := parseNamedAllergenList(matches[1])
		if !ok {
			return nil, true, false
		}
		return terms, true, true
	}
	return nil, false, false
}

func parseNamedAllergenList(value string) ([]string, bool) {
	fragments := namedAllergenListSplitter.Split(value, -1)
	terms := make([]string, 0, len(fragments))
	for _, fragment := range fragments {
		term, ok := normalizeNamedAllergen(fragment)
		if !ok {
			return nil, false
		}
		terms = append(terms, term)
	}
	if len(terms) == 0 {
		return nil, false
	}
	return terms, true
}

func normalizeNamedAllergen(value string) (string, bool) {
	value = strings.Join(strings.Fields(strings.Trim(foldForMatch(value), " .!?;:")), " ")
	for {
		trimmed := value
		for _, prefix := range []string{
			"a las ", "a los ", "a la ", "al ", "a ",
			"the ", "an ", "el ", "la ", "los ", "las ", "un ", "una ", "unos ", "unas ",
		} {
			if strings.HasPrefix(trimmed, prefix) {
				trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
				break
			}
		}
		if trimmed == value {
			break
		}
		value = trimmed
	}
	words := strings.Fields(value)
	if len(words) == 0 || len(words) > 4 {
		return "", false
	}
	disallowed := map[string]bool{
		"allergy": true, "allergies": true, "allergic": true, "allergen": true, "allergens": true,
		"alergia": true, "alergias": true, "alergico": true, "alergica": true, "alergicos": true, "alergicas": true,
		"severe": true, "severely": true, "serious": true, "grave": true, "graves": true, "severa": true, "severas": true,
		"anaphylaxis": true, "anaphylactic": true, "anafilaxia": true, "anafilactica": true,
		"food": true, "foods": true, "comida": true, "alimento": true, "alimentos": true,
		"ingredient": true, "ingredients": true, "ingrediente": true, "ingredientes": true,
		"unknown": true, "unspecified": true, "multiple": true, "various": true, "several": true,
		"desconocido": true, "desconocida": true, "varios": true, "varias": true,
		"something": true, "anything": true, "algo": true, "ciertos": true, "ciertas": true,
		"avoid": true, "avoiding": true, "evitar": true, "except": true, "excepto": true,
		"intolerance": true, "intolerant": true, "sensitivity": true, "sensitive": true,
		"constraint": true, "hard": true, "strict": true, "riesgo": true,
		"is": true, "are": true, "with": true, "because": true, "but": true,
		"my": true, "our": true, "has": true, "have": true, "family": true,
		"child": true, "children": true, "kid": true, "son": true, "daughter": true,
	}
	hasLetter := false
	for _, word := range words {
		if !namedAllergenWord.MatchString(word) || disallowed[word] {
			return "", false
		}
		for _, character := range word {
			if character >= 'a' && character <= 'z' {
				hasLetter = true
				break
			}
		}
	}
	if !hasLetter {
		return "", false
	}
	return strings.Join(words, " "), true
}

func namedAllergenVariants(term string) []string {
	term = foldForMatch(term)
	words := strings.Fields(term)
	if len(words) == 0 {
		return nil
	}
	variants := []string{term}
	lastIndex := len(words) - 1
	last := words[lastIndex]
	addWithLastWord := func(replacement string) {
		copyWords := append([]string(nil), words...)
		copyWords[lastIndex] = replacement
		variants = append(variants, strings.Join(copyWords, " "))
	}
	switch {
	case len(last) > 3 && strings.HasSuffix(last, "ies"):
		addWithLastWord(strings.TrimSuffix(last, "ies") + "y")
	case len(last) > 3 && strings.HasSuffix(last, "s") && !strings.HasSuffix(last, "ss") && !strings.HasSuffix(last, "us") && !strings.HasSuffix(last, "is"):
		addWithLastWord(strings.TrimSuffix(last, "s"))
	case len(last) > 1 && strings.HasSuffix(last, "y") && !strings.ContainsAny(last[len(last)-2:len(last)-1], "aeiou"):
		addWithLastWord(strings.TrimSuffix(last, "y") + "ies")
	default:
		addWithLastWord(last + "s")
		if strings.HasSuffix(last, "o") {
			addWithLastWord(last + "es")
		}
	}
	return uniqueSorted(variants)
}

func namedTermCoveredByKnownGroup(variants, knownTerms []string) bool {
	for _, variant := range variants {
		variant = foldForMatch(variant)
		for _, known := range knownTerms {
			if variant == foldForMatch(known) {
				return true
			}
		}
	}
	return false
}

func foldForMatch(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.NewReplacer(
		"á", "a", "à", "a", "ä", "a", "â", "a",
		"é", "e", "è", "e", "ë", "e", "ê", "e",
		"í", "i", "ì", "i", "ï", "i", "î", "i",
		"ó", "o", "ò", "o", "ö", "o", "ô", "o",
		"ú", "u", "ù", "u", "ü", "u", "û", "u",
		"ñ", "n", "ç", "c",
	).Replace(value)
}

func isClearlySimpleFood(product alcampo.Product) bool {
	fresh := false
	rawSection := false
	processedSection := false
	for _, part := range product.CategoryPath {
		part = foldForMatch(part)
		if part == "frescos" || part == "fresh" {
			fresh = true
		}
		if containsAny(part, "fruta", "verdura", "hortaliza", "carne", "pollo", "ave", "pescado", "pescaderia", "marisco", "huevo") {
			rawSection = true
		}
		if containsAny(part, "elaborad", "preparad", "platos", "charcuteria", "hamburgues", "salchich", "marinad", "adobad", "empanad", "breaded", "ready meal") {
			processedSection = true
		}
	}
	if !fresh || !rawSection || processedSection {
		return false
	}
	text := foldForMatch(strings.Join([]string{product.Name, product.Description}, " "))
	return !containsAny(text,
		"salsa", "sauce", "rellen", "stuffed", "marinad", "adobad", "empanad", "breaded",
		"preparad", "prepared", "mezcla", " mix ", "burger", "hamburgues", "salchich", "sausage", "seasoned",
	)
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
