package food

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

func LoadHouseholdProfile(path string) (HouseholdProfile, error) {
	if strings.TrimSpace(path) == "" {
		return HouseholdProfile{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return HouseholdProfile{}, err
	}
	var profile HouseholdProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return HouseholdProfile{}, err
	}
	if profile.SchemaVersion == "" {
		profile.SchemaVersion = "1"
	}
	return profile, nil
}

func DefaultServingPolicy(enabled bool) ServingPolicy {
	return ServingPolicy{
		Enabled:                               enabled,
		PreserveRecipeServingsWhenUnspecified: true,
		AllowFractionalServings:               true,
		AllowLeftovers:                        false,
		RoundPieceIngredients:                 true,
	}
}

func ScalingPolicyFromServingPolicy(policy ServingPolicy) ScalingPolicy {
	return ScalingPolicy{
		StrictServingScaling:        policy.StrictServingScaling,
		AllowFractionalServings:     policy.AllowFractionalServings,
		RoundPieceIngredients:       policy.RoundPieceIngredients,
		PreserveToTasteQuantities:   true,
		PreserveOptionalIngredients: true,
		RequireBaseRecipeServings:   policy.RequireBaseRecipeServings,
	}
}

func ApplyServingScaling(plan MealPlan, household HouseholdProfile, policy ServingPolicy) (MealPlan, ServingPlan, ScaledMealPlan) {
	servingPlan := BuildServingPlan(plan, household, policy)
	scaled := ScaleMealPlan(plan, servingPlan, ScalingPolicyFromServingPolicy(policy))
	if !policy.Enabled {
		return plan, servingPlan, scaled
	}
	scaledPlan := applyScaledMealPlanToMealPlan(plan, scaled)
	scaledPlan.RequiredPurchases = mergeIngredients(requiredPurchasesFromScaledPlan(scaledPlan))
	summary := SummarizePlan(scaledPlan)
	if nutritionIsZero(summary) {
		scaledPlan.Nutrition = nil
	} else {
		scaledPlan.Nutrition = &summary
	}
	return scaledPlan, servingPlan, scaled
}

func BuildServingPlan(plan MealPlan, household HouseholdProfile, policy ServingPolicy) ServingPlan {
	if !policy.Enabled {
		out := ServingPlan{
			SchemaVersion:       "1",
			Status:              ServingPlanNotUsed,
			Policy:              policy,
			MealPlanFingerprint: MealPlanFingerprint(plan),
		}
		out.ServingPlanFingerprint = ServingPlanFingerprint(out)
		return out
	}
	if policy.StrictServingScaling {
		policy.RequireBaseRecipeServings = true
	}
	out := ServingPlan{
		SchemaVersion:               "1",
		Status:                      ServingPlanRecipeDefault,
		Policy:                      policy,
		MealPlanFingerprint:         MealPlanFingerprint(plan),
		HouseholdProfileFingerprint: HouseholdProfileFingerprint(household),
	}
	target, participants, status := servingTargetFromInputs(household, policy)
	if status != "" {
		out.Status = status
	}
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			slotTarget := target
			slotParticipants := participants
			if len(household.Members) > 0 {
				slotTarget, slotParticipants = servingTargetFromHouseholdSlot(household, day.Day, meal.Type)
			}
			slot := ServingSlot{
				Day:                day.Day,
				MealSlot:           meal.Type,
				RecipeID:           meal.Recipe.ID,
				RecipeTitle:        meal.Recipe.Title,
				BaseRecipeServings: mealBaseServings(meal.Recipe, plan),
				Participants:       append([]ServingParticipant(nil), slotParticipants...),
				ScalingStatus:      ScalingStatusExact,
			}
			if slotTarget <= 0 {
				slotTarget = slot.BaseRecipeServings
				slot.ScalingStatus = ScalingStatusRecipeDefault
			}
			if slotTarget <= 0 {
				slotTarget = 1
				slot.ScalingStatus = ScalingStatusNeedsReview
				slot.Warnings = append(slot.Warnings, "Recipe servings are missing; using one serving for scaling.")
				out.Warnings = append(out.Warnings, ServingIssue{
					Code:        "base_servings_missing_preserved",
					Severity:    "warning",
					Day:         day.Day,
					MealSlot:    meal.Type,
					RecipeID:    meal.Recipe.ID,
					RecipeTitle: meal.Recipe.Title,
					Message:     "Recipe servings are missing; scaling preserved a one-serving fallback.",
				})
			}
			if slot.BaseRecipeServings <= 0 && policy.RequireBaseRecipeServings {
				slot.ScalingStatus = ScalingStatusBlocked
				out.Status = ServingPlanBlocked
				out.BlockingIssues = append(out.BlockingIssues, ServingIssue{
					Code:        "base_servings_missing",
					Severity:    "blocking",
					Day:         day.Day,
					MealSlot:    meal.Type,
					RecipeID:    meal.Recipe.ID,
					RecipeTitle: meal.Recipe.Title,
					Message:     "Recipe base servings are missing and strict serving scaling is enabled.",
					Remediation: "Add recipe servings, disable --strict-servings, or run with --no-serving-scaling.",
				})
			}
			slot.TargetServingUnits = roundQty(slotTarget)
			slot.CookedServingUnits = slot.TargetServingUnits
			if policy.AllowLeftovers && policy.LeftoverServings > 0 {
				slot.LeftoverPlan = &LeftoverPlan{Enabled: true, ExtraServingUnits: roundQty(policy.LeftoverServings), IntendedUse: "unspecified"}
				slot.CookedServingUnits = roundQty(slot.CookedServingUnits + policy.LeftoverServings)
			}
			base := slot.BaseRecipeServings
			if base <= 0 {
				base = slotTarget
			}
			if base <= 0 {
				base = 1
			}
			slot.ScaleFactor = roundQty(slot.CookedServingUnits / base)
			if slot.ScaleFactor != 1 && slot.ScalingStatus == ScalingStatusExact {
				out.Status = firstServingStatus(out.Status, status)
			}
			out.Slots = append(out.Slots, slot)
		}
	}
	out.Summary = summarizeServingSlots(out.Slots)
	if len(out.BlockingIssues) > 0 {
		out.Status = ServingPlanBlocked
	} else if out.Status == "" {
		out.Status = ServingPlanRecipeDefault
	}
	out.ServingPlanFingerprint = ServingPlanFingerprint(out)
	return out
}

func ScaleMealPlan(plan MealPlan, servingPlan ServingPlan, policy ScalingPolicy) ScaledMealPlan {
	out := ScaledMealPlan{
		SchemaVersion:          "1",
		MealPlanFingerprint:    MealPlanFingerprint(plan),
		ServingPlanFingerprint: servingPlan.ServingPlanFingerprint,
	}
	slots := servingSlotsByKey(servingPlan)
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			slot := slots[servingSlotKey(day.Day, meal.Type, meal.Recipe.ID)]
			if slot.RecipeID == "" {
				slot = ServingSlot{
					Day:                day.Day,
					MealSlot:           meal.Type,
					RecipeID:           meal.Recipe.ID,
					RecipeTitle:        meal.Recipe.Title,
					BaseRecipeServings: mealBaseServings(meal.Recipe, plan),
					TargetServingUnits: mealBaseServings(meal.Recipe, plan),
					CookedServingUnits: mealBaseServings(meal.Recipe, plan),
					ScaleFactor:        1,
					ScalingStatus:      ScalingStatusRecipeDefault,
				}
			}
			scaledSlot := ScaledRecipeSlot{
				Day:                day.Day,
				MealSlot:           meal.Type,
				RecipeID:           meal.Recipe.ID,
				RecipeTitle:        meal.Recipe.Title,
				BaseServings:       slot.BaseRecipeServings,
				TargetServingUnits: slot.TargetServingUnits,
				CookedServingUnits: slot.CookedServingUnits,
				ScaleFactor:        positiveOrDefault(slot.ScaleFactor, 1),
			}
			for _, ingredient := range meal.Recipe.Ingredients {
				scaledIngredient, warning := scaleIngredientForServing(day.Day, meal.Type, meal.Recipe, ingredient, scaledSlot.TargetServingUnits, scaledSlot.CookedServingUnits, scaledSlot.ScaleFactor, policy)
				if warning.Code != "" {
					out.Warnings = append(out.Warnings, warning)
					scaledSlot.ScalingNotes = append(scaledSlot.ScalingNotes, warning.Message)
				}
				scaledSlot.Ingredients = append(scaledSlot.Ingredients, scaledIngredient)
			}
			out.Slots = append(out.Slots, scaledSlot)
		}
	}
	out.Summary = summarizeScaledSlots(out.Slots)
	out.ScaledMealPlanFingerprint = ScaledMealPlanFingerprint(out)
	return out
}

func HouseholdProfileFingerprint(profile HouseholdProfile) string {
	profile.Members = append([]HouseholdMember(nil), profile.Members...)
	sort.SliceStable(profile.Members, func(i, j int) bool { return profile.Members[i].ID < profile.Members[j].ID })
	data, err := json.Marshal(profile)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func ServingPlanFingerprint(plan ServingPlan) string {
	type canonical struct {
		Status                      ServingPlanStatus `json:"status"`
		Policy                      ServingPolicy     `json:"policy"`
		MealPlanFingerprint         string            `json:"mealplan_fingerprint,omitempty"`
		HouseholdProfileFingerprint string            `json:"household_profile_fingerprint,omitempty"`
		Slots                       []ServingSlot     `json:"slots,omitempty"`
	}
	rows := append([]ServingSlot(nil), plan.Slots...)
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Day != rows[j].Day {
			return rows[i].Day < rows[j].Day
		}
		if rows[i].MealSlot != rows[j].MealSlot {
			return rows[i].MealSlot < rows[j].MealSlot
		}
		return rows[i].RecipeID < rows[j].RecipeID
	})
	data, err := json.Marshal(canonical{Status: plan.Status, Policy: plan.Policy, MealPlanFingerprint: plan.MealPlanFingerprint, HouseholdProfileFingerprint: plan.HouseholdProfileFingerprint, Slots: rows})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func ScaledMealPlanFingerprint(plan ScaledMealPlan) string {
	type canonical struct {
		MealPlanFingerprint    string             `json:"mealplan_fingerprint,omitempty"`
		ServingPlanFingerprint string             `json:"serving_plan_fingerprint,omitempty"`
		Slots                  []ScaledRecipeSlot `json:"slots,omitempty"`
	}
	rows := append([]ScaledRecipeSlot(nil), plan.Slots...)
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Day != rows[j].Day {
			return rows[i].Day < rows[j].Day
		}
		if rows[i].MealSlot != rows[j].MealSlot {
			return rows[i].MealSlot < rows[j].MealSlot
		}
		return rows[i].RecipeID < rows[j].RecipeID
	})
	data, err := json.Marshal(canonical{MealPlanFingerprint: plan.MealPlanFingerprint, ServingPlanFingerprint: plan.ServingPlanFingerprint, Slots: rows})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func AttachServingScaling(artifact FoodRunArtifact, household HouseholdProfile, servingPlan ServingPlan, scaled ScaledMealPlan) FoodRunArtifact {
	artifact.HouseholdProfileFingerprint = servingPlan.HouseholdProfileFingerprint
	if artifact.HouseholdProfileFingerprint == "" {
		artifact.HouseholdProfileFingerprint = HouseholdProfileFingerprint(household)
	}
	artifact.ServingPlanFingerprint = servingPlan.ServingPlanFingerprint
	artifact.ScaledMealPlanFingerprint = scaled.ScaledMealPlanFingerprint
	summary := householdProfileSummary(household)
	artifact.HouseholdProfileSummary = &summary
	artifact.ServingPlan = &servingPlan
	artifact.ScaledMealPlan = &scaled
	return artifact
}

func servingTargetFromInputs(profile HouseholdProfile, policy ServingPolicy) (float64, []ServingParticipant, ServingPlanStatus) {
	if len(profile.Members) > 0 {
		total, participants := servingTargetForMembers(profile.Members)
		return total, participants, ServingPlanHouseholdProfile
	}
	if policy.AdultServings > 0 || policy.ChildServings > 0 || policy.ToddlerServings > 0 {
		var participants []ServingParticipant
		if policy.AdultServings > 0 {
			participants = append(participants, ServingParticipant{MemberID: "adult_servings", ServingFactor: policy.AdultServings, PortionType: "adult"})
		}
		if policy.ChildServings > 0 {
			participants = append(participants, ServingParticipant{MemberID: "child_servings", ServingFactor: policy.ChildServings, PortionType: "child"})
		}
		if policy.ToddlerServings > 0 {
			participants = append(participants, ServingParticipant{MemberID: "toddler_servings", ServingFactor: policy.ToddlerServings, PortionType: "toddler"})
		}
		return roundQty(policy.AdultServings + policy.ChildServings + policy.ToddlerServings), participants, ServingPlanExplicitServings
	}
	if policy.DefaultServings > 0 {
		return roundQty(policy.DefaultServings), nil, ServingPlanExplicitServings
	}
	return 0, nil, ServingPlanRecipeDefault
}

func servingTargetFromHouseholdSlot(profile HouseholdProfile, day int, mealSlot string) (float64, []ServingParticipant) {
	members := profile.Members
	for _, rule := range profile.DefaultMealParticipation {
		if mealParticipationRuleMatches(rule, day, mealSlot) {
			members = householdMembersByID(profile.Members, rule.MemberIDs)
			break
		}
	}
	return servingTargetForMembers(members)
}

func servingTargetForMembers(members []HouseholdMember) (float64, []ServingParticipant) {
	var total float64
	var participants []ServingParticipant
	for _, member := range members {
		factor := positiveOrDefault(member.DefaultServingFactor, defaultServingFactor(member.Type))
		total += factor
		participants = append(participants, ServingParticipant{
			MemberID:      member.ID,
			DisplayName:   member.DisplayName,
			ServingFactor: factor,
			PortionType:   member.Type,
		})
	}
	return roundQty(total), participants
}

func householdMembersByID(members []HouseholdMember, ids []string) []HouseholdMember {
	if len(ids) == 0 {
		return nil
	}
	byID := make(map[string]HouseholdMember, len(members))
	for _, member := range members {
		byID[member.ID] = member
	}
	out := make([]HouseholdMember, 0, len(ids))
	for _, id := range ids {
		if member, ok := byID[id]; ok {
			out = append(out, member)
		}
	}
	return out
}

func mealParticipationRuleMatches(rule MealParticipationRule, day int, mealSlot string) bool {
	if rule.MealSlot != "" && rule.MealSlot != "*" && normalizeKey(rule.MealSlot) != normalizeKey(mealSlot) {
		return false
	}
	if len(rule.Days) > 0 {
		for _, value := range rule.Days {
			if value == day {
				return true
			}
		}
		return false
	}
	if rule.FromDay > 0 && day < rule.FromDay {
		return false
	}
	if rule.ToDay > 0 && day > rule.ToDay {
		return false
	}
	return true
}

func defaultServingFactor(memberType string) float64 {
	switch normalizeKey(memberType) {
	case "child":
		return 0.6
	case "toddler":
		return 0.5
	case "infant":
		return 0.25
	default:
		return 1
	}
}

func mealBaseServings(recipe Recipe, plan MealPlan) float64 {
	if recipe.Servings > 0 {
		return float64(recipe.Servings)
	}
	if plan.People > 0 {
		return float64(plan.People)
	}
	return 0
}

func firstServingStatus(current ServingPlanStatus, fallback ServingPlanStatus) ServingPlanStatus {
	if fallback != "" && fallback != ServingPlanRecipeDefault {
		return fallback
	}
	if current != "" {
		return current
	}
	return ServingPlanExplicitServings
}

func servingSlotsByKey(plan ServingPlan) map[string]ServingSlot {
	out := map[string]ServingSlot{}
	for _, slot := range plan.Slots {
		out[servingSlotKey(slot.Day, slot.MealSlot, slot.RecipeID)] = slot
	}
	return out
}

func servingSlotKey(day int, mealSlot, recipeID string) string {
	return strconv.Itoa(day) + "|" + strings.TrimSpace(mealSlot) + "|" + strings.TrimSpace(recipeID)
}

func summarizeServingSlots(slots []ServingSlot) ServingPlanSummary {
	summary := ServingPlanSummary{TotalMealSlots: len(slots)}
	for _, slot := range slots {
		summary.TotalTargetServingUnits = roundQty(summary.TotalTargetServingUnits + slot.TargetServingUnits)
		summary.TotalCookedServingUnits = roundQty(summary.TotalCookedServingUnits + slot.CookedServingUnits)
		switch slot.ScalingStatus {
		case ScalingStatusRecipeDefault:
			summary.SlotsUsingRecipeDefault++
		case ScalingStatusBlocked:
			summary.BlockedSlots++
		case ScalingStatusNeedsReview:
			summary.SlotsNeedingReview++
		default:
			summary.SlotsScaled++
		}
		if slot.LeftoverPlan != nil && slot.LeftoverPlan.Enabled {
			summary.SlotsWithLeftovers++
		}
	}
	return summary
}

func scaleIngredientForServing(day int, mealSlot string, recipe Recipe, ingredient Ingredient, targetServingUnits, cookedServingUnits, factor float64, policy ScalingPolicy) (ScaledIngredient, ScalingWarning) {
	factor = positiveOrDefault(factor, 1)
	key := pantryIngredientKey(ingredient)
	original, ok := normalizedIngredientQuantity(ingredient.Quantity, ingredient.Unit, 0.95, "serving_original")
	if !ok {
		original = NormalizedQuantity{Raw: formatIngredientQuantity(ingredient.Quantity, ingredient.Unit), Value: ingredient.Quantity, Unit: normalizeUnit(ingredient.Unit), BaseValue: ingredient.Quantity, BaseUnit: normalizeUnit(ingredient.Unit), Confidence: 0.2, ParseMethod: "serving_unparseable"}
	}
	scaledBase := roundQty(original.BaseValue * factor)
	scaled := quantityFromBase(scaledBase, original.BaseUnit, original.Confidence, "serving_scaled")
	if original.BaseUnit == "" {
		scaled = NormalizedQuantity{Raw: formatIngredientQuantity(roundQty(ingredient.Quantity*factor), ingredient.Unit), Value: roundQty(ingredient.Quantity * factor), Unit: normalizeUnit(ingredient.Unit), BaseValue: roundQty(ingredient.Quantity * factor), BaseUnit: normalizeUnit(ingredient.Unit), Confidence: original.Confidence, ParseMethod: "serving_scaled_unparseable"}
	}
	status := ScaledQuantityExact
	shopping := scaled
	cooking := scaled
	var warning ScalingWarning
	if ingredient.Optional {
		status = ScaledQuantityOptional
	}
	if ingredient.Quantity <= 0 || isToTasteIngredient(ingredient) {
		status = ScaledQuantityToTaste
	}
	if !ok && status == ScaledQuantityExact {
		status = ScaledQuantityUnparseable
		warning = ScalingWarning{Code: "ingredient_quantity_unparseable", Severity: "warning", IngredientKey: key, IngredientName: ingredient.Name, Day: day, MealSlot: mealSlot, Message: "Ingredient quantity could not be parsed safely for serving scaling."}
		if policy.StrictServingScaling {
			status = ScaledQuantityBlocked
			warning.Severity = "blocking"
		}
	}
	if status == ScaledQuantityExact && policy.RoundPieceIngredients && original.BaseUnit == "unit" && !isWholeNumber(scaled.BaseValue) {
		rounded := math.Ceil(scaled.BaseValue)
		shopping = quantityFromBase(rounded, "unit", original.Confidence, "serving_piece_rounded")
		status = ScaledQuantityRoundedPiece
		warning = ScalingWarning{Code: "piece_ingredient_rounded", Severity: "warning", IngredientKey: key, IngredientName: ingredient.Name, Day: day, MealSlot: mealSlot, Message: ingredient.Name + " was rounded up for shopping while keeping the cooking amount approximate."}
	}
	usage := IngredientUsageRef{
		Day:                day,
		MealSlot:           mealSlot,
		RecipeID:           recipe.ID,
		RecipeTitle:        recipe.Title,
		BaseServings:       float64(recipe.Servings),
		TargetServingUnits: targetServingUnits,
		CookedServingUnits: cookedServingUnits,
		ScaleFactor:        factor,
		IngredientKey:      key,
		IngredientName:     ingredient.Name,
		RequiredQuantity:   &shopping,
	}
	return ScaledIngredient{
		IngredientKey:    key,
		IngredientName:   ingredient.Name,
		SearchTerm:       ingredient.SearchTerm,
		Category:         ingredient.Category,
		OriginalQuantity: &original,
		ScaledQuantity:   &scaled,
		ShoppingQuantity: &shopping,
		CookingQuantity:  &cooking,
		QuantityStatus:   status,
		ScalingFactor:    factor,
		RoundingApplied:  status == ScaledQuantityRoundedPiece,
		RoundingReason:   warning.Message,
		Optional:         ingredient.Optional,
		PantryCandidate:  isDefaultPantryKey(key),
		Usages:           []IngredientUsageRef{usage},
	}, warning
}

func summarizeScaledSlots(slots []ScaledRecipeSlot) ScaledMealPlanSummary {
	var summary ScaledMealPlanSummary
	for _, slot := range slots {
		for _, ingredient := range slot.Ingredients {
			summary.IngredientLines++
			switch ingredient.QuantityStatus {
			case ScaledQuantityRoundedPiece:
				summary.RoundedPieceLines++
			case ScaledQuantityToTaste:
				summary.ToTasteLines++
			case ScaledQuantityOptional:
				summary.OptionalLines++
			case ScaledQuantityUnparseable:
				summary.UnparseableLines++
			case ScaledQuantityBlocked:
				summary.BlockedLines++
			default:
				summary.ExactLines++
			}
		}
	}
	return summary
}

func cloneMealPlan(plan MealPlan) MealPlan {
	plan.Days = append([]DayPlan(nil), plan.Days...)
	for dayIdx := range plan.Days {
		plan.Days[dayIdx].Meals = append([]Meal(nil), plan.Days[dayIdx].Meals...)
		for mealIdx := range plan.Days[dayIdx].Meals {
			plan.Days[dayIdx].Meals[mealIdx].Recipe = cloneRecipe(plan.Days[dayIdx].Meals[mealIdx].Recipe)
		}
	}
	plan.PantryUsage = append([]PantryUsage(nil), plan.PantryUsage...)
	plan.RequiredPurchases = append([]Ingredient(nil), plan.RequiredPurchases...)
	plan.MissingItems = append([]string(nil), plan.MissingItems...)
	plan.Notes = append([]string(nil), plan.Notes...)
	if plan.Nutrition != nil {
		nutrition := *plan.Nutrition
		plan.Nutrition = &nutrition
	}
	return plan
}

func cloneRecipe(recipe Recipe) Recipe {
	recipe.Tags = append([]string(nil), recipe.Tags...)
	recipe.Ingredients = append([]Ingredient(nil), recipe.Ingredients...)
	recipe.Equipment = append([]string(nil), recipe.Equipment...)
	recipe.Steps = append([]RecipeStep(nil), recipe.Steps...)
	recipe.Substitutions = append([]string(nil), recipe.Substitutions...)
	recipe.AllergenNotes = append([]string(nil), recipe.AllergenNotes...)
	if recipe.NutritionPerServing != nil {
		nutrition := *recipe.NutritionPerServing
		recipe.NutritionPerServing = &nutrition
	}
	return recipe
}

func cloneServingPlan(plan ServingPlan) ServingPlan {
	plan.Slots = append([]ServingSlot(nil), plan.Slots...)
	for i := range plan.Slots {
		plan.Slots[i].Participants = append([]ServingParticipant(nil), plan.Slots[i].Participants...)
		plan.Slots[i].Warnings = append([]string(nil), plan.Slots[i].Warnings...)
		if plan.Slots[i].LeftoverPlan != nil {
			leftover := *plan.Slots[i].LeftoverPlan
			leftover.ConsumedBy = append([]string(nil), leftover.ConsumedBy...)
			plan.Slots[i].LeftoverPlan = &leftover
		}
	}
	plan.BlockingIssues = append([]ServingIssue(nil), plan.BlockingIssues...)
	plan.Warnings = append([]ServingIssue(nil), plan.Warnings...)
	return plan
}

func cloneScaledMealPlan(plan ScaledMealPlan) ScaledMealPlan {
	plan.Slots = append([]ScaledRecipeSlot(nil), plan.Slots...)
	for slotIdx := range plan.Slots {
		plan.Slots[slotIdx].Ingredients = append([]ScaledIngredient(nil), plan.Slots[slotIdx].Ingredients...)
		plan.Slots[slotIdx].ScalingNotes = append([]string(nil), plan.Slots[slotIdx].ScalingNotes...)
		for ingredientIdx := range plan.Slots[slotIdx].Ingredients {
			ingredient := &plan.Slots[slotIdx].Ingredients[ingredientIdx]
			if ingredient.OriginalQuantity != nil {
				q := *ingredient.OriginalQuantity
				ingredient.OriginalQuantity = &q
			}
			if ingredient.ScaledQuantity != nil {
				q := *ingredient.ScaledQuantity
				ingredient.ScaledQuantity = &q
			}
			if ingredient.ShoppingQuantity != nil {
				q := *ingredient.ShoppingQuantity
				ingredient.ShoppingQuantity = &q
			}
			if ingredient.CookingQuantity != nil {
				q := *ingredient.CookingQuantity
				ingredient.CookingQuantity = &q
			}
			ingredient.Usages = append([]IngredientUsageRef(nil), ingredient.Usages...)
		}
	}
	plan.Warnings = append([]ScalingWarning(nil), plan.Warnings...)
	return plan
}

func applyScaledMealPlanToMealPlan(plan MealPlan, scaled ScaledMealPlan) MealPlan {
	plan = cloneMealPlan(plan)
	scaledBySlot := map[string]ScaledRecipeSlot{}
	for _, slot := range scaled.Slots {
		scaledBySlot[servingSlotKey(slot.Day, slot.MealSlot, slot.RecipeID)] = slot
	}
	for dayIdx := range plan.Days {
		for mealIdx := range plan.Days[dayIdx].Meals {
			meal := &plan.Days[dayIdx].Meals[mealIdx]
			slot := scaledBySlot[servingSlotKey(plan.Days[dayIdx].Day, meal.Type, meal.Recipe.ID)]
			if len(slot.Ingredients) == 0 {
				continue
			}
			meal.Recipe.Servings = int(math.Ceil(positiveOrDefault(slot.CookedServingUnits, float64(meal.Recipe.Servings))))
			for i := range meal.Recipe.Ingredients {
				if i >= len(slot.Ingredients) || slot.Ingredients[i].ShoppingQuantity == nil {
					continue
				}
				q := slot.Ingredients[i].ShoppingQuantity
				meal.Recipe.Ingredients[i].Quantity = q.Value
				meal.Recipe.Ingredients[i].Unit = q.Unit
			}
		}
	}
	return plan
}

func ReplaceScaledRecipeSlot(plan MealPlan, servingPlan ServingPlan, scaled ScaledMealPlan, day int, mealSlot string, replacement Recipe, policy ScalingPolicy) (MealPlan, ServingPlan, ScaledMealPlan) {
	plan = cloneMealPlan(plan)
	servingPlan = cloneServingPlan(servingPlan)
	scaled = cloneScaledMealPlan(scaled)
	for i := range servingPlan.Slots {
		slot := &servingPlan.Slots[i]
		if slot.Day != day || strings.TrimSpace(slot.MealSlot) != strings.TrimSpace(mealSlot) {
			continue
		}
		slot.RecipeID = replacement.ID
		slot.RecipeTitle = replacement.Title
		slot.BaseRecipeServings = mealBaseServings(replacement, plan)
		base := slot.BaseRecipeServings
		if base <= 0 {
			base = slot.TargetServingUnits
		}
		if base <= 0 {
			base = 1
		}
		slot.ScaleFactor = roundQty(positiveOrDefault(slot.CookedServingUnits, base) / base)
		if slot.BaseRecipeServings <= 0 && policy.RequireBaseRecipeServings {
			slot.ScalingStatus = ScalingStatusBlocked
		} else if slot.ScaleFactor == 1 {
			slot.ScalingStatus = ScalingStatusRecipeDefault
		} else {
			slot.ScalingStatus = ScalingStatusExact
		}
		break
	}
	servingPlan.Summary = summarizeServingSlots(servingPlan.Slots)
	servingPlan.ServingPlanFingerprint = ServingPlanFingerprint(servingPlan)
	var replacementServingSlot ServingSlot
	for _, slot := range servingPlan.Slots {
		if slot.Day == day && strings.TrimSpace(slot.MealSlot) == strings.TrimSpace(mealSlot) {
			replacementServingSlot = slot
			break
		}
	}
	scaledSlot := ScaledRecipeSlot{
		Day:                day,
		MealSlot:           mealSlot,
		RecipeID:           replacement.ID,
		RecipeTitle:        replacement.Title,
		BaseServings:       replacementServingSlot.BaseRecipeServings,
		TargetServingUnits: replacementServingSlot.TargetServingUnits,
		CookedServingUnits: replacementServingSlot.CookedServingUnits,
		ScaleFactor:        positiveOrDefault(replacementServingSlot.ScaleFactor, 1),
	}
	for _, ingredient := range replacement.Ingredients {
		scaledIngredient, warning := scaleIngredientForServing(day, mealSlot, replacement, ingredient, scaledSlot.TargetServingUnits, scaledSlot.CookedServingUnits, scaledSlot.ScaleFactor, policy)
		if warning.Code != "" {
			scaled.Warnings = append(scaled.Warnings, warning)
			scaledSlot.ScalingNotes = append(scaledSlot.ScalingNotes, warning.Message)
		}
		scaledSlot.Ingredients = append(scaledSlot.Ingredients, scaledIngredient)
	}
	replaced := false
	for i := range scaled.Slots {
		if scaled.Slots[i].Day == day && strings.TrimSpace(scaled.Slots[i].MealSlot) == strings.TrimSpace(mealSlot) {
			scaled.Slots[i] = scaledSlot
			replaced = true
			break
		}
	}
	if !replaced {
		scaled.Slots = append(scaled.Slots, scaledSlot)
	}
	scaled.ServingPlanFingerprint = servingPlan.ServingPlanFingerprint
	scaled.Summary = summarizeScaledSlots(scaled.Slots)
	scaled.ScaledMealPlanFingerprint = ScaledMealPlanFingerprint(scaled)
	plan = applyScaledMealPlanToMealPlan(plan, scaled)
	plan.RequiredPurchases = mergeIngredients(requiredPurchasesFromScaledPlan(plan))
	return plan, servingPlan, scaled
}

func requiredPurchasesFromScaledPlan(plan MealPlan) []Ingredient {
	var ingredients []Ingredient
	for _, day := range plan.Days {
		for _, meal := range day.Meals {
			ingredients = append(ingredients, meal.Recipe.Ingredients...)
		}
	}
	return ingredients
}

func isToTasteIngredient(ingredient Ingredient) bool {
	text := normalizeKey(strings.Join([]string{ingredient.Name, ingredient.Unit, ingredient.SearchTerm}, " "))
	return strings.Contains(text, "to taste") || strings.Contains(text, "al gusto")
}

func isWholeNumber(value float64) bool {
	return math.Abs(value-math.Round(value)) < 0.0001
}

func householdProfileSummary(profile HouseholdProfile) HouseholdProfileSummary {
	summary := HouseholdProfileSummary{ProfileID: profile.ProfileID, MemberCount: len(profile.Members), ParticipationRules: len(profile.DefaultMealParticipation), AnonymousOnly: true}
	for _, member := range profile.Members {
		if strings.TrimSpace(member.DisplayName) != "" {
			summary.AnonymousOnly = false
		}
		factor := positiveOrDefault(member.DefaultServingFactor, defaultServingFactor(member.Type))
		summary.ServingUnitTotal = roundQty(summary.ServingUnitTotal + factor)
		switch normalizeKey(member.Type) {
		case "child":
			summary.ChildCount++
		case "toddler":
			summary.ToddlerCount++
		default:
			summary.AdultCount++
		}
	}
	return summary
}
