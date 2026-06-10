package viewer

import (
	"fmt"
	"testing"
	"time"

	"github.com/hgwk/ldgr/internal/ledger"
)

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
