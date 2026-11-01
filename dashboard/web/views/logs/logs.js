// ---------- Logs: what did the server say? ----------
// The tab answers one question: what did the server say? The order of the
// page is the state headline, the run picker, the known problems of the log
// doctor, the tail and the advanced panel (design/logs.md).
"use strict";

let logsTail = null;  // the tail of the picked run
let logsRun = "";     // the name of the picked run

/** stopLogs closes the stream of the log tab. The router calls it when the
 *  reader leaves the tab. */
function stopLogs() {
  if (logsTail) { logsTail.close(); logsTail = null; }
}

/** logSection is one section of the tab: a rule, a label and the content. */
function logSection(title, ...kids) {
  return h("section.section", sectionLabel(title), ...kids);
}

/** logRuns is the list of the runs of the API answer, the newest first. */
function logRuns(data) {
  const list = Array.isArray(data) ? data : (data && Array.isArray(data.runs) ? data.runs : []);
  return list.slice().sort((a, b) => new Date(b.started_at || 0) - new Date(a.started_at || 0));
}

/** viewLogs draws the Logs tab. */
async function viewLogs(root) {
  stopLogs();
  const tail = logTail({ onCount: (n) => setLede(n) });
  logsTail = tail;
  let picked = null;
  let runs = [];
  let headKind = "";   // the headline that owns the line under it

  const rescan = h("button.primary", { type: "button", onclick: () => refresh() }, "Re-scan");
  const download = h("button", { type: "button", disabled: true, onclick: () => {
    if (!tail.save()) toast("There is no log to download yet.");
  } }, "Download the log");
  const head = stateHead("Logs", h("div.row", rescan, download));
  const runsBox = h("div.lg-runs-box", loading());
  const findBox = h("div.lg-findings", loading());
  const advanced = logsAdvanced(() => logsRun);

  put(root, head,
    logSection("THE RUN", runsBox),
    logSection("THE KNOWN PROBLEMS", findBox),
    logSection("THE LOG", tail.el),
    advanced);

  /** setLede writes the line under the headline. The count of the lines
   *  changes with every line, the headline does not. */
  function setLede(n) {
    download.disabled = n === 0;
    if (headKind !== "running" || !picked || picked.state !== "running") return;
    const lede = head.querySelector(".lede");
    if (lede) lede.textContent = "The tail follows the log of " + picked.name + ". " + logCount(n) + " lines so far.";
  }

  /** setHeadline says in one sentence what the log of the picked run is. A
   *  serving run keeps its headline: the known problems sit above the tail
   *  and speak for themselves. */
  function setHeadline(run, matches) {
    const known = (matches || []).filter((m) => m.severity === "error");
    if (run.state === "running") {
      headKind = "running";
      setHead(head, "THE SERVER IS RUNNING.",
        "The tail follows the log of " + run.name + ". " + logCount(tail.count()) + " lines so far.");
      return;
    }
    if (run.state === "starting") {
      headKind = "starting";
      setHead(head, "THE SERVER IS STARTING.",
        [run.phase && run.phase.sentence, run.waiting].filter(Boolean).join(". ") ||
        "The server is loading the model. The tail follows it.");
      return;
    }
    if (known.length) {
      headKind = "failed";
      setHead(head, "THE SERVER FAILED FOR A KNOWN REASON.",
        "The log matched the rule " + known[0].id + ". The fix is above.");
      return;
    }
    const age = run.started_at ? ago(run.started_at) : "";
    headKind = "ended";
    setHead(head, "THE RUN ENDED" + (age ? " " + age.toUpperCase() : "."),
      "The log holds the last words of the server. The known problems are above.");
  }

  /** loadFindings reads the findings of the log doctor for the picked run. */
  async function loadFindings(run) {
    let matches = [];
    try {
      const doc = await api("/api/runs/" + encodeURIComponent(run.name) + "/findings");
      matches = doc && Array.isArray(doc.matches) ? doc.matches : [];
    } catch (e) {
      matches = run.findings || [];
    }
    tail.setFindings(matches);
    put(findBox, logFindings(matches, {
      run: run.name,
      onJump: (f) => { if (!tail.jumpToFinding(f)) toast("That line is not in the tail."); },
    }));
    setHeadline(run, matches);
  }

  /** loadLog reads the tail of the log of the run with the number of every
   *  line and the level of the log doctor. The stream carries the same lines;
   *  the seed gives every line its number. */
  async function loadLog(run) {
    try {
      const doc = await api("/api/runs/" + encodeURIComponent(run.name) + "/log?tail=2000");
      if (doc && Array.isArray(doc.lines)) tail.seed(doc);
    } catch (e) {
      // the numbers of the lines are a comfort; the tail works without them
    }
  }

  /** select follows the log of one run. */
  async function select(run) {
    picked = run;
    logsRun = run.name;
    put(runsBox, logRunPicker(runs, run.name, select));
    tail.open(run.name, run.findings || []);
    download.disabled = !tail.hasLines();
    await loadFindings(run);
    await loadLog(run);
  }

  /** refresh reads the runs again and follows the picked one. */
  async function refresh() {
    let doc = null;
    try {
      doc = await api("/api/runs");
    } catch (e) {
      put(runsBox, failed(e));
      put(findBox, emptyState("The findings could not be read.", "Try again.", "bernie doctor --run NAME"));
      headKind = "error";
      setHead(head, "THE RUNS COULD NOT BE READ.", "Try again.");
      return;
    }
    runs = logRuns(doc);
    if (!runs.length) {
      put(runsBox, emptyState("No run has ever served on this host.",
        "A run is a model server on this host: it loads the model and then answers requests.",
        "bernie serve <recipe>"));
      put(findBox, emptyState("No known failure in this log.",
        "bernie knows the failures of this server; none of them is in this log.",
        "bernie doctor --run NAME"));
      logsRun = "";
      tail.close();
      download.disabled = true;
      headKind = "none";
      setHead(head, "NO LOG YET.", "Start the server first, then follow its log here.");
      return;
    }
    const keep = runs.find((r) => r.name === logsRun);
    await select(keep || runs[0]);
  }

  await refresh();
}

/** followLogs opens the tail of a run in a card. The run table of the
 *  Dashboard tab calls it; the Logs tab opens its own tail. */
function followLogs(card, name, findings) {
  const tail = logTail();
  tail.setFindings(findings || []);
  put(card, tail.el);
  tail.open(name, findings || []);
  return tail;
}

/** stopRun stops a run and asks first. */
async function stopRun(ev, name, root) {
  if (!confirm("Stop run " + name + "?")) return;
  if (ev && ev.target) ev.target.disabled = true;
  try {
    await api("/api/runs/" + encodeURIComponent(name), { method: "DELETE" });
    toast("Stopped " + name + ".", true);
  } catch (e) {
    toast("Stop failed: " + e.message);
  }
  if (root) viewLogs(root);
}
