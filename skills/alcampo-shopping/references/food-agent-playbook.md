# Food Agent Playbook

Use this reference when the user asks for meal plans, recipes, pantry-aware shopping, product choice reasoning, autonomous grocery prep, or low-intervention food workflows.

## Operating Model

1. Load state first:
   - `alcampo market --json`
   - `alcampo food profile get --json`
   - `alcampo food pantry list --json`
   - `alcampo food pantry list --expiring-days 3 --json` when waste reduction matters
   - `alcampo food staples list --json`
   - `alcampo food recipes list --json` when selecting from or editing the recipe library
   - `alcampo food history list --limit 20 --json` when prior ratings or substitutions could affect the request.
2. Resolve only missing high-impact preferences:
   - If `selection_policy` is missing, ask once or pass `--selection-policy balanced|cheapest|quality` and let the CLI remember it.
   - If allergies are unknown and the task involves new foods, ask before shopping.
   - If no budget is known, proceed without a budget for planning but require a max spend before cart writes.
3. Prefer the one-command path for low-intervention runs:
   - `alcampo food run --days <n> --people <n> --meals dinner --selection-policy balanced --basket-out basket.txt --json`
4. Present review output before cart mutation:
   - meal plan and recipes
   - pantry/fridge items used
   - expiring pantry items and `food use-up` suggestions when relevant
   - required purchases
   - grouped shopping sections with subtotals
   - selected Alcampo products with image URLs
   - nutrition totals and warnings against saved nutrition goals
   - embedded images in PDFs when image URLs are retrievable
   - offer/deal reasoning
   - package quantity calculation
   - alternates considered
   - unavailable or unsafe items
   - basket file path when `--basket-out` was used
   - estimated total
5. Mutate cart only after explicit user approval and max spend:
   - use the basket file from `--basket-out`, or write `basket_lines` to a basket file
   - `alcampo total -f <file> --json --max <eur>`
   - `alcampo cart set-many -f <file> --max <eur> --json`
6. After the user confirms the shop was bought, picked up, or delivered, update pantry:
   - `alcampo food receive <shop-or-run.json> --json`
   - This imports selected products into pantry using purchased package counts and writes a `shop_received` history event.
7. For external purchase evidence, update pantry with imports:
   - `alcampo food import-receipt --file <text|-> --json`
   - `alcampo food import-orders --limit <n> --infer-staples --json` when an authenticated session is available.
   - Treat receipt/order parsing as best-effort and show warnings or suggested staples before relying on them.
8. After cooking, update memory:
   - `alcampo food cook <mealplan-id-or-file> --rating <1-5> --json`
   - This subtracts planned pantry/fridge usage, appends history, learns liked recipes from 4-5 star ratings, and learns rejected recipes from 1-2 star ratings.

## Product Selection Contract

Every selected product should be explainable from the JSON. Show these fields when available:

- `product.name`, `sku`, `id`, `price`, `unit_price`, `size`, `image_url`, `url`
- `product.offers` and `offer_count`
- `purchase_quantity`, `package_count`, `quantity_reason`, `line_total`
- `selection_reason`
- `alternates_considered`
- `warnings`

The CLI should prefer offers only when they remain a good fit:

- First reject unavailable, allergy-conflicting, disliked, or rejected-brand products.
- Then apply the saved policy.
- Under `balanced`, an offer earns a strong bonus only when its unit price is within about 10% of the cheapest candidate.
- Under `cheapest`, unit/package price dominates after safety checks.
- Under `quality`, brand/product-data/package-fit signals can beat price, but unsafe or unavailable items still lose.

## Quantity And Package Rules

The CLI estimates basket quantities by parsing retail sizes such as `500 g`, `1 kg`, `750 ml`, `6 x 1 l`, and `12 unidades`.

- Use recipe quantities after pantry subtraction.
- Normalize compatible units: `kg -> g`, `l/cl -> ml`, and count units to `unit`.
- Buy `ceil(required / package_size)` retail packages.
- Include `quantity_reason` so the user can audit the calculation.
- If package size is unknown or incompatible, default to one package and include a warning.

## Memory Update Rules

Use local JSON memory to reduce future questions:

- Save household defaults with `food profile set`.
- Save repeat essentials with `food staples add <item> --min <qty> --unit <unit> --search <term>`.
- Save pantry/fridge/freezer facts with `food pantry add|update`.
- Save custom recipes with `food recipes add <file|-> --json` or `food recipes add <file|-> --from-text --title <title> --json`.
- Use `food profile set --nutrition-goals "kcal<=2200,protein>=90"` when the user provides nutrition targets.
- Save product feedback with `food profile set --liked-products <sku-or-name>` or `--rejected-products <sku-or-name>` when a user rejects or asks to repeat a product.
- After a completed shop, prefer `food receive` over manual pantry edits so package quantities, pantry, and history update together.
- After cooking a generated plan, prefer `food cook` over manual pantry edits so pantry, history, liked recipes, and rejected recipes update together.
- Use `food pantry use` for ad-hoc consumed staples outside a plan.
- Use high ratings to bias future plans toward liked recipes; 1-2 star ratings save rejected recipes so future plans avoid them.

Do not store auth secrets, cURL exports, bearer tokens, CSRF tokens, or passwords in food memory.

## Autonomy Boundaries

Proceed without asking when:

- market is already set
- profile has people count and selection policy
- pantry is available or empty
- the request is read-only: plan, recipe, shop, PDF
- the user explicitly says a saved shop result was delivered and asks to update pantry

Ask or stop when:

- allergies/diet constraints are unknown and the requested plan could introduce risk
- cart mutation is requested without explicit approval or max spend
- pantry restock is requested for a shop result that has not been bought, picked up, or delivered
- Alcampo session/market is missing for priced shopping
- product data conflicts with saved allergies or dislikes
- checkout/payment/order submission is requested

## Roadmap For A Near-Perfect Skill

Delivered in the current CLI:

- import previous Alcampo orders as purchase history and pantry restock hints
- import plain-text receipts into pantry memory
- track expiry, filter expiring pantry items, and prioritize recipes that use them
- infer staple suggestions from previous orders with `--infer-staples`
- support custom recipe imports and user recipe libraries
- add nutrition targets and per-plan nutrition summaries

Remaining future improvements:

- scan barcodes into pantry memory
- add leftover generation and automatic pantry updates after cooking
- group basket by Alcampo aisle/category for faster review
- maintain substitution history per ingredient and per brand
- compare offer products against historical prices when order history is available
