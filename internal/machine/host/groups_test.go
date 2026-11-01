package host

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/opengfx1030/bernie/internal/support/execx"
)

// devBoxRoot is a root tree with the group database of par1-cs25.
func devBoxRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	b, err := os.ReadFile("testdata/groups-par1-cs25.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "etc/group"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// The dev box has video 39 and render 105: the names and the ids differ
// from the Debian defaults, and the ids come from the group database when
// no device node is readable.
func TestGPUGroupsFromDatabase(t *testing.T) {
	root := devBoxRoot(t)
	got := gpuGroups(context.Background(), root, fakeRunner{})
	want := []DeviceGroup{
		{Role: "video", GID: 39, Source: "/etc/group"},
		{Role: "render", GID: 105, Source: "/etc/group"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

// getent knows the groups of the running host.
func TestGPUGroupsFromGetent(t *testing.T) {
	root := devBoxRoot(t)
	r := fakeRunner{out: map[string]execx.Result{
		"getent group video":  {Stdout: "video:x:44:\n"},
		"getent group render": {Stdout: "render:x:109:\n"},
	}}
	// The root is not "/", so getent is not asked: it answers for the
	// running host, not for a tree.
	got := gpuGroups(context.Background(), root, r)
	if got[0].GID != 39 || got[0].Source != "/etc/group" {
		t.Errorf("getent must not answer for a fixture root: %+v", got)
	}
	// The machine that runs the test may have GPU device nodes of its own;
	// here it has none, so getent decides.
	old := listGPUDevices
	t.Cleanup(func() { listGPUDevices = old })
	listGPUDevices = func(string) []gpuDevice { return nil }
	got = gpuGroups(context.Background(), "/", r)
	want := []DeviceGroup{
		{Role: "video", GID: 44, Source: "getent group video"},
		{Role: "render", GID: 109, Source: "getent group render"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

// The owner of a device node decides the id: the group database is only
// the fallback for the groups no device node covers.
func TestGPUGroupsFromDeviceNodes(t *testing.T) {
	root := devBoxRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "dev/dri"), 0o755); err != nil {
		t.Fatal(err)
	}
	kfd := filepath.Join(root, "dev/kfd")
	if err := os.WriteFile(kfd, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	gid, ok := statGID(kfd)
	if !ok {
		t.Skip("no group id for the test file")
	}
	got := gpuGroups(context.Background(), root, fakeRunner{})
	if len(got) != 3 {
		t.Fatalf("got %+v", got)
	}
	// The result is sorted by id, and the id of the test file is the group
	// of whoever runs the test: find the entries by their source.
	bySource := map[string]DeviceGroup{}
	for _, g := range got {
		bySource[g.Source] = g
	}
	if g := bySource["/dev/kfd"]; g.GID != gid || g.Role != "kfd" {
		t.Errorf("device node group = %+v (all: %+v)", g, got)
	}
	if bySource["/etc/group"].GID == 0 {
		t.Errorf("no fallback group: %+v", got)
	}
	var ids []int
	for _, g := range got {
		if g.Source == "/etc/group" {
			ids = append(ids, g.GID)
		}
	}
	if !reflect.DeepEqual(ids, []int{39, 105}) {
		t.Errorf("fallback groups = %+v", got)
	}
}

// A host without the video and render groups gets no id for them.
func TestGPUGroupsWithoutGroups(t *testing.T) {
	root := t.TempDir()
	if got := gpuGroups(context.Background(), root, fakeRunner{}); len(got) != 0 {
		t.Errorf("got %+v, want none", got)
	}
}
