package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
)

// fakeVLLM is a stand-in for a running vLLM server: it answers the three
// endpoints an attach checks. A path in missing answers 404.
func fakeVLLM(t *testing.T, missing string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == missing {
			http.NotFound(w, r)
			return
		}
		switch r.URL.Path {
		case "/health":
			w.Write([]byte("{}"))
		case "/metrics":
			w.Write([]byte("vllm:num_requests_running 0\n"))
		case "/v1/models":
			w.Write([]byte(`{"data":[{"id":"m"}]}`))
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestAttachRegistersAForeignRun checks the attach of a running server:
// the three endpoints are probed, the record is written and the run is
// marked foreign (R29).
func TestAttachRegistersAForeignRun(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	srv := fakeVLLM(t, "")
	rec, err := Attach(context.Background(), AttachOptions{Name: "ext", URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if !rec.Foreign {
		t.Error("the run is not marked foreign")
	}
	if rec.Name != "ext" {
		t.Errorf("the name is %q", rec.Name)
	}
	if rec.Port == 0 {
		t.Error("the port is empty")
	}
	if rec.Model != "m" {
		t.Errorf("the model is %q, want the first id of /v1/models", rec.Model)
	}
	got, err := Get("ext")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Foreign {
		t.Error("the saved record is not foreign")
	}
}

// TestAttachRefusesAServerThatFailsACheck checks that the attach is
// refused, with the reason, when one of the three endpoints fails.
func TestAttachRefusesAServerThatFailsACheck(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	for _, missing := range []string{"/health", "/metrics", "/v1/models"} {
		srv := fakeVLLM(t, missing)
		_, err := Attach(context.Background(), AttachOptions{Name: "ext", URL: srv.URL})
		if err == nil {
			t.Errorf("the attach of a server without %s was accepted", missing)
			continue
		}
		if _, statErr := os.Stat(recordPath("ext")); statErr == nil {
			t.Errorf("the run was saved although %s failed", missing)
		}
	}
}

// TestStopForgetsAForeignRun checks that stopping an attached server does
// not kill it: the record goes away and the server keeps answering (R29).
func TestStopForgetsAForeignRun(t *testing.T) {
	t.Setenv("BERNIE_STATE_DIR", t.TempDir())
	srv := fakeVLLM(t, "")
	if _, err := Attach(context.Background(), AttachOptions{Name: "ext", URL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	if err := Stop(context.Background(), "ext"); err != nil {
		t.Fatal(err)
	}
	if _, err := Get("ext"); err == nil {
		t.Error("the record of the attached run is still there")
	}
	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("the attached server was stopped: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("the attached server answers %d", resp.StatusCode)
	}
}

// TestNormalizeAttachURL checks the accepted forms: a URL, a port number
// and the rejected ones.
func TestNormalizeAttachURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"8000", "http://127.0.0.1:8000"},
		{"http://127.0.0.1:8000", "http://127.0.0.1:8000"},
		{"http://localhost:8000/v1", "http://localhost:8000"},
		{"http://chardis:8000/v1", "http://chardis:8000"},
		{"http://10.0.0.1:8000", "http://10.0.0.1:8000"},
		{"https://chardis.lan", "https://chardis.lan:443"},
	}
	for _, c := range cases {
		got, err := normalizeAttachURL(c.in)
		if err != nil {
			t.Errorf("normalizeAttachURL(%q) = %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("normalizeAttachURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	for _, bad := range []string{"", "http://:8000", "ftp://127.0.0.1"} {
		if _, err := normalizeAttachURL(bad); err == nil {
			t.Errorf("normalizeAttachURL(%q) was accepted", bad)
		}
	}
	if p, err := urlPort("http://127.0.0.1:" + strconv.Itoa(8000)); err != nil || p != "8000" {
		t.Errorf("urlPort = %q, %v", p, err)
	}
}

// TestAttachedServerURL checks the address of an attached server on
// another host: its name and scheme are kept, so status, chat and the
// metrics reach the server where it runs.
func TestAttachedServerURL(t *testing.T) {
	cases := []struct {
		base, want string
	}{
		{"http://chardis:8000", "http://chardis:8000"},
		{"https://chardis.lan:443", "https://chardis.lan:443"},
		{"http://127.0.0.1:8000", "http://127.0.0.1:8000"},
	}
	for _, c := range cases {
		port, _ := urlPort(c.base)
		n, _ := strconv.Atoi(port)
		rec := Record{Host: attachHost(c.base), Port: n, Scheme: attachScheme(c.base), Foreign: true}
		if got := rec.URL(); got != c.want {
			t.Errorf("%s: URL() = %q, want %q", c.base, got, c.want)
		}
	}
}
