package recipe

import (
	"strconv"
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/support/fixture"
)

// visible returns the GPUs of the fixture host that the environment leaves
// visible (spec §7.7).
func visible(t *testing.T, value string) []gpu.GPU {
	t.Helper()
	t.Setenv("HIP_VISIBLE_DEVICES", value)
	t.Setenv("ROCR_VISIBLE_DEVICES", "")
	root := fixture.Root(t, "par1-cs25")
	vis, err := gpu.Visible(root)
	if err != nil {
		t.Fatal(err)
	}
	return vis.GPUs
}

// mkReq is the leaf recipe with another architecture and GPU count.
func mkReq(arch string, n int) *Recipe {
	body := strings.NewReplacer("gfx1030]", arch+"]", "gpus: 2", "gpus: "+strconv.Itoa(n)).Replace(leaf)
	r, err := Parse("t/vis.yaml", []byte(body))
	if err != nil {
		panic(err)
	}
	return r
}

// A recipe that needs four GPUs does not fit when the environment leaves one
// GPU visible.
func TestFitUsesTheVisibleGPUs(t *testing.T) {
	gpus := visible(t, "8")
	fit := CheckFit(mkReq("gfx1030", 4), Host{GPUs: gpus})
	if fit.Status != FitNo {
		t.Errorf("fit = %+v, want no", fit)
	}
	if !strings.Contains(strings.Join(fit.Reasons, " "), "host has 1") {
		t.Errorf("reasons = %v", fit.Reasons)
	}
	if fit := CheckFit(mkReq("gfx1030", 1), Host{GPUs: gpus}); fit.Status != FitYes {
		t.Errorf("fit = %+v, want fits", fit)
	}
}

// The plan picks the visible GPU and keeps the host numbering.
func TestPlanSelectsTheVisibleGPU(t *testing.T) {
	gpus := visible(t, "8")
	p, err := BuildPlan(mkReq("gfx1030", 1), PlanOptions{GPUs: gpus})
	if err != nil {
		t.Fatal(err)
	}
	if p.HIPDevices != "8" || p.Env["HIP_VISIBLE_DEVICES"] != "8" {
		t.Errorf("hip devices = %q, want 8", p.HIPDevices)
	}
}

// A visible GPU of another architecture does not serve a gfx1030 recipe.
func TestPlanRefusesAVisibleGPUOfAnotherArch(t *testing.T) {
	gpus := visible(t, "0") // a W7800 (gfx1100)
	_, err := BuildPlan(mkReq("gfx1030", 1), PlanOptions{GPUs: gpus})
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "gfx1100") {
		t.Errorf("err = %v", err)
	}
}

// An explicit HIP_VISIBLE_DEVICES outside the restriction is refused.
func TestPlanRefusesAGPUOutsideTheRestriction(t *testing.T) {
	gpus := visible(t, "8")
	_, err := BuildPlan(mkReq("gfx1030", 1), PlanOptions{
		GPUs:        gpus,
		Sets:        []string{"env.HIP_VISIBLE_DEVICES=2"},
		Restriction: "GPUs limited to 8 by HIP_VISIBLE_DEVICES",
	})
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "not available to bernie") {
		t.Errorf("err = %v", err)
	}
}
