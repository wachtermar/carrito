package food

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

func LoadPantryProfile(path string) (PantryProfile, error) {
	if strings.TrimSpace(path) == "" {
		return PantryProfile{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return PantryProfile{}, err
	}
	var profile PantryProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return PantryProfile{}, err
	}
	if profile.SchemaVersion == "" {
		profile.SchemaVersion = "1"
	}
	return profile, nil
}

func MergePantryMemoryIntoProfile(profile PantryProfile, pantry Pantry) PantryProfile {
	if len(pantry.Items) == 0 {
		return profile
	}
	existing := pantryItemsByKey(profile)
	for _, item := range pantry.Items {
		key := normalizePantryIngredientKey(item.Name)
		if key == "" {
			continue
		}
		if _, ok := existing[key]; ok {
			continue
		}
		if item.Quantity <= 0 {
			continue
		}
		confidence := item.Confidence
		if confidence <= 0 {
			confidence = 0.85
		}
		q, ok := normalizedIngredientQuantity(item.Quantity, item.Unit, confidence, "saved_pantry")
		if !ok {
			q = NormalizedQuantity{
				Raw:         formatIngredientQuantity(item.Quantity, item.Unit),
				Value:       item.Quantity,
				Unit:        normalizeUnit(item.Unit),
				Confidence:  confidence,
				ParseMethod: "saved_pantry_unparseable",
			}
		}
		profile.Items = append(profile.Items, PantryProfileItem{
			IngredientKey:  key,
			Names:          []string{item.Name},
			Category:       item.Location,
			Status:         PantryItemStatusConfirmedAvailable,
			Quantity:       &q,
			ExpiresAt:      item.ExpiryDate,
			LastVerifiedAt: item.LastChecked,
			Confidence:     confidence,
			Source:         "food_pantry_memory",
			Notes:          item.Notes,
			Nutrition:      item.Nutrition,
		})
		existing[key] = profile.Items[len(profile.Items)-1]
	}
	return profile
}

func DefaultPantryPolicy(enabled bool, defaultPantry string) PantryPolicy {
	defaultPantry = normalizeDefaultPantry(defaultPantry)
	return PantryPolicy{
		Enabled:                    enabled,
		DefaultPantry:              defaultPantry,
		AllowAssumedPantry:         defaultPantry == DefaultPantryMinimalSpanish,
		RequireConfirmedPantry:     false,
		UseExpiredItems:            false,
		ShopUnknownStaples:         false,
		ShopAllIngredients:         false,
		AllowPartialPantryCoverage: true,
		BlockAmbiguousPantryUnits:  true,
		AlwaysBuyFresh:             true,
	}
}

func ApplyPantryResolution(plan MealPlan, profile PantryProfile, policy PantryPolicy) (MealPlan, PantryResolution) {
	resolution := ResolvePantry(plan, profile, policy)
	if !policy.Enabled || policy.ShopAllIngredients || resolution.Status == PantryResolutionNotUsed {
		return plan, resolution
	}
	plan.RequiredPurchases = shoppingIngredientsFromResolution(resolution)
	plan.PantryUsage = pantryUsageFromResolution(resolution)
	if len(plan.PantryUsage) > 0 {
		plan.Notes = appendUniqueString(plan.Notes, "Structured pantry resolution adjusted the Alcampo shopping requirements.")
	}
	return plan, resolution
}

func StampPantryResolutionServing(resolution *PantryResolution, servingPlanFingerprint, scaledMealPlanFingerprint string) {
	if resolution == nil {
		return
	}
	resolution.ServingPlanFingerprint = strings.TrimSpace(servingPlanFingerprint)
	resolution.ScaledMealPlanFingerprint = strings.TrimSpace(scaledMealPlanFingerprint)
	resolution.PantryResolutionFingerprint = PantryResolutionFingerprint(*resolution)
	resolution.ShopRequirementsFingerprint = ShopRequirementsFingerprintFromPantryResolution(*resolution)
}

func ResolvePantry(plan MealPlan, profile PantryProfile, policy PantryPolicy) PantryResolution {
	policy.DefaultPantry = normalizeDefaultPantry(policy.DefaultPantry)
	if policy.ShopAllIngredients {
		policy.Enabled = false
	}
	profile = mergeDefaultPantryProfile(profile, policy)
	resolution := PantryResolution{
		SchemaVersion:            "1",
		Status:                   PantryResolutionNotUsed,
		Policy:                   policy,
		MealPlanFingerprint:      MealPlanFingerprint(plan),
		PantryProfileFingerprint: PantryProfileFingerprint(profile),
	}
	if !policy.Enabled {
		resolution.ShopRequirementsFingerprint = ShopRequirementsFingerprintFromIngredients(plan.RequiredPurchases)
		resolution.PantryResolutionFingerprint = PantryResolutionFingerprint(resolution)
		return resolution
	}
	requirements := pantryRequirementsFromPlan(plan)
	items := pantryItemsByKey(profile)
	for _, req := range requirements {
		line := resolvePantryLine(req, items, policy)
		resolution.Lines = append(resolution.Lines, line)
	}
	resolution.Summary = summarizePantryResolution(resolution.Lines)
	for _, line := range resolution.Lines {
		if strings.HasPrefix(line.PantryDecision, "blocked") {
			resolution.BlockingIssues = append(resolution.BlockingIssues, PantryIssue{
				Code:           line.PantryDecision,
				Severity:       "blocking",
				IngredientKey:  line.IngredientKey,
				IngredientName: line.IngredientName,
				Message:        line.Reason,
				Remediation:    "Confirm the pantry item, allow pantry assumptions, or shop all ingredients.",
			})
			continue
		}
		if line.PantryDecision == PantryDecisionPantryFullAssumed {
			resolution.Warnings = append(resolution.Warnings, PantryIssue{
				Code:           "pantry_assumption",
				Severity:       "warning",
				IngredientKey:  line.IngredientKey,
				IngredientName: line.IngredientName,
				Message:        line.IngredientName + " is assumed to be available in the pantry and is not included in the Alcampo basket.",
			})
		}
		for _, warning := range line.Warnings {
			resolution.Warnings = append(resolution.Warnings, PantryIssue{
				Code:           "pantry_line_warning",
				Severity:       "warning",
				IngredientKey:  line.IngredientKey,
				IngredientName: line.IngredientName,
				Message:        warning,
			})
		}
	}
	switch {
	case len(resolution.BlockingIssues) > 0:
		resolution.Status = PantryResolutionBlocked
	case resolution.Summary.PantryAssumedLines > 0:
		resolution.Status = PantryResolutionAppliedWithAssumptions
	case resolution.Summary.PantryCoveredIngredientLines > 0 || resolution.Summary.BuyDeltaLines > 0:
		resolution.Status = PantryResolutionApplied
	default:
		resolution.Status = PantryResolutionApplied
	}
	resolution.ShopRequirementsFingerprint = ShopRequirementsFingerprintFromPantryResolution(resolution)
	resolution.PantryResolutionFingerprint = PantryResolutionFingerprint(resolution)
	return resolution
}

func BuildShoppingRequirements(plan MealPlan, resolution *PantryResolution) ([]ShoppingRequirement, string, error) {
	if resolution == nil || resolution.Status == PantryResolutionNotUsed {
		var requirements []ShoppingRequirement
		for _, ingredient := range plan.RequiredPurchases {
			q, _ := normalizedIngredientQuantity(ingredient.Quantity, ingredient.Unit, 0.95, "shopping_requirement")
			requirements = append(requirements, ShoppingRequirement{
				IngredientKey:  pantryIngredientKey(ingredient),
				IngredientName: ingredient.Name,
				SearchTerm:     ingredient.SearchTerm,
				Category:       ingredient.Category,
				TotalRequired:  &q,
				ShopRequired:   &q,
				SourcingStatus: PantryDecisionBuyFull,
				PantryDecision: PantryDecisionBuyFull,
			})
		}
		return requirements, ShopRequirementsFingerprintFromRequirements(requirements), nil
	}
	var requirements []ShoppingRequirement
	for _, line := range resolution.Lines {
		if line.ShopRequired == nil || line.ShopRequired.BaseValue <= 0 {
			continue
		}
		requirements = append(requirements, ShoppingRequirement{
			IngredientKey:   line.IngredientKey,
			IngredientName:  line.IngredientName,
			SearchTerm:      line.SearchTerm,
			Category:        line.Category,
			TotalRequired:   cloneNormalizedQuantity(line.TotalRequired),
			PantryAllocated: cloneNormalizedQuantity(line.PantryAllocated),
			ShopRequired:    cloneNormalizedQuantity(line.ShopRequired),
			SourcingStatus:  pantrySourcingStatus(line),
			PantryDecision:  line.PantryDecision,
			Usages:          append([]IngredientUsageRef(nil), line.Usages...),
		})
	}
	return requirements, ShopRequirementsFingerprintFromRequirements(requirements), nil
}

func PantryProfileFingerprint(profile PantryProfile) string {
	profile.Items = append([]PantryProfileItem(nil), profile.Items...)
	sort.SliceStable(profile.Items, func(i, j int) bool {
		return profile.Items[i].IngredientKey < profile.Items[j].IngredientKey
	})
	data, err := json.Marshal(profile)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func PantryResolutionFingerprint(resolution PantryResolution) string {
	type canonical struct {
		Policy                    PantryPolicy           `json:"policy"`
		MealPlanFingerprint       string                 `json:"mealplan_fingerprint,omitempty"`
		ServingPlanFingerprint    string                 `json:"serving_plan_fingerprint,omitempty"`
		ScaledMealPlanFingerprint string                 `json:"scaled_mealplan_fingerprint,omitempty"`
		PantryProfileFingerprint  string                 `json:"pantry_profile_fingerprint,omitempty"`
		Lines                     []PantryResolutionLine `json:"lines,omitempty"`
	}
	rows := append([]PantryResolutionLine(nil), resolution.Lines...)
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].IngredientKey != rows[j].IngredientKey {
			return rows[i].IngredientKey < rows[j].IngredientKey
		}
		return rows[i].IngredientName < rows[j].IngredientName
	})
	data, err := json.Marshal(canonical{
		Policy:                    resolution.Policy,
		MealPlanFingerprint:       resolution.MealPlanFingerprint,
		ServingPlanFingerprint:    resolution.ServingPlanFingerprint,
		ScaledMealPlanFingerprint: resolution.ScaledMealPlanFingerprint,
		PantryProfileFingerprint:  resolution.PantryProfileFingerprint,
		Lines:                     rows,
	})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func ShopRequirementsFingerprintFromPantryResolution(resolution PantryResolution) string {
	requirements, _, _ := BuildShoppingRequirements(MealPlan{}, &resolution)
	return ShopRequirementsFingerprintFromRequirements(requirements)
}

func ShopRequirementsFingerprintFromIngredients(ingredients []Ingredient) string {
	var requirements []ShoppingRequirement
	for _, ingredient := range ingredients {
		q, _ := normalizedIngredientQuantity(ingredient.Quantity, ingredient.Unit, 0.95, "shopping_requirement")
		requirements = append(requirements, ShoppingRequirement{
			IngredientKey:  pantryIngredientKey(ingredient),
			IngredientName: ingredient.Name,
			SearchTerm:     ingredient.SearchTerm,
			Category:       ingredient.Category,
			TotalRequired:  &q,
			ShopRequired:   &q,
			SourcingStatus: PantryDecisionBuyFull,
			PantryDecision: PantryDecisionBuyFull,
		})
	}
	return ShopRequirementsFingerprintFromRequirements(requirements)
}

func ShopRequirementsFingerprintFromRequirements(requirements []ShoppingRequirement) string {
	rows := append([]ShoppingRequirement(nil), requirements...)
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].IngredientKey != rows[j].IngredientKey {
			return rows[i].IngredientKey < rows[j].IngredientKey
		}
		return rows[i].IngredientName < rows[j].IngredientName
	})
	data, err := json.Marshal(rows)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func pantryRequirementsFromPlan(plan MealPlan) []pantryRequirement {
	byKey := map[string]int{}
	var requirements []pantryRequirement
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			for _, ingredient := range meal.Recipe.Ingredients {
				key := pantryIngredientKey(ingredient)
				if key == "" {
					key = normalizeKey(ingredient.Name)
				}
				q, ok := normalizedIngredientQuantity(ingredient.Quantity, ingredient.Unit, 0.95, "recipe_ingredient")
				if !ok {
					q = NormalizedQuantity{Raw: formatIngredientQuantity(ingredient.Quantity, ingredient.Unit), Value: ingredient.Quantity, Unit: normalizeUnit(ingredient.Unit), Confidence: 0.2, ParseMethod: "recipe_ingredient_unparseable"}
				}
				ref := IngredientUsageRef{
					Day:              day.Day,
					MealSlot:         meal.Type,
					RecipeID:         meal.Recipe.ID,
					RecipeTitle:      meal.Recipe.Title,
					IngredientKey:    key,
					IngredientName:   ingredient.Name,
					RequiredQuantity: &q,
				}
				idx, exists := byKey[key+"|"+q.BaseUnit]
				if !exists {
					byKey[key+"|"+q.BaseUnit] = len(requirements)
					requirements = append(requirements, pantryRequirement{
						IngredientKey: key,
						Name:          ingredient.Name,
						SearchTerm:    ingredient.SearchTerm,
						Category:      ingredient.Category,
						Optional:      ingredient.Optional,
						Quantity:      q,
						Usages:        []IngredientUsageRef{ref},
					})
					continue
				}
				requirements[idx].Quantity.BaseValue = roundQty(requirements[idx].Quantity.BaseValue + q.BaseValue)
				requirements[idx].Quantity.Value = roundQty(requirements[idx].Quantity.Value + q.Value)
				requirements[idx].Quantity.Raw = formatIngredientQuantity(requirements[idx].Quantity.Value, requirements[idx].Quantity.Unit)
				requirements[idx].Usages = append(requirements[idx].Usages, ref)
			}
		}
	}
	if len(requirements) == 0 {
		for _, ingredient := range plan.RequiredPurchases {
			q, ok := normalizedIngredientQuantity(ingredient.Quantity, ingredient.Unit, 0.95, "required_purchase")
			if !ok {
				q = NormalizedQuantity{Raw: formatIngredientQuantity(ingredient.Quantity, ingredient.Unit), Value: ingredient.Quantity, Unit: normalizeUnit(ingredient.Unit), Confidence: 0.2, ParseMethod: "required_purchase_unparseable"}
			}
			requirements = append(requirements, pantryRequirement{IngredientKey: pantryIngredientKey(ingredient), Name: ingredient.Name, SearchTerm: ingredient.SearchTerm, Category: ingredient.Category, Optional: ingredient.Optional, Quantity: q})
		}
	}
	sort.SliceStable(requirements, func(i, j int) bool {
		return requirements[i].Name < requirements[j].Name
	})
	return requirements
}

type pantryRequirement struct {
	IngredientKey string
	Name          string
	SearchTerm    string
	Category      string
	Optional      bool
	Quantity      NormalizedQuantity
	Usages        []IngredientUsageRef
}

func resolvePantryLine(req pantryRequirement, items map[string]PantryProfileItem, policy PantryPolicy) PantryResolutionLine {
	total := req.Quantity
	line := PantryResolutionLine{
		IngredientKey:  req.IngredientKey,
		IngredientName: req.Name,
		SearchTerm:     req.SearchTerm,
		Category:       req.Category,
		Usages:         append([]IngredientUsageRef(nil), req.Usages...),
		TotalRequired:  &total,
		ShopRequired:   &total,
		PantryDecision: PantryDecisionBuyFull,
		PantryStatus:   "none",
		Confidence:     total.Confidence,
		Reason:         "No pantry coverage was applied; ingredient remains in the Alcampo shopping requirements.",
	}
	if req.Optional && (total.BaseValue <= 0 || total.Unit == "") {
		line.PantryDecision = PantryDecisionOptionalSkipped
		line.ShopRequired = nil
		line.Reason = "Optional ingredient has no required quantity and was skipped."
		return line
	}
	item, ok := items[req.IngredientKey]
	if !ok {
		for _, candidate := range items {
			if pantryItemMatchesRequirement(candidate, req) {
				item = candidate
				ok = true
				break
			}
		}
	}
	if !ok || item.Status == "" {
		if policy.ShopUnknownStaples && isDefaultPantryKey(req.IngredientKey) {
			line.Reason = "Unknown pantry staple is being shopped because policy requires shopping unknown staples."
		}
		return line
	}
	line.PantryItemKey = item.IngredientKey
	line.PantryStatus = item.Status
	line.Confidence = positiveOrDefault(item.Confidence, total.Confidence)
	if item.Status == PantryItemStatusIgnore {
		line.PantryDecision = PantryDecisionIgnored
		line.Reason = "Pantry item is marked ignore; ingredient remains in shopping requirements."
		return line
	}
	if item.Status == PantryItemStatusConfirmedUnavailable {
		line.PantryDecision = PantryDecisionBuyFull
		line.Reason = "Pantry item is confirmed unavailable; full quantity remains in shopping requirements."
		return line
	}
	if pantryItemExpired(item) && !policy.UseExpiredItems {
		line.PantryDecision = PantryDecisionBuyFull
		line.Warnings = append(line.Warnings, "Pantry item is expired and was not used.")
		line.Reason = "Expired pantry item was ignored; ingredient remains in shopping requirements."
		return line
	}
	if item.Status == PantryItemStatusAssumedAvailable {
		if policy.RequireConfirmedPantry {
			line.PantryDecision = PantryDecisionBlockedUnconfirmed
			line.ShopRequired = nil
			line.Reason = "Pantry item is assumed but confirmed pantry is required."
			return line
		}
		if policy.AllowAssumedPantry {
			line.PantryDecision = PantryDecisionPantryFullAssumed
			line.PantryAllocated = &total
			line.ShopRequired = nil
			line.Reason = "Ingredient is covered by an allowed pantry assumption and is not included in the Alcampo basket."
			return line
		}
		line.PantryDecision = PantryDecisionBuyFull
		line.Reason = "Pantry assumption is not allowed; ingredient remains in shopping requirements."
		return line
	}
	if item.Status != PantryItemStatusConfirmedAvailable {
		return line
	}
	if item.Quantity == nil {
		line.PantryDecision = PantryDecisionPantryFullConfirmed
		line.PantryAllocated = &total
		line.ShopRequired = nil
		line.Reason = "Ingredient is covered by confirmed pantry availability."
		return line
	}
	available := *item.Quantity
	line.PantryAvailable = &available
	if available.BaseUnit == "" || total.BaseUnit == "" || available.BaseUnit != total.BaseUnit {
		if policy.BlockAmbiguousPantryUnits {
			line.PantryDecision = PantryDecisionBlockedAmbiguousUnit
			line.ShopRequired = nil
			line.Reason = "Confirmed pantry quantity has an ambiguous or incompatible unit."
			return line
		}
		return line
	}
	usableBase := available.BaseValue
	if item.MinKeep != nil && item.MinKeep.BaseUnit == available.BaseUnit {
		usableBase = mathMax(0, usableBase-item.MinKeep.BaseValue)
	}
	if usableBase <= 0 {
		line.Reason = "Confirmed pantry item is reserved by min_keep; full quantity remains in shopping requirements."
		return line
	}
	if usableBase >= total.BaseValue {
		allocated := quantityFromBase(total.BaseValue, total.BaseUnit, available.Confidence, "pantry_confirmed")
		remaining := quantityFromBase(usableBase-total.BaseValue, total.BaseUnit, available.Confidence, "pantry_remaining")
		line.PantryDecision = PantryDecisionPantryFullConfirmed
		line.PantryAllocated = &allocated
		line.RemainingAfterUse = &remaining
		line.ShopRequired = nil
		line.Reason = "Ingredient is fully covered by confirmed pantry quantity."
		return line
	}
	if !policy.AllowPartialPantryCoverage {
		line.Reason = "Confirmed pantry quantity is partial, but partial pantry coverage is disabled."
		return line
	}
	allocated := quantityFromBase(usableBase, total.BaseUnit, available.Confidence, "pantry_partial")
	shopRequired := quantityFromBase(total.BaseValue-usableBase, total.BaseUnit, total.Confidence, "pantry_delta")
	line.PantryDecision = PantryDecisionPantryPartialConfirmed
	line.PantryAllocated = &allocated
	line.ShopRequired = &shopRequired
	line.Reason = "Ingredient is partially covered by confirmed pantry quantity; only the delta is shopped from Alcampo."
	return line
}

func shoppingIngredientsFromResolution(resolution PantryResolution) []Ingredient {
	var out []Ingredient
	for _, line := range resolution.Lines {
		if line.ShopRequired == nil || line.ShopRequired.BaseValue <= 0 {
			continue
		}
		out = append(out, Ingredient{
			Name:       line.IngredientName,
			Quantity:   line.ShopRequired.Value,
			Unit:       line.ShopRequired.Unit,
			Category:   line.Category,
			SearchTerm: line.SearchTerm,
		})
	}
	return mergeIngredients(out)
}

func pantryUsageFromResolution(resolution PantryResolution) []PantryUsage {
	var out []PantryUsage
	for _, line := range resolution.Lines {
		if line.PantryAllocated == nil || line.PantryAllocated.BaseValue <= 0 {
			continue
		}
		out = append(out, PantryUsage{
			Ingredient: line.IngredientName,
			PantryItem: firstNonEmptyString(line.PantryItemKey, line.IngredientName),
			Quantity:   line.PantryAllocated.Value,
			Unit:       line.PantryAllocated.Unit,
		})
	}
	return out
}

func summarizePantryResolution(lines []PantryResolutionLine) PantryResolutionSummary {
	summary := PantryResolutionSummary{TotalIngredientLines: len(lines)}
	for _, line := range lines {
		if line.ShopRequired != nil && line.ShopRequired.BaseValue > 0 {
			summary.ShopIngredientLines++
		}
		if line.PantryAllocated != nil && line.PantryAllocated.BaseValue > 0 {
			summary.PantryCoveredIngredientLines++
		}
		switch line.PantryDecision {
		case PantryDecisionBuyFull:
			summary.BuyFullLines++
		case PantryDecisionBuyDelta, PantryDecisionPantryPartialConfirmed:
			summary.BuyDeltaLines++
		case PantryDecisionPantryFullConfirmed:
			summary.PantryConfirmedLines++
		case PantryDecisionPantryFullAssumed:
			summary.PantryAssumedLines++
		case PantryDecisionOptionalSkipped:
			summary.OptionalSkippedLines++
		case PantryDecisionBlockedUnconfirmed, PantryDecisionBlockedAmbiguousUnit, PantryDecisionBlockedExpired:
			summary.BlockedLines++
		}
	}
	return summary
}

func mergeDefaultPantryProfile(profile PantryProfile, policy PantryPolicy) PantryProfile {
	defaultProfile := normalizeDefaultPantry(policy.DefaultPantry)
	profile.DefaultProfile = firstNonEmptyString(profile.DefaultProfile, defaultProfile)
	if profile.SchemaVersion == "" {
		profile.SchemaVersion = "1"
	}
	existing := pantryItemsByKey(profile)
	for _, item := range defaultPantryItems(defaultProfile) {
		if _, ok := existing[item.IngredientKey]; ok {
			continue
		}
		profile.Items = append(profile.Items, item)
	}
	return profile
}

func defaultPantryItems(defaultProfile string) []PantryProfileItem {
	switch normalizeDefaultPantry(defaultProfile) {
	case DefaultPantryMinimalSpanish:
		return []PantryProfileItem{
			defaultPantryItem("water", []string{"water", "agua"}, "staple"),
			defaultPantryItem("salt", []string{"salt", "sal", "sal fina", "sal marina"}, "spice"),
			defaultPantryItem("black_pepper", []string{"black pepper", "pimienta", "pimienta negra"}, "spice"),
		}
	case DefaultPantryMediterraneanBasic:
		items := defaultPantryItems(DefaultPantryMinimalSpanish)
		items = append(items,
			defaultPantryItem("olive_oil", []string{"olive oil", "aceite de oliva", "aceite de oliva virgen extra", "aove"}, "oil"),
			defaultPantryItem("vinegar", []string{"vinegar", "vinagre", "vinagre de vino"}, "staple"),
			defaultPantryItem("sugar", []string{"sugar", "azucar", "azúcar"}, "dry_good"),
			defaultPantryItem("flour", []string{"flour", "harina"}, "dry_good"),
			defaultPantryItem("oregano", []string{"oregano", "orégano"}, "spice"),
			defaultPantryItem("paprika", []string{"paprika", "pimenton", "pimentón"}, "spice"),
			defaultPantryItem("cumin", []string{"cumin", "comino"}, "spice"),
		)
		return items
	default:
		return nil
	}
}

func defaultPantryItem(key string, names []string, category string) PantryProfileItem {
	return PantryProfileItem{
		IngredientKey: key,
		Names:         names,
		Aliases:       names,
		Category:      category,
		Form:          "unknown",
		Status:        PantryItemStatusAssumedAvailable,
		Confidence:    0.65,
		Source:        "default_profile",
	}
}

func pantryItemsByKey(profile PantryProfile) map[string]PantryProfileItem {
	out := map[string]PantryProfileItem{}
	for _, item := range profile.Items {
		key := normalizePantryIngredientKey(firstNonEmptyString(item.IngredientKey, firstNonEmptyString(item.Names...)))
		if key == "" {
			continue
		}
		item.IngredientKey = key
		if item.Status == "" {
			item.Status = PantryItemStatusConfirmedAvailable
		}
		out[key] = item
	}
	return out
}

func pantryItemMatchesRequirement(item PantryProfileItem, req pantryRequirement) bool {
	values := append([]string{item.IngredientKey}, item.Names...)
	values = append(values, item.Aliases...)
	reqKey := normalizePantryIngredientKey(req.Name)
	searchKey := normalizePantryIngredientKey(req.SearchTerm)
	for _, value := range values {
		key := normalizePantryIngredientKey(value)
		if key != "" && (key == req.IngredientKey || key == reqKey || key == searchKey) {
			return true
		}
	}
	return false
}

func pantryIngredientKey(ingredient Ingredient) string {
	return normalizePantryIngredientKey(firstNonEmptyString(ingredient.SearchTerm, ingredient.Name))
}

func normalizePantryIngredientKey(value string) string {
	key := normalizeKey(value)
	switch key {
	case "sal", "sal fina", "sal marina":
		return "salt"
	case "pimienta", "pimienta negra":
		return "black_pepper"
	case "agua":
		return "water"
	case "aceite de oliva", "aceite de oliva virgen extra", "aove":
		return "olive_oil"
	case "vinagre", "vinagre de vino":
		return "vinegar"
	case "azucar":
		return "sugar"
	case "harina":
		return "flour"
	case "oregano":
		return "oregano"
	case "pimenton":
		return "paprika"
	case "comino":
		return "cumin"
	default:
		return strings.ReplaceAll(key, " ", "_")
	}
}

func normalizeDefaultPantry(value string) string {
	switch strings.ReplaceAll(normalizeKey(value), " ", "_") {
	case "", DefaultPantryNone:
		return DefaultPantryNone
	case "minimal_spanish", "minimal":
		return DefaultPantryMinimalSpanish
	case "mediterranean_basic", "mediterranean":
		return DefaultPantryMediterraneanBasic
	default:
		return DefaultPantryNone
	}
}

func pantrySourcingStatus(line PantryResolutionLine) string {
	switch line.PantryDecision {
	case PantryDecisionPantryPartialConfirmed:
		return "pantry_partial_shop_delta"
	case PantryDecisionBuyDelta:
		return "shop_delta"
	case PantryDecisionPantryFullConfirmed, PantryDecisionPantryFullAssumed, PantryDecisionOptionalSkipped:
		return line.PantryDecision
	default:
		return "shop_full"
	}
}

func pantryItemExpired(item PantryProfileItem) bool {
	if strings.TrimSpace(item.ExpiresAt) == "" {
		return false
	}
	expiry, err := time.Parse("2006-01-02", item.ExpiresAt)
	if err != nil {
		return false
	}
	today, _ := time.Parse("2006-01-02", time.Now().Format("2006-01-02"))
	return expiry.Before(today)
}

func isDefaultPantryKey(key string) bool {
	switch normalizePantryIngredientKey(key) {
	case "water", "salt", "black_pepper", "olive_oil", "vinegar", "sugar", "flour", "oregano", "paprika", "cumin":
		return true
	default:
		return false
	}
}

func positiveOrDefault(value, fallback float64) float64 {
	if value > 0 {
		return value
	}
	return fallback
}

func cloneNormalizedQuantity(q *NormalizedQuantity) *NormalizedQuantity {
	if q == nil {
		return nil
	}
	out := *q
	return &out
}

func mathMax(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func pantryProfileSummary(profile PantryProfile) PantryProfileSummary {
	summary := PantryProfileSummary{DefaultProfile: profile.DefaultProfile, ItemCount: len(profile.Items)}
	for _, item := range profile.Items {
		switch item.Status {
		case PantryItemStatusConfirmedAvailable:
			summary.ConfirmedItems++
		case PantryItemStatusAssumedAvailable:
			summary.AssumedItems++
		}
	}
	return summary
}

func pantryConsumptionPlan(resolution *PantryResolution) *PantryConsumptionPlan {
	if resolution == nil || resolution.Status == PantryResolutionNotUsed {
		return nil
	}
	var lines []PantryResolutionLine
	for _, line := range resolution.Lines {
		if line.PantryAllocated != nil && line.PantryAllocated.BaseValue > 0 {
			lines = append(lines, line)
		}
	}
	return &PantryConsumptionPlan{
		SchemaVersion:               "1",
		PantryResolutionFingerprint: resolution.PantryResolutionFingerprint,
		Lines:                       lines,
	}
}

func pantryResolutionByKey(resolution *PantryResolution) map[string]PantryResolutionLine {
	out := map[string]PantryResolutionLine{}
	if resolution == nil {
		return out
	}
	for _, line := range resolution.Lines {
		for _, key := range []string{line.IngredientKey, line.IngredientName, line.SearchTerm} {
			key = normalizePantryIngredientKey(key)
			if key == "" {
				continue
			}
			out[key] = line
		}
	}
	return out
}

func formatPantryQuantity(q *NormalizedQuantity) string {
	if q == nil {
		return ""
	}
	return fmt.Sprintf("%.3g %s", q.Value, q.Unit)
}
