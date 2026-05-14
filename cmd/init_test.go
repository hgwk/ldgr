package cmd

import (
	"os"
	"path/filepath"
	"testing"

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
	}
	for _, p := range mustExist {
		if _, err := os.Stat(filepath.Join(target, p)); err != nil {
			t.Fatalf("expected %s to exist: %v", p, err)
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
