package onboarding

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/support/factcache"
)

// stubGather replaces the probe of the host with one that counts its runs.
func stubGather(t *testing.T, calls *atomic.Int64) {
	t.Helper()
	old := gather
	oldTTL := DocumentTTL
	t.Cleanup(func() { gather = old; DocumentTTL = oldTTL })
	gather = func(context.Context, Options) (*Facts, error) {
		calls.Add(1)
		return &Facts{GPUs: []gpu.GPU{}}, nil
	}
}

func TestLoadProbesTheHostOnce(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	var calls atomic.Int64
	stubGather(t, &calls)

	doc, err := Load(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if doc.FactsAgeS != 0 || doc.Stale {
		t.Errorf("a fresh document = age %d, stale %v, want 0 and false", doc.FactsAgeS, doc.Stale)
	}
	doc, err = Load(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("the host was probed %d times, want 1", n)
	}
	if doc.Stale {
		t.Error("a cached document younger than its time to live is stale")
	}
}

func TestLoadServesTheStaleDocument(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	var calls atomic.Int64
	stubGather(t, &calls)
	DocumentTTL = time.Millisecond

	if _, err := Load(context.Background(), Options{}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	refreshStarted := time.Now()
	defer func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			var refreshed *Document
			age, _ := factcache.Load(documentKey(Options{}), "", time.Hour, &refreshed)
			if calls.Load() > 1 && refreshed != nil && age < time.Since(refreshStarted) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Error("the background refresh did not publish a new cached document")
	}()
	doc, err := Load(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !doc.Stale {
		t.Error("a document past its time to live is not stale")
	}
	if doc.FactsAgeS < 1 {
		t.Errorf("the age of a stale document = %d, want at least a second", doc.FactsAgeS)
	}
}

func TestLoadKeepsTheOptionsApart(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	var calls atomic.Int64
	stubGather(t, &calls)

	if _, err := Load(context.Background(), Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(context.Background(), Options{Recipe: "qwen"}); err != nil {
		t.Fatal(err)
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("the host was probed %d times, want 2: other options are other facts", n)
	}
}

func TestLoadKeepsTheRootsApart(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	var calls atomic.Int64
	stubGather(t, &calls)

	if _, err := Load(context.Background(), Options{Root: "/srv/one"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(context.Background(), Options{Root: "/srv/two"}); err != nil {
		t.Fatal(err)
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("the host was probed %d times, want 2: another host tree is another fact", n)
	}
}
