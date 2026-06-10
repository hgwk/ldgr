package cmd

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hgwk/ldgr/internal/ledger"
)

func TestSuggestCommit_ConventionalLineFromCategory(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"ticket":"BUG-9","parent_ticket":"BUG","role":"impl","status":"open","task":"fix the thing","scope":"repo","paths":["src/x.go"],"blocked_by":[],"category":"bug"}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})

	var out bytes.Buffer
	if code := RunSuggestCLI([]string{"commit", "--target", target, "--ticket", "BUG-9", "--allow-unaudited"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("suggest commit failed")
	}
	line := strings.SplitN(out.String(), "\n", 2)[0]
	if !strings.HasPrefix(line, "fix(bug): ") {
		t.Fatalf("expected fix(bug): prefix, got: %s", line)
	}
	if !strings.Contains(out.String(), "## Verification") {
		t.Fatalf("expected verification block, got:\n%s", out.String())
	}
}

func TestSuggestCommit_RefusesScaffoldBeforeAuditPass(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"ticket":"SC-1","parent_ticket":"BUG","role":"impl","status":"open","task":"fix","scope":"repo","paths":["src/x.go"],"blocked_by":[],"category":"bug"}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})

	var out bytes.Buffer
	if code := RunSuggestCLI([]string{"commit", "--target", target, "--ticket", "SC-1"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("suggest commit should warn (exit 0), got %d", code)
	}
	// Should NOT contain the Conventional Commit scaffold.
	if strings.Contains(out.String(), "## Verification") {
		t.Fatalf("scaffold should not be emitted before audit pass: %s", out.String())
	}
	// Should mention audit + --allow-unaudited.
	if !strings.Contains(out.String(), "audit") {
		t.Fatalf("output should mention audit gate: %s", out.String())
	}
	if !strings.Contains(out.String(), "--allow-unaudited") {
		t.Fatalf("output should mention --allow-unaudited override: %s", out.String())
	}
}

func TestSuggestCommit_AllowsScaffoldWithOverride(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"ticket":"SC-2","parent_ticket":"BUG","role":"impl","status":"open","task":"fix","scope":"repo","paths":["src/x.go"],"blocked_by":[],"category":"bug"}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})

	var out bytes.Buffer
	if code := RunSuggestCLI([]string{"commit", "--target", target, "--ticket", "SC-2", "--allow-unaudited"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("suggest commit --allow-unaudited failed")
	}
	line := strings.SplitN(out.String(), "\n", 2)[0]
	if !strings.HasPrefix(line, "fix(bug): ") {
		t.Fatalf("expected fix(bug): prefix, got: %s", line)
	}
	if !strings.Contains(out.String(), "## Verification") {
		t.Fatalf("expected verification block, got: %s", out.String())
	}
}

func TestSuggestCommit_AfterAuditPassNoOverrideNeeded(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	// Drive to audit pass.
	add := `{"ticket":"SC-3","parent_ticket":"BUG","role":"impl","status":"open","task":"fix","scope":"repo","paths":["src/x.go"],"blocked_by":[],"category":"bug"}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"ticket":"SC-3","status":"in_progress"}`), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"ticket":"SC-3","status":"audit_ready","evidence":["go test"]}`), &bytes.Buffer{}, &bytes.Buffer{})
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	auditN := int(rows[len(rows)-1]["n"].(float64))
	pass := fmt.Sprintf(`{"ticket":"SC-3","role":"audit","status":"done","audit_result":"pass","evidence":["go test"],"reviewed_n":%d}`, auditN)
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(pass), &bytes.Buffer{}, &bytes.Buffer{})

	var out bytes.Buffer
	if code := RunSuggestCLI([]string{"commit", "--target", target, "--ticket", "SC-3"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("audit-pass commit should succeed")
	}
	if !strings.Contains(out.String(), "## Verification") {
		t.Fatalf("expected scaffold after audit pass: %s", out.String())
	}
}
func TestSuggestCommitState_RefusesBeforeAuditPass(t *testing.T) {
	target := mustInitState(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"id":"SC-STATE-WAIT","parent":"ROOT","type":"task","state":"ready","area":"backend","priority":"P1","title":"x","blocked_by":[],"acceptance":[],"evidence":[],"event":{"role":"planner","summary":"opened","notes":""}}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	var out bytes.Buffer
	if code := RunSuggestCLI([]string{"commit", "--target", target, "--ticket", "SC-STATE-WAIT"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("suggest state-model commit guidance failed")
	}
	if strings.Contains(out.String(), "## Verification") || !strings.Contains(out.String(), "--allow-unaudited") {
		t.Fatalf("expected state-model guidance refusal, got: %s", out.String())
	}
}
