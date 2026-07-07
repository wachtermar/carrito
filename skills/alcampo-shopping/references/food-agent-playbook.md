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
2. Apply the meal-planning intake gate before planning or shopping:
   - This gate is mandatory for open-ended requests such as "make a meal plan", "full weekly plan", "breakfast lunch dinner", "shop for the week", or "plan my groceries" unless the current request or saved profile already answers the missing fields.
   - Ask all missing high-impact questions in one compact message, then stop.
   - For detailed scenario coverage, load `references/intake-scenarios.md`.
   - Required fields for weekly/full planning: people count, days/meals, diet pattern, allergies, dislikes/foods to avoid, grocery budget target or explicit no-budget preference, pantry/fridge/freezer usage, expiring items, staples, cooking-effort preference, and nutrition goals when the user cares about diet or health.
   - Required fields for shopping/product selection: target budget, selection policy (`balanced`, `cheapest`, `quality`), preferred/rejected brands/products when known, and whether to only review or also prepare a basket.
   - Required fields for cart mutation: explicit approval plus a maximum spend. Never infer approval from a planning request.
   - If the user says "surprise me", "use defaults", "just do it", or gives all high-impact details, continue; otherwise ask first.
3. Resolve saved preferences after the user answers:
   - Save durable household facts with `food profile set` when the user clearly gives them.
   - If `selection_policy` is still missing after intake, ask once for `balanced`, `cheapest`, or `quality`; do not silently choose for a real shopping run.
   - If allergies are unknown and the task involves new foods, ask before planning or shopping.
   - If no budget is known and the user wants shopping/product selection, ask before shopping; planning-only can proceed after people/diet/allergy/scope intake.
4. Prefer the one-command path for low-intervention runs after intake:
   - `alcampo food run --days <n> --people <n> --meals dinner --selection-policy balanced --basket-out basket.txt --json`
5. Present review output before cart mutation:
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
   - strict expiring-item requests verified against `pantry_usage`; if pantry matching misses a requested ingredient because names differ, use recipe-aligned pantry names or add a custom diet/allergy-safe recipe
6. Mutate cart only after explicit user approval and max spend:
   - use the basket file from `--basket-out`, or write `basket_lines` to a basket file
   - `alcampo total -f <file> --json --max <eur>`
   - `alcampo cart set-many -f <file> --max <eur> --json`
   - for cart clearing, read current cart, use a guard at or just above the verified current total, run `alcampo cart clear --yes --max <eur> --json`, then verify the cart is empty
7. After the user confirms the shop was bought, picked up, or delivered, update pantry:
   - `alcampo food receive <shop-or-run.json> --json`
   - This imports selected products into pantry using purchased package counts and writes a `shop_received` history event.
8. For external purchase evidence, update pantry with imports:
   - `alcampo food import-receipt --file <text|-> --json`
   - `alcampo food import-orders --limit <n> --infer-staples --json` when an authenticated session is available.
   - Treat receipt/order parsing as best-effort and show warnings or suggested staples before relying on them.
9. After cooking, update memory:
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
- profile or current request has people count, diet/allergy/dislike constraints, meal scope, budget stance, and selection policy when shopping
- pantry/staple behavior is known or the user explicitly says not to use pantry memory
- the request is a narrow read-only lookup such as product search, recipe lookup, current cart view, history list, or PDF generation from an existing file
- the user explicitly says a saved shop result was delivered and asks to update pantry

Ask or stop when:

- an open-ended meal plan or shopping request is missing household size, meal scope, diet/allergy/dislike constraints, pantry/staple behavior, budget stance, cooking-effort preference, or shopping budget/policy
- allergies/diet constraints are unknown and the requested plan could introduce risk
- cart mutation is requested without explicit approval or max spend
- pantry restock is requested for a shop result that has not been bought, picked up, or delivered
- Alcampo session/market is missing for priced shopping
- product data conflicts with saved allergies or dislikes
- checkout/payment/order submission is requested

Verify before saying done:

- JSON output parsed successfully and warnings/errors were surfaced.
- Open-ended meal-plan/shopping transcripts show intake questions were asked before planning when saved profile/current request did not already cover the required fields.
- Meal plan includes planned days/meals or recipe entries, pantry usage when available, required purchases, and nutrition when available.
- Multi-meal plans use recipes tagged for the requested meal type; breakfasts should not be filled by lunch/dinner-only recipes.
- Shopping output includes selected products, images when present, prices, grouped sections, reasons, alternates, and basket lines or missing-item warnings.
- Cart mutations have a read-back `cart get --json` result proving the intended state.
- Memory mutations have pantry/history read-back when the user expects persistence.

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
