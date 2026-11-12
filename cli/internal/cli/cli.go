// Package cli implements rembox's command-line interface.
package cli

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/aakashns/rembox/cli/internal/config"
	"github.com/aakashns/rembox/cli/internal/release"
	"github.com/aakashns/rembox/cli/internal/version"
)

// App is one invocation of rembox, with everything it touches outside itself
// made explicit so tests can substitute it.
type App struct {
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
	Interactive bool // stdin is a terminal, so we can ask questions

	Releases   *release.Client
	Executable func() (string, error) // path of the running binary
	GOOS       string
	GOARCH     string

	ConfigDir func() (string, error)     // where rembox keeps its files
	HomeDir   func() (string, error)     // where implode puts backups
	Now       func() time.Time           // for backups' names
	Sudo      func(args ...string) error // runs a command with sudo, at the terminal
	Euid      int                        // 0 when run as root

	in *bufio.Reader // Stdin, shared by every question
}

// Main runs rembox with the process's arguments and returns its exit code.
func Main() int {
	app := &App{
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Interactive: isTerminal(os.Stdin),
		Releases:    release.NewClient(),
		Executable:  os.Executable,
		GOOS:        runtime.GOOS,
		GOARCH:      runtime.GOARCH,
		ConfigDir:   config.Dir,
		HomeDir:     os.UserHomeDir,
		Now:         time.Now,
		Sudo:        sudo,
		Euid:        os.Geteuid(),
	}
	return app.Run(os.Args[1:])
}

func sudo(args ...string) error {
	cmd := exec.Command("sudo", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// Exit codes.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

type command struct {
	name    string
	summary string // one line, for the command list
	help    string // full help, starting with the usage line
	run     func(a *App, args []string) error
}

// commands lists every command, in the order `rembox help` shows them.
func commands() []command {
	return []command{
		{
			name:    "help",
			summary: "Show help for rembox or one of its commands",
			help:    "Usage: rembox help [command]\n\nShow help for rembox, or for one command.\n",
			run:     runHelp,
		},
		{
			name:    "version",
			summary: "Print the rembox version",
			help:    "Usage: rembox version\n\nPrint the rembox version and the commit it was built from.\n",
			run:     runVersion,
		},
		{
			name:    "upgrade",
			summary: "Upgrade rembox to the latest release",
			help:    upgradeHelp,
			run:     runUpgrade,
		},
		{
			name:    "implode",
			summary: "Remove rembox and its files from this machine",
			help:    implodeHelp,
			run:     runImplode,
		},
	}
}

func findCommand(name string) (command, bool) {
	for _, c := range commands() {
		if c.name == name {
			return c, true
		}
	}
	return command{}, false
}

// usageError means the command line was wrong; rembox exits with status 2.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

func usagef(format string, args ...any) error {
	return &usageError{fmt.Sprintf(format, args...)}
}

// Run executes the command line args (without the program name) and returns
// the exit code.
func (a *App) Run(args []string) int {
	if len(args) == 0 {
		a.printHelp()
		return exitOK
	}
	name, rest := args[0], args[1:]
	switch name {
	case "-h", "--help":
		name = "help"
	case "-v", "--version":
		name = "version"
	}
	cmd, ok := findCommand(name)
	if !ok {
		if strings.HasPrefix(name, "-") {
			return a.fail(usagef("unknown flag %s", name), "help")
		}
		return a.fail(usagef("unknown command %q", name), "help")
	}

	err := cmd.run(a, rest)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(a.Stdout, cmd.help)
		return exitOK
	}
	return a.fail(err, cmd.name)
}

// fail reports err (if any) and returns the matching exit code.
func (a *App) fail(err error, cmdName string) int {
	if err == nil {
		return exitOK
	}
	fmt.Fprintf(a.Stderr, "rembox: %v\n", err)
	var ue *usageError
	if errors.As(err, &ue) {
		if cmdName == "help" {
			fmt.Fprintln(a.Stderr, "Run 'rembox help' for usage.")
		} else {
			fmt.Fprintf(a.Stderr, "Run 'rembox help %s' for usage.\n", cmdName)
		}
		return exitUsage
	}
	return exitError
}

// parseFlags parses a command's flags. -h/--help returns flag.ErrHelp, which
// Run turns into the command's help text.
func parseFlags(fs *flag.FlagSet, args []string) error {
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return usagef("%v", err)
	}
	if fs.NArg() > 0 {
		return usagef("unexpected argument %q", fs.Arg(0))
	}
	return nil
}

func (a *App) printHelp() {
	var b strings.Builder
	b.WriteString("rembox: set up and manage remote development machines\n\n")
	b.WriteString("Usage:\n  rembox <command> [flags]\n\nCommands:\n")
	for _, c := range commands() {
		fmt.Fprintf(&b, "  %-10s %s\n", c.name, c.summary)
	}
	b.WriteString("\nRun 'rembox help <command>' for more about a command.\n")
	fmt.Fprint(a.Stdout, b.String())
}

func runHelp(a *App, args []string) error {
	switch len(args) {
	case 0:
		a.printHelp()
		return nil
	case 1:
		if args[0] == "-h" || args[0] == "--help" {
			return flag.ErrHelp
		}
		cmd, ok := findCommand(args[0])
		if !ok {
			return usagef("unknown command %q", args[0])
		}
		fmt.Fprint(a.Stdout, cmd.help)
		return nil
	default:
		return usagef("help takes at most one command")
	}
}

func runVersion(a *App, args []string) error {
	if err := parseFlags(flag.NewFlagSet("version", flag.ContinueOnError), args); err != nil {
		return err
	}
	fmt.Fprintln(a.Stdout, version.String())
	return nil
}

// errNoTerminal means a question was needed but there's no one to answer it.
var errNoTerminal = errors.New("can't ask for confirmation without a terminal; re-run with --yes to proceed")

// confirm asks a yes/no question. An empty answer (or end of input) means
// def; anything else but y/yes means no.
func (a *App) confirm(question string, def bool) (bool, error) {
	if !a.Interactive {
		return false, errNoTerminal
	}
	hint := "[y/N]"
	if def {
		hint = "[Y/n]"
	}
	fmt.Fprintf(a.Stdout, "%s %s ", question, hint)
	if a.in == nil {
		a.in = bufio.NewReader(a.Stdin)
	}
	line, err := a.in.ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(a.Stdout)
		return def, nil
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "":
		return def, nil
	case "y", "yes":
		return true, nil
	}
	return false, nil
}
