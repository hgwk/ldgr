package cmd

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/hgwk/ldgr/internal/ledger"
)

func normalizeStateTicketAdd(dir string, input map[string]any, stderr io.Writer) (map[string]any, error) {
	id, _ := input["id"].(string)
	if id == "" {
		return nil, errors.New("ticket: field 'id' is required for state-model")
	}
	rows, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r["id"] == id {
			return nil, fmt.Errorf("ticket %q already exists (use `ticket event` to update)", id)
		}
	}
	resolved, err := autoFieldsState(dir, input, stderr)
	if err != nil {
		return nil, err
	}
	required := withoutN(ledger.StateTicketRequired)
	if err := requireFields(resolved, required, "ticket"); err != nil {
		return nil, err
	}
	if err := requireNonEmpty(resolved, ledger.StateTicketNonEmpty, "ticket"); err != nil {
		return nil, err
	}
	if err := requireStateEvent(resolved); err != nil {
		return nil, err
	}
	if err := validateStateTicketWrite(resolved, nil); err != nil {
		return nil, err
	}
	return resolved, nil
}

func normalizeStateTicketEvent(dir string, input map[string]any, stderr io.Writer) (map[string]any, error) {
	id, _ := input["id"].(string)
	if id == "" {
		return nil, errors.New("ticket event: field 'id' is required for state-model")
	}
	rows, err := ledger.ReadRows(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil {
		return nil, err
	}
	var base map[string]any
	var prevRow ledger.Row
	invalidated := invalidatedTicketNs(rows)
	for _, r := range rows {
		if _, isCompanion := r["invalidates_n"]; isCompanion {
			continue
		}
		if n, ok := numberAsPositiveInt(r["n"]); ok && invalidated[n] {
			continue
		}
		if r["id"] == id {
			base = make(map[string]any)
			for k, v := range r {
				base[k] = v
			}
			prevRow = r
		}
	}
	if base == nil {
		return nil, fmt.Errorf("ticket %q does not exist (use `ticket add` first)", id)
	}
	for k, v := range input {
		base[k] = v
	}
	delete(base, "n")
	base["ts"] = ""
	resolved, err := autoFieldsState(dir, base, stderr)
	if err != nil {
		return nil, err
	}
	if err := requireFields(resolved, withoutN(ledger.StateTicketRequired), "ticket"); err != nil {
		return nil, err
	}
	if err := requireNonEmpty(resolved, ledger.StateTicketNonEmpty, "ticket"); err != nil {
		return nil, err
	}
	if err := requireStateEvent(resolved); err != nil {
		return nil, err
	}
	if err := validateStateTicketWrite(resolved, prevRow); err != nil {
		return nil, err
	}
	return resolved, nil
}

func invalidatedTicketNs(rows []ledger.Row) map[int]bool {
	out := map[int]bool{}
	for _, r := range rows {
		if n, ok := numberAsPositiveInt(r["invalidates_n"]); ok {
			out[n] = true
		}
	}
	return out
}
