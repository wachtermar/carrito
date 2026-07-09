# Food Agent Playbook

Use this reference when the user asks for meal plans, recipes, pantry-aware shopping, product choice reasoning, autonomous grocery prep, or low-intervention food workflows.

## Operating Model

1. Load state first:
   - `carrito market --json`
   - `carrito food profile get --json`
   - `carrito food pantry list --json`
   - `carrito food pantry list --expiring-days 3 --json` when waste reduction matters
   - `carrito food staples list --json`
   - `carrito food recipes list --json` when selecting from or editing the recipe library
   - `carrito food history list --limit 20 --json` when prior ratings or substitutions could affect the request.
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
   - `carrito food run --days <n> --people <n> --meals dinner --selection-policy balanced --servings <serving-units> --basket-out basket.txt --run-out run.json --quantity-ledger-out ledger.json --product-evidence-out product_evidence.json --refresh-product-evidence --nutrition-ledger-out nutrition_ledger.json --recipe-intake-out recipe_intake.json --recipe-quality-out recipe_quality.json --recovery-out recovery.json --recipe-swap-out recipe_swap.json --basket-optimization-out basket_optimization.json --budget-repair-out budget_repair.json --budget-deal-out budget_deal.json --serving-plan-out serving_plan.json --scaled-mealplan-out scaled_mealplan.json --pantry-out pantry.json --pantry-consumption-out pantry_consumption.json --readiness-out readiness.json --manifest-out manifest.json --audit-out audit.json --audit-mode fail --pdf-out food-plan.pdf --enrich-products --strict-quantity --strict-servings --strict-recipe-quality --recover-missing --allow-recipe-swap --optimize-basket --repair-budget --basket-objective safe-balanced --deal-aware --default-pantry minimal-spanish --allow-assumed-pantry --require-safe-basket --require-cook-ready --json`
   - For serious user requests, first write `intent.json` and add `--intent-file intent.json --intent-out intent.json --constraint-report-out constraint_report.json --require-intent-ready`. The intent should include only explicit constraints or saved profile facts: expected days/meals, serving units, budget, nutrition targets, exclusions/allergens, diet rules, cooking limits, pantry stance, and PDF/recipe requirements.
   - Do not put vague language into hard intent fields until you translate it. "Cheap" needs a budget or price preference, "healthy" needs nutrition targets or diet rules, and "quick" needs a cooking-time threshold. If the user does not provide enough detail and you choose not to ask, record the vague item as a soft preference and do not claim it was guaranteed.
   - Add `--recipe-file <path>`, `--recipe-dir <path>`, or `--recipe-url <url>` when the user supplies recipes. URL intake accepts schema.org Recipe JSON-LD only; unstructured recipe pages fail closed. Use `--require-recipe-source` for provenance-critical work and `--require-recipe-images` only when every active recipe needs cached/local image evidence.
   - Add `--require-nutrition-ready --min-nutrition-line-coverage <ratio> --min-nutrition-quantity-coverage <ratio>` only when the user needs calorie/macro-critical reporting. Normal meal planning should still produce a nutrition ledger, but partial nutrition must remain caveated.
   - Add `--require-budget-ready` only when the user requires the final estimate to be inside a parseable budget target. Normal runs should still produce `budget_deal_report`, but over-budget or loyalty/unclear offer evidence must remain caveated.
   - Keep `--repair-budget --budget-repair-out budget_repair.json` when the user gives a budget or asks for cheap/not-expensive planning. Product-level repair may switch only validated same-ingredient products. Add `--allow-budget-recipe-swap` only when the user explicitly allows cheaper meal substitutions; v1 records that policy but does not silently change recipes for budget.
   - Use `--adult-servings`, `--child-servings`, and `--toddler-servings`, or `--household-profile household.json`, instead of `--servings` when the household is mixed or meal participation differs by person.
   - Saved `food pantry` memory is used as confirmed structured pantry evidence. Use `--pantry-profile pantry_profile.json` for extra pantry facts, and include normalized quantities with `value`, `unit`, `base_value`, and `base_unit`.
   - For shareable cooking output, generate `carrito food html run.json --out output/html/<descriptive-name>.html [--cover-image path-or-url]` after the run. Use the HTML page as the primary user-facing artifact on mobile; keep `--pdf-out`/`food pdf` as print or evidence fallback.
   - Use `--cover-image` only for a real dish/recipe image for the active recipe. Product package photos are Alcampo shopping evidence only; never use them as hero, recipe, or dish photos. If recipe image evidence is missing, report it and let the HTML missing-photo state stand.
   - For full printable meal plans, use `--run-out` with `--pdf-out` so the PDF is generated from a combined `food_run` artifact containing both recipes and selected products.
   - If this exits 20, the run still produced diagnostic artifacts. Parse `run.json`, `readiness.json`, `intent.json`, `constraint_report.json`, `recipe_intake.json`, `recipe_quality.json`, `serving_plan.json`, `scaled_mealplan.json`, `product_evidence.json`, `nutrition_ledger.json`, `budget_repair.json`, `budget_deal.json`, `pantry.json`, and `audit.json`; report `safe_to_build`, `safe_to_cook`, `safe_to_use_recipes`, `safe_to_use_product_evidence`, `product_evidence_report.status`, `safe_to_report_nutrition`, `budget_repair_plan.status`, `safe_to_report_budget`, `safe_to_report_deals`, `constraint_satisfaction_report.status`, and `safe_to_satisfy_intent` separately. Do not use `basket.txt` for cart preparation when `safe_to_build` is false, do not call the meal plan cook-ready when `safe_to_cook` or `safe_to_use_recipes` is false, do not call products/prices current when product evidence or audit product trust flags are false, do not call calories/macros complete when `safe_to_report_nutrition` is false, do not claim the plan is within budget when `safe_to_report_budget` is false, and do not say the overall request was satisfied when `safe_to_satisfy_intent` is false.
   - If this exits 30, the artifact audit failed. Do not present the basket, PDF, cooking status, current products/prices, or nutrition numbers as ready; regenerate the run or rerun `carrito food validate-run --run run.json --manifest manifest.json --audit-out audit.json --mode hermes --audit-mode fail --pdf food-plan.pdf --basket basket.txt --json` after fixing files. Use `artifact_audit.hermes_trust_summary` as the final claim gate.
   - If this exits 31, live snapshot record/replay policy failed. Treat the snapshot-backed run as blocked evidence, not as a readiness-only diagnostic; record a fresh snapshot, fix the request shape, or remove snapshot flags for a normal live user run.
   - Use `--record-live-snapshot <dir>` and `--replay-live-snapshot <dir> --snapshot-strict` only for engineering QA, CI reproduction, or Alcampo API drift investigations. Snapshot replay is read-only, blocks mutation-like routes, and never falls back to live network.
5. Present review output before cart mutation:
   - meal plan and recipes
   - serving assumptions, target serving units, cooked serving units, leftover policy, and scaled quantity notes
   - pantry/fridge items used
   - expiring pantry items and `food use-up` suggestions when relevant
   - required purchases
   - grouped shopping sections with subtotals
   - selected Alcampo products with image URLs
   - recipe nutrition totals, `recipe_quality_report.status`, source provenance, image provenance/cache status, `nutrition_ledger.status`, `nutrition_ledger.coverage`, parsed Alcampo label nutrition when available, pantry nutrition gaps, and warnings against saved nutrition goals
   - structured pantry resolution, pantry-covered ingredients, assumptions, partial pantry deltas, and `pantry_consumption` path when present
   - `serving_plan.status`, `scaled_mealplan.summary`, serving blockers, and `serving_plan`/`scaled_mealplan` paths when serving scaling was requested
   - the mobile HTML cooking page path from `food html`, generated from the combined run artifact when available
   - recipe/dish image provenance; product package photos must be labeled as shopping evidence and never presented as dish photos
   - embedded images in PDFs when image URLs are retrievable
   - budget repair status, applied product switches, failed-repair caveats, budget/deal report status, budget target, estimated total, budget delta, offer caveats, and deal-aware optimization savings
   - package quantity calculation
   - `readiness_gate.status`, `readiness_gate.safe_to_build`, `readiness_gate.safe_to_cook`, `readiness_gate.safe_to_use_recipes`, `readiness_gate.safe_to_use_product_evidence`, `readiness_gate.product_evidence_status`, `readiness_gate.safe_to_report_nutrition`, `readiness_gate.safe_to_report_budget`, `readiness_gate.safe_to_report_deals`, cook readiness status, recipe quality status, product evidence status, serving readiness status, nutrition readiness status, budget status, blocking issues, caveats, exit code, and remediation
   - `constraint_satisfaction_report.status`, `readiness_gate.safe_to_satisfy_intent`, unsatisfied/unknown hard constraints, soft preference misses, and the `constraint_report.json` path when present
   - `artifact_audit.status`, `artifact_audit.hermes_trust_summary.trustworthy`, `may_present_basket_as_ready`, `may_present_cook_ready`, `may_present_recipes_as_cookable`, `may_present_pdf_as_complete`, `may_present_nutrition_numbers`, `may_present_budget_as_ready`, `may_present_deals_as_ready`, `may_present_request_as_satisfied`, `may_present_alcampo_products_as_current`, `may_present_product_prices_as_current`, `may_present_product_nutrition_as_current`, and any required user warning
   - `pre_recipe_swap_readiness_gate.status`, `recipe_swap_plan.status`, applied swaps, failed candidates, and remaining issues when recipe repair was evaluated
   - `quantity_ledger.status`, `basket_safety.status`, ingredient coverage, exact quantity lines, estimated variable-weight lines, needs-review lines, and nutrition label coverage from `nutrition_ledger`
   - `recovery_plan.status`, applied product recoveries, prep notes, rejected candidates when useful, and remaining issues
   - alternates considered
   - unavailable or unsafe items
   - basket file path when `--basket-out` was used; blocked basket files are diagnostic comments only
   - run artifact path when `--run-out` or `--pdf-out` was used
   - estimated total and whether the budget/deal claim gates allow the plan to be presented as within budget or deal-ready
   - HTML generated from the combined run artifact for full meal-planning tasks, not from shop-only JSON; if recipe images are missing, present the page as cookable but image-caveated instead of substituting product packaging
   - PDF generated from the combined run artifact for full meal-planning tasks, not from shop-only JSON; if `may_present_pdf_as_complete` is false, present the PDF as diagnostic/caveated
   - current Alcampo product/price claims described only from `product_evidence_report.status`, `readiness_gate.safe_to_use_product_evidence`, and `artifact_audit.hermes_trust_summary.may_present_alcampo_products_as_current` / `may_present_product_prices_as_current`; if false, say evidence is cached, partial, missing, or caveated instead of current
   - request satisfaction described only from `safe_to_satisfy_intent`, `constraint_satisfaction_report.claim_guard.may_claim_request_satisfied`, and `artifact_audit.hermes_trust_summary.may_present_request_as_satisfied`; if false, list the failed or unknown explicit constraints
   - label-based nutrition described as exact only for the product label and safe quantity conversion; consumed totals must use scaled recipe quantities, not full purchased packages; missing labels, pantry nutrition, or unsafe conversions must be reported as unknown or partial, not zero
   - basket readiness described from final `readiness_gate.safe_to_build`; if it is false, say the basket file is blocked and not safe for automatic cart creation
   - cooking readiness described from final `readiness_gate.safe_to_cook`; if basket readiness is true but cooking readiness is false, say the basket can be priced/prepared but the meal plan still needs pantry confirmation or additional ingredients
   - recipe swaps described transparently: say which day/meal changed and why; never present the old recipe as active after a swap
   - recovered basket readiness described from the final `readiness_gate`, final ledger, and final `basket_safety`, not from the pre-recovery ledger alone
   - strict expiring-item requests verified against `pantry_usage`; if pantry matching misses a requested ingredient because names differ, use recipe-aligned pantry names or add a custom diet/allergy-safe recipe
6. Mutate cart only after explicit user approval and max spend:
   - use the basket file from `--basket-out` only when `readiness_gate.safe_to_build` is true, or write `basket_lines` to a basket file after the same readiness check; if `safe_to_cook` is false, get explicit confirmation that the user still wants cart prep before pantry confirmation is resolved
   - `carrito total -f <file> --json --max <eur>`
   - `carrito cart set-many -f <file> --max <eur> --json`
   - for cart clearing, read current cart, use a guard at or just above the verified current total, run `carrito cart clear --yes --max <eur> --json`, then verify the cart is empty
7. After the user confirms the shop was bought, picked up, or delivered, update pantry:
   - `carrito food receive <shop-or-run.json> --json`
   - This imports selected products into pantry using purchased package counts and writes a `shop_received` history event.
8. For external purchase evidence, update pantry with imports:
   - `carrito food import-receipt --file <text|-> --json`
   - `carrito food import-orders --limit <n> --infer-staples --json` when an authenticated session is available.
   - Treat receipt/order parsing as best-effort and show warnings or suggested staples before relying on them.
9. After cooking, update memory:
   - `carrito food cook <mealplan-id-or-file> --rating <1-5> --json`
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

## Quantity Ledger And Package Rules

The CLI estimates basket quantities by parsing retail sizes such as `500 g`, `1 kg`, `1,5 l`, `750 ml`, `6 x 125 g`, `3x80g`, `12 ud`, `docena`, `peso aprox. 500 g`, and drained weights such as `peso escurrido 52 g`.

- Use recipe quantities after pantry subtraction.
- Normalize compatible units: `kg -> g`, `l/cl -> ml`, and count units to `unit`.
- Buy `ceil(required / package_size)` retail packages.
- Include `quantity_reason` so the user can audit the calculation.
- Read `quantity_ledger` for the authoritative quantity and basket trust layer. It aggregates repeated requirements by selected product, computes purchased quantity, excess, missing quantity, exact/estimated/low-confidence badges, conservative offer status, and total estimates. Read `nutrition_ledger` for consumed nutrition evidence, pantry nutrition gaps, recipe-declared nutrition, and coverage.
- If package size is unknown or incompatible, keep the selected product but mark the line `needs_review` or review-only; do not describe it as exact.
- If a product is variable-weight, approximate, `al peso`, `granel`, or priced by kg without a fixed pack, report it as estimated unless the ledger explicitly marks it safe.

## Recovery Rules

Use `--recover-missing` for full meal-planning-and-shopping runs unless the user asks for a pure audit. Recovery is ledger-driven:

- First try same-ingredient Spanish aliases, such as `tiras de ternera`, `ternera en tiras`, and `filetes de ternera` for `beef strips`.
- Allow same-ingredient form transforms only when the recipe can be made honest with a prep note, such as cutting thin beef fillets into strips.
- Reject unsafe product-level mismatches, such as `carne picada` or burgers for `beef strips`.
- Do not silently change core ingredients across food classes. Chicken for beef is a recipe change or recipe swap, not an exact product recovery.
- After any recovery, use the final `quantity_ledger` and final `basket_safety` to decide readiness.
- If recovery fails or is partial, tell the user the basket remains review-only/unsafe and list the remaining issues.

## Memory Update Rules

Use local JSON memory to reduce future questions:

- Save household defaults with `food profile set`.
- Save repeat essentials with `food staples add <item> --min <qty> --unit <unit> --search <term>`.
- Save pantry/fridge/freezer facts with `food pantry add|update`.
- Save custom recipes with `food recipes add <file|-> --json`, `food recipes add --url <recipe-page-url> --json` for schema.org Recipe JSON-LD pages, or `food recipes add <file|-> --from-text --title <title> --json`.
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
- Full `food run` output includes `quantity_ledger`, `basket_safety`, and when requested `basket_optimization_plan`; incomplete or needs-review ledgers must never be summarized as ready for automatic basket creation.
   - Full `food run` output includes `manifest` and `artifact_audit` when audit flags were used; audit failure or `hermes_trust_summary.trustworthy=false` overrides favorable readiness fields until the bundle is regenerated.
   - Full `food run` output includes `product_evidence_report` when product evidence flags, run/PDF outputs, manifest, or audit are requested. Verify `status`, source counts, price/package/nutrition/image coverage, `readiness_gate.safe_to_use_product_evidence`, and audit product trust flags before saying selected Alcampo products, prices, or product-label nutrition are current.
   - Full `food run` output includes `budget_repair_plan` when `--repair-budget` or `--budget-repair-out` was used; verify `status`, applied decisions, final budget status, final readiness, and `may_present_budget_as_ready` before saying the budget was repaired. If repair failed, report it as diagnostic and do not invent manual substitutions.
   - Full `food run` output includes `meal_run_intent` and `constraint_satisfaction_report` when intent flags were used; verify `status`, `claim_guard`, `readiness_gate.safe_to_satisfy_intent`, and `may_present_request_as_satisfied` before saying the user's explicit request was satisfied. If the report has unsatisfied or unknown hard constraints, list them and keep basket/PDF readiness separate.
   - Full `food run` output includes `budget_deal_report`; verify `budget_status`, `safe_to_report_budget`, `safe_to_report_deals`, offer caveats, and `may_present_budget_as_ready`/`may_present_deals_as_ready` before claiming the plan is within budget or deal-optimized.
- Full `food run` output includes `recipe_quality_report` when recipe quality flags, run/PDF outputs, manifest, or audit are requested. Verify `readiness_gate.safe_to_use_recipes`, `recipe_quality_report.status`, image evidence, and source evidence before calling recipes cookable or the PDF complete.
- Snapshot-backed engineering runs include `manifest.snapshot`; strict replay is valid only with `replay_misses=0`, matching snapshot manifest/response hashes, and no exit 31.
- If `basket_optimization_plan.status` is `applied`, report product switches and caveats after the final readiness verdict. Optimization is an improvement step, not proof of readiness.
- If `budget_repair_plan.status` is `attempted_applied`, report the validated product switches and final budget status after the final readiness/audit verdict. If it is `attempted_failed`, `skipped_unrepairable`, or `discarded_regression`, say the plan remains budget-caveated and do not suggest unvalidated replacements.
- If `pantry_resolution` exists, report its status, assumptions, confirmed pantry lines, partial deltas, and blockers. Do not hide pantry assumptions inside generic caveats.
- If `recovery_plan` exists, applied decisions are visible and remaining issues match the final ledger.
- Nutrition is presented as complete only when `readiness_gate.safe_to_report_nutrition` is true and `nutrition_ledger` coverage supports it. If label or quantity coverage is partial, report the coverage percentage and missing/skipped Alcampo labels, pantry nutrition gaps, recipe-declared-only lines, or unsafe conversions.
- Cart mutations have a read-back `cart get --json` result proving the intended state.
- Memory mutations have pantry/history read-back when the user expects persistence.

## Roadmap For A Near-Perfect Skill

Delivered in the current CLI:

- import previous Alcampo orders as purchase history and pantry restock hints
- import plain-text receipts into pantry memory
- track expiry, filter expiring pantry items, and prioritize recipes that use them
- infer staple suggestions from previous orders with `--infer-staples`
- support custom recipe imports and user recipe libraries
- add nutrition targets, per-plan nutrition summaries, and evidence-led nutrition ledgers

Remaining future improvements:

- scan barcodes into pantry memory
- add leftover generation and automatic pantry updates after cooking
- group basket by Alcampo aisle/category for faster review
- maintain substitution history per ingredient and per brand
- compare offer products against historical prices when order history is available
