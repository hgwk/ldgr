package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hgwk/ldgr/internal/ledger"
	"github.com/hgwk/ldgr/internal/legacy"
)

func composeLegacyToStatePlan(target string) (migratePlan, error) {
	cfgPath := filepath.Join(target, "ledger", "config.json")
	ticketsPath := filepath.Join(target, "ledger", "tickets.jsonl")
	worklogPath := filepath.Join(target, "ledger", "worklog.jsonl")
	if _, err := os.Stat(cfgPath); err != nil {
		return migratePlan{}, fmt.Errorf("migrate legacy-to-v1: missing ledger/config.json: %w", err)
	}
	tickets, err := ledger.ReadRows(ticketsPath)
	if err != nil {
		return migratePlan{}, err
	}
	worklogs, err := ledger.ReadRows(worklogPath)
	if err != nil {
		return migratePlan{}, err
	}
	if rowsNeedNoLegacyMigration(tickets, worklogs) {
		return migratePlan{targetDir: target}, nil
	}

	var counts migrateCounts
	samples := map[string][]string{}
	stateTickets := make([]ledger.Row, 0, len(tickets))
	for _, row := range tickets {
		if ticketRowIsStateModel(row) {
			stateTickets = append(stateTickets, row)
			continue
		}
		stateRow, rowCounts := mapLegacyTicketToState(row)
		counts.add(rowCounts)
		addMigrationSamples(samples, "ticket", row, stateRow, rowCounts)
		stateTickets = append(stateTickets, stateRow)
	}
	stateWorklogs := make([]ledger.Row, 0, len(worklogs))
	for _, row := range worklogs {
		if worklogRowIsStateModel(row) {
			stateWorklogs = append(stateWorklogs, row)
			continue
		}
		stateRow, rowCounts := mapLegacyWorklogToState(row)
		counts.add(rowCounts)
		addMigrationSamples(samples, "worklog", row, stateRow, rowCounts)
		stateWorklogs = append(stateWorklogs, stateRow)
	}
	baseline := historicalBaseline{Tickets: len(stateTickets), Worklog: len(stateWorklogs)}
	cfgBytes, err := rewriteConfigSchemaVersion(cfgPath, 1, baseline)
	if err != nil {
		return migratePlan{}, err
	}
	ticketBytes, err := marshalJSONL(stateTickets)
	if err != nil {
		return migratePlan{}, err
	}
	worklogBytes, err := marshalJSONL(stateWorklogs)
	if err != nil {
		return migratePlan{}, err
	}
	plan := migratePlan{
		targetDir: target,
		changes: []legacy.Change{
			{OutputPath: "ledger/config.json", Action: legacy.ActionReplace, NewBytes: cfgBytes},
			{OutputPath: "ledger/tickets.jsonl", Action: legacy.ActionReplace, NewBytes: ticketBytes},
			{OutputPath: "ledger/worklog.jsonl", Action: legacy.ActionReplace, NewBytes: worklogBytes},
		},
		counts:         counts,
		warningSamples: samples,
		baseline:       baseline,
	}
	plan.warnings = migrationWarnings(counts)
	return plan, nil
}
func (c *migrateCounts) add(o migrateCounts) {
	c.tickets += o.tickets
	c.worklogs += o.worklogs
	c.weakDone += o.weakDone
	c.weakRework += o.weakRework
	c.ghostTickets += o.ghostTickets
	c.ghostWorklogs += o.ghostWorklogs
	c.typeDefaulted += o.typeDefaulted
	c.areaDefaulted += o.areaDefaulted
	c.roleDefaulted += o.roleDefaulted
	c.summaryDefaulted += o.summaryDefaulted
	c.worklogTicketDefault += o.worklogTicketDefault
	c.unmappedField += o.unmappedField
}

func rowsNeedNoLegacyMigration(tickets, worklogs []ledger.Row) bool {
	for _, row := range tickets {
		if !ticketRowIsStateModel(row) {
			return false
		}
	}
	for _, row := range worklogs {
		if !worklogRowIsStateModel(row) {
			return false
		}
	}
	return true
}

func ticketRowIsStateModel(row ledger.Row) bool {
	_, hasID := row["id"]
	_, hasState := row["state"]
	_, hasEvent := row["event"]
	return hasID && hasState && hasEvent
}

func worklogRowIsStateModel(row ledger.Row) bool {
	_, hasActor := row["actor"]
	_, hasTitle := row["title"]
	_, hasSummary := row["summary"]
	return hasActor && hasTitle && hasSummary
}
