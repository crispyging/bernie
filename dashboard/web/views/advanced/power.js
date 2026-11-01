"use strict";
// Save a desired cap explicitly; the service enforces the GPU's allowed range.
function advPowerCard() {
  const card = h("div.card");
  const view = {
    el: card,
    power: null,
    update: (power) => {
      view.power = power;
      advPaintPower(card, view);
    },
    reload: async () => {
      try {
        view.power = await api("/api/power");
      } catch (e) {
        view.power = null;
      }
      advPaintPower(card, view);
    },
  };
  return view;
}

/** Current, allowed and desired limits precede the write controls. */
function advPaintPower(card, view) {
  const p = view.power;
  if (!p) {
    put(card, cardHead("Power limits", "/api/power"),
      emptyState("Power limits could not be read.",
        "Re-check the host, or inspect its power service with the command below. No limit can be changed here until it is available.",
        "bernie tune power status"));
    return;
  }
  const service = p.installed
    ? "installed, " + (p.enabled ? "enabled" : "not enabled") + ", "
      + (p.active ? "watching changes" : "not watching changes")
    : "not installed";
  const children = [
    cardHead("Power limits", "/api/power", badge(p.state || "skip")),
    h("p.small", "Changing a limit affects power draw, heat and performance. Edit a target, then save and confirm; viewing this page does not change it."),
    h("div.muted", "Service: " + service),
  ];
  if (!p.installed) {
    children.push(h("div.err",
      "The power service is not installed. Install it before changing a cap; the controls stay disabled."));
    for (const command of p.install_commands || []) {
      children.push(cmdBlock(command, "Copy the install command"));
    }
  } else if (!p.enabled || !p.active) {
    children.push(h("div.err",
      "The power service is not enabled or is not watching the desired-state file."));
  }
  const rows = (p.gpus || []).map((g) => advPowerRow(g, p.installed, view));
  if (rows.length) {
    children.push(h("div.scroll", h("table",
      tableHead(["GPU", "Current limit", "Allowed range", "Saved target", "New target", "State"]),
      h("tbody", rows))));
  } else {
    children.push(h("div.empty", "No GPU reports a controllable power limit. Check GPU detection in Setup if you expected one."));
  }
  put(card, ...children);
}

/** Editing inputs does not write; only Save limit can apply the target. */
function advPowerRow(g, installed, view) {
  const wanted = g.effective_w != null ? g.effective_w : (g.desired_w != null ? g.desired_w : g.cap_w);
  const min = g.min_w || 1;
  const max = g.max_w || Math.max(min, g.cap_w || min);
  const value = Math.max(min, Math.min(max, wanted || min));
  const range = h("input", {
    type: "range", min: String(min), max: String(max), step: "1", value: String(value),
    disabled: !installed, "aria-label": "GPU " + g.hip_index + " desired watts",
  });
  const number = h("input", {
    type: "number", min: String(min), max: String(max), step: "1", value: String(value),
    disabled: !installed, "aria-label": "GPU " + g.hip_index + " desired watts",
  });
  range.addEventListener("input", () => { number.value = range.value; });
  number.addEventListener("input", () => { range.value = number.value; });
  const save = h("button.small", { type: "button", disabled: !installed }, "Save limit");
  const apply = async () => {
    let watts = Number(number.value);
    if (!Number.isFinite(watts)) {
      toast("Enter a power cap in watts.");
      return;
    }
    watts = Math.max(min, Math.min(max, watts));
    number.value = String(watts);
    range.value = String(watts);
    if (!confirm("Set GPU " + g.hip_index + " to a desired limit of " + watts + " W? The service applies this limit, affecting power draw and performance.")) return;
    save.disabled = true;
    await advSavePower(g.hip_index, watts, view);
    save.disabled = false;
  };
  save.addEventListener("click", apply);
  const drift = g.drift ? h("span.badge.b-warn", "drift") : h("span.badge.b-ok", "in sync");
  const desired = g.desired_w == null ? "–" : Math.round(g.desired_w) + " W";
  return h("tr", { "data-power-gpu": String(g.hip_index) },
    h("td", "GPU " + g.hip_index),
    h("td", g.cap_w == null ? "not reported" : Math.round(g.cap_w) + " W"),
    h("td", Math.round(min) + "–" + Math.round(max) + " W"),
    h("td", desired),
    h("td", h("div.row", range, number, save)),
    h("td", drift));
}

/** Save the target, then read back the service's clamped result. */
async function advSavePower(gpu, watts, view) {
  try {
    const answer = await api("/api/power", { method: "PUT", body: { gpu, watts } });
    const clamped = answer && answer.clamped
      ? " The service clamped it to " + answer.watts + " W."
      : "";
    toast("Desired power cap saved. The service will apply it." + clamped, true);
    await view.reload();
  } catch (e) {
    toast("Power cap not saved: " + (e.message || e));
  }
}
