package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/opengfx1030/bernie/internal/serving/runner"
	"github.com/opengfx1030/bernie/internal/support/hostroot"
	"github.com/opengfx1030/bernie/internal/support/netx"
)

// AttachOptions are the inputs of Attach: the URL (or port) of an already
// running vLLM server, the name to register it under and the recipe it
// belongs to (optional).
type AttachOptions struct {
	URL    string
	Name   string
	Recipe string
}

// AttachResult is the answer of `bernie attach --json`: the record of the
// attached server and the sentence that says what bernie did.
type AttachResult struct {
	Run  *runner.Record `json:"run"`
	Note string         `json:"note"`
}

// Attach registers an already running vLLM server as a run. bernie never
// starts or stops it: the server keeps running when bernie stops (R29).
// The health check, the metrics and the models list are checked first; a
// server that fails one of them is refused with the reason.
func Attach(ctx context.Context, o AttachOptions) (*AttachResult, error) {
	rec, err := runner.Attach(ctx, runner.AttachOptions{Name: o.Name, URL: o.URL, Recipe: o.Recipe})
	if err != nil {
		return nil, err
	}
	return &AttachResult{Run: rec, Note: AttachSentence(&AttachResult{Run: rec})}, nil
}

// AttachSentence is the one sentence the CLI, the MCP tool and the page
// print after an attach: what bernie did, and what it will not do.
func AttachSentence(r *AttachResult) string {
	if r == nil || r.Run == nil {
		return ""
	}
	return fmt.Sprintf("attached %s at %s; bernie did not start it and will not stop it", r.Run.Name, r.Run.URL())
}

// ServerTarget reports whether the dashboard may send requests to target:
// a server of this host, or a server the operator attached with `bernie
// attach` (spec §8.4). Any other address is refused, so a page cannot
// point bernie at a host nobody registered.
func ServerTarget(ctx context.Context, target string) bool {
	if netx.Loopback(target) {
		return true
	}
	runs, err := runner.List(ctx)
	if err != nil {
		return false
	}
	want := strings.TrimRight(strings.TrimSuffix(strings.TrimSpace(target), "/v1"), "/")
	for _, r := range runs {
		if r.Foreign && r.URL() == want {
			return true
		}
	}
	return false
}

// Discover lists the vLLM servers that run on this host (spec §8.5). It
// only reads.
func Discover(ctx context.Context) ([]runner.Found, error) {
	return runner.Discover(ctx, hostroot.Root())
}
