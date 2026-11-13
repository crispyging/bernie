package suite

import (
	"fmt"

	"github.com/opengfx1030/bernie/internal/serving/bench"
)

// The verdicts of one metric.
const (
	// VerdictBetter means the second run is faster (or slower, for a
	// latency) beyond the noise band.
	VerdictBetter = "better"
	// VerdictSame means the two runs agree within the noise band.
	VerdictSame = "same"
	// VerdictWorse means the second run lost beyond the noise band.
	VerdictWorse = "worse"
	// VerdictNoisy means the repetitions of an arm disagree, so the
	// numbers of that arm cannot decide anything.
	VerdictNoisy = "noisy"
)

// Verdict is the outcome of comparing two runs of the same workload.
type Verdict struct {
	// Workload is the workload both runs measured.
	Workload bench.Workload `json:"workload"`
	A        *bench.Result  `json:"a"`
	B        *bench.Result  `json:"b"`
	// Cells holds the verdict of every cell of the run.
	Cells []CellVerdict `json:"cells"`
	// Summary counts the verdicts of the run.
	Summary Summary `json:"summary"`
	// Valid is false when the comparison could not decide.
	Valid bool `json:"valid"`
	// Reasons say why the comparison could not decide.
	Reasons []string `json:"reasons,omitempty"`
	// ExitCode is the code of the comparison: 0 when nothing is worse, 3
	// when a metric is worse, 1 when the comparison could not decide.
	ExitCode int `json:"exit_code"`
}

// CellVerdict is the verdict of the metrics of one cell.
type CellVerdict struct {
	Group       string          `json:"group"`
	Concurrency int             `json:"concurrency"`
	Metrics     []MetricVerdict `json:"metrics"`
}

// MetricVerdict is the verdict of one metric of one cell.
type MetricVerdict struct {
	Metric string `json:"metric"`
	Label  string `json:"label"`
	// Base is the number of the first run, Cand the number of the second.
	Base float64 `json:"base"`
	Cand float64 `json:"cand"`
	// Ratio is Cand ÷ Base.
	Ratio float64 `json:"ratio"`
	// DeltaPct is the change in percent.
	DeltaPct float64 `json:"delta_pct"`
	// Band is the share of a change that counts as noise.
	Band float64 `json:"band"`
	// Verdict is better, same, worse or noisy.
	Verdict string `json:"verdict"`
	// HigherIsBetter says whether a bigger number is the better one.
	HigherIsBetter bool `json:"higher_is_better"`
}

// Summary counts the verdicts of a comparison.
type Summary struct {
	Better   int  `json:"better"`
	Same     int  `json:"same"`
	Worse    int  `json:"worse"`
	Noisy    int  `json:"noisy"`
	RecallOK bool `json:"recall_ok"`
}

// Judge compares two runs of the same workload and says, for every metric
// of every cell, whether the second run is better, the same or worse than
// the first.
//
// A comparison that cannot decide says why: a cell that lost a request, a
// recall that failed, a cell that only one of the runs has, or a metric
// whose repetitions disagree.
func Judge(cmp *Comparison, band float64) *Verdict {
	v := &Verdict{Workload: cmp.Workload, A: cmp.A, B: cmp.B, Valid: true}
	if band <= 0 {
		band = cmp.Workload.CompareBand
	}
	if band <= 0 {
		band = DefaultBand
	}
	steady := cmp.Workload.CompareSteady
	if steady <= 0 {
		steady = DefaultSteady
	}
	v.Summary.RecallOK = recallOK(cmp.A) && recallOK(cmp.B)
	if !v.Summary.RecallOK {
		v.invalid("the server did not repeat the hidden fact: the numbers of a run with broken attention mean nothing")
	}
	for _, c := range cmp.A.Cells {
		if c.Incomplete {
			v.invalid(fmt.Sprintf("the cell %s is incomplete: it lost a request", CellLabel(c.Group, c.Concurrency)))
		}
	}
	for _, c := range cmp.B.Cells {
		if c.Incomplete {
			v.invalid(fmt.Sprintf("the cell %s is incomplete: it lost a request", CellLabel(c.Group, c.Concurrency)))
		}
	}
	for i := range cmp.A.Cells {
		ca := &cmp.A.Cells[i]
		cb := findCell(cmp.B, ca.Group, ca.Concurrency)
		if cb == nil {
			v.invalid(fmt.Sprintf("the run %s has no cell %s", cmp.B.ID, CellLabel(ca.Group, ca.Concurrency)))
			continue
		}
		if ca.Skipped != "" || cb.Skipped != "" {
			// A cell that did not fit the context of a server has no
			// numbers to compare; it is not a lost request.
			continue
		}
		cv := CellVerdict{Group: ca.Group, Concurrency: ca.Concurrency}
		for _, m := range Metrics {
			mv := judge(m, ca, cb, band, steady)
			if mv == nil {
				continue
			}
			cv.Metrics = append(cv.Metrics, *mv)
			switch mv.Verdict {
			case VerdictBetter:
				v.Summary.Better++
			case VerdictSame:
				v.Summary.Same++
			case VerdictWorse:
				v.Summary.Worse++
			case VerdictNoisy:
				v.Summary.Noisy++
			}
		}
		v.Cells = append(v.Cells, cv)
	}
	switch {
	case !v.Valid:
		v.ExitCode = 1
	case v.Summary.Worse > 0:
		v.ExitCode = 3
	case v.Summary.Noisy > 0:
		v.ExitCode = 1
	}
	return v
}

// invalid records a reason why the comparison could not decide.
func (v *Verdict) invalid(reason string) {
	v.Valid = false
	for _, r := range v.Reasons {
		if r == reason {
			return
		}
	}
	v.Reasons = append(v.Reasons, reason)
}

// recallOK reports whether every reply of a result held its pass key. A
// check group holds its verdict in cell.check, and a failed check is as
// fatal as a failed recall.
func recallOK(r *bench.Result) bool {
	for _, c := range r.Cells {
		if c.Recall != nil && c.Recall.Passed < c.Recall.Asked {
			return false
		}
		if c.Check != nil && c.Check.Passed < c.Check.Asked {
			return false
		}
	}
	return true
}

// findCell returns the cell of a result with this group and level.
func findCell(r *bench.Result, group string, concurrency int) *bench.Cell {
	for i := range r.Cells {
		if r.Cells[i].Concurrency == concurrency && r.Cells[i].Group == group {
			return &r.Cells[i]
		}
	}
	return nil
}

// cellLabel names a cell.
func CellLabel(group string, concurrency int) string {
	if group == "" {
		return fmt.Sprintf("c=%d", concurrency)
	}
	return fmt.Sprintf("%s c=%d", group, concurrency)
}

// judge is the verdict of one metric of one cell. It is nil when one of the
// runs did not measure the metric.
func judge(m Metric, base, cand *bench.Cell, band, steady float64) *MetricVerdict {
	b := m.Value(base)
	c := m.Value(cand)
	if b <= 0 || c <= 0 {
		return nil
	}
	// The noise band is the wider of the threshold of the suite and the
	// spread the two arms measured: a repetition that differs by 8 % makes
	// a change of 8 % noise.
	for _, cell := range []*bench.Cell{base, cand} {
		if s, ok := cell.Spread[m.Name]; ok && 1-s > band {
			band = 1 - s
		}
	}
	// The repetitions of an arm must agree, else the arm says nothing.
	for _, cell := range []*bench.Cell{base, cand} {
		if s, ok := cell.Spread[m.Name]; ok && s < steady {
			return &MetricVerdict{
				Metric: m.Name, Label: m.Label, Base: b, Cand: c,
				Ratio: c / b, DeltaPct: (c/b - 1) * 100, Band: band,
				Verdict: VerdictNoisy, HigherIsBetter: m.HigherIsBetter,
			}
		}
	}
	ratio := c / b
	verdict := VerdictSame
	switch {
	case m.HigherIsBetter:
		if ratio > 1+band && allCross(m, base, cand, above) {
			verdict = VerdictBetter
		} else if ratio < 1-band && allCross(m, base, cand, below) {
			verdict = VerdictWorse
		}
	default:
		if ratio < 1-band && allCross(m, base, cand, below) {
			verdict = VerdictBetter
		} else if ratio > 1+band && allCross(m, base, cand, above) {
			verdict = VerdictWorse
		}
	}
	return &MetricVerdict{
		Metric: m.Name, Label: m.Label, Base: b, Cand: c,
		Ratio: ratio, DeltaPct: (ratio - 1) * 100, Band: band,
		Verdict: verdict, HigherIsBetter: m.HigherIsBetter,
	}
}

// The two sides of the cross-ratio test.
const (
	above = true
	below = false
)

// allCross checks every pair of repetitions: every ratio of a repetition of
// the second run to a repetition of the first must be on the same side of
// one. Without repetitions the single pair decides.
func allCross(m Metric, base, cand *bench.Cell, wantAbove bool) bool {
	bases := repValues(m, base)
	cands := repValues(m, cand)
	if len(bases) == 0 || len(cands) == 0 {
		return true
	}
	for _, bv := range bases {
		for _, cv := range cands {
			if bv <= 0 || cv <= 0 {
				continue
			}
			ratio := cv / bv
			if wantAbove && ratio <= 1 {
				return false
			}
			if !wantAbove && ratio >= 1 {
				return false
			}
		}
	}
	return true
}

// repValues are the numbers of a metric over the repetitions of a cell.
func repValues(m Metric, c *bench.Cell) []float64 {
	var out []float64
	for _, r := range c.Reps {
		if v := m.RepValue(r); v > 0 {
			out = append(out, v)
		}
	}
	return out
}
