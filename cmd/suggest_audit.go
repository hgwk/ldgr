package cmd

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/hgwk/ldgr/internal/guidance"
	"github.com/hgwk/ldgr/internal/ledger"
)

func suggestAudit(latest ledger.Row, worklog []ledger.Row, writingLanguage string, stdout io.Writer) int {
	if _, isState := latest["state"]; isState {
		return suggestAuditState(latest, worklog, writingLanguage, stdout)
	}
	// Check if latest status is audit_ready
	if status, _ := latest["status"].(string); status != "audit_ready" {
		g := guidance.Compute(latest, worklog)
		g.WritingLanguage = writingLanguage
		fmt.Fprint(stdout, guidance.RenderText(g))
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Ticket must be in 'audit_ready' status to emit audit skeletons.")
		return 0
	}

	// Extract reviewed_n from the latest audit_ready row's n field
	reviewedN := int(latest["n"].(float64))

	// Build two skeletons: pass and changes_requested
	pass := map[string]any{
		"ticket":       latest["ticket"],
		"role":         "audit",
		"status":       "done",
		"audit_result": "pass",
		"evidence":     []any{"test:unit:<command-or-test-marker>"},
		"reviewed_n":   reviewedN,
	}
	changes := map[string]any{
		"ticket":       latest["ticket"],
		"role":         "audit",
		"status":       "changes_requested",
		"audit_result": "changes_requested",
		"audit_notes":  "",
		"evidence":     []any{},
		"reviewed_n":   reviewedN,
	}
	addWritingLanguage(pass, writingLanguage)
	addWritingLanguage(changes, writingLanguage)

	skeletons := []map[string]any{pass, changes}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(skeletons); err != nil {
		return 1
	}
	return 0
}

func suggestAuditState(latest ledger.Row, worklog []ledger.Row, writingLanguage string, stdout io.Writer) int {
	if state, _ := latest["state"].(string); state != "review" {
		g := guidance.ComputeState(latest, worklog)
		g.WritingLanguage = writingLanguage
		fmt.Fprint(stdout, guidance.RenderText(g))
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Ticket must be in 'review' state to emit state-model audit skeletons.")
		return 0
	}
	reviewedN := int(latest["n"].(float64))
	pass := map[string]any{
		"id":       latest["id"],
		"state":    "done",
		"evidence": []any{"test:unit:<command-or-test-marker>"},
		"event": map[string]any{
			"role":       "auditor",
			"result":     "pass",
			"reviewed_n": reviewedN,
			"summary":    "passed",
			"notes":      "",
		},
	}
	changes := map[string]any{
		"id":    latest["id"],
		"state": "rework",
		"event": map[string]any{
			"role":       "auditor",
			"result":     "changes_requested",
			"reviewed_n": reviewedN,
			"summary":    "changes requested",
			"notes":      "",
		},
	}
	addWritingLanguage(pass, writingLanguage)
	addWritingLanguage(changes, writingLanguage)
	skeletons := []map[string]any{pass, changes}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(skeletons); err != nil {
		return 1
	}
	return 0
}
