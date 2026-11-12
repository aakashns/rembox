// Package version holds rembox's build information and compares versions.
//
// Version, Commit and Date are set at link time by the Makefile, e.g.
//
//	-ldflags "-X github.com/aakashns/rembox/cli/internal/version.Version=0.2.0"
package version

import (
	"fmt"
	"strconv"
	"strings"
)

var (
	// Version is the release version without a leading "v" (e.g. "0.2.0"),
	// "0.2.0-dev" for other builds with the Makefile, or "dev".
	Version = "dev"
	// Commit is the short git commit the binary was built from.
	Commit = ""
	// Date is when that commit was made (RFC 3339).
	Date = ""
)

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

// parse parses MAJOR.MINOR.PATCH[-PRERELEASE], with or without a leading "v".
func parse(v string) (nums [3]int, pre string, ok bool) {
	core, pre, hasPre := strings.Cut(strings.TrimPrefix(v, "v"), "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 || (hasPre && pre == "") {
		return nums, "", false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || p == "" || (len(p) > 1 && p[0] == '0') {
			return nums, "", false
		}
		nums[i] = n
	}
	return nums, pre, true
}

// Valid reports whether v is a version, with or without a leading "v".
func Valid(v string) bool {
	_, _, ok := parse(v)
	return ok
}

// Compare returns -1, 0 or +1 depending on whether a is older than, the same
// as, or newer than b. A pre-release (0.2.0-dev) is older than its release
// (0.2.0). It returns an error if either version can't be parsed.
func Compare(a, b string) (int, error) {
	na, pa, ok := parse(a)
	if !ok {
		return 0, fmt.Errorf("not a version: %q", a)
	}
	nb, pb, ok := parse(b)
	if !ok {
		return 0, fmt.Errorf("not a version: %q", b)
	}
	for i := range na {
		if na[i] != nb[i] {
			return cmp(na[i], nb[i]), nil
		}
	}
	switch {
	case pa == pb:
		return 0, nil
	case pa == "":
		return 1, nil
	case pb == "":
		return -1, nil
	}
	return strings.Compare(pa, pb), nil
}

func cmp(a, b int) int {
	if a < b {
		return -1
	}
	return 1
}
