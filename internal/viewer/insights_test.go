package viewer

import (
	"testing"
	"time"

	"github.com/hgwk/ldgr/internal/ledger"
)

func TestInsights_ReadyQueueExcludesBlocked(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	tickets := []ledger.Row{
		{"n": float64(1), "ticket": "READY-1", "status": "open", "ts": "2026-05-14T10:00:00Z", "blocked_by": []any{}, "parent_ticket": "BUG", "task": "ready"},
		{"n": float64(2), "ticket": "BLOCKED-1", "status": "open", "ts": "2026-05-14T10:01:00Z", "blocked_by": []any{"READY-1"}, "parent_ticket": "BUG", "task": "blocked"},
	}
	ins := BuildInsights(tickets, nil, now, 24)
	if len(ins.ReadyQueue) != 1 || ins.ReadyQueue[0]["ticket"] != "READY-1" {
		t.Fatalf("ready queue should be [READY-1], got %+v", ins.ReadyQueue)
	}
}

func TestInsights_TopBlockersAggregatesDependents(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	tickets := []ledger.Row{
		{"n": float64(1), "ticket": "BUG-9", "status": "in_progress", "blocked_by": []any{}, "parent_ticket": "BUG"},
		{"n": float64(2), "ticket": "BUG-1", "status": "open", "blocked_by": []any{"BUG-9"}, "parent_ticket": "BUG"},
		{"n": float64(3), "ticket": "BUG-2", "status": "open", "blocked_by": []any{"BUG-9"}, "parent_ticket": "BUG"},
	}
	ins := BuildInsights(tickets, nil, now, 24)
	if len(ins.TopBlockers) == 0 || ins.TopBlockers[0].Ticket != "BUG-9" {
		t.Fatalf("expected BUG-9 as top blocker, got %+v", ins.TopBlockers)
	}
	if len(ins.TopBlockers[0].Dependents) != 2 {
		t.Fatalf("expected 2 dependents, got %v", ins.TopBlockers[0].Dependents)
	}
}

func TestInsights_StaleInProgressByLastTouch(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	tickets := []ledger.Row{
		{"n": float64(1), "ticket": "STALE-1", "status": "in_progress", "ts": "2026-05-12T10:00:00Z", "blocked_by": []any{}, "parent_ticket": "BUG"},
	}
	// No worklog rows so age_ms = now - ticket ts.
	ins := BuildInsights(tickets, nil, now, 24)
	if len(ins.StaleInProgress) != 1 {
		t.Fatalf("want stale entry, got %+v", ins.StaleInProgress)
	}
	if ins.StaleInProgress[0].AgeMS < int64(24*time.Hour/time.Millisecond) {
		t.Fatalf("want age > 24h, got %d ms", ins.StaleInProgress[0].AgeMS)
	}
}

func TestInsights_ClosedWithoutWorklog(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	tickets := []ledger.Row{
		{"n": float64(1), "ticket": "DOC-1", "status": "done", "ts": "2026-05-14T10:00:00Z", "blocked_by": []any{}, "parent_ticket": "DOC"},
	}
	ins := BuildInsights(tickets, nil, now, 24)
	if len(ins.ClosedWithoutWorklog) != 1 || ins.ClosedWithoutWorklog[0]["ticket"] != "DOC-1" {
		t.Fatalf("want DOC-1 listed, got %+v", ins.ClosedWithoutWorklog)
	}
}

func TestInsights_DroppedWithoutWorklogIsNotMissingDelivery(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	tickets := []ledger.Row{
		{"n": float64(1), "id": "DROP-1", "state": "dropped", "ts": "2026-05-14T10:00:00Z", "blocked_by": []any{}, "parent": "DOC"},
	}
	ins := BuildInsights(tickets, nil, now, 24)
	if len(ins.ClosedWithoutWorklog) != 0 {
		t.Fatalf("dropped tickets are not delivered work and should not require worklog: %+v", ins.ClosedWithoutWorklog)
	}
}

func TestInsights_WorklogsWithoutTicket(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	worklog := []ledger.Row{
		{"n": float64(1), "ts": "2026-05-14T10:00:00Z", "ticket": "ghost", "task": "x"},
	}
	ins := BuildInsights(nil, worklog, now, 24)
	if len(ins.WorklogsWithoutTicket) != 1 {
		t.Fatalf("want orphan worklog listed, got %+v", ins.WorklogsWithoutTicket)
	}
}

func TestInsights_InvalidatedWorklogExcludedFromCoverage(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	tickets := []ledger.Row{
		{"n": float64(1), "ticket": "DONE-1", "status": "done", "ts": "2026-05-14T10:00:00Z", "blocked_by": []any{}, "parent_ticket": "ROOT"},
	}
	worklog := []ledger.Row{
		{"n": float64(1), "ts": "2026-05-14T10:01:00Z", "ticket": "DONE-1", "task": "ghost", "result": ""},
		{"n": float64(2), "ts": "2026-05-14T10:02:00Z", "task": "invalidate", "result": "invalidate", "invalidates_n": float64(1)},
	}
	ins := BuildInsights(tickets, worklog, now, 24)
	if len(ins.ClosedWithoutWorklog) != 1 {
		t.Fatalf("invalidated worklog must not satisfy coverage, got %+v", ins.ClosedWithoutWorklog)
	}
	if len(ins.WorklogsWithoutTicket) != 0 {
		t.Fatalf("invalidated worklog must not appear as orphan, got %+v", ins.WorklogsWithoutTicket)
	}
}

func TestInsights_InvalidatedReportsCompanion(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	tickets := []ledger.Row{
		{"n": float64(1), "ticket": "ghost", "task": "", "status": "open", "parent_ticket": "LEGACY"},
		{"n": float64(2), "ticket": "x", "task": "invalidate", "status": "cancelled", "parent_ticket": "LEGACY", "invalidates_n": float64(1)},
	}
	ins := BuildInsights(tickets, nil, now, 24)
	if len(ins.Invalidated) == 0 {
		t.Fatalf("want invalidated entry, got %+v", ins.Invalidated)
	}
	if ins.Invalidated[0].N != 1 || ins.Invalidated[0].ViaN != 2 || ins.Invalidated[0].Kind != "ticket" {
		t.Fatalf("invalidated entry shape wrong: %+v", ins.Invalidated[0])
	}
}
