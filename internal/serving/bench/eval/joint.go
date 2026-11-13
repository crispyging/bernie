package eval

import (
	"cmp"
	"slices"
)

// Joint is the outcome of the graded cases of one family, at one context
// size or at all of them, the port of BroodBench's
// report_aggregation.report_buckets. Only observed cells exist: a family
// that was never asked at a size has no Joint there, and that is not a zero.
type Joint struct {
	Family   string  `json:"family"`
	Track    string  `json:"track"`
	Context  int     `json:"context_size,omitempty"`
	Passed   int     `json:"passed"`
	Failed   int     `json:"failed"`
	Errors   int     `json:"errors"`
	Total    int     `json:"total"`
	Accuracy float64 `json:"accuracy"`
}

// ContextSpeed is the telemetry of the calls at one context size: the
// decode and prefill against the context of the compare page.
type ContextSpeed struct {
	Context     int          `json:"context_size"`
	Performance *Performance `json:"performance"`
}

// joints groups the graded cases of a model by family and context, by
// family alone, and the calls by context.
func joints(cases []CaseResult) (byFamilyContext, byFamily []Joint, speed []ContextSpeed) {
	type key struct {
		family string
		size   int
	}
	cells := map[key]*Joint{}
	calls := map[int][]Call{}
	add := func(k key, c *CaseResult) {
		j := cells[k]
		if j == nil {
			j = &Joint{Family: k.family, Track: c.Track, Context: k.size}
			cells[k] = j
		}
		j.Total++
		switch c.Status {
		case StatusPass:
			j.Passed++
		case StatusFail:
			j.Failed++
		default:
			j.Errors++
		}
		j.Accuracy = float64(j.Passed) / float64(j.Total)
	}
	for i := range cases {
		c := &cases[i]
		if c.Status == StatusSkipped {
			continue
		}
		add(key{c.Family, c.Context}, c)
		add(key{c.Family, 0}, c)
		calls[c.Context] = append(calls[c.Context], c.Calls...)
	}
	for k, j := range cells {
		if k.size == 0 {
			byFamily = append(byFamily, *j)
		} else {
			byFamilyContext = append(byFamilyContext, *j)
		}
	}
	order := func(a, b Joint) int {
		return cmp.Or(cmp.Compare(a.Family, b.Family), cmp.Compare(a.Context, b.Context))
	}
	slices.SortFunc(byFamilyContext, order)
	slices.SortFunc(byFamily, order)
	for size, cs := range calls {
		speed = append(speed, ContextSpeed{Context: size, Performance: aggregate(cs)})
	}
	slices.SortFunc(speed, func(a, b ContextSpeed) int { return cmp.Compare(a.Context, b.Context) })
	return byFamilyContext, byFamily, speed
}
