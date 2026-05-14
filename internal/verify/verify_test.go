package verify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hgwk/ldgr/internal/config"
)

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := ensureParent(p); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := writeFile(p, content); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	return dir
}

func ensureParent(p string) error {
	return os.MkdirAll(filepath.Dir(p), 0o755)
}
func writeFile(p, c string) error { return os.WriteFile(p, []byte(c), 0o644) }
func mustJSON(v any) string       { b, _ := json.MarshalIndent(v, "", "  "); return string(b) }

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

func TestVerify_NonDecreasingTsFails(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:01:00Z","ticket":"a","parent_ticket":"ROOT","agent":"codex","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}
{"n":2,"ts":"2026-05-14T10:00:00Z","ticket":"b","parent_ticket":"ROOT","agent":"codex","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if len(report.Fails) == 0 {
		t.Fatalf("expected ts non-decreasing fail")
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

func TestVerify_AuditLifecycleStatusesPass(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"a","parent_ticket":"ROOT","agent":"codex","role":"impl","status":"audit_ready","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":"","evidence":["go test ./..."]}
{"n":2,"ts":"2026-05-14T10:01:00Z","ticket":"b","parent_ticket":"ROOT","agent":"codex","role":"audit","status":"changes_requested","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":"","audit_result":"changes_requested","audit_notes":"needs fixture"}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if len(report.Fails) != 0 {
		t.Fatalf("audit lifecycle statuses should be valid: %v", report.Fails)
	}
}

func TestVerify_GhostTicketFails(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"","parent_ticket":"ROOT","agent":"codex","role":"impl","status":"open","task":"","scope":"repo","paths":[],"blocked_by":[],"branch":""}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if len(report.Fails) == 0 {
		t.Fatalf("expected ghost ticket/task fail")
	}
}

func TestVerify_InvalidatedGhostTicketWarns(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"","parent_ticket":"ROOT","agent":"codex","role":"impl","status":"done","task":"","scope":"repo","paths":[],"blocked_by":[],"branch":""}
{"n":2,"ts":"2026-05-14T10:01:00Z","ticket":"invalid-ticket-row-1","parent_ticket":"ROOT","agent":"codex","role":"ops","status":"cancelled","task":"Invalidate ghost ticket row 1","scope":"ledger","paths":["ledger/tickets.jsonl"],"blocked_by":[],"branch":"","invalidates_n":1}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if len(report.Fails) != 0 {
		t.Fatalf("invalidated ghost ticket should warn, not fail: %v", report.Fails)
	}
	if len(report.Warns) == 0 {
		t.Fatalf("expected invalidated row warning")
	}
}

func TestVerify_InvalidatedGhostWorklogWarns(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json":   validConfigJSON(),
		"ledger/goal.json":     validGoalJSON(),
		"ledger/tickets.jsonl": "",
		"ledger/worklog.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"","agent":"codex","task":"ghost","scope":"repo","result":"","paths":[],"commands":[],"notes":"","branch":"","commit":""}
{"n":2,"ts":"2026-05-14T10:01:00Z","agent":"codex","task":"Invalidate ghost worklog row 1","scope":"ledger","result":"Invalidated empty historical worklog row 1.","paths":["ledger/worklog.jsonl"],"commands":["ldgr verify"],"notes":"","branch":"","commit":"","invalidates_n":1}
`,
	})
	report, _ := Run(dir)
	if len(report.Fails) != 0 {
		t.Fatalf("invalidated ghost worklog should warn, not fail: %v", report.Fails)
	}
	if len(report.Warns) == 0 {
		t.Fatalf("expected invalidated row warning")
	}
}

func TestVerify_MissingCategoryWarnsOnActiveTicket(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"a","parent_ticket":"ROOT","agent":"codex","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if len(report.Warns) == 0 {
		t.Fatalf("expected missing category warning")
	}
}

func TestVerify_StaleBlockerWarns(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"done-ticket","parent_ticket":"ROOT","agent":"codex","role":"impl","category":"feature","status":"done","task":"done","scope":"repo","paths":[],"blocked_by":[],"branch":""}
{"n":2,"ts":"2026-05-14T10:01:00Z","ticket":"blocked-ticket","parent_ticket":"ROOT","agent":"codex","role":"impl","category":"feature","status":"blocked","task":"blocked","scope":"repo","paths":[],"blocked_by":["done-ticket"],"branch":""}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if len(report.Warns) == 0 {
		t.Fatalf("expected stale blocker warning")
	}
}

func TestVerify_BlockersUseLatestTicketRows(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json": validConfigJSON(),
		"ledger/goal.json":   validGoalJSON(),
		"ledger/tickets.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"done-ticket","parent_ticket":"ROOT","agent":"codex","role":"impl","category":"feature","status":"done","task":"done","scope":"repo","paths":[],"blocked_by":[],"branch":""}
{"n":2,"ts":"2026-05-14T10:01:00Z","ticket":"blocked-ticket","parent_ticket":"ROOT","agent":"codex","role":"impl","category":"feature","status":"blocked","task":"blocked","scope":"repo","paths":[],"blocked_by":["done-ticket"],"branch":""}
{"n":3,"ts":"2026-05-14T10:02:00Z","ticket":"blocked-ticket","parent_ticket":"ROOT","agent":"codex","role":"impl","category":"feature","status":"open","task":"blocked","scope":"repo","paths":[],"blocked_by":[],"branch":""}
`,
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	for _, warn := range report.Warns {
		if warn.Message == "stale blocker is already closed: done-ticket" {
			t.Fatalf("stale blocker warning should ignore superseded row: %v", report.Warns)
		}
	}
}

func TestVerify_OrphanWorklogIsWarn(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json":   validConfigJSON(),
		"ledger/goal.json":     validGoalJSON(),
		"ledger/tickets.jsonl": "",
		"ledger/worklog.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"ghost","agent":"codex","task":"t","scope":"repo","result":"r","paths":[],"commands":[],"notes":"","branch":"","commit":""}
`,
	})
	report, _ := Run(dir)
	if len(report.Fails) != 0 {
		t.Fatalf("orphan worklog should not fail by default: %v", report.Fails)
	}
	if len(report.Warns) == 0 {
		t.Fatalf("expected orphan warn")
	}
}

func TestVerify_StrictPromotesWarns(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json":   validConfigJSON(),
		"ledger/goal.json":     validGoalJSON(),
		"ledger/tickets.jsonl": "",
		"ledger/worklog.jsonl": `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"ghost","agent":"codex","task":"t","scope":"repo","result":"r","paths":[],"commands":[],"notes":"","branch":"","commit":""}
`,
	})
	report, _ := RunStrict(dir, true)
	if len(report.Fails) == 0 {
		t.Fatalf("strict mode should fail on warn")
	}
}

func TestVerify_BadConfigFails(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"ledger/config.json":   `{"schema_version":1}`,
		"ledger/goal.json":     validGoalJSON(),
		"ledger/tickets.jsonl": "",
		"ledger/worklog.jsonl": "",
	})
	report, _ := Run(dir)
	if len(report.Fails) == 0 {
		t.Fatalf("expected config schema fail")
	}
}

func validConfigJSON() string {
	c := config.Default("myapp", "9f8a7c6b5d4e3f2a1b0c9d8e7f6a5b4c", "")
	return mustJSON(c)
}

func validGoalJSON() string {
	return `{"schema_version":1,"track":"project","version":"0.1.0","updated":"2026-05-14T00:00:00Z","source_of_truth":"README.md","summary":"x","success_criteria":[]}`
}

func hasWarn(r Report, code string) bool {
	for _, w := range r.Warns {
		if strings.Contains(w.Message, code) {
			return true
		}
	}
	return false
}

func hasFail(r Report, code string) bool {
	for _, f := range r.Fails {
		if strings.Contains(f.Message, code) {
			return true
		}
	}
	return false
}

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
