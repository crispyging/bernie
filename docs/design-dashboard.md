# Dashboard v3 — the exact design

Status: design spec, 2026-10-28. Nothing in this file is implemented yet.

This file is the contract for the front-end workers. The behaviour of the
product is [spec.md](spec.md); this file is the contract for the page.

Read this file, then the sheet of the tab you build:
[design/components.md](design/components.md) (the component catalogue),
[design/setup.md](design/setup.md), [design/dashboard.md](design/dashboard.md),
[design/benchmark.md](design/benchmark.md) and
[design/logs.md](design/logs.md) (the tab sheets). The wireframes are in
`design/*.svg`: one per tab and state, plus one per component. Every file of
this set stays under 400 lines.

The wireframes draw the layout and the information design. The colours and the
type of the shipped page are the ones of [design/tokens.md](design/tokens.md):
the drawings were made before the reference became the source of the palette,
so a drawing and the page differ in colour and in the letterforms, never in
the arrangement.

## 1. Principles

1. **Ours, on the reference's licence.** The reference (vllm-rdna-dash by
   CrispyGinger) is the source of the palette, the type and the rail. It is
   licensed under Apache-2.0, so bernie may use its design, markup and CSS;
   every file that carries them says so in its header, and the
   third-party list records the use. The words, the numbers, the API and the
   information design are ours: a state headline, one primary action, a step
   timeline, facts with reasons, KPI tiles and copy buttons are a pattern, not
   an expression. Facts that belong to the machine (a threshold, a sysfs path,
   a metric name) are re-expressed in our own words. §5 records, per
   component, what the reference does, what we do and why.
2. **Words first.** Every state is a sentence before it is a colour or a
   number. A badge is an icon and a word. A missing number is `–`, never `0`.
   Numbers carry their unit and their range.
3. **One primary action per tab.** Exactly one filled button per tab: the
   action the current state asks for. Every other control is a ghost button.
4. **Plain and legible.** System type, no web fonts, no CDN, no images except
   text glyphs. Short sentences, active voice, second person.
5. **Accessibility is the floor.** Text at 4.5:1, large text and control
   edges at 3:1, a visible focus ring, the whole page with the keyboard,
   `role="tab"` with arrow keys, `aria-live` on the state headline.
6. **No framework.** Plain HTML, CSS and JavaScript embedded in the binary.
   No npm, no build step. DOM is built with `h()`; `innerHTML` never appears.
7. **One question per tab.** Setup: what do I do to make this box serve?
   Dashboard: is it serving, and how fast? Benchmark: how fast is it, and
   does it read the whole prompt? Logs: what did the server say?
8. **390 px is a first-class width.** Nothing is hidden, nothing overflows
   the page, tables scroll inside their card.
9. **The page is a view of the CLI.** A value on the page exists in the
   `--json` output of a command with the same name. If it does not, the
   command goes first (§7 lists the gaps). Empty states teach: no runs? Say
   what a run is and show the command that starts one.

## 2. The shell

The shell is the same on every tab. It is a rail on the left and a content
column on the right.

```
┌────────────┬──────────────────────────────────────────────────┐
│ bernie     │  Setup                                           │
│ host       │  ────────────────────────────────                │
│            │  headline, lede, facts                           │
│ ▸ Setup    │  hero: copy on the left, facts on the right      │
│   Dashboard│  steps, recipes, doctor                          │
│   Benchmark│                                                  │
│   Logs     │                                                  │
│            │                                                  │
│ ● running  │                                                  │
│ [Auto][Ad] │                                                  │
└────────────┴──────────────────────────────────────────────────┘
```

### 2.1 The rail

| Part | Rule |
|---|---|
| Frame | `display: grid; grid-template-columns: 220px 1fr;` the rail is `position: sticky; top: 0; height: 100vh` |
| Padding | `22px 14px` |
| Brand | the mark of the reference (a 22 px bar in the accent and the key) and the word `bernie` in italic 900 20 px; the host name under it in 13 px `--muted` |
| Links | one per tab, a glyph and the word, `border-radius: 8px`, `padding: 9px 10px`, `gap: 2px` between them, 13 px 600; the open one has a `--surface-2` background, a 2 px `--accent` bar on the left, the key-coloured glyph and a heavier word: the marker is a shape, not only a colour |
| Counts | a tab that has something to look at shows the number: the failed or blocked steps on Setup, the failing checks of the doctor on the Dashboard, the failed P2P readiness checks on Topology |
| Footer | the run status pill with the phase sentence, then the theme control (`Auto`, `Dark`, `Light`), the Advanced button and the `?` mark with the shortcuts; `margin-top: auto` |
| Keyboard | `g` and then `s`, `d`, `b`, `t`, `l` or `c` opens a tab; the `?` mark lists the shortcuts |
| Width | fixed at 220 px, not collapsible; under 720 px it becomes a tab bar at the bottom of the page (§8.1) |

### 2.2 The tab bar

Eight tabs, in this order: **Setup**, **Dashboard**, **Chat**, **Benchmark**,
**Topology**, **Fleet**, **Logs**, **Advanced**. Six of them are the questions
of §6. Chat and Fleet are planned: their entry in the rail carries the word
"Soon" and their panel holds one honest sentence and no control. Advanced is
the reference-parity port: the old pages, the whole of them, under one tab.

The rail links are a real tablist: `role="tablist"` on the container,
`role="tab"` on each link, `aria-controls` pointing at the panel,
`aria-selected` on the open one, a roving `tabindex`, and Left/Right/Home/End
moving the focus. The panel is `role="tabpanel"` with `aria-labelledby` the
tab.

Routes are hash routes: `#setup`, `#dashboard`, `#benchmark`, `#logs`.

Old links keep working. The router rewrites them and replaces the history
entry, so the back button does not walk through them:

| Old route | Opens | Lands on |
|---|---|---|
| `#home`, `#overview` | Setup | the headline |
| `#gpus` | Dashboard | the per-GPU cards |
| `#topology` | Dashboard | the topology section |
| `#runs` | Dashboard | the run card |
| `#recipes` | Setup | the recipe chooser |
| `#bench` | Benchmark | the suite cards |

A section anchor inside a tab is `#dashboard/topology`; the router scrolls to
the section and sets the focus to its heading.

### 2.3 The run status pill

The pill in the rail footer answers "is something serving?".

| State | Icon | Word | Colour |
|---|---|---|---|
| a run is `running`: it answers `/health` and a completion | ● | `serving` | `--ok` |
| a run is loading the model | ◐ | `starting` | `--info` |
| a run exists but does not answer | ! | `not answering` | `--warn` |
| no run | – | `idle` | `--skip` |
| the run died | ✕ | `stopped` | `--fail` |

The pill carries the phase sentence as its `title` and its `aria-label`, for
example `starting — Loading the model weights, 2 of 5 shards`. A starting run
without a phase sentence carries its `waiting` reason instead (spec §8.3). The full
sentence is always written out in the state headline of the tab.

### 2.4 Theme and Advanced

- **Theme.** The rail holds a segmented control with the three choices, in
  words: `Auto`, `Dark` and `Light`. `Auto` is the default: the page follows
  the system (`prefers-color-scheme`) and follows it live, so a laptop that
  changes its palette at sunset changes the page with it. `Dark` and `Light`
  are the two palettes of the reference, as overrides. The choice is stored
  in `localStorage` (`bernie.theme`) and the palette goes on `<html>` as
  `data-theme="dark"` or `data-theme="light"`. The three buttons carry
  `aria-pressed`, and the choice is said out loud in a live region
  (`role="status"`): `The theme follows the system.` or `The theme is dark.`
  The palette is set before the first paint — `js/theme.js` is loaded in the
  head of the page — so the page never shows the other palette for a moment.
- **Advanced.** The button opens or closes every Advanced panel of every tab
  at once and stores the state (`bernie.advanced`). It has `aria-pressed`.
  Each tab also has its own Advanced panel with its own toggle, so a tab can
  be opened without the header.

### 2.5 Toast, tooltip, copy

- **Toast.** Fixed at the bottom centre, 2.6 s, `role="status"`,
  `aria-live="polite"`, one at a time. A click dismisses it. It is used for
  a copy, a saved change and a failed write. The text says what happened:
  `Copied the command.`, `Power cap saved. The service applies it.`,
  `Stop failed: the run is already gone.`
- **Tooltip.** The small `i` mark opens a panel with `role="tooltip"`. It
  opens on hover and on focus, closes on Escape and on blur. The tooltip
  never holds the only copy of a fact: the same sentence is in the label of
  the value or in the Advanced panel.
- **Copy.** Every command, plan, URL and JSON document has a copy button
  next to it. The button copies the text exactly as it is shown. The button
  reads `Copy`, then `Copied` for 1.2 s, and the toast confirms. A failure
  toasts `Copy failed — select the text and press Ctrl-C.`

### 2.6 The Advanced panel

Every tab ends with one Advanced panel. It holds two things:

1. **The raw JSON** of the endpoints the tab used, one `<pre>` per endpoint,
   each with a `json` link (the endpoint in a new tab) and a copy button.
2. **The CLI equivalent**: the commands that produce the same documents,
   each in a command block with a copy button.

| Tab | The documents | The commands |
|---|---|---|
| Setup | `/api/onboarding`, `/api/recipes`, `/api/recipes/{id}/plan`, `/api/doctor` | `bernie status --steps`, `bernie status --json`, `bernie recipe list`, `bernie recipe plan ID`, `bernie doctor` |
| Dashboard | `/api/hw`, `/api/topo`, `/api/topo/check`, `/api/doctor`, `/api/runs`, `/api/power`, `/api/telemetry` | `bernie hw`, `bernie topo`, `bernie topo --check`, `bernie doctor`, `bernie ps`, `bernie tune power status` |
| Benchmark | `/api/bench`, `/api/bench/suites`, `/api/bench/compare` | `bernie bench list`, `bernie bench suites`, `bernie bench compare A B` |
| Logs | `/api/runs`, `/api/runs/{name}/logs` | `bernie ps`, `bernie logs NAME` |

### 2.7 Loading, empty and error

| State | What it looks like |
|---|---|
| Loading | three grey skeleton lines and the sentence `Reading the host…`. Never a spinner alone. |
| Empty | two sentences: what the thing is, then the command that makes one. The command is in a command block with a copy button. |
| Error | the message in words, the endpoint that failed, a `Try again` ghost button and the raw answer in a disclosure. |
| Stale | the onboarding document is older than its time to live: a banner `bernie is checking the host again. The values below are the last known state.` with a 3 px `--warn` bar on the left. |

### 2.8 What we changed from the reference, and why

The reference is the source of the palette, the type and the rail. These are
the places where we did not copy it, and the reason of each.

| Change | Why |
|---|---|
| The palette is the reference's, with the values AA needs | the reference's light palette fails 4.5:1 for the status colours and 3:1 for the edge of a control. Every value that failed was moved by the smallest step that passes; the original and the adjusted value are written down in [design/tokens.md](design/tokens.md) |
| The fonts are self-hosted | the reference loads Montserrat, Inter and JetBrains Mono from a CDN, which this repository forbids. The Latin subsets are 128 KB, carry the OFL and are embedded in the binary |
| The dark palette is the default | the reference is dark first. The theme control follows the system by default and remembers the choice of the reader |
| The four questions come first, the first dashboard sits in the Advanced tab | a reader arrives with one question at a time. The Advanced tab keeps the first dashboard reachable, and the old hash routes still land on the tab that answers them (§2.2) |
| Every link carries a glyph and the word | the word says what the tab is; the glyph is a landmark for the eye, never the meaning |
| The open tab has a bar, a background, a heavier word and the key-coloured glyph | the marker cannot be a colour alone: the page has to be readable without the colours and in the dark palette |
| A tab shows the number of things that wait | a failed step and a failing check are the reason to come back. The count says it before the reader opens the tab |
| The shortcuts are `g` and then a letter | one key arms the shortcut, so a letter typed in a field never moves the page. The `?` mark in the footer lists them |
| Under 720 px the rail becomes a bar at the bottom | the thumb reaches the bottom of a phone, and the bar of the reference keeps its place |
| The status pill carries the phase sentence | a word alone says that something runs; the sentence says what it is doing |

## 3. Tokens

The tokens live in `dashboard/web/css/tokens.css`. The names are ours. The
full palettes, the measured contrast of every pair, the type scale, the
spacing, the radii, the elevation, the motion and the chart colours are in
[design/tokens.md](design/tokens.md). The summary:

| Group | Rule |
|---|---|
| Surfaces | `--bg` `#f4f5f7`, `--surface` `#ffffff`, `--surface-2` `#eef1f5`, `--surface-3` `#e4e8ee` in the light palette; `#0f1216`, `#171b21`, `#10141a`, `#21262e` in the dark one. The dark palette is the base of the stylesheet; the light one is the override. The theme control picks between them (§2.4). |
| Lines | `--border` is a decorative hairline; `--border-strong` is the edge of a control and holds 3:1 (`#828a93` light, `#5a6778` dark). |
| Text | `--text`, `--text-2`, `--muted`. `--muted` holds 4.5:1 on every surface. |
| Identity | `--accent` (`#b3122c` light, `#ff5c6c` dark) and `--on-accent`; `--key` (`#8a4f00` light, `#f5c451` dark) is the one number that matters on a screen. |
| Status | `--ok` `--warn` `--serious` `--fail` `--skip` `--info`, a four-step scale and two quiet ones. |
| Charts | `--series-1..6` and `--seq-1..6`. |
| Type | the system stack, no web font, no CDN. Display 900 uppercase with a hard accent shadow; lede 18 px; eyebrow 12 px uppercase; mono 12.5 px. |
| Space and shape | 4 8 12 16 20 24 32 34 48 56 80 px; radii 6 8 9 10 16 18 999 px; two soft shadows, used by the toast and the tooltip only. |
| Motion | 120 ms hover, 200 ms meter, 400 ms progress bar, a 220 ms card entrance. `prefers-reduced-motion` removes all of it. |

Every pair of §3 of [design/tokens.md](design/tokens.md) was measured with
the WCAG 2.1 relative luminance formula. The numbers are the ratios.

## 4. The component catalogue

[design/components.md](design/components.md) holds every component with its
anatomy, its states, the data it takes and the SVG that draws it. The files
are `design/state-headline.svg`, `design/button-primary.svg`,
`design/pill.svg`, `design/cmd-block.svg`, `design/fact-row.svg`,
`design/step-row.svg`, `design/kpi-tile.svg`, `design/area-chart.svg`,
`design/busy-ring.svg`, `design/meter.svg`, `design/table.svg`,
`design/finding-card.svg`, `design/suite-card.svg`, `design/verdict-chip.svg`,
`design/length-curve.svg`, `design/bar-compare.svg`, `design/toast.svg` and
`design/tooltip.svg`.

The shared JavaScript keeps its parts in `dashboard/web/components/`:
`command.js` (the command block and the copy button), `cards.js` (the card
head, the state headline, the empty and the failed state), `charts.js` (the
sparkline, the area chart, the ring, the meter), `metrics.js` (the metric
table, the verdict chip, the bar comparison, the length curve) and
`timeline.js` (the step row). A worker who needs a new part puts it in the
file of its role and tells the other workers.

## 5. Reference parity table

The reference is [vllm-rdna-dash](https://github.com/crispyging/vllm-rdna-dash).
It is licensed under Apache-2.0, so bernie may take its markup, CSS and copy.
An adapted file names the source and the licence in its header. This table
is the swap list: it says, component by component, how close bernie is to
the reference.

| Component | Reference behaviour and measurement | Our implementation | Status |
|---|---|---|---|
| Frame | 220 px sticky rail, content `padding: 34px clamp(16px, 4vw, 56px) 80px`, view at most 1240 px | the same, in §2.1 | same (done) |
| Rail | brand with 26 px of space under it, links with an 8 px radius and a 2 px gap, a footer with a status pill and a theme button | the same, plus the counts on the tabs, the Advanced tab and the shortcuts (§2.8) | ported (done) |
| Tab bar | plain links, no `aria-controls` | the rail of the reference over a real tablist: arrow keys, `aria-selected` and a roving tabindex | adapted (accessibility, done) |
| Display headline | 38–74 px, italic 900, uppercase, a hard two-layer offset shadow | the same, with the self-hosted Montserrat instead of the CDN font | ported (done) · ported 1:1 (Dashboard tab) |
| Lede | 18 px, the second text colour, 70 characters | the same | same (done) |
| Eyebrow | 12 px 800, uppercase, `.14em` | the same | same (done) · ported 1:1 (Dashboard tab) |
| Section | a 1 px top rule and 24 px of padding, a 13 px uppercase label | the same | same (done) |
| Facts strip | an uppercase 12 px label over a 17 px 700 value, hairline separated | the same, plus the reason under the value | adapted (bernie feature, done) |
| Step timeline | 36 px counter circles, a 2 px connector, one colour per state | the same; the six states are the ones of spec §3.3 | same (done) |
| Command block | mono on the second surface, an 8 px radius, the copy button pinned to the top right | the same, with the copy button beside the command so a long command wraps instead of scrolling | ported (done) · ported 1:1 (Dashboard tab) |
| Primary button | italic 900 uppercase, the key colour, a 4 px hard shadow that moves on `:active` | the same: the key colour in the dark palette, the accent in the light one, the hard shadow of the reference | ported (done) · ported 1:1 (Dashboard tab) |
| Ghost button | transparent with a 1 px border and an 8 px radius | the same | same (done) · ported 1:1 (Dashboard tab) |
| Segmented control | a 9 px radius track, 3 px of padding, a raised active segment | the same; it holds the host/container mode and the history window | same · ported 1:1 (Dashboard tab) |
| Status pill | an icon, a word and a colour | the same, and it carries the phase sentence | ported (done) · ported 1:1 (Dashboard tab) |
| KPI tile | a 30 px 900 number with its unit, a sub-line, a 34 px sparkline | the same; the eight tiles of the fleet, in `web/views/dashboard/gpus.js` | same (done) · ported 1:1 (Dashboard tab) |
| Hero number | a 64 px 900 number in the key colour with a 44 px sparkline | the same; it carries the output speed of the live run | same (done) · ported 1:1 (Dashboard tab) |
| Meter | a 6–8 px track, a 4 px radius, a coloured fill, 400 ms | the same; the memory and the power of every card, with the tick of the cap | same (done) · ported 1:1 (Dashboard tab) |
| Donut ring | an 84 px SVG ring with the number in the middle and a caption under it | the same; the load of every card | same (done) · ported 1:1 (Dashboard tab) |
| Area chart | an inline SVG, grid lines, axis labels, a five-minute window | the same, with a window the reader picks (5 min, 15 min, 1 h) | adapted (bernie feature, done) · ported 1:1 (Dashboard tab) |
| Matrix | 2 px gaps, 6 px radius cells, one colour per class | the same; the classes of the PCIe links use `--series-1..4` | same (done) |
| Bar chart | SVG bars for the comparison of two runs | the same; the five metric groups of spec §10.1 are the groups | same (done) |
| Log viewer | 12.5 px mono on near-black, 340 px tall, `pre-wrap` | the same | same |
| Progress bar | a 10 px track, a 6 px radius, the key colour, and one plain sentence under it | the same | same (done) |
| Radio card | a big radio, a title, one line of detail | the same; it picks the benchmark suite | same (done) |
| Toast | fixed at the bottom centre, 2.6 s, `role="status"`, `aria-live="polite"` | the same | same (done) |
| Tooltip | a small mark that opens a fixed panel with `role="tooltip"` | the same, and it opens on focus as well | adapted (accessibility, done) · ported 1:1 (Dashboard tab) |
| Banner | the third surface with a 3 px bar on the left, dismissible | the same; it carries the stale and the offline notice | same (done) |
| Copy on every command | one copy button per command, per endpoint and per snippet | the same | same (done) · ported 1:1 (Dashboard tab) |
| One primary action per view | one filled button, everything else a ghost | the same; the Try it box of the run card is the filled button | same (done) · ported 1:1 (Dashboard tab) |
| Empty states that teach | what the thing is, then what to do | the same; the run card and the power caps teach | same (done) · ported 1:1 (Dashboard tab) |
| Chat view | a transcript with a meta line per message | deferred, see §10 | deferred |
| Fleet view | a list of other boxes | out of scope: bernie is one host | out of scope |


The **Advanced tab** (`web/views/advanced/`, `web/css/advanced.css`) is the
first bernie dashboard, kept as it is: the same hero with its tiles, the same
cards, the same charts of the last minutes, the same run card, the same power
caps, the same topology, the same known problems and the same raw documents
at the end. It is drawn with the current design — the tokens, the fonts and
the shared components of the shell — and it reads the same answers as every
other tab. It adds no endpoint. The old route `#classic` still lands on it.

## 6. The tabs

| Tab | Question | Sheet | Wireframes |
|---|---|---|---|
| Setup | What do I do to make this box serve? | [design/setup.md](design/setup.md) | `design/setup-1280.svg`, `design/setup-390.svg`, `design/setup-blocked-1280.svg` |
| Dashboard | Is it serving, and how fast? | [design/dashboard.md](design/dashboard.md) | `design/dashboard-1280.svg`, `design/dashboard-390.svg`, `design/dashboard-empty-1280.svg` |
| Chat | What will I ask the model? | — | — |
| Benchmark | How fast is it, and does it read the whole prompt? | [design/benchmark.md](design/benchmark.md) | `design/benchmark-1280.svg`, `design/benchmark-390.svg`, `design/benchmark-empty-1280.svg` |
| Topology | Can the cards copy to each other? | [design/topology.md](design/topology.md) | — (not drawn yet) |
| Fleet | What do the other hosts do? (planned) | — | — |
| Logs | What did the server say? | [design/logs.md](design/logs.md) | `design/logs-1280.svg`, `design/logs-390.svg`, `design/logs-empty-1280.svg` |

Every tab opens with a state headline: one sentence that says what the
situation is, and one lede under it that says what to do next. The headline
is `aria-live="polite"`.

## 7. Data and API gaps

The widgets below need a field or an endpoint that does not exist yet. The
proposal is written so that a backend worker can implement it without
asking. Everything else in this design reads an endpoint of spec §11 as it
is.

| # | Widget | What is missing | Proposal |
|---|---|---|---|
| G1 | The facts with their reasons on Setup | the onboarding document has no facts | add `facts` to the document of §3.3: `{"facts": [{"id": "gpus", "label": "GPUs", "value": "8 × Radeon PRO V620", "why": "Every card is gfx1030, so one recipe family fits them all."}]}`; the CLI prints the same array in `bernie status --steps`. The field is `id` and not `key`: every answer goes through `internal/support/redact`, and a member whose name ends in `key` names a secret, so the redactor would hide it |
| G2 | The progress of the model download | the job document has no numbers | add `progress` to the job document: `{"progress": {"phase": "model.download", "bytes_done": 1800000000, "bytes_total": 2300000000, "rate_bps": 45200000, "files_done": 7, "files_total": 14, "eta_s": 11}}`; the existing `done`/`total` stay for the steps that count items. A number that is not measured is absent: the bytes, the rate and the time to finish are there for a download, the file counts are there when the step counts the files |
| G3 | The "Try it" box on Setup and on the Dashboard | there is no endpoint for one prompt | new `POST /api/smoke`, body `{"prompt": "Hello", "run": "flashnext"}`: it calls the same `onboarding.Smoke` as `bernie smoke` with the prompt of the body, and answers `{"url": "http://127.0.0.1:8000", "model": "flash-next", "health": true, "reply": "…", "ttft_ms": 214, "output_tokens": 12, "tok_s": 56.1}`; `ttft_ms`, `output_tokens` and `tok_s` are new fields of `onboarding.SmokeResult`; the target is the running run or the one named by `run`, and a name that is not running is a 404 |
| G4 | The history charts of the Dashboard | the telemetry stream holds one sample, no history | new `GET /api/gpus/history?window=15m`: `{"window": "15m", "interval_s": 2, "points": 450, "series": [{"hip_index": 0, "busy_pct": [0, 12, 30], "power_w": [11.2, 18.4, 22.0], "vram_used": [24117248, 24117248, 24117248], "tokens_per_s": [0, 0, 0]}]}`; the ring buffer holds one sample per GPU every 2 s for 30 minutes (900 points) in the hub; `window` is `5m`, `15m` or `1h`; `1h` is downsampled to one point per 15 s; the answer is at most 120 KB and is cached for 2 s; no series holds more than 240 points (a longer window is downsampled further: 15 m holds one point every 4 s, 1 h every 15 s); `tokens_per_s` is the output rate of the live run and `prompt_tok_s` is the prefill rate, both read from the hub's vLLM metrics sample; the rate is a property of the server, so every card carries the same number, and the fields are absent when no run serves; the hub samples the host only while a page watches `/api/telemetry`, so the buffer fills while the dashboard is open |
| G5 | The log viewer with levels and a search | the log stream sends plain lines | new `GET /api/runs/{name}/log?tail=2000&level=error&q=oom`: `{"name": "flashnext", "state": "exited", "total_lines": 8123, "truncated": false, "lines": [{"no": 4211, "level": "error", "text": "…"}]}`; `level` is `all`, `error`, `warn` or `info`; `q` is a case-insensitive substring; the lines are the tail of the log, newest last |
| G6 | The findings of the log doctor for one run | the findings only ride on `ps --json` for an exited run | new `GET /api/runs/{name}/findings`: `{"name": "flashnext", "matches": [{"id": "memory.kv-too-small", "severity": "error", "cause": "…", "fix": "bernie up flashnext --set args.max-model-len=16384", "doc": "…", "line": "…", "line_no": 82}]}`; it is the scan of `doctor --run`, so the same rules and the same exit code apply |
| G7 | The live progress of a benchmark | `POST /api/bench` runs to the end and answers the result | new `POST /api/bench/run` with the same body as `POST /api/bench`: it answers `202` and `{"id": "b-7f3a"}`; new `GET /api/bench/{id}` answers `{"id": "b-7f3a", "status": "running", "phase": "cell 2 of 6: 1k at concurrency 4", "done": 2, "total": 6, "result_id": ""}`; new `GET /api/bench/{id}/events` is a server-sent stream with `progress`, `log` and `done` or `failed` events; the old `POST /api/bench` stays as it is |
| G8 | The KV cache of the live run | it exists | `GET /api/telemetry`, `runs[].stats.kv_cache_usage` (a fraction from 0 to 1) | none |
| G9 | The ECC state of a card | it exists | `GET /api/doctor`, the check whose id is `gpu.ecc.<hip index>` | none |
| G10 | The endpoint and the age of a run | they exist | `GET /api/runs`, `url`, `started_at` | none |
| G11 | The receipt of a comparison | the compare endpoint answers the table only | `GET /api/bench/compare?a=ID&b=ID&format=markdown` answers `text/markdown`: the same text as `bernie bench compare A B --markdown` |
| G12 | The A/B run | the CLI has it, the API does not | `POST /api/bench/ab` with `{"base": "flashnext", "cand": "http://127.0.0.1:8001"}` answers `202` and `{"id": "b-7f3a"}`; the job endpoints of G7 carry it |

Rules for the new endpoints: they live in `dashboard/`, they call the same
`internal/app` function as the matching command, they answer the same JSON
as `--json`, writes keep the `X-Bernie` header check and the token rules of
spec §11, and every answer goes through `internal/support/redact`. A new
field in a domain struct changes `docs/spec.md` in the same commit.

## 8. Responsive, accessibility and performance

### 8.1 Responsive

| Width | The rules |
|---|---|
| ≥ 1280 px | the rail is 220 px, the content is at most 1240 px, the hero is two columns, the KPI tiles are six across |
| 960–1279 px | the tiles are four across, the GPU cards three |
| 640–959 px | the rail is 200 px, the hero stacks, the columns are two |
| ≤ 639 px | the rail becomes a bottom bar: the brand and the three buttons on the first row, the eight tabs in a grid of four columns and two rows under them; the tiles are two across, the charts and the GPU cards are full width, a table scrolls inside its card, the hero number is 40 px |
| 390 px | the page never scrolls sideways; every tab of the bar is whole (89 × 44 px), no word is cut off; a table scrolls inside its card; nothing is hidden |

### 8.2 Accessibility checklist

- [ ] The tab bar is a real tablist: `role="tablist"`, `role="tab"`, `aria-controls`, `aria-selected`, a roving `tabindex`, Left/Right/Home/End move the focus.
- [ ] Every control is a real `<button>` or `<a>`; a copy button is `type="button"`, so it never submits a form next to it. The focus ring is visible on every control: a 2 px `--accent` ring with a 2 px offset.
- [ ] Every status is an icon and a word. No meaning is carried by colour alone.
- [ ] Every chart has a text form: the same numbers are in a tile, a table row or a sentence.
- [ ] The state headline is `aria-live="polite"`; the toast is `role="status"`; the tooltip is `role="tooltip"`.
- [ ] The log viewer is `role="log"` with `aria-live="off"`; it is read on demand, not announced line by line.
- [ ] Every touch target is at least 24 × 24 px, and every control is a real `<button>` or `<a>`.
- [ ] `prefers-reduced-motion` removes every animation (§3.3).
- [ ] The page works with the keyboard alone: no trap, Escape closes a disclosure and a tooltip.
- [ ] Both palettes meet the ratios of §3.
- [ ] The empty, the loading and the error states say what happened in words.

### 8.3 Performance

The budget is measured, not promised. The Go test in
[`dashboard/assets_test.go`](../../dashboard/assets_test.go) (`TestPayloadBudget`,
`TestFirstPaintPayload`, `TestGzipSizeBudget`, `TestFirstPaintRequests`)
walks the embedded tree and fails the build when a number crosses a line;
the lines below are the current targets and the measured values 2026-10-29.

Two numbers matter. The **first paint** is what the reader waits for: it
holds the shell and the Setup tab, and nothing else. The **whole tree** is
what the binary carries: the code of the four tabs that are closed is in it
too, and it arrives when the reader asks for it.

#### The first paint

| Budget | Target | Measured | Note |
|---|---|---|---|
| The whole page (HTML + CSS + JS, uncompressed) | ≤ 375 KB | **356.6 KB** (4.3 + 275.3 + 77.1) | the whole tree; the first paint is 147.4 KB of it |
| HTML | ≤ 12 KB | **4.3 KB** | `index.html` |
| CSS | ≤ 80 KB, in eight files | **77.1 KB** | the design said four files; the split is `components.css` (the shared) + `views.css` (the per-tab) + `bench.css` + `classic.css` + `logs.css` + `dashboard.css` |
| JavaScript | ≤ 290 KB | **275.3 KB** | plain, hand-written and unminified; five tabs of it |
| JavaScript on the first paint | ≤ 112 KB | **111.6 KB** | the shell (49 KB) + the Setup tab (62 KB: the shared section/pill/empty-state helpers and the one-button path; the chooser and the path runner load on demand) |
| CSS on the first paint | ≤ 40 KB | **37.5 KB** | `tokens.css`, `layout.css`, `components.css`, `views.css` |
| Fonts | ≤ 130 KB, four woff2, OFL, `font-display: swap`, `unicode-range` | **128.4 KB**, four woff2, swap ✓, range ✓ | the license texts (24.5 KB) sit next to the files; they are not loaded by the page |
| First-paint requests | ≤ 24 | **21** (1 HTML + 4 CSS + 12 scripts + 4 fonts) | the files of the other tabs come when their tab opens |
| Transferred text on the first paint (gzip) | ≤ 60 KB | **46.4 KB** (HTML + CSS + JS, 32 %) | the gzipped HTML alone is 1.2 KB |
| Text of the whole tree (gzip) | ≤ 125 KB | **113.4 KB** (32 %) | the five tabs of plain JavaScript; the same text compresses worse in four files than in one, so the split of the Setup tab costs 1.2 KB of gzip |
| First paint on a loopback host | ≤ 1.0 s | **unmeasured** | the binary runs on this Mac; a measurement on the reference box (par1-cs25) lands in `dashboard/README.md` when the integrator pushes |
| The live numbers | one stream: `GET /api/telemetry` at 1 Hz drives every tile, every ring, every meter and every sparkline | the stream | the poller pauses while `document.hidden` and reads once on return (`web/js/data.js`) |
| The poller | one timer for the page, in `js/data.js`; no view opens its own timer | one timer | one `setTimeout` re-arms itself with a delay that grows on error (backoff, capped at 60 s) and shrinks on success |
| The charts | the history is read once per window change and then appended from the telemetry stream; the view never re-reads it | the points are capped at 240 per chart (`web/components/charts.js`) | an update writes only the path `d`; the axes are built once |
| The payload of the history | at most 120 KB for a 15-minute window; the ring buffer answers at most 900 points per GPU | the cap is 900 points (15 s × 60 s × 60 min × 2 s / 2) | the new `/api/gpus/history?window=…` (commit 6) honours it |

#### What the page costs, before and after

Measured on 2026-10-29. "Before" is the tree of `dfc99b4`, "after" is the
tree of `a3bd238` (the files of a tab come when the tab opens) and `5154cbf`
(the answers that read the host are held for a few seconds).

| What | Before | After | How |
|---|---|---|---|
| Requests on the first paint | 43 | **21** | the manifest of `web/js/boot.js` names the files of a tab |
| Text on the first paint | 341.8 KB | **147.4 KB** | the same |
| Text on the first paint, gzipped | 110.3 KB | **46.4 KB** | the same |
| JavaScript of the whole tree | 261.6 KB | 275.3 KB | the manifest adds 4 KB to `js/boot.js`; the five tabs grew with the comparison of the Benchmark tab |
| A tab switch | 0 extra requests | 6–8 extra requests, one time | the bundle of the tab, once |
| The probe of the host for one reader | 220–300 ms per answer | **the same answer for every reader of 5–15 s** | one cache per endpoint (`dashboard/internal/api/cache.go`) |
| The probe of the host for three readers | 3 probes | **1 probe** | the read is single-flight |
| The reads of `/sys` per second | 150 | **46** | the two tiers of the sampler (spec §11) |
| The cpu of `bernie dash` with a page open | 16.7 % of a core | **6.4 %** | the tiers, the cards read in parallel, the hwmon node and the sensor labels remembered |
| The cpu of `bernie dash` with no page open | 0 % | 0 % | the sampler runs only while a page watches the stream |

#### The cost of the server, measured on the 10 GPU box

`par1-cs25`, ten Radeon PRO V620, `bernie dash` on a loopback port, the
token of the dashboard. Every number is the median of two reads of the
endpoint with `curl`.

| Endpoint | Before | After | What the answer costs |
|---|---|---|---|
| `GET /api/hw` | 220–253 ms | **the first read only** | a probe of the host and of every card in `/sys` |
| `GET /api/topo` | 28–29 ms | **the first read only** | the PCIe tree of `/sys` |
| `GET /api/topo/check` | 226–303 ms | **the first read only** | the checklist of the peer-to-peer links |
| `GET /api/doctor` | 229–297 ms | **the first read only** | every check of the doctor |
| `GET /api/power` | 122–161 ms | **the first read only** | the status of the power service |
| `GET /api/runs` | 74–77 ms | **74–77 ms, at most every 2 s** | the list of the runs |
| `GET /api/onboarding` | 8–11 ms | 8–11 ms | the document is already cached (`internal/onboarding/cache.go`); the 20–25 s of the first report is gone |
| `GET /api/gpus/history?window=15m` | 9–10 ms | 9–10 ms | the ring buffer of the hub |

The sampler reads the cards once per second and the hub sends the sample to
every open page, so ten open tabs cost the same as one (spec §11).


## 9. The old tabs, the commit plan and the owners

### 9.1 Where the old tabs go

| Old tab | New home | What moves |
|---|---|---|
| Home (`#home`, `#overview`) | Setup | the headline, the facts, the step timeline, the doctor card, the run card |
| GPUs (`#gpus`) | Dashboard | the tiles, the per-GPU cards, the power caps |
| Topology (`#topology`) | Topology | the PCIe tree, the link classes and the readiness checklist: a tab of their own, not a section of the Dashboard tab |
| Recipes (`#recipes`) | Setup | the recipe list, the fit, the plan, the settings with their reasons, the mode switch |
| Runs (`#runs`) | Dashboard | the run card: state, phase, endpoint, uptime, stop, restart, the findings of the log doctor |
| Runs, the log | Logs | the live tail, the filter, the findings, the download |
| Bench (`#bench`) | Benchmark | the suites, the run, the results, the comparison, the length curve, the history |

### 9.2 The commit plan

Each commit is shippable on its own: the binary builds, the tests pass and
the page works. Each ends with a browser check.

| # | Commit | Files | The browser check |
|---|---|---|---|
| 1 | `dashboard: rebuild the shell as a rail with four tabs` | `web/index.html`, `web/css/tokens.css`, `web/css/layout.css`, `web/css/components.css`, `web/js/shell.js`, `web/js/boot.js` | Four tabs in the rail; `#home` opens Setup; the theme button flips the palette; the Advanced button opens the panels; nothing scrolls sideways at 390 px |
| 2 | `dashboard: draw the Setup tab as a timeline` | `web/views/setup/setup.js`, `web/views/setup/steps.js`, `web/views/setup/recipes.js`, `web/views/setup/advanced.js`, `web/components/timeline.js`, `web/components/cards.js` | The eight steps of `/api/onboarding` appear in order; Run starts a job and the output lands under the step; the root step shows a command with a copy button and no Run button |
| 3 | `dashboard: draw the Dashboard tab with the fleet tiles` | `web/views/dashboard/dashboard.js`, `web/views/dashboard/gpus.js`, `web/views/dashboard/power.js`, `web/views/dashboard/topology.js`, `web/views/dashboard/overview.js`, `web/components/charts.js` | The tiles follow `/api/telemetry`; the ring shows the load of the card; the power meter shows the cap; the topology section shows the matrix and the checklist |
| 4 | `dashboard: move the bench tab to Benchmark` | `web/views/benchmark/benchmark.js`, `web/views/benchmark/suites.js`, `web/css/bench.css`, `web/components/metrics.js` | The `pr` suite runs; the five metric groups are in the table; the comparison draws the bars and the verdict chips; the receipt copies as markdown |
| 5 | `dashboard: draw the Logs tab` | `web/views/logs/logs.js` | The tail follows the run; the level filter narrows it; a finding copies its fix; the download writes a file |
| 6 | `dashboard: answer the new endpoints of the design` | `docs/spec.md`, `internal/onboarding/`, `internal/app/`, `dashboard/internal/api/history.go`, `dashboard/internal/api/smoke.go`, `dashboard/internal/api/logview.go`, `dashboard/internal/api/benchrun.go`, `dashboard/internal/api/*_test.go` | `GET /api/gpus/history?window=15m`, `POST /api/smoke`, `GET /api/runs/{name}/log`, `GET /api/runs/{name}/findings`, `POST /api/bench/run` answer the JSON of §7 |

### 9.3 The owners

Five workers work in parallel. Each owns the files of its row and touches
nothing else. `js/core.js` (`h()`, `put()`, `api()`, `num()`, `bytes()`) and
`js/data.js` (the state, the fetches, the streams) stay with worker 1 and
change only by agreement.

| Worker | Files | The seam |
|---|---|---|
| 1 shell and tokens | `web/index.html`, `web/css/*`, `web/js/shell.js`, `web/js/boot.js`, `web/js/core.js`, `web/js/data.js`, `web/components/command.js` | owns the tokens and the shell; publishes `cmdBlock()`, `copyBtn()`, `cardHead()`, `stateHead()`, `setHead()`, `emptyState()`, `loading()`, `failed()`, `api()`, `openTelemetry()`, `openRunLogs()` |
| 2 Setup | `web/views/setup/*`, `web/components/timeline.js` | publishes `stepTimeline(steps, options)` |
| 3 Dashboard and charts | `web/views/dashboard/*`, `web/components/charts.js` | publishes `spark()`, `areaChart()`, `ring()`, `meter()` |
| 4 Benchmark | `web/views/benchmark/*`, `web/css/bench.css`, `web/components/metrics.js` | publishes `metricTable()`, `verdictChip()`, `barCompare()`, `lengthCurve()` |
| 5 Logs | `web/views/logs/*` | publishes nothing; it uses the components of the other workers |

## 10. The chat tab

The Chat tab ships lean: one prompt box, one answer. It asks the run that
serves and streams the answer back as it comes, and it keeps no history.

```
POST /api/chat  {"prompt": "hi"}
200 text/event-stream: delta {"text": ""}, reasoning {"text": ""},
done {"ttft_ms": 0, "output_tokens": 0, "tok_s": 0}, error {"error": ""};
400 for an empty prompt, a prompt of more than 8192 bytes or a target that
is not a server of this host; 409 when an answer is already running; 503
when nothing serves; 502 when the server does not answer; 401 without the
token; 403 without X-Bernie
```

It proxies the chat API of the running run, to a loopback target only, and
the request to the server ends when the page goes away. The reasoning of
the model comes apart from the answer when the server sends one.

The **Try it** box of the Dashboard run card and of the verify step of
Setup (G3) stays: it sends one prompt and answers with the text, the time
to the first token and the tokens per second. The richer client of the
reference (a transcript, a system prompt, a parameter panel) is not part of
this tab.
