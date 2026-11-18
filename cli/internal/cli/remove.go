package cli

import (
	"flag"
	"fmt"
	"strings"

	"github.com/aakashns/rembox/cli/internal/config"
)

const removeHelp = `Usage: rembox remove NAME

Remove the server called NAME from rembox: its Host block in
~/.config/rembox/ssh_config. It asks first. The server itself stays as it is;
to set it up again: rembox setup.
`

func runRemove(a *App, args []string) error {
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
	if !a.confirm(fmt.Sprintf("Remove %s (%s, port %d) from rembox? The server itself stays as it is.",
		s.Name, s.Destination(), s.Port), false) {
		return errDeclined
	}
	dir, err := a.ConfigDir()
	if err != nil {
		return err
	}
	if err := config.RemoveServer(dir, s.Name); err != nil {
		return fmt.Errorf("removing %s: %w", s.Name, err)
	}
	a.done(fmt.Sprintf("Removed %s.", s.Name))
	return nil
}
