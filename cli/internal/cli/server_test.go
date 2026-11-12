package cli

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// exitWith is what running a command that exits with code returns.
func exitWith(code int) error {
	return exec.Command("sh", "-c", fmt.Sprintf("exit %d", code)).Run()
}

func (ta *testApp) mustFail(t *testing.T, want string, args ...string) {
	t.Helper()
	if code := ta.Run(args); code != 1 {
		t.Fatalf("rembox %v: exit %d, want 1\nstdout:\n%s\nstderr:\n%s", args, code, ta.stdout, ta.stderr)
	}
	if !strings.Contains(ta.stderr.String(), want) {
		t.Errorf("rembox %v: stderr %q doesn't mention %q", args, ta.stderr, want)
	}
}

func (ta *testApp) wantOutput(t *testing.T, wants ...string) {
	t.Helper()
	// However the lines are wrapped.
	words := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	for _, want := range wants {
		if !strings.Contains(words(ta.stdout.String()), words(want)) {
			t.Errorf("output missing %q:\n%s", want, ta.stdout)
		}
	}
}

func TestInfo(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withServers(t, "a")
	ta.probeOut = facts("swap_kb=4194304", "virt=none", "reboot_required=yes")
	ta.mustRun(t, "info", "a")
	ta.wantOutput(t, `dev@a.example.com bash -c "$(cat probe.sh)" bash`, "System        Ubuntu 26.04.1 LTS",
		"3.7 GiB, and 4.0 GiB of swap", "Runs on       its own hardware", "needed, to finish installing updates",
		"rembox can set up this server.")

	// A server rembox can't set up is still info.
	ta.stdout.Reset()
	ta.probeOut = facts("uid=1000", "user=dev", "sudo=no")
	ta.mustRun(t, "info", "a")
	ta.wantOutput(t, "rembox logged in as dev, who isn't root, and the server has no sudo")
}

func TestInfoErrors(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"info"}, 2, "give the name of a server"},
		{[]string{"info", "nope"}, 1, "there's no server named nope"},
		{[]string{"info", "a", "b"}, 2, `unexpected argument "b"`},
		{[]string{"info", "-x"}, 2, "unknown flag -x"},
	} {
		ta := newTestApp(t, "0.1.0")
		ta.withServers(t, "a")
		if code := ta.Run(tc.args); code != tc.code {
			t.Errorf("%v: exit %d, want %d", tc.args, code, tc.code)
		}
		if !strings.Contains(ta.stderr.String(), tc.want) {
			t.Errorf("%v: stderr %q doesn't mention %q", tc.args, ta.stderr, tc.want)
		}
	}

	ta := newTestApp(t, "0.1.0")
	ta.withServers(t, "a")
	ta.probeOut = "fact os_id=ubuntu\n"
	ta.mustFail(t, "couldn't read what probe.sh found: the server didn't report its os_version", "info", "a")
}

func TestWrap(t *testing.T) {
	text := strings.Repeat("word ", 40)
	for line := range strings.Lines(wrap(text, "    ")) {
		line = strings.TrimSuffix(line, "\n")
		if len(line) > 76 || !strings.HasPrefix(line, "    word") {
			t.Errorf("line %q", line)
		}
	}
	if got := wrap("", "  "); got != "  " {
		t.Errorf("wrap of nothing: %q", got)
	}
}
