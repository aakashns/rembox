package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/aakashns/rembox/cli/internal/config"
	"github.com/aakashns/rembox/cli/internal/remote"
)

const setupHelp = `Usage: rembox setup [NAME]

Get a server ready for development, securely. rembox asks which server, the
port and user to log in with (22 and root, by default), the SSH key to log in
with, and a name for the server (its hostname, by default), and saves it, so
'rembox list' and 'rembox ssh' know it. It moves ssh to a random port, or
keeps it where it is if that isn't 22.

Then rembox runs setup.sh on the server, in three ssh sessions. First, as
root, as the user you log in as, it:

  1. Checks that rembox can set up the server: so far, Ubuntu or Debian.
  2. Installs the latest version of every package with apt.
  3. Has unattended-upgrades install security updates every day. It doesn't
     restart the server for them; setup asks to, the next time you run it.
  4. Installs tools for development: build-essential, git, gh, curl, wget,
     zip, unzip, jq, ripgrep, fd, htop, tmux, zsh, nano, man-db and dig, and
     fzf and zoxide from their latest releases on GitHub, as apt's are old.
  5. Asks for your user's name (your username on this machine, by default),
     and adds it, if it isn't there yet, with sudo and the SSH keys of the
     user you log in as, and asks you for a password for it, if it has none.
     sudo asks for it, so programs running as your user, like coding agents,
     can't run commands as root without you.
  6. Sets sshd to listen on the new port too, and let in only keys, and ufw,
     if it's on, to let in the new port.

rembox then saves the server with your user and the new port, and asks you to
open the port in your cloud provider's firewall, if it has one, before it
moves ssh there. It logs in as your user on the new port, which shows both
work, and setup.sh, as your user:

  7. Sets up zsh: autosuggestions, syntax highlighting, fzf, zoxide's z, and
     a prompt with the git branch and status. rembox's config is in
     ~/.config/rembox; yours goes in ~/.zshrc, and wins.

Then, as root:

  8. Sets sshd to listen only on the new port, with no root login.
  9. Turns on ufw, the firewall, to block every incoming connection but ssh
     on the new port, over IPv4 and IPv6. It deletes ufw's rules for port 22,
     and asks whether to keep each of its other rules that let something in.
 10. Sets up fail2ban to ban IP addresses that keep failing to log in.
 11. Adds a swap file as big as the server's memory, up to 4 GiB, unless it
     has swap already, and has it swap only once memory runs low.
 12. Adds the locale this machine uses, like en_US.UTF-8, which ssh passes on,
     so programs on the server don't warn that they can't use it.
 13. Makes zsh your user's login shell.
 14. Asks to restart the server, if an update needs it.

To reach a program on the server, like a dev server or database, forward its
port over ssh: 'ssh -L 3000:localhost:3000 NAME' makes localhost:3000 on this
machine reach port 3000 on the server. Docker's published ports get past ufw,
so publish them on 127.0.0.1 only, like 'docker run -p 127.0.0.1:3000:3000'.

ssh asks you to check the server's host key again the first time it logs in
on the new port. If it can't log in there, open the port in your cloud
provider's firewall, if it has one, then run 'rembox setup NAME' again.

rembox keeps a Host block for each server in ~/.config/rembox/ssh_config. Once
the server is set up, it asks to include that at the top of ~/.ssh/config, so
'ssh NAME', and ssh to the server's address, log in on the new port as your
user, as rembox does.

Give the NAME of a server you've added to set it up again: its address, port,
user and key are the defaults, and any you change replace them. Run it any
time, as often as you like: setup.sh looks at the server as it is now and
changes only what needs changing. rembox doesn't keep anything about the
server but how to log in to it.

Unless rembox logs in as root, setup.sh says so, then runs itself with sudo,
which may ask for the user's password on the server: always, once it logs in
as your user.
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
	// Keep ssh on the port it's on, unless that's 22.
	port := s.Port
	if port == config.DefaultPort {
		port = a.NewPort()
	}
	if err := a.saveServer(s); err != nil {
		return err
	}

	// Each session does all it can, so ssh and sudo ask for a password at
	// most once each. The first asks for your user's name, just before adding
	// it, and tells rembox.
	out := &userWriter{w: a.Stdout}
	err = a.runScript(s, asUser(s, "bash", "-c"), remote.Setup, out, "start", userDefault(a.Username), strconv.Itoa(port), strconv.Itoa(s.Port))
	out.flush()
	if cantLogIn(err) {
		return fmt.Errorf("%w. To change the address, port, user or key: rembox setup %s", err, s.Name)
	} else if err != nil {
		return err
	}
	user, err := checkYourUser(out.user)
	if err != nil {
		return fmt.Errorf("setup.sh didn't say which user it added on %s (%w)", s.Name, err)
	}

	// Saved before asking, so if you stop here, setup keeps this port next time.
	oldPort := s.Port
	s.User, s.Port = user, port
	if err := a.saveServer(s); err != nil {
		return err
	}
	if port != oldPort {
		a.note(fmt.Sprintf("If %s's cloud provider has a firewall, open port %d/tcp in it now.", s.Name, port))
		if !a.confirm(fmt.Sprintf("Move ssh to port %d, closing port %d?", port, oldPort), false) {
			return fmt.Errorf("ssh is on ports %d and %d for now. To move it to port %d: rembox setup %s",
				oldPort, port, port, s.Name)
		}
	}
	// The rest logs in as your user on the new port, which shows both work:
	// first without sudo, to set up your shell, then as root.
	onNewPort := func(err error) error {
		if cantLogIn(err) {
			return fmt.Errorf("%w. If %s's cloud provider has a firewall, open port %d/tcp in it, then run rembox setup %s again",
				err, s.Name, port, s.Name)
		}
		return err
	}
	if err := onNewPort(a.runScript(s, asUser(s, "bash", "-c"), remote.Setup, a.Stdout, "home")); err != nil {
		return err
	}
	finish := append([]string{"finish", user, strconv.Itoa(port)}, a.Locales...)
	if err := onNewPort(a.runScript(s, asUser(s, "bash", "-c"), remote.Setup, a.Stdout, finish...)); err != nil {
		return err
	}
	if err := a.offerInclude(); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "\n\n%s %s\n", a.paint(green, "==>"),
		a.paint(bold, fmt.Sprintf("%s is set up. To log in: rembox ssh %s", s.Name, s.Name)))
	return nil
}

// userMarker starts the line in which setup.sh tells rembox the name of your
// user, after asking you for it, as "\x1b]rembox;user=NAME\a": an escape
// sequence, which terminals ignore. See add_user in user.sh.
const userMarker = "\x1b]rembox;user="

// userWriter passes what setup.sh prints on to w, but for the line starting
// with userMarker, from which it keeps the user's name. The rest passes
// straight through, so questions, which don't end their lines, show at once.
type userWriter struct {
	w       io.Writer
	user    string
	held    []byte // the start of a line that may be the marker's
	midLine bool
}

func (u *userWriter) Write(p []byte) (int, error) {
	out := make([]byte, 0, len(p))
	for _, b := range p {
		switch {
		case u.held != nil:
			u.held = append(u.held, b)
			if n := len(u.held); n <= len(userMarker) && b != userMarker[n-1] {
				out = append(out, u.held...) // not the marker after all
				u.held = nil
			} else if b == '\n' {
				// The terminal ends lines with \r\n.
				line := strings.TrimRight(string(u.held[len(userMarker):]), "\r\n")
				u.user = strings.TrimSuffix(line, "\a")
				u.held = nil
			}
		case !u.midLine && b == userMarker[0]:
			u.held = []byte{b}
		default:
			out = append(out, b)
		}
		u.midLine = b != '\n'
	}
	if len(out) > 0 {
		if _, err := u.w.Write(out); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// flush passes on what's held back, once the output has ended.
func (u *userWriter) flush() {
	if len(u.held) > 0 {
		u.w.Write(u.held)
		u.held = nil
	}
}

// localePattern is a UTF-8 locale, as your Mac or a Linux machine names it,
// like en_US.UTF-8 or en_US.utf8.
var localePattern = regexp.MustCompile(`^([a-z]{2,3}_[A-Z]{2})\.(?i:utf-?8)$`)

// sentLocales are the UTF-8 locales in env, a list of KEY=VALUE, that ssh
// passes on to servers (it sends LANG and LC_*), as servers name them, like
// en_US.UTF-8, sorted, once each. Others, like C or macOS's plain UTF-8,
// can't be added to a server, so they're left out.
func sentLocales(env []string) []string {
	var locales []string
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		if k != "LANG" && !strings.HasPrefix(k, "LC_") {
			continue
		}
		if m := localePattern.FindStringSubmatch(v); m != nil {
			locales = append(locales, m[1]+".UTF-8")
		}
	}
	slices.Sort(locales)
	return slices.Compact(locales)
}

// saveServer saves s, and says so.
func (a *App) saveServer(s config.Server) error {
	dir, err := a.ConfigDir()
	if err != nil {
		return err
	}
	if err := config.SaveServer(dir, s); err != nil {
		return fmt.Errorf("saving %s: %w", s.Name, err)
	}
	a.done(fmt.Sprintf("Saved %s (%s, port %d).", s.Name, s.Destination(), s.Port))
	return nil
}

// offerInclude asks to include rembox's ssh config at the top of
// ~/.ssh/config, unless it is already, and adds it if you say yes.
func (a *App) offerInclude() error {
	home, dir, err := a.dirs()
	if err != nil {
		return err
	}
	if ok, err := hasInclude(home, dir); ok || err != nil {
		return err
	}
	line := includeLine(home, dir)
	fmt.Fprintln(a.Stdout)
	if !a.confirm(fmt.Sprintf("Add '%s' to ~/.ssh/config?", line), false) {
		return nil
	}
	if err := addInclude(home, dir); err != nil {
		return err
	}
	a.done(fmt.Sprintf("Added '%s' to ~/.ssh/config.", line))
	return nil
}

// dirs are your home folder and rembox's config folder.
func (a *App) dirs() (home, dir string, err error) {
	if home, err = a.HomeDir(); err != nil {
		return "", "", fmt.Errorf("can't find your home folder: %w", err)
	}
	dir, err = a.ConfigDir()
	return home, dir, err
}

// cantLogIn says whether err is from ssh failing to log in.
func cantLogIn(err error) bool {
	var ee *exec.ExitError
	return errors.As(err, &ee) && ee.ExitCode() == 255
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

	def := config.Server{Port: config.DefaultPort, User: "root", Key: defaultKey(home)}
	if saved != nil {
		def = *saved
		def.Key = tildePath(home, saved.Key)
	}
	s.Host, err = a.answer("Server IP address or hostname", def.Host, checkHost)
	if err != nil {
		return s, err
	}
	port, err := a.answer("SSH port", strconv.Itoa(def.Port), checkPort)
	if err != nil {
		return s, err
	}
	s.Port = mustAtoi(port)
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
	// Your user, as Debian and Ubuntu's useradd takes it.
	yourUserPattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
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
	case hasPort(v):
		return "", errors.New("enter just the IP address or hostname; rembox asks for the port next")
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

// checkYourUser checks the name of your user on the server.
func checkYourUser(user string) (string, error) {
	switch {
	case user == "":
		return "", errors.New("enter a name for your user on the server")
	case user == "root":
		return "", errors.New("pick a user other than root; rembox turns off logging in as root")
	case !yourUserPattern.MatchString(user):
		return "", fmt.Errorf("%q can't be a user on the server; use lowercase letters, digits, '-' and '_', starting with a letter", user)
	}
	return user, nil
}

// userDefault is your username on this machine, if it can be a user on the
// server, or "".
func userDefault(name string) string {
	if _, err := checkYourUser(name); err != nil {
		return ""
	}
	return name
}

// checkPort checks a port number.
func checkPort(v string) (string, error) {
	port, err := strconv.Atoi(v)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("%q isn't a port; enter a number from 1 to 65535", v)
	}
	return strconv.Itoa(port), nil
}

// mustAtoi is the number in s, which a check has accepted.
func mustAtoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return n
}

// hasPort says whether v is a host and port, like box:2222 or [::1]:2222.
func hasPort(v string) bool {
	host, port, err := net.SplitHostPort(v)
	return err == nil && host != "" && port != ""
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

// loginArgs are the ssh arguments to log in to s on its port, with its key.
func loginArgs(s config.Server) []string {
	return []string{
		"-p", strconv.Itoa(s.Port),
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
