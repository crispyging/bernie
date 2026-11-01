package host

import (
	"bufio"
	"context"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Probe inspects the host under root ("/" in production). It is
// best-effort: a missing source (no /etc/os-release on macOS or in minimal
// containers, no /proc, no sysfs) leaves its fields empty and is not an
// error; the error result is always nil. User, container engines and
// /dev/shm describe the running system and are only probed when root is
// "/".
func Probe(ctx context.Context, root string) (Info, error) {
	if root == "" {
		root = "/"
	}
	var info Info
	info.OS, _ = ReadOSRelease(root)
	info.PkgManager = PkgManager(root, info.OS)
	info.Hostname = readString(filepath.Join(root, "proc/sys/kernel/hostname"))
	info.Kernel = readString(filepath.Join(root, "proc/sys/kernel/osrelease"))
	info.Cmdline = readString(filepath.Join(root, "proc/cmdline"))
	info.CPU = ParseCPUInfo(readString(filepath.Join(root, "proc/cpuinfo")))
	info.RAMBytes = ParseMemTotal(readString(filepath.Join(root, "proc/meminfo")))
	info.Virtualization = Virtualization(root, info.CPU)
	info.SELinux = SELinux(root)
	info.Engines = []Engine{}
	if root == "/" {
		if info.Hostname == "" {
			info.Hostname, _ = os.Hostname()
		}
		info.User = currentUser()
		info.Engines = ProbeEngines(ctx, defaultRunner)
		var st syscall.Statfs_t
		if syscall.Statfs("/dev/shm", &st) == nil {
			info.ShmBytes = uint64(st.Blocks) * uint64(st.Bsize)
		}
	}
	return info, nil
}

// ReadOSRelease parses /etc/os-release (or /usr/lib/os-release).
func ReadOSRelease(root string) (OSRelease, error) {
	var b []byte
	var err error
	for _, p := range []string{"etc/os-release", "usr/lib/os-release"} {
		if b, err = os.ReadFile(filepath.Join(root, p)); err == nil {
			break
		}
	}
	if err != nil {
		return OSRelease{}, err
	}
	kv := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || strings.HasPrefix(k, "#") {
			continue
		}
		if uq, err := strconv.Unquote(v); err == nil {
			v = uq
		} else {
			v = strings.Trim(v, `'"`)
		}
		kv[k] = v
	}
	return OSRelease{ID: kv["ID"], IDLike: kv["ID_LIKE"], VersionID: kv["VERSION_ID"], Pretty: kv["PRETTY_NAME"]}, nil
}

var pkgByDistro = map[string]string{
	"fedora": "dnf", "rhel": "dnf", "centos": "dnf", "rocky": "dnf", "almalinux": "dnf",
	"ol": "dnf", "amzn": "dnf", "nobara": "dnf",
	"debian": "apt", "ubuntu": "apt", "linuxmint": "apt", "pop": "apt", "elementary": "apt", "zorin": "apt",
	"arch": "pacman", "manjaro": "pacman", "endeavouros": "pacman", "cachyos": "pacman", "garuda": "pacman",
	"opensuse": "zypper", "opensuse-leap": "zypper", "opensuse-tumbleweed": "zypper", "sles": "zypper", "suse": "zypper",
}

// PkgManager derives the package manager from ID, then ID_LIKE, then the
// binaries present under root.
func PkgManager(root string, osr OSRelease) string {
	for _, id := range append([]string{osr.ID}, strings.Fields(osr.IDLike)...) {
		if pm, ok := pkgByDistro[id]; ok {
			return pm
		}
	}
	for _, c := range []struct{ bin, pm string }{{"dnf", "dnf"}, {"apt-get", "apt"}, {"pacman", "pacman"}, {"zypper", "zypper"}} {
		if _, err := os.Stat(filepath.Join(root, "usr/bin", c.bin)); err == nil {
			return c.pm
		}
	}
	return ""
}

// ParseCPUInfo summarizes /proc/cpuinfo.
func ParseCPUInfo(s string) CPU {
	var c CPU
	sockets := map[string]bool{}
	cores := map[string]bool{}
	coresPerSocket := 0
	phys := ""
	sc := bufio.NewScanner(strings.NewReader(s))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "processor":
			c.Threads++
			phys = ""
		case "model name":
			if c.Model == "" {
				c.Model = v
			}
		case "vendor_id":
			if c.Vendor == "" {
				c.Vendor = v
			}
		case "physical id":
			phys = v
			sockets[v] = true
		case "core id":
			cores[phys+"/"+v] = true
		case "cpu cores":
			if n, err := strconv.Atoi(v); err == nil {
				coresPerSocket = n
			}
		}
	}
	c.Sockets = len(sockets)
	if c.Sockets == 0 && c.Threads > 0 {
		c.Sockets = 1
	}
	c.Cores = len(cores)
	if c.Cores == 0 {
		c.Cores = coresPerSocket * c.Sockets
	}
	if c.Cores == 0 {
		c.Cores = c.Threads
	}
	return c
}

// ParseMemTotal returns MemTotal from /proc/meminfo in bytes.
func ParseMemTotal(s string) uint64 {
	for _, line := range strings.Split(s, "\n") {
		if f := strings.Fields(line); len(f) >= 2 && f[0] == "MemTotal:" {
			kb, _ := strconv.ParseUint(f[1], 10, 64)
			return kb * 1024
		}
	}
	return 0
}

// Virtualization names the container or hypervisor bernie runs in, with
// systemd-detect-virt names; "none" on bare metal.
func Virtualization(root string, cpu CPU) string {
	if exists(filepath.Join(root, ".dockerenv")) {
		return "docker"
	}
	if exists(filepath.Join(root, "run/.containerenv")) {
		return "podman"
	}
	vendor := readString(filepath.Join(root, "sys/class/dmi/id/sys_vendor"))
	product := readString(filepath.Join(root, "sys/class/dmi/id/product_name"))
	switch {
	case strings.Contains(product, "KVM") || strings.Contains(vendor, "QEMU") || strings.Contains(product, "QEMU"):
		return "kvm"
	case strings.Contains(vendor, "VMware"):
		return "vmware"
	case vendor == "Microsoft Corporation" && strings.Contains(product, "Virtual"):
		return "microsoft"
	case strings.Contains(vendor, "Xen") || readString(filepath.Join(root, "sys/hypervisor/type")) == "xen":
		return "xen"
	case strings.Contains(vendor, "innotek") || strings.Contains(product, "VirtualBox"):
		return "oracle"
	case strings.Contains(vendor, "Amazon EC2"):
		return "amazon"
	}
	if cpuHypervisor(root) {
		return "vm"
	}
	return "none"
}

func cpuHypervisor(root string) bool {
	for _, line := range strings.Split(readString(filepath.Join(root, "proc/cpuinfo")), "\n") {
		if strings.HasPrefix(line, "flags") {
			return strings.Contains(" "+line+" ", " hypervisor ")
		}
	}
	return false
}

// SELinux returns enforcing, permissive, disabled, or "" (no SELinux).
func SELinux(root string) string {
	switch readString(filepath.Join(root, "sys/fs/selinux/enforce")) {
	case "1":
		return "enforcing"
	case "0":
		return "permissive"
	}
	if exists(filepath.Join(root, "etc/selinux/config")) {
		return "disabled"
	}
	return ""
}

func currentUser() User {
	u := User{UID: os.Getuid()}
	if cu, err := user.Current(); err == nil {
		u.Name, u.Home = cu.Username, cu.HomeDir
	}
	if u.Home == "" {
		u.Home, _ = os.UserHomeDir()
	}
	gids, _ := os.Getgroups()
	gids = append([]int{os.Getgid()}, gids...)
	u.GIDs = gids
	seen := map[string]bool{}
	for _, gid := range gids {
		name := strconv.Itoa(gid)
		if g, err := user.LookupGroupId(name); err == nil {
			name = g.Name
		}
		if !seen[name] {
			seen[name] = true
			u.Groups = append(u.Groups, name)
		}
	}
	return u
}

// InGroup reports whether the user is a member of group.
func (u User) InGroup(group string) bool {
	for _, g := range u.Groups {
		if g == group {
			return true
		}
	}
	return false
}

func readString(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// FreeBytes is the space free to the user on the file system of path. A
// path that does not exist yet is measured at its nearest existing parent.
// 0 means unknown.
func FreeBytes(path string) uint64 {
	if path == "" {
		return 0
	}
	for p := filepath.Clean(path); ; p = filepath.Dir(p) {
		var st syscall.Statfs_t
		if syscall.Statfs(p, &st) == nil {
			return uint64(st.Bavail) * uint64(st.Bsize)
		}
		if p == filepath.Dir(p) {
			return 0
		}
	}
}
