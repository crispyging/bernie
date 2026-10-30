package recipe

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/opengfx1030/bernie/internal/machine/host"
)

var (
	idPattern    = regexp.MustCompile(`^_?[a-z0-9][a-z0-9._-]*$`)
	varPattern   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	argPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	archPattern  = regexp.MustCompile(`^gfx[0-9][0-9a-z]*$`)
	refPattern   = regexp.MustCompile(`\$\{([^}]*)\}`)
	reservedArgs = map[string]string{
		"model":             "model.source",
		"served-model-name": "model.served_name",
		"revision":          "model.revision",
	}
)

// Validate checks one recipe and returns one message per problem. Call it
// on a resolved recipe. Fragments skip the rules that only a launchable
// recipe needs (name, status, model, requires, container image).
func Validate(r *Recipe) []string {
	var errs []string
	add := func(f string, a ...any) { errs = append(errs, fmt.Sprintf(f, a...)) }

	if r.Schema != 1 {
		add("schema: want 1, got %d", r.Schema)
	}
	if !idPattern.MatchString(r.ID) {
		add("id %q: use lowercase letters, digits, '.', '_' and '-' (fragments start with '_')", r.ID)
	}
	if r.Status != "" && !contains(Statuses, r.Status) {
		add("status: want one of %s, got %q", strings.Join(Statuses, ", "), r.Status)
	}
	switch r.Entrypoint {
	case "", "cli", "api":
	default:
		add("entrypoint: want cli or api, got %q", r.Entrypoint)
	}
	if !r.Fragment {
		if r.Name == "" {
			add("name: required")
		}
		if r.Status == "" {
			add("status: required (%s)", strings.Join(Statuses, ", "))
		}
		if r.Model.Source == "" {
			add("model.source: required")
		}
		if len(r.Requires.Arch) == 0 {
			add("requires.arch: required, a list of gfx names or prefixes, for example [gfx1030] or [gfx103, gfx11]")
		}
		if r.Requires.GPUs < 1 {
			add("requires.gpus: required, at least 1")
		}
		if r.Container != nil && r.Container.Image == "" {
			add("container.image: required when container is set (omit container for host mode only)")
		}
		if n, ok := worldSize(r.Args); ok && r.Requires.GPUs > 0 && n != r.Requires.GPUs {
			add("requires.gpus is %d, but tensor x pipeline x data parallel size is %d", r.Requires.GPUs, n)
		}
	}
	for _, a := range r.Requires.Arch {
		if !archPattern.MatchString(a) {
			add("requires.arch: %q is not a gfx name or prefix (for example gfx1030, gfx103, gfx11)", a)
		}
	}
	if r.Container != nil && r.Container.DeviceAccess != "" && !contains(host.DeviceAccessModes, r.Container.DeviceAccess) {
		add("container.device_access: want one of %s, got %q", strings.Join(host.DeviceAccessModes, ", "), r.Container.DeviceAccess)
	}
	if r.Requires.GPUs < 0 || r.Requires.VRAMGiB < 0 || r.Requires.RAMGiB < 0 ||
		r.Requires.DownloadGB < 0 || r.Requires.KVBytesPerToken < 0 {
		add("requires: values must not be negative")
	}
	for _, name := range sortedKeys(r.Inputs) {
		if !varPattern.MatchString(name) {
			add("inputs.%s: input names use letters, digits and '_'", name)
		}
	}
	for _, k := range sortedKeys(r.Env) {
		if !varPattern.MatchString(k) {
			add("env.%s: not a valid variable name", k)
		}
		switch r.Env[k].(type) {
		case nil, string, bool, json.Number:
		default:
			add("env.%s: want a string, number, boolean or null", k)
		}
	}
	for _, k := range sortedKeys(r.Args) {
		if !argPattern.MatchString(k) {
			add("args.%s: write flags without '--', lowercase with '-'", k)
		}
		if use, ok := reservedArgs[k]; ok {
			add("args.%s: use %s instead", k, use)
		}
	}
	// Interpolation: every ${NAME} must be a declared input.
	for _, ref := range refs(r) {
		name := ref.name
		if !varPattern.MatchString(name) {
			add("%s: bad reference ${%s}", ref.where, name)
			continue
		}
		if _, ok := r.Inputs[name]; !ok {
			add("%s: unknown input ${%s} (declare it under inputs)", ref.where, name)
		}
	}
	errs = append(errs, validateWhy(r)...)
	return errs
}

type varRef struct{ where, name string }

// refs lists the ${NAME} references in model, env and args.
func refs(r *Recipe) []varRef {
	var out []varRef
	scan := func(where, s string) {
		s = strings.ReplaceAll(s, "$${", "")
		for _, m := range refPattern.FindAllStringSubmatch(s, -1) {
			out = append(out, varRef{where, m[1]})
		}
	}
	scan("model.source", r.Model.Source)
	scan("model.served_name", r.Model.ServedName)
	scan("model.revision", r.Model.Revision)
	for _, k := range sortedKeys(r.Env) {
		if s, ok := r.Env[k].(string); ok {
			scan("env."+k, s)
		}
	}
	for _, k := range sortedKeys(r.Args) {
		walkStrings(r.Args[k], func(s string) { scan("args."+k, s) })
	}
	return out
}

func walkStrings(v any, f func(string)) {
	switch t := v.(type) {
	case string:
		f(t)
	case map[string]any:
		for _, x := range t {
			walkStrings(x, f)
		}
	case []any:
		for _, x := range t {
			walkStrings(x, f)
		}
	}
}

// worldSize is tensor x pipeline x data parallel size when the args set at
// least one of them as a number.
func worldSize(args map[string]any) (int, bool) {
	n, any := 1, false
	for _, k := range []string{"tensor-parallel-size", "pipeline-parallel-size", "data-parallel-size"} {
		v, ok := args[k]
		if !ok || v == nil {
			continue
		}
		i, ok := toInt(v)
		if !ok {
			continue
		}
		n *= i
		any = true
	}
	return n, any
}

func toInt(v any) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case int64:
		return int(t), true
	case json.Number:
		i, err := strconv.Atoi(t.String())
		return i, err == nil
	case string:
		i, err := strconv.Atoi(t)
		return i, err == nil
	}
	return 0, false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
