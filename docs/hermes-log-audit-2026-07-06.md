# Hermes Log Audit: Alcampo Intake Failure

Date: 2026-07-06

## Evidence

The Hermes session database and logs showed session `20260706_200836_04477d` started from this user request:

```text
lets get started making this work. i need a full meal plan for the week breakfast lunch dinner
```

The agent loaded `alcampo-shopping`, read empty food profile/pantry/staples, and generated a plan anyway. Captured artifacts under `.hermes-output/mealplan-week*.json` showed:

- `people: 1`
- 7 days, breakfast/lunch/dinner
- meat/fish recipes in a plan generated without asking dietary constraints:
  - Beef Vegetable Noodles
  - Chicken Fajita Bowls
  - Chicken Noodle Soup
  - Tuna And White Bean Salad
  - Cod With Spanish Pisto

This proved the skill was not enforcing intake before open-ended planning.

## Fixes Applied

- Added a mandatory intake gate to `skills/alcampo-shopping/SKILL.md`.
- Updated `references/food-agent-playbook.md` so read-only weekly plans no longer bypass questions.
- Added `references/intake-scenarios.md` with concrete required questions and failure cases.
- Added `scripts/check-intake-response.py` to regression-check captured Hermes responses.
- Updated the planner to choose recipes tagged for each requested meal type.
- Strengthened vegetarian/vegan filtering.
- Made `food plan` and `food run` fail closed when no people count is provided and no profile people count is saved.
- Added diet-aware product rejection so vegetarian/vegan shopping does not select meat/fish/dairy/egg/honey conflicts, and compatible selections no longer expose rejected products in `alternates_considered`.
- Added budget status notes to shopping JSON, including an over-budget `complete: false` result when the priced basket exceeds the target.
- Fixed nested CLI `--help` handling so Hermes sees help output as success instead of treating it as a tool failure and detouring into source inspection.
- Added Go regression tests for intake-backed planner behavior, meal-type matching, vegetarian/vegan exclusions, diet-safe product selection, budget notes, and nested help exit status.

## Verification

Hermes session `20260706_202137_f36378` with the same open-ended request now asks:

- people
- diet/allergies/dislikes
- budget
- pantry/fridge/freezer stance
- cooking preference
- nutrition goals

It does not generate a plan until answers are provided.

After answers, the resumed session generated a 7-day vegetarian breakfast/lunch/dinner plan for 2 people with:

- 21 meals
- no meat/fish
- no mushrooms
- correct meal-type tags
- no cart mutation

Hermes session `20260706_202405_a79a56` for weekly grocery shopping now asks for people, meals, diet/allergies, budget, selection policy, pantry stance, basket-vs-cart intent, and max spend before product selection or cart mutation.

Hermes session `20260706_204323_99a979` repeated the open-ended meal-plan prompt against the installed skill:

- Loaded both `food-agent-playbook.md` and `intake-scenarios.md`.
- Read empty profile, pantry, and staples.
- Asked people, diet/allergies/dislikes, budget, pantry/fridge/freezer stance, cooking preference, and nutrition goals.
- Stopped without generating a plan until answers were provided.
- The captured response passed `scripts/check-intake-response.py --mode meal-plan`.

The same session was resumed with complete answers:

- 2 people
- vegetarian
- no allergies
- avoid mushrooms
- 80 EUR weekly budget
- zero pantry
- quick/easy healthy variety
- no cart, basket, or checkout

It generated `.hermes-output/intake-test-v3/answered-plan.json`, then validated:

- 7 days
- 21 meals
- `people: 2`
- no meat/fish terms
- no mushroom terms
- no shopping/cart commands

The first resumed run also showed why the CLI help fix was needed: `alcampo food plan --help` returned nonzero before the fix, causing Hermes to inspect source files. After the fix, installed help commands exit `0`.

Direct installed-CLI shopping validation with isolated `ALCAMPO_CONFIG_DIR` generated `.hermes-output/all-details-test-v2/`:

- Vaguada market
- 2 days of vegetarian dinners
- 2 people
- cheapest selection
- 40 EUR budget
- zero pantry
- reviewed basket only

Validation found:

- 10 selected products
- basket total 13.90 EUR
- budget note: within 40.00 EUR
- no meat/fish/mushroom terms in selected products
- no meat/fish/mushroom terms in compatible alternates
- no cart mutation

Fresh Hermes shopping session `20260706_205013_e694b1` could not run after the final reinstall because the provider returned HTTP 429 usage-limit errors for all three retries before any tool call. This is an external test limitation, not a skill/CLI failure; no shopping artifacts or cart mutations were produced.

## Follow-up After Rate Limit Upgrade

On 2026-07-07, Hermes session `20260707_181608_5b9a63` reran the complete shopping prompt:

```text
Plan and shop 2 days of vegetarian dinners for 2 people. No allergies, avoid mushrooms. Budget 40 EUR. Use cheapest product selection. Start from zero pantry. Set Vaguada market if needed. Write a reviewed basket file and shopping JSON under .hermes-output/hermes-shopping-live-20260707. Do not add anything to the cart and do not touch checkout.
```

Hermes loaded `food-agent-playbook.md` and `cli-reference.md`, used an isolated zero-pantry config, set the Vaguada market, set a local profile for 2 people / vegetarian / avoid mushrooms / 40 EUR / cheapest policy, and generated:

- `.hermes-output/hermes-shopping-live-20260707/mealplan.json`
- `.hermes-output/hermes-shopping-live-20260707/shopping.json`
- `.hermes-output/hermes-shopping-live-20260707/run-output.json`
- `.hermes-output/hermes-shopping-live-20260707/reviewed-basket.txt`
- `.hermes-output/hermes-shopping-live-20260707/basket-total-verification.json`

Independent validation showed:

- 2 days, 2 dinners, `people: 2`
- `budget_eur: "40"`
- `selection_policy: "cheapest"`
- `complete: true`
- 10 selected products
- estimated total 14.40 EUR
- independently verified basket total 14.40 EUR
- 10 basket lines in JSON and the reviewed basket file
- no meat/fish/mushroom terms in plan, selected products, shopping JSON, or compatible alternates
- no `cart`, `checkout`, `set-many`, `select-slot`, `confirm-slot`, `receive`, or `cook` command calls in the Hermes tool history

The transcript still showed terminal warnings for subcommand `--help` probes and one combined setup/run command, but direct reproduction with the installed binary returned exit code `0`. To reduce future transcript noise, the skill now tells Hermes to rely on `cli-reference.md` for command shapes and avoid subcommand `--help` probes during normal user tasks.
