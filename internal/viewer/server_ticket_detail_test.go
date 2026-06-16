package viewer

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hgwk/ldgr/internal/config"
	"github.com/hgwk/ldgr/internal/jsonio"
)

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

func TestServer_EmbedsDrawerShellStyles(t *testing.T) {
	body, err := fs.ReadFile(Assets(), "style-drawer.css")
	if err != nil {
		t.Fatalf("read style-drawer.css: %v", err)
	}
	css := string(body)
	for _, want := range []string{
		"#drawer",
		"position: fixed",
		"#drawer.open",
		"transform: translateX(0)",
	} {
		if !strings.Contains(css, want) {
			t.Fatalf("style-drawer.css missing %q:\n%s", want, css)
		}
	}
}
