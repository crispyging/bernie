package flow

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/doctor"
	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/onboarding"
	"github.com/opengfx1030/bernie/internal/serving/runner"
)

func TestNextCommand(t *testing.T) {
	one := []gpu.GPU{{GFX: "gfx1030"}}
	fail := doctor.Check{ID: "gpu.kfd", Status: doctor.Fail, Title: "/dev/kfd is readable and writable",
		Fix: "load the amdgpu driver"}
	warn := doctor.Check{ID: "kernel.iommu", Status: doctor.Warn, Title: "iommu=pt"}
	waiting := Run{Status: runner.Status{Record: runner.Record{Name: "r1"}, State: runner.StateStarting,
		Waiting: "the server answers /health but not a completion yet: HTTP 503"}}
	cases := []struct {
		name string
		f    *onboarding.Facts
		runs []Run
		want []string
	}{
		{"no GPU: look at the driver", &onboarding.Facts{}, nil,
			[]string{"bernie doctor   # No AMD GPU is visible to bernie."}},
		{"fitting recipe: up", &onboarding.Facts{GPUs: one, Recipe: "r1", Best: "r1"}, nil,
			[]string{"bernie up r1"}},
		{"no fitting recipe", &onboarding.Facts{GPUs: one}, nil,
			[]string{"bernie recipe list --all   # No recipe fits this host."}},
		{"failed check goes first", &onboarding.Facts{GPUs: one, Recipe: "r1",
			Doctor: doctor.Report{Checks: []doctor.Check{warn, fail}}}, nil,
			[]string{"load the amdgpu driver   # This account cannot open the GPU devices."}},
		{"warnings alone do not add doctor", &onboarding.Facts{GPUs: one, Recipe: "r1",
			Doctor: doctor.Report{Checks: []doctor.Check{warn}}}, nil,
			[]string{"bernie up r1"}},
		{"running: dash and bench instead of up", &onboarding.Facts{GPUs: one, Recipe: "r1",
			Runs: []onboarding.Run{{Name: "r1", State: "running"}}}, nil,
			[]string{"bernie dash", "bernie bench"}},
		{"starting: follow the log, do not bench", &onboarding.Facts{GPUs: one, Recipe: "r1",
			Runs: []onboarding.Run{{Name: "r1", State: "starting"}}}, nil,
			[]string{"bernie logs -f   # the server is loading the model", "bernie dash"}},
		{"starting: the log of the run and what it waits for", &onboarding.Facts{GPUs: one, Recipe: "r1",
			Runs: []onboarding.Run{{Name: "r1", State: "starting"}}}, []Run{waiting},
			[]string{"bernie logs r1 -f   # r1 is starting: the server answers /health but not a completion yet: HTTP 503", "bernie dash"}},
		{"loading phase is not ready", &onboarding.Facts{GPUs: one, Recipe: "r1",
			Runs: []onboarding.Run{{Name: "r1", State: "running", Phase: "Loading the model weights"}}}, nil,
			[]string{"bernie logs -f   # the server is loading the model", "bernie dash"}},
		{"exited run is not running", &onboarding.Facts{GPUs: one, Recipe: "r1",
			Runs: []onboarding.Run{{Name: "r1", State: "exited"}}}, nil,
			[]string{"bernie up r1"}},
	}
	for _, c := range cases {
		s := &Summary{Onboarding: onboarding.Build(c.f), Runs: c.runs}
		if got := next(s); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: next = %q, want %q", c.name, got, c.want)
		}
	}
}

// Snapshot probes the real host; on one without GPUs (CI) the document must
// still be complete: empty lists, never nulls, and the doctor as next step.
func TestSnapshotWithoutGPUs(t *testing.T) {
	if g, _ := gpu.List("/"); len(g) > 0 {
		t.Skip("host has GPUs")
	}
	t.Setenv("BERNIE_CONFIG_DIR", t.TempDir())
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	s, err := Snapshot(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if s.GPUs == nil || s.Runs == nil || s.Problems == nil || s.Fits == nil {
		t.Errorf("nil slice in %+v", s)
	}
	if len(s.Next) == 0 {
		t.Errorf("next = %q", s.Next)
	}
}

func TestUpBlockedWithoutGPUs(t *testing.T) {
	if g, _ := gpu.List("/"); len(g) > 0 {
		t.Skip("host has GPUs")
	}
	t.Setenv("BERNIE_CONFIG_DIR", t.TempDir())
	res, err := Up(context.Background(), UpOptions{Yes: true}, &bytes.Buffer{})
	var b *Blocked
	if res != nil || !errors.As(err, &b) {
		t.Fatalf("Up = %v, %v; want *Blocked", res, err)
	}
	if b.Reason != "No AMD GPU is visible to bernie." || len(b.Commands) == 0 {
		t.Errorf("blocked = %+v", b)
	}
}

func TestConfirmFrom(t *testing.T) {
	for in, want := range map[string]bool{"y\n": true, "YES\n": true, " yes \n": true, "n\n": false, "\n": false, "": false, "maybe\n": false, "y": true} {
		var out bytes.Buffer
		if got := ConfirmFrom(strings.NewReader(in), &out)("go?"); got != want {
			t.Errorf("answer %q = %v, want %v", in, got, want)
		}
		if out.String() != "go? [y/N] " {
			t.Errorf("prompt = %q", out.String())
		}
	}
}
