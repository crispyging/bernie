package setup

import (
	"os"
	"path/filepath"
	"strings"
)

// modelDownload is the step that puts the model in the Hugging Face cache.
// bernie downloads the files itself: the hf tool gives every process its
// own temporary file, so an interrupted download starts over.
func (p planner) modelDownload() Step {
	if p.o.ModelRepo == "" {
		return Step{}
	}
	hub := hubCache(p.o.HFHome)
	return Step{
		ID: "model.download", Title: "Download " + p.o.ModelRepo, Mode: "both",
		Done: modelCached(hub, p.o.ModelRepo),
		Download: &Download{
			Repo:        p.o.ModelRepo,
			HubCache:    hub,
			ResumeBytes: partialBytes(hub, p.o.ModelRepo),
		},
		Why: "The model goes into the Hugging Face cache (" + hub + "), shared by host and container mode. " +
			"bernie downloads the files with a Range request, so an interrupted download continues where it stopped. " +
			"The hf tool does the same job (hf download " + p.o.ModelRepo + "), but it gives every process its own " +
			"temporary file: an interrupted download starts over.",
	}
}

// hubHome is the root of the Hugging Face cache: $HF_HOME, else
// ~/.cache/huggingface.
func hubHome(override string) string {
	if override == "" {
		override = os.Getenv("HF_HOME")
	}
	if override == "" {
		home, _ := os.UserHomeDir()
		override = filepath.Join(home, ".cache", "huggingface")
	}
	return override
}

// modelCached reports a complete snapshot of the repo in the hub cache.
func modelCached(hub, repo string) bool {
	if filepath.IsAbs(repo) {
		return isDir(repo)
	}
	dir := filepath.Join(hub, "models--"+strings.ReplaceAll(repo, "/", "--"))
	if _, err := os.Stat(filepath.Join(dir, "refs", "main")); err != nil {
		return false
	}
	snaps, _ := os.ReadDir(filepath.Join(dir, "snapshots"))
	for _, s := range snaps {
		files, _ := os.ReadDir(filepath.Join(dir, "snapshots", s.Name()))
		if len(files) > 0 {
			return true
		}
	}
	return false
}

// ModelCached reports a complete snapshot of the repo (or an existing local
// directory) in the Hugging Face cache of hfHome ("" = $HF_HOME, else
// ~/.cache/huggingface).
func ModelCached(hfHome, repo string) bool {
	return modelCached(hubCache(hubHome(hfHome)), repo)
}
