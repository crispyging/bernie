package bench

import (
	"context"
	"fmt"

	"github.com/opengfx1030/bernie/internal/serving/vllm"
)

// settle waits until the server is idle and, when since is set, has counted
// expected more requests, then returns its metrics and the counters of the
// model. Both are nil for a server without metrics; the snapshot is nil
// for a server whose counters cannot be attributed.
func settle(ctx context.Context, c *vllm.Client, model string, since *vllm.Snapshot, expected int) (map[string]float64, *vllm.Snapshot) {
	m := vllm.WaitIdle(ctx, c, model, since, expected, vllm.IdleWait)
	if m == nil {
		return nil, nil
	}
	return m, vllm.SnapshotOf(m, model)
}

// attribute credits the server's counters between two snapshots to the
// samples of a wave (spec §10.1), or says why it cannot.
func attribute(before, after *vllm.Snapshot, samples []sample) (*vllm.Attribution, string) {
	failed, prompt, output := 0, 0, 0
	for _, s := range samples {
		if s.err != nil {
			failed++
			continue
		}
		prompt += s.promptTokens
		output += s.completionTokens
	}
	if failed > 0 {
		return nil, fmt.Sprintf("%d request(s) failed, so the server's counters cannot be credited to the completed ones", failed)
	}
	return vllm.Attribute(before, after, len(samples), prompt, output)
}

// creditProbe attributes the server's counters to a probe wave. When they
// hold, the rate of the probe is the prompt tokens the server computed over
// the wall time of the wave; else it stays the client's usage, with the
// reason.
func (p *Probe) creditProbe(before, after *vllm.Snapshot, samples []sample) {
	if p.AggTokS > 0 {
		p.Source = "client_usage"
	}
	a, why := attribute(before, after, samples)
	if a == nil {
		p.ServerUnavailable = why
		return
	}
	p.Server = a
	if p.WallS > 0 {
		p.AggTokS, p.Source, p.Unavailable = float64(a.ComputedPromptTokens)/p.WallS, "vllm_metrics", ""
	}
}
