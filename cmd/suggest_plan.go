package cmd

import (
	"encoding/json"
	"io"

	"github.com/hgwk/ldgr/internal/ledger"
)

func suggestPlan(latest ledger.Row, ticket string, writingLanguage string, stdout io.Writer) int {
	// If ticket doesn't exist (latest is nil), create a new skeleton with defaults
	var parentTicket, scope string
	var paths []any

	if latest != nil {
		// Carry forward fields from existing ticket
		parentTicket = stringField(latest, "parent_ticket")
		scope = stringField(latest, "scope")
		if pathsVal, ok := latest["paths"].([]any); ok {
			paths = pathsVal
		} else {
			paths = []any{}
		}
	} else {
		// Defaults for new ticket
		parentTicket = "ROOT"
		scope = "repo"
		paths = []any{}
	}

	skeleton := map[string]any{
		"ticket":        ticket,
		"parent_ticket": parentTicket,
		"role":          "plan",
		"kind":          "plan",
		"priority":      "P2",
		"status":        "open",
		"task":          localizedTaskPlaceholder(writingLanguage),
		"scope":         scope,
		"paths":         paths,
		"blocked_by":    []any{},
		"acceptance":    localizedAcceptancePlaceholder(writingLanguage),
	}
	addWritingLanguage(skeleton, writingLanguage)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(skeleton); err != nil {
		return 1
	}
	return 0
}

func suggestPlanState(latest ledger.Row, ticket string, writingLanguage string, stdout io.Writer) int {
	parent := "ROOT"
	area := "ops"
	acceptance := localizedAcceptancePlaceholder(writingLanguage)
	if latest != nil {
		parent = stringField(latest, "parent")
		area = stringField(latest, "area")
		if v, ok := latest["acceptance"].([]any); ok {
			acceptance = v
		}
	}
	skeleton := map[string]any{
		"id":         ticket,
		"parent":     parent,
		"type":       "plan",
		"state":      "backlog",
		"area":       area,
		"priority":   "P2",
		"title":      localizedTaskPlaceholder(writingLanguage),
		"blocked_by": []any{},
		"acceptance": acceptance,
		"evidence":   []any{},
		"event": map[string]any{
			"role":    "planner",
			"summary": localizedTaskPlaceholder(writingLanguage),
			"notes":   "",
		},
	}
	addWritingLanguage(skeleton, writingLanguage)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(skeleton); err != nil {
		return 1
	}
	return 0
}
