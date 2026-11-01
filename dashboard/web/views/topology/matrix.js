"use strict";
// Codes describe physical paths, not measured performance.
const TOPO_CLASS_WORD = {
  PIX: "same PCIe switch",
  PXB: "across PCIe switches",
  PHB: "through the CPU host bridge",
  SYS: "across host bridges",
};

// Closest physical path first.
const TOPO_CLASS_ORDER = ["PIX", "PXB", "PHB", "SYS"];

/** topoClassWords is the meaning of one class, in one sentence. */
function topoClassWords(cls) {
  switch (cls) {
    case "PIX": return "Both cards use the same PCIe switch. Actual speed is measured separately.";
    case "PXB": return "The path crosses several PCIe switches without crossing a host bridge.";
    case "PHB": return "The path passes through the CPU's host bridge.";
    case "SYS": return "The path crosses host bridges; direct-copy support is reported separately.";
    default: return "The physical path is not known.";
  }
}

/** Direct versus host-staged copies are readable without PCIe terminology. */
function topoMatrix(data) {
  const nodes = (data && data.nodes) || [];
  const pairs = (data && data.pairs) || [];
  if (nodes.length < 2) {
    return emptyState("There is no matrix to show.",
      "The matrix compares every pair of cards. It needs at least two cards on this host.",
      "bernie topo");
  }
  const idx = nodes.map((n) => n.hip_index).sort((a, b) => a - b);
  const pairOf = (a, b) => pairs.find((p) => (p.a === a && p.b === b) || (p.a === b && p.b === a));
  const head = (text) => h("th", { scope: "col" }, text);
  const rows = idx.map((a) => h("tr", null,
    h("th", { scope: "row" }, h("span.topo-matrix-name", "GPU " + a)),
    idx.map((b) => {
      if (a === b) return h("td.self", "—");
      const p = pairOf(a, b);
      if (!p) {
        return h("td", { class: "topo-cell topo-cell-unknown",
          title: "GPU " + a + " and GPU " + b + ": the path is not known" }, "Unknown");
      }
      const word = TOPO_CLASS_WORD[p.class] || "the path is not known";
      const title = "GPU " + a + " and GPU " + b + ": " + p.class + ", " + word +
        ", " + (p.hops != null ? p.hops + " hops" : "the number of hops is not known") +
        ", " + (p.p2p ? "a direct copy is possible" : "the copy goes through the host");
      return h("td", { class: "topo-cell topo-c-" + p.class, title: title },
        h("span.topo-cell-code.regular-only", p.p2p ? "Direct" : "Via host"),
        h("span.topo-cell-code.advanced-only", p.class),
        h("span.topo-cell-mark", { "aria-hidden": "true" }, p.p2p ? "●" : "○"),
        p.gbps ? h("span.topo-cell-gbps.advanced-only", num(p.gbps) + " GB/s") : null);
    })));
  const table = h("div.scroll", { tabindex: "0", role: "region", "aria-label": "GPU connection matrix; scroll to compare all pairs" }, h("table.matrix.topo-matrix",
    h("thead", h("tr", head(""), idx.map((i) => head(h("div.topo-matrix-head", "GPU " + i))))),
    h("tbody", rows)));
  return table;
}

/** Saved measured speeds; missing cells are not zero-speed results. */
function topoBandwidthMatrix(data) {
  const nodes = (data && data.nodes) || [];
  const pairs = (data && data.pairs) || [];
  if (nodes.length < 2) {
    return emptyState("There is no matrix to show.",
      "The matrix compares every pair of cards. It needs at least two cards on this host.",
      "bernie topo");
  }
  const measured = pairs.filter((p) => p.gbps);
  if (!measured.length) {
    return emptyState("The bandwidth of this host is not measured.",
      "The measurement runs copies between the cards and takes minutes. The command below measures the GB/s of every pair and saves it, so the page shows it afterwards.",
      "bernie topo --bandwidth");
  }
  const idx = nodes.map((n) => n.hip_index).sort((a, b) => a - b);
  const pairOf = (a, b) => pairs.find((p) => (p.a === a && p.b === b) || (p.a === b && p.b === a));
  const peak = Math.max.apply(null, measured.map((p) => p.gbps));
  // Numbers carry the meaning; heat is only a secondary cue.
  const heat = (v) => {
    if (!v || !peak) return "";
    const r = v / peak;
    if (r >= 0.75) return " topo-bw-4";
    if (r >= 0.5) return " topo-bw-3";
    if (r >= 0.25) return " topo-bw-2";
    return " topo-bw-1";
  };
  const head = (text) => h("th", { scope: "col" }, text);
  const rows = idx.map((a) => h("tr",
    h("th", { scope: "row" }, h("span.topo-matrix-name", "GPU " + a)),
    idx.map((b) => {
      if (a === b) return h("td.self", "—");
      const p = pairOf(a, b);
      if (!p) {
        return h("td", { class: "topo-cell topo-cell-unknown",
          title: "GPU " + a + " and GPU " + b + ": the path is not known" }, "Unknown");
      }
      const gbps = p.gbps || 0;
      const word = gbps ? num(gbps) + " GB/s" : "not measured";
      const title = "GPU " + a + " and GPU " + b + ": " + word +
        ", " + (TOPO_CLASS_WORD[p.class] || "the path is not known") +
        ", " + (p.p2p ? "a direct copy is possible" : "the copy goes through the host");
      return h("td", { class: "topo-cell" + heat(gbps), title: title },
        h("span.topo-cell-gbps", word),
        h("span.topo-cell-code.advanced-only", p.class),
        h("span.topo-cell-mark.advanced-only", { "aria-hidden": "true" }, p.p2p ? "●" : "○"));
    })));
  const table = h("div.scroll", { tabindex: "0", role: "region", "aria-label": "Measured GPU transfer speeds; scroll to compare all pairs" }, h("table.matrix.topo-matrix.topo-bw-matrix",
    h("thead", h("tr", head(""), idx.map((i) => head(h("div.topo-matrix-head", "GPU " + i))))),
    h("tbody", rows)));
  return table;
}

/** Full pair evidence, disclosed once rather than repeated in every card. */
function topoPairTable(data) {
  const pairs = (data && data.pairs) || [];
  if (!pairs.length) {
    return emptyState("There are no pairs to show.",
      "Pairs need two detected GPUs. Re-check GPU detection in Setup if you expected more.", "bernie topo");
  }
  const cols = tableHead(["Pair", "Physical path", "Hops", "Copy path", "Measured speed"]);
  const rows = pairs.map((p) => h("tr",
    h("td", "GPU " + p.a + " and GPU " + p.b),
    h("td", h("span", { class: "topo-pair-class topo-c-" + p.class }, p.class),
      h("span.topo-pair-word", TOPO_CLASS_WORD[p.class] || "the path is not known")),
    h("td.num", p.hops == null ? "–" : String(p.hops)),
    h("td", p.p2p ? statePill("ok", "direct copy") : statePill("skip", "through the host")),
    h("td.num", p.gbps ? num(p.gbps) + " GB/s" : "not measured")));
  return h("div.scroll", h("table", cols, h("tbody", rows)));
}
