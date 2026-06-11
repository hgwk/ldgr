package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/hgwk/ldgr/internal/ledger"
	"github.com/hgwk/ldgr/internal/lifecycle"
)

func runTicketEvent(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := newFlagSet("ticket event")
	target := fs.String("target", "", "")
	jsonSpec := fs.String("json", "", "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir := resolveTarget(*target)

	input, err := ReadJSONInput(*jsonSpec, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
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
	warnMissingGitCompletionEvidence(out, stderr)
	emitTicketGuidance(dir, out, stderr)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return encErr(enc.Encode(out), stderr)
}

func normalizeTicketEvent(dir string, input map[string]any, stderr io.Writer) (map[string]any, error) {
	if isStateTicketInput(input) || isStateTarget(dir) {
		return normalizeStateTicketEvent(dir, input, stderr)
	}
	ticket, _ := input["ticket"].(string)
	if ticket == "" {
		return nil, errors.New("ticket event: field 'ticket' is required")
	}
	rows, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		return nil, err
	}
	var base map[string]any
	var prevRow ledger.Row
	for _, r := range rows {
		if r["ticket"] == ticket {
			// Make a copy of the row to avoid modifying the original
			base = make(map[string]any)
			for k, v := range r {
				base[k] = v
			}
			prevRow = r
		}
	}
	if base == nil {
		return nil, fmt.Errorf("ticket %q does not exist (use `ticket add` first)", ticket)
	}
	for k, v := range input {
		base[k] = v
	}
	delete(base, "n")
	base["ts"] = ""

	resolved, err := autoFields(dir, base, stderr)
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
	if v := lifecycle.Validate(ledger.Row(resolved), prevRow); v != nil {
		return nil, fmt.Errorf("%s\n%s", v.Message, v.Hint)
	}
	return resolved, nil
}
