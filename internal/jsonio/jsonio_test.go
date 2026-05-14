package jsonio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteThenRead(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.json")
	in := map[string]any{"a": "b", "n": float64(1)}
	if err := WriteJSON(p, in); err != nil {
		t.Fatalf("write: %v", err)
	}
	var out map[string]any
	if err := ReadJSON(p, &out); err != nil {
		t.Fatalf("read: %v", err)
	}
	if out["a"] != "b" || out["n"] != float64(1) {
		t.Fatalf("roundtrip mismatch: %v", out)
	}
}

func TestWriteJSON_AtomicNoPartial(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.json")
	// First write establishes file.
	if err := WriteJSON(p, map[string]string{"a": "1"}); err != nil {
		t.Fatalf("write1: %v", err)
	}
	// Second write replaces atomically (we can't easily induce a partial
	// write in a unit test, but verify no .tmp file is left behind).
	if err := WriteJSON(p, map[string]string{"a": "2"}); err != nil {
		t.Fatalf("write2: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}
