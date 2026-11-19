package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/kevinburke/ssh_config"
)

// Server is a machine set up with `rembox setup`. Each is a Host block in
// SSHConfig(dir), named by the block's first pattern.
type Server struct {
	Name string
	Host string // HostName
	Port int    // ssh's port
	User string
	Key  string // IdentityFile: the SSH private key's path
}

// DefaultPort is ssh's usual port, and the port of servers without one.
const DefaultPort = 22

// Destination is the server as ssh takes it: user@host.
func (s Server) Destination() string { return s.User + "@" + s.Host }

var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// CheckName says whether name can name a server. Names are lowercase, as ssh
// matches Host patterns whatever their case.
func CheckName(name string) error {
	if !validName.MatchString(name) || len(name) > 253 {
		return fmt.Errorf("%q can't be a name; use lowercase letters, digits, '.', '-' and '_', starting with a letter or digit", name)
	}
	return nil
}

// SSHConfig is where rembox keeps your servers under dir: a Host block for
// each, so `ssh NAME` (or ssh to its address) logs in as rembox does.
// ~/.ssh/config includes it if you agree.
func SSHConfig(dir string) string { return filepath.Join(dir, "ssh_config") }

const sshConfigHeader = `# Your rembox servers, a Host block each. 'rembox setup' writes a server's block
# afresh, and 'rembox remove' removes it; the rest stays as you left it.
`

// entry is a server's Host block, as read: the server, or why it can't be.
type entry struct {
	server Server
	err    error
}

// decodeSSHConfig parses SSHConfig(dir), or returns nil if there isn't one.
func decodeSSHConfig(dir string) (*ssh_config.Config, error) {
	path := SSHConfig(dir)
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	cfg, err := ssh_config.DecodeBytes(b)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return cfg, nil
}

// serverName is the name of the server h is the Host block of: its first
// pattern. It's "" if h isn't one, like Host * or Match, which aren't servers
// rembox added.
func serverName(h *ssh_config.Host) string {
	if len(h.Patterns) == 0 || strings.HasPrefix(strings.TrimSpace(h.String()), "Match") {
		return ""
	}
	if name := h.Patterns[0].String(); CheckName(name) == nil {
		return name
	}
	return ""
}

// readSSHConfig reads the servers' Host blocks in SSHConfig(dir), in order.
func readSSHConfig(dir string) ([]entry, error) {
	cfg, err := decodeSSHConfig(dir)
	if cfg == nil {
		return nil, err
	}
	path := SSHConfig(dir)
	var entries []entry
	seen := map[string]bool{}
	for _, h := range cfg.Hosts {
		name := serverName(h)
		if name == "" {
			continue
		}
		s, err := parseServer(name, h)
		if err == nil && seen[name] {
			err = errors.New("there's a Host block for it above")
		}
		if err != nil {
			err = fmt.Errorf("%s in %s: %w", name, path, err)
		}
		seen[name] = true
		entries = append(entries, entry{s, err})
	}
	return entries, nil
}

// parseServer reads the server called name from its Host block. As in ssh,
// the first value of each option counts, HostName defaults to the name, and
// Port to 22.
func parseServer(name string, h *ssh_config.Host) (Server, error) {
	s := Server{Name: name}
	port := ""
	for _, n := range h.Nodes {
		kv, ok := n.(*ssh_config.KV)
		if !ok {
			continue
		}
		var v *string
		switch strings.ToLower(kv.Key) {
		case "hostname":
			v = &s.Host
		case "port":
			v = &port
		case "user":
			v = &s.User
		case "identityfile":
			v = &s.Key
		default:
			continue
		}
		if *v == "" {
			*v = kv.Value
		}
	}
	if s.Host == "" {
		s.Host = name
	}
	s.Port = DefaultPort
	if port != "" {
		p, err := strconv.Atoi(port)
		if err != nil || p < 1 || p > 65535 {
			return s, fmt.Errorf("port %s isn't a number from 1 to 65535", port)
		}
		s.Port = p
	}
	switch {
	case s.User == "":
		return s, errors.New("no User")
	case s.Key == "":
		return s, errors.New("no IdentityFile")
	}
	return s, nil
}

// Servers returns the servers in SSHConfig(dir), sorted by name. Host blocks
// it can't read as servers are skipped and reported in bad, so one broken
// block doesn't hide the rest.
func Servers(dir string) (servers []Server, bad []error, err error) {
	entries, err := readSSHConfig(dir)
	if err != nil {
		return nil, nil, err
	}
	for _, e := range entries {
		if e.err != nil {
			bad = append(bad, e.err)
			continue
		}
		servers = append(servers, e.server)
	}
	sort.Slice(servers, func(i, j int) bool { return servers[i].Name < servers[j].Name })
	return servers, bad, nil
}

// LoadServer returns the server called name in SSHConfig(dir). If there isn't
// one, the error matches fs.ErrNotExist.
func LoadServer(dir, name string) (Server, error) {
	if err := CheckName(name); err != nil {
		return Server{}, err
	}
	entries, err := readSSHConfig(dir)
	if err != nil {
		return Server{}, err
	}
	for _, e := range entries {
		if e.server.Name == name {
			return e.server, e.err
		}
	}
	return Server{}, fmt.Errorf("no server named %s: %w", name, fs.ErrNotExist)
}

// SaveServer saves s in SSHConfig(dir), replacing any server with its name.
func SaveServer(dir string, s Server) error {
	if err := CheckName(s.Name); err != nil {
		return err
	}
	if s.Host == "" || s.User == "" || s.Key == "" || s.Port < 1 || s.Port > 65535 {
		return fmt.Errorf("%s needs a host, user, key, and port from 1 to 65535", s.Name)
	}
	return rewriteSSHConfig(dir, s.Name, &s)
}

// RemoveServer removes the server called name from SSHConfig(dir).
func RemoveServer(dir, name string) error {
	if err := CheckName(name); err != nil {
		return err
	}
	return rewriteSSHConfig(dir, name, nil)
}

// rewriteSSHConfig replaces the Host blocks of the server called name in
// SSHConfig(dir), even ones it can't read, with s's, or removes them if s is
// nil. The rest of the file, like other servers' blocks, broken or not, stays
// as it is. If it can't parse the file, it leaves it as it is. It holds a lock
// throughout, so two rembox runs saving servers at once don't lose either.
func rewriteSSHConfig(dir, name string, s *Server) error {
	unlock, err := lockSSHConfig(dir)
	if err != nil {
		return err
	}
	defer unlock()
	cfg, err := decodeSSHConfig(dir)
	if err != nil {
		return err
	}

	var b strings.Builder
	// gap puts a blank line before or after a block rembox writes, unless
	// there is one, or a comment, as for the block it replaces.
	gap := func() {
		text := strings.TrimSuffix(b.String(), "\n")
		last := strings.TrimSpace(text[strings.LastIndex(text, "\n")+1:])
		if text != "" && last != "" && !strings.HasPrefix(last, "#") {
			b.WriteString("\n")
		}
	}
	written, after := s == nil, false
	var hosts []*ssh_config.Host
	if cfg != nil {
		hosts = cfg.Hosts
	} else {
		b.WriteString(sshConfigHeader + "\n")
	}
	for _, h := range hosts {
		switch {
		case serverName(h) != name:
			if after {
				gap()
				after = false
			}
			b.WriteString(h.String())
		case !written:
			gap()
			b.WriteString(hostBlock(*s))
			written, after = true, true
		}
	}
	if !written {
		gap()
		b.WriteString(hostBlock(*s))
	}
	out := strings.TrimRight(b.String(), "\n")
	if out != "" {
		out += "\n"
	}
	return WriteFile(SSHConfig(dir), []byte(out), 0o600)
}

// hostBlock is the Host block rembox writes for s.
func hostBlock(s Server) string {
	hosts := s.Name
	if !strings.EqualFold(s.Host, s.Name) {
		hosts += " " + s.Host
	}
	return fmt.Sprintf("Host %s\n  HostName %s\n  Port %d\n  User %s\n  IdentityFile %s\n  IdentitiesOnly yes\n  ServerAliveInterval 15\n",
		hosts, s.Host, s.Port, s.User, QuoteSSH(s.Key))
}

// QuoteSSH quotes s for an ssh config file, if it has spaces.
func QuoteSSH(s string) string {
	if strings.ContainsAny(s, " \t") {
		return `"` + s + `"`
	}
	return s
}

// WriteFile writes data to path with perm, making its folder if need be
// (readable only by you). It writes a temporary file and renames it, so a
// failed write doesn't leave half a file in place of a good one.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
