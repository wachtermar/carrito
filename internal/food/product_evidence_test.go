package food

import (
	"strings"
	"testing"
)

func TestProductEvidenceReportLiveDetailCurrentClaimsSafe(t *testing.T) {
	run := readyExactRun()
	run.Shop.SelectedProducts[0].Product.Size = "1 kg"
	run.Shop.SelectedProducts[0].Product.ImageURL = "https://example.test/rice.png"
	run.Shop.SelectedProducts[0].ProductEvidenceSource = "alcampo_product_detail"
	run.Shop.SelectedProducts[0].ProductEvidenceCheckedAt = nowStamp()

	policy := DefaultProductEvidencePolicy(true)
	policy.RequireFreshEvidence = true
	policy.AllowCacheEvidence = false
	policy.AllowSearchOnlyEvidence = false
	policy.RequirePriceEvidence = true
	policy.RequireImageEvidence = true
	report := BuildProductEvidenceReport(run, policy)

	if report.Status != ProductEvidenceFreshComplete {
		t.Fatalf("status = %s, want fresh_complete: %+v", report.Status, report)
	}
	if report.ProductEvidenceFingerprint == "" || report.ProductSelectionFingerprint == "" {
		t.Fatalf("missing fingerprints: %+v", report)
	}
	if !productEvidenceCurrentClaimsSafe(report) {
		t.Fatalf("live detail evidence should allow current product claims: %+v", report)
	}
	if report.Summary.LinesWithPrice != 1 || report.Summary.LinesWithPackageEvidence != 1 || report.Summary.LinesWithImageEvidence != 1 {
		t.Fatalf("coverage summary missing price/package/image evidence: %+v", report.Summary)
	}

	attached := AttachProductEvidenceReport(run, report)
	if attached.ProductEvidenceFingerprint == "" || attached.QuantityLedger == nil || attached.QuantityLedger.ProductEvidenceFingerprint != attached.ProductEvidenceFingerprint {
		t.Fatalf("product evidence fingerprint was not attached through the run: %+v", attached)
	}
}

func TestRequireFreshProductEvidenceBlocksClaimsNotBasketMechanics(t *testing.T) {
	run := readyExactRun()
	run.Shop.SelectedProducts[0].ProductEvidenceSource = ProductEvidenceSourceEmbeddedSelection
	run.Shop.SelectedProducts[0].ProductEvidenceCheckedAt = nowStamp()

	policy := DefaultProductEvidencePolicy(false)
	policy.RequireFreshEvidence = true
	report := BuildProductEvidenceReport(run, policy)
	run = AttachProductEvidenceReport(run, report)
	gate := ApplyReadinessGate(&run, ReadinessPolicy{RequireSafeBasket: true, RequireFreshProductEvidence: true})

	if gate.SafeToUseProductEvidence {
		t.Fatalf("embedded search evidence should not satisfy fresh product evidence: %+v", gate)
	}
	if !gate.SafeToBuild {
		t.Fatalf("fresh product claim failure should not make exact basket mechanics unsafe: %+v", gate)
	}
	if gate.ExitCode != ReadinessExitBlocked || gate.ExitReason != "fresh product evidence is not ready" {
		t.Fatalf("gate exit = %d %q, want fresh product evidence block", gate.ExitCode, gate.ExitReason)
	}
	text := strings.Join(ReadinessBasketLinesWithRecipeSwapOptimizationPantryServingNutritionBudgetRepairAndBudgetDeal([]string{"rice-sku 1 # rice"}, &gate, nil, nil, nil, nil, nil, nil, nil, nil), "\n")
	if !strings.Contains(text, "Final Alcampo product evidence") || !strings.Contains(text, "PRODUCT EVIDENCE IS NOT READY") {
		t.Fatalf("basket text missing product evidence warning:\n%s", text)
	}
}
