package onboarding

import (
	"fmt"
	"strings"

	"github.com/opengfx1030/bernie/internal/serving/recipe"
)

// Recommendation is the recipe bernie recommends for this host: which one,
// what it is, why it fits and the other recipes that fit. The CLI, the MCP
// server and the page read this one record, so they cannot disagree about
// the choice.
type Recommendation struct {
	// ID is the id of the recommended recipe.
	ID string `json:"id"`
	// Name is the human name of the recipe ("Qwen3.8-27B AWQ (INT4)").
	Name string `json:"name"`
	// Description is the one line of the recipe file that says what it is.
	Description string `json:"description,omitempty"`
	// Why is the one sentence that says why this recipe is the one.
	Why string `json:"why"`
	// Alternatives are the other recipes that fit this host.
	Alternatives []string `json:"alternatives,omitempty"`
}

// Estimate is what a step costs on this host: the time it took the last
// time it ran here and the size it moved. It is empty when the step never
// ran on this host: the numbers are measured, never guessed.
type Estimate struct {
	// Seconds is the time the last run of the step took.
	Seconds int `json:"seconds,omitempty"`
	// SizeBytes is the size the last run of the step moved.
	SizeBytes int64 `json:"size_bytes,omitempty"`
	// Text is the same estimate in words: "about 2 min, 1.2 GB". The CLI,
	// the MCP server and the page print this one sentence.
	Text string `json:"text,omitempty"`
}

// recommend derives the recommendation from the facts.
func recommend(f *Facts) *Recommendation {
	if f.Recipe == "" {
		return nil
	}
	e, ok := findRecipe(f.Recipes, f.Recipe)
	if !ok {
		return nil
	}
	r := &Recommendation{ID: e.ID, Name: e.Name, Description: e.Description, Why: recommendWhy(f, e)}
	for _, o := range f.Recipes.Recipes {
		if o.ID != e.ID && o.Fit.Usable() {
			r.Alternatives = append(r.Alternatives, o.ID)
		}
	}
	return r
}

// recommendWhy is the one sentence that says why this recipe is the one.
// It names the cards of the host and the reasons the fit gave.
func recommendWhy(f *Facts, e recipe.ListEntry) string {
	parts := []string{}
	if len(e.Fit.Reasons) > 0 {
		parts = append(parts, strings.Join(e.Fit.Reasons, "; "))
	}
	if g := f.gpuSummary(); g != "" {
		parts = append(parts, "it fits "+g)
	}
	if len(parts) == 0 {
		return "it is the recipe that fits this host"
	}
	return strings.Join(parts, "; ")
}

// findRecipe returns the entry of a recipe of a listing.
func findRecipe(l recipe.Listing, id string) (recipe.ListEntry, bool) {
	for _, e := range l.Recipes {
		if e.ID == id {
			return e, true
		}
	}
	return recipe.ListEntry{}, false
}

// estimateOf returns what a step cost the last time it ran on this host.
// The numbers come from the job that ran it: they are measured, not guessed.
func estimateOf(f *Facts, step string) *Estimate {
	j := lastJob(f.Jobs, step)
	if j == nil || j.State != JobDone || j.FinishedAt == nil {
		return nil
	}
	e := &Estimate{Seconds: int(j.FinishedAt.Sub(j.StartedAt).Seconds())}
	if e.Seconds < 0 {
		e.Seconds = 0
	}
	if j.Progress != nil {
		e.SizeBytes = j.Progress.BytesTotal
	}
	if e.Seconds == 0 && e.SizeBytes == 0 {
		return nil
	}
	e.Text = estimateSentence(e)
	return e
}

// estimateSentence says what a step costs in words: "about 2 min, 1.2 GB".
// It returns "" when the step has no measured cost yet.
func estimateSentence(e *Estimate) string {
	if e == nil {
		return ""
	}
	var parts []string
	if e.Seconds > 0 {
		parts = append(parts, "about "+durationWords(e.Seconds))
	}
	if e.SizeBytes > 0 {
		parts = append(parts, sizeWords(e.SizeBytes))
	}
	return strings.Join(parts, ", ")
}

// durationWords is a number of seconds in words: "45 s", "2 min", "1 h".
func durationWords(seconds int) string {
	switch {
	case seconds < 60:
		return fmt.Sprintf("%d s", seconds)
	case seconds < 3600:
		return fmt.Sprintf("%d min", seconds/60)
	default:
		return fmt.Sprintf("%d h", seconds/3600)
	}
}

// sizeWords is a number of bytes in words: "1.2 GB".
func sizeWords(bytes int64) string {
	const unit = 1000
	switch {
	case bytes >= unit*unit*unit:
		return fmt.Sprintf("%.1f GB", float64(bytes)/(unit*unit*unit))
	case bytes >= unit*unit:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(unit*unit))
	case bytes >= unit:
		return fmt.Sprintf("%.1f kB", float64(bytes)/unit)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
