package cmd

import (
	"fmt"
	"io"

	"github.com/hgwk/ldgr/internal/legacy"
	"github.com/hgwk/ldgr/internal/verify"
)

func init() {
	Commands["migrate"] = RunMigrateCLI
}

func RunMigrateCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "legacy-to-v1" {
		fmt.Fprintln(stderr, "usage: ldgr migrate legacy-to-v1 --target PATH (--plan | --apply)")
		return 2
	}
	mode := args[0]
	fs := newFlagSet("migrate " + mode)
	target := fs.String("target", "", "")
	planFlag := fs.Bool("plan", false, "")
	applyFlag := fs.Bool("apply", false, "")
	backupFlag := fs.Bool("backup", true, "create .ldgr/backups before rewriting")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *planFlag == *applyFlag {
		fmt.Fprintln(stderr, "specify exactly one of --plan or --apply")
		return 2
	}
	if !*backupFlag {
		fmt.Fprintf(stderr, "migrate %s requires --backup=true\n", mode)
		return 2
	}
	dir := resolveTarget(*target)
	plan, err := composeLegacyToStatePlan(dir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *planFlag {
		renderMigratePlan(stdout, plan)
		return 0
	}
	if err := legacy.Apply(plan.legacyPlan(), legacy.ApplyOpts{BackupPrefix: "legacy-to-v1-"}); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	report, err := verify.Run(dir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if len(report.Fails) > 0 {
		fmt.Fprintln(stderr, "migration wrote schema v1 files but verification failed; restore from .ldgr/backups/")
		for _, fail := range report.Fails {
			fmt.Fprintf(stderr, "%s:%d %s %s\n", fail.File, fail.Line, fail.Code, fail.Message)
		}
		return 1
	}
	renderMigrateApply(stdout, plan, len(report.Warns))
	return 0
}
