"use strict";
// ---------- Dashboard ----------
// The Dashboard tab asks one question: is this box serving, and how fast?
//
// Every number comes from an endpoint of spec §11. The live numbers come from
// one stream (GET /api/telemetry at 1 Hz) and the charts of the last minutes
// from GET /api/gpus/history. The view never opens a fetch or a timer of its
// own: the stream lives in js/data.js.
//
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
};

// dashSection is one block of the tab: a rule on top, the name of the block
// and what it holds. The name is a heading the router can focus.
function dashSection(label, id, ...kids) {
  return h("section.section", { id: "dashboard-" + id, "data-section": id },
    sectionLabel(label),
    ...kids);
}

// dashInfo is the small mark that opens the explanation of a number. The
// explanation is never the only copy of the fact: the tile says it too.
function dashInfo(text) {
  if (typeof tooltip === "function") return tooltip(text, "What this number means");
  return h("span.tip", h("button", { type: "button", title: text, "aria-label": "What this number means" }, "i"));
}

// dashGroup writes a number with a thin space between the thousands.
function dashGroup(n) {
  const s = String(Math.round(n));
  return s.replace(/\B(?=(\d{3})+(?!\d))/g, " ");
}

// The stream lives in js/data.js; the shell calls renderGPUs() on every
// telemetry message. This view opens no fetch and no timer of its own.

// dashPush keeps one compact sample of the stream and the serving numbers it
// carries. The buffer holds one hour, so every chart window stays covered.
function dashPush(doc) {
  dash.tele = doc;
  const gpus = doc.gpus || [];
  const runs = doc.runs || [];
  const live = runs.find((r) => r.state !== "exited") || runs[0] || null;
  const cards = gpus.map((g) => ({
    hip: g.hip_index, busy: g.busy_pct, pow: g.power_w, mem: g.vram_used,
    tmp: g.temp_c, jtmp: g.temp_junction_c, mtmp: g.temp_mem_c,
    clk: g.sclk_mhz, mclk: g.mclk_mhz, cap: g.power_cap_w, total: g.vram_total,
  }));
  const stats = live && live.stats ? live.stats : null;
  dash.samples.push({
    t: doc.time || Date.now(),
    cards: cards,
    agg: {
      gen: live ? live.gen_tok_s : null,
      prompt: live ? live.prompt_tok_s : null,
      running: stats ? stats.requests_running : null,
      waiting: stats ? stats.requests_waiting : null,
      kv: stats ? stats.kv_cache_usage : null,
      spec: stats && stats.spec_draft_tokens_total > 0 ? stats.spec_accept_rate * 100 : null,
      statsKnown: !!stats,
      // What the server publishes: a server without the counters has no
      // such measure.
      specOn: stats ? !!stats.speculation : null,
      prefixKnown: stats ? !!stats.prefix_cache : null,
      // The measures of the server since it started: the time to the
      // first token and the share of the prompt that came from the cache.
      ttft: stats && stats.ttft ? stats.ttft.p50 : null,
      ttftP95: stats && stats.ttft ? stats.ttft.p95 : null,
      prefix: stats && stats.prefix_cache && stats.prefix_cache_queries_total > 0
        ? stats.prefix_cache_hit_rate * 100 : null,
      prefixCached: stats ? stats.prompt_tokens_cached_total : null,
      prefixQueried: stats ? stats.prompt_tokens_total : null,
    },
  });
  if (dash.samples.length > 3600) dash.samples.shift();
}

// dashSeries is the last minute of one fleet number: the sparkline of a tile.
function dashSeries(key, count) {
  const n = count || 60;
  return dash.samples.slice(-n).map((s) => (s.agg ? s.agg[key] : null));
}

// dashCardSeries is the last minute of one card.
function dashCardSeries(hip, key) {
  return dash.samples.slice(-60).map((s) => {
    const c = s.cards.find((x) => x.hip === hip);
    return c ? c[key] : null;
  });
}

// dashCardName is the name of a card: the one of /api/hw, or its index.
function dashCardName(hip) {
  const g = ((dash.hw && dash.hw.gpus) || []).find((x) => x.hip_index === hip);
  if (!g) return "GPU " + hip;
  return g.name || g.gfx || ("GPU " + hip);
}

// dashLiveRun is the run the tab talks about.
function dashLiveRun() {
  const runs = dash.runs || [];
  return runs.find((r) => r.state !== "exited") || runs[runs.length - 1] || null;
}

// dashPhase is the phase sentence of a run: what the log says it is doing.
function dashPhase(run) {
  if (!run) return "";
  if (run.phase && run.phase.sentence) return run.phase.sentence;
  if (run.state === "starting") return run.waiting || "The server is loading the model.";
  return "";
}

// dashLoad reads the documents the tab needs. One failure does not stop the
// others: the card that failed says so.
async function dashLoad() {
  const one = async (path, key) => {
    try { dash[key] = await api(path); }
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
  dashUpdateSimple();
}

// dashUpdateSimple repaints the operational view.
function dashUpdateSimple() {
  const api = dash.live.simple;
  if (!api) return;
  api.hero.update();
  api.tiles.update();
  api.charts.update();
  api.cards.update();
  api.machine.update();
  api.connect.update();
  if (api.details) api.details.update();
}

// dashHistory reads the five minutes of history that the Simple charts show.
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

// dashWindowSeries is the output or prompt rate over the last five minutes:
// the history of the server when it has one, the ring buffer of the page when
// it does not. A missing sample stays missing: the chart draws a gap.
function dashWindowSeries(key) {
  if (dash.historyOK && dash.history && dash.history.series && dash.history.series.length) {
    const series = dash.history.series;
    const n = series[0].busy_pct ? series[0].busy_pct.length : 0;
    const field = key === "prompt" ? "prompt_tok_s" : "tokens_per_s";
    const out = [];
    for (let i = 0; i < n; i++) {
      const value = series[0][field] && series[0][field][i] != null ? series[0][field][i] : null;
      out.push(value);
    }
    return out;
  }
  return dash.samples.slice(-300).map((s) => (s.agg ? s.agg[key] : null));
}

async function viewDashboard(root) {
  put(root, loading());
  dash.live = {};
  dash.errors = {};
  if (state.tele) dashPush(state.tele);
  await dashLoad();
  dashBuildSimple(root);
  await dashHistory();
  dashSubscribe();
  renderGPUs();
}

// dashSample is the stream: it keeps the sample and repaints the live parts.
function dashSample(doc) {
  dashPush(doc);
  dashUpdateSimple();
}
