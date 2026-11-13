package onboarding

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/opengfx1030/bernie/internal/logdoc"
	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/support/paths"
)

// The state of a job.
const (
	JobRunning = "running"
	JobDone    = "done"
	JobFailed  = "failed"
)

// ErrJobBusy means another job is running. One job runs at a time.
var ErrJobBusy = errors.New("one setup job runs at a time: wait for the job that runs now to finish")

// Job is one background run of an onboarding step. The state is kept on disk
// (<state>/jobs/<id>), so a page reload finds it again.
type Job struct {
	ID         string         `json:"id"`
	Step       string         `json:"step"`
	State      string         `json:"state"`
	Phase      string         `json:"phase,omitempty"`
	Progress   *JobProgress   `json:"progress,omitempty"`
	Error      string         `json:"error,omitempty"`
	Findings   []logdoc.Match `json:"findings,omitempty"`
	Lines      []string       `json:"lines,omitempty"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
	// PID is the process that runs the job. A job that is running while its
	// process is gone is failed: nothing will finish it.
	PID int `json:"pid,omitempty"`
}

// JobProgress is the progress of a running job: the phase and the numbers
// of the work. The numbers are there when the step counts them; a step that
// only counts items keeps the done and the total of the step. Sentence is
// the one-line progress sentence the CLI and the dashboard show (R20).
type JobProgress struct {
	Phase string `json:"phase"`
	// Step is the step of the path the job is on now. A job that runs the
	// whole path names the step it is on.
	Step       string `json:"step,omitempty"`
	Sentence   string `json:"sentence,omitempty"`
	BytesDone  int64  `json:"bytes_done,omitempty"`
	BytesTotal int64  `json:"bytes_total,omitempty"`
	RateBPS    int64  `json:"rate_bps,omitempty"`
	FilesDone  int    `json:"files_done,omitempty"`
	FilesTotal int    `json:"files_total,omitempty"`
	ETAS       int    `json:"eta_s,omitempty"`
}

// jobMu holds the single job slot of this process.
var jobMu sync.Mutex

// jobCancels holds the cancel function of every job that runs in this
// process, by job id. The Stop control of the Setup tab calls it.
var jobCancels = map[string]context.CancelFunc{}

// jobCancelMu guards jobCancels.
var jobCancelMu sync.Mutex

// maxJobLines is the number of output lines a job keeps.
const maxJobLines = 50

// StartJob runs a prepared onboarding action in the background and returns
// the job. The job outlives the call: it runs until it is done or it fails.
// One job runs at a time; a second call gets ErrJobBusy.
func StartJob(ctx context.Context, prepared *PreparedStep) (*Job, error) {
	if prepared == nil || prepared.Action == nil {
		return nil, ErrNoAction
	}
	return startJob(ctx, prepared.Step, prepared.Action)
}

// startJob runs a prepared action in the background. One job runs at a time.
func startJob(ctx context.Context, step string, action *Action) (*Job, error) {
	if !jobMu.TryLock() {
		return nil, ErrJobBusy
	}
	id := newJobID()
	dir, err := paths.Ensure(jobDir(id))
	if err != nil {
		jobMu.Unlock()
		return nil, err
	}
	logf, err := os.Create(filepath.Join(dir, "job.log"))
	if err != nil {
		jobMu.Unlock()
		return nil, err
	}
	job := &Job{ID: id, Step: step, State: JobRunning, StartedAt: time.Now().UTC(), PID: os.Getpid()}
	if err := saveJob(job); err != nil {
		logf.Close()
		jobMu.Unlock()
		return nil, err
	}
	// The job outlives the request that started it, but the reader can stop
	// it: the cancel function is kept under the id of the job.
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	jobCancelMu.Lock()
	jobCancels[id] = cancel
	jobCancelMu.Unlock()
	started := *job // the caller's copy: runJob keeps writing job; GetJob reads later states
	go runJob(runCtx, job, action, logf)
	return &started, nil
}

// CancelJob stops the job with this id. It reports whether a running job of
// this process was found: a job of another process cannot be stopped here.
func CancelJob(id string) bool {
	jobCancelMu.Lock()
	cancel, ok := jobCancels[id]
	jobCancelMu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}

// runJob does the action of a job and records its end.
func runJob(ctx context.Context, job *Job, action *Action, logf *os.File) {
	defer jobMu.Unlock()
	defer logf.Close()
	defer func() {
		jobCancelMu.Lock()
		delete(jobCancels, job.ID)
		jobCancelMu.Unlock()
	}()
	w := &jobWriter{job: job, f: logf}
	_, err := action.Run(ctx, w)
	now := time.Now().UTC()
	job.FinishedAt = &now
	switch {
	case err == nil:
		job.State = JobDone
	case errors.Is(err, context.Canceled):
		job.State = JobFailed
		job.Error = "the job was cancelled"
	default:
		job.State = JobFailed
		job.Error = err.Error()
		job.Findings = findingsOf(ctx, action)
	}
	_ = saveJob(job)
}

// findingsOf scans the log of the run a serve job started. It returns the
// known failures of the catalog, or nothing.
func findingsOf(ctx context.Context, action *Action) []logdoc.Match {
	if action.Kind != ActionServe || action.RunName == "" {
		return nil
	}
	rules, err := logdoc.Load()
	if err != nil {
		return nil
	}
	rec, err := runner.Get(action.RunName)
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, findingsBudget)
	defer cancel()
	rc, err := runner.OpenLog(ctx, *rec, findingsBudget)
	if err != nil {
		return nil
	}
	defer rc.Close()
	return logdoc.Scan(rc, rules)
}

// findingsBudget bounds the scan of a log.
const findingsBudget = 2 * time.Second

// jobWriter writes the output of a job: to the log file, to the last lines
// of the job and to the phase of the job.
type jobWriter struct {
	mu      sync.Mutex
	job     *Job
	f       *os.File
	lines   []string
	partial string
}

// Write takes the output of the job. A partial line waits for the rest.
func (w *jobWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	text := w.partial + string(p)
	w.partial = ""
	for {
		i := strings.IndexByte(text, '\n')
		if i < 0 {
			break
		}
		w.line(strings.TrimRight(text[:i], "\r"))
		text = text[i+1:]
	}
	w.partial = text
	return len(p), nil
}

// line records one output line.
func (w *jobWriter) line(text string) {
	if text == "" {
		return
	}
	_, _ = io.WriteString(w.f, text+"\n")
	w.lines = append(w.lines, text)
	if len(w.lines) > maxJobLines {
		w.lines = w.lines[len(w.lines)-maxJobLines:]
	}
	w.job.Phase = text
	w.job.Lines = append([]string(nil), w.lines...)
	w.progress(text)
	_ = saveJob(w.job)
}

// progress reads the numbers of a line into the progress of the job: the
// phase and, for a download, the bytes and the rate. The download writes
// three kinds of line: the bytes line (parsed for rate and ETA), the
// sentence line (copied as the sentence of the job, R20), and a file
// marker line that updates the file count.
func (w *jobWriter) progress(text string) {
	p := w.job.Progress
	if p == nil {
		p = &JobProgress{Phase: w.job.Step}
		w.job.Progress = p
	}
	// A line that names a step ("== model.download: Download ...") says
	// which step of the path the job is on.
	if id, ok := stepMarker(text); ok {
		p.Step = id
		p.Phase = id
	}
	if !strings.HasPrefix(text, "model.download: ") {
		return
	}
	p.Phase = "model.download"
	rest := strings.TrimPrefix(text, "model.download: ")
	// Sentence line: starts with "<n>%" and uses middle dots to separate
	// the parts of the sentence (R20).
	if isSentenceLine(rest) {
		p.Sentence = strings.TrimSpace(rest)
		if f, t, ok := parseFileCount(rest); ok {
			p.FilesDone, p.FilesTotal = f, t
		}
		return
	}
	// File marker line: "file 7 of 14".
	if f, t, ok := parseFileLine(rest); ok {
		p.FilesDone, p.FilesTotal = f, t
		return
	}
	done, total, rate, ok := parseDownload(text)
	if !ok {
		return
	}
	p.BytesDone, p.BytesTotal, p.RateBPS = done, total, rate
	if rate > 0 && total > done {
		p.ETAS = int((total - done) / rate)
	}
}

// isSentenceLine reports whether the line that follows "model.download: "
// is a progress sentence (the "<n>% · ..." form), not a bytes line nor
// a file marker.
func isSentenceLine(rest string) bool {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return false
	}
	if !strings.Contains(rest, "\u00b7") {
		return false
	}
	i := strings.Index(rest, "%")
	if i <= 0 {
		return false
	}
	for _, c := range rest[:i] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// parseFileLine reads "file 7 of 14" into the done and the total.
func parseFileLine(rest string) (int, int, bool) {
	rest = strings.TrimSpace(rest)
	if !strings.HasPrefix(rest, "file ") {
		return 0, 0, false
	}
	rest = strings.TrimPrefix(rest, "file ")
	parts := strings.Split(rest, " of ")
	if len(parts) != 2 {
		return 0, 0, false
	}
	f, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, false
	}
	t, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, false
	}
	return f, t, true
}

// parseFileCount reads "file X of Y" out of a sentence tail and returns
// the done and the total. A missing pair returns ok=false.
func parseFileCount(sentence string) (int, int, bool) {
	i := strings.LastIndex(sentence, "\u00b7 file ")
	if i < 0 {
		return 0, 0, false
	}
	tail := "file " + sentence[i+len("\u00b7 file "):]
	return parseFileLine(tail)
}

// stepMarker reads the step of the path a line names:
// "== model.download: Download ..." gives the model step. The setup steps
// of the venv are the Python environment step.
func stepMarker(line string) (string, bool) {
	if !strings.HasPrefix(line, "== ") {
		return "", false
	}
	rest := strings.TrimPrefix(line, "== ")
	i := strings.Index(rest, ":")
	if i <= 0 {
		return "", false
	}
	id := strings.TrimSpace(rest[:i])
	switch id {
	case "venv.create", "venv.torch", "causal-conv1d.build", "flash-attn.build", "vllm.build", "venv.verify":
		return StepPythonEnv, true
	case "model.download":
		return StepModel, true
	}
	return id, true
}

// parseDownload reads the numbers of a download line:
// "model.download: 1.2 GB of 2.3 GB (52%) at 45.2 MB/s".
func parseDownload(line string) (done, total, rate int64, ok bool) {
	rest := strings.TrimPrefix(line, "model.download: ")
	at := strings.Split(rest, " at ")
	if len(at) != 2 {
		return 0, 0, 0, false
	}
	sizes := strings.Split(at[0], " of ")
	if len(sizes) != 2 {
		return 0, 0, 0, false
	}
	done, ok = parseBytes(sizes[0])
	if !ok {
		return 0, 0, 0, false
	}
	total, ok = parseBytes(stripPercent(sizes[1]))
	if !ok {
		return 0, 0, 0, false
	}
	rate, ok = parseBytes(strings.TrimSuffix(strings.TrimSpace(at[1]), "/s"))
	if !ok {
		return 0, 0, 0, false
	}
	return done, total, rate, true
}

// stripPercent removes the share of a size: "2.3 GB (52%)" is "2.3 GB".
func stripPercent(s string) string {
	if i := strings.Index(s, " ("); i >= 0 {
		return s[:i]
	}
	return s
}

// parseBytes reads a size in bytes: "512 B", "45.2 MB". The units are
// decimal, like the ones the download prints.
func parseBytes(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && (s[i] == '.' || (s[i] >= '0' && s[i] <= '9')) {
		i++
	}
	if i == 0 {
		return 0, false
	}
	n, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0, false
	}
	var mult int64
	switch strings.TrimSpace(s[i:]) {
	case "B", "":
		mult = 1
	case "kB":
		mult = 1000
	case "MB":
		mult = 1000 * 1000
	case "GB":
		mult = 1000 * 1000 * 1000
	case "TB":
		mult = 1000 * 1000 * 1000 * 1000
	default:
		return 0, false
	}
	return int64(n * float64(mult)), true
}

// JobSummary is the list document of a job: the id, the step, the state and
// the progress. The page re-attaches to a job with its id.
type JobSummary struct {
	ID         string       `json:"id"`
	Step       string       `json:"step"`
	State      string       `json:"state"`
	StartedAt  time.Time    `json:"started_at"`
	FinishedAt *time.Time   `json:"finished_at,omitempty"`
	Phase      string       `json:"phase,omitempty"`
	Progress   *JobProgress `json:"progress,omitempty"`
}

// DefaultJobLimit is the number of jobs the list answers.
const DefaultJobLimit = 50

// JobSummaries returns the jobs of this host, newest first, at most limit of
// them. A limit below one means the default.
func JobSummaries(limit int) []JobSummary {
	if limit < 1 {
		limit = DefaultJobLimit
	}
	jobs := RecentJobs()
	out := make([]JobSummary, 0, min(len(jobs), limit))
	for _, j := range jobs {
		if len(out) == limit {
			break
		}
		out = append(out, JobSummary{
			ID: j.ID, Step: j.Step, State: j.State, StartedAt: j.StartedAt,
			FinishedAt: j.FinishedAt, Phase: j.Phase, Progress: j.Progress,
		})
	}
	return out
}

// The store of the job documents lives in jobs_store.go.
