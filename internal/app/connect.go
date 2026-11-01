package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/opengfx1030/bernie/internal/onboarding"
	"github.com/opengfx1030/bernie/internal/serving/vllm"
)

// Connection is the `connect --json` document: the config an app needs to
// talk to a run, ready to paste (spec §3.2). It writes nothing.
type Connection struct {
	App     string `json:"app"`
	Run     string `json:"run"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	// Where says where the config goes, in one sentence.
	Where string `json:"where"`
	// Config is the text to paste.
	Config string `json:"config"`
}

// ConnectOptions are the inputs of Connect.
type ConnectOptions struct {
	App string // one of ConnectApps; "" = openai
	Run string // the run name; "" = the run that is serving
}

// connectApps renders the config of each app from the base URL and the
// model name.
var connectApps = map[string]func(base, model string) (where, config string){
	"openai":    openAIConfig,
	"opencode":  opencodeConfig,
	"openwebui": openWebUIConfig,
}

// ConnectApps are the app names connect knows, sorted.
func ConnectApps() []string {
	out := make([]string, 0, len(connectApps))
	for k := range connectApps {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Connect returns the config of an app for a run. The model name is the
// first id the server lists on /v1/models, so it is the name the server
// answers to.
func Connect(ctx context.Context, o ConnectOptions) (*Connection, error) {
	app := o.App
	if app == "" {
		app = "openai"
	}
	render, ok := connectApps[app]
	if !ok {
		return nil, fmt.Errorf("unknown app %q: use one of %s", app, strings.Join(ConnectApps(), ", "))
	}
	var run *onboarding.Run
	var err error
	if o.Run != "" {
		run, err = onboarding.RunByName(ctx, o.Run)
	} else {
		run, err = onboarding.LiveRun(ctx)
	}
	if err != nil {
		return nil, err
	}
	ids, err := vllm.New(run.URL).Models(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s does not answer /v1/models yet (%v); see: bernie ps", run.Name, err)
	}
	if len(ids) == 0 {
		return nil, errors.New(run.Name + " lists no model on /v1/models")
	}
	c := &Connection{App: app, Run: run.Name, BaseURL: run.URL + "/v1", Model: ids[0]}
	c.Where, c.Config = render(c.BaseURL, c.Model)
	return c, nil
}

func openAIConfig(base, model string) (string, string) {
	body, _ := json.Marshal(map[string]any{
		"model":    model,
		"messages": []map[string]string{{"role": "user", "content": "Hello!"}},
	})
	return "Any OpenAI-compatible client: set these variables, or paste the base URL and the model name into its settings. No API key is needed; a client that insists takes any value.",
		"export OPENAI_BASE_URL=" + base + "\n" +
			"export OPENAI_API_KEY=none\n" +
			"export OPENAI_MODEL=" + model + "\n\n" +
			"curl " + base + "/chat/completions \\\n" +
			"  -H 'Content-Type: application/json' \\\n" +
			"  -d '" + string(body) + "'\n"
}

func opencodeConfig(base, model string) (string, string) {
	doc := map[string]any{
		"$schema": "https://opencode.ai/config.json",
		"provider": map[string]any{
			"bernie": map[string]any{
				"npm":     "@ai-sdk/openai-compatible",
				"name":    "bernie (vLLM)",
				"options": map[string]any{"baseURL": base},
				"models":  map[string]any{model: map[string]any{"name": model}},
			},
		},
		"model": "bernie/" + model,
	}
	b, _ := json.MarshalIndent(doc, "", "  ")
	return "opencode: put this in opencode.json in your project, or merge it into ~/.config/opencode/opencode.json.",
		string(b) + "\n"
}

func openWebUIConfig(base, model string) (string, string) {
	return "Open WebUI: Admin Panel, Settings, Connections, OpenAI API: add the URL, any key, and pick " + model +
			". In a container, give it these variables instead; a container that does not use the host network reaches the host as host.docker.internal, not 127.0.0.1.",
		"OPENAI_API_BASE_URL=" + base + "\n" +
			"OPENAI_API_KEY=none\n"
}
