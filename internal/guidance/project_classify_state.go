package guidance

import "github.com/hgwk/ldgr/internal/ledger"

func classifyStateForRole(r ledger.Row, role string) (ProjectQueueItem, bool) {
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
		if s == "done" && !isStateAuditPassRow(r) {
			relevant = true
			severity = "critical"
			reason = "weak done (no state-model audit-pass event)"
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
