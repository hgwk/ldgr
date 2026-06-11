package cmd

import (
	"fmt"
	"io"
	"os"
)

func init() {
	Commands["ticket"] = func(args []string, stdout, stderr io.Writer) int {
		return RunTicketCLI(args, os.Stdin, stdout, stderr)
	}
}

// RunTicketCLI is the entry for `ldgr ticket ...`.
func RunTicketCLI(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		fmt.Fprintln(stderr, "usage: ldgr ticket <add|event|ready> [flags]")
		fmt.Fprintln(stderr, "try: ldgr ticket add --example")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "add":
		return runTicketAdd(rest, stdin, stdout, stderr)
	case "event":
		return runTicketEvent(rest, stdin, stdout, stderr)
	case "ready":
		return runTicketReady(rest, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown ticket subcommand: %s\n", sub)
		return 2
	}
}
