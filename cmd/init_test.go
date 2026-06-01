package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hgwk/ldgr/internal/config"
	"github.com/hgwk/ldgr/internal/registry"
)

func TestRunInit_CreatesFiles(t *testing.T) {
	target := t.TempDir()
	regDir := t.TempDir()
	store := registry.New(filepath.Join(regDir, "registry.json"), filepath.Join(regDir, "registry.lock"))

	err := RunInit(target, InitOpts{Slug: "myapp"}, store)
	if err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	mustExist := []string{
		"ledger/config.json",
		"ledger/goal.json",
		"ledger/tickets.jsonl",
		"ledger/worklog.jsonl",
		"ledger/instructions/ldgr.md",
		"AGENTS.md",
		"CLAUDE.md",
	}
	for _, p := range mustExist {
		if _, err := os.Stat(filepath.Join(target, p)); err != nil {
			t.Fatalf("expected %s to exist: %v", p, err)
		}
	}
}

func TestRunInit_InstallsInstructionPointers(t *testing.T) {
	target := t.TempDir()
	regDir := t.TempDir()
	store := registry.New(filepath.Join(regDir, "registry.json"), filepath.Join(regDir, "registry.lock"))
	if err := os.WriteFile(filepath.Join(target, "AGENTS.md"), []byte("# Project\nkeep me\n"), 0o644); err != nil {
		t.Fatalf("seed AGENTS.md: %v", err)
	}

	if err := RunInit(target, InitOpts{Slug: "myapp"}, store); err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(target, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	for _, needle := range []string{"@ledger/instructions/ldgr.md", "keep me"} {
		if !contains(string(data), needle) {
			t.Fatalf("AGENTS.md missing %q:\n%s", needle, data)
		}
	}
}

func TestRunInit_RegistersProject(t *testing.T) {
	target := t.TempDir()
	regDir := t.TempDir()
	store := registry.New(filepath.Join(regDir, "registry.json"), filepath.Join(regDir, "registry.lock"))

	if err := RunInit(target, InitOpts{Slug: "myapp"}, store); err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	r, _ := store.Load()
	if len(r.Projects) != 1 {
		t.Fatalf("expected 1 project registered, got %d", len(r.Projects))
	}
	if r.Projects[0].Slug != "myapp" {
		t.Fatalf("slug mismatch: %s", r.Projects[0].Slug)
	}
	if r.Projects[0].Paths[0] != target {
		t.Fatalf("path mismatch: %v", r.Projects[0].Paths)
	}
}

func TestRunInit_IsIdempotent(t *testing.T) {
	target := t.TempDir()
	regDir := t.TempDir()
	store := registry.New(filepath.Join(regDir, "registry.json"), filepath.Join(regDir, "registry.lock"))

	if err := RunInit(target, InitOpts{Slug: "myapp"}, store); err != nil {
		t.Fatalf("first: %v", err)
	}
	r1, _ := store.Load()
	id := r1.Projects[0].ProjectID

	if err := RunInit(target, InitOpts{Slug: "myapp"}, store); err != nil {
		t.Fatalf("second: %v", err)
	}
	r2, _ := store.Load()
	if len(r2.Projects) != 1 {
		t.Fatalf("second init should not create a new project entry, got %d", len(r2.Projects))
	}
	if r2.Projects[0].ProjectID != id {
		t.Fatalf("project_id should be preserved across re-init: %s vs %s", id, r2.Projects[0].ProjectID)
	}
}

func TestRunInit_WritingLanguage(t *testing.T) {
	target := t.TempDir()
	regDir := t.TempDir()
	store := registry.New(filepath.Join(regDir, "registry.json"), filepath.Join(regDir, "registry.lock"))

	if err := RunInit(target, InitOpts{Slug: "myapp", WritingLanguage: "ko"}, store); err != nil {
		t.Fatalf("init: %v", err)
	}
	cfg, err := config.Load(filepath.Join(target, "ledger", "config.json"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.WritingLanguage != "ko" {
		t.Fatalf("expected writing_language=ko, got %+v", cfg)
	}

	if err := RunInit(target, InitOpts{Slug: "myapp", WritingLanguage: "en"}, store); err != nil {
		t.Fatalf("re-init: %v", err)
	}
	cfg, _ = config.Load(filepath.Join(target, "ledger", "config.json"))
	if cfg.WritingLanguage != "en" {
		t.Fatalf("expected re-init to update writing_language=en, got %+v", cfg)
	}
}

func TestRunInit_WritingLanguagePreservesLegacyConfigShape(t *testing.T) {
	target := t.TempDir()
	regDir := t.TempDir()
	store := registry.New(filepath.Join(regDir, "registry.json"), filepath.Join(regDir, "registry.lock"))
	if err := os.MkdirAll(filepath.Join(target, "ledger"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(target, "ledger", "config.json")
	legacy := `{
  "version": 1,
  "project_id": "9f8a7c6b5d4e3f2a1b0c9d8e7f6a5b4c",
  "slug": "legacy",
  "name": "Legacy",
  "parents": [
    { "ticket": "MA", "label": "Milestone A", "match": ["^as-ma-"] }
  ],
  "generated": { "ticketTree": "docs/release/TICKETS.md" },
  "branch": { "defaultPrefix": "work" }
}`
	if err := os.WriteFile(configPath, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := RunInit(target, InitOpts{WritingLanguage: "ko"}, store); err != nil {
		t.Fatalf("init: %v", err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	for _, needle := range []string{`"writing_language": "ko"`, `"generated"`, `"ticketTree"`, `"branch"`, `"label": "Milestone A"`} {
		if !contains(string(data), needle) {
			t.Fatalf("config lost %q:\n%s", needle, string(data))
		}
	}
}

func TestRunInit_UpdatesGitignore(t *testing.T) {
	target := t.TempDir()
	regDir := t.TempDir()
	store := registry.New(filepath.Join(regDir, "registry.json"), filepath.Join(regDir, "registry.lock"))

	RunInit(target, InitOpts{Slug: "myapp"}, store)

	data, err := os.ReadFile(filepath.Join(target, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	for _, needle := range []string{"ledger/.lock", "ledger/.backup/", "ledger/import-errors.jsonl"} {
		if !contains(string(data), needle) {
			t.Fatalf(".gitignore missing %q; got:\n%s", needle, data)
		}
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && idx(s, sub) >= 0 }
func idx(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
