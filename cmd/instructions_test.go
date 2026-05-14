package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const legacyBlock = `<!-- LEDGER_KIT_START -->
old pointer body
<!-- LEDGER_KIT_END -->
`

func TestInstructionsInstall_CreatesBodiesAndPointer(t *testing.T) {
	dir := t.TempDir()
	if code := RunInstructionsCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("install failed")
	}
	for _, p := range []string{"ledger/instructions/AGENTS.ldgr.md", "ledger/instructions/CLAUDE.ldgr.md"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Fatalf("missing %s: %v", p, err)
		}
	}
	for _, p := range []string{"AGENTS.md", "CLAUDE.md"} {
		data, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil {
			t.Fatalf("expected %s: %v", p, err)
		}
		if !strings.Contains(string(data), "LDGR_START") {
			t.Fatalf("missing pointer in %s: %s", p, data)
		}
	}
}

func TestInstructionsInstall_PreservesExistingMarkdown(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# My project\nuser content\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	RunInstructionsCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{})
	data, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if !strings.Contains(string(data), "user content") {
		t.Fatalf("user content lost: %s", data)
	}
	if !strings.Contains(string(data), "LDGR_START") {
		t.Fatalf("missing marker: %s", data)
	}
}

func TestInstructionsInstall_Idempotent(t *testing.T) {
	dir := t.TempDir()
	RunInstructionsCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{})
	first, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	RunInstructionsCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{})
	second, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if string(first) != string(second) {
		t.Fatalf("re-install changed pointer:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestInstructionsInstall_MigratesLegacyMarker(t *testing.T) {
	dir := t.TempDir()
	body := legacyBlock + "# project\nuser content\n"
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(body), 0o644)
	if code := RunInstructionsCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("install failed")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if strings.Contains(string(data), "LEDGER_KIT_START") {
		t.Fatalf("legacy marker should be migrated, still present: %s", data)
	}
	if !strings.Contains(string(data), "LDGR_START") {
		t.Fatalf("new marker missing: %s", data)
	}
	if !strings.Contains(string(data), "user content") {
		t.Fatalf("user content lost: %s", data)
	}
}

func TestInstructionsUninstall_RemovesPointer(t *testing.T) {
	dir := t.TempDir()
	RunInstructionsCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{})
	if code := RunInstructionsCLI([]string{"uninstall", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("uninstall failed")
	}
	for _, p := range []string{"AGENTS.md", "CLAUDE.md"} {
		data, err := os.ReadFile(filepath.Join(dir, p))
		if err == nil && strings.Contains(string(data), "LDGR_START") {
			t.Fatalf("marker survived uninstall in %s: %s", p, data)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "ledger", "instructions", "AGENTS.ldgr.md")); !os.IsNotExist(err) {
		t.Fatalf("body should be removed: err=%v", err)
	}
}

func TestInstructionsUninstall_KeepBodies(t *testing.T) {
	dir := t.TempDir()
	RunInstructionsCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{})
	RunInstructionsCLI([]string{"uninstall", "--target", dir, "--keep-bodies"}, &bytes.Buffer{}, &bytes.Buffer{})
	if _, err := os.Stat(filepath.Join(dir, "ledger", "instructions", "AGENTS.ldgr.md")); err != nil {
		t.Fatalf("body should be kept: %v", err)
	}
}
