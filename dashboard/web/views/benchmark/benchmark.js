"use strict";
// The Benchmark tab: the state headline, the suites, the result, the
// comparison, the history and the A/B run. See docs/design/benchmark.md.

/** benchState is what the tab knows between two calls. */
const benchState = {
  suites: [],
  results: [],
  suite: "",
  result: null,
  compare: null,
  stop: null,
};

/** benchSection is one section of the tab: its label and what it holds. */
function benchSection(label, ...body) {
  return h("section.section", sectionLabel(label), ...body);
}

/** benchFailed says what could not be read and offers to read it again. */
function benchFailed(what, e, retry) {
  const btn = h("button", { type: "button" }, "Try again");
  if (retry) btn.addEventListener("click", retry);
  return h("div.err", h("b", what), h("p.small", String((e && e.message) || e || "")), btn);
}

/** benchResults reads the saved results, the newest first. */
async function benchResults() {
  const d = await api("/api/bench");
  return (Array.isArray(d) ? d : (d && d.results) || []).slice()
    .sort((a, b) => new Date(b.time) - new Date(a.time));
}

/** benchSuiteLine names the suite of a result and its digest. */
function benchSuiteLine(r) {
  if (!r || !r.suite) return null;
  const digest = String(r.suite.digest || "");
  return "Suite " + r.suite.id + " · digest " + (digest ? digest.slice(0, 12) : "unknown") +
    " · version " + (r.suite.version || 1);
}

/** benchHeadline writes the state headline of the tab: the peak of the
 *  last benchmark and the cell it came from, from the summary of the
 *  result, so the number can be found in the table below. */
function benchHeadline(head, results) {
  const last = results && results[0];
  if (!last) {
    setHead(head, "Nothing has been measured yet.",
      "A benchmark measures the tokens per second of a running server. Run one below and this page fills with its numbers.");
    return;
  }
  const s = last.summary || {};
  const title = s.peak
    ? "The last benchmark peaked at " + benchNum(s.peak_output_tok_s, 1) + " tok/s."
    : "The last benchmark has no output rate.";
  const lede = "The " + benchSuiteOf(last) + " suite ran " + (s.cells - s.skipped || 0) + " of " + (s.cells || 0) +
    " cells" + (s.peak ? "; the peak is " + benchRefName(s.peak) : "") + ". " +
    (s.completed || 0) + " of " + (s.requests || 0) + " requests answered.";
  setHead(head, title, lede);
}

/** benchCopy puts a text on the clipboard. The shell owns the control; the
 *  view only asks for it. */
async function benchCopy(text) {
  if (typeof copy === "function") return await copy(text);
  if (typeof copyText === "function") return await copyText(text);
  return false;
}

/** benchReceipt reads the markdown receipt of a comparison. */
async function benchReceipt(a, b) {
  return await apiText("/api/bench/compare?a=" + encodeURIComponent(a) + "&b=" + encodeURIComponent(b) + "&format=markdown");
}

/** benchStartError says why a run could not start: the answer of the server
 *  when it gave one, the plain sentence when it did not. */
function benchStartError(what, e) {
  const reason = e && e.status ? String(e.message || "") : "";
  return reason ? what + ": " + reason : what;
}

/** cancelBench stops a running suite (R30). The run ends as failed and is
 *  marked incomplete: the partial result, when one was saved, stays in
 *  the history. */
async function cancelBench(id, runBtn, progress) {
  try {
    await api("/api/bench/" + encodeURIComponent(id) + "/cancel", { method: "POST", body: {} });
  } catch (e) {
    progress.fail("The run could not be stopped: " + (e.message || e));
    if (runBtn) runBtn.disabled = false;
    return;
  }
  if (progress) progress.fail("The run was stopped.");
  if (runBtn) runBtn.disabled = false;
}

/** deleteBenchResult removes a saved result after the reader confirms it
 *  (R30). A saved result is the evidence of a measurement, so the page
 *  asks before it removes anything. */
async function deleteBenchResult(r, btn, reload) {
  if (!confirm("Delete the saved result " + r.id + "?")) return;
  btn.disabled = true;
  try {
    await api("/api/bench/" + encodeURIComponent(r.id), { method: "DELETE", body: { confirm: true } });
  } catch (e) {
    btn.disabled = false;
    toast("The result was not deleted: " + (e.message || e));
    return;
  }
  await reload();
}

async function viewBenchmark(root) {
  const runBtn = h("button.primary", { type: "button" }, "Run the suite");
  const stopBtn = h("button", { type: "button", hidden: true }, "Stop");
  stopBtn.addEventListener("click", () => {
    if (benchState.stop) benchState.stop();
  });
  const head = stateHead("Benchmark", h("div.head-actions", stopBtn, runBtn));

  const suitesCard = h("div.card.bench-card");
  const progress = benchProgress();
  const resultCard = h("div.card.bench-card");
  const compareCard = h("div.card.bench-card");
  const historyCard = h("div.card.bench-card");
  const advanced = benchAdvanced(benchDocs);

  const base = h("select", { id: "bench-cmp-base" });
  const cand = h("select", { id: "bench-cmp-cand" });
  const cmpBtn = h("button", { type: "button" }, "Compare");
  const cmpOut = h("div.bench-compare-out");
  const receiptBtn = h("button", { type: "button" }, "Copy the receipt");
  const abOut = h("div.bench-ab-out");

  put(root, head,
    benchSection("The suites", suitesCard, progress.el),
    benchSection("The result", resultCard),
    benchSection("The comparison", h("div.card.bench-card",
      h("div.bench-pickers",
        h("label", { for: "bench-cmp-base" }, "Baseline"), base,
        h("label", { for: "bench-cmp-cand" }, "Candidate"), cand,
        cmpBtn, receiptBtn),
      cmpOut)),
    benchSection("The history", historyCard),
    benchSection("An A/B run", benchAB((b, c, out, btn, prog) => runAB(b, c, out, btn, prog))),
    advanced.el);

  /** benchDocs reads the raw documents of the endpoints of the tab: the
   *  saved results, the suites and the comparison of the two picked
   *  results. */
  async function benchDocs() {
    const paths = ["/api/bench", "/api/bench/suites"];
    const a = base.value, b = cand.value;
    const suiteOf = (id) => {
      const r = (benchState.results || []).find((x) => x.id === id);
      return r ? benchSuiteOf(r) : "";
    };
    // Two results of different suites cannot be compared: the endpoint
    // would answer 400. The panel asks only for a comparison that holds.
    if (a && b && a !== b && suiteOf(a) && suiteOf(a) === suiteOf(b)) {
      paths.push("/api/bench/compare?a=" + encodeURIComponent(a) + "&b=" + encodeURIComponent(b));
    }
    const docs = [];
    for (const p of paths) {
      let text = "";
      try {
        text = await apiText(p);
      } catch (e) {
        text = String(e.message || e);
      }
      docs.push({ path: p, text: text });
    }
    return docs;
  }

  /** loadSuites reads the built-in suites. */
  async function loadSuites() {
    try {
      const list = await api("/api/bench/suites");
      benchState.suites = list || [];
      put(suitesCard, benchSuiteCards(benchState.suites, benchState.suite, pickSuite));
    } catch (e) {
      put(suitesCard, benchFailed("The suites could not be read.", e, loadSuites));
    }
  }

  /** pickSuite takes the suite of the next run. */
  function pickSuite(id) {
    benchState.suite = id;
    runBtn.disabled = false;
    for (const card of suitesCard.querySelectorAll(".bench-suite")) {
      const radio = card.querySelector("input");
      if (radio) {
        radio.checked = radio.value === id;
        card.classList.toggle("picked", radio.checked);
      }
    }
  }

  /** fillPickers fills the two pickers of the comparison. */
  function fillPickers(list) {
    const keep = [base.value, cand.value];
    put(base, list.map((r) => h("option", { value: r.id }, r.id + " · " + benchSuiteOf(r))));
    put(cand, list.map((r) => h("option", { value: r.id }, r.id + " · " + benchSuiteOf(r))));
    if (list.length > 1) {
      base.value = keep[0] && list.some((r) => r.id === keep[0]) ? keep[0] : list[1].id;
      cand.value = keep[1] && list.some((r) => r.id === keep[1]) ? keep[1] : list[0].id;
    }
    const off = list.length < 2;
    cmpBtn.disabled = off;
    receiptBtn.disabled = off;
  }

  /** loadResults reads the saved results and draws the tab. */
  async function loadResults(pickId) {
    try {
      const list = await benchResults();
      benchState.results = list;
      benchHeadline(head, list);
      const pick = pickId ? list.find((r) => r.id === pickId) : (list[0] || null);
      benchState.result = pick || null;
      put(resultCard, benchReport(pick));
      put(historyCard, benchHistory(list, null, (r) => {
        cand.value = r.id;
        compare();
      }, (r, btn) => deleteBenchResult(r, btn, loadResults)));
      fillPickers(list);
    } catch (e) {
      put(resultCard, benchFailed("The results could not be read.", e, () => loadResults()));
      setHead(head, "The results could not be read.", "Try again: the endpoint may be busy.");
    }
  }

  /** compare draws the comparison of the two picked results. */
  async function compare() {
    const a = base.value, b = cand.value;
    if (!a || !b || a === b) {
      put(cmpOut, emptyState("Pick two results to compare.",
        "Two saved results side by side, with a verdict on every cell.", "bernie bench list"));
      return;
    }
    cmpBtn.disabled = true;
    put(cmpOut, loading());
    try {
      const cmp = await api("/api/bench/compare?a=" + encodeURIComponent(a) + "&b=" + encodeURIComponent(b));
      benchState.compare = cmp;
      if (cmp.valid === false) {
        // The two runs measured the same thing, but the comparison could
        // not decide: the reasons say why.
        put(cmpOut, h("div",
          h("p.small.muted", (cmp.reasons || []).join(" · ") || "The comparison could not decide."),
          compareChart(cmp)));
      } else {
        put(cmpOut, compareChart(cmp));
      }
    } catch (e) {
      put(cmpOut, benchFailed("The two results measured different things.", e, compare));
    }
    cmpBtn.disabled = false;
  }

  /** runSuite starts the run of the picked suite. */
  async function runSuite() {
    if (!benchState.suite) return;
    runBtn.disabled = true;
    progress.show({ done: 0, total: 0 });
    let job;
    try {
      job = await api("/api/bench/run", { method: "POST", body: { suite: benchState.suite } });
    } catch (e) {
      progress.fail(benchStartError("The run could not be started.", e));
      runBtn.disabled = false;
      return;
    }
    benchState.runJobID = job.id;
    benchState.stop = () => cancelBench(job.id, runBtn, progress);
    stopBtn.hidden = false;
    watchBenchJob(job.id, {
      progress: (j) => progress.show(j),
      log: (line) => progress.log(line),
      done: async (j) => {
        progress.hide();
        runBtn.disabled = false;
        stopBtn.hidden = true;
        benchState.stop = null;
        await loadResults(j && j.result_id);
      },
      failed: (e) => {
        progress.fail(String(e.message || e));
        runBtn.disabled = false;
        stopBtn.hidden = true;
        benchState.stop = null;
      },
    });
  }

  /** runAB starts the A/B run of the two arms. */
  async function runAB(baseArm, candArm, out, btn, prog) {
    btn.disabled = true;
    put(out, h("p.small.muted", "The A/B run starts. It visits the base, then the candidate, then the candidate again, then the base."));
    let job;
    try {
      job = await api("/api/bench/ab", { method: "POST", body: { base: baseArm, cand: candArm } });
    } catch (e) {
      put(out, benchFailed(benchStartError("The A/B run could not be started.", e), null, null));
      btn.disabled = false;
      return;
    }
    if (prog) prog.show({ done: 0, total: 0 });
    watchBenchJob(job.id, {
      progress: (j) => { if (prog) prog.show(j); },
      log: (line) => { if (prog) prog.log(line); },
      done: async () => {
        btn.disabled = false;
        if (prog) prog.hide();
        await loadResults();
        put(out, h("p.small.muted", "The A/B run ended. Pick its two results in the comparison above."));
      },
      failed: (e) => {
        btn.disabled = false;
        if (prog) prog.fail(String(e.message || e));
        put(out, benchFailed("The A/B run stopped.", e, null));
      },
    });
  }

  runBtn.addEventListener("click", runSuite);
  cmpBtn.addEventListener("click", compare);
  receiptBtn.addEventListener("click", async () => {
    const a = base.value, b = cand.value;
    if (!a || !b || a === b) return;
    receiptBtn.disabled = true;
    try {
      const text = await benchReceipt(a, b);
      const ok = await benchCopy(text);
      toast(ok ? "Copied the receipt." : "Copy failed: select the text and press Ctrl-C.", ok);
    } catch (e) {
      toast("The receipt could not be read.", false);
    }
    receiptBtn.disabled = false;
  });

  await loadSuites();
  await loadResults();
  if (!benchState.suite && benchState.suites.length) pickSuite(benchState.suites[0].id);
}
