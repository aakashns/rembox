// Package remote holds the scripts rembox runs on servers, and reads what
// they report.
//
// rembox sends a script to `bash -s` over ssh, so nothing is copied to the
// server. Each script is lib.sh followed by the script itself.
package remote

import (
	"embed"
	"fmt"
	"strconv"
	"strings"

	"github.com/aakashns/rembox/cli/internal/version"
)

//go:embed *.sh
var files embed.FS

// Script is a script rembox runs on servers.
type Script struct {
	Name string // its file, like probe.sh
	Body string // lib.sh, then the script
}

var (
	// Probe reports what kind of machine a server is, as facts for Parse. It
	// changes nothing.
	Probe = load("probe.sh")
	// Update installs the latest version of every package with apt.
	Update = load("update.sh")
)

func load(name string) Script {
	lib, err := files.ReadFile("lib.sh")
	if err != nil {
		panic(err)
	}
	body, err := files.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return Script{Name: name, Body: string(lib) + "\n" + string(body)}
}

// URL is where to read the script, as this version of rembox has it.
func (s Script) URL() string {
	ref := "main" // development builds
	if version.Valid(version.Version) && !strings.Contains(version.Version, "-") {
		ref = "v" + version.Version
	}
	return "https://github.com/aakashns/rembox/blob/" + ref + "/cli/internal/remote/" + s.Name
}

// Machine is what Probe reports about a server.
type Machine struct {
	OSID, OSVersion, OSName string // from /etc/os-release, like "ubuntu", "26.04", "Ubuntu 26.04.1 LTS"
	Arch                    string // as Debian names it, like arm64 or amd64
	Kernel, Hostname        string
	Virt                    string // from systemd-detect-virt, like kvm, or none on a physical machine
	CPUs                    int
	MemoryKB, SwapKB        int64
	DiskKB, DiskFreeKB      int64 // of the filesystem /
	User                    string
	UID                     int
	Sudo                    bool // the server has sudo, to become root with
	APT, Systemd            bool
	RebootRequired          bool   // an update needs a restart to take effect
	BootID                  string // changes every time the server starts
}

// Parse reads Probe's output: "fact NAME=VALUE" lines. Other lines (like any
// the server's shell prints when it starts) are ignored.
func Parse(out string) (Machine, error) {
	facts := map[string]string{}
	for line := range strings.Lines(out) {
		if kv, ok := strings.CutPrefix(strings.TrimRight(line, "\r\n"), "fact "); ok {
			if k, v, ok := strings.Cut(kv, "="); ok {
				facts[k] = v
			}
		}
	}
	var m Machine
	var missing []string
	str := func(name string) string {
		v, ok := facts[name]
		if !ok {
			missing = append(missing, name)
		}
		return v
	}
	var bad error
	num := func(name string) int64 {
		v := str(name)
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil && bad == nil && v != "" {
			bad = fmt.Errorf("the server reported %s=%q, which isn't a number", name, v)
		}
		return n
	}
	yes := func(name string) bool { return str(name) == "yes" }

	m.OSID, m.OSVersion, m.OSName = str("os_id"), str("os_version"), str("os_name")
	m.Arch, m.Kernel, m.Hostname, m.Virt = str("arch"), str("kernel"), str("hostname"), str("virt")
	m.CPUs = int(num("cpus"))
	m.MemoryKB, m.SwapKB = num("memory_kb"), num("swap_kb")
	m.DiskKB, m.DiskFreeKB = num("disk_kb"), num("disk_free_kb")
	m.User, m.UID = str("user"), int(num("uid"))
	m.Sudo, m.APT, m.Systemd, m.RebootRequired = yes("sudo"), yes("apt"), yes("systemd"), yes("reboot_required")
	m.BootID = str("boot_id")
	if len(missing) > 0 {
		return m, fmt.Errorf("the server didn't report its %s", strings.Join(missing, ", "))
	}
	return m, bad
}

// Problems are the reasons rembox can't set up the machine, if any.
func (m Machine) Problems() []string {
	var p []string
	if m.OSID != "ubuntu" && m.OSID != "debian" {
		name := m.OSName
		if name == "" {
			name = "an unknown system"
		}
		p = append(p, fmt.Sprintf("rembox sets up Ubuntu and Debian servers, and this one runs %s.", name))
	}
	if !m.APT {
		p = append(p, "It has no apt-get, which rembox installs packages with.")
	}
	if !m.Systemd {
		p = append(p, "It doesn't run systemd, which rembox runs services with.")
	}
	if m.Arch != "arm64" && m.Arch != "amd64" {
		p = append(p, fmt.Sprintf("Its processor is %s; rembox supports arm64 and amd64.", m.Arch))
	}
	if m.UID != 0 && !m.Sudo {
		p = append(p, fmt.Sprintf("rembox logged in as %s, who isn't root, and the server has no sudo "+
			"to become root with. Run rembox setup again with root as the user.", m.User))
	}
	return p
}
