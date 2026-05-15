package viewer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hgwk/ldgr/internal/config"
	"github.com/hgwk/ldgr/internal/jsonio"
	"github.com/hgwk/ldgr/internal/registry"
	"github.com/hgwk/ldgr/internal/verify"
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

// silence unused import warning if test compilation strips it
var _ = registry.Registry{}

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
	if len(cols) != 4 {
		t.Fatalf("want 4 columns, got %d", len(cols))
	}
	wantIDs := []string{"plan", "implement", "verify", "complete"}
	for i, raw := range cols {
		col, _ := raw.(map[string]any)
		if col["id"] != wantIDs[i] {
			t.Fatalf("column %d id=%v want %s", i, col["id"], wantIDs[i])
		}
	}
}

func TestServer_TicketDetailShape(t *testing.T) {
	srv, pid := newTestServer(t)
	var resp map[string]any
	if c := getJSON(t, srv.Handler(), "/api/projects/"+pid+"/tickets/BUG-1", &resp); c != 200 {
		t.Fatalf("status %d", c)
	}
	if resp["ticket"] != "BUG-1" {
		t.Fatalf("ticket field wrong: %v", resp["ticket"])
	}
	if _, ok := resp["latest"]; !ok {
		t.Fatalf("missing latest: %+v", resp)
	}
	if _, ok := resp["history"]; !ok {
		t.Fatalf("missing history: %+v", resp)
	}
	if _, ok := resp["worklog"]; !ok {
		t.Fatalf("missing worklog: %+v", resp)
	}
}

func TestServer_TicketDetailExposesEnrichedFields(t *testing.T) {
	// Seed a project where the latest ticket row carries the enriched fields
	// the drawer renders (audit_notes, acceptance, decision, notes, claim,
	// handoff, reviewed_n). The viewer is read-only and passes rows through
	// verbatim, so this guards the projection shape.
	dir := t.TempDir()
	cfg := config.Default("myapp", "id-1", "")
	if err := os.MkdirAll(filepath.Join(dir, "ledger"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := config.Save(filepath.Join(dir, "ledger", "config.json"), cfg); err != nil {
		t.Fatalf("config save: %v", err)
	}
	jsonio.WriteJSON(filepath.Join(dir, "ledger", "goal.json"), map[string]any{
		"schema_version":   1,
		"summary":          "hello",
		"success_criteria": []any{},
	})
	rows := strings.Join([]string{
		`{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"BUG-1","parent_ticket":"BUG","agent":"codex","role":"impl","status":"audit_ready","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":"","notes":"archived=foo; borrow=bar; reference=baz; new=qux"}`,
		`{"n":2,"ts":"2026-05-14T11:00:00Z","ticket":"BUG-1","parent_ticket":"BUG","agent":"codex","role":"audit","status":"done","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":"","audit_notes":"looks good","acceptance":["A","B"],"decision":"approved","reviewed_n":1,"claimed_by":"alice","claim_until":"2026-05-14T13:00:00Z","handoff_to":"bob","handoff":"see PR"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "ledger", "tickets.jsonl"), []byte(rows), 0o644); err != nil {
		t.Fatalf("write tickets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ledger", "worklog.jsonl"), []byte{}, 0o644); err != nil {
		t.Fatalf("write worklog: %v", err)
	}
	srv, err := NewSingleProjectServer(dir)
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	srv.Now = func() time.Time { return time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC) }

	var resp map[string]any
	if c := getJSON(t, srv.Handler(), "/api/projects/id-1/tickets/BUG-1", &resp); c != 200 {
		t.Fatalf("status %d", c)
	}
	latest, ok := resp["latest"].(map[string]any)
	if !ok {
		t.Fatalf("latest not an object: %+v", resp["latest"])
	}
	want := map[string]any{
		"audit_notes": "looks good",
		"decision":    "approved",
		"claimed_by":  "alice",
		"claim_until": "2026-05-14T13:00:00Z",
		"handoff_to":  "bob",
		"handoff":     "see PR",
	}
	for k, v := range want {
		if got := latest[k]; got != v {
			t.Fatalf("latest[%q] = %v, want %v", k, got, v)
		}
	}
	acc, ok := latest["acceptance"].([]any)
	if !ok || len(acc) != 2 || acc[0] != "A" || acc[1] != "B" {
		t.Fatalf("acceptance = %v", latest["acceptance"])
	}
	rn, ok := latest["reviewed_n"].(float64)
	if !ok || int(rn) != 1 {
		t.Fatalf("reviewed_n = %v", latest["reviewed_n"])
	}
	hist, ok := resp["history"].([]any)
	if !ok || len(hist) != 2 {
		t.Fatalf("history len = %d, want 2", len(hist))
	}
	h0, _ := hist[0].(map[string]any)
	if notes, _ := h0["notes"].(string); !strings.Contains(notes, "archived=") {
		t.Fatalf("history[0].notes missing provenance: %v", h0["notes"])
	}
}

func TestServer_TicketDetailUnknown404(t *testing.T) {
	srv, pid := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/projects/"+pid+"/tickets/does-not-exist", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestServer_ServesIndex(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content-type %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "<title>ldgr</title>") {
		t.Fatalf("index body missing title:\n%s", rec.Body.String())
	}
}

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

func TestServer_VerifyEndpointSuccess(t *testing.T) {
	srv, pid := newTestServer(t)
	// Inject a stub verifier that returns warns only (default mode).
	srv.RunVerify = func(target string, strict bool) (verify.Report, error) {
		return verify.Report{
			Warns: []verify.Issue{
				{Code: "WEAK_DONE", Message: "weak A", File: "ledger/tickets.jsonl", Line: 1},
				{Code: "WEAK_DONE", Message: "weak B"},
				{Code: "ORPHAN_WORKLOG", Message: "orphan"},
			},
		}, nil
	}
	var resp map[string]any
	if c := getJSON(t, srv.Handler(), "/api/projects/"+pid+"/verify", &resp); c != 200 {
		t.Fatalf("status %d", c)
	}
	if resp["strict"] != false {
		t.Fatalf("strict=%v", resp["strict"])
	}
	if resp["fail_count"] != float64(0) || resp["warn_count"] != float64(3) {
		t.Fatalf("counts wrong: %+v", resp)
	}
	bc, _ := resp["by_code"].(map[string]any)
	if bc["WEAK_DONE"] != float64(2) || bc["ORPHAN_WORKLOG"] != float64(1) {
		t.Fatalf("by_code: %+v", bc)
	}
	samples, _ := resp["samples"].([]any)
	if len(samples) == 0 || len(samples) > 5 {
		t.Fatalf("samples len=%d", len(samples))
	}
	s0, _ := samples[0].(map[string]any)
	if s0["severity"] != "warning" {
		t.Fatalf("sample severity=%v", s0["severity"])
	}
}

func TestServer_VerifyEndpointStrictPromotesWarnings(t *testing.T) {
	srv, pid := newTestServer(t)
	// Honor strict: when strict=true, warns are promoted to fails.
	srv.RunVerify = func(target string, strict bool) (verify.Report, error) {
		rep := verify.Report{
			Warns: []verify.Issue{{Code: "WEAK_DONE", Message: "weak"}},
		}
		if strict {
			rep.Fails = append(rep.Fails, rep.Warns...)
			rep.Warns = nil
		}
		return rep, nil
	}
	var loose map[string]any
	if c := getJSON(t, srv.Handler(), "/api/projects/"+pid+"/verify", &loose); c != 200 {
		t.Fatalf("status %d", c)
	}
	if loose["fail_count"] != float64(0) || loose["warn_count"] != float64(1) {
		t.Fatalf("default mode wrong: %+v", loose)
	}

	var strict map[string]any
	if c := getJSON(t, srv.Handler(), "/api/projects/"+pid+"/verify?strict=1", &strict); c != 200 {
		t.Fatalf("status %d", c)
	}
	if strict["fail_count"] != float64(1) || strict["warn_count"] != float64(0) {
		t.Fatalf("strict mode wrong: %+v", strict)
	}
	samples, _ := strict["samples"].([]any)
	if len(samples) != 1 {
		t.Fatalf("want 1 sample, got %d", len(samples))
	}
	s0, _ := samples[0].(map[string]any)
	if s0["severity"] != "fail" {
		t.Fatalf("sample severity=%v want fail", s0["severity"])
	}
}

func TestServer_VerifyEndpointMissingProject(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.RunVerify = func(target string, strict bool) (verify.Report, error) {
		return verify.Report{}, nil
	}
	c := getJSON(t, srv.Handler(), "/api/projects/does-not-exist/verify", nil)
	if c != http.StatusNotFound {
		t.Fatalf("want 404, got %d", c)
	}
}

func TestServer_VerifyEndpointCachesResults(t *testing.T) {
	srv, pid := newTestServer(t)
	calls := 0
	srv.RunVerify = func(target string, strict bool) (verify.Report, error) {
		calls++
		return verify.Report{}, nil
	}
	// Two requests within TTL should hit the cache; only one underlying run.
	for i := 0; i < 3; i++ {
		var resp map[string]any
		if c := getJSON(t, srv.Handler(), "/api/projects/"+pid+"/verify", &resp); c != 200 {
			t.Fatalf("status %d", c)
		}
	}
	if calls != 1 {
		t.Fatalf("expected cache hit, calls=%d", calls)
	}
}
