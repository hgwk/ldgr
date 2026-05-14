package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/hgwk/ldgr/internal/guidance"
	"github.com/hgwk/ldgr/internal/ledger"
)

func init() {
	Commands["worklog"] = func(args []string, stdout, stderr io.Writer) int {
		return RunWorklogCLI(args, os.Stdin, stdout, stderr)
	}
}

func RunWorklogCLI(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "add" {
		fmt.Fprintln(stderr, "usage: ldgr worklog add --json @-")
		return 2
	}
	fs := newFlagSet("worklog add")
	target := fs.String("target", "", "")
	jsonSpec := fs.String("json", "", "")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	dir := resolveTarget(*target)

	input, err := ReadJSONInput(*jsonSpec, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	row, err := autoFields(dir, input, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if _, hasCommit := row["commit"]; !hasCommit {
		row["commit"] = ""
	}
	if _, hasNotes := row["notes"]; !hasNotes {
		row["notes"] = ""
	}
	// Check all required fields except 'n' which is assigned by Append
	required := make([]string, 0, len(ledger.WorklogRequired))
	for _, f := range ledger.WorklogRequired {
		if f != "n" {
			required = append(required, f)
		}
	}
	if err := requireFields(row, required, "worklog"); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := requireNonEmpty(row, ledger.WorklogNonEmpty, "worklog"); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	// Audit-pass gate: worklog rows must reference an existing ticket
	// whose latest row is an audit-pass done row.
	ticketID, _ := input["ticket"].(string)
	if ticketID == "" {
		fmt.Fprintln(stderr, "worklog: ticket is required for delivery records.")
		fmt.Fprintln(stderr, "  Worklog rows without a ticket are reserved for ldgr-internal automations (e.g. goal set --log).")
		return 1
	}
	ticketRows, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	latest, ok := findLatestTicket(ticketRows, ticketID)
	if !ok {
		fmt.Fprintf(stderr, "worklog: ticket %q does not exist.\n", ticketID)
		return 1
	}
	if !isWorklogAllowed(latest) {
		fmt.Fprintf(stderr, "worklog: ticket %q is not audit-pass done; cannot record a delivery yet.\n", ticketID)
		fmt.Fprintf(stderr, "  Run `ldgr next --ticket %s` for the required audit step.\n", ticketID)
		return 1
	}

	out, err := ledger.Append(filepath.Join(dir, "ledger", "worklog.jsonl"), filepath.Join(dir, "ledger", ".lock"), ledger.Row(row))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	emitWorklogGuidance(dir, out, stderr)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return encErr(enc.Encode(out), stderr)
}

// emitWorklogGuidance emits guidance for the ticket associated with the worklog entry,
// if present. If the worklog row has no ticket field, guidance is skipped.
func emitWorklogGuidance(dir string, row map[string]any, stderr io.Writer) {
	id, _ := row["ticket"].(string)
	if id == "" {
		return
	}
	tickets, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		return
	}
	latest, ok := findLatestTicket(tickets, id)
	if !ok {
		return
	}
	worklog, _ := ledger.ReadRows(filepath.Join(dir, "ledger", "worklog.jsonl"))
	g := guidance.Compute(latest, worklog)
	fmt.Fprint(stderr, guidance.RenderText(g))
}

// isWorklogAllowed returns true iff the latest ticket row has a confirmed
// audit-pass closure (status=done, role=audit, audit_result=pass).
func isWorklogAllowed(latest ledger.Row) bool {
	if s, _ := latest["status"].(string); s != "done" {
		return false
	}
	if r, _ := latest["role"].(string); r != "audit" {
		return false
	}
	if ar, _ := latest["audit_result"].(string); ar != "pass" {
		return false
	}
	return true
}
