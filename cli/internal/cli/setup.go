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

const setupHelp = `Usage: rembox setup [NAME]

Get a server ready for development. rembox asks which server, the user to log
in as (root, by default), the SSH key to log in with, and a name for the
server (its hostname, by default), and saves it, so 'rembox list' and 'rembox
ssh' know it. Give the NAME of a server you've added to set it up again: its
address, user and key are the defaults, and any you change replace them.

Then rembox logs in with ssh and runs setup.sh on the server, which shows
each command it runs as it:

  1. Checks that rembox can set up the server: so far, Ubuntu or Debian.
  2. Installs the latest version of every package with apt.
  3. Asks to restart the server, if an update needs it.

Unless rembox logs in as root, it runs setup.sh with sudo, which may ask for
the user's password on the server.

Run it again any time, as often as you like: setup.sh looks at the server as
it is now and changes only what needs changing. rembox doesn't keep anything
about the server but how to log in to it.

The first time you connect to a server, ssh asks you to check its host key,
then adds it to ~/.ssh/known_hosts; if the key later changes, ssh refuses to
connect.
`

// defaultKeys are the private keys setup offers, in order of preference.
var defaultKeys = []string{"id_ed25519", "id_ecdsa", "id_rsa"}

func runSetup(a *App, args []string) error {
	var saved *config.Server
	switch {
	case len(args) == 0:
	case args[0] == "-h" || args[0] == "--help":
		return flag.ErrHelp
	case strings.HasPrefix(args[0], "-"):
		return usagef("unknown flag %s", args[0])
	case len(args) > 1:
		return usagef("unexpected argument %q", args[1])
	default:
		s, err := a.loadServer(args[0])
		if err != nil {
			return err
		}
		saved = &s
	}

	s, err := a.askServer(saved)
	if err != nil {
		return err
	}
	dir, err := a.ConfigDir()
	if err != nil {
		return err
	}
	if err := config.SaveServer(dir, s); err != nil {
		return fmt.Errorf("saving %s: %w", s.Name, err)
	}
	a.done(fmt.Sprintf("Saved %s (%s).", s.Name, s.Destination()))

	// One ssh session does everything, so ssh and sudo each ask for a
	// password at most once.
	err = a.runScript(s, asRoot(s, "bash", "-c"), remote.Setup, a.Stdout)
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 255 {
		return fmt.Errorf("%w. To change the address, user or key: rembox setup %s", err, s.Name)
	} else if err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "\n\n%s %s\n", a.paint(green, "==>"),
		a.paint(bold, fmt.Sprintf("%s is set up. To log in: rembox ssh %s", s.Name, s.Name)))
	return nil
}

// askServer asks which server to set up. saved is the server to set up again,
// if any: its settings are the defaults, and it keeps its name.
func (a *App) askServer(saved *config.Server) (config.Server, error) {
	var s config.Server
	dir, err := a.ConfigDir()
	if err != nil {
		return s, err
	}
	home, err := a.HomeDir()
	if err != nil {
		return s, fmt.Errorf("can't find your home folder: %w", err)
	}

	def := config.Server{User: "root", Key: defaultKey(home)}
	if saved != nil {
		def = *saved
		def.Key = tildePath(home, saved.Key)
	}
	s.Host, err = a.answer("Server IP address or hostname", def.Host, checkHost)
	if err != nil {
		return s, err
	}
	s.User, err = a.answer("User to log in as", def.User, checkUser)
	if err != nil {
		return s, err
	}
	s.Key, err = a.answer("SSH private key", def.Key, func(p string) (string, error) {
		return checkKey(home, p)
	})
	if err != nil {
		return s, err
	}
	if saved != nil {
		s.Name = saved.Name
	} else if s.Name, err = a.askName(dir, home, s); err != nil {
		return s, err
	}
	return s, nil
}

// askName asks for a name for s, offering its hostname, and asks before
// replacing another server with that name.
func (a *App) askName(dir, home string, s config.Server) (string, error) {
	def := ""
	if net.ParseIP(s.Host) == nil && config.CheckName(strings.ToLower(s.Host)) == nil {
		def = strings.ToLower(s.Host)
	}
	for {
		name, err := a.answer("Name for this server", def, func(n string) (string, error) {
			return n, config.CheckName(n)
		})
		if err != nil {
			return "", err
		}
		s.Name = name
		saved, err := config.LoadServer(dir, name)
		if errors.Is(err, fs.ErrNotExist) || (err == nil && saved == s) {
			return name, nil // a new server, or the same one again
		}
		if err != nil {
			return "", err
		}
		if a.confirm(fmt.Sprintf("There's already a server named %s, which logs in to %s with %s. Replace it?",
			name, saved.Destination(), tildePath(home, saved.Key)), false) {
			return name, nil
		}
		def = ""
	}
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
