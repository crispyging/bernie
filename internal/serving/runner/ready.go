package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/vllm"
)

// answerTimeout bounds the one-token completion of the ready check.
const answerTimeout = 10 * time.Second

// answered makes sure the run answered a one-token completion once. The
// first answer is recorded in <run dir>/answered with the start time of the
// run, so a later poll does not ask again and a new run with the same name
// asks afresh.
func answered(ctx context.Context, r *Record) error {
	mark := filepath.Join(RunDir(r.Name), "answered")
	stamp := r.StartedAt.UTC().Format(time.RFC3339Nano)
	if b, err := os.ReadFile(mark); err == nil && string(b) == stamp {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, answerTimeout)
	defer cancel()
	c := vllm.New(r.URL())
	model := servedModel(r)
	if model == "" {
		ids, err := c.Models(ctx)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return errors.New("/v1/models lists no model")
		}
		model = ids[0]
	}
	if _, err := c.CompleteStream(ctx, model, "Hello", 1); err != nil {
		return err
	}
	_ = os.WriteFile(mark, []byte(stamp), 0o644) // a failed write asks again next time
	return nil
}

// servedModel is the model name of the run's plan, "" when the run has no
// plan (an attached server).
func servedModel(r *Record) string {
	if r.Plan == nil {
		return ""
	}
	if r.Plan.ServedName != "" {
		return r.Plan.ServedName
	}
	return r.Plan.Model
}
