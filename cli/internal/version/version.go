// Package version holds rembox's build information and compares versions.
//
// Version, Commit and Date are set at link time by GoReleaser (see
// .goreleaser.yaml) or the Makefile, e.g.
//
//	-ldflags "-X github.com/aakashns/rembox/cli/internal/version.Version=0.2.0"
//
// Builds made without ldflags (plain `go build` or `go install`) fall back to
// what the Go toolchain recorded in the binary.
package version

import (
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"
)

var (
	// Version is the release version without a leading "v" (e.g. "0.2.0"),
	// or "dev" for builds that aren't from a release tag.
	Version = "dev"
	// Commit is the short git commit the binary was built from.
	Commit = ""
	// Date is when that commit was made (RFC 3339).
	Date = ""
)

func init() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	// `go install github.com/aakashns/rembox/cli/cmd/rembox@...` records the version.
	if Version == "dev" {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			Version = strings.TrimPrefix(v, "v")
		}
	}
	// `go build` inside a git checkout records the commit.
	for _, s := range info.Settings {
		switch {
		case s.Key == "vcs.revision" && Commit == "":
			Commit = s.Value
			if len(Commit) > 7 {
				Commit = Commit[:7]
			}
		case s.Key == "vcs.time" && Date == "":
			Date = s.Value
		}
	}
}

// String describes this build, e.g. "rembox 0.2.0 (commit abc1234, 2026-11-12)".
func String() string {
	s := "rembox " + Version
	var extra []string
	if Commit != "" {
		extra = append(extra, "commit "+Commit)
	}
	if len(Date) >= 10 {
		extra = append(extra, Date[:10])
	}
	if len(extra) > 0 {
		s += " (" + strings.Join(extra, ", ") + ")"
	}
	return s
}

// semver is a parsed MAJOR.MINOR.PATCH[-PRERELEASE] version.
type semver struct {
	nums [3]int
	pre  string
}

func parse(v string) (semver, bool) {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexByte(v, '+'); i >= 0 { // build metadata doesn't count
		v = v[:i]
	}
	var s semver
	core := v
	if i := strings.IndexByte(v, '-'); i >= 0 {
		core, s.pre = v[:i], v[i+1:]
		if s.pre == "" {
			return semver{}, false
		}
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || p == "" || (len(p) > 1 && p[0] == '0') {
			return semver{}, false
		}
		s.nums[i] = n
	}
	return s, true
}

// Valid reports whether v is a semantic version, with or without a leading "v".
func Valid(v string) bool {
	_, ok := parse(v)
	return ok
}

// Compare returns -1, 0 or +1 depending on whether a is older than, the same
// as, or newer than b. A pre-release (1.0.0-rc.1) is older than its release
// (1.0.0); pre-releases of the same version are compared field by field, as
// semver specifies. It returns an error if either version can't be parsed.
func Compare(a, b string) (int, error) {
	va, ok := parse(a)
	if !ok {
		return 0, fmt.Errorf("not a version: %q", a)
	}
	vb, ok := parse(b)
	if !ok {
		return 0, fmt.Errorf("not a version: %q", b)
	}
	for i := range va.nums {
		if c := cmpInt(va.nums[i], vb.nums[i]); c != 0 {
			return c, nil
		}
	}
	switch {
	case va.pre == vb.pre:
		return 0, nil
	case va.pre == "":
		return 1, nil
	case vb.pre == "":
		return -1, nil
	}
	return comparePre(va.pre, vb.pre), nil
}

func comparePre(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		an, aErr := strconv.Atoi(as[i])
		bn, bErr := strconv.Atoi(bs[i])
		switch {
		case aErr == nil && bErr == nil:
			if c := cmpInt(an, bn); c != 0 {
				return c
			}
		case aErr == nil: // numeric fields sort before alphanumeric ones
			return -1
		case bErr == nil:
			return 1
		default:
			if c := strings.Compare(as[i], bs[i]); c != 0 {
				return c
			}
		}
	}
	return cmpInt(len(as), len(bs))
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
