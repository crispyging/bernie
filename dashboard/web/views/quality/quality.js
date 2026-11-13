"use strict";
// The Quality tab, after BroodBench's compare page: does the server answer
// right, and how do saved runs compare? Pick runs as chips; the page warns
// when they asked different things, puts every model of every run in one
// score table, and gives each a report: the outcome ring, the family ×
// context heatmap, decode and prefill against the context, and the families
// that failed, which open their cases. Every number comes from the saved
// result (spec §10.5); nothing is computed here but the layout.

/** quality is what the tab keeps between two paints. */
const quality = { runs: [], picked: [], full: {} };

/** qPct writes a share as a percent. */
function qPct(x) {
  return x == null ? "–" : Math.round(x * 100) + " %";
}

/** qLabel names a saved run in a chip or a picker. */
function qLabel(r) {
  return ((r.options.models || [])[0] || "model") + " · " + r.options.suite + " · " + new Date(r.time).toLocaleString();
}

/** qSame is what two runs must share to be compared, after BroodBench. */
function qSame(r) {
  const o = r.options;
  return JSON.stringify([r.version, o.suite, o.seed, o.context_sizes, o.temperature, o.repeats, o.max_tokens, o.tracks || []]);
}

/** qScores is the score table: one row per model of every picked run. */
function qScores(runs) {
  const rate = (p, k) => (p && p[k] ? Math.round(p[k]) : "–");
  return h("div.scroll", h("table.bench-table",
    h("thead", h("tr", ["MODEL", "RUN", "PASS / FAIL / ERROR", "SUCCESS", "CRITICAL / LOOPS", "LATENCY", "DECODE tok/s", "PREFILL tok/s", "TOKENS"]
      .map((x, i) => h("th", { class: i > 1 ? "num" : "" }, x)))),
    h("tbody", runs.flatMap((r) => (r.summary || []).map((s) => h("tr",
      h("td", h("b", s.model)), h("td.num.small", r.id),
      h("td.num", s.passed + " / " + s.failed + " / " + s.errors), h("td.num", qPct(s.graded ? s.accuracy : null)),
      h("td.num", s.critical_failures + " / " + s.loops), h("td.num", (s.avg_latency_s || 0).toFixed(2) + " s"),
      h("td.num", rate(s.performance, "decode_tok_s")), h("td.num", rate(s.performance, "prefill_tok_s")),
      h("td.num", s.tokens == null ? "unknown" : s.tokens.toLocaleString())))))));
}

/** qRing is the outcome ring: pass, fail and error over the graded cases. */
function qRing(s) {
  const t = s.graded || 0, a = t ? 100 * s.passed / t : 0, b = t ? 100 * (s.passed + s.failed) / t : 0;
  const ring = h("div.q-ring", { role: "img", "aria-label": s.passed + " passed, " + s.failed + " failed, " + s.errors + " errors of " + t },
    h("span", t ? qPct(s.accuracy) : "–"));
  // The page's CSP refuses a style attribute; the style property is allowed.
  if (t) ring.style.background = "conic-gradient(var(--ok) 0 " + a + "%,var(--fail) " + a + "% " + b + "%,var(--warn) " + b + "% 100%)";
  return ring;
}

/** qHeat is the family × context heatmap: observed passed / total, and a
 *  family never asked at a size says so. A cell opens its cases. */
function qHeat(s, open) {
  const sizes = [...new Set((s.by_family_context || []).map((j) => j.context_size))].sort((a, b) => a - b);
  const fams = [...new Set((s.by_family_context || []).map((j) => j.family))].sort();
  if (!fams.length) return emptyState("No graded case.", "Every case was skipped or the run was stopped.", "bernie bench eval --context auto");
  return h("div.scroll", h("table.bench-table.q-heat",
    h("thead", h("tr", h("th", "FAMILY"), sizes.map((z) => h("th.num", benchCtxName(z))))),
    h("tbody", fams.map((f) => h("tr", h("th", f), sizes.map((z) => {
      const j = s.by_family_context.find((x) => x.family === f && x.context_size === z);
      if (!j) return h("td.muted.small", "not asked");
      const kind = j.errors ? "error" : j.accuracy >= 0.8 ? "good" : j.accuracy >= 0.5 ? "mixed" : "poor";
      const b = h("button.q-cell." + kind, { type: "button", "aria-label": f + " at " + z + " tokens: " + j.passed + " of " + j.total + " passed, " + j.errors + " errors" },
        h("b", j.passed + "/" + j.total), h("small", qPct(j.accuracy) + (j.errors ? " · " + j.errors + " err" : "")));
      b.addEventListener("click", () => open(f, z));
      return h("td", b);
    }))))));
}

/** qSpeed is decode and prefill against the context, drawn by the chart of
 *  the Benchmark tab. */
function qSpeed(s) {
  const pts = (s.speed_by_context || []).filter((p) => p.performance && (p.performance.decode_tok_s || p.performance.prefill_tok_s));
  if (!pts.length) return h("p.small.muted", "Decode and prefill: the server reported no timing and no attributable counters, so there is no speed by context. Unknown is not zero.");
  const grid = { axis: "context (prompt tokens)", xs: pts.map((p) => benchCtxName(p.context_size)), series: [{ name: s.model, cells: pts }] };
  const src = (k) => [...new Set(pts.flatMap((p) => p.performance[k] || []))].join(", ") || "no source";
  return h("div.bench-charts",
    benchChart(grid, { get: (p) => p.performance.decode_tok_s, title: "Decode vs context", unit: "tok/s", digits: 0, note: "Source: " + src("decode_sources") + "." }),
    benchChart(grid, { get: (p) => p.performance.prefill_tok_s, title: "Prefill vs context", unit: "tok/s", digits: 0, note: "Source: " + src("prefill_sources") + ". TTFT is never used as prefill." }));
}

/** qWhy is what the diagnosis of a case says. */
function qWhy(c) {
  const d = c.diagnosis || {};
  const out = [(d.title || c.status) + ": " + (d.summary || c.error || "")];
  for (const k of d.checks || []) {
    if (k.status === "fail") out.push(k.name + ": expected " + JSON.stringify(k.expected) + ", got " + JSON.stringify(k.actual) + ".");
  }
  if (d.first_failure_step) out.push("The first failing tool call is step " + d.first_failure_step + ".");
  return out.concat(d.limitations || []);
}

/** qCase is one case of the table, with its reasons under it. */
function qCase(c) {
  const kind = { pass: "ok", fail: "fail", error: "serious", skipped: "skipped" }[c.status] || "warn";
  const state = h("td", statePill(kind, c.status));
  const row = h("tr", h("td", h("b", c.case_id)), h("td", c.track), h("td.num", String(c.context_size)), state,
    h("td.num", c.status === "skipped" ? "–" : c.latency_s.toFixed(1) + " s"), h("td", (c.diagnosis || {}).title || ""));
  if (c.status === "pass") return [row];
  const more = h("tr.bench-why", { hidden: true }, h("td", { colspan: 6 }, h("ul", qWhy(c).map((w) => h("li", w)))));
  const btn = h("button.bench-why-btn", { type: "button", "aria-expanded": "false" }, "Why");
  btn.addEventListener("click", () => {
    more.hidden = !more.hidden;
    btn.setAttribute("aria-expanded", String(!more.hidden));
  });
  state.append(" ", btn);
  return [row, more];
}

/** qReport is the report of one model of one run. */
function qReport(r, s) {
  const cases = h("details.bench-fallback", h("summary", "The cases"));
  const body = h("div");
  cases.append(body);
  const open = (family, size, failures) => {
    const list = (r.cases || []).filter((c) => c.model === s.model && (!family || c.family === family) &&
      (!size || c.context_size === size) && (!failures || c.status !== "pass"));
    put(body, h("p.small.muted", list.length + " case(s)" + (family ? " of " + family : "") + (size ? " at " + size + " tokens" : "")),
      h("div.scroll", h("table.bench-table", h("tbody", list.map(qCase)))));
    cases.open = true;
    cases.scrollIntoView({ block: "nearest" });
  };
  open();
  cases.open = false;
  const fails = (s.by_family || []).filter((j) => j.failed + j.errors).sort((a, b) => b.failed + b.errors - a.failed - a.errors);
  return h("section.section", sectionLabel(s.model + " · " + r.id),
    h("div.card.bench-card",
      h("div.q-overview", qRing(s), h("div",
        h("p.q-display", "VERIFIED OUTCOMES"),
        h("p", s.passed + " pass · " + s.failed + " fail · " + s.errors + " errors · " + s.graded + " graded, " + s.skipped + " skipped"),
        h("p.small.muted", (s.by_family || []).length + " families. Errors stay in the denominator; a case never asked is not a zero."),
        h("a.small", { href: "/api/eval/" + encodeURIComponent(r.id) + "?format=csv", download: "" }, "Download CSV"))),
      h("h3", "Family × context"), qHeat(s, (f, z) => open(f, z)),
      qSpeed(s),
      h("h3", "Failure focus"),
      fails.length ? h("div.q-fails", fails.map((j) => {
        const b = h("button", { type: "button" }, j.family + " · " + j.failed + " failed · " + j.errors + " errors / " + j.total);
        b.addEventListener("click", () => open(j.family, 0, true));
        return b;
      })) : h("p.small", "No failed or errored case."),
      cases));
}

async function viewQuality(root) {
  const pick = h("select", { id: "q-pick", "aria-label": "Add a run to compare" });
  const clear = h("button", { type: "button" }, "Clear");
  const head = stateHead("Quality", h("div.head-actions", pick, clear));
  const chips = h("div.q-chips"), body = h("div");
  put(root, head, chips, body);
  try {
    quality.runs = (await api("/api/eval")).sort((a, b) => new Date(b.time) - new Date(a.time));
  } catch (e) {
    setHead(head, "The eval results could not be read.", String(e.message || e));
    return;
  }
  if (!quality.runs.length) {
    pick.hidden = clear.hidden = true;
    setHead(head, "Nothing has been graded yet.", "An eval asks the server seeded questions and grades every answer. Run one and its score shows here.");
    put(body, emptyState("No eval yet.", "Run the smoke suite against the server that serves.", "bernie bench eval --context auto"));
    return;
  }
  if (!quality.picked.length) quality.picked = [quality.runs[0].id];
  async function paint() {
    put(pick, h("option", { value: "" }, "Add a run to compare…"),
      quality.runs.filter((r) => !quality.picked.includes(r.id)).map((r) => h("option", { value: r.id }, qLabel(r))));
    const runs = [];
    for (const id of quality.picked) {
      if (!quality.full[id]) {
        try { quality.full[id] = await api("/api/eval/" + encodeURIComponent(id)); } catch (e) { continue; }
      }
      runs.push(quality.full[id]);
    }
    put(chips, runs.map((r) => {
      const c = h("button.q-chip", { type: "button", title: "Remove from the comparison" }, qLabel(r) + " ×");
      c.addEventListener("click", () => { quality.picked = quality.picked.filter((x) => x !== r.id); paint(); });
      return c;
    }));
    if (!runs.length) {
      setHead(head, "No run is picked.", "Add a saved eval above to read its score.");
      put(body, null);
      return;
    }
    const s = (runs[0].summary || [])[0] || {};
    setHead(head, runs.length > 1 ? runs.length + " runs side by side." :
      (s.model || "The model") + " passed " + (s.passed || 0) + " of " + (s.graded || 0) + " graded cases (" + qPct(s.graded ? s.accuracy : null) + ").",
      "Suite " + runs[0].options.suite + " v" + runs[0].version + ", contexts " + (runs[0].options.context_sizes || []).join(", ") +
      ". A heatmap cell and a failing family open their cases; every case that did not pass opens on its cause.");
    const same = runs.every((r) => qSame(r) === qSame(runs[0]));
    put(body, same ? null : h("p.err", "These runs asked different things (suite, version, seed, contexts, temperature, repeats, reply tokens or tracks): their scores are not comparable."),
      h("section.section", sectionLabel("The score"), h("div.card.bench-card", qScores(runs))),
      runs.flatMap((r) => (r.summary || []).map((m) => qReport(r, m))));
  }
  pick.addEventListener("change", () => { if (pick.value) { quality.picked.push(pick.value); paint(); } });
  clear.addEventListener("click", () => { quality.picked = []; paint(); });
  await paint();
}
