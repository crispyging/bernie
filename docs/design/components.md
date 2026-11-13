# Dashboard v3 — the component catalogue

Part of the design of the dashboard. The main file is
[../design-dashboard.md](../design-dashboard.md).

Every component is built with `h()` in `dashboard/web/js/core.js`. There is
no `innerHTML`, no image and no dependency. The SVG of each component is in
`design/<name>.svg`; it is the drawing, not the markup.

## The shared component API

| File | What it holds | What it publishes |
|---|---|---|
| `web/js/core.js` | the DOM helpers | `h()`, `put()`, `add()`, `$`, `num()`, `bytes()`, `ago()` |
| `web/js/shell.js` | the page-wide behaviour | `toast(msg, ok)`, `copy(text)`, `tooltip()` |
| `web/js/data.js` | the state, the fetches, the streams | `api(path, opts)`, `state`, `openTelemetry()`, `openRunLogs()`, `openJobEvents()` |
| `web/components/command.js` | the command block and the copy button | `cmdBlock(text, label)`, `copyBtn(text, label)` |
| `web/components/cards.js` | the frames | `cardHead(title, path, ...rest)`, `stateHead(eyebrow, action)`, `setHead(head, title, lede)`, `emptyState(what, next, command)`, `loading()`, `failed(e)`, `factRow(label, value, why)` |
| `web/components/charts.js` | the drawings | `spark(values, max, label, opts)`, `areaChart(input, opts)`, `ring(pct, label, opts)`, `meter(value, max, opts)` |
| `web/components/metrics.js` | the numbers of a benchmark | `metricTable(cell)`, `verdictChip(v)`, `barCompare(a, b)`, `lengthCurve(cells)` |
| `web/components/timeline.js` | the steps of the checklist | `stepTimeline(steps, options)` |

A worker who needs a new part puts it in the file of its role and tells the
other workers. No worker writes in the file of another.

## The state headline

`design/state-headline.svg`

| Part | Rule |
|---|---|
| Eyebrow | the name of the tab, 12 px 800 uppercase, `--muted` |
| Headline | one `h1`, 900 uppercase, the hard accent shadow. It says the situation in one sentence, never the mechanism. |
| Lede | one sentence under it: what it means and what to do next. `--text-2`, 18 px. |
| Action | the row of the actions of the tab; exactly one of them is filled. |
| Live | `aria-live="polite"` on the `h1`: a screen reader hears the new state when it changes. |

States: loading (`Reading the host…`), ready, stale (a banner above it:
`bernie is checking the host again. The values below are the last known
state.`), error (the message, the endpoint, `Try again`).

Data: the tab fills the headline and the lede. Nothing else.

## The primary action button

`design/button-primary.svg`

| Part | Rule |
|---|---|
| Label | one verb and one object: `Set up this machine`, `Run the suite`, `Try it`. |
| Fill | `--accent` background, `--on-accent` text, 900 uppercase, a 3 px hard shadow `3px 3px 0 color-mix(in srgb, var(--accent) 45%, #000)` |
| Active | the shadow moves down 2 px and the button moves up 2 px |
| Disabled | `--surface-2` background, `--muted` text, no shadow, `aria-disabled="true"` |
| Busy | the label stays, a small spin sits before it, the button is disabled |

Every other button of the tab is a ghost button: transparent, a 1 px
`--border-strong` border, an 8 px radius, `--text` label.

Data: the label, the action, whether it is disabled.

## The status pill

`design/pill.svg`

An icon and a word, never a colour alone.

| State | Icon | Word | Colour |
|---|---|---|---|
| good | ✓ | `ok` | `--ok` |
| warning | ! | `warn` | `--warn` |
| serious | ▲ | `serious` | `--serious` |
| bad | ✕ | `fail` | `--fail` |
| idle | – | `skip` | `--skip` |
| info | i | `info` | `--info` |
| running | a spin | `running` | `--info` |

The pill is 12 px 600, a 1 px border in the colour, a `--radius-pill`
border-radius and 2 px of vertical padding. Data: the state and the word.

## The command block

`design/cmd-block.svg`

| Part | Rule |
|---|---|
| Command | `--font-mono` 12.5 px on `--surface-2`, `pre-wrap`, the text exactly as it is run |
| Copy | a ghost button pinned to the top right, `type="button"`, `aria-label="Copy the command"` |
| Long | a command longer than the card wraps; it never scrolls sideways |

Every command of the page is in a command block: the fix of a finding, the
root command of a step, the launch command of a plan, the URL of a run, the
curl of a request.

Data: the command text.

## The fact row

`design/fact-row.svg`

| Part | Rule |
|---|---|
| Label | 12 px uppercase `--muted`, the name of the fact |
| Value | 17 px 700, the value with its unit |
| Why | 12 px `--muted`, one sentence: why this value and what it means for the reader |

The fact strip of the hero shows four facts, separated by a hairline and
not by boxes. A fact without a reason shows the value alone. A missing
value is `–`.

Data: the label, the value and the reason.

## The step timeline row

`design/step-row.svg`

| Part | Rule |
|---|---|
| Mark | a 36 px circle with the number of the step, or the icon of the state, and a 2 px connector to the next row |
| Title | the title of the step and the state pill |
| Sentence | the summary of the step, one plain sentence |
| Action | `Run` when bernie can do the step; a command block and no button when the step needs root |
| Progress | one sentence and one bar while the step runs: `Downloading the model weights, 2 of 5 shards` |
| Log | a disclosure under the step with the output of the job |
| Problems | the doctor checks that hold the step back, each with its fix |

| State | Mark | Word | What the row offers |
|---|---|---|---|
| done | ✓ | `done` | the sentence, and `Run again` as a ghost button |
| todo | the number | `todo` | `Run` as the primary button of the step |
| running | a spin | `running` | the progress sentence, the bar and the live log |
| blocked | ! | `blocked` | the root commands with a copy button, and `Re-check` |
| skipped | – | `skipped` | the sentence and the reason it does not apply |
| failed | ✕ | `failed` | the reason, the log and the fix |

Data: the step record of spec §3.3 (`id`, `title`, `state`, `summary`,
`detail`, `command`, `can_run`, `root_commands`, `progress`, `problems`) and
the job document while the step runs.

## The KPI tile

`design/kpi-tile.svg`

| Part | Rule |
|---|---|
| Label | the name of the number, with an `i` mark that opens a tooltip |
| Number | 30 px 900 with the unit in 13 px `--muted` |
| Sub | one line of context: `1-min avg 44.0 tok/s` |
| Spark | a 34 px sparkline of the last minute, with its numbers in a table under it |

A missing number shows `–` and the unit greyed. The tile never grows a
second line of numbers. Data: the label, the value, the unit, the sub-line
and the series.

## The area chart

`design/area-chart.svg`

| Part | Rule |
|---|---|
| Title | the name of the measure and the window |
| Window | a segmented control: `5 min`, `15 min`, `1 h` |
| Y axis | three labels, with the unit |
| X axis | the time, every fifth point |
| Area | the fill in `--series-1` at 18 % and the line at full strength |
| Readout | on hover, the time and the value under the pointer |

The chart is an inline SVG, `overflow: visible`, with the grid lines in
`--border`. A measure with no data draws an empty axis with the sentence
`No samples yet.`. Data: one numeric history series (G4) and the window;
unmeasured points stay gaps. All current callers use this single-series form.

The chart is built once: the grid, the two axes, the paths, the crosshair
and the dot stay in the DOM. An update writes the path of a series and
moves the labels; it never builds a node again, and a frame whose document
did not change writes nothing at all. A series holds at most 240 points:
the cap keeps the lowest and the highest value of every bucket, so a peak
of one sample is never lost.

Under the chart the same numbers are in a table, in a `details` element:
`The numbers of the chart`. One row per point, the unit in the head, the
caption in words. The rows are written, never built again.

## The busy ring

`design/busy-ring.svg`

A horizontal 0–100 % bar: the track in `--surface-3`, the fill in
`--series-1`, the number above it at 900, and a caption under it: `GPU 0`.
The number is the load of the card in percent; a card that does not report
one shows `–`.

Data: the load of the card and its name.

## The meter

`design/meter.svg`

| Part | Rule |
|---|---|
| Label | the name of the measure |
| Value | the value with its unit, right aligned |
| Track | 8 px high, a 4 px radius, `--surface-3` |
| Fill | `--series-1`; `--serious` above the cap, `--ok` below it |
| Cap | a 1 px tick on the track where the cap sits |

The power meter draws the cap as a tick and says so in the caption: `38 W of
175 W`. A value that was not measured shows `–` and an empty track.

Data: the value, the maximum and the cap.

## The table

`design/table.svg`

| Part | Rule |
|---|---|
| Head | 12 px uppercase `--muted`, left for names, right for numbers |
| Rows | a hairline rule between them, numbers right aligned with tabular figures |
| Numbers | every number carries its unit in the head of its column |
| Missing | `–` |
| Wide | the table scrolls inside its card; the card never scrolls the page |

States: loading (three skeleton rows), empty (the empty state of §2.7),
error (the failed state), scrolling.

Data: the columns and the rows.

## The finding card

`design/finding-card.svg`

| Part | Rule |
|---|---|
| Head | the state pill and the id of the rule |
| Cause | one sentence: what the log said and what it means |
| Fix | the command that fixes it, in a command block with a copy button |
| Doc | the link to the rule in the documentation |

A finding is never a bare error code. States: `error` (a red pill), `warn`
(an amber pill), none (the card says `No known failure in this log.`).

Data: the match of spec §5.3 (`id`, `severity`, `cause`, `fix`, `doc`,
`line`, `line_no`).

## The suite card

`design/suite-card.svg`

| Part | Rule |
|---|---|
| Radio | a big radio; one suite is picked at a time |
| Name | the name of the suite |
| Detail | one line: what it measures |
| Digest | the digest of the suite in mono, with a copy button |
| Size | the number of cells and the estimated time |

States: picked, not picked, running (the card shows the progress of the
run), done (the card shows the result). Data: the suite of `GET
/api/bench/suites` (the id, the name, the description, the digest and the
cells).

## The verdict chip

`design/verdict-chip.svg`

An icon and a word: `▲ better`, `– same`, `▼ worse`, `! noisy`. The colour
is `--ok`, `--skip`, `--fail` and `--warn`. The chip sits next to the
number it judges, never alone. A comparison that cannot be decided says so
in words: `the repetitions disagree, so this metric has no verdict`.

The chip of a comparison carries the band it used and, when the
repetitions disagree, their spread: `! noisy, band 10.9 %, spread ±10.0 %`.
The band is the share of a change that counts as noise; the backend widens
the band of the suite with the spread the arms measured.

Data: the verdict of one metric of one cell, its band and the spread of
the cell.

## The charts by context

Three charts side by side, one measure each: the output rate, the aggregate
prefill and the time to first token (p50). They replace the length curve.

| Part | Rule |
|---|---|
| X | the contexts of the result, evenly spaced (2K, 4K, … 128K); a result of one context puts the in-flight levels along x |
| Y | every chart starts at zero and its own maximum; the unit is in the caption |
| Series | one line per in-flight level, `--series-1` to `--series-4`, each with its own dash pattern; one legend above the three charts names them |
| Points | every point has a `title` with its series, its x and its number; a missing point breaks the line and is never interpolated |

Data: the cells of the result that ran. The context × in-flight table under
the charts holds the same numbers as text.

## The bar comparison

`design/bar-compare.svg`

The comparison is cell by cell: one block per cell, named `1k at
concurrency 4`, and inside it the five metric groups of spec §10.1. No
metric is ever taken from two different cells.

| Part | Rule |
|---|---|
| Blocks | one per cell of the comparison, each with its name |
| Groups | the five metric groups of spec §10.1, one row per group |
| Bars | the baseline in `--series-1`, the candidate in `--series-2`; every bar of every block is indexed to the baseline on one scale for the whole chart (the baseline is 100) |
| Labels | the two numbers of the row with their unit, and the change in percent |
| Verdict | a verdict chip per row, with the band it used |
| Direct labels | the two arms are named on the marks, not only in the key |
| Invalid | when the two results do not measure the same workload, the chart is not drawn: the card says `These two runs measured different things.` |

Under the chart the same numbers are in a table, in a `details` element:
`The numbers of the comparison`.

Data: two results and their comparison (`GET /api/bench/compare`).

## The toast

`design/toast.svg`

Fixed at the bottom centre, 2.6 s, `role="status"`, `aria-live="polite"`.
One at a time: a new one replaces the old one. A click dismisses it. Kinds:
the plain one, the good one (a tick) and the bad one (a cross).

Data: the message and the kind.

## The tooltip

`design/tooltip.svg`

A small `i` mark opens a fixed panel with `role="tooltip"`. It opens on
hover and on focus, closes on Escape and on blur, and it never holds the
only copy of a fact.

Data: the text of the tooltip.
