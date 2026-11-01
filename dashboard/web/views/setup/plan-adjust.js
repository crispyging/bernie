// "The GPUs and the settings of this plan" disclosure of the recipe plan.
// Loaded lazily so the first paint stays under budget. Holds the GPU
// selection (ia.md R44) and the "Save as default" button (ia.md R43).
// Adapted from vllm-rdna-dash by CrispyGinger (Apache-2.0).
"use strict";

/** setupPlanAdjustBody is the disclosure body: GPU checkboxes, a one-line
 *  note about settings, and the "Save as default" hint. The structured
 *  form lives in recipes.js; this owns plan-wide things. */
function setupPlanAdjustBody() {
  return h("div.recipe-plan-adjust-body",
    setupPlanGPUSelector(),
    h("p.small.muted",
      "Settings and reasons are in the list below. Re-running the plan after a change picks the new GPUs and rebuilds the command."),
    setupSaveDefaultNote());
}

/** setupPlanGPUSelector: checkbox list of GPUs bernie may use (ia.md R44).
 *  Sends env.HIP_VISIBLE_DEVICES=... — the same flag spec §7.7 accepts. */
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
    h("p.small.muted", "One checkbox per detected card. The selection writes env.HIP_VISIBLE_DEVICES in the plan and the launch command."),
    rows,
    h("button.small", { type: "button", onclick: setupClearGPUSelection }, "Use the plan's default GPUs"));
}

/** setupPlanGPUSelected: HIP indices the reader has checked. Reads live
 *  checkboxes so the selection survives the plan reload. */
function setupPlanGPUSelected() {
  const box = setupState.planBox;
  if (!box) return [];
  const out = [];
  box.querySelectorAll(".recipe-gpu-selector input[type=checkbox]").forEach((cb) => {
    if (cb.checked) out.push(Number(cb.value));
  });
  return out;
}

/** setupApplyGPUSelection writes env.HIP_VISIBLE_DEVICES=... and rebuilds. */
async function setupApplyGPUSelection() {
  const prefix = "env.HIP_VISIBLE_DEVICES=";
  const sets = setupState.sets.filter((s) => !s.startsWith(prefix));
  const picked = setupPlanGPUSelected();
  if (picked.length) sets.push(prefix + picked.join(","));
  setupState.sets = sets;
  await loadSetupPlan();
}

/** setupClearGPUSelection drops env.HIP_VISIBLE_DEVICES so the plan picks
 *  GPUs with the rule of spec §7.7. */
async function setupClearGPUSelection() {
  setupState.sets = setupState.sets.filter((s) => !s.startsWith("env.HIP_VISIBLE_DEVICES="));
  await loadSetupPlan();
}

/** setupSaveDefaultButton: ghost button next to the mode switch. Renders
 *  only when the config knows "mode" (ia.md R43: "Save as default" only
 *  if the config key exists — checked against `bernie config show`). */
function setupSaveDefaultButton() {
  const keys = setupState.prefsKeys;
  if (!Array.isArray(keys) || !keys.includes("mode")) return null;
  const btn = h("button.small", { type: "button", onclick: setupSaveDefaultMode }, "Save the mode as default");
  btn.title = "bernie config set mode " + setupState.mode;
  return btn;
}

/** setupSaveDefaultNote: the only persistable field is mode. */
function setupSaveDefaultNote() {
  const keys = setupState.prefsKeys;
  if (!Array.isArray(keys) || !keys.includes("mode")) return null;
  return h("p.small.muted",
    "Save the mode as default persists across runs (bernie config set mode). Port, max context length, run name and model are per-run.");
}

/** setupSaveDefaultMode writes the current mode to the user defaults. */
async function setupSaveDefaultMode() {
  try {
    await api("/api/prefs", { method: "PUT", body: { key: "mode", value: setupState.mode } });
    toast("Saved mode = " + setupState.mode + " as default.", true);
  } catch (e) {
    toast("Save failed: " + (e.message || e));
  }
}
