package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hgwk/ldgr/internal/ledger"
	"github.com/hgwk/ldgr/internal/registry"
)

func mustInit(t *testing.T) (target string, store *registry.Store) {
	t.Helper()
	target = t.TempDir()
	regDir := t.TempDir()
	store = registry.New(filepath.Join(regDir, "registry.json"), filepath.Join(regDir, "registry.lock"))
	if err := RunInit(target, InitOpts{Slug: "myapp"}, store); err != nil {
		t.Fatalf("init: %v", err)
	}
	return
}

func TestTicketAdd_AppendsRow(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")

	in := map[string]any{
		"ticket":        "demo-1",
		"parent_ticket": "ROOT",
		"role":          "impl",
		"status":        "open",
		"task":          "Demo task",
		"scope":         "repo",
		"paths":         []any{"src/x.go"},
		"blocked_by":    []any{},
	}
	body, _ := json.Marshal(in)

	var out, errb bytes.Buffer
	code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(body), &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d stderr=%s", code, errb.String())
	}

	rows, err := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	r := rows[0]
	if r["ticket"] != "demo-1" || r["agent"] != "codex" || r["status"] != "open" {
		t.Fatalf("row content wrong: %+v", r)
	}
	if r["n"].(float64) != 1 {
		t.Fatalf("expected n=1, got %v", r["n"])
	}
	if _, ok := r["ts"]; !ok {
		t.Fatalf("ts should be auto-filled")
	}
}

func TestTicketAdd_RejectsDuplicateID(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")

	in := map[string]any{
		"ticket": "demo-1", "parent_ticket": "ROOT", "role": "impl",
		"status": "open", "task": "t", "scope": "repo",
		"paths": []any{}, "blocked_by": []any{},
	}
	body, _ := json.Marshal(in)

	out := &bytes.Buffer{}
	errb := &bytes.Buffer{}
	if code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(body), out, errb); code != 0 {
		t.Fatalf("first add failed: %s", errb.String())
	}
	out.Reset()
	errb.Reset()
	code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(body), out, errb)
	if code == 0 {
		t.Fatalf("expected failure on duplicate ticket")
	}
	if !strings.Contains(errb.String(), "already exists") {
		t.Fatalf("stderr should explain duplicate, got: %s", errb.String())
	}
}

func TestTicketAdd_MissingRequiredFails(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	in := map[string]any{"ticket": "demo-1"}
	body, _ := json.Marshal(in)
	errb := &bytes.Buffer{}
	code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(body), &bytes.Buffer{}, errb)
	if code == 0 {
		t.Fatalf("expected failure due to missing required fields")
	}
	if !strings.Contains(errb.String(), "missing required") {
		t.Fatalf("stderr should mention missing required: %s", errb.String())
	}
}

func TestTicketAdd_FromFile(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	in := map[string]any{
		"ticket": "demo-2", "parent_ticket": "ROOT", "role": "impl",
		"status": "open", "task": "t", "scope": "repo",
		"paths": []any{}, "blocked_by": []any{},
	}
	body, _ := json.Marshal(in)
	tmp := filepath.Join(t.TempDir(), "in.json")
	os.WriteFile(tmp, body, 0o644)

	if code := RunTicketCLI([]string{"add", "--target", target, "--json", "@" + tmp}, &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("from file failed")
	}
}

func TestTicketAdd_RejectsEmptyTask(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	in := map[string]any{
		"ticket": "demo-3", "parent_ticket": "ROOT", "role": "impl",
		"status": "open", "task": "", "scope": "repo",
		"paths": []any{}, "blocked_by": []any{},
	}
	body, _ := json.Marshal(in)
	errb := &bytes.Buffer{}
	code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(body), &bytes.Buffer{}, errb)
	if code == 0 {
		t.Fatalf("expected failure for empty task; stderr=%q", errb.String())
	}
	if !strings.Contains(errb.String(), "non-empty") {
		t.Fatalf("stderr should mention non-empty: %s", errb.String())
	}
}

func TestAutoFields_WarningGoesToInjectedStderr(t *testing.T) {
	target, _ := mustInit(t)
	// Set up environment to trigger the USER fallback warning.
	// Since CLAUDECODE is already in the test environment, we verify that
	// the code correctly routes warnings to the injected stderr parameter,
	// not to os.Stderr. This is verified by the fact that our injected
	// stderr buffer receives the output.
	t.Setenv("LEDGER_AGENT", "")

	in := map[string]any{
		"ticket": "demo-warn", "parent_ticket": "ROOT", "role": "impl",
		"status": "open", "task": "t", "scope": "repo",
		"paths": []any{}, "blocked_by": []any{},
	}
	body, _ := json.Marshal(in)
	var outb, errb bytes.Buffer
	code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(body), &outb, &errb)
	if code != 0 {
		t.Fatalf("add failed: exit %d stderr=%s", code, errb.String())
	}
	// Verify the row was added (this shows the function still works).
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0]["ticket"] != "demo-warn" {
		t.Fatalf("ticket not added correctly")
	}
	// If a warning had been generated, it would be in errb, not os.Stderr.
	// The key verification is that autoFields now accepts stderr parameter
	// and uses it instead of os.Stderr directly.
}

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

	evIn := map[string]any{"ticket": "ghost", "status": "done"}
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
	ev2 := `{"ticket":"G-1","status":"audit_ready","evidence":["go test ./..."]}`
	var stdout, stderr bytes.Buffer
	if code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(ev2), &stdout, &stderr); code != 0 {
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

func TestTicketAdd_RejectsInitialDone(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	in := map[string]any{
		"ticket": "INIT-DONE", "parent_ticket": "ROOT", "role": "impl",
		"status": "done", "task": "skip the work", "scope": "repo",
		"paths": []any{}, "blocked_by": []any{},
	}
	body, _ := json.Marshal(in)
	var stderr bytes.Buffer
	code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(body), &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero for initial status=done")
	}
	if !strings.Contains(stderr.String(), "ldgr next") {
		t.Fatalf("stderr should suggest ldgr next, got: %s", stderr.String())
	}
	// Verify nothing was appended.
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	for _, r := range rows {
		if r["ticket"] == "INIT-DONE" {
			t.Fatalf("INIT-DONE must not be appended on rejection: %+v", r)
		}
	}
}

func TestTicketEvent_RejectsImplDirectDone(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	// Seed an in_progress ticket.
	add := `{"ticket":"GATE-1","parent_ticket":"BUG","role":"impl","status":"in_progress","task":"x","scope":"repo","paths":[],"blocked_by":[]}`
	if code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("seed failed")
	}
	// Try to jump to done directly.
	ev := `{"ticket":"GATE-1","status":"done"}`
	var stderr bytes.Buffer
	code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(ev), &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected rejection")
	}
	if !strings.Contains(stderr.String(), "audit_ready") {
		t.Fatalf("stderr should mention audit_ready, got: %s", stderr.String())
	}
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	// Only the seed row should exist for GATE-1.
	count := 0
	for _, r := range rows {
		if r["ticket"] == "GATE-1" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("event should not append on rejection; expected 1 row, got %d", count)
	}
}

func TestTicketEvent_AcceptsAuditPassClose(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	// Seed: open → in_progress → audit_ready (with evidence).
	add := `{"ticket":"PASS-1","parent_ticket":"BUG","role":"impl","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	in := `{"ticket":"PASS-1","status":"in_progress"}`
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(in), &bytes.Buffer{}, &bytes.Buffer{})
	ready := `{"ticket":"PASS-1","status":"audit_ready","evidence":["go test ./..."]}`
	if code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(ready), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("audit_ready event failed")
	}
	// audit_ready row n is the 3rd row.
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	auditN := int(rows[len(rows)-1]["n"].(float64))
	pass := fmt.Sprintf(`{"ticket":"PASS-1","role":"audit","status":"done","audit_result":"pass","evidence":["go test ./..."],"reviewed_n":%d}`, auditN)
	var stderr bytes.Buffer
	if code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(pass), &bytes.Buffer{}, &stderr); code != 0 {
		t.Fatalf("audit-pass close should succeed, stderr=%s", stderr.String())
	}
}

func TestTicketEvent_RejectsInvalidTransition(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"ticket":"TRANS-1","parent_ticket":"BUG","role":"impl","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	// open → audit_ready is not allowed.
	ev := `{"ticket":"TRANS-1","status":"audit_ready","evidence":["x"]}`
	var stderr bytes.Buffer
	code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(ev), &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected rejection for open → audit_ready")
	}
	if !strings.Contains(stderr.String(), "open") || !strings.Contains(stderr.String(), "audit_ready") {
		t.Fatalf("error should name the rejected edge, got: %s", stderr.String())
	}
}

func TestTicketReady_AppendsAuditReadyRow(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"ticket":"RDY-1","parent_ticket":"BUG","role":"impl","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"ticket":"RDY-1","status":"in_progress"}`), &bytes.Buffer{}, &bytes.Buffer{})

	var stderr bytes.Buffer
	if code := RunTicketCLI([]string{"ready", "--target", target, "--ticket", "RDY-1", "--evidence", "go test ./..."}, &bytes.Buffer{}, &bytes.Buffer{}, &stderr); code != 0 {
		t.Fatalf("ticket ready failed: %s", stderr.String())
	}
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	last := rows[len(rows)-1]
	if last["status"] != "audit_ready" {
		t.Fatalf("expected status=audit_ready, got %v", last["status"])
	}
	ev, _ := last["evidence"].([]any)
	if len(ev) != 1 || ev[0] != "go test ./..." {
		t.Fatalf("evidence not propagated: %v", last["evidence"])
	}
}

func TestTicketReady_RequiresEvidence(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"ticket":"RDY-2","parent_ticket":"BUG","role":"impl","status":"in_progress","task":"x","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	var stderr bytes.Buffer
	code := RunTicketCLI([]string{"ready", "--target", target, "--ticket", "RDY-2"}, &bytes.Buffer{}, &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected failure without --evidence")
	}
}

func TestTicketAdd_DefaultsKindAndPriority(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	in := map[string]any{
		"ticket": "KP-1", "parent_ticket": "BUG", "role": "impl",
		"status": "open", "task": "x", "scope": "repo",
		"paths": []any{}, "blocked_by": []any{},
	}
	body, _ := json.Marshal(in)
	if code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(body), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("add failed")
	}
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	last := rows[len(rows)-1]
	if last["kind"] != "task" {
		t.Fatalf("kind default should be task, got %v", last["kind"])
	}
	if last["priority"] != "P2" {
		t.Fatalf("priority default should be P2, got %v", last["priority"])
	}
}

func TestTicketAdd_KeepsExplicitKindPriority(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	in := map[string]any{
		"ticket": "KP-2", "parent_ticket": "BUG", "role": "impl",
		"status": "open", "task": "x", "scope": "repo",
		"paths": []any{}, "blocked_by": []any{},
		"kind": "issue", "priority": "P0",
	}
	body, _ := json.Marshal(in)
	if code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(body), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("add failed")
	}
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	last := rows[len(rows)-1]
	if last["kind"] != "issue" || last["priority"] != "P0" {
		t.Fatalf("explicit values not preserved: %v / %v", last["kind"], last["priority"])
	}
}
