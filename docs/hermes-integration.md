# Hermes Agent Integration Decision

Date reviewed: 2026-07-06

Primary references:

- https://hermes-agent.nousresearch.com/docs/developer-guide/creating-skills
- https://hermes-agent.nousresearch.com/docs/developer-guide/adding-tools
- https://hermes-agent.nousresearch.com/docs/user-guide/features/skills
- https://hermes-agent.nousresearch.com/docs/user-guide/features/plugins
- https://hermes-agent.nousresearch.com/docs/guides/build-a-hermes-plugin

## Decision

Keep Carrito as a Hermes Skill that drives the local `carrito` CLI through terminal commands.

Hermes' own guidance prefers a Skill when a capability can be expressed as instructions plus shell commands, especially when wrapping an external CLI/API. That matches this project: the Go CLI already owns product search, market context, food memory, recipe planning, JSON output, cart safety checks, auth bootstrap, receipt/order import, PDF generation, and exact money/quantity logic.

Do not add a built-in Hermes core tool. The tool guide is for changes inside the Hermes repo (`tools/*.py` plus `toolsets.py`) and says most custom tools should instead be plugins. This integration is project-specific and does not need to ship as a Hermes built-in.

Do not start with a Hermes plugin. Plugins are useful when the model needs custom Python tool schemas, lifecycle hooks, or project-local tools that should be called directly. This project already has a precise CLI API with stable JSON, so a plugin would mostly duplicate the command surface and create another layer to test.

## When To Reconsider

Build a Hermes plugin/tool later only if at least one of these becomes true:

- Hermes needs direct model-visible tool schemas for individual Alcampo operations.
- Credential setup must be managed by Hermes rather than the CLI/browser login flow.
- The integration needs lifecycle hooks, background monitoring, barcode/camera streams, or push events.
- The CLI can no longer provide reliable JSON contracts for complex operations.

## Hermes-Specific Requirements

- `SKILL.md` includes Hermes frontmatter: `name`, `description`, `version`, `author`, `license`, `platforms`, and `metadata.hermes`.
- The skill uses `${HERMES_SKILL_DIR}` for bundled scripts and references because Hermes substitutes it at load time.
- The skill keeps bulky command detail in `references/cli-reference.md` and `references/food-agent-playbook.md` so normal prompts stay focused.
- The skill tells Hermes to use JSON output, verify read-backs after writes, and never place orders or handle raw secrets in chat.

## OpenClaw Compatibility

OpenClaw expects a Git-installed skill source to expose a `SKILL.md` at the source root, so this repository keeps a root compatibility shim and the canonical portable package under `skills/carrito-shopping`.

OpenClaw metadata is declared under `metadata.openclaw` and uses a `kind: go` installer for the `carrito` CLI binary. Do not use unsupported installer kinds in the skill metadata. Keep any optional auth-related environment variables declared as optional `envVars`, not required gates, because the skill supports read-only anonymous search after a market is set.

## Test Matrix

- Go unit tests: `go test ./...`
- Build: `go build -o carrito ./cmd/carrito`
- Skill install without interactive login: `./install-skill.sh --hermes --no-login`
- Hermes discovery: `hermes skills list`
- Public live reads: set a known market, run search/product/batch/total.
- Food workflows with isolated `CARRITO_CONFIG_DIR`: profile, pantry, staples, recipes, plan, shop, run, nutrition ledger, receive, cook, history, PDF.
- Hermes prompt simulations: invoke `/carrito-shopping` or `--skills carrito-shopping` with shopping, weekly meal plan, diet, nutrition/macros, pantry, waste-reduction, cart-review, and guarded cart-prep requests. Verify agents report `safe_to_build`, `safe_to_cook`, and `safe_to_report_nutrition` separately.
- OpenClaw install simulation: `openclaw skills install git:wachtermar/carrito@main`, then invoke `/carrito-shopping` in a fresh session.
