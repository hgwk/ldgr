package cmd

import (
	"os"
	"path/filepath"

	"github.com/hgwk/ldgr/internal/registry"
)

// DefaultRegistry returns a Store rooted at $HOME/.ldgr/registry.json.
// LDGR_HOME overrides the directory (used by tests).
func DefaultRegistry() (*registry.Store, string, error) {
	home := os.Getenv("LDGR_HOME")
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return nil, "", err
		}
		home = filepath.Join(h, ".ldgr")
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return nil, "", err
	}
	regPath := filepath.Join(home, "registry.json")
	lockPath := filepath.Join(home, "registry.lock")
	return registry.New(regPath, lockPath), regPath, nil
}
