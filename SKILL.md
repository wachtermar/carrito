---
name: alcampo-shopping
description: "Use alcampo-cli for Alcampo Spain meal planning, recipes, pantry-aware shopping, product search, basket totals, cart checks, and guarded checkout slot workflows."
license: MIT
metadata:
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

This root `SKILL.md` makes the repository installable by OpenClaw Git installs, which expect a `SKILL.md` at the source root.

For the full portable skill, read `{baseDir}/skills/alcampo-shopping/SKILL.md` and follow it. Resolve `{baseDir}` in this file as the repository root. When the nested skill refers to its own resource paths from a root install, use:

- Installer: `{baseDir}/skills/alcampo-shopping/scripts/install-alcampo-cli.sh {baseDir}/alcampo-cli`
- CLI reference: `{baseDir}/skills/alcampo-shopping/references/cli-reference.md`

Use the nested skill instructions as authoritative.
