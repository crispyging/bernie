package suite

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/serving/bench"
	"github.com/opengfx1030/bernie/internal/serving/vllm/vllmtest"
)

// A group that does not fit the max-model-len of the server is skipped
// with the reason and never sent; the groups that fit are measured, and a
// comparison passes over the skipped cells.
func TestRunSuiteSkipsGroupsBeyondTheServerContext(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	srv := httptest.NewServer(vllmtest.New(vllmtest.Config{Model: "fake", TTFT: ms, ChunkDelay: ms})) // max_model_len 32768
	defer srv.Close()
	s, err := Load([]byte(`schema: 1
id: fit
name: Fit
description: one group that fits and one that does not
reps: 1
warmup: 0
seed: 1
recall: false
request_timeout_s: 60
cooldown_s: 0
size_tolerance_percent: 50
defaults:
  requests: 2
compare:
  band: 0.05
  steady: 0.5
groups:
  - name: small
    kind: throughput
    input_len: 256
    output_len: 16
    concurrency: [1]
    reps: 1
    est_s: 1
  - name: huge
    kind: throughput
    input_len: 65536
    output_len: 256
    concurrency: [1, 4]
    reps: 1
    est_s: 1
`))
	if err != nil {
		t.Fatal(err)
	}
	var log strings.Builder
	r, err := Run(context.Background(), s, bench.Options{URL: srv.URL}, &log)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Cells) != 3 {
		t.Fatalf("%d cells, want 1 measured and 2 skipped", len(r.Cells))
	}
	if r.Cells[0].Skipped != "" || r.Cells[0].Completed == 0 {
		t.Errorf("the small cell: %+v", r.Cells[0])
	}
	for _, c := range r.Cells[1:] {
		if c.Group != "huge" || !strings.Contains(c.Skipped, "exceed the server's max-model-len 32768") || c.Completed != 0 {
			t.Errorf("the huge cell: group %s skipped %q completed %d", c.Group, c.Skipped, c.Completed)
		}
	}
	if !strings.Contains(log.String(), "group huge: skipped") {
		t.Errorf("the log does not say why:\n%s", log.String())
	}
	if v := Judge(&Comparison{A: r, B: r, Workload: *r.Workload}, 0); !v.Valid {
		t.Errorf("a comparison with skipped cells is invalid: %v", v.Reasons)
	}
}
