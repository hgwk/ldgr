// Package lifecycle implements the audit-gate state machine validator.
// It is pure: no I/O, no globals. Callers pass a proposed ticket row and
// the prior latest row for the same ticket (or nil for new tickets),
// and receive a typed Violation pointer (nil means accept).
package lifecycle

import (
	"fmt"
	"strings"

	"github.com/hgwk/ldgr/internal/ledger"
)

// Violation describes a rejected transition.
type Violation struct {
	Code    string
	Message string
	Hint    string
}

func (v *Violation) Error() string {
	if v.Hint == "" {
		return v.Message
	}
	return v.Message + "\n" + v.Hint
}

// Allowed state transitions: prevStatus -> set of allowed next statuses.
// "" (empty) represents the initial state (nil prev) for new tickets.
var allowedTransitions = map[string]map[string]bool{
	"":                  {"open": true, "in_progress": true},
	"open":              {"in_progress": true, "blocked": true, "cancelled": true},
	"in_progress":       {"audit_ready": true, "blocked": true, "cancelled": true},
	"blocked":           {"in_progress": true, "cancelled": true},
	"audit_ready":       {"done": true, "changes_requested": true, "cancelled": true},
	"changes_requested": {"in_progress": true, "open": true, "cancelled": true},
	"done":              {}, // terminal
	"cancelled":         {}, // terminal
}

// Validate checks the proposed new row against the previous latest row
// for the same ticket. prev is nil iff this is the first row for the
// ticket (ticket add).
func Validate(row ledger.Row, prev ledger.Row) *Violation {
	status, _ := row["status"].(string)

	// Correction-row escape hatch: invalidates_n rows bypass all checks.
	if _, hasInv := row["invalidates_n"]; hasInv {
		return nil
	}

	// Status enum check (only if status is set; some events may not carry one,
	// e.g. metadata corrections — those are also accepted).
	if status != "" {
		if _, ok := ledger.StatusEnum[status]; !ok {
			return &Violation{
				Code:    "INVALID_STATUS",
				Message: fmt.Sprintf("status %q is not a valid lifecycle state", status),
				Hint:    "Valid statuses: open, in_progress, blocked, audit_ready, changes_requested, done, cancelled.\n",
			}
		}
	}

	// Transition graph enforcement (only when status is set).
	if status != "" {
		prevStatus := ""
		if prev != nil {
			prevStatus, _ = prev["status"].(string)
		}
		// Same-status carry-forward is always allowed (metadata-only updates).
		if prevStatus != status {
			allowed := allowedTransitions[prevStatus]
			if !allowed[status] {
				ticket := stringFromRow(row, "ticket")
				prevStatusDisplay := orPlaceholder(prevStatus, "<new>")
				return &Violation{
					Code:    "INVALID_TRANSITION",
					Message: fmt.Sprintf("lifecycle does not allow %s -> %s.", prevStatusDisplay, status),
					Hint:    fmt.Sprintf("Use status=audit_ready first, then run:\n  ldgr next --ticket %s\n", orPlaceholder(ticket, "<ticket>")),
				}
			}
		}
	}

	// Per-status field-level rules.
	switch status {
	case "audit_ready":
		if !hasNonEmptyStringList(row, "evidence") {
			return &Violation{
				Code:    "AUDIT_READY_NEEDS_EVIDENCE",
				Message: "audit_ready requires non-empty evidence.",
				Hint:    "Add `evidence` array of verification commands or artifacts.\n",
			}
		}
	case "done":
		if !isAuditPassClose(row) {
			ticket := orPlaceholder(stringFromRow(row, "ticket"), "<ticket>")
			return &Violation{
				Code:    "IMPL_DIRECT_DONE",
				Message: "impl delivery cannot move directly to done.",
				Hint: "Use status=audit_ready first, then run:\n" +
					fmt.Sprintf("  ldgr next --ticket %s\n", ticket),
			}
		}
		if !hasPositiveNumber(row, "reviewed_n") {
			return &Violation{
				Code:    "AUDIT_PASS_NEEDS_REVIEWED_N",
				Message: "audit pass row requires reviewed_n pointing at the audit_ready row.",
				Hint:    "Include `reviewed_n: <impl row n>` in the audit pass JSON.\n",
			}
		}
	case "changes_requested":
		if r, _ := row["role"].(string); r != "audit" {
			return changesRequestedViolation()
		}
		if ar, _ := row["audit_result"].(string); ar != "changes_requested" {
			return changesRequestedViolation()
		}
		if notes, _ := row["audit_notes"].(string); strings.TrimSpace(notes) == "" {
			return changesRequestedViolation()
		}
		if !hasPositiveNumber(row, "reviewed_n") {
			return changesRequestedViolation()
		}
	}

	return nil
}

// changesRequestedViolation returns a standard violation for invalid changes_requested rows.
func changesRequestedViolation() *Violation {
	return &Violation{
		Code:    "CHANGES_REQUESTED_INVALID",
		Message: "changes_requested must be a role=audit row with audit_result=changes_requested, audit_notes, and reviewed_n.",
		Hint:    "Use `ldgr audit request-changes --ticket <id> --notes ...` or include all required fields.\n",
	}
}

// isAuditPassClose recognises the only legal `status=done` path:
// role=audit AND audit_result=pass AND non-empty evidence array.
// Evidence entries that are not non-empty strings are ignored; the audit
// passes if at least one trimmed-non-empty string is present.
func isAuditPassClose(row ledger.Row) bool {
	if r, _ := row["role"].(string); r != "audit" {
		return false
	}
	if ar, _ := row["audit_result"].(string); ar != "pass" {
		return false
	}
	return hasNonEmptyStringList(row, "evidence")
}

// hasNonEmptyStringList checks if the given key contains a non-empty list
// of strings (filtering out empty-string entries).
func hasNonEmptyStringList(row ledger.Row, key string) bool {
	arr, _ := row[key].([]any)
	for _, item := range arr {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			return true
		}
	}
	return false
}

// hasPositiveNumber checks if the given key contains a positive number.
func hasPositiveNumber(row ledger.Row, key string) bool {
	switch v := row[key].(type) {
	case float64:
		return v > 0
	case int:
		return v > 0
	}
	return false
}

// stringFromRow safely extracts a string value from the row.
func stringFromRow(row ledger.Row, key string) string {
	v, _ := row[key].(string)
	return v
}

// orPlaceholder returns the given string if non-empty, otherwise the placeholder.
func orPlaceholder(s, placeholder string) string {
	if s == "" {
		return placeholder
	}
	return s
}
