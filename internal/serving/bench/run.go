package bench

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/opengfx1030/bernie/internal/serving/vllm"
	"github.com/opengfx1030/bernie/internal/support/paths"
)

// defaultSizeTolerance is the share of the prompt length a cell may miss
// when the run does not say otherwise.
const defaultSizeTolerance = 2.0

// sizeTolerance is the share of the prompt length a cell may miss.
func sizeTolerance(percent float64) float64 {
	if percent <= 0 {
		return defaultSizeTolerance
	}
	return percent
}

// runPool sends n requests with at most workers in flight and returns the
// samples in request order. from is the start of the cell: every sample
// keeps its times relative to it.
func runPool(ctx context.Context, workers, n int, from time.Time, send func(context.Context, time.Time) sample) []sample {
	out := make([]sample, n)
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range min(workers, n) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				out[i] = send(ctx, from)
			}
		}()
	}
	for i := range n {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return out
}

// newID is the UTC timestamp plus a short hash, e.g. 20261028T153012Z-1a2b3c.
func newID(t time.Time, o Options, nonce uint32) string {
	h := sha256.New()
	fmt.Fprintf(h, "%d|%s|%s|%d", t.UnixNano(), o.URL, o.Model, nonce)
	return t.Format("20060102T150405Z") + "-" + hex.EncodeToString(h.Sum(nil))[:6]
}

// Run benchmarks the server. Empty URL resolves with DefaultURL, empty Model
// to the first id of /v1/models. Progress lines go to progress (may be nil).
// Run returns an error only when the benchmark cannot start or ctx ends;
// failed requests are counted in the cells.
func Run(ctx context.Context, o Options, progress io.Writer) (*Result, error) {
	if progress == nil {
		progress = io.Discard
	}
	o, err := Normalize(o)
	if err != nil {
		return nil, err
	}
	if o.URL == "" {
		o.URL = DefaultURL()
	}
	client := vllm.New(o.URL)
	o.URL = client.BaseURL()
	if err := client.Health(ctx); err != nil && !errors.Is(err, vllm.ErrUnsupported) {
		return nil, fmt.Errorf("server %s not ready: %w", o.URL, err)
	}
	if o.Model == "" {
		ids, err := client.Models(ctx)
		if err != nil {
			return nil, fmt.Errorf("list models: %w", err)
		}
		if len(ids) == 0 {
			return nil, fmt.Errorf("server %s serves no models", o.URL)
		}
		o.Model = ids[0]
	}

	var nonce [4]byte
	_, _ = rand.Read(nonce[:])
	// A group of kind "check" uses the same pass key machinery as the
	// suite-wide recall gate. The verdict lands in cell.check, not
	// cell.recall, so the two stay distinguishable in one result.
	p := prompts{seed: o.Seed, nonce: binary.BigEndian.Uint32(nonce[:]), recall: o.Recall || o.Check}
	words, calibrated, err := calibrateCached(ctx, client, o.Model, p, o.InputLen)
	if err != nil {
		return nil, err
	}
	how := "calibrated with /tokenize"
	if !calibrated {
		how = "estimated, server has no /tokenize"
	}
	fmt.Fprintf(progress, "bench: %s model %s, prompt %d words for %d tokens (%s)\n", o.URL, o.Model, words, o.InputLen, how)

	now := time.Now().UTC()
	res := &Result{
		ID:          newID(now, o, p.nonce),
		Tag:         o.Tag,
		URL:         o.URL,
		Model:       o.Model,
		Time:        now,
		Options:     o,
		Fingerprint: fingerprint(paths.State(), o.URL),
		Weights:     modelWeights(paths.State(), o.URL),
		PromptWords: words,
		Calibrated:  calibrated,
		Workload: &Workload{
			ProtocolID:   ProtocolID,
			CorpusSHA256: corpusHash(),
			InputLen:     o.InputLen,
			OutputLen:    o.OutputLen,
			Concurrency:  o.Concurrency,
			Recall:       o.Recall,
			Check:        o.Check,
		},
	}

	maxC := 1
	for _, c := range o.Concurrency {
		maxC = max(maxC, c, o.Warmup)
	}
	hc := &http.Client{Transport: &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxIdleConns:        maxC,
		MaxIdleConnsPerHost: maxC,
		IdleConnTimeout:     90 * time.Second,
	}}
	defer hc.CloseIdleConnections()

	var seq atomic.Int64
	send := func(ctx context.Context, from time.Time) sample {
		n := int(seq.Add(1))
		prompt := p.build(n, words)
		s := streamCompletion(ctx, hc, o.URL, o.Model, prompt, o.OutputLen, o.Recall || o.Check, from)
		if o.Recall || o.Check {
			s.expect = p.passKey(n)
		}
		return s
	}
	// A probe request has its own salt like every other request, so the
	// prefix cache cannot share it with the measured requests.
	sendProbe := func(ctx context.Context, from time.Time) sample {
		return streamCompletion(ctx, hc, o.URL, o.Model, p.build(int(seq.Add(1)), words), ProbeTokens, false, from)
	}

	for i, c := range o.Concurrency {
		n := cellPrompts(o, c)
		warmFailed := 0
		if o.Warmup > 0 {
			fmt.Fprintf(progress, "bench: c=%d warmup %d request(s)\n", c, o.Warmup)
			warm := runPool(ctx, o.Warmup, o.Warmup, time.Now(), send)
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			for _, s := range warm {
				if s.err != nil {
					warmFailed++
					if warmFailed == 1 {
						fmt.Fprintf(progress, "bench: c=%d warmup failed: %v\n", c, s.err)
					}
				}
			}
		}
		var probe *Probe
		if o.Probe {
			_, ps := settle(ctx, client, o.Model, nil, 0)
			var wave []sample
			probe, wave = runProbe(ctx, c, sendProbe)
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			_, pa := settle(ctx, client, o.Model, ps, probe.Completed)
			probe.creditProbe(ps, pa, wave)
			if probe.Unavailable != "" {
				fmt.Fprintf(progress, "bench: c=%d prefill probe: %s\n", c, probe.Unavailable)
			} else {
				fmt.Fprintf(progress, "bench: c=%d prefill probe: %.0f tok/s over %d requests\n", c, probe.AggTokS, c)
			}
		}
		fmt.Fprintf(progress, "bench: c=%d running %d requests\n", c, n)
		before, bs := settle(ctx, client, o.Model, nil, 0)
		start := time.Now()
		samples := runPool(ctx, c, n, start, send)
		wall := time.Since(start)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cell := aggregate(c, samples, wall)
		after, as := settle(ctx, client, o.Model, bs, cell.Completed)
		cell.ServerSide = serverSide(before, after)
		cell.ServerBatch, cell.ServerBatchUnavailable = attribute(bs, as, samples)
		cell.Probe = probe
		cell.WarmupFailed = warmFailed
		cell.InputSizePercent = sizeDrift(float64(o.InputLen), cell.MeanPromptTokens)
		cell.SizeValid = math.Abs(cell.InputSizePercent) <= sizeTolerance(o.SizeTolerancePercent)
		cell.OutputValid = outputValid(samples)
		cell.StreamIncremental = streamIncremental(samples)
		fmt.Fprintf(progress, "bench: c=%d done in %.1fs: %d ok, %d failed, %.1f output tok/s\n",
			c, cell.DurationS, cell.Completed, cell.Failed, cell.OutputTokS)
		SendEvent(progress, Event{Kind: EventCell, Index: i + 1, Total: len(o.Concurrency), Level: c})
		if o.Recall || o.Check {
			asked, passed := recallVerdict(samples)
			if o.Check {
				cell.Check = &Check{Asked: asked, Passed: passed, Method: recallMethod(calibrated)}
				fmt.Fprintf(progress, "bench: c=%d check %d/%d passed (%s)\n", c, passed, asked, cell.Check.Method)
			} else {
				cell.Recall = &Recall{Asked: asked, Passed: passed, Depth: recallDepth, Method: recallMethod(calibrated)}
				fmt.Fprintf(progress, "bench: c=%d recall %d/%d passed (%s)\n", c, passed, asked, cell.Recall.Method)
			}
			if passed < asked {
				fmt.Fprintf(progress, "bench: c=%d the server did not repeat the hidden fact: it may not read the whole prompt, so the speed numbers of this cell are meaningless\n", c)
			}
		}
		res.Cells = append(res.Cells, cell)
	}
	return res, nil
}
