"use strict";
// ---------- Dashboard: the simple operational view ----------
// The SIMPLE view answers the question of the tab in the order a person
// reads it: is it serving, how fast is it, what are the six serving
// measures, what do the cards do, what machine is this and how does an app
// connect? It uses the shared components of bernie and the one telemetry
// stream; it opens no fetch, stream or timer of its own.
//
// The Advanced tab keeps bernie's diagnostic depth; this file owns only
// the clear operational view of the Dashboard tab.

/* ---------- the hero ---------- */

/** dashSimpleHero is the first card of the Simple view: the state in one
 * word, one sentence, the endpoint and its copy button, the action and the
 * output speed with its unit. */
function dashSimpleHero() {
  const el = h("section.state-head.dash-simple-hero");
  const grid = h("div.dash-simple-hero-grid");
  const left = h("div.dash-min0");
  const right = h("div.dash-simple-speed");
  const top = h("div.dash-simple-top");
  const pill = h("span");
  const layout = h("span.dash-muted");
  const actions = h("div.dash-simple-actions");
  const stop = h("button", { type: "button" }, "Stop");
  const start = h("a.btn.primary", { href: "#setup" }, "Start a run");
  stop.addEventListener("click", () => dashStopRun());
  actions.append(stop, start);
  const word = h("h1.display", { "aria-live": "polite" }, "NOT RUNNING");
  const lede = h("p.lede", "The model server is not running.");
  const endpoint = h("div.dash-simple-endpoint");
  const endpointCode = h("code");
  const endpointCopy = copyBtn("", "Copy the endpoint URL");
  const endpointNone = h("span.dash-muted", "No endpoint while no run is serving.");
  endpoint.append(endpointCode, endpointCopy, endpointNone);
  top.append(pill, layout, actions);
  left.append(top, word, lede, endpoint,
    h("p.small.muted", h("a", { href: "#topology" }, "P2P readiness: open the Topology tab")));
  const eyebrow = h("div.eyebrow", "Output speed");
  const value = h("div.dash-hero-num");
  const amount = h("span", "–");
  const unit = h("small", { class: "dash-hero-unit" }, "tok/s");
  value.append(amount, unit);
  const note = h("div.dash-hero-note", "No run is serving.");
  const sparkBox = h("div.dash-hero-spark");
  const chart = spark([], null, "the output speed of the live run", { unit: "tok/s" });
  sparkBox.append(chart);
  right.append(eyebrow, value, note, sparkBox);
  grid.append(left, right);
  el.append(grid);

  const api = {
    update: () => {
      const run = dashLiveRun();
      const tele = dash.tele && (dash.tele.runs || []).find((r) => run && r.name === r.name);
      const active = !!run && run.state !== "exited";
      const serving = active && !!tele && !!tele.stats;
      const starting = active && !serving;
      const kind = serving ? "ok" : starting ? "running" : "skip";
      const label = serving ? "Serving" : starting ? "Starting" : "No run";
      pill.replaceChildren(statePill(kind, label));
      layout.textContent = active
        ? [run.name, run.mode || "host", run.port ? "port " + run.port : null].filter(Boolean).join(" · ")
        : "no run on this host";
      word.textContent = serving ? "SERVING" : starting ? "STARTING" : "NOT RUNNING";
      lede.textContent = serving
        ? (run.name || "The model") + " is answering requests on port " + (run.port || "–") + "."
        : starting
          ? (dashPhase(run) || "The server is loading the model.")
          : "The model server is not running.";

      const endpointInfo = connectEndpoint(active ? run : null);
      endpointCode.textContent = endpointInfo.url;
      endpointCode.hidden = !endpointInfo.url;
      endpointCopy.setAttribute("data-copy", endpointInfo.url);
      endpointCopy.hidden = !endpointInfo.url;
      endpointNone.hidden = !!endpointInfo.url;
      stop.hidden = !active;
      start.hidden = active;

      const gen = tele ? tele.gen_tok_s : null;
      amount.textContent = gen == null ? "–" : num(gen);
      unit.style.opacity = gen == null ? ".55" : "1";
      const values = dashSeries("gen").filter((v) => v != null);
      const avg = values.length ? values.reduce((a, b) => a + b, 0) / values.length : null;
      note.textContent = gen == null
        ? "No run is serving."
        : avg == null ? "the live run" : "1-min avg " + num(avg) + " tok/s";
      chart.update(dashSeries("gen"));
    },
  };
  api.el = el;
  api.update();
  return api;
}

/** dashStopRun stops the run that serves. The write keeps the token and
 * CSRF rules of the shared api() helper. */
async function dashStopRun() {
  const run = dashLiveRun();
  if (!run || run.state === "exited") { toast("There is no run to stop."); return; }
  try {
    await api("/api/runs/" + encodeURIComponent(run.name), { method: "DELETE" });
    toast("The run " + run.name + " was stopped.", true);
    await dashLoad();
  } catch (e) {
    toast("Stop failed: " + (e.message || e));
  }
}

/* ---------- the six serving tiles ---------- */

/** dashMeaning is the plain meaning of a word from the shared glossary. */
function dashMeaning(term) {
  const words = dash.glossary || {};
  return words[term] || "bernie did not publish an explanation for this measure.";
}

/** dashSimpleTiles are the six plain-language measures of a serving host. */
function dashSimpleTiles() {
  const box = h("div.tiles.dash-simple-tiles");
  const defs = [
    { key: "prompt", label: "Prompt reading", unit: "tok/s", term: "Prompt reading" },
    { key: "running", label: "Active requests", unit: "requests", term: "Active requests" },
    { key: "ttft", label: "First token", unit: "s", term: "First token" },
    { key: "kv", label: "KV cache", unit: "%", term: "KV cache" },
    { key: "prefix", label: "Prefix cache hits", unit: "%", term: "Prefix cache hits" },
    { key: "spec", label: "Draft accepted", unit: "%", term: "Draft accepted" },
  ];
  const tiles = defs.map((d) => {
    const tile = dashTile({ label: d.label, unit: d.unit, tip: dashMeaning(d.term), value: "–", sub: "" });
    tile.key = d.key;
    box.append(tile);
    return tile;
  });
  box.items = tiles;
  box.update = () => {
    const last = dash.samples[dash.samples.length - 1];
    const agg = last ? last.agg : null;
    for (const tile of tiles) {
      const key = tile.key;
      let value = "–", sub = "n/a on this host", max = null;
      if (agg) {
        if (key === "prompt") {
          value = agg.prompt == null ? "–" : num(agg.prompt);
          sub = agg.prompt == null ? "n/a on this host" : "reading the prompt";
        } else if (key === "running") {
          value = agg.running == null ? "–" : num(agg.running, 0);
          sub = agg.running == null ? "n/a on this host"
            : agg.waiting == null ? "the waiting count is not published"
              : num(agg.waiting, 0) + " waiting in line";
        } else if (key === "ttft") {
          value = agg.ttft == null ? "–" : num(agg.ttft, 3);
          sub = agg.ttft == null
            ? (agg.statsKnown ? "the server measured no first token yet" : "n/a on this host")
            : "p95 " + num(agg.ttftP95, 3) + " s";
        } else if (key === "kv") {
          value = agg.kv == null ? "–" : num(agg.kv * 100, 0);
          sub = agg.kv == null ? "n/a on this host" : "of GPU cache in use";
          max = 100;
        } else if (key === "prefix") {
          value = agg.prefix == null ? "–" : num(agg.prefix, 1);
          sub = agg.prefix == null
            ? (agg.prefixKnown === false
              ? "this server has no prefix cache"
              : agg.statsKnown ? "the server read no prompt yet" : "n/a on this host")
            : num(agg.prefixCached, 0) + " of " + num(agg.prefixQueried, 0) + " prompt tokens came from cache";
          max = 100;
        } else if (key === "spec") {
          value = agg.spec == null ? "–" : num(agg.spec, 0);
          sub = agg.spec == null
            ? (agg.specOn === false
              ? "speculation is off"
              : agg.statsKnown ? "no draft tokens yet" : "n/a on this host")
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

/* ---------- the two charts ---------- */

/** dashSimpleCharts are the two request rates over the last five minutes. */
function dashSimpleCharts(section) {
  const grid = h("div.dash-simple-chart-grid");
  const output = areaChart([], { title: "Output speed · tok/s, last 5 min", unit: "tok/s", windowMinutes: 5 });
  const prompt = areaChart([], { title: "Prompt reading · tok/s, last 5 min", unit: "tok/s", windowMinutes: 5 });
  grid.append(output, prompt);
  section.append(grid);
  return {
    update: () => {
      output.update(dashWindowSeries("gen", null, "5m"), 5);
      prompt.update(dashWindowSeries("prompt", null, "5m"), 5);
    },
  };
}

/* ---------- this machine and connect an app ---------- */

/** dashSimpleMachine says what machine this is and what it can measure. */
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
      const rows = [
        ["Host", host.hostname || "–", ""],
        ["CPU", host.cpu && host.cpu.model ? host.cpu.model : "–", ""],
        ["RAM", host.ram_bytes ? bytes(host.ram_bytes) : "–", ""],
        ["GPUs", gpus.length ? num(gpus.length, 0) + " cards" : "–", gpus.length ? gpus.map((g) => g.name || g.gfx).filter(Boolean).join(" · ") : "no AMD GPU is visible"],
        ["CPU load", "–", "n/a on this host"],
        ["Requests served", "–", "n/a on this host"],
      ];
      put(facts, rows.map(([label, value, why]) => h("div",
        h("dt", label), h("dd", value, why ? h("span.why", why) : null))));
    },
  };
}

/** dashBuildSimple builds the operational reading order of the Dashboard
 * tab: hero, six tiles, two charts, GPU cards, machine and connect blocks. */
function dashBuildSimple(root) {
  const hero = dashSimpleHero();
  const tiles = dashSimpleTiles();
  const minutes = dashSection("The request rates", "simple-minutes");
  const gpus = dashSection("GPUs", "gpus");
  gpus.id = "dashboard-gpus";
  const bottom = h("div.dash-simple-bottom");
  const details = dashDetails();
  put(root, hero.el, tiles, minutes, gpus, bottom, details.el);
  const api = {
    hero: hero,
    tiles: tiles,
    charts: dashSimpleCharts(minutes),
    cards: dashCards(gpus),
    machine: dashSimpleMachine(bottom),
    connect: dashSimpleConnect(bottom),
    details: details,
  };
  dash.live.simple = api;
  return api;
}

/** dashSimpleConnect gives an app the address and a copyable request. */
function dashSimpleConnect(section) {
  const el = h("div.card.dash-simple-panel");
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
  const paint = () => {
    const run = dashLiveRun();
    const endpoint = connectEndpoint(run && run.state !== "exited" ? run : null);
    const key = kind + "|" + endpoint.base + "|" + endpoint.model;
    if (key === painted) return;
    painted = key;
    if (!endpoint.base) {
      put(text, "Start a model server to get an address and a ready-to-paste request. Already running one? Use ", h("code", "bernie attach <port>"), ".");
      put(command, emptyState("No run is serving.", "A run is a model server on this host.", "bernie serve <recipe>"));
      return;
    }
    put(text, "Any OpenAI-compatible app works. Give it the base URL ", h("code", endpoint.base + "/v1"),
      " and the model ", h("b", endpoint.model), ". No API key needed.");
    const snippet = connectSnippet(kind, endpoint);
    put(command, cmdBlock(snippet, "Copy the " + kind + " snippet"));
  };
  el.append(cardHead("Connect an app"), segmented, text, command);
  section.append(el);
  return { update: paint };
}
