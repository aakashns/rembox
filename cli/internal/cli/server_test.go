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

// needsRestart fakes a server that needs a restart after its updates, whose
// probe reports facts(overrides...). The reboot returns rebootErr.
func (ta *testApp) needsRestart(rebootErr error, overrides ...string) {
	updated := false
	ta.onScript = func(script string, args []string, stdout io.Writer) error {
		if script == "update.sh" {
			updated = true
		} else if updated {
			fmt.Fprint(stdout, facts(append(overrides, "reboot_required=yes")...))
		} else {
			fmt.Fprint(stdout, facts(overrides...))
		}
		return nil
	}
	inner := ta.Command
	ta.Command = func(stdout io.Writer, name string, args ...string) error {
		err := inner(stdout, name, args...)
		if name == "ssh" && slices.Equal(args[len(args)-2:], []string{"systemctl", "reboot"}) {
			return rebootErr
		}
		return err
	}
}

func TestSetupDeclineUpdates(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.typing("box", "", "", "", "n")
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
	ta.typing("box", "", "", "")
	ta.mustFail(t, "can't set up box yet", "setup")
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
	ta.onScript = func(script string, args []string, stdout io.Writer) error {
		if script == "update.sh" {
			return exitWith(100)
		}
		fmt.Fprint(stdout, facts())
		return nil
	}
	ta.typing("box", "", "", "", "")
	ta.mustFail(t, "update.sh failed on box (exit status 100); the output above says why", "setup")
}

func TestSetupCantConnect(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.onScript = func(string, []string, io.Writer) error { return exitWith(255) }
	ta.typing("box", "", "", "")
	ta.mustFail(t, "ssh couldn't connect to root@box", "setup")
}

func TestSetupRestarts(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.needsRestart(exitWith(255)) // ssh loses the connection as the server shuts down
	ta.typing("box", "", "", "", "", "y")
	ta.mustRun(t, "setup")
	ta.wantOutput(t, "box needs to restart to finish the updates",
		"-- root@box systemctl reboot\nRun this command? [y/N]", "box is restarting", "box is set up")
}

func TestSetupRestartDeclined(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.needsRestart(nil)
	ta.typing("box", "", "", "", "", "n")
	ta.mustRun(t, "setup")
	ta.wantOutput(t, "Run this command? [y/N]", "Not restarted. To restart box later: rembox ssh box systemctl reboot")
	if strings.Contains(ta.stdout.String(), "box is restarting") {
		t.Error("restarted after being told not to")
	}
}

func TestSetupWithSudo(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.probeOut = facts("user=dev", "uid=1000")
	ta.Color = true
	ta.typing("box", "dev", "", "", "")
	ta.mustRun(t, "setup")
	ta.wantOutput(t, "rembox logged in as dev, so it runs the script with sudo, which may ask for dev's password",
		`-- dev@box sudo bash -c "$(cat update.sh)" bash --color`)

	// The script goes in the command, quoted, and ssh runs it at a terminal,
	// so sudo can ask for the password.
	update := ta.commands[slices.IndexFunc(ta.commands, func(c []string) bool { return scriptIn(c) == "update.sh" })]
	i := slices.Index(update, "-c")
	if i < 0 || update[1] != "-t" || update[i-2] != "sudo" ||
		update[i+1] != shellQuote(remote.Update.Body) || !slices.Equal(update[i+2:], []string{"bash", "--color"}) {
		t.Errorf("ssh args %q", update)
	}
	// The probe needs no root.
	for _, c := range ta.commands {
		if slices.Contains(c, shellQuote(remote.Probe.Body)) && slices.Contains(c, "sudo") {
			t.Errorf("probed with sudo: %q", c)
		}
	}
}

func TestSetupRestartsWithSudo(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.needsRestart(nil, "user=dev", "uid=1000")
	ta.typing("box", "dev", "", "", "", "n")
	ta.mustRun(t, "setup")
	ta.wantOutput(t, "-- dev@box sudo systemctl reboot\n", "rembox ssh box sudo systemctl reboot")
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
