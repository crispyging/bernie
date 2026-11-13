package suite

import (
	"sort"

	"github.com/opengfx1030/bernie/internal/serving/bench"
)

// Metric is one number of a cell that two runs can be compared on.
type Metric struct {
	// Name is the key of the metric in the verdict and in the spread.
	Name string
	// Label is the name of the metric for a human, with its unit.
	Label string
	// HigherIsBetter says whether a bigger number is the better one.
	HigherIsBetter bool
	// Value reads the number of a cell.
	Value func(c *bench.Cell) float64
	// RepValue reads the number of one repetition.
	RepValue func(r bench.Rep) float64
}

// Metrics are the numbers a verdict compares. They are the five groups of
// numbers of spec §10.1: the prefill speed, the decode speed, the time to
// first token, the inter-token latency and the output rate.
var Metrics = []Metric{
	{
		Name: "prefill.agg_tok_s", Label: "prefill tok/s", HigherIsBetter: true,
		Value:    func(c *bench.Cell) float64 { return rateValue(c.Prefill) },
		RepValue: func(r bench.Rep) float64 { return rateValue(r.Prefill) },
	},
	{
		Name: "decode.agg_tok_s", Label: "decode tok/s", HigherIsBetter: true,
		Value:    func(c *bench.Cell) float64 { return rateValue(c.Decode) },
		RepValue: func(r bench.Rep) float64 { return rateValue(r.Decode) },
	},
	{
		Name: "ttft_s.p50", Label: "TTFT p50 s", HigherIsBetter: false,
		Value:    func(c *bench.Cell) float64 { return distValue(c.TTFTs, "p50") },
		RepValue: func(r bench.Rep) float64 { return distValue(r.TTFTs, "p50") },
	},
	{
		Name: "itl_ms.p50", Label: "ITL p50 ms", HigherIsBetter: false,
		Value:    func(c *bench.Cell) float64 { return distValue(c.ITLms, "p50") },
		RepValue: func(r bench.Rep) float64 { return distValue(r.ITLms, "p50") },
	},
	{
		Name: "output_tok_s", Label: "output tok/s", HigherIsBetter: true,
		Value:    func(c *bench.Cell) float64 { return c.OutputTokS },
		RepValue: func(r bench.Rep) float64 { return r.OutputTokS },
	},
}

// rateValue is the aggregate rate of a rate group.
func rateValue(r *bench.Rate) float64 {
	if r == nil {
		return 0
	}
	return r.AggTokS
}

// distValue reads one number of a distribution.
func distValue(d *bench.Dist, which string) float64 {
	if d == nil {
		return 0
	}
	switch which {
	case "mean":
		return d.Mean
	case "p50":
		return d.P50
	case "p95":
		return d.P95
	case "min":
		return d.Min
	case "max":
		return d.Max
	}
	return 0
}

// medianOf is the median of the numbers. An even count takes the mean of the
// two middle values.
func medianOf(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

// medianDist is the median of the distributions of the repetitions. A
// repetition without a distribution does not count.
func medianDist(dists []*bench.Dist) *bench.Dist {
	var out *bench.Dist
	for _, which := range []string{"mean", "p50", "p95", "min", "max"} {
		var values []float64
		for _, d := range dists {
			if d == nil {
				continue
			}
			if v := distValue(d, which); v > 0 {
				values = append(values, v)
			}
		}
		if len(values) == 0 {
			continue
		}
		if out == nil {
			out = &bench.Dist{}
		}
		switch which {
		case "mean":
			out.Mean = medianOf(values)
		case "p50":
			out.P50 = medianOf(values)
		case "p95":
			out.P95 = medianOf(values)
		case "min":
			out.Min = medianOf(values)
		case "max":
			out.Max = medianOf(values)
		}
	}
	return out
}

// medianRate is the median of the rate groups of the repetitions.
func medianRate(rates []*bench.Rate) *bench.Rate {
	var out *bench.Rate
	for _, which := range []string{"agg", "window"} {
		var values []float64
		var reqs []*bench.Dist
		for _, r := range rates {
			if r == nil {
				continue
			}
			v := r.AggTokS
			if which == "window" {
				v = r.WindowS
			}
			if v > 0 {
				values = append(values, v)
			}
			if which == "agg" {
				reqs = append(reqs, r.Req)
			}
		}
		if len(values) == 0 {
			continue
		}
		if out == nil {
			out = &bench.Rate{}
		}
		if which == "agg" {
			out.AggTokS = medianOf(values)
			out.Req = medianDist(reqs)
		} else {
			out.WindowS = medianOf(values)
		}
	}
	return out
}

// medianServerSide is the median of the server-side numbers.
func medianServerSide(sides []*bench.ServerSide) *bench.ServerSide {
	var out *bench.ServerSide
	var ttfts, itls []*bench.Dist
	var counts []float64
	for _, s := range sides {
		if s == nil {
			continue
		}
		ttfts = append(ttfts, s.TTFTs)
		itls = append(itls, s.ITLms)
		if s.Count > 0 {
			counts = append(counts, float64(s.Count))
		}
	}
	if len(counts) == 0 {
		return nil
	}
	out = &bench.ServerSide{Count: int(medianOf(counts))}
	out.TTFTs = medianDist(ttfts)
	out.ITLms = medianDist(itls)
	return out
}

// medianCell returns the cell whose numbers are the medians of the
// repetitions. The counts of the cell are the counts of one repetition: the
// repetitions measure the same number of requests, and each one holds its
// own counts.
func medianCell(reps []bench.Cell) bench.Cell {
	if len(reps) == 0 {
		return bench.Cell{}
	}
	first := reps[0]
	out := bench.Cell{
		Concurrency: first.Concurrency,
		NumPrompts:  first.NumPrompts,
		Completed:   first.Completed,
		Failed:      first.Failed,
		Group:       first.Group,
		InputLen:    first.InputLen,
		OutputLen:   first.OutputLen,
	}
	measurements := make([]bench.Measurement, len(reps))
	for i, r := range reps {
		measurements[i] = r.Measurement
	}
	out.Measurement = medianMeasurement(measurements)
	var drifts []float64
	out.SizeValid = true
	for _, r := range reps {
		drifts = append(drifts, r.InputSizePercent)
		if !r.SizeValid {
			out.SizeValid = false
		}
	}
	out.InputSizePercent = medianOf(drifts)
	return out
}

// medianMeasurement is the median of the numbers of the repetitions.
func medianMeasurement(ms []bench.Measurement) bench.Measurement {
	var out bench.Measurement
	var prefills, decodes []*bench.Rate
	var ttfts, itls []*bench.Dist
	var sides []*bench.ServerSide
	for _, m := range ms {
		prefills = append(prefills, m.Prefill)
		decodes = append(decodes, m.Decode)
		ttfts = append(ttfts, m.TTFTs)
		itls = append(itls, m.ITLms)
		sides = append(sides, m.ServerSide)
	}
	out.Prefill = medianRate(prefills)
	out.Decode = medianRate(decodes)
	out.TTFTs = medianDist(ttfts)
	out.ITLms = medianDist(itls)
	mergeExtras(&out, sides, ms)
	out.TokensExact = true
	out.OutputValid = true
	out.StreamIncremental = true
	for _, m := range ms {
		if !m.TokensExact {
			out.TokensExact = false
		}
		if !m.OutputValid {
			out.OutputValid = false
		}
		if !m.StreamIncremental {
			out.StreamIncremental = false
		}
	}
	out.TTFTP50 = medianOf(values(ms, func(m bench.Measurement) float64 { return m.TTFTP50 }))
	out.TTFTP95 = medianOf(values(ms, func(m bench.Measurement) float64 { return m.TTFTP95 }))
	out.ITLP50 = medianOf(values(ms, func(m bench.Measurement) float64 { return m.ITLP50 }))
	out.ITLP95 = medianOf(values(ms, func(m bench.Measurement) float64 { return m.ITLP95 }))
	out.DecodeTokS = medianOf(values(ms, func(m bench.Measurement) float64 { return m.DecodeTokS }))
	out.PrefillTokS = medianOf(values(ms, func(m bench.Measurement) float64 { return m.PrefillTokS }))
	out.OutputTokS = medianOf(values(ms, func(m bench.Measurement) float64 { return m.OutputTokS }))
	out.DurationS = medianOf(values(ms, func(m bench.Measurement) float64 { return m.DurationS }))
	out.MeanPromptTokens = medianOf(values(ms, func(m bench.Measurement) float64 { return m.MeanPromptTokens }))
	out.MeanOutputTokens = medianOf(values(ms, func(m bench.Measurement) float64 { return m.MeanOutputTokens }))
	out.MeanCachedPromptTokens = medianOf(values(ms, func(m bench.Measurement) float64 { return m.MeanCachedPromptTokens }))
	out.TokensPerChunk = medianOf(values(ms, func(m bench.Measurement) float64 { return m.TokensPerChunk }))
	out.WarmupFailed = 0
	for _, m := range ms {
		out.WarmupFailed = max(out.WarmupFailed, m.WarmupFailed)
	}
	// The recall check counts every reply of every repetition.
	var asked, passed int
	for _, m := range ms {
		if m.Recall == nil {
			continue
		}
		asked += m.Recall.Asked
		passed += m.Recall.Passed
	}
	if asked > 0 {
		out.Recall = &bench.Recall{Asked: asked, Passed: passed}
		for _, m := range ms {
			if m.Recall != nil {
				out.Recall.Depth = m.Recall.Depth
				out.Recall.Method = m.Recall.Method
				break
			}
		}
	}
	// The declared-answer check counts every reply of every repetition.
	var checkAsked, checkPassed int
	for _, m := range ms {
		if m.Check == nil {
			continue
		}
		checkAsked += m.Check.Asked
		checkPassed += m.Check.Passed
	}
	if checkAsked > 0 {
		out.Check = &bench.Check{Asked: checkAsked, Passed: checkPassed}
		for _, m := range ms {
			if m.Check != nil {
				out.Check.Method = m.Check.Method
				break
			}
		}
	}
	// The errors of the repetitions are the errors of the cell.
	seen := map[string]bool{}
	for _, m := range ms {
		for _, e := range m.Errors {
			if seen[e] || len(out.Errors) >= maxErrorSamples {
				continue
			}
			seen[e] = true
			out.Errors = append(out.Errors, e)
		}
	}
	return out
}

// maxErrorSamples is how many distinct error messages a cell keeps.
const maxErrorSamples = 5

// values reads one number of every measurement.
func values(ms []bench.Measurement, read func(bench.Measurement) float64) []float64 {
	out := make([]float64, 0, len(ms))
	for _, m := range ms {
		out = append(out, read(m))
	}
	return out
}

// spreadOf is the min/max ratio of the metrics over the repetitions: 1
// means the repetitions agree, a lower number says how far they differ. A
// metric without a number in every repetition is left out.
func spreadOf(reps []bench.Cell) map[string]float64 {
	out := map[string]float64{}
	for _, m := range Metrics {
		var vals []float64
		for _, r := range reps {
			if v := m.RepValue(repOf(r)); v > 0 {
				vals = append(vals, v)
			}
		}
		if len(vals) == 0 {
			continue
		}
		lo, hi := vals[0], vals[0]
		for _, v := range vals {
			lo = min(lo, v)
			hi = max(hi, v)
		}
		if lo <= 0 {
			continue
		}
		out[m.Name] = lo / hi
	}
	return out
}

// repOf is the repetition document of a cell.
func repOf(c bench.Cell) bench.Rep {
	return bench.Rep{
		NumPrompts: c.NumPrompts,
		Completed:  c.Completed,
		Failed:     c.Failed,
		Measurement: bench.Measurement{
			Prefill:                c.Prefill,
			Decode:                 c.Decode,
			TTFTs:                  c.TTFTs,
			ITLms:                  c.ITLms,
			ServerSide:             c.ServerSide,
			Probe:                  c.Probe,
			TokensExact:            c.TokensExact,
			TTFTP50:                c.TTFTP50,
			TTFTP95:                c.TTFTP95,
			ITLP50:                 c.ITLP50,
			ITLP95:                 c.ITLP95,
			DecodeTokS:             c.DecodeTokS,
			PrefillTokS:            c.PrefillTokS,
			OutputTokS:             c.OutputTokS,
			DurationS:              c.DurationS,
			MeanPromptTokens:       c.MeanPromptTokens,
			MeanOutputTokens:       c.MeanOutputTokens,
			MeanCachedPromptTokens: c.MeanCachedPromptTokens,
			TokensPerChunk:         c.TokensPerChunk,
			Recall:                 c.Recall,
			Check:                  c.Check,
			WarmupFailed:           c.WarmupFailed,
			Errors:                 c.Errors,
		},
	}
}
