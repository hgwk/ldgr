package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hgwk/ldgr/internal/ledger"
)

func TestTicketEvent_CarriesForwardUnmentionedFields(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")

	addIn := map[string]any{
		"ticket": "carry-1", "parent_ticket": "ROOT", "role": "impl",
		"status": "open", "task": "do the thing", "scope": "repo",
		"paths":      []any{"src/a.go", "src/b.go"},
		"blocked_by": []any{},
	}
	addBody, _ := json.Marshal(addIn)
	if code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(addBody), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("add failed")
	}

	// event only updates status — task and paths must be carried forward.
	evIn := map[string]any{"ticket": "carry-1", "status": "in_progress"}
	evBody, _ := json.Marshal(evIn)
	if code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, bytes.NewReader(evBody), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("event failed")
	}

	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows after add+event, got %d", len(rows))
	}
	latest := rows[1]
	if latest["status"] != "in_progress" {
		t.Fatalf("status not overlaid: %v", latest["status"])
	}
	if latest["task"] != "do the thing" {
		t.Fatalf("task should carry forward, got %v", latest["task"])
	}
	paths, ok := latest["paths"].([]any)
	if !ok || len(paths) != 2 {
		t.Fatalf("paths should carry forward (2 items), got %v", latest["paths"])
	}
}

func TestTicketEvent_ShallowReplacesArrays(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")

	addIn := map[string]any{
		"ticket": "replace-1", "parent_ticket": "ROOT", "role": "impl",
		"status": "open", "task": "t", "scope": "repo",
		"paths":      []any{"src/a.go", "src/b.go"},
		"blocked_by": []any{"x", "y"},
	}
	addBody, _ := json.Marshal(addIn)
	if code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(addBody), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("add failed")
	}

	// event sends a single-element paths array — must replace, not merge.
	evIn := map[string]any{
		"ticket":     "replace-1",
		"paths":      []any{"src/c.go"},
		"blocked_by": []any{},
	}
	evBody, _ := json.Marshal(evIn)
	if code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, bytes.NewReader(evBody), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("event failed")
	}

	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	latest := rows[1]
	paths, _ := latest["paths"].([]any)
	if len(paths) != 1 || paths[0] != "src/c.go" {
		t.Fatalf("paths should be replaced wholesale to [src/c.go], got %v", paths)
	}
	bb, _ := latest["blocked_by"].([]any)
	if len(bb) != 0 {
		t.Fatalf("blocked_by should be replaced to [], got %v", bb)
	}
}

func TestTicketEvent_RejectsUnknownTicket(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")

	evIn := map[string]any{"id": "ghost", "state": "done", "event": map[string]any{"role": "auditor", "result": "pass", "summary": "passed", "notes": "", "reviewed_n": 1}}
	body, _ := json.Marshal(evIn)
	errb := &bytes.Buffer{}
	code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, bytes.NewReader(body), &bytes.Buffer{}, errb)
	if code == 0 {
		t.Fatalf("expected failure for unknown ticket")
	}
	if !strings.Contains(errb.String(), "does not exist") {
		t.Fatalf("stderr should say 'does not exist': %s", errb.String())
	}
}

func TestTicketEvent_OverlaysNotesAndDecision(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")

	addIn := map[string]any{
		"ticket": "notes-1", "parent_ticket": "ROOT", "role": "impl",
		"status": "open", "task": "t", "scope": "repo",
		"paths": []any{}, "blocked_by": []any{},
	}
	body, _ := json.Marshal(addIn)
	if code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(body), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("add failed")
	}

	evIn := map[string]any{"ticket": "notes-1", "notes": "deferred", "decision": "wait until Q3"}
	evBody, _ := json.Marshal(evIn)
	if code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, bytes.NewReader(evBody), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("event failed")
	}

	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	latest := rows[1]
	if latest["notes"] != "deferred" || latest["decision"] != "wait until Q3" {
		t.Fatalf("notes/decision not overlaid: %+v", latest)
	}
	// status should carry forward (still "open")
	if latest["status"] != "open" {
		t.Fatalf("status should carry forward as 'open', got %v", latest["status"])
	}
}

func TestTicketEvent_PrintsGuidanceToStderr(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"ticket":"G-1","parent_ticket":"BUG","role":"impl","status":"open","task":"do","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})

	// First transition: open → in_progress
	ev1 := `{"ticket":"G-1","status":"in_progress"}`
	if code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(ev1), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("first event failed")
	}

	// Second transition: in_progress → audit_ready
	evReview := `{"ticket":"G-1","status":"audit_ready","evidence":["go test ./..."]}`
	var stdout, stderr bytes.Buffer
	if code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(evReview), &stdout, &stderr); code != 0 {
		t.Fatalf("event failed: %s", stderr.String())
	}
	// stdout is still JSON.
	var row map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &row); err != nil {
		t.Fatalf("stdout must remain JSON: %v\n%s", err, stdout.String())
	}
	if row["status"] != "audit_ready" {
		t.Fatalf("status wrong: %v", row["status"])
	}
	// stderr contains audit guidance.
	if !strings.Contains(stderr.String(), "audit_ready") || !strings.Contains(stderr.String(), "Next:") {
		t.Fatalf("stderr missing guidance: %s", stderr.String())
	}
}
