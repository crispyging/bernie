package vllm

import (
	"maps"
	"os"
	"strings"
	"testing"
)

func kvSnapshot(t *testing.T) *Snapshot {
	t.Helper()
	b, err := os.ReadFile("testdata/metrics_kv.txt")
	if err != nil {
		t.Fatal(err)
	}
	m, err := ParseMetrics(b)
	if err != nil {
		t.Fatal(err)
	}
	s := SnapshotOf(m, "qwen38-flash-next")
	if s == nil {
		t.Fatal("no snapshot of the live fixture")
	}
	return s
}

// TestSnapshotOfALiveServer checks the counters read from a captured vLLM
// /metrics (fogbox, 2026-11-13).
func TestSnapshotOfALiveServer(t *testing.T) {
	s := kvSnapshot(t)
	if s.Engine != "0" || s.ProcessStart <= 0 || s.Values["request_success_total"] != 242 ||
		s.Values["request_prefill_kv_computed_tokens_count"] != 242 || s.Values["num_requests_running"] != 0 {
		t.Errorf("snapshot %+v", s)
	}
	b, _ := os.ReadFile("testdata/metrics_kv.txt")
	m, _ := ParseMetrics(b)
	if SnapshotOf(m, "other-model") != nil {
		t.Error("a snapshot of a model the server does not serve")
	}
	if b, err := os.ReadFile("testdata/metrics_old.txt"); err == nil {
		old, _ := ParseMetrics(b)
		if SnapshotOf(old, "tiny") != nil {
			t.Error("a snapshot of a server without the per-request counters")
		}
	}
}

// after is a snapshot of n more requests with these sums.
func after(before *Snapshot, n, prompt, output, computed int, preS, decS float64) *Snapshot {
	a := &Snapshot{Values: maps.Clone(before.Values), Engine: before.Engine, ProcessStart: before.ProcessStart}
	a.Values["request_success_total"] += float64(n)
	for _, c := range snapshotCounts {
		a.Values[c+"_count"] += float64(n)
	}
	a.Values["request_prompt_tokens_sum"] += float64(prompt)
	a.Values["request_generation_tokens_sum"] += float64(output)
	a.Values["request_prefill_kv_computed_tokens_sum"] += float64(computed)
	a.Values["request_prefill_time_seconds_sum"] += preS
	a.Values["request_decode_time_seconds_sum"] += decS
	return a
}

// TestAttribute checks the attribution of a wave and every reason it is
// refused.
func TestAttribute(t *testing.T) {
	b := kvSnapshot(t)
	a, why := Attribute(b, after(b, 4, 8000, 1028, 6000, 2, 10), 4, 8000, 1028)
	if a == nil || a.PrefillTokS != 3000 || a.CachedTokens != 2000 || a.DecodeTokS != 102.4 || a.Source != "vllm_metrics" {
		t.Fatalf("attribution %+v (%s)", a, why)
	}
	busy := after(b, 4, 8000, 1028, 6000, 2, 10)
	busy.Values["num_requests_running"] = 1
	restarted := after(b, 4, 8000, 1028, 6000, 2, 10)
	restarted.ProcessStart++
	cases := map[string]struct {
		after          *Snapshot
		n, prompt, out int
		want           string
	}{
		"busy":      {busy, 4, 8000, 1028, "not idle"},
		"restart":   {restarted, 4, 8000, 1028, "restarted"},
		"foreign":   {after(b, 5, 9000, 1100, 7000, 2, 10), 4, 8000, 1028, "count mismatch"},
		"usage":     {after(b, 4, 8000, 1028, 6000, 2, 10), 4, 7999, 1028, "token totals"},
		"no metric": {nil, 4, 8000, 1028, "unavailable"},
	}
	for name, c := range cases {
		if a, why := Attribute(b, c.after, c.n, c.prompt, c.out); a != nil || !strings.Contains(why, c.want) {
			t.Errorf("%s: %+v %q, want %q", name, a, why, c.want)
		}
	}
}
