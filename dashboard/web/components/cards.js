// Shared dashboard cards, the state headline and the states a card can be in.
// Every card that shows data links to the document behind it.
"use strict";

/** jsonLink opens the raw JSON of an endpoint in a new tab. */
function jsonLink(path) {
  return h("a.json-link.advanced-only", {
    href: path,
    target: "_blank",
    rel: "noopener noreferrer",
    title: "Open the raw JSON: " + path,
  }, "json");
}

/** cardHead is the header of a card: the title, the raw JSON behind it and
 *  whatever else the card shows next to the title. */
function cardHead(title, path, ...rest) {
  return h("h2", title, h("span.grow"), ...rest, path ? jsonLink(path) : null);
}

/** stateHead is the block that opens a tab: the name of the tab, the headline
 *  that says what the situation is and the line under it. The headline is read
 *  out when it changes. */
function stateHead(eyebrow, action) {
  return h("section.state-head",
    h("p.eyebrow", eyebrow),
    h("h1.display", { "aria-live": "polite" }, "…"),
    h("p.lede", ""),
    action ? h("div.head-actions", action) : null);
}

/** setHead writes the headline of a tab: the situation in one sentence, and the
 *  reason under it. */
function setHead(head, title, lede) {
  if (!head) return;
  const t = head.querySelector(".display");
  const l = head.querySelector(".lede");
  if (t) t.textContent = title;
  if (!l) return;
  l.textContent = lede || "";
  l.hidden = !lede;
}

/** factRow is one fact: the name of it, the value and the reason under it. */
function factRow(label, value, why) {
  return h("div",
    h("dt", label),
    h("dd", value == null || value === "" ? "–" : String(value),
      why ? h("span.why", why) : null));
}

/** tableHead is the head of a table: the names of the columns with the scope
 *  a screen reader needs to read the row. A column is a string or
 *  `{label, num}`. */
function tableHead(cols) {
  return h("thead", h("tr", (cols || []).map((c) => {
    const label = typeof c === "string" ? c : c.label;
    const num = typeof c === "object" && c.num;
    return h("th", { scope: "col", class: num ? "num" : null }, label);
  })));
}

/** tableCaption is the caption of a table: what the table holds. It is read
 *  out and kept out of the way. */
function tableCaption(text) {
  return h("caption", { class: "sr-only" }, text);
}

/** emptyState teaches: what the thing is, and the command that makes one. */
function emptyState(what, next, command) {
  return h("div.empty",
    h("b", what),
    next ? h("span.next", next) : null,
    command ? cmdBlock(command, "Copy the command") : null);
}

/** sectionLabel preserves the author's wording without forced uppercase. */
function sectionLabel(text, ...rest) {
  return h("h2.section-label", { tabindex: "-1" }, String(text || ""), ...rest);
}

/** PILL_ICON is the mark of a state: the word carries the meaning, the mark
 *  only helps. One map for the whole page. */
const PILL_ICON = {
  ok: "✓", done: "✓", warn: "!", serious: "▲", fail: "✕", blocked: "!",
  skip: "–", skipped: "–", todo: "", running: "◐", info: "i",
  idle: "–", starting: "◐", serving: "●", stopped: "✕", "not answering": "!",
};

/** statePill is a state: a mark and a word, never the colour alone. */
function statePill(kind, word) {
  return h("span.pill." + kind,
    h("span.pill-icon", { "aria-hidden": "true" }, PILL_ICON[kind] || "·"),
    h("span.pill-word", word || kind));
}

/** loading says that the answer is on its way: three grey lines and a
 *  sentence. Never a spinner alone. */
function loading(what) {
  return h("div",
    h("div.skeleton", h("i"), h("i"), h("i")),
    h("p.loading-note", what || "Reading the host…"));
}

/** failed says what went wrong: the message in words, the endpoint that
 *  failed, a way to try again and the raw answer. */
function failed(e) {
  const msg = (e && e.message) || String(e || "The request failed.");
  const path = e && e.path;
  const raw = e && e.raw;
  return h("div.err-block",
    h("p.err", msg),
    path ? h("p.small.muted", "The endpoint that failed: ", h("code", path)) : null,
    h("div.row", h("button.small", { type: "button", onclick: () => route() }, "Try again")),
    raw ? h("details", h("summary.small.muted", "The raw answer"), h("pre.small", raw)) : null);
}
