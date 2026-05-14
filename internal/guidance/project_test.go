package guidance

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hgwk/ldgr/internal/ledger"
)

func sample() []ledger.Row {
	return []ledger.Row{
		{"n": float64(1), "ticket": "OPEN-1", "status": "open", "priority": "P2", "ts": "t1", "blocked_by": []any{}, "parent_ticket": "BUG"},
		{"n": float64(2), "ticket": "IP-1", "status": "in_progress", "priority": "P0", "ts": "t2", "parent_ticket": "BUG"},
		{"n": float64(3), "ticket": "AR-1", "status": "audit_ready", "priority": "P1", "ts": "t3", "parent_ticket": "BUG", "evidence": []any{"x"}},
		{"n": float64(4), "ticket": "BL-1", "status": "blocked", "priority": "P0", "ts": "t4", "blocked_by": []any{"X"}, "parent_ticket": "BUG"},
		{"n": float64(5), "ticket": "WD-1", "status": "done", "priority": "P3", "ts": "t5", "parent_ticket": "BUG", "role": "impl"},
	}
}

func TestComputeProject_CountsActive(t *testing.T) {
	pg := ComputeProject(sample(), nil, "")
	if pg.Counts.Active != 2 || pg.Counts.AuditReady != 1 || pg.Counts.Blocked != 1 || pg.Counts.StalePremature != 1 {
		t.Fatalf("counts: %+v", pg.Counts)
	}
}

func TestComputeProject_ImplementerFiltersAuditReadyOut(t *testing.T) {
	pg := ComputeProject(sample(), nil, "implementer")
	for _, h := range pg.Highlights {
		if h.Status == "audit_ready" {
			t.Fatalf("implementer should not see audit_ready: %+v", h)
		}
	}
}

func TestComputeProject_AuditorHighlightsAuditReady(t *testing.T) {
	pg := ComputeProject(sample(), nil, "auditor")
	found := false
	for _, h := range pg.Highlights {
		if h.Status == "audit_ready" {
			found = true
		}
	}
	if !found {
		t.Fatalf("auditor should see audit_ready: %+v", pg.Highlights)
	}
}

func TestComputeProject_MaintainerHighlightsWeakDone(t *testing.T) {
	pg := ComputeProject(sample(), nil, "maintainer")
	found := false
	for _, h := range pg.Highlights {
		if h.Reason == "weak done (no audit-pass row)" {
			found = true
		}
	}
	if !found {
		t.Fatalf("maintainer should see weak done: %+v", pg.Highlights)
	}
}

func TestComputeProject_SeverityOrdering(t *testing.T) {
	pg := ComputeProject(sample(), nil, "")
	// critical first
	for i := 1; i < len(pg.Highlights); i++ {
		if severityRank(pg.Highlights[i-1].Severity) > severityRank(pg.Highlights[i].Severity) {
			t.Fatalf("severity not sorted at %d: %+v", i, pg.Highlights)
		}
	}
}

func TestRenderProjectText_Header(t *testing.T) {
	pg := ComputeProject(sample(), nil, "")
	text := RenderProjectText(pg)
	if !strings.Contains(text, "Project queue") {
		t.Fatalf("text should have a project queue header: %s", text)
	}
}

func TestComputeProject_CapAt8(t *testing.T) {
	// Build 10 open tickets.
	rows := make([]ledger.Row, 10)
	for i := range rows {
		rows[i] = ledger.Row{
			"n": float64(i + 1), "ticket": fmt.Sprintf("T-%d", i+1),
			"status": "open", "priority": "P2", "blocked_by": []any{},
		}
	}
	pg := ComputeProject(rows, nil, "")
	if len(pg.Highlights) > 8 {
		t.Fatalf("highlights should be capped at 8, got %d", len(pg.Highlights))
	}
}

func TestComputeProject_EmptyInput(t *testing.T) {
	pg := ComputeProject(nil, nil, "")
	if len(pg.Highlights) != 0 {
		t.Fatalf("expected empty highlights for empty input")
	}
	if pg.Counts.Active != 0 {
		t.Fatalf("expected zero counts for empty input")
	}
}

func TestComputeProject_RoleEchoed(t *testing.T) {
	pg := ComputeProject(sample(), nil, "auditor")
	if pg.Role != "auditor" {
		t.Fatalf("role not echoed: %s", pg.Role)
	}
}
