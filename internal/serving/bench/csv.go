package bench

import (
	"bytes"
	"encoding/csv"
	"strconv"
	"strings"
)

// csvHeader is the header of a result as CSV (spec §10.1): one row per cell.
var csvHeader = []string{"group", "input_len", "output_len", "concurrency", "num_prompts", "completed", "failed", "skipped",
	"incomplete", "output_tok_s", "prefill_agg_tok_s", "prefill_req_p50_tok_s", "decode_agg_tok_s", "decode_req_p50_tok_s",
	"ttft_p50_s", "ttft_p95_s", "itl_p50_ms", "itl_p95_ms", "probe_agg_tok_s", "probe_source", "probe_unavailable",
	"server_prefill_tok_s", "server_decode_tok_s", "server_unavailable", "mean_prompt_tokens", "mean_output_tokens",
	"mean_cached_prompt_tokens", "errors"}

// CSV writes a result as one row per cell, the port of BroodBench's run
// export. A number that was not measured is empty. A cell that a
// spreadsheet would read as a formula is quoted with a leading '.
func CSV(r *Result) []byte {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	_ = w.Write(csvHeader)
	num := func(v float64) string {
		if v == 0 {
			return ""
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	rate := func(x *Rate, req bool) string {
		switch {
		case x == nil:
			return ""
		case req && x.Req != nil:
			return num(x.Req.P50)
		case req:
			return ""
		}
		return num(x.AggTokS)
	}
	dist := func(d *Dist, p95 bool) string {
		if d == nil {
			return ""
		}
		if p95 {
			return num(d.P95)
		}
		return num(d.P50)
	}
	for _, c := range r.Cells {
		var probeRate, probeSource, probeWhy, srvPre, srvDec string
		if p := c.Probe; p != nil {
			probeRate, probeSource, probeWhy = num(p.AggTokS), p.Source, p.Unavailable
		}
		if s := c.ServerBatch; s != nil {
			srvPre, srvDec = num(s.PrefillTokS), num(s.DecodeTokS)
		}
		_ = w.Write(csvSafe([]string{c.Group, strconv.Itoa(r.cellContext(c)), strconv.Itoa(c.OutputLen), strconv.Itoa(c.Concurrency),
			strconv.Itoa(c.NumPrompts), strconv.Itoa(c.Completed), strconv.Itoa(c.Failed), c.Skipped, strconv.FormatBool(c.Incomplete),
			num(c.OutputTokS), rate(c.Prefill, false), rate(c.Prefill, true), rate(c.Decode, false), rate(c.Decode, true),
			dist(c.TTFTs, false), dist(c.TTFTs, true), dist(c.ITLms, false), dist(c.ITLms, true), probeRate, probeSource, probeWhy,
			srvPre, srvDec, c.ServerBatchUnavailable, num(c.MeanPromptTokens), num(c.MeanOutputTokens), num(c.MeanCachedPromptTokens),
			strings.Join(c.Errors, " | ")}))
	}
	w.Flush()
	return b.Bytes()
}

// csvSafe quotes every cell a spreadsheet would run as a formula.
func csvSafe(row []string) []string {
	for i, s := range row {
		if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
			if _, err := strconv.ParseFloat(s, 64); err != nil {
				row[i] = "'" + s
			}
		}
	}
	return row
}
