package flow

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/serving/runner"
)

// Dashboard is the dashboard that `up` starts while the server loads the
// model (spec §3.1). The CLI implements it with the dashboard package.
type Dashboard interface {
	// Start starts the dashboard. It returns when the dashboard listens, or
	// with an error when it cannot start (the port is in use, for example).
	Start(ctx context.Context) error
}

// Runner starts a run and lists the runs. The runner package is the real
// one; a test can fake it.
type Runner = app.Runner

// serveAndWait starts the run, starts the dashboard and waits until the run
// answers /health (spec §3.1). It is the tail of Up. A dashboard that cannot
// start is reported and does not stop the wait.
func serveAndWait(ctx context.Context, o UpOptions, log io.Writer, res *UpResult) error {
	rec, err := app.Serve(ctx, app.ServeOptions{
		Prepare:  runner.PrepareOptions{Recipe: res.Recipe, Mode: res.Mode, Name: o.Name, Venv: o.Venv, Engine: o.Engine, Sets: o.Sets},
		Start:    runner.StartOptions{Venv: o.Venv, Engine: o.Engine, Progress: log},
		Prepared: o.Prepared,
		Runner:   o.Runner,
	})
	if err != nil {
		return err
	}
	if rec.Plan != nil {
		for _, a := range rec.Plan.Adjustments {
			fmt.Fprintf(log, "up: adjusted: %s\n", a)
		}
		for _, w := range rec.Plan.Warnings {
			fmt.Fprintf(log, "up: warning: %s\n", w)
		}
	}
	res.Run = rec
	res.URL = rec.URL()
	if o.Dashboard != nil {
		fmt.Fprintf(log, "up: starting the dashboard (Ctrl-C stops it; the server keeps running)\n")
		if err := o.Dashboard.Start(ctx); err != nil {
			fmt.Fprintf(log, "up: dashboard: %v\n", err)
		}
	}
	fmt.Fprintf(log, "up: waiting for %s to answer /health and a first completion (logs: bernie logs %s -f)\n", rec.Name, rec.Name)
	list := runner.List
	if o.Runner != nil {
		list = o.Runner.List
	}
	if err := waitHealthy(ctx, rec.Name, o.Wait, log, list); err != nil {
		return err
	}
	res.Healthy = true
	return nil
}

// healthPoll is the delay between two checks of the run state in the wait
// loop. A test shortens it.
var healthPoll = 3 * time.Second

// waitHealthy waits until the run answers /health (limit 0 = 30 minutes).
// list gives the runs with their live state.
func waitHealthy(ctx context.Context, name string, limit time.Duration, progress io.Writer,
	list func(context.Context) ([]runner.Status, error)) error {
	if limit == 0 {
		limit = 30 * time.Minute
	}
	deadline := time.Now().Add(limit)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	var printer phasePrinter
	var rec runner.Record
	var waiting string
	next := time.Time{}
	for {
		if time.Now().After(next) {
			next = time.Now().Add(healthPoll)
			runs, err := list(ctx)
			if err != nil {
				return err
			}
			for _, s := range runs {
				if s.Name != name {
					continue
				}
				rec, waiting = s.Record, s.Waiting
				switch s.State {
				case runner.StateRunning:
					return nil
				case runner.StateExited:
					return &StartupExit{Name: name, Findings: Findings(ctx, s.Record)}
				}
			}
			if time.Now().After(deadline) {
				if waiting == "" {
					waiting = "it never answered"
				}
				return fmt.Errorf("%s is not ready after %s (%s); see: bernie logs %s", name, limit, waiting, name)
			}
		}
		if ph, ok := runner.Phase(ctx, rec); ok {
			printer.print(name, ph)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}
