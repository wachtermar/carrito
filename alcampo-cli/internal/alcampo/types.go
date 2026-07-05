package alcampo

import "alcampo-cli/internal/money"

type Product struct {
	ID                string      `json:"id,omitempty"`
	SKU               string      `json:"sku,omitempty"`
	Name              string      `json:"name,omitempty"`
	Brand             string      `json:"brand,omitempty"`
	Price             money.Money `json:"price,omitempty"`
	UnitPrice         money.Money `json:"unit_price,omitempty"`
	Unit              string      `json:"unit,omitempty"`
	Size              string      `json:"size,omitempty"`
	URL               string      `json:"url,omitempty"`
	Category          string      `json:"category,omitempty"`
	CategoryPath      []string    `json:"category_path,omitempty"`
	Offers            []Offer     `json:"offers,omitempty"`
	Available         *bool       `json:"available,omitempty"`
	Images            []string    `json:"images,omitempty"`
	EAN               string      `json:"ean,omitempty"`
	Description       string      `json:"description,omitempty"`
	Ingredients       string      `json:"ingredients,omitempty"`
	Allergens         string      `json:"allergens,omitempty"`
	Nutrition         string      `json:"nutrition,omitempty"`
	DetailUnavailable bool        `json:"detail_unavailable,omitempty"`
	DetailMessage     string      `json:"detail_message,omitempty"`
}

type Offer struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
}

type Category struct {
	ID         string     `json:"id,omitempty"`
	RetailerID string     `json:"retailer_id,omitempty"`
	Slug       string     `json:"slug,omitempty"`
	Name       string     `json:"name,omitempty"`
	Path       string     `json:"path,omitempty"`
	Children   []Category `json:"children,omitempty"`
}

type SearchOptions struct {
	Limit    int
	RegionID string
	Sort     string
	Fresh    bool
}

type CategoryProductsOptions struct {
	Limit    int
	RegionID string
}
