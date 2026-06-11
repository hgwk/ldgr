package cmd

import (
	"fmt"
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
		return nil, "", fmt.Errorf("create ldgr home %s: %w; set LDGR_HOME or pass --home to init", home, err)
	}
	regPath := filepath.Join(home, "registry.json")
	lockPath := filepath.Join(home, "registry.lock")
	return registry.New(regPath, lockPath), regPath, nil
}

func setLDGRHomeOverride(home string) (func(), error) {
	if home == "" {
		return func() {}, nil
	}
	abs, err := filepath.Abs(home)
	if err != nil {
		return nil, err
	}
	old, hadOld := os.LookupEnv("LDGR_HOME")
	if err := os.Setenv("LDGR_HOME", abs); err != nil {
		return nil, err
	}
	return func() {
		if hadOld {
			_ = os.Setenv("LDGR_HOME", old)
		} else {
			_ = os.Unsetenv("LDGR_HOME")
		}
	}, nil
}
