package recipe

import (
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
)

// A 30 GiB card and a recipe of one GPU with the given requires and args.
func tightCase(t *testing.T, requires, args string) (*Recipe, []gpu.GPU) {
	t.Helper()
	y := "schema: 1\nname: t\nstatus: experimental\nmodel: {source: org/model}\nrequires: {arch: [gfx1030], gpus: 1, " + requires + "}\nargs: {" + args + "}\n"
	r, err := Parse("t/tight.yaml", []byte(y))
	if err != nil {
		t.Fatal(err)
	}
	return r, []gpu.GPU{{HIPIndex: 0, GFX: "gfx1030", VRAMBytes: 30 << 30}}
}

func TestCheckFitTight(t *testing.T) {
	cached := func(string) bool { return true }
	tests := []struct {
		name     string
		requires string
		args     string
		disk     uint64
		cached   func(string) bool
		status   string
		reason   string
	}{
		// 32768 tokens of 78356 bytes = 2.4 GiB, inside the 5 GB reserve.
		{"kv fits the reserve", "kv_bytes_per_token: 78356", "max-model-len: 32768, kv-cache-memory-bytes: 5000000000", 0, nil, FitYes, ""},
		{"kv over the reserve", "kv_bytes_per_token: 78356", "max-model-len: 131072, kv-cache-memory-bytes: 5000000000", 0, nil, FitTight,
			"kv: max-model-len 131072 needs 9.6 GiB of KV cache; the budget is 4.7 GiB, about 63811 tokens"},
		// 30 GiB x 0.9 - 21 GB of weights leaves 7.99 GB (7.4 GiB).
		{"kv over the estimate", "kv_bytes_per_token: 78356, download_gb: 21", "max-model-len: 131072", 0, nil, FitTight, "the budget is 7.4 GiB, about 101983 tokens"},
		{"kv inside the estimate", "kv_bytes_per_token: 78356, download_gb: 21", "max-model-len: 32768", 0, nil, FitYes, ""},
		{"no max-model-len skips", "kv_bytes_per_token: 78356", "kv-cache-memory-bytes: 5000000000", 0, nil, FitYes, ""},
		{"no kv data skips", "", "max-model-len: 262144", 0, nil, FitYes, ""},
		{"disk short", "download_gb: 21", "", 12e9, nil, FitTight, "disk: the download needs 21 GB; the model cache has 12 GB free"},
		{"disk short, model cached", "download_gb: 21", "", 12e9, cached, FitYes, ""},
		{"disk enough", "download_gb: 21", "", 100e9, nil, FitYes, ""},
		{"disk unknown skips", "download_gb: 21", "", 0, nil, FitYes, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, gpus := tightCase(t, tt.requires, tt.args)
			f := CheckFit(r, Host{GPUs: gpus, DiskFreeBytes: tt.disk, Cached: tt.cached})
			if f.Status != tt.status {
				t.Errorf("status = %s (%v), want %s", f.Status, f.Reasons, tt.status)
			}
			if tt.reason != "" && !strings.Contains(strings.Join(f.Reasons, "; "), tt.reason) {
				t.Errorf("reasons %v, want %q", f.Reasons, tt.reason)
			}
			if f.Status == FitYes && len(f.Reasons) != 0 {
				t.Errorf("a recipe that fits has reasons %v", f.Reasons)
			}
		})
	}
}

// A recipe the host cannot run stays no: the tight checks only run on a
// recipe that fits.
func TestCheckFitNoWinsOverTight(t *testing.T) {
	r, _ := tightCase(t, "download_gb: 21", "")
	f := CheckFit(r, Host{GPUs: []gpu.GPU{{GFX: "gfx1100", VRAMBytes: 48 << 30}}, DiskFreeBytes: 1e9})
	if f.Status != FitNo || len(f.Reasons) != 1 || !strings.HasPrefix(f.Reasons[0], "arch:") {
		t.Errorf("fit = %s %v, want no with the arch reason only", f.Status, f.Reasons)
	}
}

// The KV budget of a replica counts every rank of it: tensor parallel
// splits the cache over the cards, data parallel repeats it.
func TestCheckKVRanks(t *testing.T) {
	y := "schema: 1\nrequires: {arch: [gfx1030], gpus: 4, kv_bytes_per_token: 1000}\n" +
		"args: {max-model-len: 1000, kv-cache-memory-bytes: 100, tensor-parallel-size: 2, data-parallel-size: 2}\n"
	r, err := Parse("t/kv.yaml", []byte(y))
	if err != nil {
		t.Fatal(err)
	}
	k := CheckKV(r, 0)
	if k == nil || k.BudgetBytes != 200 || k.NeedBytes != 1e6 || k.Source != "kv-cache-memory-bytes" {
		t.Errorf("kv = %+v, want a budget of 2 ranks x 100 bytes", k)
	}
}

func TestBestPrefersFitsOverTight(t *testing.T) {
	l := Listing{Recipes: []ListEntry{
		{ID: "a-tight", Priority: 90, Fit: Fit{Status: FitTight}},
		{ID: "b-fits", Priority: 10, Fit: Fit{Status: FitYes}},
		{ID: "c-no", Priority: 99, Fit: Fit{Status: FitNo}},
	}}
	if got := Best(l); got != "b-fits" {
		t.Errorf("Best = %q, want b-fits", got)
	}
	l.Recipes = l.Recipes[:1]
	if got := Best(l); got != "a-tight" {
		t.Errorf("Best of a tight recipe alone = %q, want a-tight", got)
	}
}
