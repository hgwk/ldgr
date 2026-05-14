package cmd

import (
	"fmt"
	"io"
)

// Handler is a CLI subcommand entry point.
type Handler func(args []string, stdout, stderr io.Writer) int

// Commands maps top-level subcommand names to handlers. Later tasks add
// entries here from files in this same package.
var Commands = map[string]Handler{}

func Dispatch(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: ldgr <subcommand> [args]")
		return 2
	}
	name := args[0]
	handler, ok := Commands[name]
	if !ok {
		fmt.Fprintf(stderr, "unknown subcommand: %s\n", name)
		return 2
	}
	return handler(args[1:], stdout, stderr)
}
