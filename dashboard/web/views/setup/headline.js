// Setup's one next action. Adapted from vllm-rdna-dash by CrispyGinger, with permission.
"use strict";

function setupHeadline(doc) {
  const path = pathSteps(doc);
  if (!path.length) return { title: "Setup checks are unavailable", lede: "No checklist was returned. Re-check the host before starting setup.", action: "refresh" };
  const pending = path.filter((s) => s.state !== "done" && s.state !== "skipped");
  const live = setupProgress.path;
  const sameJob = live && doc.path && live.jobID === doc.path.id;
  const runningPath = live && live.running || doc.path && doc.path.state === "running" && !(sameJob && live.finished);
  const runningStep = path.find((s) => (s.job_state === "running" || s.state === "running" && !setupServerServing(s))
    && !(setupProgress[s.id] && setupProgress[s.id].finished));
  if (runningPath || runningStep || setupProgress.verify && setupProgress.verify.running) {
    const checking = !runningPath && !runningStep;
    return { title: checking ? "Checking the Python environment" : "Setup is in progress",
      lede: checking ? "This checks installed components, not a rebuild or a server test." : "Follow the current step below. You can stop the running job at any time.",
      action: "watch", step: runningPath || checking ? null : runningStep };
  }
  const serve = path.find((s) => s.id === "serve");
  if (setupServerServing(serve)) return {
    title: "Your model is serving", lede: "The server answers on " + setupServerEndpoint(serve) + ". Try a question to check its response.", action: "try",
  };
  const failed = path.find((s) => s.state === "failed");
  if (pending.length && (failed || live && live.failed || doc.path && doc.path.state === "failed")) return {
    title: "Setup needs attention", lede: "The last attempt did not finish. Review its error and output, correct the cause, then retry.", action: "recover", step: failed,
  };
  const hardware = path.find((s) => s.id === "hardware");
  if (hardware && hardware.state === "blocked") return {
    title: "A compatible GPU is needed", lede: hardware.summary || "No compatible AMD GPU is visible. Read the host checks below.", action: "doctor",
  };
  const blocked = pending.find((s) => s.state === "blocked");
  if (blocked) return {
    title: "A prerequisite needs your attention", lede: "Resolve the items in Needs you below, then re-check the host.", action: "recheck",
  };
  if (setupState.planError || setupState.recipeError || setupState.recipes && !(setupState.recipes.recipes || []).length) return {
    title: "Review the recipe plan", lede: "The serving plan is not ready. Review the recipe message below before starting setup.", action: "recipe",
  };
  if (!pending.length) return {
    title: "Ready to start the server", lede: "Setup checks are ready. Start the model server, then try a question.", action: "start",
  };
  const current = setupCurrentStep(doc);
  return {
    title: "Get this host ready to serve",
    lede: doc.primary && doc.primary.sentence || doc.next_action && doc.next_action.sentence
      || (current && (current.root_commands || []).length ? "Run the administrator commands below, then re-check the host." : "Check the prerequisites and choose a recipe below."),
    action: "setup",
  };
}

function setupServerEndpoint(serve) {
  if (serve && serve.detail && /^https?:\/\//.test(serve.detail)) return serve.detail;
  const plan = setupState.plan;
  if (plan && plan.port) return "port " + plan.port;
  return "the port of the plan";
}

function setupHeadActions(doc, say) {
  const refresh = h("button", { type: "button", onclick: () => loadSetupDocument() }, "Re-check host");
  const details = h("button.advanced-only", { type: "button", onclick: setupShowDetails }, "Technical details");
  if (say.action === "refresh") { refresh.classList.add("primary"); return [refresh]; }
  if (say.action === "doctor") return [h("button.primary", { type: "button", onclick: focusSetupDoctor }, "Read host checks"), refresh];
  if (say.action === "recheck") return [h("button.primary", {
    type: "button", onclick: () => { setupState.needsYou.scrollIntoView({ block: "center" }); },
  }, "Review prerequisites"), refresh];
  if (say.action === "recipe") return [h("button.primary", { type: "button", onclick: () => {
    const section = document.getElementById("setup-recipes");
    if (section) section.scrollIntoView({ block: "start" });
  } }, "Review recipe plan"), refresh];
  if (say.action === "watch" || say.action === "recover") return [h("button.primary", {
    type: "button", onclick: () => {
      if (say.step) { focusSetupStep(say.step.id); return; }
      const box = setupState.pathBox;
      const evidence = box && box.querySelector(".setup-run-details");
      if (evidence && say.action === "recover") evidence.open = true;
      if (box) box.scrollIntoView({ behavior: "smooth", block: "center" });
    },
  }, say.action === "recover" ? "Review setup failure" : "View current step"), refresh];
  if (say.action === "try") return [h("button.primary", { type: "button", onclick: focusSetupVerify }, "Try a question"),
    h("button", { type: "button", onclick: () => { location.hash = "#logs"; } }, "View server logs")];
  if (say.action === "start") return [h("button.primary", { type: "button", onclick: startSetupServer }, "Start server"), details];
  const primary = doc.primary;
  if (primary && primary.kind === "setup") return [h("button.primary", {
    type: "button", disabled: !setupState.recipeID || !!setupState.planLoading, onclick: setupStartPath,
  }, !setupState.recipeID ? "Loading recipe…" : primary.label || "Set up this host"), refresh,
  h("button.advanced-only", { type: "button", onclick: setupCopyNextCommand }, "Copy setup command"), details];
  const current = setupCurrentStep(doc);
  if (current && current.can_run) return [h("button.primary", {
    type: "button", onclick: () => setupStartStep(current, { onRefresh: () => loadSetupDocument() }),
  }, stepActionLabel(current, false)), details];
  if (current && (current.root_commands || []).length) return [h("button.primary", {
    type: "button", onclick: () => { setupCopyText(setupRootCommand(current)); focusSetupStep(current.id); },
  }, "Copy administrator command"), refresh, details];
  return [h("button.primary", { type: "button", onclick: focusSetupDoctor }, "Read host checks"), details];
}

function setupCurrentStep(doc) {
  const path = pathSteps(doc);
  return path.find((s) => (s.job_state === "running" || s.state === "running" && !setupServerServing(s)) && !(setupProgress[s.id] && setupProgress[s.id].finished))
    || path.find((s) => s.state === "failed")
    || path.find((s) => s.id === (doc && doc.next))
    || path.find((s) => s.state !== "done" && s.state !== "skipped") || null;
}

function setupStartStep(step, options) {
  setupEnsureModule("views/setup/steps.js")
    .then(() => runSetupStep(step, options))
    .catch((e) => toast("The step could not start: " + (e.message || e)));
}

function setupExplainWait(step) {
  const sentence = setupWaitSentence(step);
  if (!sentence) {
    toast("This step waits for the step before it.");
    return;
  }
  toast(sentence);
}

/** Name the prerequisite without making the reader decode an id. */
function setupWaitSentence(step) {
  const doc = setupState.doc;
  if (!doc || !step || !step.waits_for) return "";
  const holds = (doc.steps || []).find((s) => s.id === step.waits_for);
  if (!holds) return "";
  return "This step waits for: " + stepActionLabel(holds, false) + ".";
}

function pathSteps(doc) {
  return (doc && doc.steps || []).filter((s) => !SETUP_EXTRA_STEPS.includes(s.id));
}

/** Copy a root command, never a doctor command with a misleading label. */
function setupRootCommand(step) {
  const roots = (step && step.root_commands) || [];
  return roots.length ? roots[0] : ((step && step.command) || "");
}

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

function startSetupServer() {
  const doc = setupState.doc;
  const serve = doc && (doc.steps || []).find((s) => s.id === "serve");
  if (serve && serve.can_run) {
    setupStartStep(serve, { onRefresh: () => loadSetupDocument() });
    return;
  }
  startSetupRecipe();
}

function focusSetupStep(id) {
  const row = Array.from(document.querySelectorAll(".step-row")).find((el) => el.dataset.step === id);
  if (!row) return;
  for (let el = row.parentElement; el; el = el.parentElement) {
    if (el.tagName === "DETAILS") el.open = true;
  }
  row.scrollIntoView({ block: "center" });
  const control = Array.from(row.querySelectorAll("button, a, input, summary")).find((el) => el.getClientRects().length);
  if (control) control.focus();
}

function focusSetupDoctor() {
  const section = document.getElementById("setup-doctor");
  if (!section) return;
  section.scrollIntoView({ block: "start" });
  const control = section.querySelector("button, a");
  if (control) control.focus();
}

function focusSetupVerify() {
  focusSetupStep("verify");
}

/** Open this tab's raw documents without changing the page-wide mode. */
async function setupShowDetails() {
  await setupEnsureModule("views/setup/advanced.js");
  let panel = setupState.advanced;
  if (!panel) {
    panel = setupAdvancedPanel();
    setupState.advanced = panel;
    if (setupState.advancedBox) put(setupState.advancedBox, panel);
  }
  setupState.detailsOpen = true;
  panel.open = true;
  panel.scrollIntoView({ block: "nearest" });
}
