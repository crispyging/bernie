// Logs: status, recovery steps, then the searchable stream.
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

  const rescan = h("button.primary", { type: "button", onclick: () => refresh() }, "Refresh logs");
  const download = h("button", { type: "button", disabled: true, onclick: () => {
    if (!tail.save()) toast("There is no log to download yet.");
  } }, "Download the log");
  const head = stateHead("Logs", h("div.row", rescan, download));
  const runsBox = h("div.lg-runs-box", loading());
  const findBox = h("div.lg-findings", loading());
  const advanced = logsAdvanced(() => logsRun);

  put(root, head,
    logSection("Selected run", runsBox),
    logSection("Problems and recovery steps", findBox),
    logSection("Live log", tail.el),
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
    if (known.length) {
      headKind = "failed";
      setHead(head, "Log errors need attention.",
        "Process state: " + (run.state || "unknown") + ". " +
        (known[0].cause || "A known error pattern was found.") + " Review the recovery steps below.");
      return;
    }
    if (run.state === "running") {
      headKind = "running";
      setHead(head, "Following " + run.name + ".",
        "The tail follows the log of " + run.name + ". " + logCount(tail.count()) + " lines so far.");
      return;
    }
    if (run.state === "starting") {
      headKind = "starting";
      setHead(head, "The server is starting.",
        (run.phase && run.phase.sentence) || "The server is loading the model. The tail follows it.");
      return;
    }
    headKind = "ended";
    setHead(head, "The run has ended.",
      "The log remains available. Review any problems above before restarting in Setup.");
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
      setHead(head, "Runs could not be read.", "Press Refresh logs to try again. Their status is unknown.");
      return;
    }
    runs = logRuns(doc);
    if (!runs.length) {
      put(runsBox, emptyState("No run has ever served on this host.",
        "A run is a model server on this host: it loads the model and then answers requests.",
        "bernie serve <recipe>"));
      put(findBox, emptyState("No log to inspect yet.", "Start a model server in Setup.", "bernie serve <recipe>"));
      logsRun = "";
      tail.close();
      download.disabled = true;
      headKind = "none";
      setHead(head, "No log yet.", "Start the server first, then follow its log here.");
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
