package recipe

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/topo"
)

// selectGPUs applies spec §7.7 and sets HIP_VISIBLE_DEVICES.
func selectGPUs(r *Recipe, p *Plan, o PlanOptions) error {
	need := r.Requires.GPUs
	if v, ok := p.Env["HIP_VISIBLE_DEVICES"]; ok {
		p.HIPDevices = v
		p.GPUSelectReason = "HIP_VISIBLE_DEVICES set by the recipe or --set"
		idx, err := parseIndices(v)
		if err != nil {
			return fmt.Errorf("env.HIP_VISIBLE_DEVICES: %w", err)
		}
		p.GPUs = idx
		if o.GPUs != nil {
			byIdx := map[int]gpu.GPU{}
			for _, g := range o.GPUs {
				byIdx[g.HIPIndex] = g
			}
			for _, i := range idx {
				g, ok := byIdx[i]
				if !ok {
					if o.Restriction != "" {
						return fmt.Errorf("HIP_VISIBLE_DEVICES: GPU %d is not available to bernie (%s)", i, o.Restriction)
					}
					return fmt.Errorf("HIP_VISIBLE_DEVICES: GPU %d does not exist on this host", i)
				}
				if !gpu.ArchMatch(r.Requires.Arch, g.GFX) {
					return fmt.Errorf("HIP_VISIBLE_DEVICES: GPU %d is %s; recipe %s needs %s", i, g.GFX, r.ID, strings.Join(r.Requires.Arch, ", "))
				}
				if r.Requires.VRAMGiB > 0 && float64(g.VRAMBytes) < r.Requires.VRAMGiB*gib {
					if g.VRAMBytes == 0 {
						return fmt.Errorf("HIP_VISIBLE_DEVICES: GPU %d has no reported VRAM capacity; recipe %s needs %g GiB per GPU", i, r.ID, r.Requires.VRAMGiB)
					}
					return fmt.Errorf("HIP_VISIBLE_DEVICES: GPU %d has %g GiB VRAM; recipe %s needs %g GiB per GPU", i, float64(g.VRAMBytes)/gib, r.ID, r.Requires.VRAMGiB)
				}
			}
		}
		if len(idx) != need {
			p.Warnings = append(p.Warnings, fmt.Sprintf("HIP_VISIBLE_DEVICES lists %d GPUs; the recipe needs %d", len(idx), need))
		}
		return nil
	}
	if o.GPUs == nil {
		p.GPUSelectReason = "no GPU data: HIP_VISIBLE_DEVICES not set"
		p.Warnings = append(p.Warnings, "no GPU data: vLLM uses every visible GPU; set env.HIP_VISIBLE_DEVICES")
		return nil
	}
	var cand []int
	matching := 0
	for _, g := range o.GPUs {
		if gpu.ArchMatch(r.Requires.Arch, g.GFX) {
			matching++
			if r.Requires.VRAMGiB <= 0 || float64(g.VRAMBytes) >= r.Requires.VRAMGiB*gib {
				cand = append(cand, g.HIPIndex)
			}
		}
	}
	if matching < need {
		return fmt.Errorf("recipe %s needs %d GPUs of %s; this host has %d (%s)", r.ID, need,
			strings.Join(r.Requires.Arch, "/"), matching, archSummary(o.GPUs))
	}
	if len(cand) < need {
		return fmt.Errorf("recipe %s needs %d GPUs of %s with at least %g GiB VRAM per GPU; %d of %d matching GPUs have enough reported capacity", r.ID, need,
			strings.Join(r.Requires.Arch, "/"), r.Requires.VRAMGiB, len(cand), matching)
	}
	var sel []int
	var reason string
	if o.Topo != nil {
		sel, reason = topo.BestGroup(*o.Topo, cand, need)
	} else {
		sort.Ints(cand)
		sel = cand[:need]
		reason = fmt.Sprintf("GPUs %s: no topology data; lowest HIP indices", joinInts(sel))
	}
	if len(sel) != need {
		return fmt.Errorf("GPU selection failed: %s", reason)
	}
	p.GPUs = sel
	p.HIPDevices = joinInts(sel)
	p.GPUSelectReason = reason
	p.Env["HIP_VISIBLE_DEVICES"] = p.HIPDevices
	return nil
}

func parseIndices(s string) ([]int, error) {
	out := []int{}
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		n, err := strconv.Atoi(f)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("want comma-separated HIP indices, got %q", s)
		}
		out = append(out, n)
	}
	return out, nil
}

func archSummary(gpus []gpu.GPU) string {
	if len(gpus) == 0 {
		return "no GPUs"
	}
	count := map[string]int{}
	for _, g := range gpus {
		count[g.GFX]++
	}
	parts := make([]string, 0, len(count))
	for _, k := range sortedKeys(count) {
		parts = append(parts, fmt.Sprintf("%dx %s", count[k], k))
	}
	return strings.Join(parts, ", ")
}

func joinInts(v []int) string {
	s := make([]string, len(v))
	for i, n := range v {
		s[i] = strconv.Itoa(n)
	}
	return strings.Join(s, ",")
}
