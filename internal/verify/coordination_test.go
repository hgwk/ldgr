package verify

import "testing"

func TestVerifyWarnsOnCoordinationClaimConflictAndStale(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json":   validConfigJSON(),
		"ledger/goal.json":     validGoalJSON(),
		"ledger/tickets.jsonl": "",
		"ledger/worklog.jsonl": "",
		"ledger/coordination.jsonl": `{"n":1,"type":"claim","id":"c1","ticket":"T-1","resources":["src/api"],"claim_until":"2000-01-01T00:00:00Z"}
{"n":2,"type":"claim","id":"c2","ticket":"T-2","resources":["src/api/routes.go"],"claim_until":"2999-01-01T00:00:00Z"}
`,
	})

	report, err := Run(dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !hasWarnCode(report, "COORDINATION_CLAIM_CONFLICT") {
		t.Fatalf("expected coordination conflict warn, got %+v", report.Warns)
	}
	if !hasWarnCode(report, "COORDINATION_CLAIM_STALE") {
		t.Fatalf("expected stale claim warn, got %+v", report.Warns)
	}
}
