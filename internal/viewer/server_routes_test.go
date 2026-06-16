package viewer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hgwk/ldgr/internal/config"
	"github.com/hgwk/ldgr/internal/jsonio"
)

func TestServer_TicketsTreeShape(t *testing.T) {
	srv, pid := newTestServer(t)
	var resp map[string]any
	if c := getJSON(t, srv.Handler(), "/api/projects/"+pid+"/tickets", &resp); c != 200 {
		t.Fatalf("status %d", c)
	}
	tree, _ := resp["tree"].([]any)
	if len(tree) != 1 {
		t.Fatalf("want 1 bucket, got %v", tree)
	}
	bucket, _ := tree[0].(map[string]any)
	if bucket["parent"] != "BUG" {
		t.Fatalf("want parent=BUG, got %v", bucket["parent"])
	}
}

func TestServer_GoalEndpoint(t *testing.T) {
	srv, pid := newTestServer(t)
	var resp map[string]any
	if c := getJSON(t, srv.Handler(), "/api/projects/"+pid+"/goal", &resp); c != 200 {
		t.Fatalf("status %d", c)
	}
	if resp["summary"] != "hello" {
		t.Fatalf("goal endpoint returned wrong body: %+v", resp)
	}
}
func TestServer_InsightsHasReadyQueue(t *testing.T) {
	srv, pid := newTestServer(t)
	var resp map[string]any
	if c := getJSON(t, srv.Handler(), "/api/projects/"+pid+"/insights", &resp); c != 200 {
		t.Fatalf("status %d", c)
	}
	rq, _ := resp["readyQueue"].([]any)
	if len(rq) == 0 {
		t.Fatalf("readyQueue empty: %+v", resp)
	}
}

func TestServer_SingleProjectMode(t *testing.T) {
	srv, _ := newTestServer(t)
	var arr []map[string]any
	if c := getJSON(t, srv.Handler(), "/api/projects", &arr); c != 200 {
		t.Fatalf("status %d", c)
	}
	if len(arr) != 1 {
		t.Fatalf("single-project mode must return exactly 1 project, got %d", len(arr))
	}
}
func TestServer_DashboardShape(t *testing.T) {
	srv, pid := newTestServer(t)
	var resp map[string]any
	if c := getJSON(t, srv.Handler(), "/api/projects/"+pid+"/dashboard", &resp); c != 200 {
		t.Fatalf("status %d", c)
	}
	for _, k := range []string{"progress", "parents", "audit", "health", "recent"} {
		if _, ok := resp[k]; !ok {
			t.Fatalf("missing key %s in dashboard response: %+v", k, resp)
		}
	}
}

func TestServer_KanbanShape(t *testing.T) {
	srv, pid := newTestServer(t)
	var resp map[string]any
	if c := getJSON(t, srv.Handler(), "/api/projects/"+pid+"/kanban", &resp); c != 200 {
		t.Fatalf("status %d", c)
	}
	cols, _ := resp["columns"].([]any)
	if len(cols) != 8 {
		t.Fatalf("want 8 columns, got %d", len(cols))
	}
	wantIDs := []string{"ready", "doing", "review", "rework", "backlog", "blocked", "done", "dropped"}
	for i, raw := range cols {
		col, _ := raw.(map[string]any)
		if col["id"] != wantIDs[i] {
			t.Fatalf("column %d id=%v want %s", i, col["id"], wantIDs[i])
		}
	}
	grid, _ := resp["grid"].([]any)
	if len(grid) != 2 {
		t.Fatalf("want 2-row grid, got %+v", resp["grid"])
	}
}

func TestServer_StateKanbanShape(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default("myapp", "id-state", "")
	cfg.SchemaVersion = 1
	if err := os.MkdirAll(filepath.Join(dir, "ledger"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := config.Save(filepath.Join(dir, "ledger", "config.json"), cfg); err != nil {
		t.Fatalf("config save: %v", err)
	}
	jsonio.WriteJSON(filepath.Join(dir, "ledger", "goal.json"), map[string]any{"schema_version": 1, "summary": "hello"})
	os.WriteFile(filepath.Join(dir, "ledger", "tickets.jsonl"),
		[]byte(`{"n":1,"ts":"2026-05-14T10:00:00Z","id":"STATE-1","parent":"ROOT","type":"task","state":"ready","area":"backend","priority":"P1","title":"t","owner":"codex","blocked_by":[],"acceptance":[],"evidence":[],"event":{"actor":"codex","role":"planner","summary":"opened","notes":""}}`+"\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "ledger", "worklog.jsonl"), []byte{}, 0o644)
	srv, err := NewSingleProjectServer(dir)
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	var resp map[string]any
	if c := getJSON(t, srv.Handler(), "/api/projects/id-state/kanban", &resp); c != 200 {
		t.Fatalf("status %d", c)
	}
	cols, _ := resp["columns"].([]any)
	if len(cols) != 8 {
		t.Fatalf("want 8 state columns, got %d", len(cols))
	}
	grid, _ := resp["grid"].([]any)
	if len(grid) != 2 {
		t.Fatalf("want 2-row grid, got %+v", resp["grid"])
	}
}
