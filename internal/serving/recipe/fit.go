package recipe

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
)

// Fit statuses.
const (
	FitYes     = "fits"
	FitNo      = "no"
	FitUnknown = "unknown"
	// FitTight is a verdict only: the recipe fits, with a cost the reasons
	// name (a shorter context, a nearly full disk).
	FitTight = "tight"
)

// Fit is how a recipe fits this host (spec §7.8). Status is fits, no or
// unknown. Verdict is the same, except that a recipe that fits with a cost
// is tight. Modes are the modes the recipe supports (host always;
// container when it names an image).
type Fit struct {
	Status  string   `json:"status"`
	Verdict string   `json:"verdict"`
	Reasons []string `json:"reasons"`
	Modes   []string `json:"modes"`
}

// HostFacts is what the fit reads about the host besides its GPUs. A zero
// field skips its check. Cached reports a model already in the Hugging
// Face cache; nil means none is.
type HostFacts struct {
	RAMBytes      uint64
	DiskFreeBytes uint64
	Cached        func(source string) bool
}

// diskSlack is the share of free disk a download may fill before the fit
// says tight.
const diskSlack = 0.9

// CheckFit compares the recipe's requires block with the host. gpus == nil
// means no GPU data (unknown).
func CheckFit(r *Recipe, gpus []gpu.GPU, h HostFacts) Fit {
	f := Fit{Status: FitYes, Reasons: []string{}, Modes: Modes(r)}
	rq := r.Requires
	var tight []string
	if gpus == nil {
		f.Status = FitUnknown
		f.Reasons = append(f.Reasons, "no GPU data")
	} else {
		f.Reasons = append(f.Reasons, gpuReasons(rq, gpus)...)
		if len(f.Reasons) == 0 {
			vram := minVRAM(gpus, func(g gpu.GPU) bool { return gpu.ArchMatch(rq.Arch, g.GFX) })
			t, no := contextReasons(r, vram)
			tight = append(tight, t...)
			f.Reasons = append(f.Reasons, no...)
		}
	}
	if rq.RAMGiB > 0 && h.RAMBytes > 0 && float64(h.RAMBytes) < rq.RAMGiB*gib {
		f.Reasons = append(f.Reasons, fmt.Sprintf("ram: needs %g GiB host RAM; host has %.0f GiB", rq.RAMGiB, float64(h.RAMBytes)/gib))
	}
	t, no := diskReasons(r, h)
	tight = append(tight, t...)
	f.Reasons = append(f.Reasons, no...)
	if len(f.Reasons) > 0 && f.Status == FitYes {
		f.Status = FitNo
	}
	if f.Status == FitUnknown && len(f.Reasons) > 1 {
		f.Status = FitNo // RAM or disk alone already rules it out
	}
	f.Verdict = f.Status
	if f.Status == FitYes && len(tight) > 0 {
		f.Verdict = FitTight
		f.Reasons = tight
	}
	return f
}

// gpuReasons are the reasons the GPUs rule the recipe out: arch, count,
// VRAM per GPU.
func gpuReasons(rq Requires, gpus []gpu.GPU) []string {
	var match, vramOK int
	for _, g := range gpus {
		if !gpu.ArchMatch(rq.Arch, g.GFX) {
			continue
		}
		match++
		if rq.VRAMGiB <= 0 || float64(g.VRAMBytes) >= rq.VRAMGiB*gib {
			vramOK++
		}
	}
	switch {
	case match == 0:
		return []string{fmt.Sprintf("arch: needs %s; host has %s", strings.Join(rq.Arch, "/"), archSummary(gpus))}
	case match < rq.GPUs:
		return []string{fmt.Sprintf("gpus: needs %d %s GPUs; host has %d", rq.GPUs, strings.Join(rq.Arch, "/"), match)}
	case vramOK < rq.GPUs:
		return []string{fmt.Sprintf("vram: needs %g GiB per GPU; %d of %d matching GPUs have it", rq.VRAMGiB, vramOK, match)}
	}
	return nil
}

// diskReasons compare download_gb with the free disk of the Hugging Face
// cache. A local model, a model from an input and a cached model need no
// download.
func diskReasons(r *Recipe, h HostFacts) (tight, no []string) {
	src := r.Model.Source
	if r.Requires.DownloadGB <= 0 || h.DiskFreeBytes == 0 || src == "" ||
		filepath.IsAbs(src) || strings.Contains(src, "${") || (h.Cached != nil && h.Cached(src)) {
		return nil, nil
	}
	need, free := r.Requires.DownloadGB*1e9, float64(h.DiskFreeBytes)
	switch {
	case need > free:
		return nil, []string{fmt.Sprintf("disk: the model needs %g GB; the Hugging Face cache has %.0f GB free", r.Requires.DownloadGB, free/1e9)}
	case need > free*diskSlack:
		return []string{fmt.Sprintf("disk: the model needs %g GB of the %.0f GB free in the Hugging Face cache", r.Requires.DownloadGB, free/1e9)}, nil
	}
	return nil, nil
}

// ListEntry is one row of `recipe list` (and the dashboard's /api/recipes).
type ListEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Description is the one line of the recipe file that says what the
	// recipe is. The page shows it under the name, so a reader does not
	// have to know the id.
	Description string           `json:"description,omitempty"`
	Status      string           `json:"status"`
	Priority    int              `json:"priority"`
	Model       string           `json:"model"`
	Entrypoint  string           `json:"entrypoint"`
	GPUs        int              `json:"gpus"`
	Arch        []string         `json:"arch"`
	Tags        []string         `json:"tags"`
	Source      string           `json:"source"`
	Inputs      map[string]Input `json:"inputs"`
	Modes       []string         `json:"modes"`
	Fit         Fit              `json:"fit"`
}

// Listing is the `recipe list` document.
type Listing struct {
	Recipes []ListEntry `json:"recipes"`
	Errors  []LoadError `json:"errors"`
}

// List resolves every launchable recipe of s and checks its fit. Without
// all, recipes that do not fit are left out (unknown fit is kept). Resolve
// errors join the load errors. Entries are sorted by id.
func List(s Set, gpus []gpu.GPU, h HostFacts, all bool) Listing {
	l := Listing{Recipes: []ListEntry{}, Errors: append([]LoadError{}, s.Errors...)}
	for _, id := range s.IDs(false) {
		r, err := s.Resolve(id)
		if err == nil {
			if errs := Validate(r); len(errs) > 0 {
				err = fmt.Errorf("%s", strings.Join(errs, "; "))
			}
		}
		if err != nil {
			l.Errors = append(l.Errors, LoadError{Source: s.Recipes[id].Source, Err: err.Error()})
			continue
		}
		fit := CheckFit(r, gpus, h)
		if !all && fit.Status == FitNo {
			continue
		}
		e := ListEntry{
			ID: r.ID, Name: r.Name, Description: r.Description, Status: r.Status, Priority: r.Priority,
			Model: r.Model.Source, Entrypoint: r.Entrypoint, GPUs: r.Requires.GPUs,
			Arch: r.Requires.Arch, Tags: r.Tags, Source: r.Source, Inputs: r.Inputs,
			Modes: fit.Modes, Fit: fit,
		}
		if e.Entrypoint == "" {
			e.Entrypoint = "cli"
		}
		if e.Inputs == nil {
			e.Inputs = map[string]Input{}
		}
		if e.Tags == nil {
			e.Tags = []string{}
		}
		l.Recipes = append(l.Recipes, e)
	}
	return l
}

// Best returns the id of the best fitting recipe of a listing: fit, then
// priority (higher first), then id. "" when no recipe fits.
func Best(l Listing) string {
	var c []ListEntry
	for _, e := range l.Recipes {
		if e.Fit.Status == FitYes {
			c = append(c, e)
		}
	}
	sort.SliceStable(c, func(i, j int) bool {
		if c[i].Priority != c[j].Priority {
			return c[i].Priority > c[j].Priority
		}
		return c[i].ID < c[j].ID
	})
	if len(c) == 0 {
		return ""
	}
	return c[0].ID
}

// ValidatedArchs returns the distinct requires.arch entries (gfx names or
// prefixes) of the recipes with status validated, sorted. doctor reads it
// through its options to tell validated targets from merely known ones.
func ValidatedArchs(s Set) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range s.IDs(false) {
		r, err := s.Resolve(id)
		if err != nil || r.Status != "validated" {
			continue
		}
		for _, a := range r.Requires.Arch {
			if !seen[a] {
				seen[a] = true
				out = append(out, a)
			}
		}
	}
	sort.Strings(out)
	return out
}
