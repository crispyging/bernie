package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/opengfx1030/bernie/internal/serving/runner"
)

// TestServerTarget checks who the dashboard may talk to: this host, and a
// server on another host only once it was attached (spec §8.4).
func TestServerTarget(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	ctx := context.Background()
	// A server on another host, as `bernie attach http://chardis:8000`
	// records it. The .invalid name never resolves, so the state probe
	// fails fast.
	rec := runner.Record{Name: "chardis", Host: "chardis.invalid", Port: 8000, Foreign: true}
	b, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	dir := runner.RunDir(rec.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "run.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		target string
		want   bool
	}{
		{"http://127.0.0.1:8000", true},
		{"http://chardis.invalid:8000", true},
		{"http://chardis.invalid:8000/v1", true},
		{"http://chardis.invalid:9000", false},
		{"http://10.0.0.7:8000", false},
		{"", false},
	}
	for _, c := range cases {
		if got := ServerTarget(ctx, c.target); got != c.want {
			t.Errorf("ServerTarget(%q) = %v, want %v", c.target, got, c.want)
		}
	}
}
