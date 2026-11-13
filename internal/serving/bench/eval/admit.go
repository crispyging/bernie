package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Admission is how a case was fitted to the server before it was sent (spec
// §10.5). Only the generated distractor records are ever removed: the
// evidence, the question and the tools are never cut.
type Admission struct {
	// CountSource is "serving_tokenizer" when the server's /tokenize
	// counted the prompt, else "conservative_byte_admission": a third of
	// the bytes of the request plus 256, which stays above the 4
	// characters a token of the generated text.
	CountSource string `json:"count_source"`
	// EngineLimit is the max_model_len /tokenize reported.
	EngineLimit int `json:"engine_context_limit,omitempty"`
	// Budget is the most prompt tokens the case may have: the band, the
	// limit and the engine limit, each less the reply reservation.
	Budget int `json:"admitted_prompt_budget"`
	// PromptTokens is the count of the server's tokenizer; UpperBound is
	// the byte bound when there is no tokenizer.
	PromptTokens int `json:"actual_prompt_tokens,omitempty"`
	UpperBound   int `json:"estimated_prompt_upper_bound,omitempty"`
	// Removed is the number of distractor records taken out.
	Removed int `json:"removed_distractor_records"`
}

// admitRounds bounds the count-and-trim rounds of one case.
const admitRounds = 5

// distractor is one generated record of the padding of a case.
var distractor = regexp.MustCompile(`Archived record \d{6}: queue-\d+ sample=\d+ status=nominal\.\n`)

// counter counts the prompt tokens of a chat request and reports the
// max_model_len of the engine; ok is false when the server cannot.
type counter func(ctx context.Context, msgs []Message, tools []Tool) (n, engine int, ok bool)

// admit fits a case into the band it was built for and the context of the
// server. It returns the case to send, how it was admitted, and an error
// when the evidence itself does not fit. A server that reports no limit and
// has no tokenizer gets the case as it is.
func admit(ctx context.Context, count counter, c *Case, maxTokens, limit int) (*Case, *Admission, error) {
	out := *c
	out.Messages = append([]Message(nil), c.Messages...)
	reserve := maxTokens + 512
	a := &Admission{}
	for range admitRounds {
		n, engine, ok := count(ctx, out.Messages, out.Tools())
		if ok {
			a.CountSource, a.EngineLimit, a.PromptTokens, a.UpperBound = "serving_tokenizer", engine, n, 0
			a.Budget = c.Context
			for _, lim := range []int{limit, engine} {
				if lim > 0 {
					a.Budget = min(a.Budget, lim-reserve)
				}
			}
		} else {
			if limit <= 0 {
				return c, nil, nil
			}
			b, _ := json.Marshal(map[string]any{"messages": out.Messages, "tools": out.Tools()})
			n = len(b)/3 + 256
			a.CountSource, a.EngineLimit, a.PromptTokens, a.UpperBound = "conservative_byte_admission", 0, 0, n
			a.Budget = limit - reserve
		}
		if a.Budget <= 0 {
			return nil, a, fmt.Errorf("context admission: %d reply tokens leave no room for the prompt", maxTokens)
		}
		if n <= a.Budget {
			return &out, a, nil
		}
		removed := trimDistractors(out.Messages, min(0.95, max(0, float64(a.Budget)/float64(n)*0.9)))
		if removed == 0 {
			return nil, a, fmt.Errorf("context admission: the evidence and the tools need %d tokens, over the budget of %d; no evidence was cut", n, a.Budget)
		}
		a.Removed += removed
	}
	return nil, a, fmt.Errorf("context admission: the prompt does not fit %d tokens after %d rounds of trimming", a.Budget, admitRounds)
}

// trimDistractors keeps about keep of the distractor records of every user
// message, evenly spread, and removes the rest. It always removes at least
// one record of a message that has some. It returns how many it removed.
func trimDistractors(msgs []Message, keep float64) int {
	removed := 0
	for i, m := range msgs {
		if m.Role != "user" {
			continue
		}
		found := distractor.FindAllStringIndex(m.Content, -1)
		n := len(found)
		if n == 0 {
			continue
		}
		k := max(0, min(n-1, int(float64(n)*keep)))
		chosen := map[int]bool{}
		for j := range k {
			chosen[j*n/k] = true
		}
		var b strings.Builder
		at := 0
		for j, f := range found {
			b.WriteString(m.Content[at:f[0]])
			if chosen[j] {
				b.WriteString(m.Content[f[0]:f[1]])
			} else {
				removed++
			}
			at = f[1]
		}
		b.WriteString(m.Content[at:])
		msgs[i].Content = b.String()
	}
	return removed
}

// tokenize counts the prompt of a chat request with vLLM's /tokenize, which
// sits next to /v1, not under it. It answers ok false for a server without
// it, an Open WebUI server, or an answer it cannot read.
func (c *client) tokenize(model string, kind string) counter {
	return func(ctx context.Context, msgs []Message, tools []Tool) (int, int, bool) {
		if kind != KindOpenAI {
			return 0, 0, false
		}
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		body := map[string]any{"model": model, "messages": msgs, "add_generation_prompt": true}
		if len(tools) > 0 {
			body["tools"] = tools
		}
		root := &client{base: strings.TrimSuffix(c.base, "/v1"), key: c.key, timeout: 15 * time.Second, hc: c.hc}
		var doc struct {
			Count       *int `json:"count"`
			MaxModelLen int  `json:"max_model_len"`
		}
		if err := root.do(ctx, http.MethodPost, "/tokenize", body, &doc); err != nil || doc.Count == nil || *doc.Count < 0 {
			return 0, 0, false
		}
		return *doc.Count, doc.MaxModelLen, true
	}
}
