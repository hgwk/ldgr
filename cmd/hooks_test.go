package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeHook(t *testing.T, dir, content string) {
	t.Helper()
	p := filepath.Join(dir, ".git", "hooks")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(p, "pre-commit"), []byte(content), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func readHook(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".git", "hooks", "pre-commit"))
	if err != nil {
		return ""
	}
	return string(data)
}

func TestHooksInstall_CreatesNewHookWithMarker(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)

	if code := RunHooksCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("install failed")
	}
	hook := readHook(t, dir)
	if !strings.HasPrefix(hook, "#!") {
		n := 20
		if len(hook) < n {
			n = len(hook)
		}
		t.Fatalf("hook should start with shebang: %q", hook[:n])
	}
	if !strings.Contains(hook, "LDGR_HOOK_START") || !strings.Contains(hook, "LDGR_HOOK_END") {
		t.Fatalf("missing markers: %s", hook)
	}
	if !strings.Contains(hook, "ldgr verify") {
		t.Fatalf("missing verify call: %s", hook)
	}
}

func TestHooksInstall_PreservesExistingHook(t *testing.T) {
	dir := t.TempDir()
	writeHook(t, dir, "#!/usr/bin/env bash\necho user hook ran\n")

	if code := RunHooksCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("install failed")
	}
	hook := readHook(t, dir)
	if !strings.Contains(hook, "user hook ran") {
		t.Fatalf("existing user content lost: %s", hook)
	}
	if !strings.Contains(hook, "LDGR_HOOK_START") {
		t.Fatalf("missing ldgr marker: %s", hook)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "hooks", "pre-commit.ldgr.bak")); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
}

func TestHooksInstall_Idempotent(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	RunHooksCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{})
	first := readHook(t, dir)
	RunHooksCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{})
	second := readHook(t, dir)
	if first != second {
		t.Fatalf("re-install changed hook:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestHooksUninstall_PreservesUserContent(t *testing.T) {
	dir := t.TempDir()
	writeHook(t, dir, "#!/usr/bin/env bash\necho user hook ran\n")
	RunHooksCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{})
	if code := RunHooksCLI([]string{"uninstall", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("uninstall failed")
	}
	hook := readHook(t, dir)
	if !strings.Contains(hook, "user hook ran") {
		t.Fatalf("user content lost: %s", hook)
	}
	if strings.Contains(hook, "LDGR_HOOK_START") {
		t.Fatalf("marker survived uninstall: %s", hook)
	}
}

func TestHooksUninstall_DeletesEmptyHook(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	RunHooksCLI([]string{"install", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{})
	if code := RunHooksCLI([]string{"uninstall", "--target", dir}, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("uninstall failed")
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "hooks", "pre-commit")); !os.IsNotExist(err) {
		t.Fatalf("hook should be deleted, stat err=%v", err)
	}
}
