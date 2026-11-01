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
	FitTight   = "tight"
	FitNo      = "no"
	FitUnknown = "unknown"
)

// Fit is how a recipe fits this host (spec §7.8). Modes are the modes the
// recipe supports (host always; container when it names an image).
type Fit struct {
	Status  string   `json:"status"`
	Reasons []string `json:"reasons"`
	Modes   []string `json:"modes"`
}

// Usable reports a recipe that runs on this host: it fits, or it fits
// after the change its reasons name.
func (f Fit) Usable() bool { return f.Status == FitYes || f.Status == FitTight }

// Host is what a recipe is compared with. GPUs == nil means no GPU data.
// A zero RAMBytes or DiskFreeBytes skips that check. Cached reports a
// model the cache already holds; nil means none is cached.
type Host struct {
	GPUs          []gpu.GPU
	RAMBytes      uint64
	DiskFreeBytes uint64
	Cached        func(model string) bool
}

// CheckFit compares the recipe's requires block with the host. Arch, GPU
// count, VRAM and RAM decide no. The model download and the KV cache of
// one max-model-len request decide tight.
func CheckFit(r *Recipe, h Host) Fit {
	f := Fit{Status: FitYes, Reasons: []string{}, Modes: Modes(r)}
	rq := r.Requires
	if h.GPUs == nil {
		f.Status = FitUnknown
		f.Reasons = append(f.Reasons, "no GPU data")
	} else {
		var match, vramOK int
		for _, g := range h.GPUs {
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
			f.Reasons = append(f.Reasons, fmt.Sprintf("arch: needs %s; host has %s", strings.Join(rq.Arch, "/"), archSummary(h.GPUs)))
		case match < rq.GPUs:
			f.Reasons = append(f.Reasons, fmt.Sprintf("gpus: needs %d %s GPUs; host has %d", rq.GPUs, strings.Join(rq.Arch, "/"), match))
		case vramOK < rq.GPUs:
			f.Reasons = append(f.Reasons, fmt.Sprintf("vram: needs %g GiB per GPU; %d of %d matching GPUs have it", rq.VRAMGiB, vramOK, match))
		}
	}
	if rq.RAMGiB > 0 && h.RAMBytes > 0 && float64(h.RAMBytes) < rq.RAMGiB*gib {
		f.Reasons = append(f.Reasons, fmt.Sprintf("ram: needs %g GiB host RAM; host has %.0f GiB", rq.RAMGiB, float64(h.RAMBytes)/gib))
	}
	if len(f.Reasons) > 0 && f.Status == FitYes {
		f.Status = FitNo
	}
	if f.Status == FitUnknown && len(f.Reasons) > 1 {
		f.Status = FitNo // RAM alone already rules it out
	}
	if f.Status != FitYes {
		return f
	}
	if reason := diskReason(r, h); reason != "" {
		f.Reasons = append(f.Reasons, reason)
	}
	if k := CheckKV(r, minMatchingVRAM(r, h.GPUs)); k != nil && k.NeedBytes > k.BudgetBytes {
		f.Reasons = append(f.Reasons, k.Reason())
	}
	if len(f.Reasons) > 0 {
		f.Status = FitTight
	}
	return f
}

// diskReason says when the model download does not fit the free space of
// the model cache. A cached model or a local path needs no download.
func diskReason(r *Recipe, h Host) string {
	need := r.Requires.DownloadGB * 1e9
	if need <= 0 || h.DiskFreeBytes == 0 || filepath.IsAbs(r.Model.Source) {
		return ""
	}
	if h.Cached != nil && h.Cached(r.Model.Source) {
		return ""
	}
	if float64(h.DiskFreeBytes) >= need {
		return ""
	}
	return fmt.Sprintf("disk: the download needs %g GB; the model cache has %.0f GB free", r.Requires.DownloadGB, float64(h.DiskFreeBytes)/1e9)
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
func List(s Set, h Host, all bool) Listing {
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
		fit := CheckFit(r, h)
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

// Best returns the id of the best fitting recipe of a listing: fits before
// tight, then priority (higher first), then id. "" when no recipe is usable.
func Best(l Listing) string {
	var c []ListEntry
	for _, e := range l.Recipes {
		if e.Fit.Usable() {
			c = append(c, e)
		}
	}
	sort.SliceStable(c, func(i, j int) bool {
		if ti, tj := c[i].Fit.Status == FitTight, c[j].Fit.Status == FitTight; ti != tj {
			return tj
		}
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
