package viewer

import "testing"

func TestServer_WorklogReverseChronological(t *testing.T) {
	srv, pid := newTestServer(t)
	// Append worklog rows in-place for this test.
	srv.LoadProject = wrapWithExtraWorklog(srv.LoadProject)
	var resp map[string]any
	if c := getJSON(t, srv.Handler(), "/api/projects/"+pid+"/worklog", &resp); c != 200 {
		t.Fatalf("status %d", c)
	}
	rows, _ := resp["rows"].([]any)
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	first, _ := rows[0].(map[string]any)
	if first["ts"] != "2026-05-14T10:05:00Z" {
		t.Fatalf("want newest first, got %v", first["ts"])
	}
}
func wrapWithExtraWorklog(orig func(string) (Project, error)) func(string) (Project, error) {
	return func(id string) (Project, error) {
		p, err := orig(id)
		if err != nil {
			return p, err
		}
		p.Worklog = append(p.Worklog,
			map[string]any{"n": float64(1), "ts": "2026-05-14T10:00:00Z", "ticket": "BUG-1", "agent": "codex", "task": "old"},
			map[string]any{"n": float64(2), "ts": "2026-05-14T10:05:00Z", "ticket": "BUG-1", "agent": "codex", "task": "new"},
		)
		return p, nil
	}
}
