---
name: alcampo-shopping
description: "Use when a user wants Alcampo Spain grocery search/cart review, pantry-aware meal plans, diet-aware shopping lists, recipe PDFs, basket pricing, or guarded checkout-slot prep through alcampo-cli."
version: 1.0.0
author: Marcel Wachter
license: MIT
platforms: [linux, macos]
metadata:
  hermes:
    tags: [Shopping, Grocery, Alcampo, Meal Planning, Pantry, Recipes, Spain]
    category: productivity
    config:
      - key: alcampo.bin_path
        description: Path to the alcampo CLI executable
        default: "~/.local/bin/alcampo"
        prompt: Alcampo CLI path
      - key: alcampo.config_dir
        description: Optional alcampo CLI config directory
        default: "~/.alcampo"
        prompt: Alcampo config directory
  openclaw:
    install:
      - id: brew-go
        kind: brew
        formula: go
        bins: [go]
        label: "Install Go with Homebrew"
      - id: apt-go
        kind: apt
        package: golang-go
        bins: [go]
        label: "Install Go with apt"
---

# Alcampo Food Shopping

This root `SKILL.md` makes the repository installable by Hermes direct/Git installs and OpenClaw Git installs. The full portable skill lives in `skills/alcampo-shopping`.

For the full portable skill, read the nested `skills/alcampo-shopping/SKILL.md` and follow it. Resolve `${HERMES_SKILL_DIR}` as the directory containing the nested `SKILL.md`; when working from this root file, resolve `{repoRoot}` as this repository root. When the nested skill refers to its own resource paths from a root install, use:

- Installer: `{repoRoot}/skills/alcampo-shopping/scripts/install-alcampo-cli.sh {repoRoot}/alcampo-cli`
- CLI reference: `{repoRoot}/skills/alcampo-shopping/references/cli-reference.md`
- Food playbook: `{repoRoot}/skills/alcampo-shopping/references/food-agent-playbook.md`
- Intake scenarios: `{repoRoot}/skills/alcampo-shopping/references/intake-scenarios.md`
- Intake verifier: `{repoRoot}/skills/alcampo-shopping/scripts/check-intake-response.py`

Use the nested skill instructions as authoritative.
