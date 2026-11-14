package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSetupSSHConfig(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	key := ta.withKey(t, "id_ed25519")
	userConfig := filepath.Join(ta.home, ".ssh", "config")
	mine := "Host *\n  ServerAliveInterval 30\n"
	if err := os.WriteFile(userConfig, []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}

	ta.typing("203.0.113.10", "", "", "", "box", "y", "y")
	ta.mustRun(t, "setup")
	out := ta.stdout.String()
	include := "Include ~/.config/rembox/ssh_config"
	for _, want := range []string{
		"Add '" + include + "' to ~/.ssh/config? [y/N]",
		"Added '" + include + "' to ~/.ssh/config."} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// Asked once the server is set up.
	if strings.Index(out, "Add '") < strings.LastIndex(out, "(what setup.sh prints)") {
		t.Errorf("asked before setup.sh ran:\n%s", out)
	}
	// At the top, before your Host *, with your file as it was.
	if got := readFile(t, userConfig); got != include+"\n\n"+mine {
		t.Errorf("~/.ssh/config:\n%s", got)
	}
	if info, err := os.Stat(userConfig); err != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("~/.ssh/config's mode: %v, %v", info, err)
	}
	// The server as setup left it: your user, on the new port.
	want := "\nHost box 203.0.113.10\n  HostName 203.0.113.10\n  Port 23456\n  User ada\n  IdentityFile " + key +
		"\n  IdentitiesOnly yes\n"
	if got := readFile(t, filepath.Join(ta.configDir, "ssh_config")); !strings.HasPrefix(got, "# Written by rembox") ||
		!strings.HasSuffix(got, want) {
		t.Errorf("ssh_config:\n%s", got)
	}

	// Once it's there, setup doesn't ask again.
	ta.stdout.Reset()
	ta.typing("", "", "", "")
	ta.mustRun(t, "setup", "box")
	if strings.Contains(ta.stdout.String(), "~/.ssh/config") {
		t.Errorf("asked again:\n%s", ta.stdout)
	}

	// implode takes it out again.
	ta.stdout.Reset()
	ta.typing("y")
	ta.mustRun(t, "implode")
	if !strings.Contains(ta.stdout.String(), "  '"+include+"' in ~/.ssh/config\n") ||
		!strings.Contains(ta.stdout.String(), "Removed '"+include+"' from ~/.ssh/config") {
		t.Errorf("implode output:\n%s", ta.stdout)
	}
	if got := readFile(t, userConfig); got != mine {
		t.Errorf("~/.ssh/config after implode:\n%s", got)
	}
}

func TestSetupSSHConfigDeclined(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	ta.typing("box", "", "", "", "", "y", "n")
	ta.mustRun(t, "setup")
	if exists(filepath.Join(ta.home, ".ssh", "config")) {
		t.Error("wrote ~/.ssh/config")
	}
	// rembox keeps its own file either way.
	if !exists(filepath.Join(ta.configDir, "ssh_config")) {
		t.Error("didn't write ssh_config")
	}
}

func TestSetupSSHConfigLinked(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withKey(t, "id_ed25519")
	// A dotfiles manager's link: rembox changes the file it links to.
	dotfile := filepath.Join(t.TempDir(), "ssh_config")
	if err := os.WriteFile(dotfile, []byte("Host x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(ta.home, ".ssh", "config")
	if err := os.Symlink(dotfile, link); err != nil {
		t.Fatal(err)
	}
	ta.typing("box", "", "", "", "", "y", "y")
	ta.mustRun(t, "setup")
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Error("replaced the link")
	}
	if got := readFile(t, dotfile); got != "Include ~/.config/rembox/ssh_config\n\nHost x\n" {
		t.Errorf("linked file:\n%s", got)
	}
}
