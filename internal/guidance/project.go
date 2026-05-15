package guidance

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hgwk/ldgr/internal/ledger"
)

// ProjectGuidance is the JSON shape for `ldgr next` without --ticket.
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

// LatestCanonicalTickets returns one latest row per canonical v1 ticket ID. The name
// is historical and should be collapsed in a later mechanical rename.
func LatestCanonicalTickets(rows []ledger.Row) []ledger.Row {
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

// ComputeCanonicalProject returns project-wide guidance for canonical v1 ledgers. The
// name is historical and should be collapsed in a later mechanical rename.
func ComputeCanonicalProject(ticketRows, worklogRows []ledger.Row, role string) ProjectGuidance {
	latest := LatestCanonicalTickets(ticketRows)
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
			if !isCanonicalAuditPassRow(r) {
				pg.Counts.StalePremature++
			}
		}
	}

	var items []ProjectQueueItem
	for _, r := range latest {
		if it, ok := classifyCanonicalForRole(r, role); ok {
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

func classifyForRole(r ledger.Row, role string) (ProjectQueueItem, bool) {
	s, _ := r["status"].(string)
	t, _ := r["ticket"].(string)
	p, _ := r["priority"].(string)
	k, _ := r["kind"].(string)

	relevant := false
	severity := "hint"
	reason := ""
	suggested := ""

	if role == "" || role == "implementer" {
		switch s {
		case "in_progress":
			relevant = true
			reason = "active implementation"
			suggested = "ldgr next --ticket " + t
			if p == "P0" {
				severity = "warning"
			}
		case "changes_requested":
			relevant = true
			severity = "warning"
			reason = "audit returned changes_requested"
			suggested = "ldgr ticket event --json @-  # resume in_progress"
		case "open":
			bb, _ := r["blocked_by"].([]any)
			if len(bb) == 0 {
				relevant = true
				reason = "ready to claim"
				suggested = "ldgr ticket event --json @-  # status=in_progress"
			}
		case "blocked":
			if p == "P0" || p == "P1" {
				relevant = true
				severity = "critical"
				reason = "high-priority blocker"
				suggested = "ldgr next --ticket " + t
			}
		}
	}

	if role == "auditor" || role == "" {
		if s == "audit_ready" {
			relevant = true
			severity = "warning"
			reason = "awaiting audit"
			suggested = "ldgr audit pass --ticket " + t + " --evidence ... | ldgr audit request-changes --ticket " + t + " --notes ..."
		}
	}

	if role == "planner" || role == "" {
		if s == "open" {
			relevant = true
			reason = "needs plan / acceptance"
			suggested = "ldgr next --ticket " + t
		}
		if s == "changes_requested" {
			relevant = true
			severity = "warning"
			reason = "plan needs adjustment after audit"
			suggested = "ldgr ticket event --json @-"
		}
	}

	if role == "maintainer" || role == "" {
		if s == "done" && !isAuditPassRow(r) {
			relevant = true
			severity = "critical"
			reason = "weak done (no audit-pass row)"
			suggested = "ldgr audit pass --ticket " + t + " --evidence ..."
		}
	}

	if !relevant {
		return ProjectQueueItem{}, false
	}
	return ProjectQueueItem{
		Ticket:    t,
		Status:    s,
		Priority:  p,
		Kind:      k,
		Severity:  severity,
		Reason:    reason,
		Suggested: suggested,
	}, true
}

func classifyCanonicalForRole(r ledger.Row, role string) (ProjectQueueItem, bool) {
	s, _ := r["state"].(string)
	id, _ := r["id"].(string)
	p, _ := r["priority"].(string)
	k, _ := r["type"].(string)

	relevant := false
	severity := "hint"
	reason := ""
	suggested := ""

	if role == "" || role == "implementer" {
		switch s {
		case "doing":
			relevant = true
			reason = "active implementation"
			suggested = "ldgr next --ticket " + id
			if p == "P0" {
				severity = "warning"
			}
		case "rework":
			relevant = true
			severity = "warning"
			reason = "audit returned changes"
			suggested = "ldgr ticket event --json @-  # state=doing"
		case "ready":
			bb, _ := r["blocked_by"].([]any)
			if len(bb) == 0 {
				relevant = true
				reason = "ready to claim"
				suggested = "ldgr ticket event --json @-  # state=doing"
			}
		case "blocked":
			if p == "P0" || p == "P1" {
				relevant = true
				severity = "critical"
				reason = "high-priority blocker"
				suggested = "ldgr next --ticket " + id
			}
		}
	}

	if role == "auditor" || role == "" {
		if s == "review" {
			relevant = true
			severity = "warning"
			reason = "awaiting audit"
			suggested = "ldgr suggest audit --ticket " + id
		}
	}

	if role == "planner" || role == "" {
		if s == "backlog" {
			relevant = true
			reason = "needs plan / acceptance"
			suggested = "ldgr next --ticket " + id
		}
		if s == "rework" {
			relevant = true
			severity = "warning"
			reason = "plan may need adjustment after audit"
			suggested = "ldgr next --ticket " + id
		}
	}

	if role == "maintainer" || role == "" {
		if s == "done" && !isCanonicalAuditPassRow(r) {
			relevant = true
			severity = "critical"
			reason = "weak done (no canonical v1 audit-pass event)"
			suggested = "ldgr next --ticket " + id
		}
	}

	if !relevant {
		return ProjectQueueItem{}, false
	}
	return ProjectQueueItem{
		ID:        id,
		State:     s,
		Priority:  p,
		Kind:      k,
		Severity:  severity,
		Reason:    reason,
		Suggested: suggested,
	}, true
}

func isAuditPassRow(r ledger.Row) bool {
	if role, _ := r["role"].(string); role != "audit" {
		return false
	}
	if ar, _ := r["audit_result"].(string); ar != "pass" {
		return false
	}
	return true
}

func isCanonicalAuditPassRow(r ledger.Row) bool {
	event, _ := r["event"].(map[string]any)
	if event == nil {
		return false
	}
	if role, _ := event["role"].(string); role != "auditor" {
		return false
	}
	if result, _ := event["result"].(string); result != "pass" {
		return false
	}
	_, ok := event["reviewed_n"].(float64)
	return ok
}

func severityRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	case "hint":
		return 2
	}
	return 3
}

func priorityRank(p string) int {
	switch p {
	case "P0":
		return 0
	case "P1":
		return 1
	case "P2":
		return 2
	case "P3":
		return 3
	}
	return 9
}

// RenderProjectText renders ProjectGuidance for stderr/human consumption.
func RenderProjectText(g ProjectGuidance) string {
	var b strings.Builder

	roleLabel := g.Role
	if roleLabel == "" {
		roleLabel = "all"
	}
	fmt.Fprintf(&b, "Project queue (role=%s)\n", roleLabel)
	fmt.Fprintf(&b, "  active=%d  blocked=%d  audit_ready=%d  changes_requested=%d  stale_premature=%d\n\n",
		g.Counts.Active, g.Counts.Blocked, g.Counts.AuditReady, g.Counts.ChangesRequested, g.Counts.StalePremature)

	if len(g.Highlights) == 0 {
		b.WriteString("  (nothing to highlight)\n")
		return b.String()
	}

	for _, h := range g.Highlights {
		sev := strings.ToUpper(h.Severity[:1]) + h.Severity[1:]
		id, state := h.Ticket, h.Status
		if id == "" {
			id = h.ID
		}
		if state == "" {
			state = h.State
		}
		fmt.Fprintf(&b, "[%s] %s  status=%-18s priority=%s\n", sev, id, state, h.Priority)
		fmt.Fprintf(&b, "        %s\n", h.Reason)
		if h.Suggested != "" {
			fmt.Fprintf(&b, "        $ %s\n", h.Suggested)
		}
	}
	if g.WritingLanguage != "" {
		fmt.Fprintf(&b, "\nWriting language: %s\n", g.WritingLanguage)
	}
	return b.String()
}

// RenderProjectJSON returns the canonical machine-readable form of ProjectGuidance.
func RenderProjectJSON(g ProjectGuidance) ([]byte, error) {
	return json.MarshalIndent(g, "", "  ")
}
