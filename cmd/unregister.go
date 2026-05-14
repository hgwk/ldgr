package cmd

import (
	"fmt"
	"io"

	"github.com/hgwk/ldgr/internal/registry"
)

func init() {
	Commands["unregister"] = func(args []string, stdout, stderr io.Writer) int {
		store, _, err := DefaultRegistry()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return RunUnregisterCLI(args, store, stdout, stderr)
	}
}

func RunUnregisterCLI(args []string, store *registry.Store, stdout, stderr io.Writer) int {
	fs := newFlagSet("unregister")
	path := fs.String("path", "", "")
	id := fs.String("project-id", "", "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if (*path == "" && *id == "") || (*path != "" && *id != "") {
		fmt.Fprintln(stderr, "specify exactly one of --path or --project-id")
		return 2
	}
	if *path != "" {
		if err := store.UnregisterPath(*path); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	} else {
		if err := store.UnregisterID(*id); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	fmt.Fprintln(stdout, "ok")
	return 0
}
