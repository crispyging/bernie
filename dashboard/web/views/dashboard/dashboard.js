"use strict";
// Dashboard uses the shared telemetry stream; it opens no timer or stream.
// Adapted from vllm-rdna-dash by CrispyGinger (Apache-2.0).

const dash = {
  samples: [],   // the compact telemetry ring buffer (one hour of samples)
  history: null, // the answer of GET /api/gpus/history?window=5m
  historyOK: false,
  hw: null,      // /api/hw
  runs: [],      // /api/runs
  doctor: null,  // /api/doctor
  glossary: {},  // the plain meanings from /api/glossary
  tele: null,    // the last telemetry document
  live: {},      // the parts the stream repaints
  subscribed: false, // the repaint of this tab is registered on the stream
  // the server the page shows when several serve, kept per browser
  pick: (() => { try { return localStorage.getItem("bernie.server") || ""; } catch (e) { return ""; } })(),
  // seconds per point of the window charts (1 = every sample), kept per browser
  group: (() => { try { return Number(localStorage.getItem("bernie.chart-group")) || 1; } catch (e) { return 1; } })(),
};

/** dashGroup averages points taken every interval seconds into buckets of
 *  dash.group seconds, the newest bucket last. A bucket without a reading
 *  stays a gap. */
function dashGroup(values, interval) {
  const step = Math.round(dash.group / (interval || 1));
  if (!(step > 1)) return values;
  const out = [];
  for (let end = values.length; end > 0; end -= step) {
    const got = values.slice(Math.max(0, end - step), end).filter((v) => v != null && isFinite(v));
    out.unshift(got.length ? got.reduce((a, b) => a + b, 0) / got.length : null);
  }
  return out;
}

function dashSection(label, id, ...kids) {
  return h("section.section", { id: "dashboard-" + id, "data-section": id },
    sectionLabel(label),
    ...kids);
}

function dashInfo(text) {
  if (typeof tooltip === "function") return tooltip(text, "What this number means");
  return h("span.tip", h("button", { type: "button", title: text, "aria-label": "What this number means" }, "i"));
}

function dashPush(doc) {
  dash.tele = doc;
  const gpus = doc.gpus || [];
  const runs = doc.runs || [];
  const cards = gpus.map((g) => ({
    hip: g.hip_index, busy: g.busy_pct, pow: g.power_w, mem: g.vram_used,
    pci: (((dash.hw && dash.hw.gpus) || []).find((hw) => hw.hip_index === g.hip_index) || {}).pci || "",
    tmp: g.temp_c, cap: g.power_cap_w, total: g.vram_total,
  }));
  let gen = 0, prompt = 0, known = runs.length > 0;
  // Every live server keeps its own numbers, so the picker shows the
  // history of the server it switches to.
  const aggs = {};
  for (const run of runs) {
    if (!run.stats) known = false;
    gen += run.gen_tok_s || 0;
    prompt += run.prompt_tok_s || 0;
    if (run.state === "running" && run.stats) aggs[run.name] = dashAgg(run);
  }
  dash.samples.push({
    t: doc.time || Date.now(), aggs: aggs, cards: cards,
    fleet: { gen: known ? gen : null, prompt: known ? prompt : null },
  });
  if (dash.samples.length > 3600) dash.samples.shift();
}

/** dashAgg is the sample of one running server. */
function dashAgg(live) {
  const stats = live.stats;
  return {
    gen: live.gen_tok_s, prompt: live.prompt_tok_s,
    running: stats.requests_running, waiting: stats.requests_waiting, kv: stats.kv_cache_usage,
    spec: stats.spec_draft_tokens_total > 0 ? stats.spec_accept_rate * 100 : null,
    statsKnown: true, specOn: !!stats.speculation, prefixKnown: !!stats.prefix_cache,
    ttft: stats.ttft ? stats.ttft.p50 : null, ttftP95: stats.ttft ? stats.ttft.p95 : null,
    prefix: stats.prefix_cache && stats.prefix_cache_queries_total > 0 ? stats.prefix_cache_hit_rate * 100 : null,
    prefixCached: stats.prompt_tokens_cached_total, prefixQueried: stats.prompt_tokens_total,
  };
}

function dashSeries(key, count) {
  const run = dashLiveRun();
  return dash.samples.slice(-(count || 60)).map((s) => {
    const agg = run && run.state !== "exited" && s.aggs && s.aggs[run.name];
    return agg ? agg[key] : null;
  });
}

/** dashPickRun shows the server with this name and keeps the choice. */
function dashPickRun(name) {
  dash.pick = name;
  try { localStorage.setItem("bernie.server", name); } catch (e) { /* the page still works */ }
}

function dashLiveRun() {
  const runs = runsOf(state.runs);
  const latest = state.tele || dash.tele;
  if (latest && Array.isArray(latest.runs) && !(latest.time && Date.now() - latest.time > STALE_MS)) {
    const up = latest.runs.filter((r) => r.state !== "exited");
    const live = up.find((r) => r.name === dash.pick) || up[0];
    if (live) return Object.assign({}, runs.find((r) => r.name === live.name), live);
    const last = runs[runs.length - 1];
    return last ? Object.assign({}, last, { state: "exited" }) : null;
  }
  return runs.find((r) => r.state !== "exited" && r.name === dash.pick) || runs.find((r) => r.state !== "exited") || runs[runs.length - 1] || null;
}

// Health and metrics are independent: missing /metrics does not mean loading.
function dashRunStatus(run) {
  const tele = run && dash.tele && (dash.tele.runs || []).find((r) => r.name === run.name);
  const active = !!run && run.state !== "exited";
  const phase = run && run.phase ? run.phase.id : "";
  const latest = state.tele || dash.tele;
  const unknown = !!(latest && (latest.error || (latest.time && Date.now() - latest.time > STALE_MS)))
    || !!(!latest && dash.errors && dash.errors["/api/runs"]);
  const failed = active && phase === "failed";
  const unresponsive = active && (run.answering === false || run.state === "unresponsive"
    || (run.state === "starting" && (run.foreign || phase === "ready")));
  const ready = !unknown && active && !failed && !unresponsive && run.state === "running";
  const label = unknown ? "Status unavailable" : failed ? "Server failed" : unresponsive ? "Not responding"
    : ready ? "Ready" : active ? "Starting" : run ? "Stopped" : "No model running";
  const message = unknown ? "Live status could not be confirmed. Check the connection or open Logs."
    : failed ? "The server reported a startup failure. Open Logs before trying again."
      : unresponsive ? "The server is recorded, but its health check is not answering. Open Logs to investigate."
        : ready ? (run.name || "The model") + " passed its health check. Try a prompt in Chat or connect an app."
          : active ? dashPhase(run) : run ? "The last model server stopped. Open Logs for details, or start a model."
            : "Start a model on the Setup tab, then return here to test it.";
  return { tele, active, ready, unknown, label, message,
    kind: unknown || unresponsive ? "warn" : failed ? "fail" : ready ? "ok" : active ? "running" : "skip" };
}

function dashPhase(run) {
  if (!run) return "";
  return [run.phase && run.phase.sentence, run.waiting].filter(Boolean).join(" ") || "The server is starting; its health check is not ready yet.";
}

// Read failures stay visible without blocking the other cards.
async function dashLoad() {
  const one = async (path, key) => {
    try { dash[key] = await api(path); if (dash.errors) delete dash.errors[path]; }
    catch (e) { dash[key] = null; dash.errors = dash.errors || {}; dash.errors[path] = e.message || String(e); }
  };
  await Promise.all([
    one("/api/hw", "hw"),
    one("/api/runs", "runs"),
    one("/api/doctor", "doctor"),
  ]);
  try {
    const doc = await api("/api/glossary");
    dash.glossary = {};
    for (const word of (doc && doc.words) || []) dash.glossary[word.term] = word.meaning;
  } catch (e) {
    dash.glossary = {};
    dash.errors = dash.errors || {};
    dash.errors["/api/glossary"] = e.message || String(e);
  }
  if (!Array.isArray(dash.runs)) dash.runs = [];
  else state.runs = dash.runs;
  dashUpdateSimple();
}

function dashUpdateSimple() {
  const api = dash.live.simple;
  if (!api) return;
  api.hero.update();
  api.tiles.update();
  api.charts.update();
  api.gpus.update();
  api.machine.update();
  api.connect.update();
}

async function dashHistory() {
  try {
    dash.history = await api("/api/gpus/history?window=5m");
    dash.historyOK = true;
  } catch (e) {
    dash.history = null;
    dash.historyOK = false;
  }
  if (dash.live.simple) dash.live.simple.charts.update();
}

// History contains host-wide rates, not the rates of the displayed run.
function dashWindowSeries(key) {
  if (dash.historyOK && dash.history && dash.history.series && dash.history.series.length) {
    const field = key === "prompt" ? "prompt_tok_s" : "tokens_per_s";
    return dashGroup(dash.history.series[0][field] || [], dash.history.interval_s);
  }
  // The live samples come about once a second.
  return dashGroup(dash.samples.slice(-300).map((s) => s.fleet ? s.fleet[key] : null), 1);
}

async function viewDashboard(root) {
  put(root, loading());
  dash.live = {};
  dash.errors = {};
  dash.tele = (state.livePaused ? state.pausedTele : state.tele) || null;
  await dashLoad();
  if (dash.tele) dashPush(dash.tele);
  dashBuildSimple(root);
  await dashHistory();
  dashSubscribe();
  renderGPUs();
}

function dashSample(doc) {
  dashPush(doc);
  dashUpdateSimple();
}
