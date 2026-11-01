// Shared exact readings and compact comparison; missing values stay unknown.
"use strict";

function gpuValueTable(gpus, samples, checks, options = {}) {
  const compact = !!options.compact;
  const body = h("tbody");
  const columns = compact
    ? ["GPU", { label: "Activity", num: true }, { label: "VRAM used / total", num: true },
      { label: "Temperature", num: true }, { label: "Power", num: true }, "Warnings"]
    : ["GPU", { label: "Load", num: true }, { label: "Memory", num: true },
      { label: "Power", num: true }, { label: "Edge", num: true },
      { label: "Junction", num: true }, { label: "Memory temp", num: true },
      { label: "SCLK", num: true }, { label: "MCLK", num: true }, "ECC"];
  const el = h("div.scroll", {
    class: compact ? "gpu-comparison-scroll" : null, tabindex: "0", role: "region",
    "aria-label": compact ? "GPU comparison and device inspection" : "Exact GPU readings, scroll for more columns",
  }, h("table.gpu-value-table", { class: compact ? "gpu-comparison" : null, role: "table" },
    tableCaption(compact ? "GPU comparison; select Inspect for exact readings."
      : "Every measured value of each GPU. A dash means unknown."), tableHead(columns), body));
  const rows = new Map();
  let items = [], selected = null;
  const text = (node, value) => { if (node.textContent !== value) node.textContent = value; };
  const value = (v, unit) => Number.isFinite(v) ? num(v) + unit : "–";
  const select = (key) => {
    selected = rows.has(key) ? key : null;
    for (const r of rows.values()) {
      r.tr.classList.toggle("sel", r.key === selected);
      if (r.button) r.button.setAttribute("aria-pressed", String(r.key === selected));
    }
    return rows.get(selected) || null;
  };
  const row = (key) => {
    const r = { key: key, title: h("b"), name: h("div.small.muted"), pci: compact ? null : h("div.small.muted") };
    const identity = compact ? h("button.gpu-inspect", {
      type: "button", "aria-controls": options.controls, "aria-pressed": "false",
      onclick: () => { select(r.key); if (options.onInspect) options.onInspect(r); },
    }, r.title, h("span.gpu-inspect-hint", "Inspect"), r.name) : h("div", r.title, r.name, r.pci);
    r.button = compact ? identity : null;
    r.cells = {};
    r.values = {};
    r.meters = {};
    const fields = compact ? ["load", "mem", "edge", "pow", "warnings"]
      : ["load", "mem", "pow", "edge", "junction", "memtemp", "sclk", "mclk", "ecc"];
    r.tr = h("tr", { role: "row" }, h("th.gpu-identity", { scope: "row", role: "rowheader" }, identity));
    for (const [index, field] of fields.entries()) {
      const header = columns[index + 1];
      const cell = h("td", { class: field === "warnings" ? "gpu-warnings" : field === "ecc" ? "" : "num",
        role: "cell", "data-label": typeof header === "string" ? header : header.label });
      r.cells[field] = cell;
      if (compact) {
        r.values[field] = h("span");
        cell.append(r.values[field]);
        if (field === "load" || field === "mem") {
          const meter = h("meter.gpu-meter", { min: "0", max: "100", value: "0", "aria-hidden": "true" });
          if (field === "mem") meter.classList.add("gpu-meter-memory");
          r.meters[field] = meter;
          cell.append(meter);
        }
      }
      r.tr.append(cell);
    }
    if (!compact) { r.ecc = h("span.badge"); r.cells.ecc.append(r.ecc); }
    rows.set(key, r);
    return r;
  };
  const update = (nextGpus, nextSamples, nextChecks, meta = {}) => {
    const byIndex = new Map((nextSamples || []).map((s) => [s.hip_index, s]));
    const cards = new Map((nextGpus || []).map((g) => [g.hip_index, g]));
    if (!cards.size && !meta.inventoryKnown) for (const [i, s] of byIndex) cards.set(i, s);
    const wanted = new Set(), incoming = [];
    for (const [i, hw] of [...cards].sort((a, b) => a[0] - b[0])) {
      const fingerprint = [hw.device_id, hw.sysfs_path, hw.name, hw.gfx, hw.vram_bytes > 0 ? hw.vram_bytes : ""].filter(Boolean).join("|");
      const key = hw.pci ? "pci:" + hw.pci.toLowerCase() : "hip:" + i + (fingerprint ? "|" + fingerprint : "");
      let r = rows.get(key);
      // Enrich an unidentified telemetry row without replacing its control.
      if (!r) r = [...rows.values()].find((old) => old.hip === i && !old.hw.pci && !old.fingerprint);
      if (r && r.key !== key) {
        rows.delete(r.key);
        if (selected === r.key) selected = key;
        r.key = key;
        rows.set(key, r);
      }
      if (!r) r = row(key);
      r.hw = hw;
      r.fingerprint = fingerprint;
      r.sample = byIndex.get(i) || null;
      r.hip = i;
      wanted.add(key);
      incoming.push(r);
    }
    for (const [key, r] of rows) if (!wanted.has(key)) {
      if (r.tr.contains(document.activeElement)) el.focus({ preventScroll: true });
      r.tr.remove();
      rows.delete(key);
      if (selected === key) selected = null;
    }
    // Keep physical-device order and existing controls, including keyboard focus.
    items = items.filter((r) => wanted.has(r.key));
    for (const r of incoming) if (!items.includes(r)) items.push(r);
    items.forEach((r, i) => { if (body.children[i] !== r.tr) body.insertBefore(r.tr, body.children[i] || null); });
    for (const r of items) {
      const hw = r.hw, raw = r.sample, s = meta.stale ? null : raw, cells = compact ? r.values : r.cells;
      text(r.title, "GPU " + r.hip);
      text(r.name, hw.name || hw.gfx || "Model unknown");
      if (r.button) r.button.setAttribute("aria-label", "Inspect GPU " + r.hip + " · " + (hw.name || hw.gfx || "Model unknown"));
      if (r.pci) text(r.pci, [hw.pci ? "PCI " + hw.pci : "PCI unknown", hw.gfx].filter(Boolean).join(" · "));
      const total = s && s.vram_total > 0 ? s.vram_total : hw.vram_bytes > 0 ? hw.vram_bytes : null;
      text(cells.load, value(s && s.busy_pct, " %"));
      text(cells.mem, (s && Number.isFinite(s.vram_used) ? num(s.vram_used / GiB) : "–")
        + " / " + (total == null ? "–" : num(total / GiB)) + " GiB");
      if (compact) {
        for (const [key, reading] of [["load", s && s.busy_pct],
          ["mem", s && total && Number.isFinite(s.vram_used) ? s.vram_used / total * 100 : null]]) {
          const known = Number.isFinite(reading);
          r.meters[key].hidden = !known;
          if (known) r.meters[key].value = Math.min(100, Math.max(0, reading));
        }
      }
      text(cells.pow, value(s && s.power_w, "")
        + (s && s.power_cap_w > 0 ? " / " + num(s.power_cap_w) : "") + " W");
      text(cells.edge, value(s && s.temp_c, " °C"));
      const check = (nextChecks || []).find((c) => c.id === "gpu.ecc." + r.hip);
      const ecc = check && check.status || "unknown";
      const warnings = [];
      if (meta.stale) warnings.push("Readings stale or unavailable");
      else if (meta.paused) warnings.push("Paused snapshot");
      if (meta.slowNote) warnings.push(meta.slowNote);
      if (!raw) warnings.push("No readings");
      else if (!meta.stale) {
        const missing = [];
        for (const [field, label] of [["busy_pct", "activity"], ["vram_used", "VRAM use"],
          ["temp_c", "temperature"], ["power_w", "power"]]) {
          if (!Number.isFinite(raw[field])) missing.push(label);
        }
        if (total == null) missing.push("capacity");
        if (missing.length) warnings.push("Unknown: " + missing.join(", "));
      }
      let critical = ecc === "warn" || ecc === "fail";
      const limits = hw.temp_limits_c || {};
      for (const [field, limit, label] of [["temp_c", limits.edge, "Edge"],
        ["temp_junction_c", limits.junction, "Junction"], ["temp_mem_c", limits.mem, "Memory"]]) {
        if (raw && Number.isFinite(raw[field]) && limit != null && raw[field] >= limit) {
          critical = true;
          warnings.push((meta.stale ? "Last " : "") + label + " temperature " + num(raw[field])
            + " °C ≥ " + num(limit) + " °C limit");
        }
      }
      if (critical && check && (ecc === "warn" || ecc === "fail")) warnings.push("ECC " + ecc + ": " + (check.detail || "GPU memory check"));
      else if (ecc !== "ok") warnings.push("ECC unknown");
      r.note = warnings.join(" · ") || "No reported warnings";
      if (compact) {
        text(cells.warnings, r.note);
        cells.warnings.classList.toggle("gpu-critical", critical);
        r.tr.dataset.state = critical ? "warn" : meta.stale ? "stale" : "normal";
      } else {
        for (const [field, reading, unit] of [["junction", "temp_junction_c", " °C"],
          ["memtemp", "temp_mem_c", " °C"], ["sclk", "sclk_mhz", " MHz"], ["mclk", "mclk_mhz", " MHz"]]) {
          text(cells[field], value(s && s[reading], unit));
        }
        text(r.ecc, ecc);
        r.ecc.className = "badge b-" + (check ? ecc : "skip");
      }
    }
    select(selected);
  };
  update(gpus, samples, checks);
  return { el: el, update: update, select: select, selected: () => rows.get(selected) || null, items: () => items };
}
