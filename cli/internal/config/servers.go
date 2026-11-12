package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Server is a machine set up with `rembox setup`. Each is saved as
// servers/<name>.json under Dir().
type Server struct {
	Name string `json:"-"` // from the file's name
	Host string `json:"host"`
	User string `json:"user"`
	Key  string `json:"key"` // absolute path of the SSH private key
}

// Destination is the server as ssh takes it: user@host.
func (s Server) Destination() string { return s.User + "@" + s.Host }

var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// CheckName says whether name can name a server. Names are lowercase so they
// can't clash on case-insensitive filesystems like macOS's.
func CheckName(name string) error {
	if !validName.MatchString(name) || len(name) > 253 {
		return fmt.Errorf("%q can't be a name; use lowercase letters, digits, '.', '-' and '_', starting with a letter or digit", name)
	}
	return nil
}

func serverPath(dir, name string) string {
	return filepath.Join(dir, "servers", name+".json")
}

// HasServer says whether a server called name is saved under dir.
func HasServer(dir, name string) (bool, error) {
	_, err := os.Stat(serverPath(dir, name))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// SaveServer saves s under dir, replacing any server with its name. The file
// is readable only by you.
func SaveServer(dir string, s Server) error {
	if err := CheckName(s.Name); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	path := serverPath(dir, s.Name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	// Written to a temporary file and renamed, so a failed write doesn't
	// leave half a file in place of a good one.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Servers returns the servers saved under dir, sorted by name. Files it can't
// read are skipped and reported in bad, so one broken file doesn't hide the
// rest.
func Servers(dir string) (servers []Server, bad []error, err error) {
	entries, err := os.ReadDir(filepath.Join(dir, "servers"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || e.IsDir() || CheckName(name) != nil {
			continue
		}
		s, err := readServer(serverPath(dir, name))
		if err != nil {
			bad = append(bad, err)
			continue
		}
		s.Name = name
		servers = append(servers, s)
	}
	sort.Slice(servers, func(i, j int) bool { return servers[i].Name < servers[j].Name })
	return servers, bad, nil
}

func readServer(path string) (Server, error) {
	var s Server
	b, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("reading %s: %w", path, err)
	}
	if s.Host == "" || s.User == "" {
		return s, fmt.Errorf("reading %s: no host or user", path)
	}
	return s, nil
}
