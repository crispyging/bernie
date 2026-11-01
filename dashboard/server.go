package dashboard

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/opengfx1030/bernie/dashboard/internal/api"
	"github.com/opengfx1030/bernie/dashboard/internal/auth"
	"github.com/opengfx1030/bernie/dashboard/internal/httpjson"
	"github.com/opengfx1030/bernie/dashboard/internal/telemetry"
	"github.com/opengfx1030/bernie/internal/mcp"
)

//go:embed web
var webFS embed.FS

// Options configures Serve.
type Options struct {
	// Listen is the TCP address, default "127.0.0.1:7860".
	Listen string
	// Token gates every request when non-empty. Serve generates one when the
	// address is not loopback and Token is empty.
	Token string
	// Log receives the start line and request errors. Nil discards.
	Log io.Writer
	// MCPWrite exposes the mutating MCP tools on /mcp (spec §13.5). It takes
	// effect only with a token. Serve also sets it when BERNIE_MCP_WRITE=1.
	MCPWrite bool
	// Version is the bernie version reported to MCP clients.
	Version string
	// Ready, when set, is called once the socket listens, with the URL to
	// open (including ?token= in token mode).
	Ready func(url string)
}

// DefaultListen is the loopback address of spec §11. It is the safe
// default: the dashboard is reachable from this host only.
const DefaultListen = "127.0.0.1:7860"

// HostListen is the address of the same port on every interface. It is
// used when the operator asks for the dashboard to be reachable from
// another machine (`bernie --host`). A dashboard on this address runs in
// token mode: every request needs the token.
const HostListen = "0.0.0.0:7860"

// ResolveListen returns the address the dashboard binds to. An explicit
// address wins; otherwise the host flag picks the network address and the
// default stays on loopback, where the port is closed to the network.
func ResolveListen(listen string, host bool) string {
	if listen != "" {
		return listen
	}
	if host {
		return HostListen
	}
	return DefaultListen
}

// cookieName holds the dashboard token after the operator visits /?token=….
const cookieName = auth.CookieName

// NewToken returns a random 128-bit hex token.
func NewToken() string { return auth.NewToken() }

// IsLoopback reports whether the listen address binds only to loopback.
func IsLoopback(listen string) bool { return auth.IsLoopback(listen) }

// Serve listens and serves until ctx is done, then shuts down gracefully.
func Serve(ctx context.Context, o Options) error {
	if o.Listen == "" {
		o.Listen = DefaultListen
	}
	if o.Log == nil {
		o.Log = io.Discard
	}
	if o.Token == "" && !IsLoopback(o.Listen) {
		o.Token = NewToken()
	}
	o.MCPWrite = o.MCPWrite || os.Getenv("BERNIE_MCP_WRITE") == "1"
	if o.MCPWrite && o.Token == "" {
		fmt.Fprintln(o.Log, "bernie dash: /mcp stays read-only: the write tools need a dashboard token")
	}
	ln, err := net.Listen("tcp", o.Listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w (another dashboard may be running: `bernie dash status`, stop with `bernie dash stop`)", o.Listen, err)
	}
	s := newServer(o)
	srv := &http.Server{
		Handler:           s.handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		// No WriteTimeout: SSE streams and benchmarks run for minutes.
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	url := "http://" + displayAddr(ln.Addr(), o.Listen) + "/"
	if o.Token != "" {
		url += "?token=" + o.Token
	}
	if o.Ready != nil {
		o.Ready(url)
	} else {
		fmt.Fprintf(o.Log, "bernie dash: listening on %s\n", url)
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = srv.Shutdown(shCtx)
	if errors.Is(err, context.DeadlineExceeded) {
		err = srv.Close() // open SSE streams; their contexts end with ctx anyway
	}
	if e := <-errc; e != nil && !errors.Is(e, http.ErrServerClosed) {
		return e
	}
	return err
}

// displayAddr keeps the host the operator typed (a name stays a name) and
// takes the real port from the listener (":0" picks one).
func displayAddr(a net.Addr, listen string) string {
	h, _, err := net.SplitHostPort(listen)
	_, p, perr := net.SplitHostPort(a.String())
	if err != nil || perr != nil {
		return a.String()
	}
	if h == "" || h == "0.0.0.0" || h == "::" {
		h = "localhost"
		if !IsLoopback(listen) {
			if name, err := os.Hostname(); err == nil && name != "" {
				h = name
			}
		}
	}
	return net.JoinHostPort(h, p)
}

type server struct {
	o       Options
	tele    *telemetry.Hub
	sampler *telemetry.Sampler
	mcp     http.Handler // nil unless the dashboard listens on loopback
}

func newServer(o Options) *server {
	s := &server{o: o, sampler: &telemetry.Sampler{}}
	if o.Listen == "" || IsLoopback(o.Listen) {
		s.mcp = mcp.New(mcp.Options{Write: o.MCPWrite && o.Token != "", Transport: "http", Version: o.Version, Log: o.Log}).Handler(o.Token)
	}
	s.tele = telemetry.NewHub(time.Second, s.sampler.Sample)
	s.sampler.SetHistory(s.tele.History())
	return s
}

// handler returns the full handler chain: guard (auth, CSRF) → routes, and /mcp.
func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	web, _ := fs.Sub(webFS, "web")
	files := newAssets(web)
	mux.Handle("GET /{$}", withCSP(files))
	mux.Handle("GET /css/{file}", withCSP(files))
	mux.Handle("GET /js/{file}", withCSP(files))
	mux.Handle("GET /components/{file}", withCSP(files))
	mux.Handle("GET /fonts/{file}", withCSP(files))
	mux.Handle("GET /views/{tab}/{file}", withCSP(files))

	api.Register(mux, api.Options{Log: s.o.Log, Telemetry: s.tele})
	// The guard of the token and of the X-Bernie header, then the guard that
	// refuses a write from another site.
	g := auth.New(s.o.Token, guardWrites(mux))
	// /mcp has its own guard (spec §13.10): the dashboard cookie and the
	// X-Bernie rule do not apply to it.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" {
			g.ServeHTTP(w, r)
		} else if s.mcp == nil {
			httpjson.Error(w, http.StatusNotFound, "/mcp is served on loopback addresses only")
		} else {
			s.mcp.ServeHTTP(w, r)
		}
	})
}

// withCSP adds the CSP to the embedded UI (the guard sets Cache-Control).
func withCSP(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; object-src 'none'; form-action 'self'")
		h.ServeHTTP(w, r)
	})
}

// Starter starts a dashboard in the background of a command: `up` starts it
// while the server loads the model, `serve --dash` starts it next to the run
// (spec §3.1). The dashboard is not listening until Start is called.
type Starter struct {
	o Options
	h *Handle
}

// NewStarter returns a starter for a dashboard with these options.
func NewStarter(o Options) *Starter { return &Starter{o: o} }

// Start starts the dashboard and returns when it listens. A bind error (the
// port is in use) comes back as an error; no dashboard runs then.
func (s *Starter) Start(ctx context.Context) error {
	h, err := Start(ctx, s.o)
	if err != nil {
		return err
	}
	s.h = h
	return nil
}

// Wait blocks until the dashboard stops (Ctrl-C) and returns its error. It
// returns nil when no dashboard runs.
func (s *Starter) Wait() error {
	if s.h == nil {
		return nil
	}
	return s.h.Wait()
}

// Handle is a dashboard that serves in the background.
type Handle struct {
	url  string
	done chan error
}

// Start serves the dashboard in the background and returns when it listens.
// The URL to open goes to Options.Ready (the caller prints it). A bind error
// comes back as an error and no dashboard runs then.
func Start(ctx context.Context, o Options) (*Handle, error) {
	h := &Handle{done: make(chan error, 1)}
	ready := make(chan string, 1)
	caller := o.Ready
	// The caller's Ready runs first: Start returns once it is done, so the
	// URL is printed before the caller goes on.
	o.Ready = func(url string) {
		if caller != nil {
			caller(url)
		}
		ready <- url
	}
	go func() { h.done <- Serve(ctx, o) }()
	select {
	case url := <-ready:
		h.url = url
		return h, nil
	case err := <-h.done:
		select {
		case url := <-ready: // the server stopped right after it listened
			h.url = url
			return h, nil
		default:
		}
		if err == nil {
			err = errors.New("the dashboard stopped before it was ready")
		}
		return nil, err
	}
}

// URL is the address to open (with the token in token mode).
func (h *Handle) URL() string { return h.url }

// Wait blocks until the dashboard stops and returns its error. A stop by
// Ctrl-C is not an error.
func (h *Handle) Wait() error { return <-h.done }
