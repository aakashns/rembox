package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withConfig gives the test app some files in its config folder.
func (ta *testApp) withConfig(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(ta.configDir, "servers"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"config.toml": "a = 1\n", "servers/box1.toml": "host = 'x'\n"} {
		if err := os.WriteFile(filepath.Join(ta.configDir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestImplode(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withConfig(t)
	exe, err := filepath.EvalSymlinks(ta.exe) // implode resolves /var to /private/var on macOS
	if err != nil {
		t.Fatal(err)
	}
	ta.typing("y")

	if code := ta.Run([]string{"implode"}); code != 0 {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	out := ta.stdout.String()
	for _, want := range []string{"  " + exe + "\n", "  " + ta.configDir + "\n", "[y/N]", "rembox is gone"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if exists(ta.exe) || exists(ta.configDir) {
		t.Error("binary or config folder still exists")
	}
}

func TestImplodeCancelled(t *testing.T) {
	for _, answer := range []string{"n\n", "\n", ""} {
		ta := newTestApp(t, "0.1.0")
		ta.withConfig(t)
		ta.Stdin = strings.NewReader(answer)

		if code := ta.Run([]string{"implode"}); code != 0 {
			t.Fatalf("answer %q: exit %d: %s", answer, code, ta.stderr)
		}
		if !exists(ta.exe) || !exists(ta.configDir) {
			t.Errorf("answer %q: removed something", answer)
		}
		if !strings.Contains(ta.stdout.String(), "Nothing was removed") {
			t.Errorf("answer %q: output:\n%s", answer, ta.stdout)
		}
	}
}

func TestImplodeNoConfig(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.typing("y")

	if code := ta.Run([]string{"implode"}); code != 0 {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	if strings.Contains(ta.stdout.String(), ta.configDir) {
		t.Error("listed a config folder that isn't there")
	}
	if exists(ta.exe) {
		t.Error("binary still exists")
	}
}

func TestImplodeUsesSudoForUnwritableFolder(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}
	ta := newTestApp(t, "0.1.0")
	dir := filepath.Dir(ta.exe)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	// The fake sudo removes the file, which needs the folder writable.
	ta.onSudo = func(args []string) error {
		os.Chmod(dir, 0o755)
		return os.Remove(args[len(args)-1])
	}

	exe, err := filepath.EvalSymlinks(ta.exe) // implode resolves /var to /private/var on macOS
	if err != nil {
		t.Fatal(err)
	}

	ta.typing("y", "y")
	if code := ta.Run([]string{"implode"}); code != 0 {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	if got := strings.Join(ta.sudoArgs, " "); got != "rm -f -- "+exe {
		t.Errorf("sudo %s", got)
	}
	for _, want := range []string{"needs sudo", "$ sudo rm -f -- " + exe} {
		if !strings.Contains(ta.stdout.String(), want) {
			t.Errorf("output missing %q:\n%s", want, ta.stdout)
		}
	}
}

func TestImplodeSudoDeclined(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}
	ta := newTestApp(t, "0.1.0")
	ta.withConfig(t)
	dir := filepath.Dir(ta.exe)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	ta.typing("y", "n") // remove, but don't run sudo

	if code := ta.Run([]string{"implode"}); code != 0 {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	out := ta.stdout.String()
	sudoAt, askAt := strings.Index(out, "$ sudo rm -f -- "), strings.Index(out, "Run this command? [y/N]")
	if sudoAt < 0 || askAt < sudoAt {
		t.Errorf("didn't show the sudo command, then ask:\n%s", out)
	}
	if ta.sudoArgs != nil {
		t.Error("ran sudo after being told not to")
	}
	if !exists(ta.exe) || !exists(ta.configDir) || !strings.Contains(out, "Nothing was removed") {
		t.Errorf("removed something:\n%s", out)
	}
}

func TestImplodeRefusesSudo(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.Euid = 0
	t.Setenv("SUDO_USER", "someone")
	if code := ta.Run([]string{"implode"}); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !exists(ta.exe) {
		t.Error("removed the binary")
	}
}
