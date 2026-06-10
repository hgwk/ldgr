package guidance

import "github.com/hgwk/ldgr/internal/ledger"

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
