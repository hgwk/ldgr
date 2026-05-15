package ledger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestAppend_AssignsN(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tickets.jsonl")
	lock := filepath.Join(dir, ".lock")

	r1, err := Append(p, lock, Row{"ticket": "a"})
	if err != nil {
		t.Fatalf("append1: %v", err)
	}
	if r1["n"].(int) != 1 {
		t.Fatalf("expected n=1, got %v", r1["n"])
	}
	r2, err := Append(p, lock, Row{"ticket": "b"})
	if err != nil {
		t.Fatalf("append2: %v", err)
	}
	if r2["n"].(int) != 2 {
		t.Fatalf("expected n=2, got %v", r2["n"])
	}
}

func TestAppend_PersistsToFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tickets.jsonl")
	lock := filepath.Join(dir, ".lock")

	Append(p, lock, Row{"ticket": "a"})
	Append(p, lock, Row{"ticket": "b"})

	rows, err := ReadRows(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	// n is decoded from JSON as float64 — verify both values present.
	if rows[0]["n"].(float64) != 1 || rows[1]["n"].(float64) != 2 {
		t.Fatalf("n values wrong: %v %v", rows[0]["n"], rows[1]["n"])
	}
}

func TestAppend_OverridesCallerSuppliedN(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tickets.jsonl")
	lock := filepath.Join(dir, ".lock")

	r, err := Append(p, lock, Row{"ticket": "a", "n": 99})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if r["n"].(int) != 1 {
		t.Fatalf("Append must assign n authoritatively, got %v", r["n"])
	}
}

func TestAppend_BumpsTimestampAfterLastRow(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tickets.jsonl")
	lock := filepath.Join(dir, ".lock")

	if _, err := Append(p, lock, Row{"ticket": "a", "ts": "2026-05-15T22:35:00Z"}); err != nil {
		t.Fatalf("append1: %v", err)
	}
	r, err := Append(p, lock, Row{"ticket": "b", "ts": "2026-05-15T01:12:57Z"})
	if err != nil {
		t.Fatalf("append2: %v", err)
	}
	if got := r["ts"]; got != "2026-05-15T22:35:01Z" {
		t.Fatalf("expected bumped ts, got %v", got)
	}
}

func TestAppend_BumpsTimestampAfterFractionalLastRow(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tickets.jsonl")
	lock := filepath.Join(dir, ".lock")

	if _, err := Append(p, lock, Row{"ticket": "a", "ts": "2026-05-15T22:35:00.500Z"}); err != nil {
		t.Fatalf("append1: %v", err)
	}
	r, err := Append(p, lock, Row{"ticket": "b", "ts": "2026-05-15T22:35:00Z"})
	if err != nil {
		t.Fatalf("append2: %v", err)
	}
	if got := r["ts"]; got != "2026-05-15T22:35:01Z" {
		t.Fatalf("expected bumped ts after fractional row, got %v", got)
	}
}

func TestAppend_RemovesLockOnSuccess(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tickets.jsonl")
	lock := filepath.Join(dir, ".lock")
	Append(p, lock, Row{"ticket": "a"})
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Fatalf("lock should be gone after Append, stat err=%v", err)
	}
}

func TestAppend_ConcurrentWritersProduceConsecutiveN(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tickets.jsonl")
	lock := filepath.Join(dir, ".lock")

	const N = 50
	errs := make(chan error, N)
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			if _, err := Append(p, lock, Row{"ticket": fmt.Sprintf("t-%d", i)}); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent append failed: %v", err)
	}

	rows, err := ReadRows(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rows) != N {
		t.Fatalf("expected %d rows, got %d", N, len(rows))
	}
	// n must be 1..N consecutive with no gaps or duplicates.
	seen := make(map[int]bool)
	for i, r := range rows {
		n, ok := r["n"].(float64)
		if !ok {
			t.Fatalf("row %d has no numeric n: %+v", i, r)
		}
		ni := int(n)
		if ni < 1 || ni > N {
			t.Fatalf("row %d: n out of range: %d", i, ni)
		}
		if seen[ni] {
			t.Fatalf("duplicate n=%d at row %d", ni, i)
		}
		seen[ni] = true
	}
	for i := 1; i <= N; i++ {
		if !seen[i] {
			t.Fatalf("missing n=%d in output", i)
		}
	}
}
