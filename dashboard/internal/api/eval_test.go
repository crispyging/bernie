package api

import (
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/bench"
	"github.com/opengfx1030/bernie/internal/serving/bench/eval"
)

// TestEvalEndpoints checks the list, one result and its CSV, with a reply
// that a spreadsheet would run as a formula.
func TestEvalEndpoints(t *testing.T) {
	h := smokeHandler(t)
	want := "42"
	r := &eval.Result{ID: "eval-20261113T120000Z-abc123", Version: eval.Version, Time: time.Now().UTC(), Status: "done",
		Options: eval.Options{Models: []string{"m"}, Suite: "smoke"},
		Cases: []eval.CaseResult{
			{Model: "m", CaseID: "exact-recall-000-ctx512", Track: "recall", Status: eval.StatusPass, Expected: eval.Expected{Answer: &want}, Reply: "42"},
			{Model: "m", CaseID: "exact-recall-013-ctx512", Track: "recall", Status: eval.StatusFail, Expected: eval.Expected{Answer: &want}, Reply: "=HYPERLINK(\"x\")"},
		}}
	if _, err := eval.Save(r); err != nil {
		t.Fatal(err)
	}
	w := get(t, h, "/api/eval")
	var list []eval.Result
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list) != 1 || len(list[0].Cases) != 0 || list[0].Summary[0].Passed != 1 {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	w = get(t, h, "/api/eval/"+r.ID)
	var one eval.Result
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &one) != nil || len(one.Cases) != 2 || one.Cases[1].Diagnosis.Category != "exact_match_failure" {
		t.Fatalf("one: %d %s", w.Code, w.Body.String())
	}
	w = get(t, h, "/api/eval/"+r.ID+"?format=csv")
	rows, err := csv.NewReader(w.Body).ReadAll()
	if err != nil || w.Header().Get("Content-Type") != "text/csv; charset=utf-8" || len(rows) != 3 {
		t.Fatalf("csv: %v %q %v", err, w.Header().Get("Content-Type"), rows)
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), r.ID+".csv") {
		t.Errorf("disposition %q", w.Header().Get("Content-Disposition"))
	}
	reply := rows[2][17]
	if !strings.HasPrefix(reply, "'=") {
		t.Errorf("a formula reply is not quoted: %q", reply)
	}
	if w := get(t, h, "/api/eval/nope"); w.Code != 404 {
		t.Errorf("an unknown eval: %d", w.Code)
	}
}

// TestBenchCSVEndpoint checks the CSV of a speed result: one row per cell,
// with the skipped cell and its reason.
func TestBenchCSVEndpoint(t *testing.T) {
	h := smokeHandler(t)
	r := &bench.Result{ID: "20261113T120000Z-abc123", Time: time.Now().UTC(), Options: bench.Options{InputLen: 1024}, Cells: []bench.Cell{
		{Concurrency: 1, NumPrompts: 4, Completed: 4, Measurement: bench.Measurement{OutputTokS: 42.5}},
		{Concurrency: 8, Group: "128k", InputLen: 131072, Skipped: "does not fit max_model_len 32768"},
	}}
	if _, err := bench.Save(r); err != nil {
		t.Fatal(err)
	}
	w := get(t, h, "/api/bench/csv?id="+r.ID)
	rows, err := csv.NewReader(w.Body).ReadAll()
	if err != nil || w.Code != 200 || len(rows) != 3 {
		t.Fatalf("csv: %d %v %v", w.Code, err, rows)
	}
	if rows[0][0] != "group" || rows[1][1] != "1024" || rows[1][9] != "42.5" || rows[2][7] != "does not fit max_model_len 32768" {
		t.Errorf("rows %v", rows)
	}
	if w := get(t, h, "/api/bench/csv?id=nope"); w.Code != 404 {
		t.Errorf("an unknown result: %d", w.Code)
	}
}
