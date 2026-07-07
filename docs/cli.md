# Carrito CLI

Unofficial, agent-friendly CLI for Alcampo Spain. It builds a single static Go binary named `carrito` for product search, pantry-aware meal planning, recipes, transparent shopping selection, product/category inspection, batch lookup, basket price totals, and guarded cart/checkout workflows.

This project is not affiliated with Alcampo or Auchan. It uses private web APIs that may change or block requests.

## Install

With Go 1.25 or newer:

```sh
go install github.com/wachtermar/carrito/cmd/carrito@latest
carrito --help
carrito version
```

From a checkout:

```sh
make build
./carrito --help
```

No runtime dependencies are required after build. Configuration lives in `~/.carrito/config.toml`; set `CARRITO_CONFIG_DIR` to override it.

## Agent Skill Install

This repository includes a portable Agent Skill for Hermes Agent and OpenClaw at `skills/carrito-shopping`.

From the repository root:

```sh
./install-skill.sh
```

That copies the skill into `~/.hermes/skills/carrito-shopping` and `~/.openclaw/skills/carrito-shopping`, builds `carrito` into `~/.local/bin`, then runs `carrito login-web --if-needed` so desktop users can authenticate in a browser when no session exists. Use `./install-skill.sh --no-login` or `CARRITO_INSTALL_LOGIN=0 ./install-skill.sh` to skip the login check.

Install the published skill from GitHub:

```sh
hermes skills install wachtermar/carrito/skills/carrito-shopping
openclaw skills install git:wachtermar/carrito@main
```

## Build

```sh
make build
./carrito --help
```

Run `make test` and `make lint` before release.

## Examples

Before any price or availability read, set the market that serves your delivery address:

```sh
./carrito set-market --region-id ac90d761-9d58-4918-a37d-dd14e1ce384a --retailer-region-id 5 --name Vaguada
./carrito market
./carrito search leche --limit 3
./carrito search leche --limit 3 --json
./carrito product 54178 --json
./carrito categories
./carrito categories --id OC16 --json
printf 'leche\narroz\nagua\n' | ./carrito batch -f -
printf '54178 2\n205192 1\n' > basket.txt
./carrito total -f basket.txt --json
```

Food planning stores local memory under `CARRITO_CONFIG_DIR/food/` or `~/.carrito/food/`. Recipe data lives in SQLite at `food/recipes.db`; profile, pantry, meal plan, and history files remain transparent JSON/JSONL:

```sh
./carrito food profile set --people 2 --selection-policy balanced --budget 80 --json
./carrito food staples add milk --min 2 --unit l --search leche --json
./carrito food pantry add rice --qty 500 --unit g --location pantry --json
./carrito food pantry add "chicken breast" --qty 350 --unit g --location fridge --expiry 2026-07-07 --json
./carrito food pantry list --expiring-days 3 --json
./carrito food use-up --expiring-days 3 --json
./carrito food recipes list --tag quick --json
./carrito food recipes list --query chickpea --diet vegan --limit 5 --json
./carrito food recipes search "quick rice" --profile --json
./carrito food recipes show spanish-tortilla --json
printf '2 pechugas de pollo, 1 cebolla, 200 g arroz' | ./carrito food recipes add - --from-text --title "Pollo rapido" --json
./carrito food run --days 3 --people 2 --meals dinner --selection-policy balanced --basket-out basket.txt --json
./carrito food plan --days 3 --people 2 --meals dinner --json
./carrito food shop <mealplan-id-or-file> --basket-out basket.txt --json
./carrito food recipe "quick vegetarian pasta" --people 2 --json --out recipe.json
./carrito food pdf recipe.json --out recipe.pdf
./carrito food receive shop.json --json
./carrito food import-receipt --file receipt.txt --json
./carrito food import-orders --limit 5 --infer-staples --json
./carrito food cook <mealplan-id-or-file> --rating 5 --json
./carrito food history list --json
```

`food run` is the low-intervention path from profile/pantry to meal plan to shopped Alcampo products. `food plan` ranks meals using pantry/fridge matches, expiring items, liked cuisines, liked recipes, and rejected recipes, then adds low-stock staples from `profile.json`. Meal plans and recipes include estimated nutrition summaries when available, and saved `--nutrition-goals` can add warning notes. `food use-up` ranks recipes that consume pantry items expiring soon. `food shop` uses the saved `selection_policy` from `profile.json`, or a `--selection-policy balanced|cheapest|quality` override that is remembered, and respects liked/rejected product memory. It returns selected product images, offers, prices, grouped shopping sections with subtotals, package-count calculations, reasons, alternates, basket lines, nutrition, and an estimated total before any cart mutation. `--basket-out basket.txt` writes the guarded cart-prep file directly. After the user confirms purchase or delivery, `food receive` imports a saved shop/run JSON into pantry memory. `food import-receipt` parses plain-text receipts into pantry items, while `food import-orders` best-effort imports past Alcampo order lines from an authenticated session. `food cook` updates pantry quantities, appends cooking history, learns liked recipes from high ratings, and learns rejected recipes from low ratings.

The editable recipe library is stored in `~/.carrito/food/recipes.db` or `CARRITO_CONFIG_DIR/food/recipes.db`. The database has normalized recipe, tag, ingredient, equipment, step, substitution, allergen-note, nutrition, and FTS tables so large libraries can be searched and filtered without scanning per-recipe files. Embedded seed recipes are inserted automatically, and user recipes with the same `id` override embedded seeds. Legacy JSON files in `food/recipes/*.json` are imported once into SQLite for migration; new custom recipes should be added with `food recipes add`.

With an imported Alcampo web session:

```sh
./carrito addresses --json
./carrito set-address <delivery_destination_id>
./carrito checkout addresses --json
./carrito checkout slots --json
./carrito cart get --json
./carrito cart add <product_id_or_sku> 1 --max 20
./carrito checkout select-slot --slot <slot_id> --max 50
```

Basket format:

```text
<product_id_or_sku> <qty> # optional comment
54178 2
205192 1
```

`total` uses exact cent math and supports fractional quantities.

## JSON

Every read command supports `--json`. Example shape:

```json
[
  {
    "sku": "54178",
    "name": "AUCHAN Leche entera de vaca 6 x 1 l Producto Alcampo.",
    "price": {"amount": "5.76", "currency": "EUR", "cents": 576},
    "unit_price": {"amount": "0.96", "currency": "EUR", "cents": 96},
    "unit": "litre",
    "available": true
  }
]
```

Logs, warnings, and errors go to stderr. Data goes to stdout. Errors exit nonzero.

Food commands also emit stable JSON for agent workflows. Meal plans include recipes, pantry usage, required purchases, and the saved file path. Shopping results include one selected product per required purchase, product image URLs when available, offer data, package quantities, `selection_reason`, `quantity_reason`, `alternates_considered`, `basket_lines`, and `estimated_total`.

## Location

Search, product price merging, category product listing, batch, and total require an explicit market. This avoids silently pricing against the wrong store when availability varies by delivery address.

The anonymous region discovered during development was Vaguada:

```toml
[defaults]
region_id = "ac90d761-9d58-4918-a37d-dd14e1ce384a"
retailer_region_id = "5"
region_name = "Vaguada"
```

Use `--store <regionId>` or `--market <regionId>` to override a single read request with a known region id. `carrito set-postal <postal_code>` saves the postal code hint, but anonymous postal-to-region resolution is blocked outside a full web session, so it does not pretend to change regions.

For authenticated accounts, log in directly or import an existing web session:

```sh
./carrito login --username you@example.com
./carrito import-curl --file copied-curl.txt
./carrito addresses --json
./carrito set-address <delivery_destination_id>
```

`set-address` fetches the delivery destination, saves its resolved region id, and makes future searches use that market.

## Spending Guard

Commands that calculate or would write monetary state support the spending guard model:

- `--max <eur>`
- `CARRITO_MAX_EUR`
- `[limits] max_eur` in `config.toml`

`total --max 20` fails if the verified basket total is above the cap. Cart and checkout writes require a nonzero spending guard and fail closed when the active cart total cannot be verified.

## Auth

The CLI never accepts passwords as flags and never stores the password. Direct login uses the same HTTPS OAuth/Visualforce flow exposed by the site:

```sh
./carrito login --username you@example.com
./carrito login-web --if-needed
printf '%s\n' "$CARRITO_PASSWORD" | ./carrito login --username "$CARRITO_USERNAME" --password-stdin
CARRITO_USERNAME=you@example.com ./carrito login --json
./carrito whoami --json
```

`login-web --if-needed` starts a temporary `127.0.0.1` browser form only when no session exists, opens it automatically, uses the entered password once, and shuts down after the session is saved. This works well from desktop agents because the credential fields live in the user's browser rather than the chat or terminal transcript. The terminal `login` form prompts for the password without echo when run interactively. For automation, use `CARRITO_PASSWORD` from the runner secret manager or pipe it with `--password-stdin`.

Session import remains available:

```sh
./carrito import-har --file carrito.har
./carrito import-curl --file copied-curl.txt
./carrito import-curl --clipboard
pbpaste | ./carrito import-curl --file -
./carrito whoami --json
```

Only cookies, bearer tokens, CSRF tokens, customer/visitor ids, delivery-destination ids, and location hints are stored. When secrets are present, `config.toml` is written with mode `0600`.

No automatic refresh is implemented because no safe refresh call was verified. Run `login` or re-import when the web session expires. The CLI does not bypass CAPTCHA, MFA, disabled-account, consent, or risk checks; if the site requires one of those, login fails closed with the returned error.

For agent runners such as Hermes-style or OpenClaw-style task agents, keep auth material separate from prompts and source code. A session-import bootstrap is:

```sh
export CARRITO_CONFIG_DIR="$PWD/.carrito-agent"
printf '%s' "$CARRITO_CURL" | ./carrito import-curl --file -
./carrito set-address <delivery_destination_id>
./carrito whoami --json
```

Or use direct login from runner secrets:

```sh
export CARRITO_CONFIG_DIR="$PWD/.carrito-agent"
printf '%s\n' "$CARRITO_PASSWORD" | ./carrito login --username "$CARRITO_USERNAME" --password-stdin
./carrito set-address <delivery_destination_id>
./carrito whoami --json
```

Store `CARRITO_CURL`, `CARRITO_USERNAME`, and `CARRITO_PASSWORD` only in the runner's secret manager.

## Cart and Checkout

Supported with a logged-in or imported session:

```sh
./carrito cart get --json
./carrito cart get --json --raw
./carrito cart add <product_id_or_sku> <qty> --max <eur>
./carrito cart set <product_id_or_sku> <qty> --max <eur>
./carrito cart set-many -f basket.txt --max <eur>
./carrito cart clear --yes --max <eur>
./carrito checkout addresses
./carrito checkout slots
./carrito checkout create --max <eur>
./carrito checkout select-slot --slot <slot_id> --max <eur>
```

`cart get --json` returns a normalized cart summary for agents: item count, total, and line items with product id/SKU, name, brand, quantity, package price, unit price, line total, size, category, product URL, image URLs, offers, and availability. Use `cart get --json --raw` only when debugging Alcampo private-API response drift.

`checkout slots` prints slot ids in human output. Use one of those ids with `checkout select-slot` to reserve the slot for the configured delivery address and market.

All writes require:

- an imported cookie or bearer token
- an imported `x-csrf-token`
- a verified active cart total
- a nonzero `--max`, `CARRITO_MAX_EUR`, or `[limits] max_eur`

`checkout confirm-slot -f <json> --max <eur>` exists only for sending an explicit confirmation JSON body returned by the reservation flow. The CLI does not guess that payload.

Payment and order submission remain disabled. No fake payment or order-placement endpoint is implemented.

## Troubleshooting

- `403`: the web app may have changed headers, blocked a page, or required an authenticated web session. Try again later or import your own session.
- Prices differ from the website: check region/store context and use `--store <regionId>` if known.
- `no market set`: run `set-market` or import a HAR/cURL that contains a region id before searching.
- Product detail is partial: listing data is still returned when a product detail page or detail field is unavailable.
- Empty batch/total result: confirm that each line is a searchable term or SKU and that the site returns it for the current region.

## Development

```sh
go test ./...
go vet ./...
go build -o carrito ./cmd/carrito
make live-test
```

Normal tests use fixtures and do not hit the live site. `make live-test` intentionally calls public read endpoints.

Authenticated live checks are opt-in and use only the CLI/direct HTTP implementation:

```sh
CARRITO_LIVE_AUTH=1 make auth-live-test
CARRITO_LIVE_AUTH=1 CARRITO_TEST_ADD_REF=947535 CARRITO_TEST_MAX_EUR=20 make auth-live-test
CARRITO_LIVE_AUTH=1 CARRITO_TEST_SLOT_ID=<slot_id> CARRITO_TEST_MAX_EUR=50 make auth-live-test
```

Run `import-curl`/`import-har` and `set-address` first. The optional `CARRITO_TEST_ADD_REF` and `CARRITO_TEST_SLOT_ID` variables intentionally mutate cart/slot state but never submit an order.

For local fixture tests or controlled mock servers, `CARRITO_BASE_URL` overrides the default Alcampo origin. Do not set it for normal use.
