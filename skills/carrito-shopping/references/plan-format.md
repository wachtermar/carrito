# Meal-plan JSON format

Use one JSON object as the source for recipes, shopping, basket creation, and HTML. The validator rejects unknown fields so mistakes are visible.

## Required shape

```json
{
  "title": "Three easy family dinners",
  "household": {
    "people": 4,
    "description": "2 adults and 2 children",
    "dietary_rules": [],
    "allergies": [],
    "dislikes": []
  },
  "assumptions": ["olive oil, salt, and pepper are already at home"],
  "days": [
    {
      "label": "Monday",
      "meals": [
        {
          "type": "dinner",
          "name": "Tomato and spinach pasta",
          "servings": 4,
          "time_minutes": 25,
          "ingredients": [
            {"name": "dry pasta", "amount": "400 g", "source": "Dry pasta"},
            {"name": "chopped tomatoes", "amount": "800 g", "source": "Chopped tomatoes"},
            {"name": "baby spinach", "amount": "200 g", "source": "Baby spinach"},
            {"name": "olive oil", "amount": "1 tbsp", "source": "pantry"}
          ],
          "steps": [
            "Boil the pasta in salted water until al dente.",
            "Simmer the tomatoes for 10 minutes, then wilt in the spinach.",
            "Drain the pasta, combine with the sauce, and serve."
          ],
          "notes": ["For small children, chop the spinach before adding it."]
        }
      ]
    }
  ],
  "shopping": [
    {
      "name": "Dry pasta",
      "needed": "400 g",
      "query": "pasta macarrones",
      "product_sku": "",
      "packages": 0,
      "reason": "Choose one 500 g pack with a clear allergen label."
    },
    {
      "name": "Chopped tomatoes",
      "needed": "800 g",
      "query": "tomate triturado",
      "product_sku": "",
      "packages": 0,
      "reason": "Prefer two 400 g packs with no added allergens."
    },
    {
      "name": "Baby spinach",
      "needed": "200 g",
      "query": "espinaca baby",
      "product_sku": "",
      "packages": 0,
      "reason": "Choose the closest available pack size."
    }
  ]
}
```

## Rules

- Always include `dietary_rules`, `allergies`, `dislikes`, and `assumptions`; use `[]` when there are none.
- `household.people`, meal `servings`, and `time_minutes` must be positive integers.
- Every day needs a label and at least one meal. Every meal needs a type, name, ingredients, and complete ordered steps.
- Every ingredient needs `source`. Use `"pantry"` only when `assumptions` explicitly includes that ingredient name; otherwise use the exact `name` of one shopping item. Matching ignores case and repeated whitespace.
- Every shopping item must be referenced by at least one ingredient. Consolidate repeated needs under one unique shopping name. `shopping` may be `[]` only when every ingredient comes from the declared pantry assumptions.
- `amount` and `needed` are cooking quantities written for humans, such as `500 g`, `2 onions`, or `1.5 L`.
- `query` is concise Spanish store-search text. Do not put brand preferences into it unless the user asked for a brand.
- Leave `product_sku` empty and `packages` at `0` for the first validation/candidate pass.
- Candidate output includes `status`, `unresolved_count`, and an `items` array. Each item repeats the shopping line and provides compact, currently buildable products. A nonzero exit can still include useful `status: "incomplete"` JSON to repair.
- After candidate search, set `product_sku` to the exact candidate `sku`, not its internal `id`; SKU detail reads work without cart authentication. Set `packages` to a positive numeric package count no larger than 10,000 and with at most three decimal places; it may be fractional only for a variable-weight product.
- `reason` should briefly capture package fit, diet fit, or value. It is shown in the family HTML.
- The basket uses selected Alcampo product IDs and package counts. It does not infer cooking-unit conversions.

## Hard constraints

- Preserve the user's restriction wording under `dietary_rules` and `allergies`.
- Check every recipe ingredient against those restrictions before product search.
- The CLI text-screens obvious allergen conflicts and recognized gluten-free, vegan, vegetarian, pescatarian, dairy-free, and lactose-free wording. It does not certify safety or automatically verify other rules.
- For processed foods, inspect the chosen product detail. Missing ingredient/allergen data is unresolved, not safe; prefer a simpler or better-labelled alternative and surface any remaining package-check warning.
- Never represent inferred nutrition values as medical or exact dietary advice.
