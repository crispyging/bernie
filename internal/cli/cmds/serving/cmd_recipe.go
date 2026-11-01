package serving

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/opengfx1030/bernie/internal/cli"
	"io"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"

	"gopkg.in/yaml.v3"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/serving/recipe"
	"github.com/opengfx1030/bernie/internal/serving/runner"
)

func init() {
	cmd := &cli.Command{Name: "recipe", Summary: "List, show, validate and plan recipes"}

	list := &cli.Command{Name: "list", Summary: "Recipes with their fit to this host", Usage: "[--all]"}
	list.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(list)
		all := fs.Bool("all", false, "include recipes that do not fit this host")
		if _, err := cli.Parse(fs, args); err != nil {
			return err
		}
		l, err := app.RecipeList(*all)
		if err != nil {
			return err
		}
		return c.Emit(l, func(w io.Writer) {
			if note := app.Restriction(); note != "" {
				fmt.Fprintln(w, note)
			}
			printRecipeList(w, l, *all)
		})
	}

	show := &cli.Command{Name: "show", Summary: "The merged recipe (after extends)", Usage: "<id>"}
	show.Run = func(c *cli.Context, args []string) error {
		pos, err := cli.Parse(c.Flags(show), args)
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return cli.Usagef("recipe show needs exactly one recipe id")
		}
		r, err := recipe.Load().Resolve(pos[0])
		if err != nil {
			return err
		}
		return c.Emit(r, func(w io.Writer) {
			fmt.Fprintf(w, "# %s (%s)\n", r.ID, r.Source)
			if len(r.Extends) > 0 {
				fmt.Fprintf(w, "# extends: %s (merged below)\n", strings.Join(r.Extends, ", "))
			}
			sp := r.Spec
			sp.Extends = nil
			if b, err := toYAML(sp); err == nil {
				w.Write(b)
			}
		})
	}

	validate := &cli.Command{Name: "validate", Summary: "Validate recipe files, or all loaded recipes", Usage: "[FILE]..."}
	validate.Run = func(c *cli.Context, args []string) error {
		files, err := cli.Parse(c.Flags(validate), args)
		if err != nil {
			return err
		}
		rep := validateRecipes(files)
		bad := 0
		for _, v := range rep {
			if len(v.Errors) > 0 {
				bad++
			}
		}
		if err := c.Emit(rep, func(w io.Writer) { printValidate(w, rep) }); err != nil {
			return err
		}
		if bad > 0 {
			return cli.ChecksFailed("%d of %d recipes are not valid", bad, len(rep))
		}
		return nil
	}

	plan := &cli.Command{Name: "plan", Summary: "The plan: exact environment and command",
		Usage: "<id> [--set KEY=VALUE]... [--mode host|container] [--name N] [--venv DIR] [--engine E]"}
	plan.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(plan)
		var o runner.PrepareOptions
		var sets cli.MultiFlag
		prepareFlags(fs, &o, &sets)
		pos, err := cli.Parse(fs, args)
		if err != nil {
			return err
		}
		o.Venv = c.Venv
		if len(pos) != 1 {
			return cli.Usagef("recipe plan needs exactly one recipe id")
		}
		o.Recipe, o.Sets = pos[0], sets
		p, err := app.Plan(c.Ctx, o)
		if err != nil {
			return err
		}
		return c.Emit(p, func(w io.Writer) { printPlan(w, p) })
	}

	cmd.Subs = []*cli.Command{list, show, validate, plan}
	cli.Register(cmd)
}

// prepareFlags are the flags shared by recipe plan, serve and compose.
func prepareFlags(fs *flag.FlagSet, o *runner.PrepareOptions, sets *cli.MultiFlag) {
	fs.Var(sets, "set", "override: INPUT=VALUE or section.key=VALUE (YAML value), repeatable")
	fs.StringVar(&o.Mode, "mode", "", "host or container (default: config, else container when the recipe has an image and an engine is usable)")
	fs.StringVar(&o.Name, "name", "", "run name (default: recipe id)")
	fs.StringVar(&o.Engine, "engine", "", "docker or podman (default: config, else the first usable)")
	fs.StringVar(&o.DeviceAccess, "device-access", "", "classic, cdi or auto (default: config, else the recipe, else auto)")
	fs.StringVar(&o.Profile, "profile", "", "force a TunableOp profile (validated against the loaded rocBLAS in host mode)")
	fs.BoolVar(&o.NoTunableOp, "no-tunableop", false, "do not set TunableOp")
	fs.BoolVar(&o.Tuning, "tuning", false, "TunableOp online tuning into the per-user rows file")
}

func printRecipeList(w io.Writer, l recipe.Listing, all bool) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tFIT\tGPUS\tMODES\tPRIO\tMODEL")
	for _, e := range l.Recipes {
		fit := e.Fit.Status
		if e.Fit.Status != recipe.FitYes && len(e.Fit.Reasons) > 0 {
			fit += " (" + strings.Join(e.Fit.Reasons, "; ") + ")"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d x %s\t%s\t%d\t%s\n", e.ID, e.Status, fit, e.GPUs,
			strings.Join(e.Arch, "/"), strings.Join(e.Modes, ","), e.Priority, e.Model)
	}
	tw.Flush()
	if len(l.Recipes) == 0 {
		if all {
			fmt.Fprintln(w, "no recipes")
		} else {
			fmt.Fprintln(w, "no recipe fits this host; see bernie recipe list --all")
		}
	}
	if len(l.Errors) > 0 {
		fmt.Fprintln(w, "\nerrors:")
		for _, e := range l.Errors {
			fmt.Fprintf(w, "  %s: %s\n", e.Source, e.Err)
		}
	}
}

// validateResult is one row of `recipe validate`.
type validateResult struct {
	ID     string   `json:"id"`
	Source string   `json:"source"`
	Errors []string `json:"errors"`
}

func validateRecipes(files []string) []validateResult {
	s := recipe.Load(files...)
	out := []validateResult{}
	ids := s.IDs(true)
	if len(files) > 0 {
		ids = nil
		for _, f := range files {
			ids = append(ids, recipe.IDFromSource(f))
		}
	}
	reported := map[string]bool{}
	for _, e := range s.Errors {
		if len(files) > 0 && !slices.Contains(files, e.Source) {
			continue
		}
		id := recipe.IDFromSource(e.Source)
		out = append(out, validateResult{ID: id, Source: e.Source, Errors: []string{e.Err}})
		reported[e.Source] = true
		if len(files) > 0 {
			reported[id] = true // do not validate an older file of the same id instead
		}
	}
	for _, id := range ids {
		src := id
		if r, ok := s.Recipes[id]; ok {
			src = r.Source
		}
		if reported[src] || reported[id] {
			continue
		}
		v := validateResult{ID: id, Source: src, Errors: []string{}}
		r, err := s.Resolve(id)
		if err != nil {
			v.Errors = append(v.Errors, err.Error())
		} else {
			v.Errors = append(v.Errors, recipe.Validate(r)...)
		}
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func printValidate(w io.Writer, rep []validateResult) {
	for _, v := range rep {
		if len(v.Errors) == 0 {
			fmt.Fprintf(w, "ok    %s\n", v.ID)
			continue
		}
		fmt.Fprintf(w, "FAIL  %s (%s)\n", v.ID, v.Source)
		for _, e := range v.Errors {
			fmt.Fprintf(w, "        %s\n", e)
		}
	}
}

// printPlan prints the plan as comments, an export block and the command,
// ready to paste into a shell.
func printPlan(w io.Writer, p *recipe.Plan) {
	fmt.Fprintf(w, "# recipe:    %s (run %s, mode %s, entrypoint %s)\n", p.Recipe, p.Name, p.Mode, p.Entrypoint)
	if p.HIPDevices != "" {
		fmt.Fprintf(w, "# gpus:      %s (%s)\n", p.HIPDevices, p.GPUSelectReason)
	} else {
		fmt.Fprintf(w, "# gpus:      %s\n", p.GPUSelectReason)
	}
	if p.Restriction != "" {
		fmt.Fprintf(w, "# limited:   %s\n", p.Restriction)
	}
	switch {
	case p.TunableOpProfile != "":
		fmt.Fprintf(w, "# tunableop: %s (%s)\n", p.TunableOpProfile, p.TunableOpRows)
	case p.Env["PYTORCH_TUNABLEOP_FILENAME"] != "":
		fmt.Fprintf(w, "# tunableop: per-user rows %s\n", p.Env["PYTORCH_TUNABLEOP_FILENAME"])
	default:
		fmt.Fprintf(w, "# tunableop: off\n")
	}
	if p.Image != "" {
		fmt.Fprintf(w, "# image:     %s (pull: %v)\n", p.Image, p.Pull)
	}
	if len(p.GroupAdd) > 0 {
		gs := make([]string, 0, len(p.GroupAdd))
		for _, g := range p.GroupAdd {
			gs = append(gs, fmt.Sprintf("%d (%s, from %s)", g.GID, g.Role, g.Source))
		}
		fmt.Fprintf(w, "# groups:    %s\n", strings.Join(gs, ", "))
	}
	if p.DeviceAccess != "" {
		fmt.Fprintf(w, "# access:    %s (%s)\n", p.DeviceAccess, p.DeviceAccessReason)
		if len(p.CDIDevices) > 0 {
			fmt.Fprintf(w, "# cdi:       %s\n", strings.Join(p.CDIDevices, ", "))
		}
	}
	fmt.Fprintf(w, "# url:       %s\n", runner.Record{Host: p.Host, Port: p.Port}.URL())
	for _, a := range p.Adjustments {
		fmt.Fprintf(w, "# adjusted:  %s\n", a)
	}
	for _, warn := range p.Warnings {
		fmt.Fprintf(w, "# WARNING:   %s\n", warn)
	}
	printPlanSettings(w, p)
	ex := p.Exec
	if ex == nil {
		return
	}
	if p.Mode == recipe.ModeHost {
		fmt.Fprintln(w, "\n# --- environment ---")
		for _, k := range ex.Unset {
			fmt.Fprintf(w, "unset %s\n", k)
		}
		for _, kv := range ex.Env {
			k, v, _ := strings.Cut(kv, "=")
			fmt.Fprintf(w, "export %s=%s\n", k, runner.ShellQuote(v))
		}
		if ex.Dir != "" {
			fmt.Fprintf(w, "mkdir -p %s && cd %s\n", runner.ShellQuote(ex.Dir), runner.ShellQuote(ex.Dir))
		}
	}
	fmt.Fprintln(w, "\n# --- command ---")
	q := make([]string, len(ex.Argv))
	for i, a := range ex.Argv {
		q[i] = runner.ShellQuote(a)
		if strings.HasPrefix(a, "$VENV/") {
			q[i] = `"` + a + `"` // placeholder: no venv configured
		}
	}
	fmt.Fprintln(w, strings.Join(q, " "))
}

// printPlanSettings prints every annotated setting and its reason.
func printPlanSettings(w io.Writer, p *recipe.Plan) {
	if len(p.Settings) == 0 {
		return
	}
	fmt.Fprintln(w, "\n# --- settings ---")
	for _, s := range p.Settings {
		fmt.Fprintf(w, "# %s=%s\n", s.Setting, s.Value)
		fmt.Fprintf(w, "#   why: %s\n", s.Why)
	}
}

// toYAML renders v (JSON-tagged) as block-style YAML in field order.
func toYAML(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var n yaml.Node
	if err := yaml.Unmarshal(b, &n); err != nil {
		return nil, err
	}
	blockStyle(&n)
	return yaml.Marshal(&n)
}

func blockStyle(n *yaml.Node) {
	if n.Kind == yaml.MappingNode || n.Kind == yaml.SequenceNode {
		// Keep short scalar lists on one line.
		flow := n.Kind == yaml.SequenceNode
		for _, c := range n.Content {
			if c.Kind != yaml.ScalarNode {
				flow = false
			}
		}
		if flow && len(n.Content) > 0 {
			n.Style = yaml.FlowStyle
		} else {
			n.Style = 0
		}
	}
	if n.Kind == yaml.ScalarNode && n.Style == yaml.DoubleQuotedStyle {
		n.Style = 0
	}
	for _, c := range n.Content {
		blockStyle(c)
	}
}
