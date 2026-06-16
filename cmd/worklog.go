package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/hgwk/ldgr/internal/agent"
	"github.com/hgwk/ldgr/internal/guidance"
	"github.com/hgwk/ldgr/internal/ledger"
	"github.com/hgwk/ldgr/internal/lifecycle"
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
	if isStateTarget(dir) {
		row, err := normalizeStateWorklog(dir, input, stderr)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		out, err := ledger.Append(filepath.Join(dir, "ledger", "worklog.jsonl"), ldgrLockPath(dir), ledger.Row(row))
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return encErr(enc.Encode(out), stderr)
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
	ensureRowTSAfter(row, latest)

	out, err := ledger.Append(filepath.Join(dir, "ledger", "worklog.jsonl"), ldgrLockPath(dir), ledger.Row(row))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	emitWorklogGuidance(dir, out, stderr)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return encErr(enc.Encode(out), stderr)
}

func normalizeStateWorklog(dir string, input map[string]any, stderr io.Writer) (map[string]any, error) {
	row, err := autoFieldsStateWorklog(input, stderr)
	if err != nil {
		return nil, err
	}
	if err := requireFields(row, withoutN(ledger.StateWorklogRequired), "worklog"); err != nil {
		return nil, err
	}
	if err := requireNonEmpty(row, ledger.StateWorklogNonEmpty, "worklog"); err != nil {
		return nil, err
	}
	ticketID, _ := row["ticket"].(string)
	ticketRows, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		return nil, err
	}
	latest, ok := findLatestStateTicket(ticketRows, ticketID)
	if !ok {
		return nil, fmt.Errorf("worklog: ticket %q does not exist", ticketID)
	}
	if !isStateWorklogAllowed(latest) {
		return nil, fmt.Errorf("worklog: ticket %q is not audit-pass done; cannot record a delivery yet", ticketID)
	}
	ensureRowTSAfter(row, latest)
	return row, nil
}

func autoFieldsStateWorklog(in map[string]any, stderr io.Writer) (map[string]any, error) {
	if _, ok := in["ts"]; !ok || in["ts"] == "" {
		in["ts"] = time.Now().UTC().Format("2006-01-02T15:04:05Z")
	}
	envMap := envAsMap()
	actor, _ := in["actor"].(string)
	resolved, warn, err := agent.Resolve(actor, envMap)
	if err != nil {
		return nil, err
	}
	if actor == "" {
		in["actor"] = resolved
	}
	if _, ok := in["notes"]; !ok {
		in["notes"] = ""
	}
	if warn != "" {
		fmt.Fprintln(stderr, "warning:", warn)
	}
	return in, nil
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
	g.WritingLanguage = loadWritingLanguage(dir)
	fmt.Fprint(stderr, guidance.RenderText(g))
}

// isWorklogAllowed returns true iff the latest ticket row has a strong
// audit-pass closure.
func isWorklogAllowed(latest ledger.Row) bool {
	return lifecycle.IsAuditPassDone(latest)
}

func isStateWorklogAllowed(latest ledger.Row) bool {
	if state, _ := latest["state"].(string); state != "done" {
		return false
	}
	event, _ := latest["event"].(map[string]any)
	if event == nil {
		return false
	}
	if role, _ := event["role"].(string); role != "auditor" {
		return false
	}
	if result, _ := event["result"].(string); result != "pass" {
		return false
	}
	return hasPositiveStateNumber(event["reviewed_n"]) && ledger.HasTestEvidence(stateEvidence(latest))
}

func findLatestStateTicket(rows []ledger.Row, id string) (ledger.Row, bool) {
	var latest ledger.Row
	for _, r := range rows {
		if r["id"] == id {
			latest = r
		}
	}
	return latest, latest != nil
}

func ensureRowTSAfter(row, prior ledger.Row) {
	rowTS, _ := row["ts"].(string)
	priorTS, _ := prior["ts"].(string)
	if rowTS == "" || priorTS == "" {
		return
	}
	priorTime, err := time.Parse(time.RFC3339Nano, priorTS)
	if err != nil {
		return
	}
	rowTime, err := time.Parse(time.RFC3339Nano, rowTS)
	if err != nil || rowTime.After(priorTime) {
		return
	}
	row["ts"] = priorTime.Add(time.Second).UTC().Format("2006-01-02T15:04:05Z")
}
