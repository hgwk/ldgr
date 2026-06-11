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

func setTestLDGRHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("LDGR_HOME", home)
	return home
}

func TestInstructionsInstall_CreatesBodiesAndPointer(t *testing.T) {
	home := setTestLDGRHome(t)
	dir := t.TempDir()
	if code := RunInstructionsCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("install failed")
	}
	bodyPath := filepath.Join(home, "operating-guide.md")
	if _, err := os.Stat(bodyPath); err != nil {
		t.Fatalf("missing instruction body: %v", err)
	}
	for _, p := range []string{"AGENTS.md", "CLAUDE.md"} {
		data, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil {
			t.Fatalf("expected %s: %v", p, err)
		}
		if !strings.HasPrefix(string(data), "@"+bodyPath+"\n") {
			t.Fatalf("missing pointer in %s: %s", p, data)
		}
	}
}

func TestInstructionsInstall_HomeFlagOverridesLDGRHome(t *testing.T) {
	envHome := setTestLDGRHome(t)
	flagHome := t.TempDir()
	dir := t.TempDir()

	code := RunInstructionsCLI([]string{"install", "--target", dir, "--home", flagHome}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("install failed")
	}
	if _, err := os.Stat(filepath.Join(flagHome, "operating-guide.md")); err != nil {
		t.Fatalf("missing flag home body: %v", err)
	}
	if _, err := os.Stat(filepath.Join(envHome, "operating-guide.md")); !os.IsNotExist(err) {
		t.Fatalf("env home should not be used when --home is set: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if !strings.HasPrefix(string(data), "@"+filepath.Join(flagHome, "operating-guide.md")+"\n") {
		t.Fatalf("missing flag home pointer: %s", data)
	}
}

func TestInstructionsInstall_PreservesExistingMarkdown(t *testing.T) {
	home := setTestLDGRHome(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# My project\nuser content\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	RunInstructionsCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{})
	data, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if !strings.Contains(string(data), "user content") {
		t.Fatalf("user content lost: %s", data)
	}
	if !strings.HasPrefix(string(data), "@"+filepath.Join(home, "operating-guide.md")+"\n") {
		t.Fatalf("missing marker: %s", data)
	}
}

func TestInstructionsInstall_Idempotent(t *testing.T) {
	setTestLDGRHome(t)
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
	home := setTestLDGRHome(t)
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
	if !strings.HasPrefix(string(data), "@"+filepath.Join(home, "operating-guide.md")+"\n") {
		t.Fatalf("new marker missing: %s", data)
	}
	if !strings.Contains(string(data), "user content") {
		t.Fatalf("user content lost: %s", data)
	}
}

func TestInstructionsInstall_MigratesSplitPointer(t *testing.T) {
	home := setTestLDGRHome(t)
	dir := t.TempDir()
	body := "@ledger/instructions/AGENTS.ldgr.md\n\n---\n\n# project\nuser content\n"
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(body), 0o644)
	os.MkdirAll(filepath.Join(dir, "ledger", "instructions"), 0o755)
	os.WriteFile(filepath.Join(dir, "ledger", "instructions", "AGENTS.ldgr.md"), []byte("old"), 0o644)

	if code := RunInstructionsCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("install failed")
	}

	data, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if !strings.HasPrefix(string(data), "@"+filepath.Join(home, "operating-guide.md")+"\n") {
		t.Fatalf("new common pointer missing: %s", data)
	}
	if strings.Contains(string(data), "AGENTS.ldgr.md") {
		t.Fatalf("split pointer should be removed: %s", data)
	}
	if !strings.Contains(string(data), "user content") {
		t.Fatalf("user content lost: %s", data)
	}
	if _, err := os.Stat(filepath.Join(dir, "ledger", "instructions", "AGENTS.ldgr.md")); !os.IsNotExist(err) {
		t.Fatalf("old split body should be removed: %v", err)
	}
}

func TestInstructionsUninstall_RemovesPointer(t *testing.T) {
	home := setTestLDGRHome(t)
	dir := t.TempDir()
	RunInstructionsCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{})
	if code := RunInstructionsCLI([]string{"uninstall", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("uninstall failed")
	}
	for _, p := range []string{"AGENTS.md", "CLAUDE.md"} {
		data, err := os.ReadFile(filepath.Join(dir, p))
		if err == nil && (strings.Contains(string(data), "@"+filepath.Join(home, "operating-guide.md")) || strings.Contains(string(data), "@.ldgr/operating-guide.md") || strings.Contains(string(data), "@ledger/instructions/")) {
			t.Fatalf("pointer survived uninstall in %s: %s", p, data)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".ldgr", "operating-guide.md")); !os.IsNotExist(err) {
		t.Fatalf("body should be removed: err=%v", err)
	}
}

func TestInstructionsUninstall_KeepBodies(t *testing.T) {
	home := setTestLDGRHome(t)
	dir := t.TempDir()
	RunInstructionsCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{})
	RunInstructionsCLI([]string{"uninstall", "--target", dir, "--keep-bodies"}, &bytes.Buffer{}, &bytes.Buffer{})
	if _, err := os.Stat(filepath.Join(home, "operating-guide.md")); err != nil {
		t.Fatalf("home body should be kept: %v", err)
	}
}
