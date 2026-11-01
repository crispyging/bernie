# Roadmap

Status: draft 2 (2026-10-28).

Rule: bernie stays lean and simple, and keeps what really helps users. A
feature earns a place if it gets a new user from a bare machine to a working
server and a connected client, or keeps results honest. One command does the
job. No wizard, no new product surface.

Each item has a spec section and one test that proves it.

## Next: make `up` work the first time

| # | Item | Spec | Test |
|---|---|---|---|
| 6 | **Next command in every state.** `status --json` has `next` for: no GPU, no recipe fits, recipe fits, run starting, run ready. | §3.1 | Fixture set covers the five states. |

## Then: honest results and connected clients

| # | Item | Spec | Test |
|---|---|---|---|
| 7 | **Model file hashes.** A result stores the hashes of the model files it measured, so a number can be tied to the exact weights. | §10 | A result of a run holds the hashes of the model files. |
| 8 | **Redacted report.** `doctor --report` writes JSON with no hostname, user name, home paths or keys, and lists what it removed. | §5 | `grep` for user, host and `/home/` finds nothing. |
| 9 | **Smoke chat.** `bernie chat --run NAME "prompt"` sends one prompt, prints the reply, time to first token and tok/s. It proves the server works. | §3 | Prints reply, TTFT and tok/s for a fixture server; exits non-zero when the run is not ready. |
| 10 | **`connect <app>`.** Prints the ready-to-paste config for opencode, Open WebUI or a generic OpenAI client, using the run's port and model name. It prints only; it writes nothing. | §3 | `connect opencode` output contains the run's base URL and served model name. |

## Shipped

- **Log doctor on failure.** When a run exits during startup, `serve` and
  `up` scan its log and print every matched rule: id, cause, fix, doc link
  and the line. `--json` carries them in `log_findings`. Spec
  [§5.3](spec.md#53-log-doctor).
- **Ready means answers.** A run is `running` only after `/health`
  answers and the server answered a one-token completion. The state name
  stays `running`, since `ps --json` is an API. A run that is still
  `starting` says why in `waiting`, and `up` and `serve --wait` fail with
  that reason at their limit. Spec [§8.3](spec.md#83-run-record).
- **Plan shrinks to fit.** `recipe plan` lowers `max-model-len` to 32K,
  16K or 8K when one request does not fit the KV budget, and records the
  change in `plan.adjustments[]` with its reason. `serve`, `compose` and
  `up` print it. An override of `max-model-len` is never changed; the
  plan warns when it does not fit. Spec
  [§7.10](spec.md#710-plan-adjustments).
- **Fit in recipe data.** A recipe may list `requires.download_gb` and a
  measured `requires.kv_bytes_per_token` next to `vram_gib` and `ram_gib`.
  `recipe list` gives every recipe `fits`, `tight: <reason>` or
  `no: <reason>` in `fit.status` (the field `--json` already had), and
  `fit.reasons` is empty only when the recipe fits. A recipe is tight when
  the download does not fit the free disk of the model cache, or when one
  request of `max-model-len` needs more KV cache than the budget. `up`
  picks a recipe that fits before one that is tight. Spec
  [§7.8](spec.md#78-fit).
- **Bench suites.** A suite is data: `bench/suites/*.yaml` holds the cells,
  the repetitions and the thresholds of a run. The loader is strict, and
  the digest of the suite pins the cells of a result. `bernie bench
  suites` lists them, `bernie bench --suite pr` runs the fixed pull
  request cells, and every cell keeps its group, its lens, its repetitions
  and the spread of its metrics. Spec
  [§10.2](spec.md#102-suites).
- **Repeated cells.** Every cell of a suite runs several times. The numbers
  of the cell are the medians over the repetitions, the repetitions
  themselves are in the result, and the spread says how well they agree. A
  cell that lost a request is `incomplete`: its numbers stay, but no
  verdict uses them. The result records the power cap and the clocks of
  the cards. Spec [§10.2](spec.md#102-suites).
- **Bench verdict.** `bernie bench compare` says, for every metric of every
  cell, whether the second run is better, the same or worse. The noise
  band comes from the suite and from the spread of the repetitions, and an
  arm whose repetitions disagree gets no verdict. A failed recall or an
  incomplete cell makes the whole comparison invalid. Spec
  [§10.3](spec.md#103-the-verdict).
- **Bench A/B run.** `bernie bench ab` runs a baseline and a candidate in
  the order base, cand, cand, base, so a drift of the machine hits both
  arms the same way. One server runs at a time, and every visit saves its
  own result. Spec [§10.4](spec.md#104-the-ab-run).
- **Measurement gates.** Every cell says whether its prompt was the size it
  asked for, whether its replies look like text and whether its stream
  arrived in more than one chunk. A cell that is off size or degenerate
  keeps its numbers, but they are labelled. The recall gate of a suite
  makes a comparison invalid when the server did not repeat the hidden
  fact. Spec [§10](spec.md#10-benchmark).
- **ECC check.** The doctor check `gpu.ecc.<hip index>` reads the RAS
  counters of every card: `ok` when ECC is on and the card counted no
  memory error, `warn` when ECC is off on a card that has it or when the
  card corrected errors, `fail` when the card counted uncorrectable ones.
  The counters come from sysfs, `amd-smi` is the fallback. Spec
  [§5.2](spec.md#52-required-checks).
- **Junction and memory temperature, clocks.** The telemetry sample carries
  the temperature of the memory junction, the temperature of the memory and
  the shader and memory clocks of every card. The GPUs tab shows them with
  their units and the word `hot` when a card reaches the limit of its
  device table. Spec [§11](spec.md#11-dashboard).
- **Progress words.** One function turns the tail of a setup or serve log into one
  sentence ("Loading the model weights, 2 of 5 shards"). `serve` and `up`
  print it when it changes, `ps --json` and `status --json` carry the phase
  of a live run, and the Logs tab shows the sentence under the state badge.
  Spec [§5.4](spec.md#54-log-phase).
- **P2P readiness checklist.** `bernie topo --check` and the doctor category
  `topo.p2p` answer whether multi-GPU peer-to-peer copies and the custom
  all-reduce work on this host, and say why when they do not. The Topology
  tab shows the same checklist. Spec
  [§4.5](spec.md#45-p2p-readiness-checklist).
- **MCP server.** `bernie mcp` on stdio and `POST /mcp` on the dashboard,
  read-only by default, write tools behind `--allow-write` and a confirm
  token. It reuses the `--json` code paths. Spec [§13](spec.md#13-model-context-protocol),
  how-to [mcp.md](mcp.md).
- **Pass key recall.** `bernie bench --recall` hides a short random fact in
  every prompt and checks the reply for it. A cell that misses the fact has
  broken attention, and its speed numbers mean nothing. Spec
  [§10](spec.md#10-benchmark).
- **Bench workload.** A result stores its `workload` (`protocol_id`,
  `corpus_sha256`, input, output, concurrency, recall) and `bernie bench
  compare` refuses two results of different workloads. Spec
  [§10](spec.md#10-benchmark).
- **Bench metrics.** Every cell reports five groups of numbers: the prefill
  and the decode speed (the rate of the whole cell and the distribution of
  the rates of its requests), the time to first token in seconds, the
  inter-token latency in milliseconds and the output rate. The server's own
  numbers from `/metrics` come next to them in `server_side`. The text, the
  dashboard and the MCP tools show the same numbers with their units.
  Spec [§10](spec.md#101-what-the-numbers-mean).
- **Numeric GPU group ids.** A container run adds the numeric ids of the
  GPU groups of the host, never their names: the image has no /etc/group to
  resolve a name in. The plan shows the ids and where they came from. Spec
  [§8.2](spec.md#82-container-mode).
- **Resumable model download.** The model step fetches the files of a
  Hugging Face repo into the cache itself: a partial file continues with a
  `Range` request, the checksum of every file is checked, and the progress
  is one line with a speed. Spec [§6.1](spec.md#61-the-model-download).
- **Power cap service.** `tune power set` writes the desired caps to
  `<config>/power.yaml`, `tune power apply` writes them to hwmon as root, and
  `tune power service` prints the systemd unit pair that applies the file at
  boot and on every change. The GPUs tab shows the cap, the allowed range,
  the desired value and the drift, and writes a new desired value. Spec
  [§9.2](spec.md#92-power).
- **Copy buttons and raw JSON links.** Every command the dashboard shows has
  a copy button that copies the command as it is written, and every card
  header carries a `json` link to the raw document behind it. Spec
  [§11](spec.md#11-dashboard).
- **Bench receipt.** `bernie bench compare <a> <b> --markdown` prints the
  comparison as a receipt: the scope of the two runs, their correctness,
  the numbers with the verdict of every metric, and the command that
  reproduces it. The pull request template has a Receipt block, and
  CONTRIBUTING.md says how to fill it with `bernie bench --suite pr` and
  `bernie bench ab`. Spec [§10.3](spec.md#103-the-verdict).
- **Bench suites in the dashboard.** The Bench tab runs a suite, shows the
  five metric groups of every cell, compares a result with a saved one and
  draws the context-length curve of a ladder suite as plain SVG. Spec
  [§11](spec.md#11-dashboard).
- **Check groups.** A suite group of kind `check` asserts a declared answer
  instead of a speed. Its verdict lands in the cell field `check`, separate
  from the suite-wide recall gate. Spec [§10.2](spec.md#102-suites).
- **Long-context ladder.** The built-in `long-context` suite measures one
  cell per prompt length, from 2048 to 16384 tokens. It shows how the
  prefill and the time to first token grow with the prompt, and the
  dashboard draws it as the length curve. Spec
  [§10.2](spec.md#102-suites).

- **Dashboard structure.** The dashboard is one server package plus the
  internal packages `api`, `auth`, `telemetry`, `sse` and `httpjson`. The web
  tree is grouped by role: `web/css`, `web/js`, `web/components` and
  `web/views/<tab>` for the four tabs Setup, Dashboard, Benchmark and Logs.
  The old hash names still open their new tab.

## The next dashboard

The design of the new dashboard is written down: the shell, the tokens, the
eight tabs, the copy of every state and the wireframes are in
[design-dashboard.md](design-dashboard.md) and in [design/](design/).

The shell has shipped: the eight tabs of design §2.2 open from their hash
routes, the old routes land on their new tab, every tab has one primary
action, and no tab scrolls sideways at 390 px (checked 2026-10-31 in the
demo, light and dark, 1280 and 390 px).

The design of the dashboard and parts of its markup and CSS come from
[CrispyGinger's vllm-rdna-dash](https://github.com/crispyging/vllm-rdna-dash),
under its Apache-2.0 licence. Each adapted file says so in its header.

## Not planned

These add surface without helping first-run success. Reopen only with a
concrete user request.

- Dashboard chat page. The `chat` command covers the check. The page is
  deferred, not cancelled: its sketch is in
  [design-dashboard.md](design-dashboard.md) §10, and the Dashboard tab
  carries a one-prompt `Try it` box meanwhile.
- `connect --write` and self-check. Print-only first.
- Demo mode for the dashboard.
- "Did you mean" suggestions beyond what the flag parser prints.
- A separate privacy page. The spec's "files and paths" section covers it.
- Download byte counts in plans. Progress words cover the wait.
- Per-recipe `sources` field.

## What bernie keeps as its own

- It runs vLLM, with tensor-parallel across cards, topology and P2P tools.
- Recipes are plain YAML that anyone can add.
- CLI first. The dashboard is a view of the same data.
- It never becomes an inference engine. Engine work goes to the fork.
