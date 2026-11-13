package eval

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/vllm"
)

// Call is the telemetry of one chat request of a case (spec §10.5), the
// port of BroodBench's telemetry.from_response. Only what the server
// reported is kept: a rate is absent when the server sent no timing.
type Call struct {
	LatencyS float64 `json:"latency_s"`
	// FinishReason is the reason the server gave; Truncated is true when
	// it was length, content_filter or error.
	FinishReason string `json:"finish_reason,omitempty"`
	Truncated    bool   `json:"truncated,omitempty"`
	// The token counts of the usage of the answer. CachedTokens is
	// usage.prompt_tokens_details.cached_tokens.
	PromptTokens     *int `json:"prompt_tokens"`
	CompletionTokens *int `json:"completion_tokens"`
	CachedTokens     *int `json:"cached_tokens"`
	// EvaluatedPromptTokens is the prompt tokens the server computed, from
	// its timings (llama.cpp's prompt_n) or its counters.
	EvaluatedPromptTokens *int `json:"evaluated_prompt_tokens,omitempty"`
	// The prefill and decode rates and the time they took, with where
	// they came from: server_timings or vllm_isolated_metrics.
	PrefillTokS   float64 `json:"prefill_tok_s,omitempty"`
	PrefillS      float64 `json:"prefill_s,omitempty"`
	PrefillSource string  `json:"prefill_source,omitempty"`
	DecodeTokS    float64 `json:"decode_tok_s,omitempty"`
	DecodeS       float64 `json:"decode_s,omitempty"`
	DecodeSource  string  `json:"decode_source,omitempty"`
	Error         string  `json:"error,omitempty"`
	// MetricsUnavailable says why the counters of a vLLM server could not
	// be credited to this request alone.
	MetricsUnavailable string `json:"metrics_unavailable,omitempty"`
}

// counters reads the request counters of model once the server is idle
// and, after since, has counted this request. It is nil for a server
// without vLLM's per-request counters.
func (c *client) counters(ctx context.Context, model string, since *vllm.Snapshot, limit time.Duration) *vllm.Snapshot {
	if c.vc == nil {
		return nil
	}
	expected := 0
	if since != nil {
		expected = 1
	}
	return vllm.SnapshotOf(vllm.WaitIdle(ctx, c.vc, model, since, expected, limit), model)
}

// credit fills the rates the server did not report in its answer from its
// counters, when they hold for this request alone (the port of
// BroodBench's isolated_measurement): the server idle at both moments,
// one more completed request, and its token sums equal to the usage.
func (c *Call) credit(before, after *vllm.Snapshot) {
	if c.PromptTokens == nil || c.CompletionTokens == nil {
		c.MetricsUnavailable = "the answer carried no usage"
		return
	}
	a, why := vllm.Attribute(before, after, 1, *c.PromptTokens, *c.CompletionTokens)
	if a == nil {
		c.MetricsUnavailable = why
		return
	}
	if c.PrefillSource == "" {
		n := a.ComputedPromptTokens
		c.EvaluatedPromptTokens = &n
		c.PrefillTokS, c.PrefillS, c.PrefillSource = a.PrefillTokS, a.PrefillS, "vllm_isolated_metrics"
	}
	if c.DecodeSource == "" && a.DecodeTokS > 0 {
		c.DecodeTokS, c.DecodeS, c.DecodeSource = a.DecodeTokS, a.DecodeS, "vllm_isolated_metrics"
	}
	if c.CachedTokens == nil {
		cached := a.CachedTokens
		c.CachedTokens = &cached
	}
}

// chatAnswer is the part of a chat completion the telemetry reads.
type chatAnswer struct {
	Choices []struct {
		Message      assistant `json:"message"`
		FinishReason string    `json:"finish_reason"`
	} `json:"choices"`
	Usage   map[string]any `json:"usage"`
	Timings map[string]any `json:"timings"`
	Timing  map[string]any `json:"timing"`
}

// callOf reads the telemetry of an answer. Server timings (prompt_n,
// prompt_ms, predicted_n, predicted_ms) may sit in timings, timing or the
// usage block.
func callOf(a *chatAnswer, latency float64) Call {
	c := Call{LatencyS: latency}
	if len(a.Choices) > 0 {
		c.FinishReason = a.Choices[0].FinishReason
		c.Truncated = slices.Contains([]string{"length", "content_filter", "error"}, c.FinishReason)
	}
	c.PromptTokens, c.CompletionTokens = tokenCount(a.Usage["prompt_tokens"]), tokenCount(a.Usage["completion_tokens"])
	if d, ok := a.Usage["prompt_tokens_details"].(map[string]any); ok {
		c.CachedTokens = tokenCount(d["cached_tokens"])
	}
	t := a.Timings
	if t == nil {
		t = a.Timing
	}
	if t == nil {
		t = map[string]any{}
	}
	for _, k := range []string{"prompt_n", "prompt_ms", "predicted_n", "predicted_ms"} {
		if _, ok := t[k]; !ok {
			if v, ok := a.Usage[k]; ok {
				t[k] = v
			}
		}
	}
	pn, pm, dn, dm := number(t["prompt_n"]), number(t["prompt_ms"]), number(t["predicted_n"]), number(t["predicted_ms"])
	if pn != nil && pm != nil && *pm > 0 {
		n := int(*pn)
		c.EvaluatedPromptTokens = &n
		c.PrefillTokS, c.PrefillS, c.PrefillSource = *pn*1000 / *pm, *pm/1000, "server_timings"
	}
	if dn != nil && dm != nil && *dn > 0 && *dm > 0 {
		n := int(*dn)
		c.CompletionTokens = &n
		c.DecodeTokS, c.DecodeS, c.DecodeSource = *dn*1000 / *dm, *dm/1000, "server_timings"
	}
	return c
}

// Performance is the telemetry of the calls of a model, the port of
// BroodBench's telemetry.aggregate. A rate is the tokens of the measured
// calls over their seconds; it is absent when no call measured it.
type Performance struct {
	Calls                int      `json:"calls"`
	Truncated            int      `json:"truncated"`
	MeanLatencyS         float64  `json:"mean_latency_s"`
	MeasuredPrefillCalls int      `json:"measured_prefill_calls"`
	MeasuredDecodeCalls  int      `json:"measured_decode_calls"`
	PrefillTokS          float64  `json:"prefill_tok_s,omitempty"`
	DecodeTokS           float64  `json:"decode_tok_s,omitempty"`
	PrefillS             float64  `json:"prefill_s"`
	DecodeS              float64  `json:"decode_s"`
	PrefillSources       []string `json:"prefill_sources"`
	DecodeSources        []string `json:"decode_sources"`
}

// aggregate sums the calls of a model.
func aggregate(calls []Call) *Performance {
	p := &Performance{Calls: len(calls), PrefillSources: []string{}, DecodeSources: []string{}}
	var preTokens, decTokens, latency float64
	for _, c := range calls {
		latency += c.LatencyS
		if c.Truncated {
			p.Truncated++
		}
		if c.PrefillSource != "" && c.PrefillS > 0 {
			t := c.EvaluatedPromptTokens
			if t == nil {
				t = c.PromptTokens
			}
			if t != nil {
				p.MeasuredPrefillCalls++
				preTokens += float64(*t)
				p.PrefillS += c.PrefillS
				p.PrefillSources = addSource(p.PrefillSources, c.PrefillSource)
			}
		}
		if c.DecodeSource != "" && c.DecodeS > 0 && c.CompletionTokens != nil {
			p.MeasuredDecodeCalls++
			decTokens += float64(*c.CompletionTokens)
			p.DecodeS += c.DecodeS
			p.DecodeSources = addSource(p.DecodeSources, c.DecodeSource)
		}
	}
	if p.Calls > 0 {
		p.MeanLatencyS = latency / float64(p.Calls)
	}
	if p.PrefillS > 0 {
		p.PrefillTokS = preTokens / p.PrefillS
	}
	if p.DecodeS > 0 {
		p.DecodeTokS = decTokens / p.DecodeS
	}
	return p
}

func addSource(list []string, s string) []string {
	if !slices.Contains(list, s) {
		list = append(list, s)
		slices.Sort(list)
	}
	return list
}

// tokenCount reads a non-negative integer of a JSON document.
func tokenCount(v any) *int {
	f, ok := v.(float64)
	if !ok || f < 0 || f != float64(int(f)) {
		return nil
	}
	n := int(f)
	return &n
}

// number reads a JSON number; a bool or a string is not one.
func number(v any) *float64 {
	switch f := v.(type) {
	case float64:
		return &f
	case json.Number:
		if x, err := f.Float64(); err == nil {
			return &x
		}
	}
	return nil
}
