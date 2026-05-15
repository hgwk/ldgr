package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hgwk/ldgr/internal/verify"
)

func TestVerifyNewOnlyRequiresBaseline(t *testing.T) {
	target, _ := mustInit(t)
	var stderr bytes.Buffer
	code := RunVerifyCLI([]string{"--target", target, "--new-only"}, &bytes.Buffer{}, &stderr)
	if code != 2 {
		t.Fatalf("expected usage error, got %d", code)
	}
	if !strings.Contains(stderr.String(), "--since-ticket-n") {
		t.Fatalf("stderr should mention split baselines, got %q", stderr.String())
	}
}

func TestVerifyNewOnlyUsesSplitBaselines(t *testing.T) {
	var issues = []verify.Issue{
		{File: "ledger/tickets.jsonl", Line: 2, Code: "OLD_TICKET"},
		{File: "ledger/tickets.jsonl", Line: 4, Code: "NEW_TICKET"},
		{File: "ledger/worklog.jsonl", Line: 1, Code: "OLD_WORKLOG"},
		{File: "ledger/worklog.jsonl", Line: 3, Code: "NEW_WORKLOG"},
	}
	got := filterRowsAtOrBefore(issues, 3, 2)
	if len(got) != 2 || got[0].Code != "NEW_TICKET" || got[1].Code != "NEW_WORKLOG" {
		t.Fatalf("unexpected filtered issues: %+v", got)
	}
}

func TestVerifySummaryCallsOutHistoricalCompatibilityWarnings(t *testing.T) {
	rep := verify.Report{
		Warns: []verify.Issue{
			{File: "ledger/tickets.jsonl", Line: 1, Code: "WEAK_DONE"},
			{File: "ledger/worklog.jsonl", Line: 2, Code: "PREMATURE_WORKLOG"},
			{File: "ledger/tickets.jsonl", Line: 0, Code: "SOMETHING_ELSE"},
		},
	}
	var stdout bytes.Buffer
	printSummary(&stdout, rep)
	text := stdout.String()
	if !strings.Contains(text, "historical compatibility warnings 2") {
		t.Fatalf("summary should group compatibility warnings, got:\n%s", text)
	}
	if !strings.Contains(text, "historical rows checked against current lifecycle/taxonomy gates") {
		t.Fatalf("summary should describe historical compatibility, got:\n%s", text)
	}
	if !strings.Contains(text, "--new-only") || !strings.Contains(text, "--since-ticket-n") {
		t.Fatalf("summary should point to active append gate flags, got:\n%s", text)
	}
}
