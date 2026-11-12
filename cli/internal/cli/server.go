package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/aakashns/rembox/cli/internal/config"
	"github.com/aakashns/rembox/cli/internal/remote"
)

// sshArgs are the ssh arguments to run command on s, for setup and info. ssh
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

// asRoot is sshArgs to run command on s as root: as it is if rembox logs in as
// root (m is what the probe found), or else with sudo, at a terminal (-t) so
// sudo can ask for the user's password.
func asRoot(s config.Server, m remote.Machine, command ...string) []string {
	if m.UID == 0 {
		return sshArgs(s, command...)
	}
	return append([]string{"-t"}, sshArgs(s, append([]string{"sudo"}, command...)...)...)
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
		return fmt.Errorf("ssh couldn't connect to %s (%v)", s.Destination(), err)
	case errors.As(err, &ee) && ee.ExitCode() == 127:
		return fmt.Errorf("%s couldn't find a command it needs to run %s, like bash; the output above says which", s.Name, script.Name)
	case errors.As(err, &ee):
		return fmt.Errorf("%s failed on %s (exit status %d); the output above says why", script.Name, s.Name, ee.ExitCode())
	}
	return err
}

// probe runs remote.Probe on s and returns what it found.
func (a *App) probe(s config.Server) (remote.Machine, error) {
	var out bytes.Buffer
	if err := a.runScript(s, sshArgs(s, "bash", "-c"), remote.Probe, &out); err != nil {
		return remote.Machine{}, err
	}
	m, err := remote.Parse(out.String())
	if err != nil {
		return m, fmt.Errorf("couldn't read what %s found: %w", remote.Probe.Name, err)
	}
	return m, nil
}

// showMachine lists what the probe found, one fact to a line.
func (a *App) showMachine(m remote.Machine) {
	fmt.Fprintln(a.Stdout)
	tw := tabwriter.NewWriter(a.Stdout, 0, 0, 2, ' ', 0)
	row := func(label, value string) { fmt.Fprintf(tw, "  %s\t%s\n", label, value) }
	row("System", fmt.Sprintf("%s, kernel %s", orUnknown(m.OSName), m.Kernel))
	row("Processor", fmt.Sprintf("%s, %s", plural(m.CPUs, "core"), m.Arch))
	swap := "no swap"
	if m.SwapKB > 0 {
		swap = gib(m.SwapKB) + " of swap"
	}
	row("Memory", fmt.Sprintf("%s, and %s", gib(m.MemoryKB), swap))
	row("Disk", fmt.Sprintf("%s free of %s", gib(m.DiskFreeKB), gib(m.DiskKB)))
	row("Hostname", m.Hostname)
	switch m.Virt {
	case "none":
		row("Runs on", "its own hardware")
	case "unknown", "":
	default:
		row("Runs on", "a virtual machine ("+m.Virt+")")
	}
	row("Logged in as", m.User)
	if m.RebootRequired {
		row("Restart", a.paint(yellow, "needed, to finish installing updates"))
	}
	tw.Flush()
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

// gib shows a size in KiB as GiB.
func gib(kb int64) string {
	return fmt.Sprintf("%.1f GiB", float64(kb)/(1<<20))
}

// restart restarts s (as root, with sudo if need be), after showing the
// command and asking. m is what the probe found. It returns errDeclined if the
// answer is no.
func (a *App) restart(s config.Server, m remote.Machine) error {
	err := a.runConfirmed("ssh", asRoot(s, m, "systemctl", "reboot")...)
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 255 {
		return nil // the server closed the connection as it shut down
	}
	return err
}
