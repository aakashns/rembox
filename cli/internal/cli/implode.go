package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aakashns/rembox/cli/internal/release"
)

const implodeHelp = `Usage: rembox implode

Remove rembox from this machine: the rembox binary, its files in
~/.config/rembox (or $XDG_CONFIG_HOME/rembox), which say how to log in to
your servers, and the line in ~/.ssh/config that includes them, if 'rembox
setup' added it. Lists what it will remove and asks first. Servers set up
with rembox aren't changed.

If the binary is in a folder only root can write to, it removes it with sudo,
showing the command and asking before it runs it. Run implode itself without
sudo, so it finds your files.
`

func runImplode(a *App, args []string) error {
	if err := parseFlags(flag.NewFlagSet("implode", flag.ContinueOnError), args); err != nil {
		return err
	}
	// As root, ~ is root's, so rembox would look for the wrong files.
	if a.Euid == 0 && os.Getenv("SUDO_USER") != "" {
		return errors.New("run rembox implode without sudo; it asks for sudo itself if it needs it")
	}

	exe, err := a.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		return fmt.Errorf("can't find the rembox binary: %w", err)
	}
	dir, err := a.ConfigDir()
	if err != nil {
		return err
	}
	_, err = os.Lstat(dir)
	dirExists := err == nil
	home, err := a.HomeDir()
	if err != nil {
		return fmt.Errorf("can't find your home folder: %w", err)
	}
	included, err := hasInclude(home, dir)
	if err != nil {
		return err
	}

	fmt.Fprintln(a.Stdout, "This removes rembox from this machine:")
	fmt.Fprintf(a.Stdout, "  %s\n", exe)
	if dirExists {
		fmt.Fprintf(a.Stdout, "  %s\n", dir)
	}
	if included {
		fmt.Fprintf(a.Stdout, "  '%s' in ~/.ssh/config\n", includeLine(home, dir))
	}
	fmt.Fprintln(a.Stdout, "Servers set up with rembox aren't changed.")

	// Checked before asking, so a yes isn't followed by a permissions error.
	needSudo := release.CheckWritable(filepath.Dir(exe)) != nil

	if !a.confirm("Remove rembox and its files?", false) {
		fmt.Fprintln(a.Stdout, "Nothing was removed.")
		return nil
	}

	// The binary goes first: if sudo is declined or fails, nothing is removed.
	if needSudo {
		fmt.Fprintf(a.Stdout, "Removing %s needs sudo:\n", exe)
		err = a.runConfirmed("sudo", "rm", "-f", "--", exe)
		if errors.Is(err, errDeclined) {
			fmt.Fprintln(a.Stdout, "Nothing was removed.")
			return nil
		}
	} else {
		err = os.Remove(exe)
	}
	if err != nil {
		return fmt.Errorf("removing %s: %w; nothing was removed", exe, err)
	}
	fmt.Fprintf(a.Stdout, "Removed %s\n", exe)

	if dirExists {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("removing %s: %w; delete it yourself to finish", dir, err)
		}
		fmt.Fprintf(a.Stdout, "Removed %s\n", dir)
	}
	if included {
		if _, err := removeInclude(home, dir); err != nil {
			return fmt.Errorf("%w; take '%s' out of ~/.ssh/config yourself to finish", err, includeLine(home, dir))
		}
		fmt.Fprintf(a.Stdout, "Removed '%s' from ~/.ssh/config\n", includeLine(home, dir))
	}
	fmt.Fprintln(a.Stdout, a.paint(green, "rembox is gone.")+" To reinstall: curl -fsSL https://rembox.aakashns.com/install.sh | sh")
	return nil
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
