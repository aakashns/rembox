package cli

import (
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/aakashns/rembox/cli/internal/remote"
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

// restartingServer fakes a server that needs a restart after its updates. It
// comes back with a new boot ID after down probes of it fail.
func (ta *testApp) restartingServer(down int, rebootErr error) {
	updated, rebooted := false, false
	ta.onPipe = func(script string, args []string, stdout io.Writer) error {
		switch {
		case script == "update.sh":
			updated = true
		case script == "probe.sh" && rebooted:
			fmt.Fprint(stdout, facts("boot_id=boot-2"))
		case script == "probe.sh" && updated:
			fmt.Fprint(stdout, facts("reboot_required=yes"))
		case script == "probe.sh":
			fmt.Fprint(stdout, facts())
		case args[len(args)-1] == "/proc/sys/kernel/random/boot_id":
			if down > 0 {
				down--
				return exitWith(255)
			}
			fmt.Fprintln(stdout, "boot-2")
		default:
			return fmt.Errorf("the tests don't fake ssh %q", args)
		}
		return nil
	}
	ta.sshErr = nil
	reboot := func(args []string) bool { return slices.Equal(args[len(args)-2:], []string{"systemctl", "reboot"}) }
	// Command runs the ssh for the login check, the reboot, and the updates
	// with sudo.
	inner := ta.Command
	ta.Command = func(name string, args ...string) error {
		if slices.Contains(args, shellQuote(remote.Update.Body)) {
			updated = true
		}
		if name == "ssh" && reboot(args) {
			rebooted = true
			ta.commands = append(ta.commands, append([]string{name}, args...))
			return rebootErr
		}
		return inner(name, args...)
	}
}

func TestSetupDeclineUpdates(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.Interactive = true
	ta.Stdin = strings.NewReader("box\n\n\n\nn\n")
	ta.mustRun(t, "setup")
	ta.wantOutput(t, "Skipped installing updates. To install them later, run rembox setup again.")
	if !slices.Equal(ta.scripts, []string{"probe.sh"}) {
		t.Errorf("ran %q", ta.scripts)
	}
}

func TestSetupUnsupported(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.probeOut = facts("os_id=fedora", "os_name=Fedora Linux 44", "apt=no", "arch=riscv64", "user=dev", "uid=1000", "sudo=no")
	ta.mustFail(t, "can't set up box yet", "setup", "--host", "box", "--yes")
	ta.wantOutput(t, "this one runs Fedora Linux 44", "no apt-get", "Its processor is riscv64",
		"rembox logged in as dev")
	if !slices.Equal(ta.scripts, []string{"probe.sh"}) {
		t.Errorf("ran %q", ta.scripts)
	}
	if len(ta.servers(t)) != 1 {
		t.Error("didn't save the server it logged in to")
	}
}

func TestSetupUpdateFails(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.onPipe = func(script string, args []string, stdout io.Writer) error {
		if script == "update.sh" {
			return exitWith(100)
		}
		fmt.Fprint(stdout, facts())
		return nil
	}
	ta.mustFail(t, "update.sh failed on box (exit status 100); the output above says why", "setup", "--host", "box", "--yes")
}

func TestSetupCantConnect(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.onPipe = func(string, []string, io.Writer) error { return exitWith(255) }
	ta.mustFail(t, "ssh couldn't connect to root@box", "setup", "--host", "box", "--yes")
}

func TestSetupRestarts(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.restartingServer(2, exitWith(255)) // ssh loses the connection as the server shuts down
	ta.mustRun(t, "setup", "--host", "box", "--yes")
	ta.wantOutput(t, "box needs to restart to finish the updates",
		"root@box systemctl reboot\n", "Waiting for box to start again",
		"root@box cat /proc/sys/kernel/random/boot_id\n", "box has restarted", "box is set up")
	if strings.Contains(ta.stdout.String(), "Run this command?") {
		t.Error("asked, despite --yes")
	}
	if ta.sleeps != 3 {
		t.Errorf("checked %d times, want 3", ta.sleeps)
	}
}

func TestSetupRestartTimesOut(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.restartingServer(1000, nil)
	ta.mustFail(t, "box didn't start again within 5m0s", "setup", "--host", "box", "--yes")
	if ta.sleeps != 60 {
		t.Errorf("checked %d times, want 60", ta.sleeps)
	}
}

func TestSetupRestartDeclined(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.restartingServer(0, nil)
	ta.Interactive = true
	ta.Stdin = strings.NewReader("box\n\n\n\ny\nn\n")
	ta.mustRun(t, "setup")
	ta.wantOutput(t, "Run this command? [y/N]", "Not restarted. To restart box later: rembox ssh box systemctl reboot")
	if ta.sleeps != 0 {
		t.Error("waited for a restart that didn't happen")
	}
}

func TestSetupWithSudo(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.probeOut = facts("user=dev", "uid=1000")
	ta.Interactive, ta.Color = true, true
	ta.Stdin = strings.NewReader("box\ndev\n\n\n\n")
	ta.mustRun(t, "setup")
	ta.wantOutput(t, "rembox logged in as dev, so it runs the script with sudo, which may ask for dev's password",
		`-t -- dev@box sudo bash -c "$(cat update.sh)" bash --color`)

	// The script goes in the command, quoted, so sudo can ask for the
	// password at the terminal.
	i := slices.Index(ta.sshArgs, "-c")
	if i < 0 || !slices.Contains(ta.sshArgs, "-t") || ta.sshArgs[i-2] != "sudo" ||
		ta.sshArgs[i+1] != shellQuote(remote.Update.Body) || !slices.Equal(ta.sshArgs[i+2:], []string{"bash", "--color"}) {
		t.Errorf("ssh args %q", ta.sshArgs)
	}
	// The probe needs no root.
	if !slices.Equal(ta.scripts, []string{"probe.sh", "probe.sh"}) {
		t.Errorf("sent %q", ta.scripts)
	}

	// Without a terminal, sudo can't ask.
	ta.Interactive = false
	ta.mustRun(t, "setup", "--host", "box", "--user", "dev", "--yes")
	if i := slices.Index(ta.sshArgs, "sudo"); i < 0 || ta.sshArgs[i+1] != "-n" || slices.Contains(ta.sshArgs, "-t") {
		t.Errorf("ssh args %q", ta.sshArgs)
	}
}

func TestSetupRestartsWithSudo(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.restartingServer(0, nil)
	inner := ta.onPipe
	ta.onPipe = func(script string, args []string, stdout io.Writer) error {
		if script == "probe.sh" {
			var b strings.Builder
			inner(script, args, &b)
			fmt.Fprint(stdout, strings.Replace(b.String(), "fact uid=0", "fact uid=1000", 1))
			return nil
		}
		return inner(script, args, stdout)
	}
	ta.mustRun(t, "setup", "--host", "box", "--yes")
	ta.wantOutput(t, "-- root@box sudo -n systemctl reboot\n", "box has restarted")
}

func TestInfo(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withServers(t, "a")
	ta.probeOut = facts("swap_kb=4194304", "virt=none", "reboot_required=yes")
	ta.mustRun(t, "info", "a")
	ta.wantOutput(t, "dev@a.example.com bash -s < probe.sh\n", "System        Ubuntu 26.04.1 LTS",
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
