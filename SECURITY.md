# Security Policy

## Supported Versions

Security fixes target the latest `main` branch and the newest tagged release.

## Reporting a Vulnerability

Do not open a public issue for vulnerabilities that expose credentials, auth sessions, private user data, or unsafe cart/checkout behavior.

Report privately through GitHub Security Advisories for `wachtermar/carrito`, or contact the maintainer through the GitHub profile when advisories are unavailable. Include:

- affected version or commit
- operating system
- reproduction steps
- expected and actual impact
- whether auth material, cart state, or local food memory is involved

## Security Boundaries

- This is an unofficial private-API client. API drift, blocks, and 403s are expected failure modes.
- The CLI never stores Alcampo passwords.
- Imported cookies, bearer tokens, CSRF tokens, customer ids, visitor ids, delivery-destination ids, and location hints are stored only in the local config file.
- Config files containing auth material are written with mode `0600`.
- The CLI does not implement payment or order submission.
- Cart and checkout writes require an authenticated session plus a nonzero spending guard.
- Agent skills must not paste passwords, cURL exports, HAR files, cookies, bearer tokens, or CSRF tokens into prompts or source files.

## Safe Testing

Use an isolated config directory for tests:

```sh
export CARRITO_CONFIG_DIR="$(mktemp -d)"
```

Normal `go test ./...` runs fixture-backed tests and should not hit live Alcampo endpoints. Live and authenticated checks are opt-in and documented in `docs/cli.md`.
