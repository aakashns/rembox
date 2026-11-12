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
)

const setupHelp = `Usage: rembox setup [--host HOST] [--key PATH] [--name NAME]

Add a server to rembox. Asks for the server's IP address or hostname, the SSH
key to log in with, and a name for it (its hostname, by default). Then it
checks that ssh can log in with that key, and saves the server so that
'rembox list' shows it. Nothing is saved if ssh can't log in.

rembox logs in as root unless you give a user, as in ubuntu@203.0.113.10.
The first time you connect to a server, ssh adds its host key to
~/.ssh/known_hosts; if the key later changes, ssh refuses to connect.

Flags answer the questions up front. Without a terminal, setup uses the
defaults for any question a flag doesn't answer, and fails if there isn't one.

Flags:
  --host HOST   The server's IP address or hostname, or user@host
  --key PATH    The SSH private key to log in with (default ~/.ssh/id_ed25519,
                or another key in ~/.ssh)
  --name NAME   What to call the server in rembox
`

// defaultKeys are the private keys setup offers, in order of preference.
var defaultKeys = []string{"id_ed25519", "id_ecdsa", "id_rsa"}

func runSetup(a *App, args []string) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	var hostFlag, keyFlag, nameFlag string
	fs.StringVar(&hostFlag, "host", "", "")
	fs.StringVar(&keyFlag, "key", "", "")
	fs.StringVar(&nameFlag, "name", "", "")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	dir, err := a.ConfigDir()
	if err != nil {
		return err
	}
	home, err := a.HomeDir()
	if err != nil {
		return fmt.Errorf("can't find your home folder: %w", err)
	}

	dest, err := a.answer("host", hostFlag,
		"Server IP address or hostname (as user@host to log in as someone other than root)", "",
		checkDestination)
	if err != nil {
		return err
	}
	var s config.Server
	s.User, s.Host, _ = strings.Cut(dest, "@")

	s.Key, err = a.answer("key", keyFlag, "SSH private key", defaultKey(home), func(p string) (string, error) {
		return checkKey(home, p)
	})
	if err != nil {
		return err
	}

	defName := ""
	if net.ParseIP(s.Host) == nil && config.CheckName(strings.ToLower(s.Host)) == nil {
		defName = strings.ToLower(s.Host)
	}
	for {
		s.Name, err = a.answer("name", nameFlag, "Name for this server", defName, func(n string) (string, error) {
			return n, config.CheckName(n)
		})
		if err != nil {
			return err
		}
		taken, err := config.HasServer(dir, s.Name)
		if err != nil {
			return err
		}
		if !taken {
			break
		}
		if !a.Interactive {
			return fmt.Errorf("there's already a server named %s; pick another --name", s.Name)
		}
		replace, err := a.confirm(fmt.Sprintf("There's already a server named %s. Replace it?", s.Name), false)
		if err != nil {
			return err
		}
		if replace {
			break
		}
		nameFlag, defName = "", ""
	}

	fmt.Fprintf(a.Stdout, "Checking that ssh can log in to %s...\n", s.Destination())
	if err := a.run("ssh", sshArgs(s, a.Interactive)...); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return errors.New("can't find ssh; install OpenSSH and run rembox setup again")
		}
		return fmt.Errorf("ssh couldn't log in to %s with %s (%v). Check the address, user and key, "+
			"then run rembox setup again. Nothing was saved", s.Destination(), tildePath(home, s.Key), err)
	}
	if err := config.SaveServer(dir, s); err != nil {
		return fmt.Errorf("saving %s: %w", s.Name, err)
	}
	fmt.Fprintf(a.Stdout, "Logged in. Saved %s (%s); 'rembox list' shows your servers.\n", s.Name, s.Destination())
	return nil
}

// answer returns the flag's value if it was given, or else asks question
// until check accepts the answer. check returns the answer as rembox keeps it.
// Without a terminal, it uses def, or fails if there isn't one.
func (a *App) answer(flagName, flagValue, question, def string, check func(string) (string, error)) (string, error) {
	if flagValue != "" {
		v, err := check(flagValue)
		if err != nil {
			return "", fmt.Errorf("--%s: %w", flagName, err)
		}
		return v, nil
	}
	if !a.Interactive {
		if def == "" {
			return "", fmt.Errorf("can't ask for the %s without a terminal; pass --%s", flagName, flagName)
		}
		return check(def)
	}
	for {
		v, err := a.ask(question, def)
		if err != nil {
			return "", err
		}
		if v, err = check(v); err == nil {
			return v, nil
		}
		fmt.Fprintf(a.Stdout, "  %v\n", err)
	}
}

var (
	hostPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)
	userPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)
)

// checkDestination checks a host or user@host, and returns it as user@host.
func checkDestination(v string) (string, error) {
	user, host, ok := strings.Cut(v, "@")
	if !ok {
		user, host = "root", v
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]") // [IPv6]
	switch {
	case host == "":
		return "", errors.New("enter the server's IP address or hostname")
	case net.ParseIP(host) == nil && !hostPattern.MatchString(host):
		return "", fmt.Errorf("%q isn't an IP address or hostname", host)
	case !userPattern.MatchString(user):
		return "", fmt.Errorf("%q isn't a user name", user)
	}
	return user + "@" + host, nil
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

// sshArgs are the arguments for ssh to log in to s with its key and do
// nothing, to check that it can.
func sshArgs(s config.Server, interactive bool) []string {
	args := []string{
		"-i", s.Key,
		"-o", "IdentitiesOnly=yes", // just this key, not every key in the agent
		"-o", "PasswordAuthentication=no",
		"-o", "KbdInteractiveAuthentication=no",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=15",
	}
	if !interactive {
		args = append(args, "-o", "BatchMode=yes") // no one to type a key's passphrase
	}
	return append(args, "--", s.Destination(), "true")
}

// tildePath shows path with ~ for the home folder.
func tildePath(home, path string) string {
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
		return "~/" + filepath.ToSlash(rel)
	}
	return path
}
