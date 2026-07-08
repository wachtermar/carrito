package food

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wachtermar/carrito/internal/strutil"
)

const (
	ProductEvidenceNotRun        ProductEvidenceStatus = "not_run"
	ProductEvidenceNotNeeded     ProductEvidenceStatus = "not_needed"
	ProductEvidenceFreshComplete ProductEvidenceStatus = "fresh_complete"
	ProductEvidenceFreshPartial  ProductEvidenceStatus = "fresh_partial"
	ProductEvidenceStale         ProductEvidenceStatus = "stale"
	ProductEvidenceBlocked       ProductEvidenceStatus = "blocked"
)

const (
	ProductEvidenceSourceLiveDetail        = "live_detail"
	ProductEvidenceSourceCacheDetail       = "cache_detail"
	ProductEvidenceSourceSnapshotReplay    = "snapshot_replay"
	ProductEvidenceSourceEmbeddedSelection = "embedded_selection"
	ProductEvidenceSourceMissing           = "missing"
)

func DefaultProductEvidencePolicy(strictQuantity bool) ProductEvidencePolicy {
	return ProductEvidencePolicy{
		Enabled:                  true,
		RefreshProductEvidence:   true,
		RequireFreshEvidence:     false,
		AllowCacheEvidence:       true,
		AllowSearchOnlyEvidence:  true,
		RequirePriceEvidence:     false,
		RequirePackageEvidence:   strictQuantity,
		RequireImageEvidence:     false,
		RequireNutritionEvidence: false,
	}
}

func BuildProductEvidenceReport(run FoodRunArtifact, policy ProductEvidencePolicy) ProductEvidenceReport {
	if policy.Enabled == false {
		policy.Enabled = true
	}
	report := ProductEvidenceReport{
		SchemaVersion:               "1",
		Status:                      ProductEvidenceFreshComplete,
		Policy:                      policy,
		ProductSelectionFingerprint: firstNonEmptyString(run.ProductSelectionFingerprint, ProductSelectionFingerprint(run.Shop)),
		CheckedAt:                   nowStamp(),
	}
	if len(run.Shop.SelectedProducts) == 0 {
		report.Status = ProductEvidenceNotNeeded
		report.Summary.LinesNotNeeded = 1
		report.ProductEvidenceFingerprint = ProductEvidenceFingerprint(report)
		return report
	}
	for _, selected := range run.Shop.SelectedProducts {
		line := productEvidenceLineFromSelection(selected, policy)
		report.Lines = append(report.Lines, line)
		report.Summary.SelectedProductLines++
		if line.EvidenceStatus != "not_needed" {
			report.Summary.LinesChecked++
		}
		switch line.EvidenceSource {
		case ProductEvidenceSourceLiveDetail:
			report.Summary.FreshLines++
		case ProductEvidenceSourceCacheDetail:
			report.Summary.CacheLines++
		case ProductEvidenceSourceSnapshotReplay:
			report.Summary.SnapshotReplayLines++
		case ProductEvidenceSourceEmbeddedSelection:
			report.Summary.SearchOnlyLines++
		case ProductEvidenceSourceMissing:
			report.Summary.MissingEvidenceLines++
		}
		switch line.AvailabilityStatus {
		case "unavailable":
			report.Summary.UnavailableLines++
		case "unknown", "":
			report.Summary.AvailabilityUnknownLines++
		}
		if line.Price.Amount != "" {
			report.Summary.LinesWithPrice++
		}
		if line.PackageEvidence.NetQuantity != nil {
			report.Summary.LinesWithPackageEvidence++
		}
		if line.Offer != nil && line.Offer.Text != "" {
			report.Summary.LinesWithOfferEvidence++
		}
		if line.Nutrition != nil && line.Nutrition.Parsed {
			report.Summary.LinesWithNutritionEvidence++
		}
		if line.Image.Status == "present" {
			report.Summary.LinesWithImageEvidence++
		}
		if line.Drift.PriceChanged {
			report.Summary.PriceChangedLines++
		}
		if line.Drift.OfferChanged {
			report.Summary.OfferChangedLines++
		}
		if line.Drift.PackageChanged {
			report.Summary.PackageChangedLines++
		}
		if line.Drift.NutritionChanged {
			report.Summary.NutritionChangedLines++
		}
		for _, issue := range line.BlockingIssues {
			report.BlockingIssues = append(report.BlockingIssues, issue)
		}
		for _, warning := range line.Warnings {
			report.Warnings = append(report.Warnings, warning)
		}
	}
	sort.SliceStable(report.Lines, func(i, j int) bool {
		if report.Lines[i].ProductID != report.Lines[j].ProductID {
			return report.Lines[i].ProductID < report.Lines[j].ProductID
		}
		return report.Lines[i].ProductName < report.Lines[j].ProductName
	})
	finalizeProductEvidenceReport(&report)
	report.ProductEvidenceFingerprint = ProductEvidenceFingerprint(report)
	return report
}

func AttachProductEvidenceReport(artifact FoodRunArtifact, report ProductEvidenceReport) FoodRunArtifact {
	if report.ProductSelectionFingerprint == "" {
		report.ProductSelectionFingerprint = firstNonEmptyString(artifact.ProductSelectionFingerprint, ProductSelectionFingerprint(artifact.Shop))
	}
	report.ProductEvidenceFingerprint = ProductEvidenceFingerprint(report)
	artifact.ProductEvidenceReport = &report
	artifact.ProductEvidenceFingerprint = report.ProductEvidenceFingerprint
	if artifact.QuantityLedger != nil {
		artifact.QuantityLedger.ProductEvidenceFingerprint = report.ProductEvidenceFingerprint
	}
	if artifact.NutritionLedger != nil {
		artifact.NutritionLedger.ProductEvidenceFingerprint = report.ProductEvidenceFingerprint
		artifact.NutritionLedger.NutritionLedgerFingerprint = NutritionLedgerFingerprint(*artifact.NutritionLedger)
		artifact.NutritionLedgerFingerprint = artifact.NutritionLedger.NutritionLedgerFingerprint
	}
	if artifact.BudgetDealReport != nil {
		artifact.BudgetDealReport.ProductEvidenceFingerprint = report.ProductEvidenceFingerprint
		artifact.BudgetDealReport.BudgetDealFingerprint = BudgetDealFingerprint(*artifact.BudgetDealReport)
		artifact.BudgetDealFingerprint = artifact.BudgetDealReport.BudgetDealFingerprint
	}
	return artifact
}

func ProductEvidenceFingerprint(report ProductEvidenceReport) string {
	clone := report
	clone.ProductEvidenceFingerprint = ""
	data, err := json.Marshal(clone)
	if err != nil {
		return ""
	}
	var canonical any
	if err := json.Unmarshal(data, &canonical); err != nil {
		return ""
	}
	data, err = json.Marshal(canonical)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func productEvidenceCurrentClaimsSafe(report ProductEvidenceReport) bool {
	if report.Status != ProductEvidenceFreshComplete || len(report.BlockingIssues) > 0 || len(report.Lines) == 0 {
		return false
	}
	for _, line := range report.Lines {
		if normalizeProductEvidenceSource(line.EvidenceSource) != ProductEvidenceSourceLiveDetail {
			return false
		}
	}
	return true
}

func productEvidenceCurrentNutritionClaimsSafe(report ProductEvidenceReport) bool {
	return productEvidenceCurrentClaimsSafe(report) &&
		report.Summary.SelectedProductLines > 0 &&
		report.Summary.LinesWithNutritionEvidence == report.Summary.SelectedProductLines
}

func productEvidenceLineFromSelection(selected SelectedProduct, policy ProductEvidencePolicy) ProductEvidenceLine {
	product := selected.Product
	source := normalizeProductEvidenceSource(selected.ProductEvidenceSource)
	line := ProductEvidenceLine{
		ProductID:            strutil.FirstNonEmpty(product.ID, product.SKU),
		SKU:                  product.SKU,
		ProductName:          product.Name,
		BasketLineQuantity:   selected.PackageCount,
		IngredientKeys:       []string{normalizeKey(selected.Ingredient.Name)},
		IngredientNames:      []string{selected.Ingredient.Name},
		EvidenceSource:       source,
		EvidenceStatus:       "fresh",
		CheckedAt:            selected.ProductEvidenceCheckedAt,
		AvailabilityStatus:   availabilityStatus(product.Available),
		AvailabilityReason:   availabilityReason(product.Available),
		Price:                product.Price,
		PackageEvidence:      ParsePackageEvidence(product.Size, product.Name),
		Nutrition:            ProductNutritionEvidenceFromSelection(selected),
		Image:                productImageEvidence(product),
		DetailEvidenceHash:   productSummaryEvidenceHash(product),
		SearchEvidenceHash:   ProductSelectionFingerprint(ShopResult{SelectedProducts: []SelectedProduct{selected}}),
		ProductEvidenceError: strings.TrimSpace(selected.ProductEvidenceError),
	}
	if len(product.Offers) > 0 {
		offers := offerEvidence(product.Offers, selected.PackageCount)
		if len(offers) > 0 {
			line.Offer = &offers[0]
		}
	}
	if source == ProductEvidenceSourceMissing {
		line.EvidenceStatus = "missing"
		line.BlockingIssues = append(line.BlockingIssues, productEvidenceIssue("product_evidence_missing", "blocking", line, "Selected product could not be refreshed from Alcampo evidence.", "Rerun product selection or refresh Alcampo product evidence."))
	} else if source == ProductEvidenceSourceEmbeddedSelection {
		line.EvidenceStatus = "search_only_allowed"
		if policy.RequireFreshEvidence {
			line.EvidenceStatus = "stale"
			line.BlockingIssues = append(line.BlockingIssues, productEvidenceIssue("fresh_product_evidence_required", "blocking", line, "Selected product only has embedded search evidence, but fresh product evidence is required.", "Rerun with product detail evidence available or disable --require-fresh-product-evidence."))
		} else if !policy.AllowSearchOnlyEvidence {
			line.BlockingIssues = append(line.BlockingIssues, productEvidenceIssue("search_only_product_evidence_blocked", "blocking", line, "Search-only product evidence is not allowed by policy.", "Enable product detail refresh or allow search-only evidence."))
		} else {
			line.Warnings = append(line.Warnings, productEvidenceIssue("search_only_product_evidence", "warning", line, "Selected product uses embedded search evidence, not refreshed product detail evidence.", "Refresh product detail before claiming current Alcampo product evidence."))
		}
	} else if source == ProductEvidenceSourceCacheDetail {
		line.EvidenceStatus = "cache_allowed"
		if policy.RequireFreshEvidence || !policy.AllowCacheEvidence {
			line.EvidenceStatus = "stale"
			line.BlockingIssues = append(line.BlockingIssues, productEvidenceIssue("cached_product_evidence_not_fresh", "blocking", line, "Cached product detail evidence is not fresh enough for this policy.", "Rerun live product detail refresh or allow product evidence cache."))
		}
	} else if source == ProductEvidenceSourceSnapshotReplay {
		line.EvidenceStatus = "fresh"
	} else {
		line.EvidenceStatus = "fresh"
	}
	if product.Available != nil && !*product.Available {
		line.BlockingIssues = append(line.BlockingIssues, productEvidenceIssue("product_unavailable", "blocking", line, "Alcampo product evidence marks this selected product unavailable.", "Choose an available replacement before building the basket."))
	}
	if policy.RequirePriceEvidence && line.Price.Amount == "" {
		line.BlockingIssues = append(line.BlockingIssues, productEvidenceIssue("price_evidence_missing", "blocking", line, "Selected product is missing price evidence.", "Refresh product data before claiming current prices."))
	}
	if policy.RequirePackageEvidence && line.PackageEvidence.NetQuantity == nil {
		line.BlockingIssues = append(line.BlockingIssues, productEvidenceIssue("package_evidence_missing", "blocking", line, "Selected product is missing package-size evidence required for strict quantity math.", "Select a product with parseable package size or relax strict quantity policy."))
	}
	if policy.RequireImageEvidence && line.Image.Status != "present" {
		line.BlockingIssues = append(line.BlockingIssues, productEvidenceIssue("image_evidence_missing", "blocking", line, "Selected product is missing image evidence.", "Refresh product data or select a product with an image."))
	}
	if policy.RequireNutritionEvidence && (line.Nutrition == nil || !line.Nutrition.Parsed) {
		line.BlockingIssues = append(line.BlockingIssues, productEvidenceIssue("nutrition_evidence_missing", "blocking", line, "Selected product is missing parseable nutrition label evidence.", "Refresh product data or keep nutrition claims caveated."))
	}
	if age, ok := productEvidenceAgeSeconds(line.CheckedAt); ok {
		line.EvidenceAgeSeconds = &age
		if policy.MaxEvidenceAgeSeconds > 0 && age > policy.MaxEvidenceAgeSeconds {
			line.EvidenceStatus = "stale"
			line.BlockingIssues = append(line.BlockingIssues, productEvidenceIssue("product_evidence_too_old", "blocking", line, "Selected product evidence is older than the accepted maximum age.", "Refresh product detail evidence before presenting current Alcampo claims."))
		}
	} else if policy.MaxEvidenceAgeSeconds > 0 {
		line.EvidenceStatus = "stale"
		line.BlockingIssues = append(line.BlockingIssues, productEvidenceIssue("product_evidence_age_unknown", "blocking", line, "Selected product evidence has no parseable checked_at timestamp.", "Refresh product detail evidence before presenting current Alcampo claims."))
	}
	if line.AvailabilityStatus == "unknown" {
		line.Warnings = append(line.Warnings, productEvidenceIssue("availability_unknown", "warning", line, "Alcampo product evidence did not explicitly confirm stock availability.", "Do not claim exact stock availability; verify at cart or checkout time."))
	}
	return line
}

func productEvidenceAgeSeconds(checkedAt string) (int, bool) {
	if strings.TrimSpace(checkedAt) == "" {
		return 0, false
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(checkedAt))
	if err != nil {
		return 0, false
	}
	age := int(time.Since(t).Seconds())
	if age < 0 {
		age = 0
	}
	return age, true
}

func finalizeProductEvidenceReport(report *ProductEvidenceReport) {
	if len(report.BlockingIssues) > 0 {
		report.Status = ProductEvidenceBlocked
		return
	}
	if report.Summary.SelectedProductLines == 0 {
		report.Status = ProductEvidenceNotNeeded
		return
	}
	if report.Summary.FreshLines+report.Summary.SnapshotReplayLines == report.Summary.SelectedProductLines {
		report.Status = ProductEvidenceFreshComplete
		return
	}
	if report.Summary.FreshLines+report.Summary.SnapshotReplayLines+report.Summary.CacheLines+report.Summary.SearchOnlyLines == report.Summary.SelectedProductLines {
		report.Status = ProductEvidenceFreshPartial
		return
	}
	report.Status = ProductEvidenceStale
}

func normalizeProductEvidenceSource(source string) string {
	switch strings.TrimSpace(source) {
	case "alcampo_product_detail":
		return ProductEvidenceSourceLiveDetail
	case "alcampo_product_detail_cache":
		return ProductEvidenceSourceCacheDetail
	case "snapshot_replay":
		return ProductEvidenceSourceSnapshotReplay
	case "embedded_selection", "":
		return ProductEvidenceSourceEmbeddedSelection
	default:
		return strings.TrimSpace(source)
	}
}

func productImageEvidence(product ProductSummary) ProductImageEvidence {
	if strings.TrimSpace(product.ImageURL) == "" {
		return ProductImageEvidence{Status: "missing"}
	}
	return ProductImageEvidence{Status: "present", URL: product.ImageURL, Source: "product_data"}
}

func productEvidenceIssue(code, severity string, line ProductEvidenceLine, message, remediation string) ProductEvidenceIssue {
	return ProductEvidenceIssue{
		Code:           code,
		Severity:       severity,
		ProductID:      line.ProductID,
		ProductName:    line.ProductName,
		IngredientKey:  firstString(line.IngredientKeys),
		IngredientName: firstString(line.IngredientNames),
		Message:        message,
		Remediation:    remediation,
	}
}

func availabilityStatus(available *bool) string {
	if available == nil {
		return "unknown"
	}
	if *available {
		return "available"
	}
	return "unavailable"
}

func availabilityReason(available *bool) string {
	if available == nil {
		return "Alcampo product evidence did not expose an explicit availability field."
	}
	if *available {
		return "Alcampo product evidence marked the product available."
	}
	return "Alcampo product evidence marked the product unavailable."
}

func productSummaryEvidenceHash(product ProductSummary) string {
	data, err := json.Marshal(product)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func firstString(values []string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func ProductNutritionEvidenceFromSelection(selected SelectedProduct) *ProductNutritionEvidence {
	if selected.ProductNutrition == nil {
		return nil
	}
	return &ProductNutritionEvidence{
		ProductID:   selected.Product.ID,
		SKU:         selected.Product.SKU,
		ProductName: selected.Product.Name,
		Parsed:      true,
		Basis:       nutritionEvidenceBasisFromStructured(*selected.ProductNutrition),
		Facts:       nutritionFactsFromStructured(*selected.ProductNutrition, selected.ProductNutrition.Source, fmt.Sprintf("%.2f", selected.ProductNutrition.Confidence)),
		Confidence:  fmt.Sprintf("%.2f", selected.ProductNutrition.Confidence),
		Warnings:    append([]string(nil), selected.NutritionWarnings...),
	}
}

func ProductEvidenceSourceForSnapshotMode(source, snapshotMode string) string {
	if snapshotMode == "replay" && source == ProductEvidenceSourceLiveDetail {
		return ProductEvidenceSourceSnapshotReplay
	}
	return source
}

func ParseBasketQuantity(value string) int {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return n
}
