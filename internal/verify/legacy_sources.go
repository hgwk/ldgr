package verify

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/hgwk/ldgr/internal/legacy"
)

func checkLegacySources(rep *Report, targetDir string) {
	sources, err := legacy.Scan(targetDir)
	if err != nil {
		rep.Fails = append(rep.Fails, Issue{File: ".", Code: "LEGACY_SCAN_ERROR", Message: "cannot scan legacy ledger sources: " + err.Error()})
		return
	}

	for _, src := range sources {
		if !src.Exists || !isLegacySource(src.Kind) {
			continue
		}
		rel := relFile(targetDir, src.Path)
		rep.Warns = append(rep.Warns, Issue{
			File:    rel,
			Code:    "LEGACY_LEDGER_PRESENT",
			Message: "root legacy ledger present; run `ldgr import legacy --plan` before `ldgr import legacy --apply`",
		})
		for _, parseErr := range src.ParseErrs {
			rep.Fails = append(rep.Fails, Issue{
				File:    rel,
				Line:    parseErr.Line,
				Code:    "LEGACY_PARSE_ERROR",
				Message: "legacy ledger parse error: " + parseErr.Err,
			})
		}
	}
}

func isLegacySource(kind legacy.SourceKind) bool {
	switch kind {
	case legacy.SourceLegacyTickets, legacy.SourceLegacyWorklog, legacy.SourceLegacyGoal:
		return true
	default:
		return false
	}
}

func relFile(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

type historicalBaseline struct {
	Tickets int `json:"tickets"`
	Worklog int `json:"worklog"`
}

func loadHistoricalBaseline(path string) historicalBaseline {
	data, err := os.ReadFile(path)
	if err != nil {
		return historicalBaseline{}
	}
	var raw struct {
		HistoricalBaseline historicalBaseline `json:"historical_baseline"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return historicalBaseline{}
	}
	return raw.HistoricalBaseline
}

func filterHistoricalBaselineIssues(in []Issue, baseline historicalBaseline) []Issue {
	if baseline.Tickets <= 0 && baseline.Worklog <= 0 {
		return in
	}
	out := in[:0]
	for _, issue := range in {
		if isHistoricalBaselineIssue(issue, baseline) {
			continue
		}
		out = append(out, issue)
	}
	return out
}

func isHistoricalBaselineIssue(issue Issue, baseline historicalBaseline) bool {
	if issue.Line <= 0 {
		return false
	}
	if !isCompatibilityWarning(issue.Code) {
		return false
	}
	switch issue.File {
	case "ledger/tickets.jsonl":
		return baseline.Tickets > 0 && issue.Line <= baseline.Tickets
	case "ledger/worklog.jsonl":
		return baseline.Worklog > 0 && issue.Line <= baseline.Worklog
	default:
		return false
	}
}

func isCompatibilityWarning(code string) bool {
	switch code {
	case "MISSING_CATEGORY",
		"MISSING_REQUIRED",
		"NON_EMPTY_VIOLATION",
		"TS_NOT_INCREASING",
		"UNKNOWN_TYPE",
		"UNKNOWN_AREA",
		"UNKNOWN_PRIORITY",
		"LEGACY_STATE_VALUE",
		"UNKNOWN_STATUS",
		"UNKNOWN_EVENT_ROLE",
		"UNKNOWN_EVENT_RESULT",
		"MISSING_EVENT_FIELD",
		"EMPTY_EVENT_FIELD",
		"ORPHAN_WORKLOG",
		"PREMATURE_WORKLOG",
		"WEAK_DONE",
		"REWORK_WEAK",
		"INVALID_TRANSITION",
		"AUDIT_REVIEWED_N_MISMATCH",
		"INVALIDATED_HISTORICAL":
		return true
	default:
		return false
	}
}
