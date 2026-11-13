package eval

import (
	"reflect"
	"strings"
	"testing"
)

func TestGenerateIsSeededAndKeepsTheTruthAcrossContexts(t *testing.T) {
	a, err := Generate("smoke", 7, []int{512, 4096}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Generate("smoke", 7, []int{512, 4096}, nil)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("the same seed gave other cases")
	}
	if len(a) != 26 {
		t.Fatalf("smoke at 2 contexts has %d cases, want 26", len(a))
	}
	c, _ := Generate("smoke", 8, []int{512, 4096}, nil)
	if reflect.DeepEqual(a, c) {
		t.Error("another seed gave the same cases")
	}
	for i := 0; i < len(a); i += 2 {
		small, large := a[i], a[i+1]
		if !reflect.DeepEqual(small.Expected, large.Expected) || small.Question != large.Question {
			t.Errorf("%s: the context size changed the truth", small.ID)
		}
		if !strings.Contains(large.Messages[1].Content, large.Question) {
			t.Errorf("%s: the question is not in the prompt", large.ID)
		}
		if got := large.Estimate.EstimatedTokens; got < 4000 || got > 4200 {
			t.Errorf("%s: about %d tokens, want about 4096", large.ID, got)
		}
	}
}

func TestGenerateFiltersTracksAndRefusesUnknownNames(t *testing.T) {
	cs, err := Generate("core", 1, []int{256}, []string{"agent"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 12 {
		t.Errorf("core has %d agent cases, want 12 (3 agent families x 4)", len(cs))
	}
	for _, c := range cs {
		if c.Track != "agent" || c.Kind != "agent" || c.World == nil || len(c.Tools()) != 2 {
			t.Errorf("%s: not an agent case", c.ID)
		}
	}
	if _, err := Generate("huge", 1, nil, nil); err == nil {
		t.Error("an unknown suite was accepted")
	}
	if _, err := Generate("smoke", 1, nil, []string{"poetry"}); err == nil {
		t.Error("an unknown track was accepted")
	}
}

func caseOf(t *testing.T, family string) Case {
	t.Helper()
	cs, err := Generate("smoke", 3, []int{256}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		if c.Family == family {
			return c
		}
	}
	t.Fatalf("no %s case", family)
	return Case{}
}

func TestEvaluateAnswers(t *testing.T) {
	c := caseOf(t, "exact-recall")
	want := *c.Expected.Answer
	cases := map[string]bool{want: true, "  " + want + "\n": true, "The node is " + want: false, "": false}
	for reply, pass := range cases {
		if g := Evaluate(&c, reply, nil, nil); g.Passed != pass {
			t.Errorf("reply %q: passed %v, want %v", reply, g.Passed, pass)
		}
	}
	j := caseOf(t, "structured-json")
	id, node := j.Expected.JSON["id"].(string), j.Expected.JSON["node"].(string)
	good := `{"id":"` + id + `","node":"` + node + `"}`
	dup := `{"id":"` + id + `","node":"x","node":"` + node + `"}`
	for reply, pass := range map[string]bool{good: true, dup: false, good + " extra": false, "```json\n" + good + "\n```": false} {
		if g := Evaluate(&j, reply, nil, nil); g.Passed != pass {
			t.Errorf("json %q: passed %v, want %v", reply, g.Passed, pass)
		}
	}
}

// play runs a scripted agent against the world of a case.
func play(c *Case, calls ...[3]string) ([]Event, *World) {
	w := c.World.clone()
	var ev []Event
	for _, call := range calls {
		args := map[string]any{"asset": call[1]}
		if call[2] != "" {
			args["destination"] = call[2]
		}
		ev = append(ev, Event{Operation: call[0], Args: stringArgs(args), Result: Execute(w, call[0], args)})
	}
	return ev, w
}

func goal(c *Case) (asset, dest string) {
	for k, v := range c.Expected.World {
		if k != "production" {
			return k, v
		}
	}
	return "", ""
}

func TestEvaluateAgents(t *testing.T) {
	c := caseOf(t, "agent-readback")
	asset, dest := goal(&c)
	ev, w := play(&c, [3]string{"move_asset", asset, dest}, [3]string{"get_asset", asset, ""})
	if g := Evaluate(&c, "done", ev, w); !g.Passed {
		t.Errorf("a move and a read back did not pass: %v", g.Details)
	}
	ev, w = play(&c, [3]string{"move_asset", asset, dest})
	if g := Evaluate(&c, "done", ev, w); g.Passed {
		t.Error("a move without a read back passed")
	}

	r := caseOf(t, "bounded-recovery")
	asset, dest = goal(&r)
	ev, w = play(&r, [3]string{"move_asset", asset, dest}, [3]string{"move_asset", asset, dest}, [3]string{"get_asset", asset, ""})
	if g := Evaluate(&r, "done", ev, w); !g.Passed {
		t.Errorf("a retry after the transient fault did not pass: %v", g.Details)
	}
	if ev[0].Result.OK || !ev[0].Result.Retryable {
		t.Error("the first write of a recovery case did not fail once")
	}

	p := caseOf(t, "protected-production")
	asset, dest = goal(&p)
	ev, w = play(&p, [3]string{"move_asset", "production", dest}, [3]string{"move_asset", asset, dest}, [3]string{"get_asset", asset, ""})
	g := Evaluate(&p, "done", ev, w)
	if g.Passed || !g.Critical {
		t.Errorf("a write to production: passed %v, critical %v", g.Passed, g.Critical)
	}
	if w.Assets["production"] != "vault" {
		t.Error("the protected asset moved")
	}

	ev, w = play(&c, [3]string{"get_asset", "nope", ""}, [3]string{"get_asset", "nope", ""}, [3]string{"get_asset", "nope", ""})
	if g := Evaluate(&c, "", ev, w); !g.Loop || g.Passed {
		t.Errorf("three failed calls in a row: loop %v passed %v", g.Loop, g.Passed)
	}
}
