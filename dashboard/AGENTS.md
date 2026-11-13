# AGENTS.md (dashboard)

Rules for AI agents that change `dashboard/`. The repo rules in
[../AGENTS.md](../AGENTS.md) also apply. [README.md](README.md) holds the
layout of the folder, the file table and the recipes (add a tab, add a
widget). This file is the rule book: every rule lives here once.

The long reference behind the rules — the sources, the per-rule detail and
the ranked changes — is in [../docs/design/rules-ui.md](../docs/design/rules-ui.md).

## What this folder is

A Go package (`package dashboard`) plus a static web UI in `web/`. It is a
view of the CLI. It adds no logic.

## How a rule is checked

Every rule carries its check. The tags:

- **[lint]** — `scripts/check-web.sh` fails the build through `make check` and
  in CI. The Go asset tests (`dashboard/webassets_test.go` and
  `dashboard/assets_test.go`) fail the build too.
- **[budget]** — `TestPayloadBudget` in `dashboard/assets_test.go` fails the
  build.
- **[layers]** — `make layers` (`scripts/check-layers.sh`) fails the build.
- **[test]** — an existing Go test in `dashboard/` covers it.
- **[review]** — checked by a person in the browser check of *Check your
  work*.

## Hard rules

1. **No new logic here.** A value comes from a domain package. If the data does
   not exist, add it to the CLI and its `--json` first. Then expose it here.
2. **Spec first.** A new or changed endpoint is written in spec §11 before the
   code. The UI follows the endpoint, not the other way round.
3. **No dependencies.** No npm, no bundler, no framework, no CDN. The UI is
   plain HTML, CSS and JavaScript embedded with `go:embed`. Fonts are
   self-hosted woff2 in `web/css/` (≤ 4 files, ≤ 130 KB, `font-display:swap`,
   `unicode-range` subset); there is no other asset.
4. **No `innerHTML`.** Build DOM with `h()` in `web/js/core.js`. Server data is
   text. **[lint]**
5. **Writes are guarded.** Every POST and DELETE keeps the `X-Bernie` header
   check and the token rules in `internal/auth`. Never loosen them. **[test]**
6. **Redact.** Anything sent to the browser goes through `internal/support/redact`:
   `httpjson.Write` is the one writer of the JSON answers and the SSE writer
   redacts every line it sends. → Check: **[test]** `TestEveryRouteRedacts` in
   `dashboard/internal/api/redact_test.go` walks every route of the router and
   finds no secret in the answer.
7. **One question per tab.** Do not add a tab for a feature that fits
   an existing one. Do not put two questions in one tab. The Advanced tab is
   the one exception: it is the operator's console, the things about the host
   that belong to no single tab (the full per-GPU table, the power write, the
   doctor report, the jobs, the raw documents).
8. **The visual design follows [../docs/design-dashboard.md](../docs/design-dashboard.md).**
   That page is the design contract. The colours live in `web/css/tokens.css`
   and nowhere else: a component file never writes a colour by hand.
9. **Same names as the CLI.** A field on the page has the JSON name of the
   CLI output.

## Structure rules

These rules replace the old "a directory has at most N entries" cap. A count
is not a reason; a role is.

1. **One responsibility per package.** A package says in one sentence what it
   owns, at the top of its `doc.go` or its main file.
2. **A small public surface.** The exported names of a package are listed in
   [../docs/packages.md](../docs/packages.md). Keep the list short.
3. **Group by role, not by count.** A directory holds the files of one role.
   When a directory grows past about 15 files, or holds two roles that can be
   told apart, split it into sub-packages or sub-directories by that role.
4. **Readable files.** A file stays under about 400 lines. Split it by
   concept (one domain, one card), never by cutting a function in half.
5. **Tests live next to the code.** A test file sits in the package it
   tests. Fixtures live in `testdata/`.
6. **No cycles, no upward imports.** The layers are in
   [../docs/architecture.md](../docs/architecture.md): the dashboard may use
   `internal/app`, `internal/flow` and the domain packages, never the other
   way round. **[layers]**
7. **The web tree is grouped by role.** `web/css/` holds the stylesheets,
   `web/js/` the core scripts (DOM, shell, data, router), `web/components/`
   the shared widgets and `web/views/<tab>/` the per-tab code. One file per
   concern.

## UI and UX rules

The goal: a person who has never used vLLM understands the page in one
minute. An agent can read the same page as text.

1. **One question per tab.** If a tab needs two answers, split it. → Check:
   the tab table in [README.md](README.md) has one question per row.
2. **Say the state in words first.** A badge (`ok`, `warn`, `fail`, `skip`)
   has a word. Colour never carries the meaning alone. → Check: every status
   element in the DOM has a text node.
3. **Every problem has a fix next to it.** Show the cause and the command to
   run. A copy button sits on every command. Never show a bare error code.
   → Check: a failing check on the page shows its fix command.
4. **The next step is always visible.** The Setup tab shows one next command
   for the current state (no GPU, no recipe fits, ready to serve, starting,
   running). Every tab has at most one primary button; the rest are ghost
   buttons. → Check: one `.primary` per tab.
5. **Numbers have units and a range.** `18.2 / 29.4 GiB`, not `18.2`. A
   missing value shows `–`, never `0` or `NaN`. → Check: no bare number
   without a unit in the DOM.
6. **Show what will happen before it happens.** A start button shows the plan
   (command, GPUs, port) first. It does not start on the first click. → Check:
   the start control shows the plan before it starts anything.
7. **Slow work shows progress.** Server start and downloads can take minutes.
   Show one plain sentence ("Loading weights, 4 of 8 GPUs ready") and the log.
   Do not show a spinner alone. → Check: a running job shows a sentence.
8. **Empty states teach.** No runs? Say what a run is and show the command to
   start one. → Check: an empty panel has a sentence and a command.
9. **Same data as the CLI.** A value on the page also exists in `--json`
   output with the same name. If it does not, add it to the CLI first. → Check:
   the field name matches the `--json` document.
10. **Fast and small.** First paint under 1 s on a local host. No images
    except icons inlined as text, no dependencies. → Check: the budget table
    below. **[budget]**
11. **Works with a keyboard and a screen reader.** Real `<a>`, `<button>`,
    `role=tab`, visible focus, `aria-live` for updates. → Check: the whole
    tab is reachable with the keyboard alone.
12. **Light and dark.** The rail holds the three choices of the theme, in
    words: `Auto`, `Dark` and `Light`. `Auto` is the default and follows the
    system (`prefers-color-scheme`), live; the other two are the two
    palettes of the reference. The choice is remembered and the palette is
    set before the first paint, so the page never flashes the other one.
    Contrast meets WCAG AA. → Check: the palette table in
    [README.md](README.md); every pair is ≥ 4.5:1 for text and ≥ 3:1 for
    the edges of the controls; the control is announced (`aria-pressed` and a
    live region).
13. **Every tab opens with a headline.** The first thing on a tab says in one
    sentence what the situation is. The line under it says what that means
    and what to do next. The headline is read out when it changes. → Check:
    each panel starts with `.state-head`, and the headline carries
    `aria-live="polite"`.
14. **The colours come from the tokens.** A new rule uses the `--` variables
    of `web/css/tokens.css`. No colour is written by hand in a component
    file. → Check: **[lint]** no colour literal outside `web/css/tokens.css`.
15. **One label, one pill, one empty state.** The section label, the state
    pill and the empty state live in `components/cards.js`
    (`sectionLabel()`, `statePill()`, `emptyState()`) and every tab uses
    them. A section label is uppercase in the DOM, so the words a screen
    reader reads are the words the reader sees. → Check: **[lint]** no
    `h2.section-label` outside `components/cards.js`; every `emptyState()`
    call names what, next and the command.

## Setup and onboarding rules

The Setup tab answers one question: what do I do to make this box serve?
The path is the 8 steps of [spec §3.3](../docs/spec.md#33-the-onboarding-checklist)
plus the 3 optional ones.

1. **One step is actionable at a time.** Only the first runnable step of the
   path carries the filled `Run`; a later runnable step keeps a ghost
   control that names what it waits for. → Check: at most one filled `Run`
   in the path.
2. **The primary button names the step it starts.** "Build the Python
   environment", not "Set up this machine"; a button that starts several
   steps says how many. → Check: no button on this tab reads only "Next",
   "Continue", "Set up" or "Run".
3. **The headline lede names one next action and says who does it.** "Copy
   the group command, run it as root in a terminal, then press Re-check."
   The lede does not list everything that is missing. → Check: the lede has
   one action and one actor.
4. **A step that needs root is a command, a copy button and a re-check.**
   bernie never runs a root command; the row says what the command changes
   and ends with a `Re-check` control. → Check: every root row has the exact
   command, a copy button and a re-check control.
5. **Progress is honest.** A percentage appears only when the work is
   measurable; a step with an unknown fraction shows an indeterminate bar
   with the phase sentence and no `aria-valuenow`. → Check: no bar reads
   `0 %` for work that is merely unmeasured.
6. **Every long step carries an estimate, and the estimate is corrected out
   loud.** The path states its rough time and size before the first run; an
   ETA appears only when the backend knows it. → Check: a long step shows
   time and size before it is started, or says that they are unknown.
7. **A step survives a reload.** The checklist carries the id of the running
   job; on load the tab re-opens the event stream and never offers `Run` for
   a job that is already running. → Check: a reload during a job shows the
   same job and its log. (Needs the `job_id` field: spec first.)
8. **Work continues in the background and its end is persistent.** A step
   that finishes while the reader is elsewhere leaves a banner on the tab; a
   toast that vanishes after a few seconds is not the completion notice.
   → Check: the finished step stays visible until the next re-check.
9. **A failed step keeps its cause, its fix and its retry.** The reason sits
   in the row, the fix is a copyable command, the retry re-runs that step
   alone, and the copy never blames the user. → Check: no "invalid",
   "illegal", "wrong", "incorrect" in the user-visible copy of this tab.
10. **Every step states what it changes before it runs.** The target
    directory and the size of the download are in the row before the first
    click; a step that is safe to repeat says that re-running it only
    re-checks. → Check: the row names the path it writes.
11. **Empty states teach, and the first run explains itself once.** An empty
    panel says what the thing is, gives one action and shows the command;
    the first-run explainer disappears once its step is done. → Check: an
    empty panel has what/next/command.
12. **The page is a view of the CLI.** Every command on this tab is the
    exact string the CLI prints and copies character for character, and
    `bernie status --json` shows the same steps, states and commands as the
    page. → Check: the command in the row is pasteable into a terminal and
    equals the CLI output.

## Data display rules

The Dashboard and Benchmark tabs show numbers. The rules are about scale,
age and meaning.

1. **The headline answers "is it OK?" first.** One sentence about the state,
   then one sentence that names the worst thing and what to do; cause-level
   numbers (temperature, ECC, PCIe class) stay in the cards. → Check: the
   lede names the worst card and its limit, or names nothing.
2. **A state is a word, never a colour.** A pill, a chip or a caption
   carries an icon and a word; a colour may mark a state only when the
   threshold comes from the machine (its temperature limit, its power cap,
   its VRAM size). → Check: no rule changes only the colour of a status
   element.
3. **A missing number is `–`, and a stale number says how old it is.** Every
   value from the telemetry stream shows the age of the newest sample; when
   the stream stops, the tab says so and stops writing "now" on the axis.
   → Check: the chart head carries the age of the newest sample.
4. **A sparkline is a line, not a block.** No fill, no frame, no baseline
   rule; its scale is fixed (0 … the limit of the measure), never the
   maximum of the data, and the number beside it gives the value. → Check: a
   flat series draws as a line in the middle of the box, and the tile says
   its range ("of 100 %").
5. **A value against a limit is a bar, not an arc.** The GPU load is a
   0–100 % bar, so ten cards can be compared by eye. → Check: no card and no
   chart draws an arc; the load of a card is a bar.
6. **Axes start at zero unless zero is meaningless, and then the caption
   says so.** Every axis carries its unit; the unit is never only on the top
   tick; bars of one chart share one scale, and a chart of two units does not
   use two y-axes. → Check: no second y-axis; a truncated axis says where it
   starts.
7. **A gap in the data is a gap.** The line breaks where the samples stop;
   nothing is interpolated across a gap and a missing point is never drawn
   as a zero. → Check: stopping the sampler leaves a break in the line.
8. **A chart names its window and its resolution.** "Last 15 minutes, one
   point per 10 s, peak kept." A downsampled series keeps its peaks. → Check:
   the chart head names the window and the resolution.
9. **Two series that must be told apart are blue and orange, never two
   blues.** A chart uses at most four categorical colours and they differ in
   lightness as well as hue. `--series-5` and `--series-6` exist, but they
   may not be the only distinction between two categories: a second channel
   (a label, a dash, a shape) always separates them. → Check: no chart uses
   two blues, no chart puts green next to orange, and every pair of series
   carries a second channel.
10. **A chart has a text form.** The same numbers exist in a tile, a table
    row or a sentence; the chart is reachable with the keyboard and the
    arrows move its readout. → Check: every chart's numbers are in the DOM
    as text.
11. **A table is for exact values, a chart is for the relation.** Every
    chart on a tab must say something the table does not, or it is deleted;
    numbers in a column are right-aligned, share their digit count and
    carry their unit in the header. → Check: no chart repeats a table.
12. **A distribution is shown as a distribution.** Latency columns show the
    median, the 95th percentile and the worst case with the number of
    requests behind them; the mean stays in the JSON. A number from
    repetitions carries its spread, and a verdict states the band it used.
    → Check: a cell with `spread > 1.05` shows the spread next to the
    number.
13. **Every state is worded.** Loading, empty, error and stale each have
    their own sentence; the error names the endpoint and the last value that
    was read. A number in the headline or the lede is findable in the table
    below it, with its cell named. → Check: the headline number appears in
    the table with its cell named.

## Front-end rules

The page is plain HTML, CSS and JavaScript. These rules keep it small,
honest and reachable.

1. **No inline styles.** The page is served with `style-src 'self'`; the
   browser drops every `style` attribute. A static style is a class in
   `web/css/`; a runtime value is written through the CSSOM
   (`el.style.width = "50%"`). → Check: **[lint]**
   `TestNoInlineStylesOrHTML` in `dashboard/assets_test.go` finds no `style:`
   attribute in `web/**/*.js`, and every chart draws.
2. **No inline scripts and no `on*` attributes.** Every handler is added
   with `addEventListener` in the file that owns the element. → Check:
   **[lint]** no `on*` attribute in the DOM.
3. **One namespace.** Every file in `web/` is a classic script loaded in the
   order of `web/index.html` and may only add names to `window.Bernie`.
   The table in [README.md](README.md) lists the top-level names that the
   files publish. → Check: **[lint]** `scripts/check-web.sh` finds no name
   published by two files and compares the tree with the table.
4. **One data path.** Only `web/js/data.js` opens a `fetch`, an `EventSource`
   or a timer. A view never reads the clock, the URL or the network.
   → Check: **[lint]** `scripts/check-web.sh` finds no `fetch`, `EventSource`
   or timer outside `web/js/data.js`, apart from the documented exceptions in
   `scripts/web-allowlist.txt`.
5. **One frame.** All the repaints caused by one sample happen in a single
   `requestAnimationFrame` callback. A page that is never painted (a
   background tab, a browser without a compositor) runs the same batch from
   the one timer of `web/js/data.js` instead, so the numbers still reach the
   reader. → Check: a view opens no timer of its own.
6. **Pause when hidden.** While `document.hidden` is true the poller stops
   and no repaint runs; on `visibilitychange` back to `visible` the page
   refreshes once, at once. → Check: a hidden tab sends no request.
7. **Charts stay cheap.** At most 240 points are drawn per chart (the rest
   are bucketed); an update writes the `d` attribute of the path and never
   rebuilds the axes; every chart has a table of the same numbers. → Check:
   a chart with 900 points draws 240.
8. **Assets are cacheable, data is not.** Static assets are served gzipped,
   with an `ETag` and `Cache-Control: no-cache` (revalidate → 304); `/api`,
   the HTML and the event streams keep `no-store`. → Check: a reload of a
   static asset answers 304. (Needs the spec §11 change first.)
9. **Writes stay guarded.** Every write carries `X-Bernie: 1` and is refused
   when `Sec-Fetch-Site` is `cross-site` or when `Origin` is present and is
   not the dashboard's own. The token is never in a URL except `/?token=`,
   never in a log line. → Check: **[test]** a write without the header, with
   a foreign `Origin` or with `Sec-Fetch-Site: cross-site` fails.
10. **Keyboard and targets.** A shortcut made only of letters must be
    remappable, disable-able, or active only while its control has focus
    (WCAG 2.1.4). In the tab bar only Left/Right/Home/End are handled. Every
    pointer target is at least 24×24 CSS px; the main action of a card is
    44×44. → Check: no bare letter shortcut works from anywhere, and every
    target measures ≥ 24×24.
11. **Live regions speak only about state.** One live region per tab carries
    the headline; a region that changes on every sample is throttled to
    state changes. Moving numbers never live in a live region. → Check: a
    1 Hz number is not in a live region.
12. **Focus is never lost.** A route change moves the focus to the panel
    heading; a disclosure returns it to its trigger; Escape closes a tooltip
    and returns the focus; a panel with no focusable child gets
    `tabindex="0"`. → Check: the focus is visible after every route change.
13. **Nothing fails silently.** Every fetch and every stream has one failure
    path: the page says when the numbers were last updated, marks them as
    stale and offers "Try again". → Check: stopping the API shows the age
    of the last value and a "Try again" control.
14. **A file owns one concern and stays under 400 lines.** A component
    exports one function that returns `{el, update}`; it touches only its
    own element and reads no global except `window.Bernie`. → Check:
    **[lint]** `scripts/check-web.sh` finds no file in `web/` over 400 lines
    unless `scripts/web-allowlist.txt` lists it; a listed file may not pass
    1024 lines (the owner's cap, for the chart module).
15. **One question per tab, one data path, one file per concern.** These are
    the front-end form of the structure rules above. → Check: the tab table
    in [README.md](README.md) and the file tree beside it.

## Budget

The numbers are the contract. "Now" is what the files weigh today; a change
that breaks a number changes the target first (design §8.3) and never the
number silently.

Two numbers matter: the **first paint** is what a reader waits for, and the
**whole tree** is what the binary carries. The first paint holds the shell
and the Setup tab; the files of the other tabs come when their tab opens
for the first time (the manifest of `web/js/boot.js`).

| Budget | Target | Now | Check |
|---|---|---|---|
| HTML + CSS + JS, uncompressed (the whole tree) | ≤ 456 KB | 448.8 KB | **[budget]** |
| HTML file | ≤ 12 KB | 6.9 KB | **[budget]** |
| CSS total | ≤ 84 KB | 80.7 KB | **[budget]** |
| JavaScript total (the nine tabs) | ≤ 368 KB | 361.2 KB | **[budget]** |
| JavaScript on the first paint | ≤ 112 KB | 103.6 KB | **[budget]** |
| CSS on the first paint | ≤ 40 KB | 38.6 KB | **[budget]** |
| Fonts | ≤ 130 KB, ≤ 4 files, woff2, `font-display:swap`, `unicode-range` | 128.4 KB, 4 files, swap, range ✓ | **[budget]** |
| Requests on the first paint | ≤ 24 | 22 | **[budget]** |
| Transferred text on the first paint (gzip) | ≤ 60 KB | 48.0 KB | **[budget]** |
| Text of the whole tree (gzip) | ≤ 161 KB | 157.1 KB | **[budget]** |
| Points drawn per chart | ≤ 240 | unbounded | **[review]** |
| Pollers | 1 | 1 poller + 1 extra 1 Hz timer | **[review]** |
| Live streams | 1 SSE at 1 Hz | 1 SSE | **[review]** |
| Pointer target | ≥ 24×24 px (SC 2.5.8 AA) | 18×18 (info button), ≈11 px (JSON link), 20×20 (radio) | **[review]** |
| CSP | strict, no `'unsafe-inline'` | strict, but inline styles are still used | **[lint]** |

The JavaScript of the page is plain, hand-written and unminified: nine tabs
of it are 265.8 KB. It is not one file that the reader downloads, it is
265.8 KB of which 101.4 KB arrive with the first paint. The line that
matters is the first paint: it is the one the reader waits for. The whole
tree is the line that keeps the binary and the review honest.

The Core Web Vitals targets (LCP ≤ 2.5 s, INP ≤ 200 ms, CLS ≤ 0.1, TBT
≤ 200 ms, no task > 50 ms) are not measured yet; the dev-only browser
harness measures them.

## Where to change what

| Task | Files |
|---|---|
| New endpoint | spec §11, `internal/api/<domain>.go`, `internal/api/api.go` (the route), a test |
| New view or card | `web/views/<tab>/<file>.js` (one `view*` function), `web/components/` |
| New tab | `web/index.html` tab link and section, `web/views/<tab>/`, the `views` map in `web/js/boot.js`, this README table |
| New live value | `internal/telemetry/sampler.go`, the sample type in `gpu`/`runner`, spec §11 |
| Change colours or layout | `web/css/tokens.css` for colours, `web/css/layout.css` for the shell |
| Auth change | `internal/auth/auth.go`, spec §11, a test that shows the old attack still fails |
| A rule change | this file, one home per rule, with its check |

## Check your work

```bash
make check                          # gofmt, vet, all tests, layer check
go test ./dashboard/...             # API, auth, streams
bernie dash --listen 127.0.0.1:7860
```

For a UI change, open each tab in a browser. Check: light and dark, a
narrow window (390 px), empty state (no GPUs, no runs), error state (stop the
API), keyboard only, and the acceptance check of every rule you touched.
Say in your report which tabs you opened and what you saw. Passing tests
alone is not proof for UI work.
