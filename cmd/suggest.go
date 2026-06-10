package cmd

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/hgwk/ldgr/internal/ledger"
)

func init() {
	Commands["suggest"] = RunSuggestCLI
}

// RunSuggestCLI implements `ldgr suggest <worklog|commit|audit|correction|plan|pr> --ticket ID [--options]`.
func RunSuggestCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: ldgr suggest <worklog|commit|audit|correction|plan|pr> --ticket ID [--options]")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "worklog":
		return suggestWorklogCmd(rest, stdout, stderr)
	case "commit":
		return suggestCommitCmd(rest, stdout, stderr)
	case "audit":
		return suggestAuditCmd(rest, stdout, stderr)
	case "correction":
		return suggestCorrectionCmd(rest, stdout, stderr)
	case "plan":
		return suggestPlanCmd(rest, stdout, stderr)
	case "pr":
		return suggestPRCmd(rest, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown suggest subcommand: %s\n", sub)
		return 2
	}
}

// loadTicketContext loads the ticket context for suggest subcommands.
// For "plan", latest may be nil (creating a new ticket).
// For others, latest must exist.
func loadTicketContext(target, ticket string, allowNew bool, stderr io.Writer) (ledger.Row, []ledger.Row, []ledger.Row, string, int) {
	if ticket == "" {
		fmt.Fprintln(stderr, "--ticket is required")
		return nil, nil, nil, "", 2
	}
	dir := resolveTarget(target)
	rows, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return nil, nil, nil, "", 1
	}
	var latest ledger.Row
	var ok bool
	if isStateTarget(dir) {
		latest, ok = findLatestStateTicket(rows, ticket)
	} else {
		latest, ok = findLatestTicket(rows, ticket)
	}
	if !ok && !allowNew {
		fmt.Fprintf(stderr, "ticket %q not found\n", ticket)
		return nil, nil, nil, "", 1
	}
	worklog, _ := ledger.ReadRows(filepath.Join(dir, "ledger", "worklog.jsonl"))
	return latest, rows, worklog, dir, 0
}
