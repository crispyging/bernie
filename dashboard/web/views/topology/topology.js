"use strict";
// GPU connections: readiness first, measured speed second, physical wiring on demand.
// Adapted from vllm-rdna-dash by CrispyGinger, used with permission.
const topo = {
  data: null,   // GET /api/topo
  check: null,  // GET /api/topo/check
  errors: {},   // the path of a failed read -> the message
  live: {},     // the parts a repaint touches
};

/** Section IDs retain deep links from the router. */
function topoSection(label, id, ...kids) {
  return h("section.section", { id: "topology-" + id, "data-section": id },
    sectionLabel(label), ...kids);
}

/** Read-only inspection; re-checking does not run a bandwidth benchmark. */
async function viewTopology(root) {
  put(root, loading("Reading GPU connections…"));
  const check = h("button.primary", {
    type: "button",
    onclick: () => topoCheck(),
  }, "Check readiness");
  const head = stateHead("GPU connections", h("div.row", check));
  const scope = h("p.small.muted");
  const classes = h("div.card");
  const tree = h("div.card");
  const list = h("div.card");
  const bw = h("div.card");
  const raw = topoAdvanced();
  put(root, head, scope,
    topoSection("Readiness and next steps", "checklist", list),
    topoSection("How GPUs copy to each other", "classes", classes),
    topoSection("Measured transfer speed", "bandwidth", bw),
    topoSection("Physical wiring", "tree",
      h("details.advanced", h("summary", "PCIe tree and hardware addresses"), tree)),
    raw.el);
  topo.live = { head, scope, classes, tree, list, bw, raw };
  await topoLoad();
}

/** Read failures independently; a missing document must not look like no GPUs. */
async function topoLoad() {
  const one = async (path, key) => {
    try { topo[key] = await api(path); delete topo.errors[path]; }
    catch (e) {
      topo[key] = null;
      topo.errors[path] = e.message || String(e);
    }
  };
  await Promise.all([
    one("/api/topo", "data"),
    one("/api/topo/check", "check"),
    loadHW().catch(() => null),
  ]);
  topoPaint();
}

/** Drop cached host facts before reading them again. */
async function topoCheck() {
  const btn = topo.live.head && topo.live.head.querySelector("button.primary");
  if (btn) btn.disabled = true;
  try { await refreshHost(); } catch { /* the cache drop is best-effort */ }
  await topoLoad();
  if (btn) btn.disabled = false;
}

/** Keep the visible verdict and its raw evidence from the same read. */
function topoPaint() {
  if (!topo.live.head) return;
  const nodes = (topo.data && topo.data.nodes) || [];
  const pairs = (topo.data && topo.data.pairs) || [];
  const direct = pairs.filter((p) => p.p2p).length;
  const visible = state.hw && Array.isArray(state.hw.gpus) ? state.hw.gpus.length : null;
  topo.live.scope.textContent = (topo.data ? nodes.length + " AMD GPUs detected in topology. " : "Detected GPU count unavailable. ")
    + (visible == null ? "Serving visibility unavailable." : visible + " visible to bernie for serving.")
    + (state.hw && state.hw.restriction ? " " + state.hw.restriction + "." : "")
    + " Connections and readiness below cover detected GPUs, not a run’s selection. GPU numbers are host HIP indices.";

  put(topo.live.classes, cardHead("GPU copy paths", "/api/topo"),
    topo.errors["/api/topo"] ? failed({ message: topo.errors["/api/topo"], path: "/api/topo" })
      : h("div", topoMatrix(topo.data),
        h("p.small.regular-only", "Direct copies avoid a trip through system memory. Via host uses the CPU and system memory and may add overhead. Neither label is a speed measurement."),
        h("div.advanced-only", topoLegend()),
        h("details.advanced", h("summary", "Every GPU pair: connection details"), topoPairTable(topo.data))));
  put(topo.live.tree, cardHead("Physical GPU wiring", "/api/topo"),
    topo.errors["/api/topo"] ? failed({ message: topo.errors["/api/topo"], path: "/api/topo" }) : topoTree(topo.data));
  put(topo.live.list, cardHead("Readiness for direct GPU copies", "/api/topo/check"), topoChecklist(topo.check));
  put(topo.live.bw, cardHead("Measured transfer speed", "/api/topo"), topoBandwidth(topo.data));
  topo.live.raw.update();

  topoSetHead(nodes, pairs, direct);
}

/** Layout, readiness and measured speed are distinct kinds of evidence. */
function topoSetHead(nodes, pairs, direct) {
  const head = topo.live.head;
  if (!head) return;
  const summary = (topo.check && topo.check.summary) || {};
  const fails = summary.fail || 0, warns = summary.warn || 0, skipped = summary.skip || 0;
  if (!topo.data) {
    setHead(head, fails ? "GPU checks found blockers; connections are unknown." : "GPU connections could not be read.",
      "The hardware layout is unavailable, not empty. Review any readiness results below, then press Check readiness to try again.");
    return;
  }
  if (nodes.length < 2) {
    setHead(head, "No multi-GPU connection to compare.",
      "Found " + (nodes.length ? "one detected GPU." : "no detected GPUs.") +
      " Single-GPU serving does not need peer copies. If you expected more GPUs, check Setup and then re-check readiness.");
    return;
  }
  const expected = nodes.length * (nodes.length - 1) / 2;
  const pairWord = direct + " of " + expected + " GPU pairs report a direct connection. ";
  if (fails) {
    setHead(head, "Direct GPU copies are blocked.",
      fails + " readiness checks failed. Start with their next steps below, then re-check. " + pairWord);
  } else if (!topo.check || !(topo.check.checklist || []).length) {
    setHead(head, "The GPU layout is known; readiness is not.",
      pairWord + "The checklist is unavailable or empty. Press Check readiness; a visible connection alone does not establish working copies.");
  } else if (warns || skipped) {
    setHead(head, "GPU connections need review.", pairWord + "No blocking check was reported, but " +
      warns + " warnings and " + skipped + " unchecked items remain. Review them below; speed is measured separately.");
  } else if (pairs.length < expected) {
    setHead(head, "Some GPU connections are unknown.", pairWord + "Re-check readiness before relying on the missing paths.");
  } else {
    setHead(head, direct < expected ? "Some GPUs copy through the host." : "Direct GPU connections are available.",
      pairWord + "Readiness checks passed. See measured transfer speed below before drawing conclusions about performance.");
  }
}
