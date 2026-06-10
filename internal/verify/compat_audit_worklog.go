package verify

import (
	"fmt"
	"sort"

	"github.com/hgwk/ldgr/internal/ledger"
	"github.com/hgwk/ldgr/internal/lifecycle"
)

func checkAuditReviewedN(rep *Report, tickets []ledger.Row) {
	byTicket := map[string][]ledger.Row{}
	for _, r := range tickets {
		if _, isCompanion := r["invalidates_n"]; isCompanion {
			continue
		}
		id, _ := r["ticket"].(string)
		if id == "" {
			continue
		}
		byTicket[id] = append(byTicket[id], r)
	}

	for id, rows := range byTicket {
		sort.SliceStable(rows, func(i, j int) bool {
			ni, _ := rows[i]["n"].(float64)
			nj, _ := rows[j]["n"].(float64)
			return ni < nj
		})

		latestAuditReady := 0
		for _, r := range rows {
			s, _ := r["status"].(string)
			if s == "audit_ready" {
				if n, ok := numberAsInt(r["n"]); ok {
					latestAuditReady = n
				}
				continue
			}
			if role, _ := r["role"].(string); role != "audit" {
				continue
			}
			if s != "done" && s != "changes_requested" {
				continue
			}

			ln, _ := numberAsInt(r["n"])
			got, ok := numberAsInt(r["reviewed_n"])
			if !ok {
				rep.Warns = append(rep.Warns, Issue{
					File:    "ledger/tickets.jsonl",
					Line:    ln,
					Code:    "AUDIT_MISSING_REVIEWED_N",
					Message: fmt.Sprintf("[AUDIT_MISSING_REVIEWED_N] %s", id),
				})
				continue
			}
			if latestAuditReady == 0 || got != latestAuditReady {
				rep.Warns = append(rep.Warns, Issue{
					File:    "ledger/tickets.jsonl",
					Line:    ln,
					Code:    "AUDIT_REVIEWED_N_MISMATCH",
					Message: fmt.Sprintf("[AUDIT_REVIEWED_N_MISMATCH] %s: reviewed_n=%d latest_audit_ready=%d", id, got, latestAuditReady),
				})
			}
		}
	}
}

func checkPrematureWorklog(rep *Report, tickets, worklog []ledger.Row) {
	byTicket := map[string][]ledger.Row{}
	for _, r := range tickets {
		if _, isCompanion := r["invalidates_n"]; isCompanion {
			continue
		}
		id, _ := r["ticket"].(string)
		if id == "" {
			continue
		}
		byTicket[id] = append(byTicket[id], r)
	}
	for id := range byTicket {
		sort.SliceStable(byTicket[id], func(i, j int) bool {
			ti, _ := byTicket[id][i]["ts"].(string)
			tj, _ := byTicket[id][j]["ts"].(string)
			if c := compareTS(ti, tj); c != 0 {
				return c < 0
			}
			ni, _ := byTicket[id][i]["n"].(float64)
			nj, _ := byTicket[id][j]["n"].(float64)
			return ni < nj
		})
	}

	for _, w := range worklog {
		if _, isCompanion := w["invalidates_n"]; isCompanion {
			continue
		}
		id, _ := w["ticket"].(string)
		if id == "" {
			continue
		}
		lr, ok := latestTicketAtOrBefore(byTicket[id], stringField(w, "ts"))
		if !ok {
			ln, _ := w["n"].(float64)
			rep.Warns = append(rep.Warns, Issue{
				File:    "ledger/worklog.jsonl",
				Line:    int(ln),
				Code:    "PREMATURE_WORKLOG",
				Message: fmt.Sprintf("[PREMATURE_WORKLOG] %s", id),
			})
			continue
		}
		if !lifecycle.IsAuditPassDone(lr) {
			ln, _ := w["n"].(float64)
			rep.Warns = append(rep.Warns, Issue{
				File:    "ledger/worklog.jsonl",
				Line:    int(ln),
				Code:    "PREMATURE_WORKLOG",
				Message: fmt.Sprintf("[PREMATURE_WORKLOG] %s", id),
			})
		}
	}
}

func latestTicketAtOrBefore(rows []ledger.Row, ts string) (ledger.Row, bool) {
	var latest ledger.Row
	for _, r := range rows {
		rts, _ := r["ts"].(string)
		if ts != "" && rts != "" && compareTS(rts, ts) > 0 {
			break
		}
		latest = r
	}
	return latest, latest != nil
}
