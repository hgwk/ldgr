package viewer

import (
	"fmt"
	"testing"

	"github.com/hgwk/ldgr/internal/ledger"
)

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
