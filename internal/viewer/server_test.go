package viewer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hgwk/ldgr/internal/config"
	"github.com/hgwk/ldgr/internal/jsonio"
)

func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default("myapp", "id-1", "")
	if err := os.MkdirAll(filepath.Join(dir, "ledger"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := config.Save(filepath.Join(dir, "ledger", "config.json"), cfg); err != nil {
		t.Fatalf("config save: %v", err)
	}
	// goal seed
	jsonio.WriteJSON(filepath.Join(dir, "ledger", "goal.json"), map[string]any{
		"schema_version":   1,
		"summary":          "hello",
		"success_criteria": []any{},
	})
	// seed tickets jsonl
	os.WriteFile(filepath.Join(dir, "ledger", "tickets.jsonl"),
		[]byte(`{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"BUG-1","parent_ticket":"BUG","agent":"codex","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "ledger", "worklog.jsonl"), []byte{}, 0o644)

	srv, err := NewSingleProjectServer(dir)
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	srv.Now = func() time.Time { return time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC) }
	return srv, "id-1"
}

func getJSON(t *testing.T, h http.Handler, path string, out any) int {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK && out != nil {
		if err := json.NewDecoder(rec.Body).Decode(out); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
	}
	return rec.Code
}
