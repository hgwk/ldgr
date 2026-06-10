package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hgwk/ldgr/internal/config"
	"github.com/hgwk/ldgr/internal/ledger"
	"github.com/hgwk/ldgr/internal/verify"
)

func TestMigrateLegacyToV1_PlanWritesNothing(t *testing.T) {
	target := seedV1Ledger(t)
	before := snapshot(t, target)
	var out bytes.Buffer
	if code := RunMigrateCLI([]string{"legacy-to-v1", "--target", target, "--plan"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("plan failed: %s", out.String())
	}
	if !strings.Contains(out.String(), "Schema v1 migration plan") {
		t.Fatalf("plan output missing banner: %s", out.String())
	}
	if !strings.Contains(out.String(), "config    1") || !strings.Contains(out.String(), "goal      unchanged") {
		t.Fatalf("plan output should include config/goal counts: %s", out.String())
	}
	if !strings.Contains(out.String(), "TYPE_DEFAULTED") || !strings.Contains(out.String(), "sample: ticket n=1 id=T-1") {
		t.Fatalf("plan output should group warnings with samples: %s", out.String())
	}
	after := snapshot(t, target)
	if before != after {
		t.Fatalf("--plan must not change filesystem")
	}
}

func TestMigrateLegacyToV1_RejectsDisabledBackup(t *testing.T) {
	target := seedV1Ledger(t)
	var stderr bytes.Buffer
	code := RunMigrateCLI([]string{"legacy-to-v1", "--target", target, "--apply", "--backup=false"}, &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected disabled backup to be rejected")
	}
	if !strings.Contains(stderr.String(), "requires --backup=true") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestMigrateLegacyToV1_ApplyRewritesAndVerifies(t *testing.T) {
	target := seedV1Ledger(t)
	var out, stderr bytes.Buffer
	if code := RunMigrateCLI([]string{"legacy-to-v1", "--target", target, "--apply"}, &out, &stderr); code != 0 {
		t.Fatalf("apply failed: stderr=%s stdout=%s", stderr.String(), out.String())
	}
	cfg, err := config.Load(filepath.Join(target, "ledger", "config.json"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.SchemaVersion != 1 {
		t.Fatalf("expected schema v1, got %d", cfg.SchemaVersion)
	}
	var raw struct {
		HistoricalBaseline struct {
			Tickets int `json:"tickets"`
			Worklog int `json:"worklog"`
		} `json:"historical_baseline"`
	}
	cfgBytes, err := os.ReadFile(filepath.Join(target, "ledger", "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if err := json.Unmarshal(cfgBytes, &raw); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if raw.HistoricalBaseline.Tickets != 2 || raw.HistoricalBaseline.Worklog != 1 {
		t.Fatalf("expected migration baseline 2/1, got %+v", raw.HistoricalBaseline)
	}
	backupEntries, err := os.ReadDir(filepath.Join(target, ".ldgr", "backups"))
	if err != nil {
		t.Fatalf("backup dir missing: %v", err)
	}
	if len(backupEntries) != 1 || !strings.HasPrefix(backupEntries[0].Name(), "legacy-to-v1-") {
		t.Fatalf("expected legacy-to-v1 backup, got %+v", backupEntries)
	}
	tickets, err := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	if err != nil {
		t.Fatalf("read tickets: %v", err)
	}
	if tickets[0]["state"] != "doing" {
		t.Fatalf("expected in_progress to map to doing, got %+v", tickets[0])
	}
	report, err := verify.Run(target)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if len(report.Fails) != 0 {
		t.Fatalf("migrated ledger should have no verify fails: %+v", report.Fails)
	}
}

func TestMigrateLegacyToV1_WeakDoneMapsToReview(t *testing.T) {
	target := seedV1LedgerWithTicket(t, `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"T-1","parent_ticket":"ROOT","agent":"codex","role":"impl","category":"feature","status":"done","task":"ship thing","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n", "")
	var out bytes.Buffer
	if code := RunMigrateCLI([]string{"legacy-to-v1", "--target", target, "--apply"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("apply failed: %s", out.String())
	}
	tickets, err := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	if err != nil {
		t.Fatalf("read tickets: %v", err)
	}
	if tickets[0]["state"] != "review" {
		t.Fatalf("weak done should map to review, got %+v", tickets[0])
	}
	report, _ := verify.Run(target)
	if len(report.Fails) != 0 {
		t.Fatalf("weak done migration should avoid v1 audit fail: %+v", report.Fails)
	}
}

func TestMigrateLegacyToV1_GhostRowsProduceValidV1(t *testing.T) {
	target := seedV1LedgerWithTicket(t,
		`{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"","parent_ticket":"ROOT","agent":"codex","role":"impl","category":"feature","status":"done","task":"","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n",
		`{"n":1,"ts":"2026-05-14T10:01:00Z","ticket":"","agent":"codex","task":"","scope":"repo","result":"","paths":[],"commands":[],"notes":"","branch":"","commit":""}`+"\n")
	var out, stderr bytes.Buffer
	if code := RunMigrateCLI([]string{"legacy-to-v1", "--target", target, "--apply"}, &out, &stderr); code != 0 {
		t.Fatalf("apply failed: stderr=%s stdout=%s", stderr.String(), out.String())
	}
	report, _ := verify.Run(target)
	if len(report.Fails) != 0 {
		t.Fatalf("ghost migration should avoid v1 fails: %+v", report.Fails)
	}
	tickets, err := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	if err != nil {
		t.Fatalf("tickets: %v", err)
	}
	if tickets[0]["id"] != "invalid-ticket-row-1" || tickets[0]["state"] != "dropped" {
		t.Fatalf("ghost ticket not synthesized as dropped: %+v", tickets[0])
	}
	worklogs, err := ledger.ReadRows(filepath.Join(target, "ledger", "worklog.jsonl"))
	if err != nil {
		t.Fatalf("worklog: %v", err)
	}
	if worklogs[0]["ticket"] != "invalid-worklog-row-1" {
		t.Fatalf("ghost worklog ticket not synthesized: %+v", worklogs[0])
	}
}

func TestMigrateLegacyToV1_WeakReworkMapsToReview(t *testing.T) {
	target := seedV1LedgerWithTicket(t,
		`{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"T-1","parent_ticket":"ROOT","agent":"claude","role":"audit","category":"test","status":"changes_requested","task":"audit","scope":"repo","paths":[],"blocked_by":[],"branch":"","audit_result":"changes_requested","audit_notes":"needs work"}`+"\n",
		"")
	var out, stderr bytes.Buffer
	if code := RunMigrateCLI([]string{"legacy-to-v1", "--target", target, "--apply"}, &out, &stderr); code != 0 {
		t.Fatalf("apply failed: stderr=%s stdout=%s", stderr.String(), out.String())
	}
	tickets, err := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	if err != nil {
		t.Fatalf("tickets: %v", err)
	}
	if tickets[0]["state"] != "review" {
		t.Fatalf("weak rework should map to review, got %+v", tickets[0])
	}
	report, _ := verify.Run(target)
	if len(report.Fails) != 0 {
		t.Fatalf("weak rework migration should avoid v1 fails: %+v", report.Fails)
	}
}

func TestMigrateLegacyToV1_SecondApplyIsNoop(t *testing.T) {
	target := seedV1Ledger(t)
	if code := RunMigrateCLI([]string{"legacy-to-v1", "--target", target, "--apply"}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("first apply failed")
	}
	before := snapshot(t, target)
	var out, stderr bytes.Buffer
	if code := RunMigrateCLI([]string{"legacy-to-v1", "--target", target, "--apply"}, &out, &stderr); code != 0 {
		t.Fatalf("second apply failed: stderr=%s stdout=%s", stderr.String(), out.String())
	}
	if !strings.Contains(out.String(), "verify warnings") {
		t.Fatalf("second apply should still verify and report, got: %s", out.String())
	}
	after := snapshot(t, target)
	if before != after {
		t.Fatalf("second apply should not rewrite schema v1 files")
	}
}

func TestMigrateLegacyToV1_ConvertsLegacyRowsAppendedAfterStateMigration(t *testing.T) {
	target := seedStateLedger(t)
	if err := appendFile(filepath.Join(target, "ledger", "tickets.jsonl"), `{"n":2,"ts":"2026-05-14T10:03:00Z","ticket":"T-2","parent_ticket":"ROOT","agent":"codex","role":"ops","status":"done","task":"legacy append","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n"); err != nil {
		t.Fatalf("append ticket: %v", err)
	}
	if err := appendFile(filepath.Join(target, "ledger", "worklog.jsonl"), `{"n":2,"ts":"2026-05-14T10:04:00Z","ticket":"T-2","agent":"codex","task":"legacy worklog","scope":"repo","result":"done","paths":[],"commands":[],"notes":"","branch":"","commit":""}`+"\n"); err != nil {
		t.Fatalf("append worklog: %v", err)
	}

	var out, stderr bytes.Buffer
	if code := RunMigrateCLI([]string{"legacy-to-v1", "--target", target, "--apply"}, &out, &stderr); code != 0 {
		t.Fatalf("apply failed: stderr=%s stdout=%s", stderr.String(), out.String())
	}
	tickets, err := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	if err != nil {
		t.Fatalf("read tickets: %v", err)
	}
	if tickets[0]["id"] != "T-1" || tickets[0]["state"] != "doing" {
		t.Fatalf("state row should be preserved, got %+v", tickets[0])
	}
	if tickets[1]["id"] != "T-2" || tickets[1]["state"] != "review" {
		t.Fatalf("legacy appended row should be converted to state review, got %+v", tickets[1])
	}
	worklogs, err := ledger.ReadRows(filepath.Join(target, "ledger", "worklog.jsonl"))
	if err != nil {
		t.Fatalf("read worklog: %v", err)
	}
	if worklogs[0]["actor"] != "claude" || worklogs[0]["title"] != "state worklog" {
		t.Fatalf("state worklog should be preserved, got %+v", worklogs[0])
	}
	if worklogs[1]["actor"] != "codex" || worklogs[1]["title"] != "legacy worklog" {
		t.Fatalf("legacy worklog should be converted, got %+v", worklogs[1])
	}
	report, _ := verify.Run(target)
	if len(report.Fails) != 0 {
		t.Fatalf("mixed migration should avoid v1 fails: %+v", report.Fails)
	}
}

func TestMigrateLegacyToV1_ApplyVerifyFailureLeavesBackup(t *testing.T) {
	target := seedV1LedgerWithTicket(t,
		`{"n":1,"ts":"not-a-time","ticket":"BAD-TS","parent_ticket":"ROOT","agent":"codex","role":"impl","category":"feature","status":"open","task":"bad ts","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n",
		"")
	var stderr bytes.Buffer
	code := RunMigrateCLI([]string{"legacy-to-v1", "--target", target, "--apply"}, &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected apply to report verify failure")
	}
	if !strings.Contains(stderr.String(), "verification failed") || !strings.Contains(stderr.String(), ".ldgr/backups") {
		t.Fatalf("stderr should explain failed verify and backup path, got: %s", stderr.String())
	}
	entries, err := os.ReadDir(filepath.Join(target, ".ldgr", "backups"))
	if err != nil {
		t.Fatalf("backup dir should exist: %v", err)
	}
	if len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), "legacy-to-v1-") {
		t.Fatalf("expected legacy-to-v1 backup, got %+v", entries)
	}
}

func seedV1Ledger(t *testing.T) string {
	t.Helper()
	return seedV1LedgerWithTicket(t,
		`{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"T-1","parent_ticket":"ROOT","agent":"codex","role":"impl","category":"feature","status":"in_progress","task":"build ui","scope":"repo","paths":["ui.tsx"],"blocked_by":[],"branch":"","priority":"P1"}`+"\n"+
			`{"n":2,"ts":"2026-05-14T10:01:00Z","ticket":"T-2","parent_ticket":"ROOT","agent":"claude","role":"audit","category":"test","status":"done","task":"audit ui","scope":"repo","paths":[],"blocked_by":[],"branch":"","evidence":["go test"],"audit_result":"pass","reviewed_n":1}`+"\n",
		`{"n":1,"ts":"2026-05-14T10:02:00Z","ticket":"T-2","agent":"codex","task":"audit ui shipped","scope":"repo","result":"verified","paths":["ui.tsx"],"commands":["go test"],"notes":"","branch":"","commit":""}`+"\n")
}

func seedV1LedgerWithTicket(t *testing.T, tickets, worklog string) string {
	t.Helper()
	target := t.TempDir()
	if err := os.MkdirAll(filepath.Join(target, "ledger"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "ledger", "config.json"), []byte(`{"schema_version":1,"project_id":"9f8a7c6b5d4e3f2a1b0c9d8e7f6a5b4c","slug":"test","name":"test","parents":["ROOT"],"branch_convention":"work/{ticket}","log_goal_changes":false}`+"\n"), 0o644); err != nil {
		t.Fatalf("config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "ledger", "goal.json"), []byte(`{"schema_version":1,"track":"main","version":"0","updated":"2026-05-14T10:00:00Z","source_of_truth":"test","summary":"test","success_criteria":[]}`+"\n"), 0o644); err != nil {
		t.Fatalf("goal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "ledger", "tickets.jsonl"), []byte(tickets), 0o644); err != nil {
		t.Fatalf("tickets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "ledger", "worklog.jsonl"), []byte(worklog), 0o644); err != nil {
		t.Fatalf("worklog: %v", err)
	}
	return target
}

func seedStateLedger(t *testing.T) string {
	t.Helper()
	target := seedV1LedgerWithTicket(t,
		`{"acceptance":[],"area":"backend","blocked_by":[],"event":{"actor":"codex","notes":"","role":"implementer","summary":"state ticket"},"evidence":[],"id":"T-1","n":1,"owner":"codex","parent":"ROOT","priority":"P1","state":"doing","title":"state ticket","ts":"2026-05-14T10:00:00Z","type":"task"}`+"\n",
		`{"actor":"claude","commands":[],"n":1,"notes":"","paths":[],"summary":"state delivery","ticket":"T-1","title":"state worklog","ts":"2026-05-14T10:02:00Z"}`+"\n")
	return target
}

func appendFile(path, data string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(data)
	return err
}
