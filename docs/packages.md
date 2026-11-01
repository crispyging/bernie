# bernie package contracts

This page lists the API that packages agree on. Unexported helpers are free.
Fields and signatures here are a summary: the Go doc comments are the source.
The layers and the import rules are in [architecture.md](architecture.md).

Packages are grouped by role: `internal/machine` (what the host is),
`internal/serving` (what bernie runs) and `internal/support` (small helpers).

## The packages

| Package | Role | Public surface | Layer |
|---|---|---|---|
| `cmd/bernie` | the entry point | `main` | 4 |
| `scripts/fakevllm` | the fake vLLM server for the bench smoke test | `main` | - |
| `internal/cli` | the command framework | `Register`, `Main`, `Command`, `Context`, `ExitError`, `WriteMatches` | 4 |
| `internal/cli/cmds/health` | `doctor`, `setup`, `status`, `up` | `cli.Register` per command | 4 |
| `internal/cli/cmds/inventory` | `hw`, `topo` | `cli.Register` per command | 4 |
| `internal/cli/cmds/serving` | `recipe`, `serve`, `ps`, `stop`, `logs`, `compose`, `config` | `cli.Register` per command | 4 |
| `internal/cli/cmds/tunecmd` | `tune` | `cli.Register` per command | 4 |
| `internal/cli/cmds/benchcmd` | `bench` | `cli.Register` per command | 4 |
| `internal/cli/cmds/dashcmd` | `dash` | `cli.Register` per command | 4 |
| `internal/cli/cmds/mcpcmd` | `mcp` | `cli.Register` per command | 4 |
| `internal/mcp` | the MCP front end | `New`, `Run`, `Handler`, `IsMutating`, `Options` | 4 |
| `dashboard` | the web front end: the server, the API, the guard and the telemetry | `Serve`, `Start`, `NewStarter`, `NewToken`, `IsLoopback`, `Options` | 4 |
| `dashboard/internal/api` | the JSON and SSE endpoints, one file per domain | `Register`, `Options` | 4 |
| `dashboard/internal/auth` | the token, the loopback Host check and the CSRF guard | `New`, `NewToken`, `IsLoopback` | 4 |
| `dashboard/internal/httpjson` | the JSON response shapes and the redaction | `Write`, `Error`, `Decode`, `Redacted` | 4 |
| `dashboard/internal/sse` | the server-sent event writer | `New`, `Stream`, `LineWriter`, `Heartbeat` | 4 |
| `dashboard/internal/telemetry` | the 1 Hz sampler and the hub that shares it | `NewHub`, `Hub`, `Sampler` | 4 |
| `internal/app` | the operations the front ends share | `HW`, `Topo`, `Doctor`, `RecipeList`, `SetupPlan`, `SetupApply`, `SetupRuns`, `SetupRun`, `SetupLatestRun`, `Serve`, `Plan`, `Bench`, `BenchSuite`, `BenchCompare`, `PowerStatus`, `Onboarding`, `RunOnboardingStep`, `Stop`, `Smoke` | 3 |
| `internal/flow` | the `status` snapshot and the `up` chain | `Summary`, `Up`, `Findings`, `WaitHealthy`, `Blocked` | 3 |
| `internal/onboarding` | the onboarding checklist | `Gather`, `Build`, `Load`, `PrepareStep`, `ActionFor`, `StartJob`, `Smoke`, `Document`, `Facts` | 3 |
| `internal/bench/job` | the benchmark jobs: the store, the state machine and the typed progress events | `NewStore`, `Store`, `Request`, `Doc`, `Job`, `Progress` | 3 |
| `internal/doctor` | the host checks | `Run`, `Report`, `Check`, `Status`, `Options` | 2 |
| `internal/setup` | the install plan and the record of a run | `Plan`, `Apply`, `ApplyRun`, `Step`, `Options`, `Run`, `RunStep`, `RunStore`, `Progress` | 2 |
| `internal/logdoc` | the log knowledge | `Load`, `Scan`, `ScanFile`, `PhaseOf`, `Rule`, `Match` | 2 |
| `internal/serving` | the serving package group | none (directory only) | 2 |
| `internal/serving/recipe` | the recipe and its launch plan | `Load`, `Resolve`, `Validate`, `CheckFit`, `List`, `Best`, `BuildPlan`, `Plan` | 2 |
| `internal/serving/runner` | the run | `Prepare`, `Start`, `List`, `Stop`, `Logs`, `Get`, `OpenLog`, `Phase` | 2 |
| `internal/serving/tuning` | the tuning rows and the power cap | `Profiles`, `Select`, `Materialize`, `Env`, `PowerStatus`, `LoadPowerDesired`, `SavePowerDesired`, `GeneratePowerUnits`, `ApplyPowerDesired` | 2 |
| `internal/serving/vllm` | the vLLM client | `New`, `BaseURL`, `Health`, `Models`, `Tokenize`, `Complete`, `Metrics`, `ParseMetrics`, `Summarize`, `Histograms`, `Stats`, `Latency`, `Window`, `Reading`, `Watcher`, `NewWatcher`, `DefaultWindow` | 2 |
| `internal/serving/vllm/vllmtest` | the deterministic fake vLLM server for tests | `Config`, `Server`, `New`, `CountTokens`, `Prompts`, `ServeHTTP` | 2 |
| `internal/serving/bench` | the benchmark | `Run`, `Save`, `List`, `Get`, `Options`, `Result` | 2 |
| `internal/serving/bench/suite` | the bench suites | `Load`, `List`, `Get`, `Run`, `Compare`, `Judge`, `AB` | 2 |
| `internal/machine/gpu` | the live GPUs | `List`, `Telemetry`, `TelemetryFast`, `TelemetrySlow`, `MergeSlow`, `Archs`, `ArchMatch`, `ECC` | 1 |
| `internal/machine/gpu/devices` | the device tables | `Lookup`, `LookupArch`, `CodeTargets`, `GFXName`, `Arch`, `Device` | 1 |
| `internal/machine/host` | the OS facts | `Probe`, `Preferred`, `InstallCommand`, `GPUGroups`, `Info`, `ProbeCDI`, `SpecStale`, `EngineCDISupport`, `ResolveDeviceAccess` | 1 |
| `internal/machine/rocm` | the ROCm facts | `Probe`, `ProbeVenv`, `VenvLibPaths`, `FindROCBLAS`, `CompareVersions`, `RuntimeInstall` | 1 |
| `internal/machine/topo` | the PCIe facts | `Discover`, `MeasureBandwidth`, `BestGroup`, `P2PChecklist` | 1 |
| `internal/support/paths` | the XDG paths | `State`, `Config`, `Data`, `Cache`, `Ensure` | 0 |
| `internal/support/hostroot` | the root of the host tree bernie reads | `Root`, `Set`, `EnvVar` | 0 |
| `internal/support/execx` | the bounded exec | `Result`, `Which`, `FirstLine` | 0 |
| `internal/support/config` | the config file | `Config`, `Path`, `Save`, `Keys` | 0 |
| `internal/support/redact` | the secret redaction | `JSON`, `Text` | 0 |
| `internal/support/pyenv` | the venv layout | `HasModule`, `SearchPaths`, `SitePackages` | 0 |
| `internal/support/factcache` | the cached host facts | `Get`, `Load`, `Save` | 0 |
| `internal/support/fixture` | the test fixtures | `Root`, `Write` | 0 |
| `internal/glossary` | the words a reader may not know | `Word`, `Words`, `Terms`, `Meaning` | data |
| `internal/support/netx` | the shared loopback check | `Loopback` | 0 |
| `recipes`, `tunableop`, `tuned-moe`, `bench` | the embedded data | `FS` | data |

The layer of every package is in [architecture.md](architecture.md) §2.

## machine/gpu

```go
type GPU struct { HIPIndex, KFDNode int; Name, GFX string; GFXTargetVersion int;
    PCI, DeviceID string; VRAMBytes uint64; RenderMinor, NUMANode int;
    Link Link; PowerCapW float64; SysfsPath string }
type Sample struct { HIPIndex int; BusyPct, TempC, PowerW, PowerCapW float64;
    VRAMUsed, VRAMTotal uint64 }
func List(root string) ([]GPU, error)
func Telemetry(root string, gpus []GPU) []Sample
func TelemetryFast(root string, gpus []GPU) []Sample   // the load, the memory, the power
func TelemetrySlow(root string, gpus []GPU) []Sample   // the totals, the temperatures, the caps, the clocks
func MergeSlow(a, b *Sample)                          // copies the slow attributes of b into a
func ArchMatch(patterns []string, gfx string) bool   // full name or prefix
func Archs(gpus []GPU, idx []int) []string
```

`gpu` reads the live GPUs and imports only `devices`. No other package
names a device. The two tiers of the telemetry are the ones of spec §11:
`Telemetry` reads both.

## machine/gpu/devices

```go
// Device tables (spec §4.4): internal/machine/gpu/devices/<generation>.yaml
type Arch struct { GFX, Generation, Family string; WaveSize int; BF16, HSAOverride,
    TritonBelow string; Integrated, FlashAttn bool }
type Device struct { Arch; ID, Name string; PowerCapAdviceW int }
func GFXName(targetVersion int) string      // a KFD target version -> gfx1030
func Lookup(deviceID string) (Device, bool)   // by PCI device id
func LookupArch(gfx string) (Arch, bool)
func CodeTargets(gfxs []string) []string      // arch -> arch whose code objects it runs
```

`devices` holds the tables and the loader that walks them. It imports no
other bernie package, and no other package reads the tables.

## machine/host

```go
type Info struct { Hostname, Kernel, PkgManager, Virtualization string
    OS OSRelease; CPU CPU; RAMBytes uint64; User User; Engines []Engine
    SELinux, Cmdline string; ShmBytes uint64 }
func Probe(ctx context.Context, root string) (Info, error)   // root "/" in production
func Preferred(engines []Engine, name string) (Engine, bool)
func InstallCommand(pm, feature string) string               // package-manager fix text

type DeviceGroup struct { Role string; GID int; Source string }
func GPUGroups(ctx context.Context, root string) []DeviceGroup  // numeric ids of the GPU groups

type CDISpec struct { Path, Kind, Version string; Devices []CDIDevice }
type CDI struct { Toolkit bool; ToolkitPath, ToolkitVersion string; SpecDirs []string; Spec *CDISpec }
func ProbeCDI(ctx context.Context, root, engine string) CDI     // amd-ctk and the amd.com/gpu spec
func SpecStale(root string, s *CDISpec, gpuCount int) string   // why the spec does not fit the host
func EngineCDISupport(e Engine, root string) (bool, string)   // can the engine use a CDI spec
func ResolveDeviceAccess(req string, e Engine, c CDI, stale, why string) DeviceAccess
```

## machine/rocm

```go
type Install struct { Root, Version, Kind string; Nested []Install }   // Kind: opt|therock|distro|wheel
type Report struct { Installs []Install; Tools map[string]string; OnPath map[string]bool;
    ROCBLAS, HIPVersion string; Agents []string }
type VenvReport struct { Python, Torch, HIP, VLLM string; DeviceCount, RDNAOps int; Err string }
func Probe(ctx context.Context, root string) Report
func ProbeVenv(ctx context.Context, venv string) VenvReport
func VenvLibPaths(venv string) []string          // ROCm wheel libs + torch/lib, in load order
func FindROCBLAS(root, venv string) string       // venv wheel first, then system
func CompareVersions(a, b string) int
func ResolvePath(p string) string                // resolves the symlinks of a tool path
func InstallForPath(installs []Install, p string) (Install, bool) // the install that holds p
```

## machine/topo

```go
type Pair struct { A, B int; Class string; Hops int; P2P bool; GBps float64 }
type Node struct { HIPIndex int; PCI string; Path []string; Switch, RootPort, Root string }
type Topology struct { Nodes []Node; Pairs []Pair; Bandwidth string }
func Discover(root string, gpus []gpu.GPU) (Topology, error)
func MeasureBandwidth(ctx context.Context, t *Topology, venv string) error
func BestGroup(t Topology, candidates []int, n int) (sel []int, reason string)

// P2P readiness checklist (spec §4.5).
type CheckItem struct { ID, Title string; Status P2PStatus; Detail, Fix, Doc string }
type RecipeEnv struct { ID string; Env map[string]string }
func P2PChecklist(root string, gpus []gpu.GPU, o P2POptions) ([]CheckItem, error)
```

## doctor

```go
type Status string // ok|warn|fail|skip
type Check struct { ID, Category string; Status Status; Title, Detail, Fix, Doc string }
type Report struct { Summary map[Status]int; Checks []Check; Venv *VenvInfo }
type VenvInfo struct { Path, Source string; Candidates []string }
type Options struct { Root, Venv string; VenvSource string; VenvCandidates []string;
    Categories []string; HFHome string; ValidatedArchs []string }
func Run(ctx context.Context, o Options) Report
func (Report) Failed() bool
```

`ValidatedArchs` comes from `recipe.ValidatedArchs`. `doctor` does not import
`recipe`. Check files are grouped by role. GPU and topology checks share
`checks_gpu.go`, with the GPU-group helpers; resource checks own the
filesystem helper.

## logdoc

```go
type Rule struct { ID, Severity, Pattern, Cause, Fix, Doc string }
type Match struct { Rule; Line string; LineNo int }
func Load() ([]Rule, error)                       // embedded catalog.yaml, validated
func Scan(r io.Reader, rules []Rule) []Match      // first matching line per rule, streaming
func ScanFile(path string) ([]Match, error)
type Phase struct { ID, Sentence string; Done, Total int }
func LoadPhases() ([]PhaseRule, error)            // embedded phase.yaml, validated
func PhaseOf(tail []byte) (Phase, bool)           // phase of the last matching line
```

## setup

```go
type Step struct { ID, Title, Mode string; Root, Done bool; Commands []string; Why string
                   Download *Download }
type Download struct { Repo, HubCache string; ResumeBytes int64 }
type Options struct { Mode, Venv, Image, ModelRepo, SrcDir, HFHome string; GPUArchs []string }
func Plan(ctx context.Context, h host.Info, r rocm.Report, o Options) []Step
func Apply(ctx context.Context, steps []Step, only []string, log io.Writer) error
```

`Plan` takes the GPU targets from `GPUArchs` and asks the device tables for
code targets and the FlashAttention allowlist. The step with a `Download`
puts the files of the repo in the Hugging Face cache: a partial file
continues with a `Range` request, and the checksum of every file is
checked.

## serving/recipe

```go
type Set struct { Recipes map[string]*Recipe; Errors []LoadError }
func Load(extraDirs ...string) Set                 // layers: embedded, config, $BERNIE_RECIPES, extra
func (s Set) Resolve(id string) (*Recipe, error)   // extends merged
func Validate(r *Recipe) []string
type Fit struct { Status string; Reasons, Modes []string }   // fits|tight|no|unknown
func (f Fit) Usable() bool                         // fits or tight
type Host struct { GPUs []gpu.GPU; RAMBytes, DiskFreeBytes uint64; Cached func(model string) bool }
func CheckFit(r *Recipe, h Host) Fit
type KV struct { MaxModelLen int; BytesPerToken, NeedBytes, BudgetBytes float64; Source string }
func CheckKV(r *Recipe, minVRAM uint64) *KV        // nil: no KV data
func List(s Set, h Host, all bool) Listing
func Best(l Listing) string                        // fits, then tight, then priority, then id
func ValidatedArchs(s Set) []string
type Plan struct { Recipe, Name, Mode, Entrypoint, Model, ServedName, Host string; Port int
    Env map[string]string; Args []string; Settings []Setting; Adjustments []Adjustment; GPUs []int; HIPDevices, GPUSelectReason string
    Image, ShmSize, TunableOpProfile, TunableOpRows string; PathInputs []string
    Mounts []Mount; Warnings []string; Exec *Exec }
type Setting struct { Setting, Value, Why string }
type Adjustment struct { Setting, From, To, Reason string }   // Plan.Adjustments
type PlanOptions struct { Mode, Name string; Sets []string; GPUs []gpu.GPU; Topo *topo.Topology;
    TunedMoE func(gfx string) (string, error) }
func BuildPlan(r *Recipe, o PlanOptions) (*Plan, error)
```

## serving/tuning

```go
type Profile struct { Name, Dir, ROCBLAS, ROCm, Torch, LibSHA256, Status string;
    Arch []string; RowsPerRank int; Covers []string }
func Profiles() ([]Profile, error)                 // walks tunableop/<arch>/<profile>/profile.json
func HashLib(path string) (string, error)
func Select(libSHA, explicit string, archs []string) (*Profile, error)
func Materialize(p *Profile) (dir string, err error)       // <data>/tunableop/<name>
func Env(p *Profile, rowsDir string, tuning bool) map[string]string
func MaterializeTunedMoE(gfx string) (dir string, err error) // <data>/tuned-moe/<arch>, "" if none
type Power struct { HIPIndex int; CapW, MinW, MaxW float64; Path string }
func PowerStatus(gpus []gpu.GPU) []Power
type PowerDesired struct { Version int; GPUs map[string]float64 }
func LoadPowerDesired(path string) (PowerDesired, error)
func SavePowerDesired(path string, d PowerDesired) error
type PowerUnits struct { ServiceFile, PathFile, Service, Path string }
func GeneratePowerUnits(binary, desired, stateDir string) (PowerUnits, error)
func ApplyPowerDesired(root string, gpus []gpu.GPU, file, binary string, isRoot bool) (PowerApplyResult, error)
```

## serving/runner

```go
type PrepareOptions struct { Recipe, Mode, Name, Venv, Engine string; Sets []string;
    Profile string; NoTunableOp, Tuning bool; RecipePaths []string }
func Prepare(ctx context.Context, o PrepareOptions) (*recipe.Plan, error) // config, recipe, GPUs, plan, host env, TunableOp, exec
type Record struct { Name, Recipe, Mode string; PID int; Container, Engine, Host string;
    Port int; StartedAt time.Time; Log, Image, ImageDigest string; Plan *recipe.Plan }
type Status struct { Record; State, Waiting string }   // running|starting|exited; Waiting: why starting
func Start(ctx context.Context, p *recipe.Plan, o StartOptions) (*Record, error)
func List(ctx context.Context) ([]Status, error)
func Stop(ctx context.Context, name string) error
func Logs(ctx context.Context, name string, follow bool, tail int, w io.Writer) error
func Get(name string) (*Record, error)             // one record; the error wraps os.ErrNotExist
func OpenLog(ctx context.Context, r Record, timeout time.Duration) (io.ReadCloser, error) // whole log: file, or `<engine> logs` under the execx timeout
func HostCommand(p *recipe.Plan, venv string) (argv []string, env []string, err error)
func ContainerCommand(p *recipe.Plan, eng host.Engine, rowsDir, hfCache string) []string
func Compose(p *recipe.Plan, rowsDir, hfCache string) ([]byte, error)
```

## serving/vllm and serving/bench

```go
// vllm
func New(baseURL string) *Client
func (c *Client) Health(ctx) error
func (c *Client) Models(ctx) ([]string, error)
func (c *Client) Tokenize(ctx, model, text string) (int, error)
func (c *Client) Metrics(ctx) (map[string]float64, error)   // flattened Prometheus samples
func ParseMetrics(data []byte) (map[string]float64, error)
func Summarize(m map[string]float64) Stats                   // the numbers of the server since it started
func NewWatcher(baseURL string, window time.Duration) *Watcher
func (w *Watcher) Sample(ctx) (Reading, error)               // the stats and the window of the server
// bench
func Run(ctx context.Context, o Options, progress io.Writer) (*Result, error)
func Save(r *Result) (string, error); func List() ([]Result, error); func Get(id string) (*Result, error)
// bench/suite
func Load(data []byte) (*Suite, error)          // strict YAML: unknown keys are refused
func List() ([]*Suite, error); func Get(id string) (*Suite, error)
func (s *Suite) Digest() string                 // sha256 of the canonical suite
func Run(ctx context.Context, s *Suite, o bench.Options, progress io.Writer) (*bench.Result, error)
func Compare(a, b *bench.Result) (*Comparison, error)   // ErrWorkloadDiffers: the workloads differ
type Starter func(context.Context, string, string) (*runner.Record, error) // starts a recipe arm
func Judge(cmp *Comparison, band float64) *Verdict      // better, same, worse, noisy
func AB(ctx context.Context, o ABOptions) (*ABResult, error)  // the visits base, cand, cand, base
```

## app

```go
// Each function returns the canonical struct of its `--json` document.
func HW(ctx context.Context) (HWReport, error)                          // GPUErr: why GPUs is empty
func Topo(ctx context.Context, o TopoOptions) (topo.Topology, error)    // Bandwidth, Venv, NoKFDOK, Progress
func P2PChecklist(ctx context.Context, o P2PChecklistOptions) (P2PChecklistReport, error) // topo --check
func Doctor(ctx context.Context, categories []string, venv string) (doctor.Report, error)
func RunLog(ctx context.Context, name string) (LogReport, error)        // doctor --run
func NewLogReport(matches []logdoc.Match) LogReport                     // never null
func RecipeList(all bool) recipe.Listing
func TunableOpStatus(venv string) (TunableOpState, error)
func PowerStatus() ([]tuning.Power, error)
func PlanPowerDesired(idx []int, watts float64) (PowerDesiredPlan, error)
func PowerDesiredSet(idx []int, watts float64) (PowerDesiredResult, error)
func PowerApply(file string) (tuning.PowerApplyResult, error)          // ErrNeedsRoot without root
func GeneratePowerServicePlan() (PowerServicePlan, error)
func ReadPowerServiceStatus(ctx context.Context) (PowerServiceStatus, error)
func SetupPlan(ctx context.Context, o SetupOptions) ([]setup.Step, error)
func SetupPending(steps []setup.Step, only []string) []setup.Step
func SetupApply(ctx context.Context, steps []setup.Step, o SetupOptions, only []string, log io.Writer) (*SetupResult, error)
func SetupRuns(limit int) []*setup.Run
func SetupRun(id string) (*setup.Run, error)
func SetupLatestRun() (*setup.Run, error)
func SetupStepLog(runID, stepID string) ([]byte, error)
func Plan(ctx context.Context, o runner.PrepareOptions) (*recipe.Plan, error)
func Serve(ctx context.Context, o ServeOptions) (*runner.Record, error) // ErrPrepare, ErrRunActive
func Bench(ctx context.Context, o bench.Options, progress io.Writer) (*bench.Result, string, error) // ErrAllFailed, *SaveError
func BenchSuite(ctx context.Context, name string, o bench.Options, progress io.Writer) (*bench.Result, string, error)
func BenchCompare(aID, bID string, band float64) (*suite.Verdict, error) // ErrBenchResultNotFound
func BenchList() ([]bench.Result, error)                                // never null
func Stop(ctx context.Context, name string) (StopResult, error)
func Onboarding(ctx context.Context, o OnboardingOptions) (*onboarding.Document, error) // the checklist
func OnboardingCached(ctx context.Context, o OnboardingOptions) (*onboarding.Document, error) // the cached checklist
func RunOnboardingStep(ctx context.Context, step string, o OnboardingOptions) (OnboardingStepResult, error)
func OnboardingJob(id string) (*onboarding.Job, error)
func OnboardingJobLog(id string) ([]string, error)
func Smoke(ctx context.Context, o SmokeOptions) (*onboarding.SmokeResult, error)
```

`app` takes typed, validated input. The CLI parses flags, the dashboard parses
its JSON body, the MCP server checks its input schema. `app` never prints.

## onboarding

```go
type Document struct { Next string; Steps []Step; FactsAgeS int; Stale bool }  // the checklist
type Step struct { ID, Title, State, Summary, Detail, Command string; CanRun bool;
    RootCommands []string; Progress *Progress; Problems []Problem }
type Problem struct { ID, Title, Status, Detail, Fix, Doc string }  // a doctor check
type Facts struct { Host host.Info; GPUs []gpu.GPU; Restriction string; Doctor doctor.Report;
    Recipes recipe.Listing; Fits []string; Best, Recipe, Mode string; Setup []setup.Step;
    Venv, HFHome, Model string; Runs []Run; Jobs []Job }
func Gather(ctx context.Context, o Options) (*Facts, error)   // one probe of the host
func Build(f *Facts) *Document                                 // the steps from the facts
func Load(ctx context.Context, o Options) (*Document, error)   // the cached document, refreshed in the background when it is stale
type Action struct { Step, Kind string; Steps []setup.Step; Recipe, Mode, Venv string; Sets []string; Serve ServeFunc }
type PreparedStep struct { Step string; RootCommands []string; Action *Action }
func PrepareStep(f *Facts, step string, o Options) (*PreparedStep, error) // ErrUnknownStep, ErrNoAction
func ActionFor(f *Facts, step string, o Options) (*Action, error)  // ErrNoAction
func SetupAction(steps []setup.Step) *Action                  // what `up` applies
func (a *Action) Run(ctx context.Context, log io.Writer) error
func StartJob(ctx context.Context, prepared *PreparedStep) (*Job, error) // ErrJobBusy
func RecentJobs() []Job; func GetJob(id string) (*Job, error); func JobLog(id string) ([]string, error)
func Smoke(ctx context.Context, baseURL, model string) (*SmokeResult, error)
func LiveRun(ctx context.Context) (*Run, error)
```

`onboarding` holds one document: the ordered steps from a bare machine to a
serving model. The steps come from the facts (the doctor report, the setup
plan, the recipes, the runs), so the CLI, the dashboard and the MCP server
cannot disagree. A job is one step that runs in the background; its state is
kept under `<state>/jobs/<id>`.

## mcp

```go
type Options struct { Write bool; Transport, Version string; Log io.Writer }
func New(o Options) *Server               // registers the fixed tool catalog
func (s *Server) Run(ctx context.Context) error   // stdio
func (s *Server) Handler(token string) http.Handler // Streamable HTTP, stateless, guarded
func IsMutating(tool string) bool                 // serve, stop, bench_run, setup_apply, setup_verify, tune_power_set
```

A tool is a function `(ctx, in) -> canonical document` registered with its
input type and its output type; the SDK derives and enforces both schemas.
`confirm.go` holds the token store, `audit.go` the audit log, `http.go` the
guard of `/mcp` (Host, Origin, bearer token, POST only, `X-Bernie` for a
mutating tool; spec §13.10). `dashboard` mounts the handler on loopback only.

## flow

```go
type Summary struct { Host host.Info; GPUs []gpu.GPU; Doctor map[doctor.Status]int;
    Problems []doctor.Check; Best string; Fits []string; Runs []Run; Next []string;
    Onboarding *onboarding.Document; Venv *doctor.VenvInfo }
func Snapshot(ctx context.Context, venv string) (*Summary, error)           // `status`
type UpOptions struct { Recipe string; Sets []string; Mode, Venv, Engine, Name string;
    Yes bool; Confirm func(prompt string) bool; Wait time.Duration }
type Blocked struct { Reason string; Commands []string }                      // exit code 3
func Up(ctx context.Context, o UpOptions, log io.Writer) (*UpResult, error)
func WaitHealthy(ctx context.Context, name string, limit time.Duration, progress io.Writer) error // *StartupExit when the run exits first
type Run struct { runner.Status; Findings []logdoc.Match; Phase *logdoc.Phase } // Findings only for an exited run
type StartupExit struct { Name string; Findings []logdoc.Match }               // error of a run that exited before /health
func Runs(ctx context.Context) ([]Run, error)                                  // `ps`: runner.List plus findings
func Findings(ctx context.Context, r runner.Record) []logdoc.Match            // scan of an exited run's log, 2 s budget
```

The phase of a live run comes from `runner.Phase` (the runner owns the log).

```go
func Phase(ctx context.Context, r runner.Record) (logdoc.Phase, bool)         // in serving/runner
```

`flow` never starts the dashboard. The front end does that after `Up` returns.

## support

```go
// paths: XDG roots with BERNIE_*_DIR overrides
func Config() string; func State() string; func Data() string; func Cache() string
func Ensure(dir string) (string, error)
// execx: bounded exec, always with a timeout
func Run(ctx context.Context, timeout time.Duration, name string, args ...string) (Result, error)
// redact: hide secrets before output
func JSON(v any) (any, error); func Text(s string) string
// config: <config>/config.yaml
func Load() (Config, error); func Save(c Config) error
func (c Config) ResolveVenv(flag string) string
func (c Config) ResolveVenvInfo(flag string) VenvResolution   // path, source and candidates
type VenvResolution struct { Path, Source string; Candidates []string }
// pyenv: the layout of a venv on disk, without starting its Python
func SitePackages(venv string) []string
func SearchPaths(venv string) []string
func HasModule(venv, module string) bool
// factcache: slow, stable facts under <state>/facts
func Load(key, fingerprint string, ttl time.Duration, value any) (age time.Duration, fresh bool)
func Save(key, fingerprint string, value any) error
func Get[T any](ctx context.Context, key, fingerprint string, ttl time.Duration,
    produce func(context.Context) (T, error)) (value T, age time.Duration, stale bool, err error)
// fixture: test host snapshots
func Root(tb testing.TB, name string) string
```

## cli and dashboard

`internal/cli` holds the command framework: `Register`, `Context` (flags,
`Emit` for `--json`), exit codes. A command is one `cmd_<name>.go` file in a
family package under `internal/cli/cmds/`. It calls `cli.Register` in
`init()`. `cmd/bernie/main.go` blank-imports each family.

`dashboard` serves the JSON API and the embedded `web/` of spec §11, and
mounts `/mcp`. A handler calls the same packages as the matching command.
The root package holds the server and its public surface. The internal
packages split the work by role: `dashboard/internal/api` (one file per
domain), `dashboard/internal/auth` (the token, the Host check and the CSRF
guard), `dashboard/internal/telemetry` (the sampler and the hub),
`dashboard/internal/sse` (the event writer) and `dashboard/internal/httpjson`
(the JSON response shapes). The web tree is grouped by role:
`web/css`, `web/js`, `web/components` and `web/views/<tab>`.
