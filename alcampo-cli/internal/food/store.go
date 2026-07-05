package food

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"alcampo-cli/internal/config"
)

func Dir() (string, error) {
	base, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "food"), nil
}

func ProfilePath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "profile.json"), nil
}

func PantryPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pantry.json"), nil
}

func HistoryPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "history.jsonl"), nil
}

func MealPlansDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mealplans"), nil
}

func LoadProfile() (Profile, error) {
	path, err := ProfilePath()
	if err != nil {
		return Profile{}, err
	}
	var p Profile
	if err := readJSONFile(path, &p); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Profile{}, nil
		}
		return Profile{}, err
	}
	return p, nil
}

func SaveProfile(p Profile) error {
	if p.People < 0 {
		return errors.New("people cannot be negative")
	}
	if p.SelectionPolicy != "" {
		policy, ok := NormalizeSelectionPolicy(p.SelectionPolicy)
		if !ok {
			return fmt.Errorf("unknown selection policy %q", p.SelectionPolicy)
		}
		p.SelectionPolicy = policy
	}
	p.UpdatedAt = nowStamp()
	path, err := ProfilePath()
	if err != nil {
		return err
	}
	return writeJSONFile(path, p)
}

func LoadPantry() (Pantry, error) {
	path, err := PantryPath()
	if err != nil {
		return Pantry{}, err
	}
	var p Pantry
	if err := readJSONFile(path, &p); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Pantry{}, nil
		}
		return Pantry{}, err
	}
	return p, nil
}

func SavePantry(p Pantry) error {
	for i := range p.Items {
		if strings.TrimSpace(p.Items[i].Name) == "" {
			return fmt.Errorf("pantry item %d has empty name", i+1)
		}
		if p.Items[i].Quantity < 0 {
			return fmt.Errorf("pantry item %q has negative quantity", p.Items[i].Name)
		}
		if p.Items[i].ID == "" {
			p.Items[i].ID = itemID(p.Items[i].Name, p.Items[i].Location)
		}
		if p.Items[i].Confidence == 0 {
			p.Items[i].Confidence = 1
		}
		if p.Items[i].LastChecked == "" {
			p.Items[i].LastChecked = nowStamp()
		}
	}
	p.UpdatedAt = nowStamp()
	path, err := PantryPath()
	if err != nil {
		return err
	}
	return writeJSONFile(path, p)
}

func SaveMealPlan(plan MealPlan) (MealPlan, error) {
	if plan.ID == "" {
		plan.ID = slugID("mealplan")
	}
	if plan.CreatedAt == "" {
		plan.CreatedAt = nowStamp()
	}
	dir, err := MealPlansDir()
	if err != nil {
		return plan, err
	}
	path := filepath.Join(dir, plan.ID+".json")
	plan.File = path
	return plan, writeJSONFile(path, plan)
}

func LoadMealPlan(ref string) (MealPlan, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return MealPlan{}, errors.New("mealplan reference cannot be empty")
	}
	candidates := []string{ref}
	if !strings.ContainsRune(ref, filepath.Separator) && filepath.Ext(ref) == "" {
		dir, err := MealPlansDir()
		if err != nil {
			return MealPlan{}, err
		}
		candidates = append(candidates, filepath.Join(dir, ref+".json"))
	}
	if filepath.Ext(ref) == "" {
		candidates = append(candidates, ref+".json")
	}
	var lastErr error
	for _, path := range candidates {
		var plan MealPlan
		if err := readJSONFile(path, &plan); err == nil {
			if plan.File == "" {
				if abs, absErr := filepath.Abs(path); absErr == nil {
					plan.File = abs
				} else {
					plan.File = path
				}
			}
			return plan, nil
		} else {
			lastErr = err
		}
	}
	if lastErr != nil {
		return MealPlan{}, lastErr
	}
	return MealPlan{}, os.ErrNotExist
}

func LoadShopResult(path string) (ShopResult, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return ShopResult{}, errors.New("shop result path cannot be empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ShopResult{}, err
	}
	var shop ShopResult
	if err := json.Unmarshal(data, &shop); err == nil && len(shop.SelectedProducts) > 0 {
		return shop, nil
	}
	var wrapper struct {
		Shop ShopResult `json:"shop"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return ShopResult{}, err
	}
	if len(wrapper.Shop.SelectedProducts) == 0 {
		return ShopResult{}, fmt.Errorf("%s does not contain a food shop result", path)
	}
	return wrapper.Shop, nil
}

func AppendHistory(event any) error {
	path, err := HistoryPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	if err := json.NewEncoder(w).Encode(event); err != nil {
		return err
	}
	return w.Flush()
}

func LoadHistory(limit int) ([]map[string]any, error) {
	path, err := HistoryPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var events []map[string]any
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event map[string]any
		dec := json.NewDecoder(strings.NewReader(line))
		dec.UseNumber()
		if err := dec.Decode(&event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if limit > 0 && len(events) > limit {
		events = events[len(events)-limit:]
	}
	return events, nil
}

func readJSONFile(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func writeJSONFile(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func itemID(name, location string) string {
	base := normalizeKey(strings.TrimSpace(name) + " " + strings.TrimSpace(location))
	if base == "" {
		return slugID("item")
	}
	return base
}

func slugID(prefix string) string {
	return prefix + "-" + strings.ReplaceAll(nowStamp(), ":", "")
}
