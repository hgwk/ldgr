package viewer

import (
	"testing"
	"time"

	"github.com/hgwk/ldgr/internal/ledger"
)

func TestBuildAuditQueue_OnlyAuditReady(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	latest := []ledger.Row{
		{"n": float64(1), "ticket": "A", "status": "audit_ready", "ts": "2026-05-15T10:00:00Z", "priority": "P2"},
		{"n": float64(2), "ticket": "B", "status": "in_progress", "ts": "2026-05-15T10:01:00Z", "priority": "P0"},
		{"n": float64(3), "ticket": "C", "status": "done", "ts": "2026-05-15T10:02:00Z"},
	}
	q := BuildAuditQueue(latest, now)
	if len(q) != 1 || q[0].TicketID != "A" {
		t.Fatalf("want only A, got %+v", q)
	}
}

func TestBuildAuditQueue_PriorityOrder(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	latest := []ledger.Row{
		{"n": float64(1), "ticket": "A", "status": "audit_ready", "ts": "2026-05-15T10:00:00Z", "priority": "P3"},
		{"n": float64(2), "ticket": "B", "status": "audit_ready", "ts": "2026-05-15T10:00:00Z", "priority": "P0"},
		{"n": float64(3), "ticket": "C", "status": "audit_ready", "ts": "2026-05-15T10:00:00Z", "priority": "P2"},
		{"n": float64(4), "ticket": "D", "status": "audit_ready", "ts": "2026-05-15T10:00:00Z", "priority": "P1"},
	}
	q := BuildAuditQueue(latest, now)
	got := []string{q[0].TicketID, q[1].TicketID, q[2].TicketID, q[3].TicketID}
	want := []string{"B", "D", "C", "A"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("priority order: want %v got %v", want, got)
		}
	}
}

func TestBuildAuditQueue_AgeTiebreak(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	latest := []ledger.Row{
		{"n": float64(1), "ticket": "newer", "status": "audit_ready", "ts": "2026-05-15T11:00:00Z", "priority": "P1"},
		{"n": float64(2), "ticket": "older", "status": "audit_ready", "ts": "2026-05-15T08:00:00Z", "priority": "P1"},
	}
	q := BuildAuditQueue(latest, now)
	if len(q) != 2 || q[0].TicketID != "older" {
		t.Fatalf("older P1 should come first, got %+v", q)
	}
}

func TestBuildAuditQueue_MissingPriorityDefaultsP2(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	latest := []ledger.Row{
		{"n": float64(1), "ticket": "noprio", "status": "audit_ready", "ts": "2026-05-15T10:00:00Z"},
		{"n": float64(2), "ticket": "p1", "status": "audit_ready", "ts": "2026-05-15T10:00:00Z", "priority": "P1"},
		{"n": float64(3), "ticket": "p3", "status": "audit_ready", "ts": "2026-05-15T10:00:00Z", "priority": "P3"},
	}
	q := BuildAuditQueue(latest, now)
	if q[0].TicketID != "p1" || q[1].TicketID != "noprio" || q[2].TicketID != "p3" {
		t.Fatalf("missing priority should sort as P2, got %+v", q)
	}
	if q[1].Priority != "P2" {
		t.Fatalf("missing priority should normalize to P2, got %q", q[1].Priority)
	}
}

func TestBuildAuditQueue_FieldPropagation(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	latest := []ledger.Row{
		{"n": float64(1), "ticket": "T-1", "status": "audit_ready", "ts": "2026-05-15T09:00:00Z",
			"priority": "P0", "task": "Audit me", "claimed_by": "alice", "agent": "alice",
			"evidence": []any{"link"}},
		{"n": float64(2), "ticket": "T-2", "status": "audit_ready", "ts": "2026-05-15T09:00:00Z",
			"priority": "P0", "task": "No ev", "evidence": []any{}},
	}
	q := BuildAuditQueue(latest, now)
	if len(q) != 2 {
		t.Fatalf("want 2 items, got %d", len(q))
	}
	var t1, t2 AuditQueueItem
	for _, it := range q {
		if it.TicketID == "T-1" {
			t1 = it
		}
		if it.TicketID == "T-2" {
			t2 = it
		}
	}
	if t1.Task != "Audit me" || t1.ClaimedBy != "alice" || !t1.HasEvidence {
		t.Fatalf("T-1 fields wrong: %+v", t1)
	}
	if t2.HasEvidence {
		t.Fatalf("T-2 should not have evidence: %+v", t2)
	}
}

// --- Lifecycle latency --------------------------------------------------------

// lcRow builds a ticket history row with n, status, ts, and optional extras
// (role, audit_result). Tests share this helper to stay readable.
