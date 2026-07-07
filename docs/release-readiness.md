# Release Readiness

Last reviewed: 2026-07-07

## Public Surfaces

This repository publishes three related surfaces:

- Go CLI: `carrito`, installed from `github.com/wachtermar/carrito/cmd/carrito`.
- Hermes skill: `skills/carrito-shopping`, installable as `wachtermar/carrito/skills/carrito-shopping`.
- OpenClaw skill: root `SKILL.md` for Git installs, delegating to `skills/carrito-shopping`.

## Compatibility Notes

- Hermes loads skills from `~/.hermes/skills` and supports direct GitHub skill IDs. The nested skill is the canonical Hermes package.
- OpenClaw Git installs expect a `SKILL.md` at the source root, so the root skill is intentionally a compatibility shim.
- The Agent Skills spec expects a skill folder with `SKILL.md`, optional `scripts/`, optional `references/`, and optional `assets/`. The nested `skills/carrito-shopping` directory follows that shape.
- OpenClaw dependency metadata should use `metadata.openclaw.install` with supported installer kinds. This repo declares the CLI as a Go-installed dependency.

## Release Gates

Required before a public tag:

```sh
git diff --check
go test ./...
go vet ./...
go build -trimpath -o carrito ./cmd/carrito
goreleaser check
goreleaser release --snapshot --clean
```

Recommended manual smoke checks:

```sh
tmpdir="$(mktemp -d)"
CARRITO_CONFIG_DIR="$tmpdir" ./carrito set-market --region-id ac90d761-9d58-4918-a37d-dd14e1ce384a --retailer-region-id 5 --name Vaguada
CARRITO_CONFIG_DIR="$tmpdir" ./carrito search leche --limit 3 --json
CARRITO_CONFIG_DIR="$tmpdir" ./carrito food profile set --people 2 --selection-policy balanced --budget 40 --json
CARRITO_CONFIG_DIR="$tmpdir" ./carrito food plan --days 1 --people 2 --meals dinner --json
```

Skill install checks:

```sh
./install-skill.sh --hermes --no-login
./install-skill.sh --openclaw --no-login
~/.local/bin/carrito --help
```

Authenticated checks are optional and must never run in CI. If used, run them manually with a low spending guard and verify cart state afterward:

```sh
carrito login-web --if-needed --json
carrito cart get --json
```

## Safety Invariants

- No payment or order submission endpoint is implemented.
- Unknown checkout commands fail with `ErrCheckoutUnsupported`.
- Cart and checkout writes require auth, CSRF token, verified cart total, and nonzero max spend.
- Read-only cart review does not require max spend.
- Skill instructions must ask intake questions before open-ended meal plans or shopping runs.
- Agent instructions must keep auth material out of prompts and source files.

## Known Release Risks

- Alcampo private APIs can change without warning.
- Anonymous postal-code-to-region resolution is blocked outside a full web session.
- Auth refresh is not implemented; users must rerun login or import a fresh session when the site expires it.
- Live read checks can fail from temporary blocks, rate limits, or API drift even when fixture-backed tests pass.
- ClawHub marketplace publication has separate policy requirements, including registry licensing rules; Git install compatibility does not imply ClawHub acceptance.
