// The Advanced panel of the Logs tab: the documents behind the page and the
// commands that produce them (design-dashboard.md §2.6).
"use strict";

/** logsJsonLink opens the raw document of an endpoint in a new tab. */
function logsJsonLink(path) {
  return h("a.json-link", {
    href: path, target: "_blank", rel: "noopener noreferrer",
    title: "Open the raw answer: " + path,
  }, "json");
}

/** logsAdvanced is the disclosure at the end of the tab. It holds the JSON of
 *  the endpoints the tab used and the commands that answer the same. */
function logsAdvanced(runName) {
  const body = h("div.lg-advanced-body", loading());
  let loaded = false;
  const det = h("details.advanced.lg-advanced", h("summary", "Raw data and terminal commands"), body);
  det.addEventListener("toggle", async () => {
    if (!det.open || loaded) return;
    loaded = true;
    const name = typeof runName === "function" ? runName() : runName;
    try {
      const runs = await api("/api/runs");
      const json = JSON.stringify(runs, null, 2);
      const logPath = name ? "/api/runs/" + encodeURIComponent(name) + "/logs" : "/api/runs";
      put(body,
        h("p.small.muted", "The documents behind this page. An agent reads the same ones from the API."),
        h("div.row", logsJsonLink("/api/runs"),
          h("button", { type: "button", "data-copy": json, "aria-label": "Copy the JSON of the runs" }, "Copy the JSON")),
        h("pre", json),
        h("h3", "The log of the picked run"),
        h("div.row", logsJsonLink(logPath)),
        h("h3", "The same thing in a terminal"),
        cmdBlock("bernie ps"),
        name ? cmdBlock("bernie logs " + name) : null);
    } catch (e) {
      put(body, failed(e));
    }
  });
  return det;
}
