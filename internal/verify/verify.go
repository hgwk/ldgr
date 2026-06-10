package verify

import (
	"path/filepath"

	"github.com/hgwk/ldgr/internal/config"
	"github.com/hgwk/ldgr/internal/jsonio"
	"github.com/hgwk/ldgr/internal/ledger"
)

type Issue struct {
	File    string
	Line    int
	Message string
	Code    string
}

type Report struct {
	Fails []Issue
	Warns []Issue
}

func Run(targetDir string) (Report, error) {
	return runWith(targetDir, false)
}

func RunStrict(targetDir string, strict bool) (Report, error) {
	return runWith(targetDir, strict)
}

func runWith(targetDir string, strict bool) (Report, error) {
	var rep Report

	cfgPath := filepath.Join(targetDir, "ledger", "config.json")
	var cfg config.Config
	if loaded, err := config.Load(cfgPath); err != nil {
		rep.Fails = append(rep.Fails, Issue{File: "ledger/config.json", Code: "CONFIG_INVALID", Message: "cannot read: " + err.Error()})
	} else {
		cfg = loaded
		if cfg.SchemaVersion == 0 || cfg.ProjectID == "" || cfg.Slug == "" {
			rep.Fails = append(rep.Fails, Issue{File: "ledger/config.json", Code: "CONFIG_INVALID", Message: "missing required fields (schema_version/project_id/slug)"})
		}
	}
	baseline := loadHistoricalBaseline(cfgPath)

	goalPath := filepath.Join(targetDir, "ledger", "goal.json")
	var g ledger.Goal
	if err := jsonio.ReadJSON(goalPath, &g); err != nil {
		rep.Fails = append(rep.Fails, Issue{File: "ledger/goal.json", Code: "GOAL_INVALID", Message: "cannot read: " + err.Error()})
	} else if g.SchemaVersion == 0 {
		rep.Fails = append(rep.Fails, Issue{File: "ledger/goal.json", Code: "GOAL_INVALID", Message: "schema_version required"})
	}

	ticketRows, err := ledger.ReadRows(filepath.Join(targetDir, "ledger", "tickets.jsonl"))
	if err != nil {
		rep.Fails = append(rep.Fails, Issue{File: "ledger/tickets.jsonl", Code: "PARSING_ERROR", Message: err.Error()})
	}

	worklogRows, err := ledger.ReadRows(filepath.Join(targetDir, "ledger", "worklog.jsonl"))
	if err != nil {
		rep.Fails = append(rep.Fails, Issue{File: "ledger/worklog.jsonl", Code: "PARSING_ERROR", Message: err.Error()})
	}

	stateMode := usesStateRows(ticketRows, worklogRows)
	if stateMode {
		checkStateRows(&rep, ticketRows, worklogRows)
		checkStateBlockers(&rep, ticketRows)
	} else {
		checkRows(&rep, "ledger/tickets.jsonl", ticketRows, ledger.TicketRequired, true)
		checkRows(&rep, "ledger/worklog.jsonl", worklogRows, ledger.WorklogRequired, false)
		checkOrphans(&rep, ticketRows, worklogRows)
		checkBlockers(&rep, ticketRows)
		checkParents(&rep, ticketRows, cfg.Parents)
		checkLifecycleTransitions(&rep, ticketRows)
		checkWeakDone(&rep, ticketRows)
		checkAuditReviewedN(&rep, ticketRows)
		checkPrematureWorklog(&rep, ticketRows, worklogRows)
	}
	checkWorklogCommands(&rep, worklogRows)
	checkClaimPathConflicts(&rep, ticketRows, stateMode)
	checkReviewEvidenceQuality(&rep, ticketRows, stateMode)
	checkSuccessCriteriaCoverage(&rep, ticketRows, stateMode)
	checkDecisionContext(&rep, ticketRows, stateMode)
	checkHandoffShape(&rep, ticketRows, stateMode)
	checkRepoFileGuardrails(&rep, targetDir)
	checkArchivedProjectActiveTickets(&rep, cfg, ticketRows, stateMode)
	checkLocalVerifierDrift(&rep, targetDir)
	checkLegacySources(&rep, targetDir)

	if strict {
		rep.Fails = append(rep.Fails, rep.Warns...)
		rep.Warns = nil
	} else {
		rep.Fails = filterHistoricalBaselineIssues(rep.Fails, baseline)
		rep.Warns = filterHistoricalBaselineIssues(rep.Warns, baseline)
	}
	return rep, nil
}
