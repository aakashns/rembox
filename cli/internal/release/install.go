package release

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// CheckWritable reports whether files can be created in dir, which
// ReplaceExecutable needs.
func CheckWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".rembox-check-*")
	if err != nil {
		return err
	}
	f.Close()
	return os.Remove(f.Name())
}

// ReplaceExecutable atomically replaces the file at exe with bin.
//
// The new binary is written next to the old one and renamed over it. Renaming
// (rather than rewriting the file in place) matters on macOS: the kernel
// caches code signatures per file, and a binary modified in place gets killed
// on its next launch.
func ReplaceExecutable(exe string, bin []byte) (err error) {
	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".rembox-upgrade-*")
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return fmt.Errorf("can't write to %s; run it as a user who can, e.g. with sudo", dir)
		}
		return err
	}
	defer func() {
		if err != nil {
			os.Remove(tmp.Name())
		}
	}()

	if _, err = tmp.Write(bin); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), exe)
}
