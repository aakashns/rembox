package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/aakashns/rembox/cli/internal/config"
	"github.com/aakashns/rembox/cli/internal/remote"
)

// sshOptions are the ssh options for the commands setup and info run on s:
// log in with its key and nothing else, and share one connection between
// them, so ssh asks for a key's passphrase once rather than for each command.
func (a *App) sshOptions(s config.Server) ([]string, error) {
	opts := append(keyArgs(s),
		"-o", "PasswordAuthentication=no",
		"-o", "KbdInteractiveAuthentication=no",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=15",
		"-o", "ServerAliveInterval=15", // notice within a minute if the server goes away
		// Rather than your Mac's locale, which ssh passes on and a new server
		// may not have, so every shell on it warns about it.
		"-o", "SetEnv=LC_ALL=C.UTF-8",
	)
	if !a.Interactive {
		opts = append(opts, "-o", "BatchMode=yes") // no one to type a key's passphrase
	}
	dir, err := a.ConfigDir()
	if err != nil {
		return nil, err
	}
	// ssh names each connection's socket with %C, 40 characters, and adds up
	// to 17 more while it sets it up; macOS allows 103 in all.
	sockets := filepath.Join(dir, "ssh")
	if len(sockets)+1+40+17 > 103 {
		return opts, nil // too long to share connections; ssh works without
	}
	if err := os.MkdirAll(sockets, 0o700); err != nil {
		return nil, err
	}
	return append(opts,
		"-o", "ControlMaster=auto",
		"-o", "ControlPath="+filepath.Join(sockets, "%C"),
		"-o", "ControlPersist=60", // closes a minute after rembox's last command
	), nil
}

// runScript sends script to bash on s over ssh, with its output going to
// stdout, after showing the command. color says whether the script should
// color its output.
func (a *App) runScript(s config.Server, opts []string, script remote.Script, stdout io.Writer, color bool) error {
	args := append(slices.Clone(opts), "--", s.Destination(), "bash", "-s")
	if color {
		args = append(args, "--", "--color")
	}
	fmt.Fprintf(a.Stdout, "\n%s\n%s\n", a.paint(cyan, commandLine("ssh", args)+" < "+script.Name),
		a.paint(dim, "  "+script.Name+" comes with rembox; read it at "+script.URL()))
	return scriptErr(s, script, a.Pipe(strings.NewReader(script.Body), stdout, a.Stderr, "ssh", args...))
}

// runAsRoot runs script on s as root, with its output going to the terminal,
// after showing the command. m is what the probe found. If rembox logs in as
// root, it's the same as runScript. If not, the script runs with sudo, which
// may ask for the user's password at the terminal; so the script can't come
// in on stdin, and goes in the command instead.
func (a *App) runAsRoot(s config.Server, opts []string, m remote.Machine, script remote.Script) error {
	if m.UID == 0 {
		return a.runScript(s, opts, script, a.Stdout, a.Color)
	}
	opts, sudo := a.sudo(opts, m)
	args := append(append(opts, "--", s.Destination()), append(sudo, "bash", "-c")...)
	after := []string{"bash"} // the script's $0
	if a.Color {
		after = append(after, "--color")
	}
	fmt.Fprintf(a.Stdout, "\n%s\n%s\n", a.paint(cyan, commandLine("ssh", args)+` "$(cat `+script.Name+`)" `+strings.Join(after, " ")),
		a.paint(dim, wrap(script.Name+" comes with rembox; read it at "+script.URL()+
			" (rembox quotes it, so the server's shell passes it to bash as it is)", "  ")))
	args = append(append(args, shellQuote(script.Body)), after...)
	return scriptErr(s, script, a.Command("ssh", args...))
}

// sudo returns the ssh options, and the start of a command, to run a command
// on the server as root: nothing more as root; otherwise sudo, at a terminal
// (-t), so it can ask for the user's password. Without a terminal, sudo -n
// fails rather than ask.
func (a *App) sudo(opts []string, m remote.Machine) ([]string, []string) {
	opts = slices.Clone(opts)
	switch {
	case m.UID == 0:
		return opts, nil
	case a.Interactive:
		return append(opts, "-t"), []string{"sudo"}
	}
	return opts, []string{"sudo", "-n"}
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
func (a *App) probe(s config.Server, opts []string) (remote.Machine, error) {
	var out bytes.Buffer
	if err := a.runScript(s, opts, remote.Probe, &out, false); err != nil {
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
// command and asking (unless yes), and waits for it to start again. m is what
// the probe found before the restart. It returns errDeclined if the answer is
// no.
func (a *App) restart(s config.Server, opts []string, m remote.Machine, yes bool) error {
	rootOpts, sudo := a.sudo(opts, m)
	args := append(append(rootOpts, "--", s.Destination()), append(sudo, "systemctl", "reboot")...)
	err := a.runConfirmed(yes, "ssh", args...)
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 255 {
		err = nil // the server closed the connection as it shut down
	}
	if err != nil {
		return err
	}

	const every, tries = 5 * time.Second, 60
	args = append(slices.Clone(opts), "--", s.Destination(), "cat", "/proc/sys/kernel/random/boot_id")
	fmt.Fprintf(a.Stdout, "\nWaiting for %s to start again. Every %s, for up to %s, rembox runs this\n"+
		"to see if it has (a server's boot ID changes each time it starts):\n", s.Name, every, every*tries)
	a.showCommand("ssh", args)
	fmt.Fprint(a.Stdout, "\n  ")
	start := a.Now()
	for range tries {
		a.Sleep(every)
		var out bytes.Buffer
		err := a.Pipe(nil, &out, io.Discard, "ssh", args...)
		if id := strings.TrimSpace(out.String()); err == nil && id != "" && id != m.BootID {
			fmt.Fprintln(a.Stdout)
			a.done(fmt.Sprintf("%s has restarted, after %s.", s.Name, a.Now().Sub(start).Round(time.Second)))
			return nil
		}
		fmt.Fprint(a.Stdout, ".")
	}
	fmt.Fprintln(a.Stdout)
	return fmt.Errorf("%s didn't start again within %s; look at it in your provider's console, "+
		"then run rembox setup again", s.Name, every*tries)
}
