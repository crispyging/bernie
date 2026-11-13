package eval

import (
	"context"
	"strings"
	"testing"
)

// TestDiagnoseCategories checks the category of each kind of outcome.
func TestDiagnoseCategories(t *testing.T) {
	answer := func(s string) Expected { return Expected{Answer: &s} }
	world := map[string]any{"db": "eu-west"}
	cases := []struct {
		name string
		r    CaseResult
		want string
	}{
		{"pass", CaseResult{Status: StatusPass, Expected: answer("42"), Reply: "42"}, "passed"},
		{"exact", CaseResult{Status: StatusFail, Expected: answer("42"), Reply: "41"}, "exact_match_failure"},
		{"matches but error", CaseResult{Status: StatusError, Expected: answer("42"), Reply: " 42 "}, "harness_or_validation_error"},
		{"invalid json", CaseResult{Status: StatusFail, Expected: Expected{JSON: map[string]any{"a": "1"}}, Reply: `{"a":"1","a":"2"}`}, "invalid_json"},
		{"json mismatch", CaseResult{Status: StatusFail, Expected: Expected{JSON: map[string]any{"a": "1"}}, Reply: `{"a":"2"}`}, "json_mismatch"},
		{"skipped", CaseResult{Status: StatusSkipped, Error: "context admission: no room"}, "skipped"},
		{"timeout", CaseResult{Status: StatusError, Expected: answer("42"), Error: "no answer within 2m0s"}, "request_timeout"},
		{"http", CaseResult{Status: StatusError, Expected: answer("42"), Error: "HTTP 400: maximum context length is 8192 tokens"}, "request_error"},
		{"step limit", CaseResult{Status: StatusFail, Loop: true, Error: "step limit reached",
			Details: map[string]any{"kind": "agent", "expected_world": world, "actual_world": map[string]any{"db": "us-east"}, "readback_count": 0.0}}, "step_limit"},
		{"world", CaseResult{Status: StatusFail,
			Details: map[string]any{"kind": "agent", "expected_world": world, "actual_world": map[string]any{"db": "us-east"}, "readback_count": 1.0}}, "world_state_mismatch"},
		{"readback", CaseResult{Status: StatusFail,
			Details: map[string]any{"kind": "agent", "expected_world": world, "actual_world": world, "readback_count": 0.0}}, "missing_readback"},
		{"protected", CaseResult{Status: StatusFail, Critical: true,
			Events:  []Event{{Operation: "move_asset", Args: map[string]string{"asset": "production"}, Result: ToolResult{Error: "protected", Critical: true}}},
			Details: map[string]any{"kind": "agent", "expected_world": world, "actual_world": world, "readback_count": 1.0}}, "protected_operation_attempt"},
	}
	for _, c := range cases {
		d := Diagnose(&c.r)
		if d.Category != c.want {
			t.Errorf("%s: category %q (%s), want %q", c.name, d.Category, d.Summary, c.want)
		}
	}
}

// TestDiagnoseTrace checks the event trace of an agent case and its first
// failing step.
func TestDiagnoseTrace(t *testing.T) {
	r := CaseResult{Status: StatusPass, Details: map[string]any{"kind": "agent"}, Events: []Event{
		{Operation: "move_asset", Args: map[string]string{"asset": "db"}, Result: ToolResult{Error: "busy", Retryable: true}},
		{Operation: "move_asset", Args: map[string]string{"asset": "db"}, Result: ToolResult{OK: true}},
	}}
	d := Diagnose(&r)
	if len(d.Trace) != 2 || d.FirstFailureStep != 1 || d.Trace[0].Status != "fail" || d.Trace[1].Status != "pass" || d.Trace[0].Issue != "busy" {
		t.Errorf("trace %+v first failure %d", d.Trace, d.FirstFailureStep)
	}
}

// TestRunDiagnosesEveryCase checks that a run diagnoses its cases, counts
// the categories per model, and that a saved result gets them back.
func TestRunDiagnosesEveryCase(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	cases, _ := Generate("smoke", 5, []int{256}, []string{"recall"})
	srv := oracle(t, cases, 0, func(int) string { return "maximum context length is 128 tokens" })
	r, err := Run(context.Background(), Options{URL: srv.URL, Seed: 5, Contexts: []int{256}, Tracks: []string{"recall"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := r.Summary[0]
	if s.FailureCategories["request_error"] != s.Graded || s.Graded == 0 {
		t.Fatalf("failure categories %v over %d graded", s.FailureCategories, s.Graded)
	}
	if _, err := Save(r); err != nil {
		t.Fatal(err)
	}
	back, err := Get(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d := back.Cases[0].Diagnosis; d == nil || d.Category != "request_error" || !strings.Contains(d.Summary, "maximum context length") {
		t.Errorf("diagnosis after a read: %+v", d)
	}
}
