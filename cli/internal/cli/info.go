package cli

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"strings"

	"github.com/aakashns/rembox/cli/internal/config"
)

const infoHelp = `Usage: rembox info NAME

Show what kind of machine the server called NAME is: its system, processor,
memory and disk, and whether rembox can set it up. rembox finds out each
time, with a short script that changes nothing on the server, and doesn't
keep what it finds.
`

func runInfo(a *App, args []string) error {
	switch {
	case len(args) == 0:
		return usagef("give the name of a server; 'rembox list' shows them")
	case args[0] == "-h" || args[0] == "--help":
		return flag.ErrHelp
	case strings.HasPrefix(args[0], "-"):
		return usagef("unknown flag %s", args[0])
	case len(args) > 1:
		return usagef("unexpected argument %q", args[1])
	}
	s, err := a.loadServer(args[0])
	if err != nil {
		return err
	}
	opts, err := a.sshOptions(s)
	if err != nil {
		return err
	}
	m, err := a.probe(s, opts)
	if err != nil {
		return err
	}
	a.showMachine(m)
	if problems := m.Problems(); len(problems) > 0 {
		for _, p := range problems {
			a.note(p)
		}
	} else {
		a.done("rembox can set up this server.")
	}
	return nil
}

// loadServer returns the saved server called name.
func (a *App) loadServer(name string) (config.Server, error) {
	dir, err := a.ConfigDir()
	if err != nil {
		return config.Server{}, err
	}
	s, err := config.LoadServer(dir, name)
	if errors.Is(err, fs.ErrNotExist) {
		return s, fmt.Errorf("there's no server named %s; 'rembox list' shows your servers", name)
	}
	return s, err
}
