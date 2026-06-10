package cmd

import (
	"fmt"
	"strings"

	"github.com/hgwk/ldgr/internal/ledger"
)

func mapLegacyWorklogToState(row ledger.Row) (ledger.Row, migrateCounts) {
	var counts migrateCounts
	counts.worklogs = 1
	title := migrateStringField(row, "task")
	n, hasN := numberAsPositiveInt(row["n"])
	if title == "" {
		title = migrateStringField(row, "ticket")
		if title == "" && hasN {
			title = fmt.Sprintf("Invalid historical worklog row %d", n)
			counts.ghostWorklogs = 1
		}
		counts.summaryDefaulted = 1
	}
	summary := migrateStringField(row, "result")
	if summary == "" {
		summary = title
		counts.summaryDefaulted = 1
	}
	ticket := migrateStringField(row, "ticket")
	if strings.TrimSpace(ticket) == "" {
		if hasN {
			ticket = fmt.Sprintf("invalid-worklog-row-%d", n)
		} else {
			ticket = "invalid-worklog-row"
		}
		counts.worklogTicketDefault = 1
	}
	out := ledger.Row{
		"n":        row["n"],
		"ts":       row["ts"],
		"ticket":   ticket,
		"actor":    stringDefault(migrateStringField(row, "agent"), "unknown"),
		"title":    title,
		"summary":  summary,
		"paths":    arrayField(row, "paths"),
		"commands": arrayField(row, "commands"),
		"notes":    stringField(row, "notes"),
	}
	if extra := unknownFields(row, v1WorklogKnownFields()); len(extra) > 0 {
		out["extra"] = extra
		counts.unmappedField = len(extra)
	}
	return out, counts
}
