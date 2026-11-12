package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os/exec"
	"slices"
	"strings"

	"github.com/aakashns/rembox/cli/internal/config"
	"github.com/aakashns/rembox/cli/internal/remote"
)

// sshArgs are the ssh arguments to run command on s, for setup. ssh
// logs in with s's key and nothing else, so a key that doesn't work fails
// rather than ssh asking for a password.
func sshArgs(s config.Server, command ...string) []string {
	args := append(keyArgs(s),
		"-o", "PasswordAuthentication=no",
		"-o", "KbdInteractiveAuthentication=no",
		"-o", "ConnectTimeout=15",
		"-o", "ServerAliveInterval=15", // notice within a minute if the server goes away
		// Rather than your Mac's locale, which ssh passes on and a new server
		// may not have, so every shell on it warns about it.
		"-o", "SetEnv=LC_ALL=C.UTF-8",
		"--", s.Destination(),
	)
	return append(args, command...)
}

// asRoot is sshArgs to run command on s as root (with sudo unless rembox logs
// in as root), at a terminal (-t), so sudo and the command can ask questions.
func asRoot(s config.Server, command ...string) []string {
	if s.User != "root" {
		command = append([]string{"sudo"}, command...)
	}
	return append([]string{"-t"}, sshArgs(s, command...)...)
}

// runScript runs script on s, after showing the command, with its output
// going to stdout. args are the ssh arguments up to "bash -c", from sshArgs or
// asRoot. The script goes in the command, quoted, so the server's shell passes
// it to bash as it is.
func (a *App) runScript(s config.Server, args []string, script remote.Script, stdout io.Writer) error {
	after := []string{"bash"} // the script's $0
	if a.Color {
		after = append(after, "--color")
	}
	fmt.Fprintf(a.Stdout, "\n%s\n%s\n", a.paint(cyan, commandLine("ssh", args)+` "$(cat `+script.Name+`)" `+strings.Join(after, " ")),
		a.paint(dim, "  "+script.Name+" comes with rembox; read it at "+script.URL()))
	args = append(append(slices.Clone(args), shellQuote(script.Body)), after...)
	return scriptErr(s, script, a.Command(stdout, "ssh", args...))
}

// scriptErr explains err, from running script on s.
func scriptErr(s config.Server, script remote.Script, err error) error {
	var ee *exec.ExitError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, exec.ErrNotFound):
		return errors.New("can't find ssh; install OpenSSH and try again")
	case errors.As(err, &ee) && ee.ExitCode() == 255:
		return fmt.Errorf("ssh couldn't connect to %s (%w)", s.Destination(), err)
	case errors.As(err, &ee) && ee.ExitCode() == 127:
		return fmt.Errorf("%s couldn't find a command it needs to run %s, like bash or sudo; the output above says which", s.Name, script.Name)
	case errors.As(err, &ee):
		return fmt.Errorf("%s failed on %s (exit status %d); the output above says why", script.Name, s.Name, ee.ExitCode())
	}
	return err
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
