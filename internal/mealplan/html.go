package mealplan

import (
	"html/template"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type pageData struct {
	Build       Build
	GeneratedAt string
	MealCount   int
	DayCount    int
}

func WriteHTML(path string, build Build, generatedAt time.Time) error {
	if generatedAt.IsZero() {
		generatedAt = time.Now()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data := pageData{
		Build:       build,
		GeneratedAt: generatedAt.Format("2 Jan 2006, 15:04"),
		DayCount:    len(build.Plan.Days),
	}
	for _, day := range build.Plan.Days {
		data.MealCount += len(day.Meals)
	}
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := f.Name()
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(tempPath)
		}
	}()
	if err := f.Chmod(0o644); err != nil {
		return err
	}
	if err := cookingPage.Execute(f, data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	ok = true
	return nil
}

var cookingPage = template.Must(template.New("cooking-page").Funcs(template.FuncMap{
	"join": strings.Join,
	"count": func(value int, singular, plural string) string {
		label := plural
		if value == 1 {
			label = singular
		}
		return strconv.Itoa(value) + " " + label
	},
	"sentences": func(values []string) string {
		cleaned := make([]string, 0, len(values))
		for _, value := range values {
			value = strings.TrimSpace(value)
			value = strings.TrimRight(value, " \t\r\n.!?")
			if value != "" {
				cleaned = append(cleaned, value)
			}
		}
		if len(cleaned) == 0 {
			return ""
		}
		return strings.Join(cleaned, "; ") + "."
	},
	"purchase": func(item SelectedItem) string {
		quantity := strconv.FormatFloat(item.Shopping.Packages, 'f', -1, 64)
		if isVariableWeightProduct(item.Product) {
			unit := strings.TrimSpace(item.Product.Unit)
			if unit == "" || strings.EqualFold(unit, "unit") {
				return "Buy " + quantity + " (variable weight)"
			}
			return "Buy " + quantity + " " + unit + " (variable weight)"
		}
		if size := strings.TrimSpace(item.Product.Size); size != "" {
			return "Buy " + quantity + " × " + size
		}
		label := "packages"
		if item.Shopping.Packages == 1 {
			label = "package"
		}
		return "Buy " + quantity + " " + label
	},
	"money": func(cents int64) string {
		return "€" + strconv.FormatFloat(float64(cents)/100, 'f', 2, 64)
	},
	"unitPrice": func(item SelectedItem) string {
		amount := "€" + strconv.FormatFloat(float64(item.Product.Price.Cents)/100, 'f', 2, 64)
		if isVariableWeightProduct(item.Product) {
			if unit := strings.TrimSpace(item.Product.Unit); unit != "" && !strings.EqualFold(unit, "unit") {
				return amount + "/" + unit
			}
			return amount + " per variable-weight unit"
		}
		return amount + " each"
	},
	"add": func(value int) int { return value + 1 },
}).Parse(cookingPageHTML))

const cookingPageHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <meta name="color-scheme" content="light">
  <title>{{.Build.Plan.Title}}</title>
  <style>
    :root {
      --ink: #1f2924;
      --muted: #68736d;
      --cream: #f7f2e8;
      --paper: #fffdf8;
      --green: #1d6046;
      --green-soft: #dfece4;
      --orange: #df6b3a;
      --orange-soft: #f8e4d7;
      --line: #ded9cf;
      --shadow: 0 18px 45px rgba(31, 41, 36, .10);
      font-family: Inter, ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    }
    * { box-sizing: border-box; }
    body { min-width: 0; max-width: 100%; margin: 0; overflow-x: hidden; color: var(--ink); background: var(--cream); line-height: 1.55; }
    a { color: var(--green); }
    button { font: inherit; }
    .shell { width: min(1080px, calc(100% - 28px)); max-width: 100%; margin: 0 auto; }
    .hero { padding: 48px 0 28px; }
    .eyebrow { margin: 0 0 8px; color: var(--orange); font-size: .78rem; font-weight: 800; letter-spacing: .14em; text-transform: uppercase; }
    h1 { max-width: 760px; margin: 0; overflow-wrap: anywhere; font-family: Georgia, "Times New Roman", serif; font-size: clamp(2.35rem, 8vw, 5.6rem); line-height: .96; letter-spacing: -.045em; }
    .hero-grid { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 28px; align-items: end; }
    .hero-grid > *, .section-title > *, .shop-item > * { min-width: 0; }
    .summary { display: flex; flex-wrap: wrap; max-width: 100%; gap: 8px; margin: 24px 0 0; }
    .pill { display: inline-flex; align-items: center; gap: 6px; min-width: 0; max-width: 100%; min-height: 34px; padding: 7px 12px; overflow-wrap: anywhere; word-break: break-word; white-space: normal; border: 1px solid var(--line); border-radius: 999px; background: rgba(255,255,255,.55); font-size: .88rem; }
    .hero-actions { display: flex; gap: 8px; }
    .action { border: 0; border-radius: 999px; padding: 11px 16px; color: white; background: var(--green); font-weight: 750; cursor: pointer; }
    .section { margin: 20px 0 34px; }
    .section-title { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 20px; align-items: end; margin: 0 0 14px; }
    .section-title h2 { margin: 0; font-family: Georgia, "Times New Roman", serif; font-size: clamp(1.65rem, 4vw, 2.35rem); }
    .section-title p { margin: 0; color: var(--muted); }
    .notice { display: grid; gap: 10px; padding: 18px; overflow-wrap: anywhere; border: 1px solid #e5b69d; border-radius: 18px; background: var(--orange-soft); }
    .notice strong { color: #7e3014; }
    .assumptions { margin: 8px 0 0; padding-left: 20px; }
    .day-tabs { display: flex; gap: 8px; overflow-x: auto; padding: 0 0 12px; scrollbar-width: thin; }
    .day-tab { flex: 0 0 auto; border: 1px solid var(--line); border-radius: 999px; padding: 9px 14px; color: var(--ink); background: var(--paper); font-weight: 750; cursor: pointer; }
    .day-tab[aria-selected="true"] { color: white; border-color: var(--green); background: var(--green); }
    .day { display: none; }
    .day.active { display: block; }
    .meal-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 18px; }
    .meal { overflow: hidden; border: 1px solid var(--line); border-radius: 24px; background: var(--paper); box-shadow: var(--shadow); }
    .meal-head { padding: 21px 22px 17px; border-bottom: 1px solid var(--line); }
    .meal-type { color: var(--orange); font-size: .77rem; font-weight: 850; letter-spacing: .12em; text-transform: uppercase; }
    .meal h3 { margin: 5px 0 7px; overflow-wrap: anywhere; font-family: Georgia, "Times New Roman", serif; font-size: 1.55rem; line-height: 1.1; }
    .meal-meta { display: flex; flex-wrap: wrap; gap: 14px; color: var(--muted); font-size: .88rem; }
    .meal-body { padding: 20px 22px 23px; }
    .meal h4 { margin: 0 0 9px; font-size: .77rem; letter-spacing: .1em; text-transform: uppercase; }
    .ingredients { display: grid; gap: 7px; margin: 0 0 21px; padding: 0; list-style: none; }
    .ingredients li { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 14px; padding-bottom: 6px; border-bottom: 1px dotted var(--line); }
    .amount { color: var(--muted); white-space: nowrap; }
    .steps { display: grid; gap: 13px; margin: 0; padding: 0; list-style: none; counter-reset: step; }
    .steps li { display: grid; grid-template-columns: 28px 1fr; gap: 10px; }
    .steps li::before { counter-increment: step; content: counter(step); display: grid; place-items: center; width: 26px; height: 26px; border-radius: 50%; color: white; background: var(--green); font-size: .78rem; font-weight: 800; }
    .meal-notes { margin: 19px 0 0; padding: 12px 14px; border-radius: 13px; background: var(--green-soft); }
    .meal-notes ul { margin: 5px 0 0; padding-left: 18px; }
    .shopping-layout { display: grid; grid-template-columns: minmax(0, 1fr) 280px; gap: 18px; align-items: start; }
    .shopping-list { display: grid; gap: 10px; }
    .shop-item { display: grid; grid-template-columns: auto 58px minmax(0, 1fr) auto; gap: 13px; align-items: center; padding: 13px; border: 1px solid var(--line); border-radius: 16px; background: var(--paper); }
    .shop-item.done { opacity: .52; }
    .check { appearance: none; width: 22px; height: 22px; margin: 0; border: 2px solid #9aa49e; border-radius: 7px; cursor: pointer; }
    .check:checked { border-color: var(--green); background: var(--green); box-shadow: inset 0 0 0 4px white; }
    .product-image { width: 58px; height: 58px; border-radius: 12px; background: #f1eee7; object-fit: contain; }
    .product-image.empty { display: grid; place-items: center; color: var(--muted); font-size: .7rem; text-align: center; }
    .shop-name { overflow-wrap: anywhere; font-weight: 780; }
    .shop-need { color: var(--muted); font-size: .86rem; }
    .shop-product { margin-top: 3px; overflow-wrap: anywhere; font-size: .88rem; }
    .shop-reason { margin-top: 3px; overflow-wrap: anywhere; color: var(--muted); font-size: .82rem; }
    .shop-price { min-width: 82px; text-align: right; }
    .shop-price strong { display: block; }
    .shop-price small { color: var(--muted); }
    .totals { position: sticky; top: 14px; padding: 20px; border-radius: 20px; color: white; background: var(--green); box-shadow: var(--shadow); }
    .totals .amount-total { margin: 6px 0 12px; font-family: Georgia, "Times New Roman", serif; font-size: 2.45rem; }
    .totals p { margin: 0; color: rgba(255,255,255,.76); font-size: .86rem; }
    .warnings { margin: 13px 0 0; padding: 14px 15px; border-radius: 14px; color: #683013; background: var(--orange-soft); font-size: .86rem; }
    .warnings ul { margin: 5px 0 0; padding-left: 18px; }
    footer { padding: 22px 0 44px; color: var(--muted); font-size: .8rem; }
    @media (max-width: 760px) {
      .hero { padding-top: 32px; }
      .hero-grid, .shopping-layout { grid-template-columns: 1fr; }
      .hero-actions { display: none; }
      .meal-grid { grid-template-columns: 1fr; }
      .totals { position: static; order: -1; }
      .shop-item { grid-template-columns: auto 48px minmax(0, 1fr); }
      .product-image { width: 48px; height: 48px; }
      .shop-price { grid-column: 3; text-align: left; }
      h1 { font-size: clamp(2.7rem, 15vw, 4.6rem); }
    }
    @media (max-width: 420px) {
      .shell { width: calc(100% - 20px); }
      h1 { max-width: 100%; font-size: clamp(2.25rem, 13vw, 3.4rem); line-height: 1; }
      .ingredients li { grid-template-columns: minmax(0, 1fr); gap: 2px; }
      .amount { white-space: normal; text-align: left; overflow-wrap: anywhere; }
      .shop-item { grid-template-columns: auto 42px minmax(0, 1fr); gap: 9px; padding: 11px; }
      .product-image { width: 42px; height: 42px; }
    }
    @media print {
      body { background: white; }
      .shell { width: 100%; }
      .hero-actions, .day-tabs { display: none; }
      .day { display: block !important; break-inside: avoid; }
      .meal { box-shadow: none; break-inside: avoid; }
      .shopping-layout { grid-template-columns: 1fr; }
      .totals { position: static; }
    }
  </style>
</head>
<body>
  <header class="hero shell">
    <div class="hero-grid">
      <div>
        <p class="eyebrow">Family cooking plan</p>
        <h1>{{.Build.Plan.Title}}</h1>
        <div class="summary">
          <span class="pill">{{count .Build.Plan.Household.People "person" "people"}}</span>
          <span class="pill">{{count .DayCount "day" "days"}}</span>
          <span class="pill">{{count .MealCount "meal" "meals"}}</span>
          {{if .Build.Plan.Household.DietaryRules}}<span class="pill">{{join .Build.Plan.Household.DietaryRules ", "}}</span>{{end}}
        </div>
      </div>
      <div class="hero-actions"><button class="action" onclick="window.print()">Print / save PDF</button></div>
    </div>
  </header>

  <main class="shell">
    {{if or .Build.Plan.Household.Description .Build.Plan.Household.Allergies .Build.Plan.Household.Dislikes .Build.Plan.Assumptions}}
    <section class="section notice" aria-label="Family notes">
      {{if .Build.Plan.Household.Description}}<div><strong>Cooking for:</strong> {{.Build.Plan.Household.Description}}</div>{{end}}
      {{if .Build.Plan.Household.Allergies}}<div><strong>Allergies:</strong> {{sentences .Build.Plan.Household.Allergies}} Check every physical package before serving.</div>{{end}}
      {{if .Build.Plan.Household.Dislikes}}<div><strong>Avoid:</strong> {{sentences .Build.Plan.Household.Dislikes}}</div>{{end}}
      {{if .Build.Plan.Assumptions}}<div><strong>Already at home / assumed:</strong><ul class="assumptions">{{range .Build.Plan.Assumptions}}<li>{{.}}</li>{{end}}</ul></div>{{end}}
    </section>
    {{end}}

    <section class="section">
      <div class="section-title"><div><p class="eyebrow">What to cook</p><h2>Day by day</h2></div><p>Tap a day to focus.</p></div>
      <div class="day-tabs" role="tablist">
        {{range $i, $day := .Build.Plan.Days}}<button class="day-tab" role="tab" data-day="day-{{$i}}" aria-selected="{{if eq $i 0}}true{{else}}false{{end}}">{{$day.Label}}</button>{{end}}
      </div>
      {{range $i, $day := .Build.Plan.Days}}
      <div class="day {{if eq $i 0}}active{{end}}" id="day-{{$i}}" role="tabpanel">
        <div class="meal-grid">
          {{range $day.Meals}}
          <article class="meal">
            <div class="meal-head">
              <div class="meal-type">{{.Type}}</div>
              <h3>{{.Name}}</h3>
              <div class="meal-meta"><span>{{count .Servings "serving" "servings"}}</span><span>{{.TimeMinutes}} min</span></div>
            </div>
            <div class="meal-body">
              <h4>Ingredients</h4>
              <ul class="ingredients">{{range .Ingredients}}<li><span>{{.Name}}</span><span class="amount">{{.Amount}}</span></li>{{end}}</ul>
              <h4>Method</h4>
              <ol class="steps">{{range .Steps}}<li><span>{{.}}</span></li>{{end}}</ol>
              {{if .Notes}}<div class="meal-notes"><strong>Useful notes</strong><ul>{{range .Notes}}<li>{{.}}</li>{{end}}</ul></div>{{end}}
            </div>
          </article>
          {{end}}
        </div>
      </div>
      {{end}}
    </section>

    <section class="section">
      <div class="section-title"><div><p class="eyebrow">What was selected</p><h2>Alcampo shopping</h2></div><p>Tick items while unpacking.</p></div>
      <div class="shopping-layout">
        <div>
          <div class="shopping-list">
            {{range $i, $item := .Build.Items}}
            <article class="shop-item" data-shop-item>
              <input class="check" type="checkbox" aria-label="Mark {{$item.Shopping.Name}} complete">
              {{if $item.Product.Images}}<img class="product-image" src="{{index $item.Product.Images 0}}" alt="{{$item.Product.Name}}">{{else}}<div class="product-image empty">No photo</div>{{end}}
              <div>
                <div class="shop-name">{{$item.Shopping.Name}} · {{$item.Shopping.Needed}}</div>
                <div class="shop-product">{{if $item.Product.URL}}<a href="{{$item.Product.URL}}" target="_blank" rel="noreferrer">{{$item.Product.Name}}</a>{{else}}{{$item.Product.Name}}{{end}}</div>
                <div class="shop-need">{{purchase $item}}</div>
                {{if $item.Shopping.Reason}}<div class="shop-reason">{{$item.Shopping.Reason}}</div>{{end}}
                {{if $item.SafetyNote}}<div class="shop-reason">{{$item.SafetyNote}}</div>{{end}}
              </div>
              <div class="shop-price"><strong>{{money $item.LineTotal.Cents}}</strong><small>{{unitPrice $item}}</small></div>
            </article>
            {{end}}
          </div>
          {{if .Build.Warnings}}<div class="warnings"><strong>Please check</strong><ul>{{range .Build.Warnings}}<li>{{.}}</li>{{end}}</ul></div>{{end}}
        </div>
        <aside class="totals">
          <div>Estimated products total</div>
          <div class="amount-total">{{money .Build.EstimatedTotal.Cents}}</div>
          <p>Current Alcampo prices can change. The cart may also contain products that were already there. No order has been placed.</p>
        </aside>
      </div>
    </section>
  </main>
  <footer class="shell">Generated {{.GeneratedAt}} · Share this single HTML file with anyone who is cooking.</footer>
  <script>
    document.querySelectorAll('.day-tab').forEach(function(tab) {
      tab.addEventListener('click', function() {
        document.querySelectorAll('.day-tab').forEach(function(item) { item.setAttribute('aria-selected', 'false'); });
        document.querySelectorAll('.day').forEach(function(day) { day.classList.remove('active'); });
        tab.setAttribute('aria-selected', 'true');
        document.getElementById(tab.dataset.day).classList.add('active');
      });
    });
    document.querySelectorAll('[data-shop-item] .check').forEach(function(box) {
      box.addEventListener('change', function() { box.closest('[data-shop-item]').classList.toggle('done', box.checked); });
    });
  </script>
</body>
</html>`
