# bernie architecture

This document shows how the code is organized and how data moves through it.
The behavior contract is [spec.md](spec.md). If the two disagree, spec.md is
correct and this file is a bug.

Related pages:

- [packages.md](packages.md): the API of each package.
- [extending.md](extending.md): where to change what, and how to add a GPU
  generation.
- [recipes.md](recipes.md): how to write a recipe.
- [design-dashboard.md](design-dashboard.md): the design of the dashboard
  v3. The shell, the tokens, the four tabs and the wireframes live there and
  in [design/](design/). It is the contract for the front end; this file
  stays the contract for the packages.

## 1. Layers

Packages sit in layers. An arrow means "imports". Nothing imports upward.
A solid arrow is a main dependency. A dotted arrow means "also imports
several packages of that group directly".

```mermaid
flowchart TB
  subgraph front["Front ends"]
    CLI["internal/cli<br/>+ cli/cmds/family"]
    DASH["dashboard<br/>server + internal/api,<br/>auth, telemetry, sse"]
    MCP["internal/mcp<br/>MCP tools"]
  end
  APP["internal/app<br/>shared operations"]
  FLOW["internal/flow<br/>status, up"]
  subgraph top["Domain, top level"]
    DOC["doctor"]
    SET["setup"]
    LOG["logdoc"]
    ONB["onboarding<br/>steps, jobs"]
  end
  subgraph serving["internal/serving"]
    RUN["runner"]
    REC["recipe"]
    TUN["tuning"]
    BEN["bench"]
    VLLM["vllm client"]
  end
  subgraph machine["internal/machine"]
    TOPO["topo"]
    GPU["gpu"]
    HOST["host"]
    ROCM["rocm"]
  end
  SUP["internal/support<br/>paths, execx, config, redact,<br/>pyenv, factcache"]
  D1[("gpu/devices/*.yaml")]
  D2[("recipes/")]
  D3[("tunableop/<br/>tuned-moe/")]
  D4[("logdoc/*.yaml")]
  D5[("dashboard/web/")]
  CLI --> APP
  DASH --> APP
  MCP --> APP
  APP --> ONB
  FLOW --> ONB
  MCP --> FLOW
  DASH --> MCP
  APP -.-> top
  APP -.-> serving
  APP -.-> machine
  CLI --> FLOW
  CLI --> DASH
  CLI --> DOC
  DASH --> FLOW
  CLI --> LOG
  DASH --> DOC
  FLOW --> DOC
  FLOW --> SET
  FLOW --> RUN
  FLOW --> LOG
  ONB --> DOC
  ONB --> SET
  ONB --> REC
  ONB --> RUN
  RUN --> REC
  RUN --> TUN
  REC --> TOPO
  REC --> HOST
  DOC --> TUN
  TOPO --> GPU
  TOPO --> ROCM
  BEN --> VLLM
  CLI -.-> serving
  DASH -.-> serving
  MCP -.-> serving
  CLI -.-> machine
  DASH -.-> machine
  DOC -.-> machine
  SET -.-> machine
  serving -.-> machine
  machine --> SUP
  serving --> SUP
  GPU --> D1
  REC --> D2
  TUN --> D3
  LOG --> D4
  DASH --> D5
```

Two facts matter:

- `gpu` imports no other bernie package. It is the root of the graph.
- `dashboard` never imports `cli`. `cli` imports `dashboard` in three places:
  `dash`, `up` and `serve --dash`.
- `mcp` never imports `cli` or `dashboard`. `dashboard` imports `mcp` to
  mount `/mcp`; `cli` imports `mcp` for `bernie mcp`.

Rules:

1. Front ends (CLI, dashboard, MCP) parse input and format output. They hold
   no domain logic.
2. Domain packages return plain structs with JSON tags. They never print.
   They take a `context.Context` for anything that can block.
3. Domain packages read the host through an injectable root (a root path or
   an `fs.FS`), so tests run on fixture trees.
4. Arrows are the only allowed imports. `gpu` never imports `recipe`.
   `dashboard` never imports `cli`. `doctor` never imports `recipe`: the
   caller passes the validated architectures in `doctor.Options`.
5. The JSON field names of domain structs are the public API (spec.md).
   Rename a field only together with spec.md.
6. `flow` holds the two product-level chains (`status` and `up`).
   `app.Plan` is the front-end entry to `runner.Prepare`, the only place
   that turns a recipe id into a ready plan. `app.Serve` prepares and starts
   a run, including the active-name check.
7. `app` holds the operations that more than one front end runs and that no
   domain package owns: `hw`, `topo`, `doctor`, the run-log scan, `recipe
   list`, `tune` status and power, `setup` plan and apply, `serve` and
   `plan`, `bench` (run, save and compare), the onboarding checklist and its
   jobs. Each returns the struct of its `--json` document. `app` never
   prints and never imports a front end.
8. `onboarding` holds the checklist of spec §3.3: the facts of the host
   (one probe), the steps derived from them and the jobs that run a step.
   `flow`, `app` and the front ends read the same document, so the page and
   the command cannot disagree.

## 2. Layout and layers

### 2.1 The layers

Packages sit in five layers. A package may import a package of its own
layer or of a lower one, never of a higher one. `make check` fails on a
violation (`scripts/check-layers.sh`).

| Layer | Packages | What they hold |
|---|---|---|
| 0 support | `internal/support/*` | the small helpers every layer may use |
| 1 machine | `internal/machine/*` | what the host is |
| 2 serving | `internal/serving/*`, `internal/doctor`, `internal/setup`, `internal/logdoc` | what bernie runs, and what the host must have for it |
| 3 chains | `internal/app`, `internal/flow`, `internal/onboarding` | the operations the front ends share, and the chains that use them |
| 4 front ends | `internal/cli/*`, `internal/mcp`, `dashboard` | the input and the output of the product |

The data packages (`recipes`, `tunableop`, `tuned-moe`, `bench`) have no
layer: they hold data and import nothing of bernie.

`logdoc` sits in layer 2 with the serving packages: it reads the log of the
server bernie runs, so the runner (layer 2) uses it. `doctor` and `setup`
sit there too: they describe what the host needs to serve, and they use the
tuning data.

### 2.2 The directories

| Directory | Role | Layer |
|---|---|---|
| `cmd/bernie/` | the entry point: blank-imports the command families, calls `cli.Main` | 4 |
| `internal/cli/` | the command framework: `--json`, the exit-code contract, the version | 4 |
| `internal/cli/cmds/<family>/` | one family per area, one `cmd_<name>.go` per command | 4 |
| `internal/app/` | the operations more than one front end runs, one `--json` document each | 3 |
| `internal/flow/` | the `status` snapshot and the `up` chain | 3 |
| `internal/onboarding/` | the onboarding checklist: the facts, the steps, the jobs | 3 |
| `internal/bench/job/` | the benchmark jobs: the store, the state machine, the typed progress events | 3 |
| `internal/doctor/` | the check registry; one file per category of checks | 2 |
| `internal/setup/` | the install plan and the non-root apply | 2 |
| `internal/logdoc/` | the known-failure catalogue, the phase catalogue, the log scanner | 2 |
| `internal/machine/gpu/` | the live GPUs: KFD/DRM/PCI enumeration, the telemetry, the ECC state | 1 |
| `internal/machine/gpu/devices/` | the device tables: one YAML per generation, and the loader that reads them | 1 |
| `internal/machine/host/` | the OS, the package manager, the CPU, the RAM, the groups, the engines | 1 |
| `internal/machine/rocm/` | the ROCm installs, the tools, the libraries, the venv probe | 1 |
| `internal/machine/topo/` | the PCIe tree, the link classes, the P2P matrix, the bandwidth | 1 |
| `internal/serving/recipe/` | the schema, the load, the merge, the validation, the fit, the plan | 2 |
| `internal/serving/runner/` | the host and container backends, the run records, the logs | 2 |
| `internal/serving/tuning/` | the TunableOp profiles, the tuned-MoE folders, the power cap | 2 |
| `internal/serving/vllm/` | the client: `/health`, `/v1/models`, `/tokenize`, `/metrics` | 2 |
| `internal/serving/bench/` | the streaming benchmark and the result store | 2 |
| `internal/serving/bench/suite/` | the bench suites: the cells, the comparison, the verdict | 2 |
| `internal/support/*` | the shared helpers: paths, execx, config, redact, pyenv, factcache, fixture | 0 |
| `dashboard/` | the server, the API, the auth, the SSE, the telemetry, the JSON helpers and the embedded web assets | 4 |
| `recipes/` | the built-in recipes by family, the fragments by concern | data |
| `tunableop/`, `tuned-moe/` | the tuning rows and the fused-MoE configs, by architecture | data |
| `bench/suites/` | the bench suites, as YAML | data |
| `deploy/`, `scripts/`, `docs/` | the packaging, the installer and the checks, the documentation | - |

### 2.3 The crowded directories

The audit looked at every directory with more than ten entries. Only one
had two roles, and it was split. The others keep one role each, so they
stay one package:

| Directory | Role | Why it stays |
|---|---|---|
| `internal/app/` | the operations the front ends share | one file per operation (`doctor.go`, `power.go`, `setup.go`, ...) |
| `internal/mcp/` | the MCP front end | the catalog is already split by file (`tools_read.go`, `tools_write.go`) |
| `internal/serving/recipe/` | a recipe and the plan it produces | one concept: the schema, the merge and the plan |
| `internal/serving/runner/` | the run | the records, the logs and the backends of one run |
| `internal/doctor/` | the host checks | one file per category of checks |
| `internal/serving/bench/` | the benchmark | the run, the metrics and the result store of one benchmark |

### 2.4 Where a feature lives

| Feature | Package |
|---|---|
| `hw`, the GPU inventory | `internal/machine/gpu`, `internal/app` |
| `topo`, the PCIe tree and the P2P matrix | `internal/machine/topo` |
| `doctor`, the host checks | `internal/doctor` |
| `setup`, the install plan | `internal/setup` |
| `recipe`, the recipes and their plan | `internal/serving/recipe` |
| `serve`, `ps`, `stop`, `logs` | `internal/serving/runner` |
| `tune`, the tuning rows and the power cap | `internal/serving/tuning` |
| `bench` | `internal/serving/bench` |
| `bench --suite`, the bench suites | `internal/serving/bench/suite`, `bench/suites` |
| the benchmark jobs and their progress events | `internal/bench/job` |
| the onboarding checklist | `internal/onboarding` |
| `up`, the whole path | `internal/flow` |
| the operations the front ends share | `internal/app` |
| the CLI | `internal/cli`, `internal/cli/cmds/<family>` |
| the dashboard | `dashboard` |
| the MCP tools | `internal/mcp` |
| the log knowledge (the failure and phase catalogues) | `internal/logdoc` |

### 2.5 The tree

```text
bernie/
├── cmd/bernie/           main.go: blank-imports each cmds family, calls cli.Main
├── internal/
│   ├── cli/              command framework, --json and exit-code contract, version
│   │   └── cmds/         one family package per area, one cmd_<name>.go per command
│   │       ├── health/       doctor, setup, status, up
│   │       ├── inventory/    hw, topo
│   │       ├── serving/      recipe, serve, ps, stop, logs, compose, config
│   │       ├── tunecmd/      tune
│   │       ├── benchcmd/     bench
│   │       ├── dashcmd/      dash
│   │       └── mcpcmd/       mcp
│   ├── app/              operations shared by the front ends, one `--json` document each
│   ├── mcp/              MCP server: tools, confirm tokens, audit (official Go SDK)
│   ├── flow/             status snapshot and `up`
│   ├── onboarding/       the onboarding checklist: facts, steps, jobs
│   ├── bench/            the benchmark jobs: the store, the state machine, the events
│   ├── doctor/           check registry; related checks and helpers grouped by role
│   ├── setup/            install plan and non-root apply
│   ├── logdoc/           known-failure catalog, phase catalogue (embedded YAML) and log scanner
│   ├── machine/          what the host is
│   │   ├── gpu/              KFD/DRM/PCI enumeration, telemetry, ECC
│   │   │   └── devices/     the device tables (one YAML per generation)
│   │   │                   and their loader
│   │   ├── host/             OS, package manager, CPU, RAM, groups, container engines
│   │   ├── rocm/             ROCm installs, tools, libraries, venv probe
│   │   └── topo/             PCIe tree, link classes, P2P matrix, bandwidth
│   ├── serving/          what bernie runs
│   │   ├── recipe/           schema, load, extends/merge, validate, fit, plan
│   │   ├── runner/           host and container backends, run records, logs
│   │   ├── tuning/           TunableOp profiles and rows, tuned-MoE folders, power cap
│   │   ├── vllm/             client: /health, /v1/models, /tokenize, /metrics
│   │   └── bench/            streaming benchmark and result store
│   └── support/          small shared helpers
│       ├── paths/            XDG config, state, data, cache
│       ├── execx/            bounded exec of external tools
│       ├── config/           <config>/config.yaml
│       ├── redact/           secret redaction for documents and log text
│       ├── pyenv/            the layout of a Python venv, read without an interpreter
│       ├── factcache/        slow, stable host facts under <state>/facts
│       └── fixture/          test host snapshots (testdata/*.tar.gz)
├── dashboard/            server.go + internal/{api,auth,telemetry,sse},
│                        embedded web/{css,js,components,views} (no build step)
├── bench/suites/         the bench suites, as YAML
├── recipes/              built-in recipes by family, fragments by concern, embed.go
├── tunableop/            <arch>/<profile>/ rows and profile.json, embed.go
├── tuned-moe/            <arch>/ fused-MoE configs, embed.go
├── deploy/               packaging (nfpm), docker, compose
├── scripts/              install.sh, capture-fixture.sh, check-layers.sh
└── docs/                 spec, architecture, packages, extending, recipes, install, overview.html
```

### 2.6 Front-end parity

The CLI, the MCP server and the dashboard are the three front ends. The
matrix below is the contract for the actions they share. The shared-function
column names the function in `internal/app`. When that package has no
wrapper, the cell names the function that owns the action.

A row is `built` when every front end exposes the action. A row is `partial`
when one or more front ends do not expose it. The list after the matrix names
every gap.

| Action | CLI command | MCP tool | Dashboard endpoint | Shared function | Status |
|---|---|---|---|---|---|
| Run the full setup path | `up` | deferred: `up` tool | not built: `up` endpoint; `POST /api/onboarding/{step}/run` runs one step | `flow.Up` | partial |
| Read the host status | `status` (also the default command) | `status` | not built: one status endpoint; `GET /api/hw`, `GET /api/doctor`, `GET /api/runs`, and `GET /api/onboarding` are separate | `flow.Snapshot` | partial |
| Read the onboarding checklist | `status --steps` | `onboarding_status` | `GET /api/onboarding` | `app.Onboarding`, `app.OnboardingCached` | built |
| Run one onboarding step | not built: one-step command | deferred: one-step onboarding tool | `POST /api/onboarding/{step}/run` | `app.RunOnboardingStep` | partial |
| List the background jobs | not built: jobs command | `jobs` | `GET /api/jobs` | `app.OnboardingJobs` | partial |
| Read the hardware inventory | `hw` | `hw` | `GET /api/hw` | `app.HW` | built |
| Read the PCIe topology | `topo` | `topo` | `GET /api/topo` | `app.Topo` | built |
| Check peer-to-peer readiness | `topo --check` | not built: `topo --check` tool | `GET /api/topo/check` | `app.P2PChecklist` | partial |
| Run the host checks | `doctor` | `doctor` | `GET /api/doctor` | `app.Doctor` | built |
| Match a known log failure | `doctor --run NAME`; `doctor --log FILE` | `doctor_log` | `GET /api/runs/{name}/findings` | `app.RunLog`, `logdoc.Scan` | partial |
| Plan the setup steps | `setup plan` | not built: `setup plan` tool | not built: `setup plan` endpoint | `app.SetupPlan` | partial |
| Apply the setup steps | `setup apply` | `setup_apply` | `POST /api/onboarding/{step}/run` (one checklist step) | `app.SetupApply`, `app.RunOnboardingStep` | partial |
| Check the venv again | `setup verify` | `setup_verify` | `POST /api/setup/verify` (the button of the venv detail) | `app.SetupVerify`, `app.RunSetupVerify` | built |
| Read the record of a setup run | `setup report`, `setup log` | `setup_report` | `GET /api/setup/runs` | `app.SetupRun`, `app.SetupLatestRun`, `app.SetupStepLog` | built |
| List the recipes | `recipe list` | `recipe_list` | `GET /api/recipes` | `app.RecipeList` | built |
| Show a recipe | `recipe show ID` | `recipe_show` | not built: `recipe show` endpoint | `recipe.Load`, `Set.Resolve` | partial |
| Validate the recipes | `recipe validate` | not built: `recipe validate` tool | not built: `recipe validate` endpoint | `recipe.Load`, `Set.Resolve`, `recipe.Validate` | partial |
| Build a launch plan | `recipe plan ID` | `recipe_plan` | `GET /api/recipes/{id}/plan` | `app.Plan` | built |
| Start a run | `serve ID` | `serve` | `POST /api/runs` | `app.Serve` | built |
| List the runs | `ps` | `ps` | `GET /api/runs` | `flow.Runs` | built |
| Stop a run | `stop [NAME]` | `stop` (requires a name) | `DELETE /api/runs/{name}` | `app.Stop` | built |
| Read a run log | `logs [NAME]` | `logs` | `GET /api/runs/{name}/logs`; `GET /api/runs/{name}/log` | `runner.Logs`, `runner.OpenLog` | built |
| Smoke-test a server | `smoke` | `smoke` | `POST /api/smoke` | `app.Smoke` | built |
| List the TunableOp profiles | `tune tunableop list` | not built: `tune tunableop list` tool | not built: `tune tunableop list` endpoint | `tuning.Profiles` | partial |
| Read the TunableOp status | `tune tunableop status` | `tunableop_status` | not built: `tune tunableop status` endpoint | `app.TunableOpStatus` | partial |
| Build the TunableOp environment | `tune tunableop env` | not built: `tune tunableop env` tool | not built: `tune tunableop env` endpoint | `app.TunableOpStatus`, `tuning.Select`, `tuning.Materialize`, `tuning.Env` | partial |
| Read the actual power caps | `tune power status` | not built: `tune power status` tool | `GET /api/power` (returns the service status with the actual caps) | `app.PowerStatus`, `app.ReadPowerServiceStatus` | partial |
| Read the power service status | `tune power service status` | `power_status` | `GET /api/power` | `app.ReadPowerServiceStatus` | built |
| Set the desired power caps | `tune power set` | `tune_power_set` | `PUT /api/power` | `app.PlanPowerDesired`, `app.PowerDesiredSet` | built |
| Apply the desired power caps | `tune power apply` | not built: `tune power apply` tool | not built: `tune power apply` endpoint | `app.PowerApply` | partial |
| Build the power service plan | `tune power service print`; `tune power service install-plan`; `tune power service remove-plan` | not built: `tune power service` tool | not built: `tune power service` endpoint | `app.GeneratePowerServicePlan` | partial |
| Run a benchmark | `bench` | `bench_run` | `POST /api/bench`; `POST /api/bench/run` | `app.Bench` | built |
| Run a benchmark suite | `bench --suite NAME` | `bench_run` (with `suite`) | `POST /api/bench`; `POST /api/bench/run` (with `suite`) | `app.BenchSuite` | built |
| List the benchmark results | `bench list` | `bench_list` | `GET /api/bench` | `app.BenchList` | built |
| Show a benchmark result | `bench show ID` | `bench_show` | not built: saved-result endpoint (`GET /api/bench/{id}` is a job) | `bench.Get` | partial |
| Compare two benchmark results | `bench compare A B` | `bench_compare` | `GET /api/bench/compare` | `app.BenchCompare` | built |
| Run an A/B benchmark | `bench ab` | `bench_ab` | `POST /api/bench/ab` | `app.BenchAB` | built |
| Follow a benchmark job | not built: a job command | not built: a job tool | `GET /api/bench/{id}`; `GET /api/bench/{id}/events` | `job.Store.Get` | partial |
| Write a compose file | `compose ID` | not built: `compose` tool | not built: `compose` endpoint | `app.Plan`, `runner.Compose` | partial |
| Show or set the user defaults | `config show`; `config set` | not built: `config show` tool; deferred: `config set` tool | not built: `config show` endpoint; not built: `config set` endpoint | `config.Load`, `Config.SetKey`, `config.Save` | partial |

The deferred and unbuilt items are:

- The MCP `up` tool, the MCP `config set` tool, and the MCP one-step
  onboarding tool are deferred. Spec §13.4 says that an agent uses the CLI
  for `up` and `config set`. It also says that the one-step tool is deferred
  because the CLI has no one-step onboarding command.
- The MCP has no tools for `topo --check`, `setup plan`, `recipe validate`,
  `tune tunableop list`, `tune tunableop env`, `tune power status`,
  `tune power apply`, `tune power service print`,
  `tune power service install-plan`, `tune power service remove-plan`,
  `compose`, `config show`, or `doctor --log FILE`.
- The dashboard has no endpoints for `setup plan`, `recipe show`,
  `recipe validate`, `tune tunableop status`, `tune tunableop list`,
  `tune tunableop env`, `tune power apply`, `tune power service print`,
  `tune power service install-plan`, `tune power service remove-plan`,
  `compose`, `config show`, `config set`, or a saved-result `bench show`.
  The dashboard also has no one-step status endpoint.
- The CLI and the MCP have no command and no tool that follow a running
  benchmark job. The dashboard reads the job document and its event stream.
- The repo gives no separate reason for the unbuilt MCP tools and dashboard
  endpoints above.
- `tune power apply` needs root. bernie never runs sudo, so the CLI prints
  the root command.
- `doctor --log FILE` stays in the CLI. The MCP `doctor_log` tool accepts a
  run name, never a path. The dashboard reads the findings of a run.

## 3. Flows

### 3.1 What `up` does

`up` is a thin front end over `flow.Up`. The sequence below shows who calls
whom. A step that cannot go on returns `Blocked`. `cli` prints it and exits
with code 3.

```mermaid
sequenceDiagram
  actor U as User
  participant C as cli (health/cmd_up)
  participant F as flow
  participant D as doctor
  participant R as recipe
  participant S as setup
  participant X as runner
  participant V as vLLM
  participant W as dashboard
  U->>C: bernie up [RECIPE]
  C->>F: Up(options)
  F->>D: Run(categories gpu, kernel)
  D-->>F: report (fail = Blocked + fix commands)
  F->>R: Load, List (fit), Best or Resolve
  R-->>F: recipe id
  F->>S: Plan(mode, GPU archs)
  S-->>F: steps (root step pending = Blocked + commands)
  F->>C: Confirm (once, skipped by --yes)
  C->>U: Run setup steps and start RECIPE?
  U-->>C: yes
  F->>S: Apply(non-root steps)
  F->>X: Prepare(recipe, mode, sets)
  X-->>F: plan (see 3.2)
  F->>X: Start(plan)
  X->>V: spawn process, or engine run
  F->>W: Start(listen) (the dashboard serves while the server loads)
  loop every 3 s until --wait
    F->>X: List
    X->>V: GET /health
  end
  Note over F,X: a run that exits: F scans its log (logdoc) and returns StartupExit with the matches
  F-->>C: UpResult (url, run record)
  C->>W: Wait (Ctrl-C stops the dashboard, the run goes on)
  W->>X: List, Logs (on request)
```

### 3.2 The onboarding

The onboarding is one document with the ordered steps from a bare machine to
a serving model. It is derived from the real state of the host and read by
every front end.

```mermaid
flowchart LR
  F["onboarding.Facts<br/>one probe"] --> D["onboarding.Build<br/>the steps"]
  D --> S["status --steps"]
  D --> U["up: the first blocked step"]
  D --> A["GET /api/onboarding"]
  D --> M["MCP onboarding_status"]
  U --> J["onboarding.Action<br/>the action of a step"]
  J --> P["POST /api/onboarding/{step}/run<br/>a job"]
```

`up` and the jobs run the same action: the setup steps, the serve and the
smoke check. A job keeps its state under `<state>/jobs/<id>`, so a page
reload finds it again.

### 3.3 From recipe to command

`runner.Prepare` runs this chain. `recipe plan`, `serve`, `compose` and the
dashboard use the same chain. First, the recipe becomes a plan:

```mermaid
flowchart LR
  L["Load<br/>layers"] --> R["Resolve<br/>merge extends"]
  L --> F["CheckFit<br/>used by list, up"]
  R --> O["Apply<br/>config set, --set<br/>then Validate"]
  O --> P["BuildPlan<br/>inputs, env, args,<br/>GPU selection:<br/>arch, count, PCIe switch,<br/>host: tuned-MoE folder"]
  W["gpu.List<br/>topo.Discover"] --> P
```

Then the plan gets the host and becomes a command:

```mermaid
flowchart LR
  S["Plan (from BuildPlan)"] --> H["Host env, or<br/>container mounts"]
  H --> T["TunableOp profile<br/>host: rocBLAS hash + arch<br/>container: recipe profile"]
  T --> M{"mode"}
  M -- host --> X1["Plan.Exec<br/>venv python -m vllm"]
  M -- container --> X2["Plan.Exec<br/>engine run image,<br/>or compose file"]
```

### 3.3 Life of a run

The state of a run is not stored. `runner.List` derives it from the process
(or container), `/health` and one completion. Only `run.json` and the
`answered` mark of the first completion are stored. A plan has no record until it starts.

```mermaid
stateDiagram-v2
  [*] --> planned: Prepare (recipe plan)
  planned --> starting: Start writes run.json
  starting --> running: /health answers 200 and one completion answers
  running --> starting: process alive, /health silent
  starting --> exited: process or container gone
  running --> exited: bernie stop, or a crash
  exited --> starting: serve with the same name
  exited --> [*]
```

`exited` covers a normal `stop` and a failure. To tell them apart, run
`bernie doctor --run NAME`. It matches the log against the known-failure
catalog.

## 4. Data keyed by GPU architecture

Four kinds of data depend on the GPU. Each has one folder per key. A new
architecture adds files. It never edits the files of another architecture.

```mermaid
flowchart LR
  ARCH(["GPU arch<br/>gfx1030, gfx1100, ..."])
  ARCH --- DEV["internal/machine/gpu/devices/<br/>generation.yaml"]
  ARCH --- TOP["tunableop/arch/profile/<br/>profile.json + rows"]
  ARCH --- MOE["tuned-moe/arch/<br/>E=...json"]
  ARCH --- REC["recipes/<br/>requires.arch + fragments/arch/"]
  DEV --> R1["devices.Lookup, LookupArch, CodeTargets<br/>used by gpu, doctor, setup"]
  TOP --> R2["tuning.Profiles, Select<br/>used by runner, doctor"]
  MOE --> R3["tuning.MaterializeTunedMoE<br/>used by recipe plan"]
  REC --> R4["recipe.Load, CheckFit<br/>used by up, list, plan"]
```

Rules:

- Every reader finds the data by walking the embedded tree. No Go file lists
  a device, a profile or a recipe.
- A profile says which architectures it fits in its own `profile.json`.
- `doctor` learns the validated architectures from the recipes
  (`recipe.ValidatedArchs`), not from a list.

[extending.md](extending.md) lists the steps to add a generation.

## 5. Testing

- Unit tests read a root path. Fixture trees live in
  `internal/support/fixture/testdata/<name>.tar.gz`. `fixture.Root(t, name)`
  unpacks one. Capture a real host with `scripts/capture-fixture.sh`.
- A fixture that was not captured is synthetic. Its name ends in
  `-synthetic` and it has a `SYNTHETIC` file at its root. The README next to
  the fixtures says which values are real and which are written by hand.
  Never present a synthetic fixture as proof of hardware support.
- `devices` tests check that every device table loads, ids are unique and
  each override points to an arch of the same file.
- `tuning` tests check the layout of `tunableop/` and `tuned-moe/` (top
  folders are gfx names, a profile folder holds a manifest, rows and a
  README) and that every `profile.json` name equals its folder name.
- `recipe` tests check merge rules, interpolation, rendering and GPU
  selection. Every built-in recipe must load, resolve, validate and plan
  (`go test ./recipes/...`).
- `runner` tests check the rendered argv and compose output. They do not run
  processes.
- `bench` tests use an in-process fake OpenAI server.
- Before a release, smoke on real hardware: `doctor`, `topo`, `recipe plan`
  for each built-in recipe, one `serve` and one `bench` cell.
