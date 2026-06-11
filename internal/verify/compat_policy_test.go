package verify

import "testing"

func TestVerify_WarnsOnWeakDone(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"WD-1","parent_ticket":"BUG","agent":"codex","role":"impl","category":"bug","status":"done","task":"weak","scope":"repo","paths":[],"blocked_by":[],"branch":""}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if !hasWarn(report, "WEAK_DONE") {
		t.Fatalf("expected WEAK_DONE warn, got %+v", report)
	}
	if len(report.Fails) != 0 {
		t.Fatalf("default verify must not fail on weak done; got %+v", report.Fails)
	}
	strict, _ := RunStrict(dir, true)
	if !hasFail(strict, "WEAK_DONE") {
		t.Fatalf("strict verify must fail on weak done")
	}
}

func TestVerify_LegacyDoneWarnsOnMissingGitEvidence(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"GD-1","parent_ticket":"BUG","agent":"codex","role":"audit","category":"bug","status":"done","audit_result":"pass","evidence":["go test"],"reviewed_n":1,"task":"done","scope":"repo","paths":[],"blocked_by":[],"branch":""}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if !hasWarn(report, "DONE_MISSING_GIT_EVIDENCE") {
		t.Fatalf("expected DONE_MISSING_GIT_EVIDENCE warn, got %+v", report)
	}
}

func TestVerify_LegacyDoneAcceptsPREvidence(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"GD-2","parent_ticket":"BUG","agent":"codex","role":"audit","category":"bug","status":"done","audit_result":"pass","evidence":["go test","pr:#42"],"reviewed_n":1,"task":"done","scope":"repo","paths":[],"blocked_by":[],"branch":""}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if hasWarn(report, "DONE_MISSING_GIT_EVIDENCE") {
		t.Fatalf("did not expect DONE_MISSING_GIT_EVIDENCE warn, got %+v", report)
	}
}

func TestVerify_WarnsOnInvalidTransition(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"IT-1","parent_ticket":"BUG","agent":"codex","role":"impl","category":"bug","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[],"branch":""}
{"n":2,"ts":"2026-05-14T10:01:00Z","ticket":"IT-1","parent_ticket":"BUG","agent":"codex","role":"impl","category":"bug","status":"audit_ready","task":"x","scope":"repo","paths":[],"blocked_by":[],"branch":"","evidence":["x"]}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if !hasWarn(report, "INVALID_TRANSITION") {
		t.Fatalf("expected INVALID_TRANSITION warn: %+v", report)
	}
}

func TestVerify_WarnsOnAuditMissingReviewedN(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"AR-1","parent_ticket":"BUG","agent":"codex","role":"audit","category":"bug","status":"done","audit_result":"pass","evidence":["x"],"task":"audit","scope":"repo","paths":[],"blocked_by":[],"branch":""}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if !hasWarn(report, "AUDIT_MISSING_REVIEWED_N") {
		t.Fatalf("expected AUDIT_MISSING_REVIEWED_N warn: %+v", report)
	}
}

func TestVerify_WarnsOnAuditReviewedNMismatch(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"AR-2","parent_ticket":"BUG","agent":"codex","role":"impl","category":"bug","status":"open","task":"audit","scope":"repo","paths":[],"blocked_by":[],"branch":""}
{"n":2,"ts":"2026-05-14T10:01:00Z","ticket":"AR-2","parent_ticket":"BUG","agent":"codex","role":"impl","category":"bug","status":"in_progress","task":"audit","scope":"repo","paths":[],"blocked_by":[],"branch":""}
{"n":3,"ts":"2026-05-14T10:02:00Z","ticket":"AR-2","parent_ticket":"BUG","agent":"codex","role":"impl","category":"bug","status":"audit_ready","task":"audit","scope":"repo","paths":[],"blocked_by":[],"branch":"","evidence":["go test"]}
{"n":4,"ts":"2026-05-14T10:03:00Z","ticket":"AR-2","parent_ticket":"BUG","agent":"codex","role":"audit","category":"bug","status":"done","audit_result":"pass","evidence":["go test"],"reviewed_n":2,"task":"audit","scope":"repo","paths":[],"blocked_by":[],"branch":""}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if !hasWarn(report, "AUDIT_REVIEWED_N_MISMATCH") {
		t.Fatalf("expected AUDIT_REVIEWED_N_MISMATCH warn: %+v", report)
	}
}

func TestVerify_WarnsOnPrematureWorklog(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"PW-1","parent_ticket":"BUG","agent":"codex","role":"impl","category":"bug","status":"audit_ready","task":"x","scope":"repo","paths":[],"blocked_by":[],"branch":"","evidence":["x"]}
`,
		"ledger/worklog.jsonl": `{"n":1,"ts":"2026-05-14T10:01:00Z","ticket":"PW-1","agent":"codex","task":"early","scope":"repo","result":"too soon","paths":[],"commands":[],"notes":"","branch":"","commit":""}
`,
	})
	report, _ := Run(dir)
	if !hasWarn(report, "PREMATURE_WORKLOG") {
		t.Fatalf("expected PREMATURE_WORKLOG warn: %+v", report)
	}
}

func TestVerify_PrematureWorklogUsesWorklogTimestamp(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"PW-2","parent_ticket":"BUG","agent":"codex","role":"impl","category":"bug","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[],"branch":""}
{"n":2,"ts":"2026-05-14T10:01:00Z","ticket":"PW-2","parent_ticket":"BUG","agent":"codex","role":"impl","category":"bug","status":"in_progress","task":"x","scope":"repo","paths":[],"blocked_by":[],"branch":""}
{"n":3,"ts":"2026-05-14T10:03:00Z","ticket":"PW-2","parent_ticket":"BUG","agent":"codex","role":"impl","category":"bug","status":"audit_ready","task":"x","scope":"repo","paths":[],"blocked_by":[],"branch":"","evidence":["go test"]}
{"n":4,"ts":"2026-05-14T10:04:00Z","ticket":"PW-2","parent_ticket":"BUG","agent":"codex","role":"audit","category":"bug","status":"done","audit_result":"pass","evidence":["go test"],"reviewed_n":3,"task":"x","scope":"repo","paths":[],"blocked_by":[],"branch":""}
`,
		"ledger/worklog.jsonl": `{"n":1,"ts":"2026-05-14T10:02:00Z","ticket":"PW-2","agent":"codex","task":"early","scope":"repo","result":"too soon","paths":[],"commands":[],"notes":"","branch":"","commit":""}
`,
	})
	report, _ := Run(dir)
	if !hasWarn(report, "PREMATURE_WORKLOG") {
		t.Fatalf("expected PREMATURE_WORKLOG based on worklog timestamp: %+v", report)
	}
}

func TestVerify_PrematureWorklogUsesParsedFractionalTimestamps(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00.500Z","ticket":"PW-3","parent_ticket":"BUG","agent":"codex","role":"audit","category":"bug","status":"done","audit_result":"pass","evidence":["go test"],"reviewed_n":1,"task":"x","scope":"repo","paths":[],"blocked_by":[],"branch":""}
`,
		"ledger/worklog.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"PW-3","agent":"codex","task":"early","scope":"repo","result":"too soon","paths":[],"commands":[],"notes":"","branch":"","commit":""}
`,
	})
	report, _ := Run(dir)
	if !hasWarn(report, "PREMATURE_WORKLOG") {
		t.Fatalf("expected PREMATURE_WORKLOG with fractional ticket ts: %+v", report)
	}
}

func TestVerify_IssueCodePopulated(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"WD-1","parent_ticket":"BUG","agent":"codex","role":"impl","category":"bug","status":"done","task":"weak","scope":"repo","paths":[],"blocked_by":[],"branch":""}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	var codes []string
	for _, w := range report.Warns {
		codes = append(codes, w.Code)
	}
	if !contains(codes, "WEAK_DONE") {
		t.Fatalf("expected WEAK_DONE code among warns, got %v", codes)
	}
}

func TestVerify_WarnsOnUnknownKind(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"K-1","parent_ticket":"BUG","agent":"codex","role":"impl","category":"bug","status":"in_progress","task":"x","scope":"repo","paths":[],"blocked_by":[],"branch":"","kind":"weird"}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if !hasWarn(report, "UNKNOWN_KIND") {
		t.Fatalf("expected UNKNOWN_KIND warn")
	}
}

func TestVerify_WarnsOnUnknownPriority(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"P-1","parent_ticket":"BUG","agent":"codex","role":"impl","category":"bug","status":"in_progress","task":"x","scope":"repo","paths":[],"blocked_by":[],"branch":"","priority":"P9"}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if !hasWarn(report, "UNKNOWN_PRIORITY") {
		t.Fatalf("expected UNKNOWN_PRIORITY warn")
	}
}

func TestVerify_MissingKindOrPriorityIsSilent(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"S-1","parent_ticket":"BUG","agent":"codex","role":"impl","category":"bug","status":"in_progress","task":"x","scope":"repo","paths":[],"blocked_by":[],"branch":""}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	for _, w := range report.Warns {
		if w.Code == "UNKNOWN_KIND" || w.Code == "UNKNOWN_PRIORITY" {
			t.Fatalf("missing kind/priority should not warn; got %v", w)
		}
	}
}
