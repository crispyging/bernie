package eval

import (
	"bytes"
	"encoding/csv"
	"strconv"
	"strings"
)

// csvHeader is the header of an eval result as CSV (spec §10.5): one row
// per case, model and repeat.
var csvHeader = []string{"model", "case_id", "track", "family", "variant", "context_size", "repeat", "status", "diagnosis",
	"critical", "loop", "latency_s", "prompt_tokens", "completion_tokens", "removed_distractor_records", "tool_calls",
	"expected", "reply", "error"}

// CSV writes an eval result as one row per case, the port of BroodBench's
// run export. A cell a spreadsheet would read as a formula is quoted with a
// leading '.
func CSV(r *Result) []byte {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	_ = w.Write(csvHeader)
	ptr := func(p *int) string {
		if p == nil {
			return ""
		}
		return strconv.Itoa(*p)
	}
	for _, c := range r.Cases {
		diag, removed := "", ""
		if c.Diagnosis != nil {
			diag = c.Diagnosis.Category
		}
		if c.Admission != nil {
			removed = strconv.Itoa(c.Admission.Removed)
		}
		expected := compactJSON(expectedValue(c.Expected))
		if c.Expected.Answer != nil {
			expected = *c.Expected.Answer
		}
		_ = w.Write(csvSafe([]string{c.Model, c.CaseID, c.Track, c.Family, strconv.Itoa(c.Variant), strconv.Itoa(c.Context),
			strconv.Itoa(c.Repeat), c.Status, diag, strconv.FormatBool(c.Critical), strconv.FormatBool(c.Loop),
			strconv.FormatFloat(c.LatencyS, 'f', 3, 64), ptr(c.PromptTokens), ptr(c.CompletionTokens), removed,
			strconv.Itoa(len(c.Events)), expected, c.Reply, c.Error}))
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
