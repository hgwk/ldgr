package locks

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquire_Releases(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, ".lock")

	release, err := Acquire(lp, Options{})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if _, err := os.Stat(lp); err != nil {
		t.Fatalf("lock file should exist while held: %v", err)
	}
	if err := release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := os.Stat(lp); !os.IsNotExist(err) {
		t.Fatalf("lock file should be removed after release, stat err=%v", err)
	}
}

func TestAcquire_FailsWhenHeld(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, ".lock")

	release, err := Acquire(lp, Options{})
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer release()

	// Short retry budget so the test is fast.
	_, err = Acquire(lp, Options{TotalWait: 100 * time.Millisecond, RetryEvery: 25 * time.Millisecond})
	if err == nil {
		t.Fatalf("expected busy error")
	}
}

func TestAcquire_ReclaimsStaleLock(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, ".lock")

	// Plant a stale lock file (mtime far in the past).
	if err := os.WriteFile(lp, []byte("stale\n"), 0o644); err != nil {
		t.Fatalf("plant: %v", err)
	}
	old := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(lp, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	release, err := Acquire(lp, Options{StaleAfter: 30 * time.Second})
	if err != nil {
		t.Fatalf("expected stale reclaim to succeed: %v", err)
	}
	defer release()
}
