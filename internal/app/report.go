package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/opengfx1030/bernie/internal/doctor"
	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/host"
)

// ShareReport is the redacted report a person can paste into an issue
// (ia.md R24). No hostname, no username, no token, no path with the name
// of the user.
type ShareReport struct {
	Report   string          `json:"report"`
	Sections []ReportSection `json:"sections"`
	// Removed says what the report took out, so a reader knows it was
	// checked: one entry per kind, with the number of places.
	Removed []Removal `json:"removed"`
}

// ReportSection is one block of the report.
type ReportSection struct {
	Title string   `json:"title"`
	Lines []string `json:"lines"`
}

// BuildShareReport builds the redacted report of this host.
func BuildShareReport(ctx context.Context) (ShareReport, error) {
	h, err := host.Probe(ctx, root)
	if err != nil {
		return ShareReport{}, err
	}
	vis, gerr := gpu.Visible(root)
	if gpu.Restricted(gerr) {
		return ShareReport{}, gerr
	}
	rep, err := Doctor(ctx, nil, "")
	if err != nil {
		return ShareReport{}, err
	}
	return buildShareReport(h, vis.GPUs, rep), nil
}

func buildShareReport(h host.Info, gpus []gpu.GPU, rep doctor.Report) ShareReport {
	var sections []ReportSection

	osLine := h.OS.Pretty
	if osLine == "" {
		osLine = h.OS.ID
	}
	if h.OS.VersionID != "" {
		osLine = strings.TrimSpace(osLine + " " + h.OS.VersionID)
	}
	hostLines := []string{}
	if osLine != "" {
		hostLines = append(hostLines, "OS: "+osLine)
	}
	if h.Kernel != "" {
		hostLines = append(hostLines, "Kernel: "+h.Kernel)
	}
	if h.CPU.Model != "" {
		hostLines = append(hostLines, "CPU: "+h.CPU.Model)
	}
	if h.CPU.Threads > 0 {
		hostLines = append(hostLines, "CPU threads: "+fmt.Sprint(h.CPU.Threads))
	}
	if h.RAMBytes > 0 {
		hostLines = append(hostLines, "RAM: "+gibString(h.RAMBytes))
	}
	if h.PkgManager != "" {
		hostLines = append(hostLines, "Package manager: "+h.PkgManager)
	}
	if h.Virtualization != "" {
		hostLines = append(hostLines, "Virtualization: "+h.Virtualization)
	}
	if len(hostLines) > 0 {
		sections = append(sections, ReportSection{Title: "The host", Lines: hostLines})
	}

	if len(gpus) > 0 {
		lines := []string{fmt.Sprintf("Cards: %d", len(gpus))}
		seen := map[string]int{}
		for _, g := range gpus {
			seen[g.Name+"|"+g.GFX]++
		}
		keys := make([]string, 0, len(seen))
		for k := range seen {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			name, gfx := k, ""
			if i := strings.Index(k, "|"); i >= 0 {
				name, gfx = k[:i], k[i+1:]
			}
			line := fmt.Sprintf("%d x %s", seen[k], name)
			if gfx != "" {
				line += " (" + gfx + ")"
			}
			lines = append(lines, line)
		}
		sections = append(sections, ReportSection{Title: "The cards", Lines: lines})
	}

	summary := []string{
		fmt.Sprintf("%d ok", rep.Summary[doctor.OK]),
		fmt.Sprintf("%d warn", rep.Summary[doctor.Warn]),
		fmt.Sprintf("%d fail", rep.Summary[doctor.Fail]),
		fmt.Sprintf("%d skip", rep.Summary[doctor.Skip]),
	}
	checkLines := []string{"Summary: " + strings.Join(summary, ", ")}
	for _, c := range rep.Checks {
		if c.Status == doctor.OK {
			continue
		}
		line := fmt.Sprintf("[%s] %s: %s", strings.ToUpper(string(c.Status)), c.Title, c.Detail)
		if c.Fix != "" {
			line += "\n  fix: " + c.Fix
		}
		checkLines = append(checkLines, line)
	}
	sections = append(sections, ReportSection{Title: "The checks", Lines: checkLines})

	sc := newScrubber(h.Hostname)
	for i := range sections {
		for j, l := range sections[i].Lines {
			sections[i].Lines[j] = sc.clean(l)
		}
	}
	removed := sc.removed()
	return ShareReport{Report: renderShareReport(sections, removed), Sections: sections, Removed: removed}
}

func renderShareReport(sections []ReportSection, removed []Removal) string {
	var b strings.Builder
	b.WriteString("# bernie host report\n")
	for _, s := range sections {
		b.WriteString("\n## " + s.Title + "\n")
		for _, l := range s.Lines {
			b.WriteString(l + "\n")
		}
	}
	b.WriteString("\nThis report carries no hostname, no username, no home path and no token.\n")
	var parts []string
	for _, r := range removed {
		if r.Count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", r.Count, r.What))
		}
	}
	if len(parts) == 0 {
		parts = []string{"nothing"}
	}
	b.WriteString("Removed: " + strings.Join(parts, ", ") + ".\n")
	return b.String()
}

func gibString(b uint64) string {
	return fmt.Sprintf("%.1f GiB", float64(b)/(1<<30))
}

func currentUserName() string {
	name := os.Getenv("USER")
	if name == "" {
		name = os.Getenv("LOGNAME")
	}
	if name == "" {
		return ""
	}
	return filepath.Base(name)
}
