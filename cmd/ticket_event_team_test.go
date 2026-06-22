package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hgwk/ldgr/internal/ledger"
)

func TestTicketEvent_StateEventCarriesForwardTeam(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{
		"id":"team-carry",
		"parent":"ROOT",
		"type":"task",
		"state":"ready",
		"area":"backend",
		"priority":"P2",
		"title":"team carry",
		"team":"platform",
		"blocked_by":[],
		"acceptance":[],
		"evidence":[],
		"event":{"actor":"codex","role":"planner","summary":"opened","notes":""}
	}`
	if code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("add failed")
	}
	event := `{
		"id":"team-carry",
		"state":"doing",
		"event":{"actor":"codex","role":"implementer","summary":"started","notes":""}
	}`
	if code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(event), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("event failed")
	}
	rows, err := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	if err != nil {
		t.Fatalf("read rows: %v", err)
	}
	latest := rows[len(rows)-1]
	if latest["team"] != "platform" {
		t.Fatalf("team should carry forward: %+v", latest)
	}
}
