// Shared requests, telemetry, frame dispatch and polling.
"use strict";

const HISTORY = 60; // seconds of sparkline history
const RUN_POLL_MS = 5000; // how often the run list is read for the pill
const MAX_POLL_MS = 60000; // the longest wait after a failed read
const STALE_MS = 30000; // after this the numbers are old

const state = {
  hw: null,          // /api/hw
  tele: null,        // the last telemetry event
  livePaused: false,
  pausedTele: null, // displayed snapshot while metric refresh is paused
  pauseStarted: 0,
  hist: {},          // key -> number[] (the last HISTORY samples)
  tab: "setup",      // the tab that is open
  power: null,       // /api/power
  runs: [],          // /api/runs
  lastUpdate: 0,     // when the numbers were read (ms)
  stale: false,      // the last read did not answer
  slowAgeS: 0,       // the age of the slow fields of the telemetry
  slowIntervalS: 0,  // the time between two slow passes of the sampler
};

/** request performs one JSON request and returns its status and document.
 *  api below turns a failed request into an Error. */
async function request(path, opts = {}) {
  const init = { method: opts.method || "GET", headers: {}, credentials: "same-origin" };
  if (init.method !== "GET") init.headers["X-Bernie"] = "1";
  if (opts.body !== undefined) {
    init.headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(opts.body);
  }
  const response = await fetch(path, init);
  const text = await response.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = { error: text }; }
  return { status: response.status, ok: response.ok, data, response, text };
}

/** api fetches JSON; a mutating method carries the X-Bernie header (the CSRF
 *  guard of spec §11). */
async function api(path, opts = {}) {
  const result = await request(path, opts);
  if (!result.ok) {
    const err = new Error((result.data && result.data.error) || result.response.status + " " + result.response.statusText);
    // The card that shows the error names the endpoint and the raw answer.
    err.path = path;
    err.status = result.status;
    err.raw = result.text;
    throw err;
  }
  return result.data;
}

/* ---------- the host ---------- */

/** loadHW reads the host once and keeps it. The rail shows the name of it. */
async function loadHW(force) {
  if (!state.hw || force) {
    state.hw = await api("/api/hw");
    const name = $("#hostname");
    if (name) name.textContent = (state.hw.host && state.hw.host.hostname) || "";
    // The environment can limit the GPUs bernie uses (spec §7.7): say so.
    const note = $("#gpunote");
    if (note) note.textContent = state.hw.restriction ? "· " + state.hw.restriction : "";
  }
  return state.hw;
}

function runsOf(data) { return Array.isArray(data) ? data : (data && data.runs) || []; }
function runURL(r) { return "http://" + (r.host || "127.0.0.1") + ":" + r.port; }

/** refreshHost drops every cached answer of the host: hw, topo, doctor,
 *  onboarding, power, runs. The next reader of one of those endpoints
 *  re-probes the host. The Re-check buttons of the Setup, Topology,
 *  Advanced and Logs tabs call this before they reload, so a cable that
 *  changed or a check that just resolved shows up without a page reload
 *  (spec §11, ia.md R27). The function returns the request promise so the
 *  caller can chain the reload after the cache is dropped. */
async function refreshHost() {
  return request("/api/refresh", { method: "POST" });
}

/* ---------- the history of the live numbers ---------- */

function pushHist(key, v) {
  const a = state.hist[key] || (state.hist[key] = []);
  // A value that was not measured stays a gap: the chart breaks the line
  // there instead of drawing a zero.
  a.push(v == null ? null : v);
  if (a.length > HISTORY) a.shift();
  return a;
}

/** slowAgeNote says how old the slow fields are when they are older than
 *  twice their own interval. The fast fields (the load, the memory, the
 *  power) are one second old at most. It is empty while the slow fields
 *  are fresh. */
function slowAgeNote() {
  const age = state.slowAgeS || 0;
  const interval = state.slowIntervalS || 0;
  if (!interval || age <= 2 * interval) return "";
  return "The temperatures and the clocks are " + Math.round(age) + " s old.";
}

/* ---------- the streams ---------- */

/** openTelemetry follows the live sample of the host. It is the one stream
 *  that drives every tile, every ring and every sparkline of the page. */
function openTelemetry(handlers = {}) {
  const es = new EventSource("/api/telemetry");
  es.onopen = () => handlers.open && handlers.open();
  es.onerror = () => handlers.error && handlers.error();
  es.onmessage = (ev) => {
    let doc = null;
    try { doc = JSON.parse(ev.data); } catch { return; }
    handlers.message && handlers.message(doc, ev);
  };
  return es;
}

/** openRunLogs follows the log of one run. */
function openRunLogs(name, handlers = {}) {
  const es = new EventSource("/api/runs/" + encodeURIComponent(name) + "/logs");
  es.onopen = () => handlers.open && handlers.open();
  es.onerror = () => handlers.disconnect && handlers.disconnect();
  es.addEventListener("end", (ev) => handlers.end && handlers.end(ev));
  es.addEventListener("error", (ev) => handlers.error && handlers.error(ev));
  es.onmessage = (ev) => handlers.message && handlers.message(ev);
  return es;
}

/** openJobEvents follows the output of one onboarding job. */
function openJobEvents(id, handlers = {}) {
  const es = new EventSource("/api/jobs/" + encodeURIComponent(id) + "/events");
  es.addEventListener("line", (ev) => handlers.line && handlers.line(ev));
  es.addEventListener("done", (ev) => handlers.done && handlers.done(ev));
  es.addEventListener("failed", (ev) => handlers.failed && handlers.failed(ev));
  return es;
}

/** openBenchEvents follows the progress of one benchmark job: the phase, the
 *  log lines, and the end of the run. */
function openBenchEvents(id, handlers = {}) {
  const es = new EventSource("/api/bench/" + encodeURIComponent(id) + "/events");
  es.onopen = () => handlers.open && handlers.open();
  es.onerror = () => handlers.error && handlers.error();
  es.addEventListener("progress", (ev) => handlers.progress && handlers.progress(ev));
  es.addEventListener("log", (ev) => handlers.log && handlers.log(ev));
  es.addEventListener("done", (ev) => handlers.done && handlers.done(ev));
  es.addEventListener("failed", (ev) => handlers.failed && handlers.failed(ev));
  es.onmessage = (ev) => handlers.message && handlers.message(ev);
  return es;
}

/* ---------- the jobs of the server ---------- */

// JOB_POLL_MS is how often the document of a job is read while its stream
// is quiet. The stream carries the progress; the document is the second
// pair of eyes.
const JOB_POLL_MS = 2000;

/** watchBenchJob follows one benchmark job: the phase, the log lines and
 *  the end of the run. The stream carries the progress; the document of the
 *  job is the second pair of eyes when the stream is quiet. It returns the
 *  function that stops the watch. */
function watchBenchJob(id, handlers = {}) {
  let stopped = false;
  let misses = 0;
  let stream = null;
  const stop = () => {
    stopped = true;
    clearInterval(timer);
    if (stream) stream.close();
  };
  /** parse reads the document of an event of the stream. */
  const parse = (ev) => {
    try { return JSON.parse(ev.data); } catch (e) { return null; }
  };
  const timer = setInterval(async () => {
    if (stopped) return;
    try {
      const job = await api("/api/bench/" + encodeURIComponent(id));
      if (stopped) return;
      misses = 0;
      handlers.progress && handlers.progress(job);
      if (job && (job.status === "done" || job.status === "failed")) {
        stop();
        if (job.status === "failed") {
          handlers.failed && handlers.failed(new Error(job.error || job.phase || "The job stopped."));
        } else {
          handlers.done && handlers.done(job);
        }
      }
    } catch (e) {
      misses += 1;
      if (misses >= 3) {
        stop();
        handlers.failed && handlers.failed(e);
      }
    }
  }, JOB_POLL_MS);
  stream = openBenchEvents(id, {
    progress: (ev) => {
      if (stopped) return;
      const doc = parse(ev);
      if (doc) handlers.progress && handlers.progress(doc);
    },
    log: (ev) => { if (!stopped && handlers.log) handlers.log(ev.data); },
    done: (ev) => {
      stop();
      handlers.done && handlers.done(parse(ev) || {});
    },
    failed: (ev) => {
      stop();
      const doc = parse(ev);
      handlers.failed && handlers.failed(new Error((doc && (doc.error || doc.phase)) || "The job stopped."));
    },
  });
  return stop;
}

/** apiText reads a text answer: the markdown receipt of a comparison is not
 *  JSON. */
async function apiText(path) {
  const res = await fetch(path, { credentials: "same-origin" });
  const text = await res.text();
  if (!res.ok) throw new Error(text || res.status + " " + res.statusText);
  return text;
}

/* ---------- one frame for every repaint ---------- */

const frameJobs = new Set();
let frameHandle = 0;
let frameFallback = 0;

// FRAME_FALLBACK_MS is how long the batch waits for a frame before it runs
// itself. A page that is not painted never runs its animation frames.
const FRAME_FALLBACK_MS = 250;

/** runFrameJobs runs the batch of the repaints of one sample. One broken
 *  card must not stop the page. */
function runFrameJobs() {
  const jobs = Array.from(frameJobs);
  frameJobs.clear();
  for (const job of jobs) {
    try { job(); } catch (e) { /* one broken card must not stop the page */ }
  }
}

// Batch paint jobs; views do not own animation frames.
function onFrame(fn) {
  frameJobs.add(fn);
  if (frameHandle) return;
  frameHandle = requestAnimationFrame(() => {
    frameHandle = 0;
    if (frameFallback) { clearTimeout(frameFallback); frameFallback = 0; }
    runFrameJobs();
  });
  // Background pages may not paint; flush the same batch after a short delay.
  frameFallback = setTimeout(() => {
    frameFallback = 0;
    if (frameHandle) { cancelAnimationFrame(frameHandle); frameHandle = 0; }
    runFrameJobs();
  }, FRAME_FALLBACK_MS);
}

/* ---------- one stream, one repaint per tab ---------- */

const frameSubs = new Map();   // tab name -> the callbacks of that tab

// Subscribe while this tab is active; return an unsubscribe function.
function onTabFrame(tab, fn) {
  let set = frameSubs.get(tab);
  if (!set) { set = new Set(); frameSubs.set(tab, set); }
  set.add(fn);
  return () => set.delete(fn);
}

/* ---------- the one poller of the page ---------- */

let telemetryStream = null;
let pollTimer = null;
let pollDelay = RUN_POLL_MS;
let pollCycle = 0;

function presentTelemetry() {
  if (state.tab === "dashboard" && typeof renderGPUs === "function") renderGPUs();
  if (state.livePaused) return;
  const subs = frameSubs.get(state.tab);
  if (subs && state.tele) for (const fn of subs) fn(state.tele);
}

function setLivePaused(paused) {
  if (state.livePaused === paused) return;
  if (paused) presentTelemetry();
  state.livePaused = paused;
  state.pausedTele = paused ? state.tele : null;
  state.pauseStarted = paused ? Date.now() : 0;
  const button = $("#liveToggle"), note = $("#livePauseNote");
  button.textContent = paused ? "Resume live values" : "Pause live values";
  button.setAttribute("aria-pressed", String(paused));
  note.hidden = !paused;
  note.textContent = "Live metric refresh paused" + (state.pausedTele && state.pausedTele.time
    ? " at " + new Date(state.pausedTele.time).toLocaleTimeString() : " before the first reading")
    + ". Serving continues; server health and controls stay live. Resume for current values.";
  if (!paused) onFrame(presentTelemetry);
}

/** startTelemetry opens the live stream and keeps the history of the page. */
function startTelemetry() {
  if (telemetryStream) return telemetryStream;
  telemetryStream = openTelemetry({
    message: (t) => {
      state.tele = t;
      state.lastUpdate = Date.now();
      state.stale = false;
      // The slow fields (the temperatures, the caps, the clocks) are read
      // every few seconds: the page says how old they are when they get
      // older than twice their own interval.
      state.slowAgeS = t.slow_age_s || 0;
      state.slowIntervalS = t.slow_interval_s || 0;
      const gs = t.gpus || [];
      let pw = 0, busy = 0, vram = 0, hot = 0, sclk = 0;
      for (const s of gs) {
        pushHist("busy" + s.hip_index, s.busy_pct);
        pushHist("pow" + s.hip_index, s.power_w);
        pw += s.power_w || 0;
        busy += s.busy_pct || 0;
        vram += s.vram_used || 0;
        if ((s.temp_c || 0) > hot) hot = s.temp_c || 0;
        if ((s.sclk_mhz || 0) > sclk) sclk = s.sclk_mhz || 0;
      }
      if (gs.length) {
        pushHist("busy_avg", busy / gs.length);
        pushHist("power_total", pw);
        pushHist("vram_used", vram / GiB);
        pushHist("temp_max", hot);
        pushHist("sclk_max", sclk);
      }
      for (const r of t.runs || []) pushHist("gen:" + r.name, r.gen_tok_s);
      // A hidden tab paints nothing: the sample is kept, the repaint waits.
      if (document.hidden) return;
      onFrame(presentTelemetry);
    },
  });
  return telemetryStream;
}

/** pollRuns reads the run list. The pill in the rail answers with it. */
async function pollRuns() {
  try {
    const runs = runsOf(await api("/api/runs"));
    state.runs = runs;
    state.lastUpdate = Date.now();
    state.stale = false;
    pollDelay = RUN_POLL_MS;
    setRunPill(runs);
  } catch (e) {
    // The pill keeps its last word, and the poll waits longer before it
    // tries again: the server may be down.
    state.stale = true;
    pollDelay = Math.min(pollDelay * 2, MAX_POLL_MS);
    setRunPill(state.runs);
  }
}

/** pollCounts reads the checklist and the doctor. The rail shows how many
 *  steps and how many checks wait for the reader. */
async function pollCounts() {
  try {
    const doc = await api("/api/onboarding");
    const steps = doc.steps || [];
    setTabCount("setup", steps.filter((s) => s.state === "failed" || s.state === "blocked").length);
  } catch (e) { /* the badge keeps its last number */ }
  try {
    const rep = await api("/api/doctor");
    const sum = rep.summary || {};
    // The P2P readiness checks live on the Topology tab (spec §4.5): the
    // doctor runs the same checklist under the category "topo.p2p", so the
    // badge of that tab needs no extra request.
    const p2p = (rep.checks || []).filter((c) => c.category === "topo.p2p" && c.status === "fail").length;
    setTabCount("topology", p2p);
    setTabCount("dashboard", sum.fail || 0);
  } catch (e) { /* the badge keeps its last number */ }
}

/** schedulePoll arms the next read. The delay grows when the server does not
 *  answer and shrinks back to the normal one when it does. */
function schedulePoll() {
  clearTimeout(pollTimer);
  pollTimer = null;
  if (document.hidden) return;
  pollTimer = setTimeout(async () => {
    await pollRuns();
    // The checklist and the doctor are read every third round: they change
    // when the reader acts, not every second.
    if (++pollCycle % 3 === 0) pollCounts();
    schedulePoll();
  }, pollDelay);
}

/** startPolling opens the one timer of the page. */
function startPolling() {
  if (pollTimer) return;
  pollRuns();
  schedulePoll();
  document.addEventListener("visibilitychange", () => {
    if (document.hidden) {
      // A hidden tab asks nothing: the timer is dropped and the page is not
      // painted until the reader comes back.
      clearTimeout(pollTimer);
      pollTimer = null;
      return;
    }
    // Back in the foreground: read once, at once, then carry on.
    pollRuns();
    schedulePoll();
  });
}

