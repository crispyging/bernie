package recipe

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
)

// contextSteps are the max-model-len values a plan tries, largest first,
// when the recipe's context does not fit the KV budget (spec §7.10).
var contextSteps = []int{32768, 16384, 8192}

// Adjustment is one change the plan made so the recipe fits this host.
type Adjustment struct {
	Setting string `json:"setting"` // args.max-model-len
	From    string `json:"from"`
	To      string `json:"to"`
	Reason  string `json:"reason"`
}

// fitContext lowers args.max-model-len of r to the largest step whose KV
// cache fits the budget. A max-model-len that an override sets is kept.
// The second result is a warning when the context stays too large.
func fitContext(r *Recipe, sets []string, gpus []gpu.GPU) (*Adjustment, string) {
	k := CheckKV(r, minMatchingVRAM(r, gpus))
	if k == nil || k.NeedBytes <= k.BudgetBytes {
		return nil, ""
	}
	if overrides(sets, "args.max-model-len") {
		return nil, k.Reason() + "; kept because an override sets it"
	}
	for _, n := range contextSteps {
		if n >= k.MaxModelLen || float64(n)*k.BytesPerToken > k.BudgetBytes {
			continue
		}
		r.Args["max-model-len"] = int64(n)
		return &Adjustment{
			Setting: "args.max-model-len",
			From:    strconv.Itoa(k.MaxModelLen),
			To:      strconv.Itoa(n),
			Reason:  k.Reason() + "; " + strconv.Itoa(n) + " fits",
		}, ""
	}
	return nil, k.Reason() + "; even " + strconv.Itoa(contextSteps[len(contextSteps)-1]) +
		" tokens do not fit: raise args.kv-cache-memory-bytes or use more GPUs"
}

// overrides reports a set (config default or --set) of the key.
func overrides(sets []string, key string) bool {
	for _, s := range sets {
		if k, _, ok := strings.Cut(s, "="); ok && strings.TrimSpace(k) == key {
			return true
		}
	}
	return false
}

// minMatchingVRAM is the VRAM of the smallest GPU of the recipe's arch, 0
// when there is no GPU data.
func minMatchingVRAM(r *Recipe, gpus []gpu.GPU) uint64 {
	var m uint64
	for _, g := range gpus {
		if gpu.ArchMatch(r.Requires.Arch, g.GFX) && (m == 0 || g.VRAMBytes < m) {
			m = g.VRAMBytes
		}
	}
	return m
}

// String is the adjustment in one line.
func (a Adjustment) String() string {
	return fmt.Sprintf("%s %s -> %s (%s)", a.Setting, a.From, a.To, a.Reason)
}
