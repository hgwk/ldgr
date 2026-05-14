// Package verify implements ledger validation per spec §6.
// Fail = blocks commits; Warn = surfaced but exit 0 unless --strict.
package verify

import (
	"fmt"
	"path/filepath"
	"regexp"

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

	if strict {
		rep.Fails = append(rep.Fails, rep.Warns...)
		rep.Warns = nil
	}
	return rep, nil
}

var isoRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z$`)

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
