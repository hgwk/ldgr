package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestSuggestAudit_OnAuditReadyEmitsSkeletons(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"ticket":"AU-1","parent_ticket":"BUG","role":"impl","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"ticket":"AU-1","status":"in_progress"}`), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"ticket":"AU-1","status":"audit_ready","evidence":["go test"]}`), &bytes.Buffer{}, &bytes.Buffer{})

	var out bytes.Buffer
	if code := RunSuggestCLI([]string{"audit", "--target", target, "--ticket", "AU-1"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("suggest audit failed")
	}
	var arr []map[string]any
	if err := json.Unmarshal(out.Bytes(), &arr); err != nil {
		t.Fatalf("expected JSON array: %v\n%s", err, out.String())
	}
	if len(arr) != 2 {
		t.Fatalf("expected 2 skeletons, got %d", len(arr))
	}
	if arr[0]["audit_result"] != "pass" || arr[1]["audit_result"] != "changes_requested" {
		t.Fatalf("skeleton order wrong: %+v", arr)
	}
}

func TestSuggestAudit_OnNonAuditReadyPrintsGuidance(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"ticket":"AU-2","parent_ticket":"BUG","role":"impl","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})

	var out bytes.Buffer
	if code := RunSuggestCLI([]string{"audit", "--target", target, "--ticket", "AU-2"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("suggest audit should warn (exit 0)")
	}
	// Should not contain a JSON skeleton.
	if strings.HasPrefix(strings.TrimSpace(out.String()), "[") {
		t.Fatalf("expected guidance text, got JSON: %s", out.String())
	}
}

func TestSuggestAuditState_OnReviewEmitsSkeletons(t *testing.T) {
	target := mustInitState(t)
	t.Setenv("LEDGER_AGENT", "codex")
	reviewN := driveStateToReview(t, target, "AU-STATE")
	var out bytes.Buffer
	if code := RunSuggestCLI([]string{"audit", "--target", target, "--ticket", "AU-STATE"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("suggest state-model audit failed")
	}
	var arr []map[string]any
	if err := json.Unmarshal(out.Bytes(), &arr); err != nil {
		t.Fatalf("expected JSON array: %v\n%s", err, out.String())
	}
	if len(arr) != 2 {
		t.Fatalf("expected 2 skeletons, got %d", len(arr))
	}
	if arr[0]["id"] != "AU-STATE" || arr[0]["state"] != "done" || arr[1]["state"] != "rework" {
		t.Fatalf("unexpected state-model audit skeletons: %+v", arr)
	}
	event, _ := arr[0]["event"].(map[string]any)
	if rn, _ := event["reviewed_n"].(float64); int(rn) != reviewN {
		t.Fatalf("reviewed_n should be %d, got %+v", reviewN, event)
	}
	if _, ok := arr[0]["ticket"]; ok {
		t.Fatalf("state-model audit skeleton should not include v1 ticket: %+v", arr[0])
	}
}

func TestSuggestAuditState_OnNonReviewPrintsGuidance(t *testing.T) {
	target := mustInitState(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"id":"AU-STATE-WAIT","parent":"ROOT","type":"task","state":"ready","area":"backend","priority":"P1","title":"x","blocked_by":[],"acceptance":[],"evidence":[],"event":{"role":"planner","summary":"opened","notes":""}}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	var out bytes.Buffer
	if code := RunSuggestCLI([]string{"audit", "--target", target, "--ticket", "AU-STATE-WAIT"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("suggest state-model audit guidance failed")
	}
	if strings.HasPrefix(strings.TrimSpace(out.String()), "[") || !strings.Contains(out.String(), "review") {
		t.Fatalf("expected state-model review guidance, got: %s", out.String())
	}
}
