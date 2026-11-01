"use strict";
// Failures and their causes stay visible in both modes.
function advJobList() {
  const card = h("div.card");
  const view = {
    el: card,
    update: (jobs) => {
      const list = jobs || [];
      const error = (adv.errors || {})["/api/jobs"];
      if (!list.length) {
        put(card, cardHead("Background work", "/api/jobs"),
          emptyState(error ? "Background work could not be read." : "No background work is recorded.",
            error ? "Re-check the host to try again; job status is unknown." : "Setup actions appear here with their progress or failure reason.",
            "bernie status --steps"), error ? h("p.err", error) : null);
        return;
      }
      const failed = list.filter((j) => j.state === "failed").length;
      const running = list.filter((j) => j.state === "running").length;
      const rows = list.map((job) => h("tr",
        h("td", job.step || "Background action"),
        h("td", advJobState(job.state)),
        h("td", job.error || job.phase || "No progress message"),
        h("td", job.started_at ? ago(job.started_at) : "not recorded"),
        h("td.advanced-only", job.id || "–")));
      put(card, cardHead("Background work", "/api/jobs"),
        h("p.small", failed + " failed · " + running + " in progress. Failed actions need review in Setup before retrying."),
        h("div.scroll", h("table", h("thead", h("tr",
          ["Action", "State", "Progress or reason", "Started"].map((label) => h("th", { scope: "col" }, label)),
          h("th.advanced-only", { scope: "col" }, "Job ID"))), h("tbody", rows))));
    },
  };
  return view;
}

/** A status always has a word as well as a colour. */
function advJobState(state) {
  const kind = state === "running" ? "running" : state === "done" ? "done"
    : state === "failed" ? "fail" : "skip";
  return h("span.badge", { class: "b-" + kind }, state || "unknown");
}
