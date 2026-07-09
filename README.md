# Carrito Skill and CLI

Unofficial Alcampo Spain grocery automation for agents and terminals. This repository ships:

- `carrito`, a Go CLI for product search, market-aware prices, cart review, guarded cart/checkout preparation, pantry memory, meal planning, recipes, shopping lists, and PDFs.
- `skills/carrito-shopping`, a portable Agent Skill for Hermes Agent and OpenClaw.
- A root `SKILL.md` compatibility shim so Git-based skill installs can start from the repository root.

This project is not affiliated with Alcampo or Auchan. It uses private web APIs that can change without notice. Verify important prices, availability, delivery slots, and checkout state against the website before relying on them.

## What It Does

- Searches Alcampo Spain products and categories with explicit market/store context.
- Reads product detail, prices, unit prices, images, offers, and basket totals.
- Reviews authenticated carts in read-only mode with normalized JSON for agents.
- Prepares cart and checkout-slot workflows only behind explicit user approval and a nonzero spending guard.
- Stores local food memory: household profile, diets/allergies/dislikes, pantry/fridge/freezer, staples, recipes, meal plans, history, and nutrition goals.
- Generates pantry-aware meal plans, Alcampo shopping selections, nutrition evidence ledgers, basket files, recipe JSON, and printable PDFs.
- Supports desktop-safe login through `carrito login-web --if-needed`, so credentials are entered in a temporary local browser form instead of chat.

The CLI intentionally does not implement payment or order submission.

## Install

### CLI Only

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

### Hermes and OpenClaw Local Install

From the repository root:

```sh
./install-skill.sh
```

This copies the skill to:

- `~/.hermes/skills/carrito-shopping`
- `~/.openclaw/skills/carrito-shopping`

It also builds `carrito` into `~/.local/bin` when Go is available, then runs `carrito login-web --if-needed` so desktop users can authenticate when no session exists.

Useful options:

```sh
./install-skill.sh --hermes --no-login
./install-skill.sh --openclaw --bin-dir "$HOME/.local/bin"
CARRITO_INSTALL_RUN_TESTS=1 ./install-skill.sh --no-login
```

### Install From GitHub

Hermes direct GitHub install:

```sh
hermes skills install wachtermar/carrito/skills/carrito-shopping
```

OpenClaw Git install:

```sh
openclaw skills install git:wachtermar/carrito@main
```

OpenClaw uses the root `SKILL.md`, which delegates to the portable skill under `skills/carrito-shopping`. The skill metadata also declares a Go installer for `github.com/wachtermar/carrito/cmd/carrito`.

## Quick Start

Set a market before price or availability reads:

```sh
carrito set-market --region-id ac90d761-9d58-4918-a37d-dd14e1ce384a --retailer-region-id 5 --name Vaguada
carrito search leche --limit 3 --json
carrito product 54178 --json
printf '54178 2\n205192 1\n' > basket.txt
carrito total -f basket.txt --json
```

Food planning with isolated local state:

```sh
export CARRITO_CONFIG_DIR="$PWD/.carrito-dev"
carrito food profile set --people 2 --selection-policy balanced --budget 80 --json
carrito food pantry add rice --qty 500 --unit g --location pantry --json
carrito food run --days 3 --people 2 --meals dinner --servings 2 --basket-out basket.txt --run-out run.json --quantity-ledger-out ledger.json --product-evidence-out product_evidence.json --refresh-product-evidence --nutrition-ledger-out nutrition_ledger.json --recipe-intake-out recipe_intake.json --recipe-quality-out recipe_quality.json --recovery-out recovery.json --recipe-swap-out recipe_swap.json --basket-optimization-out basket_optimization.json --budget-repair-out budget_repair.json --budget-deal-out budget_deal.json --serving-plan-out serving_plan.json --scaled-mealplan-out scaled_mealplan.json --pantry-out pantry.json --pantry-consumption-out pantry_consumption.json --readiness-out readiness.json --manifest-out manifest.json --audit-out audit.json --audit-mode fail --pdf-out food-plan.pdf --enrich-products --strict-quantity --strict-servings --strict-recipe-quality --recover-missing --allow-recipe-swap --optimize-basket --repair-budget --basket-objective safe-balanced --deal-aware --default-pantry minimal-spanish --allow-assumed-pantry --require-safe-basket --require-cook-ready --json
carrito food pdf <recipe-or-plan-shop-run.json> --out food-plan.pdf
```

For Hermes-grade request satisfaction, also pass `--intent-file intent.json --intent-out intent.json --constraint-report-out constraint_report.json --require-intent-ready`. The intent file is a structured translation of the user's explicit meal, serving, budget, diet, cooking-time, nutrition, pantry, and PDF requirements.

Authenticated cart review:

```sh
carrito login-web --if-needed --json
carrito cart get --json
```

Cart writes require explicit approval and a spending cap:

```sh
carrito cart set-many -f basket.txt --max 40 --json
carrito cart get --json
```

## Agent Skill Behavior

The skill is designed for agent surfaces, not as a hidden autonomous purchasing tool.

- Agents must run `carrito login-web --if-needed --json` before authenticated reads when no session exists.
- Agents must use `--json` output and parse stdout as data; stderr is diagnostics.
- Open-ended meal plans and shopping runs must pass the intake gate first: people, meal scope, diet/allergy/dislike constraints, budget, pantry stance, and selection policy.
- Shareable cooking artifacts should use `carrito food html <run.json> --out output/html/<name>.html [--cover-image path-or-url]`; product package images are shopping evidence only, not dish or recipe photos.
- Product selections must show images, prices, quantities, package math, `product_evidence_report.status`, `readiness_gate.safe_to_use_product_evidence`, `serving_plan.status` and scaled serving assumptions when present, `quantity_ledger.status`, `nutrition_ledger.status` and `readiness_gate.safe_to_report_nutrition` when present, `recipe_quality_report.status`, `readiness_gate.safe_to_use_recipes`, `budget_repair_plan.status` and applied decisions when present, `budget_deal_report.status`, `readiness_gate.budget_status`, `readiness_gate.safe_to_report_budget`, `readiness_gate.safe_to_report_deals`, `constraint_satisfaction_report.status` and `readiness_gate.safe_to_satisfy_intent` when present, `pre_recipe_swap_readiness_gate.status` when present, `recipe_swap_plan.status` when present, `basket_optimization_plan.status` when present, `pantry_resolution.status` when present, final `readiness_gate.status`, final `readiness_gate.safe_to_build`, final `readiness_gate.safe_to_cook`, `artifact_audit.status` and `artifact_audit.hermes_trust_summary` when present, `recovery_plan.status` when present, nutrition coverage, selection reasons, alternates, missing/review-only items, and estimated totals before cart mutation. Treat exit code 20 from `food run --require-safe-basket`, `--require-cook-ready`, `--require-nutrition-ready`, `--require-budget-ready`, `--require-intent-ready`, or `--require-fresh-product-evidence` as a generated diagnostic artifact, not a tool crash; treat exit code 30 from `--audit-mode fail` as an untrusted artifact bundle that must be regenerated before presenting readiness claims. The basket file is structurally safe for cart prep only when final `readiness_gate.safe_to_build` is true and `artifact_audit.hermes_trust_summary.may_present_basket_as_ready` is not false, the meal plan is complete to cook only when final `readiness_gate.safe_to_cook` and `readiness_gate.safe_to_use_recipes` are true and the audit permits cookable recipes, the user's explicit request is satisfied only when `readiness_gate.safe_to_satisfy_intent` and `constraint_satisfaction_report.claim_guard.may_claim_request_satisfied` are true and audit trust exposes `may_present_request_as_satisfied=true`, the PDF is complete only when `artifact_audit.hermes_trust_summary.may_present_pdf_as_complete` is true, nutrition/macros are safe to present as evidence-backed only when final `readiness_gate.safe_to_report_nutrition` is true and the audit permits nutrition numbers, budget/deal claims are safe only when `readiness_gate.safe_to_report_budget`/`safe_to_report_deals` and the matching audit trust flags allow them, and current Alcampo product/price/product-label nutrition claims are safe only when `artifact_audit.hermes_trust_summary.may_present_alcampo_products_as_current`, `may_present_product_prices_as_current`, and `may_present_product_nutrition_as_current` allow them. Agents may say a budget was repaired only when `budget_repair_plan.status` is `attempted_applied`, final budget readiness is reportable, and audit trust allows budget claims; failed repair is diagnostic, not permission to invent manual substitutions.
- Hermes should create `intent.json` for serious meal-planning requests, using only explicit user constraints or saved profile facts. Vague terms such as "cheap", "healthy", or "quick" must be mapped to measurable budget, nutrition, or cooking-time thresholds before they become hard claims; otherwise they remain soft/unverifiable preferences.
- Snapshot record/replay flags are for engineering QA and Alcampo API drift reproduction. `--record-live-snapshot` and `--replay-live-snapshot --snapshot-strict` record read-only HTTP responses, block mutation-like routes, integrate into `manifest.snapshot` and artifact audit, and return exit code 31 when snapshot evidence is missing, corrupt, or unsafe.
- Cart and checkout writes require explicit approval plus `--max`, `CARRITO_MAX_EUR`, or `[limits] max_eur`.
- Payment and order submission are out of scope and unsupported.

See [skills/carrito-shopping/SKILL.md](skills/carrito-shopping/SKILL.md), [CLI reference](skills/carrito-shopping/references/cli-reference.md), and [food agent playbook](skills/carrito-shopping/references/food-agent-playbook.md) for the full agent contract.

## Configuration and Data

Default config lives under `~/.carrito`; override it with `CARRITO_CONFIG_DIR`.

- `config.toml`: market defaults, spending guard, session metadata, and imported auth material.
- `food/profile.json`: household size, diets, allergies, dislikes, budget, selection policy, brands/products, staples, and nutrition goals.
- `food/pantry.json`: pantry/fridge/freezer items with quantities, locations, expiry dates, and confidence.
- `food/recipes.db`: SQLite recipe library with embedded seed recipes plus user overrides.
- `food/mealplans/*.json`: generated meal plans.
- `food/history.jsonl`: cooked, received, imported, and feedback events.

Auth secrets are never stored in prompts or skill files. When secrets are present, `config.toml` is written with mode `0600`.

## Development

```sh
go test ./...
go vet ./...
go build -o carrito ./cmd/carrito
```

Normal tests use fixtures and do not hit the live site. Opt-in live checks are documented in [docs/cli.md](docs/cli.md).

Release checks from the repository root:

```sh
git diff --check
go test ./...
go vet ./...
goreleaser check
goreleaser release --snapshot --clean
```

## Release Notes

Before publishing a tag:

- Confirm the repo is clean and the GitHub remote is `wachtermar/carrito`.
- Run unit tests, `go vet`, a local build, and a snapshot release.
- Reinstall the skill locally with `./install-skill.sh --no-login`.
- Smoke-test a read-only product search with an isolated `CARRITO_CONFIG_DIR`.
- Smoke-test `carrito login-web --if-needed --json` only on a machine where interactive login is acceptable.
- Keep any live cart or checkout mutation tests opt-in and guarded by a low `--max`.

The CLI reference in [docs/cli.md](docs/cli.md) contains the full command reference and troubleshooting guide.
