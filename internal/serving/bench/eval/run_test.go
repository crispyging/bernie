package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// oracle is a fake server that knows the truth of every case: it answers
// the answer cases and drives the agent cases with tool calls (one retry
// after a transient fault, then a read back).
func oracle(t *testing.T, cases []Case, maxLen int, reject func(promptChars int) string) *httptest.Server {
	t.Helper()
	truth := map[string]*Case{}
	for i := range cases {
		truth[cases[i].Messages[1].Content] = &cases[i]
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"data":[{"id":"m","max_model_len":%d}]}`, maxLen)
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []map[string]any `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		prompt, _ := req.Messages[1]["content"].(string)
		if reject != nil {
			if msg := reject(len(prompt)); msg != "" {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, `{"error":{"message":%q}}`, msg)
				return
			}
		}
		// Admission may remove distractor records, so the oracle finds the
		// case by its evidence, which is never cut.
		var c *Case
		for _, k := range cases {
			if strings.Contains(prompt, k.Question) {
				c = truth[k.Messages[1].Content]
				break
			}
		}
		msg := map[string]any{"role": "assistant", "content": ""}
		switch {
		case c == nil:
			msg["content"] = "unknown case"
		case c.Kind == "answer" && c.Expected.JSON != nil:
			b, _ := json.Marshal(c.Expected.JSON)
			msg["content"] = string(b)
		case c.Kind == "answer":
			msg["content"] = *c.Expected.Answer
		default:
			asset, dest := goal(c)
			// The answer of the last tool call, and the name of that call.
			var last map[string]any
			lastCall := ""
			for _, m := range req.Messages {
				if m["role"] == "tool" {
					last = nil
					_ = json.Unmarshal([]byte(m["content"].(string)), &last)
				}
				if calls, ok := m["tool_calls"].([]any); ok && len(calls) > 0 {
					lastCall = calls[0].(map[string]any)["function"].(map[string]any)["name"].(string)
				}
			}
			call := func(name, args string) {
				msg["tool_calls"] = []map[string]any{{"id": "c1", "type": "function",
					"function": map[string]string{"name": name, "arguments": args}}}
			}
			switch {
			case last == nil || last["retryable"] == true:
				call("move_asset", fmt.Sprintf(`{"asset":%q,"destination":%q}`, asset, dest))
			case lastCall == "move_asset":
				call("get_asset", fmt.Sprintf(`{"asset":%q}`, asset))
			case last["asset"] == asset && c.Expected.World["production"] != "":
				// Since 1.4 every goal asset is read back, the untouched
				// production too; its ticket note is bait and is ignored.
				call("get_asset", `{"asset":"production"}`)
			default:
				msg["content"] = "done"
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": msg}},
			"usage":   map[string]int{"prompt_tokens": len(prompt) / 4, "completion_tokens": 3}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRunPassesAKnownGoodServer(t *testing.T) {
	cases, _ := Generate("smoke", 5, []int{512}, nil)
	srv := oracle(t, cases, 0, nil)
	var log strings.Builder
	r, err := Run(context.Background(), Options{URL: srv.URL, Seed: 5, Contexts: []int{512}}, &log)
	if err != nil {
		t.Fatal(err)
	}
	s := r.Summary[0]
	if s.Model != "m" || s.Passed != 13 || s.Graded != 13 || s.Accuracy != 1 {
		t.Fatalf("summary %+v\n%s", s, log.String())
	}
	if s.Tokens == nil || len(s.ByTrack) != 6 {
		t.Errorf("tokens %v, %d tracks", s.Tokens, len(s.ByTrack))
	}
	if !strings.Contains(log.String(), "[13/13]") {
		t.Errorf("the log does not count the cases:\n%s", log.String())
	}
}

// TestRunAdmitsCasesBeyondTheServerContext checks that a case built for a
// band beyond the server's context loses distractor records until it fits,
// and is graded like any other.
func TestRunAdmitsCasesBeyondTheServerContext(t *testing.T) {
	cases, _ := Generate("smoke", 5, []int{512, 8192}, []string{"recall"})
	srv := oracle(t, cases, 4096, nil)
	var log strings.Builder
	r, err := Run(context.Background(), Options{URL: srv.URL, Seed: 5, Contexts: []int{512, 8192}, Tracks: []string{"recall"}}, &log)
	if err != nil {
		t.Fatal(err)
	}
	s := r.Summary[0]
	if s.Skipped != 0 || s.Passed != s.Graded || s.Graded != len(cases) {
		t.Fatalf("summary %+v\n%s", s, log.String())
	}
	for _, c := range r.Cases {
		a := c.Admission
		if a == nil || a.CountSource != "conservative_byte_admission" || a.Budget != 4096-1024-512 {
			t.Fatalf("%s: admission %+v", c.CaseID, a)
		}
		if (c.Context == 8192) != (a.Removed > 0) {
			t.Errorf("%s: %d records removed at context %d", c.CaseID, a.Removed, c.Context)
		}
	}
}

// TestRunSkipsCasesWhoseEvidenceDoesNotFit checks that a case is skipped
// with the reason, never sent, when the reply reservation leaves no room.
func TestRunSkipsCasesWhoseEvidenceDoesNotFit(t *testing.T) {
	cases, _ := Generate("smoke", 5, []int{512}, []string{"recall"})
	srv := oracle(t, cases, 1400, nil)
	var log strings.Builder
	r, err := Run(context.Background(), Options{URL: srv.URL, Seed: 5, Contexts: []int{512}, Tracks: []string{"recall"}}, &log)
	if err != nil {
		t.Fatal(err)
	}
	if s := r.Summary[0]; s.Skipped != len(cases) || s.Graded != 0 {
		t.Fatalf("summary %+v", s)
	}
	if !strings.Contains(log.String(), "context admission: 1024 reply tokens leave no room") {
		t.Errorf("the log does not say why a case was skipped:\n%s", log.String())
	}
}

// TestRunPlansTheContexts checks --context auto against the server's
// max_model_len.
func TestRunPlansTheContexts(t *testing.T) {
	cases, _ := Generate("smoke", 5, []int{2048, 8192, 28672}, []string{"recall"})
	srv := oracle(t, cases, 32768, nil)
	r, err := Run(context.Background(), Options{URL: srv.URL, Seed: 5, AutoContext: true, Tracks: []string{"recall"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := r.ContextPlan
	if p == nil || p.ModelLimit != 32768 || !slices.Equal(r.Options.Contexts, []int{2048, 8192, 28672}) {
		t.Fatalf("plan %+v, contexts %v", p, r.Options.Contexts)
	}
	if s := r.Summary[0]; s.Passed != s.Graded || s.Graded != len(cases) {
		t.Errorf("summary %+v", s)
	}
	if _, err := Normalize(Options{AutoContext: true, Contexts: []int{4096}}); err == nil {
		t.Error("auto with named sizes was accepted")
	}
}

func TestRunRecordsTheServerReason(t *testing.T) {
	cases, _ := Generate("smoke", 5, []int{256}, []string{"recall"})
	srv := oracle(t, cases, 0, func(int) string { return "maximum context length is 128 tokens" })
	var log strings.Builder
	r, err := Run(context.Background(), Options{URL: srv.URL, Seed: 5, Contexts: []int{256}, Tracks: []string{"recall"}}, &log)
	if err != nil {
		t.Fatal(err)
	}
	if r.Summary[0].Errors != r.Summary[0].Graded {
		t.Fatalf("summary %+v", r.Summary[0])
	}
	want := "HTTP 400: maximum context length is 128 tokens"
	if r.Cases[0].Error != want || !strings.Contains(log.String(), want) {
		t.Errorf("error %q; log:\n%s", r.Cases[0].Error, log.String())
	}
}

func TestRunRefusesAModelTheServerDoesNotServe(t *testing.T) {
	srv := oracle(t, nil, 0, nil)
	if _, err := Run(context.Background(), Options{URL: srv.URL, Models: []string{"other"}}, nil); err == nil ||
		!strings.Contains(err.Error(), `does not serve model "other"`) {
		t.Errorf("err = %v", err)
	}
}
