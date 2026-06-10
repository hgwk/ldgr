package cmd

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

func migrationWarnings(c migrateCounts) []string {
	var warnings []string
	add := func(code string, n int) {
		if n > 0 {
			warnings = append(warnings, fmt.Sprintf("%s %d rows", code, n))
		}
	}
	add("WEAK_DONE_MAPPED_REVIEW", c.weakDone)
	add("WEAK_REWORK_MAPPED_REVIEW", c.weakRework)
	add("GHOST_TICKET_SYNTHESIZED", c.ghostTickets)
	add("GHOST_WORKLOG_SYNTHESIZED", c.ghostWorklogs)
	add("TYPE_DEFAULTED", c.typeDefaulted)
	add("AREA_DEFAULTED", c.areaDefaulted)
	add("ROLE_DEFAULTED", c.roleDefaulted)
	add("SUMMARY_DEFAULTED", c.summaryDefaulted)
	add("WORKLOG_TICKET_DEFAULTED", c.worklogTicketDefault)
	add("UNMAPPED_FIELD", c.unmappedField)
	sort.Strings(warnings)
	return warnings
}

func renderMigratePlan(w io.Writer, plan migratePlan) {
	fmt.Fprintln(w, "Schema v1 migration plan")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Target:")
	for _, c := range plan.changes {
		fmt.Fprintf(w, "  %s\t%s\n", c.OutputPath, actionName(c.Action))
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Rows:")
	fmt.Fprintln(w, "  config    1")
	fmt.Fprintln(w, "  goal      unchanged")
	fmt.Fprintf(w, "  tickets   %d\n", plan.counts.tickets)
	fmt.Fprintf(w, "  worklog   %d\n", plan.counts.worklogs)
	if len(plan.warnings) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Warnings:")
		for _, warning := range plan.warnings {
			fmt.Fprintf(w, "  %s\n", warning)
			code := strings.Fields(warning)[0]
			for _, sample := range plan.warningSamples[code] {
				fmt.Fprintf(w, "    sample: %s\n", sample)
			}
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Apply:")
	fmt.Fprintln(w, "  ldgr migrate legacy-to-v1 --apply")
}

func renderMigrateApply(w io.Writer, plan migratePlan, verifyWarnings int) {
	for _, c := range plan.changes {
		fmt.Fprintf(w, "%s %s\n", actionName(c.Action), c.OutputPath)
	}
	fmt.Fprintf(w, "verify warnings %d\n", verifyWarnings)
}
