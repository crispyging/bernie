// The onboarding step timeline. The Setup view owns the actions and the
// live job state; this component owns the order, the state words and the
// anatomy of one row.
// Adapted from vllm-rdna-dash by CrispyGinger (Apache-2.0).
"use strict";

// The words of the states of spec §3.3. The word carries the meaning; the
// colour only helps.
const TIMELINE_STATE_WORD = {
  done: "done",
  todo: "todo",
  running: "running",
  blocked: "blocked",
  skipped: "skipped",
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

/**
 * stepTimeline renders the ordered steps of the onboarding checklist.
 *
 * options.extraIDs   the ids that come after the path of the host
 * options.nextID     the step the reader does next
 * options.onRun      starts a runnable step: function(step, options)
 * options.onRefresh  reads the checklist again
 * options.onWait     says what a later step waits for: function(step)
 * options.noteFor    one more line under a step: function(step)
 * options.linkFor    the link of an optional step: function(step)
 * options.progress   the live progress of a step, by step id
 * options.logs       the output of a step, by step id
 * options.verify     the verify box of the server, when it is available
 * options.filled     false keeps the row button a ghost one, so a tab that
 *                    has a filled button of its own has only one
 */
function stepTimeline(steps, options = {}) {
  const list = Array.isArray(steps) ? steps : [];
  if (!list.length) {
    return emptyState("There is no checklist on this host.",
      "The checklist is the ordered list of steps that lead from a bare machine to a serving model. It comes from the doctor report, the setup plan and the runs.",
      "bernie status --steps");
  }
  const extraIDs = options.extraIDs || [];
  const path = list.filter((s) => !extraIDs.includes(s.id));
  const extra = list.filter((s) => extraIDs.includes(s.id));
  const progress = options.progress || {};
  const logs = options.logs || {};
  const currentID = stepCurrentID(path, options.nextID);
  const currentIndex = path.findIndex((s) => s.id === currentID);
  const row = (s, i, optional) => stepRow(s, i + 1, options, progress, logs, optional,
    !optional && s.id === currentID,
    !optional && currentIndex >= 0 && i > currentIndex && s.state !== "done" && s.state !== "skipped");
  return h("div.setup-timeline",
    h("ol.step-timeline", path.map((s, i) => row(s, i, false))),
    extra.length ? sectionLabel("Optional steps") : null,
    extra.length ? h("ol.step-timeline", extra.map((s, i) => row(s, i, true))) : null);
}

/** stepCurrentID is the step the reader does next: the step that runs, the
 *  one the checklist names, or the first step of the path that is not done
 *  yet. */
function stepCurrentID(path, nextID) {
  const running = path.find((s) => s.state === "running");
  if (running) return running.id;
  if (nextID && path.some((s) => s.id === nextID)) return nextID;
  const first = path.find((s) => s.state !== "done" && s.state !== "skipped");
  return first ? first.id : "";
}

/** stepRow renders one step of the checklist. */
function stepRow(step, index, options, progress, logs, optional, current, queued) {
  const state = TIMELINE_STATE_WORD[step.state] ? step.state : "todo";
  const roots = step.root_commands || [];
  const title = h("h3", step.title || step.id, " ", stepPill(state));
  if (current) title.append(h("span.pill", "next"));
  else if (queued) title.append(h("span.small.muted", "waits"));
  const body = h("div.step-body",
    title,
    h("p.step-summary", stepSummary(step, state)),
    step.detail ? h("p.step-detail", step.detail) : null);
  const note = options.noteFor ? options.noteFor(step) : null;
  if (note) body.append(h("p.step-detail", note));
  if (roots.length) {
    body.append(h("div.step-root",
      h("p", "This step needs root. Run it in a terminal, then re-check."),
      roots.map((c) => cmdBlock(c, "Copy the command"))));
  } else if (state === "todo" && !step.can_run && step.command) {
    // bernie cannot do this step: the command is all the reader gets.
    body.append(cmdBlock(step.command, "Copy the command"));
  }
  if ((step.problems || []).length) {
    body.append(h("div.step-problems", step.problems.map(stepProblem)));
  }
  body.append(stepProgressNode(step, state, progress[step.id], logs[step.id] || []));
  if (options.verify && step.id === "verify") {
    const box = options.verify(step);
    if (box) body.append(box);
  }
  const row = h("li.step-row", { class: "step-row step-" + state, "data-step": step.id },
    h("div.step-mark", { "aria-hidden": "true" }, stepMark(state, index)),
    body);
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

/** stepSummary is the one plain sentence under the title of a row. */
function stepSummary(step, state) {
  if (step.id === "model" && state === "todo") return "The model is not on this host yet.";
  if (step.summary) return step.summary;
  if (state === "done") return "This step is done.";
  if (state === "skipped") return "This step does not apply to this host.";
  return "bernie can do this step.";
}

/** stepActionLabel names the action of a step: the button says what it
 *  starts, never only "Run". The words come from the checklist
 *  (spec §3.3), so the CLI, the MCP server and the page say the same. */
function stepActionLabel(step, again) {
  const name = step.action || step.title || "Run the step";
  return again ? name + " again" : name;
}

/** stepAction is the control of a row: the action of the step, the log of a
 *  running job, a re-check of a root step, a link for an optional step.
 *  Only the step the reader does next carries the filled button; a later
 *  step keeps a ghost button that says what it waits for. */
function stepAction(step, state, options, optional, current) {
  if (optional) {
    const link = options.linkFor ? options.linkFor(step) : null;
    if (!link) return null;
    return h("div.step-actions", h("a", { href: link.href }, link.label));
  }
  if (state === "running") {
    return h("div.step-actions", h("button", { type: "button", onclick: () => stepToggleLog(step.id) }, "the log"));
  }
  if ((step.root_commands || []).length) {
    // bernie never runs a root command. The reader pastes it and asks the
    // host again.
    if (!options.onRefresh) return null;
    return h("div.step-actions", h("button", { type: "button", onclick: options.onRefresh }, "Re-check"));
  }
  if (step.can_run && options.onRun) {
    const done = state === "done";
    const label = stepActionLabel(step, done);
    if (done) {
      return h("div.step-actions", h("button", { type: "button", onclick: () => options.onRun(step, options) }, label));
    }
    if (!current) {
      // The step waits for the one before it. The button says so instead of
      // starting it.
      const say = options.onWait ? () => options.onWait(step, options) : null;
      return h("div.step-actions", h("button", { type: "button", onclick: say }, label));
    }
    return h("div.step-actions", h("button", {
      type: "button",
      class: options.filled === false ? "" : "primary",
      onclick: () => options.onRun(step, options),
    }, label));
  }
  if (state === "blocked" && options.onRefresh) {
    return h("div.step-actions", h("button", { type: "button", onclick: options.onRefresh }, "Re-check"));
  }
  return null;
}

/** stepProgressNode is the progress of a row: the sentence, the bar, the
 *  numbers under it and the output of the job. The bar is drawn only when
 *  the work is measured: an unknown fraction never reads 0 %. */
function stepProgressNode(step, state, progress, lines) {
  const box = h("div.step-progress", { "data-progress": step.id });
  const live = progress || (state === "running" ? { phase: "Working…" } : null);
  if (!live && !lines.length) return box;
  const measured = typeof live.pct === "number";
  box.append(h("p.step-progress-sentence", { "aria-live": "polite" }, live.phase || ""));
  if (measured) box.append(stepBar(step, live.pct));
  box.append(h("p.step-progress-detail", live.detail || ""));
  if (lines.length) {
    // The output of a job stays open under its step: it is the answer to
    // "what did bernie do", so the reader sees it without a click.
    box.append(h("details.step-log-box", { open: true },
      h("summary", "the log"),
      h("div.log-lines.step-log", { role: "log", "aria-live": "off" }, lines.map((l) => h("div", l)))));
  }
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

/** stepProblem is one doctor check that holds a step back: its cause and
 *  the command that fixes it. */
function stepProblem(problem) {
  const status = problem.status || "warn";
  return h("div.step-problem",
    h("div.step-problem-head",
      h("span.badge", { class: "b-" + status }, status),
      h("b", problem.title || problem.id),
      h("span.step-problem-id", problem.id || "")),
    problem.detail ? h("p.step-problem-detail", problem.detail) : null,
    problem.fix ? cmdBlock(problem.fix, "Copy the fix command") : null,
    problem.doc ? h("a.small.step-problem-doc", { href: problem.doc, target: "_blank", rel: "noopener noreferrer" }, "the rule") : null);
}

/** stepToggleLog opens or closes the output of a step. */
function stepToggleLog(id) {
  const box = document.querySelector('[data-step="' + id + '"] .step-log-box');
  if (box) box.open = !box.open;
}
