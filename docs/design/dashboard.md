# Dashboard v3 — the Dashboard tab

Part of the design of the dashboard. The main file is
[../design-dashboard.md](../design-dashboard.md); the components are in
[components.md](components.md).

The question of the tab: **is it serving, and how fast?**

The Dashboard tab is the operational answer in plain language. It follows
the reading order of the `vllm-rdna-dash` reference (credited in
[third-party.md](../third-party.md)) and is built from bernie's own
components and data.

The separate Advanced tab keeps bernie's diagnostic depth: fleet telemetry,
the complete per-GPU table, power controls, the run card and the raw
documents. The difference between the tabs is intentional: Dashboard is for
the one-minute operational answer, Advanced is for operating and diagnosing
the host.

Wireframes: `design/dashboard-1280.svg` (Simple at 1280 px),
`design/dashboard-390.svg` (Simple at 390 px),
`design/dashboard-empty-1280.svg` (nothing serves).

## The order of the Dashboard tab

1. **The hero card**: the state in one big word (`SERVING`, `STARTING`,
   `NOT RUNNING`), one plain sentence, the endpoint with **Copy URL**, the
   Stop or Start action, and the output speed with its unit.
2. **Six serving tiles**: Prompt reading, Active requests, First token, KV
   cache, Prefix cache hits and Draft accepted.
3. **Two request-rate charts**: Output speed and Prompt reading, both with
   their unit and `last 5 min` in the title.
4. **GPUs**: one card per GPU.
5. **This machine** and **Connect an app**, side by side.

This is the reading order of the reference design. The difference is the
implementation and the data: the Dashboard uses bernie's shared chart,
meter, tile, command and copy components and the one telemetry stream.

The look follows BroodBench's compare page, which shares bernie's palette:
the hero and the panels are rounded cards (18 px) with the soft shadow
`--shadow-2`; the state word is the italic display type in `--key` with
`--punch-sm`; the section labels are `--key`; the metric tiles are raised
panels (`--surface-2`, 14 px) whose number is the italic display type in
`--key` with a hard ink shadow; the charts sit on raised panels. The rules
live at the end of `views/dashboard/dashboard.css`, scoped to the tab, and
use the tokens only.

## The order of the Advanced tab

1. The state headline and Stop/Restart.
2. The fleet hero: the output speed and the eight fleet tiles.
3. `THE CARDS`: one card per GPU and the exact per-GPU table.
4. `THE LAST MINUTES`: the four area charts with the window control.
5. `THE RUN`: the run card and the smoke prompt.
6. `THE POWER CAPS`: the caps of the cards.
7. `KNOWN PROBLEMS`: the doctor findings.
8. The raw documents of every endpoint.

## The Dashboard widgets

| # | Widget | Data source | Refresh | Empty copy | Error copy |
|---|---|---|---|---|---|
| 1 | The hero state | `GET /api/telemetry` → `runs[]`; `GET /api/runs` | every sample (1 Hz) | `NOT RUNNING` and `The model server is not running.` | the state stays unknown and the endpoint names the failure |
| 2 | The endpoint and Copy URL | `GET /api/runs` → `url`, `host`, `port` | every sample | `No endpoint while no run is serving.` | the action is disabled and the error is in words |
| 3 | The output speed | `GET /api/telemetry` → `runs[].gen_tok_s` | every sample | `– tok/s` and `No run is serving.` | `– tok/s` and the stream failure is stated |
| 4 | Prompt reading | `runs[].prompt_tok_s` | every sample | `– tok/s` and `n/a on this host` | `– tok/s` and the stream failure is stated |
| 5 | Active requests | `runs[].stats.requests_running`, `requests_waiting` | every sample | `– requests` and `n/a on this host` | `– requests` and the stream failure is stated |
| 6 | First token | not published by the telemetry stream | every sample | `– s` and `n/a on this host` | `– s` and the stream failure is stated |
| 7 | KV cache | `runs[].stats.kv_cache_usage` (a fraction from 0 to 1) | every sample | `– %` and `n/a on this host` | `– %` and the stream failure is stated |
| 8 | Prefix cache hits | not published by the telemetry stream | every sample | `– %` and `n/a on this host` | `– %` and the stream failure is stated |
| 9 | Draft accepted | `runs[].stats.spec_accept_rate`, `spec_draft_tokens_total` | every sample | `– %` and `no draft tokens yet` | `– %` and the stream failure is stated |
| 10 | Output-speed chart | `GET /api/gpus/history?window=5m` | on open and when the view changes | `No samples yet.` | `The history could not be read.` plus `Try again` |
| 11 | Prompt-reading chart | `GET /api/gpus/history?window=5m` | on open and when the view changes | `No samples yet.` | `The history could not be read.` plus `Try again` |
| 12 | The GPU cards | `GET /api/telemetry` → `gpus[]`; `GET /api/hw` for names and limits | every sample | the section says that no card is visible | every unmeasured value is `–` |
| 13 | This machine | `GET /api/hw` → `host`, `gpus[]` | on open | `–` and `n/a on this host` | the card names the endpoint that failed |
| 14 | Connect an app | the same endpoint and model as the hero | every sample | the empty state and `bernie serve <recipe>` | the empty state names the failed read |

Every tile has an `(i)` mark. Its explanation comes from
`GET /api/glossary`: the same domain glossary that the Setup tab uses. The
subtitle says what the number means for a person: `0 waiting in line`, `of
GPU cache in use`, `reading the prompt`, `n/a on this host` or `no draft
tokens yet`.

## The hero

| State | The big word | The sentence |
|---|---|---|
| A run answers | `SERVING` | `<run> is answering requests on port <port>.` |
| A run loads | `STARTING` | the phase sentence of the run, for example `Loading the model weights, 2 of 5 shards` |
| No run | `NOT RUNNING` | `The model server is not running.` |

The endpoint is the OpenAI-compatible base URL with `/v1`. The Copy URL
button copies exactly that text. The Stop button uses the guarded
`DELETE /api/runs/{name}` write. With no run, **Start a run** leads to the
Setup tab, where the launch plan is shown before anything starts.

The right half of the hero holds the output speed as the large number. Its
unit is always beside it. The number under it gives the one-minute average;
the sparkline is the last minute of the same shared stream.

## The six serving tiles

| Tile | The number | The unit | The human subtitle |
|---|---|---|---|
| Prompt reading | `runs[].prompt_tok_s` | `tok/s` | `reading the prompt` |
| Active requests | `runs[].stats.requests_running` | `requests` | `n waiting in line` |
| First token | not published | `s` | `n/a on this host` |
| KV cache | `runs[].stats.kv_cache_usage` | `%` | `of GPU cache in use` |
| Prefix cache hits | not published | `%` | `n/a on this host` |
| Draft accepted | `runs[].stats.spec_accept_rate` | `%` | `MTP drafts kept` |

A value that was not measured is `–`, never `0`. The sparkline of a tile
breaks at a missing sample; it never draws a gap as zero.

## The two charts

The Dashboard tab has two large area charts: **Output speed** and **Prompt
reading**. Their titles carry the unit and the window: `tok/s, last 5 min`.
They use the shared charts facade, so both have labelled axes, a keyboard
readout and a data-table fallback with the same numbers.

The Advanced tab keeps the four machine charts: utilisation, power, memory
and tokens per second. A segmented control picks `5 min`, `15 min` or `1 h`.
A missing history point is a gap in the line.

## The card of one GPU

Each card holds, in this order: the name and PCI address of the card, its
state pill (`In use` or `Idle`), the ring of the load, the labelled memory
and power meters, the edge / junction / memory temperature line and the
shader / memory clock line. The ECC state remains a pill with a word.

A hot card says so in words: the temperature uses `--serious` and the line
reads `hot: the limit of this card is 100 °C`. A card that does not publish
the junction or the memory temperature shows `–` for it.

Advanced adds the exact table of every card: load, memory used and total,
power and cap, all three temperatures, both clocks and ECC. Every numeric
cell carries its unit.

## This machine and Connect an app

**This machine** names the host, CPU, RAM and AMD GPUs from `/api/hw`. A
host metric that the platform does not publish is `–` and says `n/a on this
host`; it is never zero.

**Connect an app** gives the base URL, the served model name and a
copyable curl or Python request. The hero, the run card and both snippets
read one endpoint helper, so the address cannot drift between the blocks.
The snippet is an OpenAI-compatible chat completion and needs no API key.

## The Advanced tab widgets

| Widget | Data source | What it keeps |
|---|---|---|
| The fleet tiles | `/api/telemetry` | load, power, memory, hottest card, highest clock, output rate, requests and KV cache |
| The GPU cards and table | `/api/telemetry`, `/api/hw`, `/api/doctor` | every per-GPU value, its limit and its ECC state |
| The four charts | `/api/gpus/history` | utilisation, power, memory and output rate with the window control |
| The run card | `/api/runs`, `/api/telemetry`, `/api/smoke` | phase, endpoint, age, Stop, Restart, one-prompt smoke test and findings |
| The power caps | `/api/power` | the service, desired and effective caps and the guarded write |
| The known problems | `/api/doctor` | failing and warning checks with their fixes |
| The raw documents | every endpoint above | the exact JSON behind the cards and the matching CLI commands |

Advanced does not change the data path: it uses the same one telemetry
stream, pauses while `document.hidden` is true and refreshes once when the
document becomes visible.
