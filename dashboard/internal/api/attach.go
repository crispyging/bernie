package api

import (
	"errors"
	"net/http"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/serving/runner"
)

// attachRequest is the body of POST /api/attach: the URL (or port) of an
// already running vLLM server, the name to register it under and the
// recipe it serves (optional).
type attachRequest struct {
	URL    string `json:"url"`
	Name   string `json:"name"`
	Recipe string `json:"recipe"`
}

// POST /api/attach — register an already running vLLM server as a run.
// The health check, the metrics and the models list are checked first. The
// server is never started or stopped by bernie: `bernie stop` on an
// attached run only forgets it (R29).
func (h *handler) postAttach(w http.ResponseWriter, r *http.Request) {
	var req attachRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	if req.URL == "" {
		httpjson.Error(w, http.StatusBadRequest, "url must name the running server (a URL or a port)")
		return
	}
	if req.Name == "" {
		req.Name = "attached"
	}
	res, err := app.Attach(r.Context(), app.AttachOptions{URL: req.URL, Name: req.Name, Recipe: req.Recipe})
	if err != nil {
		var ae *runner.AttachError
		status := http.StatusBadRequest
		if errors.As(err, &ae) {
			// A server that does not answer is a bad gateway: the
			// request was fine, the server was not.
			status = http.StatusBadGateway
		}
		httpjson.Error(w, status, err.Error())
		return
	}
	httpjson.Write(w, http.StatusCreated, res)
}

// GET /api/discover — the vLLM servers that run on this host, processes and
// containers, each with its health and the run that records it (spec
// §8.5). It only reads.
func (h *handler) getDiscover(w http.ResponseWriter, r *http.Request) {
	found, err := app.Discover(r.Context())
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, map[string]any{"servers": found})
}
