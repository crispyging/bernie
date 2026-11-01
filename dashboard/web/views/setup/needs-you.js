// Operator-only prerequisites; commands stay available in either mode.
"use strict";

function paintSetupNeedsYouRender(doc) {
  const box = setupState.needsYou;
  if (!box) return;
  if (!doc) doc = setupState.doc;
  if (!doc) return;
  const path = (doc.steps || []).filter((s) => !SETUP_EXTRA_STEPS.includes(s.id));
  const items = path.filter((s) => s.state === "blocked"
    || (s.state !== "done" && s.state !== "skipped" && (s.root_commands || []).length));
  if (!items.length) {
    box.hidden = true;
    put(box);
    return;
  }
  put(box,
    sectionLabel("Needs you"),
    h("p.small.muted", "These prerequisites need your attention before setup can continue."),
    h("div.needs-you-list", items.map(setupNeedsYouItem)),
    h("div.needs-you-foot",
      h("button", { type: "button", onclick: () => loadSetupDocument() }, "Re-check host"),
      h("button", { type: "button", onclick: focusSetupDoctor }, "Read host checks")));
  box.hidden = false;
}

function setupNeedsYouItem(step) {
  const roots = step.root_commands || [];
  return h("div.needs-you-item",
    h("div.needs-you-head", statePill("warn", roots.length ? "administrator access" : "needs attention"),
      h("span.needs-you-title", step.title || step.id || "")),
    step.summary ? h("p.needs-you-summary", step.summary) : null,
    roots.length ? h("details.advanced.needs-you-command", h("summary", "Show commands to run as administrator"),
      h("p.small.muted", "Run these in a terminal as root, then re-check the host."),
      roots.map((command) => cmdBlock(command, "Copy the command"))) : null);
}
