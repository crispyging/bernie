"use strict";
// Host-wide comparison and one selected-device inspector.
// Adapted from vllm-rdna-dash by CrispyGinger, used with permission.

// Missing measurements stay missing, never zero.
function dashTile(spec) {
  const el = h("div.tile", { class: "dash-tile" });
  const label = h("div.tile-l", spec.label);
  if (spec.tip) label.append(dashInfo(spec.tip));
  const value = h("div.tile-n");
  const amount = h("span", spec.value);
  const unit = h("small", spec.unit || "");
  value.append(amount, unit);
  const sub = h("div.tile-s", spec.sub || "");
  const sparkBox = h("div", { class: "dash-tile-spark" });
  const chart = spark(spec.series || [], spec.max, spec.label, { unit: spec.unit, format: spec.format });
  sparkBox.append(chart);
  el.append(label, value, sub, sparkBox);
  el.update = (next) => {
    amount.textContent = next.value;
    unit.textContent = next.unit === undefined ? (spec.unit || "") : next.unit;
    unit.style.opacity = next.value === "–" ? ".55" : "1";
    sub.textContent = next.sub || "";
    chart.update(next.series || [], next.max);
  };
  return el;
}


// Keep memory-health warnings visible outside the technical disclosure.
function dashEcc(hip) {
  const checks = (dash.doctor && dash.doctor.checks) || [];
  const c = checks.find((x) => x.id === "gpu.ecc." + hip);
  if (!c) return { kind: "skip", note: "GPU memory health has not been checked." };
  const kind = c.status === "ok" ? "ok" : c.status === "warn" ? "warn" : c.status === "fail" ? "fail" : "skip";
  return { kind: kind, note: c.detail || c.status };
}

function dashGPUOverview(section) {
  const scope = h("p.dash-gpu-scope");
  const plan = h("p.dash-gpu-scope");
  const restriction = h("span");
  const note = h("p.dash-gpu-scope", "Host-wide readings · other workloads may contribute. ", restriction);
  const title = h("summary", "GPU inspector · choose Inspect in the comparison");
  const status = h("p.dash-gpu-scope", { "aria-live": "polite" });
  const health = h("p.dash-gpu-scope");
  const facts = h("dl.dash-gpu-facts");
  const labels = ["Architecture", "PCI address", "PCIe link", "Shader clock", "Memory clock", "Junction temperature", "Memory temperature"];
  const values = labels.map((label) => {
    const value = h("dd");
    facts.append(h("div", h("dt", label), value));
    return value;
  });
  const history = spark([], 100, "Selected GPU activity, host-wide", { unit: "%" });
  const fields = h("div.dash-gpu-fields", facts, health,
    h("h3", "Activity history · last 60 samples"), history);
  const inspector = h("details.dash-gpu-inspector", { id: "dashboard-gpu-inspector" }, title, status, fields);
  const histories = new Map();
  let first = true, initialized = false, lastDocument = null, lost = "", freshness = {};
  const inspect = () => {
    const selected = table.selected();
    fields.hidden = !selected;
    const message = selected ? selected.note : lost || "Select a GPU above for its exact readings and activity history.";
    if (status.textContent !== message) status.textContent = message;
    if (!selected) {
      title.textContent = "GPU inspector · choose Inspect in the comparison";
      return;
    }
    title.textContent = "GPU " + selected.hip + " · " + (selected.hw.name || selected.hw.gfx || "Model unknown");
    const ecc = dashEcc(selected.hip);
    health.textContent = "Memory check: " + ecc.note;
    health.classList.toggle("gpu-critical", ecc.kind === "warn" || ecc.kind === "fail");
    const hw = selected.hw, sample = freshness.stale ? null : selected.sample;
    const reading = (name, unit) => sample && Number.isFinite(sample[name]) ? num(sample[name]) + unit : "–";
    const info = [hw.gfx || "Not reported", hw.pci || "Not reported",
      hw.link && hw.link.gen > 0 && hw.link.width > 0 ? "Gen " + hw.link.gen + " ×" + hw.link.width : "Not reported",
      reading("sclk_mhz", " MHz"), reading("mclk_mhz", " MHz"),
      reading("temp_junction_c", " °C"), reading("temp_mem_c", " °C")];
    values.forEach((node, i) => { if (node.textContent !== info[i]) node.textContent = info[i]; });
    history.update(((histories.get(selected) || {}).values || []).slice(), 100);
  };
  const table = gpuValueTable([], [], [], {
    compact: true, controls: inspector.id,
    onInspect: () => { lost = ""; inspector.open = true; inspect(); },
  });
  section.append(h("div.dash-gpu-heading", scope, h("a.btn.small", { href: "#topology" }, "GPU links")), plan, note, table.el, inspector);
  const api = {
    update: () => {
      const doc = dash.tele;
      const now = state.livePaused ? state.pauseStarted || Date.now() : Date.now();
      const age = doc && doc.slow_age_s, interval = doc && doc.slow_interval_s;
      freshness = {
        stale: !!(doc && (doc.error || (doc.time && now - doc.time > STALE_MS))),
        paused: !!state.livePaused,
        inventoryKnown: !!(dash.hw && Array.isArray(dash.hw.gpus)),
        slowNote: interval > 0 && age > 2 * interval ? "Temperature / cap / clocks " + Math.round(age) + " s old" : "",
      };
      const previous = table.selected();
      table.update((dash.hw && dash.hw.gpus) || [], (doc && doc.gpus) || [],
        (dash.doctor && dash.doctor.checks) || [], freshness);
      const items = table.items();
      table.el.hidden = !items.length;
      if (previous && !table.selected()) lost = "Previously inspected GPU " + previous.hip
        + (previous.hw.pci ? " (" + previous.hw.pci + ")" : "") + " changed or is no longer visible. Choose again.";
      const changed = !!doc && doc !== lastDocument;
      for (const r of items) {
        let series = histories.get(r);
        const seed = !series && first && dash.samples.length > 0;
        if (!series || series.key !== r.key) {
          series = { key: r.key, values: seed ? dash.samples.slice(-60).map((s) => {
            const c = s.cards.find((g) => r.hw.pci ? (g.pci || "").toLowerCase() === r.hw.pci.toLowerCase() : g.hip === r.hip);
            return c && Number.isFinite(c.busy) ? c.busy : null;
          }) : [] };
          histories.set(r, series);
        }
        if (changed && !seed) {
          series.values.push(!freshness.stale && r.sample && Number.isFinite(r.sample.busy_pct) ? r.sample.busy_pct : null);
          if (series.values.length > 60) series.values.shift();
        }
      }
      for (const r of histories.keys()) if (!items.includes(r)) histories.delete(r);
      if (items.length) first = false;
      lastDocument = doc;
      if (!initialized && items.length) {
        initialized = true;
        if (items.length === 1) { table.select(items[0].key); inspector.open = true; }
      }
      if (items.length) scope.textContent = items.length + (items.length === 1 ? " GPU" : " GPUs")
        + (freshness.inventoryKnown ? " visible to bernie." : " reporting telemetry; serving visibility is unknown.");
      else if (!dash.hw && !doc) scope.textContent = (dash.errors || {})["/api/hw"]
        ? "GPU inventory unavailable: " + dash.errors["/api/hw"] : "Waiting for GPU data…";
      else scope.textContent = "No GPUs are visible to bernie.";
      const run = dashLiveRun();
      const selected = run && !run.foreign && run.plan && Array.isArray(run.plan.gpus) ? run.plan.gpus : null;
      plan.textContent = !run ? "Configured run plan: none."
        : selected ? (run.state === "exited" ? "Last configured plan" : "Configured run plan") + " (" + run.name + "): "
          + (selected.length ? selected.map((i) => "GPU " + i).join(", ") + " · " + selected.length + " selected." : "no GPUs listed.")
          : (run.foreign ? "Attached server" : "Run plan") + " GPU selection: unknown.";
      restriction.textContent = dash.hw && dash.hw.restriction ? " · " + dash.hw.restriction : "";
      inspect();
    },
  };
  api.update();
  return api;
}

function renderGPUs() {
  if (!state.livePaused && state.tele && state.tele !== dash.tele) dashSample(state.tele);
  else dashUpdateSimple();
}

// Subscribe once; renderGPUs may already have consumed this frame.
function dashSubscribe() {
  if (dash.subscribed) return;
  dash.subscribed = true;
  if (typeof onTabFrame !== "function") return;
  onTabFrame("dashboard", (doc) => { if (!state.livePaused && doc && doc !== dash.tele) dashSample(doc); });
}
