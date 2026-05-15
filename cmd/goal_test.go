package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hgwk/ldgr/internal/jsonio"
	"github.com/hgwk/ldgr/internal/ledger"
)

func TestGoalSet_OverwritesGoalJSON(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	in := map[string]any{"summary": "new goal", "success_criteria": []any{"a"}}
	body, _ := json.Marshal(in)
	if code := RunGoalCLI([]string{"set", "--target", target, "--json", "@-"}, bytes.NewReader(body), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("set failed")
	}
	var g ledger.Goal
	if err := jsonio.ReadJSON(filepath.Join(target, "ledger", "goal.json"), &g); err != nil {
		t.Fatalf("read goal: %v", err)
	}
	if g.Summary != "new goal" || len(g.SuccessCriteria) != 1 || g.SuccessCriteria[0] != "a" {
		t.Fatalf("goal not applied: %+v", g)
	}
}

func TestGoalSet_LogFlagWritesWorklog(t *testing.T) {
	target, _ := mustInit(t)
	t.Setenv("LEDGER_AGENT", "codex")
	in := map[string]any{"summary": "canonical v1"}
	body, _ := json.Marshal(in)
	errb := &bytes.Buffer{}
	if code := RunGoalCLI([]string{"set", "--target", target, "--json", "@-", "--log"}, bytes.NewReader(body), &bytes.Buffer{}, errb); code != 0 {
		t.Fatalf("set --log failed: %s", errb.String())
	}
	rows, _ := ledger.ReadRows(filepath.Join(target, "ledger", "worklog.jsonl"))
	if len(rows) != 1 {
		t.Fatalf("expected one worklog row, got %d", len(rows))
	}
	if rows[0]["task"] != "goal set" {
		t.Fatalf("worklog task should be 'goal set', got %v", rows[0]["task"])
	}
}

func TestGoalShow_PrintsGoal(t *testing.T) {
	target, _ := mustInit(t)
	out := &bytes.Buffer{}
	if code := RunGoalCLI([]string{"show", "--target", target}, &bytes.Buffer{}, out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("show failed")
	}
	if !strings.Contains(out.String(), "schema_version") {
		t.Fatalf("show output missing schema_version, got: %s", out.String())
	}
}
