package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/onboarding"
	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/serving/vllm/vllmtest"
)

// attachFake registers a fake vLLM server that serves model as a run.
func attachFake(t *testing.T, name, model string) string {
	t.Helper()
	srv := httptest.NewServer(vllmtest.New(vllmtest.Config{Model: model}))
	t.Cleanup(srv.Close)
	if _, err := runner.Attach(context.Background(), runner.AttachOptions{Name: name, URL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	return srv.URL
}

func TestConnectPrintsTheRunAndItsModel(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	url := attachFake(t, "fake", "q27d")
	for _, a := range ConnectApps() {
		c, err := Connect(context.Background(), ConnectOptions{App: a})
		if err != nil {
			t.Fatalf("%s: %v", a, err)
		}
		if c.Run != "fake" || c.Model != "q27d" || c.BaseURL != url+"/v1" {
			t.Errorf("%s: %+v", a, c)
		}
		if !strings.Contains(c.Config, url+"/v1") || c.Where == "" {
			t.Errorf("%s: config does not carry the base URL:\n%s", a, c.Config)
		}
		if a != "openwebui" && !strings.Contains(c.Config, "q27d") {
			t.Errorf("%s: config does not name the model:\n%s", a, c.Config)
		}
	}
	// The opencode config is a JSON document that names the model.
	c, _ := Connect(context.Background(), ConnectOptions{App: "opencode"})
	var doc map[string]any
	if err := json.Unmarshal([]byte(c.Config), &doc); err != nil || doc["model"] != "bernie/q27d" {
		t.Errorf("opencode config: %v %v", err, doc["model"])
	}
}

func TestConnectNeedsAServer(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	if _, err := Connect(context.Background(), ConnectOptions{}); !errors.Is(err, onboarding.ErrNoLiveRun) {
		t.Errorf("err = %v, want ErrNoLiveRun", err)
	}
	if _, err := Connect(context.Background(), ConnectOptions{App: "nope"}); err == nil {
		t.Error("an unknown app was accepted")
	}
}
