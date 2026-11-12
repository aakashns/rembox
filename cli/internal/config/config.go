// Package config is where rembox keeps its files on this machine.
//
// Everything rembox stores locally goes under Dir(), so `rembox implode` can
// remove all of it.
package config

import (
	"fmt"
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
