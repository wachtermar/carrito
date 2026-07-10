package mealplan

import (
	"strings"
	"testing"

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/money"
)

func TestValidatePersonaMatrix(t *testing.T) {
	tests := []struct {
		name        string
		people      int
		diet        []string
		allergies   []string
		description string
	}{
		{name: "solo vegetarian", people: 1, diet: []string{"vegetarian"}, allergies: []string{}},
		{name: "gluten free couple", people: 2, diet: []string{"gluten-free"}, allergies: []string{"gluten"}},
		{name: "family with toddler", people: 3, diet: []string{}, allergies: []string{}, description: "2 adults and 1 toddler"},
		{name: "large lactose free family", people: 5, diet: []string{"lactose-free"}, allergies: []string{"milk"}},
		{name: "vegan high protein", people: 2, diet: []string{"vegan", "high-protein"}, allergies: []string{}},
		{name: "pescatarian batch cooking", people: 6, diet: []string{"pescatarian"}, allergies: []string{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := validPlan()
			plan.Household.People = test.people
			plan.Household.DietaryRules = test.diet
			plan.Household.Allergies = test.allergies
			plan.Household.Description = test.description
			plan.Days[0].Meals[0].Servings = test.people
			if issues := ValidateSelected(plan); len(issues) != 0 {
				t.Fatalf("unexpected issues: %+v", issues)
			}
		})
	}
}

func TestValidateRequiresExplicitHardConstraints(t *testing.T) {
	plan := validPlan()
	plan.Household.Allergies = nil
	plan.Household.DietaryRules = nil
	plan.Household.Dislikes = nil
	plan.Assumptions = nil
	plan.Shopping = nil
	issues := ValidateDraft(plan)
	for _, path := range []string{"household.allergies", "household.dietary_rules", "household.dislikes", "assumptions", "shopping"} {
		if !hasIssue(issues, path) {
			t.Fatalf("missing issue for %s: %+v", path, issues)
		}
	}
}

func TestValidateDraftAllowsSelectionLater(t *testing.T) {
	plan := validPlan()
	plan.Shopping[0].ProductSKU = ""
	plan.Shopping[0].Packages = 0
	if issues := ValidateDraft(plan); len(issues) != 0 {
		t.Fatalf("draft issues: %+v", issues)
	}
	issues := ValidateSelected(plan)
	if !hasIssue(issues, "shopping[0].product_sku") || !hasIssue(issues, "shopping[0].packages") {
		t.Fatalf("selected validation did not require product selection: %+v", issues)
	}
}

func TestValidateSelectedRejectsPackageRangeAndPrecision(t *testing.T) {
	for _, packages := range []float64{10_001, 1.2345} {
		plan := validPlan()
		plan.Shopping[0].Packages = packages
		if issues := ValidateSelected(plan); !hasIssue(issues, "shopping[0].packages") {
			t.Fatalf("packages %v should fail: %+v", packages, issues)
		}
	}
}

func TestDecodeRejectsUnknownFields(t *testing.T) {
	raw := `{"title":"x","surprise":true}`
	if _, err := Decode(strings.NewReader(raw)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown-field error, got %v", err)
	}
}

func TestNewBuildBlocksAllergenMatch(t *testing.T) {
	plan := validPlan()
	plan.Household.Allergies = []string{"Severe peanut and tree-nut allergy is a hard constraint"}
	products := []alcampo.Product{{
		ID:          "product-1",
		SKU:         "sku-1",
		Name:        "Satay sauce",
		Size:        "250 g",
		Price:       money.Money{Amount: "2.50", Currency: "EUR", Cents: 250},
		Available:   boolPointer(true),
		Allergens:   "Contiene cacahuetes",
		Ingredients: "cacahuetes, agua",
	}}
	_, err := NewBuild(plan, products)
	if err == nil || !strings.Contains(err.Error(), "Severe peanut") {
		t.Fatalf("expected blocking allergy error, got %v", err)
	}
}

func TestNewBuildBlocksHardDietConflict(t *testing.T) {
	plan := validPlan()
	plan.Household.DietaryRules = []string{"gluten-free"}
	products := []alcampo.Product{{
		ID:          "product-1",
		SKU:         "sku-1",
		Name:        "Wheat pasta",
		Size:        "500 g",
		Price:       money.Money{Amount: "1.50", Currency: "EUR", Cents: 150},
		Available:   boolPointer(true),
		Allergens:   "Contiene gluten",
		Ingredients: "sémola de trigo",
	}}
	_, err := NewBuild(plan, products)
	if err == nil || !strings.Contains(err.Error(), "gluten-free") {
		t.Fatalf("expected blocking diet error, got %v", err)
	}
}

func TestNewBuildBlocksSpanishProcessedAllergenForms(t *testing.T) {
	tests := []struct {
		name      string
		allergy   string
		allergens string
	}{
		{name: "accented tree nuts", allergy: "alergia severa a frutos de cáscara", allergens: "Contiene almendras"},
		{name: "shellfish plural", allergy: "alergia a los mariscos", allergens: "Contiene mariscos"},
		{name: "crustaceans plural", allergy: "alergia a crustáceos", allergens: "Contiene crustáceos"},
		{name: "molluscs plural", allergy: "alergia a moluscos", allergens: "Contiene moluscos"},
		{name: "lupins irregular plural", allergy: "alergia a altramuces", allergens: "Contiene altramuces"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := validPlan()
			plan.Household.Allergies = []string{test.allergy}
			product := alcampo.Product{
				ID:           "processed-product",
				SKU:          "sku-1",
				Name:         "Salsa preparada",
				CategoryPath: []string{"Alimentación", "Salsas preparadas"},
				Size:         "250 g",
				Price:        money.Money{Amount: "2.50", Currency: "EUR", Cents: 250},
				Available:    boolPointer(true),
				Allergens:    test.allergens,
				Ingredients:  "agua, aceite y especias",
			}
			_, err := NewBuild(plan, []alcampo.Product{product})
			if err == nil || !strings.Contains(err.Error(), "conflicts with household allergy") {
				t.Fatalf("expected %q to block processed product labelled %q, got %v", test.allergy, test.allergens, err)
			}
		})
	}
}

func TestNewBuildTreatsSpanishSevereAllergyAsFailClosedForProcessedFood(t *testing.T) {
	for _, allergy := range []string{"alergia severa al cacahuete", "riesgo severo por cacahuetes"} {
		t.Run(allergy, func(t *testing.T) {
			plan := validPlan()
			plan.Household.Allergies = []string{allergy}
			product := alcampo.Product{
				ID:           "unlabelled-processed-product",
				SKU:          "sku-1",
				Name:         "Salsa preparada",
				CategoryPath: []string{"Alimentación", "Salsas preparadas"},
				Size:         "250 g",
				Price:        money.Money{Amount: "2.50", Currency: "EUR", Cents: 250},
				Available:    boolPointer(true),
			}
			_, err := NewBuild(plan, []alcampo.Product{product})
			if err == nil || !strings.Contains(err.Error(), "severe allergy") {
				t.Fatalf("expected Spanish severe allergy %q to fail closed, got %v", allergy, err)
			}
		})
	}
}

func TestNewBuildBlocksChickenForPescatarianSpellings(t *testing.T) {
	for _, rule := range []string{"pescatarian", "pescetarian", "pescetariano", "pescetariana"} {
		t.Run(rule, func(t *testing.T) {
			plan := validPlan()
			plan.Household.DietaryRules = []string{rule}
			product := alcampo.Product{
				ID:           "chicken-croquettes",
				SKU:          "sku-1",
				Name:         "Croquetas de pollo",
				CategoryPath: []string{"Congelados", "Platos preparados"},
				Size:         "400 g",
				Price:        money.Money{Amount: "3.50", Currency: "EUR", Cents: 350},
				Available:    boolPointer(true),
				Ingredients:  "pollo, harina y aceite",
			}
			_, err := NewBuild(plan, []alcampo.Product{product})
			if err == nil || !strings.Contains(err.Error(), "conflicts with dietary rule") {
				t.Fatalf("expected chicken to conflict with %q, got %v", rule, err)
			}
		})
	}
}

func TestBasketLinesMergeDuplicateProducts(t *testing.T) {
	build := Build{Items: []SelectedItem{
		{Shopping: ShoppingItem{Packages: 1.5}, Product: alcampo.Product{ID: "same", Name: "Tomatoes"}},
		{Shopping: ShoppingItem{Packages: 2}, Product: alcampo.Product{ID: "same", Name: "Tomatoes"}},
	}}
	lines := BasketLines(build)
	if len(lines) != 1 || lines[0] != "same 3.5 # Tomatoes" {
		t.Fatalf("unexpected basket lines: %#v", lines)
	}
}

func TestValidateRequiresEveryIngredientToResolve(t *testing.T) {
	plan := validPlan()
	plan.Assumptions = []string{"salt and olive oil are at home"}
	issues := ValidateDraft(plan)
	if !hasIssue(issues, "days[0].meals[0].ingredients[1].source") {
		t.Fatalf("missing pantry coverage issue: %+v", issues)
	}
	plan.Assumptions = append(plan.Assumptions, "tomatoes are already at home")
	if issues := ValidateDraft(plan); len(issues) != 0 {
		t.Fatalf("explicit pantry assumption should validate: %+v", issues)
	}
}

func TestPantryMentionRequiresAWholeIngredientPhrase(t *testing.T) {
	plan := validPlan()
	plan.Days[0].Meals[0].Ingredients[1].Name = "salt"
	plan.Days[0].Meals[0].Ingredients[1].Source = "pantry"
	plan.Assumptions = []string{"unsalted butter is already at home"}
	issues := ValidateDraft(plan)
	if !hasIssue(issues, "days[0].meals[0].ingredients[1].source") {
		t.Fatalf("substring inside unsalted must not prove salt is in the pantry: %+v", issues)
	}
	plan.Assumptions = append(plan.Assumptions, "salt is already at home")
	if issues := ValidateDraft(plan); len(issues) != 0 {
		t.Fatalf("whole-phrase pantry mention should validate: %+v", issues)
	}
}

func TestValidateAllowsPantryOnlyPlan(t *testing.T) {
	plan := validPlan()
	plan.Shopping = []ShoppingItem{}
	plan.Assumptions = []string{"pasta and tomatoes are already at home"}
	for i := range plan.Days[0].Meals[0].Ingredients {
		plan.Days[0].Meals[0].Ingredients[i].Source = "pantry"
	}
	if issues := ValidateSelected(plan); len(issues) != 0 {
		t.Fatalf("pantry-only plan should validate: %+v", issues)
	}
}

func TestNewBuildRejectsUnknownAvailabilityAndFractionalFixedPack(t *testing.T) {
	plan := validPlan()
	product := alcampo.Product{ID: "product-1", SKU: "sku-1", Name: "Pasta", Size: "500 g", Price: money.Money{Amount: "1.50", Currency: "EUR", Cents: 150}}
	if _, err := NewBuild(plan, []alcampo.Product{product}); err == nil || !strings.Contains(err.Error(), "confirm whether") {
		t.Fatalf("expected unknown availability error, got %v", err)
	}
	product.Available = boolPointer(true)
	plan.Shopping[0].Packages = 1.5
	if _, err := NewBuild(plan, []alcampo.Product{product}); err == nil || !strings.Contains(err.Error(), "fractional packages") {
		t.Fatalf("expected fixed-pack fraction error, got %v", err)
	}
}

func TestNewBuildRejectsBasketLineInjectionInProductID(t *testing.T) {
	plan := validPlan()
	product := alcampo.Product{
		ID:        "legitimate-id\nother-product 100",
		SKU:       "sku-1",
		Name:      "Pasta",
		Size:      "500 g",
		Price:     money.Money{Amount: "1.50", Currency: "EUR", Cents: 150},
		Available: boolPointer(true),
	}
	if _, err := NewBuild(plan, []alcampo.Product{product}); err == nil || !strings.Contains(err.Error(), "invalid internal product id") {
		t.Fatalf("unsafe product id should be rejected before basket rendering: %v", err)
	}
}

func TestNewBuildRejectsNegativeOrNonEURPrice(t *testing.T) {
	for _, price := range []money.Money{
		{Amount: "-1.00", Currency: "EUR", Cents: -100},
		{Amount: "1.00", Currency: "USD", Cents: 100},
	} {
		plan := validPlan()
		product := alcampo.Product{ID: "product-1", SKU: "sku-1", Name: "Pasta", Size: "500 g", Price: price, Available: boolPointer(true)}
		if _, err := NewBuild(plan, []alcampo.Product{product}); err == nil || !strings.Contains(err.Error(), "invalid non-EUR or negative price") {
			t.Fatalf("price %+v should be rejected: %v", price, err)
		}
	}
}

func TestNewBuildAcceptsFractionalVariableWeight(t *testing.T) {
	plan := validPlan()
	plan.Shopping[0].Packages = 1.25
	product := alcampo.Product{ID: "product-1", SKU: "sku-1", Name: "Tomates al peso", Size: "750g - 1250g", Unit: "kg", Price: money.Money{Amount: "1.50", Currency: "EUR", Cents: 150}, Available: boolPointer(true)}
	build, err := NewBuild(plan, []alcampo.Product{product})
	if err != nil {
		t.Fatalf("variable-weight build: %v", err)
	}
	if build.EstimatedTotal.Cents != 188 {
		t.Fatalf("variable-weight total = %+v", build.EstimatedTotal)
	}
}

func TestNewBuildRejectsFractionalMissingSizeWithReferencePriceUnit(t *testing.T) {
	plan := validPlan()
	plan.Shopping[0].Packages = 0.75
	product := alcampo.Product{
		ID:        "product-1",
		SKU:       "sku-1",
		Name:      "Tomatoes",
		Unit:      "kg",
		Price:     money.Money{Amount: "1.50", Currency: "EUR", Cents: 150},
		Available: boolPointer(true),
	}
	if _, err := NewBuild(plan, []alcampo.Product{product}); err == nil || !strings.Contains(err.Error(), "fractional packages") {
		t.Fatalf("expected reference-price unit to be insufficient variable-weight evidence, got %v", err)
	}
}

func TestNewBuildRejectsDuplicateSelectedProduct(t *testing.T) {
	plan := validPlan()
	plan.Days[0].Meals[0].Ingredients = append(plan.Days[0].Meals[0].Ingredients, Ingredient{Name: "rice", Amount: "200 g", Source: "Rice"})
	plan.Shopping = append(plan.Shopping, ShoppingItem{Name: "Rice", Needed: "200 g", Query: "rice", ProductSKU: "sku-2", Packages: 1})
	product := alcampo.Product{ID: "same-product", Name: "Basic food", Size: "500 g", Price: money.Money{Amount: "1.50", Currency: "EUR", Cents: 150}, Available: boolPointer(true)}
	if _, err := NewBuild(plan, []alcampo.Product{product, product}); err == nil || !strings.Contains(err.Error(), "duplicates shopping[0]") {
		t.Fatalf("expected duplicate selection error, got %v", err)
	}
}

func TestNewBuildRequiresLabelForSevereAllergy(t *testing.T) {
	plan := validPlan()
	plan.Household.Allergies = []string{"severe peanut allergy"}
	product := alcampo.Product{ID: "product-1", SKU: "sku-1", Name: "Plain pasta", Size: "500 g", Price: money.Money{Amount: "1.50", Currency: "EUR", Cents: 150}, Available: boolPointer(true)}
	if _, err := NewBuild(plan, []alcampo.Product{product}); err == nil || !strings.Contains(err.Error(), "severe allergy") {
		t.Fatalf("expected fail-closed label error, got %v", err)
	}
}

func TestSafetyUnderstandsPositiveGlutenAndLactoseFreeLabels(t *testing.T) {
	plan := validPlan()
	plan.Household.DietaryRules = []string{"gluten-free", "lactose-free"}
	plan.Days[0].Meals[0].Ingredients[0].Name = "gluten-free pasta"
	product := alcampo.Product{ID: "product-1", SKU: "sku-1", Name: "Pasta sin gluten y sin lactosa", Size: "500 g", Price: money.Money{Amount: "1.50", Currency: "EUR", Cents: 150}, Available: boolPointer(true), Ingredients: "harina de maíz"}
	if _, err := NewBuild(plan, []alcampo.Product{product}); err != nil {
		t.Fatalf("positive free-from labels should not conflict: %v", err)
	}
}

func TestRecipeSafetyChecksEnglishAndSpanishTerms(t *testing.T) {
	tests := []struct {
		name       string
		rule       string
		ingredient string
	}{
		{name: "gluten wheat", rule: "gluten-free", ingredient: "wheat pasta"},
		{name: "gluten barley", rule: "sin gluten", ingredient: "barley"},
		{name: "vegan chicken", rule: "vegan", ingredient: "chicken breast"},
		{name: "vegan honey", rule: "vegano", ingredient: "honey"},
		{name: "vegan whey", rule: "vegan", ingredient: "whey protein"},
		{name: "vegan eggs plural", rule: "vegan", ingredient: "eggs"},
		{name: "vegan gelatin", rule: "vegan", ingredient: "gelatin"},
		{name: "vegetarian fish", rule: "vegetarian", ingredient: "fish fillet"},
		{name: "vegetarian lard", rule: "vegetarian", ingredient: "lard"},
		{name: "pescatarian beef", rule: "pescatarian", ingredient: "beef mince"},
		{name: "dairy whey", rule: "dairy-free", ingredient: "whey protein"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := validPlan()
			plan.Household.DietaryRules = []string{test.rule}
			plan.Days[0].Meals[0].Ingredients[0].Name = test.ingredient
			if issues := ValidateDraft(plan); !hasIssue(issues, "days[0].meals[0].ingredients[0].name") {
				t.Fatalf("expected conflict for %q under %q: %+v", test.ingredient, test.rule, issues)
			}
		})
	}
}

func TestTreeNutMatcherIsBilingualAndAccentInsensitive(t *testing.T) {
	for _, evidence := range []string{"contains almonds", "may contain walnuts", "contiene avellanas", "contiene NUECES"} {
		if term := firstUnsafeAllergyTerm(evidence, "severe tree-nut allergy"); term == "" {
			t.Fatalf("did not match %q", evidence)
		}
	}
}

func TestAllergyMatcherDoesNotRejectFoldedFreeFromPhrase(t *testing.T) {
	if term := firstUnsafeAllergyTerm("certificado sin frutos de cáscara", "tree-nut allergy"); term != "" {
		t.Fatalf("free-from phrase was treated as an allergen conflict: %q", term)
	}
}

func TestSafetyMatcherUsesWordBoundariesAndPlantAlternatives(t *testing.T) {
	for _, evidence := range []string{"eggplant", "aubergine"} {
		if term := firstUnsafeAllergyTerm(evidence, "egg allergy"); term != "" {
			t.Fatalf("egg false positive for %q: %q", evidence, term)
		}
	}
	for _, test := range []struct {
		rule string
		food string
	}{
		{rule: "vegan", food: "eggplant"},
		{rule: "vegan", food: "coconut milk"},
		{rule: "vegan", food: "soy yogurt"},
		{rule: "vegan", food: "peanut butter"},
		{rule: "dairy-free", food: "oat cream"},
		{rule: "lactose-free", food: "leche de coco"},
	} {
		if reason := dietaryConflict(test.rule, test.food); reason != "" {
			t.Fatalf("plant food %q conflicts with %q: %s", test.food, test.rule, reason)
		}
	}
	for _, test := range []struct {
		rule string
		food string
	}{
		{rule: "vegan", food: "whole egg"},
		{rule: "vegan", food: "cow's milk"},
		{rule: "dairy-free", food: "whey powder"},
		{rule: "lactose-free", food: "cow milk"},
	} {
		if reason := dietaryConflict(test.rule, test.food); reason == "" {
			t.Fatalf("real conflict %q passed under %q", test.food, test.rule)
		}
	}
}

func TestAllergySynonymsCoverCommonEnglishAndSpanishFoods(t *testing.T) {
	tests := []struct {
		allergy  string
		evidence string
	}{
		{allergy: "gluten allergy", evidence: "wheat flour"},
		{allergy: "gluten allergy", evidence: "barley malt"},
		{allergy: "fish allergy", evidence: "lomos de salmón"},
		{allergy: "fish allergy", evidence: "atún"},
		{allergy: "fish allergy", evidence: "merluza"},
		{allergy: "milk allergy", evidence: "cheese"},
		{allergy: "milk allergy", evidence: "mantequilla"},
		{allergy: "dairy allergy", evidence: "yogurt"},
	}
	for _, test := range tests {
		if term := firstUnsafeAllergyTerm(test.evidence, test.allergy); term == "" {
			t.Errorf("%q did not match %q", test.allergy, test.evidence)
		}
	}
	for _, plant := range []string{"coconut milk", "leche de avena", "soy yogurt", "peanut butter"} {
		if term := firstUnsafeAllergyTerm(plant, "milk allergy"); term != "" {
			t.Errorf("plant alternative %q matched milk allergy as %q", plant, term)
		}
	}
}

func TestNamedAllergyPhrasesBlockDraftIngredients(t *testing.T) {
	tests := []struct {
		name       string
		allergy    string
		ingredient string
	}{
		{name: "English severe singular", allergy: "severe kiwi allergy", ingredient: "sliced kiwis"},
		{name: "English allergic plural", allergy: "allergic to strawberries", ingredient: "strawberry compote"},
		{name: "English first person", allergy: "I am allergic to kiwi", ingredient: "sliced kiwi"},
		{name: "Spanish severe singular", allergy: "alergia grave al kiwi", ingredient: "kiwi fresco"},
		{name: "Spanish allergic plural", allergy: "alérgica a las fresas", ingredient: "fresa troceada"},
		{name: "Spanish first person", allergy: "soy alérgico al kiwi", ingredient: "kiwi fresco"},
		{name: "Spanish tengo phrasing", allergy: "tengo una alergia severa a las fresas", ingredient: "fresa troceada"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := validPlan()
			plan.Household.Allergies = []string{test.allergy}
			plan.Days[0].Meals[0].Ingredients[0].Name = test.ingredient
			issues := ValidateDraft(plan)
			if !hasIssue(issues, "days[0].meals[0].ingredients[0].name") {
				t.Fatalf("named allergy %q did not block ingredient %q: %+v", test.allergy, test.ingredient, issues)
			}
		})
	}
}

func TestBareNamedAllergyListsSplitEveryFood(t *testing.T) {
	tests := []struct {
		name       string
		allergy    string
		ingredient string
	}{
		{name: "English first food", allergy: "kiwi and strawberries", ingredient: "kiwi slices"},
		{name: "English second food singularized", allergy: "kiwi and strawberries", ingredient: "strawberry puree"},
		{name: "Spanish first food", allergy: "kiwi y fresas", ingredient: "kiwis frescos"},
		{name: "Spanish second food singularized", allergy: "kiwi y fresas", ingredient: "fresa troceada"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if term := firstUnsafeAllergyTerm(test.ingredient, test.allergy); term == "" {
				t.Fatalf("bare allergy list %q did not match %q", test.allergy, test.ingredient)
			}
			plan := validPlan()
			plan.Household.Allergies = []string{test.allergy}
			plan.Days[0].Meals[0].Ingredients[0].Name = test.ingredient
			if issues := ValidateDraft(plan); !hasIssue(issues, "days[0].meals[0].ingredients[0].name") {
				t.Fatalf("bare allergy list %q did not block draft ingredient %q: %+v", test.allergy, test.ingredient, issues)
			}
		})
	}
}

func TestNamedAllergyPhrasesBlockLabelledProducts(t *testing.T) {
	tests := []struct {
		name        string
		allergy     string
		ingredients string
	}{
		{name: "English kiwi", allergy: "severe kiwi allergy", ingredients: "apple, kiwi puree, sugar"},
		{name: "English strawberries", allergy: "allergic to strawberries", ingredients: "strawberry puree, oats"},
		{name: "Spanish kiwi", allergy: "alergia grave al kiwi", ingredients: "manzana, kiwi y azúcar"},
		{name: "Spanish strawberries", allergy: "alérgica a las fresas", ingredients: "puré de fresa y avena"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := validPlan()
			plan.Household.Allergies = []string{test.allergy}
			product := alcampo.Product{
				ID:           "processed-product",
				SKU:          "sku-1",
				Name:         "Prepared snack",
				CategoryPath: []string{"Alimentación", "Snacks"},
				Size:         "250 g",
				Price:        money.Money{Amount: "2.50", Currency: "EUR", Cents: 250},
				Available:    boolPointer(true),
				Allergens:    "Contains milk",
				Ingredients:  test.ingredients,
			}
			_, err := NewBuild(plan, []alcampo.Product{product})
			if err == nil || !strings.Contains(err.Error(), "conflicts with household allergy") {
				t.Fatalf("named allergy %q did not block labelled ingredients %q: %v", test.allergy, test.ingredients, err)
			}
		})
	}
}

func TestNamedAllergyEvidencePolicyForProcessedProducts(t *testing.T) {
	baseProduct := alcampo.Product{
		ID:           "processed-product",
		SKU:          "sku-1",
		Name:         "Prepared snack",
		CategoryPath: []string{"Alimentación", "Snacks"},
		Size:         "250 g",
		Price:        money.Money{Amount: "2.50", Currency: "EUR", Cents: 250},
		Available:    boolPointer(true),
	}

	for _, allergy := range []string{"severe kiwi allergy", "alergia grave al kiwi"} {
		t.Run("severe unlabelled "+allergy, func(t *testing.T) {
			plan := validPlan()
			plan.Household.Allergies = []string{allergy}
			_, err := NewBuild(plan, []alcampo.Product{baseProduct})
			if err == nil || !strings.Contains(err.Error(), "no readable online ingredient list") {
				t.Fatalf("unlabelled processed product must fail closed for named severe allergy: %v", err)
			}
		})

		t.Run("severe mismatched label "+allergy, func(t *testing.T) {
			plan := validPlan()
			plan.Household.Allergies = []string{allergy}
			product := baseProduct
			product.Allergens = "Contains milk"
			_, err := NewBuild(plan, []alcampo.Product{product})
			if err == nil || !strings.Contains(err.Error(), "no readable online ingredient list") {
				t.Fatalf("severe named allergy with an unrelated allergen-only label must fail closed: %v", err)
			}
		})

		t.Run("severe readable ingredients "+allergy, func(t *testing.T) {
			plan := validPlan()
			plan.Household.Allergies = []string{allergy}
			product := baseProduct
			product.Allergens = "Contains milk"
			product.Ingredients = "tomato, maize flour, olive oil"
			if _, err := NewBuild(plan, []alcampo.Product{product}); err != nil {
				t.Fatalf("a readable ingredient list with no direct match should remain reviewable: %v", err)
			}
		})
	}

	for _, allergy := range []string{"allergic to strawberries", "alérgica a las fresas"} {
		t.Run("non-severe unlabelled "+allergy, func(t *testing.T) {
			plan := validPlan()
			plan.Household.Allergies = []string{allergy}
			build, err := NewBuild(plan, []alcampo.Product{baseProduct})
			if err != nil {
				t.Fatalf("unlabelled product should remain explicit as a warning for a non-severe allergy: %v", err)
			}
			warnings := strings.Join(build.Warnings, " ")
			if !strings.Contains(warnings, "no readable online ingredient/allergen label") || !strings.Contains(warnings, "no readable online ingredient list") {
				t.Fatalf("unlabelled named-allergy warnings are incomplete: %+v", build.Warnings)
			}
		})

		t.Run("non-severe missing evidence "+allergy, func(t *testing.T) {
			plan := validPlan()
			plan.Household.Allergies = []string{allergy}
			product := baseProduct
			product.Allergens = "Contains milk"
			build, err := NewBuild(plan, []alcampo.Product{product})
			if err != nil {
				t.Fatalf("non-severe missing evidence should produce an explicit warning: %v", err)
			}
			if !strings.Contains(strings.Join(build.Warnings, " "), "no readable online ingredient list") {
				t.Fatalf("missing named-allergy evidence warning: %+v", build.Warnings)
			}
		})
	}
}

func TestUnparseableAllergyWordingNeverSilentlyPasses(t *testing.T) {
	product := alcampo.Product{
		ID:           "processed-product",
		SKU:          "sku-1",
		Name:         "Prepared snack",
		CategoryPath: []string{"Alimentación", "Snacks"},
		Size:         "250 g",
		Price:        money.Money{Amount: "2.50", Currency: "EUR", Cents: 250},
		Available:    boolPointer(true),
		Allergens:    "Contains milk",
		Ingredients:  "tomato, maize flour, olive oil",
	}
	for _, allergy := range []string{
		"severe allergy described only in a medical note",
		"alergia grave: consultar el informe médico",
	} {
		plan := validPlan()
		plan.Household.Allergies = []string{allergy}
		_, err := NewBuild(plan, []alcampo.Product{product})
		if err == nil || !strings.Contains(err.Error(), "could not be mechanically interpreted") {
			t.Errorf("unparseable severe allergy %q did not fail closed: %v", allergy, err)
		}
	}

	plan := validPlan()
	plan.Household.Allergies = []string{"allergy details are in the medical note"}
	build, err := NewBuild(plan, []alcampo.Product{product})
	if err != nil {
		t.Fatalf("unparseable non-severe allergy should remain explicit as a warning: %v", err)
	}
	if !strings.Contains(strings.Join(build.Warnings, " "), "could not be mechanically interpreted") {
		t.Fatalf("missing unsupported-allergy warning: %+v", build.Warnings)
	}
}

func TestUnsupportedAllergyRemainsVisibleForPantryOnlyBuild(t *testing.T) {
	pantryPlan := func(allergy string) Plan {
		plan := validPlan()
		plan.Household.Allergies = []string{allergy}
		plan.Shopping = []ShoppingItem{}
		plan.Assumptions = []string{"pasta and tomatoes are already at home"}
		for index := range plan.Days[0].Meals[0].Ingredients {
			plan.Days[0].Meals[0].Ingredients[index].Source = "pantry"
		}
		return plan
	}
	if _, err := NewBuild(pantryPlan("kiwi allergy - severe"), nil); err == nil || !strings.Contains(err.Error(), "could not be mechanically interpreted") {
		t.Fatalf("unsupported severe pantry-only allergy must fail for clarification: %v", err)
	}
	build, err := NewBuild(pantryPlan("allergy details are in the medical note"), nil)
	if err != nil {
		t.Fatalf("unsupported non-severe pantry-only allergy should warn: %v", err)
	}
	if !strings.Contains(strings.Join(build.Warnings, " "), "could not be mechanically interpreted") {
		t.Fatalf("unsupported pantry-only allergy warning is missing: %+v", build.Warnings)
	}
}

func TestAllergyGroupTriggersUseWordBoundaries(t *testing.T) {
	for _, allergy := range []string{"peanut allergy", "coconut allergy"} {
		if term := firstUnsafeAllergyTerm("contains almonds", allergy); term != "" {
			t.Fatalf("%q incorrectly expanded to tree nuts via substring %q", allergy, term)
		}
	}
	if term := firstUnsafeAllergyTerm("contains peanuts", "peanut allergy"); term == "" {
		t.Fatal("peanut allergy must still match peanuts")
	}
}

func TestNamedAllergyMatcherPreservesFreeFromNegation(t *testing.T) {
	for _, test := range []struct {
		allergy  string
		evidence string
	}{
		{allergy: "severe kiwi allergy", evidence: "certified kiwi-free fruit snack"},
		{allergy: "allergic to strawberries", evidence: "free from strawberries"},
		{allergy: "alergia grave al kiwi", evidence: "producto sin kiwi"},
		{allergy: "alérgica a las fresas", evidence: "barrita libre de fresas"},
	} {
		if term := firstUnsafeAllergyTerm(test.evidence, test.allergy); term != "" {
			t.Errorf("free-from evidence %q conflicted with %q as %q", test.evidence, test.allergy, term)
		}
	}
}

func TestCombinedMilkAndTreeNutAllergyStillSeesAlmondMilk(t *testing.T) {
	if term := firstUnsafeAllergyTerm("almond milk", "milk and tree-nut allergy"); term == "" {
		t.Fatal("combined allergy lost the tree-nut match while scrubbing plant milk")
	}
}

func TestSevereAllergyAllowsClearlyUnprocessedFoodWithWarning(t *testing.T) {
	plan := validPlan()
	plan.Household.Allergies = []string{"severe peanut allergy"}
	product := alcampo.Product{ID: "product-1", SKU: "sku-1", Name: "Tomate pera", Category: "Tomates", CategoryPath: []string{"Frescos", "Verduras y hortalizas", "Tomates"}, Size: "1 kg", Price: money.Money{Amount: "1.50", Currency: "EUR", Cents: 150}, Available: boolPointer(true)}
	build, err := NewBuild(plan, []alcampo.Product{product})
	if err != nil {
		t.Fatalf("fresh single-ingredient food should remain usable: %v", err)
	}
	if len(build.Warnings) == 0 || !strings.Contains(build.Warnings[0], "physical") {
		t.Fatalf("expected physical-check warning: %+v", build.Warnings)
	}
}

func TestSimpleFoodPolicyUsesFreshRawPathNotLookalikeWords(t *testing.T) {
	base := alcampo.Product{Name: "raw food"}
	tests := []struct {
		name string
		path []string
		want bool
	}{
		{name: "live tomato path", path: []string{"Frescos", "Verduras y hortalizas", "Tomates"}, want: true},
		{name: "live chicken path", path: []string{"Frescos", "Carne", "Pollo", "Pollo blanco"}, want: true},
		{name: "live banana path", path: []string{"Frescos", "Frutas", "Plátanos y Bananas"}, want: true},
		{name: "fresh eggs", path: []string{"Frescos", "Huevos"}, want: true},
		{name: "fruit yogurt", path: []string{"Lácteos", "Yogures", "Yogures con fruta"}, want: false},
		{name: "rice crackers", path: []string{"Alimentación", "Aperitivos", "Tortitas de arroz"}, want: false},
		{name: "prepared chicken", path: []string{"Frescos", "Carne", "Pollo preparado"}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			product := base
			product.CategoryPath = test.path
			if got := isClearlySimpleFood(product); got != test.want {
				t.Fatalf("isClearlySimpleFood(%v) = %v, want %v", test.path, got, test.want)
			}
		})
	}
}

func validPlan() Plan {
	return Plan{
		Title: "Three easy dinners",
		Household: Household{
			People:       4,
			Description:  "2 adults and 2 children",
			DietaryRules: []string{},
			Allergies:    []string{},
			Dislikes:     []string{},
		},
		Assumptions: []string{"olive oil, salt, and tomatoes are at home"},
		Days: []Day{{
			Label: "Monday",
			Meals: []Meal{{
				Type:        "dinner",
				Name:        "Tomato pasta",
				Servings:    4,
				TimeMinutes: 25,
				Ingredients: []Ingredient{{Name: "pasta", Amount: "400 g", Source: "Pasta"}, {Name: "tomatoes", Amount: "500 g", Source: "pantry"}},
				Steps:       []string{"Boil the pasta.", "Simmer the tomatoes and combine."},
			}},
		}},
		Shopping: []ShoppingItem{{Name: "Pasta", Needed: "400 g", Query: "pasta", ProductSKU: "sku-1", Packages: 2}},
	}
}

func boolPointer(value bool) *bool { return &value }

func hasIssue(issues []Issue, path string) bool {
	for _, item := range issues {
		if item.Path == path {
			return true
		}
	}
	return false
}
