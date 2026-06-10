package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func seedStateTicket(t *testing.T, target, ticket string) {
	t.Helper()
	t.Setenv("LEDGER_AGENT", "codex")
	add := fmt.Sprintf(`{"id":%q,"parent":"ROOT","type":"task","state":"ready","area":"ops","priority":"P2","title":"seed","blocked_by":[],"acceptance":[],"evidence":[],"event":{"role":"planner","summary":"seed","notes":""}}`, ticket)
	if code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("seed state ticket failed")
	}
}

func TestSuggestCommitAndPRState_AfterAuditPass(t *testing.T) {
	target := mustInitState(t)
	t.Setenv("LEDGER_AGENT", "codex")
	driveStateToReview(t, target, "CPR-STATE")
	if code := RunAuditCLI([]string{"pass", "--target", target, "--ticket", "CPR-STATE", "--evidence", "go test"}, &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("state-model audit pass failed")
	}

	var commit bytes.Buffer
	if code := RunSuggestCLI([]string{"commit", "--target", target, "--ticket", "CPR-STATE"}, &commit, &bytes.Buffer{}); code != 0 {
		t.Fatalf("suggest state-model commit failed")
	}
	if !strings.Contains(commit.String(), "feat: x") || !strings.Contains(commit.String(), "## Verification") {
		t.Fatalf("unexpected state-model commit scaffold: %s", commit.String())
	}

	var pr bytes.Buffer
	if code := RunSuggestCLI([]string{"pr", "--target", target, "--ticket", "CPR-STATE"}, &pr, &bytes.Buffer{}); code != 0 {
		t.Fatalf("suggest state-model pr failed")
	}
	if !strings.Contains(pr.String(), "# PR: CPR-STATE x") || !strings.Contains(pr.String(), "event.result=pass") {
		t.Fatalf("unexpected state-model pr scaffold: %s", pr.String())
	}
}
