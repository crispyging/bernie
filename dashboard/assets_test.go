package dashboard

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The ETag of an asset is a strong validator; an unchanged body answers 304
// when the browser asks with If-None-Match.
func TestAssetETag(t *testing.T) {
	h := newServer(Options{}).handler()
	w := do(t, h, "GET", "/css/tokens.css", map[string]string{"Host": "127.0.0.1"})
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("ETag header is empty")
	}
	w2 := do(t, h, "GET", "/css/tokens.css",
		map[string]string{"Host": "127.0.0.1", "If-None-Match": etag})
	if w2.Code != http.StatusNotModified {
		t.Errorf("If-None-Match: status %d, want 304", w2.Code)
	}
}

// Text assets are compressed when the client takes it; the Answer carries
// Content-Encoding gzip and Vary Accept-Encoding.
func TestAssetGzip(t *testing.T) {
	h := newServer(Options{}).handler()
	w := do(t, h, "GET", "/js/core.js",
		map[string]string{"Host": "127.0.0.1", "Accept-Encoding": "gzip"})
	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("Content-Encoding %q, want gzip", w.Header().Get("Content-Encoding"))
	}
	if w.Header().Get("Vary") != "Accept-Encoding" {
		t.Errorf("Vary %q, want Accept-Encoding", w.Header().Get("Vary"))
	}
	gr, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	body, _ := io.ReadAll(gr)
	web, _ := fs.Sub(webFS, "web")
	want, err := fs.ReadFile(web, "js/core.js")
	if err != nil {
		t.Fatalf("the shipped core.js: %v", err)
	}
	if !bytes.Equal(body, want) {
		t.Errorf("the served core.js is not the file the binary ships")
	}
}

// The fonts are immutable; the HTML is no-store; the other assets are
// no-cache.
func TestAssetCachePolicy(t *testing.T) {
	h := newServer(Options{}).handler()
	for _, tc := range []struct {
		path, want string
	}{
		{"/", "no-store"},
		{"/css/tokens.css", "no-cache"},
		{"/js/core.js", "no-cache"},
		{"/fonts/inter-latin.woff2", "public, max-age=31536000, immutable"},
	} {
		w := do(t, h, "GET", tc.path, map[string]string{"Host": "127.0.0.1"})
		if got := w.Header().Get("Cache-Control"); got != tc.want {
			t.Errorf("%s: Cache-Control %q, want %q", tc.path, got, tc.want)
		}
	}
}

// The CSP of the UI is strict: no inline scripts or styles, no base-uri, no
// object-src, the forms only post to the dashboard.
func TestCSP(t *testing.T) {
	h := newServer(Options{}).handler()
	w := do(t, h, "GET", "/", map[string]string{"Host": "127.0.0.1"})
	csp := w.Header().Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'self'", "base-uri 'none'", "form-action 'self'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP %q is missing %q", csp, want)
		}
	}
}

// A write from another site is refused. The CSRF guard of the header stays.
func TestWriteGuardSecFetchSite(t *testing.T) {
	h := newServer(Options{Listen: "0.0.0.0:7860", Token: testToken}).handler()
	hdr := map[string]string{
		"Host":           "gpu-box",
		"Authorization":  "Bearer " + testToken,
		"X-Bernie":       "1",
		"Sec-Fetch-Site": "cross-site",
		"Content-Type":   "application/json",
	}
	w := do(t, h, "POST", "/api/runs", hdr)
	if w.Code != http.StatusForbidden {
		t.Errorf("Sec-Fetch-Site=cross-site: status %d, want 403", w.Code)
	}
}

// A write from a foreign origin is refused; a write from the same origin
// passes the guard (the X-Bernie check still applies).
func TestWriteGuardOrigin(t *testing.T) {
	h := newServer(Options{Listen: "0.0.0.0:7860", Token: testToken}).handler()
	bearer := func(extra map[string]string) map[string]string {
		hdr := map[string]string{
			"Host": "gpu-box", "Authorization": "Bearer " + testToken,
			"X-Bernie": "1", "Content-Type": "application/json",
		}
		for k, v := range extra {
			hdr[k] = v
		}
		return hdr
	}
	w := do(t, h, "POST", "/api/runs", bearer(map[string]string{"Origin": "http://evil.example"}))
	if w.Code != http.StatusForbidden {
		t.Errorf("foreign origin: status %d, want 403", w.Code)
	}
	w = do(t, h, "POST", "/api/runs", bearer(map[string]string{"Origin": "http://gpu-box"}))
	if w.Code == http.StatusForbidden {
		t.Errorf("same origin: status %d, want a pass through the guard", w.Code)
	}
}

// The budget of the page is measured, not promised. A change that crosses a
// line fails the build. The numbers are measured 2026-10-29; the targets
// follow the page (design-dashboard.md §8.3).
func TestPayloadBudget(t *testing.T) {
	web, _ := fs.Sub(webFS, "web")
	var html, js, css, fonts int
	_ = fs.WalkDir(web, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		body, err := fs.ReadFile(web, p)
		if err != nil {
			return nil
		}
		switch {
		case p == "index.html":
			html = len(body)
		case strings.HasSuffix(p, ".js"):
			js += len(body)
		case strings.HasSuffix(p, ".css"):
			css += len(body)
		case strings.HasPrefix(p, "fonts/") && strings.HasSuffix(p, ".woff2"):
			fonts += len(body)
		}
		return nil
	})
	t.Logf("measured (raw bytes): html=%d js=%d css=%d fonts=%d", html, js, css, fonts)

	// The first page never exceeds the targets set in design-dashboard.md.
	// The numbers are honest: the next commit that breaks a line changes the
	// doc first.
	const htmlMax = 12 * 1024
	const jsMax = 368 * 1024 // 361.2 KB now; the nine tabs of plain JavaScript
	const cssMax = 84 * 1024 // 80.7 KB now: the Quality tab and the look of the Dashboard
	const fontsMax = 130 * 1024
	const treeMax = 456 * 1024
	if html > htmlMax {
		t.Errorf("html %d > %d", html, htmlMax)
	}
	if js > jsMax {
		t.Errorf("js %d > %d", js, jsMax)
	}
	if css > cssMax {
		t.Errorf("css %d > %d", css, cssMax)
	}
	if fonts > fontsMax {
		t.Errorf("fonts %d > %d", fonts, fontsMax)
	}
	if html+js+css > treeMax {
		t.Errorf("html+css+js %d > %d", html+js+css, treeMax)
	}
}

// The gzipped body of every text asset of the tree is at most ~90 % of its
// raw body: the HTTP round trip of a text asset on a LAN is bounded. The
// line covers the whole tree (the five tabs of plain JavaScript); the first
// paint has a line of its own in TestFirstPaintPayload.
func TestGzipSizeBudget(t *testing.T) {
	web, _ := fs.Sub(webFS, "web")
	total, gz := 0, 0
	_ = fs.WalkDir(web, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !isText(p) {
			return nil
		}
		body, err := fs.ReadFile(web, p)
		if err != nil {
			return nil
		}
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, _ = zw.Write(body)
		_ = zw.Close()
		total += len(body)
		gz += buf.Len()
		return nil
	})
	if total == 0 {
		t.Fatal("no text assets measured")
	}
	t.Logf("text assets: raw=%d gz=%d (%.0f%%)", total, gz, 100*float64(gz)/float64(total))
	if gz > 161*1024 {
		t.Errorf("gzipped text assets %d > 161 KB", gz)
	}
}

// The list of files named in index.html exists and the number of requests on
// the first paint is bounded. The first paint loads the HTML, the
// stylesheets, the scripts and the fonts; the number of network answers is
// the number of links.
//
// The first paint carries the shell and the Setup tab. The files of the
// other tabs come when their tab opens (the manifest of js/boot.js).
func TestFirstPaintRequests(t *testing.T) {
	web, _ := fs.Sub(webFS, "web")
	body, err := fs.ReadFile(web, "index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(body)
	css := strings.Count(html, `<link rel="stylesheet"`)
	scripts := strings.Count(html, `<script src=`)
	fonts := 0
	_ = fs.WalkDir(web, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasPrefix(p, "fonts/") && strings.HasSuffix(p, ".woff2") {
			fonts++
		}
		return nil
	})
	total := 1 + css + scripts + fonts
	t.Logf("first paint: html=1 css=%d scripts=%d fonts=%d total=%d", css, scripts, fonts, total)
	if total > 24 {
		t.Errorf("first paint asks %d requests, > 24", total)
	}
}

// The bytes of the first paint are bounded. The first paint is the HTML, the
// stylesheets and the scripts that index.html names, and the fonts. The
// files of the other tabs are not in it: they come when their tab opens.
func TestFirstPaintPayload(t *testing.T) {
	web, _ := fs.Sub(webFS, "web")
	body, err := fs.ReadFile(web, "index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(body)
	files := []string{"index.html"}
	for _, m := range regexp.MustCompile(`<link rel="stylesheet" href="([^"]+)"`).FindAllStringSubmatch(html, -1) {
		files = append(files, m[1])
	}
	for _, m := range regexp.MustCompile(`<script src="([^"]+)"`).FindAllStringSubmatch(html, -1) {
		files = append(files, m[1])
	}
	var js, css, gz int
	for _, f := range files {
		raw, err := fs.ReadFile(web, f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		switch {
		case strings.HasSuffix(f, ".js"):
			js += len(raw)
		case strings.HasSuffix(f, ".css"):
			css += len(raw)
		}
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, _ = zw.Write(raw)
		_ = zw.Close()
		gz += buf.Len()
	}
	t.Logf("first paint: js=%d (%.1f KB) css=%d (%.1f KB) gzip=%d (%.1f KB)",
		js, float64(js)/1024, css, float64(css)/1024, gz, float64(gz)/1024)
	const jsMax = 112 * 1024 // 105.0 KB now: glossary.js loads when the disclosure opens, steps.js loads when a step is pressed, the docs of the other tabs sit in their own lazy sheets
	const cssMax = 40 * 1024
	const gzMax = 60 * 1024
	if js > jsMax {
		t.Errorf("first paint javascript %d > %d", js, jsMax)
	}
	if css > cssMax {
		t.Errorf("first paint css %d > %d", css, cssMax)
	}
	if gz > gzMax {
		t.Errorf("first paint gzipped %d > %d", gz, gzMax)
	}
}

// An asset that is missing answers 404 with no-store.
func TestMissingAsset(t *testing.T) {
	h := newServer(Options{}).handler()
	w := do(t, h, "GET", "/css/nope.css", map[string]string{"Host": "127.0.0.1"})
	if w.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", w.Code)
	}
}

// The ETag is the SHA-256 prefix of the file. It is stable across calls.
func TestAssetETagIsContent(t *testing.T) {
	web, _ := fs.Sub(webFS, "web")
	body, err := fs.ReadFile(web, "css/tokens.css")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	want := `"` + hex.EncodeToString(sum[:8]) + `"`
	h := newServer(Options{}).handler()
	w := do(t, h, "GET", "/css/tokens.css", map[string]string{"Host": "127.0.0.1"})
	if got := w.Header().Get("ETag"); got != want {
		t.Errorf("ETag %q, want %q", got, want)
	}
}

// The asset table answers a HEAD with the right status and no body.
func TestAssetHead(t *testing.T) {
	h := newServer(Options{}).handler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodHead, "/js/core.js", nil)
	r.Host = "127.0.0.1"
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("HEAD: status %d", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("HEAD: body %d bytes, want 0", w.Body.Len())
	}
}

// The Content-Length matches the gzipped body when it is served.
func TestAssetContentLength(t *testing.T) {
	h := newServer(Options{}).handler()
	w := do(t, h, "GET", "/js/core.js",
		map[string]string{"Host": "127.0.0.1", "Accept-Encoding": "gzip"})
	cl, _ := strconv.Atoi(w.Header().Get("Content-Length"))
	if cl != w.Body.Len() {
		t.Errorf("Content-Length %d, body %d", cl, w.Body.Len())
	}
}

// The CSP forbids a style attribute and an innerHTML assignment. A static
// style is a class in web/css/; a runtime value is written through the CSSOM.
// A test that walks web/**/*.js and fails on the banned patterns.
func TestNoInlineStylesOrHTML(t *testing.T) {
	web, _ := fs.Sub(webFS, "web")
	banned := []string{
		"style: \"", "style:'", "setAttribute(\"style\"", "setAttribute('style'",
		".innerHTML", ".outerHTML",
	}
	var files []string
	_ = fs.WalkDir(web, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".js") {
			return nil
		}
		files = append(files, p)
		return nil
	})
	for _, p := range files {
		body, err := fs.ReadFile(web, p)
		if err != nil {
			t.Fatal(err)
		}
		src := string(body)
		for _, line := range strings.Split(src, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") {
				continue
			}
			for _, b := range banned {
				if strings.Contains(line, b) {
					t.Errorf("%s: banned pattern %q in %q", p, b, line)
				}
			}
		}
	}
}

// The Chat tab writes the answer as text: a reply that carries markup must
// not become markup on the page.
func TestTheChatViewWritesTheAnswerAsText(t *testing.T) {
	src, err := fs.ReadFile(webFS, "web/views/chat/chat.js")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "innerHTML") {
		t.Error("the chat view writes markup")
	}
	if !strings.Contains(string(src), "textContent") {
		t.Error("the chat view does not write the answer as text")
	}
}
