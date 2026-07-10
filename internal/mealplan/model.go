package mealplan

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Plan is the small hand-off between Hermes and carrito. Hermes owns the
// cooking decisions; carrito validates the hand-off and performs store work.
type Plan struct {
	Title       string         `json:"title"`
	Household   Household      `json:"household"`
	Assumptions []string       `json:"assumptions"`
	Days        []Day          `json:"days"`
	Shopping    []ShoppingItem `json:"shopping"`
}

type Household struct {
	People       int      `json:"people"`
	Description  string   `json:"description,omitempty"`
	DietaryRules []string `json:"dietary_rules"`
	Allergies    []string `json:"allergies"`
	Dislikes     []string `json:"dislikes"`
}

type Day struct {
	Label string `json:"label"`
	Meals []Meal `json:"meals"`
}

type Meal struct {
	Type        string       `json:"type"`
	Name        string       `json:"name"`
	Servings    int          `json:"servings"`
	TimeMinutes int          `json:"time_minutes"`
	Ingredients []Ingredient `json:"ingredients"`
	Steps       []string     `json:"steps"`
	Notes       []string     `json:"notes,omitempty"`
}

type Ingredient struct {
	Name   string `json:"name"`
	Amount string `json:"amount"`
	Source string `json:"source"`
}

type ShoppingItem struct {
	Name       string  `json:"name"`
	Needed     string  `json:"needed"`
	Query      string  `json:"query"`
	ProductSKU string  `json:"product_sku,omitempty"`
	Packages   float64 `json:"packages,omitempty"`
	Reason     string  `json:"reason,omitempty"`
}

type Issue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func Load(path string) (Plan, error) {
	f, err := os.Open(path)
	if err != nil {
		return Plan{}, err
	}
	defer f.Close()
	return Decode(f)
}

func Decode(r io.Reader) (Plan, error) {
	data, err := io.ReadAll(io.LimitReader(r, 4<<20))
	if err != nil {
		return Plan{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var plan Plan
	if err := dec.Decode(&plan); err != nil {
		return Plan{}, err
	}
	if err := ensureJSONEnd(dec); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func ensureJSONEnd(dec *json.Decoder) error {
	var extra any
	err := dec.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("meal plan contains more than one JSON value")
	}
	return err
}

// ValidateDraft checks the cooking plan and the search terms Hermes will use.
// Product choices are intentionally optional at this stage.
func ValidateDraft(plan Plan) []Issue {
	return validateCore(plan)
}

// ValidateSelected additionally requires one concrete Alcampo product and a
// positive package count for every shopping line.
func ValidateSelected(plan Plan) []Issue {
	issues := ValidateDraft(plan)
	for i, item := range plan.Shopping {
		base := fmt.Sprintf("shopping[%d]", i)
		if blank(item.ProductSKU) {
			issues = append(issues, issue(base+".product_sku", "is required before building the basket"))
		}
		if item.Packages <= 0 {
			issues = append(issues, issue(base+".packages", "must be greater than zero before building the basket"))
		}
		if item.Packages > 10_000 {
			issues = append(issues, issue(base+".packages", "must not exceed 10000"))
		}
		if hasMoreThanThreeDecimals(item.Packages) {
			issues = append(issues, issue(base+".packages", "may have at most three decimal places"))
		}
	}
	return issues
}

func validateCore(plan Plan) []Issue {
	var issues []Issue
	shoppingNames := map[string]int{}
	shoppingUsed := map[string]bool{}
	for i, item := range plan.Shopping {
		base := fmt.Sprintf("shopping[%d]", i)
		if blank(item.Name) {
			issues = append(issues, issue(base+".name", "is required"))
		} else {
			key := normalizedName(item.Name)
			if previous, exists := shoppingNames[key]; exists {
				issues = append(issues, issue(base+".name", fmt.Sprintf("duplicates shopping[%d].name; consolidate the ingredient", previous)))
			} else {
				shoppingNames[key] = i
			}
		}
		if blank(item.Needed) {
			issues = append(issues, issue(base+".needed", "is required"))
		}
		if blank(item.Query) {
			issues = append(issues, issue(base+".query", "is required"))
		}
		if item.Packages < 0 {
			issues = append(issues, issue(base+".packages", "cannot be negative"))
		}
	}
	if blank(plan.Title) {
		issues = append(issues, issue("title", "is required"))
	}
	if plan.Household.People <= 0 {
		issues = append(issues, issue("household.people", "must be greater than zero"))
	}
	if plan.Household.DietaryRules == nil {
		issues = append(issues, issue("household.dietary_rules", "must be present; use [] when there are none"))
	}
	if plan.Household.Allergies == nil {
		issues = append(issues, issue("household.allergies", "must be present; use [] when there are none"))
	}
	if plan.Household.Dislikes == nil {
		issues = append(issues, issue("household.dislikes", "must be present; use [] when there are none"))
	}
	if plan.Assumptions == nil {
		issues = append(issues, issue("assumptions", "must be present; use [] when there are none"))
	}
	if plan.Shopping == nil {
		issues = append(issues, issue("shopping", "must be present; use [] when no shopping is needed"))
	}
	if len(plan.Days) == 0 {
		issues = append(issues, issue("days", "must contain at least one day"))
	}
	for dayIndex, day := range plan.Days {
		dayPath := fmt.Sprintf("days[%d]", dayIndex)
		if blank(day.Label) {
			issues = append(issues, issue(dayPath+".label", "is required"))
		}
		if len(day.Meals) == 0 {
			issues = append(issues, issue(dayPath+".meals", "must contain at least one meal"))
		}
		for mealIndex, meal := range day.Meals {
			mealPath := fmt.Sprintf("%s.meals[%d]", dayPath, mealIndex)
			if blank(meal.Type) {
				issues = append(issues, issue(mealPath+".type", "is required"))
			}
			if blank(meal.Name) {
				issues = append(issues, issue(mealPath+".name", "is required"))
			}
			if meal.Servings <= 0 {
				issues = append(issues, issue(mealPath+".servings", "must be greater than zero"))
			}
			if meal.TimeMinutes <= 0 {
				issues = append(issues, issue(mealPath+".time_minutes", "must be greater than zero"))
			}
			if len(meal.Ingredients) == 0 {
				issues = append(issues, issue(mealPath+".ingredients", "must contain at least one ingredient"))
			}
			for ingredientIndex, ingredient := range meal.Ingredients {
				ingredientPath := fmt.Sprintf("%s.ingredients[%d]", mealPath, ingredientIndex)
				if blank(ingredient.Name) {
					issues = append(issues, issue(ingredientPath+".name", "is required"))
				}
				if blank(ingredient.Amount) {
					issues = append(issues, issue(ingredientPath+".amount", "is required"))
				}
				if blank(ingredient.Source) {
					issues = append(issues, issue(ingredientPath+".source", "must be \"pantry\" or exactly match a shopping item name"))
				} else if strings.EqualFold(strings.TrimSpace(ingredient.Source), "pantry") {
					if !pantryMentioned(plan.Assumptions, ingredient.Name) {
						issues = append(issues, issue(ingredientPath+".source", fmt.Sprintf("uses pantry but assumptions does not explicitly mention %q", ingredient.Name)))
					}
				} else {
					key := normalizedName(ingredient.Source)
					if _, exists := shoppingNames[key]; !exists {
						issues = append(issues, issue(ingredientPath+".source", fmt.Sprintf("%q does not match any shopping item name", ingredient.Source)))
					} else {
						shoppingUsed[key] = true
					}
				}
				for _, allergy := range plan.Household.Allergies {
					if term := firstUnsafeAllergyTerm(ingredient.Name, allergy); term != "" {
						issues = append(issues, issue(ingredientPath+".name", fmt.Sprintf("conflicts with household allergy %q (%s)", allergy, term)))
					}
				}
				for _, rule := range plan.Household.DietaryRules {
					if reason := dietaryConflict(rule, ingredient.Name); reason != "" {
						issues = append(issues, issue(ingredientPath+".name", fmt.Sprintf("conflicts with dietary rule %q (%s)", rule, reason)))
					}
				}
			}
			if len(meal.Steps) == 0 {
				issues = append(issues, issue(mealPath+".steps", "must contain at least one cooking step"))
			}
			for stepIndex, step := range meal.Steps {
				if blank(step) {
					issues = append(issues, issue(fmt.Sprintf("%s.steps[%d]", mealPath, stepIndex), "cannot be blank"))
				}
			}
		}
	}
	for key, index := range shoppingNames {
		if !shoppingUsed[key] {
			issues = append(issues, issue(fmt.Sprintf("shopping[%d]", index), "is not referenced by any recipe ingredient"))
		}
	}
	return issues
}

func normalizedName(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func hasMoreThanThreeDecimals(value float64) bool {
	text := strconv.FormatFloat(value, 'f', -1, 64)
	decimal := strings.IndexByte(text, '.')
	return decimal >= 0 && len(strings.TrimRight(text[decimal+1:], "0")) > 3
}

func pantryMentioned(assumptions []string, ingredient string) bool {
	needle := normalizedName(ingredient)
	if needle == "" {
		return false
	}
	for _, assumption := range assumptions {
		if containsBoundedPhrase(foldForMatch(assumption), foldForMatch(needle)) {
			return true
		}
	}
	return false
}

func issue(path, message string) Issue {
	return Issue{Path: path, Message: message}
}

func blank(value string) bool {
	return strings.TrimSpace(value) == ""
}

func FormatIssues(issues []Issue) error {
	if len(issues) == 0 {
		return nil
	}
	var lines []string
	for _, item := range issues {
		lines = append(lines, item.Path+": "+item.Message)
	}
	return fmt.Errorf("invalid meal plan:\n- %s", strings.Join(lines, "\n- "))
}
