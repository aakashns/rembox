package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
)

func writeSSHConfig(t *testing.T, dir, body string) {
	t.Helper()
	if err := WriteFile(SSHConfig(dir), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readSSHConfigFile(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(SSHConfig(dir))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSaveServer(t *testing.T) {
	dir := t.TempDir()
	b := Server{Name: "b", Host: "203.0.113.10", Port: 23456, User: "ada", Key: "/keys/id ed25519"}
	a := Server{Name: "a", Host: "a", Port: 22, User: "root", Key: "/keys/a"}
	for _, s := range []Server{b, a} {
		if err := SaveServer(dir, s); err != nil {
			t.Fatal(err)
		}
	}
	// New servers go at the end.
	want := sshConfigHeader + `
Host b 203.0.113.10
  HostName 203.0.113.10
  Port 23456
  User ada
  IdentityFile "/keys/id ed25519"
  IdentitiesOnly yes
  ServerAliveInterval 15

Host a
  HostName a
  Port 22
  User root
  IdentityFile /keys/a
  IdentitiesOnly yes
  ServerAliveInterval 15
`
	if got := readSSHConfigFile(t, dir); got != want {
		t.Errorf("ssh_config:\n%s\nwant:\n%s", got, want)
	}
	if info, err := os.Stat(SSHConfig(dir)); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("ssh_config: %v, %v", info, err)
	}

	// It reads back what it wrote, quotes and all.
	servers, bad, err := Servers(dir)
	if err != nil || bad != nil || !slices.Equal(servers, []Server{a, b}) {
		t.Errorf("Servers = %+v, %v, %v", servers, bad, err)
	}
	if got, err := LoadServer(dir, "b"); err != nil || got != b {
		t.Errorf("LoadServer(b) = %+v, %v", got, err)
	}

	// Saving a server with the same name replaces it.
	b.Port = 2222
	if err := SaveServer(dir, b); err != nil {
		t.Fatal(err)
	}
	if servers, _, _ := Servers(dir); !slices.Equal(servers, []Server{a, b}) {
		t.Errorf("after replacing b: %+v", servers)
	}

	if err := RemoveServer(dir, "a"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveServer(dir, "nonesuch"); err != nil {
		t.Errorf("removing a server that isn't there: %v", err)
	}
	if servers, _, _ := Servers(dir); !slices.Equal(servers, []Server{b}) {
		t.Errorf("after removing a: %+v", servers)
	}
}

func TestSaveServerInvalid(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"", "Box", "*", "-x", "a b"} {
		if err := SaveServer(dir, Server{Name: name, Host: "h", Port: 22, User: "u", Key: "/k"}); err == nil {
			t.Errorf("saved a server named %q", name)
		}
	}
	// It doesn't write a block it couldn't read back.
	for _, s := range []Server{
		{Name: "box", Port: 22, User: "u", Key: "/k"},
		{Name: "box", Host: "h", Port: 22, Key: "/k"},
		{Name: "box", Host: "h", Port: 22, User: "u"},
		{Name: "box", Host: "h", User: "u", Key: "/k"},
		{Name: "box", Host: "h", Port: 65536, User: "u", Key: "/k"},
	} {
		if err := SaveServer(dir, s); err == nil {
			t.Errorf("saved %+v", s)
		}
	}
	if _, err := os.Stat(SSHConfig(dir)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("wrote ssh_config: %v", err)
	}
}

func TestServersNone(t *testing.T) {
	servers, bad, err := Servers(t.TempDir())
	if servers != nil || bad != nil || err != nil {
		t.Errorf("Servers = %v, %v, %v", servers, bad, err)
	}
	_, err = LoadServer(t.TempDir(), "box")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("LoadServer = %v", err)
	}
}

// Servers reads Host blocks edited by hand, as ssh does.
func TestServersHandEdited(t *testing.T) {
	dir := t.TempDir()
	writeSSHConfig(t, dir, `# my notes
Host *
  User everyone

host box  # the box
	hostname=box.example.com
	PORT 2200
	user ada
	User ignored
	identityfile "~/.ssh/my key"
	ForwardAgent yes

Host bare
  User root
  IdentityFile /k

Host *.example.com !x
  User z

Match host matched
  User q
  IdentityFile /k
`)
	servers, bad, err := Servers(dir)
	want := []Server{
		{Name: "bare", Host: "bare", Port: 22, User: "root", Key: "/k"},
		{Name: "box", Host: "box.example.com", Port: 2200, User: "ada", Key: "~/.ssh/my key"},
	}
	if err != nil || bad != nil || !slices.Equal(servers, want) {
		t.Errorf("Servers = %+v, %v, %v", servers, bad, err)
	}
}

func TestServersBadBlocks(t *testing.T) {
	dir := t.TempDir()
	writeSSHConfig(t, dir, `
Host good
  User root
  IdentityFile /k

Host nouser
  IdentityFile /k

Host nokey
  User root

Host badport
  Port 70000
  User root
  IdentityFile /k

Host good
  HostName other
  User root
  IdentityFile /k
`)
	servers, bad, err := Servers(dir)
	if err != nil || len(servers) != 1 || servers[0].Name != "good" || servers[0].Host != "good" {
		t.Errorf("Servers = %+v, %v", servers, err)
	}
	var msgs []string
	for _, e := range bad {
		msgs = append(msgs, e.Error())
	}
	got := strings.Join(msgs, "\n")
	for _, want := range []string{
		"nouser in " + SSHConfig(dir) + ": no User",
		"nokey in " + SSHConfig(dir) + ": no IdentityFile",
		"badport in " + SSHConfig(dir) + ": port 70000 isn't a number from 1 to 65535",
		"good in " + SSHConfig(dir) + ": there's a Host block for it above",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("bad = %s\nwant %q", got, want)
		}
	}
	if len(bad) != 4 {
		t.Errorf("%d bad, want 4", len(bad))
	}

	// LoadServer says what's wrong with a broken block, and finds the first
	// of two.
	if _, err := LoadServer(dir, "nokey"); err == nil || !strings.Contains(err.Error(), "no IdentityFile") {
		t.Errorf("LoadServer(nokey) = %v", err)
	}
	if s, err := LoadServer(dir, "good"); err != nil || s.Host != "good" {
		t.Errorf("LoadServer(good) = %+v, %v", s, err)
	}
}

// Saving or removing a server changes only its Host blocks, even broken ones,
// and leaves the rest of the file as it is: other servers, broken or with
// options of their own, comments, Host * and Match.
func TestSaveServerKeepsTheRest(t *testing.T) {
	dir := t.TempDir()
	head := `# my notes
Host *
  ServerAliveCountMax 4

# the box
`
	other := `Host other
  HostName o.example.com # mine
  User root
  IdentityFile /k
  ForwardAgent yes

`
	typo := `Host typo
  Port 22x
  User root
  IdentityFile /k

`
	tail := `Match host *.internal
  User me
`
	box := func(host string) string {
		return "Host box " + host + "\n  HostName " + host +
			"\n  Port 22\n  User ada\n  IdentityFile /k\n  IdentitiesOnly yes\n  ServerAliveInterval 15\n"
	}
	writeSSHConfig(t, dir, head+"Host box\n  User old\n\n"+other+typo+"Host box\n  User again\n\n"+tail)

	// Its first block is replaced in place, and any more removed.
	if err := SaveServer(dir, Server{Name: "box", Host: "b1", Port: 22, User: "ada", Key: "/k"}); err != nil {
		t.Fatal(err)
	}
	want := head + box("b1") + "\n" + other + typo + tail
	if got := readSSHConfigFile(t, dir); got != want {
		t.Errorf("after saving box:\n%s\nwant:\n%s", got, want)
	}

	if err := SaveServer(dir, Server{Name: "new", Host: "n", Port: 22, User: "ada", Key: "/k"}); err != nil {
		t.Fatal(err)
	}
	newBlock := "Host new n\n  HostName n\n  Port 22\n  User ada\n  IdentityFile /k\n  IdentitiesOnly yes\n  ServerAliveInterval 15\n"
	want += "\n" + newBlock
	if got := readSSHConfigFile(t, dir); got != want {
		t.Errorf("after saving new:\n%s\nwant:\n%s", got, want)
	}

	// Removing one, broken or not, leaves the rest.
	for _, name := range []string{"other", "typo"} {
		if err := RemoveServer(dir, name); err != nil {
			t.Fatal(err)
		}
	}
	want = head + box("b1") + "\n" + tail + "\n" + newBlock
	if got := readSSHConfigFile(t, dir); got != want {
		t.Errorf("after removing other and typo:\n%s\nwant:\n%s", got, want)
	}
	if err := RemoveServer(dir, "new"); err != nil {
		t.Fatal(err)
	}
	if got := readSSHConfigFile(t, dir); got != head+box("b1")+"\n"+tail {
		t.Errorf("after removing new:\n%s", got)
	}
}

// Two rembox runs saving servers at once both get saved.
func TestSaveServerConcurrent(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- SaveServer(dir, Server{Name: fmt.Sprintf("box%d", i), Host: "h", Port: 22, User: "u", Key: "/k"})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if servers, bad, err := Servers(dir); len(servers) != 20 || bad != nil || err != nil {
		t.Errorf("saved %d servers, want 20: %v %v", len(servers), bad, err)
	}
}

// A file ssh_config can't parse is reported, and not overwritten.
func TestServersUnreadable(t *testing.T) {
	dir := t.TempDir()
	body := "Match exec true\n  User root\n"
	writeSSHConfig(t, dir, body)
	if _, _, err := Servers(dir); err == nil || !strings.Contains(err.Error(), SSHConfig(dir)) {
		t.Errorf("Servers = %v", err)
	}
	if err := SaveServer(dir, Server{Name: "box", Host: "h", Port: 22, User: "u", Key: "/k"}); err == nil {
		t.Error("saved a server over a file it can't read")
	}
	if got := readSSHConfigFile(t, dir); got != body {
		t.Errorf("ssh_config changed:\n%s", got)
	}
}
