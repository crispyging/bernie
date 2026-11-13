"use strict";
// The router and the start-up. Loaded last: every view function exists by now.
//
// The page has nine tabs: #setup, #dashboard, #chat, #benchmark, #quality, #topology,
// #fleet, #logs and #advanced. The old links keep working: the router
// rewrites them and replaces the history entry, so the back button never
// walks through them.

const VIEWS = {
  setup: "viewSetup",
  dashboard: "viewDashboard",
  chat: "viewChat",
  benchmark: "viewBenchmark",
  quality: "viewQuality",
  topology: "viewTopology",
  fleet: "viewFleet",
  logs: "viewLogs",
  advanced: "viewAdvanced",
};

// The old routes and where they land now.
const OLD_ROUTES = {
  home: ["setup", ""],
  overview: ["setup", ""],
  recipes: ["setup", "recipes"],
  gpus: ["dashboard", "gpus"],
  runs: ["dashboard", "runs"],
  bench: ["benchmark", "suites"],
  classic: ["advanced", ""],
};

/* ---------- the files of the tabs ---------- */

// The first paint carries the shell and the Setup tab (see index.html). Every
// other tab brings its own files the first time it opens: the page then holds
// the code of the tab the reader is looking at, not the code of the four
// tabs that are closed. The order inside a bundle is the order of
// index.html: a file may use the ones above it.
const TAB_SCRIPTS = {
  setup: [],       // the Setup tab is the first paint: index.html carries it
  dashboard: [
    "components/charts.js",
    "components/gputable.js",
    "views/dashboard/dashboard.js",
    "views/dashboard/details.js",
    "views/dashboard/gpus.js",
    "views/dashboard/attach.js",
    "views/dashboard/simple.js",
  ],
  benchmark: [
    "components/metrics.js",
    "components/compare.js",
    "views/benchmark/report.js",
    "views/benchmark/benchmark.js",
    "views/benchmark/suites.js",
  ],
  quality: [
    "components/metrics.js",
    "components/compare.js",
    "views/benchmark/report.js",
    "views/quality/quality.js",
  ],
  chat: [
    "views/chat/chat.js",
  ],
  topology: [
    "views/topology/topology.js",
    "views/topology/matrix.js",
    "views/topology/links.js",
    "views/topology/checklist.js",
    "views/topology/advanced.js",
  ],
  fleet: [
    "views/fleet/fleet.js",
  ],
  logs: [
    "views/logs/logs.js",
    "views/logs/levels.js",
    "views/logs/picker.js",
    "views/logs/findings.js",
    "views/logs/tail.js",
    "views/logs/advanced.js",
  ],
  advanced: [
    "components/gputable.js",
    "views/advanced/console.js",
    "views/advanced/power.js",
    "views/advanced/doctor.js",
    "views/advanced/jobs.js",
    "views/advanced/documents.js",
    "views/advanced/prefs.js",
    "views/advanced/report.js",
  ],
};

// The stylesheets of the tabs that are not open yet.
const TAB_CSS = {
  setup: [],
  dashboard: ["views/dashboard/dashboard.css", "views/dashboard/simple.css"],
  benchmark: ["css/bench.css"],
  quality: ["css/bench.css"],
  chat: ["views/chat/chat.css"],
  topology: ["views/topology/topology.css"],
  fleet: ["views/fleet/fleet.css"],
  logs: ["views/logs/logs.css"],
  advanced: ["css/advanced.css"],
};

// The tabs whose files are on the page.
const tabReady = new Set(["setup"]);
// The tabs whose files are on their way.
const tabLoading = new Map();

/** loadedScripts are the files loadScript added to the page. */
const loadedScripts = new Set();

/** loadScript adds one file of the page. The files of a bundle run in the
 *  order they are named. */
function loadScript(src) {
  // Two bundles may share a file: it runs once, or its constants clash.
  if (loadedScripts.has(src)) return Promise.resolve();
  loadedScripts.add(src);
  return new Promise((resolve, reject) => {
    const el = document.createElement("script");
    el.src = src;
    el.async = false; // the files of a bundle run in the order they are named
    el.addEventListener("load", () => resolve());
    el.addEventListener("error", () => reject(new Error("cannot load " + src)));
    document.head.append(el);
  });
}

/** loadStyle adds one stylesheet of the page. A sheet that does not arrive
 *  must not stop the tab: the tab paints without it. */
function loadStyle(href) {
  return new Promise((resolve) => {
    const el = document.createElement("link");
    el.rel = "stylesheet";
    el.href = href;
    el.addEventListener("load", () => resolve());
    el.addEventListener("error", () => resolve());
    document.head.append(el);
  });
}

/** openTab brings the files of a tab. It runs once per tab. */
function openTab(tab) {
  if (tabReady.has(tab)) return Promise.resolve();
  if (tabLoading.has(tab)) return tabLoading.get(tab);
  const p = Promise.all((TAB_CSS[tab] || []).map(loadStyle))
    .then(() => Promise.all((TAB_SCRIPTS[tab] || []).map(loadScript)))
    .then(() => { tabReady.add(tab); });
  tabLoading.set(tab, p);
  return p;
}

/** parseHash reads the route: the tab and the section inside it. */
function parseHash() {
  const raw = (location.hash || "").replace(/^#/, "");
  const [name, section = ""] = raw.split("/");
  if (OLD_ROUTES[name]) return { tab: OLD_ROUTES[name][0], section: OLD_ROUTES[name][1], old: name };
  if (VIEWS[name]) return { tab: name, section, old: "" };
  return { tab: "setup", section: "", old: "" };
}

/** route shows the tab of the address bar and paints it. */
function route() {
  const r = parseHash();
  if (r.old) {
    // The old link opens its new home and disappears from the history.
    const next = "#" + r.tab + (r.section ? "/" + r.section : "");
    history.replaceState(null, "", next);
  }
  state.tab = r.tab;

  for (const link of document.querySelectorAll("[data-tab]")) {
    const on = link.dataset.tab === r.tab;
    link.setAttribute("aria-selected", String(on));
    link.setAttribute("tabindex", on ? "0" : "-1");
  }
  for (const panel of document.querySelectorAll("[data-tab-panel]")) {
    panel.hidden = panel.dataset.tabPanel !== r.tab;
  }

  const panel = document.querySelector('[data-tab-panel="' + r.tab + '"]');
  if (!panel) return;

  const paint = () => {
    // The reader moved on while the files of the tab were on their way.
    if (state.tab !== r.tab) return;
    const fn = window[VIEWS[r.tab]];
    if (typeof fn === "function") {
      // The view may paint in one go or in many: the panel is prepared when
      // it is done.
      Promise.resolve(fn(panel)).then(() => preparePanel(panel)).catch(() => preparePanel(panel));
    }
    if (r.section) focusSection(r.tab, r.section);
  };

  if (tabReady.has(r.tab)) {
    paint();
    return;
  }
  // The tab opens for the first time: its files come first.
  put(panel, loading("Loading the tab…"));
  openTab(r.tab).then(paint).catch(() => {
    put(panel, failed({
      message: "The files of the tab did not load.",
      path: (TAB_SCRIPTS[r.tab] || []).join(", "),
    }));
  });
}

/** preparePanel gives the panel a tab stop when it holds nothing focusable
 *  (the APG pattern for a tab panel). */
function preparePanel(panel) {
  if (!panel) return;
  const focusable = panel.querySelector(
    "a[href], button, input, select, textarea, details, [tabindex]:not([tabindex='-1'])");
  if (focusable) panel.removeAttribute("tabindex");
  else panel.setAttribute("tabindex", "0");
}

/** focusSection scrolls to a section of a tab and puts the focus on its
 *  heading. */
function focusSection(tab, section) {
  const panel = document.querySelector('[data-tab-panel="' + tab + '"]');
  if (!panel) return;
  const target = panel.querySelector('[data-section="' + section + '"]') || panel.querySelector("#" + tab + "-" + section);
  if (!target) return;
  target.scrollIntoView({ block: "start" });
  const head = target.querySelector("h2, h3") || target;
  if (head.setAttribute) {
    head.setAttribute("tabindex", "-1");
    head.focus({ preventScroll: true });
  }
}

/** tabsKey moves the focus in the tab bar: Left, Right, Home and End. A
 *  horizontal tab list must not take Up and Down (the APG pattern). */
function tabsKey(ev) {
  const tabs = Array.from(document.querySelectorAll('[role="tab"]'));
  if (!tabs.length) return;
  const i = tabs.indexOf(document.activeElement);
  if (i < 0) return;
  let next = -1;
  if (ev.key === "ArrowRight") next = (i + 1) % tabs.length;
  else if (ev.key === "ArrowLeft") next = (i - 1 + tabs.length) % tabs.length;
  else if (ev.key === "Home") next = 0;
  else if (ev.key === "End") next = tabs.length - 1;
  if (next < 0) return;
  ev.preventDefault();
  const link = tabs[next];
  link.focus();
  if (link.dataset.tab && link.dataset.tab !== state.tab) location.hash = "#" + link.dataset.tab;
}

/** shortcutsKey moves between the tabs with the keyboard: g, then the letter
 *  of the tab. The letters are off until the reader turns them on in the `?`
 *  panel (WCAG 2.1.4). */
let pendingG = false;

function shortcutsKey(ev) {
  const t = ev.target;
  if (t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.isContentEditable)) return;
  if (ev.metaKey || ev.ctrlKey || ev.altKey) return;
  const k = (ev.key || "").toLowerCase();
  if (k === "escape") {
    pendingG = false;
    closeTip();
    return;
  }
  if (!shortcutsOn()) return;
  if (pendingG) {
    pendingG = false;
    const tab = { s: "setup", d: "dashboard", b: "benchmark", q: "quality", t: "topology", l: "logs", c: "advanced" }[k];
    if (tab) {
      ev.preventDefault();
      location.hash = "#" + tab;
    }
    return;
  }
  if (k === "g") {
    pendingG = true;
    toast("Now press s, d, b, t, l or c.");
    setTimeout(() => (pendingG = false), 1500);
  }
}

/* ---------- the start-up ---------- */

function boot() {
  applyTheme(themeChoice());
  applyAdvanced(advancedOn());

  for (const btn of document.querySelectorAll("[data-theme-choice]")) {
    btn.addEventListener("click", () => setTheme(btn.dataset.themeChoice));
  }
  watchSystemTheme();
  for (const btn of document.querySelectorAll("[data-ui-mode]")) {
    btn.addEventListener("click", () => {
      if ((btn.dataset.uiMode === "advanced") !== advancedOn()) toggleAdvanced();
    });
  }
  const liveToggle = $("#liveToggle");
  if (liveToggle) liveToggle.addEventListener("click", () => setLivePaused(!state.livePaused));
  const helpBtn = $("#helpBtn");
  if (helpBtn) {
    helpBtn.addEventListener("click", () => {
      if (tipPanel) { closeTip(); return; }
      openTip(helpBtn, shortcutsText());
    });
  }

  const toastEl = $("#toast");
  if (toastEl) toastEl.addEventListener("click", () => (toastEl.hidden = true));

  const list = $("#tabs");
  if (list) list.addEventListener("keydown", tabsKey);
  document.addEventListener("keydown", shortcutsKey);

  window.addEventListener("hashchange", route);
  window.addEventListener("resize", () => state.tab === "dashboard" && typeof renderGPUs === "function" && renderGPUs());

  startTelemetry();
  startPolling();
  loadHW().then(demoBanner).catch(() => {}); // the rail shows the name of the host
  route();
}

boot();
