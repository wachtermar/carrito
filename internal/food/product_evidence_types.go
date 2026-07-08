package food

import "github.com/wachtermar/carrito/internal/money"

type ProductEvidenceStatus string

type ProductEvidencePolicy struct {
	Enabled                  bool `json:"enabled"`
	RefreshProductEvidence   bool `json:"refresh_product_evidence"`
	RequireFreshEvidence     bool `json:"require_fresh_evidence"`
	MaxEvidenceAgeSeconds    int  `json:"max_evidence_age_seconds,omitempty"`
	AllowCacheEvidence       bool `json:"allow_cache_evidence"`
	AllowSearchOnlyEvidence  bool `json:"allow_search_only_evidence"`
	RequirePriceEvidence     bool `json:"require_price_evidence"`
	RequirePackageEvidence   bool `json:"require_package_evidence"`
	RequireImageEvidence     bool `json:"require_image_evidence"`
	RequireNutritionEvidence bool `json:"require_nutrition_evidence"`
}

type ProductEvidenceReport struct {
	SchemaVersion               string                 `json:"schema_version"`
	Status                      ProductEvidenceStatus  `json:"status"`
	Policy                      ProductEvidencePolicy  `json:"policy"`
	ProductSelectionFingerprint string                 `json:"product_selection_fingerprint,omitempty"`
	ProductEvidenceFingerprint  string                 `json:"product_evidence_fingerprint,omitempty"`
	StoreID                     string                 `json:"store_id,omitempty"`
	StoreName                   string                 `json:"store_name,omitempty"`
	CheckedAt                   string                 `json:"checked_at,omitempty"`
	Summary                     ProductEvidenceSummary `json:"summary"`
	Lines                       []ProductEvidenceLine  `json:"lines,omitempty"`
	BlockingIssues              []ProductEvidenceIssue `json:"blocking_issues,omitempty"`
	Warnings                    []ProductEvidenceIssue `json:"warnings,omitempty"`
}

type ProductEvidenceSummary struct {
	SelectedProductLines       int `json:"selected_product_lines"`
	LinesChecked               int `json:"lines_checked"`
	LinesNotNeeded             int `json:"lines_not_needed"`
	FreshLines                 int `json:"fresh_lines"`
	CacheLines                 int `json:"cache_lines"`
	SearchOnlyLines            int `json:"search_only_lines"`
	SnapshotReplayLines        int `json:"snapshot_replay_lines"`
	MissingEvidenceLines       int `json:"missing_evidence_lines"`
	UnavailableLines           int `json:"unavailable_lines"`
	AvailabilityUnknownLines   int `json:"availability_unknown_lines"`
	LinesWithPrice             int `json:"lines_with_price"`
	LinesWithPackageEvidence   int `json:"lines_with_package_evidence"`
	LinesWithOfferEvidence     int `json:"lines_with_offer_evidence"`
	LinesWithNutritionEvidence int `json:"lines_with_nutrition_evidence"`
	LinesWithImageEvidence     int `json:"lines_with_image_evidence"`
	PriceChangedLines          int `json:"price_changed_lines"`
	OfferChangedLines          int `json:"offer_changed_lines"`
	PackageChangedLines        int `json:"package_changed_lines"`
	NutritionChangedLines      int `json:"nutrition_changed_lines"`
}

type ProductEvidenceLine struct {
	ProductID            string                    `json:"product_id,omitempty"`
	SKU                  string                    `json:"sku,omitempty"`
	ProductName          string                    `json:"product_name,omitempty"`
	BasketLineQuantity   int                       `json:"basket_line_quantity,omitempty"`
	IngredientKeys       []string                  `json:"ingredient_keys,omitempty"`
	IngredientNames      []string                  `json:"ingredient_names,omitempty"`
	EvidenceSource       string                    `json:"evidence_source"`
	EvidenceStatus       string                    `json:"evidence_status"`
	CheckedAt            string                    `json:"checked_at,omitempty"`
	EvidenceAgeSeconds   *int                      `json:"evidence_age_seconds,omitempty"`
	StoreID              string                    `json:"store_id,omitempty"`
	AvailabilityStatus   string                    `json:"availability_status,omitempty"`
	AvailabilityReason   string                    `json:"availability_reason,omitempty"`
	Price                money.Money               `json:"price,omitempty"`
	Offer                *OfferEvidence            `json:"offer,omitempty"`
	PackageEvidence      PackageEvidence           `json:"package_evidence"`
	Nutrition            *ProductNutritionEvidence `json:"nutrition,omitempty"`
	Image                ProductImageEvidence      `json:"image"`
	Drift                ProductEvidenceDrift      `json:"drift"`
	DetailEvidenceHash   string                    `json:"detail_evidence_hash,omitempty"`
	SearchEvidenceHash   string                    `json:"search_evidence_hash,omitempty"`
	ProductEvidenceError string                    `json:"product_evidence_error,omitempty"`
	BlockingIssues       []ProductEvidenceIssue    `json:"blocking_issues,omitempty"`
	Warnings             []ProductEvidenceIssue    `json:"warnings,omitempty"`
}

type ProductImageEvidence struct {
	Status        string `json:"status"`
	URL           string `json:"url,omitempty"`
	CachedPath    string `json:"cached_path,omitempty"`
	Source        string `json:"source,omitempty"`
	FailureReason string `json:"failure_reason,omitempty"`
}

type ProductEvidenceDrift struct {
	NameChanged            bool     `json:"name_changed"`
	PriceChanged           bool     `json:"price_changed"`
	OfferChanged           bool     `json:"offer_changed"`
	PackageChanged         bool     `json:"package_changed"`
	NutritionChanged       bool     `json:"nutrition_changed"`
	ImageChanged           bool     `json:"image_changed"`
	PreviousPriceCents     *int     `json:"previous_price_cents,omitempty"`
	RefreshedPriceCents    *int     `json:"refreshed_price_cents,omitempty"`
	PreviousPackageText    string   `json:"previous_package_text,omitempty"`
	RefreshedPackageText   string   `json:"refreshed_package_text,omitempty"`
	PreviousNutritionHash  string   `json:"previous_nutrition_hash,omitempty"`
	RefreshedNutritionHash string   `json:"refreshed_nutrition_hash,omitempty"`
	Notes                  []string `json:"notes,omitempty"`
}

type ProductEvidenceIssue struct {
	Code           string `json:"code"`
	Severity       string `json:"severity"`
	ProductID      string `json:"product_id,omitempty"`
	ProductName    string `json:"product_name,omitempty"`
	IngredientKey  string `json:"ingredient_key,omitempty"`
	IngredientName string `json:"ingredient_name,omitempty"`
	Message        string `json:"message"`
	Remediation    string `json:"remediation,omitempty"`
}
