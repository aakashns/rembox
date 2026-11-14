// Package cli implements rembox's command-line interface.
package cli

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/aakashns/rembox/cli/internal/config"
	"github.com/aakashns/rembox/cli/internal/release"
	"github.com/aakashns/rembox/cli/internal/version"
)

// App is one invocation of rembox, with everything it touches outside itself
// made explicit so tests can substitute it. rembox always runs at a terminal,
// where it asks its questions.
type App struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Color  bool // color the output

	Releases   *release.Client
	Executable func() (string, error) // path of the running binary
	GOOS       string
	GOARCH     string

	ConfigDir func() (string, error) // where rembox keeps its files
	HomeDir   func() (string, error) // for ~/.ssh
	Username  string                 // yours on this machine, for your user on servers
	NewPort   func() int             // a random port to move ssh to
	Locales   []string               // the locales ssh passes on to servers, like en_US.UTF-8

	// Command runs a program at the terminal, with its output going to stdout.
	// Show the command first, as App.run does.
	Command func(stdout io.Writer, name string, args ...string) error
	Euid    int // 0 when run as root

	in *bufio.Reader // Stdin, shared by every question
}

// Main runs rembox with the process's arguments and returns its exit code.
func Main() int {
	app := &App{
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		Color:      useColor(),
		Releases:   release.NewClient(),
		Executable: os.Executable,
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
		ConfigDir:  config.Dir,
		HomeDir:    os.UserHomeDir,
		Username:   os.Getenv("USER"),
		NewPort:    func() int { return 10000 + rand.IntN(22768) }, // 10000-32767, below Linux's ports for outgoing connections
		Locales:    sentLocales(os.Environ()),
		Command:    atTerminal,
		Euid:       os.Geteuid(),
	}
	return app.Run(os.Args[1:])
}

// atTerminal runs a command with rembox's stdin and stderr, so it can ask the
// user things itself, and the given stdout.
func atTerminal(stdout io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, stdout, os.Stderr
	return cmd.Run()
}

// run shows a command, as you could paste it into a shell, then runs it.
// rembox shows every command it runs, so you can see what it does to your
// machines.
func (a *App) run(name string, args ...string) error {
	a.showCommand(name, args)
	return a.Command(a.Stdout, name, args...)
}

// errDeclined means the user chose not to run a command.
var errDeclined = errors.New("cancelled")

// runConfirmed is run for sensitive or destructive commands: it shows the
// command, then asks before running it. It returns errDeclined if the answer
// is no.
func (a *App) runConfirmed(name string, args ...string) error {
	a.showCommand(name, args)
	if !a.confirm("Run this command?", false) {
		return errDeclined
	}
	return a.Command(a.Stdout, name, args...)
}

func (a *App) showCommand(name string, args []string) {
	// A blank line sets the command apart from rembox's own output.
	fmt.Fprintf(a.Stdout, "\n%s\n", a.paint(cyan, commandLine(name, args)))
}

// commandLine is a command as you could paste it into a shell, after a $.
func commandLine(name string, args []string) string {
	words := []string{"$", shellQuote(name)}
	for _, arg := range args {
		words = append(words, shellQuote(arg))
	}
	return strings.Join(words, " ")
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// shellQuote quotes s for a POSIX shell, if it needs it.
func shellQuote(s string) string {
	if shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// useColor says whether to color rembox's output: unless the terminal can't
// show color, or NO_COLOR is set (https://no-color.org).
func useColor() bool {
	return os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
}

// ANSI styles for paint.
const (
	bold   = "1"
	dim    = "2"
	red    = "31"
	green  = "32"
	yellow = "33"
	cyan   = "36"
)

// paint styles s, if the output is in color.
func (a *App) paint(style, s string) string {
	if !a.Color {
		return s
	}
	return "\x1b[" + style + "m" + s + "\x1b[0m"
}

// done says that something worked.
func (a *App) done(msg string) {
	fmt.Fprintf(a.Stdout, "\n%s %s\n", a.paint(green, "✓"), msg)
}

// heading says what rembox does next, like logging in to a server, so the
// questions that follow, like ssh's, make sense.
func (a *App) heading(msg string) {
	fmt.Fprintf(a.Stdout, "\n\n%s %s\n", a.paint(cyan, "==>"), a.paint(bold, msg))
}

// note says something worth knowing that isn't a problem, or a problem
// rembox carries on after.
func (a *App) note(msg string) {
	fmt.Fprintf(a.Stdout, "\n%s\n", a.paint(yellow, wrap("! "+msg, "  ")[2:]))
}

// wrap fills text into lines of at most 76 characters (where words allow),
// each starting with indent.
func wrap(text, indent string) string {
	var lines []string
	line := indent
	for _, w := range strings.Fields(text) {
		if line != indent && len(line)+1+len(w) > 76 {
			lines = append(lines, line)
			line = indent
		}
		if line != indent {
			line += " "
		}
		line += w
	}
	return strings.Join(append(lines, line), "\n")
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
			name:    "setup",
			summary: "Add or change a server, and get it ready for development, securely",
			help:    setupHelp,
			run:     runSetup,
		},
		{
			name:    "list",
			summary: "List the servers you've added",
			help:    listHelp,
			run:     runList,
		},
		{
			name:    "ssh",
			summary: "Log in to a server you've added, or run a command on it",
			help:    sshHelp,
			run:     runSSH,
		},
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

// exitStatus makes rembox exit with that status without a message, as when
// passing on the status of a command it ran, which already said what went
// wrong.
type exitStatus int

func (e exitStatus) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

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
	var es exitStatus
	if errors.As(err, &es) {
		return int(es)
	}
	fmt.Fprintln(a.Stderr, a.paint(red, fmt.Sprintf("rembox: %v", err)))
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

// confirm asks a yes/no question. An empty answer (or end of input) means
// def; anything else but y/yes means no.
func (a *App) confirm(question string, def bool) bool {
	hint := "[y/N]"
	if def {
		hint = "[Y/n]"
	}
	fmt.Fprintf(a.Stdout, "%s %s ", a.paint(bold, question), hint)
	line, _ := a.readLine()
	switch strings.ToLower(line) {
	case "":
		return def
	case "y", "yes":
		return true
	}
	return false
}

// ask asks a question and returns the answer, or def if it's empty. The end
// of input (Ctrl-D) cancels, since a question may be asked until it's
// answered.
func (a *App) ask(question, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(a.Stdout, "%s %s: ", a.paint(bold, question), a.paint(dim, "[default: "+def+"]"))
	} else {
		fmt.Fprintf(a.Stdout, "%s: ", a.paint(bold, question))
	}
	line, ok := a.readLine()
	if !ok {
		return "", errors.New("cancelled")
	}
	if line == "" {
		return def, nil
	}
	return line, nil
}

// readLine reads a line of input without surrounding spaces. It returns false
// at the end of input, after ending the question's line.
func (a *App) readLine() (string, bool) {
	if a.in == nil {
		a.in = bufio.NewReader(a.Stdin)
	}
	line, err := a.in.ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(a.Stdout)
		return "", false
	}
	return strings.TrimSpace(line), true
}
