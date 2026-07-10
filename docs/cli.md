# Carrito CLI reference

All agent-facing data commands support `--json`. Diagnostics go to stderr.

## Meal plans

```text
carrito mealplan validate <plan.json> [--selected] [--json]
carrito mealplan candidates <plan.json> [--limit 1..10] [--store REGION_ID] [--json]
carrito mealplan build <plan.json> [--html-out FILE] [--basket-out FILE] [--store REGION_ID] [--json]
```

- `validate` checks the canonical plan. `--selected` also requires the exact selected candidate SKU in `product_sku` and a positive package count for each shopping line.
- `candidates` searches the configured Alcampo market for each shopping query and returns up to four products by default. Complete results use `status: "ready"`; any failed or empty search returns `status: "incomplete"`, `unresolved_count`, per-item errors, and a nonzero exit.
- `build` re-reads every selected product, rejects missing price or unavailable products, calculates the selected-product total, writes a guarded basket file, and renders the single-file cooking HTML.
- Default build paths replace `.json` with `.html` and `.basket.txt`.

The JSON format is documented in [plan-format.md](../skills/carrito-shopping/references/plan-format.md).

## Catalog

```text
carrito search <query> [--limit 1..50] [--store REGION_ID] [--json]
carrito product <sku|product-id|URL> [--store REGION_ID] [--json]
```

Search returns current listing data. Product refreshes the exact current-market listing, keeps its identity, price, size, and availability authoritative, and enriches it only with static page details such as ingredients, allergens, nutrition text, and images. SKU reads are public; resolving an internal product UUID can require an authenticated session, so meal plans store candidate SKUs in `product_sku`. Missing online label fields are unknown, not proof that a product is allergy-safe.

## Cart

```text
carrito cart get [--json] [--raw]
carrito cart add <product-ref> <quantity> --max EUR [--json]
carrito cart set <product-ref> <quantity> --max EUR [--json]
carrito cart set-many -f <basket-file|-> --max EUR [--json]
```

Every write requires:

- an authenticated Alcampo session;
- a CSRF token;
- a readable current cart total;
- a current product price;
- a positive maximum total from `--max`, `CARRITO_MAX_EUR`, or config.

The basket format is one minimum quantity per line:

```text
internal-product-id 2 # optional comment
another-product-id 1
```

Before requesting approval for a plan write, run `cart get --json` and explain that `--max` limits the whole final cart.

`set-many` ensures each named product is present at least at its basket quantity. It never reduces a larger existing quantity and does not change unrelated lines. After writing, it reads the cart back and succeeds only with `verified: true`, verified actual quantities, and the actual `cart_total_after`. If verification fails after a confirmed write, it reverses only its own deltas and verifies them, so a concurrent addition is not erased. Ambiguous or failed reversals require manual cart review rather than a stale-snapshot restore.

## Login and market

```text
carrito login-web [--if-needed] [--json]
carrito whoami [--json]
carrito market [--json]
carrito addresses [--json]
carrito set-address <delivery_destination_id> [--json]
carrito set-market --region-id UUID [--retailer-region-id ID] [--name NAME] [--json]
```

`login-web` starts a temporary local form so the user enters credentials outside chat. With `--if-needed`, it skips only when authentication material and a CSRF token are both saved, which makes the session cart-write ready. Session material is written only to the local config file. CAPTCHA, MFA, disabled-account, and consent challenges are not bypassed.

## Exit behavior

Normal failures return a nonzero exit code and an `error:` line on stderr. Invalid plan errors name the exact JSON path. Incomplete candidates also return their JSON payload before the nonzero exit. An estimated spending-guard failure occurs before mutation; a post-write verification failure triggers guarded delta reversal when it is safe.

There are no checkout, slot reservation, order submission, deterministic recipe planning, pantry lifecycle, PDF, nutrition-ledger, or artifact-audit commands.
