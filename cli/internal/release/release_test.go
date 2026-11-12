package release

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aakashns/rembox/cli/internal/release/releasetest"
)

func newTestClient(t *testing.T) (*Client, *releasetest.Server) {
	t.Helper()
	srv := releasetest.NewServer()
	t.Cleanup(srv.Close)
	c := NewClient()
	c.BaseURL = srv.BaseURL()
	c.HTTP.CheckRedirect = nil // the test server is plain HTTP
	return c, srv
}

func TestLatest(t *testing.T) {
	c, srv := newTestClient(t)
	ctx := context.Background()

	if _, err := c.Latest(ctx); !errors.Is(err, ErrNoReleases) {
		t.Fatalf("Latest() with no releases: err = %v, want ErrNoReleases", err)
	}

	srv.SetLatest("v0.2.0")
	if tag, err := c.Latest(ctx); err != nil || tag != "v0.2.0" {
		t.Fatalf("Latest() = %q, %v; want v0.2.0", tag, err)
	}

	for _, bad := range []string{"nightly", "0.2.0", "v0.2", "v0.2.0;rm"} {
		srv.SetLatest(bad)
		if tag, err := c.Latest(ctx); err == nil {
			t.Errorf("Latest() with tag %q = %q, want error", bad, tag)
		}
	}
}

func TestFetchBinary(t *testing.T) {
	c, srv := newTestClient(t)
	srv.AddRelease("v0.2.0", "linux", "amd64", []byte("new binary"))
	ctx := context.Background()

	bin, err := c.FetchBinary(ctx, "v0.2.0", "linux", "amd64")
	if err != nil {
		t.Fatalf("FetchBinary: %v", err)
	}
	if string(bin) != "new binary" {
		t.Errorf("FetchBinary returned %q", bin)
	}

	if _, err := c.FetchBinary(ctx, "v0.2.0", "plan9", "386"); err == nil || !strings.Contains(err.Error(), "no checksum") {
		t.Errorf("unsupported platform: err = %v", err)
	}
	if _, err := c.FetchBinary(ctx, "v9.9.9", "linux", "amd64"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("missing release: err = %v", err)
	}
	if _, err := c.FetchBinary(ctx, "../evil", "linux", "amd64"); err == nil {
		t.Error("bad tag: want error")
	}
}

func TestFetchBinaryChecksumMismatch(t *testing.T) {
	c, srv := newTestClient(t)
	srv.AddRelease("v0.2.0", "linux", "amd64", []byte("new binary"))
	srv.SetFile("v0.2.0", "rembox-linux-amd64", []byte("evil binary"))

	_, err := c.FetchBinary(context.Background(), "v0.2.0", "linux", "amd64")
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v, want checksum mismatch", err)
	}
}

func TestFindChecksum(t *testing.T) {
	sums := []byte("" +
		"0000000000000000000000000000000000000000000000000000000000000001  rembox-linux-amd64\n" +
		"zz  rembox-linux-arm64\n" +
		"0000000000000000000000000000000000000000000000000000000000000002 *rembox-darwin-arm64\n")
	if sum, err := findChecksum(sums, "rembox-linux-amd64"); err != nil || sum[31] != 1 {
		t.Errorf("linux_amd64: %x, %v", sum, err)
	}
	if sum, err := findChecksum(sums, "rembox-darwin-arm64"); err != nil || sum[31] != 2 {
		t.Errorf("darwin_arm64 (binary-mode line): %x, %v", sum, err)
	}
	if _, err := findChecksum(sums, "rembox-linux-arm64"); err == nil {
		t.Error("malformed entry: want error")
	}
	if _, err := findChecksum(sums, "rembox-linux"); err == nil {
		t.Error("partial name match: want error")
	}
}

func TestReplaceExecutable(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "rembox")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceExecutable(exe, []byte("new")); err != nil {
		t.Fatalf("ReplaceExecutable: %v", err)
	}
	got, err := os.ReadFile(exe)
	if err != nil || string(got) != "new" {
		t.Fatalf("after replace: %q, %v", got, err)
	}
	info, _ := os.Stat(exe)
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v, want 0755", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("leftover files in %s: %v", dir, entries)
	}
}
