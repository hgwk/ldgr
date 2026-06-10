package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/hgwk/ldgr/internal/ledger"
)

func runTicketReady(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("ticket ready")
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
	// Build event JSON as map, hand to normalizeTicketEvent.
	input := map[string]any{}
	if isStateTarget(dir) {
		input = map[string]any{
			"id":       *ticket,
			"state":    "review",
			"evidence": stringsToAny(evidence),
			"event": map[string]any{
				"role":    "implementer",
				"summary": "ready for review",
				"notes":   "",
			},
		}
	} else {
		input = map[string]any{
			"ticket":   *ticket,
			"status":   "audit_ready",
			"evidence": stringsToAny(evidence),
		}
	}
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
	emitTicketGuidance(dir, out, stderr)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return encErr(enc.Encode(out), stderr)
}
