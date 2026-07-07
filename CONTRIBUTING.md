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
- Keep cart and checkout mutations guarded by explicit user intent and a nonzero spending cap.
- Prefer JSON contracts for agent-facing behavior and keep stderr as diagnostics.
- Add or update tests for planner, shopping, money, auth parsing, cart, and checkout changes.
- Update `skills/carrito-shopping/references/cli-reference.md` when command shapes change.
- Update `skills/carrito-shopping/SKILL.md` when agent workflow or safety rules change.

## Pull Request Checklist

- `go test ./...`
- `go vet ./...`
- `go build -o carrito ./cmd/carrito`
- `git diff --check`
- README or reference docs updated when behavior changes
- no real credentials, tokens, cookies, HAR files, cURL exports, or generated personal food memory

## Live Checks

`make live-test` intentionally calls public read endpoints. Authenticated live checks are opt-in and can mutate cart or slot state only when the documented environment variables are set.
