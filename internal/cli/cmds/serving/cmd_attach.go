package serving

import (
	"fmt"
	"io"
	"strings"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/cli"
	"github.com/opengfx1030/bernie/internal/serving/runner"
)

func init() {
	cmd := &cli.Command{Name: "attach",
		Summary: "Register an already running vLLM server as a run",
		Usage:   "<url|port> [--name N] [--recipe ID] | --scan"}
	cmd.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(cmd)
		name := fs.String("name", "attached", "name of the run")
		recipe := fs.String("recipe", "", "recipe the server serves (optional)")
		scan := fs.Bool("scan", false, "list the vLLM servers that run on this host (processes and containers); records nothing")
		pos, err := cli.Parse(fs, args)
		if err != nil {
			return err
		}
		if *scan {
			if len(pos) > 0 {
				return cli.Usagef("--scan takes no URL")
			}
			found, err := app.Discover(c.Ctx)
			if err != nil {
				return err
			}
			return c.Emit(found, func(w io.Writer) { printFound(w, found) })
		}
		if len(pos) != 1 {
			return cli.Usagef("attach needs one URL or port")
		}
		res, err := app.Attach(c.Ctx, app.AttachOptions{URL: pos[0], Name: *name, Recipe: *recipe})
		if err != nil {
			return err
		}
		return c.Emit(res, func(w io.Writer) {
			fmt.Fprintf(w, "%s\n", res.Note)
			fmt.Fprintf(w, "  stop the server yourself; bernie stop %s only forgets it\n", res.Run.Name)
		})
	}
	cli.Register(cmd)
}

// printFound lists the servers of this host and the command that attaches
// each one bernie does not know yet.
func printFound(w io.Writer, found []runner.Found) {
	if len(found) == 0 {
		fmt.Fprintln(w, "no vLLM server found on this host")
		return
	}
	for _, f := range found {
		where := fmt.Sprintf("pid %d", f.PID)
		if f.Container != "" {
			where = f.Source + " " + f.Container
		}
		state := "answers /health"
		if !f.Healthy {
			state = "does not answer /health on " + f.URL
		}
		fmt.Fprintf(w, "%s  %s  %s  %s\n", f.URL, orDash(f.Model), where, state)
		switch {
		case f.Run != "":
			fmt.Fprintf(w, "  already run %s\n", f.Run)
		case f.Healthy:
			fmt.Fprintf(w, "  attach: bernie attach %s --name %s\n", f.URL, foundName(f))
		}
	}
}

// foundName is a run name for a found server: its container, else the
// last part of its model, else its port.
func foundName(f runner.Found) string {
	name := f.Container
	if name == "" && f.Model != "" {
		name = f.Model[strings.LastIndex(f.Model, "/")+1:]
	}
	name = strings.Trim(strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			return r
		}
		if r >= 'A' && r <= 'Z' {
			return r + 'a' - 'A'
		}
		return '-'
	}, name), "-._")
	if name == "" {
		name = fmt.Sprintf("vllm-%d", f.Port)
	}
	return name
}

func orDash(s string) string {
	if s == "" {
		return "–"
	}
	return s
}
