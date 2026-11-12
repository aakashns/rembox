package remote

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

const goodFacts = `Welcome to the server!
fact os_id=ubuntu
fact os_version=26.04
fact os_name=Ubuntu 26.04.1 LTS
fact arch=arm64
fact kernel=7.0.0-10-generic
fact hostname=box
fact virt=kvm
fact cpus=2
fact memory_kb=3905536
fact swap_kb=0
fact disk_kb=39634756
fact disk_free_kb=36870728
fact user=root
fact uid=0
fact sudo=yes
fact apt=yes
fact systemd=yes
fact reboot_required=no
`

func TestParse(t *testing.T) {
	m, err := Parse(strings.ReplaceAll(goodFacts, "\n", "\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := Machine{OSID: "ubuntu", OSVersion: "26.04", OSName: "Ubuntu 26.04.1 LTS", Arch: "arm64",
		Kernel: "7.0.0-10-generic", Hostname: "box", Virt: "kvm", CPUs: 2, MemoryKB: 3905536,
		DiskKB: 39634756, DiskFreeKB: 36870728, User: "root", Sudo: true, APT: true, Systemd: true}
	if m != want {
		t.Errorf("got %+v\nwant %+v", m, want)
	}
	if p := m.Problems(); p != nil {
		t.Errorf("problems: %q", p)
	}
	m.UID, m.User = 1000, "dev" // who can use sudo
	if p := m.Problems(); p != nil {
		t.Errorf("problems: %q", p)
	}
}

func TestParseErrors(t *testing.T) {
	for in, want := range map[string]string{
		"": "didn't report its os_id, os_version",
		strings.Replace(goodFacts, "fact cpus=2\n", "", 1):                "didn't report its cpus",
		strings.Replace(goodFacts, "fact cpus=2\n", "fact cpus=two\n", 1): `cpus="two", which isn't a number`,
	} {
		if _, err := Parse(in); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Parse: %v, want %q", err, want)
		}
	}
}

func TestProblems(t *testing.T) {
	m, _ := Parse(goodFacts)
	m.OSID, m.OSName, m.APT, m.Systemd, m.Arch = "alpine", "", false, false, "riscv64"
	m.UID, m.User, m.Sudo = 1000, "dev", false
	got := strings.Join(m.Problems(), "\n")
	for _, want := range []string{"runs an unknown system", "no apt-get", "doesn't run systemd", "riscv64", "logged in as dev, who isn't root"} {
		if !strings.Contains(got, want) {
			t.Errorf("problems %q don't mention %q", got, want)
		}
	}
}

func TestScripts(t *testing.T) {
	for _, s := range []Script{Probe, Setup} {
		if !strings.HasPrefix(s.Body, "# lib.sh") || !strings.HasSuffix(s.Body, "main \"$@\" </dev/null\n") {
			t.Errorf("%s isn't lib.sh then a script ending with main", s.Name)
		}
		if !strings.HasSuffix(s.URL(), "/cli/internal/remote/"+s.Name) {
			t.Errorf("%s's URL: %s", s.Name, s.URL())
		}
	}
}

// TestProbeRuns runs probe.sh here, on Linux, as on a server.
func TestProbeRuns(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("probe.sh runs on Linux")
	}
	cmd := exec.Command("bash", "-c", Probe.Body, "bash") // as rembox runs it
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("probe.sh: %v\n%s", err, out)
	}
	m, err := Parse(string(out))
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if m.CPUs < 1 || m.MemoryKB <= 0 || m.DiskKB <= 0 || m.Kernel == "" {
		t.Errorf("unlikely facts: %+v", m)
	}
}
