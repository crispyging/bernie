package eval

import (
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"strings"
)

const dossierHeader = "Synthetic dossier. Archived records are distractors.\n"

// at pads the question to about size tokens of whole distractor records,
// the port of BroodBench's _pad_14. The evidence sits at the beginning, the
// middle or the end of the padding, on a record boundary. A family that
// asks for an exact value also gets archived near-miss records: the same
// shape as the truth, one character away. The filler and the near misses
// come from their own seeds, so the truth never depends on them.
func (t caseTemplate) at(i, size int, seed int64) Case {
	target := max(size*4, len(dossierHeader)+len(t.content)+2)
	padLen := target - len(dossierHeader) - len(t.content) - 2
	pad, offset := t.filler(i, size, seed, padLen)
	full := dossierHeader + pad[:offset] + "\n" + t.content + "\n" + pad[offset:]
	c := Case{
		ID: fmt.Sprintf("%s-%03d-ctx%d", t.family, i, size), Family: t.family, Variant: i,
		Position: []string{"beginning", "middle", "end"}[i%3], Track: t.track, Context: size,
		Estimate: Estimate{TargetTokens: size, Chars: len(full), EstimatedTokens: (len(full) + 2) / 4},
		Question: t.content,
		Messages: []Message{{Role: "system", Content: systemPrompt}, {Role: "user", Content: full}},
		Expected: t.expected, Kind: "answer", MaxSteps: 6,
	}
	if t.world != nil {
		c.Kind, c.World = "agent", t.world.clone()
		c.MaxSteps, c.MinToolCalls = AgentGuard, minToolCalls[t.family]
	}
	return c
}

// seeded is a generator keyed by a text, so two streams never share draws.
func seeded(key string) *rand.Rand {
	h := fnv.New64a()
	fmt.Fprint(h, key)
	return rand.New(rand.NewPCG(h.Sum64(), 0x706164))
}

// filler is n bytes of whole distractor records and the offset of the
// evidence in them.
func (t caseTemplate) filler(i, size int, seed int64, n int) (string, int) {
	padRng := seeded(fmt.Sprintf("%d:%d:%d", seed, i, size))
	nearRng := seeded(fmt.Sprintf("%d:%d:%d:near", seed, i, size))
	record := func(j int) string {
		return fmt.Sprintf("Archived record %06d: queue-%d sample=%d status=nominal.\n", j, 1000+padRng.IntN(9000), 100000+padRng.IntN(900000))
	}
	estimate := max(1, n/72)
	slots := map[int]bool{}
	if nearMissFamilies[t.family] {
		count := max(3, estimate/25)
		for _, j := range nearRng.Perm(max(estimate, count))[:count] {
			slots[j] = true
		}
	}
	var lines []string
	length := 0
	for {
		j := len(lines)
		line := record(j)
		if slots[j] {
			line = t.nearMiss(nearRng)
		}
		if length+len(line) > n {
			break
		}
		lines = append(lines, line)
		length += len(line)
	}
	cut := []int{0, len(lines) / 2, len(lines)}[i%3]
	var tail strings.Builder
	for k := len(lines); tail.Len() < n-length; k++ {
		tail.WriteString(record(k))
	}
	pad := strings.Join(lines, "") + tail.String()[:max(0, n-length)]
	return pad, len(strings.Join(lines[:cut], ""))
}

// nearID changes one digit of an identifier.
func nearID(rng *rand.Rand, code string) string {
	digits := []byte(strings.TrimPrefix(code, "ID-"))
	pos := rng.IntN(len(digits))
	d := byte('0' + rng.IntN(9))
	if d >= digits[pos] {
		d++
	}
	digits[pos] = d
	return "ID-" + string(digits)
}

// nearMiss is one archived record that looks like the evidence of the case
// but names another identifier.
func (t caseTemplate) nearMiss(rng *rand.Rand) string {
	nid := nearID(rng, t.code)
	other := func() string {
		for {
			if n := fmt.Sprintf("node-%d", 10+rng.IntN(89)); n != t.val {
				return n
			}
		}
	}
	switch t.family {
	case "multi-hop":
		node := other()
		return fmt.Sprintf("Archived records: %s belongs to service %s; service %s is owned by team ops-%d.\n", nid, node, node, rng.IntN(9))
	case "correction-precedence":
		return fmt.Sprintf("Archived ticket: %s was at obsolete-%d; a later correction moved %s to %s.\n", nid, 1+rng.IntN(8), nid, other())
	case "in-context-memory":
		return fmt.Sprintf("Archived ephemeral token=%s (expired; from an earlier prompt).\n", nid)
	case "missing-evidence":
		return fmt.Sprintf("Archived report %s: deployment event recorded at 2025-0%d-%dT%d:00Z.\n", nid, 1+rng.IntN(9), 10+rng.IntN(19), 10+rng.IntN(14))
	}
	return fmt.Sprintf("Archived inventory record: %s is assigned to %s.\n", nid, other())
}
