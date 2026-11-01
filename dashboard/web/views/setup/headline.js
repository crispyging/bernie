// The headline of the Setup tab: the state of the host and the one next action.
// Adapted from vllm-rdna-dash by CrispyGinger (Apache-2.0).
"use strict";

/** setupHeadline says what the machine is in one sentence, with the lede
 *  under it naming the single next action and who does it. */
function setupHeadline(doc) {
  const steps = doc.steps || [];
  const path = steps.filter((s) => !SETUP_EXTRA_STEPS.includes(s.id));
  const pending = path.filter((s) => s.state !== "done" && s.state !== "skipped");
  const running = path.find((s) => s.state === "running");
  const hardware = steps.find((s) => s.id === "hardware");
  const serve = steps.find((s) => s.id === "serve");
  if (setupServerServing(serve)) {
    return {
      title: "THIS BOX IS SERVING.",
      lede: "The model answers on " + setupServerEndpoint(serve) + ". Try it, or read the logs.",
      action: "try",
    };
  }
  if (running) {
    const phase = (running.progress && running.progress.phase) || running.summary || "bernie is working on the next step.";
    return { title: "SETTING UP THIS BOX.", lede: phase, action: "continue" };
  }
  if (!pending.length) {
    return {
      title: "THIS BOX IS READY TO SERVE.",
      lede: "Everything bernie needs is in place. Start the server and it answers on " + setupServerEndpoint(serve) + ".",
      action: "start",
    };
  }
  if (hardware && hardware.state === "blocked") {
    return {
      title: "THIS BOX HAS NO GPU FOR BERNIE.",
      // The words of the state come from the step (internal/onboarding),
      // so the page and `bernie status --steps` say the same thing.
      lede: (hardware.summary || "This machine has no AMD GPU.") +
        " Read the doctor report below to see what is missing.",
      action: "doctor",
    };
  }
  // A step can be blocked on the operator (a root command, a missing
  // group, an unreadable /dev/kfd). The headline says so in words the
  // reference's reader recognises (ia.md §5 R39): the situation, one
  // short sentence, and the next move (Re-check).
  const blocked = pending.find((s) => s.state === "blocked");
  if (blocked) {
    const detail = blocked.summary || "a step waits for the operator.";
    return {
      title: "A FEW THINGS FIRST.",
      lede: detail + " Fix the items below first, then press Re-check.",
      action: "recheck",
    };
  }
  const current = setupCurrentStep(doc);
  let lede = "";
  const action = doc.next_action;
  const primary = doc.primary;
  if (primary && primary.sentence) {
    // The sentence comes from the checklist (spec §3.3): the CLI, the MCP
    // server and the page say the same thing.
    lede = primary.sentence;
  } else if (action && action.sentence) {
    lede = action.sentence;
  } else if (current && (current.root_commands || []).length) {
    lede = "Copy the command, run it as root in a terminal, then press Re-check.";
  } else {
    lede = "The doctor report says what is missing.";
  }
  return {
    title: pending.length + (pending.length === 1 ? " STEP LEFT" : " STEPS LEFT") + " BEFORE THIS BOX CAN SERVE.",
    lede: lede,
    action: "setup",
  };
}

/** setupServerEndpoint is where the server answers. */
function setupServerEndpoint(serve) {
  if (serve && serve.detail && /^https?:\/\//.test(serve.detail)) return serve.detail;
  const plan = setupState.plan;
  if (plan && plan.port) return "port " + plan.port;
  return "the port of the plan";
}

/** setupHeadActions are the actions of the tab. The filled button is the
 *  one primary action of the checklist (spec §3.3): it names what it does,
 *  never only "Run" or "Next". */
function setupHeadActions(doc, say) {
  const out = [];
  const primary = doc.primary;
  if (say.action === "doctor") {
    out.push(h("button.primary", { type: "button", onclick: focusSetupDoctor }, "Read the doctor report"));
    out.push(h("button", { type: "button", onclick: setupShowDetails }, "Show details"));
    return out;
  }
  if (say.action === "recheck") {
    out.push(h("button.primary", {
      type: "button",
      onclick: () => { focusSetupStep("setup-doctor"); loadSetupDocument(); },
    }, "Re-check"));
    out.push(h("button", { type: "button", onclick: setupShowDetails }, "Show details"));
    return out;
  }
  if (say.action === "continue") {
    out.push(h("button.primary", { type: "button", onclick: setupRunNextStep }, "Run the rest of the steps"));
    out.push(h("button", { type: "button", onclick: () => loadSetupDocument() }, "Re-check"));
    return out;
  }
  // The one action of the checklist: it does everything bernie can do on
  // this host. The words come from the checklist, so the CLI, the MCP
  // server and the page say the same thing.
  if (primary && primary.kind === "setup") {
    out.push(h("button.primary", { type: "button", onclick: setupStartPath }, primary.label || "Set up this box"));
    out.push(h("button", { type: "button", onclick: setupCopyNextCommand }, "Copy the command"));
    out.push(h("button", { type: "button", onclick: () => loadSetupDocument() }, "Re-check"));
    out.push(h("button", { type: "button", onclick: setupShowDetails }, "Show details"));
    return out;
  }
  if (say.action === "start") {
    out.push(h("button.primary", { type: "button", onclick: startSetupServer }, "Start the server"));
    out.push(h("button", { type: "button", onclick: setupShowDetails }, "Show details"));
    return out;
  }
  if (say.action === "try") {
    out.push(h("button.primary", { type: "button", onclick: focusSetupVerify }, "Try it"));
    out.push(h("button", { type: "button", onclick: () => { location.hash = "#logs"; } }, "Read the logs"));
    return out;
  }
  const current = setupCurrentStep(doc);
  const next = current && current.can_run ? current : null;
  const rootNext = !next && current && (current.root_commands || []).length ? current : null;
  if (next) {
    out.push(h("button.primary", {
      type: "button",
      onclick: () => setupStartStep(next, { onRefresh: () => loadSetupDocument() }),
    }, stepActionLabel(next, false)));
  } else if (rootNext) {
    out.push(h("button.primary", {
      type: "button",
      onclick: () => { setupCopyText(setupRootCommand(rootNext)); focusSetupStep(rootNext.id); },
    }, "Copy the root command"));
    out.push(h("button", { type: "button", onclick: setupCopyNextCommand }, "Copy the next command"));
    out.push(h("button", { type: "button", onclick: () => loadSetupDocument() }, "Re-check"));
    out.push(h("button", { type: "button", onclick: setupShowDetails }, "Show details"));
    return out;
  } else {
    out.push(h("button.primary", { type: "button", onclick: focusSetupDoctor }, "Read the doctor report"));
    out.push(h("button", { type: "button", onclick: setupShowDetails }, "Show details"));
    return out;
  }
  out.push(h("button", { type: "button", onclick: setupCopyNextCommand }, "Copy the command"));
  out.push(h("button", { type: "button", onclick: setupShowDetails }, "Show details"));
  return out;
}

/** setupNextRunnable is the next step bernie can do itself. */
function setupNextRunnable(doc) {
  const steps = (doc && doc.steps) || [];
  const path = steps.filter((s) => !SETUP_EXTRA_STEPS.includes(s.id));
  return path.find((s) => s.state === "todo" && s.can_run) || null;
}

/** setupCurrentStep is the step the reader does next: the one that runs,
 *  the one the checklist names, or the first step of the path that is not
 *  done yet. The headline and the timeline mark the same step. */
function setupCurrentStep(doc) {
  const path = pathSteps(doc);
  const running = path.find((s) => s.state === "running");
  if (running) return running;
  const named = path.find((s) => s.id === (doc && doc.next));
  if (named) return named;
  return path.find((s) => s.state !== "done" && s.state !== "skipped") || null;
}

/** setupRunNextStep starts the next step bernie can do. */
function setupRunNextStep() {
  const doc = setupState.doc;
  const next = setupNextRunnable(doc);
  if (!next) {
    toast("There is no step bernie can do right now.");
    return;
  }
  setupStartStep(next, { onRefresh: () => loadSetupDocument() });
}

/** setupStartStep starts one step of the checklist. The module of the
 *  steps comes with the press, so the first paint does not carry it. */
function setupStartStep(step, options) {
  setupEnsureModule("views/setup/steps.js")
    .then(() => runSetupStep(step, options))
    .catch(() => {});
}

/** setupExplainWait says what a later step waits for. */
function setupExplainWait(step) {
  const sentence = setupWaitSentence(step);
  if (!sentence) {
    toast("This step waits for the step before it.");
    return;
  }
  toast(sentence);
}

/** setupWaitSentence names the step that holds the path for a step that
 *  waits. The words come from the checklist (spec §3.3). */
function setupWaitSentence(step) {
  const doc = setupState.doc;
  if (!doc || !step || !step.waits_for) return "";
  const holds = (doc.steps || []).find((s) => s.id === step.waits_for);
  if (!holds) return "";
  return "This step waits for: " + stepActionLabel(holds, false) + ".";
}

/** pathSteps is the path of the checklist, with the extras removed. */
function pathSteps(doc) {
  return (doc && doc.steps || []).filter((s) => !SETUP_EXTRA_STEPS.includes(s.id));
}

/** setupRootCommand is the command the operator runs as root for a step.
 *  It is the first root command of the step, never the doctor command:
 *  the button says "root command", so it has to be one. */
function setupRootCommand(step) {
  const roots = (step && step.root_commands) || [];
  return roots.length ? roots[0] : ((step && step.command) || "");
}

/** setupCopyNextCommand copies the command of the next step. */
function setupCopyNextCommand() {
  const doc = setupState.doc;
  const next = doc && (doc.steps || []).find((s) => s.id === doc.next);
  if (next && (next.root_commands || []).length) {
    setupCopyText(setupRootCommand(next));
    return;
  }
  const text = (next && next.command) || setupPlanCommand(setupState.plan || {});
  setupCopyText(text);
}

/** setupCopyText puts a command on the clipboard and says that it worked. */
async function setupCopyText(text) {
  if (!text) {
    toast("There is nothing to copy.");
    return;
  }
  let ok = false;
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      ok = true;
    }
  } catch (e) { ok = false; }
  if (!ok) {
    try {
      const area = h("textarea");
      // The styles are set through the CSSOM, never as an inline style
      // attribute: the page may not carry inline styles.
      area.style.position = "fixed";
      area.style.left = "-9999px";
      area.style.top = "0";
      area.value = text;
      document.body.append(area);
      area.select();
      ok = document.execCommand("copy");
      area.remove();
    } catch (e) { ok = false; }
  }
  toast(ok ? "Copied the command." : "Copy failed: select the text and press Ctrl-C.", ok);
}

/** startSetupServer starts the server of the path. */
function startSetupServer() {
  const doc = setupState.doc;
  const serve = doc && (doc.steps || []).find((s) => s.id === "serve");
  if (serve && serve.can_run) {
    setupStartStep(serve, { onRefresh: () => loadSetupDocument() });
    return;
  }
  startSetupRecipe();
}

/** focusSetupStep scrolls to a step and puts the focus on its first
 *  control. */
function focusSetupStep(id) {
  if (!id) return;
  const row = document.querySelector('[data-step="' + id + '"]');
  if (!row) return;
  row.scrollIntoView({ block: "center" });
  const control = row.querySelector("button, a, input");
  if (control) control.focus();
}

/** focusSetupDoctor scrolls to the doctor report. */
function focusSetupDoctor() {
  const section = document.getElementById("setup-doctor");
  if (!section) return;
  section.scrollIntoView({ block: "start" });
  const control = section.querySelector("button, a");
  if (control) control.focus();
}

/** focusSetupVerify scrolls to the verify step. */
function focusSetupVerify() {
  focusSetupStep("verify");
}

/** setupShowDetails opens or closes the raw documents of the tab. It acts on
 *  this tab only: the rail button is the page-wide switch. */
async function setupShowDetails() {
  await setupEnsureModule("views/setup/advanced.js");
  let panel = setupState.advanced;
  if (!panel) {
    panel = setupAdvancedPanel();
    setupState.advanced = panel;
    if (setupState.advancedBox) put(setupState.advancedBox, panel);
  }
  setupState.detailsOpen = !setupState.detailsOpen;
  panel.open = !!setupState.detailsOpen;
  panel.dataset.local = "1";
  if (panel.open) panel.scrollIntoView({ block: "nearest" });
}
