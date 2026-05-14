package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/hgwk/ldgr/internal/ids"
	"github.com/hgwk/ldgr/internal/registry"
)

func init() {
	Commands["list"] = func(args []string, stdout, stderr io.Writer) int {
		store, _, err := DefaultRegistry()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return RunListCLI(args, store, stdout, stderr)
	}
}

func RunListCLI(args []string, store *registry.Store, stdout, stderr io.Writer) int {
	fs := newFlagSet("list")
	prune := fs.Bool("prune", false, "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	r, err := store.Load()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *prune {
		var toRemove []string
		for _, p := range r.Projects {
			for _, path := range p.Paths {
				if !configExists(path) {
					toRemove = append(toRemove, path)
				}
			}
		}
		for _, path := range toRemove {
			if err := store.UnregisterPath(path); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			fmt.Fprintf(stdout, "pruned %s (path missing or no ledger/config.json)\n", path)
		}
		r, _ = store.Load()
	}
	for _, p := range r.Projects {
		fmt.Fprintf(stdout, "%s\n", ids.Display(p.Slug, p.ProjectID))
		for _, path := range p.Paths {
			fmt.Fprintf(stdout, "  %s\n", path)
		}
	}
	return 0
}

func configExists(repoPath string) bool {
	_, err := os.Stat(filepath.Join(repoPath, "ledger", "config.json"))
	return err == nil
}
