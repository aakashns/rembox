package cli

import (
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
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
	ta.typing("203.0.113.10", "", "", "", "box1", "y", "y")

	ta.mustRun(t, "setup")
	out := ta.stdout.String()
	for _, want := range []string{"SSH port [default: 22]:", "User to log in as [default: root]:", "SSH private key [default: ~/.ssh/id_ed25519]:",
		"Name for this server:",
		"Saved box1 (root@203.0.113.10, port 22)", "(what setup.sh prints)",
		"Saved box1 (ada@203.0.113.10, port 23456)",
		"Is box1 behind a firewall outside it, like your cloud provider's? [y/N]",
		"Open port 23456/tcp in that firewall now. Have you opened it? [y/N]",
		"box1 is set up. To log in: rembox ssh box1",
		"rembox list", "rembox setup box1", "rembox remove box1"} {
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

	// Two ssh sessions at a terminal: as root on port 22, then as ada on the
	// new port. Neither runs sudo itself: setup.sh does, after saying so.
	sessions := ta.sessions()
	if len(sessions) != 2 || len(ta.commands) != 2 || !slices.Equal(ta.scripts, []string{"setup.sh", "setup.sh"}) {
		t.Fatalf("ran %q", ta.commands)
	}
	first, home := sessions[0], sessions[1]
	if !slices.Equal(first[:3], []string{"-t", "-p", "22"}) || !slices.Contains(first, "root@203.0.113.10") ||
		!slices.Contains(first, key) || slices.Contains(first, "sudo") ||
		!slices.Equal(scriptArgs(first), []string{"start", "ada", "23456", "22", "box1"}) {
		t.Errorf("first session's args %q", first)
	}
	if !slices.Equal(home[:3], []string{"-t", "-p", "23456"}) || !slices.Contains(home, "ada@203.0.113.10") ||
		slices.Contains(home, "sudo") || !slices.Equal(scriptArgs(home), []string{"home", "''", "''", "23456"}) {
		t.Errorf("home session's args %q", home)
	}
	files := `"$(cat lib.sh check.sh hostname.sh git.sh postgres.sh update.sh toolchains.sh agents.sh opentunnel.sh instructions.sh t3.sh user.sh sshd.sh firewall.sh zsh.sh fail2ban.sh swap.sh locale.sh restart.sh setup.sh)"`
	if !strings.Contains(out, "\n$ ssh -t -p 22 -i "+key+" -o IdentitiesOnly=yes") ||
		!strings.Contains(out, "root@203.0.113.10 bash -c "+files+" bash start ada 23456 22 box1\n"+
			"  These scripts come with rembox; read them at https://github.com/aakashns/rembox/tree/v0.1.0/cli/internal/remote\n") ||
		!strings.Contains(out, "ada@203.0.113.10 bash -c "+files+" bash home '' '' 23456\n") {
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
	if len(lines) != 2 || strings.Join(strings.Fields(lines[1]), " ") != "box1 203.0.113.10 23456 ada ~/.ssh/id_ed25519" {
		t.Errorf("list output:\n%s", ta.stdout)
	}
}

func TestSetupNotRoot(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.typing("box", "", "ubuntu", "", "", "n")
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
	if s := ta.sessions(); len(s) != 2 || !slices.Equal(scriptArgs(s[0]), []string{"start", "ada", "2222", "2222", "box"}) {
		t.Errorf("sessions %q", s)
	}
}

func TestSetupLocales(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.Locales = []string{"en_IN.UTF-8", "en_US.UTF-8"}
	ta.typing("203.0.113.10", "", "", "", "box1", "n")
	ta.mustRun(t, "setup")
	// Only the home session adds them, as it finishes as root.
	s := ta.sessions()
	if len(s) != 2 || !slices.Equal(scriptArgs(s[0]), []string{"start", "ada", "23456", "22", "box1"}) ||
		!slices.Equal(scriptArgs(s[1]), []string{"home", "''", "''", "23456", "en_IN.UTF-8", "en_US.UTF-8"}) {
		t.Errorf("sessions %q", s)
	}
}

func TestSetupGitDefaults(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.GitConfig = func(key string) string {
		return map[string]string{"user.name": "Ada Lovelace", "user.email": "ada@example.com"}[key]
	}
	ta.typing("203.0.113.10", "", "", "", "box1", "n")
	ta.mustRun(t, "setup")
	// The home session asks for them, with these as defaults.
	s := ta.sessions()
	if len(s) != 2 || !slices.Equal(scriptArgs(s[1]), []string{"home", "'Ada Lovelace'", "ada@example.com", "23456"}) {
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
		!strings.Contains(out, "==> Reconnecting to devbox as ada, on port 2222, to set up your shell, GitHub, toolchains, coding agents and T3 Code, then finish as root") {
		t.Errorf("output:\n%s", out)
	}
	if c := readFile(t, filepath.Join(ta.configDir, "ssh_config")); !strings.Contains(c, "Host devbox") || strings.Contains(c, "Host box ") || strings.Contains(c, "Host box\n") {
		t.Errorf("ssh_config:\n%s", c)
	}
	// setup.sh makes the new name the server's hostname.
	if s := ta.sessions(); len(s) != 2 || !slices.Equal(scriptArgs(s[0]), []string{"start", "ada", "2222", "2222", "devbox"}) {
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
	ta.typing("", "", "", "", "", "n")
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

func TestMarkerWriter(t *testing.T) {
	var b strings.Builder
	u := &markerWriter{w: &b}
	// The markers' lines, split across writes, between a question that doesn't
	// end its line and colored output.
	for _, chunk := range []string{
		"\r\n    Username [default: ada]: ", "bob\r\n", "\x1b]rem", "box;user=bob\a", "\r\n",
		"\x1b]rembox;check=ok Added your user, bob\a\r\n", "\x1b]rembox;check=warn Not ", "restarted\a\r\n",
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
	if u.user != "bob" || b.String() != want ||
		!slices.Equal(u.checks, []string{"ok Added your user, bob", "warn Not restarted"}) {
		t.Errorf("user %q, checks %q, output %q", u.user, u.checks, b.String())
	}
}

func TestSetupChecklist(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	// Each session sends lines; rembox shows them all, in order, at the end.
	ta.onScript = func(_ string, args []string, stdout io.Writer) error {
		if scriptArgs(args)[0] == "home" {
			fmt.Fprintf(stdout, "%scheck=skip OpenCode and Pi: skipped\a\r\n", marker)
			fmt.Fprintf(stdout, "%scheck=warn Not restarted, but an update needs it: sudo systemctl reboot\a\r\n", marker)
			return nil
		}
		fmt.Fprintf(stdout, "%scheck=ok Named the server box, with 4 GiB of swap\a\r\n", marker)
		return tellUser(args, stdout)
	}
	ta.typing("box", "", "", "", "", "n")
	ta.mustRun(t, "setup")
	want := "box is set up. To log in: rembox ssh box\n\n" +
		"  ✓ Named the server box, with 4 GiB of swap\n" +
		"  – OpenCode and Pi: skipped\n" +
		"  ! Not restarted, but an update needs it: sudo systemctl reboot\n\n" +
		"  rembox ssh box"
	if out := ta.stdout.String(); !strings.Contains(out, want) || strings.Contains(out, "check=") {
		t.Errorf("output:\n%s", out)
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
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.onScript = func(_ string, args []string, stdout io.Writer) error {
		if slices.Contains(args, "home") {
			return exitWith(255)
		}
		return tellUser(args, stdout)
	}
	ta.typing("box.example.com", "", "", "", "", "n")
	ta.mustFail(t, "ssh couldn't log in to ada@box.example.com on port 23456 (exit status 255). "+
		"If box.example.com's cloud provider has a firewall, open port 23456/tcp in it, then run rembox setup box.example.com again", "setup")
	if strings.Contains(ta.stdout.String(), "~/.ssh/config") {
		t.Errorf("offered to change ~/.ssh/config before the server was set up:\n%s", ta.stdout)
	}
	// Saved with the new user and port, so running setup again tries them.
	if got := ta.servers(t); len(got) != 1 || got[0].User != "ada" || got[0].Port != 23456 {
		t.Errorf("saved %+v", got)
	}
}

func TestSetupNotMoved(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.typing("box.example.com", "", "", "", "", "y", "n")
	ta.mustFail(t, "ssh is on ports 22 and 23456 for now. Once port 23456/tcp is open in the firewall: rembox setup box.example.com", "setup")
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
	ta.typing("Dev.Example.com", "", "ubuntu", "", "", "n")
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
	// ssh stays on 2222, unless you give setup.sh another port.
	if got := ta.servers(t); len(got) != 1 || got[0].Name != "box" || got[0].User != "ada" || got[0].Port != 2222 ||
		got[0].Key != key || !slices.Equal(scriptArgs(ta.sessions()[0]), []string{"start", "ada", "2222", "2222", "box"}) ||
		ta.sessions()[0][2] != "2222" {
		t.Errorf("saved %+v", got)
	}
}

func TestSetupNewPort(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	// setup.sh asks for the port, and tells rembox the one you gave.
	ta.onScript = func(_ string, args []string, stdout io.Writer) error {
		if sa := scriptArgs(args); sa[0] == "start" {
			fmt.Fprintf(stdout, "%suser=ada\a\r\n%sport=40000\a\r\n", marker, marker)
		}
		return nil
	}
	ta.typing("box.example.com", "", "", "", "", "n")
	ta.mustRun(t, "setup")
	s := ta.sessions()
	if got := ta.servers(t); len(got) != 1 || got[0].Port != 40000 || len(s) != 2 ||
		!slices.Equal(scriptArgs(s[0]), []string{"start", "ada", "23456", "22", "box.example.com"}) ||
		s[1][2] != "40000" || !slices.Equal(scriptArgs(s[1]), []string{"home", "''", "''", "40000"}) {
		t.Errorf("saved %+v, sessions %q", got, s)
	}
	if !strings.Contains(ta.stdout.String(), "Is box.example.com behind a firewall outside it") {
		t.Errorf("didn't ask about a firewall:\n%s", ta.stdout)
	}
}

func TestSetupNoPort(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.onScript = func(_ string, args []string, stdout io.Writer) error {
		fmt.Fprintf(stdout, "%suser=ada\a\r\n", marker)
		return nil
	}
	ta.typing("box.example.com", "", "", "", "")
	ta.mustFail(t, `setup.sh didn't say which port it moved ssh to on box.example.com ("" isn't a port; enter a number from 1 to 65535)`, "setup")
	if len(ta.sessions()) != 1 {
		t.Errorf("sessions %q", ta.sessions())
	}
}

func TestSetupNameTaken(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	key := ta.withKey(t, "id_ed25519")
	if err := config.SaveServer(ta.configDir, config.Server{Name: "box", Host: "old", User: "root", Key: key}); err != nil {
		t.Fatal(err)
	}

	// Declining to replace it asks for another name.
	ta.typing("new", "", "", "", "box", "n", "box2", "n")
	ta.mustRun(t, "setup")
	if got := ta.servers(t); len(got) != 2 || got[0].Host != "old" || got[1].Name != "box2" {
		t.Errorf("saved %+v", got)
	}

	// Agreeing replaces it.
	ta.typing("newer", "", "", "", "box", "y", "n")
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

// TestSetupHelpMatchesSite checks that 'rembox help setup' lists the same
// steps, in the same words, as "What does rembox setup do?" on the site.
func TestSetupHelpMatchesSite(t *testing.T) {
	page, err := os.ReadFile("../../../site/public/index.html")
	if err != nil {
		t.Fatal(err)
	}
	steps := regexp.MustCompile(`(?s)<ol class="steps">(.*?)</ol>`).FindSubmatch(page)
	if steps == nil {
		t.Fatal(`no <ol class="steps"> in index.html`)
	}
	site := html.UnescapeString(regexp.MustCompile(`<[^>]+>`).ReplaceAllString(string(steps[1]), ""))
	site = strings.Join(strings.Fields(strings.ReplaceAll(site, "’", "'")), " ")

	// The help's steps: the indented lines after the first paragraph, less
	// their numbers and dashes.
	_, list, _ := strings.Cut(setupHelp, "rembox setup:\n")
	var words []string
	for _, line := range strings.Split(list, "\n") {
		if line != "" && !strings.HasPrefix(line, " ") {
			break
		}
		words = append(words, strings.Fields(regexp.MustCompile(`^\s*(\d+\.|-)\s`).ReplaceAllString(line, ""))...)
	}
	if help := strings.Join(words, " "); help != site {
		t.Errorf("rembox help setup's steps differ from the site's:\nhelp: %s\nsite: %s", help, site)
	}
}

func TestDefaultKeyPrefersECDSA(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_rsa")
	ta.withKey(t, "id_ed25519")
	if got := defaultKey(ta.home); got != "~/.ssh/id_ed25519" {
		t.Errorf("without id_ecdsa, defaultKey = %q", got)
	}
	ta.withKey(t, "id_ecdsa")
	if got := defaultKey(ta.home); got != "~/.ssh/id_ecdsa" {
		t.Errorf("defaultKey = %q, want ~/.ssh/id_ecdsa", got)
	}
}

func TestCompletePath(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "devbox")
	ta.withKey(t, "deploy")
	ta.withKey(t, "work")
	ssh := filepath.Join(ta.home, ".ssh")
	for _, f := range []string{"config", ".hidden"} {
		if err := os.WriteFile(filepath.Join(ssh, f), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(ssh, "keys"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		typed, want string
		choices     []string
	}{
		{"~", "~/", nil},
		{"~/.s", "~/.ssh/", nil},
		{"~/.ssh/w", "~/.ssh/work", nil},  // not work.pub
		{"~/.ssh/k", "~/.ssh/keys/", nil}, // a folder
		{"~/.ssh/de", "~/.ssh/de", []string{"deploy", "devbox"}},
		{"~/.ssh/dev", "~/.ssh/devbox", nil},
		{"~/.ssh/", "~/.ssh/", []string{"config", "deploy", "devbox", "keys/", "work"}},
		{"~/.ssh/.", "~/.ssh/.hidden", nil},
		{"~/.ssh/x", "~/.ssh/x", nil},
		{"~/nowhere/x", "~/nowhere/x", nil},
		{ssh + "/wo", ssh + "/work", nil},
	} {
		got, choices := completePath(ta.home, tc.typed)
		if got != tc.want || !slices.Equal(choices, tc.choices) {
			t.Errorf("completePath(%q) = %q, %q; want %q, %q", tc.typed, got, choices, tc.want, tc.choices)
		}
	}
}
