package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aakashns/rembox/cli/internal/release"
	"github.com/aakashns/rembox/cli/internal/release/releasetest"
	"github.com/aakashns/rembox/cli/internal/version"
)

type testApp struct {
	*App
	stdout, stderr *bytes.Buffer
	exe            string // fake installed binary
	srv            *releasetest.Server
}

// newTestApp returns an App running as version current, installed at a temp
// path, talking to a fake release server.
func newTestApp(t *testing.T, current string) *testApp {
	t.Helper()
	old := version.Version
	version.Version = current
	t.Cleanup(func() { version.Version = old })

	srv := releasetest.NewServer()
	t.Cleanup(srv.Close)
	rc := release.NewClient()
	rc.BaseURL = srv.BaseURL()
	rc.HTTP.CheckRedirect = nil // the test server is plain HTTP

	exe := filepath.Join(t.TempDir(), "rembox")
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	ta := &testApp{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}, exe: exe, srv: srv}
	ta.App = &App{
		Stdin:      strings.NewReader(""),
		Stdout:     ta.stdout,
		Stderr:     ta.stderr,
		Releases:   rc,
		Executable: func() (string, error) { return exe, nil },
		GOOS:       "linux",
		GOARCH:     "amd64",
	}
	return ta
}

func (ta *testApp) installed(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(ta.exe)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestHelp(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}, {"-h"}, {"--help"}} {
		ta := newTestApp(t, "0.1.0")
		if code := ta.Run(args); code != 0 {
			t.Errorf("rembox %v: exit %d", args, code)
		}
		for _, want := range []string{"Usage:", "help", "version", "upgrade"} {
			if !strings.Contains(ta.stdout.String(), want) {
				t.Errorf("rembox %v: output missing %q:\n%s", args, want, ta.stdout)
			}
		}
	}
}

func TestCommandHelp(t *testing.T) {
	for _, args := range [][]string{{"help", "upgrade"}, {"upgrade", "-h"}, {"upgrade", "--help"}} {
		ta := newTestApp(t, "0.1.0")
		if code := ta.Run(args); code != 0 {
			t.Errorf("rembox %v: exit %d", args, code)
		}
		if !strings.HasPrefix(ta.stdout.String(), "Usage: rembox upgrade") {
			t.Errorf("rembox %v: got %q", args, ta.stdout)
		}
	}
}

func TestVersion(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"-v"}, {"--version"}} {
		ta := newTestApp(t, "1.2.3")
		if code := ta.Run(args); code != 0 {
			t.Errorf("rembox %v: exit %d", args, code)
		}
		if !strings.HasPrefix(ta.stdout.String(), "rembox 1.2.3") {
			t.Errorf("rembox %v: got %q", args, ta.stdout)
		}
	}
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{{"bogus"}, {"--bogus"}, {"version", "extra"}, {"upgrade", "--bogus"}, {"help", "bogus"}} {
		ta := newTestApp(t, "0.1.0")
		if code := ta.Run(args); code != 2 {
			t.Errorf("rembox %v: exit %d, want 2", args, code)
		}
		if !strings.Contains(ta.stderr.String(), "for usage") {
			t.Errorf("rembox %v: stderr %q doesn't point to help", args, ta.stderr)
		}
	}
}

func TestUpgradeConfirmed(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.srv.AddRelease("v0.2.0", "linux", "amd64", []byte("new binary"))
	ta.Interactive = true
	ta.Stdin = strings.NewReader("y\n")

	if code := ta.Run([]string{"upgrade"}); code != 0 {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	if got := ta.installed(t); got != "new binary" {
		t.Errorf("installed binary = %q", got)
	}
	out := ta.stdout.String()
	for _, want := range []string{"0.1.0 -> 0.2.0", "[y/N]", "Upgraded rembox to 0.2.0"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestUpgradeDeclined(t *testing.T) {
	for _, answer := range []string{"n\n", "\n", "", "maybe\n"} {
		ta := newTestApp(t, "0.1.0")
		ta.srv.AddRelease("v0.2.0", "linux", "amd64", []byte("new binary"))
		ta.Interactive = true
		ta.Stdin = strings.NewReader(answer)

		if code := ta.Run([]string{"upgrade"}); code != 0 {
			t.Fatalf("answer %q: exit %d: %s", answer, code, ta.stderr)
		}
		if got := ta.installed(t); got != "old binary" {
			t.Errorf("answer %q: binary was replaced", answer)
		}
		if ta.srv.Downloads() != 0 {
			t.Errorf("answer %q: downloaded before confirmation", answer)
		}
	}
}

func TestUpgradeYesFlag(t *testing.T) {
	for _, flag := range []string{"-y", "--yes"} {
		ta := newTestApp(t, "0.1.0")
		ta.srv.AddRelease("v0.2.0", "linux", "amd64", []byte("new binary"))

		if code := ta.Run([]string{"upgrade", flag}); code != 0 {
			t.Fatalf("%s: exit %d: %s", flag, code, ta.stderr)
		}
		if got := ta.installed(t); got != "new binary" {
			t.Errorf("%s: installed binary = %q", flag, got)
		}
	}
}

func TestUpgradeNonInteractiveNeedsYes(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.srv.AddRelease("v0.2.0", "linux", "amd64", []byte("new binary"))
	ta.Stdin = strings.NewReader("y\n") // piped input isn't a confirmation

	if code := ta.Run([]string{"upgrade"}); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(ta.stderr.String(), "--yes") {
		t.Errorf("stderr doesn't mention --yes: %s", ta.stderr)
	}
	if got := ta.installed(t); got != "old binary" {
		t.Error("binary was replaced")
	}
}

func TestUpgradeUpToDate(t *testing.T) {
	for _, current := range []string{"0.2.0", "0.3.0-rc.1"} {
		ta := newTestApp(t, current)
		ta.srv.AddRelease("v0.2.0", "linux", "amd64", []byte("new binary"))
		ta.Interactive = true

		if code := ta.Run([]string{"upgrade"}); code != 0 {
			t.Fatalf("%s: exit %d: %s", current, code, ta.stderr)
		}
		if strings.Contains(ta.stdout.String(), "[y/N]") || ta.srv.Downloads() != 0 {
			t.Errorf("%s: offered or downloaded an upgrade:\n%s", current, ta.stdout)
		}
	}
}

func TestUpgradeDevBuild(t *testing.T) {
	ta := newTestApp(t, "dev")
	ta.srv.AddRelease("v0.2.0", "linux", "amd64", []byte("new binary"))

	if code := ta.Run([]string{"upgrade", "--yes"}); code != 0 {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	if !strings.Contains(ta.stdout.String(), "development build") {
		t.Errorf("output doesn't mention development build:\n%s", ta.stdout)
	}
	if got := ta.installed(t); got != "new binary" {
		t.Errorf("installed binary = %q", got)
	}
}

func TestUpgradeTampered(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.srv.AddRelease("v0.2.0", "linux", "amd64", []byte("new binary"))
	ta.srv.SetFile("v0.2.0", "rembox-linux-amd64", []byte("evil binary"))

	if code := ta.Run([]string{"upgrade", "--yes"}); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(ta.stderr.String(), "checksum mismatch") {
		t.Errorf("stderr: %s", ta.stderr)
	}
	if got := ta.installed(t); got != "old binary" {
		t.Error("tampered binary was installed")
	}
}

func TestUpgradeNoReleases(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	if code := ta.Run([]string{"upgrade", "--yes"}); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(ta.stderr.String(), "no releases") {
		t.Errorf("stderr: %s", ta.stderr)
	}
}

func TestUpgradeFollowsSymlink(t *testing.T) {
	ta := newTestApp(t, "0.1.0")
	ta.srv.AddRelease("v0.2.0", "linux", "amd64", []byte("new binary"))
	link := filepath.Join(t.TempDir(), "rembox")
	if err := os.Symlink(ta.exe, link); err != nil {
		t.Fatal(err)
	}
	ta.Executable = func() (string, error) { return link, nil }

	if code := ta.Run([]string{"upgrade", "--yes"}); code != 0 {
		t.Fatalf("exit %d: %s", code, ta.stderr)
	}
	if got := ta.installed(t); got != "new binary" {
		t.Errorf("target binary = %q", got)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink was replaced instead of its target")
	}
}

func TestUpgradeUnwritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}
	ta := newTestApp(t, "0.1.0")
	ta.srv.AddRelease("v0.2.0", "linux", "amd64", []byte("new binary"))
	ta.Interactive = true
	ta.Stdin = strings.NewReader("y\n")
	dir := filepath.Dir(ta.exe)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	if code := ta.Run([]string{"upgrade"}); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(ta.stderr.String(), "sudo") {
		t.Errorf("stderr doesn't suggest sudo: %s", ta.stderr)
	}
	if strings.Contains(ta.stdout.String(), "[y/N]") {
		t.Error("asked for confirmation before finding it can't write")
	}
}
