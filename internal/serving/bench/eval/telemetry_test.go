package eval

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/opengfx1030/bernie/internal/serving/vllm"
)

func parseAnswer(t *testing.T, body string) *chatAnswer {
	t.Helper()
	var a chatAnswer
	if err := json.Unmarshal([]byte(body), &a); err != nil {
		t.Fatal(err)
	}
	return &a
}

// TestCallOf checks the telemetry of a vLLM answer and of a llama.cpp
// answer with server timings.
func TestCallOf(t *testing.T) {
	v := callOf(parseAnswer(t, `{"choices":[{"message":{"content":"x"},"finish_reason":"length"}],
		"usage":{"prompt_tokens":900,"completion_tokens":64,"prompt_tokens_details":{"cached_tokens":512}}}`), 1.5)
	if !v.Truncated || v.FinishReason != "length" || *v.PromptTokens != 900 || *v.CompletionTokens != 64 || *v.CachedTokens != 512 {
		t.Errorf("vllm call %+v", v)
	}
	if v.PrefillSource != "" || v.DecodeSource != "" {
		t.Errorf("a rate without server timings: %+v", v)
	}
	l := callOf(parseAnswer(t, `{"choices":[{"message":{"content":"x"},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":1000,"completion_tokens":50},
		"timings":{"prompt_n":800,"prompt_ms":400,"predicted_n":50,"predicted_ms":1000}}`), 2)
	if l.Truncated || l.PrefillTokS != 2000 || l.PrefillS != 0.4 || *l.EvaluatedPromptTokens != 800 || l.DecodeTokS != 50 || l.DecodeSource != "server_timings" {
		t.Errorf("llama.cpp call %+v", l)
	}
}

// TestAggregateWeighsByTime checks that the rate of a model is the tokens of
// its measured calls over their seconds, not the mean of the rates.
func TestAggregateWeighsByTime(t *testing.T) {
	n := func(v int) *int { return &v }
	p := aggregate([]Call{
		{LatencyS: 1, PromptTokens: n(1000), EvaluatedPromptTokens: n(1000), PrefillS: 1, PrefillTokS: 1000, PrefillSource: "server_timings",
			CompletionTokens: n(100), DecodeS: 1, DecodeTokS: 100, DecodeSource: "server_timings"},
		{LatencyS: 3, PromptTokens: n(1000), EvaluatedPromptTokens: n(1000), PrefillS: 3, PrefillTokS: 333, PrefillSource: "vllm_isolated_metrics", Truncated: true},
		{LatencyS: 2},
	})
	if p.Calls != 3 || p.Truncated != 1 || p.MeanLatencyS != 2 || p.MeasuredPrefillCalls != 2 || p.PrefillTokS != 500 ||
		p.MeasuredDecodeCalls != 1 || p.DecodeTokS != 100 || len(p.PrefillSources) != 2 {
		t.Errorf("performance %+v", p)
	}
}

// TestTruncationIsDiagnosed checks that a truncated reply names the cause of
// a failure and leaves a pass alone.
func TestTruncationIsDiagnosed(t *testing.T) {
	want := "42"
	r := CaseResult{Status: StatusFail, Expected: Expected{Answer: &want}, Reply: "4", Calls: []Call{{Truncated: true, FinishReason: "length"}}}
	if d := Diagnose(&r); d.Category != "truncated_response" {
		t.Errorf("category %q", d.Category)
	}
	r.Status, r.Reply = StatusPass, "42"
	if d := Diagnose(&r); d.Category != "passed" || len(d.Limitations) == 0 {
		t.Errorf("a truncated pass: %+v", d)
	}
}

// TestRunRecordsTheCalls checks that every case keeps its calls and the
// summary its performance.
func TestRunRecordsTheCalls(t *testing.T) {
	cases, _ := Generate("smoke", 5, []int{512}, []string{"recall", "agent"})
	srv := oracle(t, cases, 0, nil)
	r, err := Run(context.Background(), Options{URL: srv.URL, Seed: 5, Contexts: []int{512}, Tracks: []string{"recall", "agent"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	for _, c := range r.Cases {
		if len(c.Calls) == 0 || c.PromptTokens == nil {
			t.Errorf("%s: %d calls, prompt tokens %v", c.CaseID, len(c.Calls), c.PromptTokens)
		}
		calls += len(c.Calls)
	}
	if p := r.Summary[0].Performance; p == nil || p.Calls != calls || p.MeanLatencyS < 0 {
		t.Errorf("performance %+v, want %d calls", p, calls)
	}
}

// TestCreditIsolatedCounters checks that the counters of a vLLM server fill
// the rates of one request, and that other traffic leaves them out.
func TestCreditIsolatedCounters(t *testing.T) {
	snap := func(done, prompt, gen, computed int, preS, decS float64, running int) *vllm.Snapshot {
		v := map[string]float64{"request_success_total": float64(done), "num_requests_running": float64(running), "num_requests_waiting": 0,
			"request_prompt_tokens_sum": float64(prompt), "request_generation_tokens_sum": float64(gen),
			"request_prefill_kv_computed_tokens_sum": float64(computed), "request_prefill_time_seconds_sum": preS,
			"request_decode_time_seconds_sum": decS}
		for _, k := range []string{"request_prompt_tokens", "request_generation_tokens", "request_prefill_time_seconds", "request_decode_time_seconds", "request_prefill_kv_computed_tokens"} {
			v[k+"_count"] = float64(done)
		}
		return &vllm.Snapshot{Values: v, Engine: "0", ProcessStart: 1}
	}
	n := func(v int) *int { return &v }
	before := snap(10, 5000, 500, 4000, 2, 5, 0)
	c := Call{PromptTokens: n(1000), CompletionTokens: n(101)}
	c.credit(before, snap(11, 6000, 601, 4800, 2.5, 6, 0))
	if c.PrefillSource != "vllm_isolated_metrics" || c.PrefillTokS != 1600 || *c.EvaluatedPromptTokens != 800 || *c.CachedTokens != 200 ||
		c.DecodeTokS != 100 || c.MetricsUnavailable != "" {
		t.Errorf("credited call %+v", c)
	}
	busy := Call{PromptTokens: n(1000), CompletionTokens: n(101)}
	busy.credit(before, snap(11, 6000, 601, 4800, 2.5, 6, 1))
	if busy.PrefillSource != "" || busy.MetricsUnavailable == "" {
		t.Errorf("a busy server was credited: %+v", busy)
	}
	timed := Call{PromptTokens: n(1000), CompletionTokens: n(101), PrefillTokS: 9, PrefillS: 1, PrefillSource: "server_timings"}
	timed.credit(before, snap(11, 6000, 601, 4800, 2.5, 6, 0))
	if timed.PrefillSource != "server_timings" || timed.PrefillTokS != 9 {
		t.Errorf("the counters replaced the server timings: %+v", timed)
	}
}
