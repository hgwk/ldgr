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

// Guidance is the wire shape for both stderr text rendering and the `next` JSON output.
type Guidance struct {
	Ticket            string   `json:"ticket"`
	Status            string   `json:"status"`
	Summary           string   `json:"summary"`
	Actions           []string `json:"actions"`
	Warnings          []string `json:"warnings"`
	SuggestedCommands []string `json:"suggested_commands"`
	SuggestedJSON     []any    `json:"suggested_json"`
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
			g.Warnings = append(g.Warnings, "status=blocked but blocked_by is empty; add the actual blocker ticket ids")
		} else {
			for _, b := range blockers {
				g.Warnings = append(g.Warnings, fmt.Sprintf("blocked by %s", b))
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
			g.Warnings = []string{"closure is weak: no audit pass row was found"}
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
		g.Warnings = []string{fmt.Sprintf("unrecognized status: %q", g.Status)}
	}
	_ = worklog
	return g
}

// RenderText formats Guidance for stderr / human consumption.
func RenderText(g Guidance) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Ticket %s is %s — %s\n", g.Ticket, g.Status, g.Summary)
	if len(g.Actions) > 0 {
		b.WriteString("\nNext:\n")
		for _, a := range g.Actions {
			fmt.Fprintf(&b, "- %s\n", a)
		}
	}
	if len(g.Warnings) > 0 {
		b.WriteString("\nWarnings:\n")
		for _, w := range g.Warnings {
			fmt.Fprintf(&b, "- %s\n", w)
		}
	}
	if len(g.SuggestedCommands) > 0 {
		b.WriteString("\nSuggested:\n")
		for _, c := range g.SuggestedCommands {
			fmt.Fprintf(&b, "  %s\n", c)
		}
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

func isAuditPass(latest ledger.Row) bool {
	v, _ := latest["audit_result"].(string)
	return v == "pass"
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
