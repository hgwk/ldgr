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

func TestTicketAdd_RejectsInitialDone(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	in := map[string]any{
		"ticket": "INIT-DONE", "parent_ticket": "ROOT", "role": "impl",
		"status": "done", "task": "skip the work", "scope": "repo",
		"paths": []any{}, "blocked_by": []any{},
	}
	body, _ := json.Marshal(in)
	var stderr bytes.Buffer
	code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(body), &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero for initial status=done")
	}
	if !strings.Contains(stderr.String(), "ldgr next") {
		t.Fatalf("stderr should suggest ldgr next, got: %s", stderr.String())
	}
	// Verify nothing was appended.
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	for _, r := range rows {
		if r["ticket"] == "INIT-DONE" {
			t.Fatalf("INIT-DONE must not be appended on rejection: %+v", r)
		}
	}
}

func TestTicketEvent_RejectsImplDirectDone(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	// Seed an in_progress ticket.
	add := `{"ticket":"GATE-1","parent_ticket":"BUG","role":"impl","status":"in_progress","task":"x","scope":"repo","paths":[],"blocked_by":[]}`
	if code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("seed failed")
	}
	// Try to jump to done directly.
	ev := `{"ticket":"GATE-1","status":"done"}`
	var stderr bytes.Buffer
	code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(ev), &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected rejection")
	}
	if !strings.Contains(stderr.String(), "audit_ready") {
		t.Fatalf("stderr should mention audit_ready, got: %s", stderr.String())
	}
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	// Only the seed row should exist for GATE-1.
	count := 0
	for _, r := range rows {
		if r["ticket"] == "GATE-1" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("event should not append on rejection; expected 1 row, got %d", count)
	}
}

func TestTicketEvent_AcceptsAuditPassClose(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	// Seed: open → in_progress → audit_ready (with evidence).
	add := `{"ticket":"PASS-1","parent_ticket":"BUG","role":"impl","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	in := `{"ticket":"PASS-1","status":"in_progress"}`
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(in), &bytes.Buffer{}, &bytes.Buffer{})
	ready := `{"ticket":"PASS-1","status":"audit_ready","evidence":["go test ./..."]}`
	if code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(ready), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("audit_ready event failed")
	}
	// audit_ready row n is the 3rd row.
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	auditN := int(rows[len(rows)-1]["n"].(float64))
	pass := fmt.Sprintf(`{"ticket":"PASS-1","role":"audit","status":"done","audit_result":"pass","evidence":["go test ./..."],"reviewed_n":%d}`, auditN)
	var stderr bytes.Buffer
	if code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(pass), &bytes.Buffer{}, &stderr); code != 0 {
		t.Fatalf("audit-pass close should succeed, stderr=%s", stderr.String())
	}
}

func TestTicketEvent_RejectsInvalidTransition(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"ticket":"TRANS-1","parent_ticket":"BUG","role":"impl","status":"open","task":"x","scope":"repo","paths":[],"blocked_by":[]}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	// open → audit_ready is not allowed.
	ev := `{"ticket":"TRANS-1","status":"audit_ready","evidence":["x"]}`
	var stderr bytes.Buffer
	code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(ev), &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected rejection for open → audit_ready")
	}
	if !strings.Contains(stderr.String(), "open") || !strings.Contains(stderr.String(), "audit_ready") {
		t.Fatalf("error should name the rejected edge, got: %s", stderr.String())
	}
}
