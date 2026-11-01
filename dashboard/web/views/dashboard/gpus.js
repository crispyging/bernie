"use strict";
// ---------- Dashboard tiles and GPU cards ----------
// One reusable tile and one card per AMD GPU on the host.
//
// Adapted from vllm-rdna-dash by CrispyGinger (Apache-2.0).

// dashTile is one KPI tile: the name of the number, the number with its
// unit, one line of context and the last minute of it. A number that was not
// measured is a dash and a greyed unit, never a zero.
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
    if (next.unit != null) unit.textContent = next.unit;
    unit.style.opacity = next.value === "–" ? ".55" : "1";
    sub.textContent = next.sub || "";
    chart.update(next.series || [], next.max);
  };
  return el;
}

// dashCard is the card of one GPU: its name, its state, the ring of its
// load, the memory and the power meters, the temperatures, the clocks, the
// ECC state and the last minute of its load.
function dashCard(hip) {
  const hw = ((dash.hw && dash.hw.gpus) || []).find((g) => g.hip_index === hip) || null;
  const el = h("div.card", { class: "dash-card" });
  const top = h("div.gpu-top", { class: "dash-card-top" });
  const names = h("div", { class: "dash-min0" });
  const name = h("h3", { class: "dash-card-name" }, dashCardName(hip));
  const where = h("div.small-t", { class: "dash-card-where" }, (hw ? hw.pci : "") + (hw && hw.gfx ? " · " + hw.gfx : ""));
  const state = h("span.dash-pill-wrap");
  names.append(name, where);
  top.append(names, state);
  const mid = h("div.gpu-mid", { class: "dash-card-mid" });
  const ringBox = ring(null, "GPU " + hip);
  const stats = h("div.gpu-stats", { class: "dash-card-stats" });
  const mem = meter(null, null, { label: "Memory", unit: "GiB", format: (v) => num(v / (1024 * 1024 * 1024)) });
  const pow = meter(null, null, { label: "Power", unit: "W", sep: " of ", format: (v) => num(Math.round(v)) });
  const temps = h("div.stat-row", { class: "dash-stat-row" });
  const clocks = h("div.stat-row", { class: "dash-stat-row dash-stat-row-center" });
  const hot = h("div", { class: "dash-stat-hot" });
  const age = h("div", { class: "small muted" });
  stats.append(mem, pow, temps, hot, age, clocks);
  mid.append(ringBox, stats);
  const sparkBox = h("div.gpu-spark", { class: "dash-mt12" });
  const chart = spark([], 100, "the load of GPU " + hip, { unit: "%" });
  sparkBox.append(chart);
  el.append(top, mid, sparkBox);

  const pair = (label, value, cls) => h("span", { class: "dash-nowrap" },
    h("span", { class: "dash-muted" }, label + " "),
    h("b", { class: "dash-stat-value" + (cls ? " " + cls : "") }, value));

  el.update = () => {
    const sample = dash.tele ? (dash.tele.gpus || []).find((g) => g.hip_index === hip) : null;
    const last = dash.samples[dash.samples.length - 1];
    const card = last ? last.cards.find((c) => c.hip === hip) : null;
    const limits = (hw && hw.temp_limits_c) || {};
    const limit = limits.edge != null ? limits.edge : (hw && hw.power_cap_w ? null : null);
    name.textContent = dashCardName(hip);
    where.textContent = (hw ? hw.pci : "") + (hw && hw.gfx ? " · " + hw.gfx : "");
    const run = dashLiveRun();
    const inUse = !!run && run.state !== "exited";
    state.replaceChildren(statePill(inUse ? "ok" : "skip", inUse ? "In use" : "Idle"));
    ringBox.update(card ? card.busy : null);
    mem.update(card ? card.mem : null, card ? card.total : null);
    pow.update(card ? card.pow : null, card ? card.cap : null);
    const t = card ? card.tmp : null;
    const isHot = t != null && limit != null && t >= limit;
    temps.replaceChildren(
      pair("TEMP", t == null ? "–" : num(t) + " °C", isHot ? "dash-stat-serious" : ""),
      pair("JUNCTION", card && card.jtmp != null ? num(card.jtmp) + " °C" : "–"),
      pair("MEM", card && card.mtmp != null ? num(card.mtmp) + " °C" : "–"));
    hot.textContent = isHot ? "hot: the limit of this card is " + limit + " °C" : "";
    // The temperatures and the clocks are read every few seconds: the
    // card says how old they are when they get older than twice their own
    // interval.
    const ageNote = slowAgeNote();
    age.textContent = ageNote;
    age.hidden = !ageNote;
    const ecc = dashEcc(hip);
    clocks.replaceChildren(
      pair("SCLK", card && card.clk != null ? dashGroup(card.clk) + " MHz" : "–"),
      pair("MCLK", card && card.mclk != null ? dashGroup(card.mclk) + " MHz" : "–"),
      ecc.pill);
    chart.update(dashCardSeries(hip, "busy"), 100);
    if (sample && sample.hip_index === hip) { /* the sample is in the card list */ }
  };
  el.update();
  return el;
}

// dashEcc is the ECC state of a card: the check of the doctor whose id is
// gpu.ecc.<hip index>.
function dashEcc(hip) {
  const checks = (dash.doctor && dash.doctor.checks) || [];
  const c = checks.find((x) => x.id === "gpu.ecc." + hip);
  if (!c) {
    return { pill: statePill("skip", "unknown"), note: "The doctor has not checked the ECC counters yet." };
  }
  const kind = c.status === "ok" ? "ok" : c.status === "warn" ? "warn" : c.status === "fail" ? "fail" : "skip";
  return { pill: statePill(kind, c.status === "skip" ? "unknown" : c.status), note: c.detail || "" };
}

// dashCards is the section with one card per card of the host.
function dashCards(section) {
  const grid = h("div", { class: "dash-cards" });
  const note = h("div", { class: "dash-cards-note" });
  section.append(note, grid);
  let built = [];
  const build = () => {
    const hw = (dash.hw && dash.hw.gpus) || [];
    const tele = (dash.tele && dash.tele.gpus) || [];
    const hips = hw.length ? hw.map((g) => g.hip_index) : tele.map((g) => g.hip_index);
    if (hips.length === built.length && hips.every((v, i) => v === built[i])) return false;
    built = hips;
    grid.replaceChildren();
    for (const hip of hips) grid.append(dashCard(hip));
    return true;
  };
  const api = {
    update: () => {
      if (!build()) for (const c of grid.children) if (c.update) c.update();
      const hips = built.length;
      note.textContent = hips ? hips + (hips === 1 ? " card on this host." : " cards on this host.") : "No AMD GPU is visible to bernie.";
    },
  };
  api.update();
  return api;
}

// renderGPUs subscribes to the shared stream through the onTabFrame seam.
// The stream lives in js/data.js; this view opens no fetch and no timer of
// its own. One stream, one frame per sample, for the whole page. A hidden
// tab paints nothing: data.js pauses the poller and skips the frame.
function renderGPUs() {
  if (dash.live.simple) dash.live.simple.cards.update();
}

// dashSubscribe registers the repaint of this tab on the shared stream. The
// stream lives in js/data.js; this view opens no fetch and no timer of its
// own. One stream, one frame per sample. The subscription happens once: the
// shell calls renderGPUs() on every sample and on a resize.
function dashSubscribe() {
  if (dash.subscribed) return;
  dash.subscribed = true;
  if (typeof onTabFrame !== "function") return;
  onTabFrame("dashboard", (doc) => { if (doc) dashSample(doc); });
}
