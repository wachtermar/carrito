package alcampo

import (
	"encoding/json"
	"strings"

	"github.com/wachtermar/carrito/internal/money"
)

type CartSummary struct {
	ItemCount int               `json:"item_count"`
	Total     money.Money       `json:"total,omitempty"`
	Items     []CartItemSummary `json:"items"`
}

type CartItemSummary struct {
	ProductID string      `json:"product_id,omitempty"`
	SKU       string      `json:"sku,omitempty"`
	Name      string      `json:"name,omitempty"`
	Brand     string      `json:"brand,omitempty"`
	Quantity  string      `json:"quantity"`
	Price     money.Money `json:"price,omitempty"`
	UnitPrice money.Money `json:"unit_price,omitempty"`
	Unit      string      `json:"unit,omitempty"`
	LineTotal money.Money `json:"line_total,omitempty"`
	Size      string      `json:"size,omitempty"`
	Category  string      `json:"category,omitempty"`
	URL       string      `json:"url,omitempty"`
	ImageURL  string      `json:"image_url,omitempty"`
	Images    []string    `json:"images,omitempty"`
	Offers    []Offer     `json:"offers,omitempty"`
	Available *bool       `json:"available,omitempty"`
}

func SummarizeCart(root any, baseURL string) CartSummary {
	summary := CartSummary{Items: collectCartItems(root, baseURL)}
	if total, ok := CartTotalCents(root); ok {
		summary.Total = money.Money{Amount: money.FormatAmount(total), Currency: "EUR", Cents: total}
	}
	summary.ItemCount = len(summary.Items)
	if summary.ItemCount == 0 {
		summary.ItemCount = CartItemCount(root)
	}
	return summary
}

func CartSummaryProductIDs(summary CartSummary) []string {
	ids := make([]string, 0, len(summary.Items))
	for _, item := range summary.Items {
		ids = append(ids, item.ProductID)
	}
	return uniqueNonEmpty(ids)
}

func EnrichCartSummary(summary *CartSummary, products []Product) {
	if summary == nil || len(products) == 0 {
		return
	}
	byID := map[string]Product{}
	for _, product := range products {
		if product.ID == "" {
			continue
		}
		byID[strings.ToLower(product.ID)] = product
	}
	for i := range summary.Items {
		product, ok := byID[strings.ToLower(summary.Items[i].ProductID)]
		if !ok {
			continue
		}
		enrichCartItem(&summary.Items[i], product)
	}
}

func enrichCartItem(item *CartItemSummary, product Product) {
	if item.ProductID == "" {
		item.ProductID = product.ID
	}
	if item.SKU == "" {
		item.SKU = product.SKU
	}
	if item.Name == "" {
		item.Name = product.Name
	}
	if item.Brand == "" {
		item.Brand = product.Brand
	}
	if item.Price.Amount == "" {
		item.Price = product.Price
	}
	if item.UnitPrice.Amount == "" {
		item.UnitPrice = product.UnitPrice
	}
	if item.Unit == "" {
		item.Unit = product.Unit
	}
	if item.Size == "" {
		item.Size = product.Size
	}
	if item.Category == "" {
		item.Category = product.Category
	}
	if item.URL == "" {
		item.URL = product.URL
	}
	if len(item.Images) == 0 {
		item.Images = product.Images
	}
	if item.ImageURL == "" && len(item.Images) > 0 {
		item.ImageURL = item.Images[0]
	}
	if len(item.Offers) == 0 {
		item.Offers = product.Offers
	}
	if item.Available == nil {
		item.Available = product.Available
	}
}

func collectCartItems(root any, baseURL string) []CartItemSummary {
	var items []CartItemSummary
	seen := map[string]bool{}
	for _, m := range cartLineMaps(root) {
		item, ok := cartItemFromMap(m, baseURL)
		if !ok {
			continue
		}
		key := cartItemKey(item, m)
		if seen[key] {
			continue
		}
		seen[key] = true
		items = append(items, item)
	}
	return items
}

func cartItemFromMap(m map[string]any, baseURL string) (CartItemSummary, bool) {
	qty := quantityFromMap(m)
	if qty == "" {
		return CartItemSummary{}, false
	}
	product, err := cartLineProductFromMap(m, baseURL)
	if err != nil {
		return CartItemSummary{}, false
	}
	if product.ID == "" && product.SKU == "" && product.Name == "" {
		return CartItemSummary{}, false
	}
	lineTotal := cartLineTotal(m)
	if lineTotal.Amount == "" && product.Price.Amount != "" {
		if cents, err := money.MultiplyCentsByQuantity(product.Price.Cents, qty); err == nil {
			currency := product.Price.Currency
			if currency == "" {
				currency = "EUR"
			}
			lineTotal = money.Money{Amount: money.FormatAmount(cents), Currency: currency, Cents: cents}
		}
	}
	item := CartItemSummary{
		ProductID: product.ID,
		SKU:       product.SKU,
		Name:      product.Name,
		Brand:     product.Brand,
		Quantity:  qty,
		Price:     product.Price,
		UnitPrice: product.UnitPrice,
		Unit:      product.Unit,
		LineTotal: lineTotal,
		Size:      product.Size,
		Category:  product.Category,
		URL:       product.URL,
		Images:    product.Images,
		Offers:    product.Offers,
		Available: product.Available,
	}
	if len(item.Images) > 0 {
		item.ImageURL = item.Images[0]
	}
	return item, true
}

func cartProductFromMap(m map[string]any, baseURL string) Product {
	var product Product
	for _, key := range []string{"product", "decoratedProduct", "bopData", "productData", "article"} {
		if child, ok := m[key].(map[string]any); ok && isProductMap(child) {
			mergeProduct(&product, productFromMap(child, baseURL))
		}
	}
	if isProductMap(m) {
		mergeProduct(&product, productFromMap(m, baseURL))
	}
	if product.ID == "" {
		product.ID = stringFromKeys(m, "productId", "productID", "id")
	}
	if product.SKU == "" {
		product.SKU = stringFromKeys(m, "retailerProductId", "retailerProductID", "sku", "retailerSku")
	}
	if product.Name == "" {
		product.Name = stringFromKeys(m, "name", "displayName", "productName", "title")
	}
	if product.Brand == "" {
		product.Brand = brandFrom(mapValue(m, "brand"))
	}
	if product.Price.Amount == "" {
		product.Price = priceFrom(mapValue(m, "productPrice", "salesPrice", "fopPrice", "price"))
	}
	if product.UnitPrice.Amount == "" {
		product.UnitPrice, product.Unit = unitPriceFrom(mapValue(m, "unitPrice", "pricePerUnit", "referencePrice"))
	}
	if product.Size == "" {
		product.Size = stringFromKeys(m, "size", "format", "packSize", "packSizeDescription", "netContent")
	}
	if product.Category == "" {
		product.Category = stringFromKeys(m, "category", "categoryName")
	}
	if product.URL == "" {
		product.URL = absoluteProductURL(baseURL, stringFromKeys(m, "url", "productUrl", "canonicalUrl", "href"), product.SKU)
	}
	if len(product.Images) == 0 {
		product.Images = imageURLs(baseURL, mapValue(m, "images", "image", "media"))
	}
	if len(product.Offers) == 0 {
		product.Offers = offersFrom(mapValue(m, "promotions", "offers", "badges"))
	}
	return product
}

func cartLineTotal(m map[string]any) money.Money {
	for _, key := range []string{"lineTotal", "line_total", "totalPrice", "total_price", "subtotal", "subTotal", "itemTotal", "item_total", "rowTotal", "row_total"} {
		if p := priceFrom(mapValue(m, key)); p.Amount != "" {
			return p
		}
	}
	for _, container := range []string{"display", "totals"} {
		if child := mapFromKeys(m, container); child != nil {
			for _, key := range []string{"itemPriceAfterPromos", "lineTotal", "totalPrice", "subtotal", "total"} {
				if p := priceFrom(mapValue(child, key)); p.Amount != "" {
					return p
				}
			}
		}
	}
	return money.Money{}
}

func cartItemKey(item CartItemSummary, raw map[string]any) string {
	key := strings.ToLower(strings.TrimSpace(item.ProductID + "|" + item.SKU + "|" + item.Quantity + "|" + item.Name))
	if key != "|||" {
		return key
	}
	data, _ := json.Marshal(raw)
	return string(data)
}
