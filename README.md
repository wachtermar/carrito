# Alcampo Food Agent Skill

Portable Agent Skill package for using the unofficial `alcampo` CLI from Hermes Agent and OpenClaw. It supports Alcampo Spain product search plus pantry-aware meal plans, recipes, recipe/shopping PDFs, transparent product selection, basket pricing, and guarded cart/checkout workflows.

## One-command local install

```sh
./install-skill.sh
```

This installs `skills/alcampo-shopping` into:

- `~/.hermes/skills/alcampo-shopping`
- `~/.openclaw/skills/alcampo-shopping`

It also builds the `alcampo` binary into `~/.local/bin` when Go is available.

## Install after publishing

Once this repository is pushed to GitHub, replace `OWNER/REPO` with the real repository path:

```sh
openclaw skills install git:OWNER/REPO
hermes skills install OWNER/REPO/skills/alcampo-shopping
```

The root `SKILL.md` exists for OpenClaw Git installs. The portable skill package lives at `skills/alcampo-shopping`.

## CLI source

The Go CLI lives in `alcampo-cli/`. See `alcampo-cli/README.md` for command examples, auth handling, food memory, and development notes.
