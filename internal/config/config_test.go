package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoundtrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	in := Config{
		SchemaVersion:    1,
		ProjectID:        "9f8a7c6b5d4e3f2a1b0c9d8e7f6a5b4c",
		Slug:             "myapp",
		Name:             "My App",
		Parents:          []string{"ROOT", "BUG"},
		BranchConvention: "work/{ticket}",
		LogGoalChanges:   false,
	}
	if err := Save(p, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.ProjectID != in.ProjectID || got.Slug != in.Slug || len(got.Parents) != 2 {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
}

func TestDefault(t *testing.T) {
	c := Default("myapp", "9f8a7c6b5d4e3f2a1b0c9d8e7f6a5b4c", "")
	if c.SchemaVersion != 1 {
		t.Fatalf("schema_version should default to 1, got %d", c.SchemaVersion)
	}
	if c.Slug != "myapp" || c.Name != "myapp" {
		t.Fatalf("name should default to slug, got slug=%s name=%s", c.Slug, c.Name)
	}
	if c.BranchConvention != "work/{ticket}" {
		t.Fatalf("unexpected branch convention: %s", c.BranchConvention)
	}
	if len(c.Parents) == 0 {
		t.Fatalf("default parents should be non-empty")
	}
}

func TestLoad_LegacyParentObjects(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	if err := os.WriteFile(p, []byte(`{
  "version": 1,
  "project_id": "9f8a7c6b5d4e3f2a1b0c9d8e7f6a5b4c",
  "slug": "legacy",
  "parents": [
    { "ticket": "MA", "label": "Milestone A" },
    { "ticket": "BUG", "label": "Bugs" }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.SchemaVersion != 1 {
		t.Fatalf("legacy version should map to schema_version, got %d", got.SchemaVersion)
	}
	if len(got.Parents) != 2 || got.Parents[0] != "MA" || got.Parents[1] != "BUG" {
		t.Fatalf("legacy parents not normalized: %+v", got.Parents)
	}
}
