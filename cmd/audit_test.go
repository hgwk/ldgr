package cmd

import (
	"bytes"
	"fmt"
	"os"
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

func setGitEvidencePolicy(t *testing.T, target, policy string) {
	t.Helper()
	path := filepath.Join(target, "ledger", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	body := strings.TrimSpace(string(data))
	body = strings.TrimSuffix(body, "}")
	body += fmt.Sprintf(",\n  \"git_evidence\": %q\n}\n", policy)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
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

func TestAuditPass_GitEvidenceFailRejectsDoneWithoutCommitEvidence(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	setGitEvidencePolicy(t, target, "fail")
	driveToAuditReady(t, target, "AUP-GIT")
	var stderr bytes.Buffer
	code := RunAuditCLI([]string{"pass", "--target", target, "--ticket", "AUP-GIT", "--evidence", "go test"}, &bytes.Buffer{}, &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected git evidence gate to reject done")
	}
	if !strings.Contains(stderr.String(), "commit:<sha>") {
		t.Fatalf("stderr should explain git evidence requirement, got: %s", stderr.String())
	}
}

func TestAuditPass_GitEvidenceFailAcceptsCommitEvidence(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	setGitEvidencePolicy(t, target, "fail")
	driveToAuditReady(t, target, "AUP-GIT-OK")
	var stderr bytes.Buffer
	code := RunAuditCLI([]string{"pass", "--target", target, "--ticket", "AUP-GIT-OK", "--evidence", "go test", "--evidence", "commit:abc1234"}, &bytes.Buffer{}, &bytes.Buffer{}, &stderr)
	if code != 0 {
		t.Fatalf("expected commit evidence to pass: %s", stderr.String())
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

func driveStateToReview(t *testing.T, target, ticket string) int {
	t.Helper()
	add := fmt.Sprintf(`{"id":%q,"parent":"ROOT","type":"task","state":"ready","area":"backend","priority":"P1","title":"x","blocked_by":[],"acceptance":["verify"],"evidence":[],"event":{"role":"planner","summary":"opened","notes":""}}`, ticket)
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(fmt.Sprintf(`{"id":%q,"state":"doing","event":{"role":"implementer","summary":"started","notes":""}}`, ticket)), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(fmt.Sprintf(`{"id":%q,"state":"review","evidence":["go test"],"event":{"role":"implementer","summary":"ready","notes":""}}`, ticket)), &bytes.Buffer{}, &bytes.Buffer{})
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	return int(rows[len(rows)-1]["n"].(float64))
}

func TestAuditPassState_AutoSetsReviewedN(t *testing.T) {
	target := mustInitState(t)
	t.Setenv("LEDGER_AGENT", "codex")
	n := driveStateToReview(t, target, "AUP-STATE")
	var stderr bytes.Buffer
	if code := RunAuditCLI([]string{"pass", "--target", target, "--ticket", "AUP-STATE", "--evidence", "go test"}, &bytes.Buffer{}, &bytes.Buffer{}, &stderr); code != 0 {
		t.Fatalf("state-model audit pass failed: %s", stderr.String())
	}
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	last := rows[len(rows)-1]
	if last["state"] != "done" {
		t.Fatalf("state-model audit pass state wrong: %+v", last)
	}
	event, _ := last["event"].(map[string]any)
	if event["role"] != "auditor" || event["result"] != "pass" {
		t.Fatalf("state-model audit event malformed: %+v", event)
	}
	if rn, _ := event["reviewed_n"].(float64); int(rn) != n {
		t.Fatalf("reviewed_n should be %d, got %v", n, event["reviewed_n"])
	}
}

func TestAuditRequestChangesState_AppendsRework(t *testing.T) {
	target := mustInitState(t)
	t.Setenv("LEDGER_AGENT", "codex")
	n := driveStateToReview(t, target, "ACR-STATE")
	var stderr bytes.Buffer
	if code := RunAuditCLI([]string{"request-changes", "--target", target, "--ticket", "ACR-STATE", "--notes", "missing regression tests"}, &bytes.Buffer{}, &bytes.Buffer{}, &stderr); code != 0 {
		t.Fatalf("state-model request-changes failed: %s", stderr.String())
	}
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	last := rows[len(rows)-1]
	if last["state"] != "rework" {
		t.Fatalf("expected rework, got %+v", last)
	}
	event, _ := last["event"].(map[string]any)
	if event["result"] != "changes_requested" || event["notes"] != "missing regression tests" {
		t.Fatalf("state-model request changes event malformed: %+v", event)
	}
	if rn, _ := event["reviewed_n"].(float64); int(rn) != n {
		t.Fatalf("reviewed_n should be %d, got %v", n, event["reviewed_n"])
	}
}
