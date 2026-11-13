"use strict";
// The Quality tab: does the server answer right? It reads the saved results
// of `bernie bench eval` (spec §10.5): the score of every model by track and
// by context, the causes of the failures, every case with its diagnosis, and
// the CSV of a result. Nothing is computed here but the layout.

/** qPct writes a share as a percent. */
function qPct(x) {
  return x == null ? "–" : Math.round(x * 100) + " %";
}

/** qBuckets is the table of the score by track or by context. */
function qBuckets(title, rows) {
  return h("div.scroll", h("table.bench-table",
    h("thead", h("tr", h("th", title), h("th.num", "PASSED", h("small", "of graded")), h("th.num", "ACCURACY"))),
    h("tbody", (rows || []).map((b) => h("tr", h("td", b.name), h("td.num", b.passed + " / " + b.graded), h("td.num", qPct(b.accuracy)))))));
}

/** qFacts are the headline numbers of one model. */
function qFacts(s) {
  const p = s.performance || {}, a = s.agent_efficiency;
  const fact = (label, value, why) => h("div", h("dt", label), h("dd", value, h("span.why", why)));
  const rate = (v, src) => (v ? Math.round(v) + " tok/s" + (src && src.length ? " (" + src.join(", ") + ")" : "") : "not reported");
  return h("dl.facts.bench-kpis",
    fact("Accuracy", qPct(s.graded ? s.accuracy : null), s.passed + " of " + s.graded + " graded, " + s.skipped + " skipped"),
    fact("Not passed", String(s.failed + s.errors), s.failed + " failed, " + s.errors + " errors, " + s.critical_failures + " critical, " + s.loops + " loops"),
    fact("Speed", (s.avg_latency_s || 0).toFixed(1) + " s a case", "prefill " + rate(p.prefill_tok_s, p.prefill_sources) + ", decode " + rate(p.decode_tok_s, p.decode_sources)),
    fact("Tool efficiency", a && a.ratio != null ? a.ratio.toFixed(2) : "–",
      a ? a.tool_calls + " calls for a minimum of " + a.min_tool_calls + ", " + a.budget_exhausted + " hit the guard" : "no agent case"));
}

/** qWhy is what the diagnosis of a case says: the cause, the comparisons
 *  that failed and what the evidence cannot show. */
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
  const row = h("tr", h("td", h("b", c.case_id), h("small.bench-lens", c.model)), h("td", c.track),
    h("td.num", String(c.context_size)), state, h("td.num", c.status === "skipped" ? "–" : c.latency_s.toFixed(1) + " s"),
    h("td", (c.diagnosis || {}).title || ""));
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

/** qModel is the section of one model: its numbers, its scores and its cases. */
function qModel(r, s) {
  const cats = Object.entries(s.failure_categories || {}).sort((a, b) => b[1] - a[1]);
  const cases = (r.cases || []).filter((c) => c.model === s.model)
    .sort((a, b) => (a.status === "pass") - (b.status === "pass") || a.case_id.localeCompare(b.case_id));
  return h("section.section", sectionLabel(s.model),
    h("div.card.bench-card",
      qFacts(s),
      cats.length ? h("p.small", "Why cases did not pass: " + cats.map((c) => c[0].replace(/_/g, " ") + " " + c[1]).join(", ") + ".") : null,
      h("div.bench-charts", qBuckets("TRACK", s.by_track), qBuckets("CONTEXT", s.by_context)),
      h("div.scroll", h("table.bench-table",
        h("thead", h("tr", ["CASE", "TRACK", "CONTEXT", "STATE", "LATENCY", "DIAGNOSIS"].map((x, i) => h("th", { class: i === 2 || i === 4 ? "num" : "" }, x)))),
        h("tbody", cases.map(qCase))))));
}

async function viewQuality(root) {
  const pick = h("select", { id: "q-pick", "aria-label": "The eval result" });
  const csv = h("a", { download: "" }, "Download CSV");
  const head = stateHead("Quality", h("div.head-actions", pick, csv));
  const body = h("div");
  put(root, head, body);
  let list;
  try {
    list = await api("/api/eval");
  } catch (e) {
    setHead(head, "The eval results could not be read.", String(e.message || e));
    return;
  }
  if (!list.length) {
    pick.hidden = csv.hidden = true;
    setHead(head, "Nothing has been graded yet.", "An eval asks the server seeded questions and grades every answer. Run one and its score shows here.");
    put(body, emptyState("No eval yet.", "Run the smoke suite against the server that serves.", "bernie bench eval --context auto"));
    return;
  }
  list.sort((a, b) => new Date(b.time) - new Date(a.time));
  put(pick, list.map((r) => h("option", { value: r.id }, r.id + " · " + r.options.suite)));
  async function show(id) {
    csv.href = "/api/eval/" + encodeURIComponent(id) + "?format=csv";
    put(body, loading("Reading the result…"));
    let r;
    try {
      r = await api("/api/eval/" + encodeURIComponent(id));
    } catch (e) {
      put(body, emptyState("The result could not be read.", String(e.message || e), "bernie bench eval show " + id));
      return;
    }
    const s = (r.summary || [])[0] || {};
    setHead(head, (s.model || "The model") + " passed " + (s.passed || 0) + " of " + (s.graded || 0) + " graded cases (" + qPct(s.graded ? s.accuracy : null) + ").",
      "Suite " + r.options.suite + " v" + r.version + ", contexts " + (r.options.context_sizes || []).join(", ") + ", " + (r.summary || []).length +
      " model(s), " + r.status + ". Every case that did not pass opens on its cause.");
    put(body, (r.summary || []).map((m) => qModel(r, m)));
  }
  pick.addEventListener("change", () => show(pick.value));
  await show(list[0].id);
}
