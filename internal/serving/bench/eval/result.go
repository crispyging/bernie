package eval

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/opengfx1030/bernie/internal/support/paths"
)

// Result is one saved evaluation run.
type Result struct {
	ID      string         `json:"id"`
	Version string         `json:"version"`
	URL     string         `json:"url"`
	Time    time.Time      `json:"time"`
	Status  string         `json:"status"` // done or cancelled
	Options Options        `json:"options"`
	Summary []ModelSummary `json:"summary"`
	Cases   []CaseResult   `json:"cases"`
}

// Bucket counts the cases of one track or one context size.
type Bucket struct {
	Key      string  `json:"key"`
	Passed   int     `json:"passed"`
	Graded   int     `json:"graded"`
	Accuracy float64 `json:"accuracy"`
}

// ModelSummary is the score of one model. Accuracy is passed ÷ graded; a
// skipped case was never sent and is not graded, an error counts as not
// passed.
type ModelSummary struct {
	Model            string   `json:"model"`
	Passed           int      `json:"passed"`
	Failed           int      `json:"failed"`
	Errors           int      `json:"errors"`
	Skipped          int      `json:"skipped"`
	Graded           int      `json:"graded"`
	Accuracy         float64  `json:"accuracy"`
	AvgLatencyS      float64  `json:"avg_latency_s"`
	Tokens           *int     `json:"tokens"`
	CriticalFailures int      `json:"critical_failures"`
	Loops            int      `json:"loops"`
	ByTrack          []Bucket `json:"by_track"`
	ByContext        []Bucket `json:"by_context"`
}

func newResult(o Options, base string) *Result {
	var b [3]byte
	_, _ = rand.Read(b[:])
	now := time.Now().UTC()
	return &Result{ID: "eval-" + now.Format("20060102T150405Z") + "-" + hex.EncodeToString(b[:]),
		Version: Version, URL: base, Time: now, Options: o, Cases: []CaseResult{}}
}

// finish computes the summary of every model.
func (r *Result) finish() {
	r.Summary = nil
	for _, m := range r.Options.Models {
		s := ModelSummary{Model: m}
		tracks, sizes := map[string]*Bucket{}, map[string]*Bucket{}
		tokens, tokensKnown, latency := 0, true, 0.0
		for _, c := range r.Cases {
			if c.Model != m {
				continue
			}
			switch c.Status {
			case StatusSkipped:
				s.Skipped++
				continue
			case StatusPass:
				s.Passed++
			case StatusFail:
				s.Failed++
			case StatusError:
				s.Errors++
			}
			s.Graded++
			latency += c.LatencyS
			if c.Critical {
				s.CriticalFailures++
			}
			if c.Loop {
				s.Loops++
			}
			if c.PromptTokens == nil || c.CompletionTokens == nil {
				tokensKnown = false
			} else {
				tokens += *c.PromptTokens + *c.CompletionTokens
			}
			count(tracks, c.Track, c.Status == StatusPass)
			count(sizes, fmt.Sprint(c.Context), c.Status == StatusPass)
		}
		if s.Graded > 0 {
			s.Accuracy = float64(s.Passed) / float64(s.Graded)
			s.AvgLatencyS = latency / float64(s.Graded)
			if tokensKnown {
				s.Tokens = &tokens
			}
		}
		s.ByTrack = buckets(tracks, func(a, b string) bool { return a < b })
		s.ByContext = buckets(sizes, func(a, b string) bool { return len(a) < len(b) || (len(a) == len(b) && a < b) })
		r.Summary = append(r.Summary, s)
	}
}

func count(m map[string]*Bucket, key string, passed bool) {
	b := m[key]
	if b == nil {
		b = &Bucket{Key: key}
		m[key] = b
	}
	b.Graded++
	if passed {
		b.Passed++
	}
	b.Accuracy = float64(b.Passed) / float64(b.Graded)
}

func buckets(m map[string]*Bucket, less func(a, b string) bool) []Bucket {
	out := make([]Bucket, 0, len(m))
	for _, b := range m {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool { return less(out[i].Key, out[j].Key) })
	return out
}

// Dir is where results are saved: <state>/bench/eval.
func Dir() string { return filepath.Join(paths.State(), "bench", "eval") }

// Save writes the result as <Dir>/<id>.json and returns the path.
func Save(r *Result) (string, error) {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(Dir(), r.ID+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return "", err
	}
	return path, os.Rename(tmp, path)
}

// Get reads one saved result.
func Get(id string) (*Result, error) {
	if id == "" || strings.ContainsAny(id, `/\`) || strings.HasPrefix(id, ".") {
		return nil, fmt.Errorf("invalid eval id %q", id)
	}
	b, err := os.ReadFile(filepath.Join(Dir(), id+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("no eval result %s", id)
	}
	if err != nil {
		return nil, err
	}
	var r Result
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("eval result %s: %w", id, err)
	}
	return &r, nil
}

// List reads every saved result, newest first, without the cases.
func List() ([]Result, error) {
	entries, err := os.ReadDir(Dir())
	if errors.Is(err, os.ErrNotExist) {
		return []Result{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Result{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		r, err := Get(strings.TrimSuffix(name, ".json"))
		if err != nil {
			continue
		}
		r.Cases = nil
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out, nil
}
