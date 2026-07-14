const fixture = {
  items: [
    { sku: "MOCK-PASTA-500", name: "Pasta 500 g", target: 1, price: 1.20 },
    { sku: "MOCK-SAUCE-400", name: "Tomato sauce 400 g", target: 1, price: 1.70 },
  ],
};

const cart = new Map();
const steps = [...document.querySelectorAll(".pipeline-step")];
const log = document.querySelector("#log");
const outcome = document.querySelector("#outcome");
const capInput = document.querySelector("#cap");
const runButton = document.querySelector("#run-demo");
const resetButton = document.querySelector("#reset-demo");
const cartState = document.querySelector("#cart-state");
const cartTotal = document.querySelector("#cart-total");
let runVersion = 0;

const money = (value) => `€${value.toFixed(2)}`;
const wait = (ms) => new Promise((resolve) => window.setTimeout(resolve, ms));

function totalFor(state) {
  return fixture.items.reduce((total, item) => total + (state.get(item.sku) || 0) * item.price, 0);
}

function projectedState() {
  const projected = new Map(cart);
  fixture.items.forEach((item) => {
    projected.set(item.sku, Math.max(projected.get(item.sku) || 0, item.target));
  });
  return projected;
}

function renderCart() {
  const lineCount = [...cart.values()].filter((quantity) => quantity > 0).length;
  cartState.textContent = lineCount ? `${lineCount} in-memory lines` : "Fixture ready";
  cartTotal.textContent = money(totalFor(cart));
}

function resetRunDisplay() {
  steps.forEach((step) => { step.className = "pipeline-step"; });
  log.innerHTML = '<li class="terminal-empty">Run the fixture to inspect every guard decision.</li>';
  outcome.className = "outcome";
  outcome.textContent = "Nothing has been written. The demo is ready.";
  renderCart();
}

function reset() {
  runVersion += 1;
  cart.clear();
  runButton.disabled = false;
  resetRunDisplay();
}

function setStep(index, state) {
  steps[index].className = `pipeline-step ${state}`;
}

function appendLog(label, message, error = false) {
  const empty = log.querySelector(".terminal-empty");
  if (empty) empty.remove();
  const line = document.createElement("li");
  line.className = `log-line${error ? " error" : ""}`;
  const tag = document.createElement("span");
  const text = document.createElement("span");
  tag.textContent = label;
  text.textContent = message;
  line.append(tag, text);
  log.append(line);
}

async function pause(version) {
  await wait(180);
  return version === runVersion;
}

async function run() {
  const version = ++runVersion;
  runButton.disabled = true;
  resetRunDisplay();
  const cap = Number.parseFloat(capInput.value);

  setStep(0, "active");
  appendLog("PLAN", "Loaded sanitized two-dinner fixture.");
  if (!(await pause(version))) return;
  setStep(0, "done");

  setStep(1, "active");
  appendLog("VALIDATE", "Schema valid; two shopping lines resolved.");
  if (!(await pause(version))) return;
  setStep(1, "done");

  setStep(2, "active");
  appendLog("READ", `Independent cart read: ${money(totalFor(cart))}.`);
  if (!(await pause(version))) return;
  setStep(2, "done");

  setStep(3, "active");
  if (!Number.isFinite(cap) || cap <= 0) {
    appendLog("GUARD", "Refused: enter a positive whole-cart cap.", true);
    setStep(3, "blocked");
    outcome.className = "outcome blocked";
    outcome.textContent = "Blocked before write · invalid cap · no mutation attempted.";
    renderCart();
    runButton.disabled = false;
    return;
  }

  const projected = projectedState();
  const projectedTotal = totalFor(projected);
  appendLog("GUARD", `Projected whole-cart total ${money(projectedTotal)} ≤ ${money(cap)} cap.`);
  if (!(await pause(version))) return;

  if (projectedTotal > cap) {
    appendLog("GUARD", "Refused before mutation: projected total exceeds the explicit cap.", true);
    setStep(3, "blocked");
    outcome.className = "outcome blocked";
    outcome.textContent = `Blocked before write · ${money(projectedTotal)} exceeds ${money(cap)} · no mutation attempted.`;
    renderCart();
    runButton.disabled = false;
    return;
  }
  setStep(3, "done");

  setStep(4, "active");
  const deltas = [];
  fixture.items.forEach((item) => {
    const before = cart.get(item.sku) || 0;
    const after = Math.max(before, item.target);
    cart.set(item.sku, after);
    if (after !== before) deltas.push({ sku: item.sku, before, after });
  });
  appendLog("WRITE", deltas.length
    ? `Mutated in-memory cart fixture: ${deltas.length} quantity deltas.`
    : "No mutation needed: minimum targets already persisted.");
  if (!(await pause(version))) return;
  setStep(4, "done");

  setStep(5, "active");
  const readBack = new Map(cart);
  const readBackTotal = totalFor(readBack);
  const quantitiesMatch = fixture.items.every((item) => readBack.get(item.sku) === item.target);
  const totalMatches = Math.abs(readBackTotal - projectedTotal) < Number.EPSILON;
  const verified = quantitiesMatch && totalMatches && readBackTotal <= cap;
  appendLog("READBACK", `Independent snapshot: quantities=${quantitiesMatch}; total=${totalMatches}; verified=${verified}.`, !verified);
  if (!(await pause(version))) return;

  if (!verified) {
    setStep(5, "blocked");
    outcome.className = "outcome blocked";
    outcome.textContent = "Verification failed · automation stops for manual review.";
    renderCart();
    runButton.disabled = false;
    return;
  }

  setStep(5, "done");
  renderCart();
  cartState.textContent = `${fixture.items.length} lines verified`;
  outcome.className = "outcome";
  outcome.textContent = `Verified: true · ${money(readBackTotal)} ≤ ${money(cap)} · automation stops before checkout.`;
  runButton.disabled = false;
}

runButton.addEventListener("click", run);
resetButton.addEventListener("click", reset);
reset();
