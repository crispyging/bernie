// Recipe selection and preview. Adapted from vllm-rdna-dash by CrispyGinger (Apache-2.0).
"use strict";

async function loadSetupRecipes(gen = setupState.generation) {
  const box = setupState.recipeBox;
  if (!box) return;
  setupState.recipeError = "";
  let list;
  try { list = await api("/api/recipes"); } catch (e) {
    if (gen !== setupState.generation) return;
    setupState.recipeError = String(e.message || e);
    put(box, h("p.recipe-warning", "Recipes could not be read."),
      h("details.advanced", h("summary", "Recipe loading error"), h("pre.step-error", setupState.recipeError)),
      h("button", { type: "button", onclick: () => loadSetupRecipes() }, "Try again"));
    if (setupState.doc) paintSetupHead(setupState.doc);
    return;
  }
  if (gen !== setupState.generation) return;
  setupState.recipes = list;
  const recipes = list.recipes || [];
  if (!setupState.recipeID || !recipes.some((r) => r.id === setupState.recipeID)) {
    const best = setupRecommendedRecipe(recipes);
    setupState.recipeID = best ? best.id : (recipes[0] ? recipes[0].id : "");
  }
  const selected = setupRecipe();
  const modes = selected && selected.modes || ["host"];
  if (!modes.includes(setupState.mode)) setupState.mode = modes[0];
  put(setupState.recipeErrors, (list.errors || []).map(setupRecipeError));
  put(box, recipes.length ? setupRecommendedCard(recipes) : emptyState("No recipe is available.",
    "A recipe selects the model and its serving settings. Check the recipe files, then try again.", "bernie recipe list --all"));
  paintSetupRecipeHint();
  await loadSetupPlan();
}


function setupRecipeOrder(recipes) {
  const rank = (r) => {
    const fit = setupFitStatus(r);
    const i = ["fits", "tight", "unknown"].indexOf(fit);
    return i < 0 ? 3 : i;
  };
  return recipes.slice().sort((a, b) => {
    const d = rank(a) - rank(b);
    if (d) return d;
    return (b.priority || 0) - (a.priority || 0);
  });
}

function setupRecipeError(error) {
  return h("div.recipe-error", h("span.recipe-error-source", error.source || "recipe"),
    h("span.recipe-error-text", "This recipe could not load."),
    h("details.advanced", h("summary", "Recipe error"), h("pre.step-error", error.error || error.err || "No error detail was returned.")));
}

function setupRecommendedCard(recipes) {
  const best = setupRecommendedRecipe(recipes) || recipes[0];
  const others = setupRecipeOrder(recipes).filter((r) => r.id !== best.id);
  const list = h("div.recipe-chooser");
  const recommended = setupRecommendedRecipe(recipes);
  const card = h("label.recipe-card", { class: best.id === setupState.recipeID ? "picked" : "" },
    h("input", { type: "radio", name: "setup-recipe", value: best.id, checked: best.id === setupState.recipeID,
      onchange: () => setupEnsureModule("views/setup/chooser.js").then(() => setupSelectRecipe(best.id)) }),
    h("h3", best.name || best.id, recommended ? h("span.pill", "recommended") : null),
    best.description ? h("p.recipe-what", best.description) : null,
    h("p.recipe-fit", { class: "fit-" + setupFitStatus(best) + (setupFitStatus(best) === "fits" ? " advanced-only" : "") }, setupFitLine(best)),
    recommended ? h("p.small.muted", setupRecommendationWhy(best)) : null);
  return h("div", h("p.small.muted", "A recipe defines the model and how to serve it. Review the choice before starting setup."), card,
    others.length ? h("details.advanced", {
      id: "setup-recipes-other",
      ontoggle: (ev) => {
        if (!ev.target.open || list.childElementCount) return;
        setupEnsureModule("views/setup/chooser.js").then(() => setupPaintRecipeList(list, others))
          .catch((e) => put(list, h("p", "Recipes could not load: " + (e.message || e))));
      },
    }, h("summary", "Choose another recipe (" + others.length + ")"),
      h("p.small.muted", "Changing recipe resets per-run overrides. No running server is changed."), list) : null);
}

function setupRecommendedRecipe(recipes) {
  const rec = setupState.doc && setupState.doc.recommendation;
  const list = recipes || (setupState.recipes && setupState.recipes.recipes) || [];
  if (rec && rec.id) {
    const found = list.find((r) => r.id === rec.id);
    if (found) return found;
  }
  return list.find((r) => setupFitStatus(r) === "fits") || null;
}

function setupFitStatus(recipe) {
  return recipe && recipe.fit && recipe.fit.status || "unknown";
}
function setupFitLine(recipe) {
  const status = setupFitStatus(recipe);
  const reasons = recipe && recipe.fit && recipe.fit.reasons || [];
  if (status === "unknown") return "Compatibility has not been confirmed." + (reasons.length ? " " + reasons.join("; ") : "");
  if (status === "no") return "Not compatible: " + (reasons.join("; ") || "this recipe does not fit this host.");
  if (status === "tight") return "Fits with reduced context." + (reasons.length ? " " + reasons.join("; ") : "");
  return "Compatible with this host." + (reasons.length ? " " + reasons.join("; ") : "");
}
async function loadSetupPlan() {
  const box = setupState.planBox;
  if (!box) return;
  if (!setupState.recipeID) {
    setupState.planRequest++;
    setupState.planLoading = false;
    setupState.plan = null;
    put(box);
    if (setupState.doc) paintSetupHead(setupState.doc);
    return;
  }
  const request = ++setupState.planRequest;
  const gen = setupState.generation;
  const recipe = setupRecipe();
  const local = new Map(Array.from(box.querySelectorAll("details.advanced"))
    .filter((d) => d.dataset.local === "1").map((d) => [d.className, d.open]));
  setupState.planLoading = true;
  setupState.planError = "";
  if (setupState.doc) paintSetupHead(setupState.doc);
  put(box, loading());
  const query = new URLSearchParams({ mode: setupState.mode });
  if (setupState.name) query.set("name", setupState.name);
  for (const set of setupState.sets) query.append("set", set);
  let plan;
  try { plan = await api("/api/recipes/" + encodeURIComponent(setupState.recipeID) + "/plan?" + query.toString()); }
  catch (e) {
    if (request !== setupState.planRequest || gen !== setupState.generation) return;
    setupState.planLoading = false;
    setupState.planError = String(e.message || e);
    put(box, h("p.recipe-warning", "The serving plan could not be prepared. Review the error or try another recipe."),
      h("details.advanced", { open: false }, h("summary", "Plan error"), h("pre.step-error", setupState.planError)),
      h("button", { type: "button", onclick: loadSetupPlan }, "Try plan again"));
    if (setupState.doc) paintSetupHead(setupState.doc);
    return;
  }
  if (request !== setupState.planRequest || gen !== setupState.generation) return;
  setupState.plan = plan;
  await setupEnsurePrefs();
  if (request !== setupState.planRequest || gen !== setupState.generation) return;
  setupState.planLoading = false;
  put(box, setupPlanView(plan, recipe, local));
  for (const detail of box.querySelectorAll("details.advanced")) {
    if (local.has(detail.className)) { detail.open = local.get(detail.className); detail.dataset.local = "1"; }
  }
  if (setupState.doc) { paintSetupHead(setupState.doc); paintSetupFacts(setupState.doc); }
}

/** Only config-backed fields can be saved as defaults. */
async function setupEnsurePrefs() {
  if (Array.isArray(setupState.prefsKeys)) return;
  try {
    const rep = await api("/api/prefs");
    setupState.prefsKeys = Array.isArray(rep && rep.keys) ? rep.keys.slice() : [];
  } catch (e) {
    setupState.prefsKeys = [];
  }
}
function setupRecipe() {
  const recipes = (setupState.recipes && setupState.recipes.recipes) || [];
  return recipes.find((r) => r.id === setupState.recipeID) || null;
}
function setupPlanView(plan, recipe, local) {
  const modes = recipe && recipe.modes || [plan.mode || "host"];
  return h("div.recipe-plan",
    h("div.recipe-plan-head", h("h3", "Where this model will run"),
      h("div.segmented-control", { role: "group", "aria-label": "Where to run the model" },
        modes.map((m) => h("button", { type: "button", class: m === setupState.mode ? "active" : "",
          "aria-pressed": String(m === setupState.mode), onclick: () => setupSwitchMode(m),
        }, m === "host" ? "Host · direct" : m === "container" ? "Container · isolated" : m)))),
    h("p.small.muted", setupState.mode === "container"
      ? "Use the recipe's image with Docker or Podman. The host still needs a working GPU driver and device access."
      : "Use a local Python environment and run directly on this machine. Setup prepares any missing software."),
    Object.values(setupProgress).some((s) => s.running) ? h("p.small.muted", "Changes here apply to the next run, not the job already running.") : null,
    setupFitStatus(recipe) !== "fits" ? h("p.recipe-warning", setupFitLine(recipe)) : null,
    (plan.warnings || []).map((w) => h("p.recipe-warning", w)),
    setupPlanAdjustments(plan),
    setupPlanOverrides(recipe), setupPlanAdjust(plan), setupPlanLaunchBox(plan, local));
}

function setupPlanLaunchBox(plan, local) {
  const box = h("div.recipe-plan-launch-box");
  setupEnsureModule("views/setup/launch-disclosure.js").then(() => {
    if (!box.isConnected) return;
    const panel = setupPlanLaunchDisclosure(plan);
    if (local && local.has(panel.className)) { panel.open = local.get(panel.className); panel.dataset.local = "1"; }
    put(box, panel);
  }).catch((e) => put(box, failed(e)));
  return box;
}

function setupPlanCommand(plan) {
  if (plan.command) return plan.command;
  const exec = plan.exec || {};
  if (Array.isArray(exec.argv) && exec.argv.length) return exec.argv.map(setupQuote).join(" ");
  return "bernie serve " + (plan.recipe || setupState.recipeID) + " --mode " + (plan.mode || setupState.mode);
}
function setupQuote(arg) {
  const text = String(arg == null ? "" : arg);
  return /^[A-Za-z0-9_@%+=:,./-]+$/.test(text) ? text : "'" + text.replace(/'/g, "'\\''") + "'";
}
function setupPlanSettings(plan) {
  const settings = plan.settings || [];
  if (!settings.length) return null;
  return h("div.recipe-settings",
    h("h4", "The settings"),
    settings.map((s) => h("div.recipe-setting",
      h("span.recipe-setting-name", s.setting),
      h("span.recipe-setting-value", s.value == null || s.value === "" ? "–" : String(s.value)),
      s.why ? h("span.recipe-setting-why", s.why) : null)));
}
function setupPlanAdjustments(plan) {
  const adjustments = plan.adjustments || [];
  if (!adjustments.length) return null;
  return h("div.recipe-adjustments",
    h("h4", "What the plan changed"),
    adjustments.map((a) => h("div.recipe-adjustment",
      h("span.recipe-adjustment-what", a.setting + ": " + a.from + " → " + a.to),
      h("span.recipe-adjustment-why", a.reason))));
}
// Required inputs stay visible; optional overrides fold away.
function setupPlanOverrides(recipe) {
  if (!recipe) return null;
  const inputs = recipe.inputs || {};
  const fields = Object.keys(inputs).sort().map((name) => {
    const input = inputs[name];
    return { required: !!input.required, node: h("label.recipe-input",
      h("span.recipe-input-name", name + (input.required ? " (required)" : "")),
      h("input", { type: "text", name, value: setupSetValue(name, input.default || ""),
        placeholder: input.path ? "/absolute/path" : "", required: !!input.required, onchange: setupApplyOverrides })) };
  });
  return h("div.recipe-overrides", fields.filter((f) => f.required).map((f) => f.node),
    h("details.advanced.recipe-tuning", h("summary", "Optional settings and expert overrides"),
      h("p.small.muted", "Leave these empty to use the recipe defaults. Changes rebuild the preview, not the running server."),
      fields.filter((f) => !f.required).map((f) => f.node), setupPlanCommonOverrides(), setupPlanSaveDefaultButton()));
}
/** Structured overrides use the same keys as the CLI. */
function setupPlanCommonOverrides() {
  const port = h("input", {
    type: "number", min: "1", step: "1",
    name: "args.port",
    value: setupSetValue("args.port", ""),
    placeholder: "8080",
    onchange: () => setupApplyOverrides(),
  });
  const ctx = h("input", {
    type: "number", min: "1", step: "1",
    name: "args.max-model-len",
    value: setupSetValue("args.max-model-len", ""),
    placeholder: "32768",
    onchange: () => setupApplyOverrides(),
  });
  const name = h("input", {
    type: "text",
    name: "_name",
    value: setupState.name || "",
    placeholder: "run name (default: recipe id)",
    onchange: () => { setupState.name = name.value.trim(); setupApplyOverrides(); },
  });
  const model = h("input", {
    type: "text",
    name: "model.source",
    value: setupSetValue("model.source", ""),
    placeholder: "/path/to/model or org/repo",
    onchange: () => setupApplyOverrides(),
  });
  return h("div.recipe-common-overrides",
    h("label.recipe-input", h("span.recipe-input-name", "Port"), port),
    h("label.recipe-input", h("span.recipe-input-name", "Max context length"), ctx),
    h("label.recipe-input", h("span.recipe-input-name", "Run name"), name),
    h("label.recipe-input", h("span.recipe-input-name", "Model"), model));
}
function setupSetValue(name, fallback) {
  const prefix = name + "=";
  const found = setupState.sets.find((s) => s.startsWith(prefix));
  return found ? found.slice(prefix.length) : fallback;
}
async function setupApplyOverrides() {
  const box = setupState.planBox;
  if (!box) return;
  const fields = Array.from(box.querySelectorAll(".recipe-overrides input"));
  const keys = new Set(fields.map((f) => f.name));
  const sets = setupState.sets.filter((s) => !keys.has(s.slice(0, s.indexOf("="))));
  for (const field of fields) {
    const value = String(field.value || "").trim();
    if (field.name && field.name !== "_name" && value !== "") sets.push(field.name + "=" + value);
  }
  setupState.sets = sets;
  await loadSetupPlan();
}

function setupPlanAdjust(plan) {
  const controls = h("div");
  const panel = h("details.advanced.recipe-plan-adjust", h("summary", "GPU selection and effective settings"),
    controls,
    h("div.recipe-gpus", h("span.recipe-gpus-label", "GPUs"),
      h("span.recipe-gpus-value", (plan.gpus || []).length ? plan.gpus.join(", ") : "not reported"),
      plan.gpu_select_reason ? h("span.recipe-gpus-why", plan.gpu_select_reason) : null),
    setupPlanSettings(plan));
  let loaded = false;
  panel.addEventListener("toggle", async () => {
    if (!panel.open || loaded) return;
    loaded = true;
    try {
      await setupEnsureModule("views/setup/plan-adjust.js");
      put(controls, setupPlanAdjustBody());
    } catch (e) { put(controls, failed(e)); }
  });
  return panel;
}

/** The save action exists only when mode is a config key. */
function setupPlanSaveDefaultButton() {
  const keys = setupState.prefsKeys;
  if (!Array.isArray(keys) || !keys.includes("mode")) return null;
  const btn = h("button.small", { type: "button", onclick: setupPlanSaveDefaultClick }, "Save the mode as default");
  btn.title = "bernie config set mode " + setupState.mode;
  return btn;
}
async function setupPlanSaveDefaultClick() {
  try {
    await setupEnsureModule("views/setup/plan-adjust.js");
  } catch (e) { toast("Defaults could not be saved: " + (e.message || e)); return; }
  return setupSaveDefaultMode();
}
async function setupSwitchMode(mode) {
  if (mode === setupState.mode) return;
  setupState.mode = mode;
  await loadSetupPlan();
}
async function startSetupRecipe() {
  const recipe = setupRecipe();
  if (!recipe) return;
  try {
    const run = await api("/api/runs", {
      method: "POST",
      body: { recipe: recipe.id, set: setupState.sets, mode: setupState.mode },
    });
    toast("Started " + (run.name || recipe.id) + ".", true);
    if (setupState.onRefresh) setupState.onRefresh();
  } catch (e) {
    toast("Start failed: " + (e.message || e));
  }
}
function paintSetupRecipeHint() {
  const box = setupState.recipeHint;
  if (!box) return;
  const list = (setupState.recipes && setupState.recipes.recipes) || [];
  if (!list.length) {
    put(box);
    return;
  }
  if (list.some((r) => ["fits", "tight"].includes(setupFitStatus(r)))) {
    put(box);
    return;
  }
  const why = list.map(setupFitLine).filter(Boolean).slice(0, 2).join(" ");
  put(box, emptyState("No recipe fits this host yet.",
    why + " A recipe is a model and the way it is served. Fix the steps above, then press Re-check.",
    "bernie recipe list --all"));
}

function setupRecommendationWhy(recipe) {
  const rec = setupState.doc && setupState.doc.recommendation;
  if (rec && rec.id === recipe.id && rec.why) return rec.why + ".";
  return setupFitLine(recipe);
}

function setupRecipeNote() {
  const rec = setupState.doc && setupState.doc.recommendation;
  if (!rec) return null;
  return "Recommended: " + setupRecipeLabel(rec) + " - " + rec.why + ".";
}

function setupRecipeLabel(recipe) {
  const name = recipe.name || recipe.id;
  return recipe.description ? name + " - " + recipe.description : name;
}
