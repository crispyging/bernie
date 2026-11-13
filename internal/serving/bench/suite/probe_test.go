package suite

import (
	"testing"

	"github.com/opengfx1030/bernie/internal/serving/bench"
)

// TestMedianProbe checks the merge of the probes of the repetitions: the
// median rate of the probes that measured one, the summed counts, and the
// reason when none measured a rate.
func TestMedianProbe(t *testing.T) {
	rep := func(p *bench.Probe) bench.Measurement { return bench.Measurement{Probe: p} }
	got := medianProbe([]bench.Measurement{
		rep(&bench.Probe{Requests: 4, Completed: 4, AggTokS: 9000, WallS: 1}),
		rep(&bench.Probe{Requests: 4, Completed: 3, Unavailable: "1 of 4 probe requests failed"}),
		rep(&bench.Probe{Requests: 4, Completed: 4, AggTokS: 8000, WallS: 2}),
		rep(nil),
	})
	if got == nil || got.AggTokS != 8500 || got.Requests != 12 || got.Completed != 11 || got.Unavailable != "" {
		t.Errorf("merged probe = %+v, want 8500 tok/s over 11 of 12 requests", got)
	}
	none := medianProbe([]bench.Measurement{rep(&bench.Probe{Requests: 1, Unavailable: "the server sent no token usage for the probe"})})
	if none == nil || none.AggTokS != 0 || none.Unavailable == "" {
		t.Errorf("merged probe = %+v, want no rate and the reason", none)
	}
	if medianProbe([]bench.Measurement{rep(nil)}) != nil {
		t.Error("a cell without a probe got one")
	}
}

// TestTheProbeKeepsOldDigests checks that the probe field leaves the digest
// of a suite that does not use it unchanged.
func TestTheProbeKeepsOldDigests(t *testing.T) {
	s := &Suite{Schema: 1, ID: "x", Name: "x", Reps: 1}
	before := s.Digest()
	s.Probe = true
	if s.Digest() == before {
		t.Error("turning the probe on kept the digest")
	}
	s.Probe = false
	if s.Digest() != before {
		t.Error("the probe field changed the digest of a suite without it")
	}
}
