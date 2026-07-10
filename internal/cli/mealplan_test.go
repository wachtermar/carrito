package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/config"
	"github.com/wachtermar/carrito/internal/mealplan"
	"github.com/wachtermar/carrito/internal/money"
)

func TestMealPlanCandidatesBuildAndGuardedCartReadback(t *testing.T) {
	mock := newAlcampoMock(t)
	defer mock.server.Close()

	configDir := t.TempDir()
	t.Setenv("CARRITO_CONFIG_DIR", configDir)
	t.Setenv("CARRITO_BASE_URL", mock.server.URL)
	configureTestSession(t)

	planPath := filepath.Join(t.TempDir(), "family-plan.json")
	plan := testMealPlan()
	writeTestJSON(t, planPath, plan)

	var stdout, stderr bytes.Buffer
	if err := Run([]string{"mealplan", "candidates", planPath, "--limit", "3", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("candidates: %v\nstderr: %s", err, stderr.String())
	}
	var candidates []struct {
		Products []alcampo.Product `json:"products"`
	}
	var candidatePayload struct {
		Status          string `json:"status"`
		UnresolvedCount int    `json:"unresolved_count"`
		Items           []struct {
			Products []alcampo.Product `json:"products"`
		} `json:"items"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &candidatePayload); err != nil {
		t.Fatal(err)
	}
	candidates = candidatePayload.Items
	if len(candidates) != 1 || len(candidates[0].Products) != 1 || candidates[0].Products[0].SKU != "pasta-sku" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
	if candidatePayload.Status != "ready" || candidatePayload.UnresolvedCount != 0 || strings.Contains(stdout.String(), "\"images\"") {
		t.Fatalf("candidate response was not compact and ready: %s", stdout.String())
	}

	htmlPath := filepath.Join(t.TempDir(), "share", "family-plan.html")
	basketPath := filepath.Join(t.TempDir(), "cart", "family-plan.txt")
	stdout.Reset()
	stderr.Reset()
	if err := Run([]string{"mealplan", "build", planPath, "--html-out", htmlPath, "--basket-out", basketPath, "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("build: %v\nstderr: %s", err, stderr.String())
	}
	var built mealPlanBuildResult
	if err := json.Unmarshal(stdout.Bytes(), &built); err != nil {
		t.Fatal(err)
	}
	if built.EstimatedTotal.Cents != 350 {
		t.Fatalf("total = %+v", built.EstimatedTotal)
	}
	assertFileContains(t, htmlPath, "Quick tomato pasta")
	assertFileContains(t, basketPath, "pasta-id 2")

	stdout.Reset()
	stderr.Reset()
	if err := Run([]string{"cart", "set-many", "-f", basketPath, "--max", "10", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("cart set-many: %v\nstderr: %s", err, stderr.String())
	}
	if mock.quantity() != 2 {
		t.Fatalf("mock cart quantity = %v, want 2", mock.quantity())
	}

	stdout.Reset()
	stderr.Reset()
	if err := Run([]string{"cart", "get", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("cart get: %v\nstderr: %s", err, stderr.String())
	}
	var summary alcampo.CartSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	if summary.ItemCount != 1 || summary.Items[0].Quantity != "2" || summary.Total.Cents != 350 {
		t.Fatalf("unexpected cart readback: %+v", summary)
	}

	stdout.Reset()
	stderr.Reset()
	if err := Run([]string{"cart", "set-many", "-f", basketPath, "--max", "10", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("idempotent cart set-many: %v\nstderr: %s", err, stderr.String())
	}
	var repeated CartSetManyResult
	if err := json.Unmarshal(stdout.Bytes(), &repeated); err != nil {
		t.Fatal(err)
	}
	if !repeated.Noop || mock.quantity() != 2 {
		t.Fatalf("repeated set was not idempotent: result=%+v quantity=%v", repeated, mock.quantity())
	}
	if !repeated.Verified || repeated.Lines[0].ActualQuantity != "2" {
		t.Fatalf("repeated set did not report deterministic verification: %+v", repeated)
	}
}

func TestMealPlanCandidatesReturnsIncompleteStatusAndError(t *testing.T) {
	mock := newAlcampoMock(t)
	mock.noProducts = true
	defer mock.server.Close()
	t.Setenv("CARRITO_CONFIG_DIR", t.TempDir())
	t.Setenv("CARRITO_BASE_URL", mock.server.URL)
	configureTestSession(t)
	planPath := filepath.Join(t.TempDir(), "plan.json")
	writeTestJSON(t, planPath, testMealPlan())
	var stdout, stderr bytes.Buffer
	err := Run([]string{"mealplan", "candidates", planPath, "--json"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "no usable candidates") {
		t.Fatalf("expected unresolved-candidate error, got %v", err)
	}
	var response candidateResponse
	if jsonErr := json.Unmarshal(stdout.Bytes(), &response); jsonErr != nil {
		t.Fatal(jsonErr)
	}
	if response.Status != "incomplete" || response.UnresolvedCount != 1 || response.Items[0].Error == "" {
		t.Fatalf("unexpected incomplete response: %+v", response)
	}
}

func TestBuildableCandidateRequiresTheFieldsBuildNeeds(t *testing.T) {
	available := true
	base := alcampo.Product{ID: "id", SKU: "sku", Name: "Pasta", Price: moneyValue(150), Size: "500 g", Available: &available}
	tests := []struct {
		name   string
		mutate func(*alcampo.Product)
		want   bool
	}{
		{name: "ready", want: true},
		{name: "missing id", mutate: func(product *alcampo.Product) { product.ID = "" }},
		{name: "missing sku", mutate: func(product *alcampo.Product) { product.SKU = "" }},
		{name: "missing price", mutate: func(product *alcampo.Product) { product.Price = money.Money{} }},
		{name: "unknown availability", mutate: func(product *alcampo.Product) { product.Available = nil }},
		{name: "missing size", mutate: func(product *alcampo.Product) { product.Size = "" }},
		{name: "reference price unit is not variable weight", mutate: func(product *alcampo.Product) { product.Size = ""; product.Unit = "kg" }},
		{name: "explicit variable weight", mutate: func(product *alcampo.Product) {
			product.Size = ""
			product.Unit = "kg"
			product.Name = "Tomates al peso"
		}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			product := base
			if test.mutate != nil {
				test.mutate(&product)
			}
			if got := buildableCandidate(product); got != test.want {
				t.Fatalf("buildableCandidate(%+v) = %v, want %v", product, got, test.want)
			}
		})
	}
}

func TestMealPlanPantryOnlyBuildNeedsNoStoreOrSession(t *testing.T) {
	t.Setenv("CARRITO_CONFIG_DIR", t.TempDir())
	plan := testMealPlan()
	plan.Shopping = []mealplan.ShoppingItem{}
	plan.Assumptions = []string{"pasta and tomatoes are already at home"}
	for index := range plan.Days[0].Meals[0].Ingredients {
		plan.Days[0].Meals[0].Ingredients[index].Source = "pantry"
	}
	planPath := filepath.Join(t.TempDir(), "pantry.json")
	writeTestJSON(t, planPath, plan)
	htmlPath := filepath.Join(t.TempDir(), "pantry.html")
	basketPath := filepath.Join(t.TempDir(), "pantry.basket.txt")
	var stdout, stderr bytes.Buffer
	if err := Run([]string{"mealplan", "build", planPath, "--html-out", htmlPath, "--basket-out", basketPath, "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("pantry-only build: %v", err)
	}
	assertFileContains(t, htmlPath, "Quick tomato pasta")
	if raw, err := os.ReadFile(basketPath); err != nil || len(raw) != 0 {
		t.Fatalf("pantry basket = %q, err=%v", raw, err)
	}
}

func TestMealPlanBuildRejectsIdenticalOutputPaths(t *testing.T) {
	t.Setenv("CARRITO_CONFIG_DIR", t.TempDir())
	planPath := filepath.Join(t.TempDir(), "plan.json")
	writeTestJSON(t, planPath, testMealPlan())
	outputPath := filepath.Join(t.TempDir(), "same.out")
	var stdout, stderr bytes.Buffer
	err := Run([]string{"mealplan", "build", planPath, "--html-out", outputPath, "--basket-out", outputPath, "--json"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "different paths") {
		t.Fatalf("expected same-output error, got %v", err)
	}
}

func TestMealPlanBuildRejectsOutputsAliasedBySymlinkedParents(t *testing.T) {
	t.Setenv("CARRITO_CONFIG_DIR", t.TempDir())
	plan := testMealPlan()
	plan.Shopping = []mealplan.ShoppingItem{}
	plan.Assumptions = []string{"pasta and tomatoes are already at home"}
	for index := range plan.Days[0].Meals[0].Ingredients {
		plan.Days[0].Meals[0].Ingredients[index].Source = "pantry"
	}
	root := t.TempDir()
	realOutputDir := filepath.Join(root, "actual")
	if err := os.Mkdir(realOutputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	firstAlias := filepath.Join(root, "first-alias")
	secondAlias := filepath.Join(root, "second-alias")
	if err := os.Symlink(realOutputDir, firstAlias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realOutputDir, secondAlias); err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(root, "plan.json")
	writeTestJSON(t, planPath, plan)
	var stdout, stderr bytes.Buffer
	err := Run([]string{
		"mealplan", "build", planPath,
		"--html-out", filepath.Join(firstAlias, "same-output"),
		"--basket-out", filepath.Join(secondAlias, "same-output"),
		"--json",
	}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "different paths") {
		t.Fatalf("expected symlink-parent alias error, got %v", err)
	}
}

func TestMealPlanBuildCannotOverwriteInputThroughPathOrSymlink(t *testing.T) {
	t.Setenv("CARRITO_CONFIG_DIR", t.TempDir())
	plan := testMealPlan()
	plan.Shopping = []mealplan.ShoppingItem{}
	plan.Assumptions = []string{"pasta and tomatoes are already at home"}
	for index := range plan.Days[0].Meals[0].Ingredients {
		plan.Days[0].Meals[0].Ingredients[index].Source = "pantry"
	}
	planPath := filepath.Join(t.TempDir(), "plan.json")
	writeTestJSON(t, planPath, plan)
	linkPath := filepath.Join(t.TempDir(), "plan-link")
	if err := os.Symlink(planPath, linkPath); err != nil {
		t.Fatal(err)
	}
	for _, outputPath := range []string{planPath, linkPath} {
		var stdout, stderr bytes.Buffer
		err := Run([]string{"mealplan", "build", planPath, "--html-out", outputPath, "--json"}, &stdout, &stderr)
		if err == nil || !strings.Contains(err.Error(), "must not overwrite") {
			t.Fatalf("output %s: expected overwrite error, got %v", outputPath, err)
		}
	}
	if _, err := mealplan.Load(planPath); err != nil {
		t.Fatalf("input was damaged: %v", err)
	}
}

func TestMealPlanBuildRejectsUnavailableProduct(t *testing.T) {
	mock := newAlcampoMock(t)
	mock.available = false
	defer mock.server.Close()
	t.Setenv("CARRITO_CONFIG_DIR", t.TempDir())
	t.Setenv("CARRITO_BASE_URL", mock.server.URL)
	configureTestSession(t)
	planPath := filepath.Join(t.TempDir(), "plan.json")
	writeTestJSON(t, planPath, testMealPlan())

	var stdout, stderr bytes.Buffer
	err := Run([]string{"mealplan", "build", planPath, "--json"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("expected unavailable error, got %v", err)
	}
}

func TestCartSetManyRefusesSpendAboveGuard(t *testing.T) {
	mock := newAlcampoMock(t)
	defer mock.server.Close()
	t.Setenv("CARRITO_CONFIG_DIR", t.TempDir())
	t.Setenv("CARRITO_BASE_URL", mock.server.URL)
	configureTestSession(t)
	basket := filepath.Join(t.TempDir(), "basket.txt")
	if err := os.WriteFile(basket, []byte("pasta-id 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := Run([]string{"cart", "set-many", "-f", basket, "--max", "2", "--json"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "exceeds spending guard") {
		t.Fatalf("expected guard error, got %v", err)
	}
	if mock.quantity() != 0 {
		t.Fatalf("cart mutated despite guard: %v", mock.quantity())
	}
}

func TestCartRefusesPriceIncreaseAfterMealPlanPreview(t *testing.T) {
	mock := newAlcampoMock(t)
	defer mock.server.Close()
	t.Setenv("CARRITO_CONFIG_DIR", t.TempDir())
	t.Setenv("CARRITO_BASE_URL", mock.server.URL)
	configureTestSession(t)
	planPath := filepath.Join(t.TempDir(), "plan.json")
	writeTestJSON(t, planPath, testMealPlan())
	basketPath := filepath.Join(t.TempDir(), "basket.txt")

	var stdout, stderr bytes.Buffer
	if err := Run([]string{"mealplan", "build", planPath, "--basket-out", basketPath, "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("preview build: %v", err)
	}
	var preview mealPlanBuildResult
	if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.EstimatedTotal.Cents != 350 {
		t.Fatalf("preview total = %+v", preview.EstimatedTotal)
	}

	mock.price = 4.00 // Two packages now cost €8, above the approved €5 cap.
	stdout.Reset()
	stderr.Reset()
	err := Run([]string{"cart", "set-many", "-f", basketPath, "--max", "5", "--json"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "exceeds spending guard") {
		t.Fatalf("expected changed-price guard error, got %v", err)
	}
	if mock.quantity() != 0 {
		t.Fatalf("cart mutated after price increase: %v", mock.quantity())
	}
}

func TestCartSetManyPreservesLargerExistingQuantity(t *testing.T) {
	mock := newAlcampoMock(t)
	mock.cartQty = 3
	defer mock.server.Close()
	t.Setenv("CARRITO_CONFIG_DIR", t.TempDir())
	t.Setenv("CARRITO_BASE_URL", mock.server.URL)
	configureTestSession(t)
	basket := filepath.Join(t.TempDir(), "basket.txt")
	if err := os.WriteFile(basket, []byte("pasta-id 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run([]string{"cart", "set-many", "-f", basket, "--max", "10", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("set-many: %v", err)
	}
	var result CartSetManyResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Noop || !result.Verified || !result.Lines[0].PreservedExisting || result.Lines[0].AppliedDelta != "0" || result.Lines[0].ActualQuantity != "3" || mock.quantity() != 3 {
		t.Fatalf("larger quantity was not preserved: result=%+v qty=%v", result, mock.quantity())
	}
}

func TestCartSetManyNoopRereadsQuantityAfterDecoration(t *testing.T) {
	mock := newAlcampoMock(t)
	mock.cartQty = 2
	removed := 0.0
	mock.setQuantityOnDecoration = &removed
	defer mock.server.Close()
	t.Setenv("CARRITO_CONFIG_DIR", t.TempDir())
	t.Setenv("CARRITO_BASE_URL", mock.server.URL)
	configureTestSession(t)
	basket := filepath.Join(t.TempDir(), "basket.txt")
	if err := os.WriteFile(basket, []byte("pasta-id 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := Run([]string{"cart", "set-many", "-f", basket, "--max", "10", "--json"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("expected fresh no-op quantity verification failure, got %v", err)
	}
	if mock.applies() != 0 || mock.quantity() != 0 {
		t.Fatalf("no-op verification mutated cart: applies=%d qty=%v", mock.applies(), mock.quantity())
	}
}

func TestCartSetManyNoopRereadsWholeCartTotalAfterDecoration(t *testing.T) {
	mock := newAlcampoMock(t)
	mock.cartQty = 2
	mock.totalOverrideAfterDecoration = 20
	defer mock.server.Close()
	t.Setenv("CARRITO_CONFIG_DIR", t.TempDir())
	t.Setenv("CARRITO_BASE_URL", mock.server.URL)
	configureTestSession(t)
	basket := filepath.Join(t.TempDir(), "basket.txt")
	if err := os.WriteFile(basket, []byte("pasta-id 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := Run([]string{"cart", "set-many", "-f", basket, "--max", "10", "--json"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "actual cart total 20.00 EUR exceeds spending guard") {
		t.Fatalf("expected fresh no-op total guard failure, got %v", err)
	}
	if mock.applies() != 0 || mock.quantity() != 2 {
		t.Fatalf("no-op total verification mutated cart: applies=%d qty=%v", mock.applies(), mock.quantity())
	}
}

func TestCartSetManyRejectsUnknownOrUnavailableProduct(t *testing.T) {
	for _, test := range []struct {
		name    string
		prepare func(*alcampoMock)
		want    string
	}{
		{name: "unknown", prepare: func(mock *alcampoMock) { mock.availabilityUnknown = true }, want: "availability is unknown"},
		{name: "unavailable", prepare: func(mock *alcampoMock) { mock.available = false }, want: "unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			mock := newAlcampoMock(t)
			test.prepare(mock)
			defer mock.server.Close()
			t.Setenv("CARRITO_CONFIG_DIR", t.TempDir())
			t.Setenv("CARRITO_BASE_URL", mock.server.URL)
			configureTestSession(t)
			basket := filepath.Join(t.TempDir(), "basket.txt")
			if err := os.WriteFile(basket, []byte("pasta-id 2\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			err := Run([]string{"cart", "set-many", "-f", basket, "--max", "10", "--json"}, &stdout, &stderr)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q error, got %v", test.want, err)
			}
			if mock.quantity() != 0 {
				t.Fatalf("cart mutated: %v", mock.quantity())
			}
		})
	}
}

func TestCartSetManyRollsBackActualTotalAboveGuard(t *testing.T) {
	mock := newAlcampoMock(t)
	mock.totalOverrideAfterFirstApply = 6
	defer mock.server.Close()
	t.Setenv("CARRITO_CONFIG_DIR", t.TempDir())
	t.Setenv("CARRITO_BASE_URL", mock.server.URL)
	configureTestSession(t)
	basket := filepath.Join(t.TempDir(), "basket.txt")
	if err := os.WriteFile(basket, []byte("pasta-id 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := Run([]string{"cart", "set-many", "-f", basket, "--max", "5", "--json"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "actual cart total") || !strings.Contains(err.Error(), "reversed and verified") {
		t.Fatalf("expected guarded rollback error, got %v", err)
	}
	if mock.quantity() != 0 {
		t.Fatalf("cart was not rolled back: %v", mock.quantity())
	}
}

func TestCartSetManyRefusesUnsafeReversalAfterPartialWrite(t *testing.T) {
	mock := newAlcampoMock(t)
	mock.firstApplyFactor = 0.5
	defer mock.server.Close()
	t.Setenv("CARRITO_CONFIG_DIR", t.TempDir())
	t.Setenv("CARRITO_BASE_URL", mock.server.URL)
	configureTestSession(t)
	basket := filepath.Join(t.TempDir(), "basket.txt")
	if err := os.WriteFile(basket, []byte("pasta-id 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := Run([]string{"cart", "set-many", "-f", basket, "--max", "5", "--json"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "verification failed") || !strings.Contains(err.Error(), "automatic reversal is unsafe") {
		t.Fatalf("expected manual-review error, got %v", err)
	}
	if mock.quantity() != 1 {
		t.Fatalf("unsafe reversal changed the partial write: %v", mock.quantity())
	}
}

func TestCartSetManyDoesNotReverseOptimisticUnpersistedResponse(t *testing.T) {
	mock := newAlcampoMock(t)
	mock.cartQty = 1
	mock.optimisticFirstApplyResponse = true
	defer mock.server.Close()
	t.Setenv("CARRITO_CONFIG_DIR", t.TempDir())
	t.Setenv("CARRITO_BASE_URL", mock.server.URL)
	configureTestSession(t)
	basket := filepath.Join(t.TempDir(), "basket.txt")
	if err := os.WriteFile(basket, []byte("pasta-id 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := Run([]string{"cart", "set-many", "-f", basket, "--max", "10", "--json"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "persisted cart read-back did not conservatively confirm") || !strings.Contains(err.Error(), "automatic reversal is unsafe") {
		t.Fatalf("expected non-destructive manual-review error, got %v", err)
	}
	if mock.applies() != 1 || mock.quantity() != 1 {
		t.Fatalf("optimistic response erased original quantity: applies=%d qty=%v", mock.applies(), mock.quantity())
	}
}

func TestCartSetManyReportsRollbackFailure(t *testing.T) {
	mock := newAlcampoMock(t)
	mock.totalOverrideAfterFirstApply = 6
	mock.failApplyNumber = 2
	defer mock.server.Close()
	t.Setenv("CARRITO_CONFIG_DIR", t.TempDir())
	t.Setenv("CARRITO_BASE_URL", mock.server.URL)
	configureTestSession(t)
	basket := filepath.Join(t.TempDir(), "basket.txt")
	if err := os.WriteFile(basket, []byte("pasta-id 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := Run([]string{"cart", "set-many", "-f", basket, "--max", "5", "--json"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "automatic rollback failed") || !strings.Contains(err.Error(), "review the cart manually") {
		t.Fatalf("expected rollback-failure warning, got %v", err)
	}
	if mock.quantity() != 2 {
		t.Fatalf("test did not leave the simulated partial write in place: %v", mock.quantity())
	}
}

func TestCartSetManyReversalPreservesConcurrentSameProductAddition(t *testing.T) {
	mock := newAlcampoMock(t)
	mock.concurrentDeltaAfterFirstApply = 1
	defer mock.server.Close()
	t.Setenv("CARRITO_CONFIG_DIR", t.TempDir())
	t.Setenv("CARRITO_BASE_URL", mock.server.URL)
	configureTestSession(t)
	basket := filepath.Join(t.TempDir(), "basket.txt")
	if err := os.WriteFile(basket, []byte("pasta-id 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := Run([]string{"cart", "set-many", "-f", basket, "--max", "10", "--json"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "verification failed") || !strings.Contains(err.Error(), "reversed and verified") {
		t.Fatalf("expected verified reversal after concurrent edit, got %v", err)
	}
	if mock.quantity() != 1 {
		t.Fatalf("concurrent addition was not preserved: %v", mock.quantity())
	}
}

func TestCartSetManyRejectsDuplicateResolvedProduct(t *testing.T) {
	mock := newAlcampoMock(t)
	defer mock.server.Close()
	t.Setenv("CARRITO_CONFIG_DIR", t.TempDir())
	t.Setenv("CARRITO_BASE_URL", mock.server.URL)
	configureTestSession(t)
	basket := filepath.Join(t.TempDir(), "basket.txt")
	if err := os.WriteFile(basket, []byte("pasta-id 1\npasta-sku 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := Run([]string{"cart", "set-many", "-f", basket, "--max", "5", "--json"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "duplicates the product") {
		t.Fatalf("expected duplicate-product error, got %v", err)
	}
	if mock.quantity() != 0 {
		t.Fatalf("cart mutated: %v", mock.quantity())
	}
}

func TestCartSetManyRejectsUnreasonableQuantityBeforeMutation(t *testing.T) {
	mock := newAlcampoMock(t)
	defer mock.server.Close()
	t.Setenv("CARRITO_CONFIG_DIR", t.TempDir())
	t.Setenv("CARRITO_BASE_URL", mock.server.URL)
	configureTestSession(t)
	for _, test := range []struct {
		quantity string
		want     string
	}{
		{quantity: "10001", want: "10000"},
		{quantity: "1.2345", want: "decimal places"},
		{quantity: "1/3", want: "ordinary decimal"},
		{quantity: "1e0", want: "ordinary decimal"},
	} {
		basket := filepath.Join(t.TempDir(), "basket.txt")
		if err := os.WriteFile(basket, []byte("pasta-id "+test.quantity+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		err := Run([]string{"cart", "set-many", "-f", basket, "--max", "5", "--json"}, &stdout, &stderr)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("quantity %s: expected %q error, got %v", test.quantity, test.want, err)
		}
		if mock.quantity() != 0 {
			t.Fatalf("cart mutated for %s: %v", test.quantity, mock.quantity())
		}
	}
}

func configureTestSession(t *testing.T) {
	t.Helper()
	cfg := config.Default()
	cfg.Defaults.MarketSet = true
	cfg.Defaults.RegionID = "test-region"
	cfg.Auth.Cookie = "session=test"
	cfg.Auth.CSRFToken = "csrf-test"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
}

func testMealPlan() mealplan.Plan {
	return mealplan.Plan{
		Title: "Weeknight plan",
		Household: mealplan.Household{
			People:       4,
			Description:  "2 adults and 2 children",
			DietaryRules: []string{},
			Allergies:    []string{},
			Dislikes:     []string{},
		},
		Assumptions: []string{"salt, olive oil, and tomatoes are at home"},
		Days: []mealplan.Day{{
			Label: "Monday",
			Meals: []mealplan.Meal{{
				Type:        "dinner",
				Name:        "Quick tomato pasta",
				Servings:    4,
				TimeMinutes: 25,
				Ingredients: []mealplan.Ingredient{{Name: "pasta", Amount: "400 g", Source: "Pasta"}, {Name: "tomatoes", Amount: "500 g", Source: "pantry"}},
				Steps:       []string{"Boil the pasta.", "Cook the sauce and combine."},
			}},
		}},
		Shopping: []mealplan.ShoppingItem{{
			Name:       "Pasta",
			Needed:     "400 g",
			Query:      "pasta trigo",
			ProductSKU: "pasta-sku",
			Packages:   2,
			Reason:     "Two small packs avoid leftovers.",
		}},
	}
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), want) {
		t.Fatalf("%s does not contain %q", path, want)
	}
}

type alcampoMock struct {
	t                              *testing.T
	server                         *httptest.Server
	mu                             sync.Mutex
	cartQty                        float64
	available                      bool
	availabilityUnknown            bool
	price                          float64
	applyCount                     int
	firstApplyFactor               float64
	failApplyNumber                int
	totalOverrideAfterFirstApply   float64
	totalOverrideConsumed          bool
	concurrentDeltaAfterFirstApply float64
	concurrentDeltaConsumed        bool
	setQuantityOnDecoration        *float64
	totalOverrideAfterDecoration   float64
	decorationOccurred             bool
	optimisticFirstApplyResponse   bool
	noProducts                     bool
}

func newAlcampoMock(t *testing.T) *alcampoMock {
	t.Helper()
	mock := &alcampoMock{t: t, available: true, price: 1.75}
	mock.server = httptest.NewServer(http.HandlerFunc(mock.serveHTTP))
	return mock
}

func (m *alcampoMock) serveHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/":
		_, _ = w.Write([]byte(`<html></html>`))
	case r.URL.Path == "/api/webproductpagews/v6/product-pages/search":
		products := []any{m.product()}
		if m.noProducts || r.URL.Query().Get("q") == "pasta-id" {
			products = []any{}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"products": products})
	case r.URL.Path == "/api/webproductpagews/v6/products" && r.Method == http.MethodPut:
		m.mu.Lock()
		m.decorationOccurred = true
		if m.setQuantityOnDecoration != nil {
			m.cartQty = *m.setQuantityOnDecoration
		}
		m.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"products": []any{m.product()}})
	case r.URL.Path == "/api/cart/v2/carts/active/cart-view" && r.Method == http.MethodGet:
		_ = json.NewEncoder(w).Encode(m.cartForGet())
	case r.URL.Path == "/api/cart/v1/carts/active/apply-quantity" && r.Method == http.MethodPost:
		var changes []struct {
			ProductID string      `json:"productId"`
			Quantity  json.Number `json:"quantity"`
		}
		dec := json.NewDecoder(r.Body)
		dec.UseNumber()
		if err := dec.Decode(&changes); err != nil {
			m.t.Errorf("decode cart changes: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		m.mu.Lock()
		m.applyCount++
		applyNumber := m.applyCount
		if m.failApplyNumber == applyNumber {
			m.mu.Unlock()
			http.Error(w, "simulated rollback failure", http.StatusInternalServerError)
			return
		}
		factor := 1.0
		if applyNumber == 1 && m.firstApplyFactor != 0 {
			factor = m.firstApplyFactor
		}
		responseQty := m.cartQty
		for _, change := range changes {
			if change.ProductID != "pasta-id" {
				m.t.Errorf("unexpected product id %q", change.ProductID)
			}
			delta, _ := change.Quantity.Float64()
			responseQty += delta * factor
			if !(applyNumber == 1 && m.optimisticFirstApplyResponse) {
				m.cartQty += delta * factor
			}
		}
		m.mu.Unlock()
		if applyNumber == 1 && m.optimisticFirstApplyResponse {
			_ = json.NewEncoder(w).Encode(m.cartWithQuantity(responseQty))
			return
		}
		_ = json.NewEncoder(w).Encode(m.cart())
	case strings.HasPrefix(r.URL.Path, "/products/"):
		http.NotFound(w, r) // Exercise the product-detail fallback to exact search data.
	default:
		http.NotFound(w, r)
	}
}

func (m *alcampoMock) product() map[string]any {
	product := map[string]any{
		"productId":         "pasta-id",
		"retailerProductId": "pasta-sku",
		"name":              "Pasta 500 g",
		"brand":             "Test Brand",
		"size":              "500 g",
		"price":             map[string]any{"amount": fmt.Sprintf("%.2f", m.price), "currency": "EUR"},
		"images":            []any{"/images/pasta.jpg"},
	}
	if !m.availabilityUnknown {
		product["available"] = m.available
	}
	return product
}

func (m *alcampoMock) cartForGet() map[string]any {
	m.mu.Lock()
	if m.applyCount == 1 && m.concurrentDeltaAfterFirstApply != 0 && !m.concurrentDeltaConsumed {
		m.cartQty += m.concurrentDeltaAfterFirstApply
		m.concurrentDeltaConsumed = true
	}
	overrideTotal := m.applyCount == 1 && m.totalOverrideAfterFirstApply > 0 && !m.totalOverrideConsumed
	if overrideTotal {
		m.totalOverrideConsumed = true
	}
	decorationTotal := m.totalOverrideAfterDecoration
	decorationOccurred := m.decorationOccurred
	m.mu.Unlock()
	root := m.cart()
	if overrideTotal {
		root["totals"] = map[string]any{"display": map[string]any{"itemPriceAfterPromos": map[string]any{"amount": fmt.Sprintf("%.2f", m.totalOverrideAfterFirstApply), "currency": "EUR"}}}
	} else if decorationOccurred && decorationTotal > 0 {
		root["totals"] = map[string]any{"display": map[string]any{"itemPriceAfterPromos": map[string]any{"amount": fmt.Sprintf("%.2f", decorationTotal), "currency": "EUR"}}}
	}
	return root
}

func (m *alcampoMock) cart() map[string]any {
	qty := m.quantity()
	return m.cartWithQuantity(qty)
}

func (m *alcampoMock) cartWithQuantity(qty float64) map[string]any {
	root := map[string]any{
		"totals": map[string]any{
			"display": map[string]any{
				"itemPriceAfterPromos": map[string]any{"amount": fmt.Sprintf("%.2f", qty*m.price), "currency": "EUR"},
			},
		},
	}
	if qty > 0 {
		root["items"] = []any{map[string]any{"quantity": qty, "product": m.product()}}
	}
	return root
}

func (m *alcampoMock) quantity() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cartQty
}

func (m *alcampoMock) applies() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.applyCount
}
