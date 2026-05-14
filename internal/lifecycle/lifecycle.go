// Package lifecycle implements the audit-gate state machine validator.
// It is pure: no I/O, no globals. Callers pass a proposed ticket row and
// receive a typed Violation pointer (nil means accept).
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

// Validate returns nil when row is an acceptable proposed ticket event,
// otherwise a *Violation describing the rejection.
func Validate(row ledger.Row) *Violation {
	status, _ := row["status"].(string)

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

	// Audit-gate enforcement: status=done is only legal as an audit-pass row
	// or as an explicit correction/cancellation row.
	if status == "done" {
		if isCorrectionRow(row) || isAuditPassClose(row) {
			return nil
		}
		ticket, _ := row["ticket"].(string)
		if ticket == "" {
			ticket = "<ticket>"
		}
		return &Violation{
			Code:    "IMPL_DIRECT_DONE",
			Message: "impl delivery cannot move directly to done.",
			Hint: "Use status=audit_ready first, then run:\n" +
				fmt.Sprintf("  ldgr next --ticket %s\n", ticket),
		}
	}

	return nil
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
	ev, _ := row["evidence"].([]any)
	if len(ev) == 0 {
		return false
	}
	// Filter out empty-string evidence entries to avoid trivially passing.
	for _, item := range ev {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			return true
		}
	}
	return false
}

// isCorrectionRow recognises explicit correction rows that bypass the audit
// gate: rows carrying `invalidates_n`. Cancellation rows do not need an
// override since Validate accepts status=cancelled upstream.
func isCorrectionRow(row ledger.Row) bool {
	_, hasInv := row["invalidates_n"]
	return hasInv
}
