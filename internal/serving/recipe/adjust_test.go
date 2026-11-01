package recipe

import (
	"strings"
	"testing"
)

func TestPlanShrinksContext(t *testing.T) {
	const kv = "kv_bytes_per_token: 78356" // 5e9 bytes hold 63811 tokens
	tests := []struct {
		name     string
		args     string
		sets     []string
		wantLen  string // the rendered --max-model-len
		wantAdj  string // "" = no adjustment
		wantWarn string
	}{
		{"fits as written", "max-model-len: 32768, kv-cache-memory-bytes: 5000000000", nil, "32768", "", ""},
		{"shrinks to 32K", "max-model-len: 131072, kv-cache-memory-bytes: 5000000000", nil, "32768", "131072 -> 32768", ""},
		{"skips steps above the length", "max-model-len: 20000, kv-cache-memory-bytes: 1000000000", nil, "8192", "20000 -> 8192", ""},
		{"a set is never changed", "max-model-len: 32768, kv-cache-memory-bytes: 5000000000",
			[]string{"args.max-model-len=131072"}, "131072", "", "kept because an override sets it"},
		{"a set that fits is quiet", "max-model-len: 131072, kv-cache-memory-bytes: 5000000000",
			[]string{"args.max-model-len=8192"}, "8192", "", ""},
		{"a set budget counts", "max-model-len: 131072, kv-cache-memory-bytes: 5000000000",
			[]string{"args.kv-cache-memory-bytes=20000000000"}, "131072", "", ""},
		{"no step fits", "max-model-len: 131072, kv-cache-memory-bytes: 100000000", nil, "131072", "", "even 8192 tokens do not fit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, gpus := tightCase(t, kv, tt.args)
			p, err := BuildPlan(r, PlanOptions{Sets: tt.sets, GPUs: gpus})
			if err != nil {
				t.Fatal(err)
			}
			if got := argValue(p.Args, "--max-model-len"); got != tt.wantLen {
				t.Errorf("--max-model-len = %q, want %s", got, tt.wantLen)
			}
			switch {
			case tt.wantAdj == "" && len(p.Adjustments) > 0:
				t.Errorf("adjustments %v, want none", p.Adjustments)
			case tt.wantAdj != "" && (len(p.Adjustments) != 1 || !strings.Contains(p.Adjustments[0].String(), tt.wantAdj)):
				t.Errorf("adjustments %v, want %q", p.Adjustments, tt.wantAdj)
			case tt.wantAdj != "" && p.Adjustments[0].Reason == "":
				t.Error("an adjustment has no reason")
			}
			warns := strings.Join(p.Warnings, "; ")
			if tt.wantWarn != "" && !strings.Contains(warns, tt.wantWarn) {
				t.Errorf("warnings %q, want %q", warns, tt.wantWarn)
			}
			if tt.wantWarn == "" && strings.Contains(warns, "kv:") {
				t.Errorf("unexpected kv warning %q", warns)
			}
		})
	}
}

// argValue is the value after flag in a rendered argument list.
func argValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
		if v, ok := strings.CutPrefix(a, flag+"="); ok {
			return v
		}
	}
	return ""
}
