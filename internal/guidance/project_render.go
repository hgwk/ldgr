package guidance

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hgwk/ldgr/internal/ledger"
)

func isAuditPassRow(r ledger.Row) bool {
	if role, _ := r["role"].(string); role != "audit" {
		return false
	}
	if ar, _ := r["audit_result"].(string); ar != "pass" {
		return false
	}
	return true
}

func isStateAuditPassRow(r ledger.Row) bool {
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
	if g.Team != "" {
		fmt.Fprintf(&b, "Project queue (role=%s team=%s)\n", roleLabel, g.Team)
	} else {
		fmt.Fprintf(&b, "Project queue (role=%s)\n", roleLabel)
	}
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

// RenderProjectJSON returns the state-shaped machine-readable form of ProjectGuidance.
func RenderProjectJSON(g ProjectGuidance) ([]byte, error) {
	return json.MarshalIndent(g, "", "  ")
}
