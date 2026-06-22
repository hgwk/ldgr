package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/hgwk/ldgr/internal/coordination"
	"github.com/hgwk/ldgr/internal/ledger"
)

func TestClaimAddReleaseAndNoteAdd(t *testing.T) {
	t.Setenv("LEDGER_AGENT", "codex")
	target := t.TempDir()
	if err := os.MkdirAll(filepath.Join(target, "ledger"), 0o755); err != nil {
		t.Fatalf("mkdir ledger: %v", err)
	}

	var out, err bytes.Buffer
	code := RunClaimCLI([]string{
		"add", "--target", target, "--ticket", "T-1", "--resource", "src/api",
		"--lane", "api", "--team", "platform", "--summary", "touch api",
	}, &out, &err)
	if code != 0 {
		t.Fatalf("claim add failed: code=%d stderr=%s", code, err.String())
	}
	rows, readErr := ledger.ReadRows(coordination.Path(target))
	if readErr != nil {
		t.Fatalf("read coordination: %v", readErr)
	}
	if len(rows) != 1 || rows[0]["type"] != "claim" || rows[0]["owner"] != "codex" {
		t.Fatalf("unexpected claim row: %+v", rows)
	}

	out.Reset()
	err.Reset()
	code = RunNoteCLI([]string{"add", "--target", target, "--kind", "decision", "--scope", "api", "--ticket", "T-1", "--summary", "keep v2"}, &out, &err)
	if code != 0 {
		t.Fatalf("note add failed: code=%d stderr=%s", code, err.String())
	}

	out.Reset()
	err.Reset()
	code = RunClaimCLI([]string{"release", "--target", target, "--ticket", "T-1", "--summary", "done"}, &out, &err)
	if code != 0 {
		t.Fatalf("claim release failed: code=%d stderr=%s", code, err.String())
	}
	rows, _ = ledger.ReadRows(coordination.Path(target))
	if len(rows) != 3 || rows[2]["type"] != "release" {
		t.Fatalf("expected claim/note/release rows, got %+v", rows)
	}
}
