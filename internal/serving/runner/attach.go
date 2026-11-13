package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// AttachError is the reason an attach failed in words. The CLI and the
// dashboard surface it as the body of a 4xx answer.
type AttachError struct {
	Reason string
	Err    error
}

func (e *AttachError) Error() string {
	if e.Err != nil {
		return e.Reason + ": " + e.Err.Error()
	}
	return e.Reason
}
func (e *AttachError) Unwrap() error { return e.Err }

// AttachOptions is what `bernie attach` and `POST /api/attach` accept.
// Name is the run name; URL is the http(s) base URL of the vLLM server
// (a port number is shorthand for http://127.0.0.1:PORT; the host may be
// another machine, spec §8.4). Recipe is the
// recipe id to attach under; "" means the attached server does not
// belong to a recipe.
type AttachOptions struct {
	Name   string
	URL    string
	Recipe string
}

// Attach probes an already running vLLM server and registers it as a
// run. The server is never started or stopped by bernie: the record
// carries a Foreign flag and Stop only removes the record (R29).
//
// The probe checks /health, /metrics and /v1/models in order. Each one
// that fails is reported; the run is registered only when all three pass.
func Attach(ctx context.Context, o AttachOptions) (*Record, error) {
	if o.Name == "" {
		return nil, &AttachError{Reason: "name is required (--name NAME)"}
	}
	if prev, err := readRecord(o.Name); err == nil {
		if st := state(ctx, prev); st != StateExited {
			return nil, &AttachError{Reason: fmt.Sprintf("run %s is %s; stop it first or use --name", o.Name, st)}
		}
	}
	base, err := normalizeAttachURL(o.URL)
	if err != nil {
		return nil, &AttachError{Reason: "url is not a server address", Err: err}
	}
	if _, err := attachProbe(ctx, base, "/health"); err != nil {
		return nil, &AttachError{Reason: "/health did not answer", Err: err}
	}
	if _, err := attachProbe(ctx, base, "/metrics"); err != nil {
		return nil, &AttachError{Reason: "/metrics did not answer", Err: err}
	}
	models, err := attachProbe(ctx, base, "/v1/models")
	if err != nil {
		return nil, &AttachError{Reason: "/v1/models did not answer", Err: err}
	}
	port := 0
	if p, _ := urlPort(base); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}
	rec := &Record{
		Name: o.Name, Recipe: o.Recipe, Host: attachHost(base),
		Port:      port,
		StartedAt: time.Now().UTC().Truncate(time.Second),
		Foreign:   true,
		Scheme:    attachScheme(base),
		Model:     firstModel(models),
	}
	if err := writeRecord(rec); err != nil {
		return nil, &AttachError{Reason: "save the record", Err: err}
	}
	return rec, nil
}

// Forget removes the record of a run (foreign or local) without
// touching any process. Stop for foreign runs calls Forget under the
// hood (R29).
func Forget(name string) error {
	path := recordPath(name)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("no run named %s", name)
	} else if err != nil {
		return err
	}
	return os.Remove(path)
}

func normalizeAttachURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("empty url")
	}
	if !strings.Contains(raw, "://") {
		if !strings.ContainsAny(raw, "/?#") {
			var n int
			if _, err := fmt.Sscanf(raw, "%d", &n); err == nil {
				return "http://127.0.0.1:" + raw, nil
			}
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("url %q is not http(s)", raw)
	}
	if u.Hostname() == "" {
		return "", fmt.Errorf("url %q names no host", raw)
	}
	if u.Port() == "" {
		if u.Scheme == "https" {
			u.Host = u.Host + ":443"
		} else {
			u.Host = u.Host + ":80"
		}
	}
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

// attachProbe asks the server for path and returns the start of the body.
func attachProbe(ctx context.Context, base, path string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%s: %s", path, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<10))
}

// firstModel is the first model id of a /v1/models answer, or "".
func firstModel(body []byte) string {
	var doc struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &doc) != nil || len(doc.Data) == 0 {
		return ""
	}
	return doc.Data[0].ID
}

// attachScheme is the scheme of an attached server, empty for http (the
// default of a record).
func attachScheme(base string) string {
	if u, err := url.Parse(base); err == nil && u.Scheme == "https" {
		return "https"
	}
	return ""
}

func attachHost(base string) string {
	u, err := url.Parse(base)
	if err != nil {
		return "127.0.0.1"
	}
	return u.Hostname()
}

func urlPort(base string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	return u.Port(), nil
}
