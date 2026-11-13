// The comparison of two runs, cell by cell. Plain SVG and plain DOM: no
// innerHTML, no image, no dependency. The chart has a text form: a table
// with the same numbers.
"use strict";

/** NO_VERDICT is the sentence of a metric that cannot be judged. */
const NO_VERDICT = "the repetitions disagree, so this metric has no verdict";

/** svgEl builds one element of an SVG drawing. */
function svgEl(name, attrs, text) {
  const el = document.createElementNS("http://www.w3.org/2000/svg", name);
  for (const [k, v] of Object.entries(attrs || {})) el.setAttribute(k, v);
  if (text != null) el.textContent = text;
  return el;
}

/* ---------- the comparison, cell by cell ---------- */

/** compareCellName names a cell of a comparison: the group of the suite and
 *  the concurrency of the cell. */
function compareCellName(cell) {
  return ((cell && cell.group) || "–") + " at concurrency " + ((cell && cell.concurrency) || 0);
}

/** compareSpread reads how much the repetitions of one metric of one cell
 *  disagree: the ratio of the slowest to the fastest repetition. */
function compareSpread(cmp, cell, metric) {
  let spread = 0;
  for (const arm of [(cmp && cmp.a), (cmp && cmp.b)]) {
    const c = ((arm && arm.cells) || []).find((x) => x.group === cell.group && x.concurrency === cell.concurrency);
    const s = c && c.spread && c.spread[metric];
    if (s && s > spread) spread = s;
  }
  return spread;
}

/** compareBandText writes the band a verdict used: the noise band of the
 *  suite, in percent. The backend widens the band of the suite with the
 *  spread of the repetitions, so the band already carries the noise. */
function compareBandText(m) {
  return "band " + (m.band ? benchNum(m.band * 100, 1) + " %" : "–");
}

/** compareSpreadText writes how much the repetitions of a cell disagree: the
 *  share between the slowest and the fastest repetition. */
function compareSpreadText(spread) {
  if (!spread || spread >= SPREAD_STEADY) return null;
  return "spread ±" + benchNum((1 - spread) * 100, 1) + " %";
}

/** compareChipText writes the whole text of the verdict of one metric: the
 *  word, the band it used and, when the repetitions disagree, their spread. */
function compareChipText(m, spread) {
  const d = VERDICTS[m.verdict];
  const word = d ? d.icon + " " + d.word : "– no verdict";
  const parts = [compareBandText(m)];
  const spreadText = compareSpreadText(spread);
  if (spreadText) parts.push(spreadText);
  return word + ", " + parts.join(", ");
}

/** compareChipWidth is the width the chip of one verdict takes. */
function compareChipWidth(text) {
  return 16 + text.length * 6.3;
}

/** compareChip draws the verdict of one metric: a pill with the word and the
 *  band. It returns the width it took. */
function compareChip(svg, x, y, text, tone) {
  const w = compareChipWidth(text);
  svg.appendChild(svgEl("rect", {
    class: "chip-bg " + tone, x: x, y: y, width: w, height: 20, rx: 999,
  }));
  svg.appendChild(svgEl("text", {
    class: "chip-text " + tone, x: x + w / 2, y: y + 14, "text-anchor": "middle",
  }, text));
  return w;
}

/** compareBlock draws the comparison of one cell: the five metric groups of
 *  that cell, their bars indexed to the baseline, the change in percent and
 *  the verdict of each. The bars of every row of every cell share one scale
 *  (the baseline is 100), so the size of a change stays visible and two cells
 *  can be read against each other. */
function compareBlock(cmp, cell, scaleMax) {
  const rows = [];
  for (const m of COMPARE_METRICS) {
    const v = (cell.metrics || []).find((x) => x.metric === m.metric);
    if (v) rows.push({ m: m, v: v });
  }
  if (!rows.length) return null;
  const width = 760, rowH = 76, barX = 110, plotW = 200, valueX = 320;
  const height = 6 + rows.length * rowH;
  const svg = svgEl("svg", {
    class: "bench-bars", viewBox: "0 0 " + width + " " + height, role: "img",
    "aria-label": "The five metric groups of the cell " + compareCellName(cell) + ", indexed to the baseline",
  });
  // The bars are indexed to the baseline: the baseline is 100 and every row
  // of every cell of the chart is drawn on the same scale, so a change of
  // 5 % and a change of 50 % do not look the same.
  const scale = Math.max(100, scaleMax || 100);
  const px = (index) => (Math.max(0, Math.min(scale, index)) / scale) * plotW;
  const baseX = barX + px(100);
  rows.forEach((row, i) => {
    const y = 6 + i * rowH;
    const m = row.m, v = row.v;
    const spread = compareSpread(cmp, cell, m.metric);
    const tone = (VERDICTS[v.verdict] || {}).tone || "skip";
    const digits = benchDigits(m.unit);
    svg.appendChild(svgEl("text", { class: "bar-label", x: 0, y: y + 40 }, m.label));
    svg.appendChild(svgEl("line", {
      class: "bar-index", x1: baseX, x2: baseX, y1: y + 22, y2: y + 60,
    }));
    svg.appendChild(svgEl("rect", {
      class: "bar-rect s1", x: barX, y: y + 26, width: px(100), height: 10, rx: 5,
    }));
    const ratio = v.base > 0 ? v.cand / v.base : 0;
    const w = px(ratio * 100);
    if (w > 0) {
      svg.appendChild(svgEl("rect", { class: "bar-rect s2", x: barX, y: y + 44, width: w, height: 10, rx: 5 }));
    } else {
      svg.appendChild(svgEl("line", { class: "bar-missing", x1: barX, x2: barX + 40, y1: y + 49, y2: y + 49 }));
    }
    if (i === 0) {
      // The two arms are named on the marks themselves, not only in the key.
      svg.appendChild(svgEl("text", { class: "bar-sub", x: baseX + 6, y: y + 35 }, "baseline"));
      svg.appendChild(svgEl("text", { class: "bar-sub", x: barX + w + 6, y: y + 53 }, "candidate"));
    }
    svg.appendChild(svgEl("text", {
      class: "bar-value", x: valueX, y: y + 30,
    }, benchNum(v.base, digits) + " → " + benchNum(v.cand, digits) + " " + m.unit));
    const d = v.delta_pct;
    svg.appendChild(svgEl("text", {
      class: "bar-delta " + tone, x: valueX, y: y + 50,
    }, (d > 0 ? "+" : d < 0 ? "−" : "") + benchNum(Math.abs(d), 1) + " %"));
    const text = compareChipText(v, spread);
    compareChip(svg, width - 4 - compareChipWidth(text), y + 30, text, tone);
  });
  return svg;
}

/** compareScale is the biggest index the chart has to hold: the largest
 *  change of any metric of any cell, rounded up. Every bar of the chart is
 *  drawn on that one scale. */
function compareScale(cmp) {
  let max = 100;
  for (const cell of (cmp && cmp.cells) || []) {
    for (const v of cell.metrics || []) {
      const idx = v.base > 0 ? (v.cand / v.base) * 100 : 100;
      if (idx > max) max = idx;
    }
  }
  return Math.ceil(max / 25) * 25;
}

/** compareTable is the text form of the comparison: every cell, every metric,
 *  the two numbers, the change and the verdict. */
function compareTable(cmp) {
  const rows = [];
  for (const cell of (cmp && cmp.cells) || []) {
    for (const m of COMPARE_METRICS) {
      const v = (cell.metrics || []).find((x) => x.metric === m.metric);
      if (!v) continue;
      const spread = compareSpread(cmp, cell, m.metric);
      const digits = benchDigits(m.unit);
      const d = v.delta_pct;
      rows.push(h("tr",
        h("td", compareCellName(cell)),
        h("td", m.label),
        h("td.num", benchNum(v.base, digits) + " " + m.unit),
        h("td.num", benchNum(v.cand, digits) + " " + m.unit),
        h("td.num", (d > 0 ? "+" : d < 0 ? "−" : "") + benchNum(Math.abs(d), 1) + " %"),
        h("td", verdictChip(v.verdict, [compareBandText(v), compareSpreadText(spread)]))));
    }
  }
  if (!rows.length) return null;
  return h("details.bench-fallback",
    h("summary", "The numbers of the comparison"),
    h("div.scroll", h("table.bench-table",
      h("thead", h("tr",
        h("th", "CELL"), h("th", "METRIC"),
        h("th.num", "BASELINE"), h("th.num", "CANDIDATE"), h("th.num", "CHANGE"), h("th", "VERDICT"))),
      h("tbody", rows))));
}

/** compareChart draws the comparison of two runs: one block per cell, the
 *  five metric groups of that cell in each block, and the numbers in a
 *  table under the chart. */
function compareChart(cmp) {
  const blocks = [];
  const noVerdict = [];
  const scale = compareScale(cmp);
  for (const cell of (cmp && cmp.cells) || []) {
    const svg = compareBlock(cmp, cell, scale);
    if (!svg) continue;
    blocks.push(h("div.bench-compare-cell",
      h("p.bench-compare-cellhead", compareCellName(cell)),
      svg));
    for (const m of COMPARE_METRICS) {
      const v = (cell.metrics || []).find((x) => x.metric === m.metric);
      if (v && !VERDICTS[v.verdict]) noVerdict.push(m.label);
    }
  }
  if (!blocks.length) return emptyState("Pick two results to compare.",
    "Two saved results side by side, with a verdict on every cell.", "bernie bench list");
  const legend = h("div.bench-legend",
    h("span", h("i.key.s1"), "baseline"),
    h("span", h("i.key.s2"), "candidate"),
    h("span", "indexed: the baseline is 100, the scale ends at " + scale));
  const note = noVerdict.length
    ? h("p.small.muted", NO_VERDICT + ": " + noVerdict.join(", ") + ".")
    : null;
  return h("div.bench-compare", legend, blocks, note, compareTable(cmp));
}
