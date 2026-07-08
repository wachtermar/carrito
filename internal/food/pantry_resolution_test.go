package food

import "testing"

func TestResolvePantryMinimalSpanishAssumptions(t *testing.T) {
	plan := pantryTestPlan([]Ingredient{
		{Name: "salt", Quantity: 2, Unit: "g", SearchTerm: "sal"},
		{Name: "black pepper", Quantity: 1, Unit: "g", SearchTerm: "pimienta negra"},
		{Name: "rice", Quantity: 400, Unit: "g", SearchTerm: "arroz"},
	})
	policy := DefaultPantryPolicy(true, DefaultPantryMinimalSpanish)
	policy.AllowAssumedPantry = true
	updated, resolution := ApplyPantryResolution(plan, PantryProfile{}, policy)
	if resolution.Status != PantryResolutionAppliedWithAssumptions {
		t.Fatalf("status = %s, want applied_with_assumptions: %+v", resolution.Status, resolution)
	}
	if resolution.Summary.PantryAssumedLines != 2 || resolution.Summary.ShopIngredientLines != 1 {
		t.Fatalf("unexpected summary: %+v", resolution.Summary)
	}
	if len(updated.RequiredPurchases) != 1 || updated.RequiredPurchases[0].SearchTerm != "arroz" {
		t.Fatalf("shopping delta should contain only rice: %+v", updated.RequiredPurchases)
	}
	if line := pantryLine(resolution, "salt"); line.PantryDecision != PantryDecisionPantryFullAssumed || line.ShopRequired != nil {
		t.Fatalf("salt line not assumed pantry: %+v", line)
	}
	if line := pantryLine(resolution, "arroz"); line.PantryDecision != PantryDecisionBuyFull || line.ShopRequired == nil {
		t.Fatalf("rice should not be minimal pantry: %+v", line)
	}
}

func TestResolvePantryConfirmedPartialCoverage(t *testing.T) {
	plan := pantryTestPlan([]Ingredient{{Name: "rice", Quantity: 400, Unit: "g", SearchTerm: "arroz"}})
	q := normalizedQuantity("150 g", 150, "g", 1, "test")
	profile := PantryProfile{Items: []PantryProfileItem{{
		IngredientKey: "rice",
		Names:         []string{"arroz", "rice"},
		Status:        PantryItemStatusConfirmedAvailable,
		Quantity:      &q,
		Confidence:    1,
		Source:        "user",
	}}}
	policy := DefaultPantryPolicy(true, DefaultPantryNone)
	updated, resolution := ApplyPantryResolution(plan, profile, policy)
	line := pantryLine(resolution, "rice")
	if line.PantryDecision != PantryDecisionPantryPartialConfirmed {
		t.Fatalf("decision = %s, want partial confirmed: %+v", line.PantryDecision, line)
	}
	if line.PantryAllocated == nil || line.PantryAllocated.BaseValue != 150 || line.ShopRequired == nil || line.ShopRequired.BaseValue != 250 {
		t.Fatalf("unexpected quantities: %+v", line)
	}
	if len(updated.RequiredPurchases) != 1 || updated.RequiredPurchases[0].Quantity != 250 {
		t.Fatalf("shopping delta = %+v, want 250g rice", updated.RequiredPurchases)
	}
}

func TestMergePantryMemoryIntoProfileAppliesSavedPantryDelta(t *testing.T) {
	plan := pantryTestPlan([]Ingredient{{Name: "rice", Quantity: 400, Unit: "g", SearchTerm: "arroz"}})
	profile := MergePantryMemoryIntoProfile(PantryProfile{}, Pantry{Items: []PantryItem{{
		Name:       "rice",
		Quantity:   150,
		Unit:       "g",
		Location:   "pantry",
		Confidence: 0.9,
	}}})
	policy := DefaultPantryPolicy(true, DefaultPantryMinimalSpanish)
	policy.AllowAssumedPantry = true
	updated, resolution := ApplyPantryResolution(plan, profile, policy)
	line := pantryLine(resolution, "rice")
	if line.PantryDecision != PantryDecisionPantryPartialConfirmed {
		t.Fatalf("decision = %s, want partial confirmed: %+v", line.PantryDecision, line)
	}
	if line.PantryAllocated == nil || line.PantryAllocated.BaseValue != 150 || line.ShopRequired == nil || line.ShopRequired.BaseValue != 250 {
		t.Fatalf("unexpected saved pantry delta: %+v", line)
	}
	if len(updated.RequiredPurchases) != 1 || updated.RequiredPurchases[0].Quantity != 250 {
		t.Fatalf("shopping delta = %+v, want 250g rice", updated.RequiredPurchases)
	}
}

func TestResolvePantryRequireConfirmedBlocksAssumptions(t *testing.T) {
	plan := pantryTestPlan([]Ingredient{{Name: "salt", Quantity: 2, Unit: "g", SearchTerm: "sal"}})
	policy := DefaultPantryPolicy(true, DefaultPantryMinimalSpanish)
	policy.RequireConfirmedPantry = true
	policy.AllowAssumedPantry = false
	updated, resolution := ApplyPantryResolution(plan, PantryProfile{}, policy)
	if resolution.Status != PantryResolutionBlocked || len(resolution.BlockingIssues) == 0 {
		t.Fatalf("resolution should block assumptions: %+v", resolution)
	}
	if len(updated.RequiredPurchases) != 0 {
		t.Fatalf("blocked pantry assumption should not create an actionable shopping line: %+v", updated.RequiredPurchases)
	}
}

func TestShopRequirementsFingerprintChangesWithPantryQuantity(t *testing.T) {
	plan := pantryTestPlan([]Ingredient{{Name: "rice", Quantity: 400, Unit: "g", SearchTerm: "arroz"}})
	policy := DefaultPantryPolicy(true, DefaultPantryNone)
	q150 := normalizedQuantity("150 g", 150, "g", 1, "test")
	q200 := normalizedQuantity("200 g", 200, "g", 1, "test")
	profile := PantryProfile{Items: []PantryProfileItem{{IngredientKey: "rice", Names: []string{"arroz"}, Status: PantryItemStatusConfirmedAvailable, Quantity: &q150}}}
	_, first := ApplyPantryResolution(plan, profile, policy)
	profile.Items[0].Quantity = &q200
	_, second := ApplyPantryResolution(plan, profile, policy)
	if first.ShopRequirementsFingerprint == "" || first.ShopRequirementsFingerprint == second.ShopRequirementsFingerprint {
		t.Fatalf("shop requirements fingerprint should change: first=%s second=%s", first.ShopRequirementsFingerprint, second.ShopRequirementsFingerprint)
	}
}

func pantryTestPlan(ingredients []Ingredient) MealPlan {
	return MealPlan{
		ID:     "pantry-test",
		People: 2,
		Days: []DayPlan{{Day: 1, Meals: []Meal{{Type: "dinner", Recipe: Recipe{
			ID:          "pantry-recipe",
			Title:       "Pantry Recipe",
			Servings:    2,
			Ingredients: ingredients,
			Steps:       []RecipeStep{{Number: 1, Text: "Cook."}},
		}}}}},
		RequiredPurchases: ingredients,
	}
}

func pantryLine(resolution PantryResolution, key string) PantryResolutionLine {
	key = normalizePantryIngredientKey(key)
	for _, line := range resolution.Lines {
		if line.IngredientKey == key || normalizePantryIngredientKey(line.IngredientName) == key || normalizePantryIngredientKey(line.SearchTerm) == key {
			return line
		}
	}
	return PantryResolutionLine{}
}
