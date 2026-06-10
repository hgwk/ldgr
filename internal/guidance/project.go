package guidance

import (
	"sort"

	"github.com/hgwk/ldgr/internal/ledger"
)

type ProjectGuidance struct {
	Role            string             `json:"role"`
	WritingLanguage string             `json:"writing_language,omitempty"`
	Highlights      []ProjectQueueItem `json:"highlights"` // top 8
	Counts          ProjectCounts      `json:"counts"`
}

// ProjectQueueItem represents a single ticket in the project-wide queue.
type ProjectQueueItem struct {
	Ticket    string `json:"ticket,omitempty"`
	Status    string `json:"status,omitempty"`
	ID        string `json:"id,omitempty"`
	State     string `json:"state,omitempty"`
	Priority  string `json:"priority,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Severity  string `json:"severity"` // critical | warning | hint
	Reason    string `json:"reason"`
	Suggested string `json:"suggested"` // shell command hint
}

// ProjectCounts summarises the project's ticket state distribution.
type ProjectCounts struct {
	Active           int `json:"active"`
	Blocked          int `json:"blocked"`
	AuditReady       int `json:"audit_ready"`
	ChangesRequested int `json:"changes_requested"`
	StalePremature   int `json:"stale_premature"` // tickets done without audit
}

// LatestTickets returns one row per ticket ID: the highest-n non-invalidates row.
func LatestTickets(rows []ledger.Row) []ledger.Row {
	type entry struct {
		n   float64
		row ledger.Row
	}
	best := map[string]entry{}
	for _, r := range rows {
		if _, companion := r["invalidates_n"]; companion {
			continue
		}
		id, _ := r["ticket"].(string)
		if id == "" {
			continue
		}
		n, _ := r["n"].(float64)
		if cur, ok := best[id]; !ok || n > cur.n {
			best[id] = entry{n, r}
		}
	}
	out := make([]ledger.Row, 0, len(best))
	for _, e := range best {
		out = append(out, e.row)
	}
	return out
}

// LatestStateTickets returns one latest row per state-model ticket ID. The name
// is historical and should be collapsed in a later mechanical rename.
func LatestStateTickets(rows []ledger.Row) []ledger.Row {
	type entry struct {
		n   float64
		row ledger.Row
	}
	best := map[string]entry{}
	for _, r := range rows {
		id, _ := r["id"].(string)
		if id == "" {
			continue
		}
		n, _ := r["n"].(float64)
		if cur, ok := best[id]; !ok || n > cur.n {
			best[id] = entry{n, r}
		}
	}
	out := make([]ledger.Row, 0, len(best))
	for _, e := range best {
		out = append(out, e.row)
	}
	return out
}

// ComputeProject returns project-wide guidance derived from latest ticket rows,
// filtered by role (implementer|auditor|planner|maintainer). Empty role = "all".
func ComputeProject(ticketRows, worklogRows []ledger.Row, role string) ProjectGuidance {
	latest := LatestTickets(ticketRows)
	pg := ProjectGuidance{Role: role}

	// Counts.
	for _, r := range latest {
		s, _ := r["status"].(string)
		switch s {
		case "open", "in_progress":
			pg.Counts.Active++
		case "blocked":
			pg.Counts.Blocked++
		case "audit_ready":
			pg.Counts.AuditReady++
		case "changes_requested":
			pg.Counts.ChangesRequested++
		case "done":
			if !isAuditPassRow(r) {
				pg.Counts.StalePremature++
			}
		}
	}

	var items []ProjectQueueItem
	for _, r := range latest {
		if it, ok := classifyForRole(r, role); ok {
			items = append(items, it)
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		si, sj := severityRank(items[i].Severity), severityRank(items[j].Severity)
		if si != sj {
			return si < sj
		}
		pi, pj := priorityRank(items[i].Priority), priorityRank(items[j].Priority)
		if pi != pj {
			return pi < pj
		}
		return items[i].Ticket < items[j].Ticket
	})

	if len(items) > 8 {
		items = items[:8]
	}
	pg.Highlights = items
	return pg
}

// ComputeStateProject returns project-wide guidance for state-model ledgers. The
// name is historical and should be collapsed in a later mechanical rename.
func ComputeStateProject(ticketRows, worklogRows []ledger.Row, role string) ProjectGuidance {
	latest := LatestStateTickets(ticketRows)
	pg := ProjectGuidance{Role: role}

	for _, r := range latest {
		s, _ := r["state"].(string)
		switch s {
		case "ready", "doing":
			pg.Counts.Active++
		case "blocked":
			pg.Counts.Blocked++
		case "review":
			pg.Counts.AuditReady++
		case "rework":
			pg.Counts.ChangesRequested++
		case "done":
			if !isStateAuditPassRow(r) {
				pg.Counts.StalePremature++
			}
		}
	}

	var items []ProjectQueueItem
	for _, r := range latest {
		if it, ok := classifyStateForRole(r, role); ok {
			items = append(items, it)
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		si, sj := severityRank(items[i].Severity), severityRank(items[j].Severity)
		if si != sj {
			return si < sj
		}
		pi, pj := priorityRank(items[i].Priority), priorityRank(items[j].Priority)
		if pi != pj {
			return pi < pj
		}
		return items[i].ID < items[j].ID
	})
	if len(items) > 8 {
		items = items[:8]
	}
	pg.Highlights = items
	_ = worklogRows
	return pg
}
