# Changelog

All notable changes will be documented here.

This project follows semantic versioning once public tags are created.

## Unreleased

- Added recipe intake and quality sidecars for food runs, including structured file/directory/schema.org URL import, strict cookability gates, image provenance/cache evidence, PDF markers, manifest/audit fingerprints, and Hermes recipe/PDF trust fields.
- Added evidence-led food-run nutrition ledgers with consumed-quantity totals, pantry nutrition gaps, PDF nutrition sections, and separate nutrition readiness reporting.
- Made the Go module installable from `github.com/wachtermar/carrito`.
- Added release-ready documentation for CLI, Hermes, and OpenClaw installs.
- Added OpenClaw-compatible `metadata.openclaw.install` Go installer metadata.
- Added repository-level license, security policy, contribution guide, CI workflow, and release workflow.
- Removed unsupported `checkout submit` from help output while keeping unsupported commands fail-closed.

## 1.0.0

- Initial public release candidate for the Alcampo CLI and `carrito-shopping` Agent Skill.
