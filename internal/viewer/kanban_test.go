package viewer

import (
	"fmt"
	"testing"

	"github.com/hgwk/ldgr/internal/ledger"
)

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
		t.Fatalf("expected active status rows in doing, got %v", got["doing"])
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

func TestStateKanban_MapsStatesToFourByTwoColumns(t *testing.T) {
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
	k := BuildKanban(tickets)
	if len(k.Columns) != 8 {
		t.Fatalf("want 8 columns, got %d", len(k.Columns))
	}
	want := []string{"ready", "doing", "review", "rework", "backlog", "blocked", "done", "dropped"}
	for i, id := range want {
		if k.Columns[i].ID != id {
			t.Fatalf("column %d id=%s want %s", i, k.Columns[i].ID, id)
		}
		if len(k.Columns[i].Tickets) != 1 {
			t.Fatalf("column %s should have one ticket, got %+v", id, k.Columns[i].Tickets)
		}
	}
	if len(k.Grid) != 2 || len(k.Grid[0]) != 4 || k.Grid[0][3] != "rework" || k.Grid[1][2] != "done" {
		t.Fatalf("unexpected grid: %+v", k.Grid)
	}
}

func TestStateKanban_UsesLatestRowByID(t *testing.T) {
	tickets := []ledger.Row{
		{"n": float64(1), "id": "A", "state": "ready", "ts": "2026-05-14T10:00:00Z"},
		{"n": float64(2), "id": "A", "state": "doing", "ts": "2026-05-14T10:01:00Z"},
	}
	k := BuildKanban(tickets)
	for _, col := range k.Columns {
		for _, row := range col.Tickets {
			if row["id"] == "A" && col.ID != "doing" {
				t.Fatalf("latest A should be in doing, got %s", col.ID)
			}
		}
	}
}

func TestStateKanban_ExcludesInvalidatedRows(t *testing.T) {
	tickets := []ledger.Row{
		{"n": float64(1), "id": "GHOST", "state": "ready", "parent": "P", "ts": "2026-05-14T10:00:00Z"},
		{"n": float64(2), "id": "INV", "state": "dropped", "parent": "LEGACY", "invalidates_n": float64(1), "ts": "2026-05-14T10:01:00Z"},
	}
	k := BuildKanban(tickets)
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
