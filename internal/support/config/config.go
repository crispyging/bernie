// Package config loads and saves the user defaults file <config>/config.yaml
// (spec §12): venv, mode, engine, hf_home and default --set overrides.
package config

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/opengfx1030/bernie/internal/support/paths"
)

// Config is the content of config.yaml.
type Config struct {
	Venv   string `json:"venv" yaml:"venv,omitempty"`
	Mode   string `json:"mode" yaml:"mode,omitempty"`     // host | container | "" (auto)
	Engine string `json:"engine" yaml:"engine,omitempty"` // docker | podman | "" (auto)
	// AutoYes, when true, makes `bernie up` skip the one-shot
	// confirmation prompt and run. The doctor still runs and any
	// step that needs root still blocks the flow with its commands.
	// Empty / false preserves the default (prompt).
	AutoYes bool `json:"auto_yes" yaml:"auto_yes,omitempty"`
	// DeviceAccess is how a container gets the GPU: classic | cdi |
	// auto ("" = auto, spec §8.2).
	DeviceAccess string `json:"device_access" yaml:"device_access,omitempty"`
	HFHome       string `json:"hf_home" yaml:"hf_home,omitempty"`
	// DownloadLimit is the bytes-per-second cap of a model download
	// (R21): "" or 0 means no cap. Live changes take effect within a
	// second; the value here is read fresh each time the downloader
	// asks for it.
	DownloadLimit int64    `json:"download_limit" yaml:"download_limit,omitempty"`
	Set           []string `json:"set" yaml:"set,omitempty"` // applied before --set
	// Fleet is the list of other bernie hosts the Fleet tab watches.
	// The Token stays in the file (mode 0600); it never reaches the browser
	// (spec §11). Name is unique within the list; URL is http or https.
	Fleet []FleetEntry `json:"fleet,omitempty" yaml:"fleet,omitempty"`
}

// Keys are the settable keys, in file order.
var Keys = []string{"venv", "mode", "engine", "auto_yes", "device_access", "hf_home", "download_limit", "set", "fleet"}

// Path is <config>/config.yaml.
func Path() string { return filepath.Join(paths.Config(), "config.yaml") }

// Load reads config.yaml. A missing file is an empty Config, not an error.
func Load() (Config, error) { return LoadFile(Path()) }

// LoadFile reads one config file. A missing file is an empty Config.
func LoadFile(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	return c, c.validate()
}

func (c Config) validate() error {
	switch c.Mode {
	case "", "host", "container":
	default:
		return fmt.Errorf("config mode must be host or container, not %q", c.Mode)
	}
	switch c.Engine {
	case "", "docker", "podman":
	default:
		return fmt.Errorf("config engine must be docker or podman, not %q", c.Engine)
	}
	switch c.DeviceAccess {
	case "", "classic", "cdi", "auto":
	default:
		return fmt.Errorf("config device_access must be classic, cdi or auto, not %q", c.DeviceAccess)
	}
	if c.DownloadLimit < 0 {
		return fmt.Errorf("config download_limit must not be negative, got %d", c.DownloadLimit)
	}
	for _, s := range c.Set {
		if k, _, ok := strings.Cut(s, "="); !ok || k == "" {
			return fmt.Errorf("config set entry %q: want KEY=VALUE", s)
		}
	}
	seen := map[string]bool{}
	for i, e := range c.Fleet {
		if err := ValidateFleetEntry(e); err != nil {
			return fmt.Errorf("config fleet[%d]: %w", i, err)
		}
		if seen[e.Name] {
			return fmt.Errorf("config fleet[%d]: duplicate name %q", i, e.Name)
		}
		seen[e.Name] = true
	}
	return nil
}

// Save writes config.yaml, creating the config directory. The file is 0600
// when the config carries a fleet token, 0644 otherwise: the token is a
// secret and the file must be readable by its owner only.
func Save(c Config) error { return SaveFile(Path(), c) }

// SaveFile writes one config file. See Save for the permissions.
func SaveFile(path string, c Config) error {
	if err := c.validate(); err != nil {
		return err
	}
	if _, err := paths.Ensure(filepath.Dir(path)); err != nil {
		return err
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if c.hasSecret() {
		mode = 0o600
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// SetKey changes one key. For "set", VALUE is one KEY=VALUE override: it
// replaces an entry with the same KEY, and "KEY=" with an empty value after
// "-" ("-KEY") removes it. An empty VALUE clears scalar keys and the list.
//
// For "fleet" VALUE is one fleet command: "add NAME URL" or "rm NAME". The
// token, when one is given, comes through "fleet add NAME URL TOKEN" or is
// already on the entry (the CLI prompts for it).
func (c *Config) SetKey(key, value string) error {
	switch key {
	case "venv":
		c.Venv = value
	case "mode":
		c.Mode = value
	case "engine":
		c.Engine = value
	case "auto_yes":
		v := strings.ToLower(strings.TrimSpace(value))
		switch v {
		case "", "false", "no", "0":
			c.AutoYes = false
		case "true", "yes", "1":
			c.AutoYes = true
		default:
			return fmt.Errorf("config auto_yes: want true or false, got %q", value)
		}
	case "device_access":
		c.DeviceAccess = value
	case "hf_home":
		c.HFHome = value
	case "download_limit":
		n, err := ParseBytesPerSecond(value)
		if err != nil {
			return fmt.Errorf("config download_limit: %w", err)
		}
		c.DownloadLimit = n
	case "set":
		c.Set = setOverride(c.Set, value)
	case "fleet":
		op, name, url, token, err := parseFleetOp(value)
		if err != nil {
			return err
		}
		switch op {
		case "add":
			c.UpsertFleet(FleetEntry{Name: name, URL: url, Token: token})
		case "rm":
			c.RemoveFleet(name)
		default:
			return fmt.Errorf("config fleet: unknown op %q (have add, rm)", op)
		}
	default:
		return fmt.Errorf("unknown config key %q (keys: %s)", key, strings.Join(Keys, ", "))
	}
	return c.validate()
}

func setOverride(list []string, value string) []string {
	if value == "" {
		return nil
	}
	if strings.HasPrefix(value, "-") {
		k := strings.TrimPrefix(value, "-")
		out := list[:0:0]
		for _, s := range list {
			if sk, _, _ := strings.Cut(s, "="); sk != k {
				out = append(out, s)
			}
		}
		return out
	}
	k, _, _ := strings.Cut(value, "=")
	out := make([]string, 0, len(list)+1)
	replaced := false
	for _, s := range list {
		if sk, _, _ := strings.Cut(s, "="); sk == k {
			if !replaced {
				out = append(out, value)
				replaced = true
			}
			continue
		}
		out = append(out, s)
	}
	if !replaced {
		out = append(out, value)
	}
	return out
}

// ResolveVenv applies the venv order of spec §6.2 and returns the selected
// path. "" when none is set or when conventional candidates are ambiguous.
func (c Config) ResolveVenv(flag string) string {
	return c.ResolveVenvInfo(flag).Path
}

// ParseBytesPerSecond reads a size string of the form "5MB", "500KB",
// "1GB" (decimal units, case-insensitive) or a plain integer of bytes.
// Empty input is 0 (no limit). Whitespace is trimmed.
func ParseBytesPerSecond(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	// A trailing "B" is optional; "MB", "M" both work the same.
	if len(value) > 1 && (value[len(value)-1] == 'B' || value[len(value)-1] == 'b') {
		value = value[:len(value)-1]
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("want a size like 5MB or 0")
	}
	// Find the split between digits and the unit suffix.
	i := len(value)
	for i > 0 && (value[i-1] < '0' || value[i-1] > '9') {
		i--
	}
	if i == 0 || i == len(value) {
		return 0, fmt.Errorf("want a size like 5MB or 0")
	}
	num, err := strconv.ParseInt(value[:i], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a number of bytes: %w", value[:i], err)
	}
	if num < 0 {
		return 0, fmt.Errorf("must not be negative")
	}
	if i == len(value) {
		return num, nil
	}
	mult := int64(1)
	switch strings.ToUpper(value[i:]) {
	case "", "B":
	case "K", "KB":
		mult = 1000
	case "M", "MB":
		mult = 1000 * 1000
	case "G", "GB":
		mult = 1000 * 1000 * 1000
	default:
		return 0, fmt.Errorf("unknown unit %q (use B, K, KB, M, MB, G, GB)", value[i:])
	}
	return num * mult, nil
}

// ResolveHFHome is config hf_home, then $HF_HOME, then ~/.cache/huggingface.
func (c Config) ResolveHFHome() string {
	if c.HFHome != "" {
		return expandHome(c.HFHome)
	}
	if v := os.Getenv("HF_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "huggingface")
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// Map returns the config as ordered key/value pairs for display. The set
// list keeps its order: later entries win. The fleet row lists the names of
// the configured peers — never the URL and never the token.
func (c Config) Map() [][2]string {
	var limit string
	if c.DownloadLimit > 0 {
		limit = strconv.FormatInt(c.DownloadLimit, 10)
	}
	names := make([]string, len(c.Fleet))
	for i, e := range c.Fleet {
		names[i] = e.Name
	}
	return [][2]string{
		{"venv", c.Venv}, {"mode", c.Mode}, {"engine", c.Engine},
		{"auto_yes", strconv.FormatBool(c.AutoYes)},
		{"device_access", c.DeviceAccess},
		{"hf_home", c.HFHome}, {"download_limit", limit},
		{"set", strings.Join(c.Set, " ")},
		{"fleet", strings.Join(names, " ")},
	}
}
