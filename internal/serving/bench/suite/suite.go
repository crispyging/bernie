// Package suite holds the benchmark suites: the data of a run, the loader
// that reads it, the runner that measures it and the verdict that compares
// two runs. A suite is data, so a new suite needs no Go change. See
// docs/spec.md §10.
package suite

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	embedded "github.com/opengfx1030/bernie/bench"
)

// Schema is the version of the suite format this build reads.
const Schema = 1

// Defaults of a suite that does not say otherwise.
const (
	// DefaultBand is the share of a change that counts as noise.
	DefaultBand = 0.02
	// DefaultSteady is how close the repetitions of one arm must be.
	DefaultSteady = 0.95
	// DefaultTolerance is the share of the prompt length a cell may miss.
	DefaultTolerance = 2.0
)

// Kinds of a group: what its cells assert.
const (
	// KindThroughput measures a speed.
	KindThroughput = "throughput"
	// KindCheck asks a question and checks the answer.
	KindCheck = "check"
)

// Sources of the prompts of a group.
const (
	SourceSynthetic = "synthetic"
	SourceAgentic   = "agentic"
	SourceSession   = "session"
	SourceVision    = "vision"
	SourceToolcall  = "toolcall"
)

// Types of a check: what a check group asserts.
const (
	// CheckTypePasskey asserts that the reply holds the hidden pass key
	// of the prompt. It is the same machinery as the recall gate.
	CheckTypePasskey = "passkey"
)

// Suite is one benchmark suite: the cells a run measures and the
// thresholds that decide whether two runs agree.
type Suite struct {
	Schema      int    `yaml:"schema" json:"schema"`
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description,omitempty"`
	// Reps is the number of repetitions per cell.
	Reps int `yaml:"reps" json:"reps"`
	// Warmup is the number of requests sent and discarded before a cell.
	Warmup int `yaml:"warmup" json:"warmup"`
	// Seed is the base seed of the prompts.
	Seed int64 `yaml:"seed" json:"seed"`
	// Recall turns on the pass key check of every prompt.
	Recall bool `yaml:"recall" json:"recall"`
	// Probe sends the prefill probe before every cell (spec §10.1). It is
	// left out of the digest when it is off, so the older suites keep
	// their digest.
	Probe bool `yaml:"probe" json:"probe,omitempty"`
	// RequestTimeoutS bounds one request of a cell.
	RequestTimeoutS int `yaml:"request_timeout_s" json:"request_timeout_s,omitempty"`
	// CooldownS is the pause between two repetitions.
	CooldownS int `yaml:"cooldown_s" json:"cooldown_s,omitempty"`
	// SizeTolerancePercent is the share of the prompt length a cell may
	// miss before its size claim counts as unproven.
	SizeTolerancePercent float64 `yaml:"size_tolerance_percent" json:"size_tolerance_percent"`
	// Defaults hold the values a group may leave out.
	Defaults Defaults `yaml:"defaults" json:"defaults"`
	// Compare holds the thresholds of the verdict.
	Compare Thresholds `yaml:"compare" json:"compare"`
	// Groups are the sets of cells of the suite.
	Groups []Group `yaml:"groups" json:"groups"`
}

// Defaults holds the values a group may leave out.
type Defaults struct {
	// Requests is the number of requests per cell; 0 means the bench
	// default: max(4, 2 × concurrency).
	Requests int `yaml:"requests" json:"requests"`
}

// Thresholds are the numbers that decide whether two runs agree.
type Thresholds struct {
	// Band is the share of a change that counts as noise.
	Band float64 `yaml:"band" json:"band"`
	// Steady is the share the repetitions of one arm must agree within.
	Steady float64 `yaml:"steady" json:"steady"`
}

// Group is one set of cells of a suite: a lens, a set of concurrency
// levels and what its cells assert.
type Group struct {
	Name string `yaml:"name" json:"name"`
	// Kind is what the cells assert: "throughput" or "check".
	Kind string `yaml:"kind" json:"kind"`
	// InputLen and OutputLen are the prompt and output token counts.
	InputLen  int `yaml:"input_len" json:"input_len"`
	OutputLen int `yaml:"output_len" json:"output_len"`
	// Concurrency are the levels of the cells of the group.
	Concurrency []int `yaml:"concurrency" json:"concurrency"`
	// Reps is the number of repetitions per cell of the group.
	Reps int `yaml:"reps" json:"reps"`
	// EstS is a rough estimate of the wall time of one repetition.
	EstS int `yaml:"est_s" json:"est_s,omitempty"`
	// Source says where the prompts come from.
	Source Source `yaml:"source" json:"source"`
	// Check is the declared answer of a check group.
	Check *Check `yaml:"check" json:"check,omitempty"`
}

// Source says where the prompts of a group come from.
type Source struct {
	Kind   string `yaml:"kind" json:"kind"`
	Corpus string `yaml:"corpus" json:"corpus,omitempty"`
}

// Check is the declared answer of a check group.
type Check struct {
	Type   string `yaml:"type" json:"type"`
	Expect string `yaml:"expect" json:"expect,omitempty"`
}

// Load reads a suite from YAML. It rejects unknown keys, a schema this
// build does not know and invalid values.
func Load(data []byte) (*Suite, error) {
	var s Suite
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("read suite: %w", err)
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err == nil {
		return nil, errors.New("read suite: the file holds more than one document")
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read suite: %w", err)
	}
	s.applyDefaults()
	if err := s.validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

// applyDefaults fills the values the file may leave out.
func (s *Suite) applyDefaults() {
	if s.Reps == 0 {
		s.Reps = 1
	}
	if s.Compare.Band == 0 {
		s.Compare.Band = DefaultBand
	}
	if s.Compare.Steady == 0 {
		s.Compare.Steady = DefaultSteady
	}
	if s.SizeTolerancePercent == 0 {
		s.SizeTolerancePercent = DefaultTolerance
	}
	for i := range s.Groups {
		g := &s.Groups[i]
		if g.Kind == "" {
			g.Kind = KindThroughput
		}
		if g.Source.Kind == "" {
			g.Source.Kind = SourceSynthetic
		}
		if g.Reps == 0 {
			g.Reps = s.Reps
		}
	}
}

// validate rejects a suite that cannot be run.
func (s *Suite) validate() error {
	if s.Schema != Schema {
		return fmt.Errorf("suite schema %d is not supported (this build reads %d)", s.Schema, Schema)
	}
	if s.ID == "" {
		return errors.New("the suite has no id")
	}
	if !validID(s.ID) {
		return fmt.Errorf("the suite id %q must hold lower case letters, digits and dashes", s.ID)
	}
	if s.Name == "" {
		return fmt.Errorf("the suite %s has no name", s.ID)
	}
	if s.Reps < 1 {
		return fmt.Errorf("the suite %s: reps must be at least 1", s.ID)
	}
	if s.Warmup < 0 {
		return fmt.Errorf("the suite %s: warmup must not be negative", s.ID)
	}
	if s.RequestTimeoutS < 0 {
		return fmt.Errorf("the suite %s: request_timeout_s must not be negative", s.ID)
	}
	if s.CooldownS < 0 {
		return fmt.Errorf("the suite %s: cooldown_s must not be negative", s.ID)
	}
	if s.SizeTolerancePercent < 0 {
		return fmt.Errorf("the suite %s: size_tolerance_percent must not be negative", s.ID)
	}
	if s.Defaults.Requests < 0 {
		return fmt.Errorf("the suite %s: defaults.requests must not be negative", s.ID)
	}
	if s.Compare.Band < 0 {
		return fmt.Errorf("the suite %s: compare.band must not be negative", s.ID)
	}
	if s.Compare.Steady <= 0 || s.Compare.Steady > 1 {
		return fmt.Errorf("the suite %s: compare.steady must be in (0, 1]", s.ID)
	}
	if len(s.Groups) == 0 {
		return fmt.Errorf("the suite %s has no groups", s.ID)
	}
	seen := map[string]bool{}
	for i := range s.Groups {
		g := &s.Groups[i]
		if g.Name == "" {
			return fmt.Errorf("the suite %s: group %d has no name", s.ID, i+1)
		}
		if seen[g.Name] {
			return fmt.Errorf("the suite %s: the group %q appears twice", s.ID, g.Name)
		}
		seen[g.Name] = true
		if g.Kind != KindThroughput && g.Kind != KindCheck {
			return fmt.Errorf("the suite %s: the group %q has the unknown kind %q", s.ID, g.Name, g.Kind)
		}
		if g.InputLen < 1 {
			return fmt.Errorf("the suite %s: the group %q needs a positive input_len", s.ID, g.Name)
		}
		if g.OutputLen < 1 {
			return fmt.Errorf("the suite %s: the group %q needs a positive output_len", s.ID, g.Name)
		}
		if len(g.Concurrency) == 0 {
			return fmt.Errorf("the suite %s: the group %q needs a concurrency level", s.ID, g.Name)
		}
		for _, c := range g.Concurrency {
			if c < 1 {
				return fmt.Errorf("the suite %s: the group %q has the concurrency %d", s.ID, g.Name, c)
			}
		}
		if g.Reps < 1 {
			return fmt.Errorf("the suite %s: the group %q needs at least one repetition", s.ID, g.Name)
		}
		if g.EstS < 0 {
			return fmt.Errorf("the suite %s: the group %q has a negative est_s", s.ID, g.Name)
		}
		if !validSource(g.Source.Kind) {
			return fmt.Errorf("the suite %s: the group %q has the unknown source %q", s.ID, g.Name, g.Source.Kind)
		}
		if g.Kind == KindCheck {
			if g.Check == nil || g.Check.Type == "" {
				return fmt.Errorf("the suite %s: the check group %q needs a check type", s.ID, g.Name)
			}
			if g.Check.Type != CheckTypePasskey {
				return fmt.Errorf("the suite %s: the check group %q has the unknown check type %q", s.ID, g.Name, g.Check.Type)
			}
		}
	}
	return nil
}

// validID reports whether a suite id may be used as a file name and in a
// command line.
func validID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
		default:
			return false
		}
	}
	return true
}

// validSource reports whether the source of a group is known.
func validSource(kind string) bool {
	switch kind {
	case SourceSynthetic, SourceAgentic, SourceSession, SourceVision, SourceToolcall:
		return true
	}
	return false
}

// Digest is the sha256 of the canonical form of the suite. The same suite
// always has the same digest, and a changed suite has another one: two runs
// of different suites are not comparable.
func (s *Suite) Digest() string {
	b, _ := json.Marshal(s) // the suite holds only plain fields
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Cells is the number of cells of the suite.
func (s *Suite) Cells() int {
	n := 0
	for _, g := range s.Groups {
		n += len(g.Concurrency)
	}
	return n
}

// EstS is a rough estimate of the wall time of the whole suite in seconds.
func (s *Suite) EstS() int {
	n := 0
	for _, g := range s.Groups {
		n += g.EstS * g.Reps
	}
	return n
}

// Info is a suite with its digest: the document of `bernie bench suites`
// and of the MCP tool `bench_suites`. The digest pins the exact suite, so
// two runs of different suites are never compared.
type Info struct {
	*Suite
	// Digest is the sha256 of the canonical form of the suite.
	Digest string `json:"digest"`
}

// List returns every built-in suite with its digest, sorted by id.
func List() ([]*Info, error) {
	all, err := loadAll()
	if err != nil {
		return nil, err
	}
	out := make([]*Info, 0, len(all))
	for _, s := range all {
		out = append(out, &Info{Suite: s, Digest: s.Digest()})
	}
	return out, nil
}

// loadAll reads every built-in suite, sorted by id.
func loadAll() ([]*Suite, error) {
	var out []*Suite
	err := fs.WalkDir(embedded.FS, "suites", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".yaml") {
			return nil
		}
		data, err := embedded.FS.ReadFile(p)
		if err != nil {
			return err
		}
		s, err := Load(data)
		if err != nil {
			return fmt.Errorf("%s: %w", path.Base(p), err)
		}
		out = append(out, s)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Get returns the built-in suite with this id.
func Get(id string) (*Suite, error) {
	all, err := loadAll()
	if err != nil {
		return nil, err
	}
	for _, s := range all {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, fmt.Errorf("no bench suite %q (bernie bench suites lists them)", id)
}
