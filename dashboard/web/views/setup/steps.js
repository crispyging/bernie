// The actions of the steps of the checklist: start a step, follow the job
// it starts and show the progress under the step.
// Adapted from vllm-rdna-dash by CrispyGinger (Apache-2.0).
"use strict";

// The maps of the live step state live in setup.js so the first paint
// carries nothing. This module reads and writes them; it is loaded
// lazily, when the reader presses a step or the page re-attaches a job.
// setupProgress, setupLogs, setupStreams and setupJobRead are declared
// in setup.js and shared across this file.

/**
 * runSetupStep starts one step of the checklist. The server answers with a
 * job; the job stream carries the output and the end of the work.
 */
async function runSetupStep(step, options = {}) {
  const id = step.id;
  setupProgress[id] = { phase: "Starting…", started: Date.now() };
  setupLogs[id] = [];
  setupStartTicker();
  paintSetupProgress(id);
  let job = null;
  try {
    job = await api("/api/onboarding/" + encodeURIComponent(id) + "/run", { method: "POST", body: {} });
  } catch (e) {
    setupProgress[id] = { phase: "Failed: " + (e.message || e), started: Date.now(), failed: true };
    paintSetupProgress(id);
    return;
  }
  if (!job || job.root_required) {
    setupProgress[id] = { phase: "This step needs root. Run it in a terminal, then re-check.", started: Date.now(), failed: true };
    paintSetupProgress(id);
    return;
  }
  followSetupJob(step, job, options);
}

/** followSetupJob follows the output of a job and refreshes the checklist
 *  when the job ends. */
function followSetupJob(step, job, options) {
  const id = step.id;
  const stop = () => {
    const stream = setupStreams[id];
    if (stream) {
      stream.close();
      delete setupStreams[id];
    }
  };
  const finish = (name, raw) => {
    stop();
    let doc = null;
    try { doc = typeof raw === "string" ? JSON.parse(raw) : raw; } catch (e) { doc = null; }
    const previous = setupProgress[id] || {};
    if (name === "done") {
      previous.phase = "Done.";
      previous.pct = 100;
      previous.failed = false;
    } else {
      previous.phase = "Failed: " + ((doc && doc.error) || "the step did not finish.");
      previous.failed = true;
      previous.detail = "";
      if (doc && Array.isArray(doc.lines) && doc.lines.length) setupLogs[id] = doc.lines.slice();
    }
    setupProgress[id] = previous;
    paintSetupProgress(id);
    if (options.onRefresh) options.onRefresh();
  };
  const stream = openJobEvents(job.id, {
    line: (ev) => {
      const text = ev.data || "";
      if (!setupLogs[id]) setupLogs[id] = [];
      setupLogs[id].push(text);
      const current = setupProgress[id] || (setupProgress[id] = {});
      // The progress sentence of a download is its own line (R20): it
      // goes under the sentence, it does not replace the phase.
      if (text.indexOf("model.download: ") === 0 && text.indexOf("·") > 0) {
        current.detail = text.slice("model.download: ".length).trim();
      } else {
        current.phase = text;
        current.detail = setupLiveDetail(current);
      }
      paintSetupProgress(id);
      loadSetupJobProgress(job.id, id);
      // The setup steps have a record of their own: the page reads it
      // instead of a second source (spec §6.4). onRefresh runs on
      // done/failed only: here it re-attaches the stream, which replays
      // every line and loops until the browser runs out of sockets.
      if (id === "python-env" || id === "path") setupRefreshRun(id);
    },
    done: (ev) => finish("done", ev.data),
    failed: (ev) => finish("failed", ev.data),
  });
  setupStreams[id] = stream;
  loadSetupJobProgress(job.id, id);
}

/** loadSetupJobProgress reads the numbers of a running job: the phase, the
 *  fraction and the rate of the work. */
async function loadSetupJobProgress(jobID, stepID) {
  const now = Date.now();
  const last = setupJobRead[jobID] || 0;
  if (now - last < 400) return;
  setupJobRead[jobID] = now;
  let job = null;
  try { job = await api("/api/jobs/" + encodeURIComponent(jobID)); } catch (e) { return; }
  if (!job) return;
  const current = setupProgress[stepID] || (setupProgress[stepID] = {});
  if (!current.started) current.started = setupJobStart(job);
  setupApplyJobProgress(current, job.progress);
  paintSetupProgress(stepID);
}

/** setupJobStart is the moment a job began, in milliseconds. */
function setupJobStart(job) {
  if (!job || !job.started_at) return Date.now();
  const t = Date.parse(job.started_at);
  return isNaN(t) ? Date.now() : t;
}

/** setupApplyJobProgress writes the numbers of a job into the live state.
 *  A step that measures nothing keeps no percent: an unknown fraction is
 *  not 0 %. */
function setupApplyJobProgress(state, p) {
  if (!p) return;
  if (state.failed) return; // the failure carries the message
  const pct = setupProgressPercent(p);
  if (pct !== null) {
    state.pct = pct;
    state.detail = setupProgressDetail(p, state);
  } else {
    state.detail = setupLiveDetail(state);
  }
  if (p.phase && !state.failed) state.phase = p.phase;
}

/** setupProgressPercent is the fraction of the work that is done, in
 *  percent, or null when the step measures nothing. */
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

/** setupProgressSentence names the work in one plain sentence. */
function setupProgressSentence(p) {
  if (p.phase) return p.phase;
  if (p.files_total) {
    return "Downloading the model weights, " + (p.files_done || 0) + " of " + p.files_total + " files.";
  }
  if (p.bytes_total) {
    return "Downloading the model weights, " + bytes(p.bytes_done || 0) + " of " + bytes(p.bytes_total) + ".";
  }
  return "Working…";
}

/** setupProgressDetail is the line under the bar: the percent, the size,
 *  the rate and the time that is left. The sentence comes from the server
 *  (R20): the same numbers the CLI prints and the MCP tool returns. A
 *  server that sends no sentence falls back to the numbers it sent. */
function setupProgressDetail(p, state) {
  if (p.sentence) return p.sentence;
  const pct = typeof state.pct === "number" ? state.pct : 0;
  let text = pct + "%";
  if (p.bytes_total) {
    text += " - " + bytes(p.bytes_done || 0) + " of " + bytes(p.bytes_total);
    if (p.rate_bps) text += " at " + setupRate(p.rate_bps);
  } else if (p.files_total) {
    text += " - " + (p.files_done || 0) + " of " + p.files_total + " files";
  }
  if (p.eta_s) {
    const eta = setupDuration(p.eta_s);
    if (setupElapsed(state) > p.eta_s + 5) text += " - longer than the " + eta + " estimate";
    else text += " - about " + eta + " left";
  }
  return text;
}

/** setupElapsed is how long the work has been going, in seconds. */
function setupElapsed(state) {
  if (!state || !state.started) return 0;
  return Math.max(0, Math.round((Date.now() - state.started) / 1000));
}

/** setupElapsedText is the elapsed time in words. */
function setupElapsedText(state) {
  const s = setupElapsed(state);
  if (s < 60) return s + " s";
  if (s < 3600) return Math.round(s / 60) + " min";
  return Math.round(s / 3600) + " h";
}

/** setupLiveDetail is the line under the sentence. When the work is not
 *  measured it says so and says how long it has been going. */
function setupLiveDetail(state) {
  if (typeof state.pct === "number") return state.detail || "";
  return "working… " + setupElapsedText(state);
}

/** setupRate is a download rate in one unit. */
function setupRate(bps) {
  if (bps >= 1000 * 1000) return (bps / (1000 * 1000)).toFixed(1) + " MB/s";
  if (bps >= 1000) return (bps / 1000).toFixed(1) + " kB/s";
  return bps + " B/s";
}

/** setupDuration is a number of seconds in words. */
function setupDuration(seconds) {
  const s = Math.max(0, Math.round(seconds));
  if (s < 60) return s + " s";
  return Math.round(s / 60) + " min";
}

// setupTicker is the 1 Hz tick that refreshes the elapsed time of a
// running step: a wheel build can go minutes without an event.
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
  for (const id of Object.keys(setupProgress)) {
    const s = setupProgress[id];
    if (s && !s.failed && s.phase && s.phase !== "Done.") {
      paintSetupProgress(id);
      any = true;
    }
  }
  if (typeof setupPaintRunRows === "function") setupPaintRunRows();
  if (!any) setupStopTicker();
}
if (typeof window !== "undefined") {
  window.addEventListener("pagehide", setupStopTicker);
  window.addEventListener("beforeunload", setupStopTicker);
}

/** attachSetupJobs re-opens the event stream of every job the checklist
 *  names, so a reload during a long step shows its progress again. */
async function attachSetupJobs(steps) {
  for (const step of steps || []) {
    if (!step.job_id) continue;
    await attachSetupJob(step);
  }
}

/** attachSetupJob shows the progress of the job of a step and follows it
 *  while it runs. The stream replays the output of the job from its first
 *  line, so the reader sees the whole thing. */
async function attachSetupJob(step) {
  const id = step.id;
  if (setupStreams[id]) return;
  let job = null;
  try { job = await api("/api/jobs/" + encodeURIComponent(step.job_id)); } catch (e) { return; }
  if (!job) return;
  const state = { phase: job.phase || "Working…", started: setupJobStart(job) };
  state.detail = setupLiveDetail(state);
  setupProgress[id] = state;
  if (step.job_state === "running") {
    setupLogs[id] = [];
    followSetupJob(step, job, { onRefresh: setupState.onRefresh });
    return;
  }
  setupLogs[id] = Array.isArray(job.lines) ? job.lines.slice() : [];
  if (step.job_state === "failed") {
    state.phase = "Failed: " + (job.error || "the step did not finish.");
    state.failed = true;
    state.detail = "";
  } else {
    state.pct = 100;
  }
  setupApplyJobProgress(state, job.progress);
  paintSetupTimeline();
}

/** paintSetupProgress writes the live progress into the row of a step. */
function paintSetupProgress(id) {
  const box = document.querySelector('[data-step="' + id + '"] .step-progress');
  if (!box) return;
  const state = setupProgress[id] || {};
  const sentence = box.querySelector(".step-progress-sentence");
  if (!sentence) {
    paintSetupTimeline();
    return;
  }
  sentence.textContent = state.phase || "";
  const measured = typeof state.pct === "number";
  const bar = box.querySelector(".step-bar");
  if (measured !== !!bar) {
    // The bar appears when the work becomes measured and goes when it does
    // not. The row is drawn again.
    paintSetupTimeline();
    return;
  }
  if (bar) {
    bar.setAttribute("aria-valuenow", String(state.pct));
    const fill = bar.querySelector("i");
    if (fill) fill.style.width = state.pct + "%";
  }
  const detail = box.querySelector(".step-progress-detail");
  if (detail) detail.textContent = setupLiveDetail(state);
  const lines = setupLogs[id] || [];
  const log = box.querySelector(".step-log");
  if (lines.length && !log) {
    paintSetupTimeline();
    return;
  }
  if (!log) return;
  while (log.childElementCount > lines.length) log.lastElementChild.remove();
  while (log.childElementCount < lines.length) log.append(h("div", ""));
  lines.forEach((line, i) => {
    if (log.children[i].textContent !== line) log.children[i].textContent = line;
  });
}

/** setupRefreshRun reads the record of the setup run and paints the row it
 *  belongs to. The record is the one source of the progress of the setup:
 *  the CLI, the MCP server and the page read it (spec §6.4). */
async function setupRefreshRun(id) {
  let doc = null;
  try { doc = await api("/api/setup/runs"); } catch (e) { return; }
  const run = (doc && doc.run) || null;
  setupState.run = run;
  if (!run || !run.steps || !run.steps.length) return;
  const steps = run.steps;
  const step = steps.find((s) => s.state === "running") || steps[steps.length - 1];
  const live = id === "path" ? setupProgress.path : (setupProgress[id] || (setupProgress[id] = {}));
  if (!live || live.failed) return;
  live.phase = step.id + ": " + step.state +
    (step.progress && step.progress.sentence ? " (" + step.progress.sentence + ")" : "");
  live.detail = step.duration_s > 0 ? setupDuration(step.duration_s) : "";
  // The verify job has no row in the checklist: its output stays in the
  // log of the step.
  if (id !== "path" && id !== "verify" && step.log_path) {
    let log = null;
    try {
      log = await api("/api/setup/runs/" + encodeURIComponent(run.id) + "/log/" + encodeURIComponent(step.id));
    } catch (e) { log = null; }
    setupLogs[id] = (log && log.lines) || [];
  }
  if (id === "path") setupPaintPath(); else paintSetupProgress(id);
}

/** setupStopStreams closes the job streams of the tab. */
function setupStopStreams() {
  for (const id of Object.keys(setupStreams)) {
    const stream = setupStreams[id];
    if (stream) stream.close();
  }
  for (const id of Object.keys(setupStreams)) delete setupStreams[id];
}
