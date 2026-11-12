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

const sshHelp = `Usage: rembox ssh [NAME] [COMMAND...]

Log in to a server added with 'rembox setup', as its user and with its SSH
key. NAME is optional if you've added just one server. Give a COMMAND to run
it on the server instead of logging in, as with ssh. rembox exits with ssh's
exit status.
`

func runSSH(a *App, args []string) error {
	if len(args) > 0 {
		switch {
		case args[0] == "-h" || args[0] == "--help":
			return flag.ErrHelp
		case strings.HasPrefix(args[0], "-"):
			return usagef("unknown flag %s", args[0])
		}
	}
	dir, err := a.ConfigDir()
	if err != nil {
		return err
	}
	var s config.Server
	if len(args) == 0 {
		if s, err = onlyServer(dir); err != nil {
			return err
		}
	} else {
		s, err = config.LoadServer(dir, args[0])
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("there's no server named %s; 'rembox list' shows your servers", args[0])
		}
		if err != nil {
			return err
		}
		args = args[1:]
	}

	sshArgs := append(keyArgs(s), "--", s.Destination())
	err = a.run("ssh", append(sshArgs, args...)...)
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

// onlyServer returns the one server saved under dir, for when no name is given.
func onlyServer(dir string) (config.Server, error) {
	servers, _, err := config.Servers(dir)
	if err != nil {
		return config.Server{}, fmt.Errorf("reading your servers: %w", err)
	}
	switch len(servers) {
	case 0:
		return config.Server{}, errors.New("no servers yet; run 'rembox setup' to add one")
	case 1:
		return servers[0], nil
	}
	names := make([]string, len(servers))
	for i, s := range servers {
		names[i] = s.Name
	}
	return config.Server{}, usagef("which server? You've added %s", strings.Join(names, ", "))
}
