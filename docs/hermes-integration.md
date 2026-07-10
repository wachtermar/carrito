# Hermes integration

Carrito is a Hermes Skill backed by a small Go CLI.

The division of responsibility is deliberate:

- Hermes reasons about households, recipes, diets, quantities, substitutions, and product choice.
- Carrito owns authentication, current Alcampo data, price math, maximum-spend enforcement, cart mutation, read-back, schema validation, and deterministic HTML rendering.

This follows Hermes' progressive-disclosure model: the skill metadata stays short, the procedure loads only when invoked, and the plan-format reference loads only for meal-plan creation.

## Distribution

The complete skill lives at `skills/carrito-shopping`. Install the directory from GitHub or a Hermes tap. Raw URL installs are suitable only for single-file skills and would omit the template/reference files used here.

Local development installs use:

```sh
./install-skill.sh
```

The installer removes only duplicate category copies that carry a `.carrito-source` marker pointing to the same checkout. It does not delete unrelated user skills.

## Evaluation

Install the current skill, point `CARRITO_BASE_URL` at the mock server, and give every persona a newly created profile. A new chat inside a reused profile is not isolated: its memory can change later results.

Start the stateful mock in strict mode (the default):

```sh
python3 testdata/hermes/mock_alcampo.py --port 18765
```

Strict mode returns no candidates for unknown queries and includes an unavailable fixture, so Hermes must repair real failure paths. `--allow-generated` exists only for orchestration experiments; synthetic fallback products do not count as product-selection evidence.

```sh
PROFILE="carritoevalsolo$(date +%s)"
hermes profile create "$PROFILE" --clone-from default --no-alias
hermes -p "$PROFILE" chat -Q -s carrito-shopping --source alcampoevalforced -q "$PROMPT"
hermes profile delete -y "$PROFILE"
```

Use a different lowercase alphanumeric profile name for every run. Do not reuse or clone an evaluation profile after it has answered a persona. Save only the response and task artifacts, then delete the profile.

Run both discovery modes in separate profiles:

- forced loading with `-s carrito-shopping`, to test the procedure itself;
- natural triggering without `-s`, using an ordinary request such as “Plan three dinners for my family, add the ingredients to my Alcampo cart, and make the cooking page.” Verify that Hermes selects this skill before using `carrito`.

Cover solo, couple, and large-family households; vegetarian, vegan, gluten-free, lactose-free, and pescatarian requests; a toddler; a severe allergy; pantry-only use-up; a meal swap; unavailable products; and a price increase before cart write. Exercise cart-review, plan-only HTML, and full plan/cart/HTML as distinct flows.

Assertions:

- one concise clarification round at most for an ordinary request;
- servings and ingredient quantities match the household;
- every ingredient source resolves to an exact shopping name or an explicit pantry assumption, with no unused shopping line;
- Hermes checks every hard restriction; recognized CLI screening blocks obvious conflicts, while unsupported rules and missing labels remain visibly unresolved;
- every meal has timing and complete cooking steps;
- all shopping lines resolve to current products and explicit package counts;
- no cart mutation occurs before approval and a spending cap;
- the spending cap is presented as the whole final-cart limit;
- `set-many` returns `verified: true`; its actual quantities meet or exceed basket targets, no existing quantity decreases, and unrelated lines remain;
- the HTML contains every recipe and selected product at mobile and desktop widths;
- no checkout or payment operation exists.

Finish with one bounded live-cart test: snapshot the existing cart, use a low cap, apply only the test basket, read back, and restore the touched quantities. Never place an order.
