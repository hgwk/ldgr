package cmd

import "github.com/hgwk/ldgr/internal/legacy"

type migratePlan struct {
	targetDir      string
	changes        []legacy.Change
	warnings       []string
	warningSamples map[string][]string
	counts         migrateCounts
	baseline       historicalBaseline
}

type migrateCounts struct {
	tickets              int
	worklogs             int
	weakDone             int
	weakRework           int
	ghostTickets         int
	ghostWorklogs        int
	typeDefaulted        int
	areaDefaulted        int
	roleDefaulted        int
	summaryDefaulted     int
	worklogTicketDefault int
	unmappedField        int
}

type historicalBaseline struct {
	Tickets int `json:"tickets"`
	Worklog int `json:"worklog"`
}

func (p migratePlan) legacyPlan() legacy.Plan {
	return legacy.Plan{TargetDir: p.targetDir, Changes: p.changes, Warnings: p.warnings}
}
