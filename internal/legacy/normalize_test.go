package legacy

import (
	"testing"

	"github.com/hgwk/ldgr/internal/ledger"
)

func TestNormalizeTickets_InfersParentFromPrefix(t *testing.T) {
	parents := []string{"ROOT", "BUG", "FE", "LEGACY"}
	in := []ledger.Row{
		{"n": float64(1), "ticket": "BUG-101", "task": "fix", "ts": "2026-05-14T10:00:00Z", "agent": "codex", "role": "impl", "status": "open", "scope": "repo", "paths": []any{}, "blocked_by": []any{}},
	}
	rows, n, _ := NormalizeTickets(in, parents, fixedNow())
	if n.ParentInferred != 1 {
		t.Fatalf("expected 1 parent inference, got %d", n.ParentInferred)
	}
	if rows[0]["parent_ticket"] != "BUG" {
		t.Fatalf("expected BUG, got %v", rows[0]["parent_ticket"])
	}
}

func TestNormalizeTickets_UnknownPrefixGetsLegacy(t *testing.T) {
	parents := []string{"ROOT", "BUG"}
	in := []ledger.Row{
		{"n": float64(1), "ticket": "XYZ-1", "task": "t", "ts": "2026-05-14T10:00:00Z", "agent": "codex", "role": "impl", "status": "open", "scope": "repo", "paths": []any{}, "blocked_by": []any{}},
	}
	rows, _, _ := NormalizeTickets(in, parents, fixedNow())
	if rows[0]["parent_ticket"] != "LEGACY" {
		t.Fatalf("expected LEGACY, got %v", rows[0]["parent_ticket"])
	}
}

func TestNormalizeTickets_NeverInfersRoot(t *testing.T) {
	parents := []string{"ROOT", "BUG"}
	in := []ledger.Row{
		{"n": float64(1), "ticket": "foo-1", "task": "t", "ts": "2026-05-14T10:00:00Z", "agent": "codex", "role": "impl", "status": "open", "scope": "repo", "paths": []any{}, "blocked_by": []any{}},
	}
	rows, _, _ := NormalizeTickets(in, parents, fixedNow())
	if rows[0]["parent_ticket"] == "ROOT" {
		t.Fatalf("ROOT must not be inferred")
	}
}

func TestNormalizeTickets_ConsecutiveN(t *testing.T) {
	in := []ledger.Row{
		{"ticket": "a", "task": "t", "ts": "2026-05-14T10:00:00Z", "agent": "codex", "role": "impl", "status": "open", "scope": "repo", "paths": []any{}, "blocked_by": []any{}, "parent_ticket": "ROOT"},
		{"n": float64(5), "ticket": "b", "task": "t", "ts": "2026-05-14T10:01:00Z", "agent": "codex", "role": "impl", "status": "open", "scope": "repo", "paths": []any{}, "blocked_by": []any{}, "parent_ticket": "ROOT"},
	}
	rows, n, _ := NormalizeTickets(in, []string{"ROOT"}, fixedNow())
	if rows[0]["n"].(int) != 1 || rows[1]["n"].(int) != 2 {
		t.Fatalf("expected n=1,2, got %v %v", rows[0]["n"], rows[1]["n"])
	}
	if n.NReassigned == 0 {
		t.Fatalf("expected reassignment count > 0")
	}
}

func TestNormalizeTickets_MissingTSGetsNowAndWarn(t *testing.T) {
	in := []ledger.Row{
		{"ticket": "a", "task": "t", "agent": "codex", "role": "impl", "status": "open", "scope": "repo", "paths": []any{}, "blocked_by": []any{}, "parent_ticket": "ROOT"},
	}
	rows, n, warns := NormalizeTickets(in, []string{"ROOT"}, fixedNow())
	if rows[0]["ts"] == "" {
		t.Fatalf("ts should be filled, got empty")
	}
	if n.TSReplaced == 0 || len(warns) == 0 {
		t.Fatalf("expected ts replacement count and warning, got %+v warns=%v", n, warns)
	}
}

func TestNormalizeTickets_DefaultsAgentToLegacy(t *testing.T) {
	in := []ledger.Row{
		{"ticket": "a", "task": "t", "ts": "2026-05-14T10:00:00Z", "role": "impl", "status": "open", "scope": "repo", "paths": []any{}, "blocked_by": []any{}, "parent_ticket": "ROOT"},
	}
	rows, n, _ := NormalizeTickets(in, []string{"ROOT"}, fixedNow())
	if rows[0]["agent"] != "legacy" {
		t.Fatalf("expected agent=legacy, got %v", rows[0]["agent"])
	}
	if n.AgentDefaulted == 0 {
		t.Fatalf("expected agent default count")
	}
}

func TestNormalizeTickets_DetectsGhostRow(t *testing.T) {
	in := []ledger.Row{
		{"ticket": "", "task": "", "ts": "2026-05-14T10:00:00Z", "agent": "codex", "role": "impl", "status": "done", "scope": "repo", "paths": []any{}, "blocked_by": []any{}, "parent_ticket": "ROOT"},
	}
	rows, n, _ := NormalizeTickets(in, []string{"ROOT"}, fixedNow())
	if n.GhostTickets != 1 {
		t.Fatalf("expected 1 ghost ticket, got %d", n.GhostTickets)
	}
	if rows[0]["ticket"] != "" {
		t.Fatalf("ghost row content must be preserved, got %v", rows[0]["ticket"])
	}
}

func TestNormalizeWorklog_TicketOptional(t *testing.T) {
	in := []ledger.Row{
		{"task": "goal change", "scope": "ledger", "result": "ok", "ts": "2026-05-14T10:00:00Z", "agent": "codex", "paths": []any{}, "commands": []any{}, "notes": "", "branch": "", "commit": ""},
	}
	rows, n, _ := NormalizeWorklog(in, fixedNow())
	if _, present := rows[0]["ticket"]; present {
		t.Fatalf("worklog without ticket should not get a ticket field")
	}
	if n.GhostWorklog != 0 {
		t.Fatalf("optional ticket is not ghost")
	}
}

func fixedNow() string { return "2026-05-14T12:00:00Z" }
