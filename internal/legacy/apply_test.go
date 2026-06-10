package legacy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hgwk/ldgr/internal/config"
)

func runPlan(t *testing.T, dir string) Plan {
	t.Helper()
	srcs, err := Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	return Compose(dir, srcs, config.Default("x", "id", ""), fixedNow())
}

func TestApply_WritesTickets(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "agent-tickets.jsonl", `{"n":1,"ticket":"BUG-1","task":"t","status":"open","ts":"2026-05-14T10:00:00Z","parent_ticket":"BUG","agent":"codex","role":"impl","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n")
	plan := runPlan(t, dir)
	if err := Apply(plan, ApplyOpts{}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err != nil || len(data) == 0 {
		t.Fatalf("tickets not written: err=%v len=%d", err, len(data))
	}
}

func TestApply_Idempotent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "agent-tickets.jsonl", `{"n":1,"ticket":"BUG-1","task":"t","status":"open","ts":"2026-05-14T10:00:00Z","parent_ticket":"BUG","agent":"codex","role":"impl","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n")
	if err := Apply(runPlan(t, dir), ApplyOpts{}); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	first, _ := os.ReadFile(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if err := Apply(runPlan(t, dir), ApplyOpts{}); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	second, _ := os.ReadFile(filepath.Join(dir, "ledger", "tickets.jsonl"))
	if string(first) != string(second) {
		t.Fatalf("second apply changed tickets file:\nfirst=%s\nsecond=%s", first, second)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, ".ldgr", "backups"))
	if len(entries) > 1 {
		t.Fatalf("second run should not create a new backup; got %d backup dirs", len(entries))
	}
}

func TestApply_BacksUpExistingFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "ledger/tickets.jsonl", `{"n":1,"ts":"2026-05-14T09:00:00Z","ticket":"OLD-1","parent_ticket":"OLD","agent":"codex","role":"impl","status":"open","task":"old","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n")
	writeFile(t, dir, "agent-tickets.jsonl", `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"BUG-1","parent_ticket":"BUG","agent":"codex","role":"impl","status":"open","task":"new","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n")
	plan := runPlan(t, dir)
	if err := Apply(plan, ApplyOpts{}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, ".ldgr", "backups"))
	if len(entries) != 1 {
		t.Fatalf("expected one backup dir, got %d", len(entries))
	}
}

func TestApply_ArchiveOriginalsMovesLegacySources(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "agent-tickets.jsonl", `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"BUG-1","parent_ticket":"BUG","agent":"codex","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n")
	writeFile(t, dir, "goal.json", `{"schema_version":1,"summary":"hi"}`)
	plan := runPlan(t, dir)
	if err := Apply(plan, ApplyOpts{ArchiveOriginals: true}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "agent-tickets.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("legacy source should be moved away after --archive-originals, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".ldgr", "legacy", "agent-tickets.jsonl")); err != nil {
		t.Fatalf("legacy source should be at .ldgr/legacy/: %v", err)
	}
}

func TestApply_NoChangesDoesNotBackup(t *testing.T) {
	dir := t.TempDir()
	plan := runPlan(t, dir) // no sources, no changes
	if err := Apply(plan, ApplyOpts{}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".ldgr", "backups")); !os.IsNotExist(err) {
		t.Fatalf(".backup should not exist when nothing was changed")
	}
}
