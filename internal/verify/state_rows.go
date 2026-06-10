package verify

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hgwk/ldgr/internal/ledger"
)

func checkStateRows(rep *Report, tickets, worklog []ledger.Row) {
	checkStateTicketRows(rep, tickets)
	checkStateWorklogRows(rep, tickets, worklog)
	checkStateTransitions(rep, tickets)
}

func checkStateTicketRows(rep *Report, rows []ledger.Row) {
	prevTS := ""
	for i, r := range rows {
		line := i + 1
		if !isStateTicketRow(r) {
			checkCompatTicketRow(rep, line, r, &prevTS)
			continue
		}
		checkCommonEnvelope(rep, "ledger/tickets.jsonl", line, r, &prevTS)
		for _, f := range ledger.StateTicketRequired {
			if _, ok := r[f]; !ok {
				rep.Fails = append(rep.Fails, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "MISSING_REQUIRED", Message: "missing required field: " + f})
			}
		}
		for _, f := range ledger.StateTicketNonEmpty {
			if v, ok := r[f].(string); !ok || strings.TrimSpace(v) == "" {
				rep.Fails = append(rep.Fails, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "NON_EMPTY_VIOLATION", Message: "field must be non-empty: " + f})
			}
		}
		checkStringEnumWarn(rep, "ledger/tickets.jsonl", line, r, "type", ledger.TicketTypeEnum, "UNKNOWN_TYPE")
		checkStateEnum(rep, "ledger/tickets.jsonl", line, r)
		checkStringEnumWarn(rep, "ledger/tickets.jsonl", line, r, "area", ledger.AreaEnum, "UNKNOWN_AREA")
		checkStringEnumWarn(rep, "ledger/tickets.jsonl", line, r, "priority", ledger.PriorityEnum, "UNKNOWN_PRIORITY")

		event, ok := r["event"].(map[string]any)
		if !ok {
			rep.Fails = append(rep.Fails, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "EVENT_INVALID", Message: "event must be an object"})
			continue
		}
		for _, f := range ledger.EventRequired {
			if _, ok := event[f]; !ok {
				if isSoftEventField(f) {
					rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "MISSING_EVENT_FIELD", Message: "missing event field: event." + f})
					continue
				}
				rep.Fails = append(rep.Fails, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "MISSING_REQUIRED", Message: "missing required field: event." + f})
			}
		}
		for _, f := range ledger.EventNonEmpty {
			if v, ok := event[f].(string); !ok || strings.TrimSpace(v) == "" {
				if isSoftEventField(f) {
					rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "EMPTY_EVENT_FIELD", Message: "field should be non-empty: event." + f})
					continue
				}
				rep.Fails = append(rep.Fails, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "NON_EMPTY_VIOLATION", Message: "field must be non-empty: event." + f})
			}
		}
		checkStringEnumWarn(rep, "ledger/tickets.jsonl", line, event, "role", ledger.EventRoleEnum, "UNKNOWN_EVENT_ROLE")
		if result, ok := event["result"].(string); ok && result != "" {
			if _, valid := ledger.EventResultEnum[result]; !valid {
				rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "UNKNOWN_EVENT_RESULT", Message: "unknown event.result: " + result})
			}
		}
		checkStateAuditRule(rep, line, r, event)
	}
}

func checkStateWorklogRows(rep *Report, tickets, rows []ledger.Row) {
	prevTS := ""
	byTicket := map[string][]ledger.Row{}
	for _, t := range tickets {
		id, _ := t["id"].(string)
		if id == "" {
			continue
		}
		byTicket[id] = append(byTicket[id], t)
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
	for i, r := range rows {
		line := i + 1
		if !isStateWorklogRow(r) {
			checkCompatWorklogRow(rep, line, r, &prevTS)
			continue
		}
		checkCommonEnvelope(rep, "ledger/worklog.jsonl", line, r, &prevTS)
		for _, f := range ledger.StateWorklogRequired {
			if _, ok := r[f]; !ok {
				rep.Fails = append(rep.Fails, Issue{File: "ledger/worklog.jsonl", Line: line, Code: "MISSING_REQUIRED", Message: "missing required field: " + f})
			}
		}
		for _, f := range ledger.StateWorklogNonEmpty {
			if v, ok := r[f].(string); !ok || strings.TrimSpace(v) == "" {
				rep.Fails = append(rep.Fails, Issue{File: "ledger/worklog.jsonl", Line: line, Code: "NON_EMPTY_VIOLATION", Message: "field must be non-empty: " + f})
			}
		}
		id, _ := r["ticket"].(string)
		t, ok := latestStateTicketAtOrBefore(byTicket[id], stringField(r, "ts"))
		if !ok {
			// State-model migration preserves old worklogs whose historical ticket
			// rows may live outside the current tickets ledger. New CLI writes
			// are gated before append, so absence here is compatibility data.
			continue
		}
		if state, _ := t["state"].(string); state != "done" {
			rep.Warns = append(rep.Warns, Issue{File: "ledger/worklog.jsonl", Line: line, Code: "PREMATURE_WORKLOG", Message: fmt.Sprintf("[PREMATURE_WORKLOG] %s", id)})
		}
	}
}

func latestStateTicketAtOrBefore(rows []ledger.Row, ts string) (ledger.Row, bool) {
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

func checkStateTransitions(rep *Report, tickets []ledger.Row) {
	byID := map[string][]ledger.Row{}
	for _, r := range tickets {
		id, _ := r["id"].(string)
		if id == "" {
			continue
		}
		byID[id] = append(byID[id], r)
	}
	for id, rows := range byID {
		sort.SliceStable(rows, func(i, j int) bool {
			ni, _ := rows[i]["n"].(float64)
			nj, _ := rows[j]["n"].(float64)
			return ni < nj
		})
		prev := ""
		for _, r := range rows {
			cur, _ := r["state"].(string)
			if cur == "" || cur == prev {
				continue
			}
			if !ledger.AllowsStateTransition(prev, cur) {
				ln, _ := numberAsInt(r["n"])
				rep.Warns = append(rep.Warns, Issue{
					File:    "ledger/tickets.jsonl",
					Line:    ln,
					Code:    "INVALID_TRANSITION",
					Message: fmt.Sprintf("[INVALID_TRANSITION] %s: %s -> %s", id, prev, cur),
				})
			}
			prev = cur
		}
	}
}
