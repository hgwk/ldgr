package verify

import (
	"regexp"
	"strings"

	"github.com/hgwk/ldgr/internal/ledger"
)

func usesStateRows(tickets, worklog []ledger.Row) bool {
	for _, r := range tickets {
		if _, ok := r["id"]; ok {
			return true
		}
		if _, ok := r["state"]; ok {
			return true
		}
		if _, ok := r["event"]; ok {
			return true
		}
	}
	for _, r := range worklog {
		if _, ok := r["actor"]; ok {
			return true
		}
		if _, ok := r["summary"]; ok {
			return true
		}
	}
	return false
}

func isStateTicketRow(r ledger.Row) bool {
	_, hasID := r["id"]
	_, hasState := r["state"]
	_, hasEvent := r["event"]
	return hasID || hasState || hasEvent
}

func isStateWorklogRow(r ledger.Row) bool {
	_, hasActor := r["actor"]
	_, hasTitle := r["title"]
	_, hasSummary := r["summary"]
	return hasActor || hasTitle || hasSummary
}

var isoRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z$`)

func checkCompatTicketRow(rep *Report, line int, r ledger.Row, prevTS *string) {
	checkCommonEnvelope(rep, "ledger/tickets.jsonl", line, r, prevTS)
	for _, f := range ledger.TicketRequired {
		if _, ok := r[f]; !ok {
			rep.Fails = append(rep.Fails, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "MISSING_REQUIRED", Message: "missing required field: " + f})
		}
	}
	for _, f := range ledger.TicketNonEmpty {
		if v, ok := r[f].(string); !ok || strings.TrimSpace(v) == "" {
			rep.Fails = append(rep.Fails, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "NON_EMPTY_VIOLATION", Message: "field must be non-empty: " + f})
		}
	}
	if s, ok := r["status"].(string); ok {
		if _, valid := ledger.StatusEnum[s]; !valid {
			rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "UNKNOWN_STATUS", Message: "unknown status: " + s})
		}
	}
}

func checkCompatWorklogRow(rep *Report, line int, r ledger.Row, prevTS *string) {
	checkCommonEnvelope(rep, "ledger/worklog.jsonl", line, r, prevTS)
	for _, f := range ledger.WorklogRequired {
		if _, ok := r[f]; !ok {
			rep.Fails = append(rep.Fails, Issue{File: "ledger/worklog.jsonl", Line: line, Code: "MISSING_REQUIRED", Message: "missing required field: " + f})
		}
	}
	for _, f := range ledger.WorklogNonEmpty {
		if v, ok := r[f].(string); !ok || strings.TrimSpace(v) == "" {
			rep.Fails = append(rep.Fails, Issue{File: "ledger/worklog.jsonl", Line: line, Code: "NON_EMPTY_VIOLATION", Message: "field must be non-empty: " + f})
		}
	}
	if v, ok := r["ticket"].(string); ok && v == "" {
		rep.Fails = append(rep.Fails, Issue{File: "ledger/worklog.jsonl", Line: line, Code: "WORKLOG_TICKET_EMPTY", Message: "field must be non-empty when present: ticket"})
	}
}
