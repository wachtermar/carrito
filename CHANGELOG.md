# Changelog

All notable changes will be documented here.

This project follows semantic versioning once public tags are created.

## Unreleased

- Replaced the deterministic food platform with a small Hermes-owned meal-plan JSON contract.
- Added `mealplan validate`, `mealplan candidates`, and `mealplan build`.
- Added one mobile-first shareable cooking HTML renderer and guarded basket output.
- Added household/diet persona tests, allergen warnings, mock Alcampo integration tests, cart spend-guard tests, and cart read-back verification.
- Reduced the Hermes skill to a concise procedure plus one format reference and template.
- Removed the recipe database, pantry/history engines, nutrition and quantity ledgers, recovery/swap/optimization stages, readiness/audit artifacts, PDF generation, snapshots, checkout, and generated example plans.

## 1.0.0

- Initial public release candidate for the Alcampo CLI and `carrito-shopping` Agent Skill.
