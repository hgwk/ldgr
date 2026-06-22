package cmd

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/hgwk/ldgr/internal/agent"
	"github.com/hgwk/ldgr/internal/ledger"
)

type suggestPlanOptions struct {
	Parent   string
	Area     string
	Owner    string
	Priority string
	Team     string
}

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

func suggestPlanState(latest ledger.Row, ticket string, writingLanguage string, opts suggestPlanOptions, stdout, stderr io.Writer) int {
	parent := "ROOT"
	area := "ops"
	priority := "P2"
	owner := opts.Owner
	team := ""
	acceptance := localizedAcceptancePlaceholder(writingLanguage)
	if latest != nil {
		parent = planStringDefault(stringField(latest, "parent"), parent)
		area = planStringDefault(stringField(latest, "area"), area)
		priority = planStringDefault(stringField(latest, "priority"), priority)
		owner = planStringDefault(owner, stringField(latest, "owner"))
		team = stringField(latest, "team")
		if v, ok := latest["acceptance"].([]any); ok {
			acceptance = v
		}
	}
	parent = planStringDefault(opts.Parent, parent)
	area = planStringDefault(opts.Area, area)
	priority = planStringDefault(opts.Priority, priority)
	team = planStringDefault(opts.Team, team)
	if owner == "" {
		resolved, warn, err := agent.Resolve("", envAsMap())
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		owner = resolved
		if warn != "" {
			fmt.Fprintln(stderr, "warning:", warn)
		}
	}
	skeleton := map[string]any{
		"id":         ticket,
		"parent":     parent,
		"type":       "plan",
		"state":      "backlog",
		"area":       area,
		"priority":   priority,
		"title":      localizedTaskPlaceholder(writingLanguage),
		"owner":      owner,
		"blocked_by": []any{},
		"acceptance": acceptance,
		"evidence":   []any{},
		"event": map[string]any{
			"actor":   owner,
			"role":    "planner",
			"summary": localizedTaskPlaceholder(writingLanguage),
			"notes":   "",
		},
	}
	if team != "" {
		skeleton["team"] = team
	}
	addWritingLanguage(skeleton, writingLanguage)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(skeleton); err != nil {
		return 1
	}
	return 0
}

func validateSuggestPlanOptions(opts suggestPlanOptions) error {
	if opts.Area != "" {
		if _, ok := ledger.AreaEnum[opts.Area]; !ok {
			return fmt.Errorf("invalid --area %q (allowed: %s)", opts.Area, allowedEnumValues(ledger.AreaEnum))
		}
	}
	if opts.Priority != "" {
		if _, ok := ledger.PriorityEnum[opts.Priority]; !ok {
			return fmt.Errorf("invalid --priority %q (allowed: %s)", opts.Priority, allowedEnumValues(ledger.PriorityEnum))
		}
	}
	return nil
}

func planStringDefault(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
