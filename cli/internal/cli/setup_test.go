package cli

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/aakashns/rembox/cli/internal/config"
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

func TestSetupInteractive(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	key := ta.withKey(t, "id_ed25519")
	ta.Interactive = true
	// An IP address has no default name, so the name is typed; the user and
	// key are the defaults.
	ta.Stdin = strings.NewReader("203.0.113.10\n\n\nbox1\n")

	ta.mustRun(t, "setup")
	out := ta.stdout.String()
	for _, want := range []string{"User to log in as [root]:", "SSH private key [~/.ssh/id_ed25519]:",
		"Name for this server:", "Saved box1 (root@203.0.113.10)"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	want := []config.Server{{Name: "box1", Host: "203.0.113.10", User: "root", Key: key}}
	if got := ta.servers(t); !slices.Equal(got, want) {
		t.Errorf("saved %+v, want %+v", got, want)
	}
	if !slices.Contains(ta.sshArgs, "root@203.0.113.10") || !slices.Contains(ta.sshArgs, key) {
		t.Errorf("ssh args %q", ta.sshArgs)
	}
	if slices.Contains(ta.sshArgs, "BatchMode=yes") {
		t.Error("ssh can't ask for the key's passphrase")
	}
	if strings.Index(out, "User to log in as") > strings.Index(out, "SSH private key") {
		t.Errorf("asked for the key before the user:\n%s", out)
	}
	if !strings.Contains(out, "\n\n$ ssh -i "+key+" -o IdentitiesOnly=yes") {
		t.Errorf("output doesn't show the ssh command:\n%s", out)
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

func TestSetupHostnameIsDefaultName(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_rsa")
	ta.Interactive = true
	ta.Stdin = strings.NewReader("Dev.Example.com\nubuntu\n\n\n")

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
	ta.Interactive = true
	ta.Stdin = strings.NewReader(strings.Join([]string{
		"", "-oProxyCommand=evil", "deploy@box.example.com", "box.example.com", // host
		"-u", "deploy", // user
		"", "~/.ssh/missing", "~/.ssh", "~/.ssh/work.pub", "~/.ssh/work", // key (no default)
		"Box!", "box", // name
	}, "\n") + "\n")

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

func TestSetupSSHFails(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.sshErr = errors.New("exit status 255")

	if code := ta.Run([]string{"setup", "--host", "box.example.com"}); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(ta.stderr.String(), "couldn't log in to root@box.example.com") ||
		!strings.Contains(ta.stderr.String(), "Nothing was saved") {
		t.Errorf("stderr: %s", ta.stderr)
	}
	if exists(ta.configDir) {
		t.Error("saved a server ssh couldn't log in to")
	}
}

func TestSetupFlags(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	key := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(key, []byte("private\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ta.mustRun(t, "setup", "--host", "10.0.0.5", "--user", "admin", "--key", key, "--name", "lab")
	want := []config.Server{{Name: "lab", Host: "10.0.0.5", User: "admin", Key: key}}
	if got := ta.servers(t); !slices.Equal(got, want) {
		t.Errorf("saved %+v, want %+v", got, want)
	}
	if !slices.Contains(ta.sshArgs, "BatchMode=yes") {
		t.Errorf("without a terminal, ssh should be in batch mode: %q", ta.sshArgs)
	}
}

func TestSetupNonInteractiveMissing(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"setup"}, "pass --host"},
		{[]string{"setup", "--host", "box.example.com"}, "pass --key"}, // no keys in ~/.ssh
		{[]string{"setup", "--host", "10.0.0.5", "--key", "/etc/hosts"}, "pass --name"},
		{[]string{"setup", "--host", "box", "--key", "/nonexistent"}, "--key: there's no file"},
		{[]string{"setup", "--host", "box", "--user", "-x", "--key", "/etc/hosts"}, `--user: "-x" isn't a user name`},
		{[]string{"setup", "--host", "a@box"}, "--host: enter just the IP address or hostname"},
	} {
		ta := newTestApp(t, "0.1.0")
		if code := ta.Run(tc.args); code != 1 {
			t.Errorf("%v: exit %d, want 1", tc.args, code)
		}
		if !strings.Contains(ta.stderr.String(), tc.want) {
			t.Errorf("%v: stderr %q doesn't mention %q", tc.args, ta.stderr, tc.want)
		}
		if ta.sshArgs != nil {
			t.Errorf("%v: ran ssh", tc.args)
		}
	}
}

func TestSetupNameTaken(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	key := ta.withKey(t, "id_ed25519")
	if err := config.SaveServer(ta.configDir, config.Server{Name: "box", Host: "old", User: "root", Key: key}); err != nil {
		t.Fatal(err)
	}

	// Without a terminal, it's an error.
	if code := ta.Run([]string{"setup", "--host", "new", "--name", "box"}); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(ta.stderr.String(), "already a server named box") {
		t.Errorf("stderr: %s", ta.stderr)
	}

	// Declining to replace it asks for another name.
	ta.Interactive = true
	ta.Stdin = strings.NewReader("new\n\n\nbox\nn\nbox2\n")
	ta.mustRun(t, "setup")
	if got := ta.servers(t); len(got) != 2 || got[0].Host != "old" || got[1].Name != "box2" {
		t.Errorf("saved %+v", got)
	}

	// Agreeing replaces it.
	ta.Stdin = strings.NewReader("newer\n\n\nbox\ny\n")
	ta.in = nil
	ta.mustRun(t, "setup")
	if got := ta.servers(t); len(got) != 2 || got[0].Host != "newer" {
		t.Errorf("saved %+v", got)
	}
}

func TestSetupCancelled(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.Interactive = true
	ta.Stdin = strings.NewReader("box.example.com\n") // then end of input

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
