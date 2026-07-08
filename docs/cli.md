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
./carrito food run --days 3 --people 2 --meals dinner --selection-policy balanced --servings 2 --basket-out basket.txt --run-out run.json --quantity-ledger-out ledger.json --nutrition-ledger-out nutrition_ledger.json --recovery-out recovery.json --recipe-swap-out recipe_swap.json --basket-optimization-out basket_optimization.json --serving-plan-out serving_plan.json --scaled-mealplan-out scaled_mealplan.json --pantry-out pantry.json --pantry-consumption-out pantry_consumption.json --readiness-out readiness.json --manifest-out manifest.json --audit-out audit.json --audit-mode fail --pdf-out food-plan.pdf --enrich-products --strict-quantity --strict-servings --recover-missing --allow-recipe-swap --optimize-basket --basket-objective safe-balanced --deal-aware --default-pantry minimal-spanish --allow-assumed-pantry --require-safe-basket --require-cook-ready --json
./carrito food run --days 1 --people 2 --meals dinner --run-out run.json --manifest-out manifest.json --audit-out audit.json --audit-mode fail --record-live-snapshot snapshots/alcampo-2026-07-08 --snapshot-id alcampo-2026-07-08 --json
./carrito food run --days 1 --people 2 --meals dinner --run-out replay-run.json --manifest-out replay-manifest.json --audit-out replay-audit.json --audit-mode fail --replay-live-snapshot snapshots/alcampo-2026-07-08 --snapshot-strict --json
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

`food run` is the low-intervention path from profile/pantry to meal plan to shopped Alcampo products. `--servings`, `--adult-servings`, `--child-servings`, `--toddler-servings`, or `--household-profile` can create a serving plan before pantry and shopping; `--serving-plan-out serving_plan.json` writes the serving assumptions, and `--scaled-mealplan-out scaled_mealplan.json` writes deterministic scaled recipe quantities. `--run-out run.json` writes a combined `food_run` artifact containing both the meal plan and shopping result, `--quantity-ledger-out ledger.json` writes the auditable trust layer, `--nutrition-ledger-out nutrition_ledger.json` writes the evidence-led consumed-nutrition ledger, `--recovery-out recovery.json` writes any ledger-driven recovery decisions, `--recipe-swap-out recipe_swap.json` writes validated meal-plan repair attempts, `--basket-optimization-out basket_optimization.json` writes audited product re-selection decisions when `--optimize-basket` is used, `--pantry-out pantry.json` writes the structured pantry/shopping delta, `--pantry-consumption-out pantry_consumption.json` writes pantry quantities planned for cooking, `--readiness-out readiness.json` writes the final readiness gate, and `--pdf-out food-plan.pdf` renders the PDF from the combined artifact so it includes day-by-day recipes, serving assumptions, scaled ingredients, cooking instructions, selected products, pantry assumptions/deltas, recovery actions, recipe swaps, basket optimization, product images, package math, quantity confidence, totals, nutrition evidence, readiness status, and safety caveats. The run artifact includes `serving_plan`, `scaled_mealplan`, `quantity_ledger.status` (`complete_exact`, `complete_estimated`, `incomplete`, or `needs_review`), `nutrition_ledger.status` (`no_coverage`, `partial`, `complete_for_purchased`, `complete_hybrid`, or `blocked`), `pantry_resolution`, `pantry_consumption_plan`, `pre_recipe_swap_readiness_gate` when repair was evaluated, `recipe_swap_plan` when requested or applied, `pre_optimization_readiness_gate`, `basket_optimization_plan`, `basket_safety`, and final `readiness_gate`; agents should treat final `readiness_gate.safe_to_build` as authoritative before saying a basket file is safe, final `readiness_gate.safe_to_cook` as authoritative before saying the whole meal plan is cook-ready, and final `readiness_gate.safe_to_report_nutrition` as authoritative before presenting calories/macros as evidence-backed. `--default-pantry minimal-spanish --allow-assumed-pantry` can exclude common staples such as water, salt, and pepper from Alcampo shopping while reporting them as assumptions; saved `food pantry` memory is merged into structured pantry resolution as confirmed evidence, and `--pantry-profile pantry_profile.json` can add normalized pantry items with `value`, `unit`, `base_value`, and `base_unit`; use `--require-confirmed-pantry --require-cook-ready` when assumed staples must block cooking until confirmed. `--strict-servings --require-cook-ready` blocks cooking readiness when a recipe cannot be scaled safely; `--no-serving-scaling` preserves legacy recipe quantities. `--nutrition-mode labels|recipe|hybrid|off` controls nutrition evidence; `hybrid` keeps Alcampo label, pantry-profile, and recipe-declared nutrition separate. Pantry profile nutrition is allowed by default and can be disabled with `--allow-pantry-profile-nutrition=false`; built-in pantry nutrition remains off unless `--allow-builtin-pantry-nutrition` is passed. `--require-nutrition-ready` returns exit code 20 when the nutrition ledger fails the requested coverage thresholds from `--min-nutrition-line-coverage` or `--min-nutrition-quantity-coverage`, but it does not by itself make `safe_to_build` or `safe_to_cook` false. `--include-purchased-excess-nutrition` reports package excess separately; default nutrition totals count only consumed scaled recipe quantities, not the whole package. `--optimize-basket` searches validated alternatives after recovery and recipe swaps, parses conservative Alcampo offers such as `3x2`, `2x1`, `lleva 3 paga 2`, and `2ª unidad -50%`, and applies a switch only when semantic match, quantity confidence, and readiness are preserved while savings, waste, or evidence improve. `--allow-recipe-swap` lets generated meal plans replace blocked recipe slots only after each candidate passes serving scaling, full Alcampo shopping, enrichment, ledger, recovery, and readiness validation. `--require-safe-basket` returns exit code 20 after writing diagnostic artifacts when basket readiness is blocked; `--require-cook-ready` returns exit code 20 when pantry or serving resolution leaves the meal plan not cook-ready. These are not crashes. When `safe_to_build` is false, `basket.txt` is intentionally comment-only/diagnostic and must not be used for cart mutation; when `safe_to_build` is true but `safe_to_cook` is false, the basket lines are structurally valid but do not complete the meal plan without additional pantry/serving confirmation or shopping. When `safe_to_report_nutrition` is false, nutrition may still be shown as a caveated recipe estimate or partial label ledger, but not as complete/exact. `--recover-missing` can repair missing products with deterministic same-ingredient Spanish alias searches and safe prep notes, but it rejects unsafe product-level mismatches such as minced beef for beef strips. `food plan` ranks meals using pantry/fridge matches, expiring items, liked cuisines, liked recipes, and rejected recipes, then adds low-stock staples from `profile.json`. Meal plans and recipes include estimated nutrition summaries when available, and saved `--nutrition-goals` can add warning notes. `food use-up` ranks recipes that consume pantry items expiring soon. `food shop` uses the saved `selection_policy` from `profile.json`, or a `--selection-policy balanced|cheapest|quality` override that is remembered, and respects liked/rejected product memory. It returns selected product images, offers, prices, grouped shopping sections with subtotals, package-count calculations, reasons, alternates, basket lines, parsed Alcampo label nutrition when present, nutrition warnings, and an estimated total before any cart mutation. Product-label nutrition is used only when a label is present and unit conversion is safe; missing labels and unsafe conversions are reported as best-effort warnings. `--basket-out basket.txt` writes the guarded cart-prep file directly only when basket readiness passes; blocked runs write diagnostic comments. After the user confirms purchase or delivery, `food receive` imports a saved shop/run JSON into pantry memory. `food import-receipt` parses plain-text receipts into pantry items, while `food import-orders` best-effort imports past Alcampo order lines from an authenticated session. `food cook` updates pantry quantities, appends cooking history, learns liked recipes from high ratings, and learns rejected recipes from low ratings.

`--manifest-out manifest.json` writes a generation manifest with artifact paths, hashes, sizes, readiness summary, generation exit code, and cross-stage fingerprints. `--audit-out audit.json --audit-mode fail` reopens the written run, sidecars, basket, and PDF from disk, verifies hashes, sidecar equality, fingerprint consistency, guarded basket actionability, PDF trust markers, and basic ledger/nutrition consistency, then returns exit code 30 if the bundle contradicts itself. Exit 30 means the artifacts are not trustworthy even if readiness would otherwise be exit 0 or 20. `carrito food validate-run --run run.json --manifest manifest.json --audit-out audit.json --mode hermes --audit-mode fail --pdf food-plan.pdf --basket basket.txt --json` reruns the same disk-based audit for an existing bundle. Agents should prefer `artifact_audit.hermes_trust_summary`: when `trustworthy` is false, regenerate the bundle before presenting readiness claims; when `may_present_basket_as_ready`, `may_present_cook_ready`, or `may_present_nutrition_numbers` is false, caveat or suppress that claim even if the raw readiness fields look favorable.

For engineering QA and live API drift investigations, `food run` can record and replay read-only Alcampo HTTP responses with `--record-live-snapshot <dir>` and `--replay-live-snapshot <dir> --snapshot-strict`. Snapshots write `snapshot_manifest.json` plus base64 response files under `responses/`; request signatures redact volatile query/body fields, forbid cart/auth/checkout/account-like mutation routes, and replay never falls back to live network. Replay uses the recorded origin for relative product URLs but still intercepts transport before any HTTP request is sent. Strict replay misses, mutation attempts, corrupt response files, duplicate signatures with different bodies, or invalid snapshot manifests return exit code 31. When a run also writes `--manifest-out` and `--audit-out`, the food manifest includes snapshot mode, entry counts, manifest hash, replay hits, and replay misses; the artifact audit verifies the snapshot manifest hash, response-file hashes, and zero strict replay misses before the bundle can be trusted.

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
