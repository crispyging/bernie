package bench

import "testing"

// TestHeadlineAMatrix checks the four headline numbers on a context ×
// concurrency matrix with a skipped and an incomplete cell.
func TestHeadlineAMatrix(t *testing.T) {
	cell := func(group string, inLen, conc, prompts, done int, out, prefill, decodeP50 float64) Cell {
		c := Cell{Group: group, InputLen: inLen, Concurrency: conc, NumPrompts: prompts,
			Completed: done, Failed: prompts - done}
		c.OutputTokS = out
		c.Prefill = &Rate{AggTokS: prefill}
		c.Decode = &Rate{AggTokS: decodeP50 * float64(conc), Req: &Dist{P50: decodeP50}}
		return c
	}
	r := &Result{Cells: []Cell{
		cell("8k", 8192, 1, 16, 16, 40, 3000, 41),
		cell("2k", 2048, 1, 16, 16, 45, 2500, 46),
		cell("2k", 2048, 8, 16, 14, 210, 9000, 27),
		{Group: "128k", InputLen: 131072, Concurrency: 1, Skipped: "131328 tokens do not fit max_model_len 32768"},
	}}
	r.Cells[2].Incomplete = true
	s := headline(r)
	if s.PeakOutputTokS != 210 || s.Peak == nil || s.Peak.Group != "2k" || s.Peak.Concurrency != 8 {
		t.Errorf("peak = %v at %+v, want 210 at 2k c8", s.PeakOutputTokS, s.Peak)
	}
	if s.Single == nil || s.Single.InputLen != 2048 || s.SingleDecodeTokS != 46 || s.SinglePrefillTokS != 2500 {
		t.Errorf("single = %+v decode %v prefill %v, want the 2k cell: 46 and 2500", s.Single, s.SingleDecodeTokS, s.SinglePrefillTokS)
	}
	if s.Requests != 48 || s.Completed != 46 || s.Failed != 2 {
		t.Errorf("requests = %d/%d failed %d, want 46/48 failed 2", s.Completed, s.Requests, s.Failed)
	}
	if s.Cells != 4 || s.Skipped != 1 || s.Incomplete != 1 {
		t.Errorf("cells = %d skipped %d incomplete %d, want 4, 1, 1", s.Cells, s.Skipped, s.Incomplete)
	}
}

// TestHeadlineAPlainRun checks that a plain run takes its context from the
// options and that a cell without the metric groups still has a decode rate.
func TestHeadlineAPlainRun(t *testing.T) {
	c := Cell{Concurrency: 1, NumPrompts: 4, Completed: 4}
	c.DecodeTokS = 33
	r := &Result{Options: Options{InputLen: 1024}, Cells: []Cell{c}}
	s := headline(r)
	if s.Single == nil || s.Single.InputLen != 1024 || s.SingleDecodeTokS != 33 {
		t.Errorf("single = %+v decode %v, want 1024 and 33", s.Single, s.SingleDecodeTokS)
	}
	if s.Peak != nil {
		t.Errorf("peak = %+v, want none: no cell has an output rate", s.Peak)
	}
}

// TestHeadlineNothingRan checks that a result whose cells were all skipped
// names no cell.
func TestHeadlineNothingRan(t *testing.T) {
	r := &Result{Cells: []Cell{{Concurrency: 1, Skipped: "does not fit"}}}
	s := headline(r)
	if s.Peak != nil || s.Single != nil || s.Requests != 0 || s.Skipped != 1 {
		t.Errorf("summary = %+v, want no cell and one skipped", s)
	}
}
