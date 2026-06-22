package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNext_IncludesCoordinationTextAndJSON(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	body := `{"ticket":"COORD-1","parent_ticket":"BUG","role":"impl","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[],"priority":"P0"}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(body), &bytes.Buffer{}, &bytes.Buffer{})
	if code := RunClaimCLI([]string{"add", "--target", target, "--ticket", "COORD-1", "--resource", "src/api", "--lane", "api"}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("claim add failed")
	}
	if code := RunNoteCLI([]string{"add", "--target", target, "--kind", "decision", "--scope", "api", "--ticket", "COORD-1", "--summary", "keep shape"}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("note add failed")
	}

	var text bytes.Buffer
	if code := RunNextCLI([]string{"--target", target}, &text, &bytes.Buffer{}); code != 0 {
		t.Fatalf("project next failed")
	}
	if !strings.Contains(text.String(), "Coordination") ||
		!strings.Contains(text.String(), "active claims=1") ||
		!strings.Contains(text.String(), "recent notes=1") {
		t.Fatalf("missing coordination text: %s", text.String())
	}

	var out bytes.Buffer
	if code := RunNextCLI([]string{"--target", target, "--format", "json"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("project next json failed")
	}
	var resp map[string]any
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	coord, ok := resp["coordination"].(map[string]any)
	if !ok {
		t.Fatalf("missing coordination json: %+v", resp)
	}
	claims, _ := coord["claims"].([]any)
	notes, _ := coord["notes"].([]any)
	if len(claims) != 1 || len(notes) != 1 {
		t.Fatalf("unexpected coordination json: %+v", coord)
	}
}
