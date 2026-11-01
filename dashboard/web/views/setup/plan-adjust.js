// Optional GPU selection and saved mode. Loaded with the plan disclosure.
"use strict";

function setupPlanAdjustBody() {
  return h("div.recipe-plan-adjust-body",
    setupPlanGPUSelector(),
    h("p.small.muted",
      "Settings below explain the effective plan. Changes update the preview, not an active server."),
    setupSaveDefaultNote());
}

/** GPU selection uses the same env override as the CLI. */
function setupPlanGPUSelector() {
  const detected = (setupState.hw && setupState.hw.gpus) || [];
  if (!detected.length) return null;
  const planned = new Set((setupState.plan && setupState.plan.gpus) || []);
  const checked = (g) => setupPlanGPUSelected().length
    ? setupPlanGPUSelected().includes(g.hip_index)
    : planned.has(g.hip_index);
  const rows = detected.map((g) => {
    const vram = g.vram_bytes ? Math.round(g.vram_bytes / (1024 * 1024 * 1024)) + " GiB" : "–";
    const cb = h("input", {
      type: "checkbox", value: String(g.hip_index),
      checked: checked(g),
      onchange: () => setupApplyGPUSelection(),
    });
    return h("label.recipe-gpu-row", cb,
      h("span.recipe-gpu-idx", "GPU " + g.hip_index),
      h("span.recipe-gpu-name", g.name || g.gfx || "GPU"),
      h("span.recipe-gpu-vram", vram));
  });
  return h("div.recipe-gpu-selector",
    h("h4", "GPUs allowed for this run"),
    h("p.small.muted", "Choose cards for the next run. Clearing every checkbox returns to the plan's default GPUs."),
    rows,
    h("button.small", { type: "button", onclick: setupClearGPUSelection }, "Use the plan's default GPUs"));
}

function setupPlanGPUSelected() {
  const box = setupState.planBox;
  if (!box) return [];
  const out = [];
  box.querySelectorAll(".recipe-gpu-selector input[type=checkbox]").forEach((cb) => {
    if (cb.checked) out.push(Number(cb.value));
  });
  return out;
}

async function setupApplyGPUSelection() {
  const prefix = "env.HIP_VISIBLE_DEVICES=";
  const sets = setupState.sets.filter((s) => !s.startsWith(prefix));
  const picked = setupPlanGPUSelected();
  if (picked.length) sets.push(prefix + picked.join(","));
  setupState.sets = sets;
  await loadSetupPlan();
}

async function setupClearGPUSelection() {
  setupState.sets = setupState.sets.filter((s) => !s.startsWith("env.HIP_VISIBLE_DEVICES="));
  await loadSetupPlan();
}


function setupSaveDefaultNote() {
  const keys = setupState.prefsKeys;
  if (!Array.isArray(keys) || !keys.includes("mode")) return null;
  return h("p.small.muted",
    "Only mode can be saved as a default here. Other settings apply to this run.");
}

async function setupSaveDefaultMode() {
  try {
    await api("/api/prefs", { method: "PUT", body: { key: "mode", value: setupState.mode } });
    toast("Saved mode = " + setupState.mode + " as default.", true);
  } catch (e) {
    toast("Save failed: " + (e.message || e));
  }
}
