package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hgwk/ldgr/internal/config"
	"github.com/hgwk/ldgr/internal/legacy"
)

func init() {
	Commands["import"] = RunImportCLI
}

func RunImportCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "legacy" {
		fmt.Fprintln(stderr, "usage: ldgr import legacy --target PATH (--plan | --apply [--archive-originals] [--force])")
		return 2
	}
	fs := newFlagSet("import legacy")
	target := fs.String("target", "", "")
	planFlag := fs.Bool("plan", false, "")
	applyFlag := fs.Bool("apply", false, "")
	archive := fs.Bool("archive-originals", false, "")
	force := fs.Bool("force", false, "")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *planFlag == *applyFlag {
		fmt.Fprintln(stderr, "specify exactly one of --plan or --apply")
		return 2
	}
	dir := resolveTarget(*target)

	cfg, err := loadOrDefaultConfig(dir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	srcs, err := legacy.Scan(dir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	plan := legacy.Compose(dir, srcs, cfg, time.Now().UTC().Format("2006-01-02T15:04:05Z"))

	if *planFlag {
		renderPlan(stdout, dir, plan, *force)
		return 0
	}

	if !*force && shrinkingTarget(plan) {
		fmt.Fprintln(stderr, "refusing to shrink an existing ledger; re-run with --force if intentional")
		return 1
	}

	if err := legacy.Apply(plan, legacy.ApplyOpts{ArchiveOriginals: *archive, Force: *force}); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	renderApply(stdout, plan)
	return 0
}

func loadOrDefaultConfig(dir string) (config.Config, error) {
	cfg, err := config.Load(filepath.Join(dir, "ledger", "config.json"))
	if err != nil && !os.IsNotExist(err) {
		return cfg, err
	}
	if err != nil || cfg.ProjectID == "" {
		// Caller did not run init yet. Synthesize a temporary config so we can
		// infer parents. The real init must follow `import legacy --apply`.
		return config.Default(filepath.Base(dir), "import-stub", ""), nil
	}
	return cfg, nil
}

func shrinkingTarget(plan legacy.Plan) bool {
	for _, c := range plan.Changes {
		if c.Action != legacy.ActionReplace {
			continue
		}
		current, err := os.ReadFile(filepath.Join(plan.TargetDir, c.OutputPath))
		if err != nil {
			continue
		}
		if strings.Count(string(current), "\n") > strings.Count(string(c.NewBytes), "\n") {
			return true
		}
	}
	return false
}

func renderPlan(w io.Writer, dir string, plan legacy.Plan, force bool) {
	fmt.Fprintln(w, "Legacy import plan")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Sources:")
	for _, s := range plan.Sources {
		fmt.Fprintf(w, "  %s\t%d rows\n", filepath.Base(s.Path), len(s.Rows))
	}
	if len(plan.Sources) == 0 {
		fmt.Fprintln(w, "  (none detected)")
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Target:")
	for _, c := range plan.Changes {
		fmt.Fprintf(w, "  %s\t%s\n", c.OutputPath, actionName(c.Action))
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Changes:")
	fmt.Fprintf(w, "  ticket rows imported     %d\n", plan.Counts.TicketsImported)
	fmt.Fprintf(w, "  worklog rows imported    %d\n", plan.Counts.WorklogImported)
	fmt.Fprintf(w, "  parent_ticket inferred   %d\n", plan.Counts.ParentInferred)
	fmt.Fprintf(w, "  n reassigned             %d\n", plan.Counts.NReassigned)
	fmt.Fprintf(w, "  ts replaced              %d\n", plan.Counts.TSReplaced)
	fmt.Fprintf(w, "  agent defaulted          %d\n", plan.Counts.AgentDefaulted)
	fmt.Fprintf(w, "  ghost tickets            %d\n", plan.Counts.GhostTickets)
	fmt.Fprintf(w, "  ghost worklog            %d\n", plan.Counts.GhostWorklog)
	fmt.Fprintf(w, "  parse errors             %d\n", plan.Counts.ParseErrors)
	fmt.Fprintln(w)
	if len(plan.Warnings) > 0 {
		fmt.Fprintln(w, "Warnings:")
		for _, w0 := range plan.Warnings {
			fmt.Fprintf(w, "  %s\n", w0)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "Original files:")
	fmt.Fprintln(w, "  preserve in place (use --archive-originals to move them under ledger/legacy/)")
	if !force && shrinkingTarget(plan) {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "WARNING: existing target has more rows than the import would produce. --apply requires --force.")
	}
}

func renderApply(w io.Writer, plan legacy.Plan) {
	if !plan.HasChanges() {
		fmt.Fprintln(w, "no changes")
		return
	}
	for _, c := range plan.Changes {
		if c.Action == legacy.ActionNoop {
			continue
		}
		fmt.Fprintf(w, "%s %s\n", actionName(c.Action), c.OutputPath)
	}
}

func actionName(a legacy.ChangeAction) string {
	switch a {
	case legacy.ActionCreate:
		return "create"
	case legacy.ActionReplace:
		return "update"
	case legacy.ActionNoop:
		return "noop"
	}
	return "?"
}
