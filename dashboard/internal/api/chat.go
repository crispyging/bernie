package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/dashboard/internal/sse"
	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/onboarding"
)

const (
	// maxChatPromptBytes caps the prompt of one chat request.
	maxChatPromptBytes = 8 << 10
	// maxChatReplyBytes caps the answer that is relayed to the page.
	maxChatReplyBytes = 256 << 10
	// chatMaxTokens caps the answer of the server.
	chatMaxTokens = 512
)

// chatBusy is set while an answer runs: one answer at a time.
var chatBusy atomic.Bool

// errChatTooLong ends the stream when the answer passes the cap.
var errChatTooLong = errors.New("the answer is longer than the limit")

// chatRequest is the body of POST /api/chat: the prompt to send.
type chatRequest struct {
	Prompt string `json:"prompt"`
	// Run names the server the dashboard shows; empty or gone means the
	// first server that serves.
	Run string `json:"run"`
}

// POST /api/chat — one prompt to the run that is serving, and the answer as
// a stream of events: "delta" (the text of the answer), "reasoning" (the
// reasoning of the model, when it sends one), "done" (the numbers of the
// answer) and "error".
//
// The target is the run that is serving: the body names no server. Nothing
// serves answers 503. A target that is neither on this host nor attached
// answers 400. A prompt that is empty or too long answers 400. An answer
// that is already running answers 409. A server that does not answer
// answers 502 with the reason.
func (h *handler) postChat(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		httpjson.Error(w, http.StatusBadRequest, "the prompt is empty")
		return
	}
	if len(req.Prompt) > maxChatPromptBytes {
		httpjson.Error(w, http.StatusBadRequest, fmt.Sprintf("the prompt is longer than %d bytes", maxChatPromptBytes))
		return
	}
	if !chatBusy.CompareAndSwap(false, true) {
		httpjson.Error(w, http.StatusConflict, "an answer is already running")
		return
	}
	defer chatBusy.Store(false)
	target, err := app.ChatTarget(r.Context(), req.Run)
	if err != nil {
		chatError(w, err)
		return
	}
	if !app.ServerTarget(r.Context(), target) {
		httpjson.Error(w, http.StatusBadRequest, "the chat only talks to a server of this host or an attached one")
		return
	}
	// The answer lives in the context of the request: it ends when the page
	// goes away. The heartbeat watches the connection and cancels it when the
	// reader is gone, so the server does not answer into a dead socket.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	stream, err := app.ChatStream(ctx, app.ChatOptions{
		URL: target, Prompt: req.Prompt, MaxTokens: chatMaxTokens,
	})
	if err != nil {
		chatError(w, err)
		return
	}
	defer stream.Close()

	out := sse.New(w)
	go sse.Heartbeat(ctx, out, cancel)
	sent := 0
	res, err := stream.Read(func(d app.ChatDelta) error {
		sent += len(d.Text)
		if sent > maxChatReplyBytes {
			return errChatTooLong
		}
		if d.Reasoning != "" {
			if e := out.Event("reasoning", chatEvent(map[string]string{"text": d.Reasoning})); e != nil {
				return e
			}
		}
		if d.Text != "" {
			if e := out.Event("delta", chatEvent(map[string]string{"text": d.Text})); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		msg := err.Error()
		if errors.Is(err, errChatTooLong) {
			msg = fmt.Sprintf("the answer is longer than %d bytes", maxChatReplyBytes)
		}
		_ = out.Event("error", chatEvent(map[string]string{"error": msg}))
		return
	}
	_ = out.Event("done", chatEvent(map[string]any{
		"ttft_ms":       res.TTFT.Milliseconds(),
		"output_tokens": res.Tokens,
		"tok_s":         res.TokS,
	}))
}

// chatEvent writes the payload of one event: a JSON object on one line.
func chatEvent(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"error":"the event cannot be written"}`
	}
	return string(b)
}

// chatError maps the failure of a chat request to a status: nothing that
// serves is a 503, a server that does not answer is a 502.
func chatError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, onboarding.ErrNoLiveRun):
		httpjson.Error(w, http.StatusServiceUnavailable, "no server is running on this host")
	default:
		httpjson.Error(w, http.StatusBadGateway, err.Error())
	}
}
