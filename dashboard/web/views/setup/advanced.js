// Raw setup evidence. Adapted from vllm-rdna-dash by CrispyGinger (Apache-2.0).
"use strict";

function setupAdvancedPanel() {
  const body = h("div.setup-advanced-body", loading());
  const summary = h("summary", "Raw data and terminal commands");
  const panel = h("details.advanced.setup-advanced", summary, body);
  panel.open = !!setupState.detailsOpen;
  let loaded = false;
  panel.addEventListener("toggle", async () => {
    if (!panel.open || loaded) return;
    loaded = true;
    await loadSetupAdvanced(body);
  });
  return panel;
}

/** These are snapshots, not a second source of live status. */
async function loadSetupAdvanced(body) {
  const entries = [];
  const read = async (path) => {
    try {
      entries.push({ path: path, doc: await api(path) });
    } catch (e) {
      entries.push({ path: path, error: String(e.message || e) });
    }
  };
  await read("/api/onboarding");
  await read("/api/recipes");
  await read("/api/doctor");
  await read("/api/setup/runs");
  if (setupState.recipeID) {
    const query = new URLSearchParams({ mode: setupState.mode });
    if (setupState.name) query.set("name", setupState.name);
    for (const set of setupState.sets) query.append("set", set);
    await read("/api/recipes/" + encodeURIComponent(setupState.recipeID) + "/plan?" + query.toString());
  }
  put(body,
    h("p.setup-advanced-note", "Data snapshots captured when this disclosure opened. The API and terminal expose the same records."),
    setupDownloadLimit(),
    entries.map(setupAdvancedDocument),
    h("h3", "The same thing in a terminal"),
    cmdBlock("bernie status --steps", "Copy the command"),
    cmdBlock("bernie status --json", "Copy the command"),
    cmdBlock("bernie recipe list", "Copy the command"),
    cmdBlock("bernie recipe plan " + (setupState.recipeID || "ID"), "Copy the command"),
    cmdBlock("bernie doctor", "Copy the command"));
}

/** The limit applies to active downloads only after the user saves it. */
function setupDownloadLimit() {
  const field = h("input", { type: "text", id: "setup-download-limit", placeholder: "5MB" });
  const note = h("span.small.muted", "");
  const btn = h("button.small", { type: "button" }, "Set the limit");
  const box = h("div.setup-download-limit",
    h("label", { for: "setup-download-limit" }, "Download limit"),
    field,
    btn,
    note);
  btn.addEventListener("click", async () => {
    const size = field.value.trim();
    btn.disabled = true;
    try {
      const res = await api("/api/download/limit", { method: "POST", body: { size: size } });
      note.textContent = (res && res.note) || "Changes take effect within a second.";
    } catch (e) {
      note.textContent = "The limit was not changed: " + (e.message || e);
    }
    btn.disabled = false;
  });
  api("/api/download/limit").then((res) => {
    const limit = res && res.limit ? String(res.limit) : "";
    if (!field.value) field.value = limit;
    note.textContent = (res && res.note) || "";
  }).catch(() => {});
  return box;
}

/** Raw JSON stays selectable and copyable. */
function setupAdvancedDocument(entry) {
  const text = entry.error
    ? JSON.stringify({ error: entry.error }, null, 2)
    : JSON.stringify(entry.doc, null, 2);
  return h("div.setup-advanced-document",
    h("div.setup-advanced-head",
      h("span.setup-advanced-path", entry.path),
      h("span.grow"),
      jsonLink(entry.path),
      copyBtn(text)),
    h("pre.setup-advanced-json", text));
}
