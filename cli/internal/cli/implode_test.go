package cli

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"sort"
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

const backupName = "rembox-backup-20261112-143000.tar.gz"

// backupContents lists the backup's entries and the contents of its files.
func backupContents(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	got := map[string]string{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return got
		}
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(tr)
		got[hdr.Name] = string(body)
	}
}

func TestImplodeWithBackup(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withConfig(t)
	ta.Interactive = true
	ta.Stdin = strings.NewReader("y\n\n") // yes, then the backup's default (yes)

	if code := ta.Run([]string{"implode"}); code != 0 {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	out := ta.stdout.String()
	for _, want := range []string{ta.exe, ta.configDir + " (2 files)", "[y/N]", "[Y/n]", "Backed up", "rembox is gone"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if exists(ta.exe) || exists(ta.configDir) {
		t.Error("binary or config folder still exists")
	}

	backup := filepath.Join(ta.home, backupName)
	info, err := os.Stat(backup)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("backup mode = %v, want 0600", info.Mode().Perm())
	}
	got := backupContents(t, backup)
	var names []string
	for name := range got {
		names = append(names, name)
	}
	sort.Strings(names)
	want := []string{"rembox/", "rembox/config.toml", "rembox/servers/", "rembox/servers/box1.toml"}
	if strings.Join(names, " ") != strings.Join(want, " ") {
		t.Errorf("backup entries = %v, want %v", names, want)
	}
	if got["rembox/servers/box1.toml"] != "host = 'x'\n" {
		t.Errorf("backed-up file = %q", got["rembox/servers/box1.toml"])
	}
}

func TestImplodeDeclineBackup(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withConfig(t)
	ta.Interactive = true
	ta.Stdin = strings.NewReader("y\nn\n")

	if code := ta.Run([]string{"implode"}); code != 0 {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	if exists(filepath.Join(ta.home, backupName)) {
		t.Error("made a backup after being told not to")
	}
	if exists(ta.exe) || exists(ta.configDir) {
		t.Error("binary or config folder still exists")
	}
}

func TestImplodeCancelled(t *testing.T) {
	for _, answer := range []string{"n\n", "\n", ""} {
		ta := newTestApp(t, "0.1.0")
		ta.withConfig(t)
		ta.Interactive = true
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

func TestImplodeNonInteractive(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withConfig(t)
	if code := ta.Run([]string{"implode"}); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(ta.stderr.String(), "--yes") || !exists(ta.exe) || !exists(ta.configDir) {
		t.Errorf("without --yes: removed something, or didn't suggest --yes: %s", ta.stderr)
	}
}

func TestImplodeYesBacksUpByDefault(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withConfig(t)
	if code := ta.Run([]string{"implode", "--yes"}); code != 0 {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	if !exists(filepath.Join(ta.home, backupName)) {
		t.Error("--yes didn't back up")
	}
	if exists(ta.exe) || exists(ta.configDir) {
		t.Error("binary or config folder still exists")
	}
}

func TestImplodeYesNoBackup(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.withConfig(t)
	if code := ta.Run([]string{"implode", "-y", "--no-backup"}); code != 0 {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	if exists(filepath.Join(ta.home, backupName)) {
		t.Error("backed up despite --no-backup")
	}
	if exists(ta.exe) || exists(ta.configDir) {
		t.Error("binary or config folder still exists")
	}
}

func TestImplodeBackupFlags(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	if code := ta.Run([]string{"implode", "--backup", "--no-backup"}); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

func TestImplodeNoConfig(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.Interactive = true
	ta.Stdin = strings.NewReader("y\n")

	if code := ta.Run([]string{"implode"}); code != 0 {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	if strings.Contains(ta.stdout.String(), "Back up") {
		t.Error("offered a backup with nothing to back up")
	}
	if exists(ta.exe) {
		t.Error("binary still exists")
	}
}

func TestImplodeEmptyConfigDir(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	if err := os.MkdirAll(ta.configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if code := ta.Run([]string{"implode", "--yes"}); code != 0 {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	if exists(filepath.Join(ta.home, backupName)) {
		t.Error("backed up an empty folder")
	}
	if exists(ta.configDir) {
		t.Error("config folder still exists")
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
	ta.Sudo = func(args ...string) error {
		ta.sudoArgs = args
		os.Chmod(dir, 0o755)
		return os.Remove(args[len(args)-1])
	}

	exe, err := filepath.EvalSymlinks(ta.exe) // implode resolves /var to /private/var on macOS
	if err != nil {
		t.Fatal(err)
	}

	if code := ta.Run([]string{"implode", "--yes"}); code != 0 {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	if got := strings.Join(ta.sudoArgs, " "); got != "rm -f -- "+exe {
		t.Errorf("sudo %s", got)
	}
	if !strings.Contains(ta.stdout.String(), "needs sudo") {
		t.Errorf("output doesn't say it needs sudo:\n%s", ta.stdout)
	}
}

func TestImplodeRefusesSudo(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.Euid = 0
	t.Setenv("SUDO_USER", "someone")
	if code := ta.Run([]string{"implode", "--yes"}); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !exists(ta.exe) {
		t.Error("removed the binary")
	}
}
