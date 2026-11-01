// Setup checklist. Adapted from vllm-rdna-dash by CrispyGinger (Apache-2.0).
"use strict";

const TIMELINE_STATE_WORD = {
  done: "ready",
  todo: "not started",
  running: "in progress",
  blocked: "needs attention",
  skipped: "not needed",
  failed: "failed",
};

// TIMELINE_MARK is the glyph in the circle of a row. A todo row carries its
// number; the other states carry an icon.
const TIMELINE_MARK = {
  done: "✓",
  todo: "",
  running: "◐",
  blocked: "!",
  skipped: "–",
  failed: "✕",
};

// TIMELINE_PILL is the class of the pill of a state.
const TIMELINE_PILL = {
  done: "done",
  todo: "todo",
  running: "running",
  blocked: "blocked",
  skipped: "skipped",
  failed: "fail",
};

/** Active work stays visible; the ordered checklist remains available on demand. */
function stepTimeline(steps, options = {}) {
  const list = Array.isArray(steps) ? steps : [];
  if (!list.length) return emptyState("The setup checklist is unavailable.",
    "Refresh the host status to find out what is needed.", "bernie status --steps");
  const extraIDs = options.extraIDs || [];
  const path = list.filter((s) => !extraIDs.includes(s.id));
  const extra = list.filter((s) => extraIDs.includes(s.id));
  const progress = options.progress || {};
  const logs = options.logs || {};
  const currentID = stepCurrentID(path, options.nextID);
  const visible = path.filter((s) => {
    const operator = options.operatorSummary && (s.state === "blocked" || (s.root_commands || []).length);
    return s.state === "failed" || (progress[s.id] && (progress[s.id].failed || progress[s.id].running)) || s.job_state === "running"
      || (!operator && !options.pathRunning && s.id === currentID)
      || (!operator && (s.problems || []).some((p) => p.status === "fail" || p.status === "warn"));
  });
  const row = (s, optional = false) => {
    const index = optional ? extra.indexOf(s) : path.indexOf(s);
    return stepRow(s, index + 1, options, progress, logs, optional, !optional && s.id === currentID);
  };
  const ready = path.filter((s) => s.state === "done" || s.state === "skipped").length;
  return h("div.setup-timeline",
    visible.length ? h("ol.step-timeline", visible.map((s) => row(s))) : null,
    h("details.advanced.setup-checklist-details",
      h("summary", "Full checklist · " + ready + " of " + path.length + " steps ready"),
      h("ol.step-timeline", path.filter((s) => !visible.includes(s)).map((s) => row(s))),
      extra.length ? sectionLabel("Optional improvements") : null,
      extra.length ? h("ol.step-timeline", extra.map((s) => row(s, true))) : null));
}

function stepCurrentID(path, nextID) {
  const running = path.find((s) => s.job_state === "running"
    || (s.state === "running" && !(s.id === "serve" && s.serving)));
  if (running) return running.id;
  if (nextID && path.some((s) => s.id === nextID)) return nextID;
  const first = path.find((s) => s.state !== "done" && s.state !== "skipped");
  return first ? first.id : "";
}

function stepRow(step, index, options, progress, logs, optional, current) {
  const live = progress[step.id];
  const state = live && live.running ? "running" : live && live.failed && !["done", "skipped"].includes(step.state) ? "failed"
    : TIMELINE_STATE_WORD[step.state] ? step.state : "todo";
  const roots = step.root_commands || [];
  const title = h("h3", step.title || step.id, " ", stepPill(state));
  if (current) title.append(h("span.pill", state === "running" ? "current step" : "next step"));
  const body = h("div.step-body", title, h("p.step-summary", stepSummary(step, state)));
  const note = options.noteFor ? options.noteFor(step) : null;
  if (step.detail || note || step.command || roots.length || state === "failed") {
    body.append(h("details.advanced.step-evidence",
      h("summary", roots.length ? "Administrator commands and details" : "Step details and terminal command"),
      state === "failed" && step.summary ? h("p", step.summary) : null,
      step.detail ? h("p.step-detail", step.detail) : null,
      note ? h("p.step-detail", note) : null,
      roots.length ? h("p", "Run these commands as root in a terminal, then re-check the host.") : null,
      roots.map((c) => cmdBlock(c, "Copy the command")),
      !roots.length && step.command ? cmdBlock(step.command, "Copy the command") : null));
  }
  if ((step.problems || []).length) body.append(h("div.step-problems", step.problems.map(stepProblem)));
  body.append(stepProgressNode(step, state, live, logs[step.id] || []));
  if (options.verify && step.id === "verify") {
    const box = options.verify(step);
    if (box) body.append(box);
  }
  const row = h("li.step-row", { class: "step-" + state, "data-step": step.id },
    h("div.step-mark", { "aria-hidden": "true" }, stepMark(state, index)), body);
  if (current) row.setAttribute("aria-current", "step");
  const action = stepAction(step, state, options, optional, current);
  if (action) row.append(action);
  return row;
}

/** stepPill is the state of a row: a mark and a word. */
function stepPill(state) {
  return statePill(TIMELINE_PILL[state] || "todo", TIMELINE_STATE_WORD[state]);
}

/** stepMark is the glyph in the circle: the number of a todo row, an icon
 *  for every other state. */
function stepMark(state, index) {
  if (state === "todo") return String(index);
  return TIMELINE_MARK[state] || "";
}

function stepSummary(step, state) {
  if (state === "failed") return "This step did not finish. Review the error and output before retrying.";
  if (step.id === "model" && state === "todo") return "The model still needs to be downloaded.";
  if (step.summary) return step.summary;
  if (state === "done") return "This step is ready.";
  if (state === "skipped") return "This step is not needed on this host.";
  if (state === "blocked") return "This step needs your attention before setup can continue.";
  return "This step runs after its prerequisites are ready.";
}

/** stepActionLabel names the action of a step: the button says what it
 *  starts, never only "Run". The words come from the checklist
 *  (spec §3.3), so the CLI, the MCP server and the page say the same. */
function stepActionLabel(step, again) {
  const name = step.action || step.title || "Run the step";
  return again ? name + " again" : name;
}

function stepAction(step, state, options, optional, current) {
  if (optional) {
    const link = options.linkFor ? options.linkFor(step) : null;
    return link ? h("div.step-actions", h("a", { href: link.href }, link.label)) : null;
  }
  if (state === "running") {
    return h("div.step-actions",
      options.onStop && (step.job_id || options.progress && options.progress[step.id] && options.progress[step.id].jobID)
        ? h("button", { type: "button", onclick: () => options.onStop(step.id) }, "Stop") : null,
      h("button", { type: "button", onclick: () => stepToggleLog(step.id) }, "View output"));
  }
  if ((step.root_commands || []).length || state === "blocked") {
    return options.onRefresh ? h("div.step-actions", h("button", { type: "button", onclick: options.onRefresh }, "Re-check host")) : null;
  }
  if (!step.can_run || !options.onRun) return null;
  const done = state === "done";
  const label = stepActionLabel(step, done);
  const queued = !done && !current && state !== "failed";
  return h("div.step-actions", { class: done || options.primarySetup && state !== "failed" ? "advanced-only" : "" },
    h("button", {
      type: "button",
      class: options.filled === false || queued || done ? "" : "primary",
      onclick: queued ? () => options.onWait && options.onWait(step, options) : () => options.onRun(step, options),
    }, queued ? "Why is this waiting?" : label));
}

/** Unknown progress has no bar; large output stays on demand. */
function stepProgressNode(step, state, progress, lines) {
  const box = h("div.step-progress", { "data-progress": step.id });
  const live = progress || (state === "running" ? { phase: "Waiting for job status…" } : {});
  if (!progress && state !== "running" && !lines.length) return box;
  box.append(h("p.step-progress-sentence", { "aria-live": "polite" }, live.phase || ""));
  if (typeof live.pct === "number" && !live.finished && !live.unavailable) box.append(stepBar(step, live.pct));
  box.append(h("p.step-progress-detail", live.detail || ""),
    h("p.small.muted.mono.step-log-line", live.logLine ? "Latest available output: " + live.logLine : ""),
    h("details.advanced.step-log-box",
      h("summary", live.failed ? "Error and job output" : "Job output"),
      h("pre.step-error", live.error || ""),
      (live.commands || []).map((c) => cmdBlock(c, "Copy administrator command")),
      h("div.log-lines.step-log", { role: "log", "aria-live": "off" }, lines.map((l) => h("div", l)))));
  return box;
}

/** stepBar is the bar of a measured progress. The width is set on the
 *  element, never in a style attribute: the page may not carry inline
 *  styles. */
function stepBar(step, pct) {
  const bar = h("div.step-bar", {
    role: "progressbar",
    "aria-valuemin": "0",
    "aria-valuemax": "100",
    "aria-valuenow": String(pct),
    "aria-label": "Progress of " + (step.title || step.id),
  }, h("i"));
  const fill = bar.querySelector("i");
  if (fill) fill.style.width = pct + "%";
  return bar;
}

function stepProblem(problem) {
  const status = problem.status || "warn";
  return h("div.step-problem",
    h("div.step-problem-head",
      h("span.badge", { class: "b-" + status }, status),
      h("b", problem.title || problem.id),
      h("span.step-problem-id.advanced-only", problem.id || "")),
    problem.detail ? h("p.step-problem-detail", problem.detail) : null,
    problem.fix ? h("details.advanced", h("summary", "Show recovery command"), cmdBlock(problem.fix, "Copy the fix command")) : null,
    problem.doc ? h("a.small.step-problem-doc", { href: problem.doc, target: "_blank", rel: "noopener noreferrer" }, "Troubleshooting guide") : null);
}

function stepToggleLog(id) {
  const row = Array.from(document.querySelectorAll(".step-row")).find((el) => el.dataset.step === id);
  const box = row && row.querySelector(".step-log-box");
  if (box) box.open = !box.open;
}
