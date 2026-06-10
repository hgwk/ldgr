package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hgwk/ldgr/internal/ledger"
)

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

func TestTicketReadyState_AppendsReviewRow(t *testing.T) {
	target := mustInitState(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"id":"RDY-STATE","parent":"ROOT","type":"task","state":"ready","area":"backend","priority":"P1","title":"x","blocked_by":[],"acceptance":["verify"],"evidence":[],"event":{"role":"planner","summary":"opened","notes":""}}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"id":"RDY-STATE","state":"doing","event":{"role":"implementer","summary":"started","notes":""}}`), &bytes.Buffer{}, &bytes.Buffer{})

	var stderr bytes.Buffer
	if code := RunTicketCLI([]string{"ready", "--target", target, "--ticket", "RDY-STATE", "--evidence", "go test ./..."}, &bytes.Buffer{}, &bytes.Buffer{}, &stderr); code != 0 {
		t.Fatalf("state-model ticket ready failed: %s", stderr.String())
	}
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	last := rows[len(rows)-1]
	if last["state"] != "review" {
		t.Fatalf("expected state=review, got %+v", last)
	}
	event, _ := last["event"].(map[string]any)
	if event["role"] != "implementer" {
		t.Fatalf("expected implementer event, got %+v", event)
	}
	if _, ok := last["status"]; ok {
		t.Fatalf("state-model ready row should not include v1 status: %+v", last)
	}
}
