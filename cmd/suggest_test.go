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

func TestSuggestWorklog_RefusesBeforeAuditPass(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	body := `{"ticket":"T-1","parent_ticket":"BUG","role":"impl","status":"open","task":"do","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(body), &bytes.Buffer{}, &bytes.Buffer{})

	var out bytes.Buffer
	code := RunSuggestCLI([]string{"worklog", "--target", target, "--ticket", "T-1"}, &out, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("suggest worklog should warn (exit 0), got %d", code)
	}
	if strings.Contains(out.String(), `"result"`) {
		t.Fatalf("should not print a worklog skeleton yet: %s", out.String())
	}
	if !strings.Contains(out.String(), "Claim") && !strings.Contains(out.String(), "Next:") {
		t.Fatalf("expected guidance with claim/next steps: %s", out.String())
	}
}

func TestSuggestWorklog_EmitsSkeletonAfterAuditPass(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	// Add: status=open
	add := `{"ticket":"T-2","parent_ticket":"BUG","role":"impl","status":"open","task":"impl T-2","scope":"repo","paths":["src/x.go"],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})

	// Transition to in_progress
	inp := `{"ticket":"T-2","status":"in_progress"}`
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(inp), &bytes.Buffer{}, &bytes.Buffer{})

	// Transition to audit_ready with evidence
	ready := `{"ticket":"T-2","status":"audit_ready","evidence":["go test ./..."]}`
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(ready), &bytes.Buffer{}, &bytes.Buffer{})

	// Look up the audit_ready row n so we can pass reviewed_n
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	auditN := int(rows[len(rows)-1]["n"].(float64))

	// Audit-pass close
	pass := fmt.Sprintf(`{"ticket":"T-2","role":"audit","status":"done","audit_result":"pass","evidence":["go test ./..."],"reviewed_n":%d}`, auditN)
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(pass), &bytes.Buffer{}, &bytes.Buffer{})

	var out bytes.Buffer
	if code := RunSuggestCLI([]string{"worklog", "--target", target, "--ticket", "T-2"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("suggest worklog failed")
	}
	var skel map[string]any
	if err := json.Unmarshal(out.Bytes(), &skel); err != nil {
		t.Fatalf("expected JSON skeleton, got: %s\nerr=%v", out.String(), err)
	}
	if skel["ticket"] != "T-2" || skel["task"] == "" || skel["scope"] == "" {
		t.Fatalf("skeleton fields wrong: %+v", skel)
	}
}

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
