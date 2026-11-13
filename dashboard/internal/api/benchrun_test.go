package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/bench/job"
	"github.com/opengfx1030/bernie/internal/serving/bench"
)

// benchHandler returns the API routes with a state directory of its own.
func benchHandler(t *testing.T) http.Handler {
	t.Helper()
	// The job of an earlier test may still hold the benchmark lock of the
	// process; a job started now would end at once as busy.
	for deadline := time.Now().Add(10 * time.Second); app.BenchRunning(); {
		if time.Now().After(deadline) {
			t.Fatal("the benchmark of an earlier test still runs")
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	mux := http.NewServeMux()
	Register(mux, Options{})
	return mux
}

// TestBenchRunStartsAJob checks that a run starts a job and that the job
// document names it.
func TestBenchRunStartsAJob(t *testing.T) {
	h := benchHandler(t)
	w := post(t, h, "/api/bench/run", `{"url":"http://127.0.0.1:1","suite":"smoke"}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("the bench run: %d %s", w.Code, w.Body.String())
	}
	var started map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &started); err != nil {
		t.Fatalf("the answer is not JSON: %v\n%s", err, w.Body.String())
	}
	id := started["id"]
	if !strings.HasPrefix(id, "b-") {
		t.Fatalf("the job id is %q", id)
	}
	w = get(t, h, "/api/bench/"+id)
	if w.Code != http.StatusOK {
		t.Fatalf("the job: %d %s", w.Code, w.Body.String())
	}
	var doc struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Phase    string `json:"phase"`
		Done     int    `json:"done"`
		Total    int    `json:"total"`
		ResultID string `json:"result_id"`
		Event    struct {
			Kind string `json:"kind"`
		} `json:"event"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("the job is not JSON: %v\n%s", err, w.Body.String())
	}
	if doc.ID != id {
		t.Errorf("the job is %q, want %q", doc.ID, id)
	}
	switch doc.Status {
	case job.StatusRunning, job.StatusDone, job.StatusFailed:
	default:
		t.Errorf("the status is %q", doc.Status)
	}
	if doc.Total != 1 {
		t.Errorf("the smoke suite has %d cells, want 1", doc.Total)
	}
}

// TestBenchRunAnswersBusy checks that a run that starts while another job
// runs answers 409 with the busy sentence.
func TestBenchRunAnswersBusy(t *testing.T) {
	h := benchHandler(t)
	// The server never answers: the job of the first request runs until
	// the test ends.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()
	w := post(t, h, "/api/bench/run", `{"url":"`+srv.URL+`"}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("the first run: %d %s", w.Code, w.Body.String())
	}
	w = post(t, h, "/api/bench/run", `{"url":"`+srv.URL+`"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("a second run: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "a benchmark is already running") {
		t.Errorf("the busy answer is %q", w.Body.String())
	}
}

// TestBenchEventsCarryTheTypedEvent checks that the stream of a job carries
// the typed event of the run: the page reads it instead of the log text.
func TestBenchEventsCarryTheTypedEvent(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	store := job.NewStore()
	j, err := store.Start(context.Background(), job.Request{Kind: job.KindRun, Total: 2,
		Run: func(ctx context.Context, p *job.Progress) (string, error) {
			p.Event(bench.Event{Kind: bench.EventCell, Index: 1, Total: 2, Level: 4})
			_, _ = p.Write([]byte("bench: c=4 done in 1.0s: 4 ok, 0 failed\n"))
			return "r1", nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	h := &handler{bench: store, log: io.Discard}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/bench/"+j.ID()+"/events", nil)
	r.SetPathValue("id", j.ID())
	h.getBenchEvents(w, r)
	body := w.Body.String()
	if !strings.Contains(body, `"kind":"cell"`) {
		t.Errorf("the stream has no typed event: %s", body)
	}
	if !strings.Contains(body, `"level":4`) {
		t.Errorf("the typed event has no payload: %s", body)
	}
	if !strings.Contains(body, "event: progress") {
		t.Errorf("the stream has no progress event: %s", body)
	}
}

// TestBenchJobNotFound checks the answer of an unknown job.
func TestBenchJobNotFound(t *testing.T) {
	h := benchHandler(t)
	if w := get(t, h, "/api/bench/nope"); w.Code != http.StatusNotFound {
		t.Errorf("an unknown job: got %d, want 404", w.Code)
	}
	if w := get(t, h, "/api/bench/nope/events"); w.Code != http.StatusNotFound {
		t.Errorf("an unknown job: got %d, want 404", w.Code)
	}
}

// TestBenchRunGuards checks the guards of the run endpoints.
func TestBenchRunGuards(t *testing.T) {
	h := benchHandler(t)
	for _, body := range []string{`{"nope":1}`, `{"concurrency":[0]}`, `{"input_len":0,"output_len":0}`} {
		if w := post(t, h, "/api/bench/run", body); w.Code != http.StatusBadRequest {
			t.Errorf("the body %q: got %d, want 400", body, w.Code)
		}
	}
	for _, body := range []string{`{"cand":"http://127.0.0.1:8001"}`, `{"base":"flashnext"}`, `{}`} {
		if w := post(t, h, "/api/bench/ab", body); w.Code != http.StatusBadRequest {
			t.Errorf("the body %q: got %d, want 400", body, w.Code)
		}
	}
	// The run only talks to a server of this host.
	for _, path := range []string{"/api/bench", "/api/bench/run"} {
		w := post(t, h, path, `{"url":"http://10.0.0.2:8001"}`)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s with a remote target: got %d, want 400", path, w.Code)
		}
	}
	w := post(t, h, "/api/bench/ab", `{"base":"flashnext","cand":"http://10.0.0.2:8001"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("a remote arm: got %d, want 400", w.Code)
	}
}

// TestBenchABStartsAJob checks that an A/B run starts a job.
func TestBenchABStartsAJob(t *testing.T) {
	h := benchHandler(t)
	w := post(t, h, "/api/bench/ab", `{"base":"http://127.0.0.1:1","cand":"http://127.0.0.1:2"}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("the A/B run: %d %s", w.Code, w.Body.String())
	}
	var started map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(started["id"], "b-") {
		t.Fatalf("the job id is %q", started["id"])
	}
}

// TestBenchCompareMarkdown checks the receipt of a comparison: the same text
// as `bernie bench compare A B --markdown`.
func TestBenchCompareMarkdown(t *testing.T) {
	h := benchHandler(t)
	fp := map[string]string{"host": "gpu-box", "gpus": "1x Radeon PRO V620", "recipe": "flash"}
	workload := bench.Workload{ProtocolID: bench.ProtocolID, SuiteID: "pr", Kind: "throughput",
		Concurrency: []int{1}}
	for _, id := range []string{"20260101T000000Z-aaaaaa", "20260101T010000Z-bbbbbb"} {
		r := &bench.Result{ID: id, Model: "m", URL: "http://127.0.0.1:8000", Fingerprint: fp,
			Workload: &workload,
			Cells: []bench.Cell{{Concurrency: 1, Group: "1k",
				Measurement: bench.Measurement{Recall: &bench.Recall{Asked: 4, Passed: 4}}}}}
		if _, err := bench.Save(r); err != nil {
			t.Fatal(err)
		}
	}
	w := get(t, h, "/api/bench/compare?a=20260101T000000Z-aaaaaa&b=20260101T010000Z-bbbbbb&format=markdown")
	if w.Code != http.StatusOK {
		t.Fatalf("the receipt: %d %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
		t.Errorf("the content type is %q", ct)
	}
	body := w.Body.String()
	for _, want := range []string{"# Benchmark receipt", "## Scope", "## Correctness", "## Verdict", "## Reproduce"} {
		if !strings.Contains(body, want) {
			t.Errorf("the receipt lacks %q:\n%s", want, body)
		}
	}
	// Without the format the answer is the verdict document.
	w = get(t, h, "/api/bench/compare?a=20260101T000000Z-aaaaaa&b=20260101T010000Z-bbbbbb")
	if w.Code != http.StatusOK {
		t.Fatalf("the verdict: %d %s", w.Code, w.Body.String())
	}
	var verdict map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &verdict); err != nil {
		t.Fatalf("the verdict is not JSON: %v", err)
	}
	if verdict["exit_code"] == nil {
		t.Errorf("the verdict lacks the exit code: %v", verdict)
	}
}
