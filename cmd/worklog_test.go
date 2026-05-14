package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hgwk/ldgr/internal/ledger"
)

func TestWorklogAdd_AppendsRow(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")

	// Create and drive ticket to audit-pass done
	add := `{"ticket":"demo-1","parent_ticket":"BUG","role":"impl","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"ticket":"demo-1","status":"in_progress"}`), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"ticket":"demo-1","status":"audit_ready","evidence":["go test"]}`), &bytes.Buffer{}, &bytes.Buffer{})
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	auditN := int(rows[len(rows)-1]["n"].(float64))
	pass := fmt.Sprintf(`{"ticket":"demo-1","role":"audit","status":"done","audit_result":"pass","evidence":["go test"],"reviewed_n":%d}`, auditN)
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(pass), &bytes.Buffer{}, &bytes.Buffer{})

	in := map[string]any{
		"ticket":   "demo-1",
		"task":     "demo work",
		"scope":    "repo",
		"result":   "done",
		"paths":    []any{},
		"commands": []any{"go test"},
	}
	body, _ := json.Marshal(in)
	errb := &bytes.Buffer{}
	if code := RunWorklogCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(body), &bytes.Buffer{}, errb); code != 0 {
		t.Fatalf("add failed: %s", errb.String())
	}
	wlRows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "worklog.jsonl"))
	if len(wlRows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(wlRows))
	}
}

func TestWorklogAdd_SucceedsWhenTicketAuditPass(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"ticket":"W-1","parent_ticket":"BUG","role":"impl","status":"open","task":"do","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	// Transition to in_progress
	inp := `{"ticket":"W-1","status":"in_progress"}`
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(inp), &bytes.Buffer{}, &bytes.Buffer{})
	// Transition to audit_ready
	evch := `{"ticket":"W-1","status":"audit_ready","evidence":["done"]}`
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(evch), &bytes.Buffer{}, &bytes.Buffer{})
	// Get the latest ticket n for the audit approval
	tickRows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	auditN := int(tickRows[len(tickRows)-1]["n"].(float64))
	// Transition to audit done with pass
	appr := fmt.Sprintf(`{"ticket":"W-1","role":"audit","status":"done","audit_result":"pass","evidence":["go test"],"reviewed_n":%d}`, auditN)
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(appr), &bytes.Buffer{}, &bytes.Buffer{})

	wl := `{"ticket":"W-1","task":"early worklog","scope":"repo","result":"too early","paths":[],"commands":[]}`
	var stderr bytes.Buffer
	if code := RunWorklogCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(wl), &bytes.Buffer{}, &stderr); code != 0 {
		t.Fatalf("worklog add should succeed after audit pass: %s", stderr.String())
	}
	// Worklog should append successfully.
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "worklog.jsonl"))
	if len(rows) != 1 {
		t.Fatalf("expected 1 worklog row after audit pass, got %d", len(rows))
	}
}

func TestWorklogAdd_RejectsMissingTicket(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	// No `ticket` field.
	in := map[string]any{
		"task":     "no ticket",
		"scope":    "ledger",
		"result":   "?",
		"paths":    []any{},
		"commands": []any{},
	}
	body, _ := json.Marshal(in)
	var stderr bytes.Buffer
	code := RunWorklogCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(body), &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected rejection for missing ticket")
	}
	if !strings.Contains(stderr.String(), "ticket is required") {
		t.Fatalf("stderr should explain missing ticket, got: %s", stderr.String())
	}
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "worklog.jsonl"))
	if len(rows) != 0 {
		t.Fatalf("no row should be appended on rejection, got %d", len(rows))
	}
}

func TestWorklogAdd_RejectsUnknownTicket(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	in := map[string]any{
		"ticket": "GHOST",
		"task":   "x", "scope": "repo", "result": "?",
		"paths": []any{}, "commands": []any{},
	}
	body, _ := json.Marshal(in)
	var stderr bytes.Buffer
	code := RunWorklogCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(body), &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected rejection for unknown ticket")
	}
	if !strings.Contains(stderr.String(), "does not exist") {
		t.Fatalf("stderr should mention non-existent ticket, got: %s", stderr.String())
	}
}

func TestWorklogAdd_RejectsAuditReady(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	// Drive ticket to audit_ready (not done yet).
	add := `{"ticket":"WL-AR","parent_ticket":"BUG","role":"impl","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	inp := `{"ticket":"WL-AR","status":"in_progress"}`
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(inp), &bytes.Buffer{}, &bytes.Buffer{})
	ready := `{"ticket":"WL-AR","status":"audit_ready","evidence":["go test ./..."]}`
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(ready), &bytes.Buffer{}, &bytes.Buffer{})

	wl := `{"ticket":"WL-AR","task":"early","scope":"repo","result":"too soon","paths":[],"commands":[]}`
	var stderr bytes.Buffer
	code := RunWorklogCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(wl), &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected rejection while ticket is audit_ready")
	}
	if !strings.Contains(stderr.String(), "audit") {
		t.Fatalf("stderr should mention audit requirement, got: %s", stderr.String())
	}
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "worklog.jsonl"))
	if len(rows) != 0 {
		t.Fatalf("no worklog row should be appended, got %d", len(rows))
	}
}

func TestWorklogAdd_RejectsChangesRequested(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"ticket":"WL-CR","parent_ticket":"BUG","role":"impl","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"ticket":"WL-CR","status":"in_progress"}`), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"ticket":"WL-CR","status":"audit_ready","evidence":["go test"]}`), &bytes.Buffer{}, &bytes.Buffer{})
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	auditN := int(rows[len(rows)-1]["n"].(float64))
	cr := fmt.Sprintf(`{"ticket":"WL-CR","role":"audit","status":"changes_requested","audit_result":"changes_requested","audit_notes":"more tests","reviewed_n":%d}`, auditN)
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(cr), &bytes.Buffer{}, &bytes.Buffer{})

	wl := `{"ticket":"WL-CR","task":"early","scope":"repo","result":"too soon","paths":[],"commands":[]}`
	var stderr bytes.Buffer
	code := RunWorklogCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(wl), &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected rejection while ticket is changes_requested")
	}
}

func TestWorklogAdd_AcceptsAfterAuditPass(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"ticket":"WL-OK","parent_ticket":"BUG","role":"impl","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"ticket":"WL-OK","status":"in_progress"}`), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"ticket":"WL-OK","status":"audit_ready","evidence":["go test"]}`), &bytes.Buffer{}, &bytes.Buffer{})
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	auditN := int(rows[len(rows)-1]["n"].(float64))
	pass := fmt.Sprintf(`{"ticket":"WL-OK","role":"audit","status":"done","audit_result":"pass","evidence":["go test"],"reviewed_n":%d}`, auditN)
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(pass), &bytes.Buffer{}, &bytes.Buffer{})

	wl := `{"ticket":"WL-OK","task":"shipped","scope":"repo","result":"deployed","paths":[],"commands":["go test"]}`
	var stderr bytes.Buffer
	if code := RunWorklogCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(wl), &bytes.Buffer{}, &stderr); code != 0 {
		t.Fatalf("expected accept after audit pass, stderr=%s", stderr.String())
	}
	wlRows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "worklog.jsonl"))
	if len(wlRows) != 1 {
		t.Fatalf("expected 1 worklog row, got %d", len(wlRows))
	}
}

func TestWorklogAdd_RejectsWeakDone(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	// "Weak done" = status=done not via audit. Under the new validator we cannot
	// reach this state via ticket event from impl. We seed via a correction row
	// (invalidates_n bypasses lifecycle), simulating historical/imported data.
	add := `{"ticket":"WL-WEAK","parent_ticket":"BUG","role":"impl","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	weak := `{"ticket":"WL-WEAK","status":"done","role":"impl","task":"forced weak done","invalidates_n":999}`
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(weak), &bytes.Buffer{}, &bytes.Buffer{})

	wl := `{"ticket":"WL-WEAK","task":"shipped","scope":"repo","result":"forced","paths":[],"commands":[]}`
	var stderr bytes.Buffer
	code := RunWorklogCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(wl), &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected rejection: weak done (no audit_result=pass) must not gate-open worklog")
	}
	if !strings.Contains(stderr.String(), "audit") {
		t.Fatalf("stderr should mention audit requirement, got: %s", stderr.String())
	}
}
