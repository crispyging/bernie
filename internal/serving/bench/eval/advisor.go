package eval

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Proposal is the run config a local model suggested for a goal, after
// bernie checked every field (spec §10.5). bernie never starts it: the
// reader runs Command.
type Proposal struct {
	OK        bool            `json:"ok"`
	Options   *Options        `json:"options,omitempty"`
	Command   string          `json:"command,omitempty"`
	Rationale string          `json:"rationale,omitempty"`
	Checks    []ProposalCheck `json:"checks"`
	LatencyS  float64         `json:"latency_s"`
	Response  string          `json:"response"`
	Error     string          `json:"error,omitempty"`
}

// ProposalCheck is one check of a proposal.
type ProposalCheck struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

// promptCap bounds the planned prompts of a proposal: sum of the context
// sizes × 4 bytes × the number of cases.
const promptCap = 128 << 20

// proposalKeys are the keys a proposal must have, no more and no less.
var proposalKeys = []string{"context_sizes", "max_tokens", "rationale", "repeats", "seed", "suite", "temperature", "timeout_s", "tracks"}

// Propose asks the model of a private or loopback server to suggest an
// eval run for goal, and checks the answer. It is the port of BroodBench's
// advisor.propose. o names the server, the kind, the model (the first one)
// and the key; the rest of o is ignored.
func Propose(ctx context.Context, o Options, goal string) *Proposal {
	start := time.Now()
	p := &Proposal{Checks: []ProposalCheck{}}
	fail := func(err error) *Proposal {
		p.Error = err.Error()
		p.Checks = append(p.Checks, ProposalCheck{Name: "proposal_validation", Detail: p.Error})
		p.LatencyS = time.Since(start).Seconds()
		return p
	}
	goal = strings.TrimSpace(goal)
	if goal == "" || len(goal) > 1000 {
		return fail(errors.New("the goal must be 1 to 1000 characters"))
	}
	if err := privateURL(ctx, o.URL); err != nil {
		return fail(err)
	}
	o.TimeoutS, o.MaxTokens, o.Temperature = 60, 1024, 0
	if o.Kind == "" {
		o.Kind = KindOpenAI
	}
	cl := newClient(o)
	served, err := cl.models(ctx)
	if err != nil {
		return fail(fmt.Errorf("list the models of %s: %w", cl.base, err))
	}
	model, limit := "", 0
	for _, m := range served {
		if len(o.Models) == 0 || m.ID == o.Models[0] {
			model, limit = m.ID, m.MaxModelLen
			break
		}
	}
	switch {
	case model == "":
		return fail(fmt.Errorf("%s does not serve the model", cl.base))
	case limit <= 0:
		return fail(fmt.Errorf("%s reports no max_model_len for %s, so a proposal cannot be checked against it", cl.base, model))
	}
	msgs := []any{
		Message{Role: "system", Content: "Return only one strict JSON object matching the requested keys. No markdown."},
		Message{Role: "user", Content: proposalPrompt(goal, limit)},
	}
	reply, _, err := cl.chat(ctx, model, msgs, nil, o)
	p.LatencyS = time.Since(start).Seconds()
	if err != nil {
		return fail(fmt.Errorf("the local model did not answer: %w", err))
	}
	p.Response = reply.Content
	run, rationale, err := parseProposal(reply.Content, limit)
	if err != nil {
		return fail(err)
	}
	run.URL, run.Kind, run.Models = o.URL, o.Kind, []string{model}
	p.OK, p.Options, p.Rationale, p.Command = true, run, rationale, proposalCommand(run)
	p.Checks = append(p.Checks,
		ProposalCheck{"local_model", true, "The proposal came from " + model + " on a private or loopback address."},
		ProposalCheck{"fixed_safety_caps", true, "One repeat, temperature 0, at most 4096 reply tokens and a 60 s request limit."},
		ProposalCheck{"proposal_schema", true, "Every key and value was checked."})
	return p
}

func proposalPrompt(goal string, limit int) string {
	ids := make([]string, len(Tracks))
	for i, t := range Tracks {
		ids[i] = t.ID
	}
	return "Suggest a safe bernie bench eval run. Goal: " + goal + "\n" +
		"Suites: " + strings.Join(SuiteIDs, ", ") + ". Valid tracks: " + strings.Join(ids, ", ") + ". " +
		"context_sizes must be 1 to 8 unique positive integers, and each context_size + max_tokens + 512 must fit the server context limit " +
		strconv.Itoa(limit) + ". Caps: max_tokens 1-4096, timeout_s 1-60, repeats exactly 1, temperature exactly 0, " +
		"nonempty unique tracks, signed 32-bit seed. The planned prompts, sum(context_sizes) * 4 * the number of cases, " +
		"must stay at most 134217728 bytes (128 MiB). Return strict JSON with exactly the keys " + strings.Join(proposalKeys, ",") + "."
}

// parseProposal checks the answer of the model: strict JSON, exactly the
// keys, and every value inside its cap.
func parseProposal(text string, limit int) (*Options, string, error) {
	v, err := strictJSON(text)
	doc, ok := v.(map[string]any)
	if err != nil || !ok {
		return nil, "", errors.New("the model must answer one strict JSON object (no prose, no code fence, no repeated key)")
	}
	keys := make([]string, 0, len(doc))
	for k := range doc {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	if !slices.Equal(keys, proposalKeys) {
		return nil, "", fmt.Errorf("the proposal must have exactly the keys %s, not %s", strings.Join(proposalKeys, ","), strings.Join(keys, ","))
	}
	integer := func(k string, lo, hi int) (int, error) {
		f, ok := doc[k].(float64)
		if !ok || f != math.Trunc(f) || f < float64(lo) || f > float64(hi) {
			return 0, fmt.Errorf("invalid %s: want an integer from %d to %d", k, lo, hi)
		}
		return int(f), nil
	}
	o := &Options{Repeats: 1}
	if o.MaxTokens, err = integer("max_tokens", 1, 4096); err != nil {
		return nil, "", err
	}
	if o.TimeoutS, err = integer("timeout_s", 1, 60); err != nil {
		return nil, "", err
	}
	if _, err = integer("repeats", 1, 1); err != nil {
		return nil, "", err
	}
	seed, err := integer("seed", math.MinInt32, math.MaxInt32)
	if err != nil {
		return nil, "", err
	}
	o.Seed = int64(seed)
	if t, ok := doc["temperature"].(float64); !ok || t != 0 {
		return nil, "", errors.New("invalid temperature: want exactly 0")
	}
	if s, ok := doc["suite"].(string); !ok || !slices.Contains(SuiteIDs, s) {
		return nil, "", errors.New("invalid suite: want " + strings.Join(SuiteIDs, ", "))
	} else {
		o.Suite = s
	}
	sizes, _ := doc["context_sizes"].([]any)
	if len(sizes) < 1 || len(sizes) > 8 {
		return nil, "", errors.New("invalid context_sizes: want 1 to 8 sizes")
	}
	for _, s := range sizes {
		f, ok := s.(float64)
		n := int(f)
		if !ok || f != math.Trunc(f) || n < 1 || n+o.MaxTokens+512 > limit || slices.Contains(o.Contexts, n) {
			return nil, "", fmt.Errorf("invalid context_sizes: each must be a new positive integer with size + max_tokens + 512 within %d", limit)
		}
		o.Contexts = append(o.Contexts, n)
	}
	tracks, _ := doc["tracks"].([]any)
	if len(tracks) == 0 {
		return nil, "", errors.New("invalid tracks: want at least one")
	}
	for _, t := range tracks {
		s, ok := t.(string)
		if !ok || !slices.ContainsFunc(Tracks, func(x Track) bool { return x.ID == s }) || slices.Contains(o.Tracks, s) {
			return nil, "", fmt.Errorf("invalid tracks: %v is not a new known track", t)
		}
		o.Tracks = append(o.Tracks, s)
	}
	rationale, _ := doc["rationale"].(string)
	if strings.TrimSpace(rationale) == "" || len(rationale) > 1000 {
		return nil, "", errors.New("invalid rationale: want 1 to 1000 characters")
	}
	cases, err := Generate(o.Suite, o.Seed, []int{128}, o.Tracks)
	if err != nil {
		return nil, "", err
	}
	total := 0
	for _, s := range o.Contexts {
		total += s
	}
	if total*4*len(cases) > promptCap {
		return nil, "", errors.New("the proposal plans more than 128 MiB of prompts")
	}
	return o, strings.TrimSpace(rationale), nil
}

// proposalCommand is the bench eval command line of a proposal.
func proposalCommand(o *Options) string {
	sizes := make([]string, len(o.Contexts))
	for i, s := range o.Contexts {
		sizes[i] = strconv.Itoa(s)
	}
	parts := []string{"bernie bench eval", "--url " + o.URL}
	if o.Kind != KindOpenAI {
		parts = append(parts, "--kind "+o.Kind)
	}
	parts = append(parts, "--model "+o.Models[0], "--suite "+o.Suite, "--context "+strings.Join(sizes, ","),
		"--tracks "+strings.Join(o.Tracks, ","), "--seed "+strconv.FormatInt(o.Seed, 10),
		"--max-tokens "+strconv.Itoa(o.MaxTokens), "--timeout "+strconv.Itoa(o.TimeoutS)+"s")
	return strings.Join(parts, " ")
}

// cgnat is the shared address space of RFC 6598, which Tailscale uses.
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// privateURL refuses a server whose name resolves to any address that is not
// private, loopback, link-local or shared: the prompt and the goal never
// leave the operator's networks.
func privateURL(ctx context.Context, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return fmt.Errorf("%q is not an http(s) URL", raw)
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	var addrs []netip.Addr
	if a, err := netip.ParseAddr(host); err == nil {
		addrs = []netip.Addr{a}
	} else {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil || len(ips) == 0 {
			return fmt.Errorf("%s does not resolve, so it cannot be proven private", host)
		}
		addrs = ips
	}
	for _, a := range addrs {
		a = a.Unmap()
		if !(a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || cgnat.Contains(a)) {
			return fmt.Errorf("the proposal server must resolve only to private or loopback addresses; %s is %s", host, a)
		}
	}
	return nil
}
