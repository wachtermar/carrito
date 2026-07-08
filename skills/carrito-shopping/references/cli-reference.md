# Alcampo CLI Reference

Use this reference for command shapes during normal agent work. Use `carrito --help` only for install/troubleshooting checks, and avoid subcommand `--help` probes inside task workflows because usage text is emitted on stderr and can be noisy in Hermes terminal transcripts. Prefer JSON output for agent work and keep stderr separate from stdout.

## Location

Price and availability reads require an explicit market:

```bash
carrito market
carrito set-market --region-id ac90d761-9d58-4918-a37d-dd14e1ce384a --retailer-region-id 5 --name Vaguada
carrito set-address <delivery_destination_id>
```

Use `--store <regionId>` or `--market <regionId>` to override one read request when the user provides a known region id. Do not silently choose a market for a user's real basket.

## Read Commands

```bash
carrito version
carrito search "leche" --limit 10 --json
carrito product 54178 --json
carrito categories --json
carrito categories --id OC16 --limit 20 --json
printf 'leche\narroz\nagua\n' | carrito batch -f - --json
```

## Food Commands

Food state is local JSON under `CARRITO_CONFIG_DIR/food/` or `~/.carrito/food/`.

Profile and product-selection policy:

```bash
carrito food profile get --json
carrito food profile set --people 2 --selection-policy balanced --budget 80 --json
carrito food profile set --allergies "peanut,shellfish" --dislikes "mushrooms" --json
carrito food profile set --liked-recipes "lentil-stew" --rejected-recipes "salmon-potatoes" --json
carrito food profile set --liked-products "947535" --rejected-products "222,brand or product name" --json
carrito food profile set --preferred-brands "AUCHAN" --rejected-brands "brand name" --json
```

Supported selection policies:

- `balanced`: compatible and available first, then unit price, package fit, offers, brand history, and product match.
- `cheapest`: lowest unit/package price after diet/allergy/dislike checks.
- `quality`: trusted brands and richer product/nutrition/freshness signals before price.

Pantry/fridge/freezer memory:

```bash
carrito food pantry list --json
carrito food pantry list --expiring-days 3 --json
carrito food pantry add rice --qty 500 --unit g --location pantry --json
carrito food pantry add "chicken breast" --qty 350 --unit g --location fridge --expiry 2026-07-07 --json
carrito food pantry use rice --qty 100 --unit g --json
carrito food pantry update rice --qty 250 --unit g --location pantry --json
```

Staple restock memory:

```bash
carrito food staples list --json
carrito food staples add milk --min 2 --unit l --search leche --json
carrito food staples add eggs --min 12 --unit unit --search huevos --json
carrito food staples remove milk --json
```

Staples are stored in `profile.json`. `food plan` and `food run` compare staples against pantry quantities and add only the missing amount to `required_purchases`.

Meal plans, recipes, shopping, and PDFs:

```bash
carrito food run --days 3 --people 2 --meals dinner --selection-policy balanced --basket-out basket.txt --json
carrito food run --days 3 --people 2 --meals dinner --selection-policy balanced --servings 2 --basket-out basket.txt --run-out run.json --quantity-ledger-out ledger.json --nutrition-ledger-out nutrition_ledger.json --recovery-out recovery.json --recipe-swap-out recipe_swap.json --basket-optimization-out basket_optimization.json --serving-plan-out serving_plan.json --scaled-mealplan-out scaled_mealplan.json --pantry-out pantry.json --pantry-consumption-out pantry_consumption.json --readiness-out readiness.json --manifest-out manifest.json --audit-out audit.json --audit-mode fail --pdf-out food-plan.pdf --enrich-products --strict-quantity --strict-servings --recover-missing --allow-recipe-swap --optimize-basket --basket-objective safe-balanced --deal-aware --default-pantry minimal-spanish --allow-assumed-pantry --require-safe-basket --require-cook-ready --json
carrito food validate-run --run run.json --manifest manifest.json --audit-out audit.json --mode hermes --audit-mode fail --pdf food-plan.pdf --basket basket.txt --json
carrito food run --days 1 --people 2 --meals dinner --run-out run.json --manifest-out manifest.json --audit-out audit.json --audit-mode fail --record-live-snapshot snapshots/alcampo-2026-07-08 --snapshot-id alcampo-2026-07-08 --json
carrito food run --days 1 --people 2 --meals dinner --run-out replay-run.json --manifest-out replay-manifest.json --audit-out replay-audit.json --audit-mode fail --replay-live-snapshot snapshots/alcampo-2026-07-08 --snapshot-strict --json
carrito food plan --days 3 --people 2 --meals dinner --json
carrito food use-up --expiring-days 3 --limit 8 --json
carrito food recipe "quick vegetarian pasta" --people 2 --json --out recipe.json
carrito food recipe <mealplan-id-or-file> --json --out recipes.json
carrito food recipes list --tag quick --json
carrito food recipes list --query chickpea --diet vegan --limit 5 --json
carrito food recipes search "quick rice" --profile --json
carrito food recipes show spanish-tortilla --json
carrito food recipes add recipe.json --json
printf '2 pechugas de pollo, 1 cebolla, 200 g arroz' | carrito food recipes add - --from-text --title "Pollo rapido" --json
carrito food recipes remove pollo-rapido --json
carrito food shop <mealplan-id-or-file> --json
carrito food shop <mealplan-id-or-file> --selection-policy cheapest --json --out shop.json --basket-out basket.txt
carrito food pdf <recipe-or-plan-shop-run.json> --out food-plan.pdf
carrito food receive <shop-or-run.json> --json
carrito food import-receipt --file receipt.txt --json
carrito food import-orders --limit 5 --since 2026-01-01 --infer-staples --json
carrito food cook <mealplan-id-or-file> --rating 5 --json
carrito food history list --limit 20 --json
```

`food plan` and `food run` require a people count from either `--people` or `profile.json`; they intentionally fail instead of inventing a household size. `food run` is the preferred low-intervention path after intake: it loads profile and pantry memory, generates the meal plan, applies optional serving scaling from `--servings`, `--adult-servings`, `--child-servings`, `--toddler-servings`, or `--household-profile`, subtracts pantry/fridge items from the scaled requirements, applies structured pantry resolution when requested, shops the remaining ingredients, optionally enriches selected products from Alcampo detail pages, builds a quantity ledger, builds a nutrition ledger, attempts audited missing-product recovery with `--recover-missing`, can repair blocked generated meal plans with `--allow-recipe-swap`, can run audited deal-aware product re-selection with `--optimize-basket`, writes a combined `food_run` JSON artifact with `--run-out`, can write `serving_plan` with `--serving-plan-out`, can write `scaled_mealplan` with `--scaled-mealplan-out`, can write a standalone `quantity_ledger` with `--quantity-ledger-out`, can write a standalone `nutrition_ledger` with `--nutrition-ledger-out`, can write `recovery_plan` with `--recovery-out`, can write `recipe_swap_plan` with `--recipe-swap-out`, can write `basket_optimization_plan` with `--basket-optimization-out`, can write `pantry_resolution` with `--pantry-out`, can write planned pantry consumption with `--pantry-consumption-out`, writes the final `readiness_gate` with `--readiness-out`, optionally renders a complete meal-plan-and-shopping PDF with `--pdf-out`, and returns one JSON object. For full meal-planning requests, prefer `--servings <n> --run-out --serving-plan-out --scaled-mealplan-out --quantity-ledger-out --nutrition-ledger-out --recovery-out --recipe-swap-out --basket-optimization-out --pantry-out --pantry-consumption-out --readiness-out --pdf-out --enrich-products --strict-quantity --strict-servings --recover-missing --allow-recipe-swap --optimize-basket --basket-objective safe-balanced --deal-aware --default-pantry minimal-spanish --allow-assumed-pantry --require-safe-basket --require-cook-ready` over creating a shop-only PDF; use adult/child/toddler flags or a household profile instead of `--servings` for mixed households. Saved `food pantry` memory is merged into structured pantry resolution as confirmed evidence, and `--pantry-profile pantry_profile.json` can add normalized pantry items with `value`, `unit`, `base_value`, and `base_unit`. With `--require-safe-basket`, `--require-cook-ready`, or `--require-nutrition-ready`, exit code 20 means the run completed and wrote diagnostic artifacts; parse `readiness_gate.blocking_issues`, `readiness_gate.safe_to_build`, `readiness_gate.safe_to_cook`, `readiness_gate.safe_to_report_nutrition`, `serving_plan`, `scaled_mealplan`, `nutrition_ledger`, and `pantry_resolution`, then report basket, cooking, and nutrition readiness separately. Do not use the basket file for cart mutation when `safe_to_build` is false. When `safe_to_build` is true but `safe_to_cook` is false, the basket lines are structurally valid but the meal plan still needs pantry, serving, or ingredient confirmation. When `safe_to_report_nutrition` is false, calories and macros can be shown only with explicit coverage caveats.

For Hermes-grade runs, also write `--manifest-out manifest.json --audit-out audit.json --audit-mode fail`. The manifest records artifact paths, hashes, sizes, readiness summary, generation exit code, and fingerprints. The audit reopens the written run JSON, sidecars, guarded basket, and PDF from disk; verifies hashes, sidecar equality, fingerprint consistency, basket actionability, PDF trust markers, and basic ledger/nutrition consistency; and returns exit code 30 when the bundle is contradictory or unsafe to trust. Exit 30 overrides readiness claims: regenerate the artifacts before saying the basket, cooking status, PDF, or nutrition totals are ready. `food validate-run` runs the same audit later for an existing bundle, and agents should read `artifact_audit.hermes_trust_summary.trustworthy`, `may_present_basket_as_ready`, `may_present_cook_ready`, and `may_present_nutrition_numbers` before making user-facing claims.

For engineering QA and live API drift investigations, `food run` supports `--record-live-snapshot <dir>` and `--replay-live-snapshot <dir> --snapshot-strict`. The snapshot harness records read-only Alcampo HTTP responses into `snapshot_manifest.json` and `responses/*.json`, redacts volatile request fields, blocks cart/auth/checkout/account-like routes, and never falls back to live network during replay. Strict replay misses, mutation attempts, invalid/corrupt snapshot files, or duplicate signatures with different response bodies return exit code 31. When manifest/audit flags are present, `manifest.snapshot` records snapshot mode, entry count, manifest hash, replay hits, and replay misses, and the artifact audit verifies response hashes plus zero strict replay misses before an agent treats the bundle as trustworthy. Do not use snapshot flags for normal user shopping unless you are intentionally debugging or reproducing a live Alcampo/API issue.

Serving behavior: serving scaling happens before pantry subtraction, shopping, recovery, recipe swaps, basket optimization, and PDF rendering. `--servings N` sets an explicit target for every planned recipe slot; `--adult-servings`, `--child-servings`, and `--toddler-servings` calculate weighted targets for mixed households; `--household-profile household.json` can specify members, per-meal participation, and default leftover policy. `--allow-leftovers` or `--leftover-servings N` can cook extra servings intentionally; `--no-leftovers` keeps cooked serving units equal to target serving units. `--strict-servings --require-cook-ready` blocks cooking readiness when a recipe cannot be scaled safely, while `--no-serving-scaling` preserves legacy recipe quantities. The `serving_plan` artifact records target/cooked serving units, participants, leftovers, warnings, and fingerprints; `scaled_mealplan` records original and scaled ingredient quantities, scale factors, rounded-piece notes, and blocked/unparseable quantities.

Recipe library behavior: embedded seed recipes are always available, and the editable library lives in the SQLite database at `CARRITO_CONFIG_DIR/food/recipes.db` or `~/.carrito/food/recipes.db`. The database stores normalized recipe, tag, ingredient, equipment, step, substitution, allergen-note, nutrition, and full-text search tables. User recipes added through `food recipes add` override seed recipes with the same `id`. Legacy `food/recipes/*.json` files are imported once into SQLite for migration, but new recipes should be added through the CLI. `food recipes add --from-text` parses comma- or newline-separated ingredient lists and auto-derives Spanish search terms. `food recipes search <query> --profile` applies saved profile diets, allergies, and dislikes.

Nutrition behavior: recipes may include `nutrition_per_serving`, and generated meal plans/shopping results include a `nutrition` total when estimates are available. Product selections preserve Alcampo's free-form nutrition label text when present and also expose parsed `nutrition_parsed`, `product_nutrition`, `required_nutrition`, `purchased_nutrition`, `product_nutrition_reports`, and `nutrition_warnings` fields when the label can be parsed safely. Combined run artifacts include `nutrition_ledger`, and `--nutrition-ledger-out nutrition_ledger.json` writes it separately. The ledger uses `--nutrition-mode labels|recipe|hybrid|off`; the default `hybrid` keeps Alcampo label evidence, pantry-profile nutrition evidence, and recipe-declared nutrition separate instead of blending them into one false exact number. Pantry profile nutrition is allowed by default and can be disabled with `--allow-pantry-profile-nutrition=false`; built-in pantry nutrition remains off unless `--allow-builtin-pantry-nutrition` is passed. Consumed nutrition is calculated from scaled recipe quantities after pantry subtraction: a 250 g rice requirement from a 500 g package counts 250 g as consumed, not the whole package. Purchased excess is excluded from consumed totals by default and appears separately only with `--include-purchased-excess-nutrition`. Pantry-covered lines require explicit pantry nutrition evidence to count as known; missing pantry nutrition is unknown, not zero. `--require-nutrition-ready` plus `--min-nutrition-line-coverage` or `--min-nutrition-quantity-coverage` can make the command return exit code 20 with `readiness_gate.safe_to_report_nutrition=false`; this does not by itself make the basket unsafe or the meal uncookable. Required-amount nutrition is calculated only for safe unit matches such as grams against `per_100g`, millilitres against `per_100ml`, or units against `per_unit`; missing labels and unsafe conversions are warnings, not invented exact values. `food profile set --nutrition-goals "kcal<=2200,protein>=90"` stores free-form goals; plans add warning notes when totals miss parseable goals.

Expiry behavior: pantry text output marks items expiring within three days with `!`, `food pantry list --expiring-days N` filters pantry output, and `food use-up` ranks recipes that consume pantry items expiring within `N` days.

`food shop` searches multiple Alcampo candidates per required purchase and returns selected product SKU/id, name, price, unit price, package size, purchase quantity, line total, image URL, product URL, availability, offers, offer count, selection reason, quantity reason, basket line, grouped shopping sections with subtotals, budget status notes, and compatible alternates considered. It rejects products that conflict with saved diets, allergies, dislikes, rejected brands, or remembered `rejected_products` matches by SKU, id, EAN, brand, or name, and boosts remembered `liked_products`. If no `selection_policy` is saved, an interactive terminal prompts once; non-interactive agents should set it with `food profile set` or pass `--selection-policy`, which is remembered.

Use `--basket-out basket.txt` on `food shop` or `food run` when the next step may be guarded cart preparation. For `food run`, trust the file only when `readiness_gate.safe_to_build` is true, and do not describe the whole meal plan as complete unless `readiness_gate.safe_to_cook` is also true. Ready files start with readiness comment headers followed by `<product_id_or_sku> <qty> # comment` lines. Blocked files start with `# NOT SAFE TO BUILD BASKET` and contain diagnostic comments only; never pass a blocked basket to `total` or `cart set-many`. When ready and after explicit approval, use `carrito total -f basket.txt --json --max <eur>` and then `carrito cart set-many -f basket.txt --max <eur> --json`.

Use `food receive <shop-or-run.json>` only after the user says the shop was actually bought, picked up, or delivered. It imports selected products into `pantry.json` using product package size times purchased package count when available, falls back to planned ingredient quantity when package size is unknown, infers pantry/fridge/freezer location from item category/name, and writes a `shop_received` event to `history.jsonl`.

Use `food import-receipt --file <text|->` for pasted/plain-text grocery receipts. It is best-effort: recognized lines become pantry items, skipped lines become warnings, and a `receipt_imported` history event is written. Use `food import-orders` only with a logged-in/imported session; it tries known private order-history endpoints, maps returned order lines into pantry items, writes `orders_imported` history, and can return `suggested_staples` with `--infer-staples`.

`food pdf` accepts recipe, meal plan, shop, or combined `food_run` JSON. Combined run PDFs include the day-by-day meal plan, serving assumptions, scaled ingredients, recipe ingredients and cooking instructions, selected products, product images, package math, quantity-ledger confidence, nutrition ledger evidence, readiness verdict, blocking issues or caveats, estimated totals, basket lines, consumed nutrition totals when coverage supports them, recipe-declared nutrition as a separate caveat, and cart/cooking/nutrition-safety caveats. The PDF renderer embeds retrievable recipe cover, step, and product images from local paths, data image URLs, or HTTP(S) image URLs. If an image cannot be loaded, the PDF falls back to the original image URL text instead of failing the whole document.

Offer behavior: offers are not blindly preferred. The selector first rejects unavailable, allergy-conflicting, disliked, or rejected-brand products, then gives a stronger offer bonus when the offered product's unit price is competitive with the cheapest candidate.

Quantity behavior: basket quantities are calculated from recipe requirements and parsed retail package sizes such as `500 g`, `1 kg`, `1,5 l`, `750 ml`, `6 x 125 g`, `3x80g`, `12 ud`, `docena`, `peso aprox. 500 g`, and `peso escurrido 52 g`. Combined run artifacts include a `quantity_ledger` with status `complete_exact`, `complete_estimated`, `incomplete`, or `needs_review`, product evidence, ingredient-product allocations, excess/missing quantities, exact/estimated/low-confidence badges, conservative offer classification, total estimates, and `basket_safety`. Variable-weight or approximate products are not treated as exact; missing products are incomplete; unsupported units such as cloves versus grams become needs-review lines.

Recovery behavior: `food run --recover-missing` extracts blocking issues from the pre-recovery ledger, searches deterministic same-ingredient Spanish aliases, rejects unsafe semantic mismatches, applies safe product recoveries, adds templated prep notes when a same-ingredient form transform is required, recomputes the final ledger, and writes recovery details into the run artifact/PDF. For example, missing `beef strips` may recover with thin `filetes de ternera` plus a prep note to cut them into strips; `carne picada` is rejected as a product-level recovery for beef strips. Recovery does not make the basket ready unless the final `readiness_gate.safe_to_build` is true.

Recipe swap behavior: `food run --allow-recipe-swap` runs only after product recovery and readiness still block the run. It maps blocking ingredient issues back to recipe slots through ledger usage refs, pulls deterministic catalog candidates that preserve meal type, serving intent, diet, allergies, and dislikes, and validates every candidate by rebuilding serving scaling for the swapped slot, shopping, enrichment, quantity ledger, recovery, basket safety, and readiness. It applies only a candidate whose final readiness is safe. The run artifact preserves `pre_recipe_swap_readiness_gate`, `recipe_swap_plan`, final `readiness_gate`, `serving_plan_fingerprint`, `scaled_mealplan_fingerprint`, and `mealplan_fingerprint` values so stale derived artifacts can be detected. If no candidate validates, the original blocked run remains diagnostic and exits 20 under `--require-safe-basket`.

Basket optimization behavior: `food run --optimize-basket` runs after recovery and recipe swap. It searches same-ingredient alternatives, parses conservative Alcampo offers such as `3x2`, `2x1`, `lleva 3 paga 2`, `llévate 3 y paga 2`, and `2ª unidad -50%`, and applies a product switch only when semantic match, quantity confidence, basket safety, and final readiness are preserved. Ambiguous percent-off offers are recorded but not double-discounted. The run artifact preserves `pre_optimization_readiness_gate`, `pre_optimization_basket_safety`, `basket_optimization_plan`, and product-selection fingerprints; stale ledgers, recovery plans, or basket safety snapshots block final readiness.

Lifecycle behavior: `food plan` ranks recipes using saved profile preferences plus pantry/fridge items, prioritizing expiring matches, skipping `rejected_recipes`, and adding low-stock staples. `food receive` restocks pantry from a completed shop result. `food cook` applies the plan's pantry usage to current pantry memory, appends a `mealplan_cooked` event with recipe IDs/titles to `history.jsonl`, adds recipes to `liked_recipes` for 4-5 star ratings, and adds recipes to `rejected_recipes` for 1-2 star ratings.

Basket files use one item per line:

```text
<product_id_or_sku> <qty> # optional comment
54178 2
205192 1
```

Price a basket:

```bash
carrito total -f basket.txt --json
carrito total -f basket.txt --max 20 --json
```

`total` uses cent math and supports fractional quantities.

## Auth Bootstrap

Use imported sessions or direct login. Keep secrets in the runner's secret manager or environment, never in prompts or source files.

Import cURL from an existing web session:

```bash
export CARRITO_CONFIG_DIR="$PWD/.carrito-agent"
printf '%s' "$CARRITO_CURL" | carrito import-curl --file -
carrito addresses --json
carrito set-address <delivery_destination_id>
carrito whoami --json
```

Direct login from secrets:

```bash
export CARRITO_CONFIG_DIR="$PWD/.carrito-agent"
carrito login-web --if-needed --json
printf '%s\n' "$CARRITO_PASSWORD" | carrito login --username "$CARRITO_USERNAME" --password-stdin
carrito addresses --json
carrito set-address <delivery_destination_id>
carrito whoami --json
```

Use `carrito login-web --if-needed --json` for Hermes Desktop or other GUI agent surfaces. It checks the local config first, then starts a temporary `127.0.0.1` browser form only when no session exists. The password is submitted only to the local CLI process, used once for login, and not printed or stored. Use `--no-open` only for headless terminals where you want to copy the local URL manually.

Stored auth material may include cookies, bearer tokens, CSRF tokens, customer or visitor ids, delivery-destination ids, and location hints. Config files with secrets should be mode `0600`.

## Cart And Checkout

Supported authenticated commands:

```bash
carrito cart get --json
carrito cart get --json --raw
carrito cart add <product_id_or_sku> <qty> --max <eur> --json
carrito cart set <product_id_or_sku> <qty> --max <eur> --json
carrito cart set-many -f basket.txt --max <eur> --json
carrito cart clear --yes --max <eur> --json
carrito checkout addresses --json
carrito checkout slots --json
carrito checkout create --max <eur> --json
carrito checkout select-slot --slot <slot_id> --max <eur> --json
```

`cart get --json` returns a normalized agent-readable cart summary: item count, total, and line items with product id/SKU, name, brand, quantity, package price, unit price, line total, size, category, product URL, image URLs, offers, and availability. Use `cart get --json --raw` only when troubleshooting private-API response drift.

All writes require an authenticated/imported session, CSRF token, verified active cart total, and nonzero spending guard. The guard can be `--max <eur>`, `CARRITO_MAX_EUR`, or `[limits] max_eur` in config.

For clear-cart requests, read the cart first, set `--max` to the verified current total or the next whole euro, run `cart clear --yes --max <eur> --json`, then verify with `cart get --json`. Report success only when the read-back result is empty and total is zero.

`checkout confirm-slot -f <json> --max <eur>` sends only an explicit confirmation payload returned by the reservation flow. Do not guess the payload.

Payment and order submission are intentionally not implemented.

## Troubleshooting

- `no market set`: run `carrito market`, `carrito set-market`, or authenticate and `carrito set-address`.
- `403`: the web app may have changed headers, blocked a page, or required an authenticated web session.
- Prices differ from the website: verify the region/store context.
- Empty batch or total result: confirm each line is a searchable term or SKU for the active market.
- Auth expired: rerun `login`, `import-curl`, or `import-har`; automatic refresh is not implemented.
