// Package flow composes the domain packages into the two product-level
// operations of spec §3.1: Snapshot (`status`) and Up (`up`). It decides
// nothing about formatting; front ends print the structs.
package flow

import (
	"context"

	"github.com/opengfx1030/bernie/internal/doctor"
	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/host"
	"github.com/opengfx1030/bernie/internal/onboarding"
	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/setup"
)

// Summary is a compact, human-sized view of one host (spec §3.1).
type Summary struct {
	Host host.Info `json:"host"`
	GPUs []gpu.GPU `json:"gpus"`
	// Restriction says that the environment limits the GPUs bernie uses
	// (spec §7.7). It is empty when nothing is restricted.
	Restriction string                `json:"restriction,omitempty"`
	Doctor      map[doctor.Status]int `json:"doctor"`
	Problems    []doctor.Check        `json:"problems"` // fail and warn checks
	Best        string                `json:"best_recipe"`
	Fits        []string              `json:"fitting_recipes"`
	Runs        []Run                 `json:"runs"`
	// Serving holds what the server of every live run measures (spec §11).
	Serving []Serving `json:"serving"`
	Next    []string  `json:"next"`
	// Onboarding is the checklist of the steps from a bare machine to a
	// serving model (spec §11). It is the same document the dashboard and
	// the MCP server read.
	Onboarding *onboarding.Document `json:"onboarding,omitempty"`
	// Venv is the host-mode venv and where it came from (spec §6.2).
	Venv *doctor.VenvInfo `json:"venv,omitempty"`
	// SetupRun is the record of the newest setup run: the time of every
	// step of the last setup (spec §6.4).
	SetupRun *setup.Run `json:"setup_run,omitempty"`
}

// Snapshot probes the host and returns the status document. It changes
// nothing. venv is the host-mode venv flag value ("" = config/env).
func Snapshot(ctx context.Context, venv string) (*Summary, error) {
	f, err := onboarding.Gather(ctx, onboarding.Options{Venv: venv})
	if err != nil {
		return nil, err
	}
	runs, _ := Runs(ctx)
	s := &Summary{
		Host: f.Host, GPUs: f.GPUs, Restriction: f.Restriction,
		Doctor: f.Doctor.Summary, Problems: []doctor.Check{},
		Best: f.Best, Fits: f.Fits, Runs: runs,
		Onboarding: onboarding.Build(f),
		Venv:       f.Doctor.Venv,
	}
	if s.GPUs == nil {
		s.GPUs = []gpu.GPU{}
	}
	if s.Runs == nil {
		s.Runs = []Run{}
	}
	s.Serving = serving(ctx, runs)
	if run, err := setup.DefaultRunStore().Latest(); err == nil {
		s.SetupRun = run
	}
	if s.Fits == nil {
		s.Fits = []string{}
	}
	for _, ch := range f.Doctor.Checks {
		if ch.Status == doctor.Fail || ch.Status == doctor.Warn {
			s.Problems = append(s.Problems, ch)
		}
	}
	s.Next = next(s)
	return s, nil
}

// next returns the commands the operator should type, most important first.
// It reads the onboarding steps: the step to do now gives the command.
func next(s *Summary) []string {
	doc := s.Onboarding
	if doc == nil {
		return []string{"bernie doctor   # the host state is unknown"}
	}
	if serve := findStep(doc.Steps, onboarding.StepServe); serve != nil && serve.State == onboarding.StateRunning {
		if serve.Serving {
			return []string{"bernie dash", "bernie bench"}
		}
		return startingNext(s.Runs)
	}
	step := findStep(doc.Steps, doc.Next)
	if step == nil {
		return []string{"bernie bench   # the server answers; measure it"}
	}
	if step.State == onboarding.StateBlocked {
		cmds := stepCommands(step)
		out := []string{cmds[0] + "   # " + step.Summary}
		return append(out, cmds[1:]...)
	}
	if step.Command == "" {
		return []string{"bernie doctor"}
	}
	return []string{step.Command}
}

// startingNext follows a run that does not answer yet: its log, with what
// it waits for, then the dashboard. Benchmarking it would measure nothing.
func startingNext(runs []Run) []string {
	for _, r := range runs {
		if r.State == runner.StateExited {
			continue
		}
		why := r.Waiting
		if why == "" {
			why = "the server is loading the model"
		}
		return []string{"bernie logs " + r.Name + " -f   # " + r.Name + " is starting: " + why, "bernie dash"}
	}
	return []string{"bernie logs -f   # the server is loading the model", "bernie dash"}
}

// findStep returns the step with the given id, or nil.
func findStep(steps []onboarding.Step, id string) *onboarding.Step {
	if id == "" {
		return nil
	}
	for i := range steps {
		if steps[i].ID == id {
			return &steps[i]
		}
	}
	return nil
}
