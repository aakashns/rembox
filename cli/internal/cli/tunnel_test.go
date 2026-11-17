package cli

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestTunnel(t *testing.T) {
	for _, tc := range []struct {
		ports, forward string
	}{
		{"3000", "3000:localhost:3000"},
		{"3000:8080", "8080:localhost:3000"},
	} {
		ta := newTestApp(t, "0.1.0")
		ta.withServers(t, "a")
		ta.mustRun(t, "tunnel", "a", tc.ports)
		want := []string{"-p", "2222", "-i", "/k/a", "-o", "IdentitiesOnly=yes", "-o", "ServerAliveInterval=15",
			"-N", "-o", "ExitOnForwardFailure=yes", "-L", tc.forward, "--", "dev@a.example.com"}
		if !slices.Equal(ta.sshArgs, want) {
			t.Errorf("%s: ssh args %q, want %q", tc.ports, ta.sshArgs, want)
		}
	}
}

func TestTunnelErrors(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"tunnel"}, 2, "give the name of a server"},
		{[]string{"tunnel", "a"}, 2, "give the port to forward"},
		{[]string{"tunnel", "-x"}, 2, "unknown flag -x"},
		{[]string{"tunnel", "a", "3000", "4000"}, 2, `unexpected argument "4000"`},
		{[]string{"tunnel", "a", "web"}, 2, `"web" isn't a port`},
		{[]string{"tunnel", "a", "0"}, 2, `"0" isn't a port`},
		{[]string{"tunnel", "a", "3000:"}, 2, `"" isn't a port`},
		{[]string{"tunnel", "a", "3000:70000"}, 2, `"70000" isn't a port`},
		{[]string{"tunnel", "nope", "3000"}, 1, "there's no server named nope"},
	} {
		ta := newTestApp(t, "0.1.0")
		ta.withServers(t, "a")
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

func TestTunnelExitStatus(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withServers(t, "a")
	ta.sshErr = exec.Command("sh", "-c", "exit 255").Run()

	if code := ta.Run([]string{"tunnel", "a", "3000"}); code != 255 {
		t.Errorf("exit %d, want 255", code)
	}
}
