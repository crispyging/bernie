package bench

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/serving/vllm"
	"github.com/opengfx1030/bernie/internal/serving/vllm/vllmtest"
)

func TestPromptsSaltAndDeterminism(t *testing.T) {
	p := prompts{seed: 7, nonce: 42}
	seen := map[string]bool{}
	for seq := range 500 {
		s := p.salt(seq)
		if seen[s] {
			t.Fatalf("salt %q repeats", s)
		}
		seen[s] = true
		if len(s) != len(p.salt(0)) {
			t.Fatalf("salt %q has variable width", s)
		}
	}
	if a, b := p.build(3, 50), p.build(3, 50); a != b {
		t.Errorf("same seed and seq gave different prompts")
	}
	if a, b := p.build(3, 50), (prompts{seed: 8, nonce: 42}).build(3, 50); a == b {
		t.Errorf("different seeds gave the same prompt")
	}
	if got := len(strings.Fields(p.build(1, 37))); got != 38 {
		t.Errorf("build(1, 37) has %d fields, want salt + 37 words", got)
	}
	if !strings.HasPrefix(p.build(9, 5), p.salt(9)+" ") {
		t.Errorf("prompt does not start with its salt")
	}
	if strings.HasPrefix((prompts{seed: 7, nonce: 43}).build(9, 5), p.salt(9)) {
		t.Errorf("salts of different runs collide")
	}
}

func TestCalibrate(t *testing.T) {
	p := prompts{seed: 1, nonce: 5}
	fake := func(ctx context.Context, text string) (int, error) { return vllmtest.CountTokens(text), nil }
	for _, target := range []int{1, 2, 5, 64, 256, 1000, 4096} {
		words, ok, err := calibrate(context.Background(), p, target, fake)
		if err != nil || !ok {
			t.Fatalf("target %d: %v %v", target, ok, err)
		}
		got := vllmtest.CountTokens(p.build(0, words))
		next := vllmtest.CountTokens(p.build(0, words+1))
		// Largest word count within the target, but never zero words.
		if words > 1 && got > target {
			t.Errorf("target %d: %d words = %d tokens, over target", target, words, got)
		}
		if got <= target && next <= target {
			t.Errorf("target %d: %d words = %d tokens, one more word still fits (%d)", target, words, got, next)
		}
	}
}

func TestCalibrateFallbackAndErrors(t *testing.T) {
	p := prompts{}
	unsupported := func(context.Context, string) (int, error) { return 0, fmt.Errorf("x: %w", vllm.ErrUnsupported) }
	words, ok, err := calibrate(context.Background(), p, 1024, unsupported)
	if err != nil || ok || words != 1024-saltOverheadEstimate {
		t.Errorf("unsupported: words %d ok %v err %v", words, ok, err)
	}
	if words, _, _ := calibrate(context.Background(), p, 4, unsupported); words != 1 {
		t.Errorf("tiny target estimate = %d words, want 1", words)
	}
	broken := func(context.Context, string) (int, error) { return 0, errors.New("HTTP 500") }
	if _, _, err := calibrate(context.Background(), p, 64, broken); err == nil {
		t.Errorf("tokenize failure not reported")
	}
	constant := func(context.Context, string) (int, error) { return 3, nil }
	if _, _, err := calibrate(context.Background(), p, 64, constant); err == nil {
		t.Errorf("non-growing token count not reported")
	}
}

func TestRunAgainstFakeServer(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	fake := vllmtest.New(vllmtest.Config{
		Model:          "fake-mtp",
		TTFT:           40 * ms,
		ChunkDelay:     10 * ms,
		TokensPerChunk: 2,
		FailEvery:      5,
	})
	srv := httptest.NewServer(fake)
	defer srv.Close()

	var progress strings.Builder
	r, err := Run(context.Background(), Options{
		URL: srv.URL, InputLen: 200, OutputLen: 21, Warmup: 1,
		Concurrency: []int{1, 3}, NumPrompts: 6, Seed: 3, Tag: "t",
	}, &progress)
	if err != nil {
		t.Fatal(err)
	}
	if r.Model != "fake-mtp" || !r.Calibrated || r.Tag != "t" || r.ID == "" {
		t.Errorf("result header: model %q calibrated %v tag %q id %q", r.Model, r.Calibrated, r.Tag, r.ID)
	}
	if len(r.Cells) != 2 {
		t.Fatalf("got %d cells", len(r.Cells))
	}
	// 1 warmup + 6 measured per cell = 14 requests; every 5th fails:
	// requests 5 and 10, both measured (the warmups are requests 1 and 8).
	totalFailed := r.Cells[0].Failed + r.Cells[1].Failed
	if totalFailed != 2 {
		t.Errorf("failed %d, want 2 (FailEvery=5 over 14 requests)", totalFailed)
	}
	for _, c := range r.Cells {
		if c.Completed+c.Failed != 6 || c.NumPrompts != 6 {
			t.Errorf("c=%d: completed %d + failed %d != 6", c.Concurrency, c.Completed, c.Failed)
		}
		if c.Failed > 0 && (len(c.Errors) != 1 || !strings.Contains(c.Errors[0], "injected failure")) {
			t.Errorf("c=%d: error samples %v", c.Concurrency, c.Errors)
		}
		// Output: 1 token, then 10 chunks of 2 tokens, 10 ms apart.
		if c.MeanOutputTokens != 21 || !near(c.TokensPerChunk, 21.0/11) {
			t.Errorf("c=%d: mean output %v tokens/chunk %v", c.Concurrency, c.MeanOutputTokens, c.TokensPerChunk)
		}
		if c.MeanPromptTokens > 200 || c.MeanPromptTokens < 190 {
			t.Errorf("c=%d: mean prompt tokens %v, want calibrated close to 200", c.Concurrency, c.MeanPromptTokens)
		}
		// Sleeps never undershoot; allow generous overshoot for slow CI.
		if c.TTFTP50 < 40 || c.TTFTP95 > 400 {
			t.Errorf("c=%d: TTFT p50 %v p95 %v ms, want ~40", c.Concurrency, c.TTFTP50, c.TTFTP95)
		}
		if c.ITLP50 < 4.5 || c.ITLP50 > 50 {
			t.Errorf("c=%d: ITL p50 %v ms, want ~5 per token", c.Concurrency, c.ITLP50)
		}
		if c.DecodeTokS > 205 || c.DecodeTokS < 20 {
			t.Errorf("c=%d: decode %v tok/s, want ~200", c.Concurrency, c.DecodeTokS)
		}
		if c.PrefillTokS > 200/0.040+1 || c.PrefillTokS <= 0 {
			t.Errorf("c=%d: prefill %v tok/s, want <= 5000", c.Concurrency, c.PrefillTokS)
		}
		wantOut := float64(21*c.Completed) / c.DurationS
		if !near(c.OutputTokS, wantOut) {
			t.Errorf("c=%d: output %v tok/s, want %v", c.Concurrency, c.OutputTokS, wantOut)
		}
	}
	// Concurrency 3 runs 6 requests in two waves, not six.
	if d := r.Cells[1].DurationS; d > 6*0.140 {
		t.Errorf("c=3 took %.3fs, requests were not concurrent", d)
	}

	prompts := fake.Prompts()
	if len(prompts) != 14 {
		t.Fatalf("server saw %d prompts, want 14", len(prompts))
	}
	salts := map[string]bool{}
	for _, pr := range prompts {
		salt := strings.Fields(pr)[0]
		if salts[salt] {
			t.Errorf("salt %q sent twice", salt)
		}
		salts[salt] = true
	}
	if !strings.Contains(progress.String(), "calibrated with /tokenize") {
		t.Errorf("progress lacks calibration line:\n%s", progress.String())
	}
}

func TestRunDefaultsAndEstimate(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	srv := httptest.NewServer(vllmtest.New(vllmtest.Config{NoTokenize: true, NoContinuousUsage: true}))
	defer srv.Close()
	r, err := Run(context.Background(), Options{URL: srv.URL + "/", OutputLen: 4, Concurrency: []int{2}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Model != "fake-model" || r.URL != srv.URL || r.Calibrated || r.Options.InputLen != DefaultInputLen {
		t.Errorf("defaults: model %q url %q calibrated %v input %d", r.Model, r.URL, r.Calibrated, r.Options.InputLen)
	}
	if c := r.Cells[0]; c.NumPrompts != 4 || c.Completed != 4 {
		t.Errorf("num prompts %d completed %d, want max(4, 2x2)=4", c.NumPrompts, c.Completed)
	}
}

func TestRunRejectsBadServer(t *testing.T) {
	srv := httptest.NewServer(vllmtest.New(vllmtest.Config{}))
	srv.Close() // nothing listens
	if _, err := Run(context.Background(), Options{URL: srv.URL}, nil); err == nil {
		t.Errorf("no error for unreachable server")
	}
	if _, err := Run(context.Background(), Options{URL: srv.URL, Concurrency: []int{0}}, nil); err == nil {
		t.Errorf("no error for concurrency 0")
	}
}

func TestCellPrompts(t *testing.T) {
	for _, tt := range []struct{ n, c, want int }{{0, 1, 4}, {0, 2, 4}, {0, 8, 16}, {10, 8, 10}} {
		if got := cellPrompts(Options{NumPrompts: tt.n}, tt.c); got != tt.want {
			t.Errorf("cellPrompts(n=%d, c=%d) = %d, want %d", tt.n, tt.c, got, tt.want)
		}
	}
}

func writeRun(t *testing.T, state, name, body string) {
	t.Helper()
	dir := filepath.Join(state, "runs", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "run.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultURL(t *testing.T) {
	tests := []struct {
		name string
		runs map[string]string
		want string
	}{
		{"no runs", nil, FallbackURL},
		{"one run on wildcard", map[string]string{"a": `{"name":"a","host":"0.0.0.0","port":18096}`}, "http://127.0.0.1:18096"},
		{"one run on address", map[string]string{"a": `{"name":"a","host":"10.0.0.5","port":8001}`}, "http://10.0.0.5:8001"},
		{"ipv6", map[string]string{"a": `{"name":"a","host":"::1","port":8001}`}, "http://[::1]:8001"},
		{"two runs", map[string]string{
			"a": `{"name":"a","host":"127.0.0.1","port":8001}`,
			"b": `{"name":"b","host":"127.0.0.1","port":8002}`,
		}, FallbackURL},
		{"broken record ignored", map[string]string{
			"a": `{"name":"a","host":"127.0.0.1","port":8001}`,
			"b": `{not json`,
		}, "http://127.0.0.1:8001"},
	}
	for _, tt := range tests {
		state := t.TempDir()
		for n, b := range tt.runs {
			writeRun(t, state, n, b)
		}
		if got := defaultURL(state); got != tt.want {
			t.Errorf("%s: %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestFingerprint(t *testing.T) {
	old := gpuList
	defer func() { gpuList = old }()
	gpuList = func(string) ([]gpu.GPU, error) {
		return []gpu.GPU{
			{HIPIndex: 0, Name: "AMD Radeon PRO W7800", GFX: "gfx1100"},
			{HIPIndex: 1, Name: "AMD Radeon PRO W7800", GFX: "gfx1100"},
			{HIPIndex: 2, Name: "AMD Radeon PRO V620", GFX: "gfx1030"},
		}, nil
	}
	state := t.TempDir()
	writeRun(t, state, "flash", `{"name":"flash","recipe":"flashnext-mtp2","host":"0.0.0.0","port":18096,"plan":{"model":"m","gpus":[2],"hip_devices":"2","env":{"HIP_VISIBLE_DEVICES":"2"}}}`)
	writeRun(t, state, "other", `{"name":"other","recipe":"x","host":"127.0.0.1","port":9000}`)

	fp := fingerprint(state, "http://localhost:18096")
	if fp["gpus"] != "1x AMD Radeon PRO V620 (gfx1030)" || fp["gpu_scope"] != "run" {
		t.Errorf("gpus = %q scope %q", fp["gpus"], fp["gpu_scope"])
	}
	if fp["run"] != "flash" || fp["recipe"] != "flashnext-mtp2" || len(fp["plan_hash"]) != 12 || fp["host"] == "" {
		t.Errorf("fingerprint = %v", fp)
	}
	// Plan hash ignores JSON formatting.
	state2 := t.TempDir()
	writeRun(t, state2, "flash", `{"name":"flash","host":"0.0.0.0","port":18096,"plan":{ "model": "m", "gpus": [2], "hip_devices": "2", "env": {"HIP_VISIBLE_DEVICES": "2"} }}`)
	if fp2 := fingerprint(state2, "http://127.0.0.1:18096"); fp2["plan_hash"] != fp["plan_hash"] {
		t.Errorf("plan hash depends on whitespace: %q vs %q", fp2["plan_hash"], fp["plan_hash"])
	}
	// Without a matching run the selection is unknown, so all host GPUs are
	// listed and the result says that they are a fallback.
	fp = fingerprint(state, "http://10.1.1.1:18096")
	if fp["run"] != "" || fp["gpu_scope"] != "all (run selection unknown)" {
		t.Errorf("remote URL matched local run: %v", fp)
	}
	if fp["gpus"] != "2x AMD Radeon PRO W7800 (gfx1100), 1x AMD Radeon PRO V620 (gfx1030)" {
		t.Errorf("fallback gpus = %q", fp["gpus"])
	}
}

func TestStore(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	if rs, err := List(); err != nil || len(rs) != 0 {
		t.Fatalf("empty List = %v, %v", rs, err)
	}
	t0 := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	newer := &Result{ID: "20261028T130000Z-bbbbbb", Time: t0.Add(time.Hour), Model: "m2", Cells: []Cell{{Concurrency: 8, Measurement: Measurement{OutputTokS: 99.5}}}}
	older := &Result{ID: "20261028T120000Z-aaaaaa", Time: t0, Model: "m1"}
	for _, r := range []*Result{newer, older} {
		path, err := Save(r)
		if err != nil || filepath.Base(path) != r.ID+".json" {
			t.Fatalf("Save = %q, %v", path, err)
		}
	}
	os.WriteFile(filepath.Join(Dir(), "junk.json"), []byte("{"), 0o644)
	rs, err := List()
	if err != nil || len(rs) != 2 || rs[0].ID != older.ID || rs[1].ID != newer.ID {
		t.Fatalf("List = %v, %v; want oldest first, junk skipped", rs, err)
	}
	if rs[1].Cells[0].OutputTokS != 99.5 {
		t.Errorf("round trip lost cells: %+v", rs[1])
	}
	for id, want := range map[string]string{
		newer.ID:           newer.ID,
		"20261028T12":      older.ID,
		"20261028T130000Z": newer.ID,
	} {
		r, err := Get(id)
		if err != nil || r.ID != want {
			t.Errorf("Get(%q) = %v, %v; want %s", id, r, err, want)
		}
	}
	for _, id := range []string{"2026", "nope", "../x", ""} {
		if _, err := Get(id); err == nil {
			t.Errorf("Get(%q): no error", id)
		}
	}
}

// A result of a run names the model files the run loads, with the hash of
// each, read from the hub cache of the plan.
func TestResultHoldsTheHashesOfTheModelFiles(t *testing.T) {
	hf := t.TempDir()
	repo := filepath.Join(hf, "hub", "models--org--m")
	snap := filepath.Join(repo, "snapshots", "c0ffee")
	for _, d := range []string{filepath.Join(repo, "refs"), filepath.Join(repo, "blobs"), snap} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	blob := strings.Repeat("ab", 32)
	_ = os.WriteFile(filepath.Join(repo, "refs", "main"), []byte("c0ffee"), 0o644)
	_ = os.WriteFile(filepath.Join(repo, "blobs", blob), []byte("w"), 0o644)
	if err := os.Symlink("../../blobs/"+blob, filepath.Join(snap, "model.safetensors")); err != nil {
		t.Fatal(err)
	}
	state := t.TempDir()
	writeRun(t, state, "m", `{"name":"m","host":"127.0.0.1","port":18097,"plan":{"model":"org/m","env":{"HF_HOME":"`+hf+`"},"args":["org/m","--port","18097"]}}`)
	w := modelWeights(state, "http://127.0.0.1:18097")
	if w == nil || w.Source != "org/m" || w.Commit != "c0ffee" || len(w.Files) != 1 || w.Files[0].SHA256 != blob {
		t.Fatalf("weights = %+v", w)
	}
	if modelWeights(state, "http://127.0.0.1:9999") != nil {
		t.Error("a URL of no run has weights")
	}
}
