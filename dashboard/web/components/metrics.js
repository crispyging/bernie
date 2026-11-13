// The words of a benchmark: how the numbers are measured, the flags of a
// cell and the verdict chip. Plain DOM: no innerHTML, no dependency.
"use strict";

/** METRIC_HELP says in one sentence how the prefill and the decode numbers
 *  of a cell are measured, and what the columns of the latencies hold. */
const METRIC_HELP = "The prefill rate counts the queueing of the cell; the decode rate counts the time in which at least one request was decoding. The latencies are the median and the 95th percentile over the answered requests; the REQUESTS column says how many of the sent requests answered. A cell whose repetitions disagree carries a flag.";

/** VERDICTS are the four verdicts of a comparison: the icon, the word and the
 *  token of the colour. The word always carries the meaning. */
const VERDICTS = {
  better: { icon: "▲", word: "better", tone: "ok" },
  same: { icon: "–", word: "same", tone: "skip" },
  worse: { icon: "▼", word: "worse", tone: "fail" },
  noisy: { icon: "!", word: "noisy", tone: "warn" },
};

/** COMPARE_METRICS are the five groups of a comparison, the name each one
 *  carries on the chart and the unit of its numbers. */
const COMPARE_METRICS = [
  { metric: "prefill.agg_tok_s", label: "PREFILL", unit: "tok/s" },
  { metric: "decode.agg_tok_s", label: "DECODE", unit: "tok/s" },
  { metric: "ttft_s.p50", label: "TTFT", unit: "s" },
  { metric: "itl_ms.p50", label: "ITL", unit: "ms" },
  { metric: "output_tok_s", label: "OUT", unit: "tok/s" },
];

/** benchNum formats a number of a benchmark. The thousands are separated by a
 *  space and a number that was not measured shows as "–". */
function benchNum(v, digits) {
  if (v == null || typeof v !== "number" || !isFinite(v)) return "–";
  const d = digits == null ? 1 : digits;
  const text = v.toFixed(d);
  const cut = text.indexOf(".");
  const whole = cut < 0 ? text : text.slice(0, cut);
  const frac = cut < 0 ? "" : text.slice(cut);
  return whole.replace(/\B(?=(\d{3})+(?!\d))/g, " ") + frac;
}

/** benchDigits is the number of decimals a unit carries. */
function benchDigits(unit) {
  return unit === "s" ? 3 : 1;
}

/** SPREAD_STEADY is how close the repetitions of one cell must agree, from
 *  spec §10.2: the min/max ratio of a metric over the repetitions. Below it,
 *  the repetitions disagree and the number says little. */
const SPREAD_STEADY = 0.95;

/** cellFlags names the flags of a cell in words. A cell without a flag has
 *  no word at all. */
function cellFlags(c) {
  const out = [];
  const add = (text, tone) => out.push({ text: text, tone: tone });
  if (c.recall && c.recall.asked > 0 && c.recall.passed < c.recall.asked) {
    add("the reply missed the hidden fact", "fail");
  }
  if (c.check && c.check.asked > 0 && c.check.passed < c.check.asked) {
    add("the declared answer was wrong", "fail");
  }
  if (c.size_valid === false) add("the prompts were not the size the cell asked for", "warn");
  if (c.output_valid === false) add("the replies look like noise", "warn");
  if (c.stream_incremental === false) add("the server buffered the whole answer", "warn");
  if (c.incomplete) add("a request was lost", "serious");
  for (const v of Object.values(c.spread || {})) {
    if (v < SPREAD_STEADY) { add("the repetitions disagree", "warn"); break; }
  }
  return out;
}

/** verdictChip is the icon and the word of a verdict, with the parts that
 *  explain it (the band the verdict used, the spread of the repetitions).
 *  The parts may be one string or a list of them; the empty ones are
 *  dropped. The colour never carries the meaning alone. */
function verdictChip(v, extra) {
  const d = VERDICTS[v];
  if (!d) return h("span.pill.verdict.skip", "–");
  const parts = Array.isArray(extra) ? extra.filter(Boolean) : (extra ? [extra] : []);
  const why = parts.length ? ", " + parts.join(", ") : "";
  return h("span.pill.verdict." + d.tone, d.icon + " " + d.word + why);
}
