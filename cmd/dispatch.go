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
		printUsage(stderr)
		return 2
	}
	name := args[0]
	if name == "help" || name == "--help" || name == "-h" {
		printUsage(stdout)
		return 0
	}
	if name == "version" || name == "--version" || name == "-V" {
		return RunVersionCLI(args[1:], stdout, stderr)
	}
	handler, ok := Commands[name]
	if !ok {
		fmt.Fprintf(stderr, "unknown subcommand: %s\n", name)
		return 2
	}
	return handler(args[1:], stdout, stderr)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `usage: ldgr <subcommand> [args]

Subcommands:
  init          initialize ledger files
  ticket        append or update ticket rows
  claim         claim or release shared coordination resources
  note          append shared coordination notes
  worklog       append completed-work rows
  audit         append audit pass or changes-requested rows
  verify        run ledger guardrails
  next          show next actions for a ticket/project
  suggest       print suggested JSON skeletons
  import        import legacy ledger files
  migrate       migrate legacy shapes
  view          start the local web viewer
  hooks         install repository hooks
  instructions  install or remove agent guide pointers
  registry      manage the project registry
  version       print the installed version`)
}
