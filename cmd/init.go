package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hgwk/ldgr/internal/config"
	"github.com/hgwk/ldgr/internal/ids"
	"github.com/hgwk/ldgr/internal/jsonio"
	"github.com/hgwk/ldgr/internal/ledger"
	"github.com/hgwk/ldgr/internal/registry"
)

func init() {
	Commands["init"] = runInitCLI
}

type InitOpts struct {
	Slug            string
	Name            string
	WritingLanguage string
}

// RunInit creates ledger/* in targetDir and registers it. Re-running on an
// already-initialized directory is a no-op for the data files and re-adds
// the path in the registry idempotently.
func RunInit(targetDir string, opts InitOpts, store *registry.Store) error {
	slug := opts.Slug
	if slug == "" {
		slug = filepath.Base(targetDir)
	}

	ledgerDir := filepath.Join(targetDir, "ledger")
	if err := os.MkdirAll(ledgerDir, 0o755); err != nil {
		return err
	}

	configPath := filepath.Join(ledgerDir, "config.json")
	var cfg config.Config
	if existing, err := config.Load(configPath); err == nil && existing.ProjectID != "" {
		cfg = existing
		if opts.WritingLanguage != "" && cfg.WritingLanguage != opts.WritingLanguage {
			cfg.WritingLanguage = opts.WritingLanguage
			if err := config.PatchWritingLanguage(configPath, opts.WritingLanguage); err != nil {
				return err
			}
		}
	} else if errors.Is(err, os.ErrNotExist) || existing.ProjectID == "" {
		cfg = config.Default(slug, ids.NewProjectID(), opts.Name)
		cfg.WritingLanguage = opts.WritingLanguage
		if err := config.Save(configPath, cfg); err != nil {
			return err
		}
	} else {
		return err
	}

	if err := ensureEmpty(filepath.Join(ledgerDir, "tickets.jsonl")); err != nil {
		return err
	}
	if err := ensureEmpty(filepath.Join(ledgerDir, "worklog.jsonl")); err != nil {
		return err
	}
	if err := ensureGoal(filepath.Join(ledgerDir, "goal.json")); err != nil {
		return err
	}
	if err := ensureGitignore(filepath.Join(targetDir, ".gitignore")); err != nil {
		return err
	}

	return store.Register(registry.Project{
		ProjectID: cfg.ProjectID,
		Slug:      cfg.Slug,
		Name:      cfg.Name,
		Paths:     []string{targetDir},
	})
}

func runInitCLI(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("init")
	target := fs.String("target", "", "target directory (defaults to cwd)")
	slug := fs.String("slug", "", "project slug (defaults to dir name)")
	name := fs.String("name", "", "project display name (defaults to slug)")
	language := fs.String("language", "", "free-text writing language for ledger content (for example ko, en)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir := *target
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	store, _, err := DefaultRegistry()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := RunInit(abs, InitOpts{Slug: *slug, Name: *name, WritingLanguage: *language}, store); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "initialized %s\n", abs)
	return 0
}

func ensureEmpty(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return f.Close()
}

func ensureGoal(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	g := ledger.Goal{
		SchemaVersion:   1,
		Track:           "project",
		Version:         "0.1.0",
		Updated:         time.Now().UTC().Format(time.RFC3339),
		SourceOfTruth:   "README.md",
		Summary:         "Fill this goal snapshot with the current project objective.",
		SuccessCriteria: []string{},
	}
	return jsonio.WriteJSON(path, g)
}

func ensureGitignore(path string) error {
	required := []string{
		"ledger/.lock",
		"ledger/.backup/",
		"ledger/import-errors.jsonl",
		"ledger/legacy/",
	}
	existing := ""
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	}
	out := existing
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	for _, line := range required {
		if !lineContains(existing, line) {
			out += line + "\n"
		}
	}
	if out == existing {
		return nil
	}
	return os.WriteFile(path, []byte(out), 0o644)
}

func lineContains(haystack, needle string) bool {
	for _, line := range strings.Split(haystack, "\n") {
		if strings.TrimSpace(line) == needle {
			return true
		}
	}
	return false
}
