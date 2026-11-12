package cli

import (
	"flag"
	"fmt"
	"text/tabwriter"

	"github.com/aakashns/rembox/cli/internal/config"
)

const listHelp = `Usage: rembox list

List the servers added with 'rembox setup': each one's name, the user and
host rembox logs in as, the port, and the SSH key it logs in with.
`

func runList(a *App, args []string) error {
	if err := parseFlags(flag.NewFlagSet("list", flag.ContinueOnError), args); err != nil {
		return err
	}
	dir, err := a.ConfigDir()
	if err != nil {
		return err
	}
	servers, bad, err := config.Servers(dir)
	if err != nil {
		return fmt.Errorf("reading your servers: %w", err)
	}
	for _, err := range bad {
		fmt.Fprintln(a.Stderr, a.paint(yellow, fmt.Sprintf("rembox: skipping a server: %v", err)))
	}
	if len(servers) == 0 {
		fmt.Fprintln(a.Stdout, "No servers yet. Run 'rembox setup' to add one.")
		return nil
	}
	home, _ := a.HomeDir() // just for showing keys as ~/...
	tw := tabwriter.NewWriter(a.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tLOGIN\tPORT\tKEY")
	for _, s := range servers {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n", s.Name, s.Destination(), s.Port, tildePath(home, s.Key))
	}
	return tw.Flush()
}
