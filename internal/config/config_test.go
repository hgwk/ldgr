package config

import (
	"encoding/json"
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
		WritingLanguage:  "ko",
	}
	if err := Save(p, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.ProjectID != in.ProjectID || got.Slug != in.Slug || got.WritingLanguage != "ko" || len(got.Parents) != 2 {
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

func TestSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	if err := os.WriteFile(p, []byte(`{
  "schema_version": 1,
  "project_id": "9f8a7c6b5d4e3f2a1b0c9d8e7f6a5b4c",
  "slug": "state"
}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	version, err := SchemaVersion(p)
	if err != nil {
		t.Fatalf("schema version: %v", err)
	}
	if version != 1 {
		t.Fatalf("expected schema version 1, got %d", version)
	}
}

func TestSchemaVersion_DefaultsMissingVersionToV1(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	if err := os.WriteFile(p, []byte(`{
  "project_id": "9f8a7c6b5d4e3f2a1b0c9d8e7f6a5b4c",
  "slug": "legacy"
}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	version, err := SchemaVersion(p)
	if err != nil {
		t.Fatalf("schema version: %v", err)
	}
	if version != 1 {
		t.Fatalf("expected missing schema version to default to v1, got %d", version)
	}
}

func TestSchemaVersion_MalformedConfigFails(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	if err := os.WriteFile(p, []byte(`{"schema_version":`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := SchemaVersion(p); err == nil {
		t.Fatalf("expected malformed config to fail")
	}
}

func TestPatchWritingLanguage_PreservesUnknownFields(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	if err := os.WriteFile(p, []byte(`{
  "version": 1,
  "project_id": "9f8a7c6b5d4e3f2a1b0c9d8e7f6a5b4c",
  "slug": "legacy",
  "parents": [
    { "ticket": "MA", "label": "Milestone A", "match": ["^as-ma-"] }
  ],
  "generated": { "ticketTree": "docs/release/TICKETS.md" },
  "branch": { "defaultPrefix": "work" }
}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := PatchWritingLanguage(p, "ko"); err != nil {
		t.Fatalf("patch: %v", err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json: %v\n%s", err, string(data))
	}
	if string(raw["writing_language"]) != `"ko"` {
		t.Fatalf("writing_language not patched: %s", raw["writing_language"])
	}
	if _, ok := raw["generated"]; !ok {
		t.Fatalf("generated field was dropped: %s", string(data))
	}
	if _, ok := raw["branch"]; !ok {
		t.Fatalf("branch field was dropped: %s", string(data))
	}
	var parents []map[string]any
	if err := json.Unmarshal(raw["parents"], &parents); err != nil {
		t.Fatalf("parents changed shape: %v\n%s", err, string(raw["parents"]))
	}
	if parents[0]["label"] != "Milestone A" {
		t.Fatalf("legacy parent object not preserved: %+v", parents)
	}
}
