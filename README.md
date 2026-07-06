# Alcampo Food Agent Skill

Portable Agent Skill package for using the unofficial `alcampo` CLI from Hermes Agent and OpenClaw. It supports Alcampo Spain product search plus read-only cart viewing with images/quantities/prices, pantry-aware meal plans, recipes, recipe/shopping PDFs, transparent product selection, basket pricing, and guarded cart/checkout workflows.

## One-command local install

```sh
./install-skill.sh
```

This installs `skills/alcampo-shopping` into:

- `~/.hermes/skills/alcampo-shopping`
- `~/.openclaw/skills/alcampo-shopping`

It also builds the `alcampo` binary into `~/.local/bin` when Go is available, then runs `alcampo login-web --if-needed` so desktop users get a browser login form when no Alcampo session exists. Use `./install-skill.sh --no-login` or `ALCAMPO_INSTALL_LOGIN=0 ./install-skill.sh` to skip the login check.

## Install after publishing

Once this repository is pushed to GitHub, replace `OWNER/REPO` with the real repository path:

```sh
openclaw skills install git:OWNER/REPO
hermes skills install OWNER/REPO/skills/alcampo-shopping
```

The root `SKILL.md` exists for OpenClaw Git installs. The portable skill package lives at `skills/alcampo-shopping`.

## CLI source

The Go CLI lives in `alcampo-cli/`. See `alcampo-cli/README.md` for command examples, auth handling, food memory, and development notes.

The food planner now uses an embedded plus user-editable JSON recipe library. Add or override recipes under `~/.alcampo/food/recipes/*.json`, or use `alcampo food recipes add <file|->` and `alcampo food recipes add <file|-> --from-text` for pasted ingredient lists. Receipt/order imports, use-up planning for expiring pantry items, and nutrition summaries are documented in the CLI README.
