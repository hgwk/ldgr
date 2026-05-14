package e2e

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportLegacy_EndToEnd(t *testing.T) {
	bin := buildBinary(t)
	work := t.TempDir()
	home := t.TempDir()

	fixDir := filepath.Join(repoRoot(t), "e2e", "fixtures", "legacy")
	if err := copyTree(fixDir, work); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}

	envBase := append(os.Environ(), "LDGR_HOME="+home, "LEDGER_AGENT=codex")
	runEnv := func(args ...string) (string, string, int) {
		c := exec.Command(bin, args...)
		c.Env = envBase
		var so, se bytes.Buffer
		c.Stdout = &so
		c.Stderr = &se
		err := c.Run()
		code := 0
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else if err != nil {
			t.Fatalf("run %v: %v", args, err)
		}
		return so.String(), se.String(), code
	}

	// Init first so config exists.
	if _, se, code := runEnv("init", "--target", work, "--slug", "fixture"); code != 0 {
		t.Fatalf("init: %s", se)
	}

	beforePlanHash := treeHash(t, work)

	if so, se, code := runEnv("import", "legacy", "--target", work, "--plan"); code != 0 {
		t.Fatalf("plan: stdout=%s stderr=%s", so, se)
	} else if !strings.Contains(so, "Legacy import plan") {
		t.Fatalf("missing plan banner: %s", so)
	}

	afterPlanHash := treeHash(t, work)
	if beforePlanHash != afterPlanHash {
		t.Fatalf("--plan must not change the filesystem")
	}

	if _, se, code := runEnv("import", "legacy", "--target", work, "--apply"); code != 0 {
		t.Fatalf("apply: %s", se)
	}

	for _, want := range []string{"ledger/tickets.jsonl", "ledger/worklog.jsonl", "ledger/goal.json"} {
		if _, err := os.Stat(filepath.Join(work, want)); err != nil {
			t.Fatalf("missing %s: %v", want, err)
		}
	}

	if so, se, code := runEnv("verify", "--target", work); code != 0 {
		t.Fatalf("verify after import: stdout=%s stderr=%s", so, se)
	}

	// Re-running apply must produce "no changes".
	if so, _, code := runEnv("import", "legacy", "--target", work, "--apply"); code != 0 || !strings.Contains(so, "no changes") {
		t.Fatalf("second apply should be no-op: code=%d stdout=%s", code, so)
	}
}

func treeHash(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		fmt.Fprintf(h, "%s\n", rel)
		if !d.IsDir() {
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			io.Copy(h, f)
			f.Close()
		}
		return nil
	})
	return fmt.Sprintf("%x", h.Sum(nil))
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}
