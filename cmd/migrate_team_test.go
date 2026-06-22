package cmd

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/hgwk/ldgr/internal/ledger"
)

func TestMigrateLegacyToV1_PreservesTeam(t *testing.T) {
	target := seedV1LedgerWithTicket(t,
		`{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"TEAM-1","parent_ticket":"ROOT","agent":"codex","role":"impl","category":"feature","status":"open","task":"team work","scope":"repo","paths":[],"blocked_by":[],"branch":"","team":"platform"}`+"\n",
		"")
	if code := RunMigrateCLI([]string{"legacy-to-v1", "--target", target, "--apply"}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("apply failed")
	}
	tickets, err := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	if err != nil {
		t.Fatalf("read tickets: %v", err)
	}
	if tickets[0]["team"] != "platform" {
		t.Fatalf("team should be top-level after migration: %+v", tickets[0])
	}
	event, _ := tickets[0]["event"].(map[string]any)
	extra, _ := event["extra"].(map[string]any)
	if _, ok := extra["team"]; ok {
		t.Fatalf("team should not be duplicated in event.extra: %+v", event)
	}
}
