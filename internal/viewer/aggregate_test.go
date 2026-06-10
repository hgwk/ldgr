package viewer

import (
	"testing"

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
