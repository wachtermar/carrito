---
name: alcampo-shopping
description: "Use when a user wants Alcampo Spain grocery search/cart review, pantry-aware meal plans, diet-aware shopping lists, recipe PDFs, basket pricing, or guarded checkout-slot prep through alcampo-cli."
version: 1.0.0
author: Marcel Wachter
license: MIT
platforms: [linux, macos]
metadata:
  hermes:
    tags: [Shopping, Grocery, Alcampo, Meal Planning, Pantry, Recipes, Spain]
    category: productivity
    config:
      - key: alcampo.bin_path
        description: Path to the alcampo CLI executable
        default: "~/.local/bin/alcampo"
        prompt: Alcampo CLI path
      - key: alcampo.config_dir
        description: Optional alcampo CLI config directory
        default: "~/.alcampo"
        prompt: Alcampo config directory
  openclaw:
    install:
      - id: brew-go
        kind: brew
        formula: go
        bins: [go]
        label: "Install Go with Homebrew"
      - id: apt-go
        kind: apt
        package: golang-go
        bins: [go]
        label: "Install Go with apt"
---

# Alcampo Food Shopping

## Overview

Use the local `alcampo` CLI for Alcampo Spain food tasks: grocery search, read-only cart review, meal planning, diet and allergy-aware recipes, pantry/fridge/freezer memory, transparent product selection, basket pricing, guarded cart preparation, and checkout slot workflows. The CLI is an unofficial private-API client, so verify important results against the site when precision matters and expect occasional API drift.

This should remain a Hermes Skill, not a built-in Hermes Tool: all precise behavior lives in the `alcampo` CLI, Hermes only needs repeatable instructions plus terminal commands, and the CLI already owns auth, JSON parsing, money math, food memory, and safety guards. Escalate to a Hermes plugin/tool only if the project later needs model-visible Python tool schemas, background hooks, streaming/barcode capture, or Hermes-managed credential flows.

In Hermes, `${HERMES_SKILL_DIR}` is substituted with this skill directory. In other agents, resolve it manually as the directory containing this `SKILL.md`.

## When to Use

- The user asks to search Alcampo products, compare prices/offers, inspect categories, price a basket, or view their current cart.
- The user asks for weekly meal plans, diet-specific plans, nutrition targets, allergies, family/household planning, recipe generation, or recipe PDFs.
- The user asks to use pantry/fridge/freezer items, reduce waste, use expiring items, remember staples, import receipts/orders, or update pantry after delivery/cooking.
- The user asks to prepare an Alcampo basket, add/update/clear cart items, inspect delivery addresses, or choose checkout slots. These are guarded workflows and require explicit approval and a spending cap for writes.

## Setup

1. Resolve the CLI executable before running commands. Prefer a configured Hermes skill setting `alcampo.bin_path` when present and executable; otherwise use `command -v alcampo`; otherwise use `$HOME/.local/bin/alcampo` if it exists. In the rest of this skill, replace `alcampo` with that resolved executable path when needed.
2. If no executable is available and this skill was installed from a repo checkout, run `${HERMES_SKILL_DIR}/scripts/install-alcampo-cli.sh`. Local installs may include `${HERMES_SKILL_DIR}/.alcampo-cli-source`, which the installer uses automatically.
3. If the installer cannot find source, ask for the local `alcampo-cli` checkout path or a repo URL, then rerun the installer with that path as the first argument.
4. Prefer `<resolved-alcampo> --help` after install to confirm the executable works. During normal user tasks, do not probe subcommand `--help`; use `references/cli-reference.md` for command shapes. If help is needed for troubleshooting, run it as a standalone diagnostic and treat usage text on stderr as normal.
5. Before authenticated commands, run `<resolved-alcampo> login-web --if-needed --json`; this opens a temporary local browser login form only when the user is not already logged in.
6. If Hermes injects `alcampo.config_dir`, set `ALCAMPO_CONFIG_DIR` for CLI commands only when the user wants that non-default state directory. Do not move auth or food memory silently.

## Workflow

1. Establish market context before price, availability, category product, batch, total, or food shopping reads.
2. Use `alcampo market` to inspect the current market. If no market is set, ask for a delivery address/session or explicitly set a known market with `alcampo set-market`.
3. Prefer `--json` for agent workflows and parse stdout as data. Treat stderr as logs, warnings, prompts, or errors.
4. If an authenticated command reports no session, or before order imports/cart/checkout/address reads, run `alcampo login-web --if-needed --json` with the resolved executable and let the user complete the browser form.
5. When the user asks to show, view, check, inspect, or list their current Alcampo cart/basket/carrito, treat it as a read-only cart request: run `alcampo login-web --if-needed --json` with the resolved executable, then `alcampo cart get --json` with the resolved executable. Present item images, names, quantities, package prices, unit prices, line totals, offers, product URLs, and cart total. Do not require a max spend for this read-only command.
6. For open-ended meal planning or shopping, run the intake gate before generating anything. This is mandatory for requests like "make a weekly meal plan", "shop for the week", or "breakfast lunch dinner" when the user did not already provide enough details in the current message or saved profile.
7. Intake gate:
   - First load `alcampo food profile get --json`, `alcampo food pantry list --json`, and `alcampo food staples list --json`.
   - If household size is missing from both the current request and profile, ask "How many people am I planning for?"
   - If diet/allergy/dislike constraints are missing and the plan introduces new foods, ask "Any diet, allergies, dislikes, or foods to avoid?"
   - If budget is missing for a weekly/full meal plan, ask "What weekly grocery budget should I target, or should I ignore budget?" Budget is required before shopping/product selection and should still be offered for planning because it changes recipe choices.
   - If meal scope is ambiguous, ask which meals and how many days; infer only explicit words such as "week", "breakfast", "lunch", or "dinner".
   - Ask whether to use pantry/fridge/freezer items, expiring foods, staples, and past liked/rejected recipes when those are unknown or empty.
   - Ask selection policy (`balanced`, `cheapest`, or `quality`) before shopping if none is saved. For planning-only requests, ask whether the plan should optimize for cheapest, balanced, quality/freshness, speed, batch-cooking, or variety. Do not silently choose one for a real user-facing shopping run.
   - Ask for cart approval and maximum spend before any cart mutation. Planning and review can proceed without cart approval after intake is complete.
   - Ask the missing questions together in one compact message and stop. Do not run `food plan`, `food shop`, or `food run` until the answers or saved profile cover the missing high-impact fields.
8. For meal planning after intake is complete, load or update food memory first:
   - `alcampo food profile get --json`
   - `alcampo food pantry list --json`
   - `alcampo food staples list --json`
   - `alcampo food recipes list --json` or `alcampo food recipes search <query> --profile --json` when the user asks what meals are available or wants custom recipe work.
   - `alcampo food profile set --selection-policy balanced|cheapest|quality` if no policy is saved.
9. Use `alcampo food pantry list --expiring-days 3 --json` and `alcampo food use-up --json` when the user wants to reduce waste or use expiring items.
10. For low-intervention planning and shopping after intake, prefer `alcampo food run --days <n> --people <n> --meals dinner --selection-policy <policy> --basket-out basket.txt --json`.
11. For separate steps, generate a plan with `alcampo food plan --days <n> --people <n> --json`, then shop it with `alcampo food shop <mealplan-id-or-file> --basket-out basket.txt --json`.
12. Show the user selected products, grouped shopping sections, item images, offers, prices, package quantity calculations, nutrition summaries, reasons chosen, alternates considered, missing/unavailable items, basket file path, and estimated total before any cart mutation.
13. When the user asks to strictly use a specific expiring quantity, verify `pantry_usage` after planning. Pantry matching is name-sensitive, so use recipe-aligned pantry names or add a custom diet/allergy-safe recipe when seed recipes cannot consume the requested amount.
14. Create printable artifacts with `alcampo food recipe <prompt|mealplan-id> --json --out <recipe.json>` and `alcampo food pdf <recipe-or-plan-or-shop.json> --out <file.pdf>`.
15. Add custom recipes with `alcampo food recipes add <file|-> --json`, or `--from-text --title <title>` for pasted ingredient lists. For external recipe webpages, prefer extracting the page's Recipe JSON-LD/schema data, then write a full recipe JSON file and import it with `alcampo food recipes add <file> --json`; this preserves ingredients, steps, timing, tags, image, allergens, and nutrition better than `--from-text`. Add Spanish Alcampo-oriented `search_term` values for ingredients when possible, then verify with both `alcampo food recipes show <id> --json` and `alcampo food recipes search <query> --json`.
16. After the user confirms a shop was actually bought or delivered, use `alcampo food receive <shop-or-run.json> --json` to add received products to pantry memory and append history.
17. Import external pantry evidence with `alcampo food import-receipt --file <text|-> --json`, or authenticated best-effort order history with `alcampo food import-orders --limit <n> --infer-staples --json`.
18. After the user cooks a plan, use `alcampo food cook <mealplan-id-or-file> --rating <1-5> --json` to update pantry quantities, append history, learn liked recipes from high ratings, and learn rejected recipes from low ratings.
19. Use `alcampo food history list --json` when prior cooking, receiving, receipt, or order import feedback would improve a new plan.
20. For basket pricing, write or receive a basket file with `<product_id_or_sku> <qty>` per line, then run `alcampo total -f <file> --json`.
21. For cart or checkout writes, require an authenticated/imported session and a nonzero spending guard: `--max <eur>`, `ALCAMPO_MAX_EUR`, or `[limits] max_eur`.
22. When the user asks to remove or clear all cart items, treat it as an explicit cart mutation but not a purchase: authenticate if needed, read the current cart total, then run `alcampo cart clear --yes --max <guard> --json` using a guard at or just above the verified current cart total. Immediately verify with `alcampo cart get --json` and report the cart is empty only if `item_count` is `0` and total is `0`.

Load `${HERMES_SKILL_DIR}/references/food-agent-playbook.md` for detailed autonomous meal-planning, product-selection, offer, image, quantity, memory, and human-approval rules. Load `${HERMES_SKILL_DIR}/references/intake-scenarios.md` when handling open-ended meal planning or shopping, or when verifying that a previous Hermes turn asked enough questions before acting.

## Food Memory

Food memory is local data under `ALCAMPO_CONFIG_DIR/food/` or `~/.alcampo/food/`:

- `profile.json`: household size, diets, allergies, dislikes, liked cuisines, liked/rejected recipes, liked/rejected products, budget, preferred/rejected brands, and persistent `selection_policy`.
- `profile.json` can store free-form `nutrition_goals` such as `kcal<=2200` or `protein>=90`; meal plans add warning notes when computed totals miss them.
- `profile.json` also stores repeat `staples` with minimum quantities so low-stock essentials can be added automatically.
- `pantry.json`: pantry/fridge/freezer items, quantities, units, locations, expiry dates, confidence, and last checked timestamps.
- `recipes.db`: SQLite recipe database with normalized recipe, tag, ingredient, step, nutrition, and full-text search tables. Embedded seed recipes are loaded automatically, custom recipes added through the CLI override seed ids, and legacy `recipes/*.json` files are imported once for migration.
- `mealplans/*.json`: saved meal plans with recipes, pantry usage, required purchases, and notes.
- `history.jsonl`: append-only cooked plans, ratings, pantry updates, substitutions, and rejected products.

Selection policy is user preference, not a hardcoded default. If `profile.json` has no `selection_policy`, ask once and then save one of:

- `balanced`: compatible and available first, then unit price, package fit, offers, brand history, and match quality.
- `cheapest`: minimize unit/package price after diet/allergy/dislike checks.
- `quality`: prefer trusted brands, richer product data, nutrition/freshness signals, and package fit before price.

When presenting shopping output, always include product image URLs when present. Do not hide why an item was chosen; surface `shopping_groups`, `selection_reason`, alternates, and any exported basket path.

## Safety

- Never submit payment or place an order. The CLI intentionally does not implement payment or order submission.
- Never store passwords, cookies, cURL exports, HAR files, CSRF tokens, or bearer tokens in prompts, source files, or skill files.
- Food memory may store preferences and pantry facts, but not Alcampo auth secrets.
- Prefer `alcampo login-web` on desktop agent surfaces so the user enters credentials in a temporary local browser form that the agent cannot see.
- Use runner secrets or local environment variables for `ALCAMPO_CURL`, `ALCAMPO_USERNAME`, `ALCAMPO_PASSWORD`, and `ALCAMPO_MAX_EUR` when automation needs non-interactive auth.
- Do not bypass CAPTCHA, MFA, disabled-account, consent, or risk checks. If the site requires one, stop and report it.
- Do not mutate cart or checkout state unless the user explicitly asks and gives a maximum spend.

## Verification

- Always parse `--json` stdout as data before summarizing. Treat stderr as diagnostics, not data.
- For search/product/category/total reads, verify a market is set and the JSON contains prices or clear warnings about unavailable data.
- For open-ended multi-day meal plans, verify that the intake gate was satisfied before any plan command ran; if the transcript shows missing people/diet/allergy/budget/scope/pantry policy questions, the run is invalid.
- For meal plans, verify the result has planned days/meals or recipe entries, pantry usage when available, required purchases, nutrition when available, and saved output paths when requested. For multi-meal plans (`breakfast,lunch,dinner`), also spot-check that each slot selected recipes tagged for that meal type; if breakfasts are filled with lunch/dinner recipes, fix the planner to filter/rank candidates per meal type and add a regression test before presenting the plan.
- For shopping results, verify every required purchase either has a selected product with price/image/reasoning or a visible unavailable/missing warning.
- For PDFs, verify the output file exists and is non-empty before telling the user it is ready.
- For cart writes, run a read-back command (`cart get --json`) and compare totals/item counts before reporting success.
- For `food receive` or `food cook`, read back pantry and/or history when the user expects memory to be updated.
- To regression-check an intake-only Hermes response, run `${HERMES_SKILL_DIR}/scripts/check-intake-response.py --mode meal-plan <response.txt>` or `--mode shopping <response.txt>`.

## References

Load `${HERMES_SKILL_DIR}/references/cli-reference.md` when you need exact command shapes, basket format, auth bootstrap examples, troubleshooting, or checkout details.
