package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SSHConfig is the ssh config rembox writes under dir: a Host block for each
// server, so `ssh NAME` (or ssh to its address) logs in as rembox does. rembox
// rewrites it whenever it saves a server; ~/.ssh/config includes it if you
// agree.
func SSHConfig(dir string) string { return filepath.Join(dir, "ssh_config") }

func writeSSHConfig(dir string) error {
	servers, _, err := Servers(dir) // files it can't read are left out
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# Written by rembox from the servers you've added, and rewritten when you add or change one.\n")
	for _, s := range servers {
		hosts := s.Name
		if !strings.EqualFold(s.Host, s.Name) {
			hosts += " " + s.Host
		}
		fmt.Fprintf(&b, "\nHost %s\n  HostName %s\n  Port %d\n  User %s\n  IdentityFile %s\n  IdentitiesOnly yes\n",
			hosts, s.Host, s.Port, s.User, QuoteSSH(s.Key))
	}
	return WriteFile(SSHConfig(dir), []byte(b.String()), 0o600)
}

// QuoteSSH quotes s for an ssh config file, if it has spaces.
func QuoteSSH(s string) string {
	if strings.ContainsAny(s, " \t") {
		return `"` + s + `"`
	}
	return s
}
