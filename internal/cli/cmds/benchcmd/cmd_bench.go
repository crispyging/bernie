package benchcmd

import (
	"errors"
	"fmt"
	"github.com/opengfx1030/bernie/internal/cli"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/serving/bench"
	"github.com/opengfx1030/bernie/internal/serving/bench/suite"
)

func init() {
	cmd := &cli.Command{
		Name:    "bench",
		Summary: "Benchmark an OpenAI-compatible server and save the result",
		Usage:   "[--suite NAME] [--url U] [--model M] [--input-len N] [--output-len N] [--concurrency 1,8] [--num-prompts N] [--warmup N] [--seed N] [--tag T] [--recall]",
	}
	cmd.Run = func(c *cli.Context, args []string) error { return runBench(c, cmd, args) }

	list := &cli.Command{Name: "list", Summary: "List saved bench results"}
	list.Run = func(c *cli.Context, args []string) error {
		if _, err := cli.Parse(c.Flags(list), args); err != nil {
			return err
		}
		rs, err := app.BenchList()
		if err != nil {
			return err
		}
		return c.Emit(rs, func(w io.Writer) { printBenchList(w, rs) })
	}

	show := &cli.Command{Name: "show", Summary: "Show a saved bench result", Usage: "<id> [--csv]"}
	show.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(show)
		asCSV := fs.Bool("csv", false, "print the cells as CSV, one row per cell")
		pos, err := cli.Parse(fs, args)
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return cli.Usagef("bench show needs exactly one result id")
		}
		if *asCSV && c.JSON {
			return cli.Usagef("--csv and --json are two formats; pick one")
		}
		r, err := bench.Get(pos[0])
		if err != nil {
			return err
		}
		if *asCSV {
			_, err = c.Out.Write(bench.CSV(r))
			return err
		}
		return c.Emit(r, func(w io.Writer) { printBenchResult(w, r) })
	}

	compare := &cli.Command{Name: "compare", Summary: "Compare two saved bench results", Usage: "<id-a> <id-b>"}
	compare.Run = func(c *cli.Context, args []string) error { return runCompare(c, compare, args) }

	suites := &cli.Command{Name: "suites", Summary: "List the built-in bench suites"}
	suites.Run = func(c *cli.Context, args []string) error { return runSuites(c, suites, args) }

	ab := &cli.Command{Name: "ab", Summary: "Run a baseline and a candidate back to back and tell the verdict", Usage: "--base ARM --cand ARM [--suite NAME]"}
	ab.Run = func(c *cli.Context, args []string) error { return runAB(c, ab, args) }

	cmd.Subs = []*cli.Command{list, show, compare, suites, ab, benchDeleteCommand(), benchEvalCommand()}
	cli.Register(cmd)
}

func runBench(c *cli.Context, cmd *cli.Command, args []string) error {
	fs := c.Flags(cmd)
	var o bench.Options
	var conc string
	fs.StringVar(&o.URL, "url", "", "server base URL (default: the only run, else "+bench.FallbackURL+")")
	fs.StringVar(&o.Model, "model", "", "model name (default: first id of /v1/models)")
	fs.IntVar(&o.InputLen, "input-len", bench.DefaultInputLen, "prompt tokens (calibrated with /tokenize when present)")
	fs.IntVar(&o.OutputLen, "output-len", bench.DefaultOutputLen, "output tokens (ignore_eos, temperature 0)")
	fs.StringVar(&conc, "concurrency", "1", "comma list of concurrency levels, one cell each (e.g. 1,8)")
	fs.IntVar(&o.NumPrompts, "num-prompts", 0, "requests per cell (default max(4, 2 x concurrency))")
	fs.IntVar(&o.Warmup, "warmup", bench.DefaultWarmup, "requests sent and discarded before each cell")
	fs.Int64Var(&o.Seed, "seed", 0, "prompt seed")
	fs.StringVar(&o.Tag, "tag", "", "free label stored with the result")
	fs.BoolVar(&o.Recall, "recall", false, "hide a short random fact in every prompt and check the reply")
	fs.BoolVar(&o.Probe, "probe", false, "send a prefill probe before every cell: N simultaneous max_tokens=1 requests")
	fs.StringVar(&o.Suite, "suite", "", "run a built-in suite (bernie bench suites lists them)")
	pos, err := cli.Parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return cli.Usagef("unexpected argument %q (subcommands: list, show, compare, suites)", pos[0])
	}
	if o.Suite != "" {
		if bad := suiteFlagConflict(fs); len(bad) > 0 {
			return cli.Usagef("--suite decides the cells itself; %s cannot be used with it", strings.Join(bad, ", "))
		}
		return runSuite(c, o)
	}
	for _, f := range strings.Split(conc, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil || n < 1 {
			return cli.Usagef("--concurrency: %q is not a positive integer", f)
		}
		o.Concurrency = append(o.Concurrency, n)
	}
	if o.InputLen < 1 || o.OutputLen < 1 {
		return cli.Usagef("--input-len and --output-len must be positive")
	}
	if o.NumPrompts < 0 || o.Warmup < 0 {
		return cli.Usagef("--num-prompts and --warmup must not be negative")
	}

	r, path, err := app.Bench(c.Ctx, o, c.Err)
	if errors.Is(err, app.ErrAllFailed) {
		printBenchResult(c.Err, r)
	}
	if err != nil {
		return err
	}
	c.Logf("bench: saved %s", path)
	return c.Emit(r, func(w io.Writer) { printBenchResult(w, r) })
}

func printBenchResult(w io.Writer, r *bench.Result) {
	fmt.Fprintf(w, "%s  %s  %s\n", r.ID, r.Model, r.URL)
	cal := "calibrated"
	if !r.Calibrated {
		cal = "estimated"
	}
	fmt.Fprintf(w, "input %d tokens (%s), output %d tokens, warmup %d", r.Options.InputLen, cal, r.Options.OutputLen, r.Options.Warmup)
	if r.Tag != "" {
		fmt.Fprintf(w, ", tag %s", r.Tag)
	}
	fmt.Fprintln(w)
	if r.Suite != nil {
		fmt.Fprintf(w, "suite %s, digest %s\n", r.Suite.ID, suite.ShortDigest(r.Suite.Digest))
	}
	if fp := fingerprintLine(r.Fingerprint); fp != "" {
		fmt.Fprintln(w, fp)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, metricHelp)
	for _, cl := range r.Cells {
		if cl.Skipped != "" {
			fmt.Fprintf(w, "cell %s skipped: %s\n", suite.CellLabel(cl.Group, cl.Concurrency), cl.Skipped)
			continue
		}
		printCell(w, &cl)
		for _, e := range cl.Errors {
			fmt.Fprintf(w, "%s error: %s\n", suite.CellLabel(cl.Group, cl.Concurrency), e)
		}
		if cl.Recall != nil {
			fmt.Fprintf(w, "%s recall %d/%d passed (hidden fact at %d%% of the prompt, %s)\n",
				suite.CellLabel(cl.Group, cl.Concurrency), cl.Recall.Passed, cl.Recall.Asked, int(cl.Recall.Depth*100), recallMethodText(cl.Recall.Method))
			if cl.Recall.Passed < cl.Recall.Asked {
				fmt.Fprintf(w, "%s the server did not repeat the hidden fact: it may not read the whole prompt, so the speed numbers of this cell are meaningless\n", suite.CellLabel(cl.Group, cl.Concurrency))
			}
		}
		if cl.Check != nil {
			fmt.Fprintf(w, "%s check %d/%d passed (%s)\n",
				suite.CellLabel(cl.Group, cl.Concurrency), cl.Check.Passed, cl.Check.Asked, recallMethodText(cl.Check.Method))
			if cl.Check.Passed < cl.Check.Asked {
				fmt.Fprintf(w, "%s the server did not hold the declared answer: it may not read the whole prompt, so the speed numbers of this cell are meaningless\n", suite.CellLabel(cl.Group, cl.Concurrency))
			}
		}
	}
}

// metricHelp explains in one line how the prefill and the decode numbers
// are measured.
const metricHelp = "prefill per request = prompt tokens ÷ time to first token (the queue time is in it), per cell = prompt tokens ÷ (first request → last first token).\n" +
	"decode per request = (tokens − 1) ÷ (last token − first token), per cell = decode tokens ÷ the time any request was decoding."

// dash is what a number that was not measured shows.
const dash = "–"

// printCell writes the table of one cell: the five groups of numbers of
// spec §10.
func printCell(w io.Writer, cl *bench.Cell) {
	fmt.Fprintf(w, "cell %s   %d ok / %d failed   wall %.2f s   %s tok/chunk   in/out %s/%s tok\n",
		suite.CellLabel(cl.Group, cl.Concurrency), cl.Completed, cl.Failed, cl.DurationS, chunkText(cl.TokensPerChunk),
		countText(cl.MeanPromptTokens), countText(cl.MeanOutputTokens))
	fmt.Fprintln(w, cellRow("metric", "agg", "req mean", "req p50", "req p95", "req min", "req max"))
	fmt.Fprintln(w, rateRow("prefill tok/s", cl.Prefill))
	fmt.Fprintln(w, rateRow("decode tok/s", cl.Decode))
	fmt.Fprintln(w, distRow("TTFT s", cl.TTFTs, secText))
	fmt.Fprintln(w, distRow("ITL ms", cl.ITLms, msText))
	fmt.Fprintln(w, cellRow("output tok/s", suite.RateText(cl.OutputTokS), dash, dash, dash, dash, dash))
	if cl.ServerSide != nil {
		fmt.Fprintf(w, "server side: %s\n", serverSideText(cl.ServerSide))
	}
}

// cellRow formats one line of the cell table: the name on the left, the
// numbers in fixed columns.
func cellRow(label string, cells ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-15s", label)
	for _, c := range cells {
		fmt.Fprintf(&b, " %9s", c)
	}
	return b.String()
}

// rateRow is the row of a speed: the rate of the cell and the rates of its
// requests.
func rateRow(label string, r *bench.Rate) string {
	if r == nil {
		return cellRow(label, dash, dash, dash, dash, dash, dash)
	}
	return cellRow(label, suite.RateText(r.AggTokS), suite.RateText(dMean(r.Req)), suite.RateText(dP50(r.Req)),
		suite.RateText(dP95(r.Req)), suite.RateText(dMin(r.Req)), suite.RateText(dMax(r.Req)))
}

// distRow is the row of a latency: the distribution over the requests.
func distRow(label string, d *bench.Dist, format func(float64) string) string {
	return cellRow(label, dash, format(dMean(d)), format(dP50(d)), format(dP95(d)), format(dMin(d)), format(dMax(d)))
}

// serverSideText names what the server measured over the same requests.
func serverSideText(s *bench.ServerSide) string {
	parts := []string{fmt.Sprintf("%d requests", s.Count)}
	if s.TTFTs != nil {
		parts = append(parts, fmt.Sprintf("TTFT s p50 %s p95 %s", secText(dP50(s.TTFTs)), secText(dP95(s.TTFTs))))
	}
	if s.ITLms != nil {
		parts = append(parts, fmt.Sprintf("ITL ms p50 %s p95 %s", msText(dP50(s.ITLms)), msText(dP95(s.ITLms))))
	}
	return strings.Join(parts, ", ")
}

// dMean, dP50, dP95, dMin and dMax read one number of a distribution. An
// absent distribution has no numbers.
func dMean(d *bench.Dist) float64 {
	if d == nil {
		return 0
	}
	return d.Mean
}

func dP50(d *bench.Dist) float64 {
	if d == nil {
		return 0
	}
	return d.P50
}

func dP95(d *bench.Dist) float64 {
	if d == nil {
		return 0
	}
	return d.P95
}

func dMin(d *bench.Dist) float64 {
	if d == nil {
		return 0
	}
	return d.Min
}

func dMax(d *bench.Dist) float64 {
	if d == nil {
		return 0
	}
	return d.Max
}

// secText formats a time in seconds and msText a time in milliseconds. A
// number that was not measured shows as "–".
func secText(v float64) string {
	if v <= 0 {
		return dash
	}
	return fmt.Sprintf("%.3f", v)
}

func msText(v float64) string {
	if v <= 0 {
		return dash
	}
	return fmt.Sprintf("%.2f", v)
}

// countText formats a token count and chunkText the tokens of a chunk. A
// number that was not measured shows as "–".
func countText(v float64) string {
	if v <= 0 {
		return dash
	}
	return fmt.Sprintf("%.0f", v)
}

func chunkText(v float64) string {
	if v <= 0 {
		return dash
	}
	return fmt.Sprintf("%.2f", v)
}

// recallMethodText names how the prompt length was measured, in words.
func recallMethodText(method string) string {
	if method == "tokenize" {
		return "prompt sized with /tokenize"
	}
	return "prompt length estimated, the server has no /tokenize"
}

// recallText names the verdict of a cell in words.
func recallText(r *bench.Recall) string {
	if r == nil {
		return "–"
	}
	return fmt.Sprintf("%d/%d passed", r.Passed, r.Asked)
}

// resultRecall totals the recall of the cells of a result. The verdict of a
// check group counts too: both say whether the server read the prompt.
func resultRecall(r *bench.Result) *bench.Recall {
	var asked, passed int
	for _, c := range r.Cells {
		if c.Recall != nil {
			asked += c.Recall.Asked
			passed += c.Recall.Passed
		}
		if c.Check != nil {
			asked += c.Check.Asked
			passed += c.Check.Passed
		}
	}
	if asked == 0 {
		return nil
	}
	return &bench.Recall{Asked: asked, Passed: passed}
}

// fingerprintLine names where a result came from.
func fingerprintLine(fp map[string]string) string {
	var parts []string
	if v := fp["host"]; v != "" {
		parts = append(parts, "host "+v)
	}
	if v := fp["gpus"]; v != "" {
		gpus := "gpus " + v
		if scope := fp["gpu_scope"]; scope != "" {
			gpus += " (" + scope + ")"
		}
		parts = append(parts, gpus)
	}
	for _, k := range []string{"run", "recipe", "plan_hash"} {
		if v := fp[k]; v != "" {
			parts = append(parts, k+" "+v)
		}
	}
	return strings.Join(parts, ", ")
}

func printBenchList(w io.Writer, rs []bench.Result) {
	if len(rs) == 0 {
		fmt.Fprintln(w, "no bench results (run `bernie bench`)")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTIME\tMODEL\tIN/OUT\tCONCURRENCY\tBEST OUT TOK/S\tBEST DECODE TOK/S\tRECALL\tTAG")
	for _, r := range rs {
		cs := make([]string, len(r.Cells))
		best, bestDecode := 0.0, 0.0
		for i, cl := range r.Cells {
			cs[i] = strconv.Itoa(cl.Concurrency)
			best = max(best, cl.OutputTokS)
			if cl.Decode != nil {
				bestDecode = max(bestDecode, cl.Decode.AggTokS)
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d/%d\t%s\t%s\t%s\t%s\t%s\n", r.ID, r.Time.Local().Format("2006-01-02 15:04"),
			r.Model, r.Options.InputLen, r.Options.OutputLen, strings.Join(cs, ","), suite.RateText(best), suite.RateText(bestDecode),
			recallText(resultRecall(&r)), r.Tag)
	}
	tw.Flush()
}
