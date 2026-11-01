"use strict";
// ---------- Topology ----------
// The Topology tab answers one question: can the cards copy to each other?
//
// Two endpoints of spec §11 carry the answer: GET /api/topo (the PCIe tree
// and the link class of every pair of cards) and GET /api/topo/check (the
// readiness checklist of `bernie topo --check`). The view opens no fetch of
// its own: it asks data.js.
//
// Adapted from vllm-rdna-dash by CrispyGinger (Apache-2.0).

// topo is the state of the tab.
const topo = {
  data: null,   // GET /api/topo
  check: null,  // GET /api/topo/check
  errors: {},   // the path of a failed read -> the message
  live: {},     // the parts a repaint touches
};

/** topoSection is one block of the tab: a rule on top, the name of the
 *  block and what it holds. The name is a heading the router can focus. */
function topoSection(label, id, ...kids) {
  return h("section.section", { id: "topology-" + id, "data-section": id },
    sectionLabel(label), ...kids);
}

/** viewTopology opens the tab. The router calls it with the panel. */
async function viewTopology(root) {
  put(root, loading("Reading the PCIe tree…"));
  const check = h("button.primary", {
    type: "button",
    onclick: () => topoCheck(),
  }, "Check readiness");
  const head = stateHead("Topology", h("div.row", check));
  const classes = h("div.card");
  const tree = h("div.card");
  const list = h("div.card");
  const bw = h("div.card");
  put(root, head,
    topoSection("THE LINK CLASSES", "classes", classes),
    topoSection("THE PCIE TREE", "tree", tree),
    topoSection("THE READINESS CHECKLIST", "checklist", list),
    topoSection("THE BANDWIDTH", "bandwidth", bw),
    topoAdvanced());
  topo.live = { head: head, classes: classes, tree: tree, list: list, bw: bw };
  await topoLoad();
}

/** topoLoad reads the two documents of the tab. One failure does not stop
 *  the other: the card that failed says so. */
async function topoLoad() {
  const one = async (path, key) => {
    try { topo[key] = await api(path); }
    catch (e) {
      topo[key] = null;
      topo.errors[path] = e.message || String(e);
    }
  };
  await Promise.all([
    one("/api/topo", "data"),
    one("/api/topo/check", "check"),
  ]);
  topoPaint();
}

/** topoCheck reads the checklist again: the reader pressed "Check
 *  readiness" after changing something on the host. The button drops every
 *  cached answer of the host (POST /api/refresh) before it reloads, so a
 *  cable that changed or a kernel module that loaded shows up at once
 *  (spec §11, ia.md R27). */
async function topoCheck() {
  const btn = topo.live.head && topo.live.head.querySelector("button.primary");
  if (btn) btn.disabled = true;
  try { await refreshHost(); } catch { /* the cache drop is best-effort */ }
  await topoLoad();
  if (btn) btn.disabled = false;
}

/** topoPaint writes the headline and the cards of the tab. */
function topoPaint() {
  if (!topo.live.head) return;
  const nodes = (topo.data && topo.data.nodes) || [];
  const pairs = (topo.data && topo.data.pairs) || [];
  const direct = pairs.filter((p) => p.p2p).length;

  put(topo.live.classes,
    cardHead("Link classes", "/api/topo"),
    topo.errors["/api/topo"] ? failed({ message: topo.errors["/api/topo"], path: "/api/topo" })
      : h("div", topoMatrix(topo.data), topoLegend()),
    topo.data ? h("div", h("h3", { class: "topo-sub" }, "Every pair"), topoPairTable(topo.data)) : null);
  put(topo.live.tree,
    cardHead("The PCIe tree", "/api/topo"),
    topo.errors["/api/topo"] ? failed({ message: topo.errors["/api/topo"], path: "/api/topo" }) : topoTree(topo.data));
  put(topo.live.list,
    cardHead("P2P readiness", "/api/topo/check"),
    topo.errors["/api/topo/check"] ? failed({ message: topo.errors["/api/topo/check"], path: "/api/topo/check" }) : topoChecklist(topo.check));
  put(topo.live.bw,
    cardHead("Bandwidth", "/api/topo"),
    topoBandwidth(topo.data));

  topoSetHead(nodes, pairs, direct);
}

/** topoSetHead writes the state headline: one sentence about the situation
 *  and one line about what to do next. */
function topoSetHead(nodes, pairs, direct) {
  const head = topo.live.head;
  if (!head) return;
  const fails = topo.check && topo.check.summary ? (topo.check.summary.fail || 0) : 0;
  const warns = topo.check && topo.check.summary ? (topo.check.summary.warn || 0) : 0;
  if (nodes.length < 2) {
    setHead(head, "THIS HOST HAS TOO FEW CARDS TO COMPARE.",
      "A topology compares the cards with each other. This host has " +
      (nodes.length === 1 ? "one card." : "no card.") +
      " Add a card and press Check readiness.");
    return;
  }
  if (!topo.data && !topo.check) {
    setHead(head, "THE TOPOLOGY OF THIS HOST IS NOT KNOWN.",
      "Neither the PCIe tree nor the checklist could be read. Press Check readiness to read them again.");
    return;
  }
  const pairsWord = direct + " of " + pairs.length + " pairs can copy to each other directly.";
  if (fails) {
    setHead(head, "MULTI-GPU COPIES WILL NOT WORK.",
      pairsWord + " " + fails + (fails === 1 ? " check failed." : " checks failed.") +
      " The fix of every check is in the checklist below.");
    return;
  }
  if (warns) {
    setHead(head, "MULTI-GPU COPIES WILL WORK, BUT NOT AT FULL SPEED.",
      pairsWord + " " + warns + (warns === 1 ? " check warns." : " checks warn.") +
      " The checklist below says what they cost.");
    return;
  }
  setHead(head, "THE CARDS CAN COPY TO EACH OTHER.",
    pairsWord + " Every readiness check of this host passed.");
}
