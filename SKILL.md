---
name: carrito-shopping
description: "Use when a user wants Alcampo Spain grocery search/cart review, pantry-aware meal plans, diet-aware shopping lists, recipe PDFs, basket pricing, or guarded checkout-slot prep through carrito."
version: 1.0.0
author: Marcel Wachter
license: MIT
compatibility: "Requires macOS or Linux plus the carrito CLI. Go 1.25+ is needed only when building the CLI from source."
platforms: [linux, macos]
metadata:
  hermes:
    homepage: https://github.com/wachtermar/carrito
    tags: [Shopping, Grocery, Alcampo, Meal Planning, Pantry, Recipes, Spain]
    category: productivity
    config:
      - key: carrito.bin_path
        description: Path to the carrito CLI executable
        default: "~/.local/bin/carrito"
        prompt: Carrito CLI path
      - key: carrito.config_dir
        description: Optional carrito CLI config directory
        default: "~/.carrito"
        prompt: Carrito config directory
  openclaw:
    homepage: https://github.com/wachtermar/carrito
    os: [darwin, linux]
    envVars:
      - name: CARRITO_CONFIG_DIR
        required: false
        description: Optional carrito CLI config directory.
      - name: CARRITO_MAX_EUR
        required: false
        description: Optional spending guard used by cart and checkout write commands.
      - name: CARRITO_CURL
        required: false
        description: Optional copied cURL session export for non-interactive auth bootstrap.
      - name: CARRITO_USERNAME
        required: false
        description: Optional Alcampo login email for direct login.
      - name: CARRITO_PASSWORD
        required: false
        description: Optional Alcampo password supplied from a secret manager for direct login.
    install:
      - id: carrito-go
        kind: go
        package: github.com/wachtermar/carrito/cmd/carrito
        bins: [carrito]
        label: "Install carrito CLI with Go"
---

# Carrito Shopping

This root `SKILL.md` makes the repository installable by Hermes direct/Git installs and OpenClaw Git installs. The full portable skill lives in `skills/carrito-shopping`.

For the full portable skill, read the nested `skills/carrito-shopping/SKILL.md` and follow it. Resolve `${HERMES_SKILL_DIR}` as the directory containing the nested `SKILL.md`; in OpenClaw, resolve `{baseDir}` as this repository root; otherwise resolve `{repoRoot}` as this repository root. When the nested skill refers to its own resource paths from a root install, use:

- Installer: `{repoRoot}/skills/carrito-shopping/scripts/install-carrito-cli.sh {repoRoot}`
- CLI reference: `{repoRoot}/skills/carrito-shopping/references/cli-reference.md`
- Food playbook: `{repoRoot}/skills/carrito-shopping/references/food-agent-playbook.md`
- Intake scenarios: `{repoRoot}/skills/carrito-shopping/references/intake-scenarios.md`
- Intake verifier: `{repoRoot}/skills/carrito-shopping/scripts/check-intake-response.py`

Use the nested skill instructions as authoritative.
