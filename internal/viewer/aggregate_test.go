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
	if !equalSet(got["plan"], []string{"P-1"}) {
		t.Fatalf("plan column wrong: %v", got["plan"])
	}
	if !equalSet(got["implement"], []string{"I-1", "I-2", "I-3"}) {
		t.Fatalf("implement column wrong: %v", got["implement"])
	}
	if !equalSet(got["verify"], []string{"V-1"}) {
		t.Fatalf("verify column wrong: %v", got["verify"])
	}
	if !equalSet(got["complete"], []string{"C-1", "C-2"}) {
		t.Fatalf("complete column wrong: %v", got["complete"])
	}
}

func TestKanban_RolePlanGoesToPlanColumn(t *testing.T) {
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
	if !equalSet(got["plan"], []string{"PLAN-1"}) {
		t.Fatalf("expected PLAN-1 in plan (active role=plan), got %v", got["plan"])
	}
	if !equalSet(got["implement"], []string{"IMPL-1"}) {
		t.Fatalf("expected IMPL-1 in implement, got %v", got["implement"])
	}
}

func TestKanban_RoleAuditActiveGoesToVerify(t *testing.T) {
	tickets := []ledger.Row{
		{"n": float64(1), "ticket": "AUD-1", "status": "in_progress", "role": "audit", "parent_ticket": "P", "ts": "2026-05-14T10:00:00Z"},
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
	if !equalSet(got["verify"], []string{"AUD-1"}) {
		t.Fatalf("expected AUD-1 in verify, got %v", got["verify"])
	}
	if !equalSet(got["complete"], []string{"AUD-2"}) {
		t.Fatalf("AUD-2 audit-done should be in complete, got %v", got["complete"])
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
	var impl []string
	for _, col := range k.Columns {
		if col.ID == "implement" {
			for _, t := range col.Tickets {
				id, _ := t["ticket"].(string)
				impl = append(impl, id)
			}
		}
	}
	if len(impl) != 2 || impl[0] != "B" || impl[1] != "A" {
		t.Fatalf("expected [B,A] (ts desc), got %v", impl)
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
