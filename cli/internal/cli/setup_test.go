package cli

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/aakashns/rembox/cli/internal/config"
	"github.com/aakashns/rembox/cli/internal/remote"
)

// withKey puts a private key (and its public key) at ~/.ssh/name and returns
// its path.
func (ta *testApp) withKey(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(ta.home, ".ssh", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	for p, body := range map[string]string{path: "private\n", path + ".pub": "public\n"} {
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func (ta *testApp) servers(t *testing.T) []config.Server {
	t.Helper()
	servers, bad, err := config.Servers(ta.configDir)
	if err != nil || len(bad) > 0 {
		t.Fatalf("reading servers: %v %v", err, bad)
	}
	return servers
}

func (ta *testApp) mustRun(t *testing.T, args ...string) {
	t.Helper()
	if code := ta.Run(args); code != 0 {
		t.Fatalf("rembox %v: exit %d\nstdout:\n%s\nstderr:\n%s", args, code, ta.stdout, ta.stderr)
	}
}

func TestSetup(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	key := ta.withKey(t, "id_ed25519")
	// An IP address has no default name, so the name is typed; the user and
	// key are the defaults.
	ta.typing("203.0.113.10", "", "", "box1")

	ta.mustRun(t, "setup")
	out := ta.stdout.String()
	for _, want := range []string{"User to log in as [root]:", "SSH private key [~/.ssh/id_ed25519]:",
		"Name for this server:", "Saved box1 (root@203.0.113.10)", "(what setup.sh prints)",
		"box1 is set up. To log in: rembox ssh box1"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	want := []config.Server{{Name: "box1", Host: "203.0.113.10", User: "root", Key: key}}
	if got := ta.servers(t); !slices.Equal(got, want) {
		t.Errorf("saved %+v, want %+v", got, want)
	}
	// One ssh session, at a terminal, which runs setup.sh as root, without sudo.
	if len(ta.commands) != 1 || !slices.Equal(ta.scripts, []string{"setup.sh"}) {
		t.Errorf("ran %q", ta.commands)
	}
	if !slices.Contains(ta.sshArgs, "root@203.0.113.10") || !slices.Contains(ta.sshArgs, key) ||
		ta.sshArgs[0] != "-t" || slices.Contains(ta.sshArgs, "sudo") {
		t.Errorf("ssh args %q", ta.sshArgs)
	}
	if !strings.Contains(out, "\n$ ssh -t -i "+key+" -o IdentitiesOnly=yes") ||
		!strings.Contains(out, `root@203.0.113.10 bash -c "$(cat setup.sh)" bash`+"\n"+
			"  setup.sh comes with rembox; read it at https://github.com/aakashns/rembox/blob/v0.1.0/cli/internal/remote/setup.sh\n") {
		t.Errorf("output doesn't show the script's command:\n%s", out)
	}
	if strings.Index(out, "User to log in as") > strings.Index(out, "SSH private key") {
		t.Errorf("asked for the key before the user:\n%s", out)
	}
	info, err := os.Stat(filepath.Join(ta.configDir, "servers", "box1.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("server file: %v, %v", info, err)
	}

	ta.stdout.Reset()
	ta.mustRun(t, "list")
	lines := strings.Split(strings.TrimSpace(ta.stdout.String()), "\n")
	if len(lines) != 2 || strings.Join(strings.Fields(lines[1]), " ") != "box1 root@203.0.113.10 ~/.ssh/id_ed25519" {
		t.Errorf("list output:\n%s", ta.stdout)
	}
}

func TestSetupWithSudo(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.typing("box", "dev", "", "")
	ta.mustRun(t, "setup")
	if !strings.Contains(ta.stdout.String(), `-- dev@box sudo bash -c "$(cat setup.sh)" bash`+"\n") {
		t.Errorf("output:\n%s", ta.stdout)
	}
	// The script goes in the command, quoted, and ssh runs it at a terminal,
	// so sudo can ask for the password.
	args := ta.sshArgs
	i := slices.Index(args, "-c")
	if i < 0 || args[0] != "-t" || args[i-2] != "sudo" || args[i+1] != shellQuote(remote.Setup.Body) ||
		!slices.Equal(args[i+2:], []string{"bash"}) {
		t.Errorf("ssh args %q", args)
	}
}

func TestSetupAgain(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	key := ta.withKey(t, "id_ed25519")
	if err := config.SaveServer(ta.configDir, config.Server{Name: "box", Host: "old.example.com", User: "dev", Key: key}); err != nil {
		t.Fatal(err)
	}
	// The saved settings are the defaults; a new address replaces the old.
	ta.typing("new.example.com", "", "")
	ta.mustRun(t, "setup", "box")
	out := ta.stdout.String()
	for _, want := range []string{"Server IP address or hostname [old.example.com]:", "User to log in as [dev]:",
		"SSH private key [~/.ssh/id_ed25519]:", "Saved box (dev@new.example.com)"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Name for this server") {
		t.Errorf("asked for the name it was given:\n%s", out)
	}
	want := []config.Server{{Name: "box", Host: "new.example.com", User: "dev", Key: key}}
	if got := ta.servers(t); !slices.Equal(got, want) {
		t.Errorf("saved %+v, want %+v", got, want)
	}
}

func TestSetupArgs(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"setup", "nope"}, 1, "there's no server named nope"},
		{[]string{"setup", "a", "b"}, 2, `unexpected argument "b"`},
		{[]string{"setup", "-x"}, 2, "unknown flag -x"},
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
}

func TestSetupFails(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.onScript = func(string, []string, io.Writer) error { return exitWith(1) }
	ta.typing("box", "", "", "")
	ta.mustFail(t, "setup.sh failed on box (exit status 1); the output above says why", "setup")
	if strings.Contains(ta.stdout.String(), "is set up") {
		t.Errorf("output:\n%s", ta.stdout)
	}
}

func TestSetupCantConnect(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.onScript = func(string, []string, io.Writer) error { return exitWith(255) }
	ta.typing("box.example.com", "", "", "")
	ta.mustFail(t, "ssh couldn't connect to root@box.example.com (exit status 255). "+
		"To change the address, user or key: rembox setup box.example.com", "setup")
	// Saved before logging in, so it can be fixed with rembox setup NAME.
	if len(ta.servers(t)) != 1 {
		t.Error("didn't save the server")
	}
}

func TestSetupHostnameIsDefaultName(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_rsa")
	ta.typing("Dev.Example.com", "ubuntu", "", "")
	ta.mustRun(t, "setup")
	if !strings.Contains(ta.stdout.String(), "Name for this server [dev.example.com]:") {
		t.Errorf("output:\n%s", ta.stdout)
	}
	got := ta.servers(t)
	if len(got) != 1 || got[0].Name != "dev.example.com" || got[0].Destination() != "ubuntu@Dev.Example.com" ||
		filepath.Base(got[0].Key) != "id_rsa" {
		t.Errorf("saved %+v", got)
	}
}

func TestSetupAsksAgain(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	key := ta.withKey(t, "work")
	ta.typing(
		"", "-oProxyCommand=evil", "deploy@box.example.com", "box.example.com", // host
		"-u", "deploy", // user
		"", "~/.ssh/missing", "~/.ssh", "~/.ssh/work.pub", "~/.ssh/work", // key (no default)
		"Box!", "box", // name
	)

	ta.mustRun(t, "setup")
	out := ta.stdout.String()
	for _, want := range []string{
		"enter the server's IP address or hostname",
		`"-oProxyCommand=evil" isn't an IP address or hostname`,
		"rembox asks for the user next",
		`"-u" isn't a user name`,
		"ssh-keygen", "there's no file at", "is a folder", "is a public key",
		`"Box!" can't be a name`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if got := ta.servers(t); len(got) != 1 || got[0].Name != "box" || got[0].User != "deploy" || got[0].Key != key {
		t.Errorf("saved %+v", got)
	}
}

func TestSetupNameTaken(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	key := ta.withKey(t, "id_ed25519")
	if err := config.SaveServer(ta.configDir, config.Server{Name: "box", Host: "old", User: "root", Key: key}); err != nil {
		t.Fatal(err)
	}

	// Declining to replace it asks for another name.
	ta.typing("new", "", "", "box", "n", "box2")
	ta.mustRun(t, "setup")
	if got := ta.servers(t); len(got) != 2 || got[0].Host != "old" || got[1].Name != "box2" {
		t.Errorf("saved %+v", got)
	}

	// Agreeing replaces it.
	ta.typing("newer", "", "", "box", "y")
	ta.mustRun(t, "setup")
	if got := ta.servers(t); len(got) != 2 || got[0].Host != "newer" {
		t.Errorf("saved %+v", got)
	}
	if !strings.Contains(ta.stdout.String(), "which logs in to root@old with ~/.ssh/id_ed25519. Replace it?") {
		t.Errorf("output:\n%s", ta.stdout)
	}

	// The same answers again don't ask: it's the same server.
	ta.stdout.Reset()
	ta.typing("newer", "", "", "box")
	ta.mustRun(t, "setup")
	if strings.Contains(ta.stdout.String(), "Replace it?") {
		t.Errorf("asked to replace the same server:\n%s", ta.stdout)
	}
}

func TestSetupCancelled(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.typing("box.example.com") // then end of input

	if code := ta.Run([]string{"setup"}); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(ta.stderr.String(), "cancelled") || exists(ta.configDir) {
		t.Errorf("stderr: %s", ta.stderr)
	}
}

func TestListEmpty(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.mustRun(t, "list")
	if !strings.Contains(ta.stdout.String(), "No servers yet") {
		t.Errorf("output: %s", ta.stdout)
	}
}

func TestListSkipsBrokenFiles(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	if err := config.SaveServer(ta.configDir, config.Server{Name: "good", Host: "h", User: "root", Key: "/k"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ta.configDir, "servers", "bad.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	ta.mustRun(t, "list")
	if !strings.Contains(ta.stdout.String(), "good") || !strings.Contains(ta.stderr.String(), "bad.json") {
		t.Errorf("stdout:\n%s\nstderr:\n%s", ta.stdout, ta.stderr)
	}
}

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"root@10.0.0.5":     "root@10.0.0.5",
		"/a/b-c_d.e":        "/a/b-c_d.e",
		"":                  "''",
		"has space":         "'has space'",
		"it's":              `'it'\''s'`,
		"$HOME":             "'$HOME'",
		"StrictHostKey=yes": "StrictHostKey=yes",
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestCheckHost(t *testing.T) {
	for in, want := range map[string]string{
		"203.0.113.10":         "203.0.113.10",
		"box.example.com":      "box.example.com",
		"[2001:db8::1]":        "2001:db8::1",
		"2001:db8::1":          "2001:db8::1",
		"":                     "",
		"box example":          "",
		"-oProxyCommand=x":     "",
		"ubuntu@box":           "",
		"box.example.com:2222": "",
	} {
		got, err := checkHost(in)
		if (err != nil) != (want == "") || got != want {
			t.Errorf("checkHost(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}
