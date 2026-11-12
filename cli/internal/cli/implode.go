package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aakashns/rembox/cli/internal/config"
	"github.com/aakashns/rembox/cli/internal/release"
)

const implodeHelp = `Usage: rembox implode [--yes] [--backup | --no-backup]

Remove rembox from this machine: the rembox binary, and its files in
~/.config/rembox (or $XDG_CONFIG_HOME/rembox). Lists what it will remove and
asks first. If there are files, it offers to back them up to a .tar.gz in your
home folder before deleting them. Servers set up with rembox aren't changed.

If the binary is in a folder only root can write to, it removes it with sudo,
showing the command and asking before it runs it. Run implode itself without
sudo, so it finds your files.

Flags:
  -y, --yes     Remove without asking (and back up, unless --no-backup)
  --backup      Back up the files without asking
  --no-backup   Don't back up the files
`

func runImplode(a *App, args []string) error {
	fs := flag.NewFlagSet("implode", flag.ContinueOnError)
	var yes, backup, noBackup bool
	fs.BoolVar(&yes, "y", false, "")
	fs.BoolVar(&yes, "yes", false, "")
	fs.BoolVar(&backup, "backup", false, "")
	fs.BoolVar(&noBackup, "no-backup", false, "")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if backup && noBackup {
		return usagef("--backup and --no-backup can't be used together")
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
	files, dirExists, err := config.CountFiles(dir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", dir, err)
	}

	fmt.Fprintln(a.Stdout, "This removes rembox from this machine:")
	fmt.Fprintf(a.Stdout, "  %s\n", exe)
	if dirExists {
		fmt.Fprintf(a.Stdout, "  %s (%s)\n", dir, plural(files, "file"))
	}
	fmt.Fprintln(a.Stdout, "Servers set up with rembox aren't changed.")

	// Checked before asking, so a yes isn't followed by a permissions error.
	needSudo := release.CheckWritable(filepath.Dir(exe)) != nil

	if !yes {
		ok, err := a.confirm("Remove rembox and its files?", false)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(a.Stdout, "Nothing was removed.")
			return nil
		}
	}

	if files > 0 {
		doBackup := backup || (yes && !noBackup)
		if !backup && !noBackup && !yes {
			if doBackup, err = a.confirm(fmt.Sprintf("Back up %s first?", dir), true); err != nil {
				return err
			}
		}
		if doBackup {
			home, err := a.HomeDir()
			if err != nil {
				return fmt.Errorf("can't find your home folder for the backup: %w", err)
			}
			dest := filepath.Join(home, "rembox-backup-"+a.Now().Format("20060102-150405")+".tar.gz")
			if err := config.Backup(dir, dest); err != nil {
				return fmt.Errorf("%w; nothing was removed", err)
			}
			fmt.Fprintf(a.Stdout, "Backed up %s to %s\n", dir, dest)
		}
	}

	// The binary goes first: if sudo is declined or fails, nothing is removed.
	if needSudo {
		fmt.Fprintf(a.Stdout, "Removing %s needs sudo:\n", exe)
		err = a.runConfirmed(yes, "sudo", "rm", "-f", "--", exe)
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
	fmt.Fprintln(a.Stdout, "rembox is gone. To reinstall: curl -fsSL https://rembox.aakashns.com/install.sh | sh")
	return nil
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
