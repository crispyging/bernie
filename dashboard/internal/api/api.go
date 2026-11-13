// Package api serves the dashboard's JSON and server-sent event endpoints.
// Each domain file owns one area of the API from spec §11.
package api

import (
	"io"
	"net/http"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/dashboard/internal/telemetry"
)

// Telemetry is the live sample source used by GET /api/telemetry. It also
// owns the ring buffer of the samples of the last minutes, which
// GET /api/gpus/history reads.
type Telemetry interface {
	Subscribe() (<-chan []byte, func())
	History() *telemetry.History
}

// Options configures the API routes.
type Options struct {
	// Log receives progress from long-running writes. Nil discards.
	Log io.Writer
	// Telemetry supplies the shared live samples.
	Telemetry Telemetry
}

type handler struct {
	log       io.Writer
	telemetry Telemetry
	bench     *benchJobs
	// The answers that read the host are held for a short time: the page
	// reads them again and again, the host does not change in between.
	hw    *hostCache
	topo  *hostCache
	check *hostCache
	doc   *hostCache
	power *hostCache
	runs  *hostCache
	hist  *hostCache
	// fleet holds the cross-host summary for a few seconds: the page polls
	// it again and again, the peers do not change in between.
	fleet *fleetCache
}

// routes is every route of the API (spec §11): the pattern of the path and
// the handler. The router is built from this list, so the test that walks
// every route reads the same list: a route that is added later is covered
// without touching the test.
var routes = []struct {
	pattern string
	handler func(*handler, http.ResponseWriter, *http.Request)
}{
	{"GET /api/hw", (*handler).getHW},
	{"GET /api/topo", (*handler).getTopo},
	{"GET /api/topo/check", (*handler).getTopoCheck},
	{"GET /api/doctor", (*handler).getDoctor},
	{"GET /api/glossary", (*handler).getGlossary},
	{"GET /api/models", (*handler).getModels},
	{"GET /api/prefs", (*handler).getPrefs},
	{"PUT /api/prefs", (*handler).putPrefs},
	{"GET /api/report", (*handler).getReport},
	{"GET /api/recipes", (*handler).getRecipes},
	{"GET /api/recipes/{id}/plan", (*handler).getPlan},
	{"GET /api/runs", (*handler).getRuns},
	{"POST /api/runs", (*handler).postRun},
	{"POST /api/attach", (*handler).postAttach},
	{"GET /api/discover", (*handler).getDiscover},
	{"DELETE /api/runs/{name}", (*handler).deleteRun},
	{"GET /api/runs/{name}/logs", (*handler).getLogs},
	{"GET /api/runs/{name}/log", (*handler).getRunLog},
	{"GET /api/runs/{name}/findings", (*handler).getRunFindings},
	{"GET /api/telemetry", (*handler).getTelemetry},
	{"GET /api/gpus/history", (*handler).getHistory},
	{"POST /api/smoke", (*handler).postSmoke},
	{"POST /api/chat", (*handler).postChat},
	{"GET /api/power", (*handler).getPower},
	{"PUT /api/power", (*handler).putPower},
	{"GET /api/bench", (*handler).getBench},
	{"GET /api/bench/suites", (*handler).getBenchSuites},
	{"GET /api/bench/compare", (*handler).getBenchCompare},
	{"GET /api/bench/csv", (*handler).getBenchCSV},
	{"GET /api/eval", (*handler).getEvalList},
	{"GET /api/eval/{id}", (*handler).getEval},
	{"POST /api/bench", (*handler).postBench},
	{"POST /api/bench/run", (*handler).postBenchRun},
	{"POST /api/bench/ab", (*handler).postBenchAB},
	{"GET /api/bench/{id}", (*handler).getBenchJob},
	{"POST /api/bench/{id}/cancel", (*handler).postBenchCancel},
	{"DELETE /api/bench/{id}", (*handler).deleteBench},
	{"GET /api/bench/{id}/events", (*handler).getBenchEvents},
	{"GET /api/onboarding", (*handler).getOnboarding},
	{"GET /api/download/limit", (*handler).getDownloadLimit},
	{"POST /api/download/limit", (*handler).postDownloadLimit},
	{"POST /api/onboarding/{step}/run", (*handler).postOnboardingRun},
	{"POST /api/onboarding/path/run", (*handler).postOnboardingPath},
	{"POST /api/refresh", (*handler).postRefresh},
	{"GET /api/jobs", (*handler).getJobs},
	{"GET /api/jobs/{id}", (*handler).getJob},
	{"POST /api/jobs/{id}/stop", (*handler).postJobStop},
	{"GET /api/jobs/{id}/events", (*handler).getJobEvents},
	{"GET /api/setup/runs", (*handler).getSetupRuns},
	{"GET /api/setup/runs/{id}/log/{step}", (*handler).getSetupStepLog},
	{"POST /api/setup/verify", (*handler).postSetupVerify},
	{"GET /api/fleet", (*handler).getFleet},
	{"POST /api/fleet", (*handler).postFleet},
	{"DELETE /api/fleet/{name}", (*handler).deleteFleet},
	{"/api/", func(_ *handler, w http.ResponseWriter, r *http.Request) {
		httpjson.Error(w, http.StatusNotFound, "no such endpoint: "+r.Method+" "+r.URL.Path)
	}},
}

// Register adds every dashboard API route to mux.
func Register(mux *http.ServeMux, o Options) {
	if o.Log == nil {
		o.Log = io.Discard
	}
	h := &handler{
		log: o.Log, telemetry: o.Telemetry, bench: newBenchJobs(),
		hw:    newHostCache(hostTTL),
		topo:  newHostCache(hostTTL),
		check: newHostCache(checkTTL),
		doc:   newHostCache(checkTTL),
		power: newHostCache(hostTTL),
		runs:  newHostCache(runsTTL),
		hist:  newHostCache(historyTTL),
		fleet: newFleetCache(),
	}

	for _, r := range routes {
		mux.HandleFunc(r.pattern, func(w http.ResponseWriter, req *http.Request) {
			r.handler(h, w, req)
		})
	}
}
