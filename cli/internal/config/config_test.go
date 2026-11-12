package config

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestBackupSkipsSockets checks a backup works while ssh has a socket in
// rembox's folder, for a connection it's sharing.
func TestBackupSkipsSockets(t *testing.T) {
	// Short, as sockets' paths must be.
	tmp, err := os.MkdirTemp("/tmp", "rb")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	dir := filepath.Join(tmp, "rembox")
	if err := SaveServer(dir, Server{Name: "box", Host: "h", User: "root", Key: "/k"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("unix", filepath.Join(dir, "ssh", "s"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	dest := filepath.Join(tmp, "backup.tar.gz")
	if err := Backup(dir, dest); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
	}
	want := []string{"rembox/", "rembox/servers/", "rembox/servers/box.json", "rembox/ssh/"}
	if !slices.Equal(names, want) {
		t.Errorf("backed up %q, want %q", names, want)
	}
}
