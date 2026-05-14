package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hgwk/ldgr/internal/ledger"
)

func TestWorklogAdd_AppendsRow(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")

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
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "worklog.jsonl"))
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
}

func TestWorklogAdd_TicketOptional(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")

	in := map[string]any{
		"task":     "goal change",
		"scope":    "ledger",
		"result":   "Updated goal.",
		"paths":    []any{"ledger/goal.json"},
		"commands": []any{},
	}
	body, _ := json.Marshal(in)
	errb := &bytes.Buffer{}
	if code := RunWorklogCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(body), &bytes.Buffer{}, errb); code != 0 {
		t.Fatalf("add without ticket should be accepted: %s", errb.String())
	}
}

func TestWorklogAdd_WarnsWhenTicketNotAuditPass(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"ticket":"W-1","parent_ticket":"BUG","role":"impl","status":"open","task":"do","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	// Transition to in_progress
	inp := `{"ticket":"W-1","status":"in_progress"}`
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(inp), &bytes.Buffer{}, &bytes.Buffer{})
	// Transition to audit_ready so worklog guidance will mention audit
	evch := `{"ticket":"W-1","status":"audit_ready","evidence":["done"]}`
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(evch), &bytes.Buffer{}, &bytes.Buffer{})
	wl := `{"ticket":"W-1","task":"early worklog","scope":"repo","result":"too early","paths":[],"commands":[]}`
	var stderr bytes.Buffer
	if code := RunWorklogCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(wl), &bytes.Buffer{}, &stderr); code != 0 {
		t.Fatalf("worklog add failed: %s", stderr.String())
	}
	// Worklog still appends — stderr must warn ticket is not audit-pass done (should mention audit).
	if !strings.Contains(stderr.String(), "audit") {
		t.Fatalf("expected audit-related stderr guidance: %s", stderr.String())
	}
}
