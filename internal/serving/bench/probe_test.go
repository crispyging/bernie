package bench

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/vllm/vllmtest"
)

// TestMeasureProbe checks the rate of a probe wave and the reasons a wave
// has no rate.
func TestMeasureProbe(t *testing.T) {
	ok := func(prompt, cached int) sample {
		return sample{promptTokens: prompt, cachedTokens: cached, completionTokens: 1,
			chunks: []chunk{{at: 200 * time.Millisecond, cum: 1}}}
	}
	p := measureProbe([]sample{ok(4000, 0), ok(4000, 1000)}, 2*time.Second)
	if p.Unavailable != "" || p.AggTokS != 3500 || p.Requests != 2 || p.Completed != 2 {
		t.Errorf("probe = %+v, want 7000 computed tokens / 2 s = 3500 tok/s", p)
	}
	if p.TTFTs == nil || p.TTFTs.P50 != 0.2 {
		t.Errorf("probe ttft = %+v, want p50 0.2 s", p.TTFTs)
	}
	cases := []struct {
		name    string
		samples []sample
		want    string
	}{
		{"failed", []sample{ok(4000, 0), {err: errors.New("HTTP 500: boom")}}, "1 of 2 probe requests failed: HTTP 500: boom"},
		{"no usage", []sample{ok(4000, 0), ok(0, 0)}, "no token usage"},
		{"all cached", []sample{ok(4000, 4000)}, "no prompt tokens"},
		{"empty", nil, "no request"},
	}
	for _, c := range cases {
		p := measureProbe(c.samples, time.Second)
		if p.AggTokS != 0 || !strings.Contains(p.Unavailable, c.want) {
			t.Errorf("%s: probe = %+v, want no rate and %q", c.name, p, c.want)
		}
	}
}

// TestRunSendsTheProbe checks that a run with the probe sends one wave of
// max_tokens=1 requests per cell before the measured requests, and that the
// summary takes its prefill from the probe.
func TestRunSendsTheProbe(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	fake := vllmtest.New(vllmtest.Config{TTFT: 20 * ms, ChunkDelay: 2 * ms})
	srv := httptest.NewServer(fake)
	defer srv.Close()

	var progress strings.Builder
	r, err := Run(context.Background(), Options{
		URL: srv.URL, InputLen: 200, OutputLen: 8, Concurrency: []int{1, 3}, NumPrompts: 4, Probe: true,
	}, &progress)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range r.Cells {
		p := c.Probe
		if p == nil || p.Requests != c.Concurrency || p.Completed != c.Concurrency || p.Unavailable != "" {
			t.Fatalf("c=%d: probe = %+v, want %d completed requests", c.Concurrency, p, c.Concurrency)
		}
		// The fake publishes vLLM's per-request counters, so the rate is the
		// prompt tokens the server computed over the wall time of the wave.
		if p.Server == nil || p.Source != "vllm_metrics" || p.ServerUnavailable != "" {
			t.Fatalf("c=%d: probe server %+v source %q: %s", c.Concurrency, p.Server, p.Source, p.ServerUnavailable)
		}
		if want := float64(p.Server.ComputedPromptTokens) / p.WallS; p.AggTokS != want || p.AggTokS <= 0 {
			t.Errorf("c=%d: probe rate %v, want %v", c.Concurrency, p.AggTokS, want)
		}
		if b := c.ServerBatch; b == nil || b.Requests != 4 || b.PrefillTokS <= 0 || c.ServerBatchUnavailable != "" {
			t.Errorf("c=%d: server batch %+v: %s", c.Concurrency, b, c.ServerBatchUnavailable)
		}
		if c.MeanOutputTokens != 8 {
			t.Errorf("c=%d: the measured requests answered %v tokens, want 8: the probe leaked into the cell", c.Concurrency, c.MeanOutputTokens)
		}
	}
	// 1 + 3 probe requests and 4 + 4 measured ones; no warm-up.
	if n := len(fake.Prompts()); n != 12 {
		t.Errorf("the server saw %d prompts, want 12", n)
	}
	s := headline(r)
	if s.SinglePrefillSource != "probe" || s.SinglePrefillTokS != r.Cells[0].Probe.AggTokS {
		t.Errorf("summary prefill = %v from %q, want the probe of c1", s.SinglePrefillTokS, s.SinglePrefillSource)
	}
	if !strings.Contains(progress.String(), "prefill probe:") {
		t.Errorf("progress lacks the probe line:\n%s", progress.String())
	}
}
