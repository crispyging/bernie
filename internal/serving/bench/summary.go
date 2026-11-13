package bench

// Summary is the headline of a result: the four numbers a reader looks at
// first (spec §10.1). It is derived from the cells on every save and every
// read, so a result saved before the field existed has one too.
type Summary struct {
	// PeakOutputTokS is the highest output rate of a cell that ran, and
	// Peak names that cell.
	PeakOutputTokS float64  `json:"peak_output_tok_s,omitempty"`
	Peak           *CellRef `json:"peak,omitempty"`
	// SingleDecodeTokS is the median decode rate of one request at
	// concurrency 1, at the shortest context that ran at that level.
	// SinglePrefillTokS is the prefill rate of the same cell, and Single
	// names it. SinglePrefillSource says where the rate comes from:
	// "probe" (the prefill probe of the cell) or "cell" (the prefill rate
	// of the measured requests, queueing included).
	SingleDecodeTokS    float64  `json:"single_decode_tok_s,omitempty"`
	SinglePrefillTokS   float64  `json:"single_prefill_tok_s,omitempty"`
	SinglePrefillSource string   `json:"single_prefill_source,omitempty"`
	Single              *CellRef `json:"single,omitempty"`
	// Requests, Completed and Failed count the measured requests of the
	// cells that ran.
	Requests  int `json:"requests"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	// Cells counts every cell; Skipped and Incomplete count the cells that
	// were never sent and the cells that lost a request.
	Cells      int `json:"cells"`
	Skipped    int `json:"skipped"`
	Incomplete int `json:"incomplete"`
}

// CellRef names one cell of a result: its group, its context and its
// concurrency.
type CellRef struct {
	Group       string `json:"group,omitempty"`
	InputLen    int    `json:"input_len"`
	Concurrency int    `json:"concurrency"`
}

// headline derives the summary of a result from its cells.
func headline(r *Result) *Summary {
	s := &Summary{Cells: len(r.Cells)}
	for _, c := range r.Cells {
		if c.Skipped != "" {
			s.Skipped++
			continue
		}
		if c.Incomplete {
			s.Incomplete++
		}
		s.Requests += c.NumPrompts
		s.Completed += c.Completed
		s.Failed += c.Failed
		if c.Completed == 0 {
			continue
		}
		ref := &CellRef{Group: c.Group, InputLen: r.cellContext(c), Concurrency: c.Concurrency}
		if c.OutputTokS > s.PeakOutputTokS {
			s.PeakOutputTokS, s.Peak = c.OutputTokS, ref
		}
		if c.Concurrency == 1 && (s.Single == nil || ref.InputLen < s.Single.InputLen) {
			s.Single = ref
			s.SingleDecodeTokS = singleDecode(c)
			s.SinglePrefillTokS, s.SinglePrefillSource = cellPrefill(c)
		}
	}
	return s
}

// cellContext is the prompt length of a cell: the lens of its group, or the
// input length of a plain run.
func (r *Result) cellContext(c Cell) int {
	if c.InputLen > 0 {
		return c.InputLen
	}
	return r.Options.InputLen
}

// singleDecode is the decode rate of one request: the median of the
// per-request rates, else the rate of the cell (at concurrency 1 the two
// measure the same stream).
func singleDecode(c Cell) float64 {
	if c.Decode == nil {
		return c.DecodeTokS
	}
	if c.Decode.Req != nil && c.Decode.Req.P50 > 0 {
		return c.Decode.Req.P50
	}
	return c.Decode.AggTokS
}

// cellPrefill is the aggregate prefill rate of a cell and where it comes
// from: the probe when it measured one, else the rate of the cell.
func cellPrefill(c Cell) (float64, string) {
	if c.Probe != nil && c.Probe.AggTokS > 0 {
		return c.Probe.AggTokS, "probe"
	}
	if c.Prefill != nil && c.Prefill.AggTokS > 0 {
		return c.Prefill.AggTokS, "cell"
	}
	return 0, ""
}
