package host

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/opengfx1030/bernie/internal/support/execx"
)

// The groups that own the GPU devices on a Linux host with AMD GPUs. The
// ids of these groups differ per host. A group name does not resolve
// inside a container image (the image has no /etc/group), so bernie uses
// the ids.
const (
	GroupVideo  = "video"
	GroupRender = "render"
)

// DeviceGroup is one numeric group id a container needs to open the GPU
// devices of the host, and where the id came from.
type DeviceGroup struct {
	Role   string `json:"role"` // video | render | kfd | card
	GID    int    `json:"gid"`
	Source string `json:"source"` // the device node, "getent group video" or "/etc/group"
}

// gpuDevice is one device node the GPUs need.
type gpuDevice struct {
	path string // relative to the root
	role string // the group name when the host does not name the gid
}

// GPUGroups returns the numeric group ids of the GPU devices of the host
// under root ("/" in production). The owner of the device nodes decides
// the ids; the group database is the fallback for the video and render
// groups. The result is sorted by id and holds no duplicate.
func GPUGroups(ctx context.Context, root string) []DeviceGroup {
	return gpuGroups(ctx, root, defaultRunner)
}

func gpuGroups(ctx context.Context, root string, r runner) []DeviceGroup {
	if root == "" {
		root = "/"
	}
	names := groupNames(root)
	var out []DeviceGroup
	seen := map[int]bool{}
	covered := map[string]bool{}
	for _, d := range listGPUDevices(root) {
		gid, ok := statGID(filepath.Join(root, d.path))
		if !ok {
			continue
		}
		role := names[gid]
		if role == "" {
			role = d.role
		}
		covered[role] = true
		if seen[gid] {
			continue
		}
		seen[gid] = true
		out = append(out, DeviceGroup{Role: role, GID: gid, Source: "/" + d.path})
	}
	for _, name := range []string{GroupVideo, GroupRender} {
		if covered[name] {
			continue
		}
		covered[name] = true
		gid, src, ok := lookupGroup(ctx, root, r, name)
		if !ok || seen[gid] {
			continue
		}
		seen[gid] = true
		out = append(out, DeviceGroup{Role: name, GID: gid, Source: src})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GID < out[j].GID })
	return out
}

// listGPUDevices is gpuDevices; a test of the root "/" replaces it, so the
// device nodes of the machine that runs the test do not count.
var listGPUDevices = gpuDevices

// gpuDevices are the device nodes a container opens for the GPUs.
func gpuDevices(root string) []gpuDevice {
	out := []gpuDevice{{path: "dev/kfd", role: "kfd"}}
	for _, g := range []struct{ glob, role string }{
		{"dev/dri/renderD*", GroupRender},
		{"dev/dri/card*", GroupVideo},
	} {
		m, _ := filepath.Glob(filepath.Join(root, g.glob))
		sort.Strings(m)
		for _, p := range m {
			rel, err := filepath.Rel(root, p)
			if err != nil {
				rel = p
			}
			out = append(out, gpuDevice{path: rel, role: g.role})
		}
	}
	return out
}

// statGID is the numeric group id of the owner of a path.
func statGID(path string) (int, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(st.Gid), true
}

// groupNames maps the group ids of the group database to their names.
func groupNames(root string) map[int]string {
	out := map[int]string{}
	for _, g := range readGroups(filepath.Join(root, "etc/group")) {
		out[g.gid] = g.name
	}
	return out
}

// lookupGroup is the numeric id of a group name: getent first (it knows the
// network groups too), then the group database of the root. getent answers
// for the running host, so it is only used for the root "/".
func lookupGroup(ctx context.Context, root string, r runner, name string) (int, string, bool) {
	if root == "/" {
		if gid, ok := getentGroup(ctx, r, name); ok {
			return gid, "getent group " + name, true
		}
	}
	for _, g := range readGroups(filepath.Join(root, "etc/group")) {
		if g.name == name {
			return g.gid, "/etc/group", true
		}
	}
	return 0, "", false
}

// getentGroup is the id of a group in the group database of the host.
func getentGroup(ctx context.Context, r runner, name string) (int, bool) {
	res := r.Run(ctx, "getent", "group", name)
	if !res.OK() {
		return 0, false
	}
	f := strings.Split(strings.TrimSpace(execx.FirstLine(res.Stdout)), ":")
	if len(f) < 3 {
		return 0, false
	}
	gid, err := strconv.Atoi(f[2])
	if err != nil {
		return 0, false
	}
	return gid, true
}

// group is one line of a group database.
type group struct {
	name string
	gid  int
}

// readGroups parses a group database (the lines of /etc/group).
func readGroups(path string) []group {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []group
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Split(strings.TrimSpace(line), ":")
		if len(f) < 3 || f[0] == "" || f[0][0] == '#' {
			continue
		}
		gid, err := strconv.Atoi(f[2])
		if err != nil {
			continue
		}
		out = append(out, group{name: f[0], gid: gid})
	}
	return out
}
