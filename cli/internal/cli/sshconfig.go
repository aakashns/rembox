package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/aakashns/rembox/cli/internal/config"
)

// userSSHConfig is ~/.ssh/config, or the file it links to (as when a dotfiles
// manager links it), so rembox changes the file rather than the link.
func userSSHConfig(home string) (string, error) {
	path := filepath.Join(home, ".ssh", "config")
	real, err := filepath.EvalSymlinks(path)
	if errors.Is(err, fs.ErrNotExist) {
		return path, nil
	}
	return real, err
}

// includeLine is the line in ~/.ssh/config that includes rembox's ssh config.
func includeLine(home, dir string) string {
	return "Include " + config.QuoteSSH(tildePath(home, config.SSHConfig(dir)))
}

// hasInclude says whether ~/.ssh/config has includeLine.
func hasInclude(home, dir string) (bool, error) {
	lines, _, err := readSSHConfig(home)
	return containsLine(lines, includeLine(home, dir)), err
}

// addInclude puts includeLine at the top of ~/.ssh/config (making it if need
// be), so rembox's Host blocks come before yours, like a Host * at the end.
func addInclude(home, dir string) error {
	lines, mode, err := readSSHConfig(home)
	if err != nil {
		return err
	}
	if len(lines) > 0 && lines[0] != "" {
		lines = append([]string{""}, lines...)
	}
	return writeSSHConfig(home, append([]string{includeLine(home, dir)}, lines...), mode)
}

// removeInclude takes includeLine out of ~/.ssh/config, with the blank line
// addInclude put after it. It says whether there was one.
func removeInclude(home, dir string) (bool, error) {
	lines, mode, err := readSSHConfig(home)
	if err != nil || !containsLine(lines, includeLine(home, dir)) {
		return false, err
	}
	var kept []string
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == includeLine(home, dir) {
			if i+1 < len(lines) && lines[i+1] == "" {
				i++
			}
			continue
		}
		kept = append(kept, lines[i])
	}
	return true, writeSSHConfig(home, kept, mode)
}

// readSSHConfig returns the lines of ~/.ssh/config, and its permissions.
func readSSHConfig(home string) ([]string, fs.FileMode, error) {
	path, err := userSSHConfig(home)
	if err != nil {
		return nil, 0, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0o600, nil
	} else if err != nil {
		return nil, 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, err
	}
	text := strings.TrimSuffix(string(b), "\n")
	if text == "" {
		return nil, info.Mode().Perm(), nil
	}
	return strings.Split(text, "\n"), info.Mode().Perm(), nil
}

func writeSSHConfig(home string, lines []string, mode fs.FileMode) error {
	path, err := userSSHConfig(home)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		return err
	}
	if err := config.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), mode); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

func containsLine(lines []string, line string) bool {
	for _, l := range lines {
		if strings.TrimSpace(l) == line {
			return true
		}
	}
	return false
}
