package cli

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/aakashns/rembox/cli/internal/release"
	"github.com/aakashns/rembox/cli/internal/version"
)

const upgradeHelp = `Usage: rembox upgrade [--yes]

Upgrade rembox to the latest release. Shows the current and latest versions
and asks before changing anything. The download is checked against the
release's SHA256SUMS before it replaces this binary. If the binary is in a
folder you can't write to (like /usr/local/bin), run it with sudo.

Flags:
  -y, --yes   Upgrade without asking
`

func runUpgrade(a *App, args []string) error {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	var yes bool
	fs.BoolVar(&yes, "y", false, "")
	fs.BoolVar(&yes, "yes", false, "")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	exe, err := a.Executable()
	if err != nil {
		return fmt.Errorf("can't find the rembox binary: %w", err)
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return fmt.Errorf("can't find the rembox binary: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Fprintln(a.Stdout, "Checking for the latest release...")
	tag, err := a.Releases.Latest(ctx)
	if err != nil {
		return err
	}
	current, latest := version.Version, strings.TrimPrefix(tag, "v")

	if cmp, err := version.Compare(current, latest); err != nil {
		fmt.Fprintf(a.Stdout, "This is a development build (%s); the latest release is %s.\n", current, latest)
	} else if cmp == 0 {
		fmt.Fprintf(a.Stdout, "rembox %s is already the latest version.\n", current)
		return nil
	} else if cmp > 0 {
		fmt.Fprintf(a.Stdout, "rembox %s is newer than the latest release (%s); nothing to do.\n", current, latest)
		return nil
	} else {
		fmt.Fprintf(a.Stdout, "A new version is available: %s -> %s\n", current, latest)
	}

	// Checked before asking, so a yes isn't followed by a permissions error.
	if err := release.CheckWritable(filepath.Dir(exe)); err != nil {
		return fmt.Errorf("can't replace %s; run it as a user who can, e.g. sudo rembox upgrade", exe)
	}

	if !yes {
		ok, err := a.confirm(fmt.Sprintf("Replace %s with rembox %s?", exe, latest), false)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(a.Stdout, "Upgrade cancelled.")
			return nil
		}
	}

	asset := release.AssetName(a.GOOS, a.GOARCH)
	fmt.Fprintf(a.Stdout, "Downloading %s...\n", asset)
	bin, err := a.Releases.FetchBinary(ctx, tag, a.GOOS, a.GOARCH)
	if err != nil {
		return err
	}
	if err := release.ReplaceExecutable(exe, bin); err != nil {
		return fmt.Errorf("installing the new version: %w", err)
	}
	fmt.Fprintf(a.Stdout, "Upgraded rembox to %s.\n", latest)
	return nil
}
