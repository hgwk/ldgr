package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/hgwk/ldgr/internal/ledger"
)

func init() {
	Commands["audit"] = func(args []string, stdout, stderr io.Writer) int {
		return RunAuditCLI(args, nil, stdout, stderr)
	}
}

// RunAuditCLI handles `ldgr audit <pass|request-changes>`.
func RunAuditCLI(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: ldgr audit <pass|request-changes> --ticket ID ...")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "pass":
		return runAuditPass(rest, stdout, stderr)
	case "request-changes":
		return runAuditRequestChanges(rest, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown audit subcommand: %s\n", sub)
		return 2
	}
}

func runAuditPass(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("audit pass")
	target := fs.String("target", "", "")
	ticket := fs.String("ticket", "", "")
	var evidence multiString
	fs.Var(&evidence, "evidence", "evidence (repeatable)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *ticket == "" {
		fmt.Fprintln(stderr, "--ticket is required")
		return 2
	}
	if len(evidence) == 0 {
		fmt.Fprintln(stderr, "at least one --evidence is required")
		return 2
	}
	dir := resolveTarget(*target)
	if isStateTarget(dir) {
		reviewedN, err := findStateReviewedN(dir, *ticket)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		input := map[string]any{
			"id":       *ticket,
			"state":    "done",
			"evidence": stringsToAny(evidence),
			"event": map[string]any{
				"role":       "auditor",
				"result":     "pass",
				"reviewed_n": float64(reviewedN),
				"summary":    "passed",
				"notes":      "",
			},
		}
		return appendTicketEvent(dir, input, stdout, stderr)
	}
	reviewedN, err := findReviewedN(dir, *ticket)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	input := map[string]any{
		"ticket":       *ticket,
		"role":         "audit",
		"status":       "done",
		"audit_result": "pass",
		"evidence":     stringsToAny(evidence),
		"reviewed_n":   float64(reviewedN),
	}
	return appendTicketEvent(dir, input, stdout, stderr)
}

func runAuditRequestChanges(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("audit request-changes")
	target := fs.String("target", "", "")
	ticket := fs.String("ticket", "", "")
	notes := fs.String("notes", "", "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *ticket == "" {
		fmt.Fprintln(stderr, "--ticket is required")
		return 2
	}
	if *notes == "" {
		fmt.Fprintln(stderr, "--notes is required")
		return 2
	}
	dir := resolveTarget(*target)
	if isStateTarget(dir) {
		reviewedN, err := findStateReviewedN(dir, *ticket)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		input := map[string]any{
			"id":    *ticket,
			"state": "rework",
			"event": map[string]any{
				"role":       "auditor",
				"result":     "changes_requested",
				"reviewed_n": float64(reviewedN),
				"summary":    "changes requested",
				"notes":      *notes,
			},
		}
		return appendTicketEvent(dir, input, stdout, stderr)
	}
	reviewedN, err := findReviewedN(dir, *ticket)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	input := map[string]any{
		"ticket":       *ticket,
		"role":         "audit",
		"status":       "changes_requested",
		"audit_result": "changes_requested",
		"audit_notes":  *notes,
		"reviewed_n":   float64(reviewedN),
	}
	return appendTicketEvent(dir, input, stdout, stderr)
}

// findStateReviewedN walks ticket rows to find the most recent review row for the
// state-model ticket. The name is historical and should be collapsed in a
// later mechanical rename.
func findStateReviewedN(dir, ticket string) (int, error) {
	rows, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		return 0, err
	}
	var best float64 = -1
	for _, r := range rows {
		if id, _ := r["id"].(string); id != ticket {
			continue
		}
		if s, _ := r["state"].(string); s != "review" {
			continue
		}
		if n, _ := r["n"].(float64); n > best {
			best = n
		}
	}
	if best < 0 {
		return 0, fmt.Errorf("audit pass requires a prior review row; run `ldgr ticket event --json @-` with state=review first.")
	}
	return int(best), nil
}

// findReviewedN walks ticket rows to find the most recent audit_ready row for the ticket.
func findReviewedN(dir, ticket string) (int, error) {
	rows, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		return 0, err
	}
	var best float64 = -1
	for _, r := range rows {
		if id, _ := r["ticket"].(string); id != ticket {
			continue
		}
		if s, _ := r["status"].(string); s != "audit_ready" {
			continue
		}
		if n, _ := r["n"].(float64); n > best {
			best = n
		}
	}
	if best < 0 {
		return 0, fmt.Errorf("audit pass requires a prior audit_ready row; run `ldgr ticket ready --ticket %s --evidence ...` first.", ticket)
	}
	return int(best), nil
}

// appendTicketEvent is a small wrapper that runs the same pipeline as ticket event.
func appendTicketEvent(dir string, input map[string]any, stdout, stderr io.Writer) int {
	row, err := normalizeTicketEvent(dir, input, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	out, err := ledger.Append(filepath.Join(dir, "ledger", "tickets.jsonl"), ldgrLockPath(dir), ledger.Row(row))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	warnMissingGitCompletionEvidence(out, stderr)
	emitTicketGuidance(dir, out, stderr)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return encErr(enc.Encode(out), stderr)
}
