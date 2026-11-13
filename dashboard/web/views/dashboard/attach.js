"use strict";
// The panel that attaches a vLLM server bernie did not start: one found on
// this host, or one at an address of another host (spec §8.4, §8.5).

/** dashAttachPanel is the "Use a server that is already running" panel. */
function dashAttachPanel() {
  const attachURL = h("input", { inputmode: "url", placeholder: "http://chardis:8000", "aria-label": "Address of the running vLLM server" });
  const attachBtn = h("button", { type: "submit" }, "Attach");
  const attachForm = h("form.dash-attach-form", attachURL, attachBtn);
  const found = h("div.dash-found");
  const attach = h("details.dash-attach", h("summary", "Use a server that is already running"), found, attachForm);
  // Opening the panel scans this host for vLLM servers bernie does not know yet.
  attach.addEventListener("toggle", async () => {
    if (!attach.open) return;
    put(found, h("p.small.muted", "Looking for vLLM servers on this host…"));
    let doc = null;
    try { doc = await api("/api/discover"); } catch (e) { put(found, h("p.small.muted", "The scan failed: " + (e.message || e))); return; }
    const free = (doc.servers || []).filter((f) => f.healthy && !f.run);
    put(found, h("p.small.muted", free.length ? "Found on this host:" : "No other vLLM server answers on this host. Enter the address of one on another host."),
      ...free.map((f) => {
        const b = h("button", { type: "button" }, "Attach " + f.url + (f.model ? " · " + f.model : ""));
        b.addEventListener("click", () => {
          attachURL.value = f.url;
          attachURL.dataset.name = (f.container || (f.model || "").split("/").pop() || "vllm-" + f.port).toLowerCase().replace(/[^a-z0-9._-]+/g, "-");
          attachForm.requestSubmit();
        });
        return b;
      }));
  });
  // An attached server is named after its host: chardis for http://chardis:8000.
  const attachName = (raw) => {
    let host = "";
    try { host = new URL(raw.includes("://") ? raw : "http://" + raw).hostname; } catch (e) { /* the server checks the address */ }
    const name = /^\d+$/.test(raw.trim()) ? "" : host.toLowerCase().replace(/[^a-z0-9._-]+/g, "-").replace(/^[^a-z0-9]+/, "");
    return !name || /^(127\.|localhost$)/.test(name) ? "attached" : name;
  };
  attachForm.addEventListener("submit", async (e) => {
    e.preventDefault();
    attachBtn.disabled = true;
    try {
      const res = await api("/api/attach", { method: "POST", body: { url: attachURL.value.trim(), name: attachURL.dataset.name || attachName(attachURL.value) } });
      toast(res.note, true);
      attachURL.value = attachURL.dataset.name = "";
      attach.open = false;
      await dashLoad();
    } catch (err) { toast("Attach failed: " + (err.message || err)); } finally { attachBtn.disabled = false; }
  });
  return attach;
}
