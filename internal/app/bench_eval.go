package app

import (
	"context"
	"io"
	"os"

	"github.com/opengfx1030/bernie/internal/serving/bench"
	"github.com/opengfx1030/bernie/internal/serving/bench/eval"
)

// EvalKeyEnv names the variable that holds the API key of the evaluated
// server. The key is never a flag, so it stays out of the shell history,
// and it is never saved in a result.
const EvalKeyEnv = "BERNIE_BENCH_API_KEY"

// BenchEval runs the quality evaluation of spec §10.5 and saves the result
// under <state>/bench/eval. An empty URL is the server of the only run,
// else the default of bernie bench. It shares the lock of the benchmark:
// a speed run and an evaluation never load the server at the same time.
func BenchEval(ctx context.Context, o eval.Options, progress io.Writer) (*eval.Result, string, error) {
	if !benchMu.TryLock() {
		return nil, "", ErrBenchBusy
	}
	defer benchMu.Unlock()
	if o.URL == "" {
		o.URL = bench.DefaultURL()
	}
	if o.APIKey == "" {
		o.APIKey = os.Getenv(EvalKeyEnv)
	}
	r, err := eval.Run(ctx, o, progress)
	if err != nil {
		return nil, "", err
	}
	path, err := eval.Save(r)
	if err != nil {
		return r, "", &SaveError{err}
	}
	return r, path, nil
}

// BenchEvalPropose asks the model of a private server to suggest an eval
// run for a goal (spec §10.5). It sends one chat request and starts
// nothing: the proposal carries the command to run. An empty URL is the
// server of the only run, else the default of bernie bench.
func BenchEvalPropose(ctx context.Context, o eval.Options, goal string) *eval.Proposal {
	if o.URL == "" {
		o.URL = bench.DefaultURL()
	}
	if o.APIKey == "" {
		o.APIKey = os.Getenv(EvalKeyEnv)
	}
	return eval.Propose(ctx, o, goal)
}
