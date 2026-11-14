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

// sessions are the ssh commands setup ran, as their args.
func (ta *testApp) sessions() [][]string {
	var ssh [][]string
	for _, c := range ta.commands {
		if c[0] == "ssh" {
			ssh = append(ssh, c[1:])
		}
	}
	return ssh
}

// scriptArgs are the args after the script's $0 in ssh's args.
func scriptArgs(args []string) []string {
	i := slices.Index(args, shellQuote(remote.Setup.Body))
	return args[i+2:]
}

func TestSetup(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	key := ta.withKey(t, "id_ed25519")
	// An IP address has no default name, so the name is typed; the rest are
	// the defaults.
	ta.typing("203.0.113.10", "", "", "", "box1", "y")

	ta.mustRun(t, "setup")
	out := ta.stdout.String()
	for _, want := range []string{"SSH port [default: 22]:", "User to log in as [default: root]:", "SSH private key [default: ~/.ssh/id_ed25519]:",
		"Name for this server:",
		"Saved box1 (root@203.0.113.10, port 22)", "(what setup.sh prints)",
		"Saved box1 (ada@203.0.113.10, port 23456)",
		"! If box1's cloud provider has a firewall, open port 23456/tcp in it now.",
		"Move ssh to port 23456, closing port 22? [y/N]",
		"box1 is set up. To log in: rembox ssh box1"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "rembox;user") {
		t.Errorf("showed the line telling rembox the user:\n%s", out)
	}
	want := []config.Server{{Name: "box1", Host: "203.0.113.10", Port: 23456, User: "ada", Key: key}}
	if got := ta.servers(t); !slices.Equal(got, want) {
		t.Errorf("saved %+v, want %+v", got, want)
	}

	// Three ssh sessions at a terminal: as root on port 22, then twice as ada
	// on the new port. None runs sudo itself: setup.sh does, after saying so.
	sessions := ta.sessions()
	if len(sessions) != 3 || len(ta.commands) != 3 || !slices.Equal(ta.scripts, []string{"setup.sh", "setup.sh", "setup.sh"}) {
		t.Fatalf("ran %q", ta.commands)
	}
	first, home, second := sessions[0], sessions[1], sessions[2]
	if !slices.Equal(first[:3], []string{"-t", "-p", "22"}) || !slices.Contains(first, "root@203.0.113.10") ||
		!slices.Contains(first, key) || slices.Contains(first, "sudo") ||
		!slices.Equal(scriptArgs(first), []string{"start", "ada", "23456", "22", "box1"}) {
		t.Errorf("first session's args %q", first)
	}
	if !slices.Equal(home[:3], []string{"-t", "-p", "23456"}) || !slices.Contains(home, "ada@203.0.113.10") ||
		slices.Contains(home, "sudo") || !slices.Equal(scriptArgs(home), []string{"home"}) {
		t.Errorf("home session's args %q", home)
	}
	if !slices.Equal(second[:3], []string{"-t", "-p", "23456"}) || !slices.Contains(second, "ada@203.0.113.10") ||
		slices.Contains(second, "sudo") || !slices.Equal(scriptArgs(second), []string{"finish", "ada", "23456"}) {
		t.Errorf("last session's args %q", second)
	}
	files := `"$(cat lib.sh check.sh hostname.sh update.sh user.sh sshd.sh firewall.sh zsh.sh fail2ban.sh swap.sh locale.sh restart.sh setup.sh)"`
	if !strings.Contains(out, "\n$ ssh -t -p 22 -i "+key+" -o IdentitiesOnly=yes") ||
		!strings.Contains(out, "root@203.0.113.10 bash -c "+files+" bash start ada 23456 22 box1\n"+
			"  These scripts come with rembox; read them at https://github.com/aakashns/rembox/tree/v0.1.0/cli/internal/remote\n") ||
		!strings.Contains(out, "ada@203.0.113.10 bash -c "+files+" bash home\n") ||
		!strings.Contains(out, "ada@203.0.113.10 bash -c "+files+" bash finish ada 23456\n") {
		t.Errorf("output doesn't show the scripts' commands:\n%s", out)
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
	if len(lines) != 2 || strings.Join(strings.Fields(lines[1]), " ") != "box1 ada@203.0.113.10 23456 ~/.ssh/id_ed25519" {
		t.Errorf("list output:\n%s", ta.stdout)
	}
}

func TestSetupNotRoot(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.typing("box", "", "ubuntu", "", "", "y")
	ta.mustRun(t, "setup")
	if !strings.Contains(ta.stdout.String(), `-- ubuntu@box bash -c "$(cat `) {
		t.Errorf("output:\n%s", ta.stdout)
	}
	// The script goes in the command, quoted, and ssh runs it at a terminal,
	// so sudo, which setup.sh runs, can ask for the password.
	args := ta.sessions()[0]
	i := slices.Index(args, "-c")
	if i < 0 || args[0] != "-t" || args[i-2] != "ubuntu@box" || args[i+1] != shellQuote(remote.Setup.Body) ||
		!slices.Equal(args[i+2:], []string{"bash", "start", "ada", "23456", "22", "box"}) {
		t.Errorf("ssh args %q", args)
	}
}

func TestSetupAgain(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	key := ta.withKey(t, "id_ed25519")
	if err := config.SaveServer(ta.configDir, config.Server{Name: "box", Host: "old.example.com", Port: 2222, User: "ada", Key: key}); err != nil {
		t.Fatal(err)
	}
	// The saved settings are the defaults; a new address replaces the old.
	ta.typing("new.example.com", "", "", "", "")
	ta.mustRun(t, "setup", "box")
	out := ta.stdout.String()
	for _, want := range []string{"Server IP address or hostname [default: old.example.com]:", "SSH port [default: 2222]:",
		"User to log in as [default: ada]:", "SSH private key [default: ~/.ssh/id_ed25519]:",
		"Name for this server [default: box]:", "Saved box (ada@new.example.com, port 2222)"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Replace it?") || strings.Contains(out, "Removed") {
		t.Errorf("treated keeping the name as a rename:\n%s", out)
	}
	if strings.Contains(out, "Move ssh") {
		t.Errorf("asked to move ssh to the port it's on:\n%s", out)
	}
	want := []config.Server{{Name: "box", Host: "new.example.com", Port: 2222, User: "ada", Key: key}}
	if got := ta.servers(t); !slices.Equal(got, want) {
		t.Errorf("saved %+v, want %+v", got, want)
	}
	if s := ta.sessions(); len(s) != 3 || !slices.Equal(scriptArgs(s[0]), []string{"start", "ada", "2222", "2222", "box"}) {
		t.Errorf("sessions %q", s)
	}
}

func TestSetupLocales(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.Locales = []string{"en_IN.UTF-8", "en_US.UTF-8"}
	ta.typing("203.0.113.10", "", "", "", "box1", "y")
	ta.mustRun(t, "setup")
	// Only the last session adds them.
	s := ta.sessions()
	if len(s) != 3 || !slices.Equal(scriptArgs(s[0]), []string{"start", "ada", "23456", "22", "box1"}) ||
		!slices.Equal(scriptArgs(s[2]), []string{"finish", "ada", "23456", "en_IN.UTF-8", "en_US.UTF-8"}) {
		t.Errorf("sessions %q", s)
	}
}

func TestSentLocales(t *testing.T) {
	env := []string{
		"HOME=/Users/ada", "LANG=en_US.UTF-8", "LC_ALL=en_IN.utf8", "LC_CTYPE=UTF-8", // macOS's plain UTF-8
		"LC_TIME=C", "LC_NUMERIC=C.UTF-8", "LC_MONETARY=de_DE.ISO8859-1", "LC_COLLATE=en_US.UTF-8",
		"LANGUAGE=fr_FR.UTF-8", // ssh doesn't send it
		"LC_MESSAGES=en_US.UTF-8; rm -rf /",
	}
	if got, want := sentLocales(env), []string{"en_IN.UTF-8", "en_US.UTF-8"}; !slices.Equal(got, want) {
		t.Errorf("sentLocales = %q, want %q", got, want)
	}
	if got := sentLocales([]string{"PATH=/bin"}); got != nil {
		t.Errorf("sentLocales with none = %q", got)
	}
}

func TestSetupRename(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	key := ta.withKey(t, "id_ed25519")
	if err := config.SaveServer(ta.configDir, config.Server{Name: "box", Host: "box.example.com", Port: 2222, User: "ada", Key: key}); err != nil {
		t.Fatal(err)
	}
	ta.typing("", "", "", "", "devbox")
	ta.mustRun(t, "setup", "box")
	want := []config.Server{{Name: "devbox", Host: "box.example.com", Port: 2222, User: "ada", Key: key}}
	if got := ta.servers(t); !slices.Equal(got, want) {
		t.Errorf("saved %+v, want %+v", got, want)
	}
	if out := ta.stdout.String(); !strings.Contains(out, "Removed box, now that it's called something else.") ||
		!strings.Contains(out, "==> Connecting to devbox as ada, on port 2222") ||
		!strings.Contains(out, "==> Reconnecting to devbox as ada, on port 2222, to set up your shell") ||
		!strings.Contains(out, "==> Reconnecting to devbox as ada, on port 2222, to finish as root") {
		t.Errorf("output:\n%s", out)
	}
	if c := readFile(t, filepath.Join(ta.configDir, "ssh_config")); !strings.Contains(c, "Host devbox") || strings.Contains(c, "Host box ") || strings.Contains(c, "Host box\n") {
		t.Errorf("ssh_config:\n%s", c)
	}
	// setup.sh makes the new name the server's hostname.
	if s := ta.sessions(); len(s) != 3 || !slices.Equal(scriptArgs(s[0]), []string{"start", "ada", "2222", "2222", "devbox"}) {
		t.Errorf("sessions %q", s)
	}
}

func TestSetupOldServer(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	key := ta.withKey(t, "id_ed25519")
	// Saved by rembox 0.6.0, without a port.
	path := filepath.Join(ta.configDir, "servers", "box.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"host": "box", "user": "root", "key": "`+key+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ta.typing("", "", "", "", "", "y")
	ta.mustRun(t, "setup", "box")
	if !strings.Contains(ta.stdout.String(), "SSH port [default: 22]:") ||
		!slices.Equal(scriptArgs(ta.sessions()[0]), []string{"start", "ada", "23456", "22", "box"}) {
		t.Errorf("output:\n%s", ta.stdout)
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
	ta.typing("box", "", "", "", "")
	ta.mustFail(t, "setup.sh failed on box (exit status 1); the output above says why", "setup")
	if len(ta.sessions()) != 1 {
		t.Errorf("carried on after the first session failed: %q", ta.commands)
	}
	if strings.Contains(ta.stdout.String(), "is set up") {
		t.Errorf("output:\n%s", ta.stdout)
	}
}

func TestSetupNoUser(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.onScript = func(string, []string, io.Writer) error { return nil } // as an old setup.sh might
	ta.typing("box", "", "", "", "")
	ta.mustFail(t, "setup.sh didn't say which user it added on box (enter a name for your user on the server)", "setup")
	if len(ta.sessions()) != 1 {
		t.Errorf("carried on without a user: %q", ta.commands)
	}
}

func TestUserWriter(t *testing.T) {
	var b strings.Builder
	u := &userWriter{w: &b}
	// The marker's line, split across writes, between a question that doesn't
	// end its line and colored output.
	for _, chunk := range []string{
		"\r\n    Username [default: ada]: ", "bob\r\n", "\x1b]rem", "box;user=bob\a", "\r\n",
		"\x1b[36m-->\x1b[0m next\r\n", "\x1b]0;title\a\r\n", "rembox;user=eve\r\n", "\x1b]rembox",
	} {
		if n, err := u.Write([]byte(chunk)); n != len(chunk) || err != nil {
			t.Fatalf("Write(%q) = %d, %v", chunk, n, err)
		}
		if chunk == "\r\n    Username [default: ada]: " && b.String() != chunk {
			t.Errorf("held back the question: %q", b.String())
		}
	}
	u.flush()
	want := "\r\n    Username [default: ada]: bob\r\n\x1b[36m-->\x1b[0m next\r\n\x1b]0;title\a\r\nrembox;user=eve\r\n\x1b]rembox"
	if u.user != "bob" || b.String() != want {
		t.Errorf("user %q, output %q", u.user, b.String())
	}
}

func TestSetupCantConnect(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.onScript = func(string, []string, io.Writer) error { return exitWith(255) }
	ta.typing("box.example.com", "", "", "", "")
	ta.mustFail(t, "ssh couldn't log in to root@box.example.com on port 22 (exit status 255). "+
		"To change the address, port, user or key: rembox setup box.example.com", "setup")
	// Saved before logging in, so it can be fixed with rembox setup NAME.
	if got := ta.servers(t); len(got) != 1 || got[0].User != "root" {
		t.Errorf("saved %+v", got)
	}
}

func TestSetupCantLogInOnNewPort(t *testing.T) {
	// Either session on the new port, as your user or as root.
	for _, failing := range []string{"home", "finish"} {
		ta := newTestApp(t, "0.1.0")
		ta.withKey(t, "id_ed25519")
		ta.onScript = func(_ string, args []string, stdout io.Writer) error {
			if slices.Contains(args, failing) {
				return exitWith(255)
			}
			return tellUser(args, stdout)
		}
		ta.typing("box.example.com", "", "", "", "", "y")
		ta.mustFail(t, "ssh couldn't log in to ada@box.example.com on port 23456 (exit status 255). "+
			"If box.example.com's cloud provider has a firewall, open port 23456/tcp in it, then run rembox setup box.example.com again", "setup")
		if strings.Contains(ta.stdout.String(), "~/.ssh/config") {
			t.Errorf("%s: offered to change ~/.ssh/config before the server was set up:\n%s", failing, ta.stdout)
		}
		// Saved with the new user and port, so running setup again tries them.
		if got := ta.servers(t); len(got) != 1 || got[0].User != "ada" || got[0].Port != 23456 {
			t.Errorf("%s: saved %+v", failing, got)
		}
		if failing == "home" && len(ta.sessions()) != 2 {
			t.Errorf("carried on after the home session failed: %q", ta.commands)
		}
	}
}

func TestSetupNotMoved(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.typing("box.example.com", "", "", "", "", "n")
	ta.mustFail(t, "ssh is on ports 22 and 23456 for now. To move it to port 23456: rembox setup box.example.com", "setup")
	if len(ta.sessions()) != 1 {
		t.Errorf("logged in on the new port after you said no: %q", ta.commands)
	}
	// Saved with the new user and port, so running setup again keeps them.
	if got := ta.servers(t); len(got) != 1 || got[0].User != "ada" || got[0].Port != 23456 {
		t.Errorf("saved %+v", got)
	}
}

func TestSetupHostnameIsDefaultName(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_rsa")
	ta.typing("Dev.Example.com", "", "ubuntu", "", "", "y")
	ta.mustRun(t, "setup")
	if !strings.Contains(ta.stdout.String(), "Name for this server [default: dev.example.com]:") {
		t.Errorf("output:\n%s", ta.stdout)
	}
	got := ta.servers(t)
	if len(got) != 1 || got[0].Name != "dev.example.com" || got[0].Destination() != "ada@Dev.Example.com" ||
		filepath.Base(got[0].Key) != "id_rsa" {
		t.Errorf("saved %+v", got)
	}
}

func TestSetupAsksAgain(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	key := ta.withKey(t, "work")
	ta.typing(
		"", "-oProxyCommand=evil", "deploy@box.example.com", "box.example.com:2222", "box.example.com", // host
		"ssh", "0", "65536", "2222", // port
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
		"rembox asks for the port next",
		`"ssh" isn't a port`, `"0" isn't a port`, `"65536" isn't a port`,
		`"-u" isn't a user name`,
		"ssh-keygen", "there's no file at", "is a folder", "is a public key",
		`"Box!" can't be a name`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if got := ta.servers(t); len(got) != 1 || got[0].Name != "box" || got[0].User != "ada" || got[0].Port != 2222 ||
		got[0].Key != key || !slices.Equal(scriptArgs(ta.sessions()[0]), []string{"start", "ada", "2222", "2222", "box"}) ||
		ta.sessions()[0][2] != "2222" {
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
	ta.typing("new", "", "", "", "box", "n", "box2", "y")
	ta.mustRun(t, "setup")
	if got := ta.servers(t); len(got) != 2 || got[0].Host != "old" || got[1].Name != "box2" {
		t.Errorf("saved %+v", got)
	}

	// Agreeing replaces it.
	ta.typing("newer", "", "", "", "box", "y", "y")
	ta.mustRun(t, "setup")
	if got := ta.servers(t); len(got) != 2 || got[0].Host != "newer" {
		t.Errorf("saved %+v", got)
	}
	if !strings.Contains(ta.stdout.String(), "which logs in to root@old with ~/.ssh/id_ed25519. Replace it?") {
		t.Errorf("output:\n%s", ta.stdout)
	}

	// The server as setup saved it doesn't ask: it's the same server.
	ta.stdout.Reset()
	ta.typing("newer", "23456", "ada", "", "box")
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
