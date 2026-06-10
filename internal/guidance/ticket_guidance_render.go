package guidance

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hgwk/ldgr/internal/ledger"
)

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

// RenderJSON returns the state-shaped machine-readable form.
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

func overlayState(base ledger.Row, fields map[string]any) map[string]any {
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

func isStateAuditPass(latest ledger.Row) bool {
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
