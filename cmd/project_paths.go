package cmd

import "path/filepath"

func ldgrLockPath(dir string) string {
	return filepath.Join(dir, ".ldgr", "lock")
}
