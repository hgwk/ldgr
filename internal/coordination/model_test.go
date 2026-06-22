package coordination

import (
	"testing"
	"time"

	"github.com/hgwk/ldgr/internal/ledger"
)

func TestBuildSummaryTracksClaimsReleasesAndConflicts(t *testing.T) {
	now := time.Date(2026, 6, 22, 1, 0, 0, 0, time.UTC)
	rows := []ledger.Row{
		{"n": float64(1), "type": "claim", "id": "c1", "ticket": "T-1", "resources": []any{"src/api"}, "claim_until": now.Add(time.Hour).Format(time.RFC3339)},
		{"n": float64(2), "type": "claim", "id": "c2", "ticket": "T-2", "resources": []any{"src/api/routes.go"}, "claim_until": now.Add(time.Hour).Format(time.RFC3339)},
		{"n": float64(3), "type": "note", "kind": "decision", "scope": "api", "summary": "keep v2"},
	}

	sum := BuildSummary(rows, now)
	if len(sum.Claims) != 2 {
		t.Fatalf("expected 2 active claims, got %+v", sum.Claims)
	}
	if len(sum.Conflicts) != 1 || sum.Conflicts[0].Resource != "src/api" {
		t.Fatalf("expected path-prefix conflict, got %+v", sum.Conflicts)
	}
	if len(sum.Notes) != 1 || sum.Notes[0].Summary != "keep v2" {
		t.Fatalf("expected recent note, got %+v", sum.Notes)
	}

	rows = append(rows, ledger.Row{"n": float64(4), "type": "release", "claim_id": "c2"})
	sum = BuildSummary(rows, now)
	if len(sum.Claims) != 1 || len(sum.Conflicts) != 0 {
		t.Fatalf("release should remove claim/conflict, got %+v", sum)
	}
}

func TestBuildSummaryMarksExpiredClaims(t *testing.T) {
	now := time.Date(2026, 6, 22, 1, 0, 0, 0, time.UTC)
	sum := BuildSummary([]ledger.Row{
		{"n": float64(1), "type": "claim", "id": "c1", "ticket": "T-1", "resources": []any{"src"}, "claim_until": now.Add(-time.Minute).Format(time.RFC3339)},
	}, now)
	if len(sum.Claims) != 1 || !sum.Claims[0].Expired {
		t.Fatalf("expected expired claim, got %+v", sum.Claims)
	}
}
