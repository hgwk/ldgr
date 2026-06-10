package viewer

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hgwk/ldgr/internal/config"
	"github.com/hgwk/ldgr/internal/jsonio"
)

func TestServer_AuditQueueShape(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default("myapp", "id-1", "")
	if err := os.MkdirAll(filepath.Join(dir, "ledger"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := config.Save(filepath.Join(dir, "ledger", "config.json"), cfg); err != nil {
		t.Fatalf("config save: %v", err)
	}
	jsonio.WriteJSON(filepath.Join(dir, "ledger", "goal.json"), map[string]any{
		"schema_version": 1, "summary": "g", "success_criteria": []any{},
	})
	rows := `{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"BUG-1","parent_ticket":"BUG","agent":"x","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}` + "\n" +
		`{"n":2,"ts":"2026-05-14T10:30:00Z","ticket":"AUD-1","parent_ticket":"AUD","agent":"codex","role":"impl","status":"audit_ready","task":"check","priority":"P0","evidence":["link"],"claimed_by":"alice"}` + "\n"
	os.WriteFile(filepath.Join(dir, "ledger", "tickets.jsonl"), []byte(rows), 0o644)
	os.WriteFile(filepath.Join(dir, "ledger", "worklog.jsonl"), []byte{}, 0o644)
	srv, err := NewSingleProjectServer(dir)
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	srv.Now = func() time.Time { return time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC) }
	var resp map[string]any
	if c := getJSON(t, srv.Handler(), "/api/projects/id-1/audit-queue", &resp); c != 200 {
		t.Fatalf("status %d", c)
	}
	items, _ := resp["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d: %+v", len(items), resp)
	}
	it, _ := items[0].(map[string]any)
	if it["ticket_id"] != "AUD-1" || it["priority"] != "P0" || it["has_evidence"] != true {
		t.Fatalf("unexpected item: %+v", it)
	}
}
