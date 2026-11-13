package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/dashboard/internal/sse"
	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/bench/job"
	"github.com/opengfx1030/bernie/internal/serving/bench"
	"github.com/opengfx1030/bernie/internal/serving/bench/suite"
)

// benchPoll is the delay between two reads of a running benchmark.
const benchPoll = 300 * time.Millisecond

// benchJobs is the store of the benchmark jobs of this process. The store
// lives in internal/bench/job: it owns the jobs, their state machine and
// their typed events.
type benchJobs = job.Store

// newBenchJobs returns an empty store of benchmark jobs.
func newBenchJobs() *benchJobs { return job.NewStore() }

// POST /api/bench/run — the same body as POST /api/bench, but the run
// happens in the background: the answer is the id of the job. The state of
// the run is GET /api/bench/{id}, its events are GET /api/bench/{id}/events.
// One benchmark job runs at a time; a job that starts while another runs
// answers 409.
func (h *handler) postBenchRun(w http.ResponseWriter, r *http.Request) {
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
	j, err := h.bench.Start(r.Context(), job.Request{Kind: job.KindRun, Options: o})
	if err != nil {
		benchFailed(w, err)
		return
	}
	httpjson.Write(w, http.StatusAccepted, map[string]string{"id": j.ID()})
}

// abRequest is the body of POST /api/bench/ab: the two arms of the run.
type abRequest struct {
	Base       string `json:"base"`
	Cand       string `json:"cand"`
	Suite      string `json:"suite"`
	Model      string `json:"model"`
	NumPrompts int    `json:"num_prompts"`
	Seed       int64  `json:"seed"`
	Tag        string `json:"tag"`
}

// POST /api/bench/ab — a baseline and a candidate, one after the other: the
// same run as `bernie bench ab`. The answer is the id of the job. One
// benchmark job runs at a time; a job that starts while another runs
// answers 409.
func (h *handler) postBenchAB(w http.ResponseWriter, r *http.Request) {
	var req abRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	if req.Base == "" || req.Cand == "" {
		httpjson.Error(w, http.StatusBadRequest, "base and cand must name the two arms of the run")
		return
	}
	for _, arm := range []string{req.Base, req.Cand} {
		if strings.Contains(arm, "://") {
			if err := benchTarget(r.Context(), arm); err != nil {
				httpjson.Error(w, http.StatusBadRequest, err.Error())
				return
			}
		}
	}
	o := suite.ABOptions{
		Base: req.Base, Cand: req.Cand, Suite: req.Suite,
		Options: bench.Options{Model: req.Model, NumPrompts: req.NumPrompts, Seed: req.Seed, Tag: req.Tag},
	}
	j, err := h.bench.Start(r.Context(), job.Request{Kind: job.KindAB, AB: o})
	if err != nil {
		benchFailed(w, err)
		return
	}
	httpjson.Write(w, http.StatusAccepted, map[string]string{"id": j.ID()})
}

// POST /api/bench/{id}/cancel — stop the running benchmark of this id.
// A job that already finished answers with `cancelled=false`; a job of
// another process answers 404. The cancelled run ends as failed and is
// marked incomplete: the partial result, when one was saved, stays for
// the record (R30).
func (h *handler) postBenchCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	j, ok := h.bench.Get(id)
	if !ok {
		httpjson.Error(w, http.StatusNotFound, "no benchmark run named "+strconv.Quote(id))
		return
	}
	cancelled := h.bench.Cancel(id)
	httpjson.Write(w, http.StatusOK, map[string]any{"id": id, "cancelled": cancelled, "status": j.Status()})
}

// DELETE /api/bench/{id} — remove a saved benchmark result with
// confirmation. The page that calls it reads the run first and asks the
// reader to confirm; the body is `{"confirm": true}` to make a stray
// click harmless (R30).
func (h *handler) deleteBench(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm bool `json:"confirm"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if !body.Confirm {
		httpjson.Error(w, http.StatusBadRequest, "pass {\"confirm\": true} to delete a saved result")
		return
	}
	id := r.PathValue("id")
	deleted, err := app.BenchDelete(id)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, app.ErrBenchResultNotFound) {
			status = http.StatusNotFound
		}
		httpjson.Error(w, status, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, map[string]any{"id": id, "deleted": deleted})
}

// GET /api/bench/{id} — the state of a benchmark run.
func (h *handler) getBenchJob(w http.ResponseWriter, r *http.Request) {
	j, ok := h.bench.Get(r.PathValue("id"))
	if !ok {
		httpjson.Error(w, http.StatusNotFound, "no benchmark run named "+strconv.Quote(r.PathValue("id")))
		return
	}
	httpjson.Write(w, http.StatusOK, j.Doc())
}

// GET /api/bench/{id}/events — the progress of a benchmark run: one "log"
// event per line, a "progress" event with the job document (with the typed
// event of the run) and a last "done" or "failed" event with the job
// document.
func (h *handler) getBenchEvents(w http.ResponseWriter, r *http.Request) {
	j, ok := h.bench.Get(r.PathValue("id"))
	if !ok {
		httpjson.Error(w, http.StatusNotFound, "no benchmark run named "+strconv.Quote(r.PathValue("id")))
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	st := sse.New(w)
	go sse.Heartbeat(ctx, st, cancel)
	sent := 0
	for {
		lines, doc, status := j.State()
		for ; sent < len(lines); sent++ {
			if st.Event("log", lines[sent]) != nil {
				return
			}
		}
		b, err := json.Marshal(doc)
		if err != nil {
			_ = st.Event("error", sse.OneLine(err.Error()))
			return
		}
		if st.Event("progress", string(b)) != nil {
			return
		}
		if status != job.StatusRunning {
			name := job.StatusDone
			if status == job.StatusFailed {
				name = job.StatusFailed
			}
			_ = st.Event(name, string(b))
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(benchPoll):
		}
	}
}

// benchBusy answers a busy request: 409 with the sentence of the busy
// answer of a benchmark. Every entry that starts a benchmark uses it.
func benchBusy(w http.ResponseWriter, err error) {
	httpjson.Error(w, http.StatusConflict, err.Error())
}

// benchFailed answers a benchmark that could not start: 409 when a
// benchmark runs already, 500 otherwise.
func benchFailed(w http.ResponseWriter, err error) {
	if errors.Is(err, app.ErrBenchBusy) {
		benchBusy(w, err)
		return
	}
	httpjson.Error(w, http.StatusInternalServerError, err.Error())
}

// writeBenchMarkdown writes the receipt of a comparison as markdown.
func writeBenchMarkdown(w http.ResponseWriter, v *suite.Verdict) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, suite.Receipt(v))
}
