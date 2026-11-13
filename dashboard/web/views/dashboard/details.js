"use strict";
// Supplementary history and raw snapshots load only when requested.
const DASH_DETAIL_WINDOWS = ["5m", "15m", "1h"];

function dashDetails() {
  const body = h("div.dash-details-body", loading());
  const det = h("details.advanced.dash-details", h("summary", "Host history, power policy and raw snapshots"), body);
  let loaded = false;
  det.addEventListener("toggle", async () => {
    if (!det.open || loaded) return;
    loaded = true;
    await dashLoadDetails(view);
  });
  const view = {
    el: det,
    body: body,
  };
  return view;
}

async function dashLoadDetails(view) {
  const body = view.body;
  const power = await api("/api/power").catch(() => null);
  const docs = [];
  await dashDetailsRead(docs, "/api/telemetry", dash.tele);
  await dashDetailsRead(docs, "/api/hw", dash.hw);
  await dashDetailsRead(docs, "/api/runs", state.runs);
  await dashDetailsRead(docs, "/api/gpus/history?window=5m", dash.historyOK ? dash.history : undefined);
  put(body,
    dashDetailsWindow(),
    dashDetailsPower(power),
    dashDetailsDocuments(docs));
}

// Failed reads remain independent of the other diagnostic documents.
async function dashDetailsRead(entries, path, doc) {
  const error = (dash.errors || {})[path];
  if (error) { entries.push({ path: path, error: error }); return; }
  if (doc !== undefined) {
    entries.push({ path: path, doc: doc });
    return;
  }
  try {
    entries.push({ path: path, doc: await api(path) });
  } catch (e) {
    entries.push({ path: path, error: String(e.message || e) });
  }
}

function dashDetailsWindow() {
  const card = h("div.card");
  const seg = h("div.segmented-control", { role: "group", "aria-label": "The window of the history" });
  const note = h("p.small.muted");
  const chart = areaChart([], { title: "Average GPU activity", unit: "%", windowMinutes: 5,
    hint: "Load history needs measured GPU samples; missing readings stay gaps." });
  let win = "5m";
  const buttons = {};
  for (const w of DASH_DETAIL_WINDOWS) {
    const button = h("button", { type: "button", "aria-pressed": String(w === win) }, w);
    button.addEventListener("click", () => {
      win = w;
      for (const [id, other] of Object.entries(buttons)) other.setAttribute("aria-pressed", String(id === win));
      paint();
    });
    buttons[w] = button;
    seg.append(button);
  }
  const paint = async () => {
    put(note, "Reading the history of the cards…");
    let doc = null;
    try {
      doc = await api("/api/gpus/history?window=" + win);
    } catch (e) {
      put(note, "The history could not be read: " + (e.message || e));
      chart.update([], dashWindowMinutes(win));
      return;
    }
    const series = (doc && doc.series) || [];
    const n = series.length ? (series[0].busy_pct || []).length : 0;
    const mean = [];
    for (let i = 0; i < n; i++) {
      const values = series.map((s) => (s.busy_pct || [])[i]).filter((v) => v != null);
      mean.push(values.length ? values.reduce((a, b) => a + b, 0) / values.length : null);
    }
    const minutes = dashWindowMinutes(win);
    chart.update(dashGroup(mean, doc && doc.interval_s), minutes);
    put(note, mean.some((v) => v != null)
      ? "Last " + ((doc && doc.window) || win) + ", sampled every " + ((doc && doc.interval_s) || "–") + " s. Average activity across GPUs."
      : "No GPU activity readings are available for this window. Check Setup if no GPU is visible.");
  };
  put(card, cardHead("GPU activity history", "/api/gpus/history"), seg, note, chart);
  paint();
  return card;
}

function dashWindowMinutes(win) {
  if (win === "1h") return 60;
  if (win === "15m") return 15;
  return 5;
}

// Read-only power limits; changes remain on the Advanced tab.
function dashDetailsPower(power) {
  const card = h("div.card");
  if (!power) {
    put(card, cardHead("The power caps", "/api/power"),
      emptyState("The power caps are not available.",
        "The caps of the cards come from the power service of the host.",
        "bernie tune power status"));
    return card;
  }
  const rows = (power.gpus || []).map((g) => h("tr",
    h("td", "GPU " + g.hip_index),
    h("td.num", Math.round(g.cap_w) + " W"),
    h("td.num", Math.round(g.min_w || 1) + "–" + Math.round(g.max_w || g.cap_w || 1) + " W"),
    h("td.num", g.desired_w == null ? "–" : Math.round(g.desired_w) + " W"),
    h("td", g.drift ? h("span.badge.b-warn", "changed outside bernie") : h("span.badge.b-ok", "in sync"))));
  put(card,
    cardHead("The power caps", "/api/power",
      power.installed ? null : h("span.badge.b-warn", "not installed")),
    rows.length
      ? h("div.scroll", h("table",
        tableHead(["GPU", { label: "Current", num: true }, { label: "Allowed", num: true },
          { label: "Desired", num: true }, "State"]),
        h("tbody", rows)))
      : emptyState("No GPU publishes a hwmon power cap.",
        "The caps come from the power service of the host.",
        "bernie tune power status"));
  return card;
}

function dashDetailsDocuments(entries) {
  return h("div.dash-details-documents",
    h("p.small.muted", "Snapshots captured when this panel opens; they do not update live."),
    entries.map((entry) => {
      const text = entry.error ? entry.error : entry.doc == null ? "No document has arrived yet."
        : JSON.stringify(entry.doc, null, 2);
      return h("details.dash-details-document",
        h("summary", entry.path, entry.error ? " · unavailable" : entry.doc == null ? " · waiting for data" : ""),
        h("div.dash-details-head", jsonLink(entry.path), copyBtn(text, "Copy " + entry.path)),
        h("pre.dash-details-json", text));
    }));
}
