package eval

import (
	"errors"
	"fmt"
	"math"
)

// commonSizes are the context bands a plan picks from: the powers of two and
// the 1.5× steps between them, all multiples of 2048.
var commonSizes = []int{2048, 4096, 8192, 12288, 16384, 24576, 32768, 49152, 65536, 98304,
	131072, 196608, 262144, 393216, 524288, 786432, 1048576}

// planBands is the number of context bands of a suite with --context auto.
var planBands = map[string]int{"smoke": 3, "core": 4, "extended": 6}

// ContextPlan is how --context auto picked the context sizes of a run
// (spec §10.5): the smallest max_model_len of the models, the room kept
// for the reply and the tools, and the bands up to the ceiling.
type ContextPlan struct {
	ModelLimit int   `json:"model_limit"`
	Headroom   int   `json:"headroom"`
	Ceiling    int   `json:"ceiling"`
	Contexts   []int `json:"contexts"`
}

// Plan picks the context bands of a suite for a server whose smallest
// max_model_len is limit. The longest band stays a reply and the tools
// below the limit, rounded down to a multiple of 2048.
func Plan(suite string, limit, maxTokens int) (*ContextPlan, error) {
	n, ok := planBands[suite]
	switch {
	case !ok:
		return nil, fmt.Errorf("unknown suite %q", suite)
	case limit <= 0:
		return nil, errors.New("the server reports no max_model_len, so --context auto has no ceiling; pass the sizes")
	}
	p := &ContextPlan{ModelLimit: limit, Headroom: max(4096, maxTokens+1024)}
	p.Ceiling = limit - p.Headroom
	if p.Ceiling < 1 {
		return nil, fmt.Errorf("max_model_len %d leaves no room for a prompt after %d tokens of headroom", limit, p.Headroom)
	}
	long := p.Ceiling
	if long >= 4096 {
		long = long / 2048 * 2048
	}
	p.Contexts = Ladder(long, n)
	return p, nil
}

// Ladder is the bands from a 2048-token baseline to long: count bands with
// the most even steps the common sizes allow. It first keeps the largest
// step between two neighbours as small as it can be, then stays closest to
// an even log scale, so a sweep never jumps from a middle length straight
// to the model's maximum.
func Ladder(long, count int) []int {
	if count <= 1 || long <= 2048 {
		return []int{long}
	}
	var inner []int
	for _, s := range commonSizes {
		if s > 2048 && s < long {
			inner = append(inner, s)
		}
	}
	if len(inner) <= count-2 {
		return append(append([]int{2048}, inner...), long)
	}
	ideal := make([]float64, count)
	for i := range ideal {
		ideal[i] = math.Log(2048) + (math.Log(float64(long))-math.Log(2048))*float64(i)/float64(count-1)
	}
	score := func(bands []int) (float64, float64) {
		worst, drift := 0.0, 0.0
		for i, b := range bands {
			if i > 0 {
				worst = max(worst, float64(b)/float64(bands[i-1]))
			}
			d := math.Log(float64(b)) - ideal[i]
			drift += d * d
		}
		return math.Round(worst*1e6) / 1e6, drift
	}
	var best []int
	bestWorst, bestDrift := math.Inf(1), math.Inf(1)
	pick := make([]int, 0, count-2)
	var walk func(from int)
	walk = func(from int) {
		if len(pick) == count-2 {
			bands := append(append([]int{2048}, pick...), long)
			w, d := score(bands)
			if w < bestWorst || (w == bestWorst && d < bestDrift) {
				best, bestWorst, bestDrift = bands, w, d
			}
			return
		}
		for i := from; i < len(inner); i++ {
			pick = append(pick, inner[i])
			walk(i + 1)
			pick = pick[:len(pick)-1]
		}
	}
	walk(0)
	return best
}
