package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
)

// Defaults of a run.
const (
	DefaultSuite     = "smoke"
	DefaultSeed      = 42
	DefaultContext   = 4096
	DefaultMaxTokens = 1024
	DefaultTimeout   = 120 * time.Second
)

// Options are the inputs of a run. Models are run one after the other on
// the same generated cases.
type Options struct {
	URL      string   `json:"url"`
	Kind     string   `json:"kind"` // openai or openwebui
	Models   []string `json:"models"`
	Suite    string   `json:"suite"`
	Seed     int64    `json:"seed"`
	Repeats  int      `json:"repeats"`
	Contexts []int    `json:"context_sizes"`
	// AutoContext picks the context sizes from the server's max_model_len
	// with Plan instead of taking Contexts.
	AutoContext bool     `json:"auto_context,omitempty"`
	Tracks      []string `json:"tracks,omitempty"`
	Temperature float64  `json:"temperature"`
	MaxTokens   int      `json:"max_tokens"`
	TimeoutS    int      `json:"timeout_s"`
	Tag         string   `json:"tag,omitempty"`
	APIKey      string   `json:"-"`
}

// Normalize fills the defaults and refuses values that cannot run.
func Normalize(o Options) (Options, error) {
	if o.Suite == "" {
		o.Suite = DefaultSuite
	}
	if o.Kind == "" {
		o.Kind = KindOpenAI
	}
	if o.Kind != KindOpenAI && o.Kind != KindOpenWebUI {
		return o, fmt.Errorf("kind must be %s or %s, not %q", KindOpenAI, KindOpenWebUI, o.Kind)
	}
	if o.Repeats == 0 {
		o.Repeats = 1
	}
	if o.AutoContext && len(o.Contexts) > 0 {
		return o, errors.New("--context auto picks the sizes itself; do not name sizes with it")
	}
	if len(o.Contexts) == 0 && !o.AutoContext {
		o.Contexts = []int{DefaultContext}
	}
	if o.MaxTokens == 0 {
		o.MaxTokens = DefaultMaxTokens
	}
	if o.TimeoutS == 0 {
		o.TimeoutS = int(DefaultTimeout / time.Second)
	}
	switch {
	case o.Repeats < 1 || o.Repeats > 10:
		return o, errors.New("repeats must be 1 to 10")
	case len(o.Contexts) > 8:
		return o, errors.New("at most 8 context sizes")
	case o.Temperature < 0 || o.Temperature > 2:
		return o, errors.New("temperature must be 0 to 2")
	case o.MaxTokens < 1 || o.MaxTokens > 32768:
		return o, errors.New("max tokens must be 1 to 32768")
	case o.TimeoutS < 1 || o.TimeoutS > 600:
		return o, errors.New("timeout must be 1 to 600 s")
	case len(o.Models) > 32:
		return o, errors.New("at most 32 models")
	}
	seen := map[int]bool{}
	for _, s := range o.Contexts {
		if s < 1 || s > 1_000_000 || seen[s] {
			return o, fmt.Errorf("context size %d is not a new size from 1 to 1000000", s)
		}
		seen[s] = true
	}
	return o, nil
}

// Case statuses.
const (
	StatusPass    = "pass"
	StatusFail    = "fail"
	StatusError   = "error"
	StatusSkipped = "skipped"
)

// CaseResult is the outcome of one case for one model and one repeat.
type CaseResult struct {
	Model            string         `json:"model"`
	CaseID           string         `json:"case_id"`
	Track            string         `json:"track"`
	Family           string         `json:"family"`
	Variant          int            `json:"variant"`
	Context          int            `json:"context_size"`
	Estimate         Estimate       `json:"context_estimate"`
	Repeat           int            `json:"repeat"`
	Status           string         `json:"status"`
	Critical         bool           `json:"critical"`
	Loop             bool           `json:"loop"`
	LatencyS         float64        `json:"latency_s"`
	PromptTokens     *int           `json:"prompt_tokens"`
	CompletionTokens *int           `json:"completion_tokens"`
	Error            string         `json:"error,omitempty"`
	Question         string         `json:"question"`
	Reply            string         `json:"reply"`
	Expected         Expected       `json:"expected"`
	Details          map[string]any `json:"details,omitempty"`
	Events           []Event        `json:"events,omitempty"`
	// Admission is how the case was fitted to the server. It is absent
	// when the server has neither a tokenizer nor a reported limit.
	Admission *Admission `json:"context_admission,omitempty"`
	// Calls is the telemetry of every chat request of the case.
	Calls []Call `json:"calls,omitempty"`
	// Diagnosis explains the outcome from the recorded evidence.
	Diagnosis *Diagnosis `json:"diagnosis,omitempty"`
}

// Run asks every model every case, one request at a time, and grades the
// replies. A case whose prompt does not fit the server's context is
// skipped with the reason, never sent. Progress lines go to progress. Run
// stops when ctx ends; the cases done so far stay in the result.
func Run(ctx context.Context, o Options, progress io.Writer) (*Result, error) {
	if progress == nil {
		progress = io.Discard
	}
	o, err := Normalize(o)
	if err != nil {
		return nil, err
	}
	cl := newClient(o)
	served, err := cl.models(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the models of %s: %w", cl.base, err)
	}
	if len(o.Models) == 0 {
		if len(served) == 0 {
			return nil, fmt.Errorf("%s serves no models", cl.base)
		}
		o.Models = []string{served[0].ID}
	}
	limits := map[string]int{}
	for _, m := range served {
		limits[m.ID] = m.MaxModelLen
	}
	for _, m := range o.Models {
		if !slices.ContainsFunc(served, func(s servedModel) bool { return s.ID == m }) {
			return nil, fmt.Errorf("%s does not serve model %q", cl.base, m)
		}
	}
	var plan *ContextPlan
	if o.AutoContext {
		// The bands of every model are the same, so the smallest limit
		// decides them.
		limit := 0
		for _, m := range o.Models {
			if l := limits[m]; l > 0 && (limit == 0 || l < limit) {
				limit = l
			}
		}
		if plan, err = Plan(o.Suite, limit, o.MaxTokens); err != nil {
			return nil, err
		}
		o.Contexts = plan.Contexts
	}
	cases, err := Generate(o.Suite, o.Seed, o.Contexts, o.Tracks)
	if err != nil {
		return nil, err
	}
	if len(cases) == 0 {
		return nil, errors.New("no case selected")
	}
	res := newResult(o, cl.base)
	res.ContextPlan = plan
	fmt.Fprintf(progress, "eval: %s, suite %s (%d cases), %d model(s), %d repeat(s), contexts %s, seed %d\n",
		cl.base, o.Suite, len(cases), len(o.Models), o.Repeats, intList(o.Contexts), o.Seed)
	total := len(cases) * o.Repeats * len(o.Models)
	done := 0
	for _, model := range o.Models {
		if lim := limits[model]; lim > 0 {
			fmt.Fprintf(progress, "eval: %s: max-model-len %d\n", model, lim)
		}
		for rep := 0; rep < o.Repeats; rep++ {
			for i := range cases {
				if ctx.Err() != nil {
					res.Status = "cancelled"
					res.finish()
					return res, nil
				}
				done++
				r := runCase(ctx, cl, o, model, rep, &cases[i], limits[model])
				res.Cases = append(res.Cases, r)
				fmt.Fprintf(progress, "eval: [%d/%d] %s\n", done, total, logLine(&r))
			}
		}
	}
	res.Status = "done"
	res.finish()
	return res, nil
}

func runCase(ctx context.Context, cl *client, o Options, model string, rep int, c *Case, limit int) CaseResult {
	r := CaseResult{Model: model, CaseID: c.ID, Track: c.Track, Family: c.Family, Variant: c.Variant,
		Context: c.Context, Estimate: c.Estimate, Repeat: rep, Question: c.Question, Expected: c.Expected}
	// A case whose evidence does not fit is skipped with the reason,
	// never sent: a refusal of the server would grade the context, not
	// the model.
	sent, adm, err := admit(ctx, cl.tokenize(model, o.Kind), c, o.MaxTokens, limit)
	r.Admission = adm
	if err != nil {
		r.Status, r.Error = StatusSkipped, err.Error()
		return r
	}
	c = sent
	start := time.Now()
	world := c.World
	if world != nil {
		world = world.clone()
	}
	reply, events, calls, err := converse(ctx, cl, o, model, c, world)
	r.LatencyS = time.Since(start).Seconds()
	r.Reply, r.Events, r.Calls = reply, events, calls
	r.PromptTokens, r.CompletionTokens = usageOf(calls)
	g := Evaluate(c, reply, events, world)
	r.Critical, r.Loop, r.Details = g.Critical, g.Loop, g.Details
	switch {
	case err == nil:
		r.Status = map[bool]string{true: StatusPass, false: StatusFail}[g.Passed]
	case errors.Is(err, errStepLimit):
		r.Status, r.Loop, r.Error = StatusFail, true, err.Error()
	case errors.Is(err, errModelOutput):
		r.Status, r.Error = StatusFail, err.Error()
	default:
		r.Status, r.Error = StatusError, err.Error()
	}
	return r
}

var (
	errStepLimit   = errors.New("step limit reached")
	errModelOutput = errors.New("invalid model output")
)

// usageOf sums the usage of the calls of a case. It is nil when a call
// came without usage.
func usageOf(calls []Call) (*int, *int) {
	prompt, completion := 0, 0
	for _, c := range calls {
		if c.PromptTokens == nil || c.CompletionTokens == nil {
			return nil, nil
		}
		prompt += *c.PromptTokens
		completion += *c.CompletionTokens
	}
	if len(calls) == 0 {
		return nil, nil
	}
	return &prompt, &completion
}

// converse sends the case and, for an agent case, runs the tool calls of
// the model against the world until it answers without one.
func converse(ctx context.Context, cl *client, o Options, model string, c *Case, w *World) (string, []Event, []Call, error) {
	var u []Call
	var events []Event
	msgs := make([]any, 0, len(c.Messages)+8)
	for _, m := range c.Messages {
		msgs = append(msgs, m)
	}
	for steps := 0; ; steps++ {
		before := cl.counters(ctx, model, nil, 2*time.Second)
		msg, call, err := cl.chat(ctx, model, msgs, c.Tools(), o)
		if err == nil && before != nil {
			call.credit(before, cl.counters(ctx, model, before, 5*time.Second))
		}
		u = append(u, call)
		if err != nil {
			return "", events, u, err
		}
		if len(msg.ToolCalls) == 0 {
			return msg.Content, events, u, nil
		}
		if len(msg.ToolCalls) > 16 {
			return msg.Content, events, u, fmt.Errorf("%w: %d tool calls in one turn", errModelOutput, len(msg.ToolCalls))
		}
		if steps+1 > c.MaxSteps {
			return msg.Content, events, u, errStepLimit
		}
		msgs = append(msgs, msg)
		for _, tc := range msg.ToolCalls {
			if len(events) >= c.MaxSteps {
				return msg.Content, events, u, fmt.Errorf("%w: tool action limit reached", errModelOutput)
			}
			var args map[string]any
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil || args == nil {
				return msg.Content, events, u, fmt.Errorf("%w: tool arguments are not a JSON object", errModelOutput)
			}
			res := Execute(w, tc.Function.Name, args)
			events = append(events, Event{Operation: tc.Function.Name, Args: stringArgs(args), Result: res})
			body, _ := json.Marshal(res)
			msgs = append(msgs, map[string]string{"role": "tool", "tool_call_id": tc.ID, "content": string(body)})
		}
	}
}

func stringArgs(args map[string]any) map[string]string {
	out := map[string]string{}
	for k, v := range args {
		if s, ok := v.(string); ok {
			out[k] = s
		} else {
			out[k] = compactJSON(v)
		}
	}
	return out
}

// logLine is the progress line of one case: what was asked, the verdict,
// and why a case did not pass.
func logLine(r *CaseResult) string {
	head := fmt.Sprintf("%s %s ctx %d rep %d: %s", r.Model, r.CaseID, r.Context, r.Repeat, r.Status)
	if r.Status != StatusSkipped {
		head += fmt.Sprintf(" (%.1f s)", r.LatencyS)
	}
	switch {
	case r.Error != "":
		return head + ": " + r.Error
	case r.Status == StatusFail && r.Critical:
		return head + ": touched a protected asset"
	case r.Status == StatusFail && r.Loop:
		return head + ": the tool calls looped"
	case r.Status == StatusFail && r.Expected.Answer != nil:
		return head + fmt.Sprintf(": expected %q, got %q", *r.Expected.Answer, clip(strings.TrimSpace(r.Reply), 80))
	case r.Status == StatusFail:
		return head + fmt.Sprintf(": expected %s, got %q", compactJSON(expectedValue(r.Expected)), clip(strings.TrimSpace(r.Reply), 80))
	}
	return head
}

func expectedValue(e Expected) any {
	if e.JSON != nil {
		return e.JSON
	}
	return e.World
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func intList(xs []int) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = fmt.Sprint(x)
	}
	return strings.Join(parts, ",")
}
