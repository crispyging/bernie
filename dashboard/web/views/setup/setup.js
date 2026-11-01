// Setup view. Adapted from vllm-rdna-dash by CrispyGinger, with permission.
"use strict";

const SETUP_EXTRA_STEPS = ["bench", "tuning", "power"];
const SETUP_SMOKE_ENABLED = true;
const setupProgress = {};
const setupLogs = {};
const setupStreams = {};
const setupJobRead = {};

const setupState = {
  doc: null,
  doctor: null,
  recipes: null,
  hw: null,
  plan: null,
  planLoading: false,
  planRequest: 0,
  planError: "",
  recipeError: "",
  recipeID: "",
  mode: "host",
  name: "",
  sets: [],
  prefsKeys: null,
  head: null,
  actions: null,
  banner: null,
  hero: null,
  timeline: null,
  recipeBox: null,
  recipeErrors: null,
  planBox: null,
  doctorBox: null,
  advanced: null,
  generation: 0,
  onRefresh: null,
  pathBox: null,
  verifyRun: null,
  runLog: null,
  verifyAttachID: "",
  runError: "",
  run: null,
  modules: {},
};

async function viewSetup(root) {
  setupState.generation++;
  const gen = setupState.generation;
  // Re-entering the tab closes old streams, not the host jobs.
  setupEnsureModule("views/setup/steps.js")
    .then(() => {
      if (gen !== setupState.generation) return;
      if (typeof setupStopStreams === "function") setupStopStreams();
    })
    .catch(() => {});

  const head = stateHead("Setup");
  const actions = h("div.head-actions");
  head.append(actions);
  const banner = h("div.banner.warn", { hidden: true });
  const hero = h("div.setup-hero");
  const pathBox = h("div.setup-path-box");
  const needsYou = h("section.section.needs-you", { id: "setup-needs-you", hidden: true });
  const timeline = h("div.setup-timeline-body");
  const recipeBox = h("div.setup-recipe-chooser");
  const recipeHint = h("div.setup-recipe-hint");
  const recipeErrors = h("div.recipe-errors");
  const planBox = h("div.recipe-plan-box");
  const doctorBox = h("div.doctor-findings");
  const glossaryBox = h("div.setup-glossary-body");
  // Glossary and raw documents load on demand.
  const glossary = h("details.advanced.setup-glossary", {
    ontoggle: (ev) => {
      if (!ev.target.open || glossaryBox.hasChildNodes()) return;
      setupEnsureModule("views/setup/glossary.js")
        .then(() => paintSetupGlossary(root))
        .catch(() => {});
    },
  }, h("summary", "Setup terms"), glossaryBox);
  const advancedBox = h("div");

  Object.assign(setupState, {
    head, actions, banner, hero, pathBox, needsYou, timeline, recipeBox, recipeHint, recipeErrors, planBox, doctorBox, glossary, glossaryBox,
    advanced: null, advancedBox,
    onRefresh: () => loadSetupDocument(),
  });

  put(root, head, banner, hero, pathBox, needsYou,
    h("section.section", { id: "setup-path" },
      sectionLabel("Setup steps"),
      timeline),
    h("section.section", { id: "setup-recipes" },
      sectionLabel("Choose what to serve"),
      recipeHint, recipeBox, recipeErrors, planBox),
    h("section.section", { id: "setup-doctor" },
      sectionLabel("Host checks", h("span.grow"),
        h("button.small", { type: "button", onclick: () => loadSetupDoctor() }, "Re-run checks")),
      doctorBox),
    glossary,
    advancedBox);

  setupEnsureModule("views/setup/advanced.js").then(() => {
    if (!advancedBox.isConnected) return;
    if (!setupState.advanced) setupState.advanced = setupAdvancedPanel();
    put(advancedBox, setupState.advanced);
  }).catch((e) => put(advancedBox, failed(e)));

  setHead(head, "Checking this host", "Reading prerequisites and any existing setup jobs…");
  put(hero, loading());
  put(timeline, loading());
  put(recipeBox, loading());
  put(doctorBox, loading());

  await Promise.all([loadSetupDocument(gen), loadSetupRecipes(gen), loadSetupDoctor(gen)]);
  paintSetupRecipeHint();
}

/** Lazy modules keep the initial status read small. Failed loads can be retried. */
function setupEnsureModule(src) {
  if (!setupState.modules[src]) {
    setupState.modules[src] = loadScript(src).catch((e) => {
      delete setupState.modules[src];
      throw e;
    });
  }
  return setupState.modules[src];
}

/** Starting setup is always an explicit action. */
async function setupStartPath() {
  try {
    await setupEnsureModule("views/setup/path.js");
  } catch (e) {
    toast("The setup could not start: " + (e.message || e));
    return;
  }
  setupRunPath();
}

/** Re-check drops the host cache, then reads the authoritative checklist. */
async function loadSetupDocument(gen) {
  if (!gen) gen = setupState.generation;
  if (gen !== setupState.generation) return;
  try { await refreshHost(); } catch { /* best-effort cache drop */ }
  if (gen && gen !== setupState.generation) return;
  let doc = null;
  try {
    doc = await api("/api/onboarding");
  } catch (e) {
    if (!gen || gen === setupState.generation) paintSetupError(e);
    return;
  }
  if (gen && gen !== setupState.generation) return;
  if (!doc || !Array.isArray(doc.steps)) { paintSetupError(new Error("The server returned no setup checklist.")); return; }
  setupState.doc = doc;
  paintSetupHead(doc);
  paintSetupStale(doc);
  paintSetupNeedsYou(doc);
  paintSetupTimeline(doc);
  await paintSetupFacts(doc);
  setupEnsureModule("views/setup/steps.js").then(() => {
    if (gen !== setupState.generation) return;
    attachSetupJobs(doc.steps);
    setupRefreshRun(doc.path ? "path" : "python-env", true);
  }).catch((e) => toast("Live setup details could not load: " + (e.message || e)));
  if (doc.path && doc.path.id) setupEnsureModule("views/setup/path.js")
    .then(() => { if (gen === setupState.generation) setupAttachPath(doc); })
    .catch((e) => toast("Setup progress could not load: " + (e.message || e)));
}

function paintSetupHead(doc) {
  const say = setupHeadline(doc);
  setHead(setupState.head, say.title, say.lede);
  put(setupState.actions, setupHeadActions(doc, say));
}

function paintSetupStale(doc) {
  const banner = setupState.banner;
  if (!banner) return;
  if (doc.stale) {
    put(banner, "Host checks are being refreshed. These are the last known values, not a new confirmation that setup is ready.");
    banner.hidden = false;
  } else {
    put(banner);
    banner.hidden = true;
  }
}

function paintSetupError(e) {
  setHead(setupState.head, "Host status is unavailable", "Re-check the host to read the setup checklist again.");
  put(setupState.actions, h("button.primary", { type: "button", onclick: () => loadSetupDocument() }, "Try again"));
  put(setupState.banner, "The host status could not be refreshed. Any existing setup progress below is the last known state.");
  setupState.banner.hidden = false;
  if (setupState.doc) {
    const doc = setupState.doc;
    paintSetupTimeline(doc);
    paintSetupNeedsYou(doc);
    paintSetupFacts(doc);
    setupEnsureModule("views/setup/steps.js").then(() => attachSetupJobs(doc.steps));
    if (doc.path) setupEnsureModule("views/setup/path.js").then(() => setupAttachPath(doc));
    else if (typeof setupPaintPath === "function") setupPaintPath();
    return;
  }
  put(setupState.hero);
  put(setupState.timeline, h("p", "The setup checklist could not be read."),
    h("details.advanced", h("summary", "Connection details"), h("pre.step-error", String(e.message || e))));
}

function paintSetupTimeline(doc) {
  if (!setupState.timeline) return;
  if (!doc) doc = setupState.doc;
  if (!doc) return;
  const steps = doc.steps || [];
  for (const step of steps) {
    const live = setupProgress[step.id];
    if (live && !live.running && !step.job_id && ["done", "skipped"].includes(step.state)) {
      delete setupProgress[step.id];
      delete setupLogs[step.id];
    }
    if (step.progress && !setupProgress[step.id]) {
      const state = { phase: step.title || "Waiting for live status…", running: step.job_state === "running", started: 0 };
      const total = step.progress.total || 0;
      if (total > 0) {
        state.pct = Math.max(0, Math.min(100, Math.round(((step.progress.done || 0) * 100) / total)));
        state.detail = state.pct + "%";
      }
      setupProgress[step.id] = state;
    }
  }
  put(setupState.timeline, stepTimeline(steps, {
    extraIDs: SETUP_EXTRA_STEPS,
    nextID: doc.next,
    onRun: setupStartStep,
    onStop: (id) => setupEnsureModule("views/setup/steps.js").then(() => setupStopJob(id)),
    onRefresh: () => loadSetupDocument(),
    onWait: setupExplainWait,
    noteFor: setupStepNote,
    linkFor: setupStepLink,
    progress: setupProgress,
    logs: setupLogs,
    verify: setupVerifyBox,
    primarySetup: !!(doc.primary && doc.primary.kind === "setup"),
    pathRunning: !!(setupProgress.path && setupProgress.path.running || doc.path && doc.path.state === "running"),
    operatorSummary: true,
    filled: false,
  }));
}

/** Operator-only prerequisites stay above the collapsed checklist. */
function paintSetupNeedsYou(doc) {
  const box = setupState.needsYou;
  if (!box) return;
  const gen = setupState.generation;
  setupEnsureModule("views/setup/needs-you.js")
    .then(() => { if (gen === setupState.generation && doc === setupState.doc) paintSetupNeedsYouRender(doc); })
    .catch((e) => toast("Prerequisite details could not load: " + (e.message || e)));
}

function setupStepLink(step) {
  if (step.id === "bench") return { href: "#benchmark", label: "the benchmark" };
  if (step.id === "tuning" || step.id === "power") return { href: "#dashboard", label: "the dashboard" };
  return null;
}

function setupStepNote(step) {
  const wait = setupWaitSentence(step);
  if (wait) return wait;
  if (step.id === "recipe") return setupRecipeNote();
  const estimate = setupEstimateNote(step);
  if (estimate) return estimate;
  return null;
}

/** Historical measurements, never a new ETA. */
function setupEstimateNote(step) {
  const e = step.estimate;
  return e && e.text ? "Last time here: " + e.text + "." : "";
}

async function loadSetupDoctor() {
  const box = setupState.doctorBox;
  if (!box) return;
  put(box, loading());
  let report = null;
  try {
    report = await api("/api/doctor");
  } catch (e) {
    put(box, h("p", "Host checks are unavailable."),
      h("details.advanced", h("summary", "Error and terminal command"), h("pre.step-error", String(e.message || e)), cmdBlock("bernie doctor", "Copy the command")),
      h("button", { type: "button", onclick: () => loadSetupDoctor() }, "Try again"));
    return;
  }
  setupState.doctor = report;
  const checks = report.checks || [];
  const bad = checks.filter((c) => c.status === "fail" || c.status === "warn");
  const summary = report.summary || {};
  put(box,
    h("div.doctor-summary", ["fail", "warn", "ok", "skip"].map((s) =>
      h("span.badge", { class: "b-" + s + (["ok", "skip"].includes(s) ? " advanced-only" : "") }, (summary[s] || 0) + " " + s))),
    bad.length ? h("div.doctor-findings-list", bad.map(setupDoctorFinding))
      : h("p.doctor-clean", checks.length ? "No warnings or failures were reported in " + checks.length + " host checks." : "No host checks were returned. Re-run checks to try again."),
    h("details.advanced.doctor-all", h("summary", "All host checks (" + checks.length + ")"), checks.map(setupDoctorFinding)));
}

/** Findings remain visible; command-sized evidence is available on demand. */
function setupDoctorFinding(check) {
  const status = check.status || "warn";
  return h("div.finding.doctor-finding",
    h("div.doctor-finding-head",
      h("span.badge", { class: "b-" + status }, status),
      h("span.doctor-finding-id.advanced-only", check.id || "")),
    h("p.doctor-finding-cause", check.title || ""),
    check.detail ? h("p.small.muted", check.detail) : null,
    check.fix ? h("details.advanced", h("summary", "Show recovery command"), cmdBlock(check.fix, "Copy the fix command")) : null,
    check.doc ? h("a.small", { href: check.doc, target: "_blank", rel: "noopener noreferrer" }, "Troubleshooting guide") : null);
}

/** A real prompt checks the server, independently of environment verification. */
function setupVerifyBox() {
  if (!SETUP_SMOKE_ENABLED) return null;
  const running = setupServerRunning();
  const answer = h("div.try-answer");
  const input = h("input", {
    type: "text",
    placeholder: "What is the capital of Portugal?",
    disabled: !running,
    "aria-label": "The question for the server",
  });
  const form = h("form.try-form", {
    onsubmit: (e) => {
      e.preventDefault();
      setupAskServer(input.value, answer);
    },
  }, input, h("button", { type: "submit", disabled: !running }, "Ask"));
  return h("div.try-box",
    h("p.try-label", "Ask the server something."),
    running ? form : h("p.try-note", "Start the server first."),
    answer);
}

function setupServerRunning() {
  const doc = setupState.doc;
  const serve = doc && (doc.steps || []).find((s) => s.id === "serve");
  return setupServerServing(serve);
}

function setupServerServing(serve) {
  if (!serve || serve.state !== "running" || serve.id && serve.id !== "serve") return false;
  if (serve.serving === true) return true;
  const text = ((serve.progress && serve.progress.phase) || "") + " " + (serve.summary || "");
  return !/loading|compiling|sizing|capturing|starting|warming/i.test(text) && /running|serving|ready|answers/i.test(text);
}

async function setupAskServer(prompt, answer) {
  if (!prompt) return;
  put(answer, h("span.spin"), " asking…");
  try {
    const result = await api("/api/smoke", { method: "POST", body: { prompt: prompt } });
    const metrics = [];
    if (result.ttft_ms != null) metrics.push(result.ttft_ms + " ms to the first token");
    if (result.tok_s != null) metrics.push(result.tok_s + " tok/s");
    put(answer,
      h("p.try-reply", result.reply || ""),
      metrics.length ? h("p.try-metrics", metrics.join(" · ")) : null);
  } catch (e) {
    put(answer,
      h("p.try-error", "The server did not answer."),
      h("p.try-reason", String(e.message || e)));
  }
}

