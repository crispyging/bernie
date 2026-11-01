"use strict";
// Bandwidth is measured explicitly in a terminal, never by opening this view.
function topoBandwidth(data) {
  if (!data) return emptyState("Transfer speed is unknown.",
    "GPU connections could not be read. Press Check readiness before deciding whether a speed test is needed.", "bernie topo");
  if ((data.nodes || []).length < 2) return emptyState("There is no GPU pair to measure.",
    "A transfer-speed comparison needs two visible GPUs. Single-GPU serving does not need it.", "bernie topo");
  const pairs = data.pairs || [];
  const measured = pairs.filter((p) => p.gbps > 0);
  const caution = "This benchmark adds GPU load and can take minutes. Run it when the GPUs are idle, then press Check readiness to reload the saved results.";
  if (!measured.length) return emptyState("No transfer speeds have been measured.",
    "Connection checks do not measure performance. " + caution, "bernie topo --bandwidth");
  const when = data.measured_at ? ago(data.measured_at) : "";
  return h("div", h("p", measured.length + " of " + pairs.length + " GPU pairs have saved speed measurements."),
    h("p.small.muted", "Higher GB/s means faster copying. These are saved results, not live measurements. " +
      (when ? "Measured " + when + "." : "The measurement time is not available."),
      data.measured_source ? h("span.advanced-only", " Source: " + data.measured_source + ".") : null),
    topoBandwidthMatrix(data),
    h("details.advanced", h("summary", "Measure transfer speeds again"), h("p.small", caution),
      cmdBlock("bernie topo --bandwidth")));
}

/** Refresh raw evidence alongside the verdict without closing its disclosure. */
function topoAdvanced() {
  const docs = [["/api/topo", "data", "bernie topo"], ["/api/topo/check", "check", "bernie topo --check"]];
  const body = h("div.advanced-body");
  const el = h("details.advanced", h("summary", "Raw results and terminal commands"), body);
  return { el, update: () => put(body, docs.map(([path, key, command]) => {
    const text = topo[key] == null ? "Unavailable: " + (topo.errors[path] || "no result returned") : JSON.stringify(topo[key], null, 2);
    return h("div", h("h3", path), cmdBlock(command, "Copy the command"),
      copyBtn(text, "Copy the result"), h("pre.topo-raw-json", text));
  })) };
}
