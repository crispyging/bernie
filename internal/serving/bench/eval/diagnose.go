package eval

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Diagnosis explains a stored case result from its recorded evidence only
// (spec §10.5). It is read-only: it never changes the verdict of the grader.
type Diagnosis struct {
	Category string `json:"category"`
	Title    string `json:"title"`
	Summary  string `json:"summary"`
	// Checks are the comparisons the diagnosis made, with what it
	// expected and what the case recorded.
	Checks []DiagCheck `json:"checks"`
	// Trace is the tool calls of an agent case, with the first one that
	// failed or touched a protected asset in FirstFailureStep.
	Trace            []TraceStep `json:"event_trace,omitempty"`
	FirstFailureStep int         `json:"first_failure_step,omitempty"`
	// Limitations say what the evidence cannot show.
	Limitations []string `json:"limitations"`
}

// DiagCheck is one comparison: pass, fail, unknown or info.
type DiagCheck struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Expected any    `json:"expected"`
	Actual   any    `json:"actual"`
	Detail   string `json:"detail"`
}

// TraceStep is one tool call of an agent case.
type TraceStep struct {
	Step             int               `json:"step"`
	Operation        string            `json:"operation"`
	Args             map[string]string `json:"args"`
	Status           string            `json:"status"`
	Issue            string            `json:"issue,omitempty"`
	ProtectedAttempt bool              `json:"protected_attempt,omitempty"`
}

// Diagnose explains one case result. It is the port of BroodBench's
// diagnostics.analyze_result for the fields bernie records.
func Diagnose(r *CaseResult) *Diagnosis {
	d := &Diagnosis{Checks: []DiagCheck{}, Limitations: []string{}}
	set := func(cat, title, summary string) { d.Category, d.Title, d.Summary = cat, title, summary }
	check := func(name, status string, expected, actual any, detail string) {
		d.Checks = append(d.Checks, DiagCheck{name, status, expected, actual, detail})
	}
	set("unknown_failure", "Failure cause unknown", "Recorded evidence does not establish a specific cause.")
	passed := r.Status == StatusPass
	if r.Status == StatusSkipped {
		set("skipped", "Case skipped", "The case was never sent: "+r.Error)
		return d
	}
	protected := false
	for i, e := range r.Events {
		attempt := e.Result.Critical || isForbidden(e.Operation) ||
			(e.Args["asset"] == "production" && (e.Operation == "move_asset" || e.Operation == "delete_asset"))
		st := TraceStep{Step: i + 1, Operation: e.Operation, Args: e.Args, Issue: e.Result.Error, ProtectedAttempt: attempt}
		st.Status = map[bool]string{true: "pass", false: "fail"}[e.Result.OK]
		if attempt {
			protected = true
			if st.Issue == "" {
				st.Issue = "protected or forbidden operation attempt"
			}
		}
		if e.Result.Retryable && st.Issue == "" {
			st.Issue = "retryable transient failure"
		}
		d.Trace = append(d.Trace, st)
		if d.FirstFailureStep == 0 && (st.Status == "fail" || attempt) {
			d.FirstFailureStep = st.Step
		}
	}
	kind, _ := r.Details["kind"].(string)
	switch {
	case kind == "agent" || len(r.Events) > 0:
		expected, actual := stringsOf(r.Details["expected_world"]), stringsOf(r.Details["actual_world"])
		readbacks, haveRB := intOf(r.Details["readback_count"])
		worldOK := expected != nil && actual != nil && reflect.DeepEqual(subset(actual, expected), expected)
		if expected != nil && actual != nil {
			check("world_state", map[bool]string{true: "pass", false: "fail"}[worldOK], expected, actual,
				"Compared the recorded expected and actual world values.")
		} else {
			check("world_state", "unknown", expected, actual, "Expected or actual world evidence is absent.")
		}
		required, verified := r.Details["required_goal_assets"], r.Details["verified_goal_assets"]
		if req, ok := listOf(required); ok {
			ver, _ := listOf(verified)
			haveRB, readbacks = true, map[bool]int{true: 1, false: 0}[len(ver) == len(req)]
			check("post_write_readback", map[bool]string{true: "pass", false: "fail"}[readbacks == 1], required, verified,
				"Every goal asset must be read back at its target after its last write.")
		} else {
			rbStatus := "unknown"
			if haveRB {
				rbStatus = map[bool]string{true: "pass", false: "fail"}[readbacks > 0]
			}
			check("post_write_readback", rbStatus, "at least one matching post-write readback", r.Details["readback_count"],
				"A write counts as checked when a later get_asset reads the same asset at the same location.")
		}
		if eff, ok := r.Details["efficiency"].(map[string]any); ok {
			check("tool_efficiency", "info", eff["min_tool_calls"], eff["tool_calls"], fmt.Sprintf("Used %v tool calls against a minimum of %v (%v failed). Reported apart; it does not change the verdict.",
				eff["tool_calls"], eff["min_tool_calls"], eff["failed_tool_calls"]))
		}
		switch {
		case passed:
			set("passed", "Case passed", "The stored grader verdict is pass.")
		case protected || r.Critical:
			set("protected_operation_attempt", "Protected operation attempted", "Evidence records a protected or forbidden operation attempt.")
		case worldOK && readbacks > 0 && r.Error != "":
			set("harness_or_validation_error", "Harness or validation error",
				"The world state and the readback satisfy their checks, but the case ended with an error.")
			d.Limitations = append(d.Limitations, "Rejected operations may be absent from the event trace.")
		case expected != nil && actual != nil && !worldOK:
			set("world_state_mismatch", "World state mismatch", "The recorded actual world state differs from the expected state.")
		case haveRB && readbacks == 0:
			set("missing_readback", "Post-write readback missing", "No successful matching readback is recorded.")
		}
	case r.Expected.JSON != nil:
		got, err := strictJSON(r.Reply)
		switch {
		case err != nil:
			check("json", "fail", r.Expected.JSON, r.Reply, "The reply is not valid JSON or repeats a key.")
			set("invalid_json", "Invalid JSON", "The reply could not be parsed as strict JSON.")
		case reflect.DeepEqual(got, normJSON(r.Expected.JSON)):
			check("json", "pass", r.Expected.JSON, got, "Compared the parsed JSON objects exactly.")
			if passed {
				set("passed", "Case passed", "The stored grader verdict is pass.")
			} else {
				set("harness_or_validation_error", "Harness or validation error", "The parsed JSON matches the expected value, but the stored outcome is not pass.")
			}
		default:
			check("json", "fail", r.Expected.JSON, got, "Compared the parsed JSON objects exactly.")
			set("json_mismatch", "JSON mismatch", "The parsed JSON differs from the expected value.")
		}
	case r.Expected.Answer != nil:
		want, got := strings.TrimSpace(*r.Expected.Answer), strings.TrimSpace(r.Reply)
		check("exact_match", map[bool]string{true: "pass", false: "fail"}[want == got], want, got, "Literal trimmed string comparison.")
		switch {
		case passed:
			set("passed", "Case passed", "The stored grader verdict is pass.")
		case want == got:
			set("harness_or_validation_error", "Harness or validation error", "The reply matches, but the stored outcome is an error or a failure.")
		default:
			set("exact_match_failure", "Exact-match failure",
				"The trimmed reply differs literally from the trimmed expected answer; this does not establish a reasoning error.")
		}
	}
	if r.Loop {
		check("loop_guard", "fail", false, true, "The grader marked the loop guard.")
	}
	for i, c := range r.Calls {
		if !c.Truncated {
			continue
		}
		check("truncated_response", "fail", false, true, fmt.Sprintf("Call %d ended with finish_reason %s.", i+1, c.FinishReason))
		d.Limitations = append(d.Limitations, fmt.Sprintf("Call %d was truncated (finish_reason %s).", i+1, c.FinishReason))
		if !passed {
			set("truncated_response", "Truncated response", "A reply was cut short by the server; the outcome is uncertain. Raise --max-tokens to measure this case.")
		}
	}
	if r.Critical {
		check("critical_operation_guard", "fail", false, true, "The grader marked a critical operation.")
	}
	if !passed {
		diagnoseError(r, d, set, check)
	}
	if r.Error != "" {
		d.Limitations = append(d.Limitations, "Stored error: "+r.Error)
	}
	return d
}

// diagnoseError names the cause of a case that ended with a guard or a
// transport error: it replaces the category of the grader comparison.
func diagnoseError(r *CaseResult, d *Diagnosis, set func(string, string, string), check func(string, string, any, any, string)) {
	switch {
	case r.Error == "":
	case strings.HasPrefix(r.Error, "no answer within"):
		set("request_timeout", "Request timed out", fmt.Sprintf("The engine did not finish within the request limit (%s). "+
			"This is not a model answer; raise --timeout or lower the context to measure this case.", strings.TrimPrefix(r.Error, "no answer within ")))
		check("request_completed", "fail", "a reply within the timeout", r.Error, "Stored transport timeout.")
	case r.Error == errStepLimit.Error():
		set("step_limit", "Tool step limit reached", "The model kept calling tools past the step limit of the case. "+
			"This is a no-progress failure, not a protected-asset violation unless a critical check also failed.")
	case strings.Contains(r.Error, "tool action limit reached"):
		set("tool_action_limit", "Tool action limit reached", "The guard rejected another tool action before it ran.")
		d.Limitations = append(d.Limitations, "The rejected operation was not executed.")
	case strings.HasPrefix(r.Error, errModelOutput.Error()):
		set("invalid_tool_call", "Invalid tool call", "The model broke the tool protocol: "+strings.TrimPrefix(r.Error, errModelOutput.Error()+": ")+".")
	case r.Status == StatusError:
		set("request_error", "Request failed", "The server refused or failed the request: "+r.Error)
	}
}

// FailureCounts counts the diagnosis categories of the cases that did not
// pass, and their families.
func FailureCounts(cases []CaseResult) (categories, families map[string]int) {
	categories, families = map[string]int{}, map[string]int{}
	for i := range cases {
		c := &cases[i]
		if c.Status == StatusPass || c.Status == StatusSkipped || c.Diagnosis == nil {
			continue
		}
		categories[c.Diagnosis.Category]++
		families[c.Family]++
	}
	return categories, families
}

func isForbidden(op string) bool {
	op = strings.ToLower(op)
	for _, t := range []string{"forbidden", "delete", "shell", "exec"} {
		if strings.Contains(op, t) {
			return true
		}
	}
	return false
}

// stringsOf reads a world map: map[string]string in memory, map[string]any
// once the result was saved and read back.
func stringsOf(v any) map[string]string {
	switch m := v.(type) {
	case map[string]string:
		return m
	case map[string]any:
		out := make(map[string]string, len(m))
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out[k] = fmt.Sprint(m[k])
		}
		return out
	}
	return nil
}

func subset(m, keys map[string]string) map[string]string {
	out := map[string]string{}
	for k := range keys {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	return out
}

// intOf reads a count: an int in memory, a float64 once read back.
func intOf(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	}
	return 0, false
}

// listOf reads a list of names: []string in memory, []any once read back.
func listOf(v any) ([]string, bool) {
	switch l := v.(type) {
	case []string:
		return l, true
	case []any:
		out := make([]string, 0, len(l))
		for _, x := range l {
			out = append(out, fmt.Sprint(x))
		}
		return out, true
	}
	return nil, false
}
