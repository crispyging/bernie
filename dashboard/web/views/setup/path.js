// The one action of the Setup tab: it runs the whole path that bernie can
// do on this host. The module comes when the reader presses the button, so
// the first paint does not carry it.
"use strict";

// setupPathLoaded waits for steps.js to be there before this module uses
// any of its functions. setupEnsureModule loads steps.js on the first call.
const setupPathLoaded = setupEnsureModule("views/setup/steps.js");

/** setupRunPath starts the whole path: the setup steps, the server and the
 *  check that proves it answers. It is one button and one job. */
async function setupRunPath() {
  await setupPathLoaded;
  setupProgress.path = { phase: "Starting\u2026", started: Date.now() };
  setupLogs.path = [];
  setupStartTicker();
  setupPaintPath();
  let job = null;
  try {
    job = await api("/api/onboarding/path/run", {
      method: "POST",
      body: { recipe: setupState.recipeID, mode: setupState.mode, set: setupState.sets },
    });
  } catch (e) {
    setupProgress.path = { phase: "Failed: " + (e.message || e), failed: true };
    setupPaintPath();
    return;
  }
  if (!job || job.root_required) {
    // The path stops at the step that needs root: the page shows the exact
    // command and the Re-check that reads the host again.
    setupProgress.path = {
      phase: "This step needs root. Run the command below as root, then re-check.",
      failed: true,
      commands: (job && job.commands) || [],
    };
    setupPaintPath();
    return;
  }
  setupState.pathJobID = job.id;
  followSetupJob({ id: "path" }, job, { onRefresh: () => { setupRefreshRun("path"); loadSetupDocument(); } });
}

/** setupStopPath stops the run of the path. The job ends and the tab says
 *  so; nothing is hidden. */
async function setupStopPath() {
  await setupPathLoaded;
  const id = setupState.pathJobID;
  if (!id) return;
  try {
    await api("/api/jobs/" + encodeURIComponent(id) + "/stop", { method: "POST", body: {} });
  } catch (e) {
    toast("The run could not be stopped: " + (e.message || e));
    return;
  }
  setupProgress.path = { phase: "Stopped.", failed: true };
  setupPaintPath();
}

/** setupAttachPath re-attaches to the job of the whole path after a
 *  reload. The page knows the id, so the run survives the browser. */
async function setupAttachPath(doc) {
  await setupPathLoaded;
  const path = doc && doc.path;
  if (!path || !path.id) return;
  setupState.pathJobID = path.id;
  setupProgress.path = { phase: path.phase || "Working\u2026" };
  setupPaintPath();
  if (path.state !== "running") return;
  setupLogs.path = [];
  followSetupJob({ id: "path" }, { id: path.id }, { onRefresh: () => { setupRefreshRun("path"); loadSetupDocument(); } });
}

/** setupRunRows draws the steps of the setup run with their state and the
 *  time they took. The record of the run is the one source (spec §6.4). */
function setupRunRows() {
  const run = setupState.run;
  if (!run || !run.steps || !run.steps.length) return null;
  return h("div.step-run", run.steps.map((s) => setupRunRow(s)));
}

/** setupRunRow is one row of the run list. The class names the state; the
 *  data-step id hooks the tick that re-paints the elapsed time of the
 *  running step. */
function setupRunRow(s) {
  const cls = "step-run-row step-run-row-" + s.state;
  return h("div", { class: cls, "data-step": s.id },
    h("span.step-run-row-id", s.id),
    h("span.step-run-row-state", s.state),
    h("span.step-run-row-duration", { "data-dur": "1" }, setupRunRowDuration(s)));
}

/** setupRunRowDuration is the elapsed-time text of a row. Frozen when the
 *  step has finished; live (ticked by setupTick) while the step runs. */
function setupRunRowDuration(s) {
  if (s.duration_s > 0) return setupDuration(s.duration_s);
  if (s.state !== "running") return "";
  // No start time of the step: fall back to the start of the path.
  const started = (setupProgress[s.id] && setupProgress[s.id].started) ||
                  (setupProgress.path && setupProgress.path.started);
  if (!started) return "";
  const sec = Math.max(0, Math.round((Date.now() - started) / 1000));
  return setupDuration(sec);
}

/** setupPaintRunRows re-paints the elapsed time of any running run row.
 *  Called every second by the tick in setup.js so a long build shows
 *  "running Ns" without waiting for the next event. */
function setupPaintRunRows() {
  const run = setupState.run;
  if (!run || !run.steps) return;
  for (const s of run.steps) {
    const row = document.querySelector('[data-step="' + s.id + '"] [data-dur]');
    if (!row) continue;
    const text = setupRunRowDuration(s);
    if (row.textContent !== text) row.textContent = text;
  }
}

/** setupPythonEnv draws the components table of the venv.verify step of
 *  the run: python, torch, triton, vllm (with the fork's git sha and
 *  branch), flash-attn, causal-conv1d, transformers, rocBLAS, RCCL and
 *  the host ROCm SDK (spec §6.3). The record is the one source: the
 *  page reads the same JSON the CLI prints and the MCP server returns.
 *  Lazy: when the run has no venv.verify step or its evidence has no
 *  components, the table is not shown.
 *
 *  The button checks the venv again: the server runs the check in the
 *  background and the page reads the new record of the run when it ends. */
function setupPythonEnv() {
  const run = setupState.run;
  if (!run || !run.steps) return null;
  const step = run.steps.find((s) => s.id === "venv.verify");
  if (!step || !step.evidence || !step.evidence.components) return null;
  const comps = step.evidence.components;
  if (!comps.length) return null;
  const live = setupProgress.verify || {};
  const running = !!live.running;
  return h("div.python-env",
    h("p.small.muted", "venv components (from the run record)"),
    h("div.python-env-rows", comps.map((c) =>
      h("div.python-env-row",
        h("span.python-env-name", c.name || ""),
        h("span.python-env-version", c.version || "absent"),
        c.note ? h("span.python-env-note", "(" + c.note + ")") : null))),
    h("button.small", { type: "button", disabled: running, onclick: setupVerifyAgain },
      running ? "Checking\u2026" : "Verify again"),
    live.phase ? h("p.step-progress-sentence", live.phase) : null);
}

/** setupVerifyAgain checks the venv again. The server runs the check of
 *  the venv in the background and records the run; the page reads that
 *  record when the job ends and paints the new components (spec §6.4).
 *  The button stays disabled while the check runs. */
async function setupVerifyAgain() {
  await setupPathLoaded;
  setupProgress.verify = { phase: "Checking the venv\u2026", started: Date.now(), running: true };
  setupStartTicker();
  setupPaintPath();
  let job = null;
  try {
    job = await api("/api/setup/verify", { method: "POST", body: { mode: setupState.mode } });
  } catch (e) {
    setupProgress.verify = { phase: "Failed: " + (e.message || e), failed: true, running: false };
    setupPaintPath();
    return;
  }
  followSetupJob({ id: "verify" }, job, {
    onRefresh: () => {
      const live = setupProgress.verify || {};
      if (live.failed || live.phase === "Done.") live.running = false;
      // The record of the run is the one source: the page reads it and
      // paints the components of the new run.
      setupRefreshRun("verify");
      setupPaintPath();
    },
  });
}

/** setupPaintPath paints the run of the path: the step it is on, the phase
 *  and the Stop control. */
function setupPaintPath() {
  const box = setupState.pathBox;
  if (!box) return;
  const live = setupProgress.path;
  if (!live || !live.phase) {
    put(box);
    return;
  }
  const running = !live.failed && live.phase !== "Done." && live.phase !== "Stopped.";
  const commands = live.commands || [];
  put(box,
    h("p.small.muted", running ? "bernie is setting up this box." : (live.failed ? "The setup stopped." : "The setup is done.")),
    h("p.step-progress-sentence", live.phase),
    live.detail ? h("p.small.muted", live.detail) : null,
    setupRunRows(),
    setupPythonEnv(),
    commands.map((c) => cmdBlock(c, "Copy the command")),
    running ? h("button.small", { type: "button", onclick: setupStopPath }, "Stop") : null,
    commands.length ? h("button.small", { type: "button", onclick: () => loadSetupDocument() }, "Re-check") : null);
}
