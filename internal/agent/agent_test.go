package agent

import (
	"strings"
	"testing"
)

func TestResolve_ExplicitWins(t *testing.T) {
	env := map[string]string{"LEDGER_AGENT": "codex", "USER": "alice"}
	got, warn, err := Resolve("claude-from-json", env)
	if err != nil || warn != "" || got != "claude-from-json" {
		t.Fatalf("got=%q warn=%q err=%v", got, warn, err)
	}
}

func TestResolve_LedgerAgentEnv(t *testing.T) {
	env := map[string]string{"LEDGER_AGENT": "codex", "USER": "alice"}
	got, warn, err := Resolve("", env)
	if err != nil || warn != "" || got != "codex" {
		t.Fatalf("got=%q warn=%q err=%v", got, warn, err)
	}
}

func TestResolve_DetectCodex(t *testing.T) {
	env := map[string]string{"CODEX_SESSION": "1"}
	got, _, err := Resolve("", env)
	if err != nil || got != "codex" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestResolve_DetectClaude(t *testing.T) {
	env := map[string]string{"CLAUDECODE": "1"}
	got, _, err := Resolve("", env)
	if err != nil || got != "claude" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestResolve_DetectCursor(t *testing.T) {
	env := map[string]string{"CURSOR_AGENT": "1"}
	got, _, err := Resolve("", env)
	if err != nil || got != "cursor" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestResolve_FallbackToUserWarns(t *testing.T) {
	env := map[string]string{"USER": "alice"}
	got, warn, err := Resolve("", env)
	if err != nil || got != "alice" || warn == "" {
		t.Fatalf("got=%q warn=%q err=%v", got, warn, err)
	}
	if !strings.Contains(warn, "USER") {
		t.Fatalf("warn should mention USER fallback, got %q", warn)
	}
}

func TestResolve_NothingFails(t *testing.T) {
	env := map[string]string{}
	_, _, err := Resolve("", env)
	if err == nil {
		t.Fatalf("expected error when nothing can resolve agent")
	}
}

func TestResolve_PrecedenceWhenMultipleEnvsPresent(t *testing.T) {
	// Both codex and cursor signals present — codex must win because it has
	// higher precedence in the §5.1 detection order.
	env := map[string]string{"CODEX_SESSION": "1", "CURSOR_AGENT": "1"}
	for i := 0; i < 50; i++ {
		got, _, err := Resolve("", env)
		if err != nil || got != "codex" {
			t.Fatalf("iteration %d: got=%q err=%v (expected stable codex)", i, got, err)
		}
	}
}
