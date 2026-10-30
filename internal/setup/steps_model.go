package setup

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
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

// ModelStore is the Hugging Face cache under hfHome as the recipe fit sees
// it: the free bytes of its file system (0: unknown) and whether a repo is
// already in it.
func ModelStore(hfHome string) (free uint64, cached func(repo string) bool) {
	hub := hubCache(hubHome(hfHome))
	for dir := hub; ; dir = filepath.Dir(dir) {
		var st syscall.Statfs_t
		if syscall.Statfs(dir, &st) == nil {
			free = uint64(st.Bavail) * uint64(st.Bsize)
			break
		}
		if dir == filepath.Dir(dir) {
			break
		}
	}
	return free, func(repo string) bool { return modelCached(hub, repo) }
}
