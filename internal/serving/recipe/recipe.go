// Package recipe loads recipe files (spec §7), resolves `extends`, validates
// recipes, checks their fit to a host and turns one recipe plus overrides
// plus the host's GPUs into a Plan. It never prints.
package recipe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/opengfx1030/bernie/internal/support/paths"
	"github.com/opengfx1030/bernie/recipes"
)

// Statuses a recipe can have.
var Statuses = []string{"validated", "experimental", "reference"}

// Model is the model a recipe serves.
type Model struct {
	Source     string `json:"source,omitempty"`
	ServedName string `json:"served_name,omitempty"`
	Revision   string `json:"revision,omitempty"`
}

// Requires is what a recipe needs from the host.
type Requires struct {
	Arch    []string `json:"arch,omitempty"`
	GPUs    int      `json:"gpus,omitempty"`
	VRAMGiB float64  `json:"vram_gib,omitempty"`
	RAMGiB  float64  `json:"ram_gib,omitempty"`
	Engine  string   `json:"engine,omitempty"`
	// DownloadGB is the size of the model download in GB (10^9 bytes).
	DownloadGB float64 `json:"download_gb,omitempty"`
	// KVBytesPerToken is the KV cache of one token over all ranks, in bytes,
	// with the recipe's cache dtype (spec §7.8).
	KVBytesPerToken float64 `json:"kv_bytes_per_token,omitempty"`
}

// Input is a named value the operator gives with --set NAME=VALUE.
type Input struct {
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Path        bool   `json:"path,omitempty"`
	Default     string `json:"default,omitempty"`
}

// Container is the image a recipe runs in. A nil *Container means the
// recipe runs in host mode only.
type Container struct {
	Image            string `json:"image,omitempty"`
	TunableOpProfile string `json:"tunableop_profile,omitempty"`
	ShmSize          string `json:"shm_size,omitempty"`
	Pull             *bool  `json:"pull,omitempty"`
	// DeviceAccess is how the container gets the GPU: classic, cdi or
	// auto (the default, spec §8.2).
	DeviceAccess string `json:"device_access,omitempty"`
}

// Spec is the file schema of spec §7.2.
type Spec struct {
	Schema      int              `json:"schema"`
	Extends     []string         `json:"extends,omitempty"`
	Name        string           `json:"name,omitempty"`
	Description string           `json:"description,omitempty"`
	Status      string           `json:"status,omitempty"`
	Priority    int              `json:"priority,omitempty"`
	Entrypoint  string           `json:"entrypoint,omitempty"`
	Tags        []string         `json:"tags,omitempty"`
	Model       Model            `json:"model"`
	Requires    Requires         `json:"requires"`
	Inputs      map[string]Input `json:"inputs,omitempty"`
	Container   *Container       `json:"container,omitempty"`
	// Env values are strings, numbers, booleans, or nil (unset the variable).
	Env map[string]any `json:"env,omitempty"`
	// Args are vllm flags without "--": scalars, booleans, maps and lists.
	Args map[string]any `json:"args,omitempty"`
	// Why maps a setting path to one short reason.
	Why map[string]string `json:"why,omitempty"`
}

// Recipe is one loaded (or resolved) recipe.
type Recipe struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	Fragment bool   `json:"fragment"`
	Spec

	doc map[string]any // the file's own document, used by Resolve
}

// LoadError is a recipe file that did not load or resolve.
type LoadError struct {
	Source string `json:"source"`
	Err    string `json:"error"`
}

// Set is every loaded recipe, by id, plus the files that failed.
type Set struct {
	Recipes map[string]*Recipe `json:"recipes"`
	Errors  []LoadError        `json:"errors"`
}

// Load reads recipes in spec §7.1 order: embedded built-ins, then
// <config>/recipes, then each directory of $BERNIE_RECIPES, then extra
// (directories or files). Directories are walked recursively. A later file
// with the same id replaces an earlier one, except inside one directory tree,
// where a duplicate id is an error naming both paths. Files that fail are
// collected in Set.Errors.
func Load(extra ...string) Set {
	s := Set{Recipes: map[string]*Recipe{}, Errors: []LoadError{}}
	s.loadFS(recipes.FS, "builtin:")
	s.loadPath(filepath.Join(paths.Config(), "recipes"), true)
	for _, d := range filepath.SplitList(os.Getenv("BERNIE_RECIPES")) {
		if d != "" {
			s.loadPath(d, false)
		}
	}
	for _, p := range extra {
		s.loadPath(p, false)
	}
	return s
}

// layer is one root of recipe files (the embedded tree, a directory or one
// file). A recipe id is unique inside a layer; a later layer replaces an
// earlier one (spec §7.1).
type layer struct {
	s    *Set
	seen map[string]string // id -> source
}

func (s *Set) newLayer() *layer { return &layer{s: s, seen: map[string]string{}} }

// loadFS walks fsys recursively and loads every *.yaml file.
func (s *Set) loadFS(fsys fs.FS, prefix string) {
	l := s.newLayer()
	_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			s.Errors = append(s.Errors, LoadError{Source: prefix + p, Err: err.Error()})
			return nil
		}
		if d.IsDir() || !isRecipeFile(p) {
			return nil
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			s.Errors = append(s.Errors, LoadError{Source: prefix + p, Err: err.Error()})
			return nil
		}
		l.add(prefix+p, b)
		return nil
	})
}

// loadPath loads one file, or every *.yaml/*.yml file below a directory
// (recursively, in path order).
func (s *Set) loadPath(p string, optional bool) {
	st, err := os.Stat(p)
	if err != nil {
		if !optional {
			s.Errors = append(s.Errors, LoadError{Source: p, Err: err.Error()})
		}
		return
	}
	l := s.newLayer()
	if !st.IsDir() {
		l.addFile(p)
		return
	}
	_ = filepath.WalkDir(p, func(f string, d fs.DirEntry, err error) error {
		if err != nil {
			s.Errors = append(s.Errors, LoadError{Source: f, Err: err.Error()})
			return nil
		}
		if !d.IsDir() && isRecipeFile(f) {
			l.addFile(f)
		}
		return nil
	})
}

func isRecipeFile(p string) bool {
	e := filepath.Ext(p)
	return e == ".yaml" || e == ".yml"
}

func (l *layer) addFile(f string) {
	b, err := os.ReadFile(f)
	if err != nil {
		l.s.Errors = append(l.s.Errors, LoadError{Source: f, Err: err.Error()})
		return
	}
	l.add(f, b)
}

// add parses one file. A second file with an id already seen in this layer
// is an error naming both paths; the first file stays.
func (l *layer) add(source string, b []byte) {
	r, err := Parse(source, b)
	if err != nil {
		l.s.Errors = append(l.s.Errors, LoadError{Source: source, Err: err.Error()})
		return
	}
	if first, dup := l.seen[r.ID]; dup {
		l.s.Errors = append(l.s.Errors, LoadError{Source: source,
			Err: fmt.Sprintf("duplicate recipe id %q: also defined in %s", r.ID, first)})
		return
	}
	l.seen[r.ID] = source
	l.s.Recipes[r.ID] = r
}

// IDFromSource is the file name stem of a recipe source.
func IDFromSource(source string) string {
	base := filepath.Base(strings.TrimPrefix(source, "builtin:"))
	return strings.TrimSuffix(strings.TrimSuffix(base, ".yaml"), ".yml")
}

// Parse decodes one recipe file. It checks YAML syntax and the schema
// (field names and types), not cross-file rules (see Resolve and Validate).
func Parse(source string, b []byte) (*Recipe, error) {
	v, err := parseYAML(b)
	if err != nil {
		return nil, err
	}
	doc, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("a recipe must be a YAML map")
	}
	r, err := decode(doc)
	if err != nil {
		return nil, err
	}
	r.ID = IDFromSource(source)
	r.Source = source
	r.Fragment = strings.HasPrefix(r.ID, "_")
	r.doc = doc
	return r, nil
}

// decode turns a document into a Recipe through JSON: unknown fields and
// wrong types are errors. Numbers in env and args become json.Number.
func decode(doc map[string]any) (*Recipe, error) {
	b, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	var sp Spec
	if err := dec.Decode(&sp); err != nil {
		return nil, schemaError(err)
	}
	return &Recipe{Spec: sp}, nil
}

func schemaError(err error) error {
	if e, ok := err.(*json.UnmarshalTypeError); ok {
		return fmt.Errorf("%s: want %s, got %s", e.Field, e.Type, e.Value)
	}
	return fmt.Errorf("%s", strings.TrimPrefix(err.Error(), "json: "))
}

// docOf returns the schema document of r (no id/source; extends removed).
func docOf(r *Recipe) map[string]any {
	sp := r.Spec
	sp.Extends = nil
	b, _ := json.Marshal(sp)
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var m map[string]any
	_ = dec.Decode(&m)
	return m
}

// IDs returns the ids of the set, sorted. Fragments are included only when
// fragments is true.
func (s Set) IDs(fragments bool) []string {
	out := make([]string, 0, len(s.Recipes))
	for id, r := range s.Recipes {
		if r.Fragment && !fragments {
			continue
		}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Resolve merges the extends chain of id (spec §7.3): parents depth first,
// left to right, then the file itself. A cycle or a missing parent is an
// error. The result keeps the file's own id, source and extends list.
func (s Set) Resolve(id string) (*Recipe, error) {
	r, ok := s.Recipes[id]
	if !ok {
		return nil, fmt.Errorf("unknown recipe %q", id)
	}
	var layers []*Recipe
	doc, err := s.resolveDoc(id, nil, &layers)
	if err != nil {
		return nil, err
	}
	delete(doc, "extends")
	delete(doc, "why")
	if why := mergeWhy(layers); len(why) > 0 {
		doc["why"] = why
	}
	out, err := decode(doc)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", id, err)
	}
	out.ID, out.Source, out.Fragment = r.ID, r.Source, r.Fragment
	out.Extends = r.Extends
	out.doc = r.doc
	return out, nil
}

func (s Set) resolveDoc(id string, stack []string, layers *[]*Recipe) (map[string]any, error) {
	for _, x := range stack {
		if x == id {
			return nil, fmt.Errorf("extends cycle: %s -> %s", strings.Join(stack, " -> "), id)
		}
	}
	r, ok := s.Recipes[id]
	if !ok {
		if len(stack) > 0 {
			return nil, fmt.Errorf("%s extends unknown recipe %q", stack[len(stack)-1], id)
		}
		return nil, fmt.Errorf("unknown recipe %q", id)
	}
	stack = append(stack, id)
	acc := map[string]any{}
	for _, parent := range r.Extends {
		pd, err := s.resolveDoc(parent, stack, layers)
		if err != nil {
			return nil, err
		}
		merge(acc, pd, "")
	}
	*layers = append(*layers, r)
	own := deepCopy(r.doc).(map[string]any)
	delete(own, "extends")
	merge(acc, own, "")
	return acc, nil
}

// mergeWhy merges reason maps in layer order. A later reason replaces an
// earlier reason for the same key. A null reason removes it.
func mergeWhy(layers []*Recipe) map[string]any {
	out := map[string]any{}
	for _, r := range layers {
		why, ok := r.doc["why"].(map[string]any)
		if !ok {
			continue
		}
		for key, reason := range why {
			if reason == nil {
				delete(out, key)
				continue
			}
			out[key] = deepCopy(reason)
		}
	}
	return out
}
