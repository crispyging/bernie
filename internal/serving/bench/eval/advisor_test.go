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

const goodProposal = `{"suite":"smoke","context_sizes":[2048,8192],"max_tokens":512,"timeout_s":60,"repeats":1,
"temperature":0,"tracks":["recall","structured"],"seed":7,"rationale":"Two bands check recall and JSON under a short and a long prompt."}`

// TestParseProposal checks the caps of a proposal.
func TestParseProposal(t *testing.T) {
	o, why, err := parseProposal(goodProposal, 32768)
	if err != nil {
		t.Fatal(err)
	}
	if o.Suite != "smoke" || !slices.Equal(o.Contexts, []int{2048, 8192}) || o.MaxTokens != 512 || o.Seed != 7 ||
		!slices.Equal(o.Tracks, []string{"recall", "structured"}) || o.Repeats != 1 || why == "" {
		t.Errorf("options %+v rationale %q", o, why)
	}
	bad := map[string]string{
		"prose":          "Here is the config: " + goodProposal,
		"duplicate key":  strings.Replace(goodProposal, `"seed":7`, `"seed":7,"seed":8`, 1),
		"extra key":      strings.Replace(goodProposal, `"seed":7`, `"seed":7,"max_calls":10`, 1),
		"over the limit": strings.Replace(goodProposal, "8192", "32768", 1),
		"repeat size":    strings.Replace(goodProposal, "8192", "2048", 1),
		"temperature":    strings.Replace(goodProposal, `"temperature":0`, `"temperature":0.7`, 1),
		"repeats":        strings.Replace(goodProposal, `"repeats":1`, `"repeats":3`, 1),
		"unknown track":  strings.Replace(goodProposal, `"structured"`, `"poetry"`, 1),
		"fraction":       strings.Replace(goodProposal, `"max_tokens":512`, `"max_tokens":512.5`, 1),
		"timeout":        strings.Replace(goodProposal, `"timeout_s":60`, `"timeout_s":600`, 1),
		"empty why":      strings.Replace(goodProposal, `"Two bands check recall and JSON under a short and a long prompt."`, `" "`, 1),
	}
	for name, text := range bad {
		if _, _, err := parseProposal(text, 32768); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// TestPrivateURL checks that a proposal never leaves the operator's
// networks.
func TestPrivateURL(t *testing.T) {
	for _, u := range []string{"http://127.0.0.1:8000", "http://192.168.50.121:8097", "http://100.121.248.70:8000", "http://[::1]:8000", "http://localhost:8000"} {
		if err := privateURL(context.Background(), u); err != nil {
			t.Errorf("%s: %v", u, err)
		}
	}
	for _, u := range []string{"http://8.8.8.8", "https://1.1.1.1/v1", "ftp://127.0.0.1", "not a url"} {
		if err := privateURL(context.Background(), u); err == nil {
			t.Errorf("%s: accepted", u)
		}
	}
}

// TestProposeAgainstALocalModel checks a whole proposal: the prompt names
// the limit of the server and the answer becomes the command.
func TestProposeAgainstALocalModel(t *testing.T) {
	var asked string
	reply := goodProposal
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"m","max_model_len":32768}]}`)
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages    []Message `json:"messages"`
			Temperature float64   `json:"temperature"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		asked = req.Messages[1].Content
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": reply}}}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := Propose(context.Background(), Options{URL: srv.URL}, "Does recall hold at long context?")
	if !p.OK || p.Options == nil || p.Options.Models[0] != "m" {
		t.Fatalf("proposal %+v", p)
	}
	if !strings.Contains(asked, "server context limit 32768") || !strings.Contains(asked, "Does recall hold") {
		t.Errorf("the prompt does not carry the goal and the limit:\n%s", asked)
	}
	want := "bernie bench eval --url " + srv.URL + " --model m --suite smoke --context 2048,8192 --tracks recall,structured --seed 7 --max-tokens 512 --timeout 60s"
	if p.Command != want {
		t.Errorf("command\n %s\nwant\n %s", p.Command, want)
	}

	reply = "```json\n" + goodProposal + "\n```"
	p = Propose(context.Background(), Options{URL: srv.URL}, "Does recall hold?")
	if p.OK || !strings.Contains(p.Error, "strict JSON") || p.Response != reply {
		t.Errorf("a fenced answer was accepted: %+v", p)
	}
}
