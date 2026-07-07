# Intake Scenarios

Use this checklist for open-ended meal planning and shopping. The goal is to avoid silently generating the wrong plan for the wrong household.

## Universal Intake

Ask these when absent from both the current user message and saved profile:

- People: household size, guests, adults/children if relevant.
- Scope: number of days and meal types. Infer explicit phrases like "week", "breakfast", "lunch", and "dinner"; ask when unclear.
- Diet pattern: omnivore, vegetarian, vegan, pescatarian, halal, kosher, low-carb, gluten-free, dairy-free, diabetic-friendly, etc.
- Safety: allergies, intolerances, pregnancy restrictions, medical food restrictions, dislikes, and hard avoid-list.
- Budget stance: target grocery budget or explicit "ignore budget".
- Pantry stance: use pantry/fridge/freezer, start from zero, use expiring items first, include staples, or ignore memory.
- Cooking constraints: time per meal, batch-cooking, leftovers, skill level, equipment, no-oven/no-microwave, kid-friendly, variety.
- Nutrition goals: calories, protein, weight loss/gain, high fiber, low sodium, macro target, or no target.
- Locale/market: current Alcampo market/address when prices or availability matter.

Ask all missing high-impact questions together, then stop. Do not run `food plan`, `food shop`, or `food run` until answers or saved profile cover the missing fields.

## Scenario Matrix

| User request | Ask before acting | Do not do yet |
| --- | --- | --- |
| "Make a weekly meal plan" | people, meals/days, diet/allergies/dislikes, budget stance, pantry stance, cooking style, nutrition goals | Do not generate a plan |
| "Breakfast lunch dinner for the week" | people, diet/allergies/dislikes, budget stance, pantry stance, cooking style, nutrition goals | Do not default to 1 or 2 people |
| "Shop for the week" | all meal-plan intake plus budget, `balanced/cheapest/quality`, reviewed basket vs cart mutation | Do not select products or write cart |
| "Prepare an Alcampo basket" | meal scope, people, diet/safety, budget, selection policy, pantry stance, explicit review vs cart add | Do not mutate cart |
| "Use what is in my fridge" | read pantry, ask whether to use expiring items first, verify `pantry_usage` after planning | Do not claim use-up without proof |
| "Vegetarian/vegan plan" | allergies, dislikes, budget, pantry, cooking style, nutrition goals | Do not include meat/fish; vegan excludes eggs/dairy/honey |
| "Cheap plan" | people, diet/safety, budget target, pantry, meal scope | Do not confuse cheap recipe planning with cart max-spend approval |
| "Healthy/high protein" | nutrition target precision, dietary exclusions, people, budget, pantry | Do not invent medical advice |
| "Add this recipe" | source recipe fields, servings, ingredients, steps, tags, allergens, image/nutrition when available | Do not reduce rich recipes to unstructured ingredient text if JSON-LD is available |
| "I bought/delivered this shop" | confirm the shop/run JSON path or receipt/order evidence | Do not update pantry from an unpurchased plan |
| "Add to cart" | explicit approval and max spend guard | Never submit payment or order |

## Good Intake Response Shape

Use a compact numbered list:

```text
I checked your saved Alcampo profile/pantry/staples and the missing setup details are:

1. How many people?
2. Any diet, allergies, dislikes, or foods to avoid?
3. Budget: target weekly grocery budget, or ignore budget?
4. Pantry/fridge/freezer: use what you have, use expiring items first, or start from zero?
5. Cooking preference: quick/easy, batch-cook, high variety, healthy/high-protein, kid-friendly, etc.?
6. Any nutrition goals?

Once you answer, I can generate the plan. I will not prepare a basket or touch the cart unless you ask and give a max spend.
```

For shopping, add:

```text
7. Product policy: cheapest, balanced, or quality?
8. Basket action: reviewed basket file only, or add to cart after review?
9. If adding to cart: maximum spend guard?
```

## Bad Responses

These are failures:

- Generating a weekly plan with no people count.
- Defaulting to `people: 1` or `people: 2` when the user did not provide it and no profile is saved.
- Creating a breakfast/lunch/dinner plan where breakfast slots use lunch/dinner-only recipes.
- Putting meat/fish in a vegetarian plan, or eggs/dairy/honey in a vegan plan.
- Selecting products without a budget stance and selection policy.
- Creating or mutating a cart without explicit approval and max spend.
- Claiming pantry use without checking `pantry_usage`.
