package viewer

import (
	"net/http"
	"testing"

	"github.com/hgwk/ldgr/internal/ledger"
)

func TestServer_ProjectsListsRegistered(t *testing.T) {
	srv, pid := newTestServer(t)
	var arr []map[string]any
	if c := getJSON(t, srv.Handler(), "/api/projects", &arr); c != 200 {
		t.Fatalf("status %d", c)
	}
	if len(arr) != 1 || arr[0]["project_id"] != pid {
		t.Fatalf("expected single project %s, got %+v", pid, arr)
	}
	disp, _ := arr[0]["display"].(string)
	if disp == "" {
		t.Fatalf("display missing: %+v", arr[0])
	}
}

func TestServer_GETProjectIDReturns404OnUnknown(t *testing.T) {
	srv, _ := newTestServer(t)
	c := getJSON(t, srv.Handler(), "/api/projects/does-not-exist", nil)
	if c != http.StatusNotFound {
		t.Fatalf("want 404, got %d", c)
	}
}
func TestServer_ProjectSummaryCountsAuditLifecycleAsActive(t *testing.T) {
	srv, pid := newTestServer(t)
	orig := srv.LoadProject
	srv.LoadProject = func(id string) (Project, error) {
		p, err := orig(id)
		if err != nil {
			return p, err
		}
		p.Tickets = append(p.Tickets,
			map[string]any{"n": float64(2), "ts": "2026-05-14T10:01:00Z", "ticket": "AUDIT-1", "parent_ticket": "ROOT", "agent": "codex", "role": "impl", "status": "audit_ready", "task": "audit", "scope": "repo", "paths": []any{}, "blocked_by": []any{}, "branch": ""},
			map[string]any{"n": float64(3), "ts": "2026-05-14T10:02:00Z", "ticket": "FIX-1", "parent_ticket": "ROOT", "agent": "codex", "role": "audit", "status": "changes_requested", "task": "fix", "scope": "repo", "paths": []any{}, "blocked_by": []any{}, "branch": ""},
			map[string]any{"n": float64(4), "ts": "2026-05-14T10:03:00Z", "id": "READY-1", "parent": "ROOT", "type": "task", "state": "ready", "title": "ready"},
			map[string]any{"n": float64(5), "ts": "2026-05-14T10:04:00Z", "id": "REVIEW-1", "parent": "ROOT", "type": "task", "state": "review", "title": "review"},
			map[string]any{"n": float64(6), "ts": "2026-05-14T10:05:00Z", "id": "REWORK-1", "parent": "ROOT", "type": "task", "state": "rework", "title": "rework"},
			map[string]any{"n": float64(7), "ts": "2026-05-14T10:06:00Z", "id": "DONE-1", "parent": "ROOT", "type": "task", "state": "done", "title": "done"},
			map[string]any{"n": float64(8), "ts": "2026-05-14T10:07:00Z", "id": "DROP-1", "parent": "ROOT", "type": "task", "state": "dropped", "title": "dropped"},
		)
		return p, nil
	}
	var arr []map[string]any
	if c := getJSON(t, srv.Handler(), "/api/projects", &arr); c != 200 {
		t.Fatalf("status %d", c)
	}
	if len(arr) != 1 || arr[0]["project_id"] != pid {
		t.Fatalf("project list wrong: %+v", arr)
	}
	if arr[0]["open_tickets"] != float64(6) {
		t.Fatalf("audit lifecycle statuses should count as active, got %+v", arr[0])
	}
	if arr[0]["total_tickets"] != float64(8) {
		t.Fatalf("total_tickets should include terminal rows, got %+v", arr[0])
	}
	if arr[0]["done_tickets"] != float64(1) || arr[0]["closed_tickets"] != float64(2) {
		t.Fatalf("terminal ticket summary wrong: %+v", arr[0])
	}
}

func TestServer_ProjectsSortByRecentActivityWithMissingLast(t *testing.T) {
	srv := &Server{
		ListProjects: func() ([]projectListEntry, error) {
			return []projectListEntry{
				{ProjectID: "old", Slug: "old", Name: "old"},
				{ProjectID: "missing", Slug: "missing", Name: "missing"},
				{ProjectID: "new", Slug: "new", Name: "new"},
			}, nil
		},
		LoadProject: func(id string) (Project, error) {
			switch id {
			case "old":
				return Project{Goal: ledger.Goal{Summary: "old"}, Tickets: []ledger.Row{{"ticket": "A", "status": "open", "ts": "2026-05-14T10:00:00Z"}}}, nil
			case "new":
				return Project{Goal: ledger.Goal{Summary: "new"}, Tickets: []ledger.Row{{"id": "B", "state": "ready", "ts": "2026-05-15T10:00:00Z"}}}, nil
			default:
				return Project{Missing: true}, nil
			}
		},
	}
	var arr []map[string]any
	if c := getJSON(t, srv.Handler(), "/api/projects", &arr); c != 200 {
		t.Fatalf("status %d", c)
	}
	got := []any{arr[0]["project_id"], arr[1]["project_id"], arr[2]["project_id"]}
	want := []any{"new", "old", "missing"}
	if got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("order = %v, want %v", got, want)
	}
}
