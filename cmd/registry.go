package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/hgwk/ldgr/internal/jsonio"
	"github.com/hgwk/ldgr/internal/locks"
	"github.com/hgwk/ldgr/internal/registry"
)

func init() {
	Commands["registry"] = func(args []string, stdout, stderr io.Writer) int {
		store, regPath, err := DefaultRegistry()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return RunRegistryCLI(args, store, regPath, stdout, stderr)
	}
}

func RunRegistryCLI(args []string, store *registry.Store, registryPath string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "repair" {
		fmt.Fprintln(stderr, "usage: ldgr registry repair")
		return 2
	}
	release, err := locks.Acquire(store.LockPath(), locks.Options{})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer release()

	data, err := os.ReadFile(registryPath)
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if len(data) > 0 {
		bak := filepath.Join(filepath.Dir(registryPath), fmt.Sprintf("registry.corrupt-%s.json", time.Now().UTC().Format("20060102-150405")))
		if err := os.WriteFile(bak, data, 0o644); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "backed up old registry to %s\n", bak)
	}
	if err := jsonio.WriteJSON(registryPath, registry.Registry{Version: 1}); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, "registry rebuilt (empty)")
	return 0
}
