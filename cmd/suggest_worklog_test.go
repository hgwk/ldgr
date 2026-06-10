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

func TestSuggestWorklogState_EmitsStateSkeletonAfterAuditPass(t *testing.T) {
	target := mustInitState(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"id":"SW-STATE","parent":"ROOT","type":"task","state":"ready","area":"backend","priority":"P1","title":"build state-model","blocked_by":[],"acceptance":["verify"],"evidence":[],"event":{"role":"planner","summary":"opened","notes":""}}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"id":"SW-STATE","state":"doing","event":{"role":"implementer","summary":"started","notes":""}}`), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"id":"SW-STATE","state":"review","evidence":["go test"],"event":{"role":"implementer","summary":"ready","notes":""}}`), &bytes.Buffer{}, &bytes.Buffer{})
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	reviewN := int(rows[len(rows)-1]["n"].(float64))
	pass := fmt.Sprintf(`{"id":"SW-STATE","state":"done","evidence":["go test"],"event":{"role":"auditor","result":"pass","reviewed_n":%d,"summary":"passed","notes":""}}`, reviewN)
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(pass), &bytes.Buffer{}, &bytes.Buffer{})

	var out bytes.Buffer
	if code := RunSuggestCLI([]string{"worklog", "--target", target, "--ticket", "SW-STATE"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("suggest state-model worklog failed")
	}
	var skel map[string]any
	if err := json.Unmarshal(out.Bytes(), &skel); err != nil {
		t.Fatalf("expected JSON skeleton, got %s: %v", out.String(), err)
	}
	if skel["ticket"] != "SW-STATE" || skel["title"] != "build state-model" || skel["summary"] == "" {
		t.Fatalf("wrong state-model skeleton: %+v", skel)
	}
	if _, ok := skel["task"]; ok {
		t.Fatalf("state-model skeleton should not include v1 task: %+v", skel)
	}
	if _, ok := skel["result"]; ok {
		t.Fatalf("state-model skeleton should not include v1 result: %+v", skel)
	}
}

func TestSuggestWorklogState_BeforeAuditPassPrintsStateGuidance(t *testing.T) {
	target := mustInitState(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"id":"SW-STATE-WAIT","parent":"ROOT","type":"task","state":"ready","area":"backend","priority":"P1","title":"build state-model","blocked_by":[],"acceptance":[],"evidence":[],"event":{"role":"planner","summary":"opened","notes":""}}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"id":"SW-STATE-WAIT","state":"doing","event":{"role":"implementer","summary":"started","notes":""}}`), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"id":"SW-STATE-WAIT","state":"review","evidence":["go test"],"event":{"role":"implementer","summary":"ready","notes":""}}`), &bytes.Buffer{}, &bytes.Buffer{})
	var out bytes.Buffer
	if code := RunSuggestCLI([]string{"worklog", "--target", target, "--ticket", "SW-STATE-WAIT"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("suggest state-model worklog guidance failed")
	}
	if strings.Contains(out.String(), `"task"`) || !strings.Contains(out.String(), "awaiting audit") {
		t.Fatalf("expected state-model guidance before audit pass, got: %s", out.String())
	}
}
