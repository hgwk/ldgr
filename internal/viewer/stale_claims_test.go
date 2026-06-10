package viewer

import (
	"testing"
	"time"

	"github.com/hgwk/ldgr/internal/ledger"
)

func TestDashboard_StaleClaims_NoClaims(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	tickets := []ledger.Row{
		{"n": float64(1), "ticket": "A", "status": "open", "parent_ticket": "P", "ts": "2026-05-15T10:00:00Z"},
	}
	d := BuildDashboard(tickets, nil, now)
	if d.StaleClaims.Expired != 0 || d.StaleClaims.NearExpiring != 0 || len(d.StaleClaims.Samples) != 0 {
		t.Fatalf("expected zero stale claims, got %+v", d.StaleClaims)
	}
}

func TestDashboard_StaleClaims_ExpiredOnly(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	tickets := []ledger.Row{
		{"n": float64(1), "ticket": "A", "status": "in_progress", "parent_ticket": "P",
			"ts": "2026-05-15T08:00:00Z", "claim_until": "2026-05-15T11:00:00Z", "claimed_by": "agent-1"},
		{"n": float64(2), "ticket": "B", "status": "in_progress", "parent_ticket": "P",
			"ts": "2026-05-15T08:00:00Z", "claim_until": "2026-05-15T10:00:00Z", "claimed_by": "agent-2"},
	}
	d := BuildDashboard(tickets, nil, now)
	if d.StaleClaims.Expired != 2 || d.StaleClaims.NearExpiring != 0 {
		t.Fatalf("counts wrong: %+v", d.StaleClaims)
	}
	if len(d.StaleClaims.Samples) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(d.StaleClaims.Samples))
	}
	// Most overdue first: B (10:00) before A (11:00).
	if d.StaleClaims.Samples[0].TicketID != "B" {
		t.Fatalf("expected B most overdue first, got %+v", d.StaleClaims.Samples)
	}
}

func TestDashboard_StaleClaims_NearExpiringOnly(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	tickets := []ledger.Row{
		// within 2h window
		{"n": float64(1), "ticket": "A", "status": "in_progress", "parent_ticket": "P",
			"ts": "2026-05-15T08:00:00Z", "claim_until": "2026-05-15T13:00:00Z", "claimed_by": "agent-1"},
		// outside the 2h window — must not count
		{"n": float64(2), "ticket": "B", "status": "in_progress", "parent_ticket": "P",
			"ts": "2026-05-15T08:00:00Z", "claim_until": "2026-05-15T18:00:00Z", "claimed_by": "agent-2"},
	}
	d := BuildDashboard(tickets, nil, now)
	if d.StaleClaims.Expired != 0 || d.StaleClaims.NearExpiring != 1 {
		t.Fatalf("counts wrong: %+v", d.StaleClaims)
	}
	if len(d.StaleClaims.Samples) != 1 || d.StaleClaims.Samples[0].TicketID != "A" {
		t.Fatalf("expected only A in samples, got %+v", d.StaleClaims.Samples)
	}
}

func TestDashboard_StaleClaims_BothExpiredFirst(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	tickets := []ledger.Row{
		{"n": float64(1), "ticket": "NEAR", "status": "in_progress", "parent_ticket": "P",
			"ts": "2026-05-15T08:00:00Z", "claim_until": "2026-05-15T13:00:00Z"},
		{"n": float64(2), "ticket": "EXP", "status": "in_progress", "parent_ticket": "P",
			"ts": "2026-05-15T08:00:00Z", "claim_until": "2026-05-15T11:00:00Z"},
	}
	d := BuildDashboard(tickets, nil, now)
	if d.StaleClaims.Expired != 1 || d.StaleClaims.NearExpiring != 1 {
		t.Fatalf("counts wrong: %+v", d.StaleClaims)
	}
	if d.StaleClaims.Samples[0].TicketID != "EXP" {
		t.Fatalf("expected expired sample first, got %+v", d.StaleClaims.Samples)
	}
}

func TestDashboard_StaleClaims_ExcludesTerminal(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	tickets := []ledger.Row{
		{"n": float64(1), "ticket": "A", "status": "done", "parent_ticket": "P",
			"ts": "2026-05-15T08:00:00Z", "claim_until": "2026-05-15T10:00:00Z", "audit_result": "pass"},
		{"n": float64(2), "ticket": "B", "status": "cancelled", "parent_ticket": "P",
			"ts": "2026-05-15T08:00:00Z", "claim_until": "2026-05-15T10:00:00Z"},
	}
	d := BuildDashboard(tickets, nil, now)
	if d.StaleClaims.Expired != 0 || d.StaleClaims.NearExpiring != 0 {
		t.Fatalf("terminal tickets must be excluded, got %+v", d.StaleClaims)
	}
}

func TestDashboard_StaleClaims_UnparseableIgnored(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	tickets := []ledger.Row{
		{"n": float64(1), "ticket": "A", "status": "in_progress", "parent_ticket": "P",
			"ts": "2026-05-15T08:00:00Z", "claim_until": "not-a-time"},
		{"n": float64(2), "ticket": "B", "status": "in_progress", "parent_ticket": "P",
			"ts": "2026-05-15T08:00:00Z", "claim_until": ""},
	}
	d := BuildDashboard(tickets, nil, now)
	if d.StaleClaims.Expired != 0 || d.StaleClaims.NearExpiring != 0 {
		t.Fatalf("unparseable claim_until must be silently ignored, got %+v", d.StaleClaims)
	}
}
