package app

import (
	"context"
	"errors"
	"io/fs"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/host"
	"github.com/opengfx1030/bernie/internal/machine/topo"
	"github.com/opengfx1030/bernie/internal/serving/recipe"
)

// P2PChecklistOptions select the recipe whose environment the checklist
// shows.
type P2PChecklistOptions struct {
	// Recipe is the recipe id. Empty selects the best fitting recipe.
	Recipe string
}

// P2PChecklistReport is the `topo --check --json` document.
type P2PChecklistReport struct {
	// Ready is false when an item failed: the GPUs cannot copy peer to peer.
	Ready     bool             `json:"ready"`
	Summary   map[string]int   `json:"summary"`
	Checklist []topo.CheckItem `json:"checklist"`
}

// P2PChecklist builds the P2P readiness checklist of this host.
func P2PChecklist(ctx context.Context, o P2PChecklistOptions) (P2PChecklistReport, error) {
	vis, err := gpu.Visible(root)
	if gpu.Restricted(err) {
		return P2PChecklistReport{}, err
	}
	gpus := vis.GPUs
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return P2PChecklistReport{}, err
		}
		gpus = []gpu.GPU{}
	}
	h, _ := host.Probe(ctx, root)
	env, err := recipeEnv(recipe.Load(), o.Recipe, gpus)
	if err != nil {
		return P2PChecklistReport{}, err
	}
	items, err := topo.P2PChecklist(root, gpus, topo.P2POptions{
		Cmdline: h.Cmdline, PkgManager: h.PkgManager, Recipe: env,
	})
	if err != nil {
		return P2PChecklistReport{}, err
	}
	rep := P2PChecklistReport{
		Ready:     true,
		Summary:   map[string]int{"ok": 0, "warn": 0, "fail": 0, "skip": 0},
		Checklist: items,
	}
	for _, it := range items {
		rep.Summary[string(it.Status)]++
		if it.Status == topo.P2PFail {
			rep.Ready = false
		}
	}
	return rep, nil
}

// recipeEnv resolves the environment of a recipe. An empty id selects the
// best fitting recipe; no recipe fits gives an empty environment.
func recipeEnv(set recipe.Set, id string, gpus []gpu.GPU) (topo.RecipeEnv, error) {
	if id == "" {
		id = recipe.Best(recipe.List(set, fitHost(gpus), false))
		if id == "" {
			return topo.RecipeEnv{}, nil
		}
	}
	return recipe.Env(set, id)
}
