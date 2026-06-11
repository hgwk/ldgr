package verify

import (
	"fmt"
	"sort"

	"github.com/hgwk/ldgr/internal/ledger"
)

func invalidatedLines(rows []ledger.Row) map[int]struct{} {
	out := map[int]struct{}{}
	for _, r := range rows {
		n, ok := r["invalidates_n"].(float64)
		if !ok || n <= 0 || n != float64(int(n)) {
			continue
		}
		out[int(n)] = struct{}{}
	}
	return out
}

func checkOrphans(rep *Report, tickets, worklog []ledger.Row) {
	known := map[string]struct{}{}
	for _, t := range tickets {
		if id, ok := t["ticket"].(string); ok {
			known[id] = struct{}{}
		}
	}
	for i, w := range worklog {
		id, hasTicket := w["ticket"].(string)
		if !hasTicket || id == "" {
			continue
		}
		if _, ok := known[id]; !ok {
			rep.Warns = append(rep.Warns, Issue{File: "ledger/worklog.jsonl", Line: i + 1, Code: "ORPHAN_WORKLOG", Message: "orphan worklog: ticket not found in tickets.jsonl: " + id})
		}
	}
}

func checkBlockers(rep *Report, tickets []ledger.Row) {
	latest := map[string]ledger.Row{}
	latestLine := map[string]int{}
	for i, t := range tickets {
		if id, ok := t["ticket"].(string); ok && id != "" {
			latest[id] = t
			latestLine[id] = i + 1
		}
	}
	for id, t := range latest {
		status, _ := t["status"].(string)
		if status == "done" || status == "cancelled" {
			continue
		}
		line := latestLine[id]
		blocked, _ := t["blocked_by"].([]any)
		if status == "blocked" && len(blocked) == 0 {
			rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "BLOCKED_NO_BLOCKERS", Message: "blocked ticket has empty blocked_by"})
			continue
		}
		unresolved := 0
		for _, raw := range blocked {
			id, _ := raw.(string)
			b := latest[id]
			bs, _ := b["status"].(string)
			if bs == "done" || bs == "cancelled" {
				rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "STALE_BLOCKER", Message: "stale blocker is already closed: " + id})
				continue
			}
			unresolved++
		}
		if status == "blocked" && unresolved == 0 {
			rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "BLOCKED_NO_BLOCKERS", Message: "blocked ticket has no unresolved blockers"})
		}
	}
}

func checkParents(rep *Report, tickets []ledger.Row, parents []string) {
	if len(parents) == 0 {
		return
	}
	allowed := map[string]struct{}{}
	for _, p := range parents {
		allowed[p] = struct{}{}
	}
	for i, t := range tickets {
		p, _ := t["parent_ticket"].(string)
		if p == "" {
			continue
		}
		if _, ok := allowed[p]; !ok {
			rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: i + 1, Code: "UNKNOWN_PARENT", Message: "unknown parent_ticket: " + p})
		}
	}
}

func checkLifecycleTransitions(rep *Report, tickets []ledger.Row) {
	// Bucket rows by ticket id, excluding correction rows.
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

	// For each ticket, sort rows by n and check transitions.
	for id, rows := range byTicket {
		sort.SliceStable(rows, func(i, j int) bool {
			ai, _ := rows[i]["n"].(float64)
			aj, _ := rows[j]["n"].(float64)
			return ai < aj
		})
		for i := 1; i < len(rows); i++ {
			prevS, _ := rows[i-1]["status"].(string)
			curS, _ := rows[i]["status"].(string)
			if curS == "" || prevS == curS {
				continue
			}
			if !ledger.AllowsCompatStatusTransition(prevS, curS) {
				ln, _ := rows[i]["n"].(float64)
				rep.Warns = append(rep.Warns, Issue{
					File:    "ledger/tickets.jsonl",
					Line:    int(ln),
					Code:    "INVALID_TRANSITION",
					Message: fmt.Sprintf("[INVALID_TRANSITION] %s: %s -> %s", id, prevS, curS),
				})
			}
		}
	}
}

func checkWeakDone(rep *Report, tickets []ledger.Row) {
	for _, r := range tickets {
		if _, isCompanion := r["invalidates_n"]; isCompanion {
			continue
		}
		if s, _ := r["status"].(string); s != "done" {
			continue
		}
		id, _ := r["ticket"].(string)
		ln, _ := r["n"].(float64)

		if role, _ := r["role"].(string); role != "audit" {
			rep.Warns = append(rep.Warns, Issue{
				File:    "ledger/tickets.jsonl",
				Line:    int(ln),
				Code:    "WEAK_DONE",
				Message: fmt.Sprintf("[WEAK_DONE] %s: role != audit", id),
			})
			continue
		}
		if ar, _ := r["audit_result"].(string); ar != "pass" {
			rep.Warns = append(rep.Warns, Issue{
				File:    "ledger/tickets.jsonl",
				Line:    int(ln),
				Code:    "WEAK_DONE",
				Message: fmt.Sprintf("[WEAK_DONE] %s: audit_result != pass", id),
			})
			continue
		}
		if !hasNonEmptyEvidence(r) {
			rep.Warns = append(rep.Warns, Issue{
				File:    "ledger/tickets.jsonl",
				Line:    int(ln),
				Code:    "WEAK_DONE",
				Message: fmt.Sprintf("[WEAK_DONE] %s: evidence empty", id),
			})
			continue
		}
		if !hasPositiveNumber(r, "reviewed_n") {
			rep.Warns = append(rep.Warns, Issue{
				File:    "ledger/tickets.jsonl",
				Line:    int(ln),
				Code:    "WEAK_DONE",
				Message: fmt.Sprintf("[WEAK_DONE] %s: reviewed_n missing", id),
			})
		}
		if !hasGitCompletionEvidence(r) {
			rep.Warns = append(rep.Warns, Issue{
				File:    "ledger/tickets.jsonl",
				Line:    int(ln),
				Code:    "DONE_MISSING_GIT_EVIDENCE",
				Message: fmt.Sprintf("[DONE_MISSING_GIT_EVIDENCE] %s: done row should include commit:<sha>, pr:<url-or-number>, or no_commit:<reason> evidence", id),
			})
		}
	}
}
