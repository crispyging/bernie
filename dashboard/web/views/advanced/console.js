"use strict";
// Operator console: diagnose first, inspect evidence, then make deliberate changes.

const adv = {
  hw: null,       // /api/hw
  doctor: null,   // /api/doctor
  runs: [],       // /api/runs
  jobs: [],       // /api/jobs
  tele: null,     // the last document of the telemetry stream
  server: null,   // the live run of the stream
  head: null,     // the headline of the tab
  live: {},       // the parts the tab repaints
  unsub: null,    // the way back from the shared stream
};

/** viewAdvanced opens the tab. The router calls it with the panel of the
 *  tab. */
async function viewAdvanced(root) {
  put(root, loading());
  adv.live = {};
  const head = stateHead("Host tools",
    h("button.primary", { type: "button", onclick: () => advReload() }, "Re-check the host"));
  adv.head = head;
  const table = gpuValueTable([], [], []);
  const power = advPowerCard(), tools = advToolsCard(), doctor = advDoctorReport();
  const jobs = advJobList(), runs = advRunsTable(), measures = advServerMeasures();
  const prefs = advPrefsCard(), report = advReportCard();
  const section = (id, title, ...kids) => h("section.section", {
    id: "advanced-" + id, "data-section": id,
  }, sectionLabel(title), ...kids);
  const groups = [
    ["Diagnose", [["doctor", "Health checks"], ["jobs", "Background work"]]],
    ["Inspect", [["gpus", "GPU values"], ["run", "Run history"], ["measures", "Server timings"]]],
    ["Change", [["power", "Power limits"], ["tools", "Memory and power tools"], ["prefs", "Preferences"]]],
    ["Share", [["report", "Support report"], ["documents", "Raw data"]]],
  ];
  put(root, head,
    h("nav.card", { "aria-label": "Operator console sections" },
      h("p.small.muted", "Start with health checks. Inspecting data does not change the host; controls and copied commands can."),
      groups.map(([title, links]) => h("div.row", h("b", title + ":"),
        links.map(([id, label]) => h("a", { href: "#advanced/" + id }, label))))),
    section("doctor", "Diagnose · Health checks", doctor.el),
    section("jobs", "Diagnose · Background work", jobs.el),
    section("gpus", "Inspect · GPU values",
      h("details.advanced", h("summary", "GPU measurements and hardware identifiers"),
        h("div.card", cardHead("All GPU values", "/api/hw",
          h("button.small", { type: "button", onclick: () => advCopyCardList() }, "Copy the card list")), table.el))),
    section("run", "Inspect · Run history", runs.el),
    section("measures", "Inspect · Server timings",
      h("details.advanced", h("summary", "Latency, cache and request counters"), measures.el)),
    section("power", "Change · Power limits", power.el),
    section("tools", "Change · Memory and power tools", tools.el),
    section("prefs", "Change · Preferences",
      h("details.advanced", h("summary", "Stored settings and their effective values"), prefs.el)),
    section("report", "Share · Support report", report.el),
    section("documents", "Share · Raw data", advDocuments()));
  adv.live = { table, power, tools, doctor, jobs, runs, measures, prefs, report };
  await advLoad();
  advSubscribe();
}

/** advLoad reads the documents the tab needs. One failure does not stop the
 *  others: the card that failed says so. */
async function advLoad() {
  const one = async (path, key) => {
    try { adv[key] = await api(path); if (adv.errors) delete adv.errors[path]; }
    catch (e) { adv[key] = null; adv.errors = adv.errors || {}; adv.errors[path] = e.message || String(e); }
  };
  await Promise.all([
    one("/api/hw", "hw"),
    one("/api/doctor", "doctor"),
    one("/api/runs", "runs"),
    one("/api/jobs", "jobs"),
  ]);
  if (!Array.isArray(adv.runs)) adv.runs = [];
  if (!Array.isArray(adv.jobs)) adv.jobs = [];
  adv.tele = state.livePaused ? state.pausedTele : state.tele;
  adv.server = adv.tele && ((adv.tele.runs || []).find((r) => r.state !== "exited") || null);
  advPaint();
  await adv.live.power.reload();
  await adv.live.prefs.reload();
  await adv.live.report.reload();
}

/** advReload reads the documents of the host again. The doctor is held for
 *  fifteen seconds by the server; the button drops every cached answer of
 *  the host (POST /api/refresh) before it reloads, so a check that just
 *  resolved or a cable that changed shows up at once (spec §11, ia.md R27). */
async function advReload() {
  adv.errors = {};
  try { await refreshHost(); } catch { /* the cache drop is best-effort */ }
  await advLoad();
  toast("Host re-check finished. Review any unavailable sections below.");
}

/** advPaint repaints every part of the tab with the documents it holds. */
function advPaint() {
  if (!adv.live.table) return;
  advHead(adv.head);
  adv.live.table.update(
    (adv.hw && adv.hw.gpus) || [],
    (adv.tele && adv.tele.gpus) || [],
    (adv.doctor && adv.doctor.checks) || []);
  adv.live.doctor.update(adv.doctor);
  adv.live.tools.update(adv.doctor);
  adv.live.jobs.update(adv.jobs);
  adv.live.runs.update(adv.runs);
  adv.live.measures.update(adv.server);
}

/** Summarize only checks that actually ran; missing data is not a pass. */
function advHead(head) {
  if (!head) return;
  const report = adv.doctor;
  if (!report || !(report.checks || []).length) {
    setHead(head, "Host health is not known yet.",
      "Health checks are unavailable or empty. Re-check the host, or run bernie doctor; do not treat missing results as a pass.");
    return;
  }
  const sum = report.summary || {};
  const fail = sum.fail || 0, warn = sum.warn || 0, skip = sum.skip || 0;
  const title = fail ? fail + (fail === 1 ? " blocking check needs repair." : " blocking checks need repair.")
    : warn ? warn + (warn === 1 ? " warning needs review." : " warnings need review.")
      : "No blocking health check found.";
  const next = fail ? "Start with the failed health checks below, apply the relevant fix, then re-check."
    : warn ? "Review the warnings below before changing settings or starting a demanding run."
      : "Inspect GPU values or run history below; changes are grouped separately.";
  setHead(head, title, next + (skip ? " " + skip + " checks could not run; their results remain unknown." : "")
    + (Object.keys(adv.errors || {}).length ? " Some host data is unavailable; re-check to try again." : ""));
}

/** advSubscribe registers the repaint of this tab on the shared stream. The
 *  stream lives in js/data.js; this view opens no stream of its own.
 *  A hidden tab paints nothing: data.js pauses the poller and skips the
 *  frame. */
function advSubscribe() {
  if (adv.unsub) return adv.unsub;
  if (typeof onTabFrame !== "function") return null;
  adv.unsub = onTabFrame("advanced", (doc) => {
    if (!doc) return;
    adv.tele = doc;
    const runs = doc.runs || [];
    adv.server = runs.find((r) => r.state !== "exited") || runs[0] || null;
    if (!adv.live.table) return;
    adv.live.table.update(
      (adv.hw && adv.hw.gpus) || [],
      doc.gpus || [],
      (adv.doctor && adv.doctor.checks) || []);
    adv.live.measures.update(adv.server);
  });
  return adv.unsub;
}

/** Recorded runs, including ended ones; process details are secondary. */
function advRunsTable() {
  const card = h("div.card");
  const view = {
    el: card,
    update: (runs) => {
      const list = runs || [];
      const error = (adv.errors || {})["/api/runs"];
      if (!list.length) {
        put(card, cardHead("Run history", "/api/runs"),
          emptyState(error ? "Run history could not be read." : "No runs are recorded.",
            error ? "Re-check the host to try again." : "Start a model server in Setup; its history and logs will appear here.",
            error ? "bernie ps" : "bernie serve <recipe>"), error ? h("p.err", error) : null);
        return;
      }
      const rows = list.map((r) => h("tr",
        h("td", r.recipe || r.name || "–"),
        h("td", badge(r.state || "skip")),
        h("td", r.started_at ? ago(r.started_at) : "not recorded"),
        h("td.advanced-only", r.name || "–"),
        h("td.advanced-only", r.mode || "host"),
        h("td.num.advanced-only", r.port ? String(r.port) : "–"),
        h("td.num.advanced-only", r.pid ? String(r.pid) : "–")));
      put(card, cardHead("Run history", "/api/runs", h("span.small.muted", list.length + " runs"),
        h("a.small", { href: "#logs" }, "Read run logs")),
        h("div.scroll", h("table", h("thead", h("tr",
          ["Recipe", "State", "Started", "Run", "Mode", "Port", "PID"].map((label, i) =>
            h("th", { scope: "col", class: i > 2 ? "advanced-only" : null }, label)))), h("tbody", rows))));
    },
  };
  return view;
}

/** Tab-separated GPU values for a support report. */
function advCardListText() {
  const gpus = (adv.hw && adv.hw.gpus) || [];
  const samples = (adv.tele && adv.tele.gpus) || [];
  if (!samples.length && !gpus.length) return "";
  return samples.map((s) => {
    const g = gpus.find((x) => x.hip_index === s.hip_index) || {};
    return ["GPU " + s.hip_index, g.name || g.gfx || "", num(s.busy_pct, 0) + " % busy",
      num(s.temp_c, 0) + " °C", num(s.power_w, 1) + " W",
      (s.vram_used ? bytes(s.vram_used) : "–") + " / " + (s.vram_total ? bytes(s.vram_total) : "–")].join("\t");
  }).join("\n");
}

/** advCopyCardList puts the card list on the clipboard. */
async function advCopyCardList() {
  const text = advCardListText();
  if (!text) {
    toast("There are no cards to copy yet.");
    return;
  }
  const ok = await copyText(text);
  toast(ok ? "The card list is on the clipboard." : "Copy failed: select the text and copy it.", ok);
}

/** Request measurements may be absent even while a run exists. */
function advServerMeasures() {
  const card = h("div.card");
  const lat = (d) => (d ? num(d.p50, 3) + " s p50, " + num(d.p95, 3) + " s p95" : "–");
  const count = (m) => {
    if (!m) return "–";
    const keys = Object.keys(m);
    return keys.length ? keys.map((k) => k + " " + num(m[k], 0)).join(", ") : "0";
  };
  const pair = (a, b) => num(a, 0) + " of " + num(b, 0) + " tokens";
  const view = {
    el: card,
    update: (run) => {
      const st = run && run.stats ? run.stats : null;
      const w = run && run.window ? run.window : null;
      if (!st) {
        put(card, cardHead("Server measurements", "/api/telemetry"),
          emptyState(run ? "This run has not published request measurements." : "No server measurements have arrived.",
            "Open Logs to check the run. A missing measurement does not mean zero latency or an idle server.", "bernie ps"));
        return;
      }
      const rows = [
        ["First token", lat(w && w.ttft), lat(st.ttft)],
        ["Time between tokens", lat(w && w.itl), lat(st.itl)],
        ["Whole request", lat(w && w.e2e), lat(st.e2e)],
        ["Wait in the queue", lat(w && w.queue), lat(st.queue)],
        ["Prompt tokens from the cache", w ? pair(w.prompt_tokens_cached, w.prompt_tokens) : "–",
          st.prefix_cache ? pair(st.prompt_tokens_cached_total, st.prompt_tokens_total) : "–"],
        ["External cache hits", "–",
          st.external_cache ? pair(st.external_prefix_cache_hits_total, st.external_prefix_cache_queries_total) : "–"],
        ["Preemptions", "–", num(st.num_preemptions_total, 0) + " requests"],
        ["Waiting by reason", "–", count(st.waiting_by_reason)],
        ["Finished by reason", "–", count(st.finished_by_reason)],
      ];
      put(card, cardHead("Server measurements", "/api/telemetry"),
        h("p.small.muted", "A dash means not reported, not zero. p50 is the median; p95 is the time 95% of requests finish within."),
        h("div.scroll", h("table",
          tableHead(["Measure", "Last minute", "Since the start"]),
          h("tbody", rows.map((r) => h("tr", h("td", r[0]), h("td", r[1]), h("td", r[2])))))));
    },
  };
  return view;
}
