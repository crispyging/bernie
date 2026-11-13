package vllmtest

import (
	"fmt"
	"net/http"
	"sort"
)

// The metrics of the fake server: the gauges and the counters of a running
// vLLM, and the latency histograms it publishes.

func (s *Server) metrics(w http.ResponseWriter) {
	s.mu.Lock()
	running, gen := s.running, s.genTotal
	prompt, cached, queries, hits := s.prompt, s.cached, s.queries, s.hits
	// vLLM publishes request_success_total from the start, at zero.
	finished := map[string]int{"length": 0, "stop": 0}
	for k, v := range s.finished {
		finished[k] = v
	}
	ttft := append([]float64(nil), s.ttft...)
	itl := append([]float64(nil), s.itl...)
	e2e := append([]float64(nil), s.e2e...)
	queue := append([]float64(nil), s.queue...)
	s.mu.Unlock()
	m := s.cfg.Model
	fmt.Fprintf(w, "# HELP vllm:num_requests_running Number of requests in model execution batches.\n")
	fmt.Fprintf(w, "# TYPE vllm:num_requests_running gauge\n")
	fmt.Fprintf(w, "vllm:num_requests_running{engine=\"0\",model_name=\"%s\"} %d\n", m, running)
	fmt.Fprintf(w, "vllm:num_requests_waiting{engine=\"0\",model_name=\"%s\"} 0\n", m)
	fmt.Fprintf(w, "vllm:num_requests_waiting_by_reason{engine=\"0\",model_name=\"%s\",reason=\"capacity\"} 0\n", m)
	fmt.Fprintf(w, "vllm:kv_cache_usage_perc{engine=\"0\",model_name=\"%s\"} 0.25\n", m)
	fmt.Fprintf(w, "vllm:prompt_tokens_total{engine=\"0\",model_name=\"%s\"} %d\n", m, prompt)
	fmt.Fprintf(w, "vllm:prompt_tokens_cached_total{engine=\"0\",model_name=\"%s\"} %d\n", m, cached)
	fmt.Fprintf(w, "vllm:prompt_tokens_by_source_total{engine=\"0\",model_name=\"%s\",source=\"local_compute\"} %d\n", m, prompt-cached)
	fmt.Fprintf(w, "vllm:prompt_tokens_by_source_total{engine=\"0\",model_name=\"%s\",source=\"local_cache_hit\"} %d\n", m, cached)
	fmt.Fprintf(w, "vllm:prefix_cache_queries_total{engine=\"0\",model_name=\"%s\"} %d\n", m, queries)
	fmt.Fprintf(w, "vllm:prefix_cache_hits_total{engine=\"0\",model_name=\"%s\"} %d\n", m, hits)
	fmt.Fprintf(w, "vllm:external_prefix_cache_queries_total{engine=\"0\",model_name=\"%s\"} 0\n", m)
	fmt.Fprintf(w, "vllm:external_prefix_cache_hits_total{engine=\"0\",model_name=\"%s\"} 0\n", m)
	fmt.Fprintf(w, "vllm:num_preemptions_total{engine=\"0\",model_name=\"%s\"} 0\n", m)
	fmt.Fprintf(w, "vllm:generation_tokens_total{engine=\"0\",model_name=\"%s\"} %d\n", m, gen)
	for _, reason := range sortedKeys(finished) {
		fmt.Fprintf(w, "vllm:request_success_total{engine=\"0\",finished_reason=\"%s\",model_name=\"%s\"} %d\n", reason, m, finished[reason])
	}
	// The per-request counters vLLM publishes since 0.10: their counts are
	// the finished requests, their sums the tokens and the seconds.
	done := 0
	for _, n := range finished {
		done += n
	}
	var prefillS, e2eS float64
	for _, v := range ttft {
		prefillS += v
	}
	for _, v := range e2e {
		e2eS += v
	}
	fmt.Fprintf(w, "process_start_time_seconds 1.7e+09\n")
	for _, c := range []struct {
		name string
		sum  float64
	}{
		{"request_prompt_tokens", float64(prompt)},
		{"request_generation_tokens", float64(gen)},
		{"request_prefill_kv_computed_tokens", float64(prompt - cached)},
		{"request_prefill_time_seconds", prefillS},
		{"request_decode_time_seconds", max(0, e2eS-prefillS)},
	} {
		fmt.Fprintf(w, "vllm:%s_count{engine=\"0\",model_name=\"%s\"} %d\n", c.name, m, done)
		fmt.Fprintf(w, "vllm:%s_sum{engine=\"0\",model_name=\"%s\"} %g\n", c.name, m, c.sum)
	}
	writeHistogram(w, "vllm:time_to_first_token_seconds", m, ttft)
	writeHistogram(w, "vllm:inter_token_latency_seconds", m, itl)
	writeHistogram(w, "vllm:e2e_request_latency_seconds", m, e2e)
	writeHistogram(w, "vllm:request_queue_time_seconds", m, queue)
}

// sortedKeys returns the keys of a map in order, so the fake writes the
// same page every time.
func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// latencyBuckets are the bucket bounds of the vLLM latency histograms.
var latencyBuckets = []float64{0.001, 0.005, 0.01, 0.02, 0.04, 0.06, 0.08, 0.1, 0.25, 0.5, 0.75,
	1, 2.5, 5, 7.5, 10, 20, 40, 80, 160, 640, 2560}

// writeHistogram writes a Prometheus histogram of the observations, with the
// cumulative counts vLLM publishes.
func writeHistogram(w http.ResponseWriter, name, model string, obs []float64) {
	fmt.Fprintf(w, "# HELP %s Latency in seconds.\n", name)
	fmt.Fprintf(w, "# TYPE %s histogram\n", name)
	var sum float64
	for _, o := range obs {
		sum += o
	}
	for _, b := range latencyBuckets {
		var n int
		for _, o := range obs {
			if o <= b {
				n++
			}
		}
		fmt.Fprintf(w, "%s_bucket{engine=\"0\",model_name=\"%s\",le=\"%g\"} %d\n", name, model, b, n)
	}
	fmt.Fprintf(w, "%s_bucket{engine=\"0\",model_name=\"%s\",le=\"+Inf\"} %d\n", name, model, len(obs))
	fmt.Fprintf(w, "%s_sum{engine=\"0\",model_name=\"%s\"} %g\n", name, model, sum)
	fmt.Fprintf(w, "%s_count{engine=\"0\",model_name=\"%s\"} %d\n", name, model, len(obs))
}
