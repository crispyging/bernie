# bernie specification

Status: draft 1 (2026-10-28). This document is the contract. Code, tests and
the dashboard follow it. Change this file in the same pull request as the
behavior.

Writing rules for this file: short sentences, one fact or one instruction per
sentence, active voice, defined terms only (see [Terms](#terms)).

## 1. Purpose

bernie helps an operator run vLLM on AMD ROCm GPUs. It replaces the
serve scripts and `.env`
recipes of the `opengfx1030/vllm-rdna` fork with one tool.

bernie does these tasks:

1. Inspect the host: hardware, PCIe topology, ROCm, libraries, containers.
2. Report what is missing, and give the fix for the host's package manager.
3. Install the software stack (host venv or container image).
4. Load recipes and turn one recipe into one vLLM launch.
5. Start, stop, list and log vLLM servers (host process or container).
6. Apply tuning: TunableOp rows, GPU power cap.
7. Benchmark an OpenAI-compatible server.
8. Show all of the above in a web dashboard.
9. Expose the same operations to agents through MCP ([§13](#13-model-context-protocol)).

## 2. Principles

- **CLI first.** Every feature is a CLI command. The dashboard and the MCP
  server call the same Go packages. Neither has a feature that the CLI does
  not have.
- **Machine-readable.** Every command accepts `--json` and then writes exactly
  one JSON document to stdout. Agents and scripts use this mode.
- **One static binary.** The binary embeds the built-in recipes, the TunableOp
  rows, the tuned-MoE configs, the device tables and the dashboard assets. The
  target host needs no Python, Node or Go to run bernie.
- **Read-only by default.** Inspection commands never change the host.
  Commands that change the host print the plan first. bernie never runs
  `sudo`. Steps that need root are printed as commands for the operator.
- **Data over code.** Model and hardware knowledge lives in recipes (YAML).
  Go code holds only generic mechanics.
- **No `/tmp`.** All files go under the XDG paths in [§12](#12-files-and-paths).
- **Docker optional.** Every feature works with Docker or Podman and
  without any container engine. Every recipe runs in host mode. Container
  mode is an option a recipe adds with `container.image`. A host without an
  engine is a normal host: container checks are `skip`, never `fail`, and
  `up` selects host mode.
- **Never break other platforms.** A recipe declares its GPU architectures.
  bernie refuses to launch a recipe on GPUs that do not match.
- **Many devices.** Support covers AMD RDNA GPUs from the Radeon PRO V620 and
  RX 6800 XT (RDNA2, gfx103x) to the RX 9070 XT (RDNA4, gfx12) and later.
  Device facts live in data tables, one file per GPU generation
  (`internal/machine/gpu/devices/rdna2.yaml`, `rdna3.yaml`, ...; see
  [§4.4](#44-device-tables)). Tuning data and recipes are keyed by
  architecture. No Go code names a device.
- **Lean, readable, segmented.** See [§2.1](#21-structure-rules).

### 2.1 Structure rules

These rules apply to code, data and docs. A pull request that breaks one
needs a reason in its description.

1. **Lean.** Add nothing that no command, test or recipe uses. Delete dead
   code in the same change that makes it dead.
2. **Readable.** A Go file has at most about 400 lines. A function has at
   most about 60 lines. When a file grows past the limit, split it by
   concept, not by line count. A spec or doc page has one topic.
3. **Segmented.** A directory is grouped by role, and every package has one
   responsibility (rule 6):
   - `internal/` by role: `machine/` (host, gpu, rocm, topo), `serving/`
     (recipe, runner, tuning, vllm, bench) and `support/` (execx, paths,
     config, redact, pyenv, factcache, fixture). Packages that sit on top
     (`cli`, `app`, `flow`, `doctor`, `setup`, `logdoc`, `onboarding`) stay
     at the top;
   - a package with two clearly separable roles becomes two packages, or
     two sub-packages, once it passes about 15 files. Split by a real axis
     (device generation, model family, concern), never to satisfy a count;
   - recipes by model family (`recipes/flashnext/`, `recipes/qwen-dense/`),
     shared fragments by concern (`recipes/_fragments/<concern>/`);
   - tuning data by architecture, then by build
     (`tunableop/gfx1030/<profile>/`, `tuned-moe/gfx1030/`);
   - device tables by GPU generation
     (`internal/machine/gpu/devices/<generation>.yaml`);
   - CLI commands stay one file per command, in a family package
     (`internal/cli/cmds/<family>/cmd_<name>.go`).
4. **Keyed by device.** Data that depends on the GPU is stored under the
   architecture it applies to. A new architecture adds a folder. It never
   edits another architecture's files.
5. **Discovered, not listed.** Embedded trees are found by walking the
   embedded file system. No hand-edited index lists more than a screen of
   entries. Each tuning profile carries its own `profile.json` manifest.
6. **One owner per package.** A package has one responsibility that fits
   in one sentence at the top of its `doc.go` or main file.
7. **Honest fixtures.** A test fixture that was not captured on a real host
   is named `<name>-synthetic.tar.gz` and has a `SYNTHETIC` file at its root.
   `internal/support/fixture/testdata/README.md` says which values are
   captured and which are written by hand.
8. **Layered.** A package never imports a package of a higher layer. The
   layers and the import rules are in
   [architecture.md](architecture.md) §2; `make check` fails on a violation.

## Terms

| Term | Meaning |
|---|---|
| host | The Linux machine with the GPUs. |
| mode | How vLLM runs: `host` (a Python venv on the host) or `container` (Docker or Podman). |
| engine | The container program: `docker` or `podman`. |
| device access | How a container gets the GPU: `classic` (the device nodes of the host) or `cdi` (the AMD Container Toolkit). |
| recipe | A YAML file that describes one vLLM configuration. |
| fragment | A recipe whose id starts with `_`. It is only used through `extends`. It is never listed or launched. |
| input | A named value that a recipe needs from the operator, for example a local path. |
| plan | The resolved launch: environment, arguments, GPUs, image. A plan is the output of a recipe plus overrides plus the host. |
| run | One started plan. A run has a name, a record on disk and a log. |
| check | One doctor test with a status: `ok`, `warn`, `fail` or `skip`. |
| HIP index | The GPU index that `HIP_VISIBLE_DEVICES` uses. It is the order of GPU nodes in the KFD topology. |
| arch | A gfx target name such as `gfx1030`, or a prefix such as `gfx103`. Data and recipes are keyed by arch. |
| generation | A GPU family: `rdna2`, `rdna3`, `rdna3.5`, `rdna4`. One device-table file per generation. |
| code target | The gfx target whose code objects a GPU runs. It is the GPU's own arch, or the arch of its `hsa_override`. |
| device tables | The embedded YAML files in `internal/machine/gpu/devices/`. They hold the facts of each arch and each PCI device. |
| MCP | Model Context Protocol. The agent-facing protocol of bernie ([§13](#13-model-context-protocol)). |
| confirm token | A single-use, five-minute token that binds one MCP mutation to the plan the operator saw ([§13.5](#135-write-gate)). |

## 3. Command surface

### 3.1 The default: run the binary

The whole product is one file. The operator downloads it and runs it. No
other file, script or compose file is required.

- `bernie` with no arguments runs the host in one screen and opens the
  dashboard in the foreground. The status output is printed first: the
  GPUs, the doctor summary, the best recipe for this host, the running
  runs, the venv it uses with its source
  ([§6.2](#62-the-host-venv)), and the one next command to type. The
  dashboard then binds to `--listen` (default `127.0.0.1:7860`) and
  prints the URL to open. A dashboard that is already running on the
  same port is detected: its URL is printed instead of failing to bind.
  It changes nothing.
- `bernie status [--steps]` prints the overview (without opening the
  dashboard). `--steps` prints the onboarding checklist
  ([§3.3](#33-the-onboarding-checklist)): the ordered steps from a bare
  machine to a serving model, the state of each step, the command that does
  it and the step to do now. `status --json` carries the same steps in the
  `onboarding` field. `--steps` changes the human text only: the JSON
  document is the same with and without it.
- `bernie setup apply` runs the steps of the install plan
  ([\u00a76](#6-setup)). It streams one line per step and keeps the log of
  every step. `bernie setup report` prints the record of the run and
  `bernie setup log` the log of one step ([\u00a76.4](#64-the-run-record)).
- `bernie up [RECIPE]` does the full path in one command:
  1. It reads the onboarding steps ([§3.3](#33-the-onboarding-checklist)). A
     blocked step (the GPUs are not usable by this user yet) stops `up` with
     the fix commands.
  2. It selects the best recipe that fits (or RECIPE): fit first, then
     `priority`, then id. A recipe with a required input and no value is
     skipped. A named recipe with a missing input stops `up`.
  3. It selects the mode (flag, config, else `container` when the recipe has
     an image and an engine is usable, else `host`).
  4. It plans the setup steps. A pending root step stops `up` and prints the
     commands.
  5. It asks for confirmation once, unless `--yes` is given.
  6. It applies the non-root steps and starts the run. The run is not waited
     for yet.
  7. It starts the dashboard on `--listen` (default `127.0.0.1:7860`), unless
     `--no-dash` is given, and prints the URL to open (with `?token=` in token
     mode). With `--json` no dashboard is started. The dashboard is reachable
     while the server loads the model: the
     Logs tab shows the run as `starting`, its phase sentence and its log.
  8. It waits until the run is `running` ([§8.3](#83-run-record)): `/health`
     answers and the server answered a one-token completion (`--wait`,
     default 30 minutes). It prints the progress. A run that is still
     `starting` at the limit fails with the `waiting` reason.

  A dashboard that cannot bind its address (the port is in use) is reported
  on stderr; `up` keeps waiting for the server. Ctrl-C stops the dashboard;
  the run keeps running.
  A stop before step 6 ("blocked") prints what the operator must do and
  exits with code 3. A run that exits before `/health` answers ends `up`
  with a failure: `up` scans the run's log ([§5.3](#53-log-doctor)). When a
  rule matches, `up` prints every match (rule id, cause, fix, doc link, the
  matching line) on stderr and exits with code 3. Otherwise it prints
  `NAME exited during startup; see: bernie logs NAME` and exits with code 1.
  While it waits, `up` prints one line per phase change: what the server is
  doing now, in words ("Loading the model weights, 2 of 5 shards"). The
  sentence comes from the tail of the run's log ([§5.4](#54-log-phase)).
- With `--json`, `status` and `up` write one JSON document. `status --json`
  carries the venv and its source in the `venv` field
  ([§6.2](#62-the-host-venv)). `up --json`
  never prompts; it requires `--yes` to change the host. A blocked `up`
  writes `{"blocked": {"reason": "", "commands": []}}`. An `up` whose run
  exited during startup with matching rules writes
  `{"run": "NAME", "log_findings": [Match]}` (exit code 3; `Match` is the
  object of [§5.3](#53-log-doctor)). `status --json` and `ps --json` add
  `findings` to the entry of an exited run ([§8.3](#83-run-record)) and
  `phase` to the entry of a run that is still alive
  ([§5.4](#54-log-phase)). `status --json` carries in the `serving` field
  what the server of every live run measures: the `stats` object of
  [§11](#11-dashboard), with the reason in `error` when the server does
  not answer.

### 3.2 Rules

- `--json` can be anywhere before a `--`. After `--` everything is
  positional and kept verbatim. A `--json` that directly follows a flag
  (`--set --json`) is that flag's value if the flag takes a value. Otherwise
  it is the global flag.
- `--venv DIR` is global too. It can come before the command, between a
  command family and its subcommand, or after the command. It names the
  host-mode venv for commands that use one.
- `--root DIR` is global too, in the same places. It names the root of the
  host tree bernie reads: `/` by default, another tree when it points at a
  captured host. `BERNIE_ROOT` holds the same value for a process that
  cannot take a flag (the MCP server, a service).
- `--host` is global too, in the same places. It asks the dashboard to
  listen on every interface (`0.0.0.0:7860`) instead of the loopback
  address, so it is reachable from another machine. The default stays on
  loopback: the port is closed to the network unless the operator asks for
  it. A dashboard on the network runs in token mode, and the token is
  persisted, so the URL is the same after a restart.
- Explicit help (`--help`, `-h`, `help`) is printed on stdout and exits 0.
  `help <command>` prints the top-level help: the topic is ignored.
- A command that takes no positional argument ignores the positional
  arguments it is given. `version` ignores every argument and every flag.
- `bernie --json` without a command prints the status document
  (the same one `bernie status --json` produces) and exits 0. An agent
  reads data; the dashboard is not started, so the call returns.
- Diagnostics and progress go to stderr.
- Flags and positional arguments can be in any order.
- Exit codes: `0` success, `1` runtime failure, `2` usage error,
  `3` checks failed (the report is still printed, with no error envelope).
- A bad flag or a usage error prints one `bernie: <message>` line on stderr
  and exits 2. With `--json`, it writes the error envelope
  `{"error": "...", "exit_code": 2}` instead.
- With `--json`, a runtime failure writes `{"error": "...", "exit_code": 1}`.

| Command | Changes host | Purpose |
|---|---|---|
| `bernie` / `bernie status [--steps]` | no | The product surface: bare `bernie` prints the overview and opens the dashboard in the foreground. `bernie status` prints the overview only. `--steps` adds the onboarding checklist ([§3.3](#33-the-onboarding-checklist)). |
| `bernie up [RECIPE] [--set K=V]... [--mode M] [--venv DIR] [--engine E] [--name N] [--yes] [--no-dash] [--listen ADDR] [--wait D] [-v]` | yes | Doctor → setup → best recipe → serve → dashboard → wait for health. `-v` also streams the raw output of the setup steps. |
| `bernie hw` | no | Hardware and OS inventory. |
| `bernie topo [--bandwidth] [--venv DIR]` | no | PCIe tree, P2P matrix, link classes. Optional bandwidth test. |
| `bernie topo --check [--recipe ID]` | no | P2P readiness checklist ([§4.5](#45-p2p-readiness-checklist)). Exit 3 if an item fails. |
| `bernie doctor [--category C]... [--venv DIR]` | no | All checks. Exit 3 if one check fails. |
| `bernie doctor --log FILE\|-` / `doctor --run NAME` | no | Match a vLLM log against known failures ([§5.3](#53-log-doctor)). |
| `bernie setup plan [--mode M] [--venv DIR] [--image I] [--model M]` | no | Ordered install steps for this host. |
| `bernie setup apply [--mode M] [--step ID]... [--yes] [--venv DIR] [--image I] [--model M] [-v]` | yes | Run the steps that do not need root. Print the root steps. `-v` also streams the raw output of the steps. |
| `bernie setup verify [--venv DIR] [-v]` | yes (its own run record) | Run the check of the venv again, even when it passed before: versions of the main parts, the fork ops, the libraries. It records a run, so `setup report` shows fresh evidence. Exit code 1 when a check fails. The MCP tool `setup_verify` ([§13.4](#134-tools)) and the button of the venv detail of the dashboard ([§11](#11-dashboard)) run the same check and leave the same record. |
| `bernie recipe list [--all]` | no | Recipes, with fit to this host. `--all` includes recipes that do not fit. |
| `bernie recipe show <id>` | no | The merged recipe (after `extends`). |
| `bernie recipe validate [FILE]...` | no | Validate files, or all loaded recipes. |
| `bernie recipe plan <id> [--set K=V]... [--mode M] [--name N] [--venv DIR] [--engine E] [--device-access A] [--profile P] [--no-tunableop] [--tuning]` | no | The plan, as an exact command and environment. |
| `bernie serve <id> [--set K=V]... [--mode M] [--name N] [--venv DIR] [--engine E] [--device-access A] [--profile P] [--no-tunableop] [--tuning] [--foreground] [--no-pull] [--wait D] [--dash[=ADDR]] [--dry-run]` | yes | Start a run. |
| `bernie ps` | no | Runs, with state and health. For an exited run: the known failures in its log. |
| `bernie smoke [--url URL] [--model NAME]` | no | Check that the running server answers: `/health`, then one chat completion of `What is 2+2? Answer with one number.` so the model applies its chat template. The answer carries the time to the first token, the number of tokens and the output speed under `--json`; the human output prints the server, the model, the answer, the verdict of the check (`ok: contains 4` or `answered`), the TTFT and the tok/s. `--url` picks the server (default: the running run). |
| `bernie stop [NAME]` | yes | Stop a run (the only run if NAME is absent). |
| `bernie logs [NAME] [-f] [--tail N]` | no | Show the log of a run. |
| `bernie compose <id> [--set K=V]... [--mode M] [--name N] [--venv DIR] [--engine E] [--device-access A] [--profile P] [--no-tunableop] [--tuning] [-o FILE]` | no* | Write a `compose.yml` for the plan (*writes FILE only). |
| `bernie tune tunableop list` | no | Registered TunableOp profiles. |
| `bernie tune tunableop status [--venv DIR]` | no | Loaded rocBLAS build, matching profile. |
| `bernie tune tunableop env [--venv DIR] [--profile P] [--tuning]` | no* | `export` lines for TunableOp (*materializes rows). |
| `bernie tune power status` | no | Actual power cap per GPU. |
| `bernie tune power set <WATTS> [--gpu IDX]...` | yes (config file) | Write the desired power cap. Without `--gpu`, the value applies to all GPUs. |
| `bernie tune power apply [--file FILE]` | yes | Apply the desired caps to hwmon. Needs root. |
| `bernie tune power service print` | no* | Print the generated systemd unit pair. |
| `bernie tune power service install-plan` | no* | Print the root commands that install the unit pair. |
| `bernie tune power service status` | no | Service state, desired caps, actual caps and drift. |
| `bernie tune power service remove-plan` | no* | Print the root commands that remove the unit pair. |
| `bernie bench [--url U] [flags]` | no | Benchmark a server. Save the result. With no arguments it runs the benchmark with the default flags; it never prints help. |
| `bernie bench suites` | no | The built-in suites with their digest and their groups. |
| `bernie bench list` / `bench show <ID>` | no | Saved results. |
| `bernie bench compare <ID-A> <ID-B> [--band P] [--markdown]` | no | Two results side by side with a verdict. Refuses two different workloads. `--markdown` prints the receipt of the comparison as markdown. |
| `bernie bench ab --base ARM --cand ARM` | yes (it starts and stops a server for every arm) | Run a baseline and a candidate in the order base, cand, cand, base and tell the verdict. |
| `bernie config show` / `config set <KEY> <VALUE>` | yes (config file) | User defaults. |
| `bernie fleet add <NAME> <URL> [--token T]` / `fleet rm <NAME>` / `fleet ls` | yes (config file) | List of other bernie hosts the Fleet tab watches. The token is stored in the config file with `0600`; it is never sent to the browser ([§11](#11-dashboard)). |
| `bernie dash [--listen ADDR] [--token T] [--demo] [--host]` | no | Start the dashboard in the foreground. `--demo` serves a captured host and a fake run from a sandbox under the user cache, with the banner "Demo: nothing here is real"; it needs no GPU and changes nothing on the host. `--host` binds every interface ([§3.2](#32-rules)). |
| `bernie dash start [--listen ADDR] [--token T] [--rotate-token] [--no-systemd] [--show-token] [--host] [--json]` | yes (a background process) | Start the dashboard as a service that outlives the shell ([§11.1](#111-the-dashboard-service)). |
| `bernie dash stop [--json]` | yes | Stop the dashboard service. |
| `bernie dash status [--show-token] [--json]` | no | The state of the dashboard service and the URLs to open. |
| `bernie mcp [--allow-write]` | no | Serve MCP on stdin and stdout ([§13](#13-model-context-protocol)). With `--allow-write` it exposes the tools that change the host. |
| `bernie version` | no | Version. |

A `no*` command writes only its generated files under the state directory.

`recipe plan`, `serve` and `compose` share the launch flags: `--set`,
`--mode`, `--name`, `--venv`, `--engine`, `--device-access` (`classic`,
`cdi` or `auto`), `--profile` (force a TunableOp
profile), `--no-tunableop` and `--tuning`. `serve` adds `--foreground`,
`--no-pull`, `--wait D`, `--dash[=ADDR]` and `--dry-run` (same output as
`recipe plan`). `serve --wait D` waits up to `D` until the run is
`running`, like `up`; a run that exits first is reported like in `up`. While it waits,
`serve` prints the phase sentence of the run ([§5.4](#54-log-phase)).
Without `--wait`, `serve` returns once the run is started. `serve --dash`
starts the dashboard next to the run (default address `127.0.0.1:7860`,
`--dash=ADDR` picks another one) and prints its URL: the dashboard is
reachable while the model loads. `serve` then waits for Ctrl-C; the run
keeps running. Without `--dash`, `serve` stays a pure launcher.
`serve --foreground` reports the log findings of a server that exits with
an error. `compose` writes container plans only. `setup plan` and `setup
apply` also take `--venv`, `--image` and `--model`. `status` takes
`--venv`.

### 3.3 The onboarding checklist

The onboarding is the ordered list of steps that lead from a bare machine to
a serving model. One document holds it. It is derived from the real state of
the host (the doctor report, the setup plan, the recipes and the runs), and
every front end reads that one document, so the page and the command cannot
disagree.

A step has this record:

```json
{"id": "hardware", "title": "", "state": "done|todo|running|blocked|skipped",
 "summary": "", "detail": "", "command": "", "can_run": false,
 "root_commands": [], "progress": {"phase": "", "done": 0, "total": 0},
 "problems": [], "job_id": "", "job_state": "", "serving": false,
 "waits_for": "", "action": "",
 "estimate": {"seconds": 0, "size_bytes": 0}}
```

- `id` is stable: `hardware`, `access`, `rocm`, `python-env`, `model`,
  `recipe`, `serve`, `verify`, then the extras `bench`, `tuning` and
  `power`. The steps come in that order. The extras never hold the path.
- `state` is `done` (the postcondition holds), `todo` (something is missing
  and bernie can do it), `running` (a job is doing it now), `blocked` (only
  the operator can do it) or `skipped` (the step does not apply to this
  host).
- `summary` is one plain sentence. `detail` says more.
- `command` is the exact command that does the step.
- `can_run` says that bernie can do the step itself, without root.
- `root_commands` are the commands the operator runs as root. bernie prints
  them and never runs them.
- `progress` is the phase sentence of a running step, with the fraction of
  the work when the step knows it.
- `problems` are the doctor checks that hold the step back, with their fix
  and their documentation link.
- `job_id` is the id of the job of this step, when a job is running or
  has finished less than five minutes ago. `job_state` is the state of that job:
  `running`, `done` or `failed`. A step that is `running` without a `job_id`
  is the serving run, not an onboarding job. The page re-attaches to the job
  with the id.
- `serving` is true on the serve step when the server answers requests now
  (its health check passed). The page reads it instead of guessing from the
  phase, and `next` becomes `verify` when the server serves.
- `waits_for` is the id of the step that holds the path, on a runnable step
  that comes after it. The step says in words what it waits for.
- `action` names what the step does, in the words of the reader. The button
  of the step says this.
- `estimate` says what the step costs: the time in seconds and the size in
  bytes it took the last time it ran on this host. It is absent when the
  step never ran here: the numbers are measured, never guessed.

The document also holds the facts of the host: the numbers the page shows
above the steps, each with the reason why it matters.

```json
{"id": "gpus", "label": "GPUs", "value": "8× gfx1030 Radeon PRO V620",
 "why": "Every card is gfx1030, so one recipe family fits them all."}
```

`bernie status --steps` prints the same array. The name of the fact is `id`
and not `key`: a member whose name ends in `key` names a secret, so the
redactor of [§13.7](#137-untrusted-content) would hide it.

The document is `{"next": "<id>", "next_action": {"step": "", "title": "",
"state": "", "action": "", "sentence": "", "command": "", "root": false,
"root_commands": []}, "primary": {"kind": "", "label": "", "sentence": "",
"command": "", "root_commands": [], "steps": []},
"recommendation": {"id": "", "name": "", "description": "", "why": "",
"alternatives": []},
"steps": [...], "facts": [...], "facts_age_s": 0,
"stale": false}`.
`primary` is the one action of the first run: the button the reader presses.
Its `kind` is `root` (only the operator can move the path on), `setup`
(bernie does everything that is missing), `serve`, `verify` or `done`. The
`label` is the text of the button, the `sentence` says what the action does
and who does it, the `command` is the command that does the same work in a
terminal, the `root_commands` are the commands the operator runs as root and
the `steps` are the ids of the steps the action covers. The CLI, the MCP
server and the page read this one record, so they cannot disagree about the
button.
`recommendation` is the recipe bernie recommends for this host: the `id`, the
human `name`, the one-line `description` of the recipe file, the `why` (one
sentence that says why this one) and the `alternatives` (the other recipes
that fit). It is absent when no recipe fits.
`next` is the step to do
now: the first step of the path that is not done and not skipped. It is
empty when the path is complete. `next_action` is that step in words: the
`action` names what it does, the `sentence` says what to do and who does it,
the `command` is the command that does it and `root` says that the operator
runs the `root_commands` as root. It is empty when the path is complete. The
CLI, the MCP server and the page print this one record, so they cannot
disagree about the next step. `facts_age_s` is the age in seconds of the
facts behind the document; it is 0 when the host was read now. `stale` is
true when the document was served from the cache while it was past its time
to live, so a refresh is running: the document is the last known state of
the host, not the current one.

The dashboard reads the document from the cache
([§12](#12-files-and-paths)): a document younger than 15 s is served as it
is, an older one is served and refreshed in the background. The CLI and the
MCP server read the host every time.

| Front end | What it shows |
|---|---|
| `bernie status --steps` | the checklist as a plain list |
| `bernie status --json` | the same steps in the `onboarding` field |
| `GET /api/onboarding` | the document ([§11](#11-dashboard)) |
| MCP `onboarding_status` | the document ([§13](#13-model-context-protocol)) |

`up` reads the same steps: it stops on the first blocked step of the path
and runs the steps that follow it. The action of a step is the same in `up`
and in a job, so there is one code path.

A step that bernie can run does it in a job. `POST
/api/onboarding/{step}/run` starts the job and returns its id. `GET
/api/jobs/{id}` returns the job, `GET /api/jobs/{id}/events` streams it
(server-sent events: one `line` event per output line, then a `done` or
`failed` event with the job document and the known failures of its log).
One job runs at a time. The state of a job is kept under
`<state>/jobs/<id>`, so a page reload finds it again. A step that needs root
answers with the commands to run and runs nothing.

A job carries the numbers of the work when the step counts them:

```json
{"progress": {"phase": "model.download", "bytes_done": 1800000000,
 "bytes_total": 2300000000, "rate_bps": 45200000, "files_done": 7,
 "files_total": 14, "eta_s": 11}}
```

The phase is the step the job is on. The bytes, the rate and the time to
finish are there for a download; the file counts are there when the step
counts the files. A number that is not measured is absent.

`GET /api/jobs` lists the jobs of this host, newest first:
`[{id, step, state, started_at, finished_at, phase, progress, pid}]`. The list
holds at most 50 jobs. `pid` is the process that runs the job: a job that is
still `running` while its process is gone is marked `failed`.

`GET /api/jobs/{id}/events` replays the buffered lines of the job from the
beginning, so a page that reloads finds the whole output again. Every line
event carries the number of the line as its id; a client that re-attaches
sends it back in the `Last-Event-ID` header (or in `last_event_id`) and gets
the lines after it. A job that failed or finished stays readable: its
document lives on disk.

The Setup tab reads the record of the setup runs: `GET /api/setup/runs`
returns the records of this host, newest first, and the newest one in full;
`GET /api/setup/runs/{id}/log/{step}` returns the log of one step of a run
(the tail, at most 400 lines). The button of the Setup tab is a setup run:
it writes the record of [\u00a76.4](#64-the-run-record), and the page shows
the step that runs now, the time every step took, and the log of the step
under its row. A reload re-attaches: the record is on disk.

## 4. Host inspection (`hw`, `topo`)

### 4.1 Sources

bernie reads sysfs and procfs first. It runs external tools only to add
information. A missing tool is a `skip`, not an error.

| Data | Source |
|---|---|
| GPUs, gfx target, HIP index | `/sys/class/kfd/kfd/topology/nodes/*/properties` (`gfx_target_version`, `location_id`, `drm_render_minor`). Nodes with `simd_count > 0` are GPUs. |
| GPU name | The DRM `product_name`. Consumer cards have none: the device tables by PCI device id ([§4.4](#44-device-tables)), else `AMD GPU <gfx>`. |
| PCI address, VRAM, link | `/sys/class/drm/renderD<minor>/device` → `mem_info_vram_total`, `current_link_speed`, `current_link_width`, `numa_node`. |
| Telemetry | `gpu_busy_percent`, `mem_info_vram_used`, `hwmon/*/temp*_input`, `power1_average`, `power1_cap`. The edge temperature is the hwmon sensor labelled `edge`, else `temp1_input`; the junction temperature the sensor labelled `junction`; the memory temperature the sensor labelled `mem`. The clocks come from `pp_dpm_sclk` and `pp_dpm_mclk`: the line with the star is the state the card runs in. |
| ECC | `/sys/class/drm/cardN/device/ras/umc_err_count` (`ue` uncorrectable, `ce` correctable, `de` degraded) and `ras/features`. The device tables say which cards have ECC ([§4.4](#44-device-tables)). When the kernel exposes no counters, `amd-smi metric --json` is the fallback. |
| PCIe tree | The real path of the PCI device in `/sys/devices`. |
| P2P | `nodes/<n>/io_links` and `nodes/<n>/p2p_links`. |
| OS | `/etc/os-release`, `uname`, `/proc/cmdline`, `/proc/meminfo`, `/proc/cpuinfo`. |
| GPU group ids | the owner of `/dev/kfd` and `/dev/dri/*` (stat), else `getent group video` and `getent group render`, else `/etc/group`. |

`gfx_target_version` `100300` is `gfx1030`. The rule: major = v/10000,
minor = (v/100)%100, step = v%100; name = `gfx` + major + minor + step, with
minor and step in hex.

The paths above are read under the root of the host tree: `/` on the real
host. `BERNIE_ROOT` (or the global flag `--root`, see [§3.2](#32-rules))
points bernie at another tree, a captured host snapshot. Then `hw`, `doctor`,
`status`, the dashboard and the MCP server read that tree instead of the
machine, and every path above is read under it. bernie never writes there.
The flag is for demos and for tests.

### 4.2 Link classes

`topo` classifies every GPU pair with the RCCL names:

| Class | Meaning |
|---|---|
| `PIX` | Both GPUs are below the same PCIe switch. |
| `PXB` | The path crosses more than one switch, but not the root complex. |
| `PHB` | The path crosses one root complex (host bridge), or root complexes on the same NUMA node. |
| `SYS` | The path crosses NUMA nodes (CPU interconnect). |

Two rules keep the classes meaningful. A switch inside a GPU card (the AMD
bridge pair on Navi cards) does not count as a switch. The two internal
levels of a Broadcom PEX88096 count as one switch.
`topo` also reports `p2p: true|false` from the KFD links.
`topo --bandwidth` runs `rocm-bandwidth-test` when it is installed. Without
it, and with `--venv`, it runs a PyTorch peer-copy probe. It reports GB/s per
pair.

### 4.3 JSON shape (`hw`)

```json
{
  "host": {"hostname": "", "os": {"id": "fedora", "version_id": "43", "pretty": ""},
           "kernel": "", "pkg_manager": "dnf", "cpu": {"model": "", "vendor": "",
           "sockets": 1, "cores": 48, "threads": 96}, "ram_bytes": 0,
           "virtualization": "none"},
  "gpus": [{"hip_index": 0, "kfd_node": 1, "name": "", "gfx": "gfx1030",
            "pci": "0000:07:00.0", "device_id": "0x73a1", "vram_bytes": 0,
            "render_minor": 128, "numa_node": 0,
            "link": {"gen": 4, "width": 16}, "power_cap_w": 0,
            "temp_limits_c": {"edge": 100, "junction": 100, "mem": 98}}]
}
```

`temp_limits_c` holds the maximum operating temperatures of the card in
degrees Celsius, keyed by the hwmon label. It is absent when the device
tables do not know the card.

`restriction` says that the environment limits the GPUs bernie uses
([§7.7](#77-gpu-selection)). It is absent when nothing is restricted.

### 4.4 Device tables

Device facts live in embedded YAML files, one per GPU generation:
`internal/machine/gpu/devices/<generation>.yaml`. The file stem equals the
`generation` value. The loader walks the folder. A malformed file is a build
defect: the tests catch it.

```yaml
generation: rdna2          # equals the file stem
wave_size: 32
triton_below: "3.9"        # optional: Triton from this version dropped the generation
arches:                    # gfx targets of this generation
  gfx1030: {family: navi21, bf16: none}
  gfx1031: {family: navi22, bf16: none, hsa_override: "10.3.0"}
  gfx1035: {family: yellow_carp, bf16: none, hsa_override: "10.3.0", integrated: true}
devices:                   # PCI device ids, lower-case 0x%04x
  - {ids: ["0x73a1"], name: Radeon PRO V620, gfx: gfx1030, power_cap_advice_w: 180,
     ecc: true, temp_limits_c: {edge: 100, junction: 100, mem: 98}}
```

| Field | Meaning |
|---|---|
| `bf16` | `native` (BF16 matrix instructions), `none`, or absent when unsourced. |
| `hsa_override` | The `HSA_OVERRIDE_GFX_VERSION` that makes the arch run the code objects of a sibling arch of the same file. |
| `integrated` | The arch is an APU. It shares memory with the CPU. |
| `flash_attn` | The arch is on the FlashAttention ROCm allowlist. |
| `power_cap_advice_w` | Recommended power cap of this device. Optional, only with a source. |
| `ecc` | The card counts its memory errors: it has ECC. Optional, only with a source. |
| `temp_limits_c` | The maximum operating temperatures of the card in degrees Celsius, keyed by the hwmon label (`edge`, `junction`, `mem`). Optional, only with a source. |

Rules:

- An arch name and a device id are unique across all files.
- A device names an arch of its own file.
- Every file lists its sources in a header comment.
- A value without a source is left out. Absent means "unknown", not "no".

Go code reads the tables only through `devices.Lookup(deviceID)`,
`devices.LookupArch(gfx)` and `devices.CodeTargets(gfxs)`. These users exist:

| User | What it takes |
|---|---|
| `gpu.List` | The device name when the DRM node has no `product_name`, and the temperature limits of the card. |
| `doctor` | `gpu.supported.<gfx>`, `gpu.hsa_override.<gfx>`, `gpu.igpu`, `gpu.ecc.<hip index>`, the Triton check, the power-cap advice. |
| `setup` | Code targets (wheels, build targets), the FlashAttention allowlist. |

### 4.5 P2P readiness checklist

`bernie topo --check` answers one question: will multi-GPU peer-to-peer
copies, and the custom all-reduce that uses them, work on this host? It
prints an ordered checklist. Every item has a status (`ok`, `warn`, `fail`
or `skip`), one line of cause, and a fix command or a documentation link.

| Item | What it reads | `ok` | `warn` | `fail` |
|---|---|---|---|---|
| `kfd_links` | the KFD `p2p_links` and `io_links` of every GPU node, and the KFD peer link of every GPU pair | every pair has a peer link and every GPU has an io_link | a pair has no peer link, so it stages through the host | no pair has a peer link |
| `large_bar` | the largest memory region of `/sys/bus/pci/devices/<bdf>/resource` and the VRAM of the GPU | the BAR covers the VRAM, or the kernel command line pins the firmware BAR (`pci=realloc=off`) | the BAR is smaller than the VRAM and nothing pins it | – |
| `iommu` | `iommu=pt`, `iommu=on` or `iommu=off` in `/proc/cmdline` | `iommu=pt` or `iommu=off` | the IOMMU translates peer addresses | – |
| `acs` | `pcie_acs_override` and `disable_acs_redir` in `/proc/cmdline` | ACS redirection is off, or no PCIe switch sits between the GPUs | ACS can block a peer copy across a switch | – |
| `link_speed` | `current_link_speed` and `current_link_width` against `max_link_speed` and `max_link_width` | every GPU runs at the maximum of its slot | a link is degraded or below gen4 x8 | – |
| `link_classes` | the link classes of every GPU pair ([§4.2](#42-link-classes)) | every pair is PIX or PXB | a pair crosses a host bridge (PHB) or the CPU (SYS) | – |
| `bandwidth_test` | the `rocm-bandwidth-test` binary | it is installed | – | never |
| `recipe_env` | the NCCL, RCCL and RDNA all-reduce variables of the recipe | it shows them (informational) | – | never |

`--recipe ID` picks the recipe whose environment the last item shows. Without
it the best fitting recipe is used. The item is `skip` when no recipe fits.

The JSON document is:

```json
{"ready": true,
 "summary": {"ok": 5, "warn": 1, "fail": 0, "skip": 2},
 "checklist": [{"id": "kfd_links", "title": "KFD peer links", "status": "ok",
                "detail": "45 of 45 GPU pairs have a KFD P2P link; every GPU has 9 p2p_links and 1 io_link",
                "fix": "", "doc": "https://..."}]}
```

`ready` is false when an item failed. The command exits with code 3 when an
item failed.

The doctor category `topo.p2p` runs the same checklist: every item becomes
one check (`topo.p2p.<item>`) with the same status, detail, fix and doc.

## 5. Doctor

### 5.1 Check record

```json
{"id": "rocm.hipcc", "category": "rocm", "status": "ok|warn|fail|skip",
 "title": "hipcc is installed", "detail": "/opt/rocm/core-7.14/bin/hipcc (HIP 7.14.60850)",
 "fix": "sudo dnf install hipcc", "doc": "https://..."}
```

The report is `{"summary": {"ok": 0, "warn": 0, "fail": 0, "skip": 0}, "checks": [...],
"venv": {"path": "", "source": "flag", "candidates": []}}`.
`fix` is a command for the host's package manager when one exists, else one
sentence. Rule: `fail` blocks a launch; `warn` costs performance or
stability; `skip` means the check cannot run here. The `venv` field says
which venv the python checks used and where it came from
([§6.2](#62-the-host-venv)). It is absent when the caller does not say.

### 5.2 Required checks

| Category | Checks |
|---|---|
| `os` | Distro and version known. Package manager known. Kernel version. |
| `gpu` | `/dev/kfd` exists and is readable/writable (`gpu.kfd`). `/dev/dri/renderD*` exist. `gpu.groups` compares group ids: the `video` and `render` gids come from `/etc/group`, then from `getent group <name>` (3 s timeout). The user's gids are the primary and supplementary groups of the process. A group that neither source knows makes the check `warn` ("cannot tell"), never `fail`. At least one GPU. One `gpu.supported.<gfx>` per gfx target on the host: `ok` when the target is in the device tables and a validated recipe lists it, `warn` when the target is in the tables but no validated recipe lists it, `warn` ("not in the device tables") when the tables do not know it (for example a CDNA target). One `gpu.hsa_override.<gfx>` (`warn`) per target whose table row has an `hsa_override`, with the `HSA_OVERRIDE_GFX_VERSION` advice. `gpu.igpu` warns when an integrated GPU mixes with discrete GPUs. One `gpu.ecc.<hip index>` per GPU: `ok` when ECC is on and the card counted no memory error, `warn` when ECC is off on a card that has it or when the card corrected errors, `fail` when the card counted uncorrectable errors, `skip` when the card has no ECC or the kernel exposes no counters. |
| `kernel` | `amdgpu` is loaded. IOMMU mode from `/proc/cmdline` (`iommu=pt` advice for P2P). PCIe links are gen4 x8 or better. |
| `topo.p2p` | The P2P readiness checklist of [§4.5](#45-p2p-readiness-checklist): one check per item (`topo.p2p.<item>`). |
| `rocm` | ROCm installs found (`/opt/rocm*`, `/opt/rocm/core-*`, `/usr`) with versions. A tree inside another ROCm tree (for example `/opt/rocm-7.2.0/core-7.14`) is one install with a nested layout, not a second install. More than one install or a nested layout with different versions: `warn`. The warning names the ROCm install whose tools win on `PATH` and says how to check it with `which hipcc`. `rocminfo` lists the GPUs. `hipcc`, `amd-smi` or `rocm-smi` present. `librocblas.so.5` found. ROCm 7.2.1 to 7.13 with more than one GPU: `warn` (RCCL TP>1 deadlock range). |
| `python` | With `--venv` (or the configured venv): Python version, `torch` imports, `torch.version.hip` set, GPU count matches, `rocm-sdk-device` wheels match the GPU code targets, Triton is below the version that dropped a GPU generation (`triton_below` in the device tables; `fail`), `vllm` imports, fork ops registered (`_rocm_C::*rdna*` schema count > 10). |
| `container` | `docker` or `podman` present, daemon or socket usable, compose present, rootless state, SELinux mode. The device access of a container run: `container.cdi` (the AMD Container Toolkit and its `amd.com/gpu` spec; `skip` without the toolkit, `warn` with the toolkit and no spec, the fix is `sudo amd-ctk cdi generate --output=/etc/cdi/amd.json`), `container.cdi.valid` (the spec still fits this host; the fix is `sudo amd-ctk cdi generate --output=/etc/cdi/amd.json && sudo amd-ctk cdi validate`), `container.cdi.engine` (the engine reads CDI: docker from 28.3.0 on, podman from 4.0 on) and `container.device_access` (the mode a container run would use and why). |
| `resources` | RAM. Free disk in the Hugging Face cache. `/dev/shm` size. |
| `tuning` | TunableOp profile matches the loaded rocBLAS. Without a known venv this check is `skip`: "pass --venv to check the rocBLAS your server loads". The system rocBLAS is not the library a host-mode venv loads. Power cap per GPU: `warn` when a cap is above the advice of the device table ([§9.2](#92-power)). |

### 5.3 Log doctor

`doctor --log FILE|-` and `doctor --run NAME` match a vLLM log against a
catalog of known failures. `-` reads standard input. `--run` reads the `log`
file of the run record (`<state>/runs/<name>/run.json`, `serve.log` in the run
directory when that file is gone). For a container run it reads the engine's
`logs` output. `--log` and `--run` exclude each other and `--category` and
`--venv`. With either flag, `doctor` prints only the log findings and runs no
host checks.

Each rule has an id (`<family>.<slug>`), a severity (`error` or `warn`), a
regular expression (Go RE2, matched per line), a cause, a fix and a `doc`
link. The catalog is data (embedded YAML, `internal/logdoc/catalog.yaml`), not
code. The report lists each matched rule once, with the first matching line
and its line number. Lines are matched up to 1 MiB; a log can be any size.

```json
{"matches": [{"id": "memory.kv-too-small", "severity": "error", "pattern": "...",
              "cause": "", "fix": "", "doc": "https://...", "line": "", "line_no": 82}]}
```

`matches` is `[]` when nothing matched. Exit code 3 when at least one rule
matches.

`up`, `serve --wait`, `serve --foreground`, `ps` and `status` use the same
scan on a run that exited, so the operator and agents do not need a second
command. One call scans for at most 2 seconds, whatever the log size (a
container log is read with the `execx` timeout). A scan that runs out of time
reports the rules it matched so far. A log that cannot be read gives no
findings. The log of an exited run does not change, so a long-lived process
(`bernie dash`) scans it once.

### 5.4 Log phase

The tail of a serve log names the phase the server is in. The phases are
data (embedded YAML, `internal/logdoc/phase.yaml`), not code. One phase is
one pattern (Go RE2, matched per line), one plain sentence and an optional
progress pattern with two groups: done and total.

The last line of the tail that matches a pattern names the phase. A tail
with nothing known in it has no phase. The tail is bounded: at most 256 KiB
are read from the end of the log, so a multi-gigabyte log costs the same as
a small one.

```json
{"id": "loading-weights", "sentence": "Loading the model weights, 2 of 5 shards",
 "done": 2, "total": 5}
```

`done` and `total` are absent when the log does not say how far the step is.
`up` and `serve --wait` print the sentence on stderr when it changes, at
most one line every 5 seconds. `ps --json` and `status --json` carry the
phase of a run that is not exited ([§8.3](#83-run-record)).

## 6. Setup

`setup plan` returns ordered steps. The steps read the GPU targets and the
device tables ([§4.4](#44-device-tables)) to choose wheels and build targets.
Each step has this record:

```json
{"id": "venv.create", "title": "", "mode": "host|container|both",
 "root": false, "done": false, "commands": ["..."], "why": "",
 "download": {"repo": "", "hub_cache": "", "resume_bytes": 0}}
```

`download` is set on the `model.download` step only ([§6.1](#61-the-model-download)).

`done` is true when the step's postcondition already holds (bernie checks it).
`setup apply` runs, in order, the steps that are not done and do not need
root. It stops at the first failure. It prints root steps and does not run
them. `--yes` skips the confirmation prompt. `--step` limits the run to named
steps.

Steps (filtered by mode and host):

| ID | Root | Mode | Content |
|---|---|---|---|
| `groups` | yes | both | `usermod -aG video,render $USER`. |
| `rocm.runtime` | yes | host | Install the ROCm runtime and the HIP SDK from the AMD repository of the distribution ([§6.3](#63-the-rocm-repository)). |
| `container.engine` | yes | container | Install Docker or Podman and compose. |
| `venv.create` | no | host | Create the venv (`uv` if present, else `python3 -m venv`). |
| `venv.torch` | no | host | Install `torch`, `torchvision`, `torchaudio`, `triton` and one `rocm-sdk-device-<gfx>` + `amd-torch-device-<gfx>` wheel per code target from the AMD multi-arch index `https://repo.amd.com/rocm/whl-multi-arch/`. The code target of a GPU is its gfx target, or the target of its `hsa_override` ([§4.4](#44-device-tables)): gfx1031 to gfx1036 use the gfx1030 wheels. The index has no plain `rocm-sdk` package: `rocm-sdk-device-<gfx>` pulls in `rocm-sdk-libraries`. |
| `causal-conv1d.build` | no | host | Build causal-conv1d from source for the host GPU targets. |
| `flash-attn.build` | no | host | Only when a GPU has `flash_attn: true` in the device tables. gfx1030 does not. |
| `vllm.build` | no | host | Clone the fork and build it in the venv, with `SETUPTOOLS_SCM_PRETEND_VERSION`: the branch carries no tag. |
| `venv.verify` | no | host | The checks of the gfx1030 wiki: torch and vLLM import, the fork registers more than ten RDNA ops with an `all_reduce`, the extension finds every library and rocBLAS comes from the wheel of the venv. |
| `image.pull` | no | container | Pull the recipe image. |
| `model.download` | no | both | The files of the repo into the Hugging Face cache ([§6.1](#61-the-model-download)). |

`setup plan` and `setup apply` take `--rocm VERSION`: the ROCm line to
install, `7.14` (the default) or `7.2`.

### 6.1 The model download

`setup apply` puts the files of a model repo in the Hugging Face cache
(`<HF_HOME>/hub`), the same layout the `hf` tool uses: the content in
`models--<org>--<name>/blobs/<checksum>`, the snapshot
`models--<org>--<name>/snapshots/<commit>/<path>` with a symlink per file,
and the ref `models--<org>--<name>/refs/main`. vLLM finds the model in the
cache without asking the Hub again.

bernie downloads the files itself, with the Go standard library:

- The file list comes from `GET /api/models/<repo>?blobs=true`. Every file
  has a size and a checksum: the sha256 of an LFS file, the git blob id of
  a small file.
- A file that is complete in the cache is skipped.
- A partial file continues with a `Range` request. The partial file is
  `<checksum>.incomplete` in the blobs directory. The step reports the
  bytes that are already there (`resume_bytes`).
- The checksum of the content is checked before the file is moved in place.
- The progress is one line every two seconds and one at the end:
  `model.download: 78% · 1.8 GB of 2.3 GB · 45.2 MB/s · file 7 of 14`.
  The fields are `pct`, `done of total`, `rate` and `file i of n`, joined
  by ` · `; the file count is omitted before the first file finishes. The
  run record ([§6.4](#64-the-run-record)) carries the same numbers in
  `progress` (`bytes_done`, `bytes_total`, `rate_bps`, `files_done`,
  `files_total`) so the dashboard and the CLI can render the sentence.

The download runs on the host. The container image of bernie holds no
HTTPS client and no Python, and it never downloads anything.

Why bernie does not call the `hf` tool: `huggingface_hub` 2.x gives every
process its own temporary file
(`<etag>.<random>.incomplete`, see `file_download.py` in the package), so
an interrupted download starts over. The `hf` command does the same job
(`hf download <repo>`) and stays the way to fetch a model by hand.

### 6.2 The host venv

Host mode needs a Python venv with vLLM. bernie picks it in this order:

1. `--venv DIR`.
2. `$BERNIE_VENV`.
3. The config key `venv` ([§12](#12-files-and-paths)).
4. `$VIRTUAL_ENV`.
5. One conventional venv, when exactly one exists:
   - `<data>/venv*` (the default of `setup apply`, e.g. `<data>/venv-rocm7.14.0`)
   - `~/.venvs/vllm*`
   - `~/Apps/vllm/venv*`

A conventional candidate must have `bin/python` and a `site-packages` that
holds the `vllm` module: the `vllm` package, a `vllm.py` module or an
editable install of it. bernie reads the directory instead of starting the
venv's Python: a venv that imports vLLM at startup takes seconds to start,
and the answer does not change between two reads of the same tree. When
several candidates exist, bernie picks none: `doctor` and `status` list the
candidates and ask the operator to choose one with `bernie config set venv
DIR`.

`setup apply` and `up` pick the venv the same way, and the run record
([§6.4](#64-the-run-record)) carries the one the steps used. When none of
the sources above is set and the conventional list is empty, `setup apply`
and `up` create the venv at `<data>/venv-rocm<VERSION>` and write that path
to the config key `venv` when no explicit `--venv`, `$BERNIE_VENV`, config
`venv` or `$VIRTUAL_ENV` is set: a later run in the same dirs finds it
through step 3. An explicit user choice is never overwritten.

`doctor` and `status` say which venv they used and where it came from:
`flag`, `$BERNIE_VENV`, `config`, `$VIRTUAL_ENV` or `conventional`. An
ambiguous choice has the source `ambiguous` and lists the candidates.

The venv holds the stack of the gfx1030 wiki:

1. The ROCm device wheels, `torch`, `torchvision`, `torchaudio` and
   `triton` (the `venv.torch` step).
2. The build backend and the build tools of the fork: `setuptools`,
   `setuptools-scm`, `setuptools-rust`, `cmake`, `ninja`, `wheel`,
   `jinja2` and `packaging`. The fork is built with
   `--no-build-isolation`, so the venv must hold them: without them the
   build stops with `No module named setuptools_scm`.
3. `causal-conv1d`, built from source for the targets of the host. The
   HIP compiler takes the targets separated by commas.
4. The fork of vLLM (`opengfx1030/vllm-rdna`, branch `rdna_extras`),
   editable, with its runtime dependencies from `requirements/common.txt`.

The build of the fork runs with these variables:

| Variable | Value |
|---|---|
| `VLLM_TARGET_DEVICE` | `rocm` |
| `VLLM_USE_PRECOMPILED` | `0` |
| `PYTORCH_ROCM_ARCH` | the code targets of the host, separated by `;` |
| `ROCM_HOME`, `ROCM_PATH`, `HIP_PATH` | the ROCm install that has `hipcc` |
| `CMAKE_HIP_COMPILER` | `<install>/bin/hipcc` |
| `HIP_ROOT_DIR`, `CMAKE_HIP_COMPILER_ROCM_ROOT` | the same install |
| `CMAKE_BUILD_TYPE` | `RelWithDebInfo` |
| `SETUPTOOLS_SCM_PRETEND_VERSION` | `0.31.0+rdna` (the branch carries no tag) |
| `MAX_JOBS` | the cores of the host, at most 16 |
| `PATH` | the bin directory of the venv first: `cmake` and `ninja` live there |

The `venv.verify` step is the postcondition of `vllm.build`. It runs the
checks of the gfx1030 wiki in the venv: `torch` and `vLLM` import, the
fork registers more than ten RDNA ops with an `all_reduce`, the extension
finds every library it needs and rocBLAS comes from the wheel of the venv.
The step is done when the checks pass; the `doctor` reports the same
findings.

The evidence of the step carries a small ordered `components` list of the
MOST IMPORTANT parts of the venv: `python`, `torch` (with the HIP build),
the `rocm-sdk-core`/`rocm` wheel, `triton`, `vllm` (with the fork's git
short sha and branch when the editable source has a `.git`, read from
files, not by running git), `flash-attn` (the `flash_attn` package when
it is there, else the fork's FA-RDNA2 op count from the registered ops of
`vllm._rocm_C`), `causal-conv1d`, `transformers`, `rocBLAS` (with the
wheel library version: the SONAME of the resolved `librocmblas.so.N`,
the rocBLAS release when its symbol/version file is readable, and the
`rocm-sdk-libraries` package version as the floor — a resolved rocBLAS
is never `absent`), `RCCL` (`torch.cuda.nccl.version()`) and the host
ROCm SDK (the version of `/opt/rocm`, for comparison). Every entry has
`name`, `version` and `note`; a missing part is reported as `"absent"`,
never omitted. The list has at most twelve entries and the probe collects
them in one Python pass with `importlib.metadata`, no extra process.

### 6.3 The ROCm repository

AMD publishes ROCm in two repository shapes. The multi-arch repository
carries one package per GPU architecture (`amdrocm-core-sdk7.14-gfx1030`)
and serves ROCm 7.13 and later. The per-release repository carries the
packages of one ROCm release, serves ROCm 7.2 and earlier, and names no
architecture.

bernie picks the repository of the distribution from `/etc/os-release`:

| Distribution | Multi-arch directory | Per-release directory |
|---|---|---|
| Fedora 42 and later | `rhel10` | `el10` |
| Fedora 39 to 41 | `rhel9` | `el9` |
| Fedora 8 to 38 | `rhel8` | `el8` |
| RHEL 10, Rocky 10, AlmaLinux 10 | `rhel10` | `el10` |
| RHEL 9, Rocky 9, AlmaLinux 9 | `rhel9` | `el9` |
| RHEL 8, Rocky 8, AlmaLinux 8 | `rhel8` | `el8` |
| Ubuntu 22.04 | `ubuntu2204` | `jammy` |
| Ubuntu 24.04 | `ubuntu2404` | `noble` |
| Ubuntu 26.04 | `ubuntu2604` | none |
| Debian 12 | `debian12` | `bookworm` |
| Debian 13 | `debian13` | `trixie` |

The multi-arch directory is a directory of
`https://repo.amd.com/rocm/packages-multi-arch/`. The per-release directory
is an "elN" directory of `https://repo.radeon.com/rocm/` or an apt suite of
`https://repo.radeon.com/rocm/apt/7.2`.

The step installs, for every code target of the host
([§4.4](#44-device-tables)):

- ROCm 7.14: `amdrocm-core-sdk7.14-<gfx>`, `amdrocm-core7.14-<gfx>` and
  `amdrocm-blas7.14-<gfx>`.
- ROCm 7.2: `rocm-core`, `rocm-hip-sdk`, `hipcc`, `rocminfo`,
  `rocm-smi-lib`, `amd-smi-lib`, `rocblas`, `rocm-device-libs` and
  `rocm-developer-tools`.

The step is done when the host has a ROCm tree with `hipcc`. Three layouts
count: the versioned tree (`/opt/rocm-7.2.0`), the TheRock tree
(`/opt/rocm/core-7.14`) and the plain tree (`/opt/rocm`).

The step prints the command that writes
`/etc/ld.so.conf.d/10-rocm-<version>.conf` only when the tree is there and
the file is not: the ROCm 7.14 packages ship it, the ROCm 7.2 packages do
not.

A distribution without an AMD repository (Arch, for example) gets the link
to the AMD install guide instead of a command.

### 6.4 The run record

Every setup run leaves a record: the CLI, the MCP server and the dashboard
read that one record, so they cannot disagree.

The record of a run is one JSON file, `<state>/setup/<run-id>/run.json`:

```json
{"id": "20261030T101112Z-3f585", "mode": "host", "recipe": "", "venv": "/v",
 "state": "running|done|failed|blocked",
 "started_at": "", "ended_at": "", "duration_s": 0,
 "steps": [{"id": "venv.verify", "title": "", "state": "done|skipped|running|failed|blocked",
   "started_at": "", "ended_at": "", "duration_s": 0, "exit_code": 0,
   "summary": "", "log_path": "", "commands": [],
   "evidence": {"python": "", "torch": "", "hip": "", "vllm": "", "device_count": 0,
    "rocm_ops": 0, "rdna_ops": 0, "all_reduce": false,
    "rocm_c": "", "rocblas": "", "rocblas_wheel": false,
    "missing_libs": [], "verify_errors": [],
    "components": [{"name": "", "version": "", "note": ""}]},
   "progress": {"phase": "", "sentence": "", "fraction": 0,
    "bytes_done": 0, "bytes_total": 0, "rate_bps": 0,
    "files_done": 0, "files_total": 0}}],
 "error": "", "dir": ""}
```

The state of a step: `done` (it ran and passed), `skipped` (its
postcondition already held), `running` (it runs now), `failed` (it failed)
or `blocked` (it needs root). `progress` carries the phase, the sentence and
the numbers of the step while it runs; the build of the fork reads its
fraction from the `[N/M]` count of ninja, the model download reads its
bytes, rate and file count from the sentence of [§6.1](#61-the-model-download).
`bytes_done`, `bytes_total`, `rate_bps`, `files_done` and `files_total` are
absent when the step does not measure them. `evidence` is what the step
produced: the verification JSON of the check of the venv, with the versions,
the torch and the HIP version, the count of the RDNA ops, the `all_reduce`,
the result of `ldd`, the path of the rocBLAS and the ordered `components`
list of [§6.2](#62-the-host-venv) — the same JSON the CLI prints, the MCP
server returns and the Setup tab of the dashboard renders.

Every step keeps its own log, `<state>/setup/<run-id>/<step>.log`: the
command line first, then the output with a timestamp on every line. The
record is written atomically after every step and every two seconds while a
step runs, so a crash or a reload still shows the truth. The store keeps the
last ten runs; `<state>/setup/latest` points at the newest one.

The commands:

- `bernie setup apply` streams the progress of the run. On a TTY the
  progress line is the only live line: it shows the step that runs now,
  the time it has been running and its phase, and it is replaced in
  place. The raw output of a step (its command echo, the output of the
  tool) stays in the log of the step: `--verbose` streams it too, and
  `bernie setup log -f` follows the log of a step. Off a TTY (CI, an
  agent, a file) every event is one line of its own: "[3/7] venv.torch:
  running", then "done in 4m12s, log: PATH", and a progress line with the
  phase of the step at most once every ten seconds. The evidence of the
  check of the venv (`BERNIE_JSON:...`) never reaches the terminal: it
  goes to the record of the run, and `setup report` prints it. With
  `--json` the command writes one JSON event per line (NDJSON) and ends
  with the record of the run.
- `bernie setup report [RUN]` prints the record of a run (the newest one
  when no id is given): every step with its state, its duration and its log,
  then the evidence of the check of the venv. With `--json` it writes the
  record itself.
- `bernie setup verify` runs the check of the venv again through the same
  apply function, even when its postcondition holds, and records a run with
  the new evidence and component versions. It needs a host plan. The MCP
  tool `setup_verify` and the button of the venv detail of the dashboard
  run that one function, so the three front ends leave the same record.
- `bernie setup log [STEP] [-f]` prints the log of a step of the newest run
  and keeps printing it with `-f`.
- `bernie status --steps` carries the time of every step of the newest run.

The button of the Setup tab is a setup run: it writes the same record, and
the page reads it ([§11](#11-dashboard)). The venv detail of the tab holds
a second button, `Verify again`: it starts the check of the venv in the
background (`POST /api/setup/verify`) and the page reads the new record
when the job ends. The MCP server reads the record with the
`setup_report` tool ([§13.4](#134-tools)).

## 7. Recipes

### 7.1 Files and lookup

A recipe is one YAML file. The file name stem is the recipe id. bernie loads
recipes in layers, in this order:

1. Built-in recipes (embedded from `recipes/`).
2. `<config>/recipes/`.
3. Each directory in `$BERNIE_RECIPES` (`:`-separated).
4. The files and directories given to `recipe validate`.

Rules:

- Each directory is read recursively. Every `*.yaml` (and `*.yml` outside the
  built-in tree) below it is a recipe. Folders only group files. The folder
  never changes the id.
- An id is unique inside one layer. Two files with the same id in one layer
  are a load error. The error names both paths. The first file stays.
- A later layer replaces an earlier recipe with the same id.


A file that does not parse or validate is reported by `recipe list` and
`recipe validate`. It is never dropped silently.

### 7.2 Schema

```yaml
schema: 1                       # required, integer
extends: [_base, _rdna-ar]      # optional; fragments or recipes, merged left to right
name: Qwen3.8 Flash-Next AWQ + MTP=2
description: One paragraph. What, why, validated where.
status: validated               # validated | experimental | reference
priority: 30                    # higher wins among recipes that fit (used by `up`); default 0
tags: [qwen, moe, mtp]
entrypoint: cli                 # cli = `vllm serve` (default); api = openai.api_server (as validated by the fork)

model:
  source: wtdcode/Qwen3.8-Flash-Next-AWQ-W4A16   # HF repo id or absolute path
  served_name: flash-next
  revision: main                # optional

requires:
  arch: [gfx1030]               # gfx targets or prefixes ("gfx11" matches gfx1100)
  gpus: 4                       # GPUs of a matching arch
  vram_gib: 28                  # per GPU
  ram_gib: 64                   # host RAM
  engine: vllm-rdna             # vllm | vllm-rdna (needs the fork's kernels)
  download_gb: 21               # optional; the model download in GB (10^9 bytes)
  kv_bytes_per_token: 78356     # optional; KV cache of one token over all ranks, measured

inputs:                         # values the operator gives with --set NAME=VALUE
  PLE_QUANT_DIR:
    description: ples_int4 directory from primitive-ai/Qwen3.8-Flash-Next-PLE-quant
    required: true
    path: true                  # mounted into the container at the same path
    default: ""

container:                      # omit (or null) = host mode only: the image lacks this feature
  image: docker.io/blivioniag/vllm-rdna:v0.31.0-extras
  tunableop_profile: rocm7.14-rocblas5.5   # rows for the image's rocBLAS build
  shm_size: 16g
  pull: true                    # pull before start (tags are refreshed in place); --no-pull skips
  device_access: auto           # classic | cdi | auto (auto: CDI when the host can, else classic)

env:                            # environment for vLLM; null removes a key
  VLLM_RDNA_AR: "1"

args:                           # vllm serve flags without the leading --
  tensor-parallel-size: 4
  max-model-len: 262144
  enable-prefix-caching: true
  compilation-config: {mode: 3, cudagraph_mode: FULL_AND_PIECEWISE}

why:                            # optional; one short reason per setting
  env.VLLM_ROCM_USE_AITER: AITER has no RDNA2 kernels.
  args.max-num-seqs: Eight slots match the validated serving profile.
  tuning.tunableop_profile: The solver rows match this rocBLAS build.
```

### 7.3 Merge rules

- `extends` is resolved depth first, then left to right, then the file itself.
- Maps merge key by key. Lists replace. Scalars replace.
- The `why` map merges key by key. A later layer replaces the reason for the
  same setting path. A `null` reason removes an inherited reason.
- A `null` value deletes the key (for example, to remove an inherited flag).
- A cycle in `extends` is an error.

### 7.4 Interpolation

`${NAME}` in any string of `model`, `env` or `args` is replaced by the input
`NAME`. A required input with no value is an error at plan time. An unknown
`${NAME}` is a validation error.

### 7.5 Overrides (`--set`)

- `--set KEY=VALUE` where KEY has no dot sets the input KEY.
- `--set section.key=VALUE` sets a field, for example
  `--set args.max-model-len=32768` or `--set env.VLLM_RDNA_AR=0`.
- VALUE is parsed as YAML: `true`, `32768`, `null`, `{a: 1}`, `[1, 2]`.
- Precedence: recipe < config defaults < `--set`.

### 7.6 Argument rendering

- The command is `vllm serve <model.source>` followed by `args` sorted by key.
- `true` gives `--key`. `false` and `null` give nothing.
- A map or list gives `--key '<compact JSON>'`.
- Other values give `--key VALUE`.
- `served-model-name` comes from `model.served_name`. `revision` comes from
  `model.revision`.

### 7.7 GPU selection

When the plan has no `HIP_VISIBLE_DEVICES`, bernie selects GPUs:

1. Keep GPUs whose arch matches `requires.arch`.
2. Need `requires.gpus` of them, else the recipe does not fit.
3. Prefer a set that shares one PCIe switch (`PIX`), then the lowest HIP
   indices.

`recipe plan` shows the selection and the reason.

#### The visible GPUs

`HIP_VISIBLE_DEVICES` and `ROCR_VISIBLE_DEVICES` limit the GPUs of the
host. When one of them is set in the environment of bernie, bernie
considers only the GPUs it names: the fit of a recipe, the selection, the
plan, `status`, `hw` and `doctor` all see the visible set only.

- The value is a comma list of HIP indices: the numbering of the KFD
  topology, the same numbering bernie writes into `HIP_VISIBLE_DEVICES` of
  the child process. A UUID or any other form is ignored with a warning.
- The first variable that is set wins.
- An index the host does not have is an error. The command stops and says
  which index is wrong.
- The plan keeps the host numbering: with `HIP_VISIBLE_DEVICES=8` the plan
  of a one-GPU recipe selects GPU 8 and writes `HIP_VISIBLE_DEVICES=8` for
  the child process.
- `--set env.HIP_VISIBLE_DEVICES=...` still wins over the selection, but
  it must name visible GPUs. A GPU outside the restriction is an error.
- `status`, `hw` and `recipe plan` say in one line that a restriction is
  active: `GPUs limited to 8 by HIP_VISIBLE_DEVICES`.

### 7.8 Fit

`recipe list` gives each recipe a fit in `fit.status`, with the reasons
in `fit.reasons`:

- `fits`: the recipe runs on this host as written. `reasons` is empty.
- `tight`: the recipe runs on this host after one change. Each reason
  names the change.
- `no`: the host cannot run the recipe. Reasons: arch, GPU count, VRAM,
  RAM.
- `unknown`: no GPU data.

A recipe is `tight` when it passes the `no` checks and one of these holds:

- `disk`: the model download (`requires.download_gb`) is larger than the
  free space of the Hugging Face cache. A model the cache holds, or a local
  path, needs no download.
- `kv`: one request of `args.max-model-len` tokens needs more KV cache
  (`requires.kv_bytes_per_token` times the length) than the budget of a
  replica. The budget is `args.kv-cache-memory-bytes` times the ranks of a
  replica when the recipe sets it. Otherwise it is an estimate:
  `gpu-memory-utilization` (default 0.9) of the smallest matching card,
  times the ranks, minus the weights (`requires.download_gb`). A recipe
  without `kv_bytes_per_token` or a numeric `max-model-len` skips the check.

`kv_bytes_per_token` is a measured value: the KV reserve divided by the
tokens vLLM reports for it, at the recipe's parallel layout and cache
dtype. The ranks of a replica are `requires.gpus` divided by
`data-parallel-size`.

`up` and `status` pick a recipe that `fits` before one that is `tight`.

### 7.9 Setting reasons

A recipe may attach a short reason to a setting with the optional `why` map.
A key is the path of the setting:

- `env.NAME` names an environment variable.
- `args.FLAG` names a `vllm serve` flag without `--`.
- `tuning.tunableop_profile` names the TunableOp profile choice.

An `env` or `args` reason must name a setting in the resolved recipe. A
reason is one non-empty line. An override changes the value but keeps the
reason. Remove an inherited reason with `null` when it no longer applies.

`recipe plan` shows each reason under its setting. The text plan prints the
setting path, value and reason. The JSON plan keeps the existing `env` map
and `args` array unchanged. It adds a `settings` array with one object per
annotated setting:

```json
{"setting": "env.GPU_MAX_HW_QUEUES", "value": "4",
 "why": "Four queues avoid skew in the persistent fork kernels."}
```

The `settings` array is sorted by `setting`. `value` is the final rendered
value as a string. The array is absent when the recipe has no reasons, so a
legacy recipe keeps the same plan JSON. The MCP `recipe_plan` tool and the
dashboard use this same plan document.

### 7.10 Plan adjustments

A plan changes a recipe setting only to make the recipe fit this host. Each
change is one entry of `adjustments` in the JSON plan, with `setting`,
`from`, `to` and `reason`. The text plan prints it as `# adjusted:`, and
`serve`, `compose` and `up` print it before they start. The field is absent
when the plan changed nothing.

The one adjustment is the context:

- When one request of `args.max-model-len` does not fit the KV budget
  (§7.8), the plan lowers `max-model-len` to the largest of 32768, 16384
  and 8192 that is smaller than the old value and fits.
- When none fits, the plan keeps the value and adds a warning that names
  the budget.
- An override (a config default or `--set`) of `args.max-model-len` is
  never changed. When it does not fit, the plan keeps it and warns.
- An override of `args.kv-cache-memory-bytes` changes the budget the plan
  compares with.

## 8. Runs

### 8.1 Host mode

- Python: `<venv>/bin/python -m vllm.entrypoints.cli.main serve <model> ...`
  for `entrypoint: cli`, or
  `<venv>/bin/python -m vllm.entrypoints.openai.api_server --model <model> ...`
  for `entrypoint: api`. Container mode uses the same choice
  (`--entrypoint python3` with the module for `api`).
- `ROCM_HOME`, `ROCM_PATH`, `HIP_PATH` come from the detected ROCm install
  (never a hard-coded path) unless the recipe or `--set` gives them. The
  install is the tree whose tools win on `PATH`, else the newest install
  that has a `hipcc`. A tree nested in another one counts: a TheRock tree
  inside a package install is the one the operator runs, and it is newer
  than the package around it.
- `serve` refuses a port that another process already listens on. The
  message names the run that holds the port, the command that stops it and
  the flag that picks another port. When no run holds the port, the message
  also names `bernie attach <port>`, which registers a vLLM server that
  bernie did not start.
- `HF_HUB_OFFLINE` and `TRANSFORMERS_OFFLINE` are `1` when `model.source`
  is a local path, and `0` when it is a Hugging Face repo id.
- Host mode needs the fork built in the venv for `requires.engine: vllm-rdna`.
  It is not "the container without Docker".
- Venv: the order of [§6.2](#62-the-host-venv): `--venv`, `$BERNIE_VENV`,
  config `venv`, `$VIRTUAL_ENV`, then one conventional venv.
- `LD_LIBRARY_PATH` starts with the venv's ROCm wheel libraries when present:
  `_rocm_sdk_libraries/lib`, `_rocm_sdk_core/lib`,
  `_rocm_sdk_core/lib/{host-math,rocm_sysdeps,core}/lib`,
  `torch/lib`. Reason: the system rocBLAS must not load before the wheel one.
- Caches: `VLLM_CACHE_ROOT`, `TRITON_CACHE_DIR`, `TORCHINDUCTOR_CACHE_DIR`
  under `<cache>/runs/<name>/`.
- The process starts in a new session. The working directory is the run
  directory (never the vLLM source tree: `vllm/entrypoints/cli/openai.py`
  shadows the `openai` package).
- `stop` sends SIGTERM to the process group, waits 20 s, then SIGKILL.

### 8.2 Container mode

The run command is:

```
<engine> run -d --name bernie-<name> --label bernie.run=<name>
  --network host --ipc host --shm-size <shm>
  --device /dev/kfd --device /dev/dri --group-add <gid> --group-add <gid>
  --security-opt seccomp=unconfined --security-opt label=disable
  -v <hf-cache>:/root/.cache/huggingface
  -v <tunableop rows>:/bernie/tunableop:ro
  -v <tuned-moe dir>:/bernie/tuned-moe:ro   (when the recipe sets VLLM_TUNED_CONFIG_FOLDER)
  -v <path input>:<path input>         (one per path input)
  -e KEY=VALUE ...
  <image> serve <model> <args>
```

The group flags carry the numeric ids of the GPU groups of the host, never
the names. The engine resolves a group name inside the image, and the image
has no `/etc/group` to resolve it in. bernie reads the ids from the owner of
the device nodes (`/dev/kfd`, `/dev/dri/renderD*`, `/dev/dri/card*`), and
falls back to the group database (`getent group video`, `getent group
render`, then `/etc/group`). The plan carries the ids and their source in
`group_add`:

```json
"group_add": [{"role": "video", "gid": 39, "source": "/dev/dri/card0"},
              {"role": "render", "gid": 105, "source": "/dev/kfd"}]
```

Rootless Podman uses `--group-add keep-groups` instead of the group flags.
Its container runs in its own user namespace, where a group id of the host
is not mapped: only the groups of the user are. `compose` writes the same
plan as a Compose file that works with `docker compose`, `podman compose`
and `podman-compose`.

#### 8.2.1 Device access

A container gets the GPU in one of two ways:

- `classic`: the device nodes of the host are passed in (`--device /dev/kfd
  --device /dev/dri`). Every GPU of the host is visible in the container.
- `cdi`: the AMD Container Toolkit injects the GPUs through a CDI spec
  (`--device amd.com/gpu=<n>`). Only the selected GPUs are visible in the
  container.

The mode is `classic`, `cdi` or `auto` (the default). It comes from the
recipe field `container.device_access`, the `--device-access` flag and the
config key `device_access`: the flag wins, then the config, then the
recipe. `auto` uses CDI when the host has a valid `amd.com/gpu` spec and
the engine can use it, else the classic form. A host without the toolkit
changes nothing.

The plan carries the decision:

| Field | Meaning |
|---|---|
| `device_access_request` | the mode that was asked for: `classic`, `cdi` or `auto` |
| `device_access` | the mode the run uses: `classic` or `cdi` |
| `device_access_reason` | one sentence saying why that mode was chosen |
| `cdi_devices` | the CDI device names of the selected GPUs |

The index of a CDI device is the PCI-address order of the GPUs of the
host, not the HIP order that `HIP_VISIBLE_DEVICES` uses. The plan keeps
both index sets: `gpus` are the host HIP indices and
`container_hip_devices` are the indices inside the container. The
renumbering of the HIP indices inside a CDI container is not confirmed on
a live host yet, so the plan warns when the two sets differ.

In classic mode the device flags of the command are `--device /dev/kfd
--device /dev/dri`. In CDI mode they are `--device amd.com/gpu=<n>`, one
per selected GPU (`amd.com/gpu=all` when every GPU of the host is
selected), and the group flags stay: the CDI spec sets the owner and the
mode of the injected nodes, but it adds no group to the process of the
container. A run never mixes the two forms: the classic form passes every
render node of the host.

The Compose file carries the same devices in its `devices:` list. That the
compose engine accepts a CDI name there is not confirmed on a live host
yet: the generated file says so and carries the classic form as a
comment.

### 8.3 Run record

`<state>/runs/<name>/run.json`:

```json
{"name": "flashnext-mtp2", "recipe": "flashnext-mtp2", "mode": "host",
 "pid": 1234, "container": "", "engine": "", "host": "127.0.0.1", "port": 18096,
 "started_at": "RFC3339", "log": "<state>/runs/<name>/serve.log",
 "image": "", "image_digest": "", "plan": {...}}
```

`image_digest` is set in container mode from `<engine> image inspect` after
the pull. Tags such as `v0.31.0-extras` are refreshed in place, so the tag
alone does not identify the image.

`ps` state is derived from the process (or container), `/health` and one
completion. The state is not stored in `run.json`:

| State | Meaning |
|---|---|
| `starting` | The process or container is alive. `/health` does not answer 200, or the server has not answered a completion yet. |
| `running` | The process or container is alive. `/health` answers 200 and the server answered a one-token completion. |
| `exited` | The process or container is gone, whether `stop` ended it or it failed. |

A run is `running` only when it answers. Once `/health` answers, bernie
sends one streaming completion of `max_tokens: 1` to the served model (the
plan's served name, else the first id of `/v1/models`), with a 10 second
limit. The first answer is recorded in `<state>/runs/<name>/answered` with
the start time of the run, so a later `ps` does not ask again and a new run
of the same name asks afresh.

A `starting` run has a `waiting` string in `ps --json` and `status --json`:
what it waits for, for example `the server answers /health but not a
completion yet: HTTP 503`. The field is absent for a `running` or `exited`
run.

An exited run has a `findings` array in `ps --json` and `status --json` when
the scan of its log ([§5.3](#53-log-doctor)) matched at least one rule. Each
entry is the `Match` object of §5.3. The field is absent for a run that is
`starting` or `running`, and for an exited run without a match.

A run that is `starting` or `running` has a `phase` object in `ps --json` and
`status --json`: what the tail of its log says the server is doing now
([§5.4](#54-log-phase)). The field is absent when the log says nothing
known.

`doctor --run NAME` explains a failed start ([§5.3](#53-log-doctor)). One
name has one run. `serve` refuses a name that is not `exited`.

## 9. Tuning

### 9.1 TunableOp

- Rows are build-specific and architecture-specific. A profile is valid only
  for the rocBLAS build (sha256 of `librocblas.so.5`) that produced it and for
  the GPU architectures in its manifest.
- Profiles live in `tunableop/<arch>/<profile>/`, embedded in the binary. Each
  profile has a `profile.json` manifest (`name`, `arch`, `rocblas`, `rocm`,
  `torch`, `lib_sha256`, `rows_per_rank`, `status`, `covers`) and its
  `tunableop_results<rank>.csv` files. There is no central registry: bernie
  discovers profiles by walking the embedded tree. `name` equals the folder
  name and is unique across architectures. `arch` lists gfx names or prefixes
  (`gfx1030`, `gfx103`, `gfx11`).
- `env` and `serve` materialize the rows into `<data>/tunableop/<profile>/`.
- Host mode: bernie hashes the loaded `librocblas.so.5` (venv wheel first,
  then `/opt/rocm/lib`) and selects the profile with the same sha256 prefix
  (12 hex) whose `arch` names the architecture of every selected GPU. An
  explicit profile must match both the hash and the architecture, else it is
  an error.
- No match: per-user rows at `$HOME/.cache/tunableop/tunableop_results.csv`.
- Container mode: the recipe names the profile for its pinned image. The
  profile must still fit the selected GPUs.
- Lookup only by default (`PYTORCH_TUNABLEOP_TUNING=0`). `--tuning` enables
  tuning into the per-user file.
- Environment set: `PYTORCH_TUNABLEOP_ENABLED=1`, `PYTORCH_TUNABLEOP_TUNING`,
  `PYTORCH_TUNABLEOP_HIPBLASLT_ENABLED=0`, `PYTORCH_TUNABLEOP_FILENAME`.
- Tuned fused-MoE configs follow the same keying: `tuned-moe/<arch>/` holds a
  flat folder of vLLM's `E=...json` files. A folder name is a gfx name or a
  prefix; the longest name that prefixes the GPU's gfx wins. Host mode sets
  `VLLM_TUNED_CONFIG_FOLDER` to the materialized folder of the selected GPUs'
  architecture (`<data>/tuned-moe/<arch>`). An architecture without a folder,
  and a selection that spans several architectures, gets no variable.

### 9.2 Power

`tune power status` reads `power1_cap`, `power1_cap_min` and
`power1_cap_max` of every GPU with a hwmon power cap.

#### Desired state

The desired state is `<config>/power.yaml`. It is YAML with a version and a
map of GPU selectors to watts:

```yaml
version: 1
gpus:
  all: 180
  "0": 160
  "0000:47:00.0": 150
```

A selector is `all`, a HIP index, a PCI address, or the KFD unique id of one
GPU. A GPU-specific selector overrides `all`. A selector that matches no GPU
is ignored. A value must be a positive, finite number of watts.

`tune power set` writes this file. It does not need root and it never writes
hwmon. Without `--gpu`, it writes the `all` selector. With repeatable
`--gpu`, it writes one selector per HIP index. A later write to the same
selector replaces its value.

One value has to fit every card it touches, so the wanted value is clamped
before it is written: to the card's own `power1_cap_min` and `power1_cap_max`
for one card, and to the range the cards share (the highest `power1_cap_min`
and the lowest `power1_cap_max` of the cards the command touches) for `all`.
The answer names the cards whose own range did not hold the wanted value in
its `clamped` field; they take the clamped value. Cards with no power range
are left alone.

`tune power apply [--file FILE]` reads the desired state. `FILE` defaults to
`<config>/power.yaml`. The command needs root because it writes hwmon. As a
normal user it writes nothing, prints the exact one-line root command and
exits with code 3 and the words `needs root`. It never runs `sudo`.

For every GPU with a desired value, `apply` clamps the value to that GPU's
observed `power1_cap_min` and `power1_cap_max`. It writes only
`power1_cap`, in microwatts. It skips a GPU whose cap already has the
clamped value. It prints or logs each changed, unchanged and clamped GPU, so
the result is idempotent and auditable.

#### Service

`tune power service` manages a systemd unit pair:

- `bernie-power.service` is a oneshot service. It runs
  `<absolute bernie path> tune power apply --file <config>/power.yaml` at
  boot.
- `bernie-power.path` watches `<config>/power.yaml`. A change starts the
  service again.

The unit pair uses these hardening rules:

- no network access;
- a read-only system and home;
- write access only to `/sys/class/drm` and `/sys/class/hwmon`;
- `NoNewPrivileges`, `PrivateTmp`, `ProtectKernelTunables`,
  `ProtectKernelLogs`, `ProtectControlGroups`, `LockPersonality`,
  `MemoryDenyWriteExecute` and `SystemCallArchitectures=native`.

The generated files live under `<state>/power/`. `service print` writes them
and prints their contents. `service install-plan` writes them and prints
root commands that copy them to `/etc/systemd/system`, reload systemd and
enable the pair. `service remove-plan` prints the inverse root commands.
bernie never runs those commands.

`service status` reports whether the pair is installed, enabled and active.
It also reports the desired and effective value, the actual cap and drift
for every GPU. A missing `systemctl` makes the service state `skip`; the
GPU report still works. A specific desired value outside the observed range
is shown with its effective clamped value.

The advice comes from the device tables ([§4.4](#44-device-tables)): a row
with `power_cap_advice_w` gives the recommended cap of that device (V620:
180 W). `doctor` compares each GPU's cap with it. A device without that
field has no advice.

## 10. Benchmark

`bernie bench` sends streaming requests to `/v1/completions`.

| Flag | Default | Meaning |
|---|---|---|
| `--url` | the only running run, else `http://127.0.0.1:8000` | Server base URL. |
| `--model` | first id of `/v1/models` | Model name. |
| `--input-len` | `1024` | Prompt tokens (calibrated with `/tokenize` when present). |
| `--output-len` | `512` | Output tokens (`ignore_eos`, temperature 0). |
| `--concurrency` | `1` | Comma list, for example `1,8`. One cell per value. |
| `--num-prompts` | `max(4, 2 × concurrency)` | Requests per cell. |
| `--warmup` | `1` | Requests sent and discarded before each cell. |
| `--seed` | `0` | Prompt seed. |
| `--tag` | none | Free label stored with the result. |
| `--recall` | off | Hide a short random fact in every prompt and check the reply. |
| `--suite` | none | Run a built-in suite instead of the flags above. The suite decides the cells, the repetitions, the warm-up and the recall gate. |

Every prompt gets a unique salt so the prefix cache does not change the
result. Prompts are sized with `/tokenize` when the server has it. Without
it, the word count is an estimate and the real mean is in the result.
With MTP, a chunk can carry several tokens. bernie divides the gap of a
chunk by its token count, so ITL is per token.

With `--recall`, every prompt hides a short random fact (a six-digit pass
key) at 50 % of its depth and asks for it at the end. The reply passes when
it holds the pass key. The key is salted per request, so two requests of a
run never share one. The prompt is sized with `/tokenize` like every other
prompt; without it the length is an estimate, and the cell says which one it
was in `recall.method`. The reply has a fixed length (`ignore_eos` where the
server supports it, temperature 0). A cell that misses the fact has broken
attention: its speed numbers mean nothing.

Results go to `<state>/bench/<id>.json` (id = UTC timestamp + short hash).
The fingerprint has: host, GPU summary, recipe name, image digest (container
runs) and run plan hash. The GPU summary lists only the GPUs used by the run:
bernie reads `HIP_VISIBLE_DEVICES` and the selected GPU indices from the plan
in the matching run record. When there is no matching run record, or its plan
has no GPU selection, the summary lists every host GPU and `gpu_scope` is
`all (run selection unknown)`. A known run selection has `gpu_scope` `run`.
The text and dashboard history show the same summary and scope.

### 10.1 What the numbers mean

Every cell reports five groups of numbers. The unit is part of the name:
seconds for the time to first token, milliseconds for the inter-token
latency, tokens per second for the speeds.

| Group | Definition |
|---|---|
| Prefill, per request | `prompt_tokens ÷ ttft_s`. The time to first token holds the queueing and the scheduling of the request, so this is the rate the caller saw, not the speed of the engine alone. A prompt the server answered from its prefix cache still counts all of its tokens; the cell reports `mean_cached_prompt_tokens` when the server says how many were cached. |
| Prefill, per cell | `Σ prompt_tokens ÷ prefill window`. The window runs from the send of the first request of the cell to the first token of the last request. It holds the queueing of the cell, so the rate is the throughput of the prefill under that concurrency. |
| Decode, per request | `(output_tokens − 1) ÷ (last token time − first token time)`. The first token is left out: it came out of the prefill. |
| Decode, per cell | `Σ decode tokens ÷ decode window`. The window is the union of the decode intervals of the requests: the time in which at least one request was decoding. It leaves out the prefill of the later requests and the idle gaps, so it is the rate the server reached while it decoded. |
| Time to first token | `ttft_s`: from the send of the request to its first token. |
| Inter-token latency | `itl_ms`: the gap between two chunks of the stream. A chunk with several tokens (MTP) is one gap for all of them: the gap is divided by the number of tokens in the chunk and the per-token latency is recorded once per token. The percentiles describe tokens, not chunks. |
| Output rate | `output_tok_s`: all completion tokens of the cell ÷ the wall time of the cell. |

Every distribution holds `mean`, `p50`, `p95`, `min` and `max`. The text
and the dashboard show the mean, p50 and p95; the JSON has all five.

The token counts come from the server's `usage`
(`stream_options: include_usage`). `tokens_exact` is true when every
request of the cell sent usage. False means the counts are estimates (one
token per chunk).

When the server has a `/metrics` endpoint, bernie reads its latency
histograms (`vllm:time_to_first_token_seconds` and
`vllm:inter_token_latency_seconds`) before and after every cell. The
difference is what the server measured over the same requests, and the
cell keeps it under `server_side`. A server without metrics has none, and
the cell says nothing about it.

The text output has one table per cell. Its rows are the groups above, its
columns the aggregate rate and the per-request distribution. The units are
in the row names (`prefill tok/s`, `decode tok/s`, `TTFT s`, `ITL ms`). A
number that was not measured shows as `–`. One line above the tables says
how the prefill and the decode numbers are measured. The dashboard shows
the same tables and the same units.

Cell fields:

| Field | Meaning |
|---|---|
| `concurrency`, `num_prompts`, `completed`, `failed` | Cell size and outcome. |
| `prefill` | `agg_tok_s` (the rate of the cell), `window_s` (the time it was measured over) and `req` (the distribution of the per-request rates). |
| `decode` | The same shape as `prefill`. |
| `ttft_s` | Distribution of the time to first token in seconds. |
| `itl_ms` | Distribution of the inter-token latency in milliseconds. |
| `server_side` | What the server measured over the same requests: `ttft_s`, `itl_ms`, `count` and `source`. Absent when the server has no metrics. |
| `tokens_exact` | True when the token counts come from the server's usage. |
| `ttft_p50_ms`, `ttft_p95_ms` | Time to first token in milliseconds (the same numbers as `ttft_s`). |
| `itl_p50_ms`, `itl_p95_ms` | Inter-token latency (the same numbers as `itl_ms`). |
| `decode_tok_s` | Mean of the per-request decode rates. |
| `prefill_tok_s` | Mean of the per-request prefill rates. |
| `output_tok_s` | All completion tokens ÷ cell wall time. |
| `mean_prompt_tokens`, `mean_output_tokens`, `mean_cached_prompt_tokens`, `tokens_per_chunk` | Real sizes seen. |
| `duration_s`, `errors` | Wall time; sample of error strings. |
| `recall` | Pass key check of the cell: `asked`, `passed`, `depth`, `method`. Absent when the check did not run. |
| `check` | The declared-answer verdict of the cell: `asked`, `passed`, `method`. It is separate from `recall`, so a group of kind `check` stays distinguishable in one result. Absent when the check did not run. |
| `group`, `input_len`, `output_len` | The suite group of the cell and the lens of that group. Absent in a plain run. |
| `kind` | What the cell asserts: `throughput` (a speed) or `check` (a declared answer). Absent in a plain run. |
| `reps` | The repetitions of the cell. Each entry holds the numbers of one repetition. |
| `spread` | The min/max ratio of the metrics over the repetitions. |
| `incomplete` | True when the cell lost a warm-up or a repetition. Its numbers stay visible, but no verdict is issued for it. |
| `warmup_failed` | The number of warm-up requests that failed. |
| `size_valid`, `input_size_percent` | The drift of the measured prompt length and whether it stays inside the tolerance of the run (2 % by default). |
| `output_valid` | False when the replies of the cell are degenerate: fewer than 8 % distinct words. |
| `stream_incremental` | False when a reply arrived in one chunk: the server buffered the whole answer, so there is no decode window. |

A cell where every request fails makes `bench` exit 1 and saves nothing.

The gates name why a number may not be a speed number. A cell that is off
size, degenerate or buffered keeps its numbers visible, because hiding a
cell is worse than labelling it. The verdict reads them as labels, not as
gates: only a failed recall, an incomplete cell and a missing cell stop a
verdict.

A result also has a `workload`: `protocol_id`, `corpus_sha256`,
`input_len`, `output_len`, `concurrency`, `recall` and `check`. It says what the
benchmark asked for. Results saved before the field existed have none. A
suite run adds `suite_id`, `suite_digest` and `kind`. A result of a suite
run also has a `suite` block: the id, the digest and the version of the
suite.

The `protocol_id` names the shape of the measurement. It is
`bernie-bench-2`: the groups of §10.1. A result saved before it
(`bernie-bench-1`) still loads: its millisecond fields become the new
distributions, and what it did not measure shows as `–`.

`bernie bench compare <id-a> <id-b>` puts two results side by side. It
refuses (exit 1, "workload differs") when the workloads differ: the
numbers of two different workloads cannot be compared. Two results of
different protocol ids are refused too: they measured different things,
and only the numbers both of them measured could be compared.

### 10.2 Suites

A suite is the data of a benchmark run: the cells, the repetitions and the
thresholds that decide whether two runs agree. The suites are embedded YAML
files under `bench/suites/`. The file stem is the suite id. `bernie bench
suites` lists them with their digest and their groups.

`bernie bench --suite NAME` runs a suite. The suite decides the lens of
its cells, the concurrency levels, the repetitions, the warm-up, the seed
and the recall gate. `--url`, `--model`, `--tag`, `--num-prompts` and
`--seed` override the suite. `--input-len`, `--output-len`,
`--concurrency`, `--warmup` and `--recall` cannot be used with `--suite`:
the suite decides the cells, and a half-overridden cell is no cell.

In the dashboard and over MCP a request that names a suite ignores its
cell flags: the suite decides them.

A suite file:

```yaml
schema: 1
id: pr
name: Pull request suite
description: The cells a pull request must pass.
reps: 3
warmup: 1
seed: 1234
recall: true
request_timeout_s: 3600
cooldown_s: 5
size_tolerance_percent: 2
defaults:
  requests: 0
compare:
  band: 0.02
  steady: 0.95
groups:
  - name: 1k
    kind: throughput
    input_len: 1024
    output_len: 512
    concurrency: [1, 4, 8]
    reps: 3
    est_s: 60
```

The loader is strict: an unknown key, a `schema` other than `1`, a group
without a lens or without a level, a negative length or an empty group
list is refused. The defaults of a file that leaves a value out: `reps` 1,
`warmup` 0, `seed` 0, `recall` off, `band` 0.02, `steady` 0.95,
`size_tolerance_percent` 2, the kind `throughput` and the source
`synthetic`.

A group of kind `check` asserts a declared answer instead of a speed. It
needs a `check` block with a `type`; the only known type is `passkey`,
which uses the same pass key machinery as the recall gate. The verdict of
a check group lands in the cell field `check`, not in `recall`, so the two
kinds stay distinguishable in one result. A failed check makes a
comparison invalid, like a failed recall. The `kind` of the workload is
`check` when a group of the suite is a check group, else `throughput`.

The built-in `pr` suite has the fixed cells of a pull request: 1024 prompt
tokens and 512 output tokens at the levels 1, 4 and 8, and 16384 prompt
tokens and 1024 output tokens at the same levels. The built-in `smoke`
suite has one short cell: it proves that the server answers. The built-in
`long-context` suite is a ladder: one cell per prompt length, from 2048 to
16384 tokens, at the level 1. It shows how the prefill and the time to
first token grow with the prompt, and the dashboard draws it as the length
curve.

The digest of a suite is the sha256 of its canonical form. Every result of
a suite run carries the digest, so two runs of different suites are never
compared.

A suite run has one result with the cells of every group. A cell names its
group in `group` and the lens of the group in `input_len` and
`output_len`. The `workload` of a suite run carries `suite_id` and
`suite_digest`; its `input_len` and `output_len` are zero, because the lens
of a suite run is per cell.

Every cell of a group runs `reps` times. The numbers of the cell are the
medians over those repetitions. The repetitions themselves are in the cell
under `reps`, so a reader can recompute the medians. The `spread` of the
cell is the min/max ratio of every metric over the repetitions: a spread
near 1 means the repetitions agree.

The suite pauses `cooldown_s` seconds between two repetitions. The pause
keeps the card out of a thermal and power steady state, so the repetitions
measure the same work.

A cell is `incomplete` when a warm-up request failed or a repetition lost a
request. The numbers of an incomplete cell stay visible, because a hidden
cell is worse than a labelled one, but no verdict is issued for it.

The fingerprint of a result records the power cap and the clocks of the
cards of the run, when the host publishes them. They say what the cards
did while the benchmark ran.

### 10.3 The verdict

`bernie bench compare <id-a> <id-b>` says, for every metric of every cell,
whether the second run is better, the same or worse than the first. The
metrics are the five groups of §10.1: the prefill speed, the decode speed,
the time to first token, the inter-token latency and the output rate. A
rate is better when it is bigger, a latency when it is smaller.

The rules:

- The two results must measure the same workload. Otherwise the comparison
  is refused (exit 1, "workload differs").
- A recall that failed, a cell that lost a request (`incomplete`) and a
  cell that only one of the runs has make the comparison invalid. No
  verdict is issued for it.
- The noise band of a metric is the wider of the band of the suite and the
  spread the two arms measured.
- The repetitions of an arm must agree within `compare.steady`. An arm
  whose repetitions disagree gets no verdict for that metric: the metric
  is `noisy`.
- A metric is `worse` when the ratio of the two runs is beyond the band and
  every ratio of a repetition of the second run to a repetition of the
  first is on the same side. It is `better` when the same holds in the
  other direction. Otherwise it is `same`.

The exit codes:

| Code | Meaning |
|---|---|
| 0 | The comparison is valid and no metric is worse. |
| 3 | A metric is worse. The table is printed. |
| 1 | The comparison could not decide: the results are not comparable, a cell is incomplete, the recall failed or a metric was too noisy. |

`--band P` replaces the band of the suite.

`--markdown` prints the comparison as a markdown receipt instead of the
text table. The receipt has the scope of the two runs (the ids, the model,
the server, the suite and its digest, the host, the cards and their
scope), the correctness of the two runs (the recall gate, the declared
answer check, the failed requests, the failed warm-ups and the incomplete
cells), the numbers of every cell with the verdict of every metric, the
verdict of the run and the command that reproduces it. It is the text that
goes into a pull request.

### 10.4 The A/B run

`bernie bench ab --base ARM --cand ARM` runs a baseline and a candidate back
to back and tells the verdict of the two arms. An arm is a recipe id or the
URL of a server.

The visits run in the order base, cand, cand, base. `--visits` changes the
order. The arms alternate, so a drift of the machine hits both of them the
same way. One server runs at a time: when an arm is a recipe, its server
starts before the visit and stops after it. The bench lock is held for the
whole run, so no other benchmark interleaves.

Every visit saves its own result under the tag `ab:<arm>:<number>`, so the
visits can be inspected with `bernie bench show`.

The verdict compares the medians of the visits of the two arms. Its rules
are the rules of §10.3.

## 11. Dashboard

`bernie dash` serves an embedded web UI and a JSON API. It listens on
`127.0.0.1:7860` by default: the port is closed to the network, and a
remote operator reaches it through an SSH tunnel
(`ssh -L 7860:127.0.0.1:7860 <host>`), which the command prints when it
starts. `--host` (or `--listen 0.0.0.0:7860`) opens the port to the
network instead; the LAN URLs are then printed, each with the token. On a
non-loopback address the dashboard runs in token mode: every request needs
the token (`Authorization: Bearer <t>` or the cookie). The token is
`--token`, else `$BERNIE_DASH_TOKEN`, else the persisted token (generated
once, mode 0600 under the state directory), else a random token printed at
start. POST, PUT and DELETE requests also need the header `X-Bernie: 1`.
When no token is set, the `Host` header must be a loopback name (this
blocks DNS rebinding). Errors are `{"error": "...", "exit_code": N}`.

`up` starts the dashboard before the server is healthy
([§3.1](#31-the-default-run-the-binary)). The Runs tab then shows the run
as `starting`, with the phase sentence of its log and the log itself. The
page asks for nothing that a run without a health answer does not have.
The header says when the environment limits the GPUs bernie uses
([§7.7](#77-gpu-selection)).

### 11.1 The dashboard service

`bernie dash start` runs the dashboard as a service that outlives the
shell that started it. `bernie dash stop` ends it. `bernie dash status`
prints its state and the URLs to open. The three commands and the
installer flag `--start` are the same operation
([§3](#3-command-surface)); the MCP server exposes the same three
operations as tools ([§13.4](#134-tools)).

The state of the service lives under `<state>/dashboard/`:

| File | Mode | What it holds |
|---|---|---|
| `token` | 0600 | the bearer token the guard reads. The service writes one when it starts and reuses it after a restart. |
| `dash.pid` | 0644 | the PID of the dashboard process. |
| `dash.log` | 0644 | the output of the dashboard process. |
| `startup.json` | 0600 | the URL the dashboard bound to, its PID and the time it started. |

The token rules:

- The token file is written once (0600) and reused across restarts. It
  changes only with `--rotate-token`.
- A listen address that is not loopback always runs in token mode: the
  token is required on every request, and the URL that carries it is
  the only way in.
- The text output of `dash start` and `dash status` carries the token
  when the process can read the token file. The file is 0600, so only
  the owner (and root) read it.
- The JSON document of `dash start` and `dash status` redacts the token
  (`<redacted>`). `--show-token` puts the real value in it.
- The MCP tool `dash_status` redacts the token the same way;
  `show_token: true` reveals it when the file is readable.

The service runs in one of two ways:

- A `systemd --user` unit (`bernie-dash.service`) when the user manager
  answers. The unit is written under `<state>/dashboard/` and linked
  with `systemctl --user link`. The dashboard then survives the logout
  only after the operator runs `loginctl enable-linger $USER` as root.
  bernie prints that command; it never runs it.
- A detached process otherwise: the same binary with the internal flag
  `--child`, in its own session, with the output in `dash.log`.

`dash start` refuses when a dashboard is already running and says which
one (the PID). A pidfile of a process that is gone is stale: the next
start clears it.

`dash stop` sends `SIGTERM`, waits five seconds, then sends `SIGKILL`.
It removes the pidfile. A stop with no dashboard running is not an
error.

`dash status` prints the state (`running`, `stopped` or `stale`), the
listen address, the loopback URL, the LAN URLs of the non-loopback
interfaces, the manager (`process` or `systemd`), the token path and the
log path. The LAN URLs are empty when the listener is bound to one
address; then the output carries an `ssh -L` hint.

Rules for responses and tokens:

- Every response carries a cache header. The HTML and the `/api`
  answers carry `Cache-Control: no-store`; the static assets carry
  `Cache-Control: no-cache` (the browser revalidates with `If-None-Match`,
  the answer is 304); the fonts carry
  `Cache-Control: public, max-age=31536000, immutable`.
- Every static asset carries an `ETag`: the SHA-256 prefix of the file. A
  revalidation with a matching `If-None-Match` answers 304 with no body.
- Every text asset is gzipped when the client sends `Accept-Encoding: gzip`;
  the answer carries `Content-Encoding: gzip` and `Vary: Accept-Encoding`.
- The HTML and the assets carry a strict CSP:
  `default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self';
  connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; object-src 'none';
  form-action 'self'`.
- POST, PUT and DELETE are refused on two conditions:
  `Sec-Fetch-Site: cross-site` (the browser's own word) or an `Origin` header
  that names a host other than the request's. The custom `X-Bernie: 1`
  header check of spec §11 stays.
- In token mode, `/?token=<t>` is the only URL that accepts the token. It
  sets an HttpOnly, SameSite=Lax cookie and redirects (303) to `/` with no
  query.
- A browser that opens a page of the dashboard in token mode without a
  valid token gets a plain HTML page that says what to do: open the address
  that `bernie dash` printed, the one that ends in `/?token=…`. This includes
  a missing token and an invalid query token. The answer is 401, it sets no
  cookie and the invalid token is ignored. The token is never in the page.
  The `/api` paths keep the JSON error shape.
- A `token` query parameter on any `/api/` path is refused with 400
  (`{"error": "...", "exit_code": 2}`) in both modes, even when the token is
  valid. API clients send `Authorization: Bearer <token>`.
- On other paths a query token is ignored. It sets no cookie.
- The server writes no per-request log, so a query string never reaches the
  log. The start line prints the bootstrap URL once.
- Every answer is redacted before it leaves the process. The JSON documents
  go through `internal/support/redact` — `httpjson.Write` is the one writer of
  the API — and the event streams redact every line they send. A secret never
  reaches the browser: the value of every secret name (`*_TOKEN`, `*_KEY`,
  `*_SECRET`, `*_PASSWORD`) in an environment, an argument or a log line, and
  every credential that is secret by shape (`hf_...`, `sk-...`, a bearer
  token). The routes that carry host, run, log or config text are the ones
  where the rule has something to remove: `GET /api/recipes/{id}/plan` (the
  environment and the command of the run), `GET /api/runs`, `POST /api/runs`
  and `DELETE /api/runs/{name}` (the run record), `GET /api/runs/{name}/log`,
  `GET /api/runs/{name}/findings` and `GET /api/runs/{name}/logs` (the log of
  the run), `GET /api/topo/check` (the environment of the recipe of the
  check), `GET /api/doctor` (the checks and the commands they name),
  `GET /api/onboarding`, `GET /api/jobs/{id}`,
  `POST /api/onboarding/{step}/run`, `POST /api/onboarding/path/run` and
  `POST /api/setup/verify` (the
  checklist and the output of a step), `POST /api/smoke` and `POST /api/chat`
  (the answer of the model). The other routes carry only numbers, identifiers
  and the static words of the catalogue — `GET /api/hw`, `GET /api/topo`,
  `GET /api/glossary`, `GET /api/recipes`, `GET /api/gpus/history`,
  `GET /api/telemetry`, `GET /api/power`, `PUT /api/power`, `GET /api/bench`,
  `GET /api/bench/suites`, `GET /api/bench/compare`, `POST /api/bench`,
  `POST /api/bench/run`, `POST /api/bench/ab`, `GET /api/bench/{id}`,
  `GET /api/jobs` and `POST /api/jobs/{id}/stop` — so there is no secret in
  them for the rule to remove. They go through the same rule all the same, so
  a route that is added later cannot forget it.

The page has eight tabs: Setup, Dashboard, Chat, Benchmark, Topology, Fleet,
Logs and Advanced. The theme of the page is the choice of the reader and
lives in the browser, not in the API: the rail holds the three choices in
words, `Auto` (the default: the page follows the system and follows it
live), `Dark` and `Light`; the choice is kept in `localStorage`
(`bernie.theme`) and the palette is put on `<html>` as `data-theme="dark"`
or `data-theme="light"` before the first paint, so the page never shows the
other palette for a moment. The Topology tab is the tab of the PCIe tree, of
the link classes and of the P2P readiness checklist: it reads `/api/topo` and
`/api/topo/check`, its route is `#topology`, and the old route
`#dashboard/topology` is gone (the topology is a tab of its own now, not a
section of the Dashboard tab). The Chat tab sends one prompt to the run that
serves and streams the answer back; it keeps no history. Fleet is planned:
its tab carries one honest sentence and no control. The Advanced tab is the
operator's console: the things about the host that belong to no single tab.
It holds the exact per-GPU table, the power caps with the only write of the
page, the doctor report in full (every category, every check), the jobs of
the host, the run record with the measures of the server and the raw
documents of every endpoint it reads. It reads `/api/hw`, `/api/runs`,
`/api/doctor`, `/api/power`, `/api/jobs` and the shared telemetry stream of
`/api/telemetry`, it adds no endpoint, and the old route `#classic` still
lands on it. The old first-dashboard layout is gone from it: the tiles and
the cards are on the Dashboard tab, the PCIe tree and the P2P checklist are
on the Topology tab, and the known problems are on the Logs tab. The
Dashboard tab keeps its clear operational view and ends with a `Show details`
disclosure: the same per-GPU table, the window of the history, the power
read-out of every card and the raw documents of `/api/telemetry`,
`/api/hw`, `/api/runs` and `/api/gpus/history`. The history of the cards
(`/api/gpus/history`) is read by the Dashboard tab. A number this host does
not measure is shown as a dash with the words "n/a on this host".

| Method | Path | Body / output |
|---|---|---|
| GET | `/api/hw` | `hw` JSON |
| GET | `/api/topo` | `topo` JSON. A pair that was measured carries `gbps`; the document carries `measured_at` and `measured_source` when a measurement was saved (ia.md §5 R32). |
| GET | `/api/topo/check` | `topo --check` JSON (query `recipe`) |
| GET | `/api/doctor` | `doctor` JSON (query `category`, repeatable; `venv`) |
| GET | `/api/glossary` | the words bernie uses that a reader who has never run vLLM may not know: `{"words": [{"term": "HIP", "meaning": "The number a program uses to name a GPU."}]}`. The page lists the words that stand on the tab it is on. |
| GET | `/api/recipes` | `recipe list --all` JSON |
| GET | `/api/recipes/{id}/plan` | `recipe plan` JSON (query `set=K=V`, repeatable; `mode`; `name`). A container plan with an image also carries `compose`: the generated `compose.yml` (ia.md §5 R42). |
| GET | `/api/runs` | `ps` JSON. An exited run has `findings` ([§8.3](#83-run-record)); the Logs tab shows them next to its log. A live run has `phase` ([§5.4](#54-log-phase)); the Logs tab shows the sentence under its state badge. |
| POST | `/api/runs` | `{"recipe": "", "set": ["K=V"], "mode": "", "name": ""}` → run record. A name that is already running answers 409 Conflict. |
| DELETE | `/api/runs/{name}` | stop |
| GET | `/api/runs/{name}/logs?tail=N` | Server-sent events, one line per event (`tail` default 200). Ends with an `end` or `error` event. |
| GET | `/api/runs/{name}/log?tail=2000&level=all&info&warn&error&q=text` | the tail of the log of a run: `{name, state, total_lines, truncated, lines: [{no, level, text}]}`. Every line carries its number in the log and its level. `level` is `all` (the default), `info`, `warn` or `error`; `q` is a case-insensitive substring; `tail` is 2000 lines by default. The levels are the rules of the log doctor: a line with `ERROR`, `Traceback`, `RuntimeError`, `CUDA error` or `Error:` is an error, a line with `WARNING` or `WARN` is a warning, every other line is info. A line that a finding points at is an error. |
| GET | `/api/runs/{name}/findings` | the known failures in the log of a run: `{name, matches: [{id, severity, cause, fix, doc, line, line_no}]}`. It is the scan of `doctor --run`, with the same rules. |
| GET | `/api/telemetry` | Server-sent events, 1 Hz: `{time, slow_age_s, slow_interval_s, gpus: [Sample], runs: [{name, url, state, stats, window, gen_tok_s, prompt_tok_s, error}]}`. The sampler reads the cards of the host in two tiers: the fast fields every second, the slow fields every `slow_interval_s` seconds. `slow_age_s` is the age in seconds of the slow fields of this event. `stats` holds the numbers of the server of the run since it started, `window` what it did in the last minute (below). |
| GET | `/api/gpus/history?window=5m\|15m\|1h` | the samples of the last minutes, one series per card: `{window, interval_s, points, series: [{hip_index, busy_pct, power_w, vram_used, tokens_per_s, prompt_tok_s, temp_c, sclk_mhz, mclk_mhz}]}`. The window is 15 m when the request names none; another window answers 400. The slow fields of a card (`temp_c`, `sclk_mhz`, `mclk_mhz`) are null at the points that hold no slow measurement: the ring never carries a slow value forward, and a gap is a gap. The ring buffer of the telemetry hub keeps 30 minutes at one point every 2 s; the hub samples the host only while a page watches `/api/telemetry`, so the buffer fills while the dashboard is open and the answer is empty (`points: 0`, no series) before the first sample; a window is downsampled so that no series holds more than 240 points: a window that fits the cap keeps every point of the ring (`points` is then the number of points of a series, 5 m: one point every 2 s), and a longer window is split into buckets of `interval_s` seconds; every bucket keeps its lowest and its highest value (MinMax) in the order of the window, so a peak that lasted one sample is never lost (15 m: every 8 s, 1 h: every 15 s). `tokens_per_s` and `prompt_tok_s` are the output and prefill rates of the live run, read from the hub's vLLM metrics sample; the rate is a property of the server, so every card carries the same number, and the values are zero (the fields absent from a sample without a scrape) when no run serves. The answer is cached for 2 s. |
| POST | `/api/smoke` | one chat prompt to a run of this host: `{"prompt": "", "run": "", "model": ""}` → `{url, model, health, reply, ttft_ms, output_tokens, tok_s, ok, check}`. The target is the run that serves, or the run named by `run`; a name that does not serve answers 404. The prompt is empty by default (then the prompt is `What is 2+2? Answer with one number.` and `check` says `ok: contains 4` when the answer has the digit, `answered` otherwise). The target is always a server of this host; a server that does not answer answers 502 with the reason. |
| POST | `/api/chat` | one prompt to the run that serves: `{"prompt": ""}` → a stream of server-sent events. The events are `delta` (`{"text": ""}`) with the text of the answer, `reasoning` (`{"text": ""}`) with the reasoning of the model when it sends one, `done` (`{"ttft_ms": 0, "output_tokens": 0, "tok_s": 0}`) with the numbers of the answer and `error` (`{"error": ""}`). The target is the run that serves: the body names no server, and a target that is not a server of this host answers 400. Nothing serves answers 503, an empty prompt or a prompt of more than 8192 bytes answers 400, an answer that is already running answers 409, and a server that does not answer answers 502 with the reason. The request to the server ends when the page goes away. The stream ends with the `done` event and the terminator of the chunked body, in that order: a reader that reads the body to the end sees the final event and no error. The `Stop` control of the page aborts the request on purpose; that abort is the reader's own doing and is not an error of the answer. |
| GET | `/api/power` | `tune power service status` JSON |
| PUT | `/api/power` | `{"gpu": "all", "watts": 180}` → write the desired cap. `gpu` is `all` or a HIP index. The power service must be installed. The value is clamped to the range the cards share (spec §9.2); the answer names the cards that were clamped in `clamped`. |
| GET | `/api/bench` | `bench list` JSON |
| GET | `/api/bench/suites` | `bench suites` JSON: the built-in suites with their digest and their groups |
| POST | `/api/bench` | bench flags as JSON (`concurrency` is a list or `"1,8"`) → result. The target is always a loopback server of this host. One bench at a time; a second gets 409. |
| POST | `/api/bench/run` | the same body as `POST /api/bench`, but the run happens in the background → `{"id": "b-7f3a"}` (202). The target is always a loopback server of this host. The state of the run is `GET /api/bench/{id}`. One benchmark job runs at a time; a job that starts while another job runs answers 409. |
| POST | `/api/bench/ab` | `{"base": "", "cand": "", "suite": "", "model": "", "num_prompts": 0, "seed": 0, "tag": ""}` → `{"id": "b-7f3a"}` (202). A URL arm must be a server of this host. The state of the run is `GET /api/bench/{id}`. One benchmark job runs at a time; a job that starts while another job runs answers 409. |
| GET | `/api/bench/{id}` | the state of a benchmark run: `{"id", "status", "phase", "event", "done", "total", "result_id"}`. `status` is `running`, `done` or `failed`; `phase` names the cell the run is on; `done` counts the finished cells, `total` is the number of cells of the run and `result_id` is the id of the saved result. `event` is the typed event of the run (below). |
| GET | `/api/bench/{id}/events` | Server-sent events of a benchmark run: one `log` event per progress line, a `progress` event with the job document, then a `done` or `failed` event with the job document. The document carries the typed event of the run, so the page reads the state of the run and not the text of the log. |
| GET | `/api/bench/compare?a=ID&b=ID&format=markdown` | the receipt of the comparison as `text/markdown`: the text of `bernie bench compare A B --markdown`. |
| GET | `/api/onboarding` | the onboarding checklist of this host ([§3.3](#33-the-onboarding-checklist)). The document comes from the cache when it is younger than 15 s; an older one is served with `stale` and refreshed in the background. |
| GET | `/api/setup/runs` | the records of the setup runs of this host, newest first, and the newest one in full ([§6.4](#64-the-run-record)). The answer is `{"runs": [Run], "run": Run}`. |
| GET | `/api/setup/runs/{id}/log/{step}` | the log of one step of a run: `{"run": "", "step": "", "lines": [""], "truncated": false}`. The tail of the log, at most 400 lines. |
| POST | `/api/setup/verify` | start the check of the venv again in the background → the job document. The job runs the `venv.verify` step through the same function the CLI and the MCP server use ([§6.4](#64-the-run-record)), so the record of the run is the one of the other front ends. The body is `{"mode": "", "venv": ""}`. One job runs at a time: a call that finds the slot taken answers 409. |
| POST | `/api/onboarding/{step}/run` | start the action of a step in the background → the job document. A step that needs root answers `{"step": "", "root_required": true, "commands": []}` and runs nothing. |
| POST | `/api/onboarding/path/run` | start the whole path that bernie can do itself in the background → the job document. The body is the same as the step run. It is the one action of the Setup tab. A step that needs root answers `{"step": "path", "root_required": true, "commands": []}` and runs nothing. |
| POST | `/api/jobs/{id}/stop` | stop the job with this id → `{"id": "", "stopped": true}`. `stopped` is false when the job runs in another process and cannot be stopped from here. |
| GET | `/api/jobs` | the jobs of this host, newest first: `[{id, step, state, started_at, finished_at, phase, progress}]`. The list holds at most 50 jobs; `limit` takes 1 to 200. |
| GET | `/api/jobs/{id}` | the job document: `{"id", "step", "state", "phase", "progress", "error", "findings", "lines", "started_at", "finished_at"}`. `progress` holds the numbers of the work when the step counts them ([§3.3](#33-the-onboarding-checklist)). |
| GET | `/api/jobs/{id}/events` | Server-sent events of a job: one `line` event per output line, then a `done` or `failed` event with the job document. The stream replays the buffered lines from the beginning; every line event carries the number of the line as its id, and `Last-Event-ID` (or `last_event_id`) asks for the lines after it. |
| POST | `/api/refresh` | drop every cached answer of the host (`hw`, `topo`, `doctor`, `onboarding`, `power`, `runs`). The next reader of one of those endpoints re-probes the host. The Re-check buttons of the Setup, Topology and Advanced tabs call it before they reload, so a cable that changed or a check that just resolved shows up without a page reload (ia.md R27). |
| GET | `/api/models` | the `bernie models` document: the ids `/v1/models` answers when a run is serving, the repos the Hugging Face cache on this host holds with their snapshot commit and the hub cache root. A failure to read the live server is in `live_error` and does not stop the cached part of the report (ia.md R31). |
| GET | `/api/prefs` | the `bernie config show` document: the ordered list of keys with their value, the source (`config`, `$BERNIE_VENV`, `$VIRTUAL_ENV`, `$HF_HOME`, `default`, `conventional`, `ambiguous`, `none`), the resolved value when one applies, the choices and the hint. Secrets (the fleet token) are stripped before the document leaves the file (ia.md R28). |
| PUT | `/api/prefs` | write one `{key, value}` pair: the write goes through the same `Config.SetKey` the CLI uses, so the same validation runs. An unknown key answers 400. The new document is returned. The `X-Bernie: 1` guard of the auth middleware applies. |
| GET | `/api/report` | the redacted share report of this host: a markdown document an operator can paste into an issue, a chat or a ticket. The same document is the answer of `bernie doctor --report` and of the MCP tool `report`. No hostname, no username, no token, no path with the name of the user (ia.md R24). |
| GET | `/api/fleet` | the fleet of this box: `{"path", "hosts": [{"name", "url", "me", "reachable", "error", "state", "recipe", "gen_tok_s", "gpus", "vram_total_gb"}]}`. This box comes first, then every configured peer. The token of a peer stays in the config file and is never in the answer. The read of a peer is SSRF-safe: only the configured hosts are reached, only the fixed paths of the allowlist, with a three second timeout, a 256 KiB body cap and no redirect followed. A peer that does not answer keeps its row and says why: the gap stays a gap. The answer is held for five seconds (ia.md R23). |
| POST | `/api/fleet` | add one peer to the configured fleet: `{"name", "url", "token"}`. The token is stored in the config file (0600 when a token is set) and is never echoed in the response. A bad URL answers 400; a name that already exists answers 409. The `X-Bernie: 1` guard of the auth middleware applies. |
| DELETE | `/api/fleet/{name}` | forget one peer: the token goes with it. The answer is `{"removed": name}`. A name that is not in the list answers 404. |
| POST | `/mcp` | MCP over Streamable HTTP, with its own guard ([§13.10](#1310-http-guard)). Loopback addresses only. |

Two endpoints of the table have no consumer on the page: `POST /api/bench`
(the synchronous form; the Benchmark tab starts a run in the background with
`POST /api/bench/run`) and `GET /api/jobs` (the page reads the job of a step
from the job document of that step). The `level` and `q` parameters of `GET
/api/runs/{name}/log` are answered by the API, but the Logs tab filters the
lines in the browser.

A benchmark job is a benchmark that runs in the background. The job has an
id, a state (`running`, `done` or `failed`), a phase, the counters of the
work and the typed event of the run.

The typed event says where the run is. Its `kind` is one of:

| Kind | Meaning | Fields |
|---|---|---|
| `visit` | a visit of an A/B run starts | `index`, `total`, `arm` |
| `group` | a group of a suite run starts | `index`, `total`, `group` |
| `cell` | a cell of the run finished | `index`, `total`, `level` |

`index` is the number of the visit, of the group or of the cell, starting at
one. `total` is the number of visits, of groups or of cells of the run.
`arm` is the arm of a visit (`base` or `cand`). `group` is the name of a
group of a suite run. `level` is the concurrency of a cell.

The typed event is in the job document of `GET /api/bench/{id}` and in the
`progress` event of `GET /api/bench/{id}/events`. The page reads the event
and never the text of the log to know where a run is.

One benchmark runs at a time. Every entry that starts a benchmark answers
the same way when a benchmark runs already: `POST /api/bench`, `POST
/api/bench/run` and `POST /api/bench/ab` answer 409, the command `bernie
bench` prints `a benchmark is already running` and exits with a non-zero
code, and the tools `bench_run` and `bench_ab` of the MCP server return the
same sentence as an error.

The store of the jobs keeps at most 32 finished jobs. The oldest finished
job is dropped when a new one finishes.

The `Sample` of the telemetry stream is one GPU:

```json
{"hip_index": 0, "busy_pct": 0, "temp_c": 33, "power_w": 12.5, "power_cap_w": 175,
 "vram_used": 24117248, "vram_total": 32195477504,
 "temp_junction_c": 35, "temp_mem_c": 34, "sclk_mhz": 2570, "mclk_mhz": 1000}
```

`temp_junction_c`, `temp_mem_c`, `sclk_mhz` and `mclk_mhz` are absent when
the card does not publish them. The dashboard shows `–` for an absent value.

The fields of a `Sample` are read in two tiers, because every read of an
amdgpu attribute waits for its card:

| Tier | Fields | Read every |
|---|---|---|
| fast | `busy_pct`, `power_w`, `vram_used` | one second |
| slow | `temp_c`, `temp_junction_c`, `temp_mem_c`, `power_cap_w`, `vram_total`, `sclk_mhz`, `mclk_mhz` | `slow_interval_s` seconds (four) |

A sample holds the last value of the slow fields and `slow_age_s` says how
old it is. The page shows the age of the slow fields when it passes twice
their own interval; below that the age is not worth a word on the page.

The `stats` of a run are the numbers of its server since it started. The
`window` of a run is what that server did in the last minute: it is
computed from two readings of the server, the counters it advanced and the
observations it recorded in between. A server that restarted empties the
window, because nothing can be measured across a restart.

| Field of `stats` | Unit | Meaning |
|---|---|---|
| `requests_running` | requests | the requests the server answers now |
| `requests_waiting` | requests | the requests that wait for a free slot |
| `kv_cache_usage` | fraction (0..1) | the share of the KV cache in use |
| `prompt_tokens_total` | tokens | the prompt tokens the server read |
| `generation_tokens_total` | tokens | the tokens the server wrote |
| `prefix_cache_queries_total` | tokens | the tokens the server looked up in its prefix cache |
| `prefix_cache_hits_total` | tokens | the tokens it found there |
| `prefix_cache_hit_rate` | fraction (0..1) | hits / queries since the server started |
| `prefix_cache` | true/false | the server publishes the prefix cache counters |
| `external_prefix_cache_queries_total` | tokens | the tokens it looked up in the cache of another instance |
| `external_prefix_cache_hits_total` | tokens | the tokens it found there |
| `external_cache` | true/false | the server publishes the counters of the cache of another instance |
| `prompt_tokens_cached_total` | tokens | the prompt tokens the server took from a cache |
| `prompt_tokens_by_source` | tokens | the prompt tokens per source (`local_compute`, `local_cache_hit`, `external_kv_transfer`) |
| `num_preemptions_total` | requests | the requests the engine took back and ran again |
| `waiting_by_reason` | requests | the waiting requests per reason (`capacity`, `deferred`) |
| `finished_by_reason` | requests | the finished requests per reason (`stop`, `length`, `abort`, `error`, `repetition`) |
| `spec_drafts_total`, `spec_draft_tokens_total`, `spec_accepted_tokens_total` | tokens | the counters of the speculative decoding |
| `spec_accept_rate` | fraction (0..1) | the accepted draft tokens / the draft tokens |
| `speculation` | true/false | the server publishes the counters of the speculative decoding |
| `spec_mean_accept_len` | tokens | the mean tokens written per draft step |
| `ttft`, `itl`, `e2e`, `queue` | seconds | the latencies the server measured: `ttft` is the time to the first token, `itl` the time between two tokens, `e2e` the time of the whole request, `queue` the wait in the queue |

A latency holds `p50`, `p95`, `mean` and `count`, the number of
observations behind the numbers. A percentile is the upper bound of the
bucket the observation lies in, not a precise value.

A measure the server does not publish is absent. An old release names the
prefix cache counters `gpu_prefix_cache_queries_total` and
`gpu_prefix_cache_hits_total` and the KV cache gauge
`gpu_cache_usage_perc`; bernie reads both names. A release without the
cross-instance cache, without the prompt tokens per source and without the
queue latency publishes no such measure: the page shows a dash and says
why.

The `window` of a run holds the same measures over the last minute:
`seconds` (the length of the window), `prefix_cache_queries`,
`prefix_cache_hits`, `prefix_cache_hit_rate` (absent when the window holds
no query), `prompt_tokens`, `prompt_tokens_cached`, `generation_tokens`,
`finished` and the four latencies. The first reading of a server has no
window.

Every card of the page carries a `json` link in its header. It opens the
raw JSON of the endpoint behind the card in a new tab, so the document a
person reads and the document an agent gets are the same one. Every
command the page shows has a copy button, and the button copies the
command exactly as it is shown.

The Setup tab is the first tab of the page. It renders the onboarding
document of [§3.3](#33-the-onboarding-checklist) and holds no state of its
own: what the server reports is what the page shows, and a re-check asks
the server again. A step that bernie can do is started with `POST
/api/onboarding/{step}/run`; its output arrives on the event stream of the
job (`GET /api/jobs/{id}/events`) and the page shows it under the step. A
A step that needs root shows its commands and a copy button, and the page
never runs them. The venv detail of the tab (the components of the
`venv.verify` step) holds a `Verify again` button: it starts the check of
the venv in the background (`POST /api/setup/verify`), it is disabled while
the check runs, and the page paints the components of the new record when
the job ends. The Advanced disclosure of the Setup tab holds the raw
document of `GET /api/onboarding` and the commands that produce it
(`bernie status --steps`, `bernie status --json`).

## 12. Files and paths

| Name | Default | Override |
|---|---|---|
| config | `~/.config/bernie` | `$BERNIE_CONFIG_DIR`, `$XDG_CONFIG_HOME` |
| state | `~/.local/state/bernie` | `$BERNIE_STATE_DIR`, `$XDG_STATE_HOME` |
| data | `~/.local/share/bernie` | `$BERNIE_DATA_DIR`, `$XDG_DATA_HOME` |
| cache | `~/.cache/bernie` | `$BERNIE_CACHE_DIR`, `$XDG_CACHE_HOME` |

Config file `<config>/config.yaml`:

```yaml
venv: /home/me/Apps/vllm/venv-7.14.0   # host-mode default
mode: host                              # host | container
engine: podman                          # docker | podman (auto when empty)
device_access: auto                     # classic | cdi | auto (auto when empty)
hf_home: /data/hf                       # Hugging Face cache root
set: ["env.GPU_MAX_HW_QUEUES=2"]        # default overrides, applied before --set
```

The desired power state is `<config>/power.yaml` ([§9.2](#92-power)). The
generated systemd units are `<state>/power/bernie-power.service` and
`<state>/power/bernie-power.path`.

The cached facts live in `<state>/facts`. A fact has a key, a fingerprint of
the host state it was read from and a time to live. A fact older than its
time to live is still served and is refreshed in the background: a reader
never waits for a probe it does not need. A fingerprint that does not match
the host state anymore makes the fact a miss, so a rebuilt venv or a new
ROCm tree is probed again. The fingerprint of a venv probe follows the GPU
restriction of the environment: the probe counts the GPUs that torch sees,
and that answer changes when the operator limits them.

| Key | Fact | Time to live |
|---|---|---|
| `venv/probe/<venv>` | the torch and vLLM imports of a venv | 10 min |
| `rocm/installs/<root>` | the ROCm trees under a root | 10 min |
| `onboarding/document/<options>` | the onboarding checklist of this host | 15 s |

The setup runs live in `<state>/setup`: one directory per run, with the
record of the run (`run.json`) and the log of every step. The file
`latest` points at the newest run ([\u00a76.4](#64-the-run-record)).

The `venv` key is the third step of the venv order of
[§6.2](#62-the-host-venv). It names the venv of every host-mode command
that does not get a `--venv` of its own.

## 13. Model Context Protocol

### 13.1 Purpose

MCP is the third front end, next to the CLI and the dashboard. It exposes
bernie to agents.

MCP does not replace the CLI or the dashboard API. It adds no feature of its
own: every tool calls the code of a CLI command, and returns the same JSON
document as that command with `--json`.

The server uses the official Go SDK. It speaks protocol revision
`2026-07-28`, and legacy clients that start with `initialize`. It does not
implement the deprecated HTTP+SSE transport.

### 13.2 Command

`bernie mcp [--allow-write]` serves one client on stdin and stdout.

The server writes only MCP messages to stdout. It writes diagnostics to
stderr. The command has no `--json` output: the protocol is structured.

`--allow-write`, or `BERNIE_MCP_WRITE=1` in the environment, enables the
mutating tools ([§13.5](#135-write-gate)).

### 13.3 Transports

| Transport | Where | Notes |
|---|---|---|
| stdio | `bernie mcp` | The client starts the process. No listening port. |
| Streamable HTTP | `POST /mcp` on `bernie dash` | Stateless: one request per POST. Rules in [§13.10](#1310-http-guard). |

### 13.4 Tools

The catalog is fixed. Names, descriptions and schemas never depend on logs,
recipes or other host data. Every tool has a closed input JSON Schema
(`additionalProperties: false`, enums, patterns, bounds) and an output JSON
Schema derived from the Go type of its CLI document.

Read-only tools. Each returns the document of its command as
`structuredContent`, and the same JSON as text. A command whose document is
an array (`ps`, `bench list`, `tune power status`) returns that array.
Secrets are redacted ([§13.7](#137-untrusted-content)).

| Tool | Same document as | Input |
|---|---|---|
| `status` | `status` | `venv` |
| `hw` | `hw` | none |
| `topo` | `topo` | `bandwidth`, `venv` |
| `doctor` | `doctor` | `categories`, `venv` |
| `doctor_log` | `doctor --run` | `run` (a run name, never a path) |
| `smoke` | `smoke` | `url` (loopback only), `model` |
| `models` | `models` | none |
| `prefs_get` | `config show` | none |
| `report` | `doctor --report` | none |
| `fleet` | `fleet ls` | none |
| `recipe_list` | `recipe list` | `all` |
| `recipe_show` | `recipe show` | `id` |
| `recipe_plan` | `recipe plan` | the launch flags: `id`, `set`, `mode`, `name`, `venv`, `engine`, `device_access`, `profile`, `no_tunableop`, `tuning` |
| `ps` | `ps` | none |
| `logs` | none (text) | `name`, `tail` (1 to 10000, default 100) |
| `bench_list` | `bench list` | none |
| `bench_show` | `bench show` | `id` |
| `bench_suites` | `bench suites` | none |
| `bench_compare` | `bench compare` | `a`, `b` (two saved result ids) |
| `tunableop_status` | `tune tunableop status` | `venv` |
| `power_status` | `tune power service status` | none |
| `onboarding_status` | `status --steps` (the `onboarding` field of `status --json`) | `venv` |
| `setup_plan` | `setup plan` | `mode`, `venv`, `image`, `model`, `rocm` |
| `setup_report` | `setup report` | `run` (the id of the run; the newest one when it is empty) |
| `dash_status` | `dash status` | `show_token` (the token is redacted without it) |
| `jobs` | none (the onboarding jobs of this host) | none |

Mutating tools ([§13.5](#135-write-gate)):

| Tool | Command | Result |
|---|---|---|
| `serve` | `serve` (no `--foreground`, `--dry-run`) | `runner.Record`, as `serve --json` |
| `stop` | `stop NAME` (the name is required) | `{"name", "stopped"}` |
| `bench_run` | `bench` (loopback `url` only, `recall`, `suite` optional) | the saved `bench.Result` |
| `bench_ab` | `bench ab` (loopback `base` / `cand` URLs only) | the `suite.ABResult` |
| `setup_apply` | `setup apply` | the setup steps after the run and the record of the run (`run_id`) |
| `setup_verify` | `setup verify` | the record of the run (`run_id`) with the evidence and the components of the check of the venv |
| `tune_power_set` | `tune power set` | `{"changed", "file", "watts", "gpu_indexes", "selectors", "clamped"}` |
| `fleet_add` | `fleet add` | `{"name", "url"}` (the token is never in the result) |
| `fleet_rm` | `fleet rm` | `{"removed": name}` |
| `dash_start` | `dash start` | the state of the dashboard service ([§11.1](#111-the-dashboard-service)) |
| `dash_stop` | `dash stop` | `{"stopped": true}` |

`setup_verify` is a mutating tool: it starts the Python of the venv, which
imports torch and vLLM and touches the GPU, and it writes the record of the
run under the state directory. A tool that only writes a state file (the
desired power cap) is mutating for the same reason.

`up` and `config set` are not tools. An agent runs them through the CLI.

A tool that runs one onboarding step is deferred. The CLI has no command
that runs one checklist step, so such a tool would exist only in MCP and
would break the one-surface rule. An agent uses `setup_apply` and `serve`,
the CLI actions behind the runnable steps.

`logs` returns a text document, not a CLI document, because `bernie logs`
is a text stream:
`{"source": "run-log", "untrusted": true, "run": "", "truncated": false, "content": ""}`.
`content` holds at most 256 KiB, cut at a line start.

`recipe_plan` and `topo` keep the side effects of their commands: `recipe
plan` may write TunableOp rows under the data directory. `topo` with
`bandwidth` loads the GPUs.

### 13.5 Write gate

MCP is read-only by default.

With the write gate off, the server does not list the mutating tools and
rejects a call to one as an unknown tool. `bernie mcp --allow-write` opens
the gate for that process. `BERNIE_MCP_WRITE=1` opens it for a process that
reads the variable at start (`bernie mcp` and `bernie dash`).

A mutating tool runs in two calls:

1. The first call has no `confirm_token`. The server computes the plan and
   changes nothing: no run starts and no state is written. The plan has the
   side effects of the command it plans (`recipe plan` may write TunableOp
   rows). `serve` computes its plan on both calls. It returns
   `{"executed": false, "plan": {...}, "confirm_token": "...", "expires_at": "..."}`.
2. The second call repeats the same arguments and adds the token. The server
   verifies the token, runs the action and returns
   `{"executed": true, "result": {...}}`, with the CLI document in `result`.
   The token is verified before the plan is computed, so a call the gate
   refuses never has the side effects of a plan.

The `plan` is the document the operator confirms: the resolved plan for
`serve`, `{name, recipe, state, mode, pid, container, url}` for `stop`, the
normalized options for `bench_run`, `{run, manual}` for `setup_apply` (the
non-root steps bernie runs, and the root steps it never runs) and
`{file, watts, gpu_indexes, desired}` for `tune_power_set`, and
`{name, url}` for `fleet_add` and `{name}` for `fleet_rm`.
`setup_apply` with nothing to run returns `executed: false`, a `note` and no
token.

A token:

- has 256 random bits;
- is bound to the tool, the normalized arguments, the digest of the plan and
  the host name;
- expires after five minutes;
- works once: the server consumes a token whatever the verdict, so a token
  with changed arguments or a changed plan is dead and needs a new plan call;
- is stored as a file named by its SHA-256 under `<state>/mcp/tokens/` (mode
  `0600`, directory `0700`). The raw token is never stored.

The server runs one host-changing operation at a time (`serve`, `stop`,
`setup_apply`, `setup_verify`, `tune_power_set`, `fleet_add`, `fleet_rm`) and
one benchmark at a time. A call that
finds the slot taken fails with a busy error. The benchmark tools fail with
`a benchmark is already running`, the sentence of every front end that
starts a benchmark ([§11](#11-dashboard)).

### 13.6 Host safety

MCP never runs `sudo` and never escalates. A step that needs root comes back
as a command in the result (`setup_apply`). `tune_power_set` writes the
desired power file. It never writes hwmon. The power service applies that
file as root.

A plan call changes nothing on the host, except the TunableOp rows that
`recipe plan` writes to the data directory. A call that the gate refuses — a
token that is unknown, expired, or bound to another tool or to other
arguments — computes no plan at all and changes nothing.

A request cancelled by the client stops `setup_apply` and `bench_run`.
`serve` and `stop` finish once they started, like the dashboard.

### 13.7 Untrusted content

Run logs and model output are untrusted data.

The `logs` result carries `source` and `untrusted: true`. No tool description,
schema or catalog entry is built from log text.

The server redacts every result and every progress message before it leaves
the process. It uses the rules of the dashboard: values of secret names
(`*_TOKEN`, `*_KEY`, `*_SECRET`, `*_PASSWORD`) in environments, arguments and
log lines, and credentials by shape (`hf_...`, `sk-...`, bearer tokens). A
credential that is secret by shape is removed from every string of a result,
so a log line that carries one loses it. `doctor_log` is the preferred way to
diagnose a run: it returns catalog matches, not raw log text.

### 13.8 Progress

When the client sends a progress token, `serve`, `setup_apply` and
`bench_run` send each line of their progress output (the lines the CLI writes
to stderr) as `notifications/progress`. The progress value rises with each
message.

### 13.9 Audit

The server appends one JSON line per tool call to `<state>/mcp/audit.jsonl`
(mode `0600`): time, transport, client name, tool, action digest, redacted
argument summary (at most 512 bytes), token id (hash prefix), result (`ok`,
`planned`, `applied`, `rejected`, `failed`), duration and error class.

The file never holds a bearer token, a confirm token, an environment map or a
log body.

### 13.10 HTTP guard

`bernie dash` serves `/mcp` only when it listens on a loopback address.
Otherwise `/mcp` answers 404.

The guard of `/mcp` differs from the guard of `/api` ([§11](#11-dashboard)):

- `/mcp` does not use the dashboard cookie, the `X-Bernie` rule for every
  `POST`, or the `/` token bootstrap.
- The `Host` header must name a loopback host.
- A request with an `Origin` header needs a loopback origin.
- When the dashboard has a token, the request needs
  `Authorization: Bearer <token>`. The cookie does not authenticate `/mcp`.
  A `token` query parameter is refused with 400.
- The request body is a JSON-RPC message. A `tools/call` of a mutating tool
  also needs the header `X-Bernie: 1`. A read-only call does not.
- The mutating tools are listed only when the dashboard has a token and
  `BERNIE_MCP_WRITE=1` is set at start. Without a token the gate stays closed
  and the dashboard logs it.
- Only `POST` is served. A response is not cacheable: `Cache-Control:
  no-store` on a refusal, the SDK's `no-cache, no-transform` on a protocol
  response. The endpoint sets no CORS header.

## 14. Out of scope (draft 1)

- Model quantization and checkpoint conversion.
- Engine changes (kernels, MoE offload). They belong to the fork.
- Multi-host fleet control. The dashboard shows one host.
