package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/vllm"
)

// Kinds of server a run talks to.
const (
	KindOpenAI    = "openai"    // an OpenAI-compatible server: vLLM, an attached server
	KindOpenWebUI = "openwebui" // Open WebUI: its presets and pipelines are part of what is measured
)

// client speaks the chat completions API of one server.
type client struct {
	base    string // ends in /v1 (openai) or /api (openwebui)
	key     string
	timeout time.Duration
	hc      *http.Client
	// vc reads the /metrics of an openai-kind server; nil for Open WebUI.
	vc *vllm.Client
}

func newClient(o Options) *client {
	base := strings.TrimRight(o.URL, "/")
	suffix := "/v1"
	if o.Kind == KindOpenWebUI {
		suffix = "/api"
	}
	if !strings.HasSuffix(base, suffix) {
		base += suffix
	}
	// No redirect is followed: the key goes to the address the operator named.
	hc := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	c := &client{base: base, key: o.APIKey, timeout: time.Duration(o.TimeoutS) * time.Second, hc: hc}
	if o.Kind == KindOpenAI {
		c.vc = vllm.New(strings.TrimSuffix(base, "/v1"))
	}
	return c
}

type servedModel struct {
	ID          string `json:"id"`
	MaxModelLen int    `json:"max_model_len"` // vLLM only; 0 when the server does not say
}

// models lists the models of the server, with the context of each when the
// server reports it.
func (c *client) models(ctx context.Context) ([]servedModel, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var doc struct {
		Data   []servedModel `json:"data"`
		Models []servedModel `json:"models"`
	}
	if err := c.do(ctx, http.MethodGet, "/models", nil, &doc); err != nil {
		return nil, err
	}
	if len(doc.Data) > 0 {
		return doc.Data, nil
	}
	return doc.Models, nil
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// assistant is the message of the model; it goes back into the
// conversation as is when it calls tools.
type assistant struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []toolCall `json:"tool_calls,omitempty"`
}

// chat sends one chat completion and returns the message of the model and
// the telemetry of the request.
func (c *client) chat(ctx context.Context, model string, msgs []any, tools []Tool, o Options) (assistant, Call, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req := map[string]any{"model": model, "messages": msgs, "temperature": o.Temperature, "max_tokens": o.MaxTokens}
	if len(tools) > 0 {
		req["tools"], req["tool_choice"] = tools, "auto"
	}
	start := time.Now()
	var doc chatAnswer
	if err := c.do(ctx, http.MethodPost, "/chat/completions", req, &doc); err != nil {
		return assistant{}, Call{LatencyS: time.Since(start).Seconds(), Error: err.Error()}, err
	}
	call := callOf(&doc, time.Since(start).Seconds())
	if len(doc.Choices) == 0 {
		err := fmt.Errorf("%w: the answer has no choice", errModelOutput)
		call.Error = err.Error()
		return assistant{}, call, err
	}
	m := doc.Choices[0].Message
	m.Role = "assistant"
	return m, call, nil
}

// do sends one request. An answer that is not 2xx becomes an error that
// carries the status and the start of the body: the server's own words
// ("maximum context length is 8192 tokens") are the reason a case failed.
func (c *client) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("no answer within %s", c.timeout)
		}
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, errorText(data))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("the answer is not JSON: %w", err)
	}
	return nil
}

// errorText is the message of an error answer: the message field of an
// OpenAI error document, else the start of the body.
func errorText(body []byte) string {
	var doc struct {
		Error   any    `json:"error"`
		Message string `json:"message"`
		Detail  any    `json:"detail"`
	}
	if json.Unmarshal(body, &doc) == nil {
		if m, ok := doc.Error.(map[string]any); ok {
			if s, ok := m["message"].(string); ok && s != "" {
				return clip(s, 300)
			}
		}
		if s, ok := doc.Error.(string); ok && s != "" {
			return clip(s, 300)
		}
		if doc.Message != "" {
			return clip(doc.Message, 300)
		}
		if s, ok := doc.Detail.(string); ok && s != "" {
			return clip(s, 300)
		}
	}
	return clip(strings.TrimSpace(string(body)), 300)
}
