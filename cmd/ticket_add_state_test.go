package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hgwk/ldgr/internal/ledger"
)

func TestTicketAdd_StateMissingRequiredReportsAllTopLevelFields(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	body := []byte(`{"id":"agent-guide-parent-sync"}`)
	var stderr bytes.Buffer

	code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(body), &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected failure")
	}
	for _, want := range []string{
		`missing required field "parent"`,
		`missing required field "type"`,
		`missing required field "state"`,
		`missing required field "area"`,
		`missing required field "priority"`,
		`missing required field "blocked_by"`,
		`missing required field "acceptance"`,
		`missing required field "evidence"`,
		`missing required field "event"`,
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q:\n%s", want, stderr.String())
		}
	}
}

func TestTicketAdd_StateMissingEventFieldsReportsAllFields(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	body := []byte(`{
		"id":"agent-guide-parent-sync",
		"parent":"ROOT",
		"type":"task",
		"state":"ready",
		"area":"docs",
		"priority":"P2",
		"title":"Record guide update",
		"blocked_by":[],
		"acceptance":[],
		"evidence":[],
		"event":{"role":"planner"}
	}`)
	var stderr bytes.Buffer

	code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(body), &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected failure")
	}
	for _, want := range []string{
		`missing required field "event.summary"`,
		`missing required field "event.notes"`,
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q:\n%s", want, stderr.String())
		}
	}
}

func TestTicketAdd_StateInvalidEnumsShowAllowedValues(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	body := []byte(`{
		"id":"agent-guide-parent-sync",
		"parent":"ROOT",
		"type":"docs",
		"state":"ready",
		"area":"docs",
		"priority":"P2",
		"title":"Record guide update",
		"blocked_by":[],
		"acceptance":[],
		"evidence":[],
		"event":{"role":"planner","summary":"opened","notes":""}
	}`)
	var stderr bytes.Buffer

	code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, bytes.NewReader(body), &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected failure")
	}
	for _, want := range []string{`invalid type "docs"`, `allowed: audit, epic, issue, ops, plan, task`} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q:\n%s", want, stderr.String())
		}
	}
}

func TestTicketEvent_StateFailureShowsExampleHint(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	var stderr bytes.Buffer

	code := RunTicketCLI([]string{"event", "--target", target, "--json", "@-"}, strings.NewReader(`{"id":"missing"}`), &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected failure")
	}
	if !strings.Contains(stderr.String(), "ldgr ticket add --example") {
		t.Fatalf("stderr missing example hint:\n%s", stderr.String())
	}
}

func TestTicketAdd_ExampleIsValidStateTicket(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	var example bytes.Buffer
	if code := RunTicketCLI([]string{"add", "--example"}, &bytes.Buffer{}, &example, &bytes.Buffer{}); code != 0 {
		t.Fatalf("example failed")
	}
	var out, stderr bytes.Buffer
	code := RunTicketCLI([]string{"add", "--target", target, "--json", "@-"}, strings.NewReader(example.String()), &out, &stderr)
	if code != 0 {
		t.Fatalf("example did not append: %s", stderr.String())
	}
	rows, err := ledger.ReadRows(filepath.Join(target, "ledger", "tickets.jsonl"))
	if err != nil {
		t.Fatalf("read rows: %v", err)
	}
	if len(rows) != 1 || rows[0]["id"] != "agent-guide-parent-sync" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}
