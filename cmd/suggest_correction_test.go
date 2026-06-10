package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestSuggestCorrection_EmitsOpsCancellation(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"ticket":"CR-1","parent_ticket":"BUG","role":"impl","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})

	var out bytes.Buffer
	if code := RunSuggestCLI([]string{"correction", "--target", target, "--ticket", "CR-1", "--invalidates-n", "1", "--notes", "ghost row"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("suggest correction failed")
	}
	var skel map[string]any
	if err := json.Unmarshal(out.Bytes(), &skel); err != nil {
		t.Fatalf("expected JSON: %v\n%s", err, out.String())
	}
	if skel["role"] != "ops" || skel["status"] != "cancelled" {
		t.Fatalf("unexpected skeleton: %+v", skel)
	}
	if n, _ := skel["invalidates_n"].(float64); int(n) != 1 {
		t.Fatalf("invalidates_n wrong: %v", skel["invalidates_n"])
	}
}
func TestSuggestCorrectionState_EmitsDroppedEvent(t *testing.T) {
	target := mustInitState(t)
	seedStateTicket(t, target, "SEED-STATE")
	var out bytes.Buffer
	if code := RunSuggestCLI([]string{"correction", "--target", target, "--ticket", "CORR-STATE", "--invalidates-n", "3", "--notes", "bad row"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("suggest state-model correction failed: %s", out.String())
	}
	var skel map[string]any
	if err := json.Unmarshal(out.Bytes(), &skel); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if skel["id"] != "CORR-STATE" || skel["state"] != "dropped" {
		t.Fatalf("state-model correction skeleton wrong: %+v", skel)
	}
	event, _ := skel["event"].(map[string]any)
	if event["role"] != "operator" || event["result"] != "corrected" {
		t.Fatalf("state-model correction event wrong: %+v", event)
	}
}
