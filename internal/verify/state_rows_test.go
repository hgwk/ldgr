package verify

import "testing"

func TestVerify_SchemaStateValidLedgerPasses(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSONState(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","id":"T-1","parent":"ROOT","type":"task","state":"ready","area":"frontend","priority":"P1","title":"build ui","owner":"codex","blocked_by":[],"acceptance":["test"],"evidence":[],"event":{"actor":"codex","role":"planner","summary":"opened","notes":""}}
{"n":2,"ts":"2026-05-14T10:01:00Z","id":"T-1","parent":"ROOT","type":"task","state":"doing","area":"frontend","priority":"P1","title":"build ui","owner":"codex","blocked_by":[],"acceptance":["test"],"evidence":[],"event":{"actor":"codex","role":"implementer","summary":"started","notes":""}}
{"n":3,"ts":"2026-05-14T10:02:00Z","id":"T-1","parent":"ROOT","type":"task","state":"review","area":"frontend","priority":"P1","title":"build ui","owner":"codex","blocked_by":[],"acceptance":["test"],"evidence":["go test"],"event":{"actor":"codex","role":"implementer","summary":"ready for review","notes":""}}
{"n":4,"ts":"2026-05-14T10:03:00Z","id":"T-1","parent":"ROOT","type":"task","state":"done","area":"frontend","priority":"P1","title":"build ui","owner":"codex","blocked_by":[],"acceptance":["test"],"evidence":["go test"],"event":{"actor":"claude","role":"auditor","result":"pass","reviewed_n":3,"summary":"passed","notes":""}}
`,
		"ledger/worklog.jsonl": `{"n":1,"ts":"2026-05-14T10:04:00Z","ticket":"T-1","actor":"codex","title":"build ui shipped","summary":"implemented","paths":["ui.tsx"],"commands":["go test"],"notes":""}
`,
	})
	report, err := Run(dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(report.Fails) != 0 || len(report.Warns) != 0 {
		t.Fatalf("expected clean state-model verify, got fails=%+v warns=%+v", report.Fails, report.Warns)
	}
}

func TestVerify_SchemaStateWarnsOnWeakDone(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSONState(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","id":"T-1","parent":"ROOT","type":"task","state":"done","area":"frontend","priority":"P1","title":"build ui","owner":"codex","blocked_by":[],"acceptance":[],"evidence":[],"event":{"actor":"codex","role":"implementer","summary":"done","notes":""}}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if !hasWarnCode(report, "WEAK_DONE") {
		t.Fatalf("expected WEAK_DONE, got %+v", report.Warns)
	}
}

func TestVerify_SchemaStateWarnsOnBadTransition(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSONState(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","id":"T-1","parent":"ROOT","type":"task","state":"ready","area":"frontend","priority":"P1","title":"build ui","owner":"codex","blocked_by":[],"acceptance":[],"evidence":[],"event":{"actor":"codex","role":"planner","summary":"ready","notes":""}}
{"n":2,"ts":"2026-05-14T10:01:00Z","id":"T-1","parent":"ROOT","type":"task","state":"done","area":"frontend","priority":"P1","title":"build ui","owner":"codex","blocked_by":[],"acceptance":[],"evidence":["go test"],"event":{"actor":"claude","role":"auditor","result":"pass","reviewed_n":1,"summary":"passed","notes":""}}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if !hasWarnCode(report, "INVALID_TRANSITION") {
		t.Fatalf("expected INVALID_TRANSITION warn, got %+v", report.Warns)
	}
}

func TestVerify_SchemaStateWarnsOnInvalidInitialState(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSONState(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","id":"T-1","parent":"ROOT","type":"task","state":"review","area":"frontend","priority":"P1","title":"build ui","owner":"codex","blocked_by":[],"acceptance":[],"evidence":[],"event":{"actor":"codex","role":"implementer","summary":"review","notes":""}}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if !hasWarnCode(report, "INVALID_TRANSITION") {
		t.Fatalf("expected INVALID_TRANSITION warn for invalid initial state state, got %+v", report.Warns)
	}
}

func TestVerify_SchemaStateWarnsOnPrematureWorklog(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSONState(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","id":"T-1","parent":"ROOT","type":"task","state":"review","area":"frontend","priority":"P1","title":"build ui","owner":"codex","blocked_by":[],"acceptance":[],"evidence":["go test"],"event":{"actor":"codex","role":"implementer","summary":"review","notes":""}}
`,
		"ledger/worklog.jsonl": `{"n":1,"ts":"2026-05-14T10:01:00Z","ticket":"T-1","actor":"codex","title":"build ui shipped","summary":"implemented","paths":["ui.tsx"],"commands":["go test"],"notes":""}
`,
	})
	report, _ := Run(dir)
	if !hasWarnCode(report, "PREMATURE_WORKLOG") {
		t.Fatalf("expected PREMATURE_WORKLOG warn, got %+v", report.Warns)
	}
}

func TestVerify_StateWarnsOnWorklogEmptyCommands(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSONState(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","id":"T-1","parent":"ROOT","type":"task","state":"done","area":"frontend","priority":"P1","title":"build ui","owner":"codex","blocked_by":[],"acceptance":[],"evidence":["go test"],"event":{"actor":"claude","role":"auditor","result":"pass","reviewed_n":1,"summary":"passed","notes":""}}
`,
		"ledger/worklog.jsonl": `{"n":1,"ts":"2026-05-14T10:01:00Z","ticket":"T-1","actor":"codex","title":"build ui shipped","summary":"implemented","paths":["ui.tsx"],"commands":[],"notes":""}
`,
	})
	report, _ := Run(dir)
	if !hasWarnCode(report, "WORKLOG_COMMANDS_EMPTY") {
		t.Fatalf("expected WORKLOG_COMMANDS_EMPTY warn, got %+v", report.Warns)
	}
}

func TestVerify_StateWarnsOnReviewWeakEvidence(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSONState(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","id":"T-1","parent":"ROOT","type":"task","state":"review","area":"frontend","priority":"P1","title":"build ui","owner":"codex","blocked_by":[],"acceptance":[],"evidence":["ok"],"event":{"actor":"codex","role":"implementer","summary":"ready","notes":""}}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if !hasWarnCode(report, "REVIEW_EVIDENCE_WEAK") {
		t.Fatalf("expected REVIEW_EVIDENCE_WEAK warn, got %+v", report.Warns)
	}
}

func TestVerify_StateWarnsOnMissingSuccessCriteriaAtReview(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSONState(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","id":"T-1","parent":"ROOT","type":"task","state":"review","area":"frontend","priority":"P1","title":"build ui","owner":"codex","blocked_by":[],"acceptance":[],"evidence":["go test"],"event":{"actor":"codex","role":"implementer","summary":"ready","notes":""}}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if !hasWarnCode(report, "SUCCESS_CRITERIA_MISSING") {
		t.Fatalf("expected SUCCESS_CRITERIA_MISSING warn, got %+v", report.Warns)
	}
}

func TestVerify_StateWarnsOnWeakDecisionContext(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSONState(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","id":"T-1","parent":"ROOT","type":"task","state":"blocked","area":"frontend","priority":"P1","title":"build ui","owner":"codex","blocked_by":["T-2"],"acceptance":["go test"],"evidence":[],"event":{"actor":"codex","role":"implementer","summary":"stopped","notes":"waiting"}}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if !hasWarnCode(report, "DECISION_CONTEXT_WEAK") {
		t.Fatalf("expected DECISION_CONTEXT_WEAK warn, got %+v", report.Warns)
	}
}

func TestVerify_StateWarnsOnBlockedNoBlockers(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSONState(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","id":"T-1","parent":"ROOT","type":"task","state":"blocked","area":"frontend","priority":"P1","title":"build ui","owner":"codex","blocked_by":[],"acceptance":[],"evidence":[],"event":{"actor":"codex","role":"implementer","summary":"blocked","notes":""}}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if !hasWarnCode(report, "BLOCKED_NO_BLOCKERS") {
		t.Fatalf("expected BLOCKED_NO_BLOCKERS warn, got %+v", report.Warns)
	}
}

func TestVerify_WarnsOnClaimPathConflict(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSONState(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","id":"T-1","parent":"ROOT","type":"task","state":"doing","area":"frontend","priority":"P1","title":"build ui","owner":"codex","paths":["apps/web/page.tsx"],"blocked_by":[],"acceptance":[],"evidence":[],"event":{"actor":"codex","role":"implementer","summary":"started","notes":""}}
{"n":2,"ts":"2026-05-14T10:01:00Z","id":"T-2","parent":"ROOT","type":"task","state":"doing","area":"frontend","priority":"P1","title":"build ui too","owner":"claude","paths":["./apps/web/page.tsx"],"blocked_by":[],"acceptance":[],"evidence":[],"event":{"actor":"claude","role":"implementer","summary":"started","notes":""}}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if !hasWarnCode(report, "CLAIM_PATH_CONFLICT") {
		t.Fatalf("expected CLAIM_PATH_CONFLICT warn, got %+v", report.Warns)
	}
}

func TestVerify_WarnsOnIncompleteHandoff(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSONState(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","id":"T-1","parent":"ROOT","type":"task","state":"doing","area":"frontend","priority":"P1","title":"build ui","owner":"codex","blocked_by":[],"acceptance":[],"evidence":[],"event":{"actor":"codex","role":"implementer","summary":"handoff to reviewer","notes":"handoff: continue this ticket"}}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if !hasWarnCode(report, "HANDOFF_INCOMPLETE") {
		t.Fatalf("expected HANDOFF_INCOMPLETE warn, got %+v", report.Warns)
	}
}
