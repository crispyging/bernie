package onboarding

import (
	"fmt"
)

// Fact is one fact of the host with the reason why it matters. The facts
// are the numbers the page shows on Setup, and the reason is what a reader
// needs to know about them.
type Fact struct {
	// ID is the name of the fact: "gpus", "memory", "recipe".
	ID    string `json:"id"`
	Label string `json:"label"`
	Value string `json:"value"`
	Why   string `json:"why"`
}

// factsOf derives the facts of the host from the gathered state. The order
// is the order of the page: the hardware, then the software, then the
// model.
func factsOf(f *Facts) []Fact {
	out := []Fact{}
	// The GPU fact is always there. A host without a card is exactly the
	// fact a first-time reader needs, and a missing value is a dash.
	out = append(out, Fact{
		ID:    "gpus",
		Label: "GPUs",
		Value: dashIfEmpty(f.gpuSummary()),
		Why:   gpuWhy(f),
	})
	if f.Host.RAMBytes > 0 {
		out = append(out, Fact{
			ID:    "memory",
			Label: "Host RAM",
			Value: fmt.Sprintf("%.1f GiB", float64(f.Host.RAMBytes)/(1<<30)),
			Why:   "Host RAM is separate from GPU VRAM; GPU capacity is checked per selected card.",
		})
	}
	if f.Recipe != "" {
		out = append(out, Fact{
			ID:    "recipe",
			Label: "Recipe",
			Value: f.Recipe,
			Why:   recipeWhy(f),
		})
	}
	out = append(out, Fact{
		ID:    "python",
		Label: "Python",
		Value: pythonValue(f),
		Why:   "The Python environment holds the torch and vLLM builds that match the ROCm of this host.",
	})
	if f.Model != "" {
		out = append(out, Fact{
			ID:    "model",
			Label: "Model",
			Value: f.Model,
			Why:   "The model is the weight the server loads before it answers.",
		})
	}
	return out
}

// dashIfEmpty is the value of a fact that the host does not have: a dash,
// never an empty cell.
func dashIfEmpty(v string) string {
	if v == "" {
		return "–"
	}
	return v
}

// gpuWhy says why the cards of this host are what they are.
func gpuWhy(f *Facts) string {
	unique := map[string]bool{}
	for _, a := range archs(f.GPUs) {
		unique[a] = true
	}
	if len(unique) == 0 {
		return "This machine has no AMD GPU, so bernie cannot serve a model here."
	}
	if len(unique) == 1 {
		for a := range unique {
			return fmt.Sprintf("Every card reports %s; recipe compatibility also depends on each selected GPU's VRAM and host RAM.", a)
		}
	}
	return "GPU architectures differ; check the architecture and VRAM of each selected GPU against the recipe."
}

// recipeWhy says why this recipe is the one to serve.
func recipeWhy(f *Facts) string {
	if len(f.Fits) == 0 {
		return "No recipe fits this host yet: the checks below say what is missing."
	}
	if f.Recipe == f.Best {
		return "It is the recipe that fits this host best: its inputs are all here."
	}
	return fmt.Sprintf("It was chosen by name; %s is the recipe that fits this host best.", f.Best)
}

// pythonValue names the venv of the host and where the name comes from.
func pythonValue(f *Facts) string {
	if f.Venv == "" {
		return "not set"
	}
	if f.VenvSource == "" {
		return f.Venv
	}
	return f.Venv + " (" + f.VenvSource + ")"
}
