package suite

import (
	"context"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/bench"
)

// Run measures the suite and returns one result with the cells of every
// group. The cells of a group keep the lens of the group. Progress lines go
// to progress (may be nil).
//
// The suite decides the lens of its cells, the repetitions, the warm-up and
// the recall gate. The caller may point the run at another server, name the
// model, label the result and change the number of requests per cell.
//
// The numbers of a cell are the medians over its repetitions. The
// repetitions themselves are in the cell, so a reader can recompute them.
func Run(ctx context.Context, s *Suite, o bench.Options, progress io.Writer) (*bench.Result, error) {
	if progress == nil {
		progress = io.Discard
	}
	o = applyOverrides(s, o)
	var merged *bench.Result
	var cells []bench.Cell
	limit := contextLimit(ctx, o)
	for gi, g := range s.Groups {
		bench.SendEvent(progress, bench.Event{Kind: bench.EventGroup, Group: g.Name, Index: gi + 1, Total: len(s.Groups)})
		if reason := skipReason(g, limit); reason != "" {
			fmt.Fprintf(progress, "bench: suite %s group %s: skipped: %s\n", s.ID, g.Name, reason)
			cells = append(cells, skippedCells(g, reason)...)
			continue
		}
		fmt.Fprintf(progress, "bench: suite %s group %s: %d prompt tokens, %d output tokens, c=%v, %d repetition(s)\n",
			s.ID, g.Name, g.InputLen, g.OutputLen, g.Concurrency, g.Reps)
		reps, first, err := runGroup(ctx, s, g, o, progress)
		if err != nil {
			return nil, err
		}
		if merged == nil {
			merged = first
		}
		cells = append(cells, groupCells(g, reps)...)
	}
	if merged == nil && len(cells) > 0 {
		return nil, fmt.Errorf("no group of the suite %s fits the server: %s", s.ID, cells[0].Skipped)
	}
	if merged == nil {
		return nil, fmt.Errorf("the suite %s has no groups", s.ID)
	}
	merged.Options = o
	merged.Cells = cells
	merged.Suite = &bench.SuiteRef{ID: s.ID, Digest: s.Digest(), Version: s.Schema}
	merged.Workload = workload(s, o, merged.Workload)
	return merged, nil
}

// runGroup runs the repetitions of a group and returns the cells of every
// repetition.
func runGroup(ctx context.Context, s *Suite, g Group, o bench.Options, progress io.Writer) ([]bench.Cell, *bench.Result, error) {
	var out []bench.Cell
	var first *bench.Result
	for i := range g.Reps {
		opts := o
		opts.InputLen = g.InputLen
		opts.OutputLen = g.OutputLen
		opts.Concurrency = slices.Clone(g.Concurrency)
		opts.Warmup = s.Warmup
		opts.SizeTolerancePercent = s.SizeTolerancePercent
		// A group of kind "check" runs the pass key machinery and the
		// verdict lands in cell.check, separate from the suite-wide
		// recall gate. A throughput group never turns the check on.
		opts.Check = g.Kind == KindCheck
		opts.Recall = o.Recall && !opts.Check
		r, err := bench.Run(ctx, opts, progress)
		if err != nil {
			return nil, nil, err
		}
		if first == nil {
			first = r
		}
		out = append(out, r.Cells...)
		if i+1 < g.Reps {
			if err := cooldown(ctx, s.CooldownS); err != nil {
				return nil, nil, err
			}
		}
	}
	return out, first, nil
}

// groupCells builds the cells of a group: one cell per concurrency level,
// with the medians over the repetitions, the repetitions themselves and the
// spread of the metrics.
func groupCells(g Group, reps []bench.Cell) []bench.Cell {
	var out []bench.Cell
	for _, level := range g.Concurrency {
		var ofLevel []bench.Cell
		for _, c := range reps {
			if c.Concurrency == level {
				ofLevel = append(ofLevel, c)
			}
		}
		if len(ofLevel) == 0 {
			continue
		}
		cell := medianCell(ofLevel)
		cell.Group = g.Name
		cell.InputLen = g.InputLen
		cell.OutputLen = g.OutputLen
		cell.Kind = g.Kind
		for _, r := range ofLevel {
			cell.Reps = append(cell.Reps, repOf(r))
			if r.WarmupFailed > 0 || r.Failed > 0 {
				cell.Incomplete = true
			}
		}
		cell.Spread = spreadOf(ofLevel)
		out = append(out, cell)
	}
	return out
}

// cooldown waits between two repetitions. It keeps the GPU out of a
// thermal and power steady state, so the repetitions measure the same work.
func cooldown(ctx context.Context, seconds int) error {
	if seconds <= 0 {
		return nil
	}
	timer := time.NewTimer(time.Duration(seconds) * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// applyOverrides lets the caller change the server, the model, the label,
// the requests per cell and the seed. The suite keeps the rest.
func applyOverrides(s *Suite, o bench.Options) bench.Options {
	o.Suite = s.ID
	o.Reps = s.Reps
	if o.NumPrompts == 0 {
		o.NumPrompts = s.Defaults.Requests
	}
	if o.Seed == 0 {
		o.Seed = s.Seed
	}
	if s.Recall {
		o.Recall = true
	}
	if s.Probe {
		o.Probe = true
	}
	o.InputLen = 0
	o.OutputLen = 0
	o.SizeTolerancePercent = s.SizeTolerancePercent
	return o
}

// workload describes what the suite asked the server to do. The lens of a
// suite run is per cell, so the workload carries the suite and the levels.
func workload(s *Suite, o bench.Options, base *bench.Workload) *bench.Workload {
	w := &bench.Workload{ProtocolID: bench.ProtocolID, Recall: o.Recall}
	if base != nil {
		w.ProtocolID = base.ProtocolID
		w.CorpusSHA256 = base.CorpusSHA256
	}
	w.SuiteID = s.ID
	w.SuiteDigest = s.Digest()
	// The kind of the workload is "check" if any group is a check group,
	// else "throughput". A mixed suite compares with another mixed
	// suite of the same digest; a pure throughput suite compares with
	// another pure throughput suite.
	w.Kind = KindThroughput
	for _, g := range s.Groups {
		if g.Kind == KindCheck {
			w.Kind = KindCheck
			w.Check = true
			break
		}
	}
	w.CompareBand = s.Compare.Band
	w.CompareSteady = s.Compare.Steady
	for _, g := range s.Groups {
		for _, c := range g.Concurrency {
			if !slices.Contains(w.Concurrency, c) {
				w.Concurrency = append(w.Concurrency, c)
			}
		}
	}
	slices.Sort(w.Concurrency)
	return w
}
