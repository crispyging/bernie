// The known problems of the log: the findings of the log doctor
// (design/logs.md). The findings sit above the tail: a reader who opens the
// tab sees what is wrong before the noise.
// The shape of a finding card follows vllm-rdna-dash by CrispyGinger
// (Apache-2.0).
"use strict";

/** logPill is a state as an icon and a word. The colour never carries the
 *  meaning alone. */
function logPill(kind, word) {
  const icons = { fail: "✕", warn: "!", ok: "✓", info: "i", skip: "–", running: "●" };
  return h("span.pill.lg-pill", { class: "lg-pill-" + kind },
    h("span.lg-pill-icon", icons[kind] || "•"),
    h("span", word));
}

/** logSeverityPill is the pill of a finding of the log doctor. */
function logSeverityPill(finding) {
  const error = finding.severity === "error";
  return logPill(error ? "fail" : "warn", error ? "fail" : "warn");
}

/** logFixIsCommand says whether the fix of a finding is a command to run in
 *  a shell. A fix that is a command gets the block of a command and its copy
 *  button; a fix that is a sentence stays a sentence with a copy button. */
function logFixIsCommand(fix) {
  return /^(bernie\s|export\s|Export\s|\$\s|[A-Z][A-Z0-9_]*=)/.test(fix);
}

/** logFinding is one card of the known problems: what the log said, what it
 *  means, the command that fixes it and the rule behind it. */
function logFinding(finding, options) {
  const o = options || {};
  const fix = finding.fix || "";
  const fail = finding.severity === "error";
  return h("div.finding.lg-finding", { class: fail ? "lg-sev-fail" : "lg-sev-warn" },
    h("div.lg-finding-head",
      logSeverityPill(finding),
      h("span.lg-rule", finding.id || "rule"),
      finding.line_no != null ? h("span.lg-line-no", "line " + finding.line_no) : null),
    h("p.lg-cause", finding.cause || ""),
    fix ? (logFixIsCommand(fix) ? cmdBlock(fix) : h("p.lg-fix", fix, " ", copyBtn(fix, "Copy the fix"))) : null,
    h("div.lg-finding-foot",
      finding.doc ? h("a.lg-doc", { href: finding.doc, target: "_blank", rel: "noopener noreferrer" }, "Read the rule →") : null,
      o.onJump ? h("button", { type: "button", onclick: () => o.onJump(finding) }, "Go to the line") : null));
}

/** logFindings is the list of the known problems of a log. */
function logFindings(matches, options) {
  const o = options || {};
  const list = matches || [];
  if (!list.length) {
    return emptyState("No known failure in this log.",
      "bernie knows the failures of this server; none of them is in this log.",
      "bernie doctor --run " + (o.run || "NAME"));
  }
  return h("div.findings", list.map((f) => logFinding(f, o)));
}
