// Package verify implements ledger validation per spec §6.
// Fail = blocks commits; Warn = surfaced but exit 0 unless --strict.
package verify

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/hgwk/ldgr/internal/config"
	"github.com/hgwk/ldgr/internal/jsonio"
	"github.com/hgwk/ldgr/internal/ledger"
)

type Issue struct {
	File    string
	Line    int
	Message string
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
		rep.Fails = append(rep.Fails, Issue{File: "ledger/config.json", Message: "cannot read: " + err.Error()})
	} else {
		cfg = loaded
		if cfg.SchemaVersion == 0 || cfg.ProjectID == "" || cfg.Slug == "" {
			rep.Fails = append(rep.Fails, Issue{File: "ledger/config.json", Message: "missing required fields (schema_version/project_id/slug)"})
		}
	}

	goalPath := filepath.Join(targetDir, "ledger", "goal.json")
	var g ledger.Goal
	if err := jsonio.ReadJSON(goalPath, &g); err != nil {
		rep.Fails = append(rep.Fails, Issue{File: "ledger/goal.json", Message: "cannot read: " + err.Error()})
	} else if g.SchemaVersion == 0 {
		rep.Fails = append(rep.Fails, Issue{File: "ledger/goal.json", Message: "schema_version required"})
	}

	ticketRows, err := ledger.ReadRows(filepath.Join(targetDir, "ledger", "tickets.jsonl"))
	if err != nil {
		rep.Fails = append(rep.Fails, Issue{File: "ledger/tickets.jsonl", Message: err.Error()})
	}
	checkRows(&rep, "ledger/tickets.jsonl", ticketRows, ledger.TicketRequired, true)

	worklogRows, err := ledger.ReadRows(filepath.Join(targetDir, "ledger", "worklog.jsonl"))
	if err != nil {
		rep.Fails = append(rep.Fails, Issue{File: "ledger/worklog.jsonl", Message: err.Error()})
	}
	checkRows(&rep, "ledger/worklog.jsonl", worklogRows, ledger.WorklogRequired, false)

	checkOrphans(&rep, ticketRows, worklogRows)
	checkBlockers(&rep, ticketRows)
	checkParents(&rep, ticketRows, cfg.Parents)
	checkLifecycleTransitions(&rep, ticketRows)
	checkWeakDone(&rep, ticketRows)
	checkAuditReviewedN(&rep, ticketRows)
	checkPrematureWorklog(&rep, ticketRows, worklogRows)

	if strict {
		rep.Fails = append(rep.Fails, rep.Warns...)
		rep.Warns = nil
	}
	return rep, nil
}

var isoRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z$`)

// lifecycleTransitions mirrors the state machine in internal/lifecycle.
var lifecycleTransitions = map[string]map[string]bool{
	"":                  {"open": true, "in_progress": true},
	"open":              {"in_progress": true, "blocked": true, "cancelled": true},
	"in_progress":       {"audit_ready": true, "blocked": true, "cancelled": true},
	"blocked":           {"in_progress": true, "cancelled": true},
	"audit_ready":       {"done": true, "changes_requested": true, "cancelled": true},
	"changes_requested": {"in_progress": true, "open": true, "cancelled": true},
}

func checkRows(rep *Report, file string, rows []ledger.Row, required []string, isTicket bool) {
	prevTS := ""
	invalidated := invalidatedLines(rows)
	for i, r := range rows {
		line := i + 1
		if n, ok := r["n"].(float64); !ok || int(n) != line {
			rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Message: fmt.Sprintf("n must equal %d, got %v", line, r["n"])})
		}
		for _, f := range required {
			if _, ok := r[f]; !ok {
				rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Message: "missing required field: " + f})
			}
		}
		nonEmpty := ledger.WorklogNonEmpty
		if isTicket {
			nonEmpty = ledger.TicketNonEmpty
		}
		for _, f := range nonEmpty {
			if v, ok := r[f].(string); !ok || v == "" {
				if _, ok := invalidated[line]; ok {
					rep.Warns = append(rep.Warns, Issue{File: file, Line: line, Message: "invalid historical row superseded by invalidates_n"})
					continue
				}
				rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Message: "field must be non-empty: " + f})
			}
		}
		if !isTicket {
			if v, ok := r["ticket"].(string); ok && v == "" {
				if _, ok := invalidated[line]; ok {
					rep.Warns = append(rep.Warns, Issue{File: file, Line: line, Message: "invalid historical row superseded by invalidates_n"})
					continue
				}
				rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Message: "field must be non-empty when present: ticket"})
			}
		}
		ts, _ := r["ts"].(string)
		if !isoRe.MatchString(ts) {
			rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Message: "ts not ISO8601 UTC: " + ts})
		} else if prevTS != "" && ts < prevTS {
			rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Message: "ts decreases relative to previous row"})
		}
		prevTS = ts
		if isTicket {
			if s, ok := r["status"].(string); ok {
				if _, valid := ledger.StatusEnum[s]; !valid {
					rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Message: "invalid status: " + s})
				}
			}
			if c, ok := r["category"].(string); !ok || c == "" {
				rep.Warns = append(rep.Warns, Issue{File: file, Line: line, Message: "missing category"})
			} else if _, valid := ledger.CategoryEnum[c]; !valid {
				rep.Warns = append(rep.Warns, Issue{File: file, Line: line, Message: "unknown category: " + c})
			}
		}
	}
}

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
			rep.Warns = append(rep.Warns, Issue{File: "ledger/worklog.jsonl", Line: i + 1, Message: "orphan worklog: ticket not found in tickets.jsonl: " + id})
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
			rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: line, Message: "blocked ticket has empty blocked_by"})
			continue
		}
		unresolved := 0
		for _, raw := range blocked {
			id, _ := raw.(string)
			b := latest[id]
			bs, _ := b["status"].(string)
			if bs == "done" || bs == "cancelled" {
				rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: line, Message: "stale blocker is already closed: " + id})
				continue
			}
			unresolved++
		}
		if status == "blocked" && unresolved == 0 {
			rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: line, Message: "blocked ticket has no unresolved blockers"})
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
			rep.Warns = append(rep.Warns, Issue{File: "ledger/tickets.jsonl", Line: i + 1, Message: "unknown parent_ticket: " + p})
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
			if !lifecycleTransitions[prevS][curS] {
				ln, _ := rows[i]["n"].(float64)
				rep.Warns = append(rep.Warns, Issue{
					File:    "ledger/tickets.jsonl",
					Line:    int(ln),
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
				Message: fmt.Sprintf("[WEAK_DONE] %s: role != audit", id),
			})
			continue
		}
		if ar, _ := r["audit_result"].(string); ar != "pass" {
			rep.Warns = append(rep.Warns, Issue{
				File:    "ledger/tickets.jsonl",
				Line:    int(ln),
				Message: fmt.Sprintf("[WEAK_DONE] %s: audit_result != pass", id),
			})
			continue
		}
		if !hasNonEmptyEvidence(r) {
			rep.Warns = append(rep.Warns, Issue{
				File:    "ledger/tickets.jsonl",
				Line:    int(ln),
				Message: fmt.Sprintf("[WEAK_DONE] %s: evidence empty", id),
			})
		}
	}
}

func checkAuditReviewedN(rep *Report, tickets []ledger.Row) {
	for _, r := range tickets {
		if _, isCompanion := r["invalidates_n"]; isCompanion {
			continue
		}
		if role, _ := r["role"].(string); role != "audit" {
			continue
		}
		s, _ := r["status"].(string)
		if s != "done" && s != "changes_requested" {
			continue
		}
		if !hasPositiveNumber(r, "reviewed_n") {
			id, _ := r["ticket"].(string)
			ln, _ := r["n"].(float64)
			rep.Warns = append(rep.Warns, Issue{
				File:    "ledger/tickets.jsonl",
				Line:    int(ln),
				Message: fmt.Sprintf("[AUDIT_MISSING_REVIEWED_N] %s", id),
			})
		}
	}
}

func checkPrematureWorklog(rep *Report, tickets, worklog []ledger.Row) {
	// Build a map of latest ticket row per id (excluding correction rows).
	latest := map[string]ledger.Row{}
	for _, r := range tickets {
		if _, isCompanion := r["invalidates_n"]; isCompanion {
			continue
		}
		id, _ := r["ticket"].(string)
		if id == "" {
			continue
		}
		if cur, ok := latest[id]; ok {
			cn, _ := cur["n"].(float64)
			n, _ := r["n"].(float64)
			if n <= cn {
				continue
			}
		}
		latest[id] = r
	}

	// Helper: check if a row is audit-pass done.
	isAuditPass := func(r ledger.Row) bool {
		if s, _ := r["status"].(string); s != "done" {
			return false
		}
		if role, _ := r["role"].(string); role != "audit" {
			return false
		}
		if ar, _ := r["audit_result"].(string); ar != "pass" {
			return false
		}
		return true
	}

	// Check each worklog row.
	for _, w := range worklog {
		if _, isCompanion := w["invalidates_n"]; isCompanion {
			continue
		}
		id, _ := w["ticket"].(string)
		if id == "" {
			continue
		}
		lr, ok := latest[id]
		if !ok {
			continue // Orphan worklog caught elsewhere.
		}
		if !isAuditPass(lr) {
			ln, _ := w["n"].(float64)
			rep.Warns = append(rep.Warns, Issue{
				File:    "ledger/worklog.jsonl",
				Line:    int(ln),
				Message: fmt.Sprintf("[PREMATURE_WORKLOG] %s", id),
			})
		}
	}
}

func hasNonEmptyEvidence(r ledger.Row) bool {
	arr, _ := r["evidence"].([]any)
	for _, v := range arr {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return true
		}
	}
	return false
}

func hasPositiveNumber(r ledger.Row, key string) bool {
	switch v := r[key].(type) {
	case float64:
		return v > 0
	case int:
		return v > 0
	}
	return false
}
