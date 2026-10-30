package recipe

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
)

// KVReserve is the VRAM of one GPU that the KV budget leaves for
// activations, CUDA graphs and the allocator (spec §7.8).
const KVReserve = 2 * gib

// MinContext is the smallest max-model-len the plan lowers to (spec §7.8).
const MinContext = 8192

// DefaultMemoryUtilization is vLLM's gpu-memory-utilization default.
const DefaultMemoryUtilization = 0.9

// contextSteps are the max-model-len values the plan tries, largest first.
var contextSteps = []int{131072, 65536, 32768, 16384, MinContext}

// kvTokens estimates how many tokens of KV cache fit on GPUs of vramBytes
// each (spec §7.8). The weights (weights_gb, or download_gb) are split over
// the ranks; the KV cache of a token is too. A recipe that sets
// kv-cache-memory-bytes fixes the KV cache of a GPU, so that size counts and
// gpu-memory-utilization does not. ok is false when the recipe lacks the data.
// weightsFit is false when the weights and the KV cache do not fit.
func kvTokens(rq Requires, args map[string]any, vramBytes uint64) (tokens int, weightsFit, ok bool) {
	if rq.DownloadGB <= 0 || rq.KVBytesPerToken <= 0 || vramBytes == 0 {
		return 0, false, false
	}
	world, set := worldSize(args)
	if !set || world < 1 {
		world = 1
	}
	weights := rq.WeightsGB
	if weights <= 0 {
		weights = rq.DownloadGB
	}
	room := float64(vramBytes) - weights*1e9/float64(world) - KVReserve
	if kv, found := toFloat(args["kv-cache-memory-bytes"]); found && kv > 0 {
		if room < kv {
			return 0, false, true
		}
		return int(kv * float64(world) / rq.KVBytesPerToken), true, true
	}
	util := DefaultMemoryUtilization
	if v, found := toFloat(args["gpu-memory-utilization"]); found && v > 0 && v <= 1 {
		util = v
	}
	budget := float64(vramBytes)*util - weights*1e9/float64(world) - KVReserve
	if budget <= 0 {
		return 0, false, true
	}
	return int(budget * float64(world) / rq.KVBytesPerToken), true, true
}

// fitContext returns the largest step below maxLen that fits in tokens, or 0
// when not even MinContext fits.
func fitContext(maxLen, tokens int) int {
	for _, n := range contextSteps {
		if n < maxLen && n <= tokens {
			return n
		}
	}
	return 0
}

// maxModelLen returns the recipe's max-model-len, when it sets a number.
func maxModelLen(args map[string]any) (int, bool) {
	v, ok := args["max-model-len"]
	if !ok || v == nil {
		return 0, false
	}
	n, ok := toInt(v)
	return n, ok && n > 0
}

// minVRAM returns the smallest VRAM of the GPUs that keep returns true for.
func minVRAM(gpus []gpu.GPU, keep func(gpu.GPU) bool) uint64 {
	var low uint64
	for _, g := range gpus {
		if keep(g) && g.VRAMBytes > 0 && (low == 0 || g.VRAMBytes < low) {
			low = g.VRAMBytes
		}
	}
	return low
}

// contextReasons are the fit reasons of the KV budget: tight when the plan
// will lower max-model-len, no when the weights or MinContext do not fit.
func contextReasons(r *Recipe, vramBytes uint64) (tight, no []string) {
	tokens, weightsFit, ok := kvTokens(r.Requires, r.Args, vramBytes)
	if !ok {
		return nil, nil
	}
	vram := float64(vramBytes) / gib
	switch {
	case !weightsFit:
		weights := r.Requires.WeightsGB
		if weights <= 0 {
			weights = r.Requires.DownloadGB
		}
		return nil, []string{fmt.Sprintf("vram: %g GB of weights leave no KV cache on %.0f GiB GPUs", weights, vram)}
	case tokens < MinContext:
		return nil, []string{fmt.Sprintf("vram: the KV cache holds about %d tokens on %.0f GiB GPUs; the floor is %d", tokens, vram, MinContext)}
	}
	maxLen, set := maxModelLen(r.Args)
	if set && tokens < maxLen {
		return []string{fmt.Sprintf("context: the KV cache holds about %d tokens; the plan lowers max-model-len from %d to %d",
			tokens, maxLen, fitContext(maxLen, tokens))}, nil
	}
	return nil, nil
}

func toFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(t, 64)
		return f, err == nil
	}
	return 0, false
}

// Adjustment is one setting the plan changed to fit the host.
type Adjustment struct {
	Setting string `json:"setting"`
	From    string `json:"from"`
	To      string `json:"to"`
	Reason  string `json:"reason"`
}

// shrinkContext lowers args.max-model-len to the largest step whose KV
// cache fits the selected GPUs, and records the change (spec §7.8). It
// leaves a max-model-len from --set or the config alone.
func shrinkContext(r *Recipe, p *Plan, o PlanOptions, interp func(string) string) error {
	maxLen, set := maxModelLen(r.Args)
	if !set || len(p.GPUs) == 0 || setsArg(o.Sets, "max-model-len") {
		return nil
	}
	selected := map[int]bool{}
	for _, i := range p.GPUs {
		selected[i] = true
	}
	vram := minVRAM(o.GPUs, func(g gpu.GPU) bool { return selected[g.HIPIndex] })
	tokens, weightsFit, ok := kvTokens(r.Requires, r.Args, vram)
	if !ok || tokens >= maxLen {
		return nil
	}
	to := 0
	if weightsFit {
		to = fitContext(maxLen, tokens)
	}
	if to == 0 {
		p.Warnings = append(p.Warnings, fmt.Sprintf("the KV cache holds about %d tokens on the selected GPUs, below the %d floor; vLLM will likely refuse to start", tokens, MinContext))
		return nil
	}
	args := make(map[string]any, len(r.Args))
	for k, v := range r.Args {
		args[k] = v
	}
	args["max-model-len"] = to
	r.Args = args
	if err := planArgs(r, p, interp); err != nil {
		return err
	}
	p.Adjustments = append(p.Adjustments, Adjustment{
		Setting: "args.max-model-len", From: strconv.Itoa(maxLen), To: strconv.Itoa(to),
		Reason: fmt.Sprintf("the KV cache holds about %d tokens on %.0f GiB GPUs; set args.max-model-len to keep a value", tokens, float64(vram)/gib),
	})
	return nil
}

// setsArg reports whether sets name args.<flag> (or replace all args).
func setsArg(sets []string, flag string) bool {
	for _, s := range sets {
		o, err := ParseSet(s)
		if err != nil || o.Input || len(o.Path) == 0 || o.Path[0] != "args" {
			continue
		}
		if len(o.Path) == 1 || o.Path[1] == flag {
			return true
		}
	}
	return false
}
