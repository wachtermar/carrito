# Architecture and safety flow

Carrito separates flexible planning from deterministic cart work. The model can propose meals and product candidates. The CLI owns schema validation, price and availability checks, spending limits, cart writes, read-back, rollback, and the hard stop before checkout.

## End-to-end path

| Stage | Owner | Input | Required evidence before continuing | Failure behavior |
|---|---|---|---|---|
| 1. Plan | Hermes Agent | Household rules and meal request | Complete plan JSON | Ask only for missing hard facts |
| 2. Validate | `mealplan validate` | Plan JSON | Valid schema and consistent quantities | Exit nonzero; no catalog or cart action |
| 3. Resolve | `mealplan candidates` | Shopping lines | Usable product ID, SKU, price, size, and availability for every line | Return `status: "incomplete"` and unresolved count |
| 4. Build | `mealplan build` | Resolved plan | Shareable HTML and basket file written to distinct safe paths | Refuse aliased/overwriting output paths |
| 5. Authenticate | `login-web --if-needed` | Local browser session | Authenticated session and CSRF token | Refuse all writes |
| 6. Read current state | `cart get` | Active cart | Whole-cart EUR total and unambiguous product quantities | Refuse the write if state cannot be verified |
| 7. Guard | `cart set-many --max` | Basket file and explicit EUR cap | Estimated whole-cart total at or below the cap | Refuse before mutation |
| 8. Write and read back | Cart client | Minimum target quantities | Persisted quantities and whole-cart total match the requested deltas | Attempt bounded recovery only when the result is safe to reverse |
| 9. Recover | Rollback path | Original quantities and Carrito's own deltas | Reversal read-back matches the bounded change | Stop and require manual review if the outcome is ambiguous |
| 10. Checkout | Person | Verified cart | Human review of products, delivery, substitutions, and payment | Carrito has no checkout or order-submission command |

## Trust boundaries

### Model to CLI

The plan is treated as untrusted input. [`internal/mealplan`](../internal/mealplan) validates its shape and output paths. Catalog candidates are not considered buildable until the fields used by later stages are present.

### CLI to retailer API

Cart writes require both an authenticated local session and a CSRF token. [`internal/cli/safety.go`](../internal/cli/safety.go) also requires a positive spending guard and rejects an unreadable or non-EUR whole-cart total.

### Write response to persisted state

A successful write response is not accepted as final state. [`internal/cli/cart.go`](../internal/cli/cart.go) reads the cart again, checks exact quantities and the final total, and reports `verified: true` only after those checks pass.

### Automatic recovery to manual review

Rollback is limited to Carrito's own deltas. It does not restore a stale whole-cart snapshot over concurrent additions. If the write response and persisted state do not conservatively identify what changed, the CLI stops and asks for manual review.

### Cart to checkout

The automation boundary is explicit: Carrito prepares and verifies the cart. A person owns checkout, payment, delivery details, and substitutions.

## Reproducible verification

The focused integration test runs the complete safe path against a local mock retailer service: candidate resolution, output generation, guarded cart mutation, read-back, and an idempotent second run.

```sh
go test ./internal/cli \
  -run TestMealPlanCandidatesBuildAndGuardedCartReadback \
  -v
```

Abridged expected output:

```text
=== RUN   TestMealPlanCandidatesBuildAndGuardedCartReadback
--- PASS: TestMealPlanCandidatesBuildAndGuardedCartReadback
PASS
```

The full local gate is:

```sh
make check
```

It runs all Go tests, `go vet`, an installer clone smoke test, and a clean CLI build. Tests use mock servers and do not mutate a real cart.
