package recipe

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/opengfx1030/bernie/internal/machine/gpu"
	"github.com/opengfx1030/bernie/internal/machine/host"
	"github.com/opengfx1030/bernie/internal/machine/topo"
)

// Modes.
const (
	ModeHost      = "host"
	ModeContainer = "container"
)

// DefaultPort is vLLM's port when the recipe sets none.
const DefaultPort = 8000

// DefaultShmSize is the container shared memory size when the recipe sets none.
const DefaultShmSize = "16g"

// ImageTunedMoE is the fork's tuned MoE config folder inside the vllm-rdna
// image. A host plan replaces it with PlanOptions.TunedMoE for the selected
// GPUs' architecture, or drops it: a container path never reaches a host
// process.
const ImageTunedMoE = "/src/vllm/tuned-moe"

const gib = 1 << 30

// Exec is the exact process a plan starts: argv, the environment added to
// (Env) and removed from (Unset) the inherited one, and the working
// directory. The runner fills it.
type Exec struct {
	Argv  []string `json:"argv"`
	Env   []string `json:"env,omitempty"`
	Unset []string `json:"unset,omitempty"`
	Dir   string   `json:"dir,omitempty"`
}

// Mount is one extra bind mount of a container plan.
type Mount struct {
	Host      string `json:"host"`
	Container string `json:"container"`
	ReadOnly  bool   `json:"read_only"`
}

// Plan is one resolved launch (spec terms: recipe + overrides + host).
type Plan struct {
	Recipe     string `json:"recipe"`
	Name       string `json:"name"`
	Mode       string `json:"mode"`
	Entrypoint string `json:"entrypoint"` // cli | api
	Model      string `json:"model"`
	ServedName string `json:"served_name,omitempty"`
	Host       string `json:"host"` // --host; "" = vLLM default
	Port       int    `json:"port"`
	// Env is the vLLM environment. Unset lists variables to remove from the
	// inherited environment (env: KEY: null).
	Env   map[string]string `json:"env"`
	Unset []string          `json:"unset,omitempty"`
	// Args follow "serve": the model, then flags sorted by name.
	Args []string `json:"args"`
	// Settings holds the settings with a recipe-provided reason.
	Settings        []Setting `json:"settings,omitempty"`
	GPUs            []int     `json:"gpus"`
	HIPDevices      string    `json:"hip_devices"`
	GPUSelectReason string    `json:"gpu_select_reason"`
	// Restriction says that the environment limits the GPUs bernie uses
	// (spec §7.7). It is empty when nothing is restricted.
	Restriction string `json:"restriction,omitempty"`
	Image       string `json:"image,omitempty"`
	ShmSize     string `json:"shm_size,omitempty"`
	Pull        bool   `json:"pull"`
	// TunableOpProfile is the selected profile; TunableOpRows its host
	// directory (empty: no rows, or the per-user fallback).
	TunableOpProfile string   `json:"tunableop_profile,omitempty"`
	TunableOpRows    string   `json:"tunableop_rows,omitempty"`
	PathInputs       []string `json:"path_inputs"`
	Mounts           []Mount  `json:"mounts,omitempty"`
	// GroupAdd are the numeric group ids a container run needs to open the
	// GPU devices of the host (spec §8.2).
	GroupAdd []host.DeviceGroup `json:"group_add,omitempty"`
	// DeviceAccessRequest is the device access mode that was asked for:
	// classic, cdi or auto (spec §8.2).
	DeviceAccessRequest string `json:"device_access_request,omitempty"`
	// DeviceAccess is the mode the run uses: classic or cdi.
	DeviceAccess string `json:"device_access,omitempty"`
	// DeviceAccessReason says why that mode was chosen, in one sentence.
	DeviceAccessReason string `json:"device_access_reason,omitempty"`
	// CDIDevices are the CDI device names of the selected GPUs.
	CDIDevices []string `json:"cdi_devices,omitempty"`
	// ContainerHIPDevices are the HIP indices inside the container.
	ContainerHIPDevices string   `json:"container_hip_devices,omitempty"`
	Warnings            []string `json:"warnings"`
	// Adjustments are the changes the plan made so the recipe fits this
	// host (spec §7.10). An override is never adjusted. Absent when the
	// plan changed nothing.
	Adjustments []Adjustment `json:"adjustments,omitempty"`
	// Compose is the generated docker-compose.yml of the plan. It is set
	// when the plan is a container plan with an image; host plans leave it
	// empty. The dashboard renders it in the "Show the exact launch
	// command" disclosure (ia.md §5 R42).
	Compose string `json:"compose,omitempty"`
	Exec    *Exec  `json:"exec,omitempty"`
}

// PlanOptions are the inputs of BuildPlan. Sets are applied in order (config
// defaults first, then --set). GPUs nil means no GPU data (no selection).
// TunedMoE returns the host folder of the tuned MoE configs for one gfx
// architecture, or "" when there is none (host mode; nil means none).
type PlanOptions struct {
	Mode, Name string
	Sets       []string
	GPUs       []gpu.GPU
	Topo       *topo.Topology
	TunedMoE   func(gfx string) (string, error)
	// Restriction is the note of the GPU restriction of the environment
	// (spec §7.7). It is empty when nothing is restricted.
	Restriction string
}

// Modes returns the modes a recipe supports: host always, container when it
// names an image.
func Modes(r *Recipe) []string {
	if r.Container != nil && r.Container.Image != "" {
		return []string{ModeHost, ModeContainer}
	}
	return []string{ModeHost}
}

// BuildPlan turns a resolved recipe into a Plan.
func BuildPlan(r *Recipe, o PlanOptions) (*Plan, error) {
	if r.Fragment {
		return nil, fmt.Errorf("%s is a fragment; fragments are only used through extends", r.ID)
	}
	r, inputVals, err := Apply(r, o.Sets)
	if err != nil {
		return nil, err
	}
	if errs := Validate(r); len(errs) > 0 {
		return nil, fmt.Errorf("recipe %s is not valid:\n  %s", r.ID, strings.Join(errs, "\n  "))
	}
	mode, err := planMode(r, o.Mode)
	if err != nil {
		return nil, err
	}
	vals, err := inputValues(r, inputVals)
	if err != nil {
		return nil, err
	}
	interp := func(s string) string { return interpolate(s, vals) }
	adj, warn := fitContext(r, o.Sets, o.GPUs)

	p := &Plan{
		Recipe:      r.ID,
		Name:        o.Name,
		Mode:        mode,
		Entrypoint:  r.Entrypoint,
		Model:       interp(r.Model.Source),
		ServedName:  interp(r.Model.ServedName),
		Env:         map[string]string{},
		GPUs:        []int{},
		PathInputs:  []string{},
		Warnings:    []string{},
		Restriction: o.Restriction,
	}
	if adj != nil {
		p.Adjustments = append(p.Adjustments, *adj)
	}
	if warn != "" {
		p.Warnings = append(p.Warnings, warn)
	}
	if p.Name == "" {
		p.Name = r.ID
	}
	if !idPattern.MatchString(p.Name) || strings.HasPrefix(p.Name, "_") {
		return nil, fmt.Errorf("run name %q: use lowercase letters, digits, '.', '_' and '-'", p.Name)
	}
	if p.Entrypoint == "" {
		p.Entrypoint = "cli"
	}
	local := filepath.IsAbs(p.Model)
	planEnv(r, p, interp, local)
	if err := planArgs(r, p, interp); err != nil {
		return nil, err
	}
	planPaths(r, p, vals, local)
	planContainer(r, p)
	if err := selectGPUs(r, p, o); err != nil {
		return nil, err
	}
	if mode == ModeHost {
		if err := hostTunedMoE(p, o); err != nil {
			return nil, err
		}
	}
	planSettings(r, p, interp)
	return p, nil
}

// planMode returns the mode of the plan (host by default) and checks that
// the recipe supports it.
func planMode(r *Recipe, mode string) (string, error) {
	if mode == "" {
		mode = ModeHost
	}
	switch mode {
	case ModeHost:
	case ModeContainer:
		if r.Container == nil {
			return "", fmt.Errorf("recipe %s needs features not in any published image; use --mode host", r.ID)
		}
	default:
		return "", fmt.Errorf("mode must be host or container, not %q", mode)
	}
	return mode, nil
}

// inputValues returns the value of every input: --set, then default.
// Required inputs must have one.
func inputValues(r *Recipe, given map[string]string) (map[string]string, error) {
	vals := map[string]string{}
	var missing []string
	for _, name := range sortedKeys(r.Inputs) {
		in := r.Inputs[name]
		v, ok := given[name]
		if !ok {
			v = in.Default
		}
		if in.Required && v == "" {
			hint := ""
			if in.Description != "" {
				hint = " (" + in.Description + ")"
			}
			missing = append(missing, fmt.Sprintf("input %s is required%s: --set %s=...", name, hint, name))
		}
		vals[name] = v
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("recipe %s: %s", r.ID, strings.Join(missing, "; "))
	}
	return vals, nil
}

// Env returns the environment of a recipe: the variables its launch sets.
// It is informational: the P2P checklist shows the communication variables
// of the recipe bernie would run.
func Env(set Set, id string) (topo.RecipeEnv, error) {
	r, err := set.Resolve(id)
	if err != nil {
		return topo.RecipeEnv{}, err
	}
	env := map[string]string{}
	for k, v := range r.Env {
		if v == nil {
			continue
		}
		env[k] = scalarString(v)
	}
	return topo.RecipeEnv{ID: r.ID, Env: env}, nil
}

// planEnv fills p.Env and p.Unset. HF_HUB_OFFLINE and TRANSFORMERS_OFFLINE
// default to 1 for a local model path and 0 for a Hugging Face repo id.
func planEnv(r *Recipe, p *Plan, interp func(string) string, local bool) {
	for _, k := range sortedKeys(r.Env) {
		v := r.Env[k]
		if v == nil {
			p.Unset = append(p.Unset, k)
			continue
		}
		p.Env[k] = interp(scalarString(v))
	}
	for _, k := range []string{"HF_HUB_OFFLINE", "TRANSFORMERS_OFFLINE"} {
		if _, set := r.Env[k]; !set {
			p.Env[k] = map[bool]string{true: "1", false: "0"}[local]
		}
	}
}

// planArgs renders the vllm flags into p.Args and sets p.Host and p.Port.
func planArgs(r *Recipe, p *Plan, interp func(string) string) error {
	args := map[string]any{}
	for k, v := range r.Args {
		args[k] = interpAny(v, interp)
	}
	if p.ServedName != "" {
		args["served-model-name"] = p.ServedName
	}
	if revision := interp(r.Model.Revision); revision != "" {
		args["revision"] = revision
	}
	flags, err := RenderArgs(args)
	if err != nil {
		return err
	}
	p.Args = append([]string{p.Model}, flags...)
	p.Host = scalarString(args["host"])
	p.Port = DefaultPort
	if v, ok := args["port"]; ok && v != nil {
		n, ok := toInt(v)
		if !ok || n <= 0 || n > 65535 {
			return fmt.Errorf("args.port: want a TCP port, got %v", v)
		}
		p.Port = n
	}
	return nil
}

// planPaths fills p.PathInputs: path inputs and a local model are host paths
// (mounted in a container at the same path).
func planPaths(r *Recipe, p *Plan, vals map[string]string, local bool) {
	seen := map[string]bool{}
	addPath := func(path, what string) {
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		if !filepath.IsAbs(path) {
			p.Warnings = append(p.Warnings, fmt.Sprintf("%s %q is not an absolute path", what, path))
			return
		}
		p.PathInputs = append(p.PathInputs, path)
		if _, err := os.Stat(path); err != nil {
			p.Warnings = append(p.Warnings, fmt.Sprintf("%s %s does not exist on this host", what, path))
		}
	}
	if local {
		addPath(p.Model, "model")
	}
	for _, name := range sortedKeys(r.Inputs) {
		if r.Inputs[name].Path {
			addPath(vals[name], "input "+name)
		}
	}
	sort.Strings(p.PathInputs)
}

// planContainer sets the image fields. Host mode selects the TunableOp
// profile from the loaded rocBLAS (runner), so it clears them.
func planContainer(r *Recipe, p *Plan) {
	if r.Container != nil {
		p.Image = r.Container.Image
		p.ShmSize = r.Container.ShmSize
		if p.ShmSize == "" {
			p.ShmSize = DefaultShmSize
		}
		p.Pull = r.Container.Pull == nil || *r.Container.Pull
		p.TunableOpProfile = r.Container.TunableOpProfile
		p.DeviceAccessRequest = r.Container.DeviceAccess
	}
	if p.Mode == ModeHost {
		p.Image, p.ShmSize, p.Pull, p.TunableOpProfile = "", "", false, ""
		p.DeviceAccessRequest = ""
	}
}

// hostTunedMoE replaces the image path of VLLM_TUNED_CONFIG_FOLDER with the
// host folder of the selected GPUs' architecture. When the selected GPUs
// have no single architecture with a folder, the variable is dropped (vLLM
// uses its built-in configs).
func hostTunedMoE(p *Plan, o PlanOptions) error {
	if p.Env["VLLM_TUNED_CONFIG_FOLDER"] != ImageTunedMoE {
		return nil
	}
	dir := ""
	if archs := gpu.Archs(o.GPUs, p.GPUs); len(archs) == 1 && o.TunedMoE != nil {
		d, err := o.TunedMoE(archs[0])
		if err != nil {
			return fmt.Errorf("tuned MoE configs for %s: %w", archs[0], err)
		}
		dir = d
	}
	if dir == "" {
		delete(p.Env, "VLLM_TUNED_CONFIG_FOLDER")
		return nil
	}
	p.Env["VLLM_TUNED_CONFIG_FOLDER"] = dir
	return nil
}
