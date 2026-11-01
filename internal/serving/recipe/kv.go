package recipe

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// defaultGPUMemoryUtilization is the vLLM default of gpu-memory-utilization.
const defaultGPUMemoryUtilization = 0.9

// KV is the KV cache one request of max-model-len needs, next to the KV
// budget of one model replica (spec §7.8).
type KV struct {
	MaxModelLen   int     `json:"max_model_len"`
	BytesPerToken float64 `json:"bytes_per_token"`
	NeedBytes     float64 `json:"need_bytes"`
	BudgetBytes   float64 `json:"budget_bytes"`
	// Source says where the budget comes from: "kv-cache-memory-bytes" (the
	// recipe reserves it) or "estimate" (the VRAM share left after the
	// weights).
	Source string `json:"source"`
}

// Tokens is the number of tokens the budget holds.
func (k KV) Tokens() int { return int(k.BudgetBytes / k.BytesPerToken) }

// Reason is the one line that says the context does not fit the budget.
func (k KV) Reason() string {
	return fmt.Sprintf("kv: max-model-len %d needs %.1f GiB of KV cache; the budget is %.1f GiB, about %d tokens",
		k.MaxModelLen, k.NeedBytes/gib, k.BudgetBytes/gib, k.Tokens())
}

// CheckKV compares the KV cache of one max-model-len request with the
// budget of a replica. The budget is args.kv-cache-memory-bytes per rank
// when the recipe sets it. Else it is the gpu-memory-utilization share of
// the smallest matching card minus the weights (requires.download_gb),
// over the ranks of a replica; minVRAM == 0 means no GPU data. Nil when
// the recipe gives no kv_bytes_per_token, no numeric max-model-len, or no
// way to size the budget.
func CheckKV(r *Recipe, minVRAM uint64) *KV {
	per := r.Requires.KVBytesPerToken
	n, ok := toInt(r.Args["max-model-len"])
	if per <= 0 || !ok || n <= 0 {
		return nil
	}
	ranks := r.Requires.GPUs
	if ranks < 1 {
		ranks = 1
	}
	if dp, ok := toInt(r.Args["data-parallel-size"]); ok && dp > 1 {
		ranks /= dp
	}
	k := &KV{MaxModelLen: n, BytesPerToken: per, NeedBytes: per * float64(n)}
	if mem, ok := toFloat(r.Args["kv-cache-memory-bytes"]); ok && mem > 0 {
		k.BudgetBytes, k.Source = mem*float64(ranks), "kv-cache-memory-bytes"
		return k
	}
	weights := r.Requires.DownloadGB * 1e9
	if minVRAM == 0 || weights <= 0 {
		return nil
	}
	util, ok := toFloat(r.Args["gpu-memory-utilization"])
	if !ok || util <= 0 {
		util = defaultGPUMemoryUtilization
	}
	budget := float64(ranks)*float64(minVRAM)*util - weights
	if budget <= 0 {
		return nil // the weights alone do not fit: requires.vram_gib says so
	}
	k.BudgetBytes, k.Source = budget, "estimate"
	return k
}

func toFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case float64:
		return t, true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(t, 64)
		return f, err == nil
	}
	return 0, false
}
