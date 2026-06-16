package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/hgwk/ldgr/internal/ledger"
	"github.com/hgwk/ldgr/internal/lifecycle"
)

func runTicketAdd(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := newFlagSet("ticket add")
	target := fs.String("target", "", "")
	jsonSpec := fs.String("json", "", "")
	example := fs.Bool("example", false, "print an example state-model ticket JSON")
	userApproved := fs.String("user-approved", "", "record explicit user approval reason on the ticket")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *example {
		fmt.Fprintln(stdout, ticketAddExample())
		return 0
	}
	dir := resolveTarget(*target)

	input, err := ReadJSONInput(*jsonSpec, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	recordUserApproval(input, *userApproved)

	row, err := normalizeTicketAdd(dir, input, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		if isStateTicketInput(input) {
			fmt.Fprintln(stderr, "hint: run `ldgr ticket add --example` for a complete state-model JSON payload")
		}
		return 1
	}

	path := filepath.Join(dir, "ledger", "tickets.jsonl")
	lock := ldgrLockPath(dir)
	out, err := ledger.Append(path, lock, ledger.Row(row))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	emitTicketGuidance(dir, out, stderr)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return encErr(enc.Encode(out), stderr)
}

func recordUserApproval(input map[string]any, reason string) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return
	}
	marker := "user_approved:" + reason
	if event, ok := input["event"].(map[string]any); ok {
		event["notes"] = appendMarker(stringValue(event["notes"]), marker)
	}
	if evidence, ok := input["evidence"].([]any); ok && !containsEvidenceMarker(evidence, marker) {
		input["evidence"] = append(evidence, marker)
	}
	if !isStateTicketInput(input) {
		input["notes"] = appendMarker(stringValue(input["notes"]), marker)
	}
}

func appendMarker(notes, marker string) string {
	notes = strings.TrimSpace(notes)
	if notes == "" {
		return marker
	}
	if strings.Contains(notes, marker) || strings.Contains(notes, "user_approved:") {
		return notes
	}
	return notes + "\n" + marker
}

func containsEvidenceMarker(evidence []any, marker string) bool {
	for _, entry := range evidence {
		if entry == marker {
			return true
		}
	}
	return false
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func normalizeTicketAdd(dir string, input map[string]any, stderr io.Writer) (map[string]any, error) {
	if isStateTicketInput(input) {
		return normalizeStateTicketAdd(dir, input, stderr)
	}
	ticket, _ := input["ticket"].(string)
	if ticket == "" {
		return nil, errors.New("ticket: field 'ticket' is required")
	}
	rows, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r["ticket"] == ticket {
			return nil, fmt.Errorf("ticket %q already exists (use `ticket event` to update)", ticket)
		}
	}

	// Set defaults for kind and priority
	if _, has := input["kind"]; !has {
		input["kind"] = "task"
	}
	if _, has := input["priority"]; !has {
		input["priority"] = "P2"
	}

	resolved, err := autoFields(dir, input, stderr)
	if err != nil {
		return nil, err
	}
	// Check all required fields except 'n' which is assigned by Append
	required := make([]string, 0, len(ledger.TicketRequired))
	for _, f := range ledger.TicketRequired {
		if f != "n" {
			required = append(required, f)
		}
	}
	if err := requireFields(resolved, required, "ticket"); err != nil {
		return nil, err
	}
	if err := requireNonEmpty(resolved, ledger.TicketNonEmpty, "ticket"); err != nil {
		return nil, err
	}
	if v := lifecycle.Validate(ledger.Row(resolved), nil); v != nil {
		return nil, fmt.Errorf("%s\n%s", v.Message, v.Hint)
	}
	return resolved, nil
}

func ticketAddExample() string {
	return `{
  "id": "agent-guide-parent-sync",
  "parent": "ROOT",
  "type": "task",
  "state": "ready",
  "area": "docs",
  "priority": "P2",
  "title": "Record AGENTS.md guide pointer update",
  "owner": "codex",
  "blocked_by": [],
  "acceptance": ["AGENTS.md / CLAUDE.md guide pointer state is recorded"],
  "evidence": [],
  "event": {
    "actor": "codex",
    "role": "planner",
    "summary": "Record agent guide update",
    "notes": "agent guide updated"
  }
}`
}
