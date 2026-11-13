package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/opengfx1030/bernie/internal/onboarding"
	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/serving/vllm"
)

// ChatDelta is one piece of a streamed chat answer: the text of the answer
// and, when the model sends one, the reasoning behind it.
type ChatDelta = vllm.ChatDelta

// ChatResult carries the numbers of a streamed chat answer.
type ChatResult = vllm.ChatResult

// ChatOptions are the inputs of one chat request.
type ChatOptions struct {
	URL       string // base URL of the server; "" = the running run
	Model     string // model name; "" = the first id of /v1/models
	Prompt    string // the prompt to send
	MaxTokens int    // the cap on the answer; 0 = the default of the server
}

// ChatTarget returns the URL of the run a chat request talks to: the run
// named name when it serves (the server the dashboard shows), else the
// first run that serves. It fails with onboarding.ErrNoLiveRun when
// nothing serves.
func ChatTarget(ctx context.Context, name string) (string, error) {
	if name != "" {
		if run, err := onboarding.RunByName(ctx, name); err == nil {
			return run.URL, nil
		}
	}
	run, err := onboarding.LiveRun(ctx)
	if err != nil {
		return "", err
	}
	return run.URL, nil
}

// ChatStream opens a streamed chat answer from a server: the answer comes
// piece by piece while the server writes it.
func ChatStream(ctx context.Context, o ChatOptions) (*vllm.ChatStream, error) {
	url := o.URL
	if url == "" {
		run, err := onboarding.LiveRun(ctx)
		if err != nil {
			return nil, err
		}
		url = run.URL
	}
	return vllm.New(url).ChatStream(ctx, o.Model, o.Prompt, o.MaxTokens)
}

// ChatAnswer is the `chat --json` document: one prompt, the answer and its
// numbers (spec §3.2).
type ChatAnswer struct {
	Run       string `json:"run"`
	Model     string `json:"model"`
	Reply     string `json:"reply"`
	Reasoning string `json:"reasoning,omitempty"`
	// TTFTMS is the time to the first token of the answer, in milliseconds.
	TTFTMS       int     `json:"ttft_ms"`
	OutputTokens int     `json:"output_tokens"`
	TokS         float64 `json:"tok_s"`
}

// ErrNotReady means the run exists but does not answer requests yet.
var ErrNotReady = errors.New("the run is not ready")

// Chat sends one prompt to a run (by name, else the run that serves) and
// streams the answer to onDelta (nil = no stream). A run that is not
// running yet is ErrNotReady: chat proves that the server answers.
func Chat(ctx context.Context, run, prompt string, maxTokens int, onDelta func(ChatDelta) error) (*ChatAnswer, error) {
	if strings.TrimSpace(prompt) == "" {
		return nil, errors.New("the prompt is empty")
	}
	var r *onboarding.Run
	var err error
	if run != "" {
		r, err = onboarding.RunByName(ctx, run)
	} else {
		r, err = onboarding.LiveRun(ctx)
	}
	if err != nil {
		return nil, err
	}
	if r.State != runner.StateRunning {
		return nil, fmt.Errorf("%w: %s is %s; see: bernie ps", ErrNotReady, r.Name, r.State)
	}
	s, err := vllm.New(r.URL).ChatStream(ctx, "", prompt, maxTokens)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	if onDelta == nil {
		onDelta = func(ChatDelta) error { return nil }
	}
	res, err := s.Read(onDelta)
	if err != nil {
		return nil, err
	}
	return &ChatAnswer{
		Run: r.Name, Model: s.Model(), Reply: res.Text, Reasoning: res.Reasoning,
		TTFTMS: int(res.TTFT.Milliseconds()), OutputTokens: res.Tokens, TokS: res.TokS,
	}, nil
}
