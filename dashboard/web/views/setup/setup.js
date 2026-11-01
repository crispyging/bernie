// The Setup tab answers one question: what do I do to make this box serve?
// Adapted from vllm-rdna-dash by CrispyGinger (Apache-2.0).
"use strict";

// The Setup tab answers one question: what do I do to make this box serve?
// It reads the onboarding checklist, the doctor report and the recipes, and
// it never changes the host on its own.
// Adapted from vllm-rdna-dash by CrispyGinger (Apache-2.0).
"use strict";

// SETUP_EXTRA_STEPS are the steps that come after the path of the host.
const SETUP_EXTRA_STEPS = ["bench", "tuning", "power"];

// SETUP_SMOKE_ENABLED says whether the endpoint that answers one prompt
// exists. The backend landed POST /api/smoke with the shape of spec §11.
const SETUP_SMOKE_ENABLED = true;

// setupProgress holds the live progress of a step, by step id. The map
// lives here so the first paint is one round-trip; steps.js fills it
// when its module arrives.
const setupProgress = {};

// setupLogs is the output of a step, by step id. Same ownership.
const setupLogs = {};

// setupStreams holds the open job streams, by step id. steps.js owns the
// streams; the map is declared here so the first paint carries nothing.
const setupStreams = {};

// setupJobRead is the moment each job was read the last time, so a replay
// of many lines does not ask the server for the same document again and
// again.
const setupJobRead = {};

// setupState holds the documents of the tab and the nodes it paints.
const setupState = {
  doc: null,
  doctor: null,
  recipes: null,
  hw: null,
  plan: null,
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
  pathJobID: "",
  // run is the record of the newest setup run (spec §6.4).
  run: null,
  modules: {},
};

/** viewSetup draws the tab: the headline, the plan, the checklist, the
 *  recipes, the doctor report and the raw documents. */
async function viewSetup(root) {
  setupState.generation++;
  const gen = setupState.generation;
  // Stop any streams the previous view opened. steps.js owns the streams;
  // it is loaded lazily, so the call comes back when the module arrives.
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
  // The glossary sits inside a disclosure: the reader opens it to read the
  // words on this page. Its module comes with the first open, so the
  // first paint does not carry it.
  const glossary = h("details.advanced.setup-glossary", {
    ontoggle: (ev) => {
      if (!ev.target.open || glossaryBox.hasChildNodes()) return;
      setupEnsureModule("views/setup/glossary.js")
        .then(() => paintSetupGlossary(root))
        .catch(() => {});
    },
  }, h("summary", "The words on this page"), glossaryBox);
  // The raw documents are the last thing of the tab: their module comes with
  // the first press, so the first paint does not carry it.
  const advancedBox = h("div");

  Object.assign(setupState, {
    head, actions, banner, hero, pathBox, needsYou, timeline, recipeBox, recipeHint, recipeErrors, planBox, doctorBox, glossary, glossaryBox,
    advanced: null, advancedBox,
    onRefresh: () => loadSetupDocument(),
  });

  put(root, head, banner, hero, pathBox, needsYou,
    h("section.section", { id: "setup-path" },
      sectionLabel("The path to a serving model"),
      timeline),
    h("section.section", { id: "setup-recipes" },
      sectionLabel("The recipe"),
      recipeHint, recipeBox, recipeErrors, planBox),
    h("section.section", { id: "setup-doctor" },
      sectionLabel("Doctor", h("span.grow"),
        h("button.small", { type: "button", onclick: loadSetupDoctor }, "Re-run")),
      doctorBox),
    glossary,
    advancedBox);

  setupEnsureModule("views/setup/advanced.js").then(() => {
    if (!advancedBox.isConnected) return;
    if (!setupState.advanced) setupState.advanced = setupAdvancedPanel();
    put(advancedBox, setupState.advanced);
  }).catch((e) => put(advancedBox, failed(e)));

  setHead(head, "BERNIE IS CHECKING THE HOST.", "Reading the host. The checklist arrives in a moment.");
  put(hero, loading());
  put(timeline, loading());
  put(recipeBox, loading());
  put(doctorBox, loading());

  await Promise.all([loadSetupDocument(gen), loadSetupRecipes(gen), loadSetupDoctor(gen)]);
  paintSetupRecipeHint();
}

/** setupEnsureModule loads a file of the tab the first time it is needed
 *  and answers when it is there. The first paint stays small: the reader
 *  waits for the headline, not for the chooser. */
function setupEnsureModule(src) {
  if (!setupState.modules[src]) {
    setupState.modules[src] = loadScript(src).catch((e) => {
      delete setupState.modules[src];
      throw e;
    });
  }
  return setupState.modules[src];
}

/** setupStartPath starts the whole path. The module of the path comes with
 *  the press. */
async function setupStartPath() {
  try {
    await setupEnsureModule("views/setup/path.js");
  } catch (e) {
    toast("The setup could not start: " + (e.message || e));
    return;
  }
  setupRunPath();
}

/** loadSetupDocument reads the onboarding checklist and paints the
 *  headline, the facts and the timeline. The Re-check buttons of the tab
 *  call this: it first drops the host caches (POST /api/refresh) so the
 *  next read is a fresh probe, then asks for the onboarding document.
 *  The first paint also drops the cache: the cost is one empty-map clear
 *  and the reader's wait stays one round-trip. */
async function loadSetupDocument(gen) {
  if (gen && gen !== setupState.generation) return;
  try { await refreshHost(); } catch { /* the cache drop is best-effort */ }
  if (gen && gen !== setupState.generation) return;
  let doc = null;
  try {
    doc = await api("/api/onboarding");
  } catch (e) {
    if (!gen || gen === setupState.generation) paintSetupError(e);
    return;
  }
  if (gen && gen !== setupState.generation) return;
  setupState.doc = doc;
  paintSetupHead(doc);
  paintSetupStale(doc);
  paintSetupNeedsYou(doc);
  paintSetupTimeline(doc);
  await paintSetupFacts(doc);
  // The progress streams and the job re-attach live in steps.js; load it
  // when the doc has settled and attach the running jobs.
  setupEnsureModule("views/setup/steps.js")
    .then(() => {
      if (gen !== setupState.generation) return;
      attachSetupJobs(doc.steps);
    })
    .catch(() => {});
  if (doc.path && doc.path.id) {
    setupEnsureModule("views/setup/path.js")
      .then(() => setupAttachPath(doc))
      .catch(() => {});
  }
}

/** paintSetupHead writes the state of the machine in one sentence and the
 *  actions that follow from it. */
function paintSetupHead(doc) {
  const say = setupHeadline(doc);
  setHead(setupState.head, say.title, say.lede);
  put(setupState.actions, setupHeadActions(doc, say));
  // The line under the one button: the checklist below is there for the
  // reader who wants to go step by step.
  const primary = doc.primary;
  if (say.action === "setup" && primary && primary.kind === "setup") {
    setupState.actions.append(h("span.small.muted.step-by-step", "or go step by step below."));
  }
}

/** paintSetupStale shows the banner of a checklist that is older than its
 *  time to live. */
function paintSetupStale(doc) {
  const banner = setupState.banner;
  if (!banner) return;
  if (doc.stale) {
    put(banner, "bernie is checking the host again. The values below are the last known state.");
    banner.hidden = false;
  } else {
    put(banner);
    banner.hidden = true;
  }
}

/** paintSetupError says that the state of the machine is unknown. */
function paintSetupError(e) {
  setHead(setupState.head, "THE STATE OF THIS MACHINE IS UNKNOWN.", "The dashboard could not read /api/onboarding.");
  put(setupState.actions, h("button.primary", { type: "button", disabled: true, "aria-disabled": "true" }, "Try again"));
  put(setupState.hero);
  put(setupState.timeline, h("div.setup-error",
    h("p", "The dashboard could not read the checklist."),
    h("p", String(e.message || e)),
    h("button", { type: "button", onclick: () => loadSetupDocument() }, "Try again")));
}

/** paintSetupTimeline draws the checklist of the host. */
function paintSetupTimeline(doc) {
  if (!setupState.timeline) return;
  if (!doc) doc = setupState.doc;
  if (!doc) return;
  const steps = doc.steps || [];
  // The inline progress of a step lands in setupProgress on first paint;
  // the live state from steps.js takes over when its module arrives.
  for (const step of steps) {
    if (step.progress && !setupProgress[step.id]) {
      const state = { phase: step.progress.phase || "", started: Date.now() };
      const total = step.progress.total || 0;
      if (total) {
        const pct = Math.round(((step.progress.done || 0) * 100) / total);
        state.pct = Math.max(0, Math.min(100, pct));
        state.detail = state.pct + "%";
      } else {
        state.detail = "working…";
      }
      setupProgress[step.id] = state;
    }
  }
  put(setupState.timeline, stepTimeline(steps, {
    extraIDs: SETUP_EXTRA_STEPS,
    nextID: doc.next,
    // The press of a step loads steps.js; the first paint does not carry it.
    onRun: (step, options) => {
      setupEnsureModule("views/setup/steps.js")
        .then(() => runSetupStep(step, options))
        .catch(() => {});
    },
    onRefresh: () => loadSetupDocument(),
    onWait: setupExplainWait,
    noteFor: setupStepNote,
    linkFor: setupStepLink,
    progress: setupProgress,
    logs: setupLogs,
    verify: setupVerifyBox,
    // The headline carries the one filled button of the tab.
    filled: !(doc.primary && doc.primary.kind === "setup"),
  }));
}

/** paintSetupNeedsYou writes the "Needs you" section: the items only the
 *  operator can do, above the path. The section is hidden when there is
 *  nothing to do; the timeline below carries the routine steps.
 *  See ia.md §5 R40. The module is lazy so the first paint stays small. */
function paintSetupNeedsYou(doc) {
  const box = setupState.needsYou;
  if (!box) return;
  setupEnsureModule("views/setup/needs-you.js")
    .then(() => paintSetupNeedsYouRender(doc))
    .catch(() => {});
}

/** setupStepLink is the link of an optional step. */
function setupStepLink(step) {
  if (step.id === "bench") return { href: "#benchmark", label: "the benchmark" };
  if (step.id === "tuning" || step.id === "power") return { href: "#dashboard", label: "the dashboard" };
  return null;
}

/** setupStepNote is the extra line under a step of the checklist. */
function setupStepNote(step) {
  const wait = setupWaitSentence(step);
  if (wait) return wait;
  if (step.id === "recipe") return setupRecipeNote();
  const estimate = setupEstimateNote(step);
  if (estimate) return estimate;
  return null;
}

/** setupEstimateNote says what the step cost the last time it ran on this
 *  host. The words come from the checklist (spec §3.3), so the CLI, the
 *  MCP server and the page say the same thing. */
function setupEstimateNote(step) {
  const e = step.estimate;
  return e && e.text ? "Last time here: " + e.text + "." : "";
}

/** loadSetupDoctor reads the doctor report and paints the findings. */
async function loadSetupDoctor() {
  const box = setupState.doctorBox;
  if (!box) return;
  put(box, loading());
  let report = null;
  try {
    report = await api("/api/doctor");
  } catch (e) {
    put(box, h("p", "The doctor could not run."),
      h("p", String(e.message || e)),
      cmdBlock("bernie doctor", "Copy the command"),
      h("button", { type: "button", onclick: loadSetupDoctor }, "Try again"));
    return;
  }
  setupState.doctor = report;
  const checks = report.checks || [];
  const bad = checks.filter((c) => c.status === "fail" || c.status === "warn");
  const summary = report.summary || {};
  put(box,
    h("div.doctor-summary", ["fail", "warn", "ok", "skip"].map((s) =>
      h("span.badge", { class: "b-" + s }, (summary[s] || 0) + " " + s))),
    bad.length
      ? h("div.doctor-findings-list", bad.map(setupDoctorFinding))
      : h("div.doctor-clean",
        h("p", "The doctor found nothing to report."),
        h("p", "Every check of the host passed.")),
    h("details.doctor-all", h("summary", "All checks (" + checks.length + ")"), checks.map(setupDoctorFinding)));
}

/** setupDoctorFinding is one finding: its state, its rule, the cause and
 *  the command that fixes it. */
function setupDoctorFinding(check) {
  const status = check.status || "warn";
  return h("div.finding.doctor-finding",
    h("div.doctor-finding-head",
      h("span.badge", { class: "b-" + status }, status),
      h("span.doctor-finding-id", check.id || "")),
    h("p.doctor-finding-cause", check.title || ""),
    check.detail ? h("p.small.muted", check.detail) : null,
    check.fix ? cmdBlock(check.fix, "Copy the fix command") : null,
    check.doc ? h("a.small", { href: check.doc, target: "_blank", rel: "noopener noreferrer" }, "the rule") : null);
}

/** setupVerifyBox is the box that asks the server one question. It appears
 *  when the endpoint exists; until then the step has no box. */
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

/** setupServerRunning says whether a server answers on this host. */
function setupServerRunning() {
  const doc = setupState.doc;
  const serve = doc && (doc.steps || []).find((s) => s.id === "serve");
  return setupServerServing(serve);
}

/** setupServerServing says whether the server of this host is up. A run that
 *  still loads the model is not serving yet: it is setting up. */
function setupServerServing(serve) {
  if (!serve || serve.state !== "running") return false;
  // The backend says whether the server answers (spec §3.3). The phase
  // words are the fallback for an older answer.
  if (serve.serving) return true;
  const text = ((serve.progress && serve.progress.phase) || "") + " " + (serve.summary || "");
  return !/loading|compiling|capturing|starting/i.test(text);
}

/** setupAskServer sends one prompt to the server and shows the answer. */
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

