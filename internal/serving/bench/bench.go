// Package bench benchmarks an OpenAI-compatible server (spec §10): streaming
// /v1/completions requests with calibrated prompt lengths, one cell per
// concurrency level, and a result store under <state>/bench.
package bench

import (
	"fmt"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/vllm"
	"github.com/opengfx1030/bernie/internal/serving/weights"
)

// Defaults of spec §10.
const (
	DefaultInputLen  = 1024
	DefaultOutputLen = 512
	DefaultWarmup    = 1
	FallbackURL      = "http://127.0.0.1:8000"
	// ProtocolID names the shape of a benchmark: its prompt, its request
	// and its metrics. It changes when the measurement changes.
	ProtocolID = "bernie-bench-2"
)

// Options are the bench flags. Zero values take the spec defaults in Run,
// except Warmup, which is used as given (the CLI default is DefaultWarmup).
type Options struct {
	URL         string `json:"url"`
	Model       string `json:"model"`
	Tag         string `json:"tag,omitempty"`
	InputLen    int    `json:"input_len"`
	OutputLen   int    `json:"output_len"`
	Warmup      int    `json:"warmup"`
	Concurrency []int  `json:"concurrency"`
	// NumPrompts is the measured requests per cell; 0 means
	// max(4, 2 × concurrency) for each cell.
	NumPrompts int   `json:"num_prompts"`
	Seed       int64 `json:"seed"`
	// Recall turns on the pass key check: every prompt hides a short
	// random fact and the reply must contain it. The verdict lands in
	// cell.recall.
	Recall bool `json:"recall"`
	// Check turns on a declared-answer check: the same pass key
	// machinery, but the verdict lands in cell.check so a group whose
	// assertion is "check" stays distinguishable from the suite-wide
	// recall gate.
	Check bool `json:"check,omitempty"`
	// Suite is the name of the suite that shaped this run. Empty for a
	// plain run.
	Suite string `json:"suite,omitempty"`
	// Reps is the number of repetitions per cell of a suite run.
	Reps int `json:"reps,omitempty"`
	// SizeTolerancePercent is the share of the prompt length a cell may
	// miss. Zero takes the default of 2 %.
	SizeTolerancePercent float64 `json:"size_tolerance_percent,omitempty"`
	// Probe sends the prefill probe before every cell: as many
	// simultaneous max_tokens=1 requests as the cell has in flight.
	Probe bool `json:"probe,omitempty"`
}

// Measurement holds the numbers of a cell or of one repetition of it.
type Measurement struct {
	// Prefill is the prefill speed of the cell: the rate of the whole cell
	// and the distribution of the rates of its requests.
	Prefill *Rate `json:"prefill,omitempty"`
	// Decode is the decode speed of the cell, in the same shape.
	Decode *Rate `json:"decode,omitempty"`
	// TTFTs is the distribution of the time to first token in seconds.
	TTFTs *Dist `json:"ttft_s,omitempty"`
	// ITLms is the distribution of the inter-token latency in
	// milliseconds.
	ITLms *Dist `json:"itl_ms,omitempty"`
	// ServerSide is what the server measured over the same requests. It is
	// nil when the server has no metrics.
	ServerSide *ServerSide `json:"server_side,omitempty"`
	// Probe is the prefill probe of the cell. It is nil when the run sent
	// none.
	Probe *Probe `json:"probe,omitempty"`
	// ServerBatch is what the server's counters say about the measured
	// requests: their prefill and decode rates per request. When the
	// counters cannot be credited to them, ServerBatchUnavailable says why.
	ServerBatch            *vllm.Attribution `json:"server_batch,omitempty"`
	ServerBatchUnavailable string            `json:"server_batch_unavailable,omitempty"`
	// TokensExact tells whether the token counts of the cell come from the
	// server's usage. False when a request came without usage.
	TokensExact bool    `json:"tokens_exact"`
	TTFTP50     float64 `json:"ttft_p50_ms"`
	TTFTP95     float64 `json:"ttft_p95_ms"`
	ITLP50      float64 `json:"itl_p50_ms"`
	ITLP95      float64 `json:"itl_p95_ms"`
	// DecodeTokS is the mean per-request decode rate.
	DecodeTokS float64 `json:"decode_tok_s"`
	// PrefillTokS is the mean per-request prompt tokens / TTFT.
	PrefillTokS float64 `json:"prefill_tok_s"`
	// OutputTokS is all completion tokens of the cell / cell wall time.
	OutputTokS float64 `json:"output_tok_s"`
	DurationS  float64 `json:"duration_s"`
	// MeanPromptTokens and MeanOutputTokens come from the server's usage;
	// they show how close the prompt calibration landed.
	MeanPromptTokens float64 `json:"mean_prompt_tokens"`
	MeanOutputTokens float64 `json:"mean_output_tokens"`
	// MeanCachedPromptTokens is the mean number of prompt tokens the
	// server had in its prefix cache. Zero when the server does not say.
	MeanCachedPromptTokens float64 `json:"mean_cached_prompt_tokens,omitempty"`
	// TokensPerChunk is completion tokens per streamed chunk; above 1 with
	// speculative decoding (MTP).
	TokensPerChunk float64 `json:"tokens_per_chunk"`
	// Recall is the suite-wide pass key check. It is nil when the gate
	// did not run.
	Recall *Recall `json:"recall,omitempty"`
	// Check is the declared-answer check of the cell. It is separate
	// from recall so a group of kind "check" stays distinguishable in
	// one result.
	Check *Check `json:"check,omitempty"`
	// WarmupFailed counts the warm-up requests that failed.
	WarmupFailed int `json:"warmup_failed,omitempty"`
	// OutputValid tells whether the replies look like text. A reply that
	// repeats one token over and over is not a speed number.
	OutputValid bool `json:"output_valid"`
	// StreamIncremental tells whether the streams arrived in more than
	// one chunk. A server that buffers the whole answer has no decode
	// window.
	StreamIncremental bool     `json:"stream_incremental"`
	Errors            []string `json:"errors,omitempty"` // distinct error samples
}

// Cell is the result for one concurrency level. The flat latency fields are
// milliseconds; the new groups carry the same numbers in the units of spec
// §10: seconds for the time to first token, milliseconds for the
// inter-token latency.
type Cell struct {
	Concurrency int `json:"concurrency"`
	NumPrompts  int `json:"num_prompts"`
	Completed   int `json:"completed"`
	Failed      int `json:"failed"`
	// Measurement holds the numbers of the cell.
	Measurement
	// Group is the suite group of the cell. InputLen and OutputLen are
	// the lens of that group.
	Group     string `json:"group,omitempty"`
	InputLen  int    `json:"input_len,omitempty"`
	OutputLen int    `json:"output_len,omitempty"`
	// Kind is the assertion of the group: "throughput" (a speed) or
	// "check" (a declared answer). Empty for a plain run.
	Kind string `json:"kind,omitempty"`
	// Reps holds one entry per repetition, so a reader can recompute the
	// numbers of the cell.
	Reps []Rep `json:"reps,omitempty"`
	// Spread is the min/max ratio of the metrics over the repetitions.
	Spread map[string]float64 `json:"spread,omitempty"`
	// Incomplete marks a cell that lost a warm-up or a repetition. Its
	// numbers stay visible, but no verdict is issued for it.
	Incomplete bool `json:"incomplete,omitempty"`
	// Skipped says why a cell was never sent: its prompt and output do
	// not fit the context of the server (spec §10.2). It has no numbers.
	Skipped string `json:"skipped,omitempty"`
	// InputSizePercent is the share of the prompt length the server
	// measured off the requested length. SizeValid tells whether the
	// drift stays inside the tolerance of the run.
	InputSizePercent float64 `json:"input_size_percent,omitempty"`
	SizeValid        bool    `json:"size_valid"`
}

// Rep is one repetition of a cell: the numbers it measured, without the
// repetitions themselves.
type Rep struct {
	NumPrompts int `json:"num_prompts"`
	Completed  int `json:"completed"`
	Failed     int `json:"failed"`
	// Measurement holds the numbers of the repetition.
	Measurement
}

// Workload says what a benchmark asked the server to do. Two results with
// the same workload can be compared; results with different workloads
// cannot.
type Workload struct {
	// ProtocolID names the shape of the measurement.
	ProtocolID string `json:"protocol_id"`
	// CorpusSHA256 is the hash of the word list the prompts are built from.
	CorpusSHA256 string `json:"corpus_sha256"`
	// InputLen and OutputLen are the prompt and output token counts.
	// They are zero for a suite run: the lens of a suite run is per cell.
	InputLen  int `json:"input_len,omitempty"`
	OutputLen int `json:"output_len,omitempty"`
	// Concurrency are the levels of the cells.
	Concurrency []int `json:"concurrency"`
	// Recall tells whether the pass key check ran.
	Recall bool `json:"recall"`
	// Check tells whether the declared-answer check ran. A suite whose
	// groups are of kind "check" sets it.
	Check bool `json:"check,omitempty"`
	// SuiteID and SuiteDigest name the suite that shaped the workload.
	// Two results of different suites cannot be compared.
	SuiteID     string `json:"suite_id,omitempty"`
	SuiteDigest string `json:"suite_digest,omitempty"`
	// Kind is what the cells assert: "throughput" or "check".
	Kind string `json:"kind,omitempty"`
	// CompareBand and CompareSteady are the thresholds of the verdict
	// between two runs of this workload.
	CompareBand   float64 `json:"compare_band,omitempty"`
	CompareSteady float64 `json:"compare_steady,omitempty"`
}

// SuiteRef names the suite that produced a result.
type SuiteRef struct {
	ID      string `json:"id"`
	Digest  string `json:"digest"`
	Version int    `json:"version"`
}

// Result is one saved benchmark.
type Result struct {
	ID          string            `json:"id"`
	Tag         string            `json:"tag,omitempty"`
	URL         string            `json:"url"`
	Model       string            `json:"model"`
	Time        time.Time         `json:"time"`
	Options     Options           `json:"options"`
	Fingerprint map[string]string `json:"fingerprint"`
	// PromptWords is the corpus word count per prompt; Calibrated tells
	// whether /tokenize set it (else it is an estimate).
	PromptWords int  `json:"prompt_words"`
	Calibrated  bool `json:"calibrated"`
	// Workload is what the benchmark asked for. It is nil in results
	// saved before the field existed.
	Workload *Workload `json:"workload,omitempty"`
	// Suite names the suite that produced the result. It is nil for a
	// plain run.
	Suite *SuiteRef `json:"suite,omitempty"`
	// Weights are the model files the run loads, with their hashes. It is
	// nil for a server bernie did not start and in older results.
	Weights *weights.Model `json:"weights,omitempty"`
	Cells   []Cell         `json:"cells"`
	// Summary is the headline of the cells. Save and the readers fill it,
	// so it is never stale.
	Summary *Summary `json:"summary,omitempty"`
}

// Completed is the number of successful measured requests over all cells.
func (r *Result) Completed() int {
	n := 0
	for _, c := range r.Cells {
		n += c.Completed
	}
	return n
}

// Normalize fills the defaults that do not need the server: input and output
// length, concurrency [1], negative warmup to 0. It rejects invalid values.
func Normalize(o Options) (Options, error) {
	if o.InputLen == 0 {
		o.InputLen = DefaultInputLen
	}
	if o.OutputLen == 0 {
		o.OutputLen = DefaultOutputLen
	}
	if len(o.Concurrency) == 0 {
		o.Concurrency = []int{1}
	}
	if o.Warmup < 0 {
		o.Warmup = 0
	}
	switch {
	case o.InputLen < 1:
		return o, fmt.Errorf("input length must be positive, got %d", o.InputLen)
	case o.OutputLen < 1:
		return o, fmt.Errorf("output length must be positive, got %d", o.OutputLen)
	case o.NumPrompts < 0:
		return o, fmt.Errorf("num prompts must not be negative, got %d", o.NumPrompts)
	}
	for _, c := range o.Concurrency {
		if c < 1 {
			return o, fmt.Errorf("concurrency must be positive, got %d", c)
		}
	}
	return o, nil
}

// prompts per cell when Options.NumPrompts is 0.
func cellPrompts(o Options, concurrency int) int {
	if o.NumPrompts > 0 {
		return o.NumPrompts
	}
	return max(4, 2*concurrency)
}
