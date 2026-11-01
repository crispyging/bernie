package mcpcmd_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/opengfx1030/bernie/internal/cli"
	_ "github.com/opengfx1030/bernie/internal/cli/cmds/health"
	_ "github.com/opengfx1030/bernie/internal/cli/cmds/mcpcmd"
	_ "github.com/opengfx1030/bernie/internal/cli/cmds/serving"
)

// The test binary doubles as `bernie mcp`: the stdio test starts it as a
// child process and talks to it over real pipes.
func TestMain(m *testing.M) {
	if os.Getenv("BERNIE_TEST_AS_MCP") == "1" {
		os.Exit(cli.Main([]string{"mcp"}))
	}
	os.Exit(m.Run())
}

// cliJSON runs a bernie command in this process and decodes its --json document.
func cliJSON(t *testing.T, args ...string) any {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	code := cli.Main(append(args, "--json"))
	os.Stdout = old
	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	if code != 0 {
		t.Fatalf("bernie %v: exit %d: %s", args, code, buf.String())
	}
	var v any
	if err := json.Unmarshal(buf.Bytes(), &v); err != nil {
		t.Fatalf("bernie %v: %v: %s", args, err, buf.String())
	}
	return v
}

func TestStdioSession(t *testing.T) {
	dir := t.TempDir()
	for _, k := range []string{"BERNIE_STATE_DIR", "BERNIE_CONFIG_DIR", "BERNIE_DATA_DIR", "BERNIE_CACHE_DIR"} {
		t.Setenv(k, filepath.Join(dir, k))
	}
	t.Setenv("BERNIE_VENV", "")
	t.Setenv("VIRTUAL_ENV", "")
	t.Setenv("BERNIE_RECIPES", "")

	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "BERNIE_TEST_AS_MCP=1")
	// The child's stderr is copied in while the test reads it.
	var stderr lockedBuffer
	cmd.Stderr = &stderr
	ctx := context.Background()
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "stdio-test", Version: "1"}, nil).Connect(ctx, &sdk.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("handshake: %v\nstderr: %s", err, stderr.String())
	}
	defer cs.Close()

	var tools []string
	for tool, err := range cs.Tools(ctx, nil) {
		if err != nil {
			t.Fatal(err)
		}
		tools = append(tools, tool.Name)
	}
	if len(tools) == 0 {
		t.Fatalf("tools/list is empty")
	}
	// The catalogue is sorted and holds the bench tools of spec §13.4.
	slices.Sort(tools)
	want := []string{"bench_compare", "bench_list", "bench_show", "bench_suites"}
	for _, w := range want {
		if !slices.Contains(tools, w) {
			t.Errorf("tools/list lacks %s: %v", w, tools)
		}
	}

	for _, c := range []struct {
		tool string
		args map[string]any
		cli  []string
	}{
		{"status", map[string]any{}, []string{"status"}},
		{"recipe_plan", map[string]any{"id": "27b-awq", "mode": "host"}, []string{"recipe", "plan", "27b-awq", "--mode", "host"}},
		{"recipe_list", map[string]any{"all": true}, []string{"recipe", "list", "--all"}},
	} {
		res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: c.tool, Arguments: c.args})
		if err != nil || res.IsError {
			t.Fatalf("%s: %v %+v", c.tool, err, res)
		}
		got, want := res.StructuredContent, cliJSON(t, c.cli...)
		if g := roundTrip(t, got); !reflect.DeepEqual(g, want) {
			t.Errorf("%s: structuredContent differs from `bernie %s --json`", c.tool, strings.Join(c.cli, " "))
		}
	}
	if stderr.Len() == 0 {
		t.Error("the server should log its start on stderr")
	}
}

// lockedBuffer is a bytes.Buffer that one goroutine writes while another
// reads.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func (l *lockedBuffer) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Len()
}

func roundTrip(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var g any
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestHelpListsMCP(t *testing.T) {
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	code := cli.Main([]string{"--help"})
	os.Stdout = old
	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	if code != 0 || !strings.Contains(buf.String(), "  mcp ") {
		t.Errorf("bernie --help (exit %d) does not list mcp:\n%s", code, buf.String())
	}
}
