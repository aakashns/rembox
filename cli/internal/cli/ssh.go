package cli

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os/exec"
	"strings"

	"github.com/aakashns/rembox/cli/internal/config"
)

const sshHelp = `Usage: rembox ssh NAME [COMMAND...]

Log in to the server called NAME, added with 'rembox setup', as its user and
with its SSH key. Give a COMMAND to run it on the server instead of logging
in, as with ssh. rembox exits with ssh's exit status.
`

func runSSH(a *App, args []string) error {
	switch {
	case len(args) == 0:
		return usagef("give the name of a server; 'rembox list' shows them")
	case args[0] == "-h" || args[0] == "--help":
		return flag.ErrHelp
	case strings.HasPrefix(args[0], "-"):
		return usagef("unknown flag %s", args[0])
	}
	dir, err := a.ConfigDir()
	if err != nil {
		return err
	}
	name, command := args[0], args[1:]
	s, err := config.LoadServer(dir, name)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("there's no server named %s; 'rembox list' shows your servers", name)
	}
	if err != nil {
		return err
	}

	sshArgs := append(keyArgs(s), "--", s.Destination())
	err = a.run("ssh", append(sshArgs, command...)...)
	var ee *exec.ExitError
	switch {
	case errors.Is(err, exec.ErrNotFound):
		return errors.New("can't find ssh; install OpenSSH and try again")
	case errors.As(err, &ee) && ee.ExitCode() > 0:
		// ssh, or the command on the server, already said what went wrong.
		return exitStatus(ee.ExitCode())
	}
	return err
}
