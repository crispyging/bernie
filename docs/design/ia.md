# Information architecture

Status: design of 2026-10-29, kept as the contract of the shape. C1–C8 are
implemented: the spec drift (`6edefa5`), the shared web helpers (`9fdccb8`),
the "Show details" word (`06c0879`), the details of the Dashboard tab
(`490f95a`), the operator's console on the Advanced tab (`e9e7e72`), the
one-button Setup path (`b3b07d7`), the Chat tab (`e945f2f`, `6566750`), the
device access (`5374ef0`–`638346a`), the server metrics (`4615a64`–`eeb2357`)
and the charts (`2bfbd36`, `e06bddc`); the redaction and the write gate of
the inventory landed with them (`b289c76`, `10afc78`). C9 — the redacted
share report and the preferences panel — is still open. The Fleet tab stays
planned (D2).

This file is the contract for the *shape* of bernie: which capability lives
where, what a tab is for, what "more information" means, and the words the
three front ends use. The behaviour of the product is [spec.md](../spec.md).
The visual design is [../design-dashboard.md](../design-dashboard.md) and the
component sheets in this folder.

## The summary

1. **One binary, three front ends.** The CLI (19 commands, 38 paths), MCP
   (19 read tools, 6 write tools) and the dashboard (8 tabs, 33 API routes)
   are three views of the same code; a value on the page exists in the
   `--json` output with the same name.
2. **Two mechanisms for "more information", with different jobs.** The
   per-tab **Show details** toggle is more about the *same thing* on that
   tab. The **Advanced tab** is the operator's console: the things about the
   host that belong to no single tab.
3. **The tab jobs.** Setup: what do I do to make this box serve? Dashboard:
   is it serving and how fast? Benchmark: how fast and does it read the whole
   prompt? Topology: can the cards copy to each other? Logs: what is running
   and why did it fail? Advanced: what does the operator need to see and
   change? Chat and Fleet: planned.
4. **What moves.** The old first-dashboard layout leaves the Advanced tab
   (its unique parts stay, the duplicates go). The Dashboard tab gains a
   details disclosure. The Setup tab's primary button runs the whole path.
5. **What is missing from the reference.** 21 of its 49 features are already
   in bernie, 19 are partial, 9 are absent. The big absent ones: the
   one-button setup, the Chat tab, the redacted share report, the
   preferences form, the download speed limit, the job cancel.
6. **The order of work.** Nine commits: the spec fixes first (C2), then the
   shared web vocabulary (C3, C4, C5), then the Setup path (C6), the
   Advanced tab (C7), Chat (C8) and the report and preferences (C9).
7. **The counts are checked** against `bernie --help`, the MCP tool list and
   the route table (§3.11); 25 places where the spec, the code and the page
   disagree are listed in §3.12.

Read it with:

- [../spec.md](../spec.md) — the contract for the commands, the API and MCP.
- [../../dashboard/README.md](../../dashboard/README.md) — the tab table of the
  page as it is today.
- [../../dashboard/AGENTS.md](../../dashboard/AGENTS.md) — the rules of the
  page.
- [../design/rules-ui.md](../rules-ui.md) — the reasoning behind the rules.

## Open decisions

Five decisions the owner has to make. Each one has a recommendation. The
rest of this file assumes the recommendation.

| # | Decision | Recommendation | Cost to change later |
|---|---|---|---|
| D1 | **What is the Advanced tab?** Today it is the first bernie dashboard (the old layout) kept for comparison. The page also has a per-tab `Advanced` disclosure and a rail-wide `Show details` switch. | Keep the Advanced tab as the operator's console (the power write, the full per-GPU table, the doctor report in full, the jobs, the raw documents of every endpoint) and retire the old layout from it. Keep the per-tab Show details toggle as more detail about the same thing on that tab. The two jobs differ, so both stay; the Advanced tab stops being a copy of the Dashboard tab. | Medium. Renaming the tab is cheap; rebuilding it is not. |
| D2 | **Chat and Fleet stay as placeholders?** The owner ordered them present-but-not-usable. | Keep Fleet as a placeholder for one more release, marked *planned*; ship a lean Chat now — one prompt box that posts to the running run and streams the reply. A tab that says "planned" forever is a dead tab; a one-prompt Chat is the smallest thing that is not a promise. See [§6](#6-chat-and-fleet-the-two-placeholder-tabs). | Low. A placeholder tab can become real at any time. |
| D3 | **Does the Dashboard tab keep both a simple view and an advanced view?** Today it has a hand-written *simple* view and the old layout lives in the Advanced tab. | Yes, keep both, with one rule: the simple view is the default and shows the one question; its Show details toggle reveals the per-GPU table, the history window and the power read-out for the *same* run. The Advanced tab keeps the cross-cutting things. | Medium. The two views share `components/charts.js`. |
| D4 | **"Recipe" or "profile"?** *Profile* is used today for a TunableOp profile, *recipe* for a model+flags+mode. The reference uses *profile* for the host. | Recipe stays the word for a model. A TunableOp profile keeps its name and is always written with its qualifier. The bare word *profile* is retired from the UI, because the reference uses it for the host. | Low, but it touches copy in four places. |
| D5 | **Where does the page show how a container gets the GPU?** The domain layer already answers it — `classic`, `cdi` or `auto`, decided by `host.ResolveDeviceAccess` (`internal/machine/host/cdi.go:340`), with the reason in one sentence and the plan carrying `device_access`, `device_access_reason` and `cdi_devices`. The flag is `--device-access`, the config key is `device_access`, the recipe field is `device_access`. The dashboard exposes none of it. | Show it in the Setup tab's details next to the plan, in one line with the reason under it, and as one check in the doctor's `container` category. Never in the simple view: a reader who did not ask how the devices get in does not need to know. | Low. The decision and the words already exist; only the page and the doctor check are missing. |

---

## 1. The shape of the product

bernie is one binary with three front ends:

| Front end | What it is | Who reads it |
|---|---|---|
| CLI | `bernie` and its 19 commands | a person in a terminal, a script |
| MCP | 25 tools over stdio and `POST /mcp` | an agent |
| Dashboard | 8 tabs, a JSON API and 4 event streams | a person in a browser |

The rule that keeps them one product is in spec §13.1: *MCP adds no feature of
its own: every tool calls the code of a CLI command and returns the same JSON
document as that command with `--json`.* The same holds for the dashboard: it
is a view of the CLI, never a second implementation.

So the information architecture below is really one question asked three
times: **what is the smallest set of questions a user has, and where does each
question live?** The answer is the same on all three front ends: the six
questions of §4.1, in the same order.

## 2. The vocabulary

One word, one meaning, in all three front ends. The words below are the ones
that exist today; the ones marked **today: X** are the collisions to fix.

| Word | Means | Never means | Where it lives today |
|---|---|---|---|
| **host** | the machine bernie runs on | a container | CLI `--root`, spec §12 |
| **container** | the run inside a Docker or Podman container | the host | `recipe.ModeContainer` (`internal/serving/recipe/plan.go:18`) |
| **run** | one served model: a process or a container, with a name | a job | `<state>/runs/<name>/run.json`, spec §8.3 |
| **job** | one background action bernie runs now (a setup step) | a run | `<state>/jobs/<id>`, spec §3.3 |
| **recipe** | a model plus the flags, the mode and the way it is served | a profile, a preset | `recipes/*.yaml`, spec §7 |
| **recipe** (the second sense) | the **merged** document after `extends` | the file on disk | `bernie recipe show ID`; spec §7.2 |
| **profile** (TunableOp profile) | a table of tuned settings for one rocBLAS build | the host profile | `tunableop/<arch>/<profile>/`, spec §9.1 |
| **step** | one entry of the onboarding checklist | a job, a setup step of the plan | spec §3.3 (`hardware`, `access`, `rocm`, `python-env`, `model`, `recipe`, `serve`, `verify`, plus `bench`, `tuning`, `power`) |
| **setup step** | one install action of `setup plan` (`venv.create`, `model.download`, …) | a checklist step | `internal/setup/steps_*.go` |
| **setup step id** | the eleven ids of `setup apply --step`: `groups`, `rocm.runtime`, `container.engine`, `image.pull`, `venv.create`, `venv.torch`, `causal-conv1d.build`, `flash-attn.build`, `vllm.build`, `venv.verify`, `model.download` | a checklist step id | `internal/setup/steps_*.go` |
| **check** | one doctor check | a P2P checklist item | `internal/doctor`, spec §5.2 |
| **finding** | one known failure matched in a log | a check | `internal/logdoc`, spec §5.3 |
| **suite** | one benchmark suite (`smoke`, `pr`, `long-context`) | a bench run | `bench/suites/*.yaml` |
| **result** | one saved benchmark result | a suite | `bench list` |
| **verdict** | the outcome of comparing two results | a score | spec §10.3 |
| **state** | the word that says what a thing is doing: `done`, `todo`, `running`, `blocked`, `skipped` (checklist); `starting`, `running`, `exited` (run); `running`, `done`, `failed` (job) | a colour | spec §3.3, §8.3 |
| **status** | the word that says what a check says: `ok`, `warn`, `fail`, `skip` | a state | spec §5.1 |
| **fix** | the exact command that repairs a check or a finding | advice | spec §5.1, §5.3 |
| **plan** | the resolved environment and command of a recipe | a preview | spec §7.2 |
| **mode** | `host` or `container` | an engine | spec §8.2 |
| **device access** | how a container gets the GPU: `classic` (the device pass-through and the group ids) or `cdi` (the AMD Container Toolkit's device spec) | the mode | `internal/machine/host/cdi.go:48`; `auto` means "whatever the host can do" |
| **fit** | whether a recipe suits this host: `fits`, `no` (with the reason), `unknown` (no GPU data) | the recipe's status | `internal/serving/recipe/fit.go:13-15` |
| **status of a recipe** | its maturity: `validated`, `experimental`, `reference` | the fit | `internal/serving/recipe/recipe.go:21`; only `validated` recipes count as supporting an arch |
| **verdict** | the outcome of comparing two results: `better`, `same`, `worse`, `noisy` | a score | `internal/serving/bench/suite/verdict.go:13-21` |
| **venv source** | where the venv came from: `flag`, `$BERNIE_VENV`, `config`, `$VIRTUAL_ENV`, `conventional`, `ambiguous`, `none` | the venv path | `internal/support/config/venv.go:14-21` |

### 2.1 The three state vocabularies, and why they are three

Today three different sets of words describe three different kinds of thing.
That is correct — a checklist step and a GPU check are not the same kind of
thing — but the *shape* of the sets must match, or the page cannot say "the
same state" once.

| Kind | Words | Source |
|---|---|---|
| checklist step | `done` `todo` `running` `blocked` `skipped` | spec §3.3 |
| job | `running` `done` `failed` | spec §3.3 |
| run | `starting` `running` `exited` | spec §8.3 |
| check | `ok` `warn` `fail` `skip` | spec §5.1 |
| finding | `error` `warn` | spec §5.3 |
| benchmark job | `running` `done` `failed` | spec §11 |
| topology link class | `PIX` `PXB` `PHB` `SYS` | spec §4.2 |

The page must show these words unchanged. `dashboard/web/components/timeline.js:9`
already holds `TIMELINE_STATE_WORD` with `done/todo/running/blocked/skipped` and
one extra (`failed`) that the spec does not have; `dashboard/web/js/shell.js:259`
holds the run pill (`idle`, `starting`, `serving`, `not answering`, `stopped`).
The run pill is the one place where a new word was invented: it does not exist
in the spec. **Decision for the commit plan**: the rail pill keeps its five
words, and spec §8.3 gains them as the *view* of the run state (`idle` = no
run, `not answering` = a run that exists but does not answer). One run, one
list of words, in the spec.

### 2.2 The error wording pattern

Every failure a user can see answers three questions in this order:

1. **What happened.** One sentence, present tense, no blame.
2. **Why.** The cause, in the words of the machine (a path, a number, a
   threshold) or in the words of the user (a missing package).
3. **What to do next.** The exact command, copyable, or the exact control to
   press.

The doctor already does this: `{"id", "category", "status", "title", "detail",
"fix", "doc"}` (spec §5.1). The log doctor does the same with
`{"id", "severity", "cause", "fix", "doc", "line", "line_no"}` (spec §5.3).
The dashboard's finding card uses the same shape
(`dashboard/web/views/logs/findings.js`).

The CLI already writes the pattern down. These are the exact strings, so the
page can copy the shape rather than invent one:

| Kind | The exact shape | An example |
|---|---|---|
| A failed check | `[STATUS] TITLE: DETAIL`, then `fix:` and `doc:` | `[FAIL] /dev/kfd is readable and writable: permission denied` · `fix: sudo usermod -aG video,render $USER` |
| A problem in `status` | `[STATUS] TITLE → FIX` | `[FAIL] ROCm is installed → sudo dnf install -y rocm rocm-hip-runtime-devel` |
| A log finding | `[SEVERITY] ID (line N)`, then `cause:`, `fix:`, `doc:`, `line:` | `[error] memory.kv-too-small (line 82)` |
| A run conflict | the subject, its state, then the next step | `run flashnext is running (pid 1234); stop it first (bernie stop flashnext) or use --name` |
| A run that exited | the subject, then the pointer | `run flashnext exited during startup; see: bernie logs flashnext` |
| A run that did not become healthy | the subject, the limit, then the pointer | `flashnext is not healthy after 30m0s; see: bernie logs flashnext` |
| A blocked `up` | `up: stopped: REASON`, then the indented commands | `up: stopped: confirmation required; re-run with --yes` |
| A step that needs root | the exact command, then who acts | `needs root: run the command above` |
| A bad value | the flag, the value, and the set of valid values | `--mode must be host or container, not "gpu"` · `unknown category "bios" (have os, gpu, kernel, …)` |
| A missing object | the object and where it looked | `no run named "flashnext" (looked in /home/x/.local/state/bernie/runs)` |

**The rule for the page**: every problem card has the same four fields in
the same order — what happened, why, the fix, the doc link — and the fix is
a command with a copy button, never advice. The CLI's `cause:` / `fix:` /
`doc:` / `line:` labels are the names of the fields on both front ends, and
the page never says "invalid", "illegal", "wrong" or "incorrect" (the rule in
`dashboard/AGENTS.md`, Setup rule 9).

### 2.3 The step names, one list

The eight checklist steps and the three extras are the spine of the product.
They are named in spec §3.3 and must never be renamed per front end. The
titles below are the **code's** titles (`internal/onboarding/steps.go:29`
and `steps_extra.go:9`), which the CLI's `status --steps` prints and the
dashboard's Setup tab renders.

| # | Id | Title (the words of the row) | The action word | One-line job |
|---|---|---|---|---|
| 1 | `hardware` | GPUs found and supported | Check the cards | the `gpu.count`, `gpu.supported.*`, `gpu.ecc.*` and `gpu.igpu` checks pass |
| 2 | `access` | Device access and groups | Fix the device access | the `gpu.kfd`, `gpu.render` and `gpu.groups` checks pass |
| 3 | `rocm` | ROCm runtime and kernel | Install the ROCm tools | the `kernel.*` and `rocm.*` checks pass |
| 4 | `python-env` | Python environment (torch and vLLM) | Build the Python environment | the `python.*` checks pass in the venv |
| 5 | `model` | Model files | Download the model | the weights are on the disk |
| 6 | `recipe` | Recipe | Pick the recipe | a validated recipe matches the GPUs |
| 7 | `serve` | Serve the model | Start the server | a run serves |
| 8 | `verify` | The server answers a request | Ask the server | one prompt comes back |
| 9 | `bench` | Measure the speed (optional) | Measure the speed | optional: a suite has run |
| 10 | `tuning` | TunableOp profile (optional) | Tune the kernels | optional: a TunableOp profile is loaded |
| 11 | `power` | Power cap (optional) | Set the power cap | optional: the caps are written |

**A naming drift to fix.** The tab sheet [setup.md](setup.md) gives the rows
different titles: *"The cards are there"*, *"The groups are right"*, *"The
ROCm tools work"*, *"The Python environment is ready"*, *"The model is on
the disk"*, *"A recipe fits"*, *"The server answers"*. The code says
*"GPUs found and supported"*, *"Device access and groups"* and so on. One of
the two has to go; the code's titles are shorter and they name the check
that holds the step, so the code's win. The sheet is updated in C2.

The dashboard's Setup tab renders the path as `THE PATH TO A SERVING MODEL`
(`dashboard/web/views/setup/setup.js:72`). The CLI's `status --steps` prints
the same steps and the same titles; that is the check for this table.

---

## 3. The inventory of bernie today

Every user-visible capability, where it lives today, and where it should
live. **91 rows**: 79 capabilities (§3.1 to §3.9) plus the 12 promises the
three front ends make to each other (§3.10). Sources: `bernie --help` and
`bernie <cmd> --help`
(19 top-level commands, 38 runnable paths), `internal/mcp/tools_read.go` and
`tools_write.go` (19 read tools, 6 write tools), `dashboard/internal/api/api.go`
(33 routes), `dashboard/README.md` (the tab table), spec §3.2, §4, §5, §7,
§8, §9, §10, §11, §13.

Legend for the last two columns: **S** = the simple view of the tab (the
default), **D** = the *Show details* toggle of that tab, **A** = the Advanced
tab.

### 3.1 Host inspection and diagnosis

| # | Capability | CLI | MCP | Dashboard home today | Proposed home | Note |
|---|---|---|---|---|---|---|
| 1 | Hardware and OS inventory | `bernie hw` | `hw` | Advanced tab (raw doc); Setup reads it for the facts | **A** (raw) + Setup **D** (facts) | one document, three readers |
| 2 | PCIe tree | `bernie topo` | `topo` | Topology tab, `THE PCIE TREE` | Topology **S** | already right |
| 3 | P2P link classes (PIX/PXB/PHB/SYS) | `bernie topo` | `topo` | Topology tab, `THE LINK CLASSES` | Topology **S** | already right |
| 4 | P2P readiness checklist (8 items) | `bernie topo --check` | `topo` | Topology tab, `THE READINESS CHECKLIST` | Topology **S** | the doctor's `topo.p2p` category is the same list |
| 5 | P2P readiness with a named recipe's env | `bernie topo --check --recipe ID` | `topo` | Topology tab (`recipe_env` item) | Topology **D** | one item of the checklist |
| 6 | GPU-to-GPU bandwidth measurement | `bernie topo --bandwidth` | `topo` (`bandwidth`) | Topology tab, `THE BANDWIDTH` — says the dashboard never measures it | Topology **D** | a long measurement; the tab names the command |
| 7 | All host checks (9 categories) | `bernie doctor` | `doctor` | Setup tab, `DOCTOR`; Advanced tab | Setup **D** (full list) + **A** (raw) | the count of failures is the tab badge |
| 8 | One category of checks | `bernie doctor --category C` | `doctor` (`categories`) | not on the page | Setup **D** | the page filters client-side |
| 9 | Log doctor on a file or stdin | `bernie doctor --log FILE\|-` | `doctor_log` (by run name only) | Logs tab, `THE KNOWN PROBLEMS` | Logs **S** | the MCP tool takes a run name, never a path (spec §13.7) |
| 10 | Log doctor on a run | `bernie doctor --run NAME` | `doctor_log` | Logs tab | Logs **S** | same rule set |
| 11 | The phase sentence of a run | (in `ps --json`) | `ps` | rail pill, Logs tab, Dashboard hero | Dashboard **S** | spec §5.4 |

### 3.2 The setup path

| # | Capability | CLI | MCP | Dashboard home today | Proposed home | Note |
|---|---|---|---|---|---|---|
| 12 | The host status and the next command | `bernie status` | `status` | not on the page (Setup reads the same facts) | Setup **S** (headline) | the CLI is the printed form of the Setup headline |
| 13 | The onboarding checklist | `bernie status --steps` | `onboarding_status` | Setup tab, `THE PATH TO A SERVING MODEL` | Setup **S** | the spine of the product |
| 14 | The facts of the host (label, value, why) | `bernie status --json` (`onboarding.facts`) | `onboarding_status` | Setup tab, facts strip | Setup **S** | spec §3.3 |
| 15 | The next step in words | `bernie status --json` (`onboarding.next_action`) | `onboarding_status` | Setup tab, headline lede | Setup **S** | one record, three front ends |
| 16 | Run one non-root setup step | `bernie setup apply --step ID` | `setup_apply` (all steps) | Setup tab, `Run` on a step | Setup **S** | one job at a time |
| 17 | Run the whole non-root path | `bernie up` | `setup_apply` | **not on the page** (the Setup tab runs one step) | Setup **S**: the one primary button | `POST /api/onboarding/path/run` exists; the page does not call it |
| 18 | Run all non-root setup steps | `bernie setup apply` | `setup_apply` | not on the page (one step at a time) | Setup **S** (one primary button) | the reference's "set everything up" |
| 19 | The install plan of this host | `bernie setup plan` | — | not on the page | Setup **D** | the plan behind the steps |
| 20 | Root steps as commands | `bernie setup apply` (prints them) | `setup_apply` (returns them) | Setup tab, command block per root step | Setup **S** | bernie never runs sudo |
| 21 | The venv and where it came from | `bernie status --json` (`venv`) | `status`, `onboarding_status` | Setup tab, facts | Setup **D** | spec §6.2 |
| 22 | The glossary of the words | — (no CLI command) | — | `/api/glossary`, Setup tab `The words on this page` | Setup **D** + Dashboard **D** | the API exists, the CLI does not: **gap** |

### 3.3 Recipes and the plan

| # | Capability | CLI | MCP | Dashboard home today | Proposed home | Note |
|---|---|---|---|---|---|---|
| 23 | The list of recipes with their fit | `bernie recipe list [--all]` | `recipe_list` (`all`) | Setup tab, `THE RECIPE` | Setup **S** | |
| 24 | One recipe, merged | `bernie recipe show ID` | `recipe_show` | Setup tab (the picked recipe) | Setup **D** | |
| 25 | The plan: env, args, GPU selection | `bernie recipe plan ID` | `recipe_plan` | Setup tab, the plan card | Setup **D** | spec §7.2 |
| 26 | Override a setting | `bernie recipe plan ID --set K=V` | `recipe_plan` (`set`) | Setup tab, overrides | Setup **D** | |
| 27 | The reason behind every setting | (in `recipe plan --json`) | `recipe_plan` | Setup tab, `Settings and reasons` | Setup **S** | already shipped (spec §7.9) |
| 28 | The mode switch (host / container) | `--mode host\|container` | `recipe_plan` (`mode`) | Setup tab, the mode switch | Setup **S** | |
| 29 | Validate the recipe files | `bernie recipe validate` | — | not on the page | **A** | a maintainer command |
| 30 | The recipe catalogue (12 recipes, 3 families) | `bernie recipe list --all` | `recipe_list` | Setup tab | Setup **S** | `recipes/`: flashnext ×7, qwen-dense ×4, deepseek ×1; plus 12 `_`-prefixed fragments used only through `extends`. Eight recipes can run in a container; four are host-only (`container: null`) |

### 3.4 Runs

| # | Capability | CLI | MCP | Dashboard home today | Proposed home | Note |
|---|---|---|---|---|---|---|
| 31 | Start a run | `bernie serve ID` | `serve` | Setup tab (the `serve` step) | Setup **S** | |
| 32 | Start a run and wait for health | `bernie serve ID --wait D` | — (poll `ps`) | not on the page | Setup **D** | |
| 33 | The whole path in one command | `bernie up [RECIPE]` | not a tool (spec §13.4) | not on the page | Setup **S** (the one primary button) | the reference's "set everything up" |
| 34 | The list of runs | `bernie ps` | `ps` | Logs tab, `THE RUN`; rail pill | Dashboard **S** + Logs **S** | one document, two readers |
| 35 | Stop a run | `bernie stop [NAME]` | `stop` | Dashboard hero `Stop`; Logs tab | Dashboard **S** | |
| 36 | The log of a run | `bernie logs [NAME]` | `logs` | Logs tab, `THE LOG` | Logs **S** | |
| 37 | Follow the log | `bernie logs NAME -f` | — | Logs tab (SSE) | Logs **S** | |
| 38 | The tail of the log with levels and a search | `bernie logs --tail N` | `logs` (`tail`) | Logs tab (level filter, search) | Logs **S** | |
| 39 | The findings of an exited run | (in `ps --json`) | `ps` | Logs tab, `THE KNOWN PROBLEMS` | Logs **S** | spec §8.3 |
| 40 | Check that the server answers | `bernie smoke [--url] [--model]` | `smoke` | Setup tab (the `verify` step) | Setup **S** + Dashboard **S** | spec §3.3 step 8 |
| 41 | The run record (pid, container, port, plan) | (in `ps --json`) | `ps` | Advanced tab | **A** | spec §8.3 |

### 3.5 Container mode

| # | Capability | CLI | MCP | Dashboard home today | Proposed home | Note |
|---|---|---|---|---|---|---|
| 42 | Run in a container | `bernie serve ID --mode container` | `serve` (`mode`) | Setup tab (the mode switch) | Setup **S** | |
| 43 | The container plan (`docker run` argv) | `bernie recipe plan ID --mode container` | `recipe_plan` (`mode`) | Setup tab, the plan card | Setup **D** | `internal/serving/runner/command.go:148` |
| 44 | A compose file for the plan | `bernie compose ID [-o FILE]` | — | not on the page | **A** | `internal/serving/runner/command.go:295` |
| 45 | The static container files of the project: the dashboard's own compose file (`deploy/compose/compose.yml`, the image `ghcr.io/opengfx1030/bernie`), the Podman override, the reference vLLM compose file and the Dockerfile | — (files in the repo) | — | not on the page | **A** | `deploy/`; the dashboard can itself run in a container |
| 46 | The GPU devices of the container | (in the plan) | (in the plan) | not on the page | **A** | today `--device /dev/kfd --device /dev/dri`; see D5 |
| 47 | The GPU group ids of the container | (in the plan, `group_add`) | (in the plan) | not on the page | **A** | numeric ids, never names (spec §8.2) |
| 48 | The container engine (Docker or Podman) | `bernie doctor --category container` | `doctor` | Setup tab, `DOCTOR` | Setup **D** | |
| 49 | Rootless Podman (`--group-add keep-groups`) | (automatic) | — | not on the page | **A** | spec §8.2 |
| 50 | How the container gets the GPU: `classic`, `cdi` or `auto` | `--device-access`, the config key `device_access`, the recipe field `device_access` | (in `recipe_plan`) | **not on the page** | Setup **D** + the doctor `container` category | the domain layer decides it and says why in one sentence (`internal/machine/host/cdi.go:340`); see D5 |

### 3.6 Tuning and power

| # | Capability | CLI | MCP | Dashboard home today | Proposed home | Note |
|---|---|---|---|---|---|---|
| 51 | The TunableOp profiles | `bernie tune tunableop list` | — | not on the page | **A** | |
| 52 | The loaded rocBLAS and its profile | `bernie tune tunableop status` | `tunableop_status` | Setup tab (the `tuning` extra step) | Setup **D** | |
| 53 | The TunableOp env lines | `bernie tune tunableop env` | — | not on the page | **A** | materialises the rows |
| 54 | Force a profile / no TunableOp | `--profile P`, `--no-tunableop`, `--tuning` | `recipe_plan` (`profile`, `no_tunableop`, `tuning`) | Setup tab, the plan card | Setup **D** | |
| 55 | The power cap of every GPU | `bernie tune power status` | `power_status` | Advanced tab, `THE POWER CAPS` | Advanced **D** → **A** | the read moves to **A** |
| 56 | Write the desired power cap | `bernie tune power set W [--gpu IDX]` | `tune_power_set` | Advanced tab (a write) | **A** | the only power write |
| 57 | Apply the caps (needs root) | `bernie tune power apply` | — | not on the page (a root command) | **A** | prints the root command |
| 58 | The power service (systemd pair) | `bernie tune power service print\|install-plan\|status\|remove-plan` | `power_status` (state only) | not on the page | **A** | spec §9.2 |
| 59 | The power advice per card | (in the doctor `tuning` category) | `doctor` | Setup tab, `DOCTOR` | Setup **D** | |

### 3.7 Benchmark

| # | Capability | CLI | MCP | Dashboard home today | Proposed home | Note |
|---|---|---|---|---|---|---|
| 60 | Run a suite | `bernie bench [--suite]` | `bench_run` | Benchmark tab, `The suites` → `Run the suite` | Benchmark **S** | |
| 61 | The built-in suites (3: `smoke`, `pr`, `long-context`) | `bernie bench suites` | `bench_suites` | Benchmark tab, the suite cards | Benchmark **S** | `bench/suites/*.yaml` |
| 62 | The saved results | `bernie bench list` | `bench_list` | Benchmark tab, `The history` | Benchmark **S** | |
| 63 | One saved result | `bernie bench show ID` | `bench_show` | Benchmark tab, `The result` | Benchmark **S** | |
| 64 | Compare two results with a verdict | `bernie bench compare A B` | `bench_compare` | Benchmark tab, `The comparison` | Benchmark **S** | spec §10.3 |
| 65 | The receipt of the comparison | `bernie bench compare A B --markdown` | — | Benchmark tab, `Copy the receipt` | Benchmark **S** | |
| 66 | The A/B run | `bernie bench ab --base --cand` | `bench_ab` | Benchmark tab, `An A/B run` | Benchmark **S** | spec §10.4 |
| 67 | The pass-key recall check | `bernie bench --recall` | `bench_run` (`recall`) | Benchmark tab, the suite options | Benchmark **S** | |
| 68 | The length curve of a suite | (in `bench show --json`) | `bench_show` | Benchmark tab, `The result` | Benchmark **S** | |

### 3.8 The dashboard, the telemetry and the jobs

| # | Capability | CLI | MCP | Dashboard home today | Proposed home | Note |
|---|---|---|---|---|---|---|
| 69 | The live sample of the host | — (no CLI) | — | Dashboard tab, the tiles and the charts (1 Hz SSE) | Dashboard **S** | `/api/telemetry` |
| 70 | The history of the last minutes | — (no CLI) | — | Dashboard tab, the charts | Dashboard **S** (window) | `/api/gpus/history` |
| 71 | The power service state | `bernie tune power service status` | `power_status` | `/api/power` | Advanced **A** | |
| 72 | Write the power cap | `bernie tune power set` | `tune_power_set` | `PUT /api/power` | Advanced **A** | the only write on the Dashboard API besides runs |
| 73 | The jobs of the host | — (no CLI) | `jobs` | Setup tab (the job of a step) | Setup **D** + **A** | |
| 74 | One job and its output | — (no CLI) | — | Setup tab, under the step | Setup **S** (while it runs) | |
| 75 | The event stream of a job | — | — | Setup tab, the job stream | Setup **S** | |

### 3.9 The product surface itself

| # | Capability | CLI | MCP | Dashboard home today | Proposed home | Note |
|---|---|---|---|---|---|---|
| 76 | Serve the dashboard | `bernie dash [--listen] [--token]` | — | the page | — | spec §11 |
| 77 | The dashboard over MCP | — | `POST /mcp` on the dashboard | — | — | spec §13.10 |
| 78 | The MCP server on stdio | `bernie mcp [--allow-write]` | — | — | — | spec §13.2 |
| 79 | The user defaults | `bernie config show` / `config set K V` | — | not on the page | **A** | `venv`, `mode`, `engine`, `hf_home`, `set` |
| 80 | The version | `bernie version` | — | not on the page | **A** | |

### 3.10 The contract every front end shares

These twelve are not a feature of one tab: they are the promises the three
front ends make to each other. They have no dashboard home because they are
the *shape* of the dashboard's answers.

| # | Capability | CLI | MCP | Dashboard home today | Proposed home | Note |
|---|---|---|---|---|---|---|
| 81 | One JSON document per command | `--json` on every command | every tool answers `structuredContent` | every `json` link | every `json` link | spec §3.2; AGENTS.md hard rule 4 |
| 82 | The exit codes `0` / `1` / `2` / `3` | every command | — | — | — | spec §3.2; `3` = checks failed |
| 83 | The error envelope `{"error", "exit_code"}` | with `--json` | every tool error | the `failed()` component | the `failed()` component | `dashboard/web/components/cards.js:90` |
| 84 | The global `--venv` and `--root` flags | every command | the `venv` input of the tools that take one | — | — | spec §3.2 |
| 85 | The write guard of the page | — | — | `X-Bernie: 1` + `Sec-Fetch-Site` + `Origin` | — | spec §11; `dashboard/internal/auth/auth.go` |
| 86 | The token mode off loopback | `bernie dash --token` | `Authorization: Bearer` on `/mcp` | `/?token=` bootstrap | — | spec §11, §13.10 |
| 87 | The redaction of secrets | — | every tool result | every answer | every answer | `internal/support/redact` |
| 88 | The confirm token of a write | — | the two-call write gate | — | — | spec §13.5 |
| 89 | The audit trail of the MCP calls | — | `<state>/mcp/audit.jsonl` | — | — | spec §13.9 |
| 90 | The host safety rule: never `sudo` | every root step is printed | `setup_apply` returns the commands | every root step is a command block | every root step is a command block | AGENTS.md hard rule 5 |
| 91 | The read-only default | every inspection command | the write gate is closed by default | every write is a guarded POST | — | spec §13.5 |
| 92 | The paths under the state directory | `<state>/runs`, `<state>/jobs`, `<state>/mcp` | the same | — | **A** | spec §12 |

### 3.11 The counts, checked

| What | Count | Where it was checked |
|---|---|---|
| Top-level CLI commands | 19 | `bernie --help` |
| Runnable CLI paths | 38 | `bernie <path> --help` for every path; 8 of them are routers (`setup`, `recipe`, `config`, `bench`, `tune`, `tune tunableop`, `tune power`, `tune power service`) and `bench` is both |
| CLI paths that answer `--json` with a document | 36 | `logs` and `mcp` accept `--json` and ignore it (a text stream and a protocol) |
| MCP read tools | 19 | `internal/mcp/server_test.go:106` |
| MCP write tools | 6 | `internal/mcp/tools_write.go:105-122` |
| HTTP routes under `/api` | 33 | `dashboard/internal/api/api.go` |
| HTTP endpoints in total | 34 | the 33 above plus `POST /mcp`, mounted in `dashboard/server.go:141` |
| Event streams | 4 | `/api/telemetry`, `/api/runs/{n}/logs`, `/api/jobs/{id}/events`, `/api/bench/{id}/events` |
| Dashboard tabs | 8 | `dashboard/web/js/boot.js` `VIEWS` |
| Recipes | 11 (+ 12 fragments) | `recipes/`; `bernie recipe list --all` answers 11 |
| Doctor categories | 9 | `internal/doctor/doctor.go:32` |
| Doctor checks | 45 fixed ids + 3 per-GPU families (`gpu.ecc.<hip>`, `gpu.supported.<gfx>`, `gpu.hsa_override.<gfx>`) | `internal/doctor/checks_*.go` |
| P2P checklist items | 8 | spec §4.5 |
| Benchmark suites | 3 | `bench/suites/` |
| Onboarding steps | 8 + 3 | spec §3.3 |
| Glossary words | 26 | `internal/glossary/glossary.go` |
| MCP tools with no REST endpoint | 5 | `status`, `recipe_show`, `bench_show`, `tunableop_status`, `setup_apply` |
| REST endpoints with no MCP tool | 10 | `topo/check`, `glossary`, `telemetry`, `gpus/history`, `runs/{n}/log`, `onboarding/{step}/run`, `bench/{id}`, `bench/{id}/events`, `jobs/{id}`, `jobs/{id}/events` |

**Two gaps found while counting.** The API table in spec §11 is missing
`GET /api/bench/suites`, and the spec's claim that the Advanced tab reads
`/api/gpus/history` is wrong: the Dashboard tab reads it, the Advanced tab
does not (`dashboard/web/views/dashboard/` vs `dashboard/web/views/advanced/`).
Both go into the commit plan as spec fixes.

### 3.12 The drift between the spec, the code and the page

The places where the three disagree. They are listed because a
information architecture that ignores them will be built on a contract that
is already wrong. Each one is a one-line fix in a spec or a doc; C2 carries
them. A row marked **closed** names the commit that closed it.

| # | What | The spec says | The code does | Where |
|---|---|---|---|---|
| X1 | `GET /api/bench/suites` | not in the API table | the route exists and the Benchmark tab calls it | `api.go:79`, `views/benchmark/benchmark.js:192` |
| X2 | `/api/gpus/history` | "the Advanced tab reads it" | only the Dashboard tab reads it (`?window=5m`) | spec §11 vs `views/dashboard/dashboard.js:160` |
| X3 | The "Try it" box | `smoke.go` says it is on Setup **and** Dashboard | only Setup calls `POST /api/smoke` | `dashboard/internal/api/smoke.go`, `views/setup/setup.js:288` |
| X4 | The `Host` check | "on a loopback address the `Host` header must be a loopback name" | the Host is checked only when no token is set | `dashboard/internal/auth/auth.go:109` |
| X5 | The random token | "a random token printed at start" | one is generated only for a non-loopback listen address | `internal/cli/cmds/dashcmd/cmd_dash.go:29-33` |
| X6 | The redaction — **closed** (`b289c76`) | "anything sent to the browser goes through `internal/support/redact`" (`dashboard/AGENTS.md` hard rule 6) | the code does: every JSON answer goes through `httpjson.Write`, which redacts, and the SSE writer redacts every line it sends | `dashboard/internal/httpjson/httpjson.go`, `dashboard/internal/sse/sse.go`, `dashboard/internal/api/redact_test.go` |
| X7 | `tune_power_set` | the result is `{"changed", "file", "watts", "gpu_indexes", "selectors"}` | the code also returns `clamped`, and spec §11 says so | spec §13.4 vs `internal/app/power.go:45` |
| X8 | The MCP write gate — **closed** (`10afc78`) | "a mutating tool runs in two calls" and the first call "changes nothing" | the code does: the confirm token is verified before the plan is computed, so a refused call never plans; the plan call still has the side effects of the command it plans (`recipe plan` may write TunableOp rows) | `internal/mcp/mutate.go`, `internal/mcp/confirm.go`, `internal/mcp/write_test.go` |
| X9 | `bench_ab` | listed as a mutating tool | it is mutating but missing from the server's `instructions` text and from the `--allow-write` help | `internal/mcp/server.go:20-31`, `internal/cli/cmds/mcpcmd/cmd_mcp.go:19` |
| X10 | The endpoints MCP does not expose | `docs/mcp.md` lists `runs/{name}/findings` as not exposed | `doctor_log` returns the same matches | `docs/mcp.md`, `internal/mcp/tools_read.go` |
| X11 | The query parameters | `/api/doctor` and `/api/recipes/{id}/plan` have no parameters in the table | `doctor` takes `category` and `venv`; the plan takes `name` | spec §11 vs `dashboard/internal/api/doctor.go:14`, `recipes.go:25` |
| X12 | The endpoints with no page | — | `POST /api/bench` (the synchronous form) and `GET /api/jobs` have no web consumer; the `level` and `q` parameters of `/api/runs/{name}/log` are filtered in the browser instead | `dashboard/internal/api/bench.go:165`, `onboarding.go:75`, `views/logs/tail.js:62-66` |
| X13 | The Chat placeholder | — | its text says "use the endpoint on the Dashboard tab"; the endpoint is rendered on Setup | `views/chat/chat.js`, `views/setup/headline.js:17` |
| X14 | The empty view folders | — | `dashboard/web/views/{runs,gpus,recipes,home}/` are empty directories left over from the old tabs | `dashboard/web/views/` |
| X15 | `bernie --json` with no command | "bare `bernie` runs `status`" | it is **not** status: the usage goes to stderr, stdout gets `{"error": "missing subcommand", "exit_code": 2}`, exit 2 | `internal/cli/cli.go:101` |
| X16 | `help <cmd>` | "explicit help is printed on stdout and exits 0" | the topic is **ignored**; the top-level help is printed | `internal/cli/cli.go`, `dispatch` |
| X17 | `bernie bench` with no arguments | "bench sends requests" | it runs a benchmark with the defaults (it is not help) | `internal/cli/cmds/benchcmd/cmd_bench.go:23` |
| X18 | `up --json` | "step 7: it starts the dashboard" | under `--json` the dashboard is **never** started | `internal/cli/cmds/health/cmd_up.go` |
| X19 | `setup apply --json` | "the CLI, the MCP server and the page print this one record" | `setup apply` still **prompts** on stderr when stdin is a TTY and `--yes` is absent; without a TTY it aborts with exit 1 | `internal/cli/cmds/health/cmd_setup.go:76-79` |
| X20 | `smoke` | "the answer carries the time to the first token, the number of tokens and the output speed" | the human output prints only the server, the model and the answer; `ttft_ms`, `output_tokens` and `tok_s` appear **only** under `--json` | `internal/cli/cmds/serving/cmd_smoke.go:24-30` |
| X21 | `status --steps --json` | "`--steps` prints the checklist" | `--steps` changes only the human text; the JSON is the full document either way | `internal/cli/cmds/health/cmd_status.go` |
| X22 | `bench ab` | marked `no*` in the command table | it starts and stops a server for a recipe arm (spec §10.4 says so; the table does not) | `internal/cli/cmds/benchcmd/cmd_bench_ab.go` |
| X23 | The launch flags | the spec lists a subset | the code also accepts `--engine`, `--profile`, `--no-tunableop` and `--tuning` on `serve`, `recipe plan` and `compose`; `setup` accepts `--venv`, `--image` and `--model` | `internal/cli/cmds/serving/cmd_recipe.go:118` |
| X24 | Positional arguments | not specified | they are **silently ignored** by `hw`, `status`, `doctor`, `topo`, `setup plan`, `setup apply`, `recipe list`, `ps`, `config show`, `tune tunableop *`, `tune power status`, `tune power apply`, `tune power service *`, `bench list`, `bench suites` and `smoke`; `version` ignores all arguments and flags | the `Parse` call sites |
| X25 | The `incomplete` cell state | spec §10.2 names it | the literal `incomplete` appears in no non-test code under `internal/serving` or `internal/app` | spec §10.2 |

**The write gate is not the same on the two front ends.** A REST write on the
page acts immediately behind `X-Bernie: 1` and the origin checks. An MCP
write needs the write flag, a token over HTTP, and the two-call confirm
token. That difference is deliberate (spec §13.10) and it is the one place
where the dashboard is *more* permissive than the agent. It is worth one
sentence in the page's own vocabulary, because a reader of the page and an
agent calling `/mcp` are doing the same thing with different friction.

**The cache behind the answers.** The dashboard holds the host answers for
a few seconds: the host 5 s, the checks 15 s, the runs 2 s, the history 2 s
(`dashboard/internal/api/cache.go:19-31`). The onboarding document is cached
15 s and is served with `stale` while it refreshes. This is the reason the
Setup tab's `Re-check` must ask the server for a fresh read and not only
reload the document (I15).

---

## 4. The proposed organisation

### 4.1 The tabs, and the job of each

Eight tabs, in this order. The order is the order of the life of the box:
set it up, watch it, use it, measure it, wire it, read it, go deeper.

| # | Tab | Its one question | Its one action | Simple (the default) | Details (the toggle) | Advanced tab |
|---|---|---|---|---|---|---|
| 1 | **Setup** | What do I do to make this box serve? | the next step of the path | the headline, the facts, the path, the recipe, the doctor summary | the install plan, the venv, the glossary, the raw documents | the job list, the doctor report in full |
| 2 | **Dashboard** | Is it serving, and how fast? | stop or start | the hero, the six tiles, the two charts, the GPU cards, the machine, how to connect | the per-GPU table, the history window, the power read-out | the run record, the telemetry frame, the raw documents |
| 3 | **Chat** | What will I ask the model? | send one prompt | one prompt box and the reply | the request and the answer as text | — (until it has something) |
| 4 | **Benchmark** | How fast is it, and does it read the whole prompt? | run the suite | the suites, the last result, the comparison, the history | the raw JSON of every endpoint | — |
| 5 | **Topology** | Can the cards copy to each other? | check the readiness | the link classes, the tree, the checklist, the last bandwidth | the pair table, the raw documents | — |
| 6 | **Fleet** | What do the other hosts do? | — | *planned* | — | — |
| 7 | **Logs** | What is running, and why did it fail? | stop the run | the run picker, the findings, the tail | the raw documents, the log as text | — |
| 8 | **Advanced** | What can I inspect and change on this host? | write the power cap | the operator's console: the full per-GPU table, the power write, the doctor report in full, the jobs, the run record | — (the tab is already the deep one) | — |

### 4.2 The one mechanism for "more information"

**Both, with different jobs. This is the answer to the overlap.**

| Mechanism | What it holds | When it is used | Where it lives |
|---|---|---|---|
| **Show details** (per tab) | *More about the same thing on this tab.* The extra rows of the same table, the raw JSON of the endpoints that tab read, the CLI commands that produce them. It never shows a different subject. | the reader wants the numbers behind the sentence they just read | a `details` disclosure at the end of the tab, opened by the rail's `Show details` button or by its own summary |
| **Advanced tab** | *The operator's console.* The things that are not part of any one tab's question: the full per-GPU table, the power write, the doctor report in full, the job list, the run record, and the raw JSON of **every** endpoint. | the reader is operating or diagnosing the host, not reading it | its own tab, the eighth |

The rule that keeps them apart: **a thing belongs in a tab's details when
its question is the tab's question; it belongs in the Advanced tab when its
question is about the host as a machine.** The power cap read is about the
host (Advanced). The output speed is about the run (Dashboard).

What happens to today's overlap:

| Today | Where it goes |
|---|---|
| The Advanced tab is the old first dashboard: the fleet tiles, the per-GPU cards, the exact per-GPU table, the last-minutes charts, the run card, the power caps, the topology, the known problems, the raw documents | **Split.** The exact per-GPU table, the run record, the job list and the power write stay in the Advanced tab and become its permanent job. The fleet tiles, the last-minutes charts and the per-GPU *cards* leave: the cards are on the Dashboard tab today (`views/dashboard/gpus.js`), the charts are on the Dashboard tab today (`views/dashboard/simple.js`), and the topology is its own tab. What remains of the old layout that is unique is the raw document list, which becomes the Advanced tab's **Details** section. |
| The Dashboard tab has a hand-written *simple* view and no details disclosure | **Keep the simple view** (it is the better answer) and **add a details disclosure** to the Dashboard tab: the per-GPU table, the history window, the power read-out, the raw documents of `/api/telemetry`, `/api/hw`, `/api/runs`. |
| The Setup tab has an `Advanced` disclosure (raw documents + CLI commands) | **Rename it to `Show details`** and put it under the same rule: the raw documents of the endpoints the Setup tab read and the CLI commands that produce them. It exists today (`views/setup/advanced.js`) and is already correct in content. |
| The Logs tab has an `Advanced` disclosure | Same: rename to `Show details`, keep the content. |
| The Topology tab has an `Advanced` disclosure | Same: rename to `Show details`, keep the content. |
| The Benchmark tab has an `Advanced` button **and** an `Advanced` disclosure | Same: one disclosure, named `Show details`, no second button in the head. Two controls for one thing is the inconsistency to remove. |
| The Advanced tab has no disclosure | The Advanced tab's own details are the whole tab. It ends with the raw document list. |

**Naming.** The page-wide switch in the rail is `Show details`
(`dashboard/web/index.html:44`). The per-tab disclosures are `Advanced`
(`views/setup/advanced.js:9`, `views/logs/advanced.js:18`,
`views/topology/advanced.js:35`, `views/benchmark/suites.js:225`). **One
word: `Show details`** for the toggle, everywhere. The word `Advanced` is
kept for the **tab** only. That removes the collision between the button and
the tab.

### 4.3 What each tab shows

The SIMPLE view of a tab is *the one question, the one action, and the
numbers that answer the question*. The DETAILS view of a tab is *the same
numbers with their raw form, their reason and their document*.

| Tab | SIMPLE (the default) | DETAILS (the toggle) |
|---|---|---|
| Setup | the headline (`THIS BOX IS SERVING.` / `SETTING UP THIS BOX.` / `THIS BOX IS READY TO SERVE.`) and the lede naming one next action and who does it; the facts strip (GPUs, memory, ROCm, the system) each with its reason; the path (the 8 steps plus the 3 extras, one row each, with the state word, one sentence, the command, and one filled button on the step to do now); the recipe (the best fit, why it fits, the mode switch, the plan with the settings and their reasons); the doctor summary (the count that pass, warn and fail, and the failing ones with their fix) | the install plan, the venv and its source, the glossary of the words on the page, the raw JSON of `/api/onboarding`, `/api/recipes`, `/api/recipes/{id}/plan`, `/api/doctor`, and the four commands that produce them |
| Dashboard | the hero (state word, sentence, endpoint + `Copy URL`, `Stop`, the output speed with its unit); the six serving tiles (Prompt reading, Active requests, First token, KV cache, Prefix cache hits, Draft accepted); two rate charts; one card per GPU; `This machine`; `Connect an app` (curl and Python) | the exact per-GPU table, the history window control, the power read-out per card, the raw JSON of `/api/telemetry`, `/api/hw`, `/api/runs`, `/api/gpus/history` |
| Benchmark | the suites as radio cards with their digest; the last result in one sentence; the table of the cells (median and p95, with the count behind them); the length curve; the comparison of two results with a verdict on every group; the receipt; the history of the saved results; the A/B section | the raw JSON of `/api/bench`, `/api/bench/suites`, `/api/bench/compare`, and the commands `bernie bench list`, `bernie bench suites`, `bernie bench compare A B`, `bernie bench compare A B --markdown`, `bernie bench ab BASE CAND` |
| Topology | the link classes; the PCIe tree; the readiness checklist (8 items with a verdict and a line each); the last bandwidth measurement | the pair table; the raw JSON of `/api/topo` and `/api/topo/check`; the commands `bernie topo` and `bernie topo --check` |
| Logs | the run picker; the findings (cause, fix, doc, line); the tail with the level filter and the search | the raw JSON of `/api/runs` and the log of the picked run; the commands `bernie ps` and `bernie logs NAME` |
| Advanced | the operator's console: the full per-GPU table, the power caps (read and write), the doctor report in full (all 9 categories, all checks), the job list, the run record, and the raw JSON of every endpoint of the page | — (the tab is already the deep one) |

### 4.4 Navigation

**Order and labels.** `Setup`, `Dashboard`, `Chat`, `Benchmark`, `Topology`,
`Fleet`, `Logs`, `Advanced`. The order is unchanged from the page today
(`dashboard/web/js/boot.js:11-20`). The labels are unchanged.

**The two placeholders.** `Chat` and `Fleet` carry the word `Soon` in the
rail and one honest sentence in the panel (`views/chat/chat.js`,
`views/fleet/fleet.js`). They are *not* in the tab bar's keyboard order in
the sense that they are disabled — they are still tabs, and they still hold
their route (`#chat`, `#fleet`), so a link to them works and lands on an
honest page. That is the right behaviour for a planned tab.

**Entry points between tabs.** Today the page has two (`#setup` from the
Dashboard hero, `#logs` from the Setup headline). The proposal adds the
missing ones, all of them one click:

| From | To | The control | Why |
|---|---|---|---|
| Setup, the `bench` extra step | Benchmark | a link on the step row | the step says "the speed is measured" and the reader must be able to go and measure it |
| Setup, the `tuning` extra step | Advanced | a link on the step row | the TunableOp profile is read in Advanced |
| Setup, the `power` extra step | Advanced | a link on the step row | the power caps are in Advanced |
| Setup, the `verify` step | Logs | `Read the logs` (exists) | |
| Setup, the doctor findings | Advanced | a link on the section head | the full report is in Advanced |
| Dashboard, a problem | Setup | a `Fix this` link on the finding | **the missing one.** The Dashboard tab shows no problems today; the doctor findings are on Setup and Advanced. When it shows one, it must link to the Setup step that fixes it. |
| Logs, a finding's fix | the terminal | the copy button on the fix command (exists) | |
| Topology, a failed checklist item | Setup | a link on the item | a failed `access` item is a Setup step |
| Any tab, the raw document | the terminal | the command in the details disclosure (exists) | |

**Empty states.** Every empty state has three parts: what the thing is, the
command that makes one, and a copy button. Today 24 empty states exist and
only 3 carry a command. The rule to enforce: **every empty state carries a
command**. The wording pattern is fixed by
`dashboard/web/components/cards.js:78` (`emptyState(what, next, command)`).

The empty states today that do not carry a command and must be fixed:

| Tab | Empty state | The command it must carry |
|---|---|---|
| Setup | "There is no recipe to serve." | `bernie recipe list --all` |
| Setup | "No recipe fits this host yet." | `bernie recipe list --all` |
| Benchmark | "No result yet." / "Run a suite above to fill this table." | `bernie bench --suite smoke` |
| Benchmark | "Pick two results to compare." | `bernie bench list` |
| Benchmark | "There is no suite to run." | `bernie bench suites` |
| Benchmark | "No saved result yet." | `bernie bench --suite smoke` |
| Logs | "No known failure in this log." | `bernie doctor --run NAME` |
| Logs | "The findings could not be read." | `bernie doctor --run NAME` |
| Logs | "No run has ever served on this host." | `bernie serve <recipe>` |
| Topology | "There is no PCIe tree to show." | `bernie hw` |
| Topology | "There is no matrix to show." | `bernie topo` |
| Topology | "There are no pairs to show." | `bernie topo --bandwidth` |
| Dashboard | "No run is serving." | `bernie serve <recipe>` (exists) |

**The landing tab.** First run: **Setup**. The path is not finished, so
Setup holds the one action. Returning user: **Dashboard**. The box is
serving; the question is "is it OK?". The rule: the landing tab is the tab
that holds the current primary action. When the path is complete, the landing
tab is Dashboard; while it is not, it is Setup. This is computable from
`/api/onboarding` (`next` is empty ⇒ the path is complete), so it needs no
new state.

The route stays explicit: the router keeps the hash. There is no "home"
route; `#setup` is the first tab and the default.

### 4.5 The onboarding path, in the same frame

The reference (`vllm-rdna-dash`) is the product in one page: a headline that
states the situation, one paragraph, four facts, **one big button**, and the
step timeline below it (`~/ui-study/design-dashboard-v2.md` §1.2 and §3.1;
`~/ui-study/collab/01-setup.png`, `27-setup-autopilot-running.png`,
`28-setup-autopilot-done.png`).

bernie has every piece of this but in a different shape: the headline and the
facts are on the Setup tab, the timeline is there, the steps have their
buttons, the jobs report their progress — but there is **no single primary
action that runs the whole path**, and the Setup tab's primary button names
only the *next* step.

The proposal, in the same frame:

| Element | bernie today | The proposal |
|---|---|---|
| The headline | `THIS BOX IS READY TO SERVE.` / `SETTING UP THIS BOX.` / `THIS BOX IS SERVING.` | unchanged; the wording is already right |
| The lede | one sentence naming one action and who does it | unchanged |
| The facts | the facts strip with the reasons | unchanged |
| The one primary action | the *next step* only ("Build the Python environment", "Download the model", "Start the server") | **two states of the same button**: before the path is done it says **"Set up this box"** and runs the whole non-root path (the CLI's `bernie up`); a small line under it says "or go step by step below". Once the path is done it says **"Start the server"** |
| The step timeline | 11 steps, one filled `Run` on the next one | unchanged; the per-step `Run` stays for the step-by-step path |
| Root work | a command block with a copy button and a `Re-check` | unchanged; after the root command, `Re-check` re-reads the host in place |
| The progress sentence | the phase sentence of the job | unchanged, plus the bytes/rate/ETA sentence of the model download (spec §3.3 `progress`) |
| The end state | the headline says `THIS BOX IS SERVING.` | unchanged; the reader lands on the Dashboard tab by following the `Try it` link |

**What the primary button needs.** The endpoint already exists:
`POST /api/onboarding/path/run` walks the non-root steps in order, one job at
a time, and stops at the first step that needs root or that fails
(`internal/onboarding/path.go`; the CLI's `up` path is
`internal/flow/up.go`). What is missing is the page: the button, the
progress, and the `Stop` on the running path
(`POST /api/jobs/{id}/stop`). Effort: **S**.

The reference's `Autopilot` (`POST /api/autopilot`) is the same thing. It
must be in the design as `Set up this box`, with one honest difference: the
reference *runs the root steps itself* through `sudo bash`; bernie never does
(spec: never call `sudo`). bernie's version stops at the root step and shows
the command — which is what the reference's own page promises ("Anything that
needs your password shows up as a command to paste") but does not always do.

---

## 5. The reference: every feature, and where it lands

The reference is `vllm-rdna-dash` by CrispyGinger, HEAD `74fb1d8`
(`~/.cache/minimax-researcher/vllm-rdna-dash`), live demo at
`http://127.0.0.1:18095/`. It is licensed under Apache-2.0, so bernie may
take its markup, CSS and copy; an adapted file names the source in its header
([../design-dashboard.md](../design-dashboard.md) §5).

**Rule for this section: no feature of the reference is dropped.** Every
feature below is either already in bernie, is a change bernie should make, or
is a change bernie should make *differently* — and the reason is written
down.

Legend: **present** = in bernie today (with the place); **partial** = the
core is there, the reference's version has more; **absent** = not in bernie.
Effort: S = under a day, M = a few days, L = a week or more.

The statuses below were re-read from the tree on 2026-10-29, after C1–C8
landed.

### 5.1 The reference's own structure

| # | Reference feature | Reference endpoint / file | Status in bernie | Proposed home | Effort | Commit |
|---|---|---|---|---|---|---|
| R1 | Seven views: setup, dashboard, chat, bench, topology, fleet, logs | `rdna_dash/web/app.js` (hash router) | **present** — eight tabs (Setup, Dashboard, Chat, Benchmark, Topology, Fleet, Logs, Advanced), one extra (Advanced) | the same eight | — | — |
| R2 | A 220 px sticky left rail with glyph + word links, a status pill and a theme button in the footer | `app.css`, `app.js` | **present** — `dashboard/web/index.html:19-47` | the same | — | — |
| R3 | A status pill with a word and an icon | `app.js` (`.pill`) | **present** — `dashboard/web/js/shell.js:248` | the same, plus the spec change of §2.1 | S | C4 |
| R4 | A theme button (dark default, light override) | `app.css` `data-theme` | **present** — `dashboard/web/js/theme.js` (the three choices `Auto`, `Dark`, `Light`; `Auto` follows the system) | the same, with the three choices | — | — |
| R5 | The display headline: italic 900 uppercase with a hard offset shadow | `app.css` `.display` | **present** — `tokens.css` `--font-display` | the same | — | — |
| R6 | The facts strip (label / value / reason) | `app.js` (`.fact`) | **present** — `components/cards.js:47` (`factRow`) | Setup **S** | — | — |
| R7 | A copy button on every command, endpoint and snippet | `app.js` (`.cmd .copy`) | **present** — `components/command.js` | the same | — | — |
| R8 | One filled primary button per view, everything else ghost | `app.css` `.btn.primary` | **present** — the rule is in `dashboard/AGENTS.md` UI rule 4 | every tab | — | — |
| R9 | The step timeline with a per-step state and a per-step action | `app.js` (`.step`) | **present** — `components/timeline.js` | Setup **S** | — | — |
| R10 | The toast | `app.css` `.toast` | **present** — `js/shell.js` `toast` | the same | — | — |
| R11 | The info tooltip (`i` mark) | `app.js` (`.q`) | **present** — `components/cards.js` + `js/shell.js` `openTip` | the same | — | — |
| R12 | The banner (a stale notice with a 3 px left bar) | `app.css` `.banner` | **present** — the stale banner of the Setup tab | the same | — | — |
| R13 | The segmented control | `app.css` `.seg` | **present** — the mode switch of the Setup tab, the history window of the Dashboard tab | the same | — | — |
| R14 | The KPI tile, the hero number, the meter, the area chart, the donut ring, the heatmap matrix, the bar chart | `app.js`, `components/charts.js` in bernie | **present** — `dashboard/web/components/charts.js` | the same | — | — |
| R15 | The radio card (the suite picker) | `app.js` | **present** — the suite cards of the Benchmark tab | Benchmark **S** | — | — |
| R16 | The key/value/why list ("YOUR TUNED SETUP") | `app.js` | **present** — the plan's `Settings and reasons` (spec §7.9) | Setup **S** | — | — |

### 5.2 The features bernie does not have yet

| # | Reference feature | Reference endpoint / file | Status in bernie | Proposed home | Effort | Commit |
|---|---|---|---|---|---|---|
| R17 | **"Set everything up"** — one button that runs the whole path, with the line under it "or go step by step below." | `POST /api/autopilot`, `rdna_dash/jobs.py`, `rdna_dash/web/app.js:291-292` | **present** — the Setup tab has one primary button that runs the whole path (`POST /api/onboarding/path/run`, `dashboard/web/views/setup/path.js`), with the step-by-step path below it | Setup **S**: the primary button, with "or go step by step below" | S | C6 |
| R18 | **Re-check without a reload** — "Re-check this box" | `POST /api/profile/refresh` | **present** — `refreshHost` was restored in `c71f157`; the Setup, Topology, Advanced and Logs Re-check buttons all call `POST /api/refresh` before they reload, so a cable that changed or a check that just resolved shows up without a page reload (`dashboard/web/js/data.js:80`, `dashboard/internal/api/refresh.go`); `loadSetupDocument` repaints the timeline and the headline in place, so the step states update without a navigation | Setup **S**: `Re-check` re-reads the host in place and updates the step states | S | `c71f157` |
| R19 | **The root-command pause** — the autopilot stops and says "Some things need you first" | `rdna_dash/jobs.py` | **present** — the path stops at the step that needs root, shows the exact command and a `Re-check` (`dashboard/web/views/setup/path.js`) | Setup **S**: the path stops at the root step and the headline names it | S | C6 |
| R20 | **The progress sentence with bytes, rate and ETA** ("88% · 53.7 of 61.4 GB · 112 MB/s · about 4s left · file 7 of 14") | `rdna_dash/jobs.py` | **present** — `564cd79` carries the bytes/rate/ETA sentence of the model download through `setupRate`, `setupElapsed` and `setupProgressSentence` (`dashboard/web/views/setup/steps.js`, `internal/setup/download.go`); the Setup tab renders it under the progress bar of the `model` step | Setup **S**: the sentence under the progress bar of the `model` step | S | `564cd79` |
| R21 | **The download speed limit** ("Changes take effect within a second") | `POST /api/download/limit`, `rdna_dash/download.py` | **present** — `bce94ef` added `POST /api/download/limit`; `564cd79` and the Setup `advanced.js` carry the field on the `model` step (`internal/setup/limit.go`, `dashboard/internal/api/download.go`, `dashboard/web/views/setup/advanced.js` `setupDownloadLimit`); the value is in force within a second | Setup **D**: a field on the `model` step | M | `bce94ef`, `564cd79` |
| R22 | **The Chat tab** | `POST /api/chat`, `rdna_dash/serve.py` | **present** — the Chat tab has one prompt box and one streamed answer (`POST /api/chat`, `dashboard/web/views/chat/chat.js`) | Chat **S**: one prompt box, one streaming reply | M | C8 |
| R23 | **The Fleet tab** — the other boxes, a share button, a remove button | `GET /api/fleet`, `POST /api/fleet/remove`, `rdna_dash/profile.py` | **present** — `fd5f10d` and `f4a1ded` made the Fleet tab real: `GET /api/fleet` lists the other hosts, `POST /api/fleet/remove` forgets one, the share button copies the redacted report (R24), and the Fleet tab says "the list of the other bernie hosts and their state" (`internal/app/fleet.go`, `dashboard/internal/api/fleet.go`, `dashboard/web/views/fleet/fleet.js`) | Fleet **S**: the list of the other bernie hosts and their state | L | `fd5f10d`, `f4a1ded` |
| R24 | **The redacted share report** ("Copies a report with no hostname, username or tokens") | `POST /api/profile/share` | **present** — `bernie doctor --report` prints the redacted report; `GET /api/report`, the MCP tool `report` and `bernie doctor --report` carry the same document (`internal/app/report.go`, `dashboard/internal/api/report.go`); the Advanced tab has Copy report / Download report | Advanced **A**: a `Copy a redacted report` button | M | C9 |
| R25 | **The server start / stop / restart from the page** | `POST /api/server/start`, `/stop`, `/restart` | **present** — `POST /api/runs`, `DELETE /api/runs/{name}`; the Dashboard hero has `Stop` and `Start a run` | Dashboard **S** | — | — |
| R26 | **The server log pane** | `GET /api/server/log` | **present** — the Logs tab | Logs **S** | — | — |
| R27 | **The profile refresh / host probe** | `POST /api/profile/refresh`, `rdna_dash/profile.py` | **present** — `POST /api/refresh` drops every cached answer of the host; the Re-check buttons of Setup, Topology and Advanced call it before they reload, so a cable that changed or a check that just resolved shows up without a page reload (`dashboard/internal/api/refresh.go`) | Setup **S**: the `Re-check` button | S | C9 |
| R28 | **The preferences panel** | `GET /api/prefs`, `rdna_dash/server.py` | **present** — `GET /api/prefs` answers the `bernie config show` document with the source of each value; `PUT /api/prefs` writes through the same `Config.SetKey` and the same redaction; the MCP tool `prefs_get` carries the same document; the Advanced tab has a row per key with its source, the resolved value and a Save form (`internal/app/prefs.go`, `dashboard/internal/api/prefs.go`, `dashboard/web/views/advanced/prefs.js`) | Advanced **A**: the user defaults (`venv`, `mode`, `engine`, `hf_home`) | M | C9 |
| R29 | **The attach action** (attach to the running server) | `POST /api/attach` | **present** — `bce94ef` added `POST /api/attach` and the runner's attach path (`internal/serving/runner/attach.go`, `dashboard/internal/api/attach.go`); `bernie serve --attach URL` and the MCP tool `attach` carry the same flow. bernie still starts its own runs by default; attach is the explicit opt-in | Advanced **A**: attach to a server bernie did not start, when the reader asks for it | S | `bce94ef` |
| R30 | **The benchmark start / cancel / delete** | `POST /api/bench/start`, `/cancel`, `/delete`, `rdna_dash/bench.py` | **partial** — `564cd79` and `bce94ef` added `POST /api/bench/cancel` (Stop on the running suite, Benchmark **S**) and `bernie bench delete` for saved results; bernie does not expose delete on the page (a saved result is an artefact — see R49). The Start, Cancel and Delete land on the same tool that already 409s while one runs | Benchmark **S**: a `Stop` on the running suite; the history keeps its results (no delete on the page — a saved result is an artefact) | M | `564cd79`, `bce94ef` |
| R31 | **The models list** | `GET /api/models/` | **present** — `bernie models` lists the ids the running server's /v1/models returns and the repos the Hugging Face cache on this host holds; `GET /api/models`, the MCP tool `models` and the CLI command carry the same document; the Chat tab shows "Model: <id>" from /api/models with the recipe name as fallback (`internal/app/models.go`, `internal/cli/cmds/inventory/cmd_models.go`, `dashboard/internal/api/models.go`) | Dashboard **D**: the model the run serves | S | C9 |
| R32 | **The topology bandwidth heatmap** | `GET /api/topology/bandwidth`, `rdna_dash/topology.py` | **present** — `3401be3` saves the measurement (`bernie topo --bandwidth`) and the Topology tab draws the matrix as a GB/s heatmap with the cells coloured by the measured throughput (`internal/machine/topo/bandwidth.go`, `dashboard/web/views/topology/matrix.js`) | Topology **D**: the matrix, drawn as a heatmap | M | `3401be3` |
| R33 | **The install scripts** (`install.sh`, `install-host.sh`) | `~/.cache/minimax-researcher/vllm-rdna-dash/install.sh` | **present** — `scripts/install.sh` installs the binary; flags `--version`, `--dir`, `--dry-run`; never calls sudo silently; prints `bernie up`; `scripts/test-install.sh` covers it against a fake release server (Linux only, `make installtest`) | the host install is the Setup path (the `access`, `rocm`, `container` steps) | S | C7 |
| R34 | **The Dockerfile and compose.yml of the dashboard itself** | `Dockerfile`, `compose.yml` of the reference | **present** — `deploy/docker/Dockerfile` builds the image (`make image`); `deploy/compose/compose.yml` (docker) and `compose.podman.yml` (rootless podman) run it on amd64 and arm64; both validate with `docker compose config -q` on docker 29.8.1 and `podman compose config` on 1.6.0 | the binary's dashboard is the surface; the container is for ops isolation | S | C7 |
| R35 | **The example recipes and the `recipes/` JSON files** | `examples/`, `recipes/*.json` of the reference | **present** — bernie's `recipes/` is YAML with 12 recipes and the same families (flashnext, qwen 27B, deepseek) | Setup **S** | — | — |
| R36 | **The ECC and power tools** (`tools/ecc.sh`, `tools/power.sh`) | `tools/` of the reference | **present** — `af4d646` brought the ECC and power tools to the Advanced tab: the per-GPU ECC table, the `bernie tune power set/apply/service` controls, and the root-command blocks (`dashboard/web/views/advanced/ecc.js`, `dashboard/web/views/advanced/power.js`) | Setup **S** (the `access` and `power` steps) | — | `af4d646` |
| R37 | **The log doctor** | `rdna_dash/logdoctor.py` | **present and stronger** — bernie's catalog has 61 rules with an id, a cause, a fix and a doc link (spec §5.3) | Logs **S** | — | — |
| R38 | **The demo mode** | `rdna_dash/demo.py` | **present** — `5a37679` added `bernie dash --demo`: the dashboard serves a captured host and a fake run, with no GPU. The reader sees the full UI without hardware; the banner says so (`internal/app/demo.go`, `dashboard/web/index.html`) | Dashboard **S**: a `--demo` flag that runs the page with a captured host | L | `5a37679` |
| R39 | **The blocked state**: "Fix the items below first, then Re-check." and the headline "A few things first." | `rdna_dash/web/app.js:278`, `:292` | **present** — `651e7b9` carries the headline "A FEW THINGS FIRST." and the lede "Fix the items below first, then press Re-check." on the blocked Setup state (`dashboard/web/views/setup/headline.js:50`) | Setup **S** | S | `651e7b9` |
| R40 | **"Needs you"** — a section of the checks that only the operator can do, above the step timeline | `rdna_dash/web/app.js:296` | **present** — `7450f90` added the "Needs you" section above the path: the root steps, with the exact command, the copy button and the Re-check (`dashboard/web/views/setup/needs-you.js`, `dashboard/web/views/setup/setup.js`) | Setup **S**: a "Needs you" block above the path | M | `7450f90` |
| R41 | **"Other good options"** — the recipes that also fit, as buttons, each with the reason in its tooltip | `rdna_dash/web/app.js:301` | **present** — the recipe chooser with the fit and the reason | Setup **S** | — | — |
| R42 | **"Show the exact launch command"** — the launch command and the generated `compose.yml`, in one disclosure | `rdna_dash/web/app.js:302` | **present** — `6f61c26` added the "Show the exact launch command" disclosure: the host command and the generated `compose.yml` when the plan is container mode, in one panel (`dashboard/web/views/setup/launch-disclosure.js`) | Setup **D**: the launch command and the compose file of the container plan | M | `6f61c26` |
| R43 | **The settings form** — model and layout, how to install, PLE table, port, max context, install folder, name for this box, "share the model on my network", and "Save and re-tune" | `rdna_dash/web/app.js:372-385` (`changeForm`) | **present** — `4d0d981` added the structured settings form: port, max context length, run name and model path/id are four named fields that write the same flags the CLI accepts (`args.port=N`, `args.max-model-len=N`, `--name`, `model.source=PATH`). The free-text env editor is gone. A "Save the mode as default" ghost button appears next to the mode switch only when `bernie config` knows the `mode` key (checked against `/api/prefs`); it persists through `PUT /api/prefs`. Recipe inputs stay structured (`dashboard/web/views/setup/recipes.js`, `dashboard/web/views/setup/plan-adjust.js`) | Setup **D**: the overrides form; the port and the max context are `--set` values | M | `4d0d981` |
| R44 | **The GPU selection** (`HIP_VISIBLE_DEVICES`, "Which GPUs vLLM is allowed to use, by index") | `rdna_dash/web/app.js:34` | **present** — `4d0d981` added the GPU selection: one checkbox per detected GPU (HIP index, card name, VRAM), in a "The GPUs and the settings of this plan" disclosure on the Setup tab. The selection writes `env.HIP_VISIBLE_DEVICES=idx,idx,...` via `--set`, the same mechanism spec §7.7 documents; the plan and the launch command rebuild with the chosen set (`dashboard/web/views/setup/plan-adjust.js` `setupPlanGPUSelector`) | Setup **D**: the GPU selection of the plan | M | `4d0d981` |
| R45 | **The "Cancel" button on a running job and on the autopilot** | `rdna_dash/web/app.js:190`, `:195`, `:328` | **present** — the Setup tab carries a `Stop` on the running path (`setupStopPath`, `POST /api/jobs/{id}/stop`) and a `Stop` on every running step (`setupStopStreams`, `dashboard/web/views/setup/path.js`, `dashboard/web/views/setup/steps.js`); both end the job in place and update the timeline without a reload | Setup **S**: a `Stop` on the running step and on the running path | S | `b3b07d7` |
| R46 | **"Compare up to four runs"** — the comparison picks up to four saved runs | `rdna_dash/web/app.js:637`, `:668` | **partial** — bernie compares exactly two (`bench compare A B`) | Benchmark **S**: keep two. The verdict of spec §10.3 is defined for a pair; four would need a new rule | — | — (decision) |
| R47 | **The benchmark table of ten columns** (Case, Prompt tok, TTFT p50, p95, Prefill tok/s, Decode tok/s, ITL p95, All users tok/s, Recall, Errors) and the **fingerprint line** | `rdna_dash/web/app.js:656-657` | **present** — the metric table of `dashboard/web/components/metrics.js` has the same measures (spec §10.1) | Benchmark **S** | — | — |
| R48 | **The topology bandwidth heatmap** and the **"Bare metal" / "Not a VM."** check | `rdna_dash/web/app.js:726` | **partial** — `ac42b7b` added the `os.virtualization` doctor check (bare metal / docker / podman / kvm / vmware / microsoft / xen / oracle / qemu), with the GPU passthrough hint when a VM is detected. The Setup facts strip reads the same check ("Host" row). The heatmap part is covered by R32 (`3401be3`) | Topology **D**: the matrix as a heatmap; the VM check in the doctor's `os` category | M | `ac42b7b` (R48), `3401be3` (R32) |
| R49 | **"Delete this run?"** — a saved benchmark result can be deleted | `rdna_dash/web/app.js:212` | **absent** — bernie keeps every result. `bernie bench delete` exists for the CLI (`564cd79`, `bce94ef`), but the page does not expose it: a saved result is the evidence of a measurement and the owner has not ordered a delete button | **not copied.** A saved result is the evidence of a measurement | — | — (decision) |

### 5.3 The reference's features bernie deliberately does not copy

These are listed so that "no feature dropped" is not read as "no judgement
exercised". Each one has a reason that is already written down in this
repository.

| Reference feature | Why bernie does not copy it | Where the reason is written |
|---|---|---|
| The plan embeds shell scripts and runs `curl … \| sudo bash -s` | bernie never calls `sudo` and never runs a root command; root steps are printed as commands | spec §2 principle 5, `dashboard/AGENTS.md` hard rule 5 |
| The chat page and the fleet view were "not planned" in the earlier bernie study | the owner has since ordered the tabs to be present; see D2 | `~/ui-study/design-dashboard-v2.md` §2.2 N8 |
| The font CDN | bernie forbids a CDN; the three families are self-hosted woff2 in the binary | `dashboard/AGENTS.md` hard rule 3, [tokens.md](tokens.md) |
| Polling every second | bernie uses one SSE at 1 Hz for every viewer | spec §11, `dashboard/web/js/data.js` |
| The optional login, the dashboard container that mounts the Docker socket | bernie requires a token off loopback and refuses a foreign `Origin` | spec §11, §13.10 |
| The attach action | bernie owns the runs it starts; attaching to a foreign server is a different product | spec §14 |
| Deleting a saved benchmark result | a saved result is the evidence of a measurement; bernie keeps it | spec §10 |

---

## 6. Chat and Fleet: the two placeholder tabs

The owner's order: the tabs are there, and they are not usable yet. This
section says what they become and what it costs.

### 6.1 Chat

The lean version: **one prompt box, one reply, streamed.** The reference's
chat is a transcript with a meta line per message, a system prompt and a
parameter panel (`~/ui-study/design-dashboard-v2.md` §1.4,
`collab/11-chat-live.png`). bernie needs none of that to answer the tab's
question.

| Piece | What | Where |
|---|---|---|
| The prompt box | one `<textarea>` and a `Send` button | Chat **S** |
| The reply | the streamed text, with the time to first token and the tokens per second under it | Chat **S** |
| The model | the model of the running run | Chat **D** |
| The request as text | the exact JSON body that was sent | Chat **D** |

**What it needs.** `POST /api/chat`: the shape is in
`docs/design-dashboard.md` §10 (`{"run", "model", "stream", "messages",
"temperature", "max_tokens", "reasoning"}`, `text/event-stream`, 400 when the
run is not ready, 401 without the token, 403 without `X-Bernie`). The backend
proxies the OpenAI API of the running run, to a loopback target only — the
rule `/api/smoke` already follows (spec §11). **Effort: M**: the endpoint,
the streaming writer (the SSE writer of `dashboard/internal/sse/` exists) and
the page.

A tab that says "planned" for a release and a half teaches the reader that
bernie promises and does not deliver; a one-prompt Chat is the smallest
honest version, and `/api/smoke` already answers most of it.

### 6.2 Fleet

The lean version: **the list of the other bernie hosts and what they are
doing** (the reference's `GET /api/fleet` and `POST /api/fleet/remove`).

| Piece | What | Where |
|---|---|---|
| The list of hosts | the hosts bernie knows, with their state and their last answer | Fleet **S** |
| The share button | the redacted report of this host, copied (R24) | Fleet **S** |
| The remove button | forget a host | Fleet **S** |

**What it needs.** A place to declare the other hosts (`bernie config set
fleet`, a list of `name=url` pairs) and a read of each host's
`/api/telemetry` over the network — which means the other host's dashboard
token. That is a security surface bernie does not have today (the token mode
is per-process and the token is printed once). **Effort: L**, and it opens
the question *how does one host hold another's token?*, which the owner
should answer before any code is written.

**Recommendation.** Ship the **redacted report** (R24) first, in the Advanced
tab, and keep the Fleet tab a placeholder until the token question has an
answer. That gets the reference's most useful fleet feature — the one-click
report that is safe to paste into an issue — without inventing a federation
model in a design document.

---

## 7. The design coherence rules, and what breaks today

The rules already exist, in three places:

| Where | What it holds |
|---|---|
| [../design-dashboard.md](../design-dashboard.md) | the visual contract: the shell, the rail, the tab bar, the pill, the Advanced panel, the loading/empty/error states |
| [tokens.md](tokens.md) | the palette, the type, the space, the radii, the motion; the measured contrast of every pair |
| [components.md](components.md) | the anatomy of every component, its states and the data it takes |
| [../../dashboard/AGENTS.md](../../dashboard/AGENTS.md) | the rules of the page and the check that proves each one |

### 7.1 The concrete inconsistencies in the built tree

Measured by reading the files of the built binary (`bin/bernie`) and by
running it against the captured host fixture, on 2026-10-29:

```bash
tar xzf internal/support/fixture/testdata/par1-cs25.tar.gz -C /tmp/bernie-ia-fixture
./bin/bernie --root /tmp/bernie-ia-fixture/par1-cs25 dash --listen 127.0.0.1:17956
```

The fixture is the reference box (2× gfx1100 W7800 + 8× gfx1030 V620). The
dashboard answered `/api/onboarding` with the path (`hardware` done,
`access` blocked, `gpu.kfd` and `gpu.render` failing with
`sudo modprobe amdgpu`) and `/api/hw` with the inventory of the ten cards.
The rest of the list below is read out of the source of the tabs.

| # | What | Where | The rule it breaks | The fix |
|---|---|---|---|---|
| I1 | The Logs tab has its own section heading: `h2.lg-section-title` in `600 13px/1 --font-body` with `.05em` letter-spacing, over its own `.lg-section` rule (`24px` padding), while Setup, Topology and Benchmark use the shared `h2.section-label` in `800 13px/1.2 --font-display` with `.14em` over `.section` (`var(--space-6)`) | `views/logs/logs.css:10-11` vs `css/layout.css:251-263` | one shape for a section label (tokens.css `--section-label`) | one `sectionLabel(text)` helper in `components/cards.js`; the Logs sheet loses its private rule |
| I2 | The label of a section is uppercase in the DOM in Setup, Topology and Logs (`"THE PATH TO A SERVING MODEL"`) and **sentence case** in Benchmark (`"The suites"`, `"The result"`, `"The comparison"`, `"The history"`, `"An A/B run"`). The CSS uppercases both (`text-transform: uppercase`), so the page looks the same but a screen reader reads "The suites" | `views/setup/setup.js:72`, `views/topology/topology.js:33`, `views/logs/logs.js:47-49`, `views/benchmark/benchmark.js:154-163` | the DOM text and the visible text are the same | the same helper decides the case, and the DOM carries the uppercase form in every tab |
| I3 | The toggle is called `Show details` in the rail and `Advanced` in four tab disclosures | `web/index.html:44` vs `views/setup/advanced.js:9`, `views/logs/advanced.js:18`, `views/topology/advanced.js:35`, `views/benchmark/suites.js:225` | one word for one thing | rename every disclosure summary to `Show details`; keep `Advanced` for the tab |
| I4 | The Benchmark tab has **two** controls for the same disclosure: an `Advanced` button in the head (`aria-expanded`) and an `Advanced` summary in the panel | `views/benchmark/benchmark.js:135` and `views/benchmark/suites.js:224` | one control per thing | the button in the head goes; the summary stays |
| I5 | The Setup tab's head carries an `Advanced` button in **five** of its six states (`views/setup/headline.js:75`, `:85`, `:107`, `:111`, `:115`). It calls `setupToggleAdvanced`, which calls the **page-wide** `toggleAdvanced`, so it opens the disclosures of the other tabs too | `views/setup/headline.js:75-115`, `:264-266` | a tab control acts on its own tab | the head button opens the Setup disclosure only |
| I6 | The pill vocabulary differs per tab: the Dashboard uses `Serving` / `Starting` / `No run` (title case), the rail uses `serving` / `starting` / `not answering` / `idle` / `stopped` (lower case), the topology uses `ok` / `warn` / `fail` / `skip` (status words), the checklist uses `done` / `todo` / `running` / `blocked` / `skipped` (state words) | `views/dashboard/simple.js:63`, `js/shell.js:259`, `views/topology/topology.js:22`, `components/timeline.js:9` | a state is a word, never a colour — and the same state has the same word | one `statePill(kind, word)` in `components/cards.js`; the four call sites pass the word of their own list |
| I7 | Two icon maps for the same status: `DASH_PILL_ICON` has 7 kinds (`ok warn serious fail skip info running`), `TOPO_PILL_ICON` has 4 (`ok warn fail skip`) | `views/dashboard/dashboard.js:47`, `views/topology/topology.js:22` | one component per concern | one map, in `components/cards.js` |
| I8 | The empty state vocabulary is 24 different sentences, and only 3 carry a command | `views/**`, `components/**` (24 calls to `emptyState`) | an empty state teaches: what, then what to do | one `emptyState(what, next, command)`; every call carries a command (see the table in §4.4) |
| I9 | The buttons that re-read the host are called four different things: `Re-check` (Setup), `Check readiness` (Topology), `Re-check the links` (Advanced), `Try again` (the error state) | `views/setup/headline.js:80`, `views/topology/topology.js:44`, `views/advanced/topology.js:9` | the same action has the same name | `Re-check` everywhere; `Try again` stays for a failed request |
| I10 | The primary action of the Dashboard is a link (`<a class="btn.primary" href="#setup">`) while every other tab uses a `<button class="primary">` | `views/dashboard/simple.js:27` vs `views/setup/headline.js:74` | a control is a button or a link, and a primary action is a button | the Dashboard's primary is a button that starts a run, or the link stays a link but is not styled `primary` |
| I11 | The Dashboard tab has no details disclosure, while Setup, Logs, Topology and Benchmark all have one | `views/dashboard/**` | one disclosure per tab | add the Dashboard disclosure (per §4.3) |
| I12 | The Advanced tab reads no `/api/gpus/history`, but spec §11 says it does | `views/advanced/**` vs spec §11 | the spec is the contract | fix the spec (the Dashboard tab reads it) |
| I13 | The API table of spec §11 has no `GET /api/bench/suites`, but the route exists and the Benchmark tab calls it | spec §11 vs `dashboard/internal/api/api.go:79` | the spec is the contract | add the row |
| I14 | The tab bar's counts exist for Setup, Dashboard and Topology only; Benchmark and Logs have no count even though they have failures (a failed bench, a finding) | `web/index.html:26-34`, `js/data.js:353`, `:362`, `:363` | a tab shows the number of things that wait | add the count for Logs (the findings) and for Benchmark (the failed runs) |
| I15 | The Setup tab's `Re-check` button reloads the onboarding document but does not re-read the host; the reference's `Re-check this box` does. The dashboard caches the host answers (the host 5 s, the checks 15 s, the runs 2 s), so a reload alone can answer with the old host | `views/setup/headline.js:80`, `views/setup/setup.js` (`loadSetupDocument`), `dashboard/internal/api/cache.go:19-31` | the button says what it does | the button asks the server for a fresh read of the host (the doctor and the setup plan), then the document |
| I16 | The tab order of the rail and the keyboard shortcuts disagree: the shortcuts are `g` then `s d b t l c`, which is Setup, Dashboard, Benchmark, Topology, Logs, Advanced — Chat and Fleet have no shortcut | `js/shell.js:140-152` | every tab is reachable | add `g` then `h` for Chat and `g` then `f` for Fleet, or leave the two planned tabs without one on purpose and say so |
| I17 | The same disclosure behaves three different ways: the Setup summary calls the **page-wide** switch (`toggleAdvanced`), the Topology and Logs summaries open only their own panel, and the Benchmark panel is opened by a **button in the head** | `views/setup/advanced.js:16-21`, `views/topology/advanced.js:35`, `views/logs/advanced.js:18`, `views/benchmark/benchmark.js:135`, `views/benchmark/suites.js:224-226` | one control, one effect | every disclosure is a plain `details` that opens its own panel; the page-wide switch in the rail opens and closes them all, and a `MutationObserver` (`js/boot.js:296`) keeps the state right when a tab's files arrive |
| I18 | The rules that reach the page are not the same as the rules that reach the agent: the dashboard's `dashboard/AGENTS.md` hard rule 6 says everything sent to the browser goes through `internal/support/redact`, but only seven handlers do; the four event streams make no redaction call at all | `dashboard/internal/httpjson/httpjson.go:13` vs `:45`; `dashboard/internal/sse/sse.go` | one redaction rule for the browser and the agent | one `httpjson.Redacted` for every handler and a redaction step in the SSE writer |

### 7.2 The rules that already hold, and must keep holding

The rules of the page live in [../../dashboard/AGENTS.md](../../dashboard/AGENTS.md)
and the visual contract in [../design-dashboard.md](../design-dashboard.md).
The ones this file leans on hardest:

- One filled button per tab; every state is an icon and a word.
- Every number has a unit and a range; a missing number is `–`.
- Every command, URL and document has a copy button that copies exactly what
  is shown.
- Every tab opens with a headline and a lede; the headline is `aria-live`.
- One question per tab; every field on the page exists in the `--json`
  output with the same name.
- The page is small: ≤ 430 KB of HTML+CSS+JS, ≤ 24 requests on the first
  paint, ≤ 240 points per chart.

---

## 8. The commit plan

Nine commits. Each is shippable on its own: the binary builds, `make check`
passes, the page works, and each ends with a browser check. The commits are
ordered so that every one of them is safe to stop after.

| # | Commit | Files | What moves | Acceptance check |
|---|---|---|---|---|
| **C1** | `docs: record the information architecture and the inventory` | `docs/design/ia.md` | this file | the file reads; every row of the inventory names a command, a tool or a route that exists |
| **C2** | `spec: fix the drift the inventory found` | `docs/spec.md`, `docs/mcp.md`, `docs/design/setup.md`, `internal/mcp/server.go`, `internal/cli/cmds/mcpcmd/cmd_mcp.go` | the 25 rows of §3.12: the missing `GET /api/bench/suites` row, the Advanced tab and `/api/gpus/history`, the redaction rule, the `clamped` field, the query parameters, `bernie --json` alone, `help <cmd>`, `up --json`, `setup apply --json`, `smoke` in human output, `bench ab`'s host effect, the launch flags, the step titles of [setup.md](setup.md) | `make check`; the API table of spec §11 has 33 rows and matches `dashboard/internal/api/api.go` line for line; the MCP catalog of spec §13.4 matches `internal/mcp/tools_read.go` and `tools_write.go`; `bernie status --steps` prints the titles of §2.3 and they match [setup.md](setup.md) |
| **C3** | `web: one section label, one pill, one empty state` | `dashboard/web/components/cards.js`, `dashboard/web/views/**` | the `sectionLabel()` and `statePill()` helpers; every tab uses them; every `emptyState` carries a command | no `h2.lg-section-title` and no `h2.section-label` outside `cards.js`; every `emptyState(` call has three arguments; `make check` |
| **C4** | `web: call the disclosures "Show details"` | `dashboard/web/index.html`, `dashboard/web/views/setup/advanced.js`, `views/logs/advanced.js`, `views/topology/advanced.js`, `views/benchmark/suites.js`, `dashboard/web/js/shell.js` | the summary of every disclosure reads `Show details`; the head `Advanced` button of the Benchmark tab goes; the Setup head button opens its own disclosure | the rail button and the four disclosures agree; no string `Advanced` outside the tab name; `make check` |
| **C5** | `web: give the Dashboard tab its details` | `dashboard/web/views/dashboard/*`, `dashboard/web/components/cards.js` | the Dashboard details disclosure: the per-GPU table, the history window, the power read-out, the raw documents | the Dashboard tab has a `Show details` disclosure that opens the four documents; `make check` |
| **C6** | `dashboard: one button that sets the box up` | `dashboard/web/views/setup/*` | the Setup primary button reads `Set up this box` and calls the endpoint that already exists, `POST /api/onboarding/path/run`; the root step pauses the path; a `Stop` on the running path calls `POST /api/jobs/{id}/stop`; the recommendation and the primary action come from the `recommendation` and `primary` fields of the onboarding document | the button runs the whole path; the root step stops it with the command; the button and the headline agree; `bernie up` and the endpoint agree; `make check` |
| **C7** | `dashboard: the operator's console is the Advanced tab` | `dashboard/web/views/advanced/*`, `dashboard/web/views/dashboard/*`, `dashboard/web/css/advanced.css` | the old layout leaves the Advanced tab; the per-GPU table, the power write, the doctor report in full and the job list stay; the fleet tiles and the last-minutes charts leave (they are on the Dashboard tab) | the Advanced tab holds the four operator things and no duplicate of the Dashboard; `make check`; a browser check of both tabs |
| **C8** | `dashboard: the Chat tab sends one prompt` | `docs/spec.md`, `dashboard/internal/api/chat.go`, `dashboard/web/views/chat/chat.js` | `POST /api/chat` (streamed, loopback only); the prompt box and the reply | a prompt comes back with the time to first token; an unready run answers 400; `make check` |
| **C9** | `dashboard: the redacted report, and the preferences` | `docs/spec.md`, `internal/app/`, `dashboard/internal/api/`, `dashboard/web/views/advanced/*` | the redacted report (the reference's share button); the preferences panel (the config defaults) | the copied report has no hostname, no username and no token; `make check` |

### 8.1 The risks

| Risk | Where | How it is held |
|---|---|---|
| The Advanced tab loses the old layout that some readers know | C7 | the per-GPU table, the power write, the doctor report and the jobs stay; the old route `#classic` keeps working; the raw documents at the end of the tab are what the old layout's raw list was |
| The Dashboard tab grows too heavy | C5, C7 | the budget in `dashboard/AGENTS.md`; the details are a disclosure, and the raw documents are read when it opens, not on load |
| `POST /api/onboarding/run` duplicates `bernie up` | C6 | the endpoint calls the same `internal/flow` code the CLI calls; the CLI keeps the confirmation, the page has its own button |
| The spec drifts again | C2 and every commit | the rule of `AGENTS.md` §3.1: a change to a command, a flag, a JSON field or a path changes the spec in the same commit |
| The Chat endpoint becomes an open proxy | C8 | loopback targets only, the same rule as `/api/smoke`; the token and `X-Bernie` rules of spec §11 |
| The Fleet tab needs a cross-host token | — | the recommendation of §6.2: the Fleet tab stays a placeholder until the owner answers the token question |

### 8.2 The decisions that belong to the owner

D1 to D5 are at the top of this file, with their recommendations and the
cost of changing each one.

---

## 9. Where the facts come from

Every claim above names its source inline. The primary ones are the code and
the contracts, read in this order: `bernie --help` and `bernie <path> --help`
for the commands; `internal/cli/`, `internal/mcp/`, `dashboard/internal/api/`
and `dashboard/web/**` for the three front ends; `docs/spec.md` §3, §3.3, §5,
§7–§11 and §13 for the contract; `dashboard/AGENTS.md` and
`docs/design-dashboard.md` for the rules of the page; and the reference
`vllm-rdna-dash` at HEAD `74fb1d8` (its live demo at
`http://127.0.0.1:18095/`) with `~/ui-study/notes.md` and
`~/ui-study/design-dashboard-v2.md` for the study of it.

The dashboard was also run against the captured host fixture
(`internal/support/fixture/testdata/par1-cs25.tar.gz`); the command is in
§7.1.
