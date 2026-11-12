// Package remote holds the scripts rembox runs on servers.
//
// rembox runs a script with `bash -c` over ssh, so nothing is copied to the
// server. Each script is lib.sh, then the files with the functions for its
// steps, then the script itself, which calls them.
package remote

import (
	"embed"
	"strings"

	"github.com/aakashns/rembox/cli/internal/version"
)

//go:embed *.sh
var files embed.FS

// Script is a script rembox runs on servers.
type Script struct {
	Name  string   // its file, like setup.sh
	Files []string // the files it's made of, in order: lib.sh, its steps, then Name
	Body  string   // the files, one after another
}

// Setup gets a server ready for development.
var Setup = load("setup.sh", "check.sh", "update.sh", "user.sh", "sshd.sh", "firewall.sh", "fail2ban.sh", "restart.sh")

func load(name string, steps ...string) Script {
	s := Script{Name: name, Files: append(append([]string{"lib.sh"}, steps...), name)}
	parts := make([]string, len(s.Files))
	for i, f := range s.Files {
		b, err := files.ReadFile(f)
		if err != nil {
			panic(err)
		}
		parts[i] = string(b)
	}
	s.Body = strings.Join(parts, "\n")
	return s
}

// URL is where to read the scripts, as this version of rembox has them.
func URL() string {
	ref := "main" // development builds
	if version.Valid(version.Version) && !strings.Contains(version.Version, "-") {
		ref = "v" + version.Version
	}
	return "https://github.com/aakashns/rembox/tree/" + ref + "/cli/internal/remote"
}
