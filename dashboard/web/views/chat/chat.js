"use strict";
// One prompt, one streamed answer; no stored conversation history.
const chatState = { errors: {} };

async function viewChat(root) {
  const head = stateHead("Chat");
  const prompt = h("textarea.chat-prompt", {
    id: "chat-prompt", rows: "3", placeholder: "Ask the model one question",
    "aria-label": "Your question", disabled: true,
  });
  const send = h("button.primary", { type: "button", id: "chat-send", disabled: true }, "Send prompt");
  const stop = h("button", { type: "button", id: "chat-stop", hidden: true }, "Stop answer");
  const check = h("button", { type: "button" }, "Check again");
  const reasoningText = h("div");
  const reasoning = h("details.chat-reasoning", { hidden: true }, h("summary", "Model reasoning"), reasoningText);
  const reply = h("div.chat-reply");
  const metrics = h("p.chat-metrics", { role: "status", "aria-live": "polite" });
  const modelLine = h("div.chat-model");
  const box = h("div.chat-box", modelLine, h("label", { for: "chat-prompt" }, "Your question"), prompt,
    h("div.chat-actions", send, stop, check), h("p.small.muted", "⌘/Ctrl + Enter to send. Stop answer cancels only this response, not the model server."));
  put(root, head, box, h("div.chat-answer", reasoning, reply, metrics),
    h("p.chat-note", "A one-prompt test, not a conversation. The dashboard keeps no chat history. Use Check again after changing servers."),
    h("p.chat-note", h("a", { href: "#dashboard" }, "Model status and connection details"), " · ",
      h("a", { href: "#logs" }, "Open Logs"), " · ", h("a", { href: "#setup" }, "Start a model")));
  let controller = null, busy = false, ready = false;

  function numbers(doc) {
    const parts = [];
    if (doc.ttft_ms != null) parts.push(doc.ttft_ms + " ms to the first token");
    if (doc.tok_s != null) parts.push(doc.tok_s + " tokens/s");
    return parts.join(" · ");
  }

  function event(frame) {
    let name = "", data = "";
    for (const line of frame.split("\n")) {
      if (line.startsWith("event: ")) name = line.slice(7).trim();
      else if (line.startsWith("data: ")) data += line.slice(6);
    }
    if (!name) return null;
    let doc = null;
    try { doc = JSON.parse(data); } catch { return null; }
    return { name: name, doc: doc };
  }

  // Drain completed responses normally; cancellation uses the same request.
  async function stream(text, handlers) {
    controller = new AbortController();
    const response = await fetch("/api/chat", {
      method: "POST", headers: { "Content-Type": "application/json", "X-Bernie": "1" },
      credentials: "same-origin", body: JSON.stringify({ prompt: text, run: chatState.run || "" }), signal: controller.signal,
    });
    if (!response.ok) {
      let msg = response.status + " " + response.statusText;
      try { const doc = await response.json(); if (doc && doc.error) msg = doc.error; } catch { /* non-JSON error */ }
      throw new Error(msg);
    }
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buf = "";
    try {
      for (;;) {
        const chunk = await reader.read();
        if (chunk.done) break;
        buf += decoder.decode(chunk.value, { stream: true });
        let cut = buf.indexOf("\n\n");
        while (cut >= 0) {
          const frame = buf.slice(0, cut);
          buf = buf.slice(cut + 2);
          const ev = event(frame);
          if (ev && handlers[ev.name]) handlers[ev.name](ev.doc);
          cut = buf.indexOf("\n\n");
        }
      }
    } finally {
      try { reader.releaseLock(); } catch { /* already closed */ }
    }
  }

  async function ask() {
    if (busy || !ready) return;
    const text = prompt.value.trim();
    if (!text) { prompt.focus(); return; }
    busy = true;
    send.disabled = check.disabled = true;
    stop.hidden = false;
    reply.textContent = reasoningText.textContent = "";
    reasoning.hidden = true;
    metrics.textContent = "Sending prompt. Waiting for the model to respond…";
    let ended = false;
    try {
      await stream(text, {
        delta: (doc) => {
          if (!reply.textContent) metrics.textContent = "Receiving the answer…";
          reply.textContent += doc.text || "";
        },
        reasoning: (doc) => {
          if (reasoning.hidden) metrics.textContent = "The model is reasoning before answering…";
          reasoning.hidden = false;
          reasoningText.textContent += doc.text || "";
        },
        done: (doc) => {
          ended = true;
          metrics.textContent = numbers(doc) || "Answer complete.";
          if (!reply.textContent) reply.textContent = "The model finished without answer text.";
          stop.hidden = true;
        },
        error: (doc) => {
          ended = true;
          metrics.textContent = doc.error || "The answer failed. Open Logs for details.";
          chatState.errors["/api/chat"] = metrics.textContent;
          stop.hidden = true;
        },
      });
      if (!ended) {
        metrics.textContent = "The answer stream ended before completion. Check Logs, then try again.";
        chatState.errors["/api/chat"] = metrics.textContent;
      }
    } catch (e) {
      if (e && e.name === "AbortError") metrics.textContent = "The answer was stopped. This does not stop the model server.";
      else {
        chatState.errors["/api/chat"] = (e && e.message) || "The request failed.";
        metrics.textContent = chatState.errors["/api/chat"];
      }
    } finally {
      busy = false;
      controller = null;
      send.disabled = !ready;
      check.disabled = false;
      stop.hidden = true;
    }
  }

  async function loadModel() {
    if (busy) return;
    ready = false;
    send.disabled = prompt.disabled = check.disabled = true;
    setHead(head, "Checking model availability", "Checking which server this one-prompt test will use.");
    modelLine.textContent = "Reading model servers…";
    try {
      const runs = runsOf(await api("/api/runs"));
      const active = runs.filter((r) => r.state !== "exited");
      // Matches ChatTarget: the server picked on the Dashboard, else the oldest active one.
      let picked = "";
      try { picked = localStorage.getItem("bernie.server") || ""; } catch (e) { /* the oldest one */ }
      const live = active.find((r) => r.name === picked) || active[0];
      chatState.run = live ? live.name : "";
      if (!live) {
        setHead(head, "Start a model to chat", "No model server is active on this host.");
        put(modelLine, emptyState("No model is available yet.", h("a", { href: "#setup" }, "Start a model in Setup"), "bernie serve <recipe>"));
        return;
      }
      const phase = (live.phase || {}).id;
      const failed = phase === "failed";
      const unresponsive = live.answering === false || live.state === "unresponsive"
        || (live.state === "starting" && (live.foreign || phase === "ready"));
      ready = live.state === "running" && !failed && !unresponsive;
      let reported = "", modelError = "";
      if (ready) {
        try {
          const mod = await api("/api/models");
          if (mod && Array.isArray(mod.running) && live === active[0]) reported = mod.running[0] || "";
          modelError = mod && mod.live_error || "";
        } catch (e) { modelError = e.message || String(e); }
      }
      const plan = live.plan || {};
      const model = reported || plan.served_name || plan.model || live.model || live.recipe || live.name;
      put(modelLine, h("b", (reported ? "Model: " : "Configured model: ") + model), " · Server: " + live.name,
        active.length > 1 ? h("p.small.muted", active.length + " servers are active. Chat uses " + live.name + ", the server picked on the Dashboard.") : null,
        modelError ? h("p.small", "The model list could not be read: " + modelError + ". A prompt will test whether this server can answer.") : null);
      setHead(head, ready ? "Ready for a test prompt" : failed ? "The server reported a failure"
        : unresponsive ? "The server is not responding" : "The model is still starting",
        ready ? "The health check passed. Send one question to test an actual answer."
          : failed || unresponsive ? "Open Logs to investigate, then use Check again."
            : ((live.phase || {}).sentence || "The server has not passed its health check yet.") + " Follow progress on Dashboard, then use Check again.");
    } catch (e) {
      chatState.errors["/api/runs"] = e.message || String(e);
      setHead(head, "Model status unavailable", "The server list could not be checked; this does not mean no model is running.");
      put(modelLine, emptyState("Could not check model availability.", chatState.errors["/api/runs"] + " · Use Check again or open Dashboard.", "bernie ps"));
    } finally {
      send.disabled = prompt.disabled = !ready;
      check.disabled = false;
    }
  }

  send.addEventListener("click", ask);
  stop.addEventListener("click", () => { if (controller) controller.abort(); });
  check.addEventListener("click", loadModel);
  prompt.addEventListener("keydown", (e) => {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) { e.preventDefault(); ask(); }
  });
  await loadModel();
  if (ready) prompt.focus();
}
