package suite

import (
	"github.com/opengfx1030/bernie/internal/serving/bench"
	"github.com/opengfx1030/bernie/internal/serving/vllm"
)

// medianProbe merges the prefill probes of the repetitions of a cell. The
// rate is the median of the probes that measured one; the counts are the
// sums over every repetition. When no probe measured a rate, the first
// reason stays, so the cell still says why.
func medianProbe(ms []bench.Measurement) *bench.Probe {
	var out *bench.Probe
	var rates, walls []float64
	var ttfts []*bench.Dist
	for _, m := range ms {
		p := m.Probe
		if p == nil {
			continue
		}
		if out == nil {
			out = &bench.Probe{}
		}
		out.Requests += p.Requests
		out.Completed += p.Completed
		out.PromptTokens += p.PromptTokens
		out.CachedTokens += p.CachedTokens
		walls = append(walls, p.WallS)
		ttfts = append(ttfts, p.TTFTs)
		if p.Unavailable != "" {
			if out.Unavailable == "" {
				out.Unavailable = p.Unavailable
			}
			continue
		}
		rates = append(rates, p.AggTokS)
	}
	if out == nil {
		return nil
	}
	out.WallS = medianOf(walls)
	out.TTFTs = medianDist(ttfts)
	if len(rates) > 0 {
		out.AggTokS = medianOf(rates)
		out.Unavailable = ""
	}
	return out
}

// medianBatch merges the server attributions of the repetitions of a cell:
// the median prefill and decode rates when every repetition has one, else
// the first reason one was refused.
func medianBatch(ms []bench.Measurement) (*vllm.Attribution, string) {
	var pre, dec []float64
	var first *vllm.Attribution
	for _, m := range ms {
		if m.ServerBatch == nil {
			if m.ServerBatchUnavailable != "" {
				return nil, m.ServerBatchUnavailable
			}
			continue
		}
		if first == nil {
			c := *m.ServerBatch
			first = &c
		}
		pre = append(pre, m.ServerBatch.PrefillTokS)
		dec = append(dec, m.ServerBatch.DecodeTokS)
	}
	if first != nil {
		first.PrefillTokS, first.DecodeTokS = medianOf(pre), medianOf(dec)
	}
	return first, ""
}

// mergeExtras merges what the server and the probe said about the
// repetitions of a cell.
func mergeExtras(out *bench.Measurement, sides []*bench.ServerSide, ms []bench.Measurement) {
	out.ServerSide, out.Probe = medianServerSide(sides), medianProbe(ms)
	out.ServerBatch, out.ServerBatchUnavailable = medianBatch(ms)
}
