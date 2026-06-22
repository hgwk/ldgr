package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestSuggestPlan_NewTicketSkeleton(t *testing.T) {
	t.Setenv("LEDGER_AGENT", "codex")
	target, _ := mustInit(t)
	var out bytes.Buffer
	if code := RunSuggestCLI([]string{"plan", "--target", target, "--ticket", "PLAN-1"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("suggest plan failed: %s", out.String())
	}
	var skel map[string]any
	json.Unmarshal(out.Bytes(), &skel)
	if skel["id"] != "PLAN-1" || skel["state"] != "backlog" || skel["type"] != "plan" {
		t.Fatalf("plan skeleton wrong: %+v", skel)
	}
	if skel["area"] != "ops" || skel["owner"] != "codex" {
		t.Fatalf("plan skeleton should include usable defaults: %+v", skel)
	}
	event, _ := skel["event"].(map[string]any)
	if event["actor"] != "codex" {
		t.Fatalf("plan skeleton should include event.actor: %+v", skel)
	}
}

func TestSuggestPlan_AppliesStatePlanOptions(t *testing.T) {
	target := mustInitState(t)
	var out bytes.Buffer
	args := []string{
		"plan", "--target", target, "--ticket", "PLAN-AREA",
		"--parent", "EPIC-1",
		"--area", "backend",
		"--owner", "claude",
		"--priority", "P1",
		"--team", "platform",
	}
	if code := RunSuggestCLI(args, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("suggest plan failed: %s", out.String())
	}
	var skel map[string]any
	if err := json.Unmarshal(out.Bytes(), &skel); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	for key, want := range map[string]string{
		"parent":   "EPIC-1",
		"area":     "backend",
		"owner":    "claude",
		"priority": "P1",
		"team":     "platform",
	} {
		if skel[key] != want {
			t.Fatalf("%s = %v, want %s; skeleton=%+v", key, skel[key], want, skel)
		}
	}
	event, _ := skel["event"].(map[string]any)
	if event["actor"] != "claude" {
		t.Fatalf("event.actor should follow owner: %+v", event)
	}
}

func TestSuggestPlan_RejectsInvalidArea(t *testing.T) {
	target := mustInitState(t)
	var out, errOut bytes.Buffer
	code := RunSuggestCLI([]string{"plan", "--target", target, "--ticket", "PLAN-BAD", "--area", "product"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected usage error, got %d stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), `invalid --area "product"`) {
		t.Fatalf("missing invalid area message: %s", errOut.String())
	}
}

func TestSuggestPlan_UsesWritingLanguage(t *testing.T) {
	target, store := mustInit(t)
	if err := RunInit(target, InitOpts{Slug: "myapp", WritingLanguage: "ko"}, store); err != nil {
		t.Fatalf("set language: %v", err)
	}
	var out bytes.Buffer
	if code := RunSuggestCLI([]string{"plan", "--target", target, "--ticket", "PLAN-KO"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("suggest plan failed: %s", out.String())
	}
	var skel map[string]any
	if err := json.Unmarshal(out.Bytes(), &skel); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if skel["writing_language"] != "ko" {
		t.Fatalf("missing writing_language: %+v", skel)
	}
	if skel["title"] != "<한 줄 작업 설명>" {
		t.Fatalf("expected Korean title placeholder, got %+v", skel["title"])
	}
}

func TestSuggestPlanState_NewTicketSkeleton(t *testing.T) {
	target := mustInitState(t)
	seedStateTicket(t, target, "SEED-STATE")
	var out bytes.Buffer
	if code := RunSuggestCLI([]string{"plan", "--target", target, "--ticket", "PLAN-STATE"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("suggest state-model plan failed: %s", out.String())
	}
	var skel map[string]any
	if err := json.Unmarshal(out.Bytes(), &skel); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if skel["id"] != "PLAN-STATE" || skel["state"] != "backlog" || skel["type"] != "plan" {
		t.Fatalf("state-model plan skeleton wrong: %+v", skel)
	}
	if _, ok := skel["ticket"]; ok {
		t.Fatalf("state-model plan skeleton should not include v1 ticket: %+v", skel)
	}
}
