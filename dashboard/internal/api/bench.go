package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/serving/bench"
	"github.com/opengfx1030/bernie/internal/serving/bench/suite"
)

// GET /api/bench — `bernie bench list --json`.
func (h *handler) getBench(w http.ResponseWriter, r *http.Request) {
	rs, err := app.BenchList()
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, rs)
}

// GET /api/bench/suites — the built-in suites with their digest.
func (h *handler) getBenchSuites(w http.ResponseWriter, r *http.Request) {
	all, err := suite.List()
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, all)
}

// GET /api/bench/compare?a=ID&b=ID — the verdict of two saved results. With
// `format=markdown` the answer is the receipt of the comparison as markdown:
// the text of `bernie bench compare A B --markdown`.
func (h *handler) getBenchCompare(w http.ResponseWriter, r *http.Request) {
	a := r.URL.Query().Get("a")
	b := r.URL.Query().Get("b")
	if a == "" || b == "" {
		httpjson.Error(w, http.StatusBadRequest, "a and b must name two results")
		return
	}
	verdict, err := app.BenchCompare(a, b, 0)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, app.ErrBenchResultNotFound) {
			status = http.StatusNotFound
		}
		httpjson.Error(w, status, err.Error())
		return
	}
	if r.URL.Query().Get("format") == "markdown" {
		writeBenchMarkdown(w, verdict)
		return
	}
	httpjson.Write(w, http.StatusOK, verdict)
}

// benchRequest mirrors the `bernie bench` flags. Absent fields take the CLI
// defaults; concurrency is a list or a comma string ("1,8").
type benchRequest struct {
	URL         string          `json:"url"`
	Model       string          `json:"model"`
	InputLen    *int            `json:"input_len"`
	OutputLen   *int            `json:"output_len"`
	Concurrency json.RawMessage `json:"concurrency"`
	NumPrompts  int             `json:"num_prompts"`
	Warmup      *int            `json:"warmup"`
	Seed        int64           `json:"seed"`
	Tag         string          `json:"tag"`
	Recall      bool            `json:"recall"`
	// Suite names a built-in suite. It decides the cells, so the cell
	// flags of the request are ignored.
	Suite string `json:"suite"`
}

func (b benchRequest) options() (bench.Options, error) {
	o := bench.Options{URL: b.URL, Model: b.Model, Tag: b.Tag, NumPrompts: b.NumPrompts, Seed: b.Seed,
		Recall:   b.Recall,
		InputLen: bench.DefaultInputLen, OutputLen: bench.DefaultOutputLen, Warmup: bench.DefaultWarmup}
	if b.Suite != "" {
		// The suite decides the cells: the cell flags of the request do
		// not apply. The recall gate is one of them.
		o.Suite = b.Suite
		o.InputLen, o.OutputLen = 0, 0
		o.Concurrency = nil
		o.Recall = false
		return o, nil
	}
	if b.InputLen != nil {
		o.InputLen = *b.InputLen
	}
	if b.OutputLen != nil {
		o.OutputLen = *b.OutputLen
	}
	if b.Warmup != nil {
		o.Warmup = *b.Warmup
	}
	conc, err := parseConcurrency(b.Concurrency)
	if err != nil {
		return o, err
	}
	o.Concurrency = conc
	if o.InputLen < 1 || o.OutputLen < 1 {
		return o, errors.New("input_len and output_len must be positive")
	}
	if o.NumPrompts < 0 || o.Warmup < 0 {
		return o, errors.New("num_prompts and warmup must not be negative")
	}
	return o, nil
}

// benchTarget rejects a target outside this host that was not attached. An
// empty target takes the local default when the run starts.
func benchTarget(ctx context.Context, raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || !app.ServerTarget(ctx, raw) {
		return fmt.Errorf("the benchmark only targets a server of this host or an attached one: %s", raw)
	}
	return nil
}

func parseConcurrency(raw json.RawMessage) ([]int, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return []int{1}, nil
	}
	var list []int
	if err := json.Unmarshal(raw, &list); err != nil {
		var str string
		if json.Unmarshal(raw, &str) != nil {
			return nil, errors.New(`concurrency must be a list of integers or a string like "1,8"`)
		}
		for _, f := range strings.Split(str, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(f))
			if err != nil {
				return nil, fmt.Errorf("concurrency: %q is not a positive integer", f)
			}
			list = append(list, n)
		}
	}
	if len(list) == 0 {
		return []int{1}, nil
	}
	for _, n := range list {
		if n < 1 {
			return nil, fmt.Errorf("concurrency: %d is not a positive integer", n)
		}
	}
	return list, nil
}

// POST /api/bench — `bernie bench …`: runs, saves and returns the result.
// One benchmark runs at a time; a second request gets 409.
func (h *handler) postBench(w http.ResponseWriter, r *http.Request) {
	var req benchRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	o, err := req.options()
	if err != nil {
		httpjson.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := benchTarget(r.Context(), o.URL); err != nil {
		httpjson.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	var res *bench.Result
	if o.Suite != "" {
		res, _, err = app.BenchSuite(r.Context(), o.Suite, o, h.log)
	} else {
		res, _, err = app.Bench(r.Context(), o, h.log)
	}
	var saveErr *app.SaveError
	switch {
	case errors.Is(err, app.ErrBenchBusy):
		benchBusy(w, err)
	case errors.Is(err, app.ErrAllFailed):
		httpjson.Error(w, http.StatusBadGateway, err.Error()+app.FirstCellError(res))
	case errors.As(err, &saveErr):
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
	case err != nil:
		httpjson.Error(w, http.StatusBadGateway, err.Error())
	default:
		httpjson.Write(w, http.StatusOK, res)
	}
}
