package suite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/bench"
	"github.com/opengfx1030/bernie/internal/serving/vllm"
)

// TemplateTokens is the allowance of the chat template and the special
// tokens on top of the prompt of a cell.
const TemplateTokens = 256

// contextLimit is the max_model_len of the model the run measures, read
// from the models list of the server (vLLM). 0 means the server does not
// say, and every group is sent.
func contextLimit(ctx context.Context, o bench.Options) int {
	url := o.URL
	if url == "" {
		url = bench.DefaultURL()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, vllm.New(url).BaseURL()+"/v1/models", nil)
	if err != nil {
		return 0
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	var doc struct {
		Data []struct {
			ID          string `json:"id"`
			MaxModelLen int    `json:"max_model_len"`
		} `json:"data"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&doc) != nil || len(doc.Data) == 0 {
		return 0
	}
	for _, m := range doc.Data {
		if m.ID == o.Model {
			return m.MaxModelLen
		}
	}
	return doc.Data[0].MaxModelLen
}

// skipReason says why a group cannot run on a server of this context, or
// "" when it fits.
func skipReason(g Group, limit int) string {
	need := g.InputLen + g.OutputLen + TemplateTokens
	if limit <= 0 || need <= limit {
		return ""
	}
	return fmt.Sprintf("%d prompt + %d output + %d template tokens exceed the server's max-model-len %d",
		g.InputLen, g.OutputLen, TemplateTokens, limit)
}

// skippedCells are the cells of a group that was never sent.
func skippedCells(g Group, reason string) []bench.Cell {
	out := make([]bench.Cell, 0, len(g.Concurrency))
	for _, c := range g.Concurrency {
		out = append(out, bench.Cell{Concurrency: c, Group: g.Name, InputLen: g.InputLen, OutputLen: g.OutputLen,
			Kind: g.Kind, Skipped: reason})
	}
	return out
}
