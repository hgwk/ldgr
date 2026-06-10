package ledger

import "testing"

func TestPolicy_StatusTransitions(t *testing.T) {
	if !AllowsCompatStatusTransition("in_progress", "audit_ready") {
		t.Fatal("expected in_progress -> audit_ready")
	}
	if AllowsCompatStatusTransition("open", "done") {
		t.Fatal("did not expect open -> done")
	}
	next := NextCompatStatuses("audit_ready")
	next[0] = "mutated"
	if NextCompatStatuses("audit_ready")[0] != "done" {
		t.Fatal("NextCompatStatuses should return a copy")
	}
}

func TestPolicy_StateTransitionsAndBoard(t *testing.T) {
	if !AllowsStateTransition("review", "done") {
		t.Fatal("expected review -> done")
	}
	if AllowsStateTransition("ready", "done") {
		t.Fatal("did not expect ready -> done")
	}
	if got := StatusToState("changes_requested"); got != "rework" {
		t.Fatalf("changes_requested maps to %q", got)
	}
	cols := BoardColumns()
	if len(cols) != 8 || cols[0].ID != "ready" || cols[7].ID != "dropped" {
		t.Fatalf("unexpected kanban columns: %+v", cols)
	}
	cols[0].ID = "mutated"
	if BoardColumns()[0].ID != "ready" {
		t.Fatal("BoardColumns should return a copy")
	}
}
