package dashboard

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

// tabOrder is the order of the rail: Setup, Dashboard, Chat, Benchmark,
// Quality, Topology, Fleet, Logs and Advanced.
var tabOrder = []string{"setup", "dashboard", "chat", "benchmark", "quality", "topology", "fleet", "logs", "advanced"}

// tabFiles are the files the manifest of js/boot.js names for the tabs that
// are not the first paint.
var tabFiles = []string{
	"views/chat/chat.js",
	"views/chat/chat.css",
	"views/fleet/fleet.js",
	"views/quality/quality.js",
	"views/topology/topology.js",
	"views/topology/matrix.js",
	"views/topology/links.js",
	"views/topology/checklist.js",
	"views/topology/advanced.js",
	"views/topology/topology.css",
}

// The rail holds the nine tabs in the order of the design, and the page
// holds one panel for each of them.
func TestTheRailHoldsTheNineTabs(t *testing.T) {
	h := newServer(Options{Listen: "127.0.0.1:7860"}).handler()
	w := do(t, h, "GET", "/", map[string]string{"Host": "127.0.0.1:7860"})
	if w.Code != http.StatusOK {
		t.Fatalf("GET /: %d", w.Code)
	}
	body := w.Body.String()
	last := -1
	for _, tab := range tabOrder {
		at := strings.Index(body, `data-tab="`+tab+`"`)
		if at < 0 {
			t.Fatalf("the rail has no %s tab", tab)
		}
		if at < last {
			t.Errorf("the %s tab is out of the order of the rail", tab)
		}
		last = at
		if !strings.Contains(body, `data-tab-panel="`+tab+`"`) {
			t.Errorf("the page has no panel for the %s tab", tab)
		}
	}
}

// The manifest names the files of the tabs and the server serves them: a
// file the manifest names and the server does not serve leaves the tab
// empty.
func TestTheShellServesTheFilesOfTheTabs(t *testing.T) {
	h := newServer(Options{Listen: "127.0.0.1:7860"}).handler()
	boot := do(t, h, "GET", "/js/boot.js", map[string]string{"Host": "127.0.0.1:7860"})
	if boot.Code != http.StatusOK {
		t.Fatalf("GET /js/boot.js: %d", boot.Code)
	}
	manifest := boot.Body.String()
	for _, file := range tabFiles {
		if !strings.Contains(manifest, file) {
			t.Errorf("the manifest does not name %s", file)
		}
		r := do(t, h, "GET", "/"+file, map[string]string{"Host": "127.0.0.1:7860"})
		if r.Code != http.StatusOK {
			t.Errorf("GET /%s: %d", file, r.Code)
		}
	}
}

// Every rail tab has a panel, a route, a script bundle and a row in the
// README. A tab that misses one of them opens empty or disappears from the
// documentation without a tab-by-tab smoke noticing.
func TestEveryRailTabHasItsContract(t *testing.T) {
	h := newServer(Options{Listen: "127.0.0.1:7860"}).handler()
	page := do(t, h, "GET", "/", map[string]string{"Host": "127.0.0.1:7860"})
	boot := do(t, h, "GET", "/js/boot.js", map[string]string{"Host": "127.0.0.1:7860"})
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	views := map[string]string{
		"setup": "viewSetup", "dashboard": "viewDashboard", "chat": "viewChat",
		"benchmark": "viewBenchmark", "quality": "viewQuality", "topology": "viewTopology", "fleet": "viewFleet",
		"logs": "viewLogs", "advanced": "viewAdvanced",
	}
	labels := map[string]string{
		"setup": "Setup", "dashboard": "Dashboard", "chat": "Chat",
		"benchmark": "Benchmark", "quality": "Quality", "topology": "Topology", "fleet": "Fleet",
		"logs": "Logs", "advanced": "Advanced",
	}
	for _, tab := range tabOrder {
		t.Run(tab, func(t *testing.T) {
			if !strings.Contains(page.Body.String(), `data-tab-panel="`+tab+`"`) {
				t.Error("the page has no panel")
			}
			if !strings.Contains(boot.Body.String(), tab+`: "`+views[tab]+`"`) {
				t.Error("the router has no route")
			}
			if !strings.Contains(boot.Body.String(), tab+": [") {
				t.Error("the manifest has no script bundle")
			}
			if !strings.Contains(string(readme), "| "+labels[tab]+" |") {
				t.Error("the README has no tab row")
			}
		})
	}
}

// The old alias of the topology is gone: #topology opens the Topology tab.
func TestTheTopologyAliasIsGone(t *testing.T) {
	h := newServer(Options{Listen: "127.0.0.1:7860"}).handler()
	boot := do(t, h, "GET", "/js/boot.js", map[string]string{"Host": "127.0.0.1:7860"})
	body := boot.Body.String()
	if !strings.Contains(body, `topology: "viewTopology"`) {
		t.Error("the manifest does not name the view of the Topology tab")
	}
	if strings.Contains(body, `topology: ["dashboard", "topology"]`) {
		t.Error("the old alias #topology still opens the Dashboard tab")
	}
}

// No tab is planned any more: the Chat tab and the Fleet tab both ship.
func TestNoTabIsPlanned(t *testing.T) {
	h := newServer(Options{Listen: "127.0.0.1:7860"}).handler()
	w := do(t, h, "GET", "/", map[string]string{"Host": "127.0.0.1:7860"})
	body := w.Body.String()
	for _, tab := range []string{"chat", "fleet"} {
		entry := railEntry(body, tab)
		if entry == "" {
			t.Fatalf("the rail has no %s tab", tab)
		}
		if strings.Contains(entry, "tab-planned") || strings.Contains(entry, "soon-note") {
			t.Errorf("the %s tab is still marked as planned: %s", tab, entry)
		}
	}
	if strings.Contains(body, "soon-note") {
		t.Error("the rail still carries the planned note")
	}
}

// The theme of the page is a three-state control in words: Auto (the page
// follows the system), Dark and Light. The palette is set before the first
// paint, so the page never shows the other one for a moment.
func TestTheThemeHasThreeChoicesInWords(t *testing.T) {
	h := newServer(Options{Listen: "127.0.0.1:7860"}).handler()
	w := do(t, h, "GET", "/", map[string]string{"Host": "127.0.0.1:7860"})
	if w.Code != http.StatusOK {
		t.Fatalf("GET /: %d", w.Code)
	}
	page := w.Body.String()
	for _, choice := range []string{"auto", "dark", "light"} {
		if !strings.Contains(page, `data-theme-choice="`+choice+`"`) {
			t.Errorf("the theme control has no %s choice", choice)
		}
	}
	for _, word := range []string{">Auto<", ">Dark<", ">Light<"} {
		if !strings.Contains(page, word) {
			t.Errorf("the theme control does not say %s", word)
		}
	}
	if !strings.Contains(page, `data-theme-choice="auto" aria-pressed="true"`) {
		t.Error("the theme does not start on Auto")
	}
	if !strings.Contains(page, `id="themeLive"`) || !strings.Contains(page, `aria-live="polite"`) {
		t.Error("the theme choice is not announced")
	}
	// The palette is set before the first paint: the theme script is in the
	// head, before the stylesheets.
	end := strings.Index(page, "</head>")
	if end < 0 {
		t.Fatal("the page has no head")
	}
	head := page[:end]
	at := strings.Index(head, `<script src="js/theme.js">`)
	if at < 0 {
		t.Fatal("the theme script is not in the head of the page")
	}
	if at > strings.Index(head, `rel="stylesheet"`) {
		t.Error("the theme script comes after the stylesheets: the page would flash")
	}
	js := do(t, h, "GET", "/js/theme.js", map[string]string{"Host": "127.0.0.1:7860"})
	if js.Code != http.StatusOK {
		t.Fatalf("GET /js/theme.js: %d", js.Code)
	}
	for _, want := range []string{`"auto", "dark", "light"`, "prefers-color-scheme: dark", "bernie.theme"} {
		if !strings.Contains(js.Body.String(), want) {
			t.Errorf("js/theme.js does not hold %q", want)
		}
	}
}

// railEntry returns the rail entry of one tab.
func railEntry(body, tab string) string {
	at := strings.Index(body, `data-tab="`+tab+`"`)
	if at < 0 {
		return ""
	}
	end := strings.Index(body[at:], "</a>")
	if end < 0 {
		return ""
	}
	return body[at : at+end]
}
