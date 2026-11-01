package onboarding

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/opengfx1030/bernie/internal/doctor"
	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/host"
	"github.com/opengfx1030/bernie/internal/machine/rocm"
	"github.com/opengfx1030/bernie/internal/machine/topo"
	"github.com/opengfx1030/bernie/internal/serving/recipe"
	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/setup"
	"github.com/opengfx1030/bernie/internal/support/config"
	"github.com/opengfx1030/bernie/internal/support/hostroot"
)

// Options are the inputs of the onboarding: the flags of `up` and of the
// onboarding endpoints. An empty field means "the config, then the default".
type Options struct {
	Venv    string   // host-mode venv
	Recipe  string   // "" = the best fitting recipe
	Mode    string   // host | container
	Sets    []string // --set overrides
	Engine  string   // container engine
	Name    string   // run name
	Root    string   // root of the host tree to read; "" = the real host
	Verbose bool     // stream raw step output on top of the progress printer
}

// Facts is the state of the host, gathered once. The steps are derived from
// it, so every front end sees the same state.
type Facts struct {
	Host           host.Info
	GPUs           []gpu.GPU
	Restriction    string // the note of the active GPU restriction (spec §7.7)
	Doctor         doctor.Report
	Recipes        recipe.Listing
	Fits           []string // the recipes that fit this host
	Best           string   // the best fitting recipe
	Recipe         string   // the recipe to serve (the named one, else the best)
	Mode           string   // host | container
	Setup          []setup.Step
	Venv           string
	VenvSource     string
	VenvCandidates []string
	HFHome         string
	Model          string // the model repo of the recipe
	Runs           []Run
	Jobs           []Job
	// roc is the ROCm report of this host. The doctor report and the setup
	// plan share it: the ROCm trees are probed once.
	roc rocm.Report
}

// Run is one run of this host.
type Run struct {
	Name   string
	Recipe string
	State  string
	Mode   string
	URL    string
	Phase  string // the phase sentence of a live run (spec §5.4)
}

// recipeEnv is the environment of the chosen recipe. It fills the
// informational recipe item of the topo.p2p category.
func recipeEnv(set recipe.Set, id string) topo.RecipeEnv {
	if id == "" {
		return topo.RecipeEnv{}
	}
	env, err := recipe.Env(set, id)
	if err != nil {
		return topo.RecipeEnv{}
	}
	return env
}

// Gather probes the host once and returns the facts the steps are derived
// from. The independent probes run together: the host, the GPUs and the
// ROCm trees do not depend on each other, and neither do the doctor
// report, the setup plan and the runs.
func Gather(ctx context.Context, o Options) (*Facts, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	venv := cfg.ResolveVenvInfo(o.Venv)
	f := &Facts{
		GPUs: []gpu.GPU{},
		Venv: venv.Path, VenvSource: venv.Source, VenvCandidates: venv.Candidates,
		HFHome: cfg.ResolveHFHome(),
	}
	var wg sync.WaitGroup
	run := func(fn func()) {
		wg.Add(1)
		go func() { defer wg.Done(); fn() }()
	}
	// The root of the host tree: "/" on the real host, another tree when
	// the caller points at a captured host (spec §4).
	root := o.Root
	if root == "" {
		root = hostroot.Root()
	}
	var gpuErr error
	run(func() { f.Host, _ = host.Probe(ctx, root) }) // best-effort, never fails
	run(func() {
		vis, err := gpu.Visible(root)
		if gpu.Restricted(err) {
			gpuErr = err // the restriction names a GPU this host does not have
			return
		}
		if vis.GPUs != nil {
			f.GPUs = vis.GPUs
		}
		f.Restriction = vis.Restriction.Note()
	})
	run(func() { f.roc = rocm.Probe(ctx, root) })
	wg.Wait()
	if gpuErr != nil {
		return nil, gpuErr
	}
	set := recipe.Load()
	f.Recipes = recipe.List(set, recipe.Host{
		GPUs: f.GPUs, RAMBytes: f.Host.RAMBytes, DiskFreeBytes: host.FreeBytes(f.HFHome),
		Cached: func(model string) bool { return setup.ModelCached(f.HFHome, model) },
	}, true)
	for _, e := range f.Recipes.Recipes {
		if e.Fit.Usable() {
			f.Fits = append(f.Fits, e.ID)
		}
	}
	f.Best = recipe.Best(f.Recipes)
	f.Recipe = pick(f.Recipes, o.Sets)
	if f.Recipe != "" {
		if r, err := set.Resolve(f.Recipe); err == nil {
			f.Model = modelRepo(r.Model.Source)
		}
	}
	f.Mode = mode(f.Host, cfg, o, set, f.Recipe)
	run(func() {
		f.Doctor = doctor.Run(ctx, doctor.Options{
			Root: root, Venv: f.Venv, VenvSource: f.VenvSource, VenvCandidates: f.VenvCandidates,
			HFHome:         f.HFHome,
			ValidatedArchs: recipe.ValidatedArchs(set), RecipeEnv: recipeEnv(set, f.Recipe),
			Host: &f.Host, ROCm: &f.roc,
		})
	})
	run(func() {
		f.Setup = setup.Plan(ctx, f.Host, f.roc, setup.Options{
			Mode: f.Mode, Venv: f.Venv, HFHome: f.HFHome, GPUArchs: archs(f.GPUs), ModelRepo: f.Model,
		})
	})
	run(func() { f.Runs = runs(ctx) })
	wg.Wait()
	f.Jobs = RecentJobs()
	return f, nil
}

// pick returns the recipe to serve: the best fitting recipe whose required
// inputs are all satisfied. When none is, the best fitting recipe, so the
// onboarding can name the input that is missing.
func pick(l recipe.Listing, sets []string) string {
	var ready recipe.Listing
	for _, e := range l.Recipes {
		if !e.Fit.Usable() {
			continue
		}
		if missingInput(e, sets) == "" {
			ready.Recipes = append(ready.Recipes, e)
		}
	}
	if id := recipe.Best(ready); id != "" {
		return id
	}
	return recipe.Best(l)
}

// missingInput returns the name of the first required input of a recipe that
// has no value, or "" when the recipe is ready to serve.
func missingInput(e recipe.ListEntry, sets []string) string {
	for name, in := range e.Inputs {
		if !in.Required || in.Default != "" {
			continue
		}
		if hasSet(sets, name) {
			continue
		}
		return name
	}
	return ""
}

// hasSet reports whether a --set gives a value to name.
func hasSet(sets []string, name string) bool {
	for _, s := range sets {
		if strings.HasPrefix(s, name+"=") {
			return true
		}
	}
	return false
}

// modelRepo returns the Hugging Face repo of a recipe, or "" when the recipe
// serves a local path.
func modelRepo(source string) string {
	if source == "" || strings.HasPrefix(source, "/") || strings.HasPrefix(source, "${") {
		return ""
	}
	return source
}

// mode returns the mode the recipe would run in: the flag, then the config,
// then container when the recipe has an image and an engine works, else host.
func mode(h host.Info, cfg config.Config, o Options, set recipe.Set, recipeID string) string {
	if o.Mode != "" {
		return o.Mode
	}
	if cfg.Mode != "" {
		return cfg.Mode
	}
	r, err := set.Resolve(recipeID)
	if err != nil {
		return recipe.ModeHost
	}
	if slices.Contains(recipe.Modes(r), recipe.ModeContainer) && engineUsable(h, firstNonEmpty(o.Engine, cfg.Engine)) {
		return recipe.ModeContainer
	}
	return recipe.ModeHost
}

// engineUsable reports whether a container engine is usable. want names the
// engine the operator asked for.
func engineUsable(h host.Info, want string) bool {
	for _, e := range h.Engines {
		if e.Usable && (want == "" || e.Name == want) {
			return true
		}
	}
	return false
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// archs returns the gfx targets of the GPUs.
func archs(gpus []gpu.GPU) []string {
	var out []string
	for _, g := range gpus {
		out = append(out, g.GFX)
	}
	return out
}

// runs lists the runs of this host with their state and phase.
func runs(ctx context.Context) []Run {
	list, _ := runner.List(ctx)
	out := []Run{}
	for _, r := range list {
		run := Run{Name: r.Name, Recipe: r.Recipe, State: r.State, Mode: r.Mode, URL: r.URL()}
		if r.State != runner.StateExited {
			if ph, ok := runner.Phase(ctx, r.Record); ok {
				run.Phase = ph.Sentence
			}
		}
		out = append(out, run)
	}
	return out
}

// live returns the run that is not exited, or nil.
func (f *Facts) live() *Run {
	for i := range f.Runs {
		if f.Runs[i].State != runner.StateExited {
			return &f.Runs[i]
		}
	}
	return nil
}

// summary of the GPUs, for the hardware step.
func (f *Facts) gpuSummary() string {
	counts := map[string]int{}
	var order []string
	for _, g := range f.GPUs {
		k := fmt.Sprintf("%s %s", g.GFX, g.Name)
		if counts[k] == 0 {
			order = append(order, k)
		}
		counts[k]++
	}
	parts := make([]string, 0, len(order))
	for _, k := range order {
		parts = append(parts, fmt.Sprintf("%d× %s", counts[k], k))
	}
	return strings.Join(parts, "; ")
}
