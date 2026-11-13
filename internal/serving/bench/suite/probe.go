package suite

import "github.com/opengfx1030/bernie/internal/serving/bench"

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
