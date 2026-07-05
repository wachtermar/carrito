# alcampo-cli

Unofficial, agent-friendly CLI for Alcampo Spain. It builds a single static Go binary named `alcampo` for product search, pantry-aware meal planning, recipes, transparent shopping selection, product/category inspection, batch lookup, basket price totals, and guarded cart/checkout workflows.

This project is not affiliated with Alcampo or Auchan. It uses private web APIs that may change or block requests.

## Agent Skill Install

This repository includes a portable Agent Skill for Hermes Agent and OpenClaw at `../skills/alcampo-shopping`.

From the repository root:

```sh
./install-skill.sh
```

That copies the skill into `~/.hermes/skills/alcampo-shopping` and `~/.openclaw/skills/alcampo-shopping`, then builds `alcampo` into `~/.local/bin`.

After publishing this repository, use:

```sh
openclaw skills install git:OWNER/REPO
hermes skills install OWNER/REPO/skills/alcampo-shopping
```

## Build

```sh
make build
./alcampo --help
```

No runtime dependencies are required after build. Configuration lives in `~/.alcampo/config.toml`; set `ALCAMPO_CONFIG_DIR` to override it.

## Examples

Before any price or availability read, set the market that serves your delivery address:

```sh
./alcampo set-market --region-id ac90d761-9d58-4918-a37d-dd14e1ce384a --retailer-region-id 5 --name Vaguada
./alcampo market
./alcampo search leche --limit 3
./alcampo search leche --limit 3 --json
./alcampo product 54178 --json
./alcampo categories
./alcampo categories --id OC16 --json
printf 'leche\narroz\nagua\n' | ./alcampo batch -f -
printf '54178 2\n205192 1\n' > basket.txt
./alcampo total -f basket.txt --json
```

Food planning stores transparent local JSON under `ALCAMPO_CONFIG_DIR/food/` or `~/.alcampo/food/`:

```sh
./alcampo food profile set --people 2 --selection-policy balanced --budget 80 --json
./alcampo food staples add milk --min 2 --unit l --search leche --json
./alcampo food pantry add rice --qty 500 --unit g --location pantry --json
./alcampo food pantry add "chicken breast" --qty 350 --unit g --location fridge --expiry 2026-07-07 --json
./alcampo food run --days 3 --people 2 --meals dinner --selection-policy balanced --basket-out basket.txt --json
./alcampo food plan --days 3 --people 2 --meals dinner --json
./alcampo food shop <mealplan-id-or-file> --basket-out basket.txt --json
./alcampo food recipe "quick vegetarian pasta" --people 2 --json --out recipe.json
./alcampo food pdf recipe.json --out recipe.pdf
./alcampo food receive shop.json --json
./alcampo food cook <mealplan-id-or-file> --rating 5 --json
./alcampo food history list --json
```

`food run` is the low-intervention path from profile/pantry to meal plan to shopped Alcampo products. `food plan` ranks meals using pantry/fridge matches, expiring items, liked cuisines, liked recipes, and rejected recipes, then adds low-stock staples from `profile.json`. `food shop` uses the saved `selection_policy` from `profile.json`, or a `--selection-policy balanced|cheapest|quality` override that is remembered, and respects liked/rejected product memory. It returns selected product images, offers, prices, grouped shopping sections with subtotals, package-count calculations, reasons, alternates, basket lines, and an estimated total before any cart mutation. `--basket-out basket.txt` writes the guarded cart-prep file directly. After the user confirms purchase or delivery, `food receive` imports a saved shop/run JSON into pantry memory. `food cook` updates pantry quantities, appends cooking history, learns liked recipes from high ratings, and learns rejected recipes from low ratings.

With an imported Alcampo web session:

```sh
./alcampo addresses --json
./alcampo set-address <delivery_destination_id>
./alcampo checkout addresses --json
./alcampo checkout slots --json
./alcampo cart get --json
./alcampo cart add <product_id_or_sku> 1 --max 20
./alcampo checkout select-slot --slot <slot_id> --max 50
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

Use `--store <regionId>` or `--market <regionId>` to override a single read request with a known region id. `alcampo set-postal <postal_code>` saves the postal code hint, but anonymous postal-to-region resolution is blocked outside a full web session, so it does not pretend to change regions.

For authenticated accounts, log in directly or import an existing web session:

```sh
./alcampo login --username you@example.com
./alcampo import-curl --file copied-curl.txt
./alcampo addresses --json
./alcampo set-address <delivery_destination_id>
```

`set-address` fetches the delivery destination, saves its resolved region id, and makes future searches use that market.

## Spending Guard

Commands that calculate or would write monetary state support the spending guard model:

- `--max <eur>`
- `ALCAMPO_MAX_EUR`
- `[limits] max_eur` in `config.toml`

`total --max 20` fails if the verified basket total is above the cap. Cart and checkout writes require a nonzero spending guard and fail closed when the active cart total cannot be verified.

## Auth

The CLI never accepts passwords as flags and never stores the password. Direct login uses the same HTTPS OAuth/Visualforce flow exposed by the site:

```sh
./alcampo login --username you@example.com
printf '%s\n' "$ALCAMPO_PASSWORD" | ./alcampo login --username "$ALCAMPO_USERNAME" --password-stdin
ALCAMPO_USERNAME=you@example.com ./alcampo login --json
./alcampo whoami --json
```

The first form prompts for the password without echo when run in an interactive terminal. For automation, use `ALCAMPO_PASSWORD` from the runner secret manager or pipe it with `--password-stdin`.

Session import remains available:

```sh
./alcampo import-har --file alcampo.har
./alcampo import-curl --file copied-curl.txt
./alcampo import-curl --clipboard
pbpaste | ./alcampo import-curl --file -
./alcampo whoami --json
```

Only cookies, bearer tokens, CSRF tokens, customer/visitor ids, delivery-destination ids, and location hints are stored. When secrets are present, `config.toml` is written with mode `0600`.

No automatic refresh is implemented because no safe refresh call was verified. Run `login` or re-import when the web session expires. The CLI does not bypass CAPTCHA, MFA, disabled-account, consent, or risk checks; if the site requires one of those, login fails closed with the returned error.

For agent runners such as Hermes-style or OpenClaw-style task agents, keep auth material separate from prompts and source code. A session-import bootstrap is:

```sh
export ALCAMPO_CONFIG_DIR="$PWD/.alcampo-agent"
printf '%s' "$ALCAMPO_CURL" | ./alcampo import-curl --file -
./alcampo set-address <delivery_destination_id>
./alcampo whoami --json
```

Or use direct login from runner secrets:

```sh
export ALCAMPO_CONFIG_DIR="$PWD/.alcampo-agent"
printf '%s\n' "$ALCAMPO_PASSWORD" | ./alcampo login --username "$ALCAMPO_USERNAME" --password-stdin
./alcampo set-address <delivery_destination_id>
./alcampo whoami --json
```

Store `ALCAMPO_CURL`, `ALCAMPO_USERNAME`, and `ALCAMPO_PASSWORD` only in the runner's secret manager.

## Cart and Checkout

Supported with a logged-in or imported session:

```sh
./alcampo cart get --json
./alcampo cart add <product_id_or_sku> <qty> --max <eur>
./alcampo cart set <product_id_or_sku> <qty> --max <eur>
./alcampo cart set-many -f basket.txt --max <eur>
./alcampo cart clear --yes --max <eur>
./alcampo checkout addresses
./alcampo checkout slots
./alcampo checkout create --max <eur>
./alcampo checkout select-slot --slot <slot_id> --max <eur>
```

`checkout slots` prints slot ids in human output. Use one of those ids with `checkout select-slot` to reserve the slot for the configured delivery address and market.

All writes require:

- an imported cookie or bearer token
- an imported `x-csrf-token`
- a verified active cart total
- a nonzero `--max`, `ALCAMPO_MAX_EUR`, or `[limits] max_eur`

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
go build -o alcampo ./cmd/alcampo
make live-test
```

Normal tests use fixtures and do not hit the live site. `make live-test` intentionally calls public read endpoints.

Authenticated live checks are opt-in and use only the CLI/direct HTTP implementation:

```sh
ALCAMPO_LIVE_AUTH=1 make auth-live-test
ALCAMPO_LIVE_AUTH=1 ALCAMPO_TEST_ADD_REF=947535 ALCAMPO_TEST_MAX_EUR=20 make auth-live-test
ALCAMPO_LIVE_AUTH=1 ALCAMPO_TEST_SLOT_ID=<slot_id> ALCAMPO_TEST_MAX_EUR=50 make auth-live-test
```

Run `import-curl`/`import-har` and `set-address` first. The optional `ALCAMPO_TEST_ADD_REF` and `ALCAMPO_TEST_SLOT_ID` variables intentionally mutate cart/slot state but never submit an order.

For local fixture tests or controlled mock servers, `ALCAMPO_BASE_URL` overrides the default Alcampo origin. Do not set it for normal use.
