package lifecycle

import (
	"strings"
	"testing"

	"github.com/hgwk/ldgr/internal/ledger"
)

func row(fields map[string]any) ledger.Row {
	out := ledger.Row{
		"ticket": "T-1", "parent_ticket": "P", "role": "impl",
		"task": "demo", "scope": "repo", "paths": []any{}, "blocked_by": []any{},
	}
	for k, v := range fields {
		out[k] = v
	}
	return out
}

func TestValidate_AcceptsOpen(t *testing.T) {
	if v := Validate(row(map[string]any{"status": "open"}), nil); v != nil {
		t.Fatalf("expected accept, got %v", v)
	}
}

func TestValidate_AcceptsInProgress(t *testing.T) {
	if v := Validate(row(map[string]any{"status": "in_progress"}), nil); v != nil {
		t.Fatalf("expected accept, got %v", v)
	}
}

func TestValidate_AcceptsAuditReady(t *testing.T) {
	prev := row(map[string]any{"status": "in_progress"})
	if v := Validate(row(map[string]any{"status": "audit_ready", "evidence": []any{"go test"}}), prev); v != nil {
		t.Fatalf("expected accept, got %v", v)
	}
}

func TestValidate_AcceptsChangesRequested(t *testing.T) {
	prev := row(map[string]any{"status": "audit_ready", "evidence": []any{"go test"}})
	if v := Validate(row(map[string]any{
		"status": "changes_requested", "role": "audit",
		"audit_result": "changes_requested", "audit_notes": "needs work",
		"reviewed_n": float64(1),
	}), prev); v != nil {
		t.Fatalf("expected accept, got %v", v)
	}
}

func TestValidate_AcceptsCancelled(t *testing.T) {
	prev := row(map[string]any{"status": "in_progress"})
	if v := Validate(row(map[string]any{"status": "cancelled"}), prev); v != nil {
		t.Fatalf("expected accept, got %v", v)
	}
}

func TestValidate_RejectsImplDirectDone(t *testing.T) {
	// Fresh ticket can't go to done (violates transition graph).
	v := Validate(row(map[string]any{"status": "done"}), nil)
	if v == nil || v.Code != "INVALID_TRANSITION" {
		t.Fatalf("expected INVALID_TRANSITION violation for fresh->done, got %v", v)
	}
	if !strings.Contains(v.Hint, "audit_ready") {
		t.Fatalf("hint should mention audit_ready, got %q", v.Hint)
	}
}

func TestValidate_RejectsAuditDoneWithoutPass(t *testing.T) {
	prev := row(map[string]any{"status": "audit_ready", "evidence": []any{"x"}})
	v := Validate(row(map[string]any{"status": "done", "role": "audit", "audit_result": "changes_requested", "evidence": []any{"x"}}), prev)
	if v == nil || v.Code != "IMPL_DIRECT_DONE" {
		t.Fatalf("expected reject when audit_result != pass, got %v", v)
	}
}

func TestValidate_RejectsAuditPassWithoutEvidence(t *testing.T) {
	prev := row(map[string]any{"status": "audit_ready", "evidence": []any{"x"}})
	v := Validate(row(map[string]any{"status": "done", "role": "audit", "audit_result": "pass"}), prev)
	if v == nil || v.Code != "IMPL_DIRECT_DONE" {
		t.Fatalf("expected reject when evidence is missing, got %v", v)
	}
	v = Validate(row(map[string]any{"status": "done", "role": "audit", "audit_result": "pass", "evidence": []any{}}), prev)
	if v == nil || v.Code != "IMPL_DIRECT_DONE" {
		t.Fatalf("expected reject when evidence is empty array, got %v", v)
	}
}

func TestValidate_AcceptsAuditPassWithEvidence(t *testing.T) {
	prev := row(map[string]any{"status": "audit_ready", "evidence": []any{"x"}})
	v := Validate(row(map[string]any{
		"status":       "done",
		"role":         "audit",
		"audit_result": "pass",
		"evidence":     []any{"go test ./..."},
		"reviewed_n":   float64(1),
	}), prev)
	if v != nil {
		t.Fatalf("expected accept for audit pass with evidence, got %v", v)
	}
}

// status=cancelled is always accepted regardless of role; ops cancellation
// is therefore just one expression of that rule.
func TestValidate_AcceptsOpsCancellation(t *testing.T) {
	prev := row(map[string]any{"status": "in_progress"})
	v := Validate(row(map[string]any{"status": "cancelled", "role": "ops"}), prev)
	if v != nil {
		t.Fatalf("status=cancelled should pass, got %v", v)
	}
}

func TestValidate_AcceptsInvalidatesNRow(t *testing.T) {
	// invalidates_n rows are correction rows; allowed regardless of status fields.
	v := Validate(row(map[string]any{"status": "done", "role": "impl", "invalidates_n": float64(7)}), nil)
	if v != nil {
		t.Fatalf("invalidates_n row should pass even with impl/done, got %v", v)
	}
}

func TestValidate_RejectsUnknownStatus(t *testing.T) {
	v := Validate(row(map[string]any{"status": "wat"}), nil)
	if v == nil || v.Code != "INVALID_STATUS" {
		t.Fatalf("expected INVALID_STATUS, got %v", v)
	}
}

func TestValidate_ErrorImplementsError(t *testing.T) {
	v := Validate(row(map[string]any{"status": "done"}), nil)
	if v == nil {
		t.Fatal("expected violation")
	}
	if v.Error() == "" {
		t.Fatal("Error() should be non-empty")
	}
}

func TestValidate_AcceptsBlocked(t *testing.T) {
	prev := row(map[string]any{"status": "in_progress"})
	v := Validate(row(map[string]any{"status": "blocked", "blocked_by": []any{"X-1"}}), prev)
	if v != nil {
		t.Fatalf("expected accept, got %v", v)
	}
}

func TestValidate_RejectsAuditReadyWithoutEvidence(t *testing.T) {
	prev := row(map[string]any{"status": "in_progress"})
	v := Validate(row(map[string]any{"status": "audit_ready"}), prev)
	if v == nil || v.Code != "AUDIT_READY_NEEDS_EVIDENCE" {
		t.Fatalf("want AUDIT_READY_NEEDS_EVIDENCE, got %v", v)
	}
}

func TestValidate_AcceptsAuditReadyWithEvidence(t *testing.T) {
	prev := row(map[string]any{"status": "in_progress"})
	v := Validate(row(map[string]any{"status": "audit_ready", "evidence": []any{"go test ./..."}}), prev)
	if v != nil {
		t.Fatalf("want accept, got %v", v)
	}
}

func TestValidate_RejectsAuditPassWithoutReviewedN(t *testing.T) {
	prev := row(map[string]any{"status": "audit_ready", "evidence": []any{"x"}})
	v := Validate(row(map[string]any{
		"status": "done", "role": "audit", "audit_result": "pass",
		"evidence": []any{"go test"},
	}), prev)
	if v == nil || v.Code != "AUDIT_PASS_NEEDS_REVIEWED_N" {
		t.Fatalf("want AUDIT_PASS_NEEDS_REVIEWED_N, got %v", v)
	}
}

func TestValidate_AcceptsAuditPassWithReviewedN(t *testing.T) {
	prev := row(map[string]any{"n": float64(7), "status": "audit_ready", "evidence": []any{"x"}})
	v := Validate(row(map[string]any{
		"status": "done", "role": "audit", "audit_result": "pass",
		"evidence": []any{"go test"}, "reviewed_n": float64(7),
	}), prev)
	if v != nil {
		t.Fatalf("want accept, got %v", v)
	}
}

func TestValidate_RejectsAuditPassReviewedNMismatch(t *testing.T) {
	prev := row(map[string]any{"n": float64(7), "status": "audit_ready", "evidence": []any{"x"}})
	v := Validate(row(map[string]any{
		"status": "done", "role": "audit", "audit_result": "pass",
		"evidence": []any{"go test"}, "reviewed_n": float64(6),
	}), prev)
	if v == nil || v.Code != "AUDIT_PASS_NEEDS_REVIEWED_N" {
		t.Fatalf("want AUDIT_PASS_NEEDS_REVIEWED_N mismatch, got %v", v)
	}
}

func TestValidate_RejectsChangesRequestedMissingFields(t *testing.T) {
	prev := row(map[string]any{"status": "audit_ready", "evidence": []any{"x"}})
	// Missing audit_notes.
	v := Validate(row(map[string]any{
		"status": "changes_requested", "role": "audit",
		"audit_result": "changes_requested", "reviewed_n": float64(7),
	}), prev)
	if v == nil || v.Code != "CHANGES_REQUESTED_INVALID" {
		t.Fatalf("want CHANGES_REQUESTED_INVALID for missing notes, got %v", v)
	}
	// Missing reviewed_n.
	v = Validate(row(map[string]any{
		"status": "changes_requested", "role": "audit",
		"audit_result": "changes_requested", "audit_notes": "missing tests",
	}), prev)
	if v == nil || v.Code != "CHANGES_REQUESTED_INVALID" {
		t.Fatalf("want CHANGES_REQUESTED_INVALID for missing reviewed_n, got %v", v)
	}
}

func TestValidate_AcceptsChangesRequestedWithAllFields(t *testing.T) {
	prev := row(map[string]any{"n": float64(7), "status": "audit_ready", "evidence": []any{"x"}})
	v := Validate(row(map[string]any{
		"status": "changes_requested", "role": "audit",
		"audit_result": "changes_requested",
		"audit_notes":  "missing regression coverage",
		"reviewed_n":   float64(7),
	}), prev)
	if v != nil {
		t.Fatalf("want accept, got %v", v)
	}
}

func TestValidate_RejectsChangesRequestedReviewedNMismatch(t *testing.T) {
	prev := row(map[string]any{"n": float64(7), "status": "audit_ready", "evidence": []any{"x"}})
	v := Validate(row(map[string]any{
		"status": "changes_requested", "role": "audit",
		"audit_result": "changes_requested",
		"audit_notes":  "missing regression coverage",
		"reviewed_n":   float64(5),
	}), prev)
	if v == nil || v.Code != "CHANGES_REQUESTED_INVALID" {
		t.Fatalf("want CHANGES_REQUESTED_INVALID mismatch, got %v", v)
	}
}

func TestValidate_RejectsInvalidTransitionFromOpen(t *testing.T) {
	prev := row(map[string]any{"status": "open"})
	// open -> audit_ready is not a legal edge.
	v := Validate(row(map[string]any{"status": "audit_ready", "evidence": []any{"x"}}), prev)
	if v == nil || v.Code != "INVALID_TRANSITION" {
		t.Fatalf("want INVALID_TRANSITION, got %v", v)
	}
}

func TestValidate_AcceptsSameStatusCarryForward(t *testing.T) {
	prev := row(map[string]any{"status": "in_progress"})
	v := Validate(row(map[string]any{"status": "in_progress", "notes": "more work"}), prev)
	if v != nil {
		t.Fatalf("same-status carry-forward should pass, got %v", v)
	}
}

func TestValidate_RejectsTransitionsOutOfTerminalDone(t *testing.T) {
	prev := row(map[string]any{
		"status": "done", "role": "audit", "audit_result": "pass",
		"evidence": []any{"x"}, "reviewed_n": float64(1),
	})
	v := Validate(row(map[string]any{"status": "in_progress"}), prev)
	if v == nil || v.Code != "INVALID_TRANSITION" {
		t.Fatalf("want INVALID_TRANSITION leaving done, got %v", v)
	}
}

func TestValidate_AllowsCorrectionFromTerminal(t *testing.T) {
	prev := row(map[string]any{
		"status": "done", "role": "audit", "audit_result": "pass",
		"evidence": []any{"x"}, "reviewed_n": float64(1),
	})
	v := Validate(row(map[string]any{
		"status": "cancelled", "role": "ops", "invalidates_n": float64(1),
	}), prev)
	if v != nil {
		t.Fatalf("correction row from terminal should pass, got %v", v)
	}
}

func TestValidate_FirstRowOpenAccepted(t *testing.T) {
	if v := Validate(row(map[string]any{"status": "open"}), nil); v != nil {
		t.Fatalf("want accept for fresh open ticket, got %v", v)
	}
}

func TestValidate_FirstRowAuditReadyRejected(t *testing.T) {
	v := Validate(row(map[string]any{"status": "audit_ready", "evidence": []any{"x"}}), nil)
	if v == nil || v.Code != "INVALID_TRANSITION" {
		t.Fatalf("first row can't be audit_ready, got %v", v)
	}
}
