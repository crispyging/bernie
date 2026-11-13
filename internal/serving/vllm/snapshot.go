package vllm

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

// snapshotCounts are the per-request histograms whose count and sum a
// snapshot keeps.
var snapshotCounts = []string{"request_prompt_tokens", "request_generation_tokens",
	"request_prefill_time_seconds", "request_decode_time_seconds", "request_prefill_kv_computed_tokens"}

// snapshotKeys are the vllm: metrics, without the prefix, a snapshot needs.
var snapshotKeys = func() map[string]bool {
	m := map[string]bool{"request_success_total": true, "num_requests_running": true, "num_requests_waiting": true}
	for _, n := range snapshotCounts {
		m[n+"_count"], m[n+"_sum"] = true, true
	}
	return m
}()

// Snapshot is the request counters of one model of a vLLM server at one
// moment, the port of BroodBench's metrics_capture.parse_snapshot. It is
// what two moments are compared on.
type Snapshot struct {
	Values       map[string]float64 `json:"values"`
	Engine       string             `json:"engine"`
	ProcessStart float64            `json:"process_start"`
}

// SnapshotOf picks the counters of model out of parsed metrics. It is nil
// when a counter is missing or negative, when the model runs on more than
// one engine, or when the server does not publish its start time: then no
// interval can be attributed.
func SnapshotOf(m map[string]float64, model string) *Snapshot {
	start, ok := m["process_start_time_seconds"]
	if !ok || m == nil {
		return nil
	}
	s := &Snapshot{Values: map[string]float64{}, ProcessStart: start}
	engines := map[string]bool{}
	for key, v := range m {
		name, labels := splitMetric(key)
		short, ok := strings.CutPrefix(name, "vllm:")
		if !ok || !snapshotKeys[short] {
			continue
		}
		if mn, _ := labelValue(labels, "model_name"); mn != model {
			continue
		}
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return nil
		}
		eng, _ := labelValue(labels, "engine")
		engines[eng] = true
		s.Values[short] += v
	}
	if len(engines) != 1 || len(s.Values) != len(snapshotKeys) {
		return nil
	}
	for e := range engines {
		s.Engine = e
	}
	return s
}

// Attribution is what the server's own counters say about a set of
// requests: the prefill rate of each request (computed prompt tokens over
// the prefill seconds the server spent on them) and the decode rate.
type Attribution struct {
	Requests             int                `json:"requests"`
	PrefillTokS          float64            `json:"prefill_tok_s"`
	PrefillS             float64            `json:"prefill_s"`
	DecodeTokS           float64            `json:"decode_tok_s,omitempty"`
	DecodeS              float64            `json:"decode_s"`
	ComputedPromptTokens int                `json:"computed_prompt_tokens"`
	CachedTokens         int                `json:"cached_tokens"`
	Source               string             `json:"source"`
	Delta                map[string]float64 `json:"raw_metrics_delta"`
}

// Attribute credits the difference of two snapshots to exactly requests
// requests with these usage totals, or says why it cannot (the port of
// BroodBench's batch_measurement and isolated_measurement). The server must
// be idle at both moments, the same process and engine, and its counts and
// token sums must equal the client's exactly: anything else means another
// request was in the interval.
func Attribute(before, after *Snapshot, requests, prompt, output int) (*Attribution, string) {
	switch {
	case before == nil || after == nil:
		return nil, "server metrics unavailable (the endpoint exposes no parseable vLLM /metrics)"
	case before.Engine != after.Engine || before.ProcessStart != after.ProcessStart:
		return nil, "the engine restarted between the snapshots"
	}
	for _, s := range []*Snapshot{before, after} {
		if s.Values["num_requests_running"] != 0 || s.Values["num_requests_waiting"] != 0 {
			return nil, "the endpoint was not idle at a boundary (other traffic)"
		}
	}
	delta := map[string]float64{}
	for k := range snapshotKeys {
		if strings.HasPrefix(k, "num_requests_") {
			continue
		}
		delta[k] = after.Values[k] - before.Values[k]
		if delta[k] < 0 {
			return nil, "a counter was reset between the snapshots"
		}
	}
	n := float64(requests)
	if delta["request_success_total"] != n {
		return nil, fmt.Sprintf("completed-request count mismatch (server %g, ours %d): foreign traffic or failed requests", delta["request_success_total"], requests)
	}
	for _, c := range snapshotCounts {
		if delta[c+"_count"] != n {
			return nil, fmt.Sprintf("completed-request count mismatch in %s (server %g, ours %d)", c, delta[c+"_count"], requests)
		}
	}
	if delta["request_prompt_tokens_sum"] != float64(prompt) || delta["request_generation_tokens_sum"] != float64(output) {
		return nil, "the server's token totals do not match the client's usage"
	}
	computed, pre, dec := delta["request_prefill_kv_computed_tokens_sum"], delta["request_prefill_time_seconds_sum"], delta["request_decode_time_seconds_sum"]
	if computed <= 0 || computed > float64(prompt) || pre <= 0 {
		return nil, "the server recorded no computed prefill tokens or no prefill time"
	}
	a := &Attribution{Requests: requests, PrefillTokS: computed / pre, PrefillS: pre, DecodeS: dec,
		ComputedPromptTokens: int(computed), CachedTokens: prompt - int(computed), Source: "vllm_metrics", Delta: delta}
	if decodeTokens := output - requests; dec > 0 && decodeTokens > 0 {
		a.DecodeTokS = float64(decodeTokens) / dec
	}
	return a, ""
}

// IdleWait is how long WaitIdle waits for the server to settle.
const IdleWait = 15 * time.Second

// WaitIdle reads the metrics until model has no running or waiting request
// and, when since is not nil, until the server counted expected more
// completed requests than since. It returns the last metrics it read, or
// nil when the server has none. It gives up after limit.
func WaitIdle(ctx context.Context, c *Client, model string, since *Snapshot, expected int, limit time.Duration) map[string]float64 {
	deadline := time.Now().Add(limit)
	for {
		rctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		m, err := c.Metrics(rctx)
		cancel()
		if err != nil {
			return nil
		}
		s := SnapshotOf(m, model)
		if s == nil {
			return m
		}
		idle := s.Values["num_requests_running"] == 0 && s.Values["num_requests_waiting"] == 0
		counted := since == nil || s.Values["request_success_total"]-since.Values["request_success_total"] >= float64(expected)
		if (idle && counted) || time.Now().After(deadline) {
			return m
		}
		select {
		case <-ctx.Done():
			return m
		case <-time.After(200 * time.Millisecond):
		}
	}
}
