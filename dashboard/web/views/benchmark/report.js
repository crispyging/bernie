"use strict";
// The result of a benchmark: the four headline numbers, the three charts by
// context and the context × in-flight table, where a failed, skipped or
// flagged cell opens on its reasons. The numbers come from the result and
// its summary (spec §10.1); nothing is computed here but the layout.

/** benchCtx is the prompt length of a cell: the lens of its group, or the
 *  input length of a plain run. */
function benchCtx(r, c) {
  return c.input_len || ((r && r.options) || {}).input_len || 0;
}

/** benchCtxName writes a context the short way: 2048 is "2K". */
function benchCtxName(n) {
  if (!n) return "–";
  return n >= 1024 && n % 1024 === 0 ? n / 1024 + "K" : String(n);
}

/** benchRefName names the cell of a summary: "c8 at 2K context". */
function benchRefName(ref) {
  return ref ? "c" + ref.concurrency + " at " + benchCtxName(ref.input_len) + " context" : "no cell ran";
}

/** benchKpis are the four headline numbers of a result. */
function benchKpis(r) {
  const s = r.summary || {};
  const fact = (label, value, why) => h("div", h("dt", label), h("dd", value, h("span.why", why)));
  const rate = (v, d) => (v ? benchNum(v, d) + " tok/s" : "–");
  const single = s.single ? "c1 at " + benchCtxName(s.single.input_len) + " context" : "no cell ran at concurrency 1";
  const left = [];
  if (s.failed) left.push(s.failed + " failed");
  if (s.skipped) left.push(s.skipped + " of " + s.cells + " cells skipped");
  return h("dl.facts.bench-kpis",
    fact("Peak output", rate(s.peak_output_tok_s, 1), "aggregate, " + benchRefName(s.peak)),
    fact("Single-stream decode", rate(s.single_decode_tok_s, 1), "one request, " + single),
    fact("Prefill at c=1", rate(s.single_prefill_tok_s, 0), single +
      (s.single_prefill_source === "probe" ? ", max_tokens=1 probe" : "; includes queueing")),
    fact("Requests ok", (s.completed || 0) + " / " + (s.requests || 0), left.join(", ") || "none failed"));
}

/** benchAggPrefill is the aggregate prefill of a cell: the rate of its probe
 *  when the run sent one, else the rate of the cell. */
function benchAggPrefill(c) {
  return (c.probe && c.probe.agg_tok_s) || (c.prefill || {}).agg_tok_s;
}

/** BENCH_CHARTS are the three charts of a result: the metric, its title, its
 *  unit, its decimals and how it is measured. */
const BENCH_CHARTS = [
  { get: (c) => c.output_tok_s, title: "Output throughput", unit: "tok/s", digits: 0,
    note: "All completion tokens of the cell ÷ its wall time." },
  { get: benchAggPrefill, title: "Aggregate prefill", unit: "tok/s", digits: 0,
    note: "N simultaneous max_tokens=1 requests: computed prompt tokens ÷ wall time. Without the probe, the cell's rate, queueing included." },
  { get: (c) => (c.ttft_s || {}).p50, title: "Time to first token, p50", unit: "s", digits: 2,
    note: "Includes queueing behind the other requests in flight." },
];

/** benchGrid lays the cells out for the charts: the contexts along x and one
 *  line per in-flight level. A result of one context puts the levels along x
 *  instead, with one line. */
function benchGrid(r) {
  const ran = (r.cells || []).filter((c) => !c.skipped && c.completed);
  const ctxs = [...new Set(ran.map((c) => benchCtx(r, c)))].sort((a, b) => a - b);
  const levels = [...new Set(ran.map((c) => c.concurrency))].sort((a, b) => a - b);
  const find = (ctx, lv) => ran.find((c) => benchCtx(r, c) === ctx && c.concurrency === lv);
  if (ctxs.length > 1) {
    return { axis: "context (prompt tokens)", xs: ctxs.map(benchCtxName),
      series: levels.map((lv) => ({ name: "c" + lv, cells: ctxs.map((x) => find(x, lv)) })) };
  }
  return { axis: "requests in flight", xs: levels.map(String),
    series: [{ name: benchCtxName(ctxs[0]) + " context", cells: levels.map((lv) => find(ctxs[0], lv)) }] };
}

/** benchChart draws one metric of the grid as lines. A missing point breaks
 *  the line: it is never interpolated. */
function benchChart(grid, spec) {
  const W = 360, H = 210, L = 52, R = 12, T = 12, B = 44;
  const vals = grid.series.map((s) => s.cells.map((c) => (c ? spec.get(c) : null)));
  const max = Math.max(1e-9, ...vals.flat().filter((v) => v > 0)) * 1.1;
  const n = grid.xs.length;
  const x = (i) => (n < 2 ? (L + W - R) / 2 : L + (W - L - R) * i / (n - 1));
  const y = (v) => T + (H - T - B) * (1 - v / max);
  const svg = svgEl("svg", { class: "bench-curve", viewBox: "0 0 " + W + " " + H, role: "img",
    "aria-label": spec.title + " by " + grid.axis + "; the numbers are in the table below" });
  for (const f of [0, 0.5, 1]) {
    const v = max / 1.1 * f;
    svg.appendChild(svgEl("line", { class: "grid", x1: L, x2: W - R, y1: y(v), y2: y(v) }));
    svg.appendChild(svgEl("text", { class: "axis-label", x: L - 6, y: y(v) + 3, "text-anchor": "end" },
      benchNum(v, v < 10 ? spec.digits : 0)));
  }
  grid.xs.forEach((t, i) => svg.appendChild(svgEl("text",
    { class: "axis-label", x: x(i), y: H - B + 16, "text-anchor": "middle" }, t)));
  svg.appendChild(svgEl("text", { class: "axis-label", x: (L + W - R) / 2, y: H - 6, "text-anchor": "middle" }, grid.axis));
  vals.forEach((row, si) => {
    const cls = "s" + (si % 4 + 1);
    let prev = null;
    row.forEach((v, i) => {
      if (!(v > 0)) { prev = null; return; }
      if (prev) svg.appendChild(svgEl("line", { class: "curve-line " + cls, x1: prev[0], y1: prev[1], x2: x(i), y2: y(v) }));
      const dot = svgEl("circle", { class: "curve-dot " + cls, cx: x(i), cy: y(v), r: 4 });
      dot.appendChild(svgEl("title", {}, grid.series[si].name + " at " + grid.xs[i] + ": " +
        benchNum(v, spec.digits) + " " + spec.unit));
      svg.appendChild(dot);
      prev = [x(i), y(v)];
    });
  });
  return h("figure.bench-chart",
    h("figcaption", h("b", spec.title), h("small", " " + spec.unit)),
    svg,
    h("p.small.muted", spec.note));
}

/** benchCharts are the three charts with one legend. */
function benchCharts(r) {
  const grid = benchGrid(r);
  if (!grid.xs.length) return null;
  return h("div",
    h("div.bench-legend", grid.series.map((s, i) => h("span", h("i.key.s" + (i % 4 + 1)), s.name))),
    h("div.bench-charts", BENCH_CHARTS.map((spec) => benchChart(grid, spec))));
}

/** benchCellState is the state of a cell in a word, with its reasons. */
function benchCellState(c) {
  const why = [];
  if (c.skipped) return { kind: "skipped", word: "skipped", why: [c.skipped] };
  if (c.failed) why.push(c.failed + " of " + c.num_prompts + " requests failed.");
  for (const e of c.errors || []) why.push("Error: " + e);
  if (c.warmup_failed) why.push(c.warmup_failed + " warm-up requests failed.");
  if (c.incomplete) why.push("The cell lost a warm-up or a repetition, so no verdict is issued for it.");
  for (const f of cellFlags(c)) if (f.text !== "a request was lost") why.push("Flag: " + f.text + ".");
  // A measurement the server could not give is a note: it does not flag
  // the cell.
  const notes = [["Prefill probe", (c.probe || {}).unavailable], ["Probe server counters", (c.probe || {}).server_unavailable],
    ["Server counters", c.server_batch_unavailable]].filter((n) => n[1]).map((n) => n[0] + " unavailable: " + n[1] + ".");
  const st = !c.completed ? ["fail", "failed"] : c.failed || c.incomplete ? ["serious", "incomplete"] : why.length ? ["warn", "flagged"] : ["ok", "ok"];
  return { kind: st[0], word: st[1], why: why.concat(notes) };
}

/** benchServer is the rate the server's counters give, after the client's. */
function benchServer(c, key, d) {
  const v = (c.server_batch || {})[key];
  return v ? " (server " + benchNum(v, d) + ")" : "";
}

/** BENCH_COLS are the columns of the table: the head, the unit and the text
 *  of a cell. */
const BENCH_COLS = [
  ["OUTPUT", "tok/s", (c) => benchNum(c.output_tok_s, 1)],
  ["DECODE", "tok/s / req p50", (c) => benchNum(((c.decode || {}).req || {}).p50, 1) + benchServer(c, "decode_tok_s", 1)],
  ["PREFILL", "tok/s agg", (c) => benchNum(benchAggPrefill(c), 0) + (c.probe && c.probe.agg_tok_s ? " probe" : "")],
  ["PREFILL", "tok/s / req p50", (c) => benchNum(((c.prefill || {}).req || {}).p50, 0) + benchServer(c, "prefill_tok_s", 0)],
  ["TTFT", "s p50 / p95", (c) => benchNum((c.ttft_s || {}).p50, 3) + " / " + benchNum((c.ttft_s || {}).p95, 3)],
  ["ITL", "ms p50 / p95", (c) => benchNum((c.itl_ms || {}).p50, 1) + " / " + benchNum((c.itl_ms || {}).p95, 1)],
  ["TOKENS", "in / out / cached", (c) => benchNum(c.mean_prompt_tokens, 0) + " / " +
    benchNum(c.mean_output_tokens, 0) + " / " + (c.mean_cached_prompt_tokens ? benchNum(c.mean_cached_prompt_tokens, 0) : "–")],
];

/** benchRow is one cell of the table. A cell with reasons has a button that
 *  opens them in the row under it. */
function benchRow(r, c, peak) {
  const st = benchCellState(c);
  const isPeak = peak && peak.concurrency === c.concurrency && peak.input_len === benchCtx(r, c);
  const state = h("td", statePill(st.kind, st.word));
  const ctx = benchCtxName(benchCtx(r, c));
  const row = h("tr", { class: isPeak ? "is-headline" : "" },
    h("td", h("b", ctx), c.group && c.group.toUpperCase() !== ctx ? h("small.bench-lens", c.group) : null,
      isPeak ? h("span.bench-headline-tag", "peak") : null),
    h("td.num", String(c.concurrency)),
    state,
    h("td.num", c.skipped ? "–" : c.completed + " / " + c.num_prompts),
    // A cell where nothing answered measured nothing: its numbers are "–".
    BENCH_COLS.map((col) => h("td.num", c.completed ? col[2](c) : "–")));
  if (!st.why.length) return [row];
  const more = h("tr.bench-why", { hidden: true },
    h("td", { colspan: 4 + BENCH_COLS.length }, h("ul", st.why.map((w) => h("li", w)))));
  const btn = h("button.bench-why-btn", { type: "button", "aria-expanded": "false" }, "Why");
  btn.addEventListener("click", () => {
    more.hidden = !more.hidden;
    btn.setAttribute("aria-expanded", String(!more.hidden));
  });
  state.append(" ", btn);
  return [row, more];
}

/** benchMatrix is the context × in-flight table of a result. */
function benchMatrix(r) {
  const cells = (r.cells || []).slice().sort((a, b) =>
    benchCtx(r, a) - benchCtx(r, b) || a.concurrency - b.concurrency);
  const peak = (r.summary || {}).peak;
  return h("div.scroll", h("table.bench-table",
    h("thead", h("tr",
      [["CONTEXT", "tokens"], ["IN FLIGHT", "requests"], ["STATE", ""], ["REQUESTS", "ok / sent"]]
        .concat(BENCH_COLS).map((col, i) => h("th", { class: i === 0 || i === 2 ? "" : "num" }, col[0], h("small", col[1]))))),
    h("tbody", cells.map((c) => benchRow(r, c, peak)))));
}

/** benchReport is the whole result: what ran, the headline numbers, the
 *  charts, the table and how the numbers are measured. */
function benchReport(r) {
  if (!r) return emptyState("No result yet.", "Run a suite above and its numbers show here.", "bernie bench --suite smoke");
  return h("div",
    h("p.bench-result-title", "Result " + r.id + (r.model ? " · " + r.model : "") + " ",
      h("a.small", { href: "/api/bench/csv?id=" + encodeURIComponent(r.id), download: "" }, "Download CSV")),
    h("p.small.muted", benchSuiteLine(r)),
    benchKpis(r),
    benchCharts(r),
    benchMatrix(r),
    h("details.bench-fallback",
      h("summary", "How the numbers are measured"),
      h("p.small.muted", METRIC_HELP)));
}
