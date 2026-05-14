package cmd

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hgwk/ldgr/internal/ledger"
)

func driveToAuditReady(t *testing.T, target, ticket string) int {
	t.Helper()
	add := fmt.Sprintf(`{"ticket":%q,"parent_ticket":"BUG","role":"impl","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[]}`, ticket)
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(fmt.Sprintf(`{"ticket":%q,"status":"in_progress"}`, ticket)), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(fmt.Sprintf(`{"ticket":%q,"status":"audit_ready","evidence":["go test"]}`, ticket)), &bytes.Buffer{}, &bytes.Buffer{})
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	return int(rows[len(rows)-1]["n"].(float64))
}

func TestAuditPass_AutoSetsReviewedN(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	n := driveToAuditReady(t, target, "AUP-1")
	var stderr bytes.Buffer
	if code := RunAuditCLI([]string{"pass", "--target", target, "--ticket", "AUP-1", "--evidence", "go test"}, &bytes.Buffer{}, &bytes.Buffer{}, &stderr); code != 0 {
		t.Fatalf("audit pass failed: %s", stderr.String())
	}
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	last := rows[len(rows)-1]
	if last["status"] != "done" || last["audit_result"] != "pass" || last["role"] != "audit" {
		t.Fatalf("audit pass row malformed: %+v", last)
	}
	if rn, _ := last["reviewed_n"].(float64); int(rn) != n {
		t.Fatalf("reviewed_n should be %d (the audit_ready row), got %v", n, last["reviewed_n"])
	}
}

func TestAuditPass_FailsWithoutPriorAuditReady(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"ticket":"AUP-N","parent_ticket":"BUG","role":"impl","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	var stderr bytes.Buffer
	code := RunAuditCLI([]string{"pass", "--target", target, "--ticket", "AUP-N", "--evidence", "go test"}, &bytes.Buffer{}, &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected failure without prior audit_ready")
	}
	if !strings.Contains(stderr.String(), "audit_ready") {
		t.Fatalf("error should mention audit_ready, got: %s", stderr.String())
	}
}

func TestAuditRequestChanges_AppendsCorrectRow(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	n := driveToAuditReady(t, target, "ACR-1")
	var stderr bytes.Buffer
	if code := RunAuditCLI([]string{"request-changes", "--target", target, "--ticket", "ACR-1", "--notes", "missing regression tests"}, &bytes.Buffer{}, &bytes.Buffer{}, &stderr); code != 0 {
		t.Fatalf("audit request-changes failed: %s", stderr.String())
	}
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	last := rows[len(rows)-1]
	if last["status"] != "changes_requested" || last["audit_result"] != "changes_requested" || last["role"] != "audit" {
		t.Fatalf("changes_requested row malformed: %+v", last)
	}
	if last["audit_notes"] != "missing regression tests" {
		t.Fatalf("audit_notes wrong: %v", last["audit_notes"])
	}
	if rn, _ := last["reviewed_n"].(float64); int(rn) != n {
		t.Fatalf("reviewed_n should be %d, got %v", n, last["reviewed_n"])
	}
}
