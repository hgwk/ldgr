package verify

import (
	"fmt"

	"github.com/hgwk/ldgr/internal/ledger"
)

func checkRows(rep *Report, file string, rows []ledger.Row, required []string, isTicket bool) {
	prevTS := ""
	invalidated := invalidatedLines(rows)
	for i, r := range rows {
		line := i + 1
		if n, ok := r["n"].(float64); !ok || int(n) != line {
			rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Code: "N_NOT_CONSECUTIVE", Message: fmt.Sprintf("n must equal %d, got %v", line, r["n"])})
		}
		for _, f := range required {
			if _, ok := r[f]; !ok {
				rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Code: "MISSING_REQUIRED", Message: "missing required field: " + f})
			}
		}
		nonEmpty := ledger.WorklogNonEmpty
		if isTicket {
			nonEmpty = ledger.TicketNonEmpty
		}
		for _, f := range nonEmpty {
			if v, ok := r[f].(string); !ok || v == "" {
				if _, ok := invalidated[line]; ok {
					rep.Warns = append(rep.Warns, Issue{File: file, Line: line, Code: "INVALIDATED_HISTORICAL", Message: "invalid historical row superseded by invalidates_n"})
					continue
				}
				rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Code: "NON_EMPTY_VIOLATION", Message: "field must be non-empty: " + f})
			}
		}
		if !isTicket {
			if v, ok := r["ticket"].(string); ok && v == "" {
				if _, ok := invalidated[line]; ok {
					rep.Warns = append(rep.Warns, Issue{File: file, Line: line, Code: "INVALIDATED_HISTORICAL", Message: "invalid historical row superseded by invalidates_n"})
					continue
				}
				rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Code: "WORKLOG_TICKET_EMPTY", Message: "field must be non-empty when present: ticket"})
			}
		}
		ts, _ := r["ts"].(string)
		if !isoRe.MatchString(ts) {
			rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Code: "TS_NOT_ISO", Message: "ts not ISO8601 UTC: " + ts})
		} else if prevTS != "" && compareTS(ts, prevTS) < 0 {
			rep.Warns = append(rep.Warns, Issue{File: file, Line: line, Code: "TS_NOT_INCREASING", Message: "ts decreases relative to previous row"})
		}
		prevTS = ts
		if isTicket {
			if s, ok := r["status"].(string); ok {
				if _, valid := ledger.StatusEnum[s]; !valid {
					rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Code: "INVALID_STATUS", Message: "invalid status: " + s})
				}
			}
			if c, ok := r["category"].(string); !ok || c == "" {
				rep.Warns = append(rep.Warns, Issue{File: file, Line: line, Code: "MISSING_CATEGORY", Message: "missing category"})
			} else if _, valid := ledger.CategoryEnum[c]; !valid {
				rep.Warns = append(rep.Warns, Issue{File: file, Line: line, Code: "UNKNOWN_CATEGORY", Message: "unknown category: " + c})
			}
			if k, ok := r["kind"].(string); ok && k != "" {
				if _, valid := ledger.KindEnum[k]; !valid {
					rep.Warns = append(rep.Warns, Issue{File: file, Line: line, Code: "UNKNOWN_KIND", Message: fmt.Sprintf("[UNKNOWN_KIND] unknown kind: %s", k)})
				}
			}
			if p, ok := r["priority"].(string); ok && p != "" {
				if _, valid := ledger.PriorityEnum[p]; !valid {
					rep.Warns = append(rep.Warns, Issue{File: file, Line: line, Code: "UNKNOWN_PRIORITY", Message: fmt.Sprintf("[UNKNOWN_PRIORITY] unknown priority: %s", p)})
				}
			}
		}
	}
}
