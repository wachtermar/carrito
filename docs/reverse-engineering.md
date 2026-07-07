# Reverse Engineering Notes

Last checked: 2026-07-04.

This project uses only normal HTTPS requests that the public Alcampo web app also uses. It does not bypass CAPTCHA, anti-bot controls, login risk checks, payment, or checkout protections.

## Site Shape

- `https://www.alcampo.es/` redirects into `https://www.compraonline.alcampo.es/compra-online/`.
- The active shop origin is `https://www.compraonline.alcampo.es`.
- The homepage sets anonymous cookies such as `VISITORID`, `AWSALB`, `AWSALBCORS`, and `global_sid`.
- Public read data is exposed through OSP-style REST endpoints under `/api/...`.
- The web app sends `ecom-request-source: web` and `ecom-request-source-version`. A checked version was `2.0.0-2026-07-02-08h35m10s-4a524088`; the CLI keeps this as a default and tries to refresh it from the homepage.

## Anonymous Region

The anonymous listing/search responses tested from Spain used:

- `regionId`: `ac90d761-9d58-4918-a37d-dd14e1ce384a`
- `retailerRegionId`: `5`
- `regionName`: `Vaguada`

Prices and availability can depend on region/store. The CLI now requires an explicit market for price/availability reads instead of silently using this discovered anonymous region. Use `set-market`, `--store`/`--market`, or import an Alcampo web session that contains region data.

## Search

Endpoint:

```text
GET /api/webproductpagews/v6/product-pages/search
```

Useful query parameters:

- `q=<term>`
- `tag=web`
- `includeAdditionalPageInfo=true`
- `maxProductsToDecorate=<n>`
- `maxPageSize=<n>`
- `regionId=<uuid>`
- `sortOptionId=<id>` if the caller wants to pass a site sort id

The response includes `productGroups[].decoratedProducts[]`. Products include stable listing data such as internal product id, retailer product id/SKU, name, brand, price, unit/reference price, image URLs, category path, promotions, and availability.

Example verified query:

```text
GET /api/webproductpagews/v6/product-pages/search?q=leche&tag=web&includeAdditionalPageInfo=true&maxProductsToDecorate=50&maxPageSize=50&regionId=ac90d761-9d58-4918-a37d-dd14e1ce384a
```

## Categories

Endpoint:

```text
GET /api/webproductpagews/v1/categories?decoration=false&categoryDepth=3
```

This returns a nested category tree with ids, retailer ids, names, slugs/paths, and child categories.

Products in a category use:

```text
GET /api/webproductpagews/v6/product-pages
```

Useful query parameters:

- `retailerCategoryId=<id>` for retailer category ids such as `OC16`
- `categoryId=<uuid>` for internal category ids
- `tag=web`
- `includeAdditionalPageInfo=true`
- `maxProductsToDecorate=<n>`
- `maxPageSize=<n>`
- `regionId=<uuid>`

## Product Details

Direct product decoration works with the same web-app style headers used by the CLI:

```text
PUT /api/webproductpagews/v6/products?regionId=<uuid>
```

The JSON body is an array of internal `productId` strings. The response includes `products[]` with internal product id, retailer product id/SKU, name, brand, pack size, price, unit/reference price, image URLs/srcsets, availability, and basket quantity when applicable. The CLI uses this after cart reads because the active cart view can return only internal product ids, prices, quantities, and totals.

The public product page works for retailer product ids:

```text
GET /products/<retailerProductId>
```

For example, `/products/54178` redirected to a canonical slug URL and returned a product page. The page contains:

- JSON-LD in a `data-test="product-details-structured-data"` script.
- `window.__QUERY_INITIAL_STATE__` with BOP detail data, including descriptions, ingredients, nutrition/allergen sections when available, breadcrumbs, media, and promotions.
- Listing data in hydration state for some pages.

The CLI uses the page data first, then merges live listing/search data for current price, unit price, category, offers, images, and availability.

## Batch and Totals

No verified multi-search endpoint was found. `batch` performs one rate-limited search per line and returns the top hit. `total` searches each basket reference, prefers exact SKU/internal id matches when available, and computes exact cents locally.

## Address and Market Discovery

The web bundle references these location endpoints:

```text
PUT /api/address/v1/addresses/by-postcode?sorted=true
PUT /api/address/v3/address-deliverability/by-address-details?countryCode=<country>
PUT /api/ecomdeliverydestinations/ecomdeliverydestinations/v2/deliverability
POST /api/ecomdeliverydestinations/ecomdeliverydestinations/v2/temporary-delivery-destinations
GET /api/ecomdeliverydestinations/ecomdeliverydestinations/v4/delivery-addresses?deliveryMethod=HOME_DELIVERY
PUT /api/customersessions/v2/sessions/active
```

The postcode resolver returned `403` outside a full web session during CLI testing, so runtime reads do not depend on it. HAR/cURL imports can still capture `regionId` and `deliveryDestinationId`; when a region id is imported the CLI marks the market as explicitly set.

Authenticated web-session testing showed `GET /api/ecomdeliverydestinations/v4/delivery-addresses/<deliveryDestinationId>` returns:

- `deliveryDestinationId`
- `postalCode`
- `deliverability`
- `deliveryMethod`
- `resolvedRegionId`
- `propositions[].regionId`

The CLI uses this through `addresses` and `set-address`.

## Checkout and Slots

Navigating to `/checkout` with no cart redirected to the delivery slot page and did not touch order submission. The web app made:

```text
GET /api/checkout-groups/v1/default-shipping-group/HOME_DELIVERY?deliveryDestinationId=<id>
POST /api/ecomslots/v2/slots
```

The slots request body included:

```json
{
  "deliveryDestinationId": "<id>",
  "regionId": "<resolvedRegionId>",
  "displayConfiguration": "DELIVERY_METHOD",
  "shippingGroupType": "Alcampo Vans",
  "numberOfDays": 7
}
```

The response includes available slot ids, start/end windows, delivery prices, attributes, and minimum order value. The CLI implements `checkout addresses` and `checkout slots` as authenticated read commands. Human slot output includes the slot id so it can be passed to `checkout select-slot`.

The bundle also exposes:

```text
POST /api/cart/v1/carts/active/checkout-start
POST /api/ecomslots/v1/slots/reservation
PUT /api/ecomslots/v1/slots/confirm
```

The slot reservation body is:

```json
{
  "regionId": "<resolvedRegionId>",
  "slotId": "<slotId>",
  "deliveryDestinationId": "<deliveryDestinationId>"
}
```

`checkout create` and `checkout select-slot` are implemented with required imported session material and a required spending guard. `checkout confirm-slot` only sends an explicit JSON body supplied by the user from a prior reservation flow; the CLI does not infer or synthesize confirmation data. `checkout submit` remains disabled.

## Auth and Writes

Direct username/password login is implemented without browser automation. The observed flow is:

```text
GET /login?destination=/
302 https://ar-customerdiamond-es.my.site.com/services/oauth2/authorize/expid_esANDloyalty...
302 /setup/secur/RemoteAccessAuthorizationPage.apexp?source=...
302 /ARCD_CIAM_Redirect?startURL=...
POST /ARCD_CIAM_Redirect
GET /authorization?language=es&startURL=...
POST /ARCD_CIAM_Login
GET /secur/frontdoor.jsp?...&retURL=/setup/secur/RemoteAccessAuthorizationPage.apexp?source=...
POST /ARCD_CIAM_Intermediary
GET /setup/secur/RemoteAccessAuthorizationPage.apexp?source=...
302 https://www.compraonline.alcampo.es/sso-login?code=...&state=...
```

`/ARCD_CIAM_Redirect` is a Salesforce Visualforce relay form with `com.salesforce.visualforce.ViewState` fields. `/authorization` returns the credential form and exposes a RichFaces/Ajax4JSF `login(username,password,startUrl,hpot)` wrapper. The CLI reproduces the same form POST directly:

```text
POST /ARCD_CIAM_Login
Content-Type: application/x-www-form-urlencoded; charset=UTF-8

AJAXREQUEST=_viewRoot
username=<email>
password=<password>
startUrl=<encoded oauth start URL>
hpot=
<form id>=<form id>
<form id>:login=<form id>:login
com.salesforce.visualforce.ViewState=...
com.salesforce.visualforce.ViewStateVersion=...
com.salesforce.visualforce.ViewStateMAC=...
```

On successful credential submission, Salesforce returns an Ajax redirect envelope (`meta name="Ajax-Response" content="redirect"` and `meta name="Location" ...`) pointing to `frontdoor.jsp`. That page then JavaScript-redirects to `/apex/ARCD_CIAM_Intermediary`, which is another Visualforce form. Submitting its `finishLF` action completes the OAuth return to Alcampo.

The password is accepted only from a no-echo terminal prompt, `CARRITO_PASSWORD`, or `--password-stdin`; it is never stored. The resulting Alcampo cookies, homepage CSRF token, customer id, visitor id, delivery destination, and market hints are saved into the same session config as imported sessions. The CLI does not bypass CAPTCHA, MFA, disabled-account, consent, or risk controls.

Supported session import:

- `import-har --file <har|->`
- `import-curl --file <curl-file|->`
- `import-curl --clipboard`

These extract only cookies, bearer tokens, CSRF tokens, customer/visitor ids, and location hints from the user's own web-session export.

The bundle exposes these cart endpoints:

```text
GET /api/cart/v1/carts/active
GET /api/cart/v2/carts/active/cart-view
POST /api/cart/v1/carts/active/apply-quantity?cartProductSorting=CATEGORIES
PUT /api/cart/v1/carts/active/select
PUT /api/cart/v1/carts/active/unselect
DELETE /api/cart/v1/carts/active/items
```

The CLI implements cart get/add/set/set-many/clear through the active-cart and apply-quantity/delete endpoints. Add and set send product quantity deltas in the form:

```json
[
  {"productId": "<internalProductId>", "quantity": 1}
]
```

`cart get` reads `cart-view`, then decorates the returned internal product ids through `PUT /api/webproductpagews/v6/products?regionId=...` so agents can show names, SKUs, images, unit prices, quantities, line totals, product URLs, and offers instead of raw cart ids.

All cart and checkout writes require logged-in/imported cookies or bearer token, CSRF token, verified active cart total, and a nonzero spending guard. Checkout payment and order submission are not implemented.
