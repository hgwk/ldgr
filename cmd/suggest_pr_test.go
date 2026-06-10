package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestSuggestPR_RefusesBeforeAuditPass(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"ticket":"PR-1","parent_ticket":"BUG","role":"impl","status":"open","task":"fix","scope":"repo","paths":[],"blocked_by":[],"category":"bug"}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})

	var out bytes.Buffer
	if code := RunSuggestCLI([]string{"pr", "--target", target, "--ticket", "PR-1"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("suggest pr should warn (exit 0)")
	}
	if strings.Contains(out.String(), "## Verification") && !strings.Contains(out.String(), "--allow-unaudited") {
		t.Fatalf("PR scaffold should not appear before audit; got: %s", out.String())
	}
}

func TestSuggestPR_AllowsScaffoldWithOverride(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"ticket":"PR-2","parent_ticket":"BUG","role":"impl","status":"open","task":"fix the thing","scope":"repo","paths":[],"blocked_by":[],"category":"bug"}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})

	var out bytes.Buffer
	if code := RunSuggestCLI([]string{"pr", "--target", target, "--ticket", "PR-2", "--allow-unaudited"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("suggest pr --allow-unaudited failed")
	}
	if !strings.Contains(out.String(), "# PR:") || !strings.Contains(out.String(), "## Verification") {
		t.Fatalf("expected PR scaffold, got: %s", out.String())
	}
}
