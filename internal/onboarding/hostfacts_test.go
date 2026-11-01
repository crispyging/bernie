package onboarding

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestStepsCarryTheJobOfTheStep checks that a step names the job that made
// it: the page re-attaches to it with the id.
func TestStepsCarryTheJobOfTheStep(t *testing.T) {
	now := time.Now().UTC()
	old := now.Add(-time.Hour)
	f := &Facts{Jobs: []Job{
		{ID: "job-old", Step: StepModel, State: JobDone, StartedAt: old, FinishedAt: &old},
		{ID: "job-new", Step: StepModel, State: JobRunning, StartedAt: now},
		{ID: "job-serve", Step: StepServe, State: JobDone, StartedAt: now, FinishedAt: &now},
	}}
	doc := Build(f)
	steps := map[string]Step{}
	for _, s := range doc.Steps {
		steps[s.ID] = s
	}
	model := steps[StepModel]
	if model.JobID != "job-new" || model.JobState != JobRunning {
		t.Errorf("the model step carries %q (%q)", model.JobID, model.JobState)
	}
	serve := steps[StepServe]
	if serve.JobID != "job-serve" || serve.JobState != JobDone {
		t.Errorf("the serve step carries %q (%q)", serve.JobID, serve.JobState)
	}
	// A job that finished long ago belongs to no step.
	if hw := steps[StepHardware]; hw.JobID != "" {
		t.Errorf("the hardware step carries the job %q", hw.JobID)
	}
}

// TestJobProgressReadsTheDownload checks the progress of a job: the phase
// and the numbers of a download.
func TestJobProgressReadsTheDownload(t *testing.T) {
	dir := t.TempDir()
	log, err := os.Create(filepath.Join(dir, "job.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	job := &Job{ID: "j1", Step: "model", State: JobRunning}
	w := &jobWriter{job: job, f: log}
	w.line("model.download: 1.2 GB of 2.3 GB (52%) at 45.2 MB/s")
	if job.Progress == nil {
		t.Fatal("the job has no progress")
	}
	p := job.Progress
	if p.Phase != "model.download" {
		t.Errorf("the phase is %q", p.Phase)
	}
	if p.BytesDone != 1_200_000_000 || p.BytesTotal != 2_300_000_000 {
		t.Errorf("the bytes are %d of %d", p.BytesDone, p.BytesTotal)
	}
	if p.RateBPS != 45_200_000 {
		t.Errorf("the rate is %d", p.RateBPS)
	}
	if p.ETAS != 24 {
		t.Errorf("the time to finish is %d s, want 24", p.ETAS)
	}
}

// TestJobProgressReadsTheSentence checks the sentence of a job: the
// dashboard and the CLI both read it, so the formatting matches the
// reference (R20).
func TestJobProgressReadsTheSentence(t *testing.T) {
	dir := t.TempDir()
	log, err := os.Create(filepath.Join(dir, "job.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	job := &Job{ID: "j2", Step: "model", State: JobRunning}
	w := &jobWriter{job: job, f: log}
	w.line("model.download: 1.2 GB of 2.3 GB (52%) at 45.2 MB/s")
	w.line("model.download: 52% \u00b7 1.2 GB of 2.3 GB \u00b7 45.2 MB/s \u00b7 about 24s left \u00b7 file 7 of 14")
	p := job.Progress
	if p == nil {
		t.Fatal("the job has no progress")
	}
	if p.Sentence == "" {
		t.Fatal("the sentence is empty")
	}
	want := "52% \u00b7 1.2 GB of 2.3 GB \u00b7 45.2 MB/s \u00b7 about 24s left \u00b7 file 7 of 14"
	if p.Sentence != want {
		t.Errorf("the sentence is %q, want %q", p.Sentence, want)
	}
	if p.FilesDone != 7 || p.FilesTotal != 14 {
		t.Errorf("the file count is %d of %d", p.FilesDone, p.FilesTotal)
	}
}

// TestJobProgressReadsTheFileMarker checks that a "file N of M" line
// updates the file count on its own (without a sentence line).
func TestJobProgressReadsTheFileMarker(t *testing.T) {
	dir := t.TempDir()
	log, err := os.Create(filepath.Join(dir, "job.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	job := &Job{ID: "j3", Step: "model", State: JobRunning}
	w := &jobWriter{job: job, f: log}
	w.line("model.download: file 7 of 14")
	p := job.Progress
	if p == nil {
		t.Fatal("the job has no progress")
	}
	if p.FilesDone != 7 || p.FilesTotal != 14 {
		t.Errorf("the file count is %d of %d", p.FilesDone, p.FilesTotal)
	}
}

// TestJobProgressOfAnotherStep checks that a step that counts no bytes
// still carries its phase.
func TestJobProgressOfAnotherStep(t *testing.T) {
	dir := t.TempDir()
	log, err := os.Create(filepath.Join(dir, "job.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	job := &Job{ID: "j2", Step: "serve", State: JobRunning}
	w := &jobWriter{job: job, f: log}
	w.line("the server answers /health")
	if job.Progress == nil || job.Progress.Phase != "serve" {
		t.Fatalf("the progress is %+v", job.Progress)
	}
	if job.Progress.BytesTotal != 0 {
		t.Errorf("the progress holds bytes: %+v", job.Progress)
	}
}
