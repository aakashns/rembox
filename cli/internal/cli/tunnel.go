package cli

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
)

const tunnelHelp = `Usage: rembox tunnel NAME PORT
       rembox tunnel NAME SERVER-PORT:LOCAL-PORT

Forward a port from this machine to the server called NAME over ssh, so
localhost:PORT here reaches port PORT on the server, as with ssh -L. Give
SERVER-PORT:LOCAL-PORT to reach the server's port SERVER-PORT at
localhost:LOCAL-PORT instead. Only this machine can use the tunnel; it stays
open until you press Ctrl-C.
`

func runTunnel(a *App, args []string) error {
	switch {
	case len(args) == 0:
		return usagef("give the name of a server; 'rembox list' shows them")
	case args[0] == "-h" || args[0] == "--help":
		return flag.ErrHelp
	case strings.HasPrefix(args[0], "-"):
		return usagef("unknown flag %s", args[0])
	case len(args) == 1:
		return usagef("give the port to forward, like 3000, or 3000:8080 for the server's port 3000 at localhost:8080")
	case len(args) > 2:
		return usagef("unexpected argument %q", args[2])
	}
	serverPort, localPort, err := parsePorts(args[1])
	if err != nil {
		return err
	}
	s, err := a.loadServer(args[0])
	if err != nil {
		return err
	}

	a.heading(fmt.Sprintf("Forwarding localhost:%d to port %d on %s; Ctrl-C stops it", localPort, serverPort, s.Name))
	sshArgs := append(loginArgs(s),
		"-N",                             // just the tunnel, no shell
		"-o", "ExitOnForwardFailure=yes", // stop if the local port is taken
		"-L", fmt.Sprintf("%d:localhost:%d", localPort, serverPort),
		"--", s.Destination(),
	)
	// Ctrl-C reaches ssh too, which closes the tunnel; rembox waits for it.
	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt)
	defer signal.Stop(interrupted)
	err = a.run("ssh", sshArgs...)
	if len(interrupted) > 0 {
		return nil
	}
	return sshError(err)
}

// parsePorts parses tunnel's PORT or SERVER-PORT:LOCAL-PORT.
func parsePorts(arg string) (serverPort, localPort int, err error) {
	server, local, ok := strings.Cut(arg, ":")
	if !ok {
		local = server
	}
	if serverPort, err = parsePort(server); err == nil {
		localPort, err = parsePort(local)
	}
	return serverPort, localPort, err
}

func parsePort(s string) (int, error) {
	p, err := strconv.Atoi(s)
	if err != nil || p < 1 || p > 65535 {
		return 0, usagef("%q isn't a port; give a number from 1 to 65535", s)
	}
	return p, nil
}
