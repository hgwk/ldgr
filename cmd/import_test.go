package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hgwk/ldgr/internal/config"
)

func TestImportLegacy_PlanWritesNothing(t *testing.T) {
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "agent-tickets.jsonl"),
		[]byte(`{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"BUG-1","parent_ticket":"BUG","agent":"codex","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	before := snapshot(t, target)

	out := &bytes.Buffer{}
	if code := RunImportCLI([]string{"legacy", "--target", target, "--plan"}, out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("plan failed")
	}
	if !strings.Contains(out.String(), "Legacy import plan") {
		t.Fatalf("plan output should contain banner, got: %s", out.String())
	}

	after := snapshot(t, target)
	if before != after {
		t.Fatalf("--plan must not change the filesystem")
	}
}

func TestImportLegacy_ApplyCreatesLedger(t *testing.T) {
	target := t.TempDir()
	regDir := t.TempDir()
	t.Setenv("LDGR_HOME", regDir)
	if err := os.WriteFile(filepath.Join(target, "agent-tickets.jsonl"),
		[]byte(`{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"BUG-1","parent_ticket":"BUG","agent":"codex","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if code := RunImportCLI([]string{"legacy", "--target", target, "--apply", "--init"}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("apply --init failed")
	}
	if _, err := os.Stat(filepath.Join(target, "ledger", "tickets.jsonl")); err != nil {
		t.Fatalf("tickets file not produced: %v", err)
	}
}

func TestImportLegacy_ApplyRejectsWithoutConfig(t *testing.T) {
	target := t.TempDir()
	os.WriteFile(filepath.Join(target, "agent-tickets.jsonl"),
		[]byte(`{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"BUG-1","parent_ticket":"BUG","agent":"codex","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n"), 0o644)
	var stderr bytes.Buffer
	code := RunImportCLI([]string{"legacy", "--target", target, "--apply"}, &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatalf("expected rejection when config missing")
	}
	if !strings.Contains(stderr.String(), "ldgr init") {
		t.Fatalf("stderr should suggest ldgr init / --init, got: %s", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(target, "ledger", "tickets.jsonl")); err == nil {
		t.Fatalf("apply must not write target on rejection")
	}
}

func TestImportLegacy_ApplyInitBootstraps(t *testing.T) {
	target := t.TempDir()
	regDir := t.TempDir()
	t.Setenv("LDGR_HOME", regDir)
	os.WriteFile(filepath.Join(target, "agent-tickets.jsonl"),
		[]byte(`{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"BUG-1","parent_ticket":"BUG","agent":"codex","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n"), 0o644)
	if code := RunImportCLI([]string{"legacy", "--target", target, "--apply", "--init"}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("apply --init should succeed")
	}
	// config.json must be a real config with non-stub project_id.
	cfg, err := config.Load(filepath.Join(target, "ledger", "config.json"))
	if err != nil {
		t.Fatalf("config not created: %v", err)
	}
	if cfg.ProjectID == "" || cfg.ProjectID == "import-stub" {
		t.Fatalf("project_id should be a real id, got %q", cfg.ProjectID)
	}
	// Tickets imported.
	if _, err := os.Stat(filepath.Join(target, "ledger", "tickets.jsonl")); err != nil {
		t.Fatalf("tickets file missing: %v", err)
	}
}

func TestImportLegacy_PlanStillWorksWithoutConfig(t *testing.T) {
	target := t.TempDir()
	os.WriteFile(filepath.Join(target, "agent-tickets.jsonl"),
		[]byte(`{"n":1,"ts":"2026-05-14T10:00:00Z","ticket":"BUG-1","parent_ticket":"BUG","agent":"codex","role":"impl","status":"open","task":"t","scope":"repo","paths":[],"blocked_by":[],"branch":""}`+"\n"), 0o644)
	var out bytes.Buffer
	if code := RunImportCLI([]string{"legacy", "--target", target, "--plan"}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("plan should still work without config")
	}
	if !strings.Contains(out.String(), "Legacy import plan") {
		t.Fatalf("plan output missing banner: %s", out.String())
	}
}

func snapshot(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		b.WriteString(rel)
		b.WriteString("|")
		if d.IsDir() {
			b.WriteString("DIR")
		} else {
			data, _ := os.ReadFile(p)
			b.WriteString(string(data))
		}
		b.WriteString("\n")
		return nil
	})
	return b.String()
}
