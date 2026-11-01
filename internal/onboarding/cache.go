package onboarding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/opengfx1030/bernie/internal/support/factcache"
	"github.com/opengfx1030/bernie/internal/support/hostroot"
)

// DocumentTTL is how long the cached onboarding document of this host stays
// fresh. The document is a fact of the host: it changes when the host does.
// The tests lower it.
var DocumentTTL = 15 * time.Second

// gather is the probe of the host. The tests replace it.
var gather = Gather

// Load returns the onboarding document of this host (spec §3.3). It serves
// the cached document when it is younger than the time to live. When it is
// older it serves it and refreshes it in the background, so a reader never
// waits for a probe it does not need.
//
// The document says how old it is: facts_age_s is the age of the facts in
// seconds, stale says that the document was past its time to live and that
// a refresh is running.
func Load(ctx context.Context, o Options) (*Document, error) {
	// The probe is read once here: the refresh may run in the background
	// after Load returns, and must not read the package variable then.
	probe := gather
	doc, age, stale, err := factcache.Get(ctx, documentKey(o), "", DocumentTTL,
		func(ctx context.Context) (*Document, error) {
			f, err := probe(ctx, o)
			if err != nil {
				return nil, err
			}
			return Build(f), nil
		})
	if err != nil {
		return nil, err
	}
	doc.FactsAgeS = int(age.Seconds())
	doc.Stale = stale
	return doc, nil
}

// documentKey is the cache key of the document of the given options.
func documentKey(o Options) string { return "onboarding/document/" + optionsFingerprint(o) }

// optionsFingerprint names the options of a document: two documents with
// other options are other facts. The root of the host tree is one of them:
// the document of a captured host is not the document of the real one.
func optionsFingerprint(o Options) string {
	h := sha256.New()
	root := o.Root
	if root == "" {
		root = hostroot.Root()
	}
	for _, s := range append([]string{root, o.Venv, o.Recipe, o.Mode, o.Engine, o.Name}, o.Sets...) {
		fmt.Fprintf(h, "%d:%s", len(s), s)
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}
