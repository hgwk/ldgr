package ledger

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadRows_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.jsonl")
	if err := os.WriteFile(p, []byte(""), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	rows, err := ReadRows(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
}

func TestReadRows_MissingFile(t *testing.T) {
	dir := t.TempDir()
	rows, err := ReadRows(filepath.Join(dir, "absent.jsonl"))
	if err != nil {
		t.Fatalf("read of missing file should be nil error, got %v", err)
	}
	if rows != nil && len(rows) != 0 {
		t.Fatalf("expected empty rows for missing file")
	}
}

func TestReadRows_MultipleRows(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.jsonl")
	content := `{"n":1,"ticket":"a"}
{"n":2,"ticket":"b"}
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	rows, err := ReadRows(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rows) != 2 || rows[0]["ticket"] != "a" || rows[1]["ticket"] != "b" {
		t.Fatalf("unexpected rows: %v", rows)
	}
}

func TestReadRows_SkipsBlankLines(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.jsonl")
	content := "\n{\"n\":1}\n\n{\"n\":2}\n"
	os.WriteFile(p, []byte(content), 0o644)
	rows, err := ReadRows(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

func TestReadRows_ReportsParseErrorLine(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.jsonl")
	content := `{"n":1}
not json
`
	os.WriteFile(p, []byte(content), 0o644)
	_, err := ReadRows(p)
	if err == nil {
		t.Fatalf("expected parse error")
	}
	// Error message should mention the line number for debuggability.
	if got := err.Error(); !contains(got, "line 2") {
		t.Fatalf("error should mention line 2, got %q", got)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && indexOf(s, sub) >= 0 }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
