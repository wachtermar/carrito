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
carrito food run --days 3 --people 2 --meals dinner --selection-policy balanced --basket-out basket.txt --pdf-out food-plan.pdf --json
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
carrito food pdf <recipe-or-plan-or-shop.json> --out food-plan.pdf
carrito food receive <shop-or-run.json> --json
carrito food import-receipt --file receipt.txt --json
carrito food import-orders --limit 5 --since 2026-01-01 --infer-staples --json
carrito food cook <mealplan-id-or-file> --rating 5 --json
carrito food history list --limit 20 --json
```

`food plan` and `food run` require a people count from either `--people` or `profile.json`; they intentionally fail instead of inventing a household size. `food run` is the preferred low-intervention path after intake: it loads profile and pantry memory, generates the meal plan, subtracts pantry/fridge items, shops the remaining ingredients, optionally writes a PDF, and returns one JSON object.

Recipe library behavior: embedded seed recipes are always available, and the editable library lives in the SQLite database at `CARRITO_CONFIG_DIR/food/recipes.db` or `~/.carrito/food/recipes.db`. The database stores normalized recipe, tag, ingredient, equipment, step, substitution, allergen-note, nutrition, and full-text search tables. User recipes added through `food recipes add` override seed recipes with the same `id`. Legacy `food/recipes/*.json` files are imported once into SQLite for migration, but new recipes should be added through the CLI. `food recipes add --from-text` parses comma- or newline-separated ingredient lists and auto-derives Spanish search terms. `food recipes search <query> --profile` applies saved profile diets, allergies, and dislikes.

Nutrition behavior: recipes may include `nutrition_per_serving`, and generated meal plans/shopping results include a `nutrition` total when estimates are available. `food profile set --nutrition-goals "kcal<=2200,protein>=90"` stores free-form goals; plans add warning notes when totals miss parseable goals.

Expiry behavior: pantry text output marks items expiring within three days with `!`, `food pantry list --expiring-days N` filters pantry output, and `food use-up` ranks recipes that consume pantry items expiring within `N` days.

`food shop` searches multiple Alcampo candidates per required purchase and returns selected product SKU/id, name, price, unit price, package size, purchase quantity, line total, image URL, product URL, availability, offers, offer count, selection reason, quantity reason, basket line, grouped shopping sections with subtotals, budget status notes, and compatible alternates considered. It rejects products that conflict with saved diets, allergies, dislikes, rejected brands, or remembered `rejected_products` matches by SKU, id, EAN, brand, or name, and boosts remembered `liked_products`. If no `selection_policy` is saved, an interactive terminal prompts once; non-interactive agents should set it with `food profile set` or pass `--selection-policy`, which is remembered.

Use `--basket-out basket.txt` on `food shop` or `food run` when the next step may be guarded cart preparation. The file uses the same `<product_id_or_sku> <qty> # comment` lines returned in `basket_lines`, so it can be passed directly to `carrito total -f basket.txt --json --max <eur>` and then `carrito cart set-many -f basket.txt --max <eur> --json` after explicit approval.

Use `food receive <shop-or-run.json>` only after the user says the shop was actually bought, picked up, or delivered. It imports selected products into `pantry.json` using product package size times purchased package count when available, falls back to planned ingredient quantity when package size is unknown, infers pantry/fridge/freezer location from item category/name, and writes a `shop_received` event to `history.jsonl`.

Use `food import-receipt --file <text|->` for pasted/plain-text grocery receipts. It is best-effort: recognized lines become pantry items, skipped lines become warnings, and a `receipt_imported` history event is written. Use `food import-orders` only with a logged-in/imported session; it tries known private order-history endpoints, maps returned order lines into pantry items, writes `orders_imported` history, and can return `suggested_staples` with `--infer-staples`.

`food pdf` embeds retrievable recipe cover, step, and product images into the generated PDF from local paths, data image URLs, or HTTP(S) image URLs. If an image cannot be loaded, the PDF falls back to the original image URL text instead of failing the whole document.

Offer behavior: offers are not blindly preferred. The selector first rejects unavailable, allergy-conflicting, disliked, or rejected-brand products, then gives a stronger offer bonus when the offered product's unit price is competitive with the cheapest candidate.

Quantity behavior: basket quantities are calculated from recipe requirements and parsed retail package sizes such as `500 g`, `1 kg`, `750 ml`, `6 x 1 l`, or `12 unidades`. If package size is unknown, the CLI buys one package and returns a warning.

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
