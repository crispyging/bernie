package app

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/opengfx1030/bernie/internal/serving/bench"
	"github.com/opengfx1030/bernie/internal/serving/bench/suite"
	"github.com/opengfx1030/bernie/internal/serving/runner"
)

// ErrAllFailed means every measured request of a benchmark failed. Bench
// returns it together with the unsaved result.
var ErrAllFailed = errors.New("all requests failed; result not saved")

// SaveError is a benchmark that ran but could not be stored.
type SaveError struct{ Err error }

func (e *SaveError) Error() string { return "save result: " + e.Err.Error() }
func (e *SaveError) Unwrap() error { return e.Err }

// ErrBenchBusy means another benchmark of this process is running.
var ErrBenchBusy = errors.New("a benchmark is already running")

var benchMu sync.Mutex // one benchmark at a time per process

// BenchRunning reports whether a benchmark of this process runs now.
func BenchRunning() bool {
	if !benchMu.TryLock() {
		return true
	}
	benchMu.Unlock()
	return false
}

// Bench runs a benchmark and saves the result under <state>/bench. Progress
// lines go to progress. It returns the result and the file it was saved to.
// When no request succeeded it saves nothing and returns the result with
// ErrAllFailed. A second concurrent call returns ErrBenchBusy.
func Bench(ctx context.Context, o bench.Options, progress io.Writer) (*bench.Result, string, error) {
	if !benchMu.TryLock() {
		return nil, "", ErrBenchBusy
	}
	defer benchMu.Unlock()
	r, err := bench.Run(ctx, o, progress)
	if err != nil {
		return nil, "", err
	}
	if r.Completed() == 0 {
		return r, "", ErrAllFailed
	}
	path, err := bench.Save(r)
	if err != nil {
		return r, "", &SaveError{err}
	}
	return r, path, nil
}

// BenchSuite runs a named suite and saves the result under <state>/bench.
// The suite decides the cells, the repetitions, the warm-up and the recall
// gate; the options only point the run at a server. It returns the result
// and the file it was saved to. A second concurrent call returns
// ErrBenchBusy.
func BenchSuite(ctx context.Context, name string, o bench.Options, progress io.Writer) (*bench.Result, string, error) {
	if !benchMu.TryLock() {
		return nil, "", ErrBenchBusy
	}
	defer benchMu.Unlock()
	s, err := suite.Get(name)
	if err != nil {
		return nil, "", err
	}
	r, err := suite.Run(ctx, s, o, progress)
	if err != nil {
		return nil, "", err
	}
	if r.Completed() == 0 {
		return r, "", ErrAllFailed
	}
	path, err := bench.Save(r)
	if err != nil {
		return r, "", &SaveError{err}
	}
	return r, path, nil
}

// BenchAB runs an A/B comparison: the visits of the two arms in the ABBA
// order. It holds the bench lock for the whole run, so no other benchmark
// interleaves, and it saves the result of every visit.
func BenchAB(ctx context.Context, o suite.ABOptions) (*suite.ABResult, []string, error) {
	if !benchMu.TryLock() {
		return nil, nil, ErrBenchBusy
	}
	defer benchMu.Unlock()
	o.Starter = func(ctx context.Context, recipeID, name string) (*runner.Record, error) {
		return Serve(ctx, ServeOptions{
			Prepare: runner.PrepareOptions{Recipe: recipeID, Name: name},
			Start:   runner.StartOptions{},
		})
	}
	res, err := suite.AB(ctx, o)
	if err != nil {
		return nil, nil, err
	}
	var paths []string
	for _, v := range res.Visits {
		if v.Result == nil || v.Result.Completed() == 0 {
			continue
		}
		p, err := bench.Save(v.Result)
		if err != nil {
			return res, paths, &SaveError{err}
		}
		paths = append(paths, p)
	}
	return res, paths, nil
}

// FirstCellError is ": <message>" of the first error sample of a result, or
// "" when no cell has one.
func FirstCellError(r *bench.Result) string {
	for _, c := range r.Cells {
		if len(c.Errors) > 0 {
			return ": " + c.Errors[0]
		}
	}
	return ""
}

// BenchDelete removes a saved benchmark result. The id may be a prefix:
// the unique matching file is removed. An unknown id answers
// ErrBenchResultNotFound (declared in bench_compare.go).
func BenchDelete(id string) (string, error) {
	return bench.Delete(id)
}

// BenchList is the `bench list --json` document: saved results, never null.
func BenchList() ([]bench.Result, error) {
	rs, err := bench.List()
	if rs == nil {
		rs = []bench.Result{}
	}
	return rs, err
}
