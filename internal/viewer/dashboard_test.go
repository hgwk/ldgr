package viewer

import (
	"fmt"
	"testing"
	"time"

	"github.com/hgwk/ldgr/internal/ledger"
)

func TestDashboard_ProgressExcludesCancelled(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	tickets := []ledger.Row{
		{"n": float64(1), "ticket": "A", "status": "done", "parent_ticket": "P", "audit_result": "pass", "ts": "2026-05-14T10:00:00Z"},
		{"n": float64(2), "ticket": "B", "status": "open", "parent_ticket": "P", "ts": "2026-05-14T10:01:00Z", "blocked_by": []any{}},
		{"n": float64(3), "ticket": "C", "status": "cancelled", "parent_ticket": "P", "ts": "2026-05-14T10:02:00Z"},
	}
	d := BuildDashboard(tickets, nil, now)
	if d.Progress.Done != 1 || d.Progress.Active != 1 || d.Progress.Cancelled != 1 {
		t.Fatalf("progress counts wrong: %+v", d.Progress)
	}
	if d.Progress.Percent != 50 {
		t.Fatalf("expected percent=50 (1/2), got %d", d.Progress.Percent)
	}
}

func TestDashboard_ParentCompletion(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	tickets := []ledger.Row{
		{"n": float64(1), "ticket": "A1", "status": "done", "parent_ticket": "DOC", "audit_result": "pass", "ts": "2026-05-14T10:00:00Z"},
		{"n": float64(2), "ticket": "A2", "status": "blocked", "parent_ticket": "DOC", "ts": "2026-05-14T10:01:00Z", "blocked_by": []any{"X"}},
		{"n": float64(3), "ticket": "B1", "status": "open", "parent_ticket": "BUG", "ts": "2026-05-14T10:02:00Z", "blocked_by": []any{}},
	}
	d := BuildDashboard(tickets, nil, now)
	if len(d.Parents) != 2 {
		t.Fatalf("expected 2 parents, got %d", len(d.Parents))
	}
	parents := map[string]ParentProgress{}
	for _, p := range d.Parents {
		parents[p.Parent] = p
	}
	if parents["DOC"].Done != 1 || parents["DOC"].Active != 1 || parents["DOC"].Blocked != 1 {
		t.Fatalf("DOC counts wrong: %+v", parents["DOC"])
	}
	if parents["DOC"].Percent != 50 {
		t.Fatalf("DOC percent=50 expected, got %d", parents["DOC"].Percent)
	}
	if parents["BUG"].Active != 1 || parents["BUG"].Percent != 0 {
		t.Fatalf("BUG wrong: %+v", parents["BUG"])
	}
}

func TestDashboard_AuditPipeline(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	tickets := []ledger.Row{
		{"n": float64(1), "ticket": "A", "status": "audit_ready", "parent_ticket": "P", "ts": "2026-05-14T10:00:00Z"},
		{"n": float64(2), "ticket": "B", "status": "changes_requested", "parent_ticket": "P", "ts": "2026-05-14T10:01:00Z"},
		{"n": float64(3), "ticket": "C", "status": "done", "parent_ticket": "P", "ts": "2026-05-14T10:02:00Z"}, // weak done
		{"n": float64(4), "ticket": "D", "status": "done", "parent_ticket": "P", "audit_result": "pass", "ts": "2026-05-14T10:03:00Z"},
	}
	d := BuildDashboard(tickets, nil, now)
	if d.Audit.AuditReady != 1 || d.Audit.ChangesRequested != 1 || d.Audit.WeakDone != 1 {
		t.Fatalf("audit pipeline wrong: %+v", d.Audit)
	}
}

func TestDashboard_DeliveryHealth(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	tickets := []ledger.Row{
		{"n": float64(1), "ticket": "A", "status": "done", "parent_ticket": "P", "audit_result": "pass", "ts": "2026-05-14T10:00:00Z"},
		{"n": float64(2), "ticket": "GHOST", "status": "open", "task": "", "parent_ticket": "LEGACY", "ts": "2026-05-14T10:01:00Z"},
		{"n": float64(3), "ticket": "INV", "status": "cancelled", "parent_ticket": "LEGACY", "invalidates_n": float64(2), "ts": "2026-05-14T10:02:00Z"},
	}
	worklog := []ledger.Row{
		{"n": float64(1), "ts": "2026-05-14T10:03:00Z", "ticket": "ORPHAN", "task": "?"},
	}
	d := BuildDashboard(tickets, worklog, now)
	if d.Health.ClosedWithoutWorklog == 0 {
		t.Fatalf("expected closed_without_worklog>0, got %+v", d.Health)
	}
	if d.Health.OrphanWorklog != 1 {
		t.Fatalf("orphan_worklog=1 expected, got %d", d.Health.OrphanWorklog)
	}
	if d.Health.Invalidated < 1 {
		t.Fatalf("invalidated>=1 expected, got %d", d.Health.Invalidated)
	}
}

func TestDashboard_DroppedWithoutWorklogIsNotMissingDelivery(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	tickets := []ledger.Row{
		{"n": float64(1), "id": "DROP-1", "state": "dropped", "parent": "P", "ts": "2026-05-14T10:00:00Z"},
		{"n": float64(2), "id": "DONE-1", "state": "done", "parent": "P", "ts": "2026-05-14T10:01:00Z", "event": map[string]any{"role": "auditor", "result": "pass"}},
	}
	d := BuildDashboard(tickets, nil, now)
	if d.Health.ClosedWithoutWorklog != 1 {
		t.Fatalf("only done should require worklog, got %+v", d.Health)
	}
}

func TestDashboard_RecentActivityNewestFirstLimited(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	var tickets []ledger.Row
	for i := 0; i < 15; i++ {
		tickets = append(tickets, ledger.Row{
			"n":             float64(i + 1),
			"ticket":        fmt.Sprintf("T-%d", i),
			"status":        "open",
			"parent_ticket": "P",
			"task":          fmt.Sprintf("task %d", i),
			"ts":            fmt.Sprintf("2026-05-14T%02d:00:00Z", i),
			"blocked_by":    []any{},
		})
	}
	var worklog []ledger.Row
	for i := 0; i < 10; i++ {
		worklog = append(worklog, ledger.Row{
			"n":      float64(i + 1),
			"ticket": fmt.Sprintf("T-%d", i),
			"ts":     fmt.Sprintf("2026-05-14T%02d:30:00Z", i),
			"task":   "wl",
			"result": "ok",
		})
	}
	d := BuildDashboard(tickets, worklog, now)
	if len(d.Recent) != 20 {
		t.Fatalf("expected 20 recent items, got %d", len(d.Recent))
	}
	// newest first
	for i := 1; i < len(d.Recent); i++ {
		if d.Recent[i-1].TS < d.Recent[i].TS {
			t.Fatalf("recent not newest-first at %d: %s before %s", i, d.Recent[i-1].TS, d.Recent[i].TS)
		}
	}
}

func TestDashboard_RecentActivityExcludesInvalidatedRows(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	tickets := []ledger.Row{
		{"n": float64(1), "ticket": "GHOST", "status": "open", "parent_ticket": "P", "ts": "2026-05-14T10:00:00Z"},
		{"n": float64(2), "ticket": "INV-T", "status": "cancelled", "parent_ticket": "LEGACY", "invalidates_n": float64(1), "ts": "2026-05-14T10:01:00Z"},
		{"n": float64(3), "ticket": "LIVE", "status": "open", "parent_ticket": "P", "ts": "2026-05-14T10:02:00Z"},
	}
	worklog := []ledger.Row{
		{"n": float64(1), "ticket": "GHOST-W", "ts": "2026-05-14T10:03:00Z", "task": "ghost", "result": "bad"},
		{"n": float64(2), "ts": "2026-05-14T10:04:00Z", "task": "invalidate worklog", "result": "bad", "invalidates_n": float64(1)},
		{"n": float64(3), "ticket": "LIVE", "ts": "2026-05-14T10:05:00Z", "task": "live", "result": "ok"},
	}
	d := BuildDashboard(tickets, worklog, now)
	for _, item := range d.Recent {
		if item.Ticket == "GHOST" || item.Ticket == "GHOST-W" || item.Ticket == "INV-T" {
			t.Fatalf("invalidated row leaked into recent activity: %+v", d.Recent)
		}
	}
}
func TestDashboard_PriorityCountsActiveOnly(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	tickets := []ledger.Row{
		{"n": float64(1), "ticket": "P0A", "status": "open", "priority": "P0", "parent_ticket": "P", "ts": "2026-05-14T10:00:00Z", "blocked_by": []any{}},
		{"n": float64(2), "ticket": "P0D", "status": "done", "priority": "P0", "audit_result": "pass", "parent_ticket": "P", "ts": "2026-05-14T10:00:00Z"},
		{"n": float64(3), "ticket": "P1A", "status": "in_progress", "priority": "P1", "parent_ticket": "P", "ts": "2026-05-14T10:00:00Z", "blocked_by": []any{}},
		{"n": float64(4), "ticket": "P2A", "status": "blocked", "priority": "P2", "parent_ticket": "P", "ts": "2026-05-14T10:00:00Z", "blocked_by": []any{"X"}},
	}
	d := BuildDashboard(tickets, nil, now)
	if d.Priority.P0 != 1 {
		t.Fatalf("P0 active expected 1, got %d", d.Priority.P0)
	}
	if d.Priority.P1 != 1 {
		t.Fatalf("P1 expected 1, got %d", d.Priority.P1)
	}
	if d.Priority.P2 != 1 {
		t.Fatalf("P2 expected 1, got %d", d.Priority.P2)
	}
}

func TestDashboard_KindDistribution(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	tickets := []ledger.Row{
		{"n": float64(1), "ticket": "A", "status": "open", "kind": "task", "parent_ticket": "P", "ts": "2026-05-14T10:00:00Z", "blocked_by": []any{}},
		{"n": float64(2), "ticket": "B", "status": "open", "kind": "task", "parent_ticket": "P", "ts": "2026-05-14T10:00:00Z", "blocked_by": []any{}},
		{"n": float64(3), "ticket": "C", "status": "open", "kind": "issue", "parent_ticket": "P", "ts": "2026-05-14T10:00:00Z", "blocked_by": []any{}},
	}
	d := BuildDashboard(tickets, nil, now)
	if len(d.Kind) != 2 {
		t.Fatalf("expected 2 kinds, got %d (%+v)", len(d.Kind), d.Kind)
	}
	if d.Kind[0].Kind != "task" || d.Kind[0].Count != 2 {
		t.Fatalf("expected task=2 first, got %+v", d.Kind[0])
	}
	if d.Kind[1].Kind != "issue" || d.Kind[1].Count != 1 {
		t.Fatalf("expected issue=1 second, got %+v", d.Kind[1])
	}
}
