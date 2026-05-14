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
	if v := Validate(row(map[string]any{"status": "open"})); v != nil {
		t.Fatalf("expected accept, got %v", v)
	}
}

func TestValidate_AcceptsInProgress(t *testing.T) {
	if v := Validate(row(map[string]any{"status": "in_progress"})); v != nil {
		t.Fatalf("expected accept, got %v", v)
	}
}

func TestValidate_AcceptsAuditReady(t *testing.T) {
	if v := Validate(row(map[string]any{"status": "audit_ready"})); v != nil {
		t.Fatalf("expected accept, got %v", v)
	}
}

func TestValidate_AcceptsChangesRequested(t *testing.T) {
	if v := Validate(row(map[string]any{"status": "changes_requested"})); v != nil {
		t.Fatalf("expected accept, got %v", v)
	}
}

func TestValidate_AcceptsCancelled(t *testing.T) {
	if v := Validate(row(map[string]any{"status": "cancelled"})); v != nil {
		t.Fatalf("expected accept, got %v", v)
	}
}

func TestValidate_RejectsImplDirectDone(t *testing.T) {
	v := Validate(row(map[string]any{"status": "done"}))
	if v == nil || v.Code != "IMPL_DIRECT_DONE" {
		t.Fatalf("expected IMPL_DIRECT_DONE violation, got %v", v)
	}
	if !strings.Contains(v.Hint, "audit_ready") || !strings.Contains(v.Hint, "ldgr next") {
		t.Fatalf("hint should mention audit_ready and ldgr next, got %q", v.Hint)
	}
	if !strings.Contains(v.Hint, "T-1") {
		t.Fatalf("hint should mention ticket id T-1, got %q", v.Hint)
	}
}

func TestValidate_RejectsAuditDoneWithoutPass(t *testing.T) {
	v := Validate(row(map[string]any{"status": "done", "role": "audit", "audit_result": "changes_requested", "evidence": []any{"x"}}))
	if v == nil || v.Code != "IMPL_DIRECT_DONE" {
		t.Fatalf("expected reject when audit_result != pass, got %v", v)
	}
}

func TestValidate_RejectsAuditPassWithoutEvidence(t *testing.T) {
	v := Validate(row(map[string]any{"status": "done", "role": "audit", "audit_result": "pass"}))
	if v == nil || v.Code != "IMPL_DIRECT_DONE" {
		t.Fatalf("expected reject when evidence is missing, got %v", v)
	}
	v = Validate(row(map[string]any{"status": "done", "role": "audit", "audit_result": "pass", "evidence": []any{}}))
	if v == nil || v.Code != "IMPL_DIRECT_DONE" {
		t.Fatalf("expected reject when evidence is empty array, got %v", v)
	}
}

func TestValidate_AcceptsAuditPassWithEvidence(t *testing.T) {
	v := Validate(row(map[string]any{
		"status":       "done",
		"role":         "audit",
		"audit_result": "pass",
		"evidence":     []any{"go test ./..."},
	}))
	if v != nil {
		t.Fatalf("expected accept for audit pass with evidence, got %v", v)
	}
}

// status=cancelled is always accepted regardless of role; ops cancellation
// is therefore just one expression of that rule.
func TestValidate_AcceptsOpsCancellation(t *testing.T) {
	v := Validate(row(map[string]any{"status": "cancelled", "role": "ops"}))
	if v != nil {
		t.Fatalf("status=cancelled should pass, got %v", v)
	}
}

func TestValidate_AcceptsInvalidatesNRow(t *testing.T) {
	// invalidates_n rows are correction rows; allowed regardless of status fields.
	v := Validate(row(map[string]any{"status": "done", "role": "impl", "invalidates_n": float64(7)}))
	if v != nil {
		t.Fatalf("invalidates_n row should pass even with impl/done, got %v", v)
	}
}

func TestValidate_RejectsUnknownStatus(t *testing.T) {
	v := Validate(row(map[string]any{"status": "wat"}))
	if v == nil || v.Code != "INVALID_STATUS" {
		t.Fatalf("expected INVALID_STATUS, got %v", v)
	}
}

func TestValidate_ErrorImplementsError(t *testing.T) {
	v := Validate(row(map[string]any{"status": "done"}))
	if v == nil {
		t.Fatal("expected violation")
	}
	if v.Error() == "" {
		t.Fatal("Error() should be non-empty")
	}
}

func TestValidate_AcceptsBlocked(t *testing.T) {
	v := Validate(row(map[string]any{"status": "blocked", "blocked_by": []any{"X-1"}}))
	if v != nil {
		t.Fatalf("expected accept, got %v", v)
	}
}
