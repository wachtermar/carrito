# Contributing

Thanks for helping improve Carrito. This project has two public surfaces that must stay aligned: the Go CLI and the Agent Skill instructions.

## Development Setup

```sh
cd carrito
go test ./...
go vet ./...
go build -o carrito ./cmd/carrito
```

Use `CARRITO_CONFIG_DIR` for manual testing so you do not modify your real local state:

```sh
export CARRITO_CONFIG_DIR="$PWD/.carrito-dev"
```

## Contribution Rules

- Keep private auth material out of commits, issues, screenshots, prompts, tests, and docs.
- Do not add payment or order-submission behavior.
- Keep cart mutations guarded by explicit user intent and a nonzero spending cap.
- Prefer JSON contracts for agent-facing behavior and keep stderr as diagnostics.
- Let Hermes own recipe and preference reasoning; keep deterministic code focused on Alcampo, safety, validation, and rendering.
- Add or update tests for meal-plan validation, household/diet personas, product selection, money, auth parsing, cart guards, read-back, and HTML changes.
- Update `docs/cli.md` and `skills/carrito-shopping/references/plan-format.md` when contracts change.
- Update `skills/carrito-shopping/SKILL.md` when agent workflow or safety rules change.

## Pull Request Checklist

- `go test ./...`
- `go vet ./...`
- `go build -o carrito ./cmd/carrito`
- `git diff --check`
- README or reference docs updated when behavior changes
- no real credentials, tokens, cookies, HAR files, cURL exports, or generated personal food memory

## Live Checks

`make live-read-test` calls public read endpoints. `make auth-read-test` performs authenticated reads only. Any real cart mutation must be run manually with a low cap, a recorded starting cart, immediate read-back, and restoration of only the touched quantities.
