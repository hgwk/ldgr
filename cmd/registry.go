package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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
	if len(args) == 0 {
		printRegistryUsage(stderr)
		return 2
	}
	switch args[0] {
	case "list":
		return runRegistryList(args[1:], store, stdout, stderr)
	case "prune":
		return runRegistryPrune(args[1:], store, stdout, stderr)
	case "repair":
		return runRegistryRepair(store, registryPath, stdout, stderr)
	default:
		printRegistryUsage(stderr)
		return 2
	}
}

func printRegistryUsage(w io.Writer) {
	fmt.Fprintln(w, `usage: ldgr registry <subcommand> [args]

Subcommands:
  list [--json]          list registered projects, newest first
  prune [--dry-run]      remove paths whose ledger/config.json is missing
  prune --json           emit a schema_versioned prune summary
  repair                 rebuild a corrupt registry after backing it up`)
}

func runRegistryList(args []string, store *registry.Store, stdout, stderr io.Writer) int {
	fs := newFlagSet("registry list")
	jsonOut := fs.Bool("json", false, "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	r, err := store.Load()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	sortProjectsByLastSeen(r.Projects)
	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(registryListPayload(r.Projects)); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	for _, p := range r.Projects {
		fmt.Fprintf(stdout, "%s\t%s\t%s\n", p.ProjectID, p.Slug, p.LastSeen)
		for _, path := range p.Paths {
			state := "ok"
			if !configExists(path) {
				state = "missing"
			}
			fmt.Fprintf(stdout, "  %s\t%s\n", state, path)
		}
	}
	return 0
}

type registryProjectStatus struct {
	ProjectID string               `json:"project_id"`
	Slug      string               `json:"slug"`
	Name      string               `json:"name,omitempty"`
	LastSeen  string               `json:"last_seen"`
	Paths     []registryPathStatus `json:"paths"`
}

type registryPathStatus struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

type registryListSummary struct {
	SchemaVersion int                     `json:"schema_version"`
	ProjectCount  int                     `json:"project_count"`
	PathCount     int                     `json:"path_count"`
	MissingCount  int                     `json:"missing_count"`
	Projects      []registryProjectStatus `json:"projects"`
}

func registryListPayload(projects []registry.Project) registryListSummary {
	out := make([]registryProjectStatus, 0, len(projects))
	pathCount := 0
	missingCount := 0
	for _, p := range projects {
		item := registryProjectStatus{
			ProjectID: p.ProjectID,
			Slug:      p.Slug,
			Name:      p.Name,
			LastSeen:  p.LastSeen,
			Paths:     make([]registryPathStatus, 0, len(p.Paths)),
		}
		for _, path := range p.Paths {
			pathCount++
			status := "ok"
			if !configExists(path) {
				status = "missing"
				missingCount++
			}
			item.Paths = append(item.Paths, registryPathStatus{Path: path, Status: status})
		}
		out = append(out, item)
	}
	return registryListSummary{
		SchemaVersion: 1,
		ProjectCount:  len(out),
		PathCount:     pathCount,
		MissingCount:  missingCount,
		Projects:      out,
	}
}

func sortProjectsByLastSeen(projects []registry.Project) {
	sort.SliceStable(projects, func(i, j int) bool {
		return projects[i].LastSeen > projects[j].LastSeen
	})
}

func runRegistryPrune(args []string, store *registry.Store, stdout, stderr io.Writer) int {
	fs := newFlagSet("registry prune")
	dryRun := fs.Bool("dry-run", false, "")
	jsonOut := fs.Bool("json", false, "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	r, err := store.Load()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	var missing []string
	missingByProject := map[string][]string{}
	for _, p := range r.Projects {
		for _, path := range p.Paths {
			if configExists(path) {
				continue
			}
			missing = append(missing, path)
			missingByProject[p.ProjectID] = append(missingByProject[p.ProjectID], path)
		}
	}
	if *jsonOut {
		payload := map[string]any{
			"schema_version": 1,
			"dry_run":        *dryRun,
			"pruned_count":   len(missing),
			"project_count":  len(missingByProject),
			"projects":       missingByProject,
			"paths":          missing,
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(payload); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	for _, path := range missing {
		if !*dryRun {
			if err := store.UnregisterPath(path); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
		}
		if !*jsonOut {
			action := "pruned"
			if *dryRun {
				action = "would prune"
			}
			fmt.Fprintf(stdout, "%s %s\n", action, path)
		}
	}
	if len(missing) == 0 && !*jsonOut {
		fmt.Fprintln(stdout, "registry clean")
	}
	return 0
}

func runRegistryRepair(store *registry.Store, registryPath string, stdout, stderr io.Writer) int {
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
