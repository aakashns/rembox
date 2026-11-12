// Package remote holds the scripts rembox runs on servers.
//
// rembox runs a script with `bash -c` over ssh, so nothing is copied to the
// server. Each script is lib.sh followed by the script itself.
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
	Name string // its file, like setup.sh
	Body string // lib.sh, then the script
}

// Setup gets a server ready for development: it checks that rembox can set it
// up, then installs updates.
var Setup = load("setup.sh")

func load(name string) Script {
	lib, err := files.ReadFile("lib.sh")
	if err != nil {
		panic(err)
	}
	body, err := files.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return Script{Name: name, Body: string(lib) + "\n" + string(body)}
}

// URL is where to read the script, as this version of rembox has it.
func (s Script) URL() string {
	ref := "main" // development builds
	if version.Valid(version.Version) && !strings.Contains(version.Version, "-") {
		ref = "v" + version.Version
	}
	return "https://github.com/aakashns/rembox/blob/" + ref + "/cli/internal/remote/" + s.Name
}
