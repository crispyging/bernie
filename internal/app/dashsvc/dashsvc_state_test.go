package dashsvc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/support/paths"
)

// This file owns the tests of the state on disk: the paths, the token
// file, the pidfile and the startup document. The service itself is
// tested in dashsvc_test.go.

// withStateDir redirects the XDG state directory for a single test.
// We restore the previous value at the end so other tests stay isolated.
// The cleanup runs even when the test fails; t.Cleanup is the standard
// hook for that.
func withStateDir(t *testing.T, dir string) {
	t.Helper()
	prev := os.Getenv("BERNIE_STATE_DIR")
	t.Setenv("BERNIE_STATE_DIR", dir)
	t.Cleanup(func() {
		if prev == "" {
			_ = os.Unsetenv("BERNIE_STATE_DIR")
		} else {
			_ = os.Setenv("BERNIE_STATE_DIR", prev)
		}
	})
}

// TestServicePathsAreUnderState verifies the XDG rule of spec §12: no
// file lands under /tmp. The state dir is the override or the default.
func TestServicePathsAreUnderState(t *testing.T) {
	withStateDir(t, t.TempDir())
	p := ServicePaths()
	if !strings.HasPrefix(p.State, os.Getenv("BERNIE_STATE_DIR")) {
		t.Errorf("state dir %q not under BERNIE_STATE_DIR", p.State)
	}
	if !strings.HasPrefix(p.Token, p.State) {
		t.Errorf("token %q not under state dir %q", p.Token, p.State)
	}
	if !strings.HasPrefix(p.Pid, p.State) {
		t.Errorf("pid %q not under state dir %q", p.Pid, p.State)
	}
	if !strings.HasPrefix(p.Log, p.State) {
		t.Errorf("log %q not under state dir %q", p.Log, p.State)
	}
	if !strings.HasPrefix(p.Startup, p.State) {
		t.Errorf("startup %q not under state dir %q", p.Startup, p.State)
	}
	// Default state dir is under HOME, never /tmp. Clear the overrides so
	// the default is what resolves.
	t.Setenv("BERNIE_STATE_DIR", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/home/bernie-test")
	def := ServicePaths()
	if strings.Contains(def.State, "/tmp/") {
		t.Errorf("default state dir %q must not be under /tmp", def.State)
	}
	// Also: paths.State() must agree (sanity).
	if got, want := def.State, filepath.Join(paths.State(), DirName); got != want {
		t.Errorf("paths.State()=%q want %q", got, want)
	}
}

// TestEnsureDirCreatesDir0700 verifies the state dir is created with a
// mode that does not let other users read the token file.
func TestEnsureDirCreatesDir0700(t *testing.T) {
	dir := t.TempDir()
	withStateDir(t, dir)
	p, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	info, err := os.Stat(p.State)
	if err != nil {
		t.Fatalf("stat state dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("state dir mode = %o, want 0700", perm)
	}
}

// TestReadTokenPersistsAndReuses verifies the persistent token file:
// missing → generate, present → reuse the same value, mode is 0600.
func TestReadTokenPersistsAndReuses(t *testing.T) {
	dir := t.TempDir()
	withStateDir(t, dir)
	tok1, _, err := ReadToken()
	if err != nil {
		t.Fatalf("first ReadToken: %v", err)
	}
	if tok1 == "" {
		t.Fatalf("first ReadToken returned empty")
	}
	info, err := os.Stat(ServicePaths().Token)
	if err != nil {
		t.Fatalf("stat token: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("token file mode = %o, want 0600", perm)
	}
	tok2, _, err := ReadToken()
	if err != nil {
		t.Fatalf("second ReadToken: %v", err)
	}
	if tok1 != tok2 {
		t.Errorf("token not persisted: %q vs %q", tok1, tok2)
	}
}

// TestWriteTokenRotates verifies WriteToken replaces the persisted value.
func TestWriteTokenRotates(t *testing.T) {
	dir := t.TempDir()
	withStateDir(t, dir)
	tok1, _, err := ReadToken()
	if err != nil {
		t.Fatalf("ReadToken: %v", err)
	}
	tok2 := "0123456789abcdef0123456789abcdef"
	if _, err := WriteToken(tok2); err != nil {
		t.Fatalf("WriteToken: %v", err)
	}
	got, _, err := ReadToken()
	if err != nil {
		t.Fatalf("ReadToken after rotate: %v", err)
	}
	if got != tok2 {
		t.Errorf("after rotate got %q, want %q", got, tok2)
	}
	if tok1 == tok2 {
		t.Errorf("rotate did not change token")
	}
}

// TestReadableTokenFollowsTheFile verifies that the readable token is
// the file content and that a missing file has none.
func TestReadableTokenFollowsTheFile(t *testing.T) {
	dir := t.TempDir()
	withStateDir(t, dir)
	if _, ok := ReadableToken(); ok {
		t.Errorf("ReadableToken on a missing file = ok, want not ok")
	}
	tok, _, err := ReadToken()
	if err != nil {
		t.Fatalf("ReadToken: %v", err)
	}
	got, ok := ReadableToken()
	if !ok {
		t.Fatalf("ReadableToken = not ok, want the token")
	}
	if got != tok {
		t.Errorf("ReadableToken = %q, want %q", got, tok)
	}
}

// TestIsAliveReportsDeadPID verifies the liveness check.
func TestIsAliveReportsDeadPID(t *testing.T) {
	if IsAlive(0) {
		t.Errorf("IsAlive(0) = true, want false")
	}
	if IsAlive(-1) {
		t.Errorf("IsAlive(-1) = true, want false")
	}
	// Pick a PID that almost certainly does not exist: high int.
	if IsAlive(999_999_999) {
		t.Errorf("IsAlive(high) = true, want false")
	}
}

// TestPidRoundTrip verifies the PID file helpers.
func TestPidRoundTrip(t *testing.T) {
	dir := t.TempDir()
	withStateDir(t, dir)
	p, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := WritePid(p, 12345); err != nil {
		t.Fatalf("WritePid: %v", err)
	}
	got, err := ReadPid(p)
	if err != nil {
		t.Fatalf("ReadPid: %v", err)
	}
	if got != 12345 {
		t.Errorf("ReadPid = %d, want 12345", got)
	}
	if err := RemovePid(p); err != nil {
		t.Fatalf("RemovePid: %v", err)
	}
	if _, err := ReadPid(p); !os.IsNotExist(err) {
		t.Errorf("ReadPid after remove err = %v, want not-exist", err)
	}
	if err := RemovePid(p); err != nil {
		t.Errorf("RemovePid idempotent: %v", err)
	}
}

// TestStartupRoundTrip verifies the startup document survives JSON.
func TestStartupRoundTrip(t *testing.T) {
	dir := t.TempDir()
	withStateDir(t, dir)
	p, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	want := StartupInfo{
		Listen:    "127.0.0.1:7860",
		URL:       "http://127.0.0.1:7860/?token=abc",
		PID:       4242,
		StartedAt: now,
		Version:   "v0.1.0-test",
		Manager:   "process",
	}
	if err := WriteStartup(p, want); err != nil {
		t.Fatalf("WriteStartup: %v", err)
	}
	info, err := ReadStartup(p)
	if err != nil {
		t.Fatalf("ReadStartup: %v", err)
	}
	if info.Listen != want.Listen || info.URL != want.URL || info.PID != want.PID || info.Version != want.Version || info.Manager != want.Manager {
		t.Errorf("ReadStartup = %+v, want %+v", info, want)
	}
	if !info.StartedAt.Equal(now) {
		t.Errorf("StartedAt = %v, want %v", info.StartedAt, now)
	}
}

// TestStartupReadMissing is the not-exist path of ReadStartup.
func TestStartupReadMissing(t *testing.T) {
	dir := t.TempDir()
	withStateDir(t, dir)
	p, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if _, err := ReadStartup(p); !os.IsNotExist(err) {
		t.Errorf("ReadStartup missing = %v, want not-exist", err)
	}
}
