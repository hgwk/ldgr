package legacy

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hgwk/ldgr/internal/config"
)

func TestCompose_CreatesNewLedger(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "agent-tickets.jsonl", `{"n":1,"ticket":"BUG-1","task":"fix","status":"open","ts":"2026-05-14T10:00:00Z","parent_ticket":"","agent":"codex","role":"impl","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n")
	srcs, _ := Scan(dir)
	cfg := config.Default("x", "id", "")
	plan := Compose(dir, srcs, cfg, fixedNow())
	if len(plan.Changes) == 0 {
		t.Fatalf("expected at least one Change, got 0")
	}
	var hasTickets bool
	for _, c := range plan.Changes {
		if filepath.Base(c.OutputPath) == "tickets.jsonl" && c.Action == ActionCreate {
			hasTickets = true
		}
	}
	if !hasTickets {
		t.Fatalf("expected tickets.jsonl create, got %+v", plan.Changes)
	}
}

func TestCompose_NoChangeWhenAlreadyConverted(t *testing.T) {
	dir := t.TempDir()
	row := `{"n":1,"ts":"2026-05-14T10:00:00Z","parent_ticket":"BUG","ticket":"BUG-1","agent":"codex","role":"impl","status":"open","task":"fix","scope":"repo","paths":[],"blocked_by":[],"branch":""}`

	// Parse and re-marshal to ensure consistent JSON format for byte comparison
	var m map[string]interface{}
	_ = json.Unmarshal([]byte(row), &m)
	normalized, _ := json.Marshal(m)
	normalizedStr := string(normalized) + "\n"

	writeFile(t, dir, "agent-tickets.jsonl", normalizedStr)
	writeFile(t, dir, "ledger/tickets.jsonl", normalizedStr)
	srcs, _ := Scan(dir)
	plan := Compose(dir, srcs, config.Default("x", "id", ""), fixedNow())
	for _, c := range plan.Changes {
		if filepath.Base(c.OutputPath) == "tickets.jsonl" && c.Action != ActionNoop {
			t.Fatalf("expected noop for already-converted tickets, got %v", c.Action)
		}
	}
}

func TestCompose_InsertsInvalidatesNForGhostRow(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "agent-tickets.jsonl",
		`{"n":1,"ticket":"","task":"","status":"done","ts":"2026-05-14T10:00:00Z","parent_ticket":"ROOT","agent":"codex","role":"impl","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n"+
			`{"n":2,"ticket":"BUG-1","task":"real","status":"open","ts":"2026-05-14T10:01:00Z","parent_ticket":"BUG","agent":"codex","role":"impl","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n")
	srcs, _ := Scan(dir)
	plan := Compose(dir, srcs, config.Default("x", "id", ""), fixedNow())
	var ticketsBytes []byte
	for _, c := range plan.Changes {
		if filepath.Base(c.OutputPath) == "tickets.jsonl" {
			ticketsBytes = c.NewBytes
		}
	}
	if !strings.Contains(string(ticketsBytes), `"invalidates_n":1`) {
		t.Fatalf("expected invalidates_n=1 companion row, got:\n%s", ticketsBytes)
	}
}

func TestCompose_ParseErrorsRouteToImportErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "agent-tickets.jsonl", "not json\n")
	srcs, _ := Scan(dir)
	plan := Compose(dir, srcs, config.Default("x", "id", ""), fixedNow())
	var hasErrors bool
	for _, c := range plan.Changes {
		if filepath.Base(c.OutputPath) == "import-errors.jsonl" && c.Action == ActionCreate {
			hasErrors = true
		}
	}
	if !hasErrors {
		t.Fatalf("expected import-errors.jsonl create, got %+v", plan.Changes)
	}
}
