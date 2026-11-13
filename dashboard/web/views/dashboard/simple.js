"use strict";
// Engine identity, live performance and the host's GPU comparison.
function dashSimpleHero() {
  const el = h("section.state-head.dash-simple-hero");
  const left = h("div.dash-min0");
  const right = h("div.dash-simple-speed");
  const layout = h("span.dash-muted");
  const technical = h("span.dash-muted");
  const statusBadge = h("span.dash-engine-status", { role: "status" });
  const config = dashEngineConfig();
  let lastStatus = "";
  const stop = h("button", { type: "button" }, "Stop server");
  const start = h("a.btn.primary", { href: "#setup" }, "Start a model");
  const chat = h("a.btn.primary", { href: "#chat" }, "Test in Chat");
  const connect = h("a.btn", { href: "#dashboard/connect" }, "Connect an app");
  const logs = h("a.btn", { href: "#logs" }, "Open Logs");
  const attach = dashAttachPanel();
  const pick = h("select.dash-pick", { "aria-label": "The server the dashboard shows" });
  let pickKey = "";
  pick.addEventListener("change", () => {
    dashPickRun(pick.value);
    dashUpdateSimple();
  });
  let shown = null;
  stop.addEventListener("click", async () => {
    stop.disabled = true;
    try { await dashStopRun(shown); } finally { stop.disabled = false; }
  });
  const word = h("h1.display", "Reading model status");
  const lede = h("p.lede");
  const scope = h("p.small.muted");
  const endpointLabel = h("span.dash-muted", "App base URL");
  const endpointCode = h("code");
  const endpointCopy = copyBtn("", "Copy the model API base URL");
  const endpointNone = h("span.dash-muted");
  const metricsNote = h("p.small.muted");
  left.append(h("div.dash-simple-top", h("span.dash-pick-row", h("span.eyebrow", "vLLM engine"), pick), statusBadge), word,
    h("div.dash-engine-identity", layout, technical), lede,
    h("div.dash-simple-actions", start, chat, connect, logs, stop), attach, scope,
    h("div.dash-simple-endpoint", endpointLabel, endpointCode, endpointCopy, endpointNone), metricsNote);
  const amount = h("span", "–");
  const unit = h("small.dash-hero-unit", "tokens/s");
  const note = h("div.dash-hero-note");
  const chart = spark([], null, "Output speed of the shown server", { unit: "tok/s" });
  right.append(h("div.eyebrow", "Output speed"), h("div.dash-hero-num", amount, unit),
    note, h("div.dash-hero-spark", chart));
  el.append(h("div.dash-simple-hero-grid", left, right), config.el);
  const view = {
    el: el,
    update: () => {
      const run = dashLiveRun();
      shown = run;
      const status = dashRunStatus(run);
      const tele = status.tele;
      layout.textContent = run ? "Server · " + run.name : "No server on this host";
      technical.textContent = run ? (run.foreign ? "Attached server" + (runRemote(run) ? " on " + run.host : "") : run.mode === "container"
        ? (run.engine || "Container") + " deployment" : run.mode === "host" ? "Host process" : "Deployment not recorded") : "";
      const title = run ? (run.plan && (run.plan.served_name || run.plan.model)) || run.model || run.name : status.label;
      if (word.textContent !== title) word.textContent = title;
      const statusKey = status.kind + status.label;
      if (lastStatus !== statusKey) {
        put(statusBadge, statePill(status.kind, status.label));
        lastStatus = statusKey;
      }
      el.dataset.state = status.kind;
      lede.textContent = status.ready ? "OpenAI-compatible API · health check passed." : status.message;
      config.update(run);
      const live = state.tele || dash.tele;
      const runs = !status.unknown && live && Array.isArray(live.runs) ? live.runs : runsOf(state.runs);
      const active = runs.filter((r) => r.state !== "exited");
      const ready = active.filter((r) => r.state === "running" && r.answering !== false).length;
      scope.textContent = active.length > 1 ? active.length + " servers are active"
        + (status.unknown ? "; live status is unavailable. " : ", " + ready + " ready. ")
        + "Pick one above: the numbers, Chat and Connect an app use " + run.name + "." : "";
      const names = active.map((r) => r.name);
      pick.hidden = names.length < 2;
      if (names.join("\n") !== pickKey) {
        pickKey = names.join("\n");
        pick.replaceChildren(...names.map((n) => h("option", { value: n }, n)));
      }
      if (run && pick.value !== run.name) pick.value = run.name;
      scope.hidden = !scope.textContent;
      const endpoint = connectEndpoint(status.active ? run : null);
      endpointCode.textContent = endpoint.url;
      endpointLabel.hidden = endpointCode.hidden = endpointCopy.hidden = !endpoint.url;
      endpointCopy.setAttribute("data-copy", endpoint.url);
      endpointNone.hidden = !!endpoint.url;
      endpointNone.textContent = status.active ? "No model API address was reported." : "Start a model to get an app connection URL.";
      stop.hidden = !status.active;
      stop.textContent = run && run.foreign ? "Detach server" : "Stop server";
      stop.title = status.active ? (run.foreign ? "Forget attached server " : "Stop ") + run.name : "";
      start.hidden = status.active || status.unknown;
      attach.hidden = status.unknown;
      chat.hidden = !status.ready;
      connect.hidden = !status.active;
      logs.hidden = !run && !status.unknown;
      metricsNote.textContent = tele && tele.error ? "Live metrics unavailable: " + tele.error
        : status.ready && !(tele && tele.stats) ? "The server is ready, but live speed and request metrics are not available yet." : "";
      metricsNote.hidden = !metricsNote.textContent;
      const gen = status.ready && tele && tele.stats ? tele.gen_tok_s : null;
      amount.textContent = gen == null ? "–" : num(gen);
      unit.style.opacity = gen == null ? ".55" : "1";
      const series = dashSeries("gen");
      const values = series.filter((v) => v != null);
      const avg = values.length ? values.reduce((a, b) => a + b, 0) / values.length : null;
      note.textContent = status.unknown ? "Live readings are unavailable."
        : !status.active ? "Send a prompt after starting a model."
          : !status.ready ? "Speed appears when the server is ready."
            : gen == null ? "Waiting for a live speed reading."
              : gen === 0 && tele.stats.requests_running === 0 && !tele.stats.requests_waiting ? "Idle · send a prompt to see output speed."
                : avg == null ? "Generated tokens per second." : "Recent sample average: " + num(avg) + " tokens/s";
      chart.update(status.unknown ? [] : series);
    },
  };
  view.update();
  return view;
}

// These are recorded launch settings, never inferred from GPU count or model name.
function dashEngineConfig() {
  const el = h("div.dash-engine-config");
  const source = h("p.dash-engine-source");
  const facts = h("dl.dash-engine-facts");
  const values = ["Parallelism", "Context limit", "Precision", "Quantization"].map((label) => {
    const row = factRow(label, "–");
    facts.append(row);
    return row.querySelector("dd");
  });
  el.append(source, facts);
  return { el, update: (run) => {
    el.hidden = !run;
    const plan = run && run.plan;
    facts.hidden = !plan;
    source.textContent = plan && plan.model ? "Model source · " + plan.model
      : "Model source and launch configuration were not recorded for this server.";
    const args = plan && Array.isArray(plan.args) ? plan.args : [];
    const option = (name) => {
      const flag = "--" + name;
      for (let i = 0; i < args.length; i++) {
        if (args[i] === flag && args[i + 1] && !args[i + 1].startsWith("--")) return args[i + 1];
        if (args[i].startsWith(flag + "=")) return args[i].slice(flag.length + 1);
      }
      return null;
    };
    const tp = option("tensor-parallel-size"), pp = option("pipeline-parallel-size");
    const context = option("max-model-len");
    const readings = [[tp ? "TP " + tp : "", pp ? "PP " + pp : ""].filter(Boolean).join(" · "),
      context && Number.isFinite(Number(context)) ? num(Number(context), 0) + " tokens" : context,
      option("dtype"), option("quantization")];
    readings.forEach((reading, i) => {
      const value = reading || "Not recorded";
      if (values[i].textContent !== value) values[i].textContent = value;
    });
  } };
}

// Stop only the named server shown above; attached servers are only forgotten.
/** runRemote is true for an attached server on another host. */
function runRemote(r) { return !!r && r.foreign && !/^(127\.|localhost$|::1$|0\.0\.0\.0$|$)/.test(r.host || ""); }

async function dashStopRun(run = dashLiveRun()) {
  if (!run || run.state === "exited") { toast("There is no run to stop."); return; }
  const question = run.foreign ? "Detach " + run.name + "? The model server will keep running."
    : "Stop " + run.name + "? Active requests to this server will be interrupted.";
  if (!window.confirm(question)) return;
  try {
    await api("/api/runs/" + encodeURIComponent(run.name), { method: "DELETE" });
    toast(run.foreign ? "Detached " + run.name + "; its server is still running." : "Stopped " + run.name + ".", true);
    await dashLoad();
  } catch (e) { toast((run.foreign ? "Detach" : "Stop") + " failed: " + (e.message || e)); }
}

function dashMeaning(term) {
  const words = dash.glossary || {};
  return words[term] || "bernie did not publish an explanation for this measure.";
}

function dashSimpleTiles() {
  const box = h("div.tiles.dash-simple-tiles");
  const defs = [
    { key: "prompt", label: "Prompt processing", unit: "tokens/s", term: "Prompt reading" },
    { key: "running", label: "Active requests", unit: "requests", term: "Active requests" },
    { key: "ttft", label: "Time to first token", unit: "s", term: "First token" },
    { key: "kv", label: "KV cache", unit: "%", term: "KV cache" },
    { key: "prefix", label: "Prefix cache hits", unit: "%", term: "Prefix cache hits" },
    { key: "spec", label: "Draft accepted", unit: "%", term: "Draft accepted" },
  ];
  const tiles = defs.map((d) => {
    const tile = dashTile({ label: d.label, unit: d.unit, tip: dashMeaning(d.term), value: "–", sub: "" });
    tile.key = d.key;
    if (["prefix", "spec"].includes(d.key)) tile.classList.add("advanced-only");
    box.append(tile);
    return tile;
  });
  box.items = tiles;
  box.update = () => {
    const run = dashLiveRun();
    const status = dashRunStatus(run);
    const last = dash.samples[dash.samples.length - 1];
    const agg = status.ready && status.tele && status.tele.stats && last && last.aggs ? last.aggs[run.name] || null : null;
    for (const tile of tiles) {
      const key = tile.key;
      let value = "–", max = null;
      let sub = status.unknown ? "Live readings unavailable" : !status.active ? "Start a model to see this"
        : !status.ready ? "Available when the server is ready"
          : status.tele && status.tele.error ? "Live metrics unavailable" : "Waiting for live metrics";
      if (agg) {
        if (key === "prompt") {
          value = agg.prompt == null ? "–" : num(agg.prompt);
          sub = agg.prompt == null ? "Not reported by the server" : "tokens of input processed per second";
        } else if (key === "running") {
          value = agg.running == null ? "–" : num(agg.running, 0);
          sub = agg.running == null ? "Not reported by the server"
            : agg.waiting == null ? "Queue size not reported" : num(agg.waiting, 0) + " waiting in the queue";
        } else if (key === "ttft") {
          value = agg.ttft == null ? "–" : num(agg.ttft, 3);
          sub = agg.ttft == null ? (agg.statsKnown ? "Send a prompt to measure this" : "Waiting for live metrics")
            : "Typical wait" + (agg.ttftP95 == null ? "" : " · 95% within " + num(agg.ttftP95, 3) + " s");
        } else if (key === "kv") {
          value = agg.kv == null ? "–" : num(agg.kv * 100, 0);
          sub = agg.kv == null ? "Not reported by the server" : "of GPU cache in use";
          max = 100;
        } else if (key === "prefix") {
          value = agg.prefix == null ? "–" : num(agg.prefix, 1);
          sub = agg.prefix == null
            ? (agg.prefixKnown === false
              ? "this server has no prefix cache"
              : agg.statsKnown ? "No prompt processed yet" : "Waiting for live metrics")
            : num(agg.prefixCached, 0) + " of " + num(agg.prefixQueried, 0) + " prompt tokens came from cache";
          max = 100;
        } else if (key === "spec") {
          value = agg.spec == null ? "–" : num(agg.spec, 0);
          sub = agg.spec == null
            ? (agg.specOn === false
              ? "speculation is off"
              : agg.statsKnown ? "No draft tokens yet" : "Waiting for live metrics")
            : "MTP drafts kept";
          max = 100;
        }
      }
      tile.update({ value: value, sub: sub, series: dashSeries(key), max: max });
    }
  };
  box.update();
  return box;
}

function dashSimpleCharts(section) {
  const grid = h("div.dash-simple-chart-grid");
  const hint = "Rates include all model servers. No live metrics means a gap, not zero.";
  const output = areaChart([], { title: "All servers · output tokens/s · last 5 min", unit: "tok/s", hint: hint, windowMinutes: 5 });
  const prompt = areaChart([], { title: "All servers · input tokens/s · last 5 min", unit: "tok/s", hint: hint, windowMinutes: 5 });
  grid.append(output, prompt);
  const seg = h("div.segmented-control", { role: "group", "aria-label": "Seconds per point of the charts" });
  const paint = () => {
    output.update(dashWindowSeries("gen", null, "5m"), 5);
    prompt.update(dashWindowSeries("prompt", null, "5m"), 5);
  };
  for (const g of [1, 5, 10, 15]) {
    const button = h("button", { type: "button", "aria-pressed": String(g === dash.group) }, g + " s");
    button.addEventListener("click", () => {
      dash.group = g;
      try { localStorage.setItem("bernie.chart-group", String(g)); } catch (e) { /* the page still works */ }
      for (const b of seg.children) b.setAttribute("aria-pressed", String(b === button));
      paint();
    });
    seg.append(button);
  }
  section.append(h("div.dash-group", h("span.small.muted", "One point per"), seg), grid);
  return { update: paint };
}

function dashSimpleMachine(section) {
  const el = h("div.card.dash-simple-panel");
  const facts = h("dl.kv.dash-machine-facts");
  el.append(cardHead("This machine", "/api/hw"), facts);
  section.append(el);
  return {
    update: () => {
      const hw = dash.hw || {};
      const host = hw.host || {};
      const gpus = hw.gpus || [];
      const rows = dash.hw ? [
        ["Host", host.hostname || "Not reported", ""],
        ["CPU", host.cpu && host.cpu.model ? host.cpu.model : "Not reported", ""],
        ["RAM", host.ram_bytes ? bytes(host.ram_bytes) : "Not reported", ""],
        ["GPUs", gpus.length + (gpus.length === 1 ? " card" : " cards"),
          gpus.map((g) => g.name || g.gfx).filter(Boolean).join(" · ")],
      ] : [["Hardware", "Unavailable", (dash.errors || {})["/api/hw"] || "Hardware data has not arrived."]];
      put(facts, rows.map(([label, value, why]) => h("div",
        h("dt", label), h("dd", value, why ? h("span.why", why) : null))));
    },
  };
}

// The overview leads to an action; detailed charts stay secondary.
function dashBuildSimple(root) {
  const hero = dashSimpleHero();
  const tiles = dashSimpleTiles();
  const engine = dashSection("Engine metrics", "engine");
  engine.append(tiles);
  const minutes = dashSection("Host-wide throughput history", "simple-minutes");
  minutes.classList.add("advanced-only");
  const gpus = dashSection("GPU overview", "gpus");
  gpus.id = "dashboard-gpus";
  const bottom = h("div.dash-simple-bottom");
  const details = dashDetails();
  put(root, hero.el, engine, gpus, bottom, minutes, details.el);
  const api = {
    hero: hero,
    tiles: tiles,
    charts: dashSimpleCharts(minutes),
    gpus: dashGPUOverview(gpus),
    connect: dashSimpleConnect(bottom),
    machine: dashSimpleMachine(bottom),
  };
  dash.live.simple = api;
  return api;
}

function dashSimpleConnect(section) {
  const el = h("div.card.dash-simple-panel", { id: "dashboard-connect" });
  const text = h("p.dash-connect-text");
  const command = h("div.dash-connect-command");
  const segmented = h("div.segmented-control", { role: "group", "aria-label": "The language of the connect snippet" });
  let kind = "curl";
  const buttons = {};
  for (const item of [{ id: "curl", label: "curl" }, { id: "python", label: "Python" }]) {
    const button = h("button", { type: "button", "aria-pressed": String(kind === item.id) }, item.label);
    button.addEventListener("click", () => {
      kind = item.id;
      for (const [id, other] of Object.entries(buttons)) other.setAttribute("aria-pressed", String(id === kind));
      paint();
    });
    buttons[item.id] = button;
    segmented.append(button);
  }
  let painted = "";
  const local = h("p.small.muted", "127.0.0.1 is local to the model host. An app on another machine needs that host’s reachable address and model port.");
  const paint = () => {
    const run = dashLiveRun();
    const status = dashRunStatus(run);
    const endpoint = connectEndpoint(status.active ? run : null);
    local.hidden = runRemote(run);
    const key = kind + "|" + endpoint.base + "|" + endpoint.model + "|" + status.label;
    if (key === painted) return;
    painted = key;
    if (!endpoint.base) {
      const inspect = status.active || status.unknown;
      put(text, inspect ? "The server’s API address is unavailable. " : "Start a model to get connection details. ",
        h("a", { href: inspect ? "#logs" : "#setup" }, inspect ? "Inspect the server in Logs" : "Open Setup"));
      put(command, emptyState("No model API address is available.",
        "An app needs the model server’s base URL, not the dashboard URL.", inspect ? "bernie ps" : "bernie serve <recipe>"));
      return;
    }
    put(text, "Use this base URL in an OpenAI-compatible app: ", h("code", endpoint.url),
      ". Configured model: ", h("b", endpoint.model), ". This is the model API, not the dashboard address. ",
      status.ready ? "" : "The server is not confirmed ready; check its status above before sending requests.");
    const snippet = connectSnippet(kind, endpoint);
    put(command, cmdBlock(snippet, "Copy the " + kind + " snippet"));
  };
  el.append(cardHead("Connect an app"), text, local,
    h("details.advanced", h("summary", "Example requests · curl or Python"), segmented, command));
  section.append(el);
  return { update: paint };
}
