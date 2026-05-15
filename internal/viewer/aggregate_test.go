package viewer

import (
	"fmt"
	"testing"
	"time"

	"github.com/hgwk/ldgr/internal/ledger"
)

func TestLatestTickets_PicksHighestN(t *testing.T) {
	rows := []ledger.Row{
		{"n": float64(1), "ticket": "a", "status": "open", "ts": "2026-05-14T10:00:00Z", "parent_ticket": "ROOT"},
		{"n": float64(2), "ticket": "a", "status": "done", "ts": "2026-05-14T10:05:00Z", "parent_ticket": "ROOT"},
	}
	got := LatestTickets(rows)
	if len(got) != 1 {
		t.Fatalf("want 1 latest, got %d", len(got))
	}
	if got[0]["status"] != "done" {
		t.Fatalf("want latest (status=done), got %v", got[0]["status"])
	}
}

func TestLatestTickets_DropsInvalidated(t *testing.T) {
	rows := []ledger.Row{
		{"n": float64(1), "ticket": "ghost", "status": "open", "ts": "2026-05-14T10:00:00Z", "parent_ticket": "ROOT"},
		{"n": float64(2), "ticket": "legacy-invalid-1", "status": "cancelled", "ts": "2026-05-14T10:00:00Z", "parent_ticket": "LEGACY", "invalidates_n": float64(1)},
	}
	got := LatestTickets(rows)
	for _, r := range got {
		if r["ticket"] == "ghost" {
			t.Fatalf("ghost row should be excluded")
		}
		if _, has := r["invalidates_n"]; has {
			t.Fatalf("companion invalidate row should not appear as a ticket: %+v", r)
		}
	}
}

func TestInvalidatedNs_DetectsCompanion(t *testing.T) {
	rows := []ledger.Row{
		{"n": float64(1), "ticket": "ghost"},
		{"n": float64(2), "ticket": "x", "invalidates_n": float64(1)},
	}
	got := InvalidatedNs(rows)
	if v, ok := got[1]; !ok || v != 2 {
		t.Fatalf("want {1:2}, got %v", got)
	}
}

func TestTree_GroupsByParent(t *testing.T) {
	rows := []ledger.Row{
		{"n": float64(1), "ticket": "BUG-1", "status": "open", "ts": "2026-05-14T10:00:00Z", "parent_ticket": "BUG"},
		{"n": float64(2), "ticket": "FE-1", "status": "open", "ts": "2026-05-14T10:01:00Z", "parent_ticket": "FE"},
	}
	buckets := Tree(rows)
	if len(buckets) != 2 {
		t.Fatalf("want 2 buckets, got %d", len(buckets))
	}
	// Alphabetical by parent.
	if buckets[0].Parent != "BUG" || buckets[1].Parent != "FE" {
		t.Fatalf("want BUG then FE, got %+v", buckets)
	}
}

func TestStatusCounts(t *testing.T) {
	rows := []ledger.Row{
		{"ticket": "a", "status": "open"},
		{"ticket": "b", "status": "open"},
		{"ticket": "c", "status": "done"},
	}
	got := StatusCounts(rows)
	if got["open"] != 2 || got["done"] != 1 {
		t.Fatalf("want {open:2,done:1}, got %v", got)
	}
}

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

func TestKanban_MapsStatusesToColumns(t *testing.T) {
	tickets := []ledger.Row{
		{"n": float64(1), "ticket": "P-1", "status": "open", "parent_ticket": "P", "ts": "2026-05-14T10:00:00Z"},
		{"n": float64(2), "ticket": "I-1", "status": "in_progress", "parent_ticket": "P", "ts": "2026-05-14T10:01:00Z"},
		{"n": float64(3), "ticket": "I-2", "status": "blocked", "parent_ticket": "P", "ts": "2026-05-14T10:02:00Z"},
		{"n": float64(4), "ticket": "I-3", "status": "changes_requested", "parent_ticket": "P", "ts": "2026-05-14T10:03:00Z"},
		{"n": float64(5), "ticket": "V-1", "status": "audit_ready", "parent_ticket": "P", "ts": "2026-05-14T10:04:00Z"},
		{"n": float64(6), "ticket": "C-1", "status": "done", "parent_ticket": "P", "ts": "2026-05-14T10:05:00Z"},
		{"n": float64(7), "ticket": "C-2", "status": "cancelled", "parent_ticket": "P", "ts": "2026-05-14T10:06:00Z"},
	}
	k := BuildKanban(tickets)
	got := map[string][]string{}
	for _, col := range k.Columns {
		for _, t := range col.Tickets {
			id, _ := t["ticket"].(string)
			got[col.ID] = append(got[col.ID], id)
		}
	}
	if !equalSet(got["ready"], []string{"P-1"}) {
		t.Fatalf("ready column wrong: %v", got["ready"])
	}
	if !equalSet(got["doing"], []string{"I-1"}) {
		t.Fatalf("doing column wrong: %v", got["doing"])
	}
	if !equalSet(got["blocked"], []string{"I-2"}) {
		t.Fatalf("blocked column wrong: %v", got["blocked"])
	}
	if !equalSet(got["rework"], []string{"I-3"}) {
		t.Fatalf("rework column wrong: %v", got["rework"])
	}
	if !equalSet(got["review"], []string{"V-1"}) {
		t.Fatalf("review column wrong: %v", got["review"])
	}
	if !equalSet(got["done"], []string{"C-1"}) {
		t.Fatalf("done column wrong: %v", got["done"])
	}
	if !equalSet(got["dropped"], []string{"C-2"}) {
		t.Fatalf("dropped column wrong: %v", got["dropped"])
	}
}

func TestKanban_LegacyInProgressGoesToDoingColumn(t *testing.T) {
	tickets := []ledger.Row{
		{"n": float64(1), "ticket": "PLAN-1", "status": "in_progress", "role": "plan", "parent_ticket": "P", "ts": "2026-05-14T10:00:00Z"},
		{"n": float64(2), "ticket": "IMPL-1", "status": "in_progress", "role": "impl", "parent_ticket": "P", "ts": "2026-05-14T10:01:00Z"},
	}
	k := BuildKanban(tickets)
	got := map[string][]string{}
	for _, col := range k.Columns {
		for _, t := range col.Tickets {
			id, _ := t["ticket"].(string)
			got[col.ID] = append(got[col.ID], id)
		}
	}
	if !equalSet(got["doing"], []string{"PLAN-1", "IMPL-1"}) {
		t.Fatalf("expected active legacy rows in doing, got %v", got["doing"])
	}
}

func TestKanban_LegacyAuditReadyGoesToReview(t *testing.T) {
	tickets := []ledger.Row{
		{"n": float64(1), "ticket": "AUD-1", "status": "audit_ready", "role": "audit", "parent_ticket": "P", "ts": "2026-05-14T10:00:00Z"},
		{"n": float64(2), "ticket": "AUD-2", "status": "done", "role": "audit", "audit_result": "pass", "parent_ticket": "P", "ts": "2026-05-14T10:01:00Z"},
	}
	k := BuildKanban(tickets)
	got := map[string][]string{}
	for _, col := range k.Columns {
		for _, t := range col.Tickets {
			id, _ := t["ticket"].(string)
			got[col.ID] = append(got[col.ID], id)
		}
	}
	if !equalSet(got["review"], []string{"AUD-1"}) {
		t.Fatalf("expected AUD-1 in review, got %v", got["review"])
	}
	if !equalSet(got["done"], []string{"AUD-2"}) {
		t.Fatalf("AUD-2 audit-done should be in done, got %v", got["done"])
	}
}

func TestKanban_ExcludesInvalidatedTickets(t *testing.T) {
	tickets := []ledger.Row{
		{"n": float64(1), "ticket": "GHOST", "status": "open", "parent_ticket": "P", "ts": "2026-05-14T10:00:00Z"},
		{"n": float64(2), "ticket": "INV", "status": "cancelled", "parent_ticket": "LEGACY", "invalidates_n": float64(1), "ts": "2026-05-14T10:01:00Z"},
	}
	k := BuildKanban(tickets)
	for _, col := range k.Columns {
		for _, t := range col.Tickets {
			if t["ticket"] == "GHOST" {
				tFail(col.ID, t)
			}
		}
	}
}

func TestKanban_OrderWithinColumnIsTSDesc(t *testing.T) {
	tickets := []ledger.Row{
		{"n": float64(1), "ticket": "A", "status": "in_progress", "parent_ticket": "P", "ts": "2026-05-14T10:00:00Z"},
		{"n": float64(2), "ticket": "B", "status": "in_progress", "parent_ticket": "P", "ts": "2026-05-14T11:00:00Z"},
	}
	k := BuildKanban(tickets)
	var doing []string
	for _, col := range k.Columns {
		if col.ID == "doing" {
			for _, t := range col.Tickets {
				id, _ := t["ticket"].(string)
				doing = append(doing, id)
			}
		}
	}
	if len(doing) != 2 || doing[0] != "B" || doing[1] != "A" {
		t.Fatalf("expected [B,A] (ts desc), got %v", doing)
	}
}

func TestCanonicalKanban_MapsStatesToFourByTwoColumns(t *testing.T) {
	tickets := []ledger.Row{
		{"n": float64(1), "id": "R", "state": "ready", "ts": "2026-05-14T10:00:00Z"},
		{"n": float64(2), "id": "D", "state": "doing", "ts": "2026-05-14T10:01:00Z"},
		{"n": float64(3), "id": "V", "state": "review", "ts": "2026-05-14T10:02:00Z"},
		{"n": float64(4), "id": "DN", "state": "done", "ts": "2026-05-14T10:03:00Z"},
		{"n": float64(5), "id": "B", "state": "backlog", "ts": "2026-05-14T10:04:00Z"},
		{"n": float64(6), "id": "BL", "state": "blocked", "ts": "2026-05-14T10:05:00Z"},
		{"n": float64(7), "id": "RW", "state": "rework", "ts": "2026-05-14T10:06:00Z"},
		{"n": float64(8), "id": "DR", "state": "dropped", "ts": "2026-05-14T10:07:00Z"},
	}
	k := buildCanonicalKanban(tickets)
	if len(k.Columns) != 8 {
		t.Fatalf("want 8 columns, got %d", len(k.Columns))
	}
	want := []string{"ready", "doing", "review", "done", "backlog", "blocked", "rework", "dropped"}
	for i, id := range want {
		if k.Columns[i].ID != id {
			t.Fatalf("column %d id=%s want %s", i, k.Columns[i].ID, id)
		}
		if len(k.Columns[i].Tickets) != 1 {
			t.Fatalf("column %s should have one ticket, got %+v", id, k.Columns[i].Tickets)
		}
	}
	if len(k.Grid) != 2 || len(k.Grid[0]) != 4 || k.Grid[0][0] != "ready" || k.Grid[1][3] != "dropped" {
		t.Fatalf("unexpected grid: %+v", k.Grid)
	}
}

func TestCanonicalKanban_UsesLatestRowByID(t *testing.T) {
	tickets := []ledger.Row{
		{"n": float64(1), "id": "A", "state": "ready", "ts": "2026-05-14T10:00:00Z"},
		{"n": float64(2), "id": "A", "state": "doing", "ts": "2026-05-14T10:01:00Z"},
	}
	k := buildCanonicalKanban(tickets)
	for _, col := range k.Columns {
		for _, row := range col.Tickets {
			if row["id"] == "A" && col.ID != "doing" {
				t.Fatalf("latest A should be in doing, got %s", col.ID)
			}
		}
	}
}

func TestCanonicalKanban_ExcludesInvalidatedRows(t *testing.T) {
	tickets := []ledger.Row{
		{"n": float64(1), "id": "GHOST", "state": "ready", "parent": "P", "ts": "2026-05-14T10:00:00Z"},
		{"n": float64(2), "id": "INV", "state": "dropped", "parent": "LEGACY", "invalidates_n": float64(1), "ts": "2026-05-14T10:01:00Z"},
	}
	k := buildCanonicalKanban(tickets)
	for _, col := range k.Columns {
		for _, row := range col.Tickets {
			if row["id"] == "GHOST" {
				tFail(col.ID, row)
			}
		}
	}
}

func tFail(col string, row ledger.Row) {
	panic(fmt.Sprintf("invalidated ticket leaked into column %s: %+v", col, row))
}

func equalSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]int{}
	for _, x := range a {
		m[x]++
	}
	for _, x := range b {
		m[x]--
	}
	for _, v := range m {
		if v != 0 {
			return false
		}
	}
	return true
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

func TestTree_PreservesParentTicketAsBucketLabel(t *testing.T) {
	rows := []ledger.Row{
		{"n": float64(1), "ticket": "ROOT-1", "status": "open", "ts": "2026-05-14T10:00:00Z", "parent_ticket": "BUG"},
		{"n": float64(2), "ticket": "CHILD-1", "status": "open", "ts": "2026-05-14T10:01:00Z", "parent_ticket": "ROOT-1"},
	}
	buckets := Tree(rows)
	if len(buckets) != 2 {
		t.Fatalf("expected 2 buckets (BUG + ROOT-1), got %d", len(buckets))
	}
	// The bucket labels are alphabetical.
	if buckets[0].Parent != "BUG" || buckets[1].Parent != "ROOT-1" {
		t.Fatalf("unexpected bucket order: %+v", buckets)
	}
}

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
func lcRow(n int, status, ts string, extra ...string) ledger.Row {
	r := ledger.Row{
		"n":      float64(n),
		"ticket": "T",
		"status": status,
		"ts":     ts,
	}
	for i := 0; i+1 < len(extra); i += 2 {
		r[extra[i]] = extra[i+1]
	}
	return r
}

func TestComputeLifecycleLatency_Empty(t *testing.T) {
	got := ComputeLifecycleLatency(nil, time.Now())
	if got != (LifecycleLatency{}) {
		t.Fatalf("want zero value, got %+v", got)
	}
}

func TestComputeLifecycleLatency_HappyPath(t *testing.T) {
	hist := []ledger.Row{
		lcRow(1, "open", "2026-05-14T10:00:00Z"),
		lcRow(2, "in_progress", "2026-05-14T10:30:00Z"),
		lcRow(3, "audit_ready", "2026-05-14T12:00:00Z"),
		lcRow(4, "done", "2026-05-14T13:00:00Z", "role", "audit", "audit_result", "pass"),
	}
	got := ComputeLifecycleLatency([][]ledger.Row{hist}, mustTime("2026-05-14T14:00:00Z"))
	if got.CompletedCycleCount != 1 {
		t.Fatalf("want 1 completed cycle, got %d", got.CompletedCycleCount)
	}
	if got.MedianCycleHours != 3 || got.P90CycleHours != 3 {
		t.Fatalf("want cycle 3h/3h, got %+v", got)
	}
	if got.PendingAuditCount != 1 {
		t.Fatalf("want 1 audit-latency sample (the completed one), got %d", got.PendingAuditCount)
	}
	if got.MedianAuditLatencyHours != 1 || got.P90AuditLatencyHours != 1 {
		t.Fatalf("want audit-latency 1h/1h, got %+v", got)
	}
}

func TestComputeLifecycleLatency_ChangesRequestedLoop(t *testing.T) {
	// audit_ready at 12:00 → changes_requested → in_progress → audit_ready at 16:00 → done at 17:00.
	// Cycle: 10:00 → 17:00 = 7h. Audit latency uses the SECOND audit_ready: 16:00 → 17:00 = 1h.
	hist := []ledger.Row{
		lcRow(1, "open", "2026-05-14T10:00:00Z"),
		lcRow(2, "in_progress", "2026-05-14T10:30:00Z"),
		lcRow(3, "audit_ready", "2026-05-14T12:00:00Z"),
		lcRow(4, "changes_requested", "2026-05-14T13:00:00Z"),
		lcRow(5, "in_progress", "2026-05-14T14:00:00Z"),
		lcRow(6, "audit_ready", "2026-05-14T16:00:00Z"),
		lcRow(7, "done", "2026-05-14T17:00:00Z", "role", "audit", "audit_result", "pass"),
	}
	got := ComputeLifecycleLatency([][]ledger.Row{hist}, mustTime("2026-05-14T18:00:00Z"))
	if got.MedianCycleHours != 7 {
		t.Fatalf("want cycle 7h, got %+v", got)
	}
	if got.MedianAuditLatencyHours != 1 {
		t.Fatalf("want audit latency 1h (from second audit_ready), got %+v", got)
	}
}

func TestComputeLifecycleLatency_CancelledExcluded(t *testing.T) {
	hist := []ledger.Row{
		lcRow(1, "open", "2026-05-14T10:00:00Z"),
		lcRow(2, "in_progress", "2026-05-14T10:30:00Z"),
		lcRow(3, "audit_ready", "2026-05-14T12:00:00Z"),
		lcRow(4, "cancelled", "2026-05-14T13:00:00Z"),
	}
	got := ComputeLifecycleLatency([][]ledger.Row{hist}, mustTime("2026-05-14T18:00:00Z"))
	if got.CompletedCycleCount != 0 || got.PendingAuditCount != 0 {
		t.Fatalf("cancelled should be excluded from both, got %+v", got)
	}
}

func TestComputeLifecycleLatency_PendingAuditNow(t *testing.T) {
	// Currently audit_ready, not yet done. Audit latency = now - audit_ready.
	hist := []ledger.Row{
		lcRow(1, "open", "2026-05-14T10:00:00Z"),
		lcRow(2, "in_progress", "2026-05-14T10:30:00Z"),
		lcRow(3, "audit_ready", "2026-05-14T12:00:00Z"),
	}
	got := ComputeLifecycleLatency([][]ledger.Row{hist}, mustTime("2026-05-14T14:00:00Z"))
	if got.CompletedCycleCount != 0 {
		t.Fatalf("no completed cycle expected, got %+v", got)
	}
	if got.PendingAuditCount != 1 || got.MedianAuditLatencyHours != 2 {
		t.Fatalf("want pending 1 at 2h, got %+v", got)
	}
}

func TestComputeLifecycleLatency_MixedBatchMedianP90(t *testing.T) {
	// 5 completed tickets, each with the same simple shape but varying cycle durations:
	// 1h, 2h, 3h, 4h, 10h. Median (lower of two for even n; here n=5, so 3h). P90 nearest-rank
	// idx = ceil(0.9 * 5) = 5 → 10h.
	now := mustTime("2026-05-15T00:00:00Z")
	var tickets [][]ledger.Row
	for i, hours := range []int{1, 2, 3, 4, 10} {
		start := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
		end := start.Add(time.Duration(hours) * time.Hour)
		hist := []ledger.Row{
			{"n": float64(4*i + 1), "ticket": fmt.Sprintf("T%d", i), "status": "open", "ts": start.Format(time.RFC3339)},
			{"n": float64(4*i + 2), "ticket": fmt.Sprintf("T%d", i), "status": "in_progress", "ts": start.Add(10 * time.Minute).Format(time.RFC3339)},
			{"n": float64(4*i + 3), "ticket": fmt.Sprintf("T%d", i), "status": "audit_ready", "ts": end.Add(-30 * time.Minute).Format(time.RFC3339)},
			{"n": float64(4*i + 4), "ticket": fmt.Sprintf("T%d", i), "status": "done", "role": "audit", "audit_result": "pass", "ts": end.Format(time.RFC3339)},
		}
		tickets = append(tickets, hist)
	}
	got := ComputeLifecycleLatency(tickets, now)
	if got.CompletedCycleCount != 5 {
		t.Fatalf("want 5 completed, got %d", got.CompletedCycleCount)
	}
	if got.MedianCycleHours != 3 {
		t.Fatalf("want median cycle 3h, got %v", got.MedianCycleHours)
	}
	if got.P90CycleHours != 10 {
		t.Fatalf("want p90 cycle 10h, got %v", got.P90CycleHours)
	}
	// Audit latency is 30m = 0.5h for all five.
	if got.PendingAuditCount != 5 {
		t.Fatalf("want 5 audit-latency samples, got %d", got.PendingAuditCount)
	}
	if got.MedianAuditLatencyHours != 0.5 || got.P90AuditLatencyHours != 0.5 {
		t.Fatalf("want audit 0.5/0.5, got %+v", got)
	}
}

func TestComputeLifecycleLatency_RepeatedStatusRows(t *testing.T) {
	// Two open rows in a row; cycle start should be the first open, not the duplicate.
	hist := []ledger.Row{
		lcRow(1, "open", "2026-05-14T10:00:00Z"),
		lcRow(2, "open", "2026-05-14T11:00:00Z"),
		lcRow(3, "in_progress", "2026-05-14T11:30:00Z"),
		lcRow(4, "audit_ready", "2026-05-14T12:00:00Z"),
		lcRow(5, "done", "2026-05-14T13:00:00Z", "role", "audit", "audit_result", "pass"),
	}
	got := ComputeLifecycleLatency([][]ledger.Row{hist}, mustTime("2026-05-14T14:00:00Z"))
	if got.MedianCycleHours != 3 {
		t.Fatalf("want cycle 3h from first open, got %+v", got)
	}
}

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestDashboard_ActiveAgentsMergesTicketAndWorklog(t *testing.T) {
	now := mustTime("2026-05-15T12:00:00Z")
	tickets := []ledger.Row{
		// within window, agent=alice, role=impl
		{"n": float64(1), "ts": "2026-05-15T08:00:00Z", "ticket": "T1", "parent_ticket": "P", "agent": "alice", "role": "impl", "status": "open", "blocked_by": []any{}},
		// within window, claimed_by=alice (precedence over agent), role=audit
		{"n": float64(2), "ts": "2026-05-15T10:00:00Z", "ticket": "T2", "parent_ticket": "P", "agent": "ignored", "claimed_by": "alice", "role": "audit", "status": "audit_ready"},
		// outside 24h window
		{"n": float64(3), "ts": "2026-05-13T08:00:00Z", "ticket": "T3", "parent_ticket": "P", "agent": "carol", "role": "impl", "status": "open"},
		// empty actor → unknown
		{"n": float64(4), "ts": "2026-05-15T09:00:00Z", "ticket": "T4", "parent_ticket": "P", "status": "open"},
	}
	worklog := []ledger.Row{
		// within window, agent=bob
		{"n": float64(1), "ts": "2026-05-15T11:00:00Z", "ticket": "T1", "agent": "bob", "task": "x", "result": "ok"},
		// within window, alice again (rows merge across sources)
		{"n": float64(2), "ts": "2026-05-15T11:30:00Z", "ticket": "T2", "agent": "alice", "task": "y", "result": "ok"},
	}
	d := BuildDashboard(tickets, worklog, now)
	aa := d.ActiveAgents
	if aa.WindowHours != 24 {
		t.Fatalf("window_hours=%d", aa.WindowHours)
	}
	if aa.UnknownCount != 1 {
		t.Fatalf("unknown_count=%d, want 1", aa.UnknownCount)
	}
	if len(aa.Agents) != 2 {
		t.Fatalf("want 2 agents, got %d: %+v", len(aa.Agents), aa.Agents)
	}
	// alice should come first (3 rows), bob second (1 row).
	if aa.Agents[0].Agent != "alice" || aa.Agents[0].Rows != 3 {
		t.Fatalf("agents[0]=%+v", aa.Agents[0])
	}
	if aa.Agents[1].Agent != "bob" || aa.Agents[1].Rows != 1 {
		t.Fatalf("agents[1]=%+v", aa.Agents[1])
	}
	// alice has role impl (1) + audit (1) → tie-broken to "audit" lexicographically first.
	if aa.Agents[0].Role != "audit" {
		t.Fatalf("alice role=%q want audit (tie-break)", aa.Agents[0].Role)
	}
	// latest timestamp populated
	if aa.Agents[0].Latest != "2026-05-15T11:30:00Z" {
		t.Fatalf("alice latest=%q", aa.Agents[0].Latest)
	}
}

func TestDashboard_ActiveAgentsRoleMajorityWins(t *testing.T) {
	now := mustTime("2026-05-15T12:00:00Z")
	tickets := []ledger.Row{
		{"n": float64(1), "ts": "2026-05-15T08:00:00Z", "ticket": "T1", "parent_ticket": "P", "agent": "alice", "role": "impl", "status": "open"},
		{"n": float64(2), "ts": "2026-05-15T08:30:00Z", "ticket": "T2", "parent_ticket": "P", "agent": "alice", "role": "impl", "status": "open"},
		{"n": float64(3), "ts": "2026-05-15T09:00:00Z", "ticket": "T3", "parent_ticket": "P", "agent": "alice", "role": "audit", "status": "open"},
	}
	d := BuildDashboard(tickets, nil, now)
	if len(d.ActiveAgents.Agents) != 1 {
		t.Fatalf("want 1 agent, got %d", len(d.ActiveAgents.Agents))
	}
	if d.ActiveAgents.Agents[0].Role != "impl" {
		t.Fatalf("majority role should be impl, got %q", d.ActiveAgents.Agents[0].Role)
	}
}

func TestDashboard_ActiveAgentsCapAtEight(t *testing.T) {
	now := mustTime("2026-05-15T12:00:00Z")
	var tickets []ledger.Row
	for i := 0; i < 12; i++ {
		tickets = append(tickets, ledger.Row{
			"n": float64(i + 1), "ts": "2026-05-15T08:00:00Z", "ticket": fmt.Sprintf("T%d", i),
			"parent_ticket": "P", "agent": fmt.Sprintf("agent-%02d", i), "role": "impl", "status": "open",
		})
	}
	d := BuildDashboard(tickets, nil, now)
	if len(d.ActiveAgents.Agents) != 8 {
		t.Fatalf("want cap of 8, got %d", len(d.ActiveAgents.Agents))
	}
}
