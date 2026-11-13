package api

import (
	"io"
	"net/http"
	"strings"

	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/internal/serving/bench"
	"github.com/opengfx1030/bernie/internal/serving/bench/eval"
	"github.com/opengfx1030/bernie/internal/support/redact"
)

// GET /api/eval — `bernie bench eval list --json`: the saved eval results,
// newest first, without their cases.
func (h *handler) getEvalList(w http.ResponseWriter, r *http.Request) {
	rs, err := eval.List()
	if err != nil {
		httpjson.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, rs)
}

// GET /api/eval/{id} — `bernie bench eval show ID --json`. With
// `format=csv` the answer is `bernie bench eval show ID --csv`.
func (h *handler) getEval(w http.ResponseWriter, r *http.Request) {
	res, err := eval.Get(r.PathValue("id"))
	if err != nil {
		httpjson.Error(w, http.StatusNotFound, err.Error())
		return
	}
	if r.URL.Query().Get("format") == "csv" {
		writeCSV(w, res.ID, eval.CSV(res))
		return
	}
	httpjson.Write(w, http.StatusOK, res)
}

// GET /api/bench/csv?id=ID — `bernie bench show ID --csv`.
func (h *handler) getBenchCSV(w http.ResponseWriter, r *http.Request) {
	res, err := bench.Get(r.URL.Query().Get("id"))
	if err != nil {
		httpjson.Error(w, http.StatusNotFound, err.Error())
		return
	}
	writeCSV(w, res.ID, bench.CSV(res))
}

// writeCSV sends a CSV download. The text goes through the redaction of
// every answer, like the JSON ones.
func writeCSV(w http.ResponseWriter, id string, body []byte) {
	name := strings.Map(func(r rune) rune {
		if r == '"' || r == '\\' || r < ' ' {
			return '_'
		}
		return r
	}, id)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, redact.Text(string(body)))
}
