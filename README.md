# Carrito

Carrito gives Hermes Agent one useful grocery workflow:

1. Hermes designs recipes for the actual household.
2. Carrito finds current Alcampo Spain products and prices.
3. The user approves a maximum spend.
4. Carrito updates the cart and verifies it.
5. Hermes returns one mobile-friendly HTML file to share with whoever is cooking.

The model handles flexible reasoning; the CLI handles deterministic and risky work. There is no embedded recipe database, pantry engine, nutrition ledger, readiness matrix, PDF pipeline, checkout flow, or order submission.

For the end-to-end control path and the exact code that enforces it, see [Architecture and safety flow](docs/architecture.md).

This project is unofficial and is not affiliated with Alcampo or Auchan. It uses private web APIs that may change. Product labels and physical packaging remain authoritative for allergies.

## Install for Hermes

Requirements: Hermes Agent, Go 1.25 or newer, macOS or Linux.

```sh
git clone https://github.com/wachtermar/carrito.git
cd carrito
./install-skill.sh
```

The installer copies the complete multi-file skill to `~/.hermes/skills/carrito-shopping` and builds `carrito` into `~/.local/bin`. The skill uses its bundled absolute-path launcher, so that directory does not need to be in Hermes' `PATH`. The installer does not open a login window unless requested:

```sh
./install-skill.sh --login
```

From a published GitHub source, install the skill directory rather than a raw `SKILL.md` URL so Hermes receives its reference and template files:

```sh
hermes skills install wachtermar/carrito/skills/carrito-shopping
```

Then ask Hermes:

```text
/carrito-shopping Plan five simple dinners for two adults and a toddler,
avoid peanuts, add the groceries to my Alcampo cart, and give me the cooking page.
```

Hermes asks only for missing hard facts and the final-cart spending cap. Login happens in a local browser form; credentials do not go through chat.

## The small contract

Hermes writes one plan JSON with:

- household size, hard dietary rules, allergies, and dislikes;
- day-by-day meals with servings, timing, ingredients, and cooking steps;
- one consolidated shopping list;
- each selected candidate SKU in `product_sku`, plus its package count.

The exact format is documented in [plan-format.md](skills/carrito-shopping/references/plan-format.md).

Carrito exposes three meal-plan operations:

The manual examples below assume `~/.local/bin` is in `PATH`; otherwise invoke `~/.local/bin/carrito` explicitly. Hermes uses the bundled launcher and does not need this setup.

```sh
carrito mealplan validate plan.json --json
carrito mealplan candidates plan.json --limit 4 --json
carrito mealplan build plan.json \
  --html-out family-plan.html \
  --basket-out family-plan.basket.txt \
  --json
```

`candidates` returns `status: "ready"` only when every shopping line has usable products. Otherwise it returns `status: "incomplete"` with `unresolved_count` and exits nonzero.

Before asking for approval, Hermes reads the existing cart so the user sees the current total and understands that `--max` limits the whole final cart:

```sh
carrito login-web --if-needed --json
carrito cart get --json
```

For a write, the login JSON must report both `"authenticated": true` and `"has_csrf_token": true`; `--if-needed` reopens login when either requirement is missing.

After explicit approval:

```sh
carrito cart set-many -f family-plan.basket.txt --max 100 --json
```

Basket quantities are minimum targets: `set-many` raises named products when needed, never reduces a larger existing quantity, and leaves unrelated lines alone. It reads the cart back itself, verifies the actual quantities and final total, and returns `verified: true`. If verification fails after a confirmed write, it reverses only its own deltas and verifies them; it never restores a stale snapshot over concurrent additions. Ambiguous outcomes require manual review. The CLI never submits checkout or payment.

## Setup and diagnostics

Prices depend on the delivery market:

```sh
carrito market --json
carrito addresses --json
carrito set-address <delivery_destination_id>
```

Catalog reads (use the candidate SKU before login):

```sh
carrito search "pechuga de pollo" --limit 5 --json
carrito product <sku> --json
```

Internal product UUID decoration can require an authenticated session; generated basket files use those UUIDs only at cart time. Configuration is stored in `~/.carrito/config.toml` with mode `0600`. Set `CARRITO_CONFIG_DIR` to isolate it. Set `CARRITO_BASE_URL` only for a mock/test server.

## Development

```sh
make check
```

This runs formatting checks, unit and integration tests, `go vet`, and a build. Tests use local mock servers and do not mutate a real cart.

For a real account smoke test, first record the current cart, use a low explicit `--max`, add only known test items, require `set-many` to return `verified: true`, and restore only the quantities the test changed. If automatic rollback fails, review the cart manually. Never run live cart mutation in unattended CI.
