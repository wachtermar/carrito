---
name: alcampo-shopping
description: "Use alcampo-cli for Alcampo Spain meal planning, recipes, pantry-aware shopping, product search, basket totals, cart checks, and guarded checkout slot workflows."
license: MIT
metadata:
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

Use the local `alcampo` CLI for Alcampo Spain food tasks: meal planning, recipes, pantry/fridge memory, transparent product selection, basket pricing, cart preparation, and checkout slot workflows. The CLI is an unofficial private-API client, so verify important results against the site when precision matters and expect occasional API drift.

Resolve `{baseDir}` as the directory containing this `SKILL.md`.

## Setup

1. Check for the binary with `command -v alcampo`.
2. If it is missing and this skill was installed from a repo checkout, run `{baseDir}/scripts/install-alcampo-cli.sh`.
3. If the installer cannot find source, ask for the local `alcampo-cli` checkout path or a repo URL, then rerun the installer with that path as the first argument.
4. Prefer `alcampo --help` after install to confirm the executable works.

## Workflow

1. Establish market context before price, availability, category product, batch, total, or food shopping reads.
2. Use `alcampo market` to inspect the current market. If no market is set, ask for a delivery address/session or explicitly set a known market with `alcampo set-market`.
3. Prefer `--json` for agent workflows and parse stdout as data. Treat stderr as logs, warnings, prompts, or errors.
4. For meal planning, load or update food memory first:
   - `alcampo food profile get --json`
   - `alcampo food pantry list --json`
   - `alcampo food staples list --json`
   - `alcampo food profile set --selection-policy balanced|cheapest|quality` if no policy is saved.
5. For low-intervention planning and shopping, prefer `alcampo food run --days <n> --people <n> --meals dinner --selection-policy <policy> --basket-out basket.txt --json`.
6. For separate steps, generate a plan with `alcampo food plan --days <n> --people <n> --json`, then shop it with `alcampo food shop <mealplan-id-or-file> --basket-out basket.txt --json`.
7. Show the user selected products, grouped shopping sections, item images, offers, prices, package quantity calculations, reasons chosen, alternates considered, missing/unavailable items, basket file path, and estimated total before any cart mutation.
8. Create printable artifacts with `alcampo food recipe <prompt|mealplan-id> --json --out <recipe.json>` and `alcampo food pdf <recipe-or-plan-or-shop.json> --out <file.pdf>`.
9. After the user confirms a shop was actually bought or delivered, use `alcampo food receive <shop-or-run.json> --json` to add received products to pantry memory and append history.
10. After the user cooks a plan, use `alcampo food cook <mealplan-id-or-file> --rating <1-5> --json` to update pantry quantities, append history, learn liked recipes from high ratings, and learn rejected recipes from low ratings.
11. Use `alcampo food history list --json` when prior cooking or receiving feedback would improve a new plan.
12. For basket pricing, write or receive a basket file with `<product_id_or_sku> <qty>` per line, then run `alcampo total -f <file> --json`.
13. For cart or checkout writes, require an authenticated/imported session and a nonzero spending guard: `--max <eur>`, `ALCAMPO_MAX_EUR`, or `[limits] max_eur`.

Load `{baseDir}/references/food-agent-playbook.md` for detailed autonomous meal-planning, product-selection, offer, image, quantity, memory, and human-approval rules.

## Food Memory

Food memory is local JSON under `ALCAMPO_CONFIG_DIR/food/` or `~/.alcampo/food/`:

- `profile.json`: household size, diets, allergies, dislikes, liked cuisines, liked/rejected recipes, liked/rejected products, budget, preferred/rejected brands, and persistent `selection_policy`.
- `profile.json` also stores repeat `staples` with minimum quantities so low-stock essentials can be added automatically.
- `pantry.json`: pantry/fridge/freezer items, quantities, units, locations, expiry dates, confidence, and last checked timestamps.
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
- Use runner secrets or local environment variables for `ALCAMPO_CURL`, `ALCAMPO_USERNAME`, `ALCAMPO_PASSWORD`, and `ALCAMPO_MAX_EUR`.
- Do not bypass CAPTCHA, MFA, disabled-account, consent, or risk checks. If the site requires one, stop and report it.
- Do not mutate cart or checkout state unless the user explicitly asks and gives a maximum spend.

## References

Load `{baseDir}/references/cli-reference.md` when you need exact command shapes, basket format, auth bootstrap examples, troubleshooting, or checkout details.
