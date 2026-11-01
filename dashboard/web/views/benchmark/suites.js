"use strict";
// The Benchmark tab: the suite cards, the live progress, the history, the
// A/B run and the Advanced panel. See docs/design/benchmark.md.

/** benchSuiteCells counts the cells of a suite. */
function benchSuiteCells(s) {
  let n = 0;
  for (const g of (s && s.groups) || []) n += (g.concurrency || []).length;
  return n;
}

/** benchSuiteEstS is the rough wall time of a suite in seconds. */
function benchSuiteEstS(s) {
  let n = 0;
  for (const g of (s && s.groups) || []) n += (g.est_s || 0) * (g.reps || 1);
  return n;
}

/** benchMinutes turns a number of seconds into a short time. */
function benchMinutes(s) {
  if (!s) return "–";
  if (s < 60) return s + " s";
  const m = Math.round(s / 60);
  return m + " min";
}

/** benchSuiteCard is one suite: a radio, the name, what it measures, the
 *  number of cells, the estimated time and the digest in mono. */
function benchSuiteCard(s, picked, onPick) {
  const id = "bench-suite-" + s.id;
  const digest = s.digest || "";
  const radio = h("input.bench-radio", {
    type: "radio", name: "bench-suite", id: id, value: s.id, checked: picked,
  });
  radio.addEventListener("change", () => onPick(s.id));
  return h("label.bench-suite", { for: id, class: picked ? "bench-suite picked" : "bench-suite" },
    radio,
    h("div.bench-suite-body",
      h("p.bench-suite-name", s.name || s.id),
      h("p.bench-suite-desc", s.description || ""),
      h("p.bench-suite-meta",
        benchSuiteCells(s) + (benchSuiteCells(s) === 1 ? " cell · about " : " cells · about ") + benchMinutes(benchSuiteEstS(s))),
      h("p.bench-suite-digest",
        h("code", "digest: " + (digest ? digest.slice(0, 12) : "unknown")),
        copyBtn(digest || "unknown", "Copy the digest"))));
}

/** benchSuiteCards is the section of the suites: one radio card each. */
function benchSuiteCards(list, picked, onPick) {
  if (!list || !list.length) {
    return emptyState("There is no suite to run.",
      "The built-in suites live in bench/suites/. One of them is the set of cells a pull request must pass.",
      "bernie bench suites");
  }
  return h("div.bench-suites", list.map((s) => benchSuiteCard(s, s.id === picked, onPick)));
}

/** benchSentence writes the sentence of the progress card: the phase of the
 *  job, the rate it reached and the time the run has taken. */
function benchSentence(phase, rate, secs) {
  const p = phase ? phase.charAt(0).toUpperCase() + phase.slice(1) : "Starting…";
  return p + " · " + (rate || "–") + " · " + secs + " s so far.";
}

/** benchProgress is the live progress of a run: one sentence and one bar. */
function benchProgress() {
  const sentence = h("p.bench-progress-sentence", { "aria-live": "polite" }, "");
  const bar = h("progress.bench-bar", {
    max: "100", value: "0", "aria-label": "The progress of the run",
  });
  const log = h("p.small.muted.bench-progress-log", "");
  const el = h("div.card.bench-card.bench-progress", { hidden: true }, sentence, bar, log);
  let start = Date.now();
  let job = null;
  let rate = null;
  function render() {
    el.hidden = false;
    if (!start) start = Date.now();
    const done = (job && job.done) || 0;
    const total = (job && job.total) || 0;
    bar.value = total ? Math.min(100, Math.round((done / total) * 100)) : 0;
    const secs = Math.round((Date.now() - start) / 1000);
    const phase = (job && job.phase) || (total ? "Cell " + done + " of " + total : "Starting…");
    sentence.textContent = benchSentence(phase, rate, secs);
  }
  return {
    el: el,
    /** show writes the state of the job: the phase, the numbers and the bar. */
    show(state) {
      job = state || {};
      render();
    },
    /** log writes one line of the run under the sentence. */
    log(text) {
      if (text) {
        const line = String(text);
        log.textContent = line;
        const m = line.match(/([\d.]+)\s*tok\/s/);
        if (m) rate = m[1] + " tok/s";
      }
      render();
    },
    /** fail says that the run stopped and shows the last line. */
    fail(text) {
      el.hidden = false;
      sentence.textContent = "The run stopped.";
      log.textContent = String(text || "");
    },
    /** hide takes the card away. */
    hide() {
      el.hidden = true;
      start = 0;
      job = null;
      rate = null;
    },
  };
}

/** benchBest returns the largest value a field of the cells of a result has. */
function benchBest(cells, get) {
  let best = 0;
  for (const c of cells || []) {
    const v = get(c);
    if (v != null && v > best) best = v;
  }
  return best > 0 ? best : null;
}

/** benchSuiteOf names the suite of a saved result. */
function benchSuiteOf(r) {
  if (r && r.suite && r.suite.id) return r.suite.id;
  return (r && r.tag) || "–";
}

/** benchHistoryRow is one saved result of the history table. */
function benchHistoryRow(r, verdict, onPick, onDelete) {
  const cells = r.cells || [];
  const del = h("button.small", { type: "button" }, "Delete");
  del.addEventListener("click", (ev) => {
    ev.stopPropagation();
    if (onDelete) onDelete(r, del);
  });
  const tr = h("tr",
    h("td.small", ago(r.time)),
    h("td", benchSuiteOf(r)),
    h("td.small", r.model || "–"),
    h("td.num", benchNum(benchBest(cells, (c) => c.output_tok_s), 1)),
    h("td.num", benchNum(benchBest(cells, (c) => (c.prefill || {}).agg_tok_s), 1)),
    h("td.num", benchNum(benchBest(cells, (c) => (c.decode || {}).agg_tok_s), 1)),
    h("td.num", cells.length),
    h("td", verdict ? verdictChip(verdict) : h("span.muted", "–")),
    h("td", onDelete ? del : null));
  if (onPick) {
    tr.classList.add("click");
    tr.addEventListener("click", () => onPick(r));
  }
  return tr;
}

/** benchHistory is the table of the saved results. */
function benchHistory(list, verdicts, onPick, onDelete) {
  if (!list || !list.length) {
    return emptyState("No saved result yet.",
      "Every benchmark is saved with its workload, so two results can be compared only when they measured the same thing.",
      "bernie bench --suite smoke");
  }
  return h("div.scroll", h("table.bench-table",
    h("thead", h("tr",
      h("th", "WHEN"), h("th", "SUITE"), h("th", "MODEL"),
      h("th.num", "OUT TOK/S"), h("th.num", "PREFILL"), h("th.num", "DECODE"),
      h("th.num", "CELLS"), h("th", "VERDICT"), h("th", ""))),
    h("tbody", list.map((r) => benchHistoryRow(r, verdicts && verdicts[r.id], onPick, onDelete)))));
}

/** benchAB is the A/B run: the two arms and the button that starts it. */
function benchAB(onRun) {
  const base = h("input", { type: "text", id: "bench-ab-base", placeholder: "flashnext" });
  const cand = h("input", { type: "text", id: "bench-ab-cand", placeholder: "http://127.0.0.1:8001" });
  const btn = h("button", { type: "button" }, "Run the A/B");
  const out = h("div.bench-ab-out");
  const form = h("form.bench-ab",
    h("label", { for: "bench-ab-base" }, "Base"), base,
    h("label", { for: "bench-ab-cand" }, "Cand"), cand,
    btn);
  form.addEventListener("submit", (e) => {
    e.preventDefault();
    if (!base.value.trim() || !cand.value.trim()) {
      put(out, h("p.small.muted", "An A/B run needs both arms: a recipe id or the URL of a server."));
      return;
    }
    onRun(base.value.trim(), cand.value.trim(), out, btn, prog);
  });
  btn.addEventListener("click", () => form.requestSubmit());
  const prog = benchProgress();
  return h("div.card.bench-card",
    h("p.small.muted", "An A/B run measures one server, then the other, then the first again."),
    form, prog.el, out);
}

/** benchAdvanced is the panel with the raw JSON of the endpoints of the tab
 *  and the commands that produce the same documents. The documents are read
 *  when the panel opens. The panel is a details element, so the button of
 *  the shell opens it with every other panel of the page. */
function benchAdvanced(load) {
  const body = h("div.advanced-body.bench-advanced-body");
  let filled = false;
  async function fill() {
    if (filled) return;
    filled = true;
    let docs = [];
    try {
      docs = await load();
    } catch (e) {
      docs = [{ path: "/api/bench", text: String(e.message || e) }];
    }
    for (const d of docs || []) {
      body.appendChild(h("div.bench-advanced-doc",
        h("p.bench-advanced-head",
          h("a.json-link", { href: d.path, target: "_blank", rel: "noopener noreferrer" }, "json"),
          h("code", d.path)),
        h("pre.bench-json", d.text || ""),
        copyBtn(d.text || "", "Copy the JSON")));
    }
    body.appendChild(h("div.bench-advanced-cmds",
      h("p.eyebrow", "The same things in the command line"),
      cmdBlock("bernie bench list"),
      cmdBlock("bernie bench suites"),
      cmdBlock("bernie bench compare A B"),
      cmdBlock("bernie bench compare A B --markdown"),
      cmdBlock("bernie bench ab BASE CAND")));
  }
  const det = h("details.advanced.bench-advanced", { id: "bench-advanced" },
    h("summary", "Show details"), body);
  det.addEventListener("toggle", () => {
    if (det.open) fill();
  });
  return {
    el: det,
  };
}
