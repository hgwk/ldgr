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

func TestNext_IncludesWritingLanguage(t *testing.T) {
	target, store := mustInit(t)
	if err := RunInit(target, InitOpts{Slug: "myapp", WritingLanguage: "ko"}, store); err != nil {
		t.Fatalf("set language: %v", err)
	}
	t.Setenv("LEDGER_AGENT", "codex")
	body := `{"ticket":"WL-1","parent_ticket":"BUG","role":"impl","status":"open","task":"작업","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(body), &bytes.Buffer{}, &bytes.Buffer{})

	var text bytes.Buffer
	if code := RunNextCLI([]string{"--target", target, "--ticket", "WL-1"}, &text, &bytes.Buffer{}); code != 0 {
		t.Fatalf("next text failed")
	}
	if !strings.Contains(text.String(), "Writing language: ko") {
		t.Fatalf("missing text language hint: %s", text.String())
	}

	var out bytes.Buffer
	if code := RunNextCLI([]string{"--target", target, "--ticket", "WL-1", "--format", "json"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("next json failed")
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got["writing_language"] != "ko" {
		t.Fatalf("missing json writing_language: %+v", got)
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

func TestNextState_TicketScopedTextAndJSON(t *testing.T) {
	target := mustInitState(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"id":"NEXT-STATE","parent":"ROOT","type":"task","state":"ready","area":"backend","priority":"P1","title":"build","blocked_by":[],"acceptance":["verify"],"evidence":[],"event":{"role":"planner","summary":"opened","notes":""}}`
	if code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("seed state-model add failed")
	}
	if code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"id":"NEXT-STATE","state":"doing","event":{"role":"implementer","summary":"started","notes":""}}`), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("seed state-model event failed")
	}

	var text bytes.Buffer
	if code := RunNextCLI([]string{"--target", target, "--ticket", "NEXT-STATE"}, &text, &bytes.Buffer{}); code != 0 {
		t.Fatalf("next state-model text failed")
	}
	if !strings.Contains(text.String(), "NEXT-STATE is doing") || !strings.Contains(text.String(), "review") {
		t.Fatalf("unexpected state-model text guidance: %s", text.String())
	}

	var out bytes.Buffer
	if code := RunNextCLI([]string{"--target", target, "--ticket", "NEXT-STATE", "--format", "json"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("next state-model json failed")
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if got["id"] != "NEXT-STATE" || got["state"] != "doing" {
		t.Fatalf("state-model json should use id/state: %+v", got)
	}
	if _, ok := got["ticket"]; ok {
		t.Fatalf("state-model json should not include v1 ticket field: %+v", got)
	}
	if _, ok := got["status"]; ok {
		t.Fatalf("state-model json should not include v1 status field: %+v", got)
	}
}

func TestNextState_ProjectQueueUsesStateVocabulary(t *testing.T) {
	target := mustInitState(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"id":"PRJ-STATE","parent":"ROOT","type":"task","state":"ready","area":"backend","priority":"P0","title":"build","blocked_by":[],"acceptance":["verify"],"evidence":[],"event":{"role":"planner","summary":"opened","notes":""}}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})

	var out bytes.Buffer
	if code := RunNextCLI([]string{"--target", target, "--format", "json"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("next state-model project json failed")
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	items, _ := got["highlights"].([]any)
	if len(items) == 0 {
		t.Fatalf("expected state-model project highlight: %+v", got)
	}
	first, _ := items[0].(map[string]any)
	if first["id"] != "PRJ-STATE" || first["state"] != "ready" {
		t.Fatalf("expected id/state in state-model queue item: %+v", first)
	}
	if _, ok := first["ticket"]; ok {
		t.Fatalf("state-model queue item should not include ticket: %+v", first)
	}
	if _, ok := first["status"]; ok {
		t.Fatalf("state-model queue item should not include status: %+v", first)
	}
}

func TestNextState_ProjectQueueFiltersByTeam(t *testing.T) {
	target := mustInitState(t)
	t.Setenv("LEDGER_AGENT", "codex")
	platform := `{"id":"TEAM-PLATFORM","parent":"ROOT","type":"task","state":"ready","area":"backend","priority":"P0","title":"platform work","team":"platform","blocked_by":[],"acceptance":[],"evidence":[],"event":{"role":"planner","summary":"opened","notes":""}}`
	growth := `{"id":"TEAM-GROWTH","parent":"ROOT","type":"task","state":"ready","area":"frontend","priority":"P0","title":"growth work","team":"growth","blocked_by":[],"acceptance":[],"evidence":[],"event":{"role":"planner","summary":"opened","notes":""}}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(platform), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(growth), &bytes.Buffer{}, &bytes.Buffer{})

	var out bytes.Buffer
	if code := RunNextCLI([]string{"--target", target, "--team", "platform", "--format", "json"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("next state-model project team json failed")
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if got["team"] != "platform" {
		t.Fatalf("team not echoed: %+v", got)
	}
	items, _ := got["highlights"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one team highlight: %+v", got)
	}
	first, _ := items[0].(map[string]any)
	if first["id"] != "TEAM-PLATFORM" {
		t.Fatalf("wrong team item: %+v", first)
	}
	counts, _ := got["counts"].(map[string]any)
	if counts["active"] != float64(1) {
		t.Fatalf("counts should be team-scoped: %+v", counts)
	}
}
