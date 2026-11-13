package api

import (
	"errors"
	"net/http"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/onboarding"
)

// smokeRequest is the body of POST /api/smoke: the prompt to send and the
// run to send it to.
type smokeRequest struct {
	Prompt string `json:"prompt"`
	Run    string `json:"run"`
	Model  string `json:"model"`
}

// POST /api/smoke — one prompt to a run of this host: the "Try it" box of
// the Setup tab. The target is the run that is
// serving, or the run named by the body. The answer carries the text of the
// reply, the time to the first token, the number of tokens and the output
// speed.
//
// A name that is not serving answers 404. A run that does not answer answers
// 502 with the reason.
func (h *handler) postSmoke(w http.ResponseWriter, r *http.Request) {
	var req smokeRequest
	if !httpjson.Decode(w, r, &req) {
		return
	}
	target, err := app.SmokeTarget(r.Context(), req.Run)
	if err != nil {
		smokeError(w, err)
		return
	}
	if !app.ServerTarget(r.Context(), target) {
		httpjson.Error(w, http.StatusBadRequest, "the smoke check only talks to a server of this host or an attached one")
		return
	}
	res, err := app.Smoke(r.Context(), app.SmokeOptions{URL: target, Model: req.Model, Prompt: req.Prompt})
	if err != nil {
		smokeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, res)
}

// smokeError maps the failure of the smoke check to a status: a run that is
// not serving is a 404, a server that does not answer is a 502.
func smokeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, onboarding.ErrNoLiveRun), errors.Is(err, onboarding.ErrRunGone),
		errors.Is(err, onboarding.ErrNoRun):
		httpjson.Error(w, http.StatusNotFound, err.Error())
	default:
		httpjson.Error(w, http.StatusBadGateway, err.Error())
	}
}
