package benchcmd

import (
	"errors"
	"fmt"
	"io"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/cli"
	"github.com/opengfx1030/bernie/internal/serving/bench/eval"
)

// benchEvalProposeCommand is `bernie bench eval propose`: the local model
// suggests an eval run for a goal, and bernie checks it (spec §10.5).
func benchEvalProposeCommand() *cli.Command {
	cmd := &cli.Command{Name: "propose", Summary: "Ask the local model to suggest an eval run for a goal",
		Usage: `--goal "TEXT" [--url U] [--kind openai|openwebui] [--model M]`}
	cmd.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(cmd)
		var o eval.Options
		var goal, model string
		fs.StringVar(&goal, "goal", "", "what the run should find out, in words (required)")
		fs.StringVar(&o.URL, "url", "", "the server (default: the only run, else the bench default)")
		fs.StringVar(&o.Kind, "kind", eval.KindOpenAI, "openai or openwebui")
		fs.StringVar(&model, "model", "", "the model that proposes and is evaluated (default: the first one)")
		pos, err := cli.Parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) > 0 {
			return cli.Usagef("unexpected argument %q", pos[0])
		}
		if goal == "" {
			return cli.Usagef("--goal is required")
		}
		if model != "" {
			o.Models = []string{model}
		}
		p := app.BenchEvalPropose(c.Ctx, o, goal)
		if err := c.Emit(p, func(w io.Writer) { printProposal(w, p) }); err != nil {
			return err
		}
		if !p.OK {
			return errors.New("the proposal was refused: " + p.Error)
		}
		return nil
	}
	return cmd
}

// printProposal writes the checked proposal and the command that runs it.
func printProposal(w io.Writer, p *eval.Proposal) {
	if !p.OK {
		fmt.Fprintf(w, "refused after %.1f s: %s\n", p.LatencyS, p.Error)
		if p.Response != "" {
			fmt.Fprintf(w, "the model answered: %s\n", p.Response)
		}
		return
	}
	fmt.Fprintf(w, "proposal (%.1f s): %s\n\n  %s\n\n", p.LatencyS, p.Rationale, p.Command)
	for _, c := range p.Checks {
		fmt.Fprintf(w, "  [ok] %s: %s\n", c.Name, c.Detail)
	}
	fmt.Fprintln(w, "\nbernie starts nothing: run the command above to measure it.")
}
