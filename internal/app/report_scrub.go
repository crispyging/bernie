package app

import (
	"os"
	"regexp"
	"strings"

	"github.com/opengfx1030/bernie/internal/support/redact"
)

// Removal is one kind of value the share report took out, and how often.
type Removal struct {
	What  string `json:"what"`
	Count int    `json:"count"`
}

// The kinds of a removal, in the order the report lists them.
const (
	removedHome     = "home path"
	removedUser     = "user name"
	removedHostname = "hostname"
	removedSecret   = "secret"
)

// scrubber takes out of a line what identifies the host or grants access:
// the home directory, the name of the user, the hostname and secrets by
// shape. It counts every place it changed.
type scrubber struct {
	home, user string
	host       *regexp.Regexp
	counts     map[string]int
}

func newScrubber(hostname string) *scrubber {
	sc := &scrubber{user: currentUserName(), counts: map[string]int{}}
	if home, err := os.UserHomeDir(); err == nil && len(home) > 1 {
		sc.home = home
	}
	// A hostname is replaced as a whole word, so a short name does not
	// cut other words in half.
	if len(hostname) >= 2 {
		sc.host = regexp.MustCompile(`\b` + regexp.QuoteMeta(hostname) + `\b`)
	}
	return sc
}

func (sc *scrubber) clean(s string) string {
	s = sc.replace(s, removedHome, sc.home, "~")
	if len(sc.user) >= 2 {
		s = sc.replace(s, removedUser, sc.user, "<user>")
	}
	if sc.host != nil {
		n := len(sc.host.FindAllStringIndex(s, -1))
		sc.counts[removedHostname] += n
		s = sc.host.ReplaceAllString(s, "<host>")
	}
	out := redact.Text(s)
	sc.counts[removedSecret] += strings.Count(out, "<redacted>") - strings.Count(s, "<redacted>")
	return out
}

func (sc *scrubber) replace(s, what, old, repl string) string {
	if old == "" {
		return s
	}
	sc.counts[what] += strings.Count(s, old)
	return strings.ReplaceAll(s, old, repl)
}

// removed lists every kind the scrubber looks for, with its count, so the
// reader sees what was checked and not only what was found.
func (sc *scrubber) removed() []Removal {
	out := []Removal{}
	for _, k := range []string{removedHostname, removedUser, removedHome, removedSecret} {
		out = append(out, Removal{What: k, Count: sc.counts[k]})
	}
	return out
}

// redactReportText cleans one line without a hostname.
func redactReportText(s string) string { return newScrubber("").clean(s) }
