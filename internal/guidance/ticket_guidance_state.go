package guidance

import (
	"fmt"

	"github.com/hgwk/ldgr/internal/ledger"
)

func ComputeState(latest ledger.Row, worklog []ledger.Row) Guidance {
	g := Guidance{
		ID:    stringField(latest, "id"),
		State: stringField(latest, "state"),
	}
	switch g.State {
	case "backlog":
		g.Summary = "not ready; planning or triage needed"
		g.Actions = []string{
			"Clarify acceptance and blockers before implementation.",
			"Move to ready when the next owner can start without more product decisions.",
		}
		g.SuggestedCommands = []string{"ldgr ticket event --json @-"}
		g.SuggestedJSON = []any{overlayState(latest, map[string]any{"state": "ready", "event": map[string]any{"role": "planner", "summary": "ready", "notes": ""}})}
	case "ready":
		g.Summary = "ready to claim"
		g.Actions = []string{
			"Claim the ticket by moving state=doing and setting the touched paths in notes or evidence when useful.",
			"Confirm archive/reference/new judgment is recorded in event.notes before implementation.",
		}
		g.SuggestedCommands = []string{"ldgr ticket event --json @-"}
		g.SuggestedJSON = []any{overlayState(latest, map[string]any{"state": "doing", "event": map[string]any{"role": "implementer", "summary": "started", "notes": ""}})}
	case "doing":
		g.Summary = "implementation active"
		g.Actions = []string{
			"Finish implementation and test evidence.",
			"Move to review only when test evidence is ready; do not append worklog yet.",
		}
		g.SuggestedCommands = []string{"ldgr ticket event --json @-"}
		g.SuggestedJSON = []any{overlayState(latest, map[string]any{"state": "review", "evidence": []any{"test:unit:<command-or-test-marker>"}, "event": map[string]any{"role": "implementer", "summary": "ready for review", "notes": ""}})}
	case "blocked":
		g.Summary = "blocked"
		g.Actions = []string{"Resolve blocked_by before implementation continues."}
		blockers := stringSliceField(latest, "blocked_by")
		if len(blockers) == 0 {
			g.Warnings = append(g.Warnings, Warning{Code: "BLOCKED_NO_BLOCKERS", Severity: "warning", Message: "state=blocked but blocked_by is empty"})
		}
	case "review":
		g.Summary = "awaiting audit"
		g.Actions = []string{
			"Auditor decides pass or changes_requested.",
			"Pass moves state=done with event.result=pass and reviewed_n.",
			"Requested changes move state=rework with event.result=changes_requested and reviewed_n.",
		}
		g.SuggestedCommands = []string{"ldgr ticket event --json @-"}
		g.SuggestedJSON = []any{
			overlayState(latest, map[string]any{"state": "done", "event": map[string]any{"role": "auditor", "result": "pass", "reviewed_n": 0, "summary": "passed", "notes": ""}}),
			overlayState(latest, map[string]any{"state": "rework", "event": map[string]any{"role": "auditor", "result": "changes_requested", "reviewed_n": 0, "summary": "changes requested", "notes": ""}}),
		}
	case "rework":
		g.Summary = "audit requested changes"
		g.Actions = []string{"Resume implementation with state=doing or send back to ready if planning is needed."}
		g.SuggestedCommands = []string{"ldgr ticket event --json @-"}
		g.SuggestedJSON = []any{overlayState(latest, map[string]any{"state": "doing", "event": map[string]any{"role": "implementer", "summary": "resumed", "notes": ""}})}
	case "done":
		if isStateAuditPass(latest) {
			g.Summary = "audit passed; record worklog"
			g.Actions = []string{"Append a state-model worklog row for the completed delivery."}
			g.SuggestedCommands = []string{fmt.Sprintf("ldgr suggest worklog --ticket %s", g.ID)}
		} else {
			g.Summary = "done without a valid audit pass"
			g.Warnings = []Warning{{Code: "WEAK_DONE", Severity: "critical", Message: "state=done lacks event.result=pass, reviewed_n, or evidence"}}
		}
	case "dropped":
		g.Summary = "dropped"
		g.Actions = []string{"Keep the reason in event.notes. Do not append a worklog unless the drop itself is the delivery."}
	default:
		g.Summary = "unknown state"
		g.Warnings = []Warning{{Code: "INVALID_STATE", Severity: "warning", Message: fmt.Sprintf("unrecognized state: %q", g.State)}}
	}
	g.NextTransitions = ledger.NextStates(g.State)
	_ = worklog
	return g
}
