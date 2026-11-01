package bench

import (
	"encoding/json"
	"net/url"
	"path/filepath"

	"github.com/opengfx1030/bernie/internal/serving/weights"
	"github.com/opengfx1030/bernie/internal/support/config"
	"github.com/opengfx1030/bernie/internal/support/paths"
)

// matchRun is the run whose listen address the URL reaches, nil if none.
func matchRun(stateDir, rawURL string) *runRecord {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	for _, r := range runRecords(stateDir) {
		if sameEndpoint(u, r) {
			return &r
		}
	}
	return nil
}

// planModel is the part of a run plan that names the model files.
type planModel struct {
	Model string            `json:"model"`
	Env   map[string]string `json:"env"`
	Args  []string          `json:"args"`
}

// modelWeights names the files of the model the run at the URL loads, with
// their hashes (spec §10). It is nil when the URL belongs to no run with a
// plan (an attached server) or the files cannot be read.
func modelWeights(stateDir, rawURL string) *weights.Model {
	r := matchRun(stateDir, rawURL)
	if r == nil || len(r.Plan) == 0 {
		return nil
	}
	var p planModel
	if json.Unmarshal(r.Plan, &p) != nil || p.Model == "" {
		return nil
	}
	revision := ""
	for i, a := range p.Args {
		if a == "--revision" && i+1 < len(p.Args) {
			revision = p.Args[i+1]
		}
	}
	hf := p.Env["HF_HOME"]
	if hf == "" {
		cfg, _ := config.Load()
		hf = cfg.ResolveHFHome()
	}
	m, err := weights.Of(p.Model, revision, hf, filepath.Join(paths.Cache(), "weights"))
	if err != nil {
		return nil
	}
	return m
}
