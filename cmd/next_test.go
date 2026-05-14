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

func TestNext_WithoutTicketReturnsProjectQueue(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	body := `{"ticket":"PRJ-1","parent_ticket":"BUG","role":"impl","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[],"priority":"P0"}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(body), &bytes.Buffer{}, &bytes.Buffer{})

	var out bytes.Buffer
	if code := RunNextCLI([]string{"--target", target}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("project next failed")
	}
	if !strings.Contains(out.String(), "Project queue") {
		t.Fatalf("project queue header missing: %s", out.String())
	}
}

func TestNext_JSONProjectMode(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	body := `{"ticket":"PRJ-2","parent_ticket":"BUG","role":"impl","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[],"priority":"P0"}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(body), &bytes.Buffer{}, &bytes.Buffer{})

	var out bytes.Buffer
	if code := RunNextCLI([]string{"--target", target, "--format", "json"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("project next json failed")
	}
	var resp map[string]any
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if _, ok := resp["highlights"]; !ok {
		t.Fatalf("missing highlights: %v", resp)
	}
}

func TestNext_RoleFiltersProjectQueue(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	body := `{"ticket":"PRJ-3","parent_ticket":"BUG","role":"impl","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[],"priority":"P0"}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(body), &bytes.Buffer{}, &bytes.Buffer{})

	var out bytes.Buffer
	if code := RunNextCLI([]string{"--target", target, "--role", "auditor", "--format", "json"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("project next role failed")
	}
	var resp map[string]any
	json.Unmarshal(out.Bytes(), &resp)
	if resp["role"] != "auditor" {
		t.Fatalf("role not echoed: %v", resp)
	}
}

func TestNext_RejectsBadRole(t *testing.T) {
	target, _ := mustInit(t)
	var errb bytes.Buffer
	code := RunNextCLI([]string{"--target", target, "--role", "weirdo"}, &bytes.Buffer{}, &errb)
	if code == 0 {
		t.Fatalf("expected non-zero for bad role")
	}
}

func TestNext_GitFlagInNonGitDirIsHarmless(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"ticket":"G-1","parent_ticket":"BUG","role":"impl","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})

	var out bytes.Buffer
	if code := RunNextCLI([]string{"--target", target, "--ticket", "G-1", "--git"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("--git in non-git target should still succeed")
	}
}

func TestNext_GitFlagWithJSON(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	body := `{"ticket":"GJ-1","parent_ticket":"BUG","role":"impl","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(body), &bytes.Buffer{}, &bytes.Buffer{})

	var out bytes.Buffer
	if code := RunNextCLI([]string{"--target", target, "--ticket", "GJ-1", "--git", "--format", "json"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("--git --format json failed")
	}
	var resp map[string]any
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("not json: %v\n%s", err, out.String())
	}
	// git key may or may not be present when --git is used in non-git dir, that's OK
	// We just want no error and valid JSON.
}
