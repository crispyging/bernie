package recipe

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/topo"
)

func TestGPUSelectionCapacity(t *testing.T) {
	const g = uint64(1 << 30)
	gpus := []gpu.GPU{
		{HIPIndex: 2, GFX: "gfx1030", VRAMBytes: 8 * g},
		{HIPIndex: 4, GFX: "gfx1030"},
		{HIPIndex: 7, GFX: "gfx1030", VRAMBytes: 16 * g},
		{HIPIndex: 10, GFX: "gfx1030", VRAMBytes: 32 * g},
		{HIPIndex: 14, GFX: "gfx1030", VRAMBytes: 24 * g},
		{HIPIndex: 16, GFX: "gfx1100", VRAMBytes: 64 * g},
	}
	tp := &topo.Topology{Nodes: []topo.Node{
		{HIPIndex: 2, Switch: "a"}, {HIPIndex: 4, Switch: "a"}, {HIPIndex: 7, Switch: "a"},
		{HIPIndex: 10, Switch: "b"}, {HIPIndex: 14, Switch: "b"},
	}}
	tests := []struct {
		name     string
		count    int
		vram     float64
		gpus     []gpu.GPU
		tp       *topo.Topology
		explicit string
		want     string
		wantFit  string
		wantErr  bool
		warning  bool
	}{
		{"eligible lowest indices", 2, 16, gpus, nil, "", "7,10", FitYes, false, false},
		{"topology among eligible cards", 2, 16, gpus, tp, "", "10,14", FitYes, false, false},
		{"eligible cards span switches", 3, 16, gpus, tp, "", "7,10,14", FitYes, false, false},
		{"insufficient eligible subset", 4, 16, gpus, tp, "", "", FitNo, true, false},
		{"other architecture capacity excluded", 1, 64, gpus, nil, "", "", FitNo, true, false},
		{"exact capacity boundary", 1, 16, gpus, nil, "", "7", FitYes, false, false},
		{"below capacity boundary", 1, 16.5, gpus, nil, "", "10", FitYes, false, false},
		{"no capacity requirement", 2, 0, gpus, nil, "", "2,4", FitYes, false, false},
		{"explicit insufficient card", 2, 16, gpus, tp, "2,10", "", FitYes, true, false},
		{"explicit unknown capacity", 2, 16, gpus, tp, "4,10", "", FitYes, true, false},
		{"explicit eligible cards preserve order", 2, 16, gpus, tp, "14,7", "14,7", FitYes, false, false},
		{"explicit count mismatch still warns", 2, 16, gpus, tp, "7", "7", FitYes, false, true},
		{"explicit no capacity requirement", 2, 0, gpus, nil, "4,2", "4,2", FitYes, false, false},
		{"unknown inventory automatic", 2, 16, nil, tp, "", "", FitUnknown, false, true},
		{"unknown inventory explicit", 2, 16, nil, tp, "2,10", "2,10", FitUnknown, false, false},
		{"known empty inventory", 1, 16, []gpu.GPU{}, nil, "", "", FitNo, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := mkReq("gfx1030", tt.count)
			r.Requires.VRAMGiB = tt.vram
			if fit := CheckFit(r, tt.gpus, 0); fit.Status != tt.wantFit {
				t.Fatalf("fit = %+v, want %s", fit, tt.wantFit)
			}
			o := PlanOptions{GPUs: tt.gpus, Topo: tt.tp, Restriction: "restricted host indices"}
			if tt.explicit != "" {
				o.Sets = []string{"env.HIP_VISIBLE_DEVICES=" + tt.explicit}
			}
			p, err := BuildPlan(r, o)
			if tt.wantErr {
				if err == nil {
					t.Fatal("accepted GPUs that cannot satisfy the recipe")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if p.HIPDevices != tt.want || p.Env["HIP_VISIBLE_DEVICES"] != tt.want || joinInts(p.GPUs) != tt.want {
				t.Errorf("selection = %v / %q / %q, want %q", p.GPUs, p.HIPDevices, p.Env["HIP_VISIBLE_DEVICES"], tt.want)
			}
			if (len(p.Warnings) > 0) != tt.warning {
				t.Errorf("warnings = %v, want warning: %v", p.Warnings, tt.warning)
			}
		})
	}
}

func TestGPUSelectionFullCounts(t *testing.T) {
	for count := 1; count <= 10; count++ {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			gpus := make([]gpu.GPU, count)
			want := make([]int, count)
			tp := &topo.Topology{}
			for i := range gpus {
				idx := 2*i + 1
				gpus[i] = gpu.GPU{HIPIndex: idx, GFX: "gfx1030", VRAMBytes: uint64(16+16*(i%2)) << 30}
				want[i] = idx
				tp.Nodes = append(tp.Nodes, topo.Node{HIPIndex: idx, Switch: strconv.Itoa(i / 4)})
			}
			r := mkReq("gfx1030", count)
			r.Requires.VRAMGiB = 16
			if fit := CheckFit(r, gpus, 0); fit.Status != FitYes {
				t.Fatalf("fit = %+v", fit)
			}
			for _, topology := range []*topo.Topology{nil, tp} {
				p, err := BuildPlan(r, PlanOptions{GPUs: gpus, Topo: topology})
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(p.GPUs, want) || p.HIPDevices != joinInts(want) || p.Env["HIP_VISIBLE_DEVICES"] != joinInts(want) {
					t.Errorf("selection = %v / %q / %q, want %v", p.GPUs, p.HIPDevices, p.Env["HIP_VISIBLE_DEVICES"], want)
				}
			}
		})
	}
}
