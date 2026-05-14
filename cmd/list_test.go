package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hgwk/ldgr/internal/registry"
)

func TestList_PrintsRegistered(t *testing.T) {
	target, store := mustInit(t)
	out := &bytes.Buffer{}
	if code := RunListCLI([]string{}, store, out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("list failed")
	}
	if !strings.Contains(out.String(), target) {
		t.Fatalf("expected %q in output, got: %s", target, out.String())
	}
}

func TestList_Prune_RemovesMissingPaths(t *testing.T) {
	regDir := t.TempDir()
	store := registry.New(filepath.Join(regDir, "registry.json"), filepath.Join(regDir, "registry.lock"))
	store.Register(registry.Project{ProjectID: "id1", Slug: "ghost", Paths: []string{"/nonexistent/path/x"}})

	out := &bytes.Buffer{}
	if code := RunListCLI([]string{"--prune"}, store, out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("prune failed")
	}
	r, _ := store.Load()
	if len(r.Projects) != 0 {
		t.Fatalf("expected pruned, got %d projects", len(r.Projects))
	}
}
