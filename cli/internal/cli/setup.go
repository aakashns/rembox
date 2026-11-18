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
	"text/tabwriter"

	"github.com/aakashns/rembox/cli/internal/config"
	"github.com/aakashns/rembox/cli/internal/remote"
)

const setupHelp = `Usage: rembox setup [NAME]

Get a server ready for development, securely. rembox setup:

  1. Saves your server's details and keeps it up to date
     - asks you for the server's IP/host, SSH port, key file, and username
     - saves it to ~/.ssh/config, so you can connect with ssh <name>
     - logs in, upgrades all packages, and turns on unattended-upgrades
     - adds swap as big as the server's memory (up to 4 GiB), if it has none

  2. Locks down incoming network traffic and SSH access
     - creates a non-root user with password protected sudo access
     - moves SSH from 22 to a random port to counter automated attacks
     - turns off password-based & root SSH access, and turns on fail2ban
     - turns on the ufw firewall to block all incoming traffic except SSH

  3. Installs development tools and configures the shell
     - installs git, gh, ripgrep, fd, fzf, jq, htop, Zellij, PostgreSQL, and
       other dev tools
     - installs the latest Node (with Vite+), bun, Python (with uv), Rust and
       Go
     - makes zsh your shell, with curated plugins to improve your workflow
     - sets your name and email for git, and logs gh in to GitHub with a
       code

  4. Installs AI coding agents and sets up remote access
     - installs & sets up Claude Code, Codex, OpenCode, and Pi coding agents
     - installs Playwright with Chromium, so agents can do browser testing
     - installs opentunnel, so agents can share dev server links on your
       phone
     - installs T3 Code and enables T3 Connect to let you code from your phone

To reach a program on the server, like a dev server or database, forward its
port over ssh: 'rembox tunnel NAME 3000' makes localhost:3000 on this machine
reach port 3000 on the server. Docker's published ports get past ufw,
so setup has Docker publish them on 127.0.0.1 only, except on networks made
before. To open one to the internet: 'docker run -p 0.0.0.0:80:80'.

To test a dev server on your phone, have an agent run 'opentunnel route add
PORT', which prints a public https link to it.

ssh asks you to check the server's host key again the first time it logs in
on the new port. If it can't log in there, open the port in your cloud
provider's firewall, if it has one, then run 'rembox setup NAME' again.

rembox keeps each server as a Host block in ~/.config/rembox/ssh_config, which
you can edit. Once the server is set up, it asks to include that at the top of
~/.ssh/config, so 'ssh NAME', and ssh to the server's address, log in on the
new port as your user, as rembox does.

Give the NAME of a server you've added to set it up again: its address, port,
user, key and name are the defaults, and any you change replace them. Run it any
time, as often as you like: setup.sh looks at the server as it is now and
changes only what needs changing. rembox doesn't keep anything about the
server but how to log in to it.

Unless rembox logs in as root, setup.sh says so, then runs itself with sudo,
which may ask for the user's password on the server: always, once it logs in
as your user.
`

// defaultKeys are the private keys setup offers, in order of preference.
var defaultKeys = []string{"id_ecdsa", "id_ed25519", "id_rsa"}

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
	if err := a.saveServer(s); err != nil {
		return err
	}
	if saved != nil && saved.Name != s.Name {
		if err := a.removeServer(saved.Name); err != nil {
			return err
		}
	}

	// Two sessions, each doing all it can, so ssh and sudo ask for a password
	// at most once each in each. The first asks for your user's name, just
	// before adding it, and the port to move ssh to, just before moving it, and
	// tells rembox both. Both tell it the checklist's lines. ssh stays on its
	// port, unless that's 22.
	newPort := s.Port
	if newPort == config.DefaultPort {
		newPort = a.NewPort()
	}
	out := &markerWriter{w: a.Stdout}
	a.heading(fmt.Sprintf("Connecting to %s as %s, on port %d", s.Name, s.User, s.Port))
	err = a.runScript(s, asUser(s, "bash", "-c"), remote.Setup, out, "start", userDefault(a.Username), strconv.Itoa(newPort), strconv.Itoa(s.Port), s.Name)
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
	p, err := checkPort(out.port)
	if err != nil {
		return fmt.Errorf("setup.sh didn't say which port it moved ssh to on %s (%w)", s.Name, err)
	}
	port := mustAtoi(p)

	// Saved before asking, so if you stop here, setup keeps this port next time.
	oldPort := s.Port
	s.User, s.Port = user, port
	if err := a.saveServer(s); err != nil {
		return err
	}
	if port != oldPort {
		// Port oldPort stays open until rembox has logged in on port, but a
		// firewall outside the server must let port in too, or you're locked out.
		if a.confirm(fmt.Sprintf("Is %s behind a firewall outside it, like your cloud provider's?", s.Name), false) &&
			!a.confirm(fmt.Sprintf("Open port %d/tcp in that firewall now. Have you opened it?", port), false) {
			return fmt.Errorf("ssh is on ports %d and %d for now. Once port %d/tcp is open in the firewall: rembox setup %s",
				oldPort, port, port, s.Name)
		}
	}
	// The second logs in as your user on the new port, which shows both work:
	// first without sudo, to set up your shell, then as root, with sudo.
	onNewPort := func(err error) error {
		if cantLogIn(err) {
			return fmt.Errorf("%w. If %s's cloud provider has a firewall, open port %d/tcp in it, then run rembox setup %s again",
				err, s.Name, port, s.Name)
		}
		return err
	}
	a.heading(fmt.Sprintf("Reconnecting to %s as %s, on port %d, to set up your shell, GitHub, toolchains, coding agents and T3 Code, then finish as root", s.Name, s.User, s.Port))
	home := append([]string{"home", a.GitConfig("user.name"), a.GitConfig("user.email"), strconv.Itoa(port)}, a.Locales...)
	err = a.runScript(s, asUser(s, "bash", "-c"), remote.Setup, out, home...)
	out.flush()
	if err := onNewPort(err); err != nil {
		return err
	}
	if err := a.offerInclude(); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "\n\n%s %s\n\n", a.paint(green, "==>"),
		a.paint(bold, fmt.Sprintf("%s is set up. To log in: rembox ssh %s", s.Name, s.Name)))
	a.printChecklist(out.checks)
	a.printCommands(s.Name)
	return nil
}

// printChecklist shows the checklist's lines, from setup.sh, each "MARK TEXT",
// where MARK is ok, skip or warn, then a blank line, if there are any.
func (a *App) printChecklist(lines []string) {
	marks := map[string]string{"ok": a.paint(green, "✓"), "skip": a.paint(dim, "–"), "warn": a.paint(yellow, "!")}
	for _, line := range lines {
		mark, text, _ := strings.Cut(line, " ")
		if m, ok := marks[mark]; ok {
			fmt.Fprintf(a.Stdout, "  %s %s\n", m, text)
		}
	}
	if len(lines) > 0 {
		fmt.Fprintln(a.Stdout)
	}
}

// printCommands lists the commands for the server called name, once it's set up.
func (a *App) printCommands(name string) {
	tw := tabwriter.NewWriter(a.Stdout, 0, 0, 3, ' ', 0)
	for _, c := range [][2]string{
		{"rembox ssh " + name, "Log in to " + name + ", or run a command on it"},
		{"rembox list", "List your servers"},
		{"rembox setup " + name, "Set " + name + " up again, or change how rembox logs in to it"},
		{"rembox remove " + name, "Remove " + name + " from rembox; the server stays as it is"},
	} {
		fmt.Fprintf(tw, "  %s\t%s\n", a.paint(cyan, c[0]), c[1])
	}
	tw.Flush()
}

// marker starts the lines in which setup.sh tells rembox things, as
// "\x1b]rembox;KEY=VALUE\a": an escape sequence, which terminals ignore.
// KEY is user, for the name of your user, after asking you for it (see
// add_user in user.sh), port, for the port ssh moves to, after asking you for
// it (see ask_port in sshd.sh), or check, for a line of the checklist (see checklist
// in lib.sh).
const marker = "\x1b]rembox;"

// markerWriter passes what setup.sh prints on to w, but for the lines starting
// with marker, from which it keeps the user's name, ssh's port and the
// checklist's lines.
// The rest passes straight through, so questions, which don't end their
// lines, show at once.
type markerWriter struct {
	w       io.Writer
	user    string
	port    string
	checks  []string
	held    []byte // the start of a line that may be a marker's
	midLine bool
}

func (u *markerWriter) Write(p []byte) (int, error) {
	out := make([]byte, 0, len(p))
	for _, b := range p {
		switch {
		case u.held != nil:
			u.held = append(u.held, b)
			if n := len(u.held); n <= len(marker) && b != marker[n-1] {
				out = append(out, u.held...) // not a marker after all
				u.held = nil
			} else if b == '\n' {
				// The terminal ends lines with \r\n.
				line := strings.TrimSuffix(strings.TrimRight(string(u.held[len(marker):]), "\r\n"), "\a")
				switch key, value, _ := strings.Cut(line, "="); key {
				case "user":
					u.user = value
				case "port":
					u.port = value
				case "check":
					u.checks = append(u.checks, value)
				}
				u.held = nil
			}
		case !u.midLine && b == marker[0]:
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
func (u *markerWriter) flush() {
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

// removeServer removes the saved server called name, and says so.
func (a *App) removeServer(name string) error {
	dir, err := a.ConfigDir()
	if err != nil {
		return err
	}
	if err := config.RemoveServer(dir, name); err != nil {
		return fmt.Errorf("removing %s: %w", name, err)
	}
	a.done(fmt.Sprintf("Removed %s, now that it's called something else.", name))
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
	if !a.confirm(fmt.Sprintf("Add '%s' to ~/.ssh/config?", line), true) {
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
	s.Host, err = a.answer("Remote server IP address or hostname", def.Host, checkHost)
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
	s.Key, err = a.answerCompleting("SSH private key", def.Key, func(p string) (string, error) {
		return checkKey(home, p)
	}, func(typed string) (string, []string) {
		return completePath(home, typed)
	})
	if err != nil {
		return s, err
	}
	current := ""
	if saved != nil {
		current = saved.Name
	}
	if s.Name, err = a.askName(dir, home, s, current); err != nil {
		return s, err
	}
	return s, nil
}

// askName asks for a name for s, offering current, the name of the server
// being set up again, if any, or else its hostname. It asks before replacing
// another server with that name.
func (a *App) askName(dir, home string, s config.Server, current string) (string, error) {
	def := current
	if def == "" && net.ParseIP(s.Host) == nil && config.CheckName(strings.ToLower(s.Host)) == nil {
		def = strings.ToLower(s.Host)
	}
	for {
		name, err := a.answer("Name for this server", def, func(n string) (string, error) {
			return n, config.CheckName(n)
		})
		if err != nil {
			return "", err
		}
		if name == current {
			return name, nil
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
	return a.answerCompleting(question, def, check, nil)
}

// answerCompleting is answer, with Tab completing what you've typed, at a
// terminal, with complete, if it isn't nil.
func (a *App) answerCompleting(question, def string, check func(string) (string, error), complete completer) (string, error) {
	for {
		v, err := a.askCompleting(question, def, complete)
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

// completePath completes typed, the start of a file's path (which may start
// with ~/), as a shell does: to the one file or folder it can be, else to
// as much as all of them share, else it lists them. It leaves out public keys,
// and hidden files unless typed names one.
func completePath(home, typed string) (string, []string) {
	if typed == "~" {
		return "~/", nil
	}
	dir, base := typed[:strings.LastIndex(typed, "/")+1], typed[strings.LastIndex(typed, "/")+1:]
	read := dir
	switch {
	case dir == "":
		read = "."
	case strings.HasPrefix(dir, "~/"):
		read = filepath.Join(home, dir[2:])
	}
	entries, err := os.ReadDir(read)
	if err != nil {
		return typed, nil
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, base) || (strings.HasPrefix(name, ".") && !strings.HasPrefix(base, ".")) {
			continue
		}
		if info, err := os.Stat(filepath.Join(read, name)); err == nil && info.IsDir() {
			name += "/"
		} else if strings.HasSuffix(name, ".pub") {
			continue
		}
		names = append(names, name)
	}
	switch len(names) {
	case 0:
		return typed, nil
	case 1:
		return dir + names[0], nil
	}
	shared := names[0]
	for _, n := range names[1:] {
		for !strings.HasPrefix(n, shared) {
			shared = shared[:len(shared)-1]
		}
	}
	if len(shared) > len(base) {
		return dir + shared, nil
	}
	return typed, names
}

// loginArgs are the ssh arguments to log in to s on its port, with its key.
func loginArgs(s config.Server) []string {
	return []string{
		"-p", strconv.Itoa(s.Port),
		"-i", s.Key,
		"-o", "IdentitiesOnly=yes", // just this key, not every key in the agent
		"-o", "ServerAliveInterval=15", // notice within a minute if the server goes away
	}
}

// tildePath shows path with ~ for the home folder.
func tildePath(home, path string) string {
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
		return "~/" + filepath.ToSlash(rel)
	}
	return path
}
