package bench

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/serving/tuning"
	"github.com/opengfx1030/bernie/internal/support/hostroot"
	"github.com/opengfx1030/bernie/internal/support/netx"
	"github.com/opengfx1030/bernie/internal/support/paths"
)

// Dir is where results are stored: <state>/bench.
func Dir() string { return filepath.Join(paths.State(), "bench") }

// ErrBenchResultNotFound means no saved result with the requested id.
var ErrBenchResultNotFound = errors.New("no saved benchmark result with that id")

// Save writes r to <state>/bench/<id>.json and returns the path. An empty ID
// is assigned.
func Save(r *Result) (string, error) {
	if r.ID == "" {
		r.ID = newID(r.Time.UTC(), r.Options, 0)
	}
	dir, err := paths.Ensure(Dir())
	if err != nil {
		return "", err
	}
	r.Summary = headline(r)
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, r.ID+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return "", err
	}
	return path, os.Rename(tmp, path)
}

// List returns all saved results, oldest first. Unreadable files are skipped.
func List() ([]Result, error) {
	files, err := filepath.Glob(filepath.Join(Dir(), "*.json"))
	if err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(files))
	for _, f := range files {
		r, err := load(f)
		if err != nil {
			continue
		}
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Time.Equal(out[j].Time) {
			return out[i].Time.Before(out[j].Time)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// Delete removes the saved result with this id (a prefix is enough when
// the match is unique). A missing id answers ErrBenchResultNotFound;
// the file is removed from <state>/bench.
func Delete(id string) (string, error) {
	if id == "" || strings.ContainsAny(id, `/\`) {
		return "", fmt.Errorf("invalid result id %q", id)
	}
	path := filepath.Join(Dir(), id+".json")
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return "", err
		}
		return id, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	files, _ := filepath.Glob(filepath.Join(Dir(), id+"*.json"))
	switch len(files) {
	case 0:
		return "", ErrBenchResultNotFound
	case 1:
		if err := os.Remove(files[0]); err != nil {
			return "", err
		}
		base := filepath.Base(files[0])
		return strings.TrimSuffix(base, ".json"), nil
	default:
		return "", fmt.Errorf("bench result id %q is ambiguous (%d matches)", id, len(files))
	}
}

// Get returns the result with this id, or the only one whose id starts with
// it.
func Get(id string) (*Result, error) {
	if id == "" || strings.ContainsAny(id, `/\`) {
		return nil, fmt.Errorf("invalid result id %q", id)
	}
	r, err := load(filepath.Join(Dir(), id+".json"))
	if err == nil {
		return r, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	files, _ := filepath.Glob(filepath.Join(Dir(), id+"*.json"))
	switch len(files) {
	case 0:
		return nil, fmt.Errorf("no bench result %q", id)
	case 1:
		return load(files[0])
	default:
		return nil, fmt.Errorf("bench result id %q is ambiguous (%d matches)", id, len(files))
	}
}

func load(path string) (*Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Result
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	// A result saved before the metric groups existed still shows its
	// numbers: the old fields are converted on read.
	r.upgrade()
	r.Summary = headline(&r)
	return &r, nil
}

// runRecord holds the fields of <state>/runs/<name>/run.json (spec §8.3)
// that bench needs.
type runRecord struct {
	Name   string          `json:"name"`
	Recipe string          `json:"recipe"`
	Host   string          `json:"host"`
	Port   int             `json:"port"`
	Plan   json.RawMessage `json:"plan"`
}

// planGPUFields are the GPU selection fields of a run plan.
type planGPUFields struct {
	GPUs       []int             `json:"gpus"`
	HIPDevices string            `json:"hip_devices"`
	Env        map[string]string `json:"env"`
}

// indices returns the host HIP indices used by the plan. Older records may
// have only HIP_VISIBLE_DEVICES in the environment.
func (p planGPUFields) indices() []int {
	if len(p.GPUs) > 0 {
		return p.GPUs
	}
	s := p.Env["HIP_VISIBLE_DEVICES"]
	if s == "" {
		s = p.HIPDevices
	}
	var out []int
	for _, part := range strings.Split(s, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err == nil && n >= 0 {
			out = append(out, n)
		}
	}
	return out
}

func runRecords(stateDir string) []runRecord {
	files, _ := filepath.Glob(filepath.Join(stateDir, "runs", "*", "run.json"))
	var out []runRecord
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var r runRecord
		if json.Unmarshal(data, &r) == nil && r.Port > 0 {
			out = append(out, r)
		}
	}
	return out
}

// DefaultURL is the URL of the only run record, else FallbackURL.
func DefaultURL() string { return defaultURL(paths.State()) }

func defaultURL(stateDir string) string {
	recs := runRecords(stateDir)
	if len(recs) != 1 {
		return FallbackURL
	}
	return "http://" + net.JoinHostPort(dialHost(recs[0].Host), strconv.Itoa(recs[0].Port))
}

// dialHost maps a listen address to an address a local client can dial.
func dialHost(h string) string {
	switch h {
	case "", "0.0.0.0", "::", "[::]":
		return "127.0.0.1"
	}
	return h
}

// sameEndpoint reports whether a client URL reaches the run's listen address.
func sameEndpoint(u *url.URL, r runRecord) bool {
	if u.Port() != strconv.Itoa(r.Port) {
		return false
	}
	h, rh := u.Hostname(), dialHost(r.Host)
	return h == rh || (netx.Loopback(h) && netx.Loopback(rh))
}

// gpuList is gpu.List; tests replace it.
var gpuList = gpu.List

// fingerprint records where the result came from: host name, the GPUs used
// by the run and, when the URL belongs to a run, its name, recipe and plan
// hash. Without a matching run plan the GPU selection is unknown, so every
// host GPU is listed and the scope says so.
func fingerprint(stateDir, rawURL string) map[string]string {
	fp := map[string]string{}
	if h, err := os.Hostname(); err == nil {
		fp["host"] = h
	}
	matched := matchRun(stateDir, rawURL)
	if matched != nil {
		fp["run"] = matched.Name
		if matched.Recipe != "" {
			fp["recipe"] = matched.Recipe
		}
		if len(matched.Plan) > 0 && string(matched.Plan) != "null" {
			var buf bytes.Buffer
			if json.Compact(&buf, matched.Plan) == nil {
				sum := sha256.Sum256(buf.Bytes())
				fp["plan_hash"] = hex.EncodeToString(sum[:])[:12]
			}
		}
	}
	summary, scope := gpuFingerprint(matched)
	if summary != "" {
		fp["gpus"] = summary
		fp["gpu_scope"] = scope
	}
	if gpus, _, _ := selectedGPUs(matched); len(gpus) > 0 {
		if caps := powerCaps(gpus); caps != "" {
			fp["power_cap"] = caps
		}
		if clocks := gpuClocks(gpus); clocks != "" {
			fp["clocks"] = clocks
		}
	}
	return fp
}

// powerCaps reads the power cap of the GPUs. It is empty when the host
// publishes none.
func powerCaps(gpus []gpu.GPU) string {
	var parts []string
	for _, p := range tuning.PowerStatus(gpus) {
		parts = append(parts, fmt.Sprintf("HIP %d %.0f W", p.HIPIndex, p.CapW))
	}
	return strings.Join(parts, ", ")
}

// gpuClocks reads the current clocks of the GPUs. It is empty when the
// host publishes none.
func gpuClocks(gpus []gpu.GPU) string {
	var parts []string
	for _, s := range gpu.Telemetry(hostroot.Root(), gpus) {
		if s.SclkMHz == nil && s.MclkMHz == nil {
			continue
		}
		part := fmt.Sprintf("HIP %d", s.HIPIndex)
		if s.SclkMHz != nil {
			part += fmt.Sprintf(" sclk %d MHz", *s.SclkMHz)
		}
		if s.MclkMHz != nil {
			part += fmt.Sprintf(" mclk %d MHz", *s.MclkMHz)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ", ")
}

// selectedGPUs returns the GPUs of the matched run, the scope of the
// selection and the indices the plan names but the host does not have.
// Without a run plan the selection is unknown, so every host GPU is listed.
func selectedGPUs(rec *runRecord) (gpus []gpu.GPU, scope string, missing []int) {
	if rec != nil {
		var p planGPUFields
		if json.Unmarshal(rec.Plan, &p) == nil {
			if idx := p.indices(); len(idx) > 0 {
				all, _ := gpuList(hostroot.Root())
				byIndex := make(map[int]gpu.GPU, len(all))
				for _, g := range all {
					byIndex[g.HIPIndex] = g
				}
				for _, i := range idx {
					if g, ok := byIndex[i]; ok {
						gpus = append(gpus, g)
					} else {
						missing = append(missing, i)
					}
				}
				return gpus, "run", missing
			}
		}
	}
	all, err := gpuList(hostroot.Root())
	if err != nil || len(all) == 0 {
		return nil, "all (run selection unknown)", nil
	}
	return all, "all (run selection unknown)", nil
}

// gpuFingerprint returns the GPUs used by the matched run. It falls back to
// every host GPU when there is no run plan or the plan has no selection.
func gpuFingerprint(rec *runRecord) (summary, scope string) {
	gpus, scope, missing := selectedGPUs(rec)
	if scope == "run" {
		summary = gpuSummary(gpus)
		for _, i := range missing {
			if summary != "" {
				summary += ", "
			}
			summary += fmt.Sprintf("HIP GPU %d", i)
		}
		return summary, scope
	}
	if len(gpus) == 0 {
		return "none found", scope
	}
	return gpuSummary(gpus), scope
}

// gpuSummary groups GPUs by name and gfx target in HIP order, e.g.
// "2x AMD Radeon PRO W7800 (gfx1100), 8x AMD Radeon PRO V620 (gfx1030)".
func gpuSummary(gpus []gpu.GPU) string {
	type group struct {
		label string
		n     int
	}
	var groups []*group
	idx := map[string]*group{}
	for _, g := range gpus {
		label := g.Name
		if label == "" {
			label = g.DeviceID
		}
		if g.GFX != "" {
			label += " (" + g.GFX + ")"
		}
		gr, ok := idx[label]
		if !ok {
			gr = &group{label: label}
			idx[label] = gr
			groups = append(groups, gr)
		}
		gr.n++
	}
	parts := make([]string, len(groups))
	for i, g := range groups {
		parts[i] = fmt.Sprintf("%dx %s", g.n, g.label)
	}
	return strings.Join(parts, ", ")
}
