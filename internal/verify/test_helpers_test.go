package verify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hgwk/ldgr/internal/config"
)

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := ensureParent(p); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := writeFile(p, content); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	return dir
}

func ensureParent(p string) error {
	return os.MkdirAll(filepath.Dir(p), 0o755)
}
func writeFile(p, c string) error { return os.WriteFile(p, []byte(c), 0o644) }
func mustJSON(v any) string       { b, _ := json.MarshalIndent(v, "", "  "); return string(b) }
func validConfigJSON() string {
	c := config.Default("myapp", "9f8a7c6b5d4e3f2a1b0c9d8e7f6a5b4c", "")
	return mustJSON(c)
}

func validConfigJSONState() string {
	c := config.Default("myapp", "9f8a7c6b5d4e3f2a1b0c9d8e7f6a5b4c", "")
	c.SchemaVersion = 1
	return mustJSON(c)
}

func validGoalJSON() string {
	return `{"schema_version":1,"track":"project","version":"0.1.0","updated":"2026-05-14T00:00:00Z","source_of_truth":"README.md","summary":"x","success_criteria":[]}`
}

func hasWarn(r Report, code string) bool {
	for _, w := range r.Warns {
		if strings.Contains(w.Message, code) {
			return true
		}
	}
	return false
}

func hasWarnCode(r Report, code string) bool {
	for _, w := range r.Warns {
		if w.Code == code {
			return true
		}
	}
	return false
}

func hasFail(r Report, code string) bool {
	for _, f := range r.Fails {
		if strings.Contains(f.Message, code) {
			return true
		}
	}
	return false
}

func hasFailCode(r Report, code string) bool {
	for _, f := range r.Fails {
		if f.Code == code {
			return true
		}
	}
	return false
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
