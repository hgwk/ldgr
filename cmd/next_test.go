package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNext_TextOutputForOpenTicket(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")

	body := `{"ticket":"T-1","parent_ticket":"BUG","role":"impl","status":"open","task":"do","scope":"repo","paths":[],"blocked_by":[]}`
	if code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(body), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("seed add failed")
	}

	var out bytes.Buffer
	if code := RunNextCLI([]string{"--target", target, "--ticket", "T-1"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("next failed")
	}
	if !strings.Contains(out.String(), "T-1 is open") {
		t.Fatalf("missing header line: %s", out.String())
	}
	if !strings.Contains(out.String(), "Next:") {
		t.Fatalf("missing Next section: %s", out.String())
	}
}

func TestNext_JSONOutputShape(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	body := `{"ticket":"T-1","parent_ticket":"BUG","role":"impl","status":"open","task":"do","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(body), &bytes.Buffer{}, &bytes.Buffer{})

	var out bytes.Buffer
	if code := RunNextCLI([]string{"--target", target, "--ticket", "T-1", "--format", "json"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("next json failed")
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if got["ticket"] != "T-1" || got["status"] != "open" {
		t.Fatalf("unexpected payload: %v", got)
	}
}

func TestNext_MissingTicketFails(t *testing.T) {
	target, _ := mustInit(t)
	var errb bytes.Buffer
	if code := RunNextCLI([]string{"--target", target, "--ticket", "ghost"}, &bytes.Buffer{}, &errb); code == 0 {
		t.Fatalf("expected non-zero for missing ticket")
	}
	if !strings.Contains(errb.String(), "ghost") {
		t.Fatalf("stderr should name the ticket: %s", errb.String())
	}
}
