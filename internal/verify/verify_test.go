package verify

import (
	"testing"
)

func TestVerify_EmptyLedgerPasses(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json":   validConfigJSON(),
		"ledger/goal.json":     validGoalJSON(),
		"ledger/tickets.jsonl": "",
		"ledger/worklog.jsonl": "",
	})
	report, err := Run(dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(report.Fails) != 0 {
		t.Fatalf("expected no fails, got %v", report.Fails)
	}
}

func TestVerify_WarnsOnRootLegacyLedgerSources(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json":   validConfigJSON(),
		"ledger/goal.json":     validGoalJSON(),
		"ledger/tickets.jsonl": "",
		"ledger/worklog.jsonl": "",
		"agent-tickets.jsonl":  `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"L-1","agent":"codex","role":"lead","status":"planned","task":"legacy","scope":"repo","paths":[],"blocked_by":[],"decision":"","notes":""}` + "\n",
	})
	report, err := Run(dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !hasWarnCode(report, "LEGACY_LEDGER_PRESENT") {
		t.Fatalf("expected LEGACY_LEDGER_PRESENT warn, got %+v", report.Warns)
	}
}

func TestVerify_FailsOnRootLegacyParseErrors(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json":   validConfigJSON(),
		"ledger/goal.json":     validGoalJSON(),
		"ledger/tickets.jsonl": "",
		"ledger/worklog.jsonl": "",
		"agent-tickets.jsonl":  "{not json}\n",
	})
	report, err := Run(dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !hasFailCode(report, "LEGACY_PARSE_ERROR") {
		t.Fatalf("expected LEGACY_PARSE_ERROR fail, got %+v", report.Fails)
	}
}

func TestVerify_NGapFails(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"a","parent_ticket":"ROOT","agent":"codex","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}
{"n":3,"ts":"2026-05-14T10:01:00Z","ticket":"b","parent_ticket":"ROOT","agent":"codex","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if len(report.Fails) == 0 {
		t.Fatalf("expected n gap fail")
	}
}

func TestVerify_NonDecreasingTsWarns(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:01:00Z","ticket":"a","parent_ticket":"ROOT","agent":"codex","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}
{"n":2,"ts":"2026-05-14T10:00:00Z","ticket":"b","parent_ticket":"ROOT","agent":"codex","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if !hasWarnCode(report, "TS_NOT_INCREASING") {
		t.Fatalf("expected ts non-decreasing warn, got %+v", report.Warns)
	}
}

func TestVerify_NonDecreasingTsParsesFractionalTimestamps(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00.500Z","ticket":"a","parent_ticket":"ROOT","agent":"codex","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}
{"n":2,"ts":"2026-05-14T10:00:00Z","ticket":"b","parent_ticket":"ROOT","agent":"codex","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if !hasWarnCode(report, "TS_NOT_INCREASING") {
		t.Fatalf("expected parsed fractional ts ordering warn, got %+v", report.Warns)
	}
}

func TestVerify_BadStatusFails(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"a","parent_ticket":"ROOT","agent":"codex","role":"impl","status":"weird","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if len(report.Fails) == 0 {
		t.Fatalf("expected status enum fail")
	}
}
