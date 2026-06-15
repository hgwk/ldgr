package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hgwk/ldgr/internal/locks"
	"github.com/hgwk/ldgr/internal/registry"
)

func TestRegistryRepair_BacksUpAndRebuilds(t *testing.T) {
	regDir := t.TempDir()
	regPath := filepath.Join(regDir, "registry.json")
	os.WriteFile(regPath, []byte("not json"), 0o644)

	store := registry.New(regPath, filepath.Join(regDir, "registry.lock"))
	out := &bytes.Buffer{}
	if code := RunRegistryCLI([]string{"repair"}, store, regPath, out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("repair failed")
	}

	entries, _ := os.ReadDir(regDir)
	foundBackup := false
	for _, e := range entries {
		if name := e.Name(); name != "registry.json" && name != "registry.lock" && filepath.Ext(name) == ".json" {
			foundBackup = true
		}
	}
	if !foundBackup {
		t.Fatalf("expected backup file in %s; got: %v", regDir, entries)
	}

	r, err := store.Load()
	if err != nil {
		t.Fatalf("load after repair: %v", err)
	}
	if len(r.Projects) != 0 || r.Version != 1 {
		t.Fatalf("expected fresh registry, got %+v", r)
	}
}

func acquireRegistryLock(t *testing.T, store *registry.Store) (func() error, error) {
	t.Helper()
	return locks.Acquire(store.LockPath(), locks.Options{TotalWait: 100 * time.Millisecond})
}

func TestRegistryRepair_HoldsLockDuringRebuild(t *testing.T) {
	regDir := t.TempDir()
	regPath := filepath.Join(regDir, "registry.json")
	store := registry.New(regPath, filepath.Join(regDir, "registry.lock"))

	// Pre-acquire the lock to simulate a concurrent writer.
	release, err := acquireRegistryLock(t, store)
	if err != nil {
		t.Fatalf("pre-acquire: %v", err)
	}
	defer release()

	// Repair should fail to acquire (we're holding the lock).
	done := make(chan int, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		code := RunRegistryCLI([]string{"repair"}, store, regPath, &bytes.Buffer{}, &bytes.Buffer{})
		done <- code
	}()
	wg.Wait()
	code := <-done
	if code == 0 {
		t.Fatalf("expected repair to fail while lock is held by another writer")
	}
}

func TestRegistryList_ShowsMissingPaths(t *testing.T) {
	regDir := t.TempDir()
	regPath := filepath.Join(regDir, "registry.json")
	store := registry.New(regPath, filepath.Join(regDir, "registry.lock"))
	if err := store.Register(registry.Project{
		ProjectID: "id1",
		Slug:      "ghost",
		Paths:     []string{"/nonexistent/path/x"},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	out := &bytes.Buffer{}
	if code := RunRegistryCLI([]string{"list"}, store, regPath, out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("list failed")
	}
	if !strings.Contains(out.String(), "missing") {
		t.Fatalf("expected missing marker, got: %s", out.String())
	}
}

func TestRegistryPrune_RemovesMissingPaths(t *testing.T) {
	regDir := t.TempDir()
	regPath := filepath.Join(regDir, "registry.json")
	store := registry.New(regPath, filepath.Join(regDir, "registry.lock"))
	if err := store.Register(registry.Project{
		ProjectID: "id1",
		Slug:      "ghost",
		Paths:     []string{"/nonexistent/path/x"},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	out := &bytes.Buffer{}
	if code := RunRegistryCLI([]string{"prune"}, store, regPath, out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("prune failed")
	}
	r, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(r.Projects) != 0 {
		t.Fatalf("expected pruned registry, got %+v", r)
	}
}

func TestRegistryPrune_DryRunKeepsMissingPaths(t *testing.T) {
	regDir := t.TempDir()
	regPath := filepath.Join(regDir, "registry.json")
	store := registry.New(regPath, filepath.Join(regDir, "registry.lock"))
	if err := store.Register(registry.Project{
		ProjectID: "id1",
		Slug:      "ghost",
		Paths:     []string{"/nonexistent/path/x"},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	out := &bytes.Buffer{}
	if code := RunRegistryCLI([]string{"prune", "--dry-run"}, store, regPath, out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("prune dry-run failed")
	}
	r, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(r.Projects) != 1 {
		t.Fatalf("dry-run should keep registry, got %+v", r)
	}
	if !strings.Contains(out.String(), "would prune") {
		t.Fatalf("expected dry-run text, got %s", out.String())
	}
}

func TestRegistryList_JSON(t *testing.T) {
	regDir := t.TempDir()
	regPath := filepath.Join(regDir, "registry.json")
	store := registry.New(regPath, filepath.Join(regDir, "registry.lock"))
	if err := store.Register(registry.Project{
		ProjectID: "id1",
		Slug:      "ghost",
		Paths:     []string{"/nonexistent/path/x"},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	out := &bytes.Buffer{}
	if code := RunRegistryCLI([]string{"list", "--json"}, store, regPath, out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("list json failed")
	}
	var payload registryListSummary
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out.String())
	}
	if payload.SchemaVersion != 1 || payload.MissingCount != 1 {
		t.Fatalf("unexpected summary: %+v", payload)
	}
	if len(payload.Projects) != 1 || payload.Projects[0].Paths[0].Status != "missing" {
		t.Fatalf("unexpected project status: %+v", payload.Projects)
	}
}

func TestRegistryPrune_JSONSummary(t *testing.T) {
	regDir := t.TempDir()
	regPath := filepath.Join(regDir, "registry.json")
	store := registry.New(regPath, filepath.Join(regDir, "registry.lock"))
	if err := store.Register(registry.Project{
		ProjectID: "id1",
		Slug:      "ghost",
		Paths:     []string{"/nonexistent/path/x"},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	out := &bytes.Buffer{}
	if code := RunRegistryCLI([]string{"prune", "--dry-run", "--json"}, store, regPath, out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("prune json failed")
	}
	var payload registryPruneSummary
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out.String())
	}
	if payload.SchemaVersion != 1 || !payload.DryRun || payload.PrunedCount != 1 || payload.ProjectCount != 1 {
		t.Fatalf("unexpected prune summary: %+v", payload)
	}
	if len(payload.Projects) != 1 || payload.Projects[0].ProjectID != "id1" {
		t.Fatalf("expected project id1, got %+v", payload.Projects)
	}
}

func TestRegistryList_SortsByLastSeenDesc(t *testing.T) {
	projects := []registry.Project{
		{ProjectID: "old", LastSeen: "2026-01-01T00:00:00Z"},
		{ProjectID: "new", LastSeen: "2026-06-01T00:00:00Z"},
	}
	sortProjectsByLastSeen(projects)
	if projects[0].ProjectID != "new" {
		t.Fatalf("expected newest project first, got %+v", projects)
	}
}
