// The actions of the steps of the checklist: start a step, follow the job
// it starts and show the progress under the step.
// Adapted from vllm-rdna-dash by CrispyGinger (Apache-2.0).
"use strict";

async function runSetupStep(step, options = {}) {
  const id = step.id;
  setupProgress[id] = { phase: "Starting…", started: Date.now(), running: true };
  setupLogs[id] = [];
  setupStartTicker();
  paintSetupTimeline();
  let job;
  try {
    job = await api("/api/onboarding/" + encodeURIComponent(id) + "/run", { method: "POST", body: {} });
  } catch (e) {
    Object.assign(setupProgress[id], { phase: "The step could not start.", error: String(e.message || e), failed: true, running: false, ended: Date.now() });
    paintSetupTimeline();
    return;
  }
  if (!job || job.root_required) {
    Object.assign(setupProgress[id], { phase: "Administrator access is needed. Run the commands, then re-check.", commands: job && job.commands || [], failed: true, running: false, ended: Date.now() });
    paintSetupTimeline();
    loadSetupDocument();
    return;
  }
  followSetupJob(step, job, options);
  loadSetupDocument();
}

/** Keep the job outcome separate from step titles and raw output. */
function followSetupJob(step, job, options = {}) {
  const id = step.id;
  if (setupStreams[id]) setupStreams[id].close();
  const current = setupProgress[id] || (setupProgress[id] = {});
  Object.assign(current, { jobID: job.id, running: true, finished: false, failed: false });
  setupApplyJob(current, job);
  setupStartTicker();
  const finish = (name, raw) => {
    if (setupProgress[id] !== current) return;
    if (setupStreams[id]) setupStreams[id].close();
    delete setupStreams[id];
    let doc;
    try { doc = typeof raw === "string" ? JSON.parse(raw) : raw; } catch { doc = null; }
    setupApplyJob(current, doc || { state: name, finished_at: new Date().toISOString() });
    if (!(setupLogs[id] || []).length && doc && doc.lines) setupLogs[id] = doc.lines.slice();
    paintSetupProgress(id);
    if (["path", "python-env", "verify"].includes(id)) setupRefreshRun(id, true);
    if (options.onRefresh) options.onRefresh();
  };
  setupStreams[id] = openJobEvents(job.id, {
    line: (ev) => {
      if (setupProgress[id] !== current) return;
      const text = ev.data || "";
      (setupLogs[id] || (setupLogs[id] = [])).push(text);
      const marker = text.match(/^== ([^:]+):\s*(.+)$/);
      if (marker && !current.finished) { current.markerStep = marker[1]; current.stepID = marker[1]; current.phase = marker[2]; delete current.pct; current.detail = ""; }
      setupLastOutput(current, [text]);
      paintSetupProgress(id);
      loadSetupJobProgress(job.id, id);
      // Never refresh/re-attach onboarding on each replayed log line.
      if (["python-env", "path", "verify"].includes(id)) setupRefreshRun(id);
    },
    done: (ev) => finish("done", ev.data),
    failed: (ev) => finish("failed", ev.data),
  });
  const stream = setupStreams[id];
  stream.addEventListener("error", () => {
    if (!current.finished) { current.unavailable = true; paintSetupProgress(id); }
  });
  stream.addEventListener("open", () => {
    if (!current.finished) { current.unavailable = false; paintSetupProgress(id); }
  });
  loadSetupJobProgress(job.id, id);
  paintSetupTimeline();
  paintSetupProgress(id);
}

async function loadSetupJobProgress(jobID, stepID) {
  const now = Date.now();
  if (now - (setupJobRead[jobID] || 0) < 400) return;
  setupJobRead[jobID] = now;
  let job;
  try { job = await api("/api/jobs/" + encodeURIComponent(jobID)); } catch { return; }
  const current = setupProgress[stepID];
  if (!job || !current || current.jobID !== jobID || current.finished) return;
  setupApplyJob(current, job);
  paintSetupProgress(stepID);
}

function setupJobStart(job) {
  return job && Date.parse(job.started_at) || 0;
}

function setupApplyJob(state, job) {
  state.started = setupJobStart(job) || state.started || 0;
  state.unavailable = false;
  if (job.state === "done" || job.state === "failed") {
    Object.assign(state, { running: false, finished: true, failed: job.state === "failed",
      ended: Date.parse(job.finished_at) || Date.now(), error: job.error || "", detail: "" });
    state.phase = state.failed ? (/stopped/i.test(state.error) ? "Stopped before finishing." : "This step did not finish.") : "Completed.";
    delete state.pct;
  } else if (!state.finished) {
    state.running = true;
    setupApplyJobProgress(state, job.progress);
  }
  setupLastOutput(state, job.lines || []);
}

function setupApplyJobProgress(state, p) {
  if (!p || state.finished || state.failed) return;
  const nextStep = p.step || p.phase;
  if (state.stepID !== nextStep) { delete state.pct; state.detail = ""; }
  state.stepID = nextStep;
  const pct = ["serve", "verify"].includes(nextStep) ? null : setupProgressPercent(p);
  if (pct !== null) { state.pct = pct; state.detail = setupProgressDetail(p, state); }
  else { delete state.pct; state.detail = ""; }
  if (p.phase && !(nextStep === "python-env" && state.markerStep)) state.phase = setupProgressSentence(p);
}

/** Only measured byte/file totals produce a percentage. */
function setupProgressPercent(p) {
  let done = 0;
  let total = 0;
  if (p.bytes_total) {
    done = p.bytes_done || 0;
    total = p.bytes_total;
  } else if (p.files_total) {
    done = p.files_done || 0;
    total = p.files_total;
  }
  if (!total) return null;
  return Math.max(0, Math.min(100, Math.round((done * 100) / total)));
}

function setupProgressSentence(p) {
  const id = p.step || p.phase;
  const record = id === "venv.verify" ? setupState.verifyRun : setupState.run;
  const step = ((record && record.steps) || []).find((s) => s.id === id)
    || ((setupState.doc && setupState.doc.steps) || []).find((s) => s.id === id);
  if (step) return step.title || "Working on setup";
  return p.phase && !/^[a-z0-9.-]+$/.test(p.phase) ? p.phase : "Working on setup";
}

function setupProgressDetail(p, state) {
  let text = state.pct + "%";
  if (p.bytes_total) text += " · " + bytes(p.bytes_done || 0) + " of " + bytes(p.bytes_total);
  else if (p.files_total) text += " · " + (p.files_done || 0) + " of " + p.files_total + " files";
  if (p.rate_bps > 0) text += " · " + setupRate(p.rate_bps);
  if (p.eta_s > 0) text += " · last estimate: " + setupDuration(p.eta_s) + " remaining";
  return text;
}

function setupElapsed(state) {
  if (!state || !state.started) return 0;
  return Math.max(0, Math.round(((state.ended || Date.now()) - state.started) / 1000));
}

function setupElapsedText(state) { return setupDuration(setupElapsed(state)); }

function setupLiveDetail(state) {
  const elapsed = state.started ? "Elapsed " + setupElapsedText(state) : "Elapsed time unavailable";
  const measured = typeof state.pct === "number" && !state.finished && !state.unavailable;
  return (state.unavailable ? "Live updates unavailable · " : "") + (measured && state.detail ? state.detail + " · " : "") + elapsed
    + (!measured && state.running ? " · No completion estimate" : "")
    + (state.outputAt && state.running ? " · last output " + setupDuration((Date.now() - state.outputAt) / 1000) + " ago" : "");
}

function setupRate(bps) {
  if (bps >= 1000 * 1000) return (bps / (1000 * 1000)).toFixed(1) + " MB/s";
  if (bps >= 1000) return (bps / 1000).toFixed(1) + " kB/s";
  return bps + " B/s";
}

function setupDuration(seconds) {
  const s = Math.max(0, Math.round(seconds));
  if (s < 60) return s + " s";
  if (s < 3600) return Math.floor(s / 60) + " min " + s % 60 + " s";
  return Math.floor(s / 3600) + " h " + Math.floor(s % 3600 / 60) + " min";
}

// Reuse the existing elapsed-time tick; no network polling.
let setupTicker = null;
function setupStartTicker() {
  if (setupTicker) return;
  setupTicker = setInterval(setupTick, 1000);
}
function setupStopTicker() {
  if (!setupTicker) return;
  clearInterval(setupTicker);
  setupTicker = null;
}
function setupTick() {
  let any = false;
  for (const [id, state] of Object.entries(setupProgress)) {
    if (state && state.running) { paintSetupProgress(id); any = true; }
  }
  if (typeof setupPaintRunRows === "function") setupPaintRunRows();
  if (!any) setupStopTicker();
}
if (typeof window !== "undefined") {
  window.addEventListener("pagehide", setupStopTicker);
  window.addEventListener("beforeunload", setupStopTicker);
}

async function attachSetupJobs(steps) {
  for (const step of steps || []) if (step.job_id) await attachSetupJob(step);
}

async function attachSetupJob(step, options = { onRefresh: setupState.onRefresh }) {
  const id = step.id;
  if (setupStreams[id] && setupProgress[id] && setupProgress[id].jobID === step.job_id) return;
  let job;
  try { job = await api("/api/jobs/" + encodeURIComponent(step.job_id)); } catch (e) {
    setupProgress[id] = { jobID: step.job_id, phase: "Live job status is unavailable.", error: String(e.message || e), running: step.job_state === "running", unavailable: true };
    paintSetupProgress(id);
    return;
  }
  if (!job) return;
  const previous = setupProgress[id];
  const state = previous && previous.jobID === job.id ? previous : { jobID: job.id, phase: step.title || "Setup", logTail: job.state !== "running" };
  setupApplyJob(state, job);
  setupProgress[id] = state;
  if (job.state === "running") { setupLogs[id] = []; followSetupJob(step, job, options); return; }
  if (!(setupLogs[id] || []).length) setupLogs[id] = Array.isArray(job.lines) ? job.lines.slice() : [];
  paintSetupTimeline();
  paintSetupProgress(id);
}

function paintSetupProgress(id) {
  if (id === "path" || id === "verify") {
    if (typeof setupPaintPath === "function") setupPaintPath();
    return;
  }
  const box = document.querySelector('[data-progress="' + id + '"]');
  if (!box) return;
  const state = setupProgress[id] || {};
  const sentence = box.querySelector(".step-progress-sentence");
  const measured = typeof state.pct === "number" && !state.finished && !state.unavailable;
  const bar = box.querySelector(".step-bar");
  if (!sentence || measured !== !!bar) { paintSetupTimeline(); return; }
  sentence.textContent = state.phase || "";
  if (bar) {
    bar.setAttribute("aria-valuenow", String(state.pct));
    const fill = bar.querySelector("i");
    if (fill) fill.style.width = state.pct + "%";
  }
  const detail = box.querySelector(".step-progress-detail");
  if (detail) detail.textContent = setupLiveDetail(state);
  const last = box.querySelector(".step-log-line");
  if (last) last.textContent = state.logLine ? "Latest output: " + state.logLine : "";
  const error = box.querySelector(".step-error");
  if (error) error.textContent = state.error || "";
  setupPaintLog(box.querySelector(".step-log"), setupLogs[id] || []);
}

function setupPaintLog(log, lines) {
  if (!log) return;
  while (log.childElementCount > lines.length) log.lastElementChild.remove();
  for (let i = 0; i < lines.length; i++) {
    if (!log.children[i]) log.append(h("div", ""));
    if (log.children[i].textContent !== lines[i]) log.children[i].textContent = lines[i];
  }
}

function setupLastOutput(state, lines) {
  for (let i = lines.length - 1; i >= 0; i--) {
    let text = String(lines[i] || "").trim();
    const stamp = text.match(/^\[([^\]]+)\]\s*/);
    const time = stamp ? Date.parse(stamp[1]) : 0;
    if (time) text = text.slice(stamp[0].length);
    if (!text || /^(\$ |== |# )/.test(text)) continue;
    const preview = text.length > 180 ? text.slice(0, 179) + "…" : text;
    if (time) state.outputAt = time;
    else if (state.logLine !== preview) state.outputAt = 0;
    state.logLine = preview;
    return;
  }
}

async function setupStopJob(id) {
  const state = setupProgress[id];
  if (!state || !state.jobID || state.stopping) return;
  state.stopping = true;
  paintSetupProgress(id);
  try {
    const result = await api("/api/jobs/" + encodeURIComponent(state.jobID) + "/stop", { method: "POST", body: {} });
    if (result.stopped) state.phase = "Stop requested; waiting for the job to end…";
    else toast("This job could not be stopped here. Refresh its status before retrying.");
  } catch (e) { toast("The job could not be stopped: " + (e.message || e)); }
  state.stopping = false;
  delete setupJobRead[state.jobID];
  loadSetupJobProgress(state.jobID, id);
  paintSetupProgress(id);
}

/** Standalone verification must never replace the installation record. */
const setupRefreshRun = (() => {
  let lastRead = 0;
  let sequence = 0;
  return async function setupRefreshRun(id, force = false) {
    const now = Date.now();
    if (!force && now - lastRead < 900) return;
    lastRead = now;
    const ticket = ++sequence;
    let doc;
    try { doc = await api("/api/setup/runs"); } catch (e) {
      if (ticket !== sequence) return;
      setupState.runError = "Setup history is unavailable. Refresh status to try again.";
      setupEnsureModule("views/setup/path.js").then(() => setupPaintPath()).catch((error) => toast(String(error.message || error)));
      return;
    }
    if (ticket !== sequence) return;
    setupState.runError = "";
    const runs = Array.isArray(doc.runs) ? doc.runs : doc.run ? [doc.run] : [];
    const check = (r) => r.steps && r.steps.length === 1 && r.steps[0].id === "venv.verify";
    setupState.run = runs.find((r) => r.steps && r.steps.length && !check(r)) || null;
    setupState.verifyRun = runs.find(check) || null;
    const checkRun = setupState.verifyRun;
    if (checkRun && checkRun.state === "running" && !setupStreams.verify && setupState.verifyAttachID !== checkRun.id) {
      setupState.verifyAttachID = checkRun.id;
      let jobs;
      try { jobs = await api("/api/jobs?limit=10"); } catch { jobs = []; }
      if (ticket !== sequence) return;
      const job = jobs.find((j) => j.step === "venv.verify" && setupJobStart(j) <= Date.parse(checkRun.started_at)
        && (!j.finished_at || Date.parse(j.finished_at) >= Date.parse(checkRun.started_at)));
      if (job) {
        setupProgress.verify = { jobID: job.id, runID: checkRun.id, phase: "Checking the Python environment" };
        setupLogs.verify = job.state === "running" ? [] : job.lines || [];
        setupApplyJob(setupProgress.verify, job);
        if (job.state === "running") followSetupJob({ id: "verify" }, job, { onRefresh: () => loadSetupDocument() });
      }
    }
    const live = setupProgress[id];
    const run = id === "verify" ? setupState.verifyRun : setupState.run;
    const steps = run && run.steps || [];
    const step = steps.find((s) => s.state === "running") || steps.find((s) => s.state === "failed") || steps[steps.length - 1];
    const start = run && Date.parse(run.started_at);
    const linked = live && start && live.started && start >= live.started - 1000 && start <= (live.ended || Date.now()) + 1000;
    if (linked) {
      live.runID = run.id;
      if (live.running && step && (id !== "path" || !["serve", "verify"].includes(live.stepID))) live.phase = step.title || live.phase;
    }
    if (step && step.log_path && (!live || linked)) {
      let log;
      try { log = await api("/api/setup/runs/" + encodeURIComponent(run.id) + "/log/" + encodeURIComponent(step.id)); } catch { log = null; }
      if (ticket !== sequence) return;
      if (log) {
        setupLastOutput(log, log.lines || []);
        if (id !== "verify") setupState.runLog = log;
        if (linked) setupLastOutput(live, log.lines || []);
      }
    }
    if (setupState.run || setupState.verifyRun || setupProgress.path) setupEnsureModule("views/setup/path.js")
      .then(setupPaintPath)
      .catch((e) => toast("Setup details could not load: " + (e.message || e)));
    if (id && id !== "path" && id !== "verify") paintSetupProgress(id);
  };
})();

/** Close streams when leaving/re-entering Setup; jobs continue on the host. */
function setupStopStreams() {
  for (const id of Object.keys(setupStreams)) {
    const stream = setupStreams[id];
    if (stream) stream.close();
  }
  for (const id of Object.keys(setupStreams)) delete setupStreams[id];
}
