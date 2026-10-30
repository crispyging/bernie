package setup

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/opengfx1030/bernie/internal/support/paths"
)

// The states of a setup run.
const (
	RunRunning = "running"
	RunDone    = "done"
	RunFailed  = "failed"
	RunBlocked = "blocked"
)

// The states of a step of a run.
const (
	StepDone    = "done"
	StepSkipped = "skipped"
	StepRunning = "running"
	StepFailed  = "failed"
	StepBlocked = "blocked"
)

// KeepRuns is the number of setup runs the store keeps. The older ones are
// removed with their logs.
const KeepRuns = 10

// Run is the record of one setup run. Every front end reads this one
// record: the CLI, the MCP server and the dashboard (spec §6.4). The
// record is written while the run runs, so a crash or a reload still
// shows the truth.
type Run struct {
	ID        string     `json:"id"`
	Mode      string     `json:"mode,omitempty"`
	Recipe    string     `json:"recipe,omitempty"`
	Venv      string     `json:"venv,omitempty"`
	State     string     `json:"state"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	// DurationS is the seconds the run took. It grows while the run runs.
	DurationS float64   `json:"duration_s"`
	Steps     []RunStep `json:"steps"`
	Error     string    `json:"error,omitempty"`
	// Dir is the directory of the run: the record and the step logs.
	Dir string `json:"dir"`
}

// RunStep is one step of a run: what it did, how long it took and where its
// log is.
type RunStep struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	State string `json:"state"`
	// StartedAt and EndedAt bound the step. They are nil while the step
	// has not started.
	StartedAt *time.Time `json:"started_at,omitempty"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	// DurationS is the seconds the step took.
	DurationS float64 `json:"duration_s"`
	// ExitCode is the exit code of the last command of the step. It is
	// -1 when the step did not run a command.
	ExitCode int `json:"exit_code"`
	// Summary is the one sentence that says what the step did.
	Summary string `json:"summary"`
	// LogPath is the log of the step: the command line first, then the
	// output with a timestamp on every line.
	LogPath string `json:"log_path"`
	// Commands are the command lines of the step.
	Commands []string `json:"commands,omitempty"`
	// Evidence is what the step produced: the verification JSON of the
	// venv check, for example.
	Evidence json.RawMessage `json:"evidence,omitempty"`
	// Progress is the progress of the step while it runs.
	Progress *Progress `json:"progress,omitempty"`
}

// RunStore keeps the setup runs of a host under <state>/setup: one
// directory per run, with the record and the log of every step.
type RunStore struct {
	Root string
}

// DefaultRunStore is the store of the host: <state>/setup.
func DefaultRunStore() *RunStore {
	return &RunStore{Root: filepath.Join(paths.State(), "setup")}
}

// Dir is the directory of one run.
func (s *RunStore) Dir(id string) string { return filepath.Join(s.Root, id) }

// StepLog is the path of the log of one step of a run.
func (s *RunStore) StepLog(runID, stepID string) string {
	return filepath.Join(s.Dir(runID), stepID+".log")
}

// NewRunID returns the id of a new run: the time and a short random tail,
// so the ids sort by time.
func NewRunID() string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return time.Now().UTC().Format("20060102T150405Z")
	}
	return time.Now().UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(b)
}

// Create makes the directory of a run, writes the record and points the
// latest pointer at it. The older runs beyond KeepRuns are removed.
func (s *RunStore) Create(r *Run) error {
	if err := os.MkdirAll(s.Dir(r.ID), 0o755); err != nil {
		return err
	}
	r.Dir = s.Dir(r.ID)
	if err := s.Save(r); err != nil {
		return err
	}
	if err := s.pointLatest(r.ID); err != nil {
		return err
	}
	s.Prune(KeepRuns)
	return nil
}

// Save writes the record of a run atomically: a reader sees the old
// record or the new one, never a half-written file.
func (s *RunStore) Save(r *Run) error {
	if r.Dir == "" {
		r.Dir = s.Dir(r.ID)
	}
	if err := os.MkdirAll(r.Dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return writeAtomic(filepath.Join(r.Dir, "run.json"), b)
}

// writeAtomic writes a file through a temporary one and renames it.
func writeAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// pointLatest writes the pointer to the newest run.
func (s *RunStore) pointLatest(id string) error {
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(s.Root, "latest"), []byte(id+"\n"))
}

// Latest returns the newest run of the store.
func (s *RunStore) Latest() (*Run, error) {
	b, err := os.ReadFile(filepath.Join(s.Root, "latest"))
	if err != nil {
		return nil, fmt.Errorf("no setup run yet: %w", err)
	}
	id := strings.TrimSpace(string(b))
	if id == "" {
		return nil, fmt.Errorf("no setup run yet")
	}
	return s.Load(id)
}

// Load reads the record of one run.
func (s *RunStore) Load(id string) (*Run, error) {
	if id == "" {
		return nil, fmt.Errorf("no run id")
	}
	b, err := os.ReadFile(filepath.Join(s.Dir(id), "run.json"))
	if err != nil {
		return nil, err
	}
	var r Run
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("the record of run %s is broken: %w", id, err)
	}
	if r.Dir == "" {
		r.Dir = s.Dir(id)
	}
	return &r, nil
}

// List returns the runs of the store, newest first. A run whose record
// cannot be read is left out.
func (s *RunStore) List() ([]*Run, error) {
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*Run
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		r, err := s.Load(e.Name())
		if err != nil {
			continue
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out, nil
}

// Prune removes the runs beyond the newest keep. The pointer to the
// newest run stays.
func (s *RunStore) Prune(keep int) {
	if keep < 1 {
		return
	}
	runs, err := s.List()
	if err != nil {
		return
	}
	// List is newest first by start time. Two runs of one second differ
	// only in their random suffix, so the name does not order them.
	latest := ""
	if b, err := os.ReadFile(filepath.Join(s.Root, "latest")); err == nil {
		latest = strings.TrimSpace(string(b))
	}
	kept := 0
	for _, r := range runs {
		if kept < keep || r.ID == latest {
			kept++
			continue
		}
		os.RemoveAll(s.Dir(r.ID))
	}
}

// Step returns the step of the run with this id, or nil.
func (r *Run) Step(id string) *RunStep {
	for i := range r.Steps {
		if r.Steps[i].ID == id {
			return &r.Steps[i]
		}
	}
	return nil
}

// Duration returns the seconds the run took, from its start.
func (r *Run) Duration(now time.Time) float64 {
	end := now
	if r.EndedAt != nil {
		end = *r.EndedAt
	}
	return end.Sub(r.StartedAt).Seconds()
}

// FormatDuration writes a duration in the way the CLI shows it: seconds
// under a minute, minutes and seconds above.
func FormatDuration(s float64) string {
	if s < 0 {
		s = 0
	}
	if s < 60 {
		return fmt.Sprintf("%.1fs", s)
	}
	m := int(s) / 60
	sec := int(s) % 60
	if m < 60 {
		return fmt.Sprintf("%dm%02ds", m, sec)
	}
	h := m / 60
	return fmt.Sprintf("%dh%02dm%02ds", h, m%60, sec)
}
