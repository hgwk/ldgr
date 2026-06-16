package cmd

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hgwk/ldgr/internal/ledger"
)

func TestTicketAddState_AppendsSchemaStateRow(t *testing.T) {
	target := mustInitState(t)
	t.Setenv("LEDGER_AGENT", "codex")
	in := `{"id":"STATE-1","parent":"ROOT","type":"task","state":"ready","area":"backend","priority":"P1","title":"build state-model writer","blocked_by":[],"acceptance":["verify"],"evidence":[],"event":{"role":"planner","summary":"opened","notes":""}}`
	var stderr bytes.Buffer
	if code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(in), &bytes.Buffer{}, &stderr); code != 0 {
		t.Fatalf("state-model add failed: %s", stderr.String())
	}
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	row := rows[0]
	if row["id"] != "STATE-1" || row["owner"] != "codex" || row["state"] != "ready" {
		t.Fatalf("unexpected state-model row: %+v", row)
	}
	event, _ := row["event"].(map[string]any)
	if event["actor"] != "codex" {
		t.Fatalf("event.actor should default from agent: %+v", event)
	}
}

func TestTicketEventState_AuditPassDone(t *testing.T) {
	target := mustInitState(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"id":"STATE-PASS","parent":"ROOT","type":"task","state":"ready","area":"backend","priority":"P1","title":"build","blocked_by":[],"acceptance":["verify"],"evidence":[],"event":{"role":"planner","summary":"opened","notes":""}}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"id":"STATE-PASS","state":"doing","event":{"role":"implementer","summary":"started","notes":""}}`), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"id":"STATE-PASS","state":"review","evidence":["go test"],"event":{"role":"implementer","summary":"ready","notes":""}}`), &bytes.Buffer{}, &bytes.Buffer{})
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	reviewN := int(rows[len(rows)-1]["n"].(float64))
	pass := fmt.Sprintf(`{"id":"STATE-PASS","state":"done","evidence":["go test"],"event":{"role":"auditor","result":"pass","reviewed_n":%d,"summary":"passed","notes":""}}`, reviewN)
	var stderr bytes.Buffer
	if code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(pass), &bytes.Buffer{}, &stderr); code != 0 {
		t.Fatalf("state-model audit pass failed: %s", stderr.String())
	}
	rows, _ = ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	if rows[len(rows)-1]["state"] != "done" {
		t.Fatalf("expected done row, got %+v", rows[len(rows)-1])
	}
}

func TestTicketEventState_RejectsReviewWithoutTestEvidence(t *testing.T) {
	target := mustInitState(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"id":"STATE-NO-TEST","parent":"ROOT","type":"task","state":"ready","area":"backend","priority":"P1","title":"build","blocked_by":[],"acceptance":["verify"],"evidence":[],"event":{"role":"planner","summary":"opened","notes":""}}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"id":"STATE-NO-TEST","state":"doing","event":{"role":"implementer","summary":"started","notes":""}}`), &bytes.Buffer{}, &bytes.Buffer{})
	var stderr bytes.Buffer

	code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"id":"STATE-NO-TEST","state":"review","evidence":["ok"],"event":{"role":"implementer","summary":"ready","notes":""}}`), &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected review without test evidence rejection")
	}
	if !strings.Contains(stderr.String(), "state=review requires test evidence") {
		t.Fatalf("stderr should explain test evidence requirement, got: %s", stderr.String())
	}
}

func TestTicketEventState_RejectsDoneWithOnlyNotRunTestEvidence(t *testing.T) {
	target := mustInitState(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"id":"STATE-NOT-RUN","parent":"ROOT","type":"task","state":"ready","area":"backend","priority":"P1","title":"build","blocked_by":[],"acceptance":["verify"],"evidence":[],"event":{"role":"planner","summary":"opened","notes":""}}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"id":"STATE-NOT-RUN","state":"doing","event":{"role":"implementer","summary":"started","notes":""}}`), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"id":"STATE-NOT-RUN","state":"review","evidence":["test:not_run: unavailable"],"event":{"role":"implementer","summary":"ready","notes":""}}`), &bytes.Buffer{}, &bytes.Buffer{})
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	reviewN := int(rows[len(rows)-1]["n"].(float64))
	done := fmt.Sprintf(`{"id":"STATE-NOT-RUN","state":"done","evidence":["test:not_run: unavailable"],"event":{"role":"auditor","result":"pass","reviewed_n":%d,"summary":"passed","notes":""}}`, reviewN)
	var stderr bytes.Buffer

	code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(done), &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected done with only not_run evidence rejection")
	}
	if !strings.Contains(stderr.String(), "test:not_run") {
		t.Fatalf("stderr should mention test:not_run, got: %s", stderr.String())
	}
}

func TestTicketEventState_RejectsDoneWithCommitButNoTestEvidence(t *testing.T) {
	target := mustInitState(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"id":"STATE-COMMIT-ONLY","parent":"ROOT","type":"task","state":"ready","area":"backend","priority":"P1","title":"build","blocked_by":[],"acceptance":["verify"],"evidence":[],"event":{"role":"planner","summary":"opened","notes":""}}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"id":"STATE-COMMIT-ONLY","state":"doing","event":{"role":"implementer","summary":"started","notes":""}}`), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"id":"STATE-COMMIT-ONLY","state":"review","evidence":["test:manual: local review"],"event":{"role":"implementer","summary":"ready","notes":""}}`), &bytes.Buffer{}, &bytes.Buffer{})
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	reviewN := int(rows[len(rows)-1]["n"].(float64))
	done := fmt.Sprintf(`{"id":"STATE-COMMIT-ONLY","state":"done","evidence":["commit:abc123"],"event":{"role":"auditor","result":"pass","reviewed_n":%d,"summary":"passed","notes":""}}`, reviewN)
	var stderr bytes.Buffer

	code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(done), &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected done with commit-only evidence rejection")
	}
	if !strings.Contains(stderr.String(), "passing test evidence") {
		t.Fatalf("stderr should mention passing test evidence, got: %s", stderr.String())
	}
}

func TestTicketEventState_RejectsDirectDone(t *testing.T) {
	target := mustInitState(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"id":"STATE-BAD","parent":"ROOT","type":"task","state":"ready","area":"backend","priority":"P1","title":"build","blocked_by":[],"acceptance":[],"evidence":[],"event":{"role":"planner","summary":"opened","notes":""}}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	var stderr bytes.Buffer
	code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"id":"STATE-BAD","state":"done","event":{"role":"implementer","summary":"done","notes":""}}`), &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected direct done rejection")
	}
	if !strings.Contains(stderr.String(), "ready -> done") {
		t.Fatalf("stderr should name rejected edge, got: %s", stderr.String())
	}
}

func TestTicketEventState_ReworkImplementerErrorSuggestsDoing(t *testing.T) {
	target := mustInitState(t)
	t.Setenv("LEDGER_AGENT", "codex")
	add := `{"id":"STATE-REWORK","parent":"ROOT","type":"task","state":"ready","area":"backend","priority":"P1","title":"build","blocked_by":[],"acceptance":[],"evidence":[],"event":{"role":"planner","summary":"opened","notes":""}}`
	RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(add), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"id":"STATE-REWORK","state":"doing","event":{"role":"implementer","summary":"started","notes":""}}`), &bytes.Buffer{}, &bytes.Buffer{})
	RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"id":"STATE-REWORK","state":"review","evidence":["go test"],"event":{"role":"implementer","summary":"ready","notes":""}}`), &bytes.Buffer{}, &bytes.Buffer{})

	var stderr bytes.Buffer
	code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"id":"STATE-REWORK","state":"rework","event":{"role":"implementer","summary":"rework started","notes":""}}`), &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected implementer rework row rejection")
	}
	msg := stderr.String()
	for _, want := range []string{"state=rework is an auditor", "state=doing", "event.role=implementer"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("stderr missing %q: %s", want, msg)
		}
	}
}
