package guidance

import (
	"testing"

	"github.com/hgwk/ldgr/internal/ledger"
)

func TestCompareGitToTickets_UntrackedChanges(t *testing.T) {
	tickets := []ledger.Row{
		{"ticket": "A", "status": "in_progress", "paths": []any{"src/a.go"}},
	}
	findings := CompareGitToTickets([]string{"src/a.go", "src/b.go"}, tickets, "")
	if len(findings.Untracked) != 1 || findings.Untracked[0] != "src/b.go" {
		t.Fatalf("expected src/b.go untracked, got %+v", findings.Untracked)
	}
}

func TestCompareGitToTickets_IdleTicketFocus(t *testing.T) {
	tickets := []ledger.Row{
		{"ticket": "A", "status": "in_progress", "paths": []any{"src/a.go"}},
		{"ticket": "B", "status": "in_progress", "paths": []any{"src/b.go"}},
	}
	findings := CompareGitToTickets([]string{"src/a.go"}, tickets, "B")
	if len(findings.IdleTickets) != 1 || findings.IdleTickets[0] != "B" {
		t.Fatalf("expected B idle, got %+v", findings.IdleTickets)
	}
}

func TestCompareGitToTickets_NoChangesAndIdleTicket(t *testing.T) {
	tickets := []ledger.Row{
		{"ticket": "A", "status": "in_progress", "paths": []any{"src/a.go"}},
	}
	findings := CompareGitToTickets(nil, tickets, "A")
	// No changes means the in_progress ticket A is idle (has paths but no changed files).
	if len(findings.Untracked) != 0 || len(findings.IdleTickets) != 1 {
		t.Fatalf("expected A idle with no changes: %+v", findings)
	}
}

func TestCompareGitToTickets_NoPathsNoIdleWarning(t *testing.T) {
	// Ticket with empty paths list should never be marked as idle.
	tickets := []ledger.Row{
		{"ticket": "A", "status": "in_progress", "paths": []any{}},
	}
	findings := CompareGitToTickets(nil, tickets, "A")
	if len(findings.IdleTickets) != 0 {
		t.Fatalf("empty paths should not trigger idle: %+v", findings)
	}
}
