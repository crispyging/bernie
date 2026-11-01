// Setup run and environment-check evidence. Loaded when needed.
"use strict";

const setupPathLoaded = setupEnsureModule("views/setup/steps.js");

async function setupRunPath() {
  await setupPathLoaded;
  if (Object.values(setupProgress).some((s) => s.running)) { toast("Wait for the current setup job, or stop it first."); return; }
  setupProgress.path = { phase: "Starting setup…", started: Date.now(), running: true };
  setupLogs.path = [];
  setupStartTicker();
  setupPaintPath();
  let job;
  try {
    job = await api("/api/onboarding/path/run", {
      method: "POST", body: { recipe: setupState.recipeID, mode: setupState.mode, set: setupState.sets },
    });
  } catch (e) {
    Object.assign(setupProgress.path, { phase: "Setup could not start.", error: String(e.message || e), failed: true, running: false, ended: Date.now() });
    setupPaintPath();
    return;
  }
  if (!job || job.root_required) {
    Object.assign(setupProgress.path, { phase: "Administrator access is needed before setup can continue.", failed: true, running: false, ended: Date.now(), commands: job && job.commands || [] });
    setupPaintPath();
    loadSetupDocument();
    return;
  }
  followSetupJob({ id: "path" }, job, { onRefresh: () => loadSetupDocument() });
  loadSetupDocument();
  if (setupState.pathBox) setupState.pathBox.scrollIntoView({ behavior: "smooth", block: "center" });
}

async function setupStopPath() {
  await setupPathLoaded;
  return setupStopJob("path");
}

async function setupAttachPath(doc) {
  await setupPathLoaded;
  const path = doc && doc.path;
  if (!path || !path.id) return;
  await attachSetupJob({ id: "path", title: "Setup", job_id: path.id, job_state: path.state },
    { onRefresh: () => loadSetupDocument() });
  setupRefreshRun("path", true);
  setupPaintPath();
}

function setupRunRows(run = setupState.run) {
  if (!run || !run.steps || !run.steps.length) return null;
  return h("div.step-run",
    h("p.small.muted", "Installation record · " + (run.started_at ? new Date(run.started_at).toLocaleString() : "time unavailable")),
    h("div.step-run-list", run.steps.map((s) => setupRunRow(s, run.id))));
}
function setupRunRow(s, runID) {
  return h("div", { class: "step-run-row step-run-row-" + s.state, "data-run-step": s.id },
    h("span.step-run-row-id", s.title || s.id),
    h("span.step-run-row-state", TIMELINE_STATE_WORD[s.state] || s.state),
    h("span.step-run-row-duration", { "data-dur": "1" }, setupRunRowDuration(s)),
    (s.commands || []).length ? h("details.advanced.step-run-commands", h("summary", "Commands"),
      s.commands.map((c) => cmdBlock(c, "Copy the command"))) : null,
    s.log_path ? h("a.small", { href: "/api/setup/runs/" + encodeURIComponent(runID) + "/log/" + encodeURIComponent(s.id), target: "_blank", rel: "noopener noreferrer" }, "Recorded output (JSON)") : null);
}

function setupRunRowDuration(s) {
  if (s.state === "running") {
    const t = setupRunStepStart(s);
    const active = Object.values(setupProgress).some((live) => live.running && live.runID === (setupState.run && setupState.run.id));
    return active && t ? setupDuration((Date.now() - t) / 1000) : s.duration_s > 0 ? setupDuration(s.duration_s) + " recorded" : "Live timing unavailable";
  }
  return s.duration_s > 0 ? setupDuration(s.duration_s) : "";
}

function setupRunStepStart(s) {
  return Date.parse(s.started_at) || (setupProgress[s.id] && setupProgress[s.id].started) || 0;
}

function setupPaintRunRows() {
  for (const s of (setupState.run && setupState.run.steps) || []) {
    const row = document.querySelector('[data-run-step="' + s.id + '"] [data-dur]');
    if (row) row.textContent = setupRunRowDuration(s);
  }
}

/** Verification is its own check; passing it cannot erase an installation failure. */
function setupPythonEnv() {
  const install = setupState.run;
  const check = setupState.verifyRun;
  const saved = setupProgress.verify;
  const latest = check && (!install || Date.parse(check.started_at) >= Date.parse(install.started_at)) ? check : install;
  const live = saved && (saved.running || !latest || saved.runID === latest.id || Date.parse(latest.started_at) <= (saved.started || 0)) ? saved : null;
  const record = live ? [check, install].find((r) => r && r.id === live.runID) : latest;
  const separate = !!live || record === check;
  const step = record && (record.steps || []).find((s) => s.id === "venv.verify");
  if (!step && !live) return null;
  const running = live && live.running;
  const state = live ? live.failed ? "failed" : running ? "running" : "done" : step.state;
  const comps = step && step.evidence && step.evidence.components || [];
  const busy = Object.values(setupProgress).some((s) => s.running);
  return h("div.python-env", { "data-check-job": live && live.jobID || "" },
    h("h3", "Python environment check ", statePill(state === "failed" ? "fail" : state, TIMELINE_STATE_WORD[state] || state)),
    h("p.small.muted", separate ? "Separate check; this does not rebuild or start the server." : "Check recorded during installation; this does not test the server."),
    record ? h("p.small.muted", "Recorded " + (record.started_at ? new Date(record.started_at).toLocaleString() : "at an unknown time")) : null,
    install && install.state === "failed" ? h("p.recipe-warning", "The installation failed. A later environment check does not repair or rerun that failed build.") : null,
    !live && step && step.state === "running" ? h("p.recipe-warning", "This is a saved in-progress check; live job status has not been confirmed.") : null,
    h("p.step-progress-sentence", { "data-verify-phase": "1" }, live && live.phase || (state === "failed" ? "The environment check failed. Review its error below." : "")),
    h("p.step-progress-detail", { "data-verify-time": "1" }, live ? setupLiveDetail(live) : ""),
    h("p.small.muted.mono.step-log-line", { "data-verify-output": "1" }, live && live.logLine || ""),
    h("button.small", { type: "button", disabled: busy, onclick: setupVerifyAgain }, running ? "Checking…" : "Check environment again"),
    running ? h("button.small.setup-env-stop", { type: "button", onclick: () => setupStopJob("verify") }, "Stop check") : null,
    h("details.advanced.setup-env-details", h("summary", "Components, error and check output"),
      h("pre.step-error", live && live.error || record && record.error || ""),
      comps.length ? h("div.python-env-rows", comps.map((c) => h("div.python-env-row",
        h("span.python-env-name", c.name || ""), h("span.python-env-version", c.version || "not installed"),
        c.note ? h("span.python-env-note", c.note) : null))) : h("p.small.muted", "No component evidence was recorded."),
      step && record ? h("a.small", { href: "/api/setup/runs/" + encodeURIComponent(record.id) + "/log/" + encodeURIComponent(step.id), target: "_blank", rel: "noopener noreferrer" }, "Open recorded check output (JSON)") : null,
      h("div.log-lines.step-log.setup-verify-log", { role: "log", "aria-live": "off" })));
}

async function setupVerifyAgain() {
  await setupPathLoaded;
  setupProgress.verify = { phase: "Checking the Python environment…", started: Date.now(), running: true };
  setupLogs.verify = [];
  setupStartTicker();
  setupPaintPath();
  let job;
  try { job = await api("/api/setup/verify", { method: "POST", body: { mode: setupState.mode } }); }
  catch (e) {
    Object.assign(setupProgress.verify, { phase: "The check could not start.", error: String(e.message || e), failed: true, running: false, ended: Date.now() });
    setupPaintPath();
    return;
  }
  followSetupJob({ id: "verify" }, job, { onRefresh: () => loadSetupDocument() });
}

/** Update stable nodes so elapsed ticks do not close disclosures or steal focus. */
function setupPaintPath() {
  const box = setupState.pathBox;
  if (!box) return;
  const live = setupProgress.path;
  const run = setupState.run;
  if (!live && !run && !setupState.verifyRun && !setupProgress.verify && !setupState.runError) { put(box); return; }
  if (!box.querySelector(".setup-run")) put(box,
    h("section.section.setup-run",
      h("p.small.muted", { "data-path-label": "1" }),
      h("h3", { "data-path-phase": "1", "aria-live": "polite" }),
      h("p.step-progress-detail", { "data-path-time": "1" }),
      h("p.small.muted.mono.step-log-line", { "data-path-output": "1" }),
      h("p.small", { "data-path-help": "1" }),
      h("div.setup-run-actions",
        h("button", { type: "button", "data-path-stop": "1", onclick: setupStopPath }, "Stop setup"),
        h("button", { type: "button", onclick: () => loadSetupDocument() }, "Refresh status"),
        h("button", { type: "button", "data-path-retry": "1", onclick: setupStartPath }, "Retry setup")),
      h("details.advanced.setup-run-details", h("summary", "Error, step history and output"),
        h("pre.step-error", { "data-path-error": "1" }),
        h("div.setup-run-history"),
        h("p.small.muted", { "data-path-log-note": "1" }),
        h("div.log-lines.step-log.setup-path-log", { role: "log", "aria-live": "off" }))),
    h("p.recipe-warning.setup-history-warning"), h("div.setup-check-evidence"));
  const panel = box.querySelector(".setup-run");
  panel.hidden = !live && !run;
  const running = !!(live && live.running);
  const failed = live ? live.failed : run && ["failed", "blocked"].includes(run.state);
  const linked = live && run && live.runID === run.id;
  const shown = !live || linked ? run : null;
  const steps = shown && shown.steps || [];
  const step = steps.find((s) => s.state === "running" || s.state === "failed") || steps[steps.length - 1];
  const label = running ? "Setup in progress" : live ? failed ? "Setup did not finish" : "Setup finished" : "Last recorded installation · " + (run && run.state || "unknown");
  box.querySelector("[data-path-label]").textContent = label;
  box.querySelector("[data-path-phase]").textContent = live && live.failed && step ? step.title + " did not finish" : live && live.phase || step && step.title || "No step details were recorded.";
  box.querySelector("[data-path-time]").textContent = live ? setupLiveDetail(live) : run && run.duration_s != null ? "Recorded duration: " + setupDuration(run.duration_s) : "Duration unavailable";
  const log = setupState.runLog;
  const output = live && live.logLine || shown && log && log.run === shown.id && log.logLine;
  box.querySelector("[data-path-output]").textContent = output ? "Latest available output: " + output : "No output is available yet.";
  box.querySelector("[data-path-help]").textContent = live && live.unavailable
    ? "Live status could not be read. Refresh status before starting another job."
    : failed ? "Review the error and output, correct the cause, then re-check the host before retrying."
    : running ? "You can reload; setup continues on the host. Elapsed time alone does not confirm progress."
    : shown && shown.state === "running" ? "This saved record says in progress; live job status has not been confirmed. Refresh host status."
    : "Recorded installation outcome only. Check the host status above to see whether a model is serving.";
  box.querySelector("[data-path-error]").textContent = live && live.error || shown && shown.error || "";
  const stop = box.querySelector("[data-path-stop]");
  stop.hidden = !running;
  stop.disabled = !live || !live.jobID || !!live.stopping;
  box.querySelector("[data-path-retry]").hidden = !failed || running || !(setupState.doc && setupState.doc.primary && setupState.doc.primary.kind === "setup");
  const history = box.querySelector(".setup-run-history");
  const key = JSON.stringify([shown && shown.id, steps.map((s) => s.id + s.state), live && live.commands]);
  if (history.dataset.key !== key) {
    history.dataset.key = key;
    put(history, setupRunRows(shown), (live && live.commands || []).map((c) => cmdBlock(c, "Copy the administrator command")));
  }
  const recorded = shown && log && log.run === shown.id;
  const lines = setupLogs.path || [];
  const recordedOutput = recorded && (!lines.length || live && live.logTail);
  setupPaintLog(box.querySelector(".setup-path-log"), recordedOutput ? log.lines || [] : lines);
  box.querySelector("[data-path-log-note]").textContent = recordedOutput && log.truncated ? "Showing the last 400 recorded lines." : live && live.logTail && !recordedOutput ? "Showing the last saved job output." : "";
  box.querySelector(".setup-history-warning").textContent = setupState.runError || "";
  const checkBox = box.querySelector(".setup-check-evidence");
  const verify = setupProgress.verify;
  const check = setupState.verifyRun;
  const busy = Object.values(setupProgress).some((s) => s.running);
  const checkKey = JSON.stringify([run && [run.id, run.state], check && [check.id, check.state], verify && [verify.jobID, verify.runID, verify.running, verify.failed, verify.error], busy]);
  if (checkBox.dataset.key !== checkKey) {
    checkBox.dataset.key = checkKey;
    put(checkBox, setupPythonEnv());
  }
  const card = checkBox.querySelector(".python-env");
  const currentCheck = verify && card && card.dataset.checkJob === verify.jobID;
  const phase = checkBox.querySelector("[data-verify-phase]");
  const time = checkBox.querySelector("[data-verify-time]");
  if (currentCheck && phase) phase.textContent = verify.phase || "";
  if (currentCheck && time) time.textContent = setupLiveDetail(verify);
  const preview = checkBox.querySelector("[data-verify-output]");
  if (currentCheck && preview) preview.textContent = verify.logLine ? "Latest available output: " + verify.logLine : "";
  const verifyStop = checkBox.querySelector(".setup-env-stop");
  if (verifyStop) verifyStop.disabled = !!verify.stopping || !verify.jobID;
  setupPaintLog(checkBox.querySelector(".setup-verify-log"), currentCheck ? setupLogs.verify || [] : []);
  const status = (running ? "running" : failed ? "failed" : "finished") + (verify && verify.running ? ":checking" : "");
  if (panel.dataset.status !== status) {
    panel.dataset.status = status;
    if (setupState.doc) paintSetupHead(setupState.doc);
  }
}
