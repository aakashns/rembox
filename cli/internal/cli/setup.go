package cli

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aakashns/rembox/cli/internal/config"
	"github.com/aakashns/rembox/cli/internal/remote"
)

const setupHelp = `Usage: rembox setup

Get a server ready for development, in steps, explaining each one and
showing every command it runs:

  1. Log in    Ask which server, the user to log in as (root, by default),
               the SSH key to log in with, and a name for the server (its
               hostname, by default). Check that ssh can log in, and save
               the server, so 'rembox list' and 'rembox ssh' know it.
               Nothing is saved if ssh can't log in.
  2. Check     See what system, processor, memory and disk the server has,
               and that rembox can set it up: so far, Ubuntu or Debian.
  3. Update    Install the latest version of every package, then restart
               the server if an update needs it. Asks before each. Unless
               it logs in as root, rembox does these with sudo, which may
               ask for the user's password on the server.

Run it again any time, as often as you like: each step looks at the server
as it is now and changes only what needs changing. rembox doesn't keep
anything about the server but how to log in to it.

The first time you connect to a server, ssh asks you to check its host key,
then adds it to ~/.ssh/known_hosts; if the key later changes, ssh refuses to
connect.
`

// setupSteps is how many steps setup has.
const setupSteps = 3

// defaultKeys are the private keys setup offers, in order of preference.
var defaultKeys = []string{"id_ed25519", "id_ecdsa", "id_rsa"}

func runSetup(a *App, args []string) error {
	if err := parseFlags(flag.NewFlagSet("setup", flag.ContinueOnError), args); err != nil {
		return err
	}

	fmt.Fprintln(a.Stdout, wrap(fmt.Sprintf("rembox setup gets a server ready for development, in %d steps: "+
		"log in, check the server, and install updates. It explains each step, and shows every command "+
		"it runs. You can run it again any time.", setupSteps), ""))

	s, err := a.setupLogin()
	if err != nil {
		return err
	}

	a.stage(2, setupSteps, "Check the server",
		"rembox runs a short script on the server that reports its system, processor, memory and disk, "+
			"to see whether rembox can set it up. The script changes nothing, and rembox doesn't keep "+
			"what it finds; it checks again every time.")
	m, err := a.probe(s)
	if err != nil {
		return err
	}
	a.showMachine(m)
	if problems := m.Problems(); len(problems) > 0 {
		for _, p := range problems {
			a.note(p)
		}
		return fmt.Errorf("can't set up %s yet, as the check above says", s.Name)
	}
	a.done("rembox can set up this server.")

	a.stage(3, setupSteps, "Install updates",
		"rembox runs a script on the server that installs the latest version of every package with apt, "+
			"so it starts out with the latest security fixes. It keeps the server's config files as they "+
			"are. If everything is up to date already, it changes nothing.")
	if m.UID != 0 {
		fmt.Fprintln(a.Stdout, "\n"+wrap(fmt.Sprintf("Installing packages needs root, and rembox logged in as %s, "+
			"so it runs the script with sudo, which may ask for %s's password on the server.", m.User, m.User), ""))
	}
	fmt.Fprintln(a.Stdout)
	if !a.confirm(fmt.Sprintf("Install updates on %s now?", s.Name), true) {
		a.note("Skipped installing updates. To install them later, run rembox setup again.")
		return nil
	}
	if err := a.runScript(s, asRoot(s, m, "bash", "-c"), remote.Update, a.Stdout); err != nil {
		return err
	}
	a.done("Every package is up to date.")

	fmt.Fprintln(a.Stdout, "\n"+wrap("Some updates, like a new kernel, only take effect once the server "+
		"restarts. To see if any did, rembox checks the server again:", ""))
	if m, err = a.probe(s); err != nil {
		return err
	}
	if !m.RebootRequired {
		a.done("No restart needed.")
	} else {
		fmt.Fprintln(a.Stdout, "\n"+wrap(fmt.Sprintf("%s needs to restart to finish the updates. Restarting "+
			"ends any sessions on it, and takes a minute or so.", s.Name), ""))
		err := a.restart(s, m)
		switch {
		case errors.Is(err, errDeclined):
			reboot := "systemctl reboot"
			if m.UID != 0 {
				reboot = "sudo " + reboot
			}
			a.note(fmt.Sprintf("Not restarted. To restart %s later: rembox ssh %s %s", s.Name, s.Name, reboot))
		case err != nil:
			return err
		default:
			a.done(fmt.Sprintf("%s is restarting, which takes a minute or so.", s.Name))
		}
	}

	fmt.Fprintf(a.Stdout, "\n\n%s %s\n", a.paint(green, "==>"),
		a.paint(bold, fmt.Sprintf("%s is set up. To log in: rembox ssh %s", s.Name, s.Name)))
	return nil
}

// setupLogin is setup's first step: it asks which server, checks that ssh can
// log in to it, and saves it.
func (a *App) setupLogin() (config.Server, error) {
	var s config.Server
	dir, err := a.ConfigDir()
	if err != nil {
		return s, err
	}
	home, err := a.HomeDir()
	if err != nil {
		return s, fmt.Errorf("can't find your home folder: %w", err)
	}

	a.stage(1, setupSteps, "Log in",
		"rembox logs in to the server with ssh and an SSH key. It asks which server, the user to log in "+
			"as and the key, then checks that ssh can log in, and saves the server so 'rembox list' and "+
			"'rembox ssh' know it. Nothing is saved if ssh can't log in.")
	fmt.Fprintln(a.Stdout)
	s.Host, err = a.answer("Server IP address or hostname", "", checkHost)
	if err != nil {
		return s, err
	}
	s.User, err = a.answer("User to log in as", "root", checkUser)
	if err != nil {
		return s, err
	}
	s.Key, err = a.answer("SSH private key", defaultKey(home), func(p string) (string, error) {
		return checkKey(home, p)
	})
	if err != nil {
		return s, err
	}

	defName := ""
	if net.ParseIP(s.Host) == nil && config.CheckName(strings.ToLower(s.Host)) == nil {
		defName = strings.ToLower(s.Host)
	}
	for {
		s.Name, err = a.answer("Name for this server", defName, func(n string) (string, error) {
			return n, config.CheckName(n)
		})
		if err != nil {
			return s, err
		}
		saved, err := config.LoadServer(dir, s.Name)
		if errors.Is(err, fs.ErrNotExist) || (err == nil && saved == s) {
			break // a new server, or the same one again
		}
		if err != nil {
			return s, err
		}
		if a.confirm(fmt.Sprintf("There's already a server named %s, which logs in to %s with %s. Replace it?",
			s.Name, saved.Destination(), tildePath(home, saved.Key)), false) {
			break
		}
		defName = ""
	}

	fmt.Fprintf(a.Stdout, "\nChecking that ssh can log in to %s:\n", s.Destination())
	if err := a.run("ssh", sshArgs(s, "true")...); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return s, errors.New("can't find ssh; install OpenSSH and run rembox setup again")
		}
		return s, fmt.Errorf("ssh couldn't log in to %s with %s (%v). Check the address, user and key, "+
			"then run rembox setup again. Nothing was saved", s.Destination(), tildePath(home, s.Key), err)
	}
	if err := config.SaveServer(dir, s); err != nil {
		return s, fmt.Errorf("saving %s: %w", s.Name, err)
	}
	a.done(fmt.Sprintf("Logged in, and saved the server as %s (%s).", s.Name, s.Destination()))
	return s, nil
}

// answer asks question until check accepts the answer. check returns the
// answer as rembox keeps it.
func (a *App) answer(question, def string, check func(string) (string, error)) (string, error) {
	for {
		v, err := a.ask(question, def)
		if err != nil {
			return "", err
		}
		if v, err = check(v); err == nil {
			return v, nil
		}
		fmt.Fprintf(a.Stdout, "  %s\n", a.paint(yellow, err.Error()))
	}
}

var (
	hostPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)
	userPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)
)

// checkHost checks an IP address or hostname, and returns it without
// brackets around an IPv6 address.
func checkHost(v string) (string, error) {
	host := strings.TrimSuffix(strings.TrimPrefix(v, "["), "]") // [IPv6]
	switch {
	case host == "":
		return "", errors.New("enter the server's IP address or hostname")
	case strings.Contains(host, "@"):
		return "", errors.New("enter just the IP address or hostname; rembox asks for the user next")
	case net.ParseIP(host) == nil && !hostPattern.MatchString(host):
		return "", fmt.Errorf("%q isn't an IP address or hostname", host)
	}
	return host, nil
}

// checkUser checks a user name to log in as.
func checkUser(user string) (string, error) {
	if !userPattern.MatchString(user) {
		return "", fmt.Errorf("%q isn't a user name", user)
	}
	return user, nil
}

// checkKey checks that path (which may start with ~/) is a private key file,
// and returns its absolute path.
func checkKey(home, path string) (string, error) {
	if path == "" {
		return "", errors.New("enter the path of your SSH private key; to make one, run ssh-keygen -t ed25519")
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		path = filepath.Join(home, path[1:])
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", fmt.Errorf("there's no file at %s", path)
	case err != nil:
		return "", err
	case info.IsDir():
		return "", fmt.Errorf("%s is a folder, not a key", path)
	case strings.HasSuffix(path, ".pub"):
		return "", fmt.Errorf("%s is a public key; give the private key, %s", path, strings.TrimSuffix(path, ".pub"))
	}
	return path, nil
}

// defaultKey is the first of defaultKeys in ~/.ssh, as ~/.ssh/<name>, or "".
func defaultKey(home string) string {
	for _, k := range defaultKeys {
		if info, err := os.Stat(filepath.Join(home, ".ssh", k)); err == nil && info.Mode().IsRegular() {
			return "~/.ssh/" + k
		}
	}
	return ""
}

// keyArgs are the ssh arguments to log in to s with its key.
func keyArgs(s config.Server) []string {
	return []string{
		"-i", s.Key,
		"-o", "IdentitiesOnly=yes", // just this key, not every key in the agent
	}
}

// tildePath shows path with ~ for the home folder.
func tildePath(home, path string) string {
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
		return "~/" + filepath.ToSlash(rel)
	}
	return path
}
