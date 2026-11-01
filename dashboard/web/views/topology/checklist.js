"use strict";
// Warnings, blockers and unchecked items remain visible; passed evidence can collapse.
function topoChecklist(data) {
  if (!data) {
    return h("div", emptyState("Readiness could not be established.",
      "Press Check readiness to try again, or inspect the host from a terminal.", "bernie topo --check"),
      (topo.errors || {})["/api/topo/check"] ? h("p.err", topo.errors["/api/topo/check"]) : null);
  }
  const items = data.checklist || [];
  if (!items.length) return emptyState("No readiness results were returned.",
    "An empty checklist is not a pass. Press Check readiness to try again.", "bernie topo --check");
  const sum = data.summary || {};
  const single = topo.data && (topo.data.nodes || []).length < 2;
  const kind = sum.fail ? "fail" : sum.warn ? "warn" : sum.skip ? "skip" : "ok";
  const verdicts = { fail: "Direct copies are blocked. Resolve the failed checks first.",
    warn: "Review warnings before relying on direct copies; speed or stability may be affected.",
    skip: "No blocker was reported, but some checks could not run. Those results are unknown.",
    ok: "Readiness checks passed. Transfer speed still needs a separate measurement." };
  const titles = { kfd_links: "Direct GPU connections", large_bar: "Access to GPU memory",
    iommu: "Device address translation", acs: "Traffic routing between GPUs", link_speed: "GPU link speed",
    link_classes: "Physical copy paths", bandwidth_test: "Optional speed-test tool", recipe_env: "Server copy settings" };
  const row = (it) => {
    const status = it.status || "skip";
    const word = { fail: single ? "Check failed" : "Blocking", warn: "Warning", ok: "Passed", skip: "Not checked" }[status] || "Unknown";
    return h("div.topo-check-row",
      h("div.topo-check-head", statePill(status, word), h("b.topo-check-title", titles[it.id] || it.title || "Check result")),
      h("span.small.muted.advanced-only", (it.title || "") + " · " + (it.id || "")),
      it.detail ? h("div.topo-check-detail", it.detail) : null,
      it.fix ? h("div.topo-check-fix", h("b.small", "Next step"),
        it.id === "iommu" || it.id === "acs" ? cmdBlock(it.fix, "Copy the fix")
          : h("p.small", it.fix, " ", copyBtn(it.fix, "Copy this step"))) : null,
      it.doc ? h("a.topo-check-doc", { href: it.doc, target: "_blank", rel: "noopener noreferrer" }, "Read the instructions") : null);
  };
  const passed = items.filter((it) => it.status === "ok");
  const box = h("div", h("div.topo-verdict", statePill(single ? "skip" : kind, single ? "Not applicable" : kind === "ok" ? "Ready" : kind === "skip" ? "Unverified" : "Needs attention"),
    h("span.topo-verdict-text", single ? "Peer-copy readiness is not applicable with fewer than two detected GPUs. Hardware findings remain below." : verdicts[kind])),
    h("p.small.muted", "Some fixes change kernel or firmware settings and may need a reboot. Read the instructions first; this page only displays the steps."),
    items.filter((it) => it.status !== "ok").map(row),
    passed.length ? h("details.advanced", h("summary", passed.length + " passed checks"), passed.map(row)) : null);
  return box;
}
