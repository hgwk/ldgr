// Package verify implements ledger validation per spec §6.
// Fail = blocks commits; Warn = surfaced but exit 0 unless --strict.
package verify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/hgwk/ldgr/internal/config"
	"github.com/hgwk/ldgr/internal/jsonio"
	"github.com/hgwk/ldgr/internal/ledger"
	"github.com/hgwk/ldgr/internal/lifecycle"
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

	if usesCanonicalRows(ticketRows, worklogRows) {
		checkCanonicalRows(&rep, ticketRows, worklogRows)
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

	if strict {
		rep.Fails = append(rep.Fails, rep.Warns...)
		rep.Warns = nil
	} else {
		rep.Warns = filterHistoricalBaselineWarnings(rep.Warns, baseline)
	}
	return rep, nil
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

func filterHistoricalBaselineWarnings(in []Issue, baseline historicalBaseline) []Issue {
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
		"ORPHAN_WORKLOG",
		"PREMATURE_WORKLOG",
		"WEAK_DONE",
		"INVALID_TRANSITION",
		"AUDIT_REVIEWED_N_MISMATCH",
		"INVALIDATED_HISTORICAL":
		return true
	default:
		return false
	}
}

func usesCanonicalRows(tickets, worklog []ledger.Row) bool {
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

var canonicalLifecycleTransitions = map[string]map[string]bool{
	"":        {"backlog": true, "ready": true},
	"backlog": {"ready": true, "dropped": true},
	"ready":   {"doing": true, "blocked": true, "dropped": true},
	"doing":   {"review": true, "blocked": true, "dropped": true},
	"blocked": {"ready": true, "doing": true, "dropped": true},
	"review":  {"done": true, "rework": true, "dropped": true},
	"rework":  {"doing": true, "ready": true, "dropped": true},
}

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
			rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Code: "TS_NOT_INCREASING", Message: "ts decreases relative to previous row"})
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

func checkCanonicalRows(rep *Report, tickets, worklog []ledger.Row) {
	checkCanonicalTicketRows(rep, tickets)
	checkCanonicalWorklogRows(rep, tickets, worklog)
	checkCanonicalTransitions(rep, tickets)
}

func checkCanonicalTicketRows(rep *Report, rows []ledger.Row) {
	prevTS := ""
	for i, r := range rows {
		line := i + 1
		checkCommonEnvelope(rep, "ledger/tickets.jsonl", line, r, &prevTS)
		for _, f := range ledger.CanonicalTicketRequired {
			if _, ok := r[f]; !ok {
				rep.Fails = append(rep.Fails, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "MISSING_REQUIRED", Message: "missing required field: " + f})
			}
		}
		for _, f := range ledger.CanonicalTicketNonEmpty {
			if v, ok := r[f].(string); !ok || strings.TrimSpace(v) == "" {
				rep.Fails = append(rep.Fails, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "NON_EMPTY_VIOLATION", Message: "field must be non-empty: " + f})
			}
		}
		checkStringEnum(rep, "ledger/tickets.jsonl", line, r, "type", ledger.CanonicalTypeEnum, "INVALID_TYPE")
		checkStringEnum(rep, "ledger/tickets.jsonl", line, r, "state", ledger.CanonicalStateEnum, "INVALID_STATE")
		checkStringEnum(rep, "ledger/tickets.jsonl", line, r, "area", ledger.CanonicalAreaEnum, "INVALID_AREA")
		checkStringEnum(rep, "ledger/tickets.jsonl", line, r, "priority", ledger.PriorityEnum, "INVALID_PRIORITY")

		event, ok := r["event"].(map[string]any)
		if !ok {
			rep.Fails = append(rep.Fails, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "EVENT_INVALID", Message: "event must be an object"})
			continue
		}
		for _, f := range ledger.CanonicalEventRequired {
			if _, ok := event[f]; !ok {
				rep.Fails = append(rep.Fails, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "MISSING_REQUIRED", Message: "missing required field: event." + f})
			}
		}
		for _, f := range ledger.CanonicalEventNonEmpty {
			if v, ok := event[f].(string); !ok || strings.TrimSpace(v) == "" {
				rep.Fails = append(rep.Fails, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "NON_EMPTY_VIOLATION", Message: "field must be non-empty: event." + f})
			}
		}
		checkStringEnum(rep, "ledger/tickets.jsonl", line, event, "role", ledger.CanonicalEventRoleEnum, "INVALID_EVENT_ROLE")
		if result, ok := event["result"].(string); ok && result != "" {
			if _, valid := ledger.CanonicalEventResultEnum[result]; !valid {
				rep.Fails = append(rep.Fails, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "INVALID_EVENT_RESULT", Message: "invalid event.result: " + result})
			}
		}
		checkCanonicalAuditRule(rep, line, r, event)
	}
}

func checkCanonicalWorklogRows(rep *Report, tickets, rows []ledger.Row) {
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
		checkCommonEnvelope(rep, "ledger/worklog.jsonl", line, r, &prevTS)
		for _, f := range ledger.CanonicalWorklogRequired {
			if _, ok := r[f]; !ok {
				rep.Fails = append(rep.Fails, Issue{File: "ledger/worklog.jsonl", Line: line, Code: "MISSING_REQUIRED", Message: "missing required field: " + f})
			}
		}
		for _, f := range ledger.CanonicalWorklogNonEmpty {
			if v, ok := r[f].(string); !ok || strings.TrimSpace(v) == "" {
				rep.Fails = append(rep.Fails, Issue{File: "ledger/worklog.jsonl", Line: line, Code: "NON_EMPTY_VIOLATION", Message: "field must be non-empty: " + f})
			}
		}
		id, _ := r["ticket"].(string)
		t, ok := latestCanonicalTicketAtOrBefore(byTicket[id], stringField(r, "ts"))
		if !ok {
			// Canonical migration preserves old worklogs whose historical ticket
			// rows may live outside the current tickets ledger. New CLI writes
			// are gated before append, so absence here is compatibility data.
			continue
		}
		if state, _ := t["state"].(string); state != "done" {
			rep.Warns = append(rep.Warns, Issue{File: "ledger/worklog.jsonl", Line: line, Code: "PREMATURE_WORKLOG", Message: fmt.Sprintf("[PREMATURE_WORKLOG] %s", id)})
		}
	}
}

func latestCanonicalTicketAtOrBefore(rows []ledger.Row, ts string) (ledger.Row, bool) {
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

func checkCanonicalTransitions(rep *Report, tickets []ledger.Row) {
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
			if prev == "" {
				prev = cur
				continue
			}
			if !canonicalLifecycleTransitions[prev][cur] {
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

func checkCommonEnvelope(rep *Report, file string, line int, r ledger.Row, prevTS *string) {
	if n, ok := r["n"].(float64); !ok || int(n) != line {
		rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Code: "N_NOT_CONSECUTIVE", Message: fmt.Sprintf("n must equal %d, got %v", line, r["n"])})
	}
	ts, _ := r["ts"].(string)
	if !isoRe.MatchString(ts) {
		rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Code: "TS_NOT_ISO", Message: "ts not ISO8601 UTC: " + ts})
	} else if *prevTS != "" && compareTS(ts, *prevTS) < 0 {
		rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Code: "TS_NOT_INCREASING", Message: "ts decreases relative to previous row"})
	}
	*prevTS = ts
}

func checkStringEnum(rep *Report, file string, line int, r map[string]any, key string, enum map[string]struct{}, code string) {
	v, _ := r[key].(string)
	if v == "" {
		return
	}
	if _, ok := enum[v]; !ok {
		rep.Fails = append(rep.Fails, Issue{File: file, Line: line, Code: code, Message: "invalid " + key + ": " + v})
	}
}

func checkCanonicalAuditRule(rep *Report, line int, r ledger.Row, event map[string]any) {
	state, _ := r["state"].(string)
	switch state {
	case "done":
		if role, _ := event["role"].(string); role != "auditor" {
			rep.Fails = append(rep.Fails, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "AUDIT_PASS_INVALID", Message: "state=done requires event.role=auditor"})
		}
		if result, _ := event["result"].(string); result != "pass" {
			rep.Fails = append(rep.Fails, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "AUDIT_PASS_INVALID", Message: "state=done requires event.result=pass"})
		}
		if _, ok := numberAsInt(event["reviewed_n"]); !ok {
			rep.Fails = append(rep.Fails, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "AUDIT_PASS_INVALID", Message: "state=done requires event.reviewed_n"})
		}
		if !hasNonEmptyEvidence(r) {
			rep.Fails = append(rep.Fails, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "AUDIT_PASS_INVALID", Message: "state=done requires non-empty evidence"})
		}
	case "rework":
		if role, _ := event["role"].(string); role != "auditor" {
			rep.Fails = append(rep.Fails, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "REWORK_INVALID", Message: "state=rework requires event.role=auditor"})
		}
		if result, _ := event["result"].(string); result != "changes_requested" {
			rep.Fails = append(rep.Fails, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "REWORK_INVALID", Message: "state=rework requires event.result=changes_requested"})
		}
		if _, ok := numberAsInt(event["reviewed_n"]); !ok {
			rep.Fails = append(rep.Fails, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "REWORK_INVALID", Message: "state=rework requires event.reviewed_n"})
		}
		if notes, _ := event["notes"].(string); strings.TrimSpace(notes) == "" {
			rep.Fails = append(rep.Fails, Issue{File: "ledger/tickets.jsonl", Line: line, Code: "REWORK_INVALID", Message: "state=rework requires event.notes"})
		}
	}
}

func latestCanonicalTickets(rows []ledger.Row) map[string]ledger.Row {
	out := map[string]ledger.Row{}
	for _, r := range rows {
		id, _ := r["id"].(string)
		if id == "" {
			continue
		}
		n, _ := r["n"].(float64)
		if cur, ok := out[id]; ok {
			cn, _ := cur["n"].(float64)
			if n <= cn {
				continue
			}
		}
		out[id] = r
	}
	return out
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
			if !lifecycleTransitions[prevS][curS] {
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
	}
}

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

func compareTS(a, b string) int {
	at, aerr := time.Parse(time.RFC3339Nano, a)
	bt, berr := time.Parse(time.RFC3339Nano, b)
	if aerr == nil && berr == nil {
		if at.Before(bt) {
			return -1
		}
		if at.After(bt) {
			return 1
		}
		return 0
	}
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func stringField(r ledger.Row, key string) string {
	v, _ := r[key].(string)
	return v
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
	_, ok := numberAsInt(r[key])
	return ok
}

func numberAsInt(v any) (int, bool) {
	switch v := v.(type) {
	case float64:
		if v > 0 && v == float64(int(v)) {
			return int(v), true
		}
	case int:
		if v > 0 {
			return v, true
		}
	}
	return 0, false
}
