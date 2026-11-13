# Dashboard v3 — the Benchmark tab

Part of the design of the dashboard. The main file is
[../design-dashboard.md](../design-dashboard.md); the components are in
[components.md](components.md).

The question of the tab: **how fast is it, and does it read the whole
prompt?**

Wireframes: `design/benchmark-1280.svg` (a result and a comparison),
`design/benchmark-390.svg` (the same at 390 px),
`design/benchmark-empty-1280.svg` (nothing measured yet).

## The order of the page

1. The state headline and the actions.
2. The section `THE SUITES`: the suite cards.
3. The section `THE RESULT`: the table of the cells and the charts.
4. The section `THE COMPARISON`: the bars, the verdict chips and the
   receipt.
5. The section `THE HISTORY`: the table of the saved results.
6. The section `AN A/B RUN`: the baseline and the candidate.
7. The section `SHOW DETAILS`.

## The widgets

| # | Widget | Data source | Refresh | Empty copy | Error copy |
|---|---|---|---|---|---|
| 1 | The state headline | `GET /api/bench` → the newest result and its best cell | on open and after a run | `NOTHING HAS BEEN MEASURED YET.` and `A benchmark measures the tokens per second of a running server. Run one below and this page fills with its numbers.` | `THE RESULTS COULD NOT BE READ.` plus `Try again` |
| 2 | The suite cards | `GET /api/bench/suites` → the id, the name, the description, the digest and the cells | on open | `There is no suite to run.` and `The built-in suites live in bench/suites/. One of them is the set of cells a pull request must pass.` | `The suites could not be read.` plus `Try again` |
| 3 | The digest of a suite | the same document, `digest` | with the card | the card shows `digest: unknown` | the card shows `digest: unknown` |
| 4 | The primary action `Run the suite` | `POST /api/bench/run` with the suite id (G7) | on demand | the button is disabled when no suite is picked | the button is disabled and reads `Try again` |
| 5 | The live progress of the run | `GET /api/bench/{id}` → `phase`, `done`, `total` and `GET /api/bench/{id}/events` (G7) | the job stream | the progress card is hidden | the progress card says `The run stopped.` and shows the last line |
| 6a | The headline cards | `GET /api/bench` → `results[].summary` | on open and after a run | every card shows `–` | `The results could not be read.` plus `Try again` |
| 6 | The context × in-flight table | `GET /api/bench` → `results[].cells[]` | on open and after a run | `No result yet.` and `Run a suite above and its numbers show here.` | `The results could not be read.` plus `Try again` |
| 7 | The five metric groups of a cell | the cell: `prefill`, `decode`, `ttft_s`, `itl_ms`, `output_tok_s` | with the table | `–` | `–` |
| 8 | The distribution of a metric | the same fields, `p50` and `p95` | with the table | `–` | `–` |
| 8b | The requests behind a distribution | the cell, `completed` | with the table | `–` | `–` |
| 10 | The state and the reasons of a cell | the cell: `skipped`, `failed`, `errors`, `warmup_failed`, `incomplete`, and the flags `recall`, `check`, `size_valid`, `output_valid`, `stream_incremental`, `spread` | with the table | the state is `ok` and there is no `Why` button | the state is `ok` |
| 11 | The comparison chart | `GET /api/bench/compare?a=ID&b=ID` → the cells and the verdicts | on demand | the section says `Pick two results to compare.` | `The two results measured different things.` and the reason |
| 11b | The block of one cell | the same document, one block per cell (group and concurrency) | with the chart | no block | no block |
| 11c | The change in percent | the same document, `delta_pct` | with the chart | `–` | `–` |
| 12 | The verdict chips | the same document, the verdict of every metric, its `band` and the spread of the cell | with the chart | `–` | `–` |
| 13 | The three charts by context | the cells that ran: `input_len`, `concurrency`, `output_tok_s`, `prefill.agg_tok_s`, `ttft_s.p50` | with the result | no chart when no cell ran | no chart |
| 13b | The numbers of a chart | the context × in-flight table under the charts | with the chart | `–` | `–` |
| 14 | The history table | `GET /api/bench` → the saved results | on open and after a run | `No saved result yet.` and `Every benchmark is saved with its workload, so two results can be compared only when they measured the same thing.` | `The history could not be read.` plus `Try again` |
| 15 | The compare-to-baseline picker | the same document | on open | the picker is disabled | the picker is disabled |
| 16 | The receipt | `GET /api/bench/compare?a=ID&b=ID&format=markdown` (G11) | on demand | the button is disabled | the button is disabled |
| 17 | The A/B run | `POST /api/bench/ab` with the two arms (G12) and `GET /api/bench/{id}/events` | on demand | the section says `An A/B run measures one server, then the other, then the first again.` | the section shows the error of the run |
| 18 | The Show details panel | `/api/bench`, `/api/bench/suites`, `/api/bench/compare` | on open | the panel holds the empty answer of the endpoint | the panel holds the error answer |

## The suite cards

One card per suite, a radio card each: a big radio, the name of the suite,
one line that says what it measures, the number of cells, the estimated
time and the digest in mono with a copy button.

The three built-in suites:

| Id | What it measures | Cells |
|---|---|---|
| `pr` | the cells a pull request must pass: 1024 prompt tokens and 512 output tokens at the levels 1, 4 and 8, and 16384 prompt tokens and 1024 output tokens at the same levels | 6 |
| `smoke` | one short cell: it proves that the server answers and says nothing about the speed | 1 |
| `long-context` | a ladder: one cell per prompt length, from 2048 to 16384 tokens, at the level 1 | 5 |

The digest is the sha256 of the canonical form of the suite. Two results of
different suites are never compared, so the digest is shown next to the
result of every run.

## The live progress

While a suite runs, the card under the suite cards holds one sentence and
one bar:

`Cell 2 of 6: 1k at concurrency 4 · 12.4 tok/s · 38 s so far.`

The bar is 10 px high with a 6 px radius, filled in `--key`. The sentence
is the plain text of the phase; the number that is not known shows `–`.

## The table of the cells

One table per result. The head is uppercase and 12 px. Every distribution
shows the median, the 95th percentile and the worst case; the mean stays in
the JSON. The columns:

| Column | The source |
|---|---|
| The group and the lens | `group`, `input_len`, `output_len` |
| The concurrency | `concurrency` and `num_prompts` |
| The prefill, per request | `prefill.req.p50` in tok/s |
| The prefill, per cell | `prefill.agg_tok_s` in tok/s |
| The decode, per request | `decode.req.p50` in tok/s |
| The decode, per cell | `decode.agg_tok_s` in tok/s |
| The time to first token | `ttft_s.p50`, `p95`, `max` in seconds |
| The inter-token latency | `itl_ms.p50`, `p95`, `max` in milliseconds |
| The output rate | `output_tok_s` in tok/s |
| The requests that answered | `completed` |
| The server numbers | `server_side.ttft_s` and `server_side.itl_ms` |
| The flags | the words of the next section |

A number that was not measured shows `–`. Every number that came from the
repetitions carries their spread under it: `±10.9 % · 3 reps`. The spread
appears only when the repetitions disagree (below the steady share of
0.95); above it the number speaks for itself.

The row of the cell that the headline names carries the tag `the headline`,
and the line under the table says which row it is. The headline and the
lede name that cell, so every number they carry can be found in the table.

The line above the table says how the prefill and the decode numbers are
measured, in one sentence: `The prefill rate counts the queueing of the
cell; the decode rate counts the time in which at least one request was
decoding.`

## The flags, in words

The flags of a cell are words, never colours alone:

| Field | The word | The meaning |
|---|---|---|
| `recall.passed` is false | `the reply missed the hidden fact` | the speed numbers of this cell mean nothing |
| `check.passed` is false | `the declared answer was wrong` | the cell asserts an answer, and it failed |
| `size_valid` is false | `the prompts were not the size the cell asked for` | the drift is in `input_size_percent` |
| `output_valid` is false | `the replies look like noise` | fewer than 8 % of the words are different |
| `stream_incremental` is false | `the server buffered the whole answer` | there is no decode window |
| `incomplete` is true | `a request was lost` | the numbers stay, no verdict uses them |
| `spread` below 0.95 | `the repetitions disagree` | the cell gets no verdict |

## The comparison

The section compares the two runs cell by cell: one block per cell, named
`1k at concurrency 4`, and inside it the five metric groups of spec §10.1.
No metric is ever taken from two different cells.

Every bar of every block is drawn on one scale: the bars are indexed to the
baseline, the baseline is 100 and the candidate is the baseline times the
ratio. The key says where the scale ends: `indexed: the baseline is 100,
the scale ends at 225`. A change of 5 % and a change of 50 % do not look
the same, and two cells can be read against each other.

Every row carries the two numbers (`184.9 → 188.6 tok/s`), the change in
percent (`+2.0 %`) and the verdict of the metric. The two arms are named on
the marks themselves (`baseline`, `candidate`), not only in the key.

| Verdict | The chip |
|---|---|
| better | `▲ better, band 2.0 %` |
| the same | `– same, band 2.0 %` |
| worse | `▼ worse, band 2.0 %` |
| noisy | `! noisy, band 10.9 %, spread ±10.0 %` |
| no verdict | `the repetitions disagree, so this metric has no verdict` |

The band is the share of a change that counts as noise; the backend widens
the band of the suite with the spread the two arms measured, so a
repetition that differs by 10 % makes a change of 10 % noise. The spread
is printed only when the repetitions disagree.

The section refuses a comparison that does not hold: `These two runs
measured different things.`, and it names the difference: the suite, the
digest or the workload. A comparison that could not decide keeps its
chart and names the reason above it.

Under the chart the same numbers are in a table, in a `details` element:
`The numbers of the comparison`.

The receipt is a ghost button: `Copy the receipt`. It copies the markdown
of `bernie bench compare A B --markdown` (G11), which holds the scope of the
two runs, their correctness, the numbers with the verdict of every metric
and the command that reproduces the run.

## The A/B run

The section holds two fields, `base` and `cand`, each a recipe id or the
URL of a server, and one primary button `Run the A/B`. The runs visit in
the order base, cand, cand, base, so a drift of the machine hits both arms
the same way. While it runs, the section shows the same progress sentence
as a suite run. When it ends, it holds the same comparison and the same
receipt.

## The copy of the tab

| Place | The text |
|---|---|
| The headline after a run | `THE LAST BENCHMARK REACHED 56.1 TOK/S.` |
| The lede after a run | `The pr suite measured 6 cells on 4 cards. In the cell 1k at concurrency 8 the prefill was 2 310 tok/s and the decode 56.1 tok/s.` |
| The line under the table | `The headline is the cell 1k at concurrency 8.` |
| The tag of that row | `the headline` |
| The button | `Run the suite` |
| The section `THE COMPARISON` with nothing picked | `Pick two results to compare.` |
| The section `THE HISTORY` with nothing saved | `Every benchmark is saved with its workload, so two results can be compared only when they measured the same thing.` |

## What the built tab does differently from the wireframes

The wireframes draw the layout and the information design; the tab that
ships answers the same questions with more of the numbers and fewer of the
charts. The differences:

| The wireframe | The tab | Why |
|---|---|---|
| The lede names the prefill and the decode of the suite | the lede names the cell they belong to, and the row of that cell carries the tag `the headline` | a number that cannot be found in the table is a number the reader cannot trust |
| The table shows one number per latency | the table shows the median, the 95th percentile and the worst case, plus the number of requests that answered | the mean of a latency distribution describes no request; the tail is what a reader feels |
| The table has no column for the requests | the column `REQUESTS` says how many answered | a percentile without its population is not a number |
| The table shows no spread | every number that came from the repetitions carries their spread under it | a gain inside the noise is not a gain |
| The comparison is one cluster per metric group | the comparison is one block per cell, and the bars of every block are indexed to the baseline on one scale | comparing a metric of one cell with a metric of another cell measures the cells, not the runs |
| The comparison chip holds the word | the chip holds the word, the band it used and the spread | a verdict without its band cannot be judged |
| The length curve has two axes and two lines | three charts side by side, one measure each, the contexts along x | two axes with their own maxima invite a correlation the data does not have |
| The charts have no text form | the table under the charts holds every number of every chart | the numbers have to exist as text |
| A failed or skipped cell shows only a flag | the cell has a state word and a `Why` button that opens its reasons | a reader who sees `failed` asks why; the answer sits next to it |
