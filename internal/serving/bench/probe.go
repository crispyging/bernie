package bench

import (
	"context"
	"fmt"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/vllm"
)

// Probe is the prefill probe of a cell (spec §10.1): as many simultaneous
// requests as the cell has in flight, each with max_tokens 1, sent after the
// warm-up and before the measured requests. Its rate is the prefill
// throughput of the whole server at that concurrency, without the decode of
// the other requests in the way. The wall time holds the queueing, the
// transport and the one sampled token, so the rate slightly understates the
// pure prefill.
type Probe struct {
	Requests  int     `json:"requests"`
	Completed int     `json:"completed"`
	WallS     float64 `json:"wall_s,omitempty"`
	// PromptTokens and CachedTokens are the sums of the usage of the
	// probe requests.
	PromptTokens int `json:"prompt_tokens,omitempty"`
	CachedTokens int `json:"cached_tokens,omitempty"`
	// AggTokS is (prompt tokens − cached tokens) ÷ wall time. It is
	// absent when Unavailable says why.
	AggTokS float64 `json:"agg_tok_s,omitempty"`
	// TTFTs is the distribution of the time to first token of the probe
	// requests in seconds.
	TTFTs *Dist `json:"ttft_s,omitempty"`
	// Unavailable says why the probe has no rate.
	Unavailable string `json:"unavailable,omitempty"`
	// Source says where the rate comes from: vllm_metrics (the prompt
	// tokens the server computed, from its counters) or client_usage.
	Source string `json:"source,omitempty"`
	// Server is what the server's counters say about the wave; when they
	// cannot be credited to it, ServerUnavailable says why.
	Server            *vllm.Attribution `json:"server,omitempty"`
	ServerUnavailable string            `json:"server_unavailable,omitempty"`
}

// ProbeTokens is the max_tokens of a probe request.
const ProbeTokens = 1

// runProbe sends one probe wave of n simultaneous requests and measures it.
func runProbe(ctx context.Context, n int, send func(context.Context, time.Time) sample) (*Probe, []sample) {
	start := time.Now()
	samples := runPool(ctx, n, n, start, send)
	return measureProbe(samples, time.Since(start)), samples
}

// measureProbe turns the samples of a probe wave into its rate. A wave with
// a failed request, or without token usage, has no rate: a partial wave
// does not measure the concurrency it claims.
func measureProbe(samples []sample, wall time.Duration) *Probe {
	p := &Probe{Requests: len(samples), WallS: wall.Seconds()}
	var ttft []float64
	var firstErr error
	usage := true
	for _, s := range samples {
		if s.err != nil {
			if firstErr == nil {
				firstErr = s.err
			}
			continue
		}
		p.Completed++
		p.PromptTokens += s.promptTokens
		p.CachedTokens += s.cachedTokens
		if s.promptTokens == 0 {
			usage = false
		}
		if len(s.chunks) > 0 {
			ttft = append(ttft, s.chunks[0].at.Seconds())
		}
	}
	if len(ttft) > 0 {
		d := Summarize(ttft)
		p.TTFTs = &d
	}
	switch {
	case p.Requests == 0:
		p.Unavailable = "the probe sent no request"
	case firstErr != nil:
		p.Unavailable = fmt.Sprintf("%d of %d probe requests failed: %v", p.Requests-p.Completed, p.Requests, firstErr)
	case !usage:
		p.Unavailable = "the server sent no token usage for the probe"
	case p.WallS <= 0 || p.PromptTokens <= p.CachedTokens:
		p.Unavailable = "the probe computed no prompt tokens"
	default:
		p.AggTokS = float64(p.PromptTokens-p.CachedTokens) / p.WallS
	}
	return p
}
