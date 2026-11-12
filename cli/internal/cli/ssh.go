package cli

import (
	"errors"
	"flag"
	"os/exec"
	"strings"
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
	s, err := a.loadServer(args[0])
	if err != nil {
		return err
	}
	command := args[1:]

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
