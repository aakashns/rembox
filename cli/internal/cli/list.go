package cli

import (
	"flag"
	"fmt"
	"text/tabwriter"

	"github.com/aakashns/rembox/cli/internal/config"
)

const listHelp = `Usage: rembox list

List the servers added with 'rembox setup': each one's name, host, SSH port,
the user rembox logs in as, and the SSH key it logs in with, from their Host
blocks in ~/.config/rembox/ssh_config.
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
	fmt.Fprintln(tw, "NAME\tHOST\tPORT\tUSER\tKEY")
	for _, s := range servers {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\n", s.Name, s.Host, s.Port, s.User, tildePath(home, s.Key))
	}
	return tw.Flush()
}
