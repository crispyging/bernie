package recipes_test

import (
	"io/fs"
	"path"
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/topo"
	"github.com/opengfx1030/bernie/internal/serving/recipe"
	"github.com/opengfx1030/bernie/internal/serving/tuning"
	"github.com/opengfx1030/bernie/internal/support/fixture"
	"github.com/opengfx1030/bernie/recipes"
)

// The 11 recipes ported from opengfx1030/vllm-rdna tools/rdna/recipes.
var ported = []string{
	"27b-awq", "27b-exl3", "dsv4-flash", "flashnext", "flashnext-dflash2",
	"flashnext-exl3", "flashnext-mtp0", "flashnext-mtp2", "flashnext-pp3",
	"flashnext-pp3-mtp2", "full",
}

func host(t *testing.T) ([]gpu.GPU, *topo.Topology) {
	t.Helper()
	root := fixture.Root(t, "par1-cs25")
	gpus, err := gpu.List(root)
	if err != nil {
		t.Fatal(err)
	}
	tp, err := topo.Discover(root, gpus)
	if err != nil {
		t.Fatal(err)
	}
	return gpus, &tp
}

// requiredSets gives every required input a value.
func requiredSets(r *recipe.Recipe) []string {
	var sets []string
	for name, in := range r.Inputs {
		if in.Required {
			sets = append(sets, name+"=/data/"+strings.ToLower(name))
		}
	}
	return sets
}

// TestSingleGPURecipeFitsAndIsRecommended checks the host selection that
// `up` uses: one V620 fits the one-card recipe, and a host without a GPU
// cannot select it.
func TestSingleGPURecipeFitsAndIsRecommended(t *testing.T) {
	const id = "27b-awq-1gpu"
	s := recipe.Load()
	r, err := s.Resolve(id)
	if err != nil {
		t.Fatal(err)
	}
	if r.Requires.GPUs != 1 || r.Requires.VRAMGiB != 28 || r.Requires.RAMGiB != 32 {
		t.Fatalf("requirements = %+v", r.Requires)
	}
	gpus, _ := host(t)
	v620 := gpus[2]
	if v620.GFX != "gfx1030" || v620.VRAMBytes < 28<<30 {
		t.Fatalf("fixture GPU 2 = %+v, want a V620 with at least 28 GiB", v620)
	}
	const ramBytes = 64 << 30
	fit := recipe.CheckFit(r, recipe.Host{GPUs: []gpu.GPU{v620}, RAMBytes: ramBytes})
	if fit.Status != recipe.FitYes {
		t.Fatalf("one-V620 fit = %s: %v", fit.Status, fit.Reasons)
	}
	noGPU := recipe.CheckFit(r, recipe.Host{GPUs: []gpu.GPU{}, RAMBytes: ramBytes})
	if noGPU.Status != recipe.FitNo {
		t.Fatalf("zero-GPU fit = %s: %v", noGPU.Status, noGPU.Reasons)
	}

	listing := recipe.List(s, recipe.Host{GPUs: []gpu.GPU{v620}, RAMBytes: ramBytes}, false)
	if got := recipe.Best(listing); got != id {
		t.Fatalf("up recommendation = %q, want %q", got, id)
	}
	listing = recipe.List(s, recipe.Host{GPUs: []gpu.GPU{}, RAMBytes: ramBytes}, false)
	if got := recipe.Best(listing); got != "" {
		t.Fatalf("up recommendation without a GPU = %q, want none", got)
	}
}

func TestBuiltinsLoadResolveValidatePlan(t *testing.T) {
	s := recipe.Load()
	for _, e := range s.Errors {
		if strings.HasPrefix(e.Source, "builtin:") {
			t.Errorf("load %s: %s", e.Source, e.Err)
		}
	}
	ids := s.IDs(false)
	for _, id := range ported {
		if _, ok := s.Recipes[id]; !ok {
			t.Errorf("ported recipe %s is missing", id)
		}
	}
	profiles, err := tuning.Profiles()
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, p := range profiles {
		known[p.Name] = true
	}
	gpus, tp := host(t)
	for _, id := range s.IDs(true) {
		r, err := s.Resolve(id)
		if err != nil {
			t.Errorf("resolve %s: %v", id, err)
			continue
		}
		if errs := recipe.Validate(r); len(errs) > 0 {
			t.Errorf("validate %s: %v", id, errs)
		}
	}
	for _, id := range ids {
		r, err := s.Resolve(id)
		if err != nil {
			continue
		}
		if r.Container != nil && !known[r.Container.TunableOpProfile] {
			t.Errorf("%s: tunableop_profile %q is not registered", id, r.Container.TunableOpProfile)
		}
		for _, mode := range recipe.Modes(r) {
			p, err := recipe.BuildPlan(r, recipe.PlanOptions{Mode: mode, Sets: requiredSets(r), GPUs: gpus, Topo: tp})
			if err != nil {
				t.Errorf("plan %s (%s): %v", id, mode, err)
				continue
			}
			if len(p.GPUs) != r.Requires.GPUs {
				t.Errorf("plan %s: selected %v, want %d GPUs", id, p.GPUs, r.Requires.GPUs)
			}
			for _, i := range p.GPUs {
				if gpus[i].GFX != "gfx1030" {
					t.Errorf("plan %s: selected GPU %d is %s", id, i, gpus[i].GFX)
				}
			}
			if r.Requires.GPUs == 1 {
				if !strings.Contains(p.GPUSelectReason, "lowest HIP index") {
					t.Errorf("plan %s: unexpected one-GPU selection: %s", id, p.GPUSelectReason)
				}
			} else if !strings.Contains(p.GPUSelectReason, "PIX") {
				t.Errorf("plan %s: GPUs %v not on one switch: %s", id, p.GPUs, p.GPUSelectReason)
			}
		}
	}
}

// TestFlashnextMTP2MatchesFork pins the plan of flashnext-mtp2 to the fork
// launcher output (PRINT=1 serve_rdna.sh RECIPE=flashnext-mtp2 PLE=int4).
func TestFlashnextMTP2MatchesFork(t *testing.T) {
	s := recipe.Load()
	r, err := s.Resolve("flashnext-mtp2")
	if err != nil {
		t.Fatal(err)
	}
	gpus, tp := host(t)
	p, err := recipe.BuildPlan(r, recipe.PlanOptions{Sets: []string{"PLE_QUANT_DIR=/x"}, GPUs: gpus, Topo: tp, TunedMoE: func(string) (string, error) { return "/data/tuned-moe", nil }})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"wtdcode/Qwen3.8-Flash-Next-AWQ-W4A16",
		"--attention-backend", "RDNA_ATTN",
		"--block-size", "1024",
		"--compilation-config", `{"compile_ranges_endpoints":[],"cudagraph_capture_sizes":[3,6,12,24],"cudagraph_mode":"FULL_AND_PIECEWISE","mode":3}`,
		"--dtype", "float16",
		"--enable-expert-parallel",
		"--enable-prefix-caching",
		"--host", "127.0.0.1",
		"--kv-cache-memory-bytes", "4026531840",
		"--limit-mm-per-prompt", `{"image":1}`,
		"--long-prefill-token-threshold", "0",
		"--mamba-cache-mode", "align",
		"--max-model-len", "262144",
		"--max-num-batched-tokens", "2048",
		"--max-num-seqs", "8",
		"--mm-processor-kwargs", `{"max_pixels":1605632}`,
		"--port", "18096",
		"--prefill-schedule-interval", "1",
		"--served-model-name", "flash-next",
		"--speculative-config", `{"method":"mtp","num_speculative_tokens":2,"use_local_argmax_reduction":true}`,
		"--tensor-parallel-size", "4",
	}
	if strings.Join(p.Args, "\n") != strings.Join(want, "\n") {
		t.Errorf("args:\n got %q\nwant %q", p.Args, want)
	}
	wantEnv := map[string]string{
		"GPU_MAX_HW_QUEUES": "4", "HSA_ENABLE_SDMA": "0", "HSA_FORCE_FINE_GRAIN_PCIE": "1",
		"OMP_NUM_THREADS": "4", "PYTHONFAULTHANDLER": "1", "ROCPROFILER_REGISTER_ENABLED": "0",
		"TOKENIZERS_PARALLELISM": "false", "TORCH_BLAS_PREFER_HIPBLASLT": "0",
		"VLLM_BATCH_INVARIANT": "0", "VLLM_CAUSAL_CONV1D_RDNA2_FWD": "0",
		"VLLM_CAUSAL_CONV1D_RDNA2_UPDATE": "0", "VLLM_ENABLE_STARTUP_PLAN": "0",
		"VLLM_FA_RDNA2_GQA_DECODE": "1", "VLLM_GDN_HIP_PREFILL": "1",
		"VLLM_PLE_QUANT_DIR": "/x", "VLLM_RDNA_AR": "1", "VLLM_RDNA_AR_BLOCKS": "0",
		"VLLM_RDNA_AR_MAX_KB": "64", "VLLM_RDNA_AR_ONESHOT_KB": "64", "VLLM_RDNA_AR_PACE": "0",
		"VLLM_RDNA_DENSE_GEMV": "0", "VLLM_RDNA_DENSE_INT8": "0", "VLLM_RDNA_DENSE_INT8_ONLY": "0",
		"VLLM_RDNA_FUSED_SE": "1", "VLLM_ROCM_USE_AITER": "0", "VLLM_ROCM_USE_AITER_MOE": "0",
		"VLLM_TUNED_CONFIG_FOLDER": "/data/tuned-moe", "VLLM_USE_RDNA2_FA": "1",
		"VLLM_USE_V2_MODEL_RUNNER": "1", "VLLM_WORKER_MULTIPROC_METHOD": "spawn",
		"HF_HUB_OFFLINE": "0", "TRANSFORMERS_OFFLINE": "0", "HIP_VISIBLE_DEVICES": "2,3,4,5",
	}
	for k, v := range wantEnv {
		if p.Env[k] != v {
			t.Errorf("env %s = %q, want %q", k, p.Env[k], v)
		}
	}
	for k := range p.Env {
		if _, ok := wantEnv[k]; !ok {
			t.Errorf("unexpected env %s=%s", k, p.Env[k])
		}
	}
	if strings.Join(p.Unset, ",") != "FLASH_ATTENTION_TRITON_AMD_ENABLE" {
		t.Errorf("unset = %v", p.Unset)
	}
	if p.Entrypoint != "api" || p.Port != 18096 || p.Host != "127.0.0.1" {
		t.Errorf("entrypoint/host/port = %s %s %d", p.Entrypoint, p.Host, p.Port)
	}
}

func TestHostOnlyRecipesRefuseContainerMode(t *testing.T) {
	s := recipe.Load()
	for _, id := range []string{"flashnext-pp3", "flashnext-pp3-mtp2", "flashnext-dflash2", "flashnext-exl3"} {
		r, err := s.Resolve(id)
		if err != nil {
			t.Fatal(err)
		}
		_, err = recipe.BuildPlan(r, recipe.PlanOptions{Mode: "container", Sets: requiredSets(r)})
		if err == nil || !strings.Contains(err.Error(), "use --mode host") {
			t.Errorf("%s in container mode: err = %v", id, err)
		}
	}
}

// TestTreeLayout enforces spec §2.1 on the recipe tree: every recipe file is
// a *.yaml, the tree is grouped by role (a family folder at the top, a
// concern folder under _fragments) and the ids of the tree are unique.
func TestTreeLayout(t *testing.T) {
	err := fs.WalkDir(recipes.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			if path.Ext(p) != ".yaml" {
				t.Errorf("recipes/%s is not a recipe (*.yaml)", p)
			}
			return nil
		}
		es, err := fs.ReadDir(recipes.FS, p)
		if err != nil {
			return err
		}
		for _, e := range es {
			if e.IsDir() {
				continue
			}
			switch {
			case p == ".":
				t.Errorf("recipes/%s: the top level holds only family folders", e.Name())
			case p == "_fragments":
				t.Errorf("recipes/%s: _fragments holds only concern folders", e.Name())
			case path.Dir(p) == "_fragments" && path.Ext(e.Name()) != ".yaml":
				t.Errorf("recipes/%s: a concern folder holds only recipes", e.Name())
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range recipe.Load().Errors {
		if strings.HasPrefix(e.Source, "builtin:") {
			t.Errorf("%s: %s", e.Source, e.Err)
		}
	}
}

// TestFragmentsLiveUnderFragments keeps the layout rule: a fragment is under
// _fragments/ or next to the recipes of its family; a recipe never sits there.
func TestFragmentsLiveUnderFragments(t *testing.T) {
	s := recipe.Load()
	for id, r := range s.Recipes {
		if !strings.HasPrefix(r.Source, "builtin:") {
			continue
		}
		inFragments := strings.HasPrefix(r.Source, "builtin:_fragments/")
		if inFragments && !r.Fragment {
			t.Errorf("%s: a recipe under _fragments/ (%s)", id, r.Source)
		}
		if r.Fragment && !inFragments && path.Dir(r.Source) == "builtin:." {
			t.Errorf("%s: fragment at the top of the tree (%s)", id, r.Source)
		}
	}
}

// TestPlanByArch plans every recipe on the two-architecture host (8x V620
// gfx1030, 2x W7800 gfx1100): it selects only GPUs of a declared arch, and a
// recipe that does not declare gfx1100 refuses when only the W7800s are given.
func TestPlanByArch(t *testing.T) {
	s := recipe.Load()
	gpus, tp := host(t)
	w7800 := gpus[:2]
	for _, g := range w7800 {
		if g.GFX != "gfx1100" {
			t.Fatalf("fixture: GPU %d is %s, want gfx1100", g.HIPIndex, g.GFX)
		}
	}
	for _, id := range s.IDs(false) {
		r, err := s.Resolve(id)
		if err != nil {
			t.Fatal(err)
		}
		_, err = recipe.BuildPlan(r, recipe.PlanOptions{Sets: requiredSets(r), GPUs: w7800, Topo: tp})
		wantRefuse := !declares(r, "gfx1100") || r.Requires.GPUs > len(w7800)
		switch {
		case wantRefuse && err == nil:
			t.Errorf("%s planned on the W7800s (arch %v, %d GPUs)", id, r.Requires.Arch, r.Requires.GPUs)
		case wantRefuse && !strings.Contains(err.Error(), "needs"):
			t.Errorf("%s: unexpected refusal: %v", id, err)
		case !wantRefuse && err != nil:
			t.Errorf("%s on the W7800s: %v", id, err)
		}
	}
}

func declares(r *recipe.Recipe, gfx string) bool {
	return gpu.ArchMatch(r.Requires.Arch, gfx)
}
