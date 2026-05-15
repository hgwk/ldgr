// Package guidance derives state-aware next-action guidance from the latest
// ticket row. Pure functions; no I/O.
package guidance

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hgwk/ldgr/internal/ledger"
)

// allowedNextTransitions defines the valid state transitions for each status.
var allowedNextTransitions = map[string][]string{
	"":                  {"open", "in_progress"},
	"open":              {"in_progress", "blocked", "cancelled"},
	"in_progress":       {"audit_ready", "blocked", "cancelled"},
	"blocked":           {"in_progress", "cancelled"},
	"audit_ready":       {"done", "changes_requested", "cancelled"},
	"changes_requested": {"in_progress", "open", "cancelled"},
	"done":              nil,
	"cancelled":         nil,
}

var allowedNextTransitionsCanonical = map[string][]string{
	"":        {"backlog", "ready"},
	"backlog": {"ready", "dropped"},
	"ready":   {"doing", "blocked", "dropped"},
	"doing":   {"review", "blocked", "dropped"},
	"blocked": {"ready", "doing", "dropped"},
	"review":  {"done", "rework", "dropped"},
	"rework":  {"doing", "ready", "dropped"},
	"done":    nil,
	"dropped": nil,
}

// Warning represents a severity-coded warning for a ticket.
type Warning struct {
	Code     string `json:"code"`
	Severity string `json:"severity"` // critical | warning | hint
	Message  string `json:"message"`
}

// Guidance is the wire shape for both stderr text rendering and the `next` JSON output.
type Guidance struct {
	Ticket            string    `json:"ticket,omitempty"`
	Status            string    `json:"status,omitempty"`
	ID                string    `json:"id,omitempty"`
	State             string    `json:"state,omitempty"`
	WritingLanguage   string    `json:"writing_language,omitempty"`
	Summary           string    `json:"summary"`
	Actions           []string  `json:"actions"`
	Warnings          []Warning `json:"warnings"`
	SuggestedCommands []string  `json:"suggested_commands"`
	SuggestedJSON     []any     `json:"suggested_json"`
	NextTransitions   []string  `json:"next_transitions"`
}

// Compute derives guidance for the latest ticket row.
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
		g.SuggestedJSON = []any{overlay(latest, map[string]any{"status": "audit_ready", "evidence": []any{}})}
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
				"role": "audit", "status": "done", "audit_result": "pass", "evidence": []any{},
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
				"role": "audit", "status": "done", "audit_result": "pass", "evidence": []any{},
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
	g.NextTransitions = allowedNextTransitions[g.Status]
	_ = worklog
	return g
}

// ComputeCanonical derives guidance for a canonical v1 latest ticket row. The name is
// historical and should be collapsed in a later mechanical rename.
func ComputeCanonical(latest ledger.Row, worklog []ledger.Row) Guidance {
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
		g.SuggestedJSON = []any{overlayCanonical(latest, map[string]any{"state": "ready", "event": map[string]any{"role": "planner", "summary": "ready", "notes": ""}})}
	case "ready":
		g.Summary = "ready to claim"
		g.Actions = []string{
			"Claim the ticket by moving state=doing and setting the touched paths in notes or evidence when useful.",
			"Confirm archive/reference/new judgment is recorded in event.notes before implementation.",
		}
		g.SuggestedCommands = []string{"ldgr ticket event --json @-"}
		g.SuggestedJSON = []any{overlayCanonical(latest, map[string]any{"state": "doing", "event": map[string]any{"role": "implementer", "summary": "started", "notes": ""}})}
	case "doing":
		g.Summary = "implementation active"
		g.Actions = []string{
			"Finish implementation and verification evidence.",
			"Move to review only when evidence is ready; do not append worklog yet.",
		}
		g.SuggestedCommands = []string{"ldgr ticket event --json @-"}
		g.SuggestedJSON = []any{overlayCanonical(latest, map[string]any{"state": "review", "evidence": []any{}, "event": map[string]any{"role": "implementer", "summary": "ready for review", "notes": ""}})}
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
			overlayCanonical(latest, map[string]any{"state": "done", "event": map[string]any{"role": "auditor", "result": "pass", "reviewed_n": 0, "summary": "passed", "notes": ""}}),
			overlayCanonical(latest, map[string]any{"state": "rework", "event": map[string]any{"role": "auditor", "result": "changes_requested", "reviewed_n": 0, "summary": "changes requested", "notes": ""}}),
		}
	case "rework":
		g.Summary = "audit requested changes"
		g.Actions = []string{"Resume implementation with state=doing or send back to ready if planning is needed."}
		g.SuggestedCommands = []string{"ldgr ticket event --json @-"}
		g.SuggestedJSON = []any{overlayCanonical(latest, map[string]any{"state": "doing", "event": map[string]any{"role": "implementer", "summary": "resumed", "notes": ""}})}
	case "done":
		if isCanonicalAuditPass(latest) {
			g.Summary = "audit passed; record worklog"
			g.Actions = []string{"Append a canonical v1 worklog row for the completed delivery."}
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
	g.NextTransitions = allowedNextTransitionsCanonical[g.State]
	_ = worklog
	return g
}

// RenderText formats Guidance for stderr / human consumption.
func RenderText(g Guidance) string {
	var b strings.Builder
	id, state := g.Ticket, g.Status
	if id == "" {
		id = g.ID
	}
	if state == "" {
		state = g.State
	}
	fmt.Fprintf(&b, "Ticket %s is %s — %s\n", id, state, g.Summary)
	if len(g.Actions) > 0 {
		b.WriteString("\nNext:\n")
		for _, a := range g.Actions {
			fmt.Fprintf(&b, "- %s\n", a)
		}
	}
	if len(g.Warnings) > 0 {
		b.WriteString("\nWarnings:\n")
		for _, w := range g.Warnings {
			fmt.Fprintf(&b, "- [%s/%s] %s\n", w.Severity, w.Code, w.Message)
		}
	}
	if len(g.SuggestedCommands) > 0 {
		b.WriteString("\nSuggested:\n")
		for _, c := range g.SuggestedCommands {
			fmt.Fprintf(&b, "  %s\n", c)
		}
	}
	if len(g.NextTransitions) > 0 {
		b.WriteString("\nNext transitions: " + strings.Join(g.NextTransitions, ", ") + "\n")
	}
	if g.WritingLanguage != "" {
		fmt.Fprintf(&b, "\nWriting language: %s\n", g.WritingLanguage)
	}
	return b.String()
}

// RenderJSON returns the canonical machine-readable form.
func RenderJSON(g Guidance) ([]byte, error) {
	return json.MarshalIndent(g, "", "  ")
}

func overlay(base ledger.Row, fields map[string]any) map[string]any {
	out := map[string]any{}
	for _, k := range []string{"ticket", "parent_ticket", "agent", "role", "task", "scope", "paths", "blocked_by", "branch", "category"} {
		if v, ok := base[k]; ok {
			out[k] = v
		}
	}
	out["ticket"] = stringField(base, "ticket")
	for k, v := range fields {
		out[k] = v
	}
	return out
}

func overlayCanonical(base ledger.Row, fields map[string]any) map[string]any {
	out := map[string]any{}
	for _, k := range []string{"id", "parent", "type", "state", "area", "priority", "title", "owner", "blocked_by", "acceptance", "evidence"} {
		if v, ok := base[k]; ok {
			out[k] = v
		}
	}
	for k, v := range fields {
		out[k] = v
	}
	return out
}

func isAuditPass(latest ledger.Row) bool {
	v, _ := latest["audit_result"].(string)
	return v == "pass"
}

func isCanonicalAuditPass(latest ledger.Row) bool {
	if state, _ := latest["state"].(string); state != "done" {
		return false
	}
	event, _ := latest["event"].(map[string]any)
	if event == nil {
		return false
	}
	if role, _ := event["role"].(string); role != "auditor" {
		return false
	}
	if result, _ := event["result"].(string); result != "pass" {
		return false
	}
	if _, ok := event["reviewed_n"].(float64); !ok {
		return false
	}
	return len(stringSliceField(latest, "evidence")) > 0
}

func stringField(r ledger.Row, k string) string {
	v, _ := r[k].(string)
	return v
}

func stringSliceField(r ledger.Row, k string) []string {
	raw, _ := r[k].([]any)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
