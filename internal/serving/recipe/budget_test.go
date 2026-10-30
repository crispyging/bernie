package recipe

import (
	"strings"
	"testing"
)

// budgetRecipe is one V620 recipe with a 32K context. On the 29.98 GiB
// V620 of the fixture, 18 GB of weights at 90% use leave about 8.8 GB of
// KV cache: 33.6K tokens at 256 KiB a token, 16.8K at 512 KiB.
func budgetRecipe(t *testing.T, requires string) *Recipe {
	t.Helper()
	r, err := Parse("t/budget.yaml", []byte(`schema: 1
name: Budget
status: experimental
model: {source: org/model}
requires: {arch: [gfx1030], gpus: 1, `+requires+`}
args: {max-model-len: 32768, tensor-parallel-size: 1}
`))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCheckFitVerdict(t *testing.T) {
	gpus, _ := par1(t)
	cachedRepo := func(string) bool { return true }
	tests := []struct {
		name     string
		requires string
		host     HostFacts
		status   string
		verdict  string
		reason   string
	}{
		{"fits", "download_gb: 18, kv_bytes_per_token: 262144", HostFacts{}, FitYes, FitYes, ""},
		{"no kv data fits", "download_gb: 18", HostFacts{}, FitYes, FitYes, ""},
		{"context shrinks", "download_gb: 18, kv_bytes_per_token: 524288", HostFacts{}, FitYes, FitTight, "lowers max-model-len from 32768 to 16384"},
		{"weights fill the card", "download_gb: 30, kv_bytes_per_token: 262144", HostFacts{}, FitNo, FitNo, "leave no KV cache"},
		{"below the floor", "download_gb: 18, kv_bytes_per_token: 2097152", HostFacts{}, FitNo, FitNo, "the floor is 8192"},
		{"disk too small", "download_gb: 18", HostFacts{DiskFreeBytes: 10e9}, FitNo, FitNo, "disk: the model needs 18 GB"},
		{"disk nearly full", "download_gb: 18", HostFacts{DiskFreeBytes: 19e9}, FitYes, FitTight, "disk: the model needs 18 GB of the 19 GB free"},
		{"cached model needs no disk", "download_gb: 18", HostFacts{DiskFreeBytes: 10e9, Cached: cachedRepo}, FitYes, FitYes, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := CheckFit(budgetRecipe(t, tt.requires), gpus, tt.host)
			if f.Status != tt.status || f.Verdict != tt.verdict {
				t.Fatalf("status %s verdict %s (%v), want %s %s", f.Status, f.Verdict, f.Reasons, tt.status, tt.verdict)
			}
			if (f.Verdict == FitYes) != (len(f.Reasons) == 0) {
				t.Errorf("verdict %s with reasons %v: reasons must be empty exactly when the recipe fits", f.Verdict, f.Reasons)
			}
			if tt.reason != "" && !strings.Contains(strings.Join(f.Reasons, "; "), tt.reason) {
				t.Errorf("reasons %v, want %q", f.Reasons, tt.reason)
			}
		})
	}
}

func TestPlanShrinksContext(t *testing.T) {
	gpus, tp := par1(t)
	r := budgetRecipe(t, "download_gb: 18, kv_bytes_per_token: 524288")
	p, err := BuildPlan(r, PlanOptions{GPUs: gpus, Topo: tp})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Adjustments) != 1 {
		t.Fatalf("adjustments = %+v, want one", p.Adjustments)
	}
	a := p.Adjustments[0]
	if a.Setting != "args.max-model-len" || a.From != "32768" || a.To != "16384" || a.Reason == "" {
		t.Errorf("adjustment = %+v", a)
	}
	if !strings.Contains(strings.Join(p.Args, " "), "--max-model-len 16384") {
		t.Errorf("args %v do not carry the lowered context", p.Args)
	}
	if n, _ := maxModelLen(r.Args); n != 32768 {
		t.Error("the plan changed the recipe it was given")
	}
}

func TestPlanKeepsContext(t *testing.T) {
	gpus, tp := par1(t)
	tests := []struct {
		name, requires string
		sets           []string
		want, warning  string
	}{
		{"set value stays", "download_gb: 18, kv_bytes_per_token: 524288", []string{"args.max-model-len=8192"}, "--max-model-len 8192", ""},
		{"enough KV cache", "download_gb: 18, kv_bytes_per_token: 262144", nil, "--max-model-len 32768", ""},
		{"no KV data", "download_gb: 18", nil, "--max-model-len 32768", ""},
		{"below the floor warns", "download_gb: 18, kv_bytes_per_token: 2097152", nil, "--max-model-len 32768", "below the 8192 floor"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := BuildPlan(budgetRecipe(t, tt.requires), PlanOptions{GPUs: gpus, Topo: tp, Sets: tt.sets})
			if err != nil {
				t.Fatal(err)
			}
			if len(p.Adjustments) != 0 {
				t.Errorf("adjustments = %+v, want none", p.Adjustments)
			}
			if !strings.Contains(strings.Join(p.Args, " "), tt.want) {
				t.Errorf("args %v, want %q", p.Args, tt.want)
			}
			if got := strings.Join(p.Warnings, "; "); tt.warning != "" && !strings.Contains(got, tt.warning) {
				t.Errorf("warnings %q, want %q", got, tt.warning)
			}
		})
	}
}

func TestFitContextSteps(t *testing.T) {
	tests := []struct{ maxLen, tokens, want int }{
		{262144, 200000, 131072},
		{262144, 40000, 32768},
		{32768, 20000, 16384},
		{32768, 9000, 8192},
		{32768, 8000, 0},
		{20480, 17000, 16384},
	}
	for _, tt := range tests {
		if got := fitContext(tt.maxLen, tt.tokens); got != tt.want {
			t.Errorf("fitContext(%d, %d) = %d, want %d", tt.maxLen, tt.tokens, got, tt.want)
		}
	}
}
