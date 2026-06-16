package guidance

import (
	"fmt"

	"github.com/hgwk/ldgr/internal/ledger"
)

func Compute(latest ledger.Row, worklog []ledger.Row) Guidance {
	g := Guidance{
		Ticket: stringField(latest, "ticket"),
		Status: stringField(latest, "status"),
	}
	switch g.Status {
	case "open":
		g.Summary = "ready to plan; claim before editing"
		g.Actions = []string{
			"Claim this ticket: append a ticket event with status=in_progress and paths you'll touch.",
			"Confirm `acceptance` is filled and `category`/`parent_ticket` make sense.",
			"Capture archive/reference review in `notes` before implementation.",
		}
		g.SuggestedCommands = []string{"ldgr ticket event --json @-"}
		g.SuggestedJSON = []any{overlay(latest, map[string]any{"status": "in_progress"})}
	case "in_progress":
		g.Summary = "claim active; implementing"
		g.Actions = []string{
			"Keep `paths` accurate; do not edit paths claimed by another agent.",
			"When implementation is finished, append a ticket event with status=audit_ready and include evidence.",
		}
		g.SuggestedCommands = []string{"ldgr ticket event --json @-"}
		g.SuggestedJSON = []any{overlay(latest, map[string]any{"status": "audit_ready", "evidence": []any{"test:unit:<command-or-test-marker>"}})}
	case "blocked":
		g.Summary = "waiting on blockers"
		g.Actions = []string{"Do not implement until at least one blocker clears."}
		blockers := stringSliceField(latest, "blocked_by")
		if len(blockers) == 0 {
			g.Warnings = append(g.Warnings, Warning{
				Code:     "BLOCKED_NO_BLOCKERS",
				Severity: "warning",
				Message:  "status=blocked but blocked_by is empty; add the actual blocker ticket ids",
			})
		} else {
			for _, b := range blockers {
				g.Warnings = append(g.Warnings, Warning{
					Code:     "BLOCKED_BY",
					Severity: "warning",
					Message:  fmt.Sprintf("blocked by %s", b),
				})
			}
			g.SuggestedCommands = append(g.SuggestedCommands, fmt.Sprintf("ldgr next --ticket %s", blockers[0]))
		}
	case "audit_ready":
		g.Summary = "implementation finished; awaiting audit"
		g.Actions = []string{
			"Do not append a worklog yet — worklog follows audit pass.",
			"Append a ticket event with role=audit and either audit_result=pass (status=done) or audit_result=changes_requested (status=changes_requested).",
			"Include `evidence`: tests, verify command, diff review, screenshot/report when relevant.",
		}
		g.SuggestedCommands = []string{"ldgr ticket event --json @-"}
		g.SuggestedJSON = []any{
			overlay(latest, map[string]any{
				"role": "audit", "status": "done", "audit_result": "pass", "evidence": []any{"test:unit:<command-or-test-marker>"},
			}),
			overlay(latest, map[string]any{
				"role": "audit", "status": "changes_requested", "audit_result": "changes_requested",
				"audit_notes": "", "evidence": []any{},
			}),
		}
	case "changes_requested":
		g.Summary = "audit returned changes; resume implementation"
		g.Actions = []string{
			"Do not append a worklog.",
			"Resume with status=in_progress; carry `audit_notes` into your implementation notes.",
		}
		g.SuggestedCommands = []string{"ldgr ticket event --json @-"}
		g.SuggestedJSON = []any{overlay(latest, map[string]any{
			"status": "in_progress", "notes": stringField(latest, "audit_notes"),
		})}
	case "done":
		if isAuditPass(latest) {
			g.Summary = "audit passed; record the worklog"
			g.Actions = []string{"Append a worklog row for the shipped delivery, then prepare the commit / PR."}
			g.SuggestedCommands = []string{
				fmt.Sprintf("ldgr suggest worklog --ticket %s", g.Ticket),
				fmt.Sprintf("ldgr suggest commit --ticket %s", g.Ticket),
			}
		} else {
			g.Summary = "marked done without audit evidence"
			g.Warnings = []Warning{
				{
					Code:     "WEAK_DONE",
					Severity: "critical",
					Message:  "closure is weak: no audit pass row was found",
				},
			}
			g.Actions = []string{"Append a role=audit row with audit_result=pass and evidence before treating this as shipped."}
			g.SuggestedCommands = []string{"ldgr ticket event --json @-"}
			g.SuggestedJSON = []any{overlay(latest, map[string]any{
				"role": "audit", "status": "done", "audit_result": "pass", "evidence": []any{"test:unit:<command-or-test-marker>"},
			})}
		}
	case "cancelled":
		g.Summary = "cancelled"
		g.Actions = []string{"Explain the cancellation in `notes`. Do not append a worklog unless cancellation itself is the delivery."}
	default:
		g.Summary = "unknown status"
		g.Warnings = []Warning{
			{
				Code:     "INVALID_STATUS",
				Severity: "warning",
				Message:  fmt.Sprintf("unrecognized status: %q", g.Status),
			},
		}
	}
	// Populate NextTransitions from the allowed transitions map.
	g.NextTransitions = ledger.NextCompatStatuses(g.Status)
	_ = worklog
	return g
}

// ComputeState derives guidance for a state-model latest ticket row. The name is
