// Shared SVG sparklines and single-series area charts, built once and updated.
// Missing samples stay gaps; keyboard readouts and data tables expose values.
// Adapted from vllm-rdna-dash by CrispyGinger, used with permission.
"use strict";


/** Downsample {v} points by ordered bucket minima/maxima; empty buckets stay gaps. */
function chartMinMax(points, max) {
  if (!points || points.length <= max) return points || [];
  const n = points.length;
  const buckets = Math.max(1, Math.floor(max / 2));
  const out = [];
  for (let b = 0; b < buckets; b++) {
    const from = Math.floor((b * n) / buckets);
    const to = Math.floor(((b + 1) * n) / buckets);
    if (to <= from) continue;
    let lo = -1;
    let hi = -1;
    for (let i = from; i < to; i++) {
      const v = points[i].v;
      if (v == null || !isFinite(v)) continue;
      if (lo < 0 || v < points[lo].v) lo = i;
      if (hi < 0 || v > points[hi].v) hi = i;
    }
    // A bucket without a measured value keeps a gap.
    if (lo < 0) { out.push(points[from]); continue; }
    if (lo === hi) { out.push(points[lo]); continue; }
    if (lo < hi) out.push(points[lo], points[hi]);
    else out.push(points[hi], points[lo]);
  }
  return out;
}

/** chartNumbers keeps the numbers of a series that can be drawn. */
function chartNumbers(values) {
  const out = [];
  for (const v of values || []) if (typeof v === "number" && isFinite(v)) out.push(v);
  return out;
}

/** Avoid DOM writes for unchanged series. */
function chartSame(a, b) {
  if (!a || !b || a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) if (a[i] !== b[i]) return false;
  return true;
}

/** Numeric or {v} samples; unknown points break paths. baseY closes the area. */
function chartPaths(points, px, py, baseY) {
  let line = "";
  let area = "";
  let run = [];
  const flush = () => {
    if (run.length) {
      let d = "";
      for (let k = 0; k < run.length; k++) {
        d += (k ? "L" : "M") + px(run[k].i).toFixed(1) + " " + py(run[k].v).toFixed(1);
      }
      line += d;
      if (run.length > 1 && baseY != null) {
        area += d + " L" + px(run[run.length - 1].i).toFixed(1) + " " + baseY +
          " L" + px(run[0].i).toFixed(1) + " " + baseY + " Z";
      }
    }
    run = [];
  };
  (points || []).forEach((p, i) => {
    const v = p && typeof p === "object" ? p.v : p;
    if (typeof v !== "number" || !isFinite(v)) {
      flush();
      return;
    }
    run.push({ i: i, v: v });
  });
  flush();
  return { line: line, area: area };
}

/** Pointer and keyboard share a live point count; index -1 clears the readout. */
function chartPointer(svg, count, onIndex) {
  let index = -1;
  const total = () => {
    const n = typeof count === "function" ? count() : count;
    return typeof n === "number" && n > 0 ? Math.floor(n) : 0;
  };
  const move = (i) => {
    const n = total();
    if (n < 1) return;
    index = Math.max(0, Math.min(n - 1, i));
    onIndex(index);
  };
  const at = (ev) => {
    const n = total();
    if (n < 1) return;
    const box = svg.getBoundingClientRect();
    if (!box.width) return;
    move(Math.round(((ev.clientX - box.left) / box.width) * (n - 1)));
  };
  svg.addEventListener("pointermove", at);
  svg.addEventListener("pointerleave", () => { index = -1; onIndex(-1); });
  svg.addEventListener("focus", () => move(total() - 1));
  svg.addEventListener("blur", () => { index = -1; onIndex(-1); });
  svg.addEventListener("keydown", (ev) => {
    if (ev.key === "ArrowRight") { move(index + 1); ev.preventDefault(); }
    else if (ev.key === "ArrowLeft") { move(index - 1); ev.preventDefault(); }
    else if (ev.key === "Home") { move(0); ev.preventDefault(); }
    else if (ev.key === "End") { move(total() - 1); ev.preventDefault(); }
  });
}

/** Round scale maxima up to 1, 2 or 5 times a power of ten. */
function chartNice(v) {
  if (!(v > 0)) return 1;
  const e = Math.pow(10, Math.floor(Math.log10(v)));
  const f = v / e;
  const m = f <= 1 ? 1 : f <= 2 ? 2 : f <= 5 ? 5 : 10;
  return m * e;
}

/** Drawing and pointer share a fixed limit or a rounded sample maximum. */
function sparkDomain(values, max) {
  let top = 0;
  for (const v of values || []) {
    if (typeof v === "number" && isFinite(v) && v > top) top = v;
  }
  const fixed = typeof max === "number" && max > 0;
  if (fixed) top = max;
  else top = chartNice(top);
  return { min: 0, max: top > 0 ? top : 1, fixed: fixed };
}

/** Accessible text values, with pooled rows grown as the history fills. */
function chartDataTable(caption, head, summary) {
  const box = h("details", { class: "chart-data" });
  const table = h("table", { class: "chart-data-table" });
  const row = h("tr");
  for (const name of head) row.append(h("th", { scope: "col" }, name));
  const body = h("tbody");
  table.append(h("caption", { class: "chart-data-caption" }, caption),
    h("thead", row), body);
  box.append(h("summary", { class: "chart-data-summary" }, summary || "The numbers of the chart"), table);
  // The cells of the table: they are written, never built again.
  const cells = [];
  let width = 0;
  const fill = (rows) => {
    const cols = rows.length ? rows[0].length : 0;
    if (cols !== width) {
      // The number of the columns changed: the rows are built again.
      body.replaceChildren();
      cells.length = 0;
      width = cols;
    }
    // The rows are built in twos: a series that grows by one point does
    // not build a row, and the rows that the series does not need yet are
    // hidden.
    if (rows.length > cells.length) {
      let size = cells.length > 0 ? cells.length : 1;
      while (size < rows.length) size *= 2;
      while (cells.length < size) {
        const tr = h("tr");
        const texts = [];
        for (let i = 0; i < width; i++) {
          const cell = i === 0 ? h("th", { scope: "row" }) : h("td", { class: "num" });
          const text = document.createTextNode("");
          cell.append(text);
          texts.push(text);
          tr.append(cell);
        }
        body.append(tr);
        cells.push({ row: tr, texts: texts });
      }
    }
    for (let r = 0; r < cells.length; r++) {
      const hide = r >= rows.length;
      if (cells[r].row.hidden !== hide) cells[r].row.hidden = hide;
      if (hide) continue;
      const texts = cells[r].texts;
      for (let c = 0; c < texts.length; c++) {
        const v = String(rows[r][c]);
        if (texts[c].nodeValue !== v) texts[c].nodeValue = v;
      }
    }
  };
  box.update = fill;
  return box;
}

/** Reusable sparkline: update(next, max) repaints without replacing controls. */
function spark(values, max, label, opts) {
  const W = 120, H = 34, PAD = 2;
  const options = opts || {};
  const wrap = h("div.spark-wrap");
  const svg = chartSvg("svg", {
    class: "spark-svg", viewBox: "0 0 " + W + " " + H, preserveAspectRatio: "none",
    role: "img", tabindex: "0", "aria-label": label || "the last minute",
  });
  const base = chartSvg("line", { x1: 0, x2: W, y1: H - 1, y2: H - 1,
    class: "chart-base" });
  // The spark draws the line only: a filled block under a line that never
  // moves reads as a solid bar, not as a number.
  const line = chartSvg("path", { class: "chart-line" });
  const mark = chartSvg("circle", { r: 2.4, class: "chart-pointer" });
  svg.append(base, line, mark);
  const read = h("span.chart-read", { class: "chart-readout" });
  const unit = options.unit ? " (" + options.unit + ")" : "";
  const table = chartDataTable((label || "the last minute") + ": one row per point, the newest last",
    ["Point", (label || "the value") + unit], "The numbers");
  wrap.append(svg, read, table);

  let values2 = values || [];
  // The top of the scale is the limit of the measure. A caller that names
  // one is obeyed; a percentage is of a hundred and needs no other limit.
  const limit = () => (typeof max === "number" && max > 0)
    ? max : (options.unit === "%" ? 100 : null);
  let domain = sparkDomain(values2, limit());
  const paint = (vals) => {
    const n = vals.length;
    const top = domain.max;
    const step = n > 1 ? W / (n - 1) : 0;
    const px = (i) => (n > 1 ? i * step : W / 2);
    const py = (v) => H - PAD - (Math.min(v, top) / top) * (H - 2 * PAD);
    if (!n) {
      line.setAttribute("d", "");
      mark.setAttribute("opacity", "0");
      return;
    }
    // A missing point breaks the line: the gap in the data is a gap in
    // the chart.
    line.setAttribute("d", chartPaths(vals, px, py, null).line);
    mark.setAttribute("opacity", "0");
  };
  const show = (i) => {
    if (i < 0) { read.style.opacity = "0"; mark.setAttribute("opacity", "0"); return; }
    const v = values2[i];
    if (typeof v !== "number" || !isFinite(v)) {
      read.textContent = "–" + (options.unit ? " " + options.unit : "");
      read.style.opacity = "1";
      mark.setAttribute("opacity", "0");
      return;
    }
    read.textContent = (options.format ? options.format(v) : num(v)) + (options.unit ? " " + options.unit : "");
    read.style.opacity = "1";
  };
  const rows = (vals) => {
    const out = [];
    for (let i = 0; i < vals.length; i++) {
      const v = vals[i];
      out.push([String(i + 1), (typeof v === "number" && isFinite(v))
        ? (options.format ? options.format(v) : num(v)) + (options.unit ? " " + options.unit : "")
        : "–"]);
    }
    return out;
  };
  paint(values2);
  chartPointer(svg, () => values2.length, (i) => {
    if (i < 0) { read.style.opacity = "0"; mark.setAttribute("opacity", "0"); return; }
    const v = values2[i];
    if (typeof v !== "number" || !isFinite(v)) {
      show(i);
      return;
    }
    const top = domain.max;
    const step = values2.length > 1 ? W / (values2.length - 1) : 0;
    const x = values2.length > 1 ? i * step : W / 2;
    const y = H - PAD - (Math.min(v, top) / top) * (H - 2 * PAD);
    mark.setAttribute("cx", x.toFixed(1));
    mark.setAttribute("cy", y.toFixed(1));
    mark.setAttribute("opacity", "1");
    show(i);
  });
  let shown = null;
  wrap.update = (next, nextMax) => {
    const vals = next || [];
    const same = chartSame(vals, shown) && (!nextMax || nextMax === max);
    values2 = vals;
    if (nextMax) max = nextMax;
    domain = sparkDomain(values2, limit());
    if (!same) {
      paint(values2);
      table.update(rows(values2));
    }
    shown = vals;
    const nums = chartNumbers(values2);
    const last = nums.length ? nums[nums.length - 1] : null;
    // A spark without a fixed limit says the range it draws, so the reader
    // knows what the height of the line means.
    const range = domain.fixed ? ""
      : ", 0 to " + num(domain.max) + (options.unit ? " " + options.unit : "");
    svg.setAttribute("aria-label", (label || "the last minute") + ": " +
      (last == null ? "no sample yet" : (options.format ? options.format(last) : num(last)) + (options.unit ? " " + options.unit : "")) + range);
  };
  wrap.update(values);
  return wrap;
}

/** A numeric history with gaps, keyboard readout and a text table. */
function areaChart(input, opts) {
  const options = opts || {};
  const W = 320, H = 170, L = 34, R = 312, T = 8, B = 128;
  const box = h("figure.chart", { class: "chart-figure" });
  const head = h("figcaption", { class: "chart-caption" });
  const title = h("span", { class: "chart-label" }, options.title || "");
  const read = h("span.chart-read", { class: "chart-value" }, "");
  head.append(title, read);
  const svg = chartSvg("svg", {
    viewBox: "0 0 " + W + " " + H, role: "img", tabindex: "0",
    "aria-label": options.title || "the measure", class: "chart-svg",
  });
  const grid = chartSvg("g", { class: "chart-grid" });
  const ylab = chartSvg("g", { class: "chart-axis-end" });
  const xlab = chartSvg("g", { class: "chart-axis" });
  const area = chartSvg("path", { class: "chart-area" });
  const line = chartSvg("path", { class: "chart-line-strong" });
  const cross = chartSvg("line", { y1: T, y2: B, class: "chart-cross" });
  const dot = chartSvg("circle", { r: 3.5, class: "chart-dot" });
  svg.append(grid, area, line, cross, dot, ylab, xlab);
  // The grid and the axes are built once. A frame writes the path of a
  // series and moves the labels; it never builds the axis tree again.
  const yLabels = [];
  for (const f of [0, 0.5, 1]) {
    const y = B - f * (B - T);
    grid.append(chartSvg("line", { x1: L, x2: R, y1: y, y2: y }));
    yLabels.push(chartSvg("text", { x: L - 6, y: y + 3 }));
  }
  ylab.append(...yLabels);
  const xLabels = [];
  for (const x of [L, (L + R) / 2, R]) {
    xLabels.push(chartSvg("text", {
      x: x, y: B + 18,
      "text-anchor": x === L ? "start" : x === R ? "end" : "middle",
    }));
  }
  xlab.append(...xLabels);
  const empty = h("div.chart-empty", { hidden: true },
    h("b", { class: "chart-strong" }, "No samples yet."),
    h("span", { class: "chart-sub" }, options.hint || "The chart fills after the first minute."));
  const unit = options.unit ? " (" + options.unit + ")" : "";
  const name = options.title || "the measure";
  const table = chartDataTable(name + ": one row per point, the newest last",
    ["Point", (options.unit && name.indexOf(options.unit) < 0 ? name + unit : name)],
    "The numbers of the chart");
  box.append(head, svg, empty, table);

  const norm = (values) => chartMinMax((values || []).map((v) => ({
    v: Number.isFinite(v) ? v : null,
  })), 240);
  let points = norm(input);
  let maxV = 1, scaleV = 0, windowM = 0;
  const paint = () => {
    let top = 0;
    for (const p of points) if (Number.isFinite(p.v) && p.v > top) top = p.v;
    maxV = options.max > 0 ? options.max : top || 1;
    if (maxV !== scaleV) {
      yLabels[0].textContent = "0";
      yLabels[1].textContent = num(maxV / 2, maxV < 10 ? 1 : 0);
      yLabels[2].textContent = num(maxV, maxV < 10 ? 1 : 0) + (options.unit ? " " + options.unit : "");
      scaleV = maxV;
    }
    const mins = options.windowMinutes || 5;
    if (mins !== windowM) {
      xLabels[0].textContent = "- " + mins + " min";
      xLabels[1].textContent = "- " + (mins / 2) + " min";
      xLabels[2].textContent = "now";
      windowM = mins;
    }
    empty.hidden = points.length > 0;
    const px = (i) => points.length > 1 ? L + i * (R - L) / (points.length - 1) : (L + R) / 2;
    const py = (v) => B - Math.min(v, maxV) / maxV * (B - T);
    const paths = chartPaths(points, px, py, B);
    line.setAttribute("d", paths.line);
    area.setAttribute("d", paths.area);
  };
  const fmt = (p) => Number.isFinite(p.v)
    ? (options.format ? options.format(p.v) : num(p.v)) + (options.unit ? " " + options.unit : "") : "–";
  const show = (i) => {
    cross.setAttribute("opacity", "0");
    dot.setAttribute("opacity", "0");
    if (i < 0) {
      read.textContent = points.length ? fmt(points[points.length - 1]) : "";
      return;
    }
    const p = points[i];
    if (!p) return;
    read.textContent = fmt(p);
    if (!Number.isFinite(p.v)) return;
    const x = points.length > 1 ? L + i * (R - L) / (points.length - 1) : (L + R) / 2;
    const y = B - Math.min(p.v, maxV) / maxV * (B - T);
    cross.setAttribute("x1", x.toFixed(1));
    cross.setAttribute("x2", x.toFixed(1));
    cross.setAttribute("opacity", "1");
    dot.setAttribute("cx", x.toFixed(1));
    dot.setAttribute("cy", y.toFixed(1));
    dot.setAttribute("opacity", "1");
  };
  chartPointer(svg, () => points.length, show);
  let first = true;
  box.update = (next, mins) => {
    if (mins) options.windowMinutes = mins;
    const incoming = norm(next);
    const same = incoming.length === points.length && incoming.every((p, i) => p.v === points[i].v);
    if (!first && same && (options.windowMinutes || 5) === windowM) return;
    first = false;
    points = incoming;
    paint();
    svg.setAttribute("aria-label", name + ": " + (points.length ? fmt(points[points.length - 1]) : "no sample yet"));
    show(-1);
    table.update(points.map((p, i) => [String(i + 1), fmt(p)]));
  };
  box.update(input);
  return box;
}
