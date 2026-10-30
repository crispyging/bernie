package runner

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/opengfx1030/bernie/internal/serving/recipe"
)

// TestCheckPortRefusesABusyPort checks that a run whose port another
// process holds is refused with a message that says what to do.
func TestCheckPortRefusesABusyPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	p := &recipe.Plan{Name: "busy", Host: "127.0.0.1", Port: port}
	err = checkPort(context.Background(), p)
	if err == nil {
		t.Fatal("checkPort accepted a busy port")
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Errorf("the error does not say the port is busy: %v", err)
	}
	if !strings.Contains(err.Error(), "args.port=") {
		t.Errorf("the error does not name the flag that picks another port: %v", err)
	}
	if !strings.Contains(err.Error(), "bernie attach "+strconv.Itoa(port)) {
		t.Errorf("the error does not name the command that attaches the server: %v", err)
	}
}

// TestCheckPortAcceptsAFreePort checks that a free port passes.
func TestCheckPortAcceptsAFreePort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	p := &recipe.Plan{Name: "free", Host: "127.0.0.1", Port: port}
	if err := checkPort(context.Background(), p); err != nil {
		t.Errorf("checkPort refused a free port: %v", err)
	}
}
