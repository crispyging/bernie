package app

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/serving/vllm/vllmtest"
)

func TestChatPrintsTheAnswerAndItsNumbers(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	attachFake(t, "fake", "q27d")
	var streamed strings.Builder
	res, err := Chat(context.Background(), "", "What is 2+2?", 64, func(d ChatDelta) error {
		streamed.WriteString(d.Text)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Run != "fake" || res.Model != "q27d" || res.Reply == "" || res.OutputTokens == 0 {
		t.Errorf("answer = %+v", res)
	}
	if streamed.String() != res.Reply {
		t.Errorf("streamed %q, reply %q", streamed.String(), res.Reply)
	}
}

// A server that answers /health but no completion is starting: chat
// refuses it instead of waiting on it.
func TestChatRefusesARunThatIsNotReady(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	srv := httptest.NewServer(vllmtest.New(vllmtest.Config{FailEvery: 1}))
	defer srv.Close()
	if _, err := runner.Attach(context.Background(), runner.AttachOptions{Name: "cold", URL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	_, err := Chat(context.Background(), "cold", "hi", 8, nil)
	if !errors.Is(err, ErrNotReady) || !strings.Contains(err.Error(), "cold is starting") {
		t.Errorf("err = %v, want ErrNotReady for cold", err)
	}
	if _, err := Chat(context.Background(), "cold", "  ", 8, nil); err == nil {
		t.Error("an empty prompt was sent")
	}
}
