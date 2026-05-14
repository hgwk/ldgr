package registry

import (
	"path/filepath"
	"testing"
)

func TestRegister_NewProject(t *testing.T) {
	dir := t.TempDir()
	store := New(filepath.Join(dir, "registry.json"), filepath.Join(dir, "registry.lock"))

	p := Project{
		ProjectID: "id1", Slug: "a", Name: "A",
		Paths: []string{"/p/a"},
	}
	if err := store.Register(p); err != nil {
		t.Fatalf("register: %v", err)
	}

	reg, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(reg.Projects) != 1 || reg.Projects[0].ProjectID != "id1" {
		t.Fatalf("unexpected reg: %+v", reg)
	}
}

func TestRegister_AppendsPathToExistingProject(t *testing.T) {
	dir := t.TempDir()
	store := New(filepath.Join(dir, "registry.json"), filepath.Join(dir, "registry.lock"))

	store.Register(Project{ProjectID: "id1", Slug: "a", Paths: []string{"/p/a"}})
	store.Register(Project{ProjectID: "id1", Slug: "a", Paths: []string{"/p/a2"}})

	reg, _ := store.Load()
	if len(reg.Projects) != 1 {
		t.Fatalf("should still be 1 project, got %d", len(reg.Projects))
	}
	if len(reg.Projects[0].Paths) != 2 {
		t.Fatalf("expected 2 paths, got %v", reg.Projects[0].Paths)
	}
}

func TestRegister_DedupesPaths(t *testing.T) {
	dir := t.TempDir()
	store := New(filepath.Join(dir, "registry.json"), filepath.Join(dir, "registry.lock"))

	store.Register(Project{ProjectID: "id1", Paths: []string{"/p/a"}})
	store.Register(Project{ProjectID: "id1", Paths: []string{"/p/a"}})

	reg, _ := store.Load()
	if len(reg.Projects[0].Paths) != 1 {
		t.Fatalf("duplicate path should be deduped: %v", reg.Projects[0].Paths)
	}
}

func TestUnregisterPath(t *testing.T) {
	dir := t.TempDir()
	store := New(filepath.Join(dir, "registry.json"), filepath.Join(dir, "registry.lock"))

	store.Register(Project{ProjectID: "id1", Paths: []string{"/p/a", "/p/b"}})
	if err := store.UnregisterPath("/p/a"); err != nil {
		t.Fatalf("unregister path: %v", err)
	}
	reg, _ := store.Load()
	if len(reg.Projects) != 1 || len(reg.Projects[0].Paths) != 1 || reg.Projects[0].Paths[0] != "/p/b" {
		t.Fatalf("unexpected state: %+v", reg)
	}
}

func TestUnregisterPath_LastPathRemovesProject(t *testing.T) {
	dir := t.TempDir()
	store := New(filepath.Join(dir, "registry.json"), filepath.Join(dir, "registry.lock"))

	store.Register(Project{ProjectID: "id1", Paths: []string{"/p/a"}})
	store.UnregisterPath("/p/a")

	reg, _ := store.Load()
	if len(reg.Projects) != 0 {
		t.Fatalf("project should be removed when last path goes: %+v", reg)
	}
}

func TestUnregisterByID(t *testing.T) {
	dir := t.TempDir()
	store := New(filepath.Join(dir, "registry.json"), filepath.Join(dir, "registry.lock"))

	store.Register(Project{ProjectID: "id1", Paths: []string{"/p/a", "/p/b"}})
	store.UnregisterID("id1")

	reg, _ := store.Load()
	if len(reg.Projects) != 0 {
		t.Fatalf("project should be removed by id: %+v", reg)
	}
}
