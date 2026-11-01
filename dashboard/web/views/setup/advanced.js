// The Advanced panel of the Setup tab: the raw documents behind the page
// and the commands that produce them.
// Adapted from vllm-rdna-dash by CrispyGinger (Apache-2.0).
"use strict";

/** setupAdvancedPanel is the disclosure at the end of the tab. */
function setupAdvancedPanel() {
  const body = h("div.setup-advanced-body", loading());
  const summary = h("summary", "Show details");
  const panel = h("details.advanced.setup-advanced", summary, body);
  // The tab remembers the state of the disclosure: the tab repaints itself
  // when a job reports, and the disclosure stays where the reader left it.
  panel.open = !!setupState.detailsOpen;
  panel.dataset.local = setupState.detailsOpen ? "1" : "0";
  let loaded = false;
  panel.addEventListener("toggle", async () => {
    if (!panel.open || loaded) return;
    loaded = true;
    await loadSetupAdvanced(body);
  });
  return panel;
}

/** loadSetupAdvanced reads every document the tab used and paints it with
 *  the command that produces the same answer. */
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
  if (setupState.recipeID) {
    await read("/api/recipes/" + encodeURIComponent(setupState.recipeID) + "/plan");
  }
  put(body,
    h("p.setup-advanced-note", "The documents behind this page. An agent gets the same ones from the API."),
    setupDownloadLimit(),
    entries.map(setupAdvancedDocument),
    h("h3", "The same thing in a terminal"),
    cmdBlock("bernie status --steps", "Copy the command"),
    cmdBlock("bernie status --json", "Copy the command"),
    cmdBlock("bernie recipe list", "Copy the command"),
    cmdBlock("bernie recipe plan " + (setupState.recipeID || "ID"), "Copy the command"),
    cmdBlock("bernie doctor", "Copy the command"));
}

/** setupDownloadLimit is the control for the speed limit of the model
 *  download (R21). The value is in force within a second for every
 *  running download. */
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

/** setupAdvancedDocument is one raw document with its endpoint and a copy
 *  button. */
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
