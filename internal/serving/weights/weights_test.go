package weights

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// hubRepo writes the cache layout of one repo: refs/main, a snapshot whose
// files link to blobs named by their hash.
func hubRepo(t *testing.T, home, repo, commit string, links map[string]string) {
	t.Helper()
	dir := filepath.Join(home, "hub", "models--"+strings.ReplaceAll(repo, "/", "--"))
	snap := filepath.Join(dir, "snapshots", commit)
	for _, d := range []string{filepath.Join(dir, "refs"), filepath.Join(dir, "blobs"), snap} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "refs", "main"), []byte(commit+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, blob := range links {
		if err := os.WriteFile(filepath.Join(dir, "blobs", blob), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join("..", "..", "blobs", blob), filepath.Join(snap, name)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHubFilesAreNamedByTheirBlobs(t *testing.T) {
	home := t.TempDir()
	lfs := strings.Repeat("ab", 32)
	git := strings.Repeat("cd", 20)
	hubRepo(t, home, "org/model", "c0ffee", map[string]string{"model.safetensors": lfs, "config.json": git})
	m, err := Of("org/model", "", home, "")
	if err != nil {
		t.Fatal(err)
	}
	if m.Commit != "c0ffee" || len(m.Files) != 2 {
		t.Fatalf("model = %+v", m)
	}
	cfg, w := m.Files[0], m.Files[1]
	if cfg.Path != "config.json" || cfg.GitSHA1 != git || cfg.SHA256 != "" {
		t.Errorf("config = %+v", cfg)
	}
	if w.Path != "model.safetensors" || w.SHA256 != lfs || w.Bytes != int64(len("model.safetensors")) {
		t.Errorf("weights = %+v", w)
	}
	if _, err := Of("org/other", "", home, ""); err == nil {
		t.Error("a repo that is not cached has files")
	}
}

func TestLocalFilesAreHashedOnce(t *testing.T) {
	dir, cache := t.TempDir(), t.TempDir()
	p := filepath.Join(dir, "model.safetensors")
	if err := os.WriteFile(p, []byte("weights-1"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("weights-1"))
	m, err := Of(dir, "", "", cache)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Files) != 1 || m.Files[0].SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("files = %+v", m.Files)
	}
	// Same size and time: the kept hash is used, the file is not read.
	st, _ := os.Stat(p)
	if err := os.WriteFile(p, []byte("weights-2"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(p, st.ModTime(), st.ModTime())
	again, _ := Of(dir, "", "", cache)
	if again.Files[0].SHA256 != m.Files[0].SHA256 {
		t.Error("the file was hashed again although its size and time did not change")
	}
	// A new time: the file is hashed again.
	_ = os.Chtimes(p, time.Now().Add(time.Hour), time.Now().Add(time.Hour))
	fresh, _ := Of(dir, "", "", cache)
	if fresh.Files[0].SHA256 == m.Files[0].SHA256 {
		t.Error("a changed file kept its old hash")
	}
}
