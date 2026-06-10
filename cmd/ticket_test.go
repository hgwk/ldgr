package cmd

import (
	"path/filepath"
	"testing"

	"github.com/hgwk/ldgr/internal/config"
	"github.com/hgwk/ldgr/internal/registry"
)

func mustInit(t *testing.T) (target string, store *registry.Store) {
	t.Helper()
	t.Setenv("LDGR_HOME", t.TempDir())
	target = t.TempDir()
	regDir := t.TempDir()
	store = registry.New(filepath.Join(regDir, "registry.json"), filepath.Join(regDir, "registry.lock"))
	if err := RunInit(target, InitOpts{Slug: "myapp"}, store); err != nil {
		t.Fatalf("init: %v", err)
	}
	return
}

func mustInitState(t *testing.T) string {
	t.Helper()
	target, _ := mustInit(t)
	cfgPath := filepath.Join(target, "ledger", "config.json")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.SchemaVersion = 1
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	return target
}
