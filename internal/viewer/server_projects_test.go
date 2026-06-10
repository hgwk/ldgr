package viewer

import (
	"net/http"
	"testing"
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
	if arr[0]["open_tickets"] != float64(3) {
		t.Fatalf("audit lifecycle statuses should count as active, got %+v", arr[0])
	}
}
