package cmd

import (
	"bytes"
	"os"
	"path/filepath"
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
