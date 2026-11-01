// Package weights names the exact model files a server loads: the files of
// a Hugging Face repo in the cache, or of a local directory, with a hash of
// each. A benchmark result stores them, so a number is tied to the weights
// it measured (spec §10).
package weights

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Model is the set of files of one model.
type Model struct {
	// Source is the HF repo id or the local directory of the plan.
	Source string `json:"source"`
	// Commit is the snapshot commit of an HF repo; empty for a local path.
	Commit string `json:"commit,omitempty"`
	Files  []File `json:"files"`
}

// File is one file of a model. SHA256 is the content hash. A small file
// of an HF repo that is kept in git and not in LFS has only its git blob
// hash (GitSHA1): the cache names it by that hash.
type File struct {
	Path    string `json:"path"`
	Bytes   int64  `json:"bytes"`
	SHA256  string `json:"sha256,omitempty"`
	GitSHA1 string `json:"git_sha1,omitempty"`
}

var (
	sha256Name = regexp.MustCompile(`^[0-9a-f]{64}$`)
	sha1Name   = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// Of lists the files of a model. source is an HF repo id (read from the hub
// cache under hfHome, at revision, "" = main) or an absolute directory
// (hashed, with the hashes kept in cacheDir; "" = no cache).
func Of(source, revision, hfHome, cacheDir string) (*Model, error) {
	if filepath.IsAbs(source) {
		files, err := localFiles(source, cacheDir)
		if err != nil {
			return nil, err
		}
		return &Model{Source: source, Files: files}, nil
	}
	return hubFiles(source, revision, hfHome)
}

// hubFiles reads a snapshot of the hub cache. Every file of a snapshot is
// a link to a blob, and the blob is named by its hash: the SHA-256 of the
// content for an LFS file, the git blob hash for a small one. Nothing is
// read or hashed.
func hubFiles(repo, revision, hfHome string) (*Model, error) {
	if revision == "" {
		revision = "main"
	}
	dir := filepath.Join(hfHome, "hub", "models--"+strings.ReplaceAll(repo, "/", "--"))
	commit := revision
	if b, err := os.ReadFile(filepath.Join(dir, "refs", revision)); err == nil {
		commit = strings.TrimSpace(string(b))
	}
	snap := filepath.Join(dir, "snapshots", commit)
	if _, err := os.Stat(snap); err != nil {
		return nil, fmt.Errorf("%s at %s is not in the cache %s", repo, revision, hfHome)
	}
	files := []File{}
	err := filepath.WalkDir(snap, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(snap, p)
		f := File{Path: filepath.ToSlash(rel)}
		if st, err := os.Stat(p); err == nil {
			f.Bytes = st.Size()
		}
		target, err := os.Readlink(p)
		if err != nil {
			return nil // a plain file in a snapshot has no hash by name
		}
		switch name := filepath.Base(target); {
		case sha256Name.MatchString(name):
			f.SHA256 = name
		case sha1Name.MatchString(name):
			f.GitSHA1 = name
		}
		files = append(files, f)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return &Model{Source: repo, Commit: commit, Files: files}, nil
}

// localFiles hashes the files of a directory. A hash is kept in cacheDir by
// path, size and modification time, so the weights are read once.
func localFiles(dir, cacheDir string) ([]File, error) {
	cache := loadCache(cacheDir, dir)
	files := []File{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != dir && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		st, err := os.Stat(p)
		if err != nil || !st.Mode().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		key := fmt.Sprintf("%s|%d|%d", rel, st.Size(), st.ModTime().UnixNano())
		sum, ok := cache.Sums[key]
		if !ok {
			if sum, err = hashFile(p); err != nil {
				return err
			}
			cache.Sums[key] = sum
		}
		files = append(files, File{Path: filepath.ToSlash(rel), Bytes: st.Size(), SHA256: sum})
		return nil
	})
	if err != nil {
		return nil, err
	}
	cache.save(cacheDir, dir)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// hashCache is the kept hashes of one directory, keyed by
// "path|size|mtime".
type hashCache struct {
	Sums map[string]string `json:"sums"`
}

func cacheFile(cacheDir, dir string) string {
	sum := sha256.Sum256([]byte(dir))
	return filepath.Join(cacheDir, hex.EncodeToString(sum[:8])+".json")
}

func loadCache(cacheDir, dir string) *hashCache {
	c := &hashCache{Sums: map[string]string{}}
	if cacheDir == "" {
		return c
	}
	if b, err := os.ReadFile(cacheFile(cacheDir, dir)); err == nil {
		_ = json.Unmarshal(b, c)
		if c.Sums == nil {
			c.Sums = map[string]string{}
		}
	}
	return c
}

func (c *hashCache) save(cacheDir, dir string) {
	if cacheDir == "" {
		return
	}
	b, err := json.Marshal(c)
	if err != nil {
		return
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
		return
	}
	_ = os.WriteFile(cacheFile(cacheDir, dir), b, 0o644)
}
