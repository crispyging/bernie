package benchcmd

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/cli"
	"github.com/opengfx1030/bernie/internal/serving/bench/eval"
)

// benchEvalCommand is `bernie bench eval`: the quality evaluation of spec
// §10.5, with list, show, tracks and propose below it.
func benchEvalCommand() *cli.Command {
	cmd := &cli.Command{Name: "eval", Summary: "Grade the answers of a server on seeded technical-workflow cases",
		Usage: "[--url U] [--kind openai|openwebui] [--model M[,M2]] [--suite smoke|core|extended] [--context 4096[,16384]] [--tracks T,...] [--seed N] [--repeats N]"}
	cmd.Run = func(c *cli.Context, args []string) error { return runEval(c, cmd, args) }

	list := &cli.Command{Name: "list", Summary: "List saved eval results"}
	list.Run = func(c *cli.Context, args []string) error {
		if _, err := cli.Parse(c.Flags(list), args); err != nil {
			return err
		}
		rs, err := eval.List()
		if err != nil {
			return err
		}
		return c.Emit(rs, func(w io.Writer) { printEvalList(w, rs) })
	}
	show := &cli.Command{Name: "show", Summary: "Show a saved eval result", Usage: "<id> [--csv]"}
	show.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(show)
		asCSV := fs.Bool("csv", false, "print the cases as CSV, one row per case")
		pos, err := cli.Parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return cli.Usagef("bench eval show needs exactly one result id")
		}
		if *asCSV && c.JSON {
			return cli.Usagef("--csv and --json are two formats; pick one")
		}
		r, err := eval.Get(pos[0])
		if err != nil {
			return err
		}
		if *asCSV {
			_, err = c.Out.Write(eval.CSV(r))
			return err
		}
		return c.Emit(r, func(w io.Writer) { printEvalResult(w, r) })
	}
	tracks := &cli.Command{Name: "tracks", Summary: "List the eval tracks and suites"}
	tracks.Run = func(c *cli.Context, args []string) error {
		if _, err := cli.Parse(c.Flags(tracks), args); err != nil {
			return err
		}
		doc := map[string]any{"version": eval.Version, "suites": eval.SuiteIDs, "tracks": eval.Tracks}
		return c.Emit(doc, func(w io.Writer) {
			fmt.Fprintf(w, "suites: %s\n", strings.Join(eval.SuiteIDs, ", "))
			for _, t := range eval.Tracks {
				fmt.Fprintf(w, "%-11s %s: %s\n", t.ID, t.Name, t.Description)
			}
		})
	}
	cmd.Subs = []*cli.Command{list, show, tracks, benchEvalProposeCommand()}
	return cmd
}

func runEval(c *cli.Context, cmd *cli.Command, args []string) error {
	fs := c.Flags(cmd)
	var o eval.Options
	var models, contexts, tracks string
	var timeout time.Duration
	fs.StringVar(&o.URL, "url", "", "server URL (default: the only run, else http://127.0.0.1:8000); the key comes from $"+app.EvalKeyEnv)
	fs.StringVar(&o.Kind, "kind", eval.KindOpenAI, "openai (vLLM, any OpenAI-compatible server) or openwebui")
	fs.StringVar(&models, "model", "", "comma list of models, run one after the other (default: the first model of the server)")
	fs.StringVar(&o.Suite, "suite", eval.DefaultSuite, "smoke (13 cases), core (52) or extended (78), per context size")
	fs.StringVar(&contexts, "context", strconv.Itoa(eval.DefaultContext), "comma list of approximate prompt sizes in tokens, or auto: bands up to the server's max_model_len")
	fs.StringVar(&tracks, "tracks", "", "comma list of tracks (default: all; bernie bench eval tracks lists them)")
	fs.Int64Var(&o.Seed, "seed", eval.DefaultSeed, "case seed: the same seed asks the same cases")
	fs.IntVar(&o.Repeats, "repeats", 1, "times every case is asked")
	fs.Float64Var(&o.Temperature, "temperature", 0, "sampling temperature")
	fs.IntVar(&o.MaxTokens, "max-tokens", eval.DefaultMaxTokens, "reply tokens of one request")
	fs.DurationVar(&timeout, "timeout", eval.DefaultTimeout, "time limit of one request")
	fs.StringVar(&o.Tag, "tag", "", "free label stored with the result")
	pos, err := cli.Parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return cli.Usagef("unexpected argument %q (subcommands: list, show, tracks, propose)", pos[0])
	}
	o.Models = splitList(models)
	o.Tracks = splitList(tracks)
	o.TimeoutS = int(timeout / time.Second)
	for _, f := range splitList(contexts) {
		if f == "auto" {
			o.AutoContext = true
			continue
		}
		n, err := strconv.Atoi(f)
		if err != nil || n < 1 {
			return cli.Usagef("--context: %q is not a positive integer", f)
		}
		o.Contexts = append(o.Contexts, n)
	}
	if _, err := eval.Normalize(o); err != nil {
		return cli.Usagef("%v", err)
	}
	r, path, err := app.BenchEval(c.Ctx, o, c.Err)
	var se *app.SaveError
	if err != nil && !errors.As(err, &se) {
		return err
	}
	if err != nil {
		printEvalResult(c.Err, r)
		return err
	}
	c.Logf("bench eval: saved %s", path)
	return c.Emit(r, func(w io.Writer) { printEvalResult(w, r) })
}

func splitList(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

func pct(x float64) string { return fmt.Sprintf("%.0f%%", x*100) }

// printEvalResult writes the score of every model, by track and by
// context size, then the cases that did not pass with their reason.
func printEvalResult(w io.Writer, r *eval.Result) {
	o := r.Options
	fmt.Fprintf(w, "%s  %s  %s\n", r.ID, r.URL, r.Status)
	fmt.Fprintf(w, "suite %s (v%s), seed %d, %d repeat(s), contexts %s\n", o.Suite, r.Version, o.Seed, o.Repeats, joinInts(o.Contexts))
	if p := r.ContextPlan; p != nil {
		fmt.Fprintf(w, "contexts auto: max_model_len %d - headroom %d = ceiling %d\n", p.ModelLimit, p.Headroom, p.Ceiling)
	}
	for _, s := range r.Summary {
		fmt.Fprintf(w, "\n%s: %d/%d passed (%s), %d failed, %d errors, %d skipped, %d critical, %d loops, %.1f s a case\n",
			s.Model, s.Passed, s.Graded, pct(s.Accuracy), s.Failed, s.Errors, s.Skipped, s.CriticalFailures, s.Loops, s.AvgLatencyS)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, b := range s.ByTrack {
			fmt.Fprintf(tw, "  track %s\t%d/%d\t%s\n", b.Key, b.Passed, b.Graded, pct(b.Accuracy))
		}
		for _, b := range s.ByContext {
			fmt.Fprintf(tw, "  context %s\t%d/%d\t%s\n", b.Key, b.Passed, b.Graded, pct(b.Accuracy))
		}
		for _, k := range slices.Sorted(maps.Keys(s.FailureCategories)) {
			fmt.Fprintf(tw, "  failure %s\t%d\t\n", k, s.FailureCategories[k])
		}
		tw.Flush()
	}
	first := true
	for _, cr := range r.Cases {
		if cr.Status == eval.StatusPass {
			continue
		}
		if first {
			fmt.Fprintln(w, "\nnot passed:")
			first = false
		}
		reason := cr.Error
		if reason == "" && cr.Expected.Answer != nil {
			reason = fmt.Sprintf("expected %q, got %q", *cr.Expected.Answer, strings.TrimSpace(cr.Reply))
		}
		if cr.Diagnosis != nil {
			reason = cr.Diagnosis.Title + ": " + reason
		}
		fmt.Fprintf(w, "  %s %s ctx %d rep %d: %s %s\n", cr.Model, cr.CaseID, cr.Context, cr.Repeat, cr.Status, reason)
	}
}

func joinInts(xs []int) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = strconv.Itoa(x)
	}
	return strings.Join(parts, ",")
}

func printEvalList(w io.Writer, rs []eval.Result) {
	if len(rs) == 0 {
		fmt.Fprintln(w, "no eval results")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSUITE\tCONTEXTS\tMODEL\tPASSED\tACCURACY")
	for _, r := range rs {
		for _, s := range r.Summary {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d/%d\t%s\n", r.ID, r.Options.Suite, joinInts(r.Options.Contexts), s.Model, s.Passed, s.Graded, pct(s.Accuracy))
		}
	}
	tw.Flush()
}
