// Package config is where rembox keeps its files on this machine.
//
// Everything rembox stores locally goes under Dir(), so `rembox implode` can
// remove all of it (and back it up first).
package config

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Dir is $XDG_CONFIG_HOME/rembox, or ~/.config/rembox. (Not macOS's
// ~/Library/Application Support: command-line tools are easier to find and
// back up in ~/.config.)
func Dir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "rembox"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("can't find your home folder: %w", err)
	}
	return filepath.Join(home, ".config", "rembox"), nil
}

// CountFiles returns how many files (not folders) are under dir, and whether
// dir exists at all.
func CountFiles(dir string) (n int, exists bool, err error) {
	if _, err := os.Lstat(dir); errors.Is(err, fs.ErrNotExist) {
		return 0, false, nil
	}
	err = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			n++
		}
		return nil
	})
	return n, true, err
}

// Backup writes dir to a new .tar.gz at dest, readable only by you since it
// may hold secrets. Paths in it start with dir's name (e.g. "rembox/..."), and
// symlinks are stored as links, not followed.
func Backup(dir, dest string) (err error) {
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(dest)
		}
	}()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	parent := filepath.Dir(dir)
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		link := ""
		if info.Mode()&fs.ModeSymlink != 0 {
			if link, err = os.Readlink(path); err != nil {
				return err
			}
		}
		hdr, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(parent, path)
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if d.IsDir() {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(tw, src)
		return err
	})
	if err != nil {
		return fmt.Errorf("backing up %s: %w", dir, err)
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}
