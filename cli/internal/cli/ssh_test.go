package cli

import (
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/aakashns/rembox/cli/internal/config"
)

func (ta *testApp) withServers(t *testing.T, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := config.SaveServer(ta.configDir, config.Server{Name: n, Host: n + ".example.com", User: "dev", Key: "/k/" + n}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSSH(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withServers(t, "a", "b")

	ta.mustRun(t, "ssh", "b")
	want := []string{"-i", "/k/b", "-o", "IdentitiesOnly=yes", "--", "dev@b.example.com"}
	if !slices.Equal(ta.sshArgs, want) {
		t.Errorf("ssh args %q, want %q", ta.sshArgs, want)
	}
	if !strings.Contains(ta.stdout.String(), "\n$ ssh -i /k/b -o IdentitiesOnly=yes -- dev@b.example.com\n") {
		t.Errorf("output doesn't show the ssh command:\n%s", ta.stdout)
	}

	// A command is passed on, flags and all.
	ta.mustRun(t, "ssh", "a", "ls", "-la")
	if want := []string{"--", "dev@a.example.com", "ls", "-la"}; !slices.Equal(ta.sshArgs[len(ta.sshArgs)-4:], want) {
		t.Errorf("ssh args %q", ta.sshArgs)
	}
}

func TestSSHErrors(t *testing.T) {
	for _, tc := range []struct {
		servers []string
		args    []string
		code    int
		want    string
	}{
		{[]string{"a"}, []string{"ssh"}, 2, "give the name of a server"},
		{[]string{"a"}, []string{"ssh", "nope"}, 1, "there's no server named nope"},
		{[]string{"a"}, []string{"ssh", "Bad!"}, 1, "can't be a name"},
		{[]string{"a"}, []string{"ssh", "-x"}, 2, "unknown flag -x"},
	} {
		ta := newTestApp(t, "0.1.0")
		ta.withServers(t, tc.servers...)
		if code := ta.Run(tc.args); code != tc.code {
			t.Errorf("%v: exit %d, want %d", tc.args, code, tc.code)
		}
		if !strings.Contains(ta.stderr.String(), tc.want) {
			t.Errorf("%v: stderr %q doesn't mention %q", tc.args, ta.stderr, tc.want)
		}
		if ta.sshArgs != nil {
			t.Errorf("%v: ran ssh", tc.args)
		}
	}
}

func TestSSHExitStatus(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withServers(t, "a")
	ta.sshErr = exec.Command("sh", "-c", "exit 3").Run()

	if code := ta.Run([]string{"ssh", "a", "false"}); code != 3 {
		t.Errorf("exit %d, want 3", code)
	}
	if ta.stderr.Len() != 0 {
		t.Errorf("stderr: %s", ta.stderr)
	}
}

func TestColor(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withServers(t, "a")
	ta.Color, ta.ErrColor = true, true
	ta.mustRun(t, "ssh", "a")
	if !strings.Contains(ta.stdout.String(), "\n\x1b[36m$ ssh ") {
		t.Errorf("command isn't in color: %q", ta.stdout)
	}
	ta.Run([]string{"ssh", "nope"})
	if !strings.HasPrefix(ta.stderr.String(), "\x1b[31mrembox: ") {
		t.Errorf("error isn't in color: %q", ta.stderr)
	}
}
