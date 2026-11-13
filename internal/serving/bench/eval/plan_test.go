package eval

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// TestPlanMatchesBroodBench checks the bands against the output of
// BroodBench's context_planner.context_ladder (1deb6ad) for the same inputs.
func TestPlanMatchesBroodBench(t *testing.T) {
	cases := []struct {
		suite            string
		limit, maxTokens int
		ceiling          int
		want             []int
	}{
		{"smoke", 32768, 1024, 28672, []int{2048, 8192, 28672}},
		{"core", 32768, 1024, 28672, []int{2048, 4096, 12288, 28672}},
		{"extended", 131072, 1024, 126976, []int{2048, 4096, 12288, 24576, 49152, 126976}},
		{"extended", 262144, 1024, 258048, []int{2048, 4096, 12288, 32768, 98304, 258048}},
		{"core", 40960, 1024, 36864, []int{2048, 4096, 12288, 36864}},
		{"smoke", 8192, 1024, 4096, []int{2048, 4096}},
		{"smoke", 5000, 256, 904, []int{904}},
	}
	for _, c := range cases {
		p, err := Plan(c.suite, c.limit, c.maxTokens)
		if err != nil {
			t.Fatalf("%s %d: %v", c.suite, c.limit, err)
		}
		if p.Ceiling != c.ceiling || !slices.Equal(p.Contexts, c.want) {
			t.Errorf("%s %d: ceiling %d bands %v, want %d %v", c.suite, c.limit, p.Ceiling, p.Contexts, c.ceiling, c.want)
		}
	}
}

// TestPlanRefuses checks the inputs Plan cannot turn into bands.
func TestPlanRefuses(t *testing.T) {
	for _, c := range []struct {
		suite        string
		limit, reply int
		want         string
	}{
		{"smoke", 0, 1024, "no max_model_len"},
		{"smoke", 4096, 1024, "no room for a prompt"},
		{"huge", 32768, 1024, "unknown suite"},
	} {
		if _, err := Plan(c.suite, c.limit, c.reply); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("Plan(%s, %d) = %v, want %q", c.suite, c.limit, err, c.want)
		}
	}
}

// TestAdmitTrimsOnlyDistractors checks that admission removes distractor
// records until the tokenizer count fits the band, and keeps the evidence.
func TestAdmitTrimsOnlyDistractors(t *testing.T) {
	cases, _ := Generate("smoke", 5, []int{8192}, []string{"recall"})
	c := &cases[0]
	count := func(_ context.Context, msgs []Message, _ []Tool) (int, int, bool) {
		n := 0
		for _, m := range msgs {
			n += len(m.Content) / 4
		}
		return n, 32768, true
	}
	got, a, err := admit(context.Background(), count, c, 1024, 6000)
	if err != nil {
		t.Fatal(err)
	}
	if a.CountSource != "serving_tokenizer" || a.Budget != 6000-1536 || a.PromptTokens > a.Budget || a.Removed == 0 {
		t.Errorf("admission = %+v, want a tokenizer count under %d with records removed", a, 6000-1536)
	}
	if !strings.Contains(got.Messages[1].Content, c.Question) {
		t.Error("admission cut the evidence")
	}
	if c.Messages[1].Content == got.Messages[1].Content {
		t.Error("admission changed nothing")
	}
	if len(c.Messages[1].Content) <= len(got.Messages[1].Content) {
		t.Error("admission changed the generated case instead of a copy")
	}

	// A band smaller than the evidence cannot be met without cutting it.
	_, _, err = admit(context.Background(), count, c, 1024, 1500)
	if err == nil || !strings.Contains(err.Error(), "no room for the prompt") {
		t.Errorf("err = %v, want no room", err)
	}
	// A prompt that stays too long after the last distractor is gone is
	// refused, never cut further.
	short, _ := Generate("smoke", 5, []int{512}, []string{"recall"})
	big := func(_ context.Context, _ []Message, _ []Tool) (int, int, bool) { return 5000, 0, true }
	if _, _, err = admit(context.Background(), big, &short[0], 16, 2048); err == nil || !strings.Contains(err.Error(), "no evidence was cut") {
		t.Errorf("err = %v, want the evidence to stay whole", err)
	}
}

// TestAdmitWithoutATokenizer checks the byte bound and the server without a
// limit.
func TestAdmitWithoutATokenizer(t *testing.T) {
	cases, _ := Generate("smoke", 5, []int{512}, []string{"recall"})
	none := func(context.Context, []Message, []Tool) (int, int, bool) { return 0, 0, false }
	got, a, err := admit(context.Background(), none, &cases[0], 1024, 0)
	if err != nil || a != nil || got != &cases[0] {
		t.Errorf("no limit, no tokenizer: admission %+v err %v, want the case as it is", a, err)
	}
	_, a, err = admit(context.Background(), none, &cases[0], 1024, 32768)
	if err != nil || a == nil || a.CountSource != "conservative_byte_admission" || a.UpperBound == 0 || a.Removed != 0 {
		t.Errorf("byte bound: admission %+v err %v", a, err)
	}
}
